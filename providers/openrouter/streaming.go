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
		blocks, err := convertAnnotationsToWebSearchBlocks(
			parsed.WebSearch.Annotations,
			state.CurrentIndex,
		)
		if err != nil {
			return err
		}

		for _, block := range blocks {
			eventChan <- llmprovider.StreamEvent{Block: block}
		}

		state.CurrentIndex += len(blocks)
		state.WebSearchDone = true
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
				// Get or create accumulated tool call
				idx := len(toolCallsMap)
				if existingIdx, exists := findToolCallIndex(toolCallsMap, toolCallDelta.ID); exists {
					idx = existingIdx
				}

				acc, exists := toolCallsMap[idx]
				if !exists {
					// New tool call - emit block start
					acc = &accumulatedToolCall{}
					toolCallsMap[idx] = acc

					blockType := llmprovider.BlockTypeToolUse
					blockIndex := state.CurrentIndex + 1 + idx
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
					acc.Arguments.WriteString(toolCallDelta.Function.Arguments)

					// Emit input JSON delta
					blockIndex := state.CurrentIndex + 1 + idx
					eventChan <- llmprovider.StreamEvent{
						Delta: &llmprovider.BlockDelta{
							BlockIndex:     blockIndex,
							DeltaType:      llmprovider.DeltaTypeInputJSON,
							InputJSONDelta: &toolCallDelta.Function.Arguments,
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
	for idx := 0; idx < len(toolCallsMap); idx++ {
		acc, exists := toolCallsMap[idx]
		if !exists {
			continue
		}

		// Parse accumulated arguments
		input := make(map[string]interface{})
		if acc.Arguments.Len() > 0 {
			argStr := acc.Arguments.String()
			if err := json.Unmarshal([]byte(argStr), &input); err != nil {
				return fmt.Errorf("invalid tool call arguments at index %d: received malformed JSON %q - %w", idx, argStr, err)
			}
		}

		content := map[string]interface{}{
			"tool_use_id": acc.ID,
			"tool_name":   acc.Name,
			"input":       input,
		}

		eventChan <- llmprovider.StreamEvent{
			Block: &llmprovider.Block{
				BlockType: llmprovider.BlockTypeToolUse,
				Sequence:  state.CurrentIndex,
				Content:   content,
				Provider:  &providerIDStr,
			},
		}
		state.CurrentIndex++
	}

	// Emit final metadata
	eventChan <- llmprovider.StreamEvent{
		Metadata: &llmprovider.StreamMetadata{
			Model:      model,
			StopReason: stopReason,
			// Note: OpenRouter doesn't always include usage in streaming
			// InputTokens and OutputTokens may be 0
		},
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
