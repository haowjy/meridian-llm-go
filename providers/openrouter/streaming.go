package openrouter

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	llmprovider "github.com/haowjy/meridian-llm-go"
)

// DefaultStreamingIdleTimeout is the default timeout for SSE streaming.
// If no data is received for this duration, the stream is considered stalled.
const DefaultStreamingIdleTimeout = 2 * time.Minute

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

type streamDebug struct {
	enabled bool
	logger  *slog.Logger
}

func (d streamDebug) Debug(msg string, args ...any) {
	if d.enabled && d.logger != nil {
		d.logger.Debug(msg, args...)
	}
}

func (d streamDebug) Warn(msg string, args ...any) {
	if d.enabled && d.logger != nil {
		d.logger.Warn(msg, args...)
	}
}

// ===== Streaming Block Emitter (SOLID-compliant) =====

// emitStreamingBlocks emits stream events based on parsed delta and state transition.
// Uses EventEmitter for AG-UI events and emits complete blocks for persistence.
//
// NOTE: messageID should be a stable turn-level identifier, not generationID.
// The backend should provide this via IDFactory. Until then, we use generationID
// as a placeholder (marked with TODO).
func emitStreamingBlocks(
	parsed *ParsedDelta,
	transition BlockTransition,
	state *BlockState,
	thinkingContent *strings.Builder,
	textContent *strings.Builder,
	messageID string, // TODO: Should be turn-level messageID, not generationID
	dbg streamDebug,
	emitter *llmprovider.EventEmitter,
) error {
	providerIDStr := llmprovider.ProviderOpenRouter.String()

	// 1. Emit web search blocks (if present and not done)
	if parsed.WebSearch != nil && !state.WebSearchDone {
		dbg.Debug("openrouter web search annotations received", "current_index", state.CurrentIndex)
		blocks, err := convertAnnotationsToWebSearchBlocks(
			parsed.WebSearch.Annotations,
			state.CurrentIndex,
		)
		if err != nil {
			return err
		}

		for i, block := range blocks {
			dbg.Debug("openrouter web search block emitted", "index", i, "block_type", block.BlockType, "sequence", block.Sequence)
			emitter.Block(block)
		}

		oldIndex := state.CurrentIndex
		state.CurrentIndex += len(blocks)
		state.WebSearchDone = true
		dbg.Debug("openrouter web search blocks applied", "old_index", oldIndex, "new_index", state.CurrentIndex, "added", len(blocks))
	}

	// 2. Close previous block if transition says so (emit complete block for persistence)
	if transition.ClosePrevious && state.CurrentType == "thinking" {
		// Emit complete thinking block
		if thinkingContent.Len() > 0 {
			thinkingText := thinkingContent.String()
			if strings.TrimSpace(thinkingText) == "" {
				thinkingContent.Reset()
				// Continue processing (e.g., start next text block) without emitting an empty thinking block.
			} else {
				block := &llmprovider.Block{
					BlockType:   llmprovider.BlockTypeThinking,
					Sequence:    state.CurrentIndex,
					TextContent: &thinkingText,
					Provider:    &providerIDStr,
				}

				// Emit thinking end events and block
				emitter.ThinkingTextMessageEnd()
				emitter.ThinkingEnd()
				emitter.Block(block)
			}
		}
	}

	// 3. Start new block if transition says so
	if transition.StartNew {
		if transition.NewType == "thinking" {
			// Start thinking phase
			emitter.ThinkingStart(nil)
			emitter.ThinkingTextMessageStart()
		} else {
			// Start text message
			emitter.TextMessageStart(messageID, "assistant")
		}

		state.CurrentType = transition.NewType
		state.CurrentIndex = transition.NewIndex
	}

	// 4. Emit thinking delta and accumulate content
	if parsed.Thinking != nil && state.CurrentType == "thinking" {
		// Accumulate for complete block
		thinkingContent.WriteString(parsed.Thinking.Text)

		// Emit thinking content
		emitter.ThinkingTextMessageContent(parsed.Thinking.Text)
	}

	// 5. Emit text delta and accumulate content
	if parsed.Text != nil && state.CurrentType == "text" {
		// Accumulate for complete block
		textContent.WriteString(parsed.Text.Text)

		// Emit text content
		emitter.TextMessageContent(messageID, parsed.Text.Text)
	}

	return nil
}

// ===== End of streaming block emitter =====

// StreamResponse generates a streaming response from OpenRouter.
// Routes to Responses API or Chat Completions API based on provider configuration.
func (p *Provider) StreamResponse(ctx context.Context, req *llmprovider.GenerateRequest) (<-chan llmprovider.StreamEvent, error) {
	// Route to Responses API if enabled
	// Responses API provides better tool call streaming with item-based events
	if p.useResponsesAPI {
		return p.StreamResponsesAPI(ctx, req)
	}

	// Default: Use Chat Completions API (existing implementation)
	return p.streamChatCompletionsAPI(ctx, req)
}

// streamChatCompletionsAPI generates a streaming response using the Chat Completions API.
// This is the original implementation, now factored out for clarity.
func (p *Provider) streamChatCompletionsAPI(ctx context.Context, req *llmprovider.GenerateRequest) (<-chan llmprovider.StreamEvent, error) {
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
	openrouterReq, err := p.buildChatCompletionRequest(req)
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
		p.logger.Error("openrouter HTTP request failed", "model", req.Model, "error", err)
		return nil, fmt.Errorf("openrouter HTTP request failed: %w", err)
	}

	// Check for immediate errors
	if resp.StatusCode != http.StatusOK {
		defer func() { _ = resp.Body.Close() }()
		return nil, p.handleErrorResponse(resp, req.Model)
	}

	// Create streaming channel
	eventChan := make(chan llmprovider.StreamEvent, 10) // Buffered to prevent blocking

	// Start streaming goroutine
	go func() {
		defer close(eventChan)
		defer func() { _ = resp.Body.Close() }()

		if err := p.streamEvents(ctx, resp.Body, eventChan); err != nil {
			eventChan <- llmprovider.StreamEvent{Error: err}
		}
	}()

	return eventChan, nil
}

// streamEvents reads SSE events and emits library StreamEvents.
// Uses idle timeout detection: if no data is received for the configured streaming idle timeout,
// returns ErrStreamingIdleTimeout.
func (p *Provider) streamEvents(ctx context.Context, body io.ReadCloser, eventChan chan<- llmprovider.StreamEvent) error {
	dbg := streamDebug{enabled: p.debugStreamLogs, logger: p.logger}
	emitter := llmprovider.NewEventEmitter(eventChan)

	// Get the configured streaming idle timeout (or default)
	idleTimeout := p.getStreamingIdleTimeout()

	// Close body when context is cancelled to unblock scanner.
	// scanner.Scan() blocks waiting for data and only checks ctx.Done() after reading.
	// Closing the body forces Scan() to return with an error, enabling immediate cancellation.
	go func() {
		<-ctx.Done()
		body.Close()
	}()

	// Start line reader goroutine
	lineChan := make(chan string, 10) // Buffered to avoid blocking scanner
	scanErrChan := make(chan error, 1)

	go func() {
		scanner := bufio.NewScanner(body)
		for scanner.Scan() {
			select {
			case lineChan <- scanner.Text():
			case <-ctx.Done():
				return
			}
		}
		if err := scanner.Err(); err != nil {
			scanErrChan <- err
		}
		close(lineChan)
	}()

	// Initialize block state (SOLID-compliant)
	state := BlockState{CurrentIndex: 0}

	// Accumulators for complete block content (needed for persistence)
	var thinkingContent strings.Builder // Accumulate thinking text for complete block
	var textContent strings.Builder     // Accumulate text content for complete block

	// Keep these for metadata and tool calls
	toolCallsMap := make(map[int]*accumulatedToolCall) // index -> accumulated tool call
	var model string
	var stopReason string
	var usage *Usage        // Token usage (captured from last chunk)
	var generationID string // Generation ID for querying stats after cancel

	// Idle timeout timer - reset on each line received
	idleTimer := time.NewTimer(idleTimeout)
	defer idleTimer.Stop()

	// resetIdleTimer safely resets the idle timer
	resetIdleTimer := func() {
		if !idleTimer.Stop() {
			select {
			case <-idleTimer.C:
			default:
			}
		}
		idleTimer.Reset(idleTimeout)
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case <-idleTimer.C:
			// No data received for too long - provider stalled
			p.logger.Warn("streaming idle timeout", "timeout", idleTimeout, "model", model)
			return &llmprovider.ProviderError{
				Code:      llmprovider.ErrorCodeStreamingIdleTimeout,
				Provider:  llmprovider.ProviderOpenRouter.String(),
				Message:   fmt.Sprintf("no data received for %v during streaming", idleTimeout),
				Retryable: true,
				Err:       llmprovider.ErrStreamingIdleTimeout,
			}

		case err := <-scanErrChan:
			p.logger.Error("error reading stream", "model", model, "error", err)
			return fmt.Errorf("error reading stream: %w", err)

		case line, ok := <-lineChan:
			if !ok {
				// Channel closed - scanner finished normally
				// Fall through to finalization below
				goto finalize
			}

			// Reset idle timer on any line (including empty lines, keep-alives)
			resetIdleTimer()

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
				goto finalize
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
					p.logger.Error("openrouter streaming error", "model", model, "error", errResp.Error.Message)
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

			// Capture generation ID (available in all chunks, capture from first)
			if generationID == "" && chunk.ID != "" {
				generationID = chunk.ID

				// Emit non-terminal event immediately for early persistence
				// This allows background enrichment even if stream is cancelled mid-way
				eventChan <- llmprovider.StreamEvent{
					GenerationIDDiscovered: &llmprovider.GenerationIDEvent{
						GenerationID: chunk.ID,
						Model:        model,
						Provider:     "openrouter",
					},
				}
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
			// TODO: messageID should be turn-level, not generationID. Backend should provide this.
			if err := emitStreamingBlocks(parsed, transition, &state, &thinkingContent, &textContent, generationID, dbg, emitter); err != nil {
				return err
			}

			// Process tool calls delta (keep existing logic - tool calls need accumulation)
			if len(delta.ToolCalls) > 0 {
				for _, toolCallDelta := range delta.ToolCalls {
					var indexFromOR string
					if toolCallDelta.Index != nil {
						indexFromOR = fmt.Sprintf("%d", *toolCallDelta.Index)
					} else {
						indexFromOR = "nil"
					}
					dbg.Debug("openrouter tool call delta",
						"openrouter_index", indexFromOR,
						"id", toolCallDelta.ID,
						"name", toolCallDelta.Function.Name,
						"args", toolCallDelta.Function.Arguments,
					)

					// Determine the map index to use (priority order):
					// 1. Use Index from OpenRouter if present (most reliable)
					// 2. Find existing entry by ID
					// 3. Create new entry at next available index
					var idx int
					if toolCallDelta.Index != nil {
						// Use actual index from OpenRouter response
						idx = *toolCallDelta.Index
						dbg.Debug("openrouter tool call using provider index", "index", idx)
					} else if existingIdx, exists := findToolCallIndex(toolCallsMap, toolCallDelta.ID); exists {
						// Find existing by ID
						idx = existingIdx
						dbg.Debug("openrouter tool call matched existing id", "id", toolCallDelta.ID, "map_index", idx)
					} else {
						// Fallback: create new entry
						idx = len(toolCallsMap)
						dbg.Debug("openrouter tool call created new entry (fallback)", "id", toolCallDelta.ID, "map_index", idx)
					}

					acc, exists := toolCallsMap[idx]
					if !exists {
						// New tool call - emit block start
						acc = &accumulatedToolCall{}
						toolCallsMap[idx] = acc

						blockIndex := state.CurrentIndex + 1 + idx
						dbg.Debug("openrouter tool call start",
							"map_index", idx,
							"block_index", blockIndex,
							"current_index", state.CurrentIndex,
							"id", toolCallDelta.ID,
							"name", toolCallDelta.Function.Name,
						)
						// AG-UI: ToolCallStart
						// TODO: parentMessageID should be turn-level messageID, not generationID
						emitter.ToolCallStart(toolCallDelta.ID, toolCallDelta.Function.Name, &generationID)
						_ = blockIndex // Suppress unused variable warning
					}

					// Accumulate data
					if toolCallDelta.ID != "" {
						acc.ID = toolCallDelta.ID
					}
					if toolCallDelta.Function.Name != "" {
						acc.Name = toolCallDelta.Function.Name
					}
					if toolCallDelta.Function.Arguments != "" {
						now := time.Now()
						if !acc.seenAnyArgs {
							acc.seenAnyArgs = true
							acc.firstArgsAt = now
						}
						acc.lastAnyArgsAt = now

						// Treat whitespace-only deltas *outside* JSON strings as non-progress.
						// This prevents infinite TOOL_CALL_ARGS streams where providers/models
						// keep sending whitespace after an incomplete JSON payload.
						meaningful := acc.argsProgress.Apply(toolCallDelta.Function.Arguments)
						if meaningful {
							prevLength := acc.Arguments.Len()
							acc.Arguments.WriteString(toolCallDelta.Function.Arguments)
							newLength := acc.Arguments.Len()

							dbg.Debug("openrouter tool call args accumulated",
								"id", acc.ID,
								"map_index", idx,
								"prev_len", prevLength,
								"chunk_len", len(toolCallDelta.Function.Arguments),
								"new_total", newLength,
							)

							// Emit input JSON delta with AG-UI ToolCallArgs
							emitter.ToolCallArgs(acc.ID, toolCallDelta.Function.Arguments)

							acc.seenMeaningfulArgs = true
							acc.lastMeaningfulArgsAt = now
						} else {
							dbg.Debug("openrouter tool call args ignored (non-meaningful whitespace outside string)",
								"id", acc.ID,
								"map_index", idx,
								"chunk_len", len(toolCallDelta.Function.Arguments),
							)
						}
					}
				}
			}

			// Guardrail: if tool-call args stop making meaningful progress for too long,
			// treat this as a stalled stream (even if bytes continue arriving).
			if stalledAcc := findStalledToolArgs(toolCallsMap, time.Now(), idleTimeout); stalledAcc != nil {
				dbg.Warn("openrouter tool call args stalled",
					"id", stalledAcc.ID,
					"name", stalledAcc.Name,
					"timeout", idleTimeout,
				)
				return &llmprovider.ProviderError{
					Code:      llmprovider.ErrorCodeStreamingIdleTimeout,
					Provider:  llmprovider.ProviderOpenRouter.String(),
					Message:   fmt.Sprintf("no meaningful tool call args progress for %v during streaming", idleTimeout),
					Retryable: true,
					Err:       llmprovider.ErrStreamingIdleTimeout,
				}
			}

			// Check for finish
			if choice.FinishReason != nil {
				stopReason = mapFinishReason(*choice.FinishReason)
			}
		}
	}

finalize:
	// Web search blocks are already emitted during streaming
	// Emit complete blocks for thinking/text (for persistence) before tool calls

	providerIDStr := llmprovider.ProviderOpenRouter.String()

	// Emit complete thinking block if it was started (for persistence)
	if state.CurrentType == "thinking" && thinkingContent.Len() > 0 {
		thinkingText := thinkingContent.String()
		if strings.TrimSpace(thinkingText) != "" {
			block := &llmprovider.Block{
				BlockType:   llmprovider.BlockTypeThinking,
				Sequence:    state.CurrentIndex,
				TextContent: &thinkingText,
				Provider:    &providerIDStr,
			}

			// No provider_data for OpenRouter reasoning
			// Reconstruction from TextContent via convertThinkingToReasoningDetails()

			// AG-UI: ThinkingEnd events and block
			emitter.ThinkingTextMessageEnd()
			emitter.ThinkingEnd()
			emitter.Block(block)
			state.CurrentIndex++
		}
	}

	// Emit complete text block if it was started (for persistence)
	if state.CurrentType == "text" && textContent.Len() > 0 {
		text := textContent.String()
		// AG-UI: TextMessageEnd for text
		// TODO: messageID should be turn-level, not generationID
		emitter.TextMessageEnd(generationID)
		emitter.Block(&llmprovider.Block{
			BlockType:   llmprovider.BlockTypeText,
			Sequence:    state.CurrentIndex,
			TextContent: &text,
			Provider:    &providerIDStr,
		})
		state.CurrentIndex++
	}

	// Tool call blocks (emit in order)
	dbg.Debug("openrouter finalizing tool calls", "total", toolCallsMap, "current_index", state.CurrentIndex)

	for idx := 0; idx < len(toolCallsMap); idx++ {
		acc, exists := toolCallsMap[idx]
		if !exists {
			dbg.Warn("openrouter toolCallsMap gap (unexpected)", "index", idx)
			continue
		}

		dbg.Debug("openrouter tool call finalize",
			"index", idx,
			"id", acc.ID,
			"name", acc.Name,
			"args_len", acc.Arguments.Len(),
		)
		argStr := acc.Arguments.String()

		// Parse accumulated arguments
		input := make(map[string]interface{})
		if acc.Arguments.Len() > 0 {
			if err := json.Unmarshal([]byte(argStr), &input); err != nil {
				// Instead of aborting, emit recovery blocks so LLM can see error and retry
				dbg.Warn("openrouter malformed tool JSON, emitting recovery blocks",
					"index", idx,
					"id", acc.ID,
					"name", acc.Name,
					"args_len", acc.Arguments.Len(),
					"error", err,
				)

				recovery, nextSeq := llmprovider.NewMalformedToolRecovery(
					acc.ID, acc.Name, argStr, err,
					state.CurrentIndex, &providerIDStr,
				)

				eventChan <- llmprovider.StreamEvent{Block: recovery.ToolUseBlock}
				eventChan <- llmprovider.StreamEvent{Block: recovery.ToolResultBlock}
				state.CurrentIndex = nextSeq
				continue // Continue to next tool instead of aborting
			}
			dbg.Debug("openrouter tool call args parsed successfully", "index", idx, "id", acc.ID, "name", acc.Name)
		}

		content := map[string]interface{}{
			"tool_use_id": acc.ID,
			"tool_name":   acc.Name,
			"input":       input,
		}

		// All OpenRouter tools are backend-side (executed by our backend)
		executionSide := llmprovider.ExecutionSideServer

		// AG-UI: ToolCallEnd + Block
		emitter.ToolCallEnd(acc.ID)
		emitter.Block(&llmprovider.Block{
			BlockType:     llmprovider.BlockTypeToolUse,
			Sequence:      state.CurrentIndex,
			Content:       content,
			ExecutionSide: &executionSide,
			Provider:      &providerIDStr,
		})
		state.CurrentIndex++
	}

	// Emit final metadata
	metadata := &llmprovider.StreamMetadata{
		Model:        model,
		StopReason:   stopReason,
		GenerationID: generationID,
	}

	// Extract token usage if available (typically in last chunk)
	if usage != nil {
		metadata.InputTokens = usage.PromptTokens
		metadata.OutputTokens = usage.CompletionTokens
	}
	// Note: If usage is nil, InputTokens and OutputTokens default to 0

	emitter.Metadata(metadata)

	return nil
}

// accumulatedToolCall holds state for accumulating a tool call during streaming.
type accumulatedToolCall struct {
	ID        string
	Name      string
	Arguments strings.Builder

	argsProgress         toolArgsProgress
	seenAnyArgs          bool
	firstArgsAt          time.Time
	lastAnyArgsAt        time.Time
	seenMeaningfulArgs   bool
	lastMeaningfulArgsAt time.Time
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

func findStalledToolArgs(toolCallsMap map[int]*accumulatedToolCall, now time.Time, timeout time.Duration) *accumulatedToolCall {
	for _, acc := range toolCallsMap {
		if acc == nil || !acc.seenAnyArgs || acc.firstArgsAt.IsZero() {
			continue
		}

		// If we've never seen meaningful args, treat the stream as stalled if the provider
		// has been sending args deltas (even whitespace) for too long without making progress.
		if !acc.seenMeaningfulArgs || acc.lastMeaningfulArgsAt.IsZero() {
			if now.Sub(acc.firstArgsAt) >= timeout {
				return acc
			}
			continue
		}

		if now.Sub(acc.lastMeaningfulArgsAt) >= timeout {
			return acc
		}
	}
	return nil
}
