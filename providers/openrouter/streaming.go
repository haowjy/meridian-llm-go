package openrouter

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/haowjy/meridian-llm-go"
)

// ChatCompletionChunk represents a streaming chunk from OpenRouter.
type ChatCompletionChunk struct {
	ID      string        `json:"id"`
	Object  string        `json:"object"` // "chat.completion.chunk"
	Created int64         `json:"created"`
	Model   string        `json:"model"`
	Choices []ChunkChoice `json:"choices"`
	Usage   *Usage        `json:"usage,omitempty"` // Token usage (only in last chunk)
}

// ChunkChoice represents a choice in a streaming chunk.
type ChunkChoice struct {
	Index        int     `json:"index"`
	Delta        Delta   `json:"delta"`
	FinishReason *string `json:"finish_reason"`
}

// Delta represents incremental updates in a chunk.
type Delta struct {
	Role             *string           `json:"role,omitempty"`
	Content          *string           `json:"content,omitempty"`
	ToolCalls        []ToolCall        `json:"tool_calls,omitempty"`
	Reasoning        *string           `json:"reasoning,omitempty"`         // Simple reasoning field (often just placeholder)
	ReasoningDetails []ReasoningDetail `json:"reasoning_details,omitempty"` // Actual thinking content from models like kimi-k2-thinking
	Annotations      []Annotation      `json:"annotations,omitempty"`       // Web search results from :online models
}

// ===== Streaming Block Emitter (SOLID-compliant) =====

// emitStreamingBlocks emits stream events based on parsed delta and state transition.
// Emits both deltas (for real-time UI) and complete blocks (for persistence).
func emitStreamingBlocks(
	parsed *ParsedDelta,
	transition BlockTransition,
	state *BlockState,
	thinkingContent *strings.Builder,
	textContent *strings.Builder,
	eventChan chan<- llmprovider.StreamEvent,
) error {
	providerIDStr := llmprovider.ProviderOpenRouter.String()

	// 1. Emit web search blocks (if present and not done)
	if parsed.WebSearch != nil && !state.WebSearchDone {
		fmt.Printf("[DEBUG] processing web search annotations: state.CurrentIndex=%d\n", state.CurrentIndex)
		blocks, err := convertAnnotationsToWebSearchBlocks(
			parsed.WebSearch.Annotations,
			state.CurrentIndex,
		)
		if err != nil {
			return err
		}

		fmt.Printf("[DEBUG] emitting %d web search blocks\n", len(blocks))
		for i, block := range blocks {
			fmt.Printf("[DEBUG]   web search block %d: type=%s, sequence=%d\n", i, block.BlockType, block.Sequence)
			eventChan <- llmprovider.StreamEvent{Block: block}
		}

		oldIndex := state.CurrentIndex
		state.CurrentIndex += len(blocks)
		state.WebSearchDone = true
		fmt.Printf("[DEBUG] updated state.CurrentIndex: %d -> %d (added %d web search blocks)\n",
			oldIndex, state.CurrentIndex, len(blocks))
	}

	// 2. Close previous block if transition says so (emit complete block for persistence)
	if transition.ClosePrevious && state.CurrentType == "thinking" {
		// Emit complete thinking block
		if thinkingContent.Len() > 0 {
			thinkingText := thinkingContent.String()
			eventChan <- llmprovider.StreamEvent{
				Block: &llmprovider.Block{
					BlockType:   llmprovider.BlockTypeThinking,
					Sequence:    state.CurrentIndex,
					TextContent: &thinkingText,
					Provider:    &providerIDStr,
				},
			}
		}
	}

	// 3. Start new block if transition says so
	if transition.StartNew {
		blockType := llmprovider.BlockTypeText
		if transition.NewType == "thinking" {
			blockType = llmprovider.BlockTypeThinking
		}

		eventChan <- llmprovider.StreamEvent{
			Delta: &llmprovider.BlockDelta{
				BlockIndex: transition.NewIndex,
				BlockType:  &blockType,
				DeltaType:  llmprovider.DeltaTypeText,
			},
		}

		state.CurrentType = transition.NewType
		state.CurrentIndex = transition.NewIndex
	}

	// 4. Emit thinking delta and accumulate content
	if parsed.Thinking != nil && state.CurrentType == "thinking" {
		// Accumulate for complete block
		thinkingContent.WriteString(parsed.Thinking.Text)

		// Emit delta for real-time UI
		eventChan <- llmprovider.StreamEvent{
			Delta: &llmprovider.BlockDelta{
				BlockIndex: state.CurrentIndex,
				DeltaType:  llmprovider.DeltaTypeText,
				TextDelta:  &parsed.Thinking.Text,
			},
		}
	}

	// 5. Emit text delta and accumulate content
	if parsed.Text != nil && state.CurrentType == "text" {
		// Accumulate for complete block
		textContent.WriteString(parsed.Text.Text)

		// Emit delta for real-time UI
		eventChan <- llmprovider.StreamEvent{
			Delta: &llmprovider.BlockDelta{
				BlockIndex: state.CurrentIndex,
				DeltaType:  llmprovider.DeltaTypeText,
				TextDelta:  &parsed.Text.Text,
			},
		}
	}

	return nil
}

// ===== End of streaming block emitter =====

// StreamResponse generates a streaming response from OpenRouter.
func (p *Provider) StreamResponse(ctx context.Context, req *llmprovider.GenerateRequest) (<-chan llmprovider.StreamEvent, error) {
	// Validate model
	if !p.SupportsModel(req.Model) {
		return nil, &llmprovider.ModelError{
			Model:    req.Model,
			Provider: p.Name().String(),
			Reason:   "model not supported by OpenRouter (must be in 'provider/model' format)",
			Err:      llmprovider.ErrInvalidModel,
		}
	}

	// Validate web_search requires :online suffix
	if err := p.validateWebSearchRequirements(req); err != nil {
		return nil, err
	}

	// Build OpenRouter API request (shared logic)
	openrouterReq, err := buildChatCompletionRequest(req)
	if err != nil {
		return nil, err
	}

	// Enable streaming
	openrouterReq.Stream = true

	// Make HTTP request
	httpReq, err := p.buildHTTPRequest(ctx, openrouterReq)
	if err != nil {
		return nil, err
	}

	// Set Accept header for SSE
	httpReq.Header.Set("Accept", "text/event-stream")

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("openrouter HTTP request failed: %w", err)
	}

	// Check for immediate errors
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		return nil, p.handleErrorResponse(resp)
	}

	// Create streaming channel
	eventChan := make(chan llmprovider.StreamEvent, 10) // Buffered to prevent blocking

	// Start streaming goroutine
	go func() {
		defer close(eventChan)
		defer resp.Body.Close()

		if err := p.streamEvents(ctx, resp.Body, eventChan); err != nil {
			eventChan <- llmprovider.StreamEvent{Error: err}
		}
	}()

	return eventChan, nil
}

// streamEvents reads SSE events and emits library StreamEvents.
func (p *Provider) streamEvents(ctx context.Context, body io.ReadCloser, eventChan chan<- llmprovider.StreamEvent) error {
	scanner := bufio.NewScanner(body)

	// Initialize block state (SOLID-compliant)
	state := BlockState{CurrentIndex: 0}

	// Accumulators for complete block content (needed for persistence)
	var thinkingContent strings.Builder // Accumulate thinking text for complete block
	var textContent strings.Builder     // Accumulate text content for complete block

	// Keep these for metadata and tool calls
	toolCallsMap := make(map[int]*accumulatedToolCall) // index -> accumulated tool call
	var model string
	var stopReason string
	var usage *Usage // Token usage (captured from last chunk)

	for scanner.Scan() {
		line := scanner.Text()

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, ":") {
			continue
		}

		// Parse SSE data line
		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := strings.TrimPrefix(line, "data: ")

		// Check for termination
		if data == "[DONE]" {
			break
		}

		// Parse chunk
		var chunk ChatCompletionChunk
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			// Check for error response
			var errResp struct {
				Error struct {
					Code    int    `json:"code"`
					Message string `json:"message"`
				} `json:"error"`
			}
			if json.Unmarshal([]byte(data), &errResp) == nil && errResp.Error.Message != "" {
				return fmt.Errorf("openrouter streaming error: %s", errResp.Error.Message)
			}
			// Ignore unparseable chunks (might be keep-alive or other messages)
			continue
		}

		if len(chunk.Choices) == 0 {
			continue
		}

		choice := chunk.Choices[0]
		delta := choice.Delta

		// Update model
		if chunk.Model != "" {
			model = chunk.Model
		}

		// Capture usage (typically only in last chunk)
		if chunk.Usage != nil {
			usage = chunk.Usage
		}

		// Parse delta using SOLID-compliant functions
		parsed := parseDelta(
			delta.Annotations,
			delta.ReasoningDetails,
			delta.Content,
		)

		// Determine block transitions
		transition := determineTransition(state, parsed)

		// Emit blocks/deltas based on parsed data and transition
		// Pass accumulators so complete blocks can be built for persistence
		if err := emitStreamingBlocks(parsed, transition, &state, &thinkingContent, &textContent, eventChan); err != nil {
			return err
		}

		// Process tool calls delta (keep existing logic - tool calls need accumulation)
		if len(delta.ToolCalls) > 0 {
			for _, toolCallDelta := range delta.ToolCalls {
				// DEBUG: Print each tool call delta with actual index from OpenRouter
				var indexFromOR string
				if toolCallDelta.Index != nil {
					indexFromOR = fmt.Sprintf("%d", *toolCallDelta.Index)
				} else {
					indexFromOR = "nil"
				}
				fmt.Printf("[DEBUG] processing tool call delta: openrouter_index=%s, id=%q, name=%q, args_len=%d, args_preview=%q\n",
					indexFromOR, toolCallDelta.ID, toolCallDelta.Function.Name,
					len(toolCallDelta.Function.Arguments), truncateString(toolCallDelta.Function.Arguments, 50))

				// Determine the map index to use (priority order):
				// 1. Use Index from OpenRouter if present (most reliable)
				// 2. Find existing entry by ID
				// 3. Create new entry at next available index
				var idx int
				if toolCallDelta.Index != nil {
					// Use actual index from OpenRouter response
					idx = *toolCallDelta.Index
					fmt.Printf("[DEBUG] using OpenRouter index: %d\n", idx)
				} else if existingIdx, exists := findToolCallIndex(toolCallsMap, toolCallDelta.ID); exists {
					// Find existing by ID
					idx = existingIdx
					fmt.Printf("[DEBUG] found existing tool call by ID: id=%q, map_index=%d\n", toolCallDelta.ID, idx)
				} else {
					// Fallback: create new entry
					idx = len(toolCallsMap)
					fmt.Printf("[DEBUG] creating new tool call entry (fallback): id=%q, map_index=%d\n", toolCallDelta.ID, idx)
				}

				acc, exists := toolCallsMap[idx]
				if !exists {
					// New tool call - emit block start
					acc = &accumulatedToolCall{}
					toolCallsMap[idx] = acc

					blockType := llmprovider.BlockTypeToolUse
					blockIndex := state.CurrentIndex + 1 + idx
					fmt.Printf("[DEBUG] emitting tool call START: map_index=%d, blockIndex=%d, state.CurrentIndex=%d, id=%q, name=%q\n",
						idx, blockIndex, state.CurrentIndex, toolCallDelta.ID, toolCallDelta.Function.Name)
					eventChan <- llmprovider.StreamEvent{
						Delta: &llmprovider.BlockDelta{
							BlockIndex:   blockIndex,
							BlockType:    &blockType,
							DeltaType:    llmprovider.DeltaTypeToolCallStart,
							ToolCallID:   &toolCallDelta.ID,
							ToolCallName: &toolCallDelta.Function.Name,
						},
					}
				}

				// Accumulate data
				if toolCallDelta.ID != "" {
					acc.ID = toolCallDelta.ID
				}
				if toolCallDelta.Function.Name != "" {
					acc.Name = toolCallDelta.Function.Name
				}
				if toolCallDelta.Function.Arguments != "" {
					prevLength := acc.Arguments.Len()
					acc.Arguments.WriteString(toolCallDelta.Function.Arguments)
					newLength := acc.Arguments.Len()

					// DEBUG: Print Arguments accumulation
					fmt.Printf("[DEBUG] accumulated tool call arguments: id=%q, map_index=%d, prev_len=%d, chunk_len=%d, new_total=%d, preview=%q\n",
						acc.ID, idx, prevLength, len(toolCallDelta.Function.Arguments), newLength, truncateString(acc.Arguments.String(), 100))

					// Emit input JSON delta
					blockIndex := state.CurrentIndex + 1 + idx
					eventChan <- llmprovider.StreamEvent{
						Delta: &llmprovider.BlockDelta{
							BlockIndex:  blockIndex,
							DeltaType:   llmprovider.DeltaTypeJSON,
							JSONDelta:   &toolCallDelta.Function.Arguments,
						},
					}
				}
			}
		}

		// Check for finish
		if choice.FinishReason != nil {
			stopReason = mapFinishReason(*choice.FinishReason)
		}
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("error reading stream: %w", err)
	}

	// Web search blocks are already emitted during streaming
	// Emit complete blocks for thinking/text (for persistence) before tool calls

	providerIDStr := llmprovider.ProviderOpenRouter.String()

	// Emit complete thinking block if it was started (for persistence)
	if state.CurrentType == "thinking" && thinkingContent.Len() > 0 {
		thinkingText := thinkingContent.String()
		eventChan <- llmprovider.StreamEvent{
			Block: &llmprovider.Block{
				BlockType:   llmprovider.BlockTypeThinking,
				Sequence:    state.CurrentIndex,
				TextContent: &thinkingText,
				Provider:    &providerIDStr,
			},
		}
		state.CurrentIndex++
	}

	// Emit complete text block if it was started (for persistence)
	if state.CurrentType == "text" && textContent.Len() > 0 {
		text := textContent.String()
		eventChan <- llmprovider.StreamEvent{
			Block: &llmprovider.Block{
				BlockType:   llmprovider.BlockTypeText,
				Sequence:    state.CurrentIndex,
				TextContent: &text,
				Provider:    &providerIDStr,
			},
		}
		state.CurrentIndex++
	}

	// Tool call blocks (emit in order)
	// DEBUG: Print toolCallsMap state before finalization
	fmt.Printf("[DEBUG] finalizing tool calls: total=%d, state.CurrentIndex=%d\n", len(toolCallsMap), state.CurrentIndex)

	// DEBUG: Dump entire toolCallsMap to see what indices exist
	fmt.Printf("[DEBUG] toolCallsMap contents:\n")
	for mapIdx, mapAcc := range toolCallsMap {
		fmt.Printf("[DEBUG]   index=%d: id=%q, name=%q, args_len=%d\n",
			mapIdx, mapAcc.ID, mapAcc.Name, mapAcc.Arguments.Len())
	}

	for idx := 0; idx < len(toolCallsMap); idx++ {
		acc, exists := toolCallsMap[idx]
		if !exists {
			fmt.Printf("[DEBUG] WARNING: gap in toolCallsMap at index %d (this should not happen!)\n", idx)
			continue
		}

		// DEBUG: Print accumulated tool call before JSON parsing
		argStr := acc.Arguments.String()
		fmt.Printf("[DEBUG] parsing accumulated tool call arguments: index=%d, id=%q, name=%q, args_len=%d, args_full=%q\n",
			idx, acc.ID, acc.Name, acc.Arguments.Len(), argStr)

		// Parse accumulated arguments
		input := make(map[string]interface{})
		if acc.Arguments.Len() > 0 {
			if err := json.Unmarshal([]byte(argStr), &input); err != nil {
				fmt.Printf("[ERROR] failed to parse tool call arguments: index=%d, id=%q, name=%q, malformed_json=%q, error=%v\n",
					idx, acc.ID, acc.Name, argStr, err)
				return fmt.Errorf("invalid tool call arguments at index %d: received malformed JSON %q - %w", idx, argStr, err)
			}
			fmt.Printf("[DEBUG] successfully parsed tool call arguments: index=%d, id=%q, name=%q, parsed_input=%v\n",
				idx, acc.ID, acc.Name, input)
		}

		content := map[string]interface{}{
			"tool_use_id": acc.ID,
			"tool_name":   acc.Name,
			"input":       input,
		}

		// All OpenRouter tools are client-side (executed by backend)
		executionSide := llmprovider.ExecutionSideClient

		eventChan <- llmprovider.StreamEvent{
			Block: &llmprovider.Block{
				BlockType:     llmprovider.BlockTypeToolUse,
				Sequence:      state.CurrentIndex,
				Content:       content,
				ExecutionSide: &executionSide,
				Provider:      &providerIDStr,
			},
		}
		state.CurrentIndex++
	}

	// Emit final metadata
	metadata := &llmprovider.StreamMetadata{
		Model:      model,
		StopReason: stopReason,
	}

	// Extract token usage if available (typically in last chunk)
	if usage != nil {
		metadata.InputTokens = usage.PromptTokens
		metadata.OutputTokens = usage.CompletionTokens
	}
	// Note: If usage is nil, InputTokens and OutputTokens default to 0

	eventChan <- llmprovider.StreamEvent{
		Metadata: metadata,
	}

	return nil
}

// accumulatedToolCall holds state for accumulating a tool call during streaming.
type accumulatedToolCall struct {
	ID        string
	Name      string
	Arguments strings.Builder
}

// findToolCallIndex finds the index of a tool call by ID in the accumulator map.
func findToolCallIndex(toolCallsMap map[int]*accumulatedToolCall, id string) (int, bool) {
	if id == "" {
		return 0, false
	}
	for idx, acc := range toolCallsMap {
		if acc.ID == id {
			return idx, true
		}
	}
	return 0, false
}

// truncateString truncates a string to maxLen characters, adding "..." if truncated.
// Used for debug logging to prevent excessive log output.
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return "..."
	}
	return s[:maxLen-3] + "..."
}
