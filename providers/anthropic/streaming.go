package anthropic

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/anthropics/anthropic-sdk-go"

	llmprovider "github.com/haowjy/meridian-llm-go"
)

// streamDebug wraps debug logging for streaming operations.
// Only logs when enabled via WithDebugStreamLogs(true).
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

// DefaultStreamingIdleTimeout is the default timeout for SDK streaming.
// If no event is received for this duration, the stream is considered stalled.
const DefaultStreamingIdleTimeout = 2 * time.Minute

// StreamResponse generates a streaming response from Claude.
// Returns a channel that emits StreamEvent as deltas arrive from the API.
func (p *Provider) StreamResponse(ctx context.Context, req *llmprovider.GenerateRequest) (<-chan llmprovider.StreamEvent, error) {
	// Validate model
	if !p.SupportsModel(req.Model) {
		return nil, &llmprovider.ModelError{
			Model:    req.Model,
			Provider: p.Name().String(),
			Reason:   "model not supported by Anthropic (must start with 'claude-')",
			Err:      llmprovider.ErrInvalidModel,
		}
	}

	// Extract tools from request params (pass to event transformer to preserve ExecutionSide)
	var tools []llmprovider.Tool
	if req.Params != nil {
		tools = req.Params.Tools
	}

	// Build Anthropic API parameters (shared logic with GenerateResponse)
	apiParams, err := p.buildMessageParams(req)
	if err != nil {
		return nil, err
	}

	// Create streaming channel
	eventChan := make(chan llmprovider.StreamEvent, 10) // Buffered to prevent blocking

	// Start streaming goroutine with idle timeout detection
	go func() {
		defer close(eventChan)

		// Create debug logger instance
		dbg := streamDebug{enabled: p.debugStreamLogs, logger: p.logger}

		// Get the configured streaming idle timeout (or default)
		idleTimeout := p.getStreamingIdleTimeout()

		// Call Anthropic streaming API
		stream := p.client.Messages.NewStreaming(ctx, apiParams)

		// Accumulator for final message metadata
		message := anthropic.Message{}

		// Channel for receiving events from the SDK stream reader
		type sdkEvent struct {
			event anthropic.MessageStreamEventUnion
			ok    bool
		}
		sdkEventChan := make(chan sdkEvent, 1)
		sdkErrChan := make(chan error, 1)

		// Start SDK stream reader goroutine
		go func() {
			defer close(sdkEventChan)
			for stream.Next() {
				select {
				case sdkEventChan <- sdkEvent{event: stream.Current(), ok: true}:
				case <-ctx.Done():
					return
				}
			}
			if err := stream.Err(); err != nil {
				sdkErrChan <- err
			}
		}()

		// Idle timeout timer - reset on each event received
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
				eventChan <- llmprovider.StreamEvent{Error: ctx.Err()}
				return

			case <-idleTimer.C:
				// No data received for too long - provider stalled
				p.logger.Warn("streaming idle timeout exceeded", "timeout", idleTimeout)
				eventChan <- llmprovider.StreamEvent{
					Error: &llmprovider.ProviderError{
						Code:      llmprovider.ErrorCodeStreamingIdleTimeout,
						Provider:  llmprovider.ProviderAnthropic.String(),
						Message:   fmt.Sprintf("no data received for %v during streaming", idleTimeout),
						Retryable: true,
						Err:       llmprovider.ErrStreamingIdleTimeout,
					},
				}
				return

			case err := <-sdkErrChan:
				p.logger.Error("anthropic streaming error", "error", err)
				eventChan <- llmprovider.StreamEvent{
					Error: fmt.Errorf("anthropic streaming error: %w", err),
				}
				return

			case evt, ok := <-sdkEventChan:
				if !ok {
					// Channel closed - stream finished normally, send metadata
					goto sendMetadata
				}

				// Reset idle timer on any event
				resetIdleTimer()

				// Accumulate event into final message
				if err := message.Accumulate(evt.event); err != nil {
					p.logger.Error("failed to accumulate message", "error", err)
					eventChan <- llmprovider.StreamEvent{
						Error: fmt.Errorf("failed to accumulate message: %w", err),
					}
					return
				}

				// Transform Anthropic event to library events via EventEmitter
				// Pass accumulated message so we can emit complete blocks on ContentBlockStop
				emitter := llmprovider.NewEventEmitter(eventChan)
				p.emitAnthropicStreamEvents(evt.event, &message, tools, emitter, dbg)
			}
		}

	sendMetadata:
		// Send final message metadata
		dbg.Debug("stream complete, sending metadata",
			"input_tokens", message.Usage.InputTokens,
			"output_tokens", message.Usage.OutputTokens,
			"stop_reason", message.StopReason,
		)
		metadata := &llmprovider.StreamMetadata{
			Model:        string(message.Model),
			InputTokens:  int(message.Usage.InputTokens),
			OutputTokens: int(message.Usage.OutputTokens),
			StopReason:   string(message.StopReason),
		}

		// Build response metadata with provider-specific data
		responseMetadata := make(map[string]interface{})
		if message.StopSequence != "" {
			responseMetadata["stop_sequence"] = message.StopSequence
		}
		if message.Usage.CacheCreationInputTokens > 0 {
			responseMetadata["cache_creation_input_tokens"] = int(message.Usage.CacheCreationInputTokens)
		}
		if message.Usage.CacheReadInputTokens > 0 {
			responseMetadata["cache_read_input_tokens"] = int(message.Usage.CacheReadInputTokens)
		}
		metadata.ResponseMetadata = responseMetadata

		eventChan <- llmprovider.StreamEvent{
			Metadata: metadata,
		}
	}()

	return eventChan, nil
}

// emitAnthropicStreamEvents converts Anthropic streaming events and emits them via EventEmitter.
//
// The message parameter is the SDK's accumulated message, which contains complete ContentBlocks
// as they finish streaming. We use this to emit complete, normalized blocks when ContentBlockStop arrives.
//
// The tools parameter contains the original tool definitions from the request, used to preserve
// ExecutionSide when converting tool_use blocks.
//
// The emitter is used to emit AG-UI events to the stream channel.
//
// The dbg parameter provides debug logging (metadata-only, no sensitive content).
//
// Anthropic stream events include:
// - MessageStart: Contains message metadata (id, model, role)
// - ContentBlockStart: New content block started (index, type)
// - ContentBlockDelta: Incremental content for current block (text_delta, input_json_delta)
// - ContentBlockStop: Current block finished → we emit complete block here
// - MessageDelta: Message-level delta (stop_reason, stop_sequence)
// - MessageStop: Streaming complete
func (p *Provider) emitAnthropicStreamEvents(event anthropic.MessageStreamEventUnion, message *anthropic.Message, tools []llmprovider.Tool, emitter *llmprovider.EventEmitter, dbg streamDebug) {
	switch e := event.AsAny().(type) {
	case anthropic.MessageStartEvent:
		// MessageStart event - not needed for deltas, metadata comes at the end
		return

	case anthropic.ContentBlockStartEvent:
		// ContentBlockStart - emit AG-UI start events
		dbg.Debug("content_block_start",
			"index", e.Index,
			"type", e.ContentBlock.Type,
		)

		messageID := string(message.ID)
		role := string(message.Role)

		// Emit AG-UI events based on block type
		switch e.ContentBlock.Type {
		case "text":
			// Text message start
			emitter.TextMessageStart(messageID, role)

		case "thinking":
			// Thinking phase: emit ThinkingStart + ThinkingTextMessageStart
			emitter.ThinkingStart(nil)
			emitter.ThinkingTextMessageStart()

		case "tool_use":
			// Tool call start
			if e.ContentBlock.ID != "" && e.ContentBlock.Name != "" {
				emitter.ToolCallStart(e.ContentBlock.ID, e.ContentBlock.Name, &messageID)
				dbg.Debug("tool_call_start",
					"index", e.Index,
					"id", e.ContentBlock.ID,
					"name", e.ContentBlock.Name,
				)
			}

		case "server_tool_use":
			// Provider-side tools (web_search) arrive complete in ContentBlockStart
			// No input_json_delta events will follow - input is complete on arrival
			if e.ContentBlock.ID != "" && e.ContentBlock.Name != "" {
				emitter.ToolCallStart(e.ContentBlock.ID, e.ContentBlock.Name, &messageID)
				dbg.Debug("server_tool_use_start",
					"index", e.Index,
					"id", e.ContentBlock.ID,
					"name", e.ContentBlock.Name,
				)
			}

		case "web_search_tool_result":
			// Web search results arrive complete in ContentBlockStart
			// No deltas will follow - this is a complete block
			dbg.Debug("web_search_result_start",
				"index", e.Index,
				"tool_use_id", e.ContentBlock.ToolUseID,
			)
			// AG-UI: ToolCallResult will be emitted on ContentBlockStop when we have complete data
		}

	case anthropic.ContentBlockDeltaEvent:
		// ContentBlockDelta - emit content delta events
		messageID := string(message.ID)

		switch e.Delta.Type {
		case "text_delta":
			text := e.Delta.Text
			if text != "" {
				emitter.TextMessageContent(messageID, text)
				dbg.Debug("text_delta",
					"index", e.Index,
					"chunk_len", len(text),
				)
			}

		case "thinking_delta":
			text := e.Delta.Thinking
			if text != "" {
				emitter.ThinkingTextMessageContent(text)
				dbg.Debug("thinking_delta",
					"index", e.Index,
					"chunk_len", len(text),
				)
			}

		case "signature_delta":
			// Signature deltas are Anthropic-specific, no AG-UI event
			// We could emit a custom event if needed
			sig := e.Delta.Signature
			dbg.Debug("signature_delta",
				"index", e.Index,
				"chunk_len", len(sig),
			)

		case "input_json_delta":
			jsonDelta := e.Delta.PartialJSON
			if jsonDelta != "" {
				// Get tool_use_id from the accumulated block (SDK accumulates as we stream)
				blockIndex := int(e.Index)
				if blockIndex >= 0 && blockIndex < len(message.Content) {
					block := message.Content[blockIndex]
					if block.ID != "" {
						emitter.ToolCallArgs(string(block.ID), jsonDelta)
						dbg.Debug("input_json_delta",
							"index", e.Index,
							"chunk_len", len(jsonDelta),
						)
					}
				}
			}
		}

	case anthropic.ContentBlockStopEvent:
		// ContentBlockStop - emit complete normalized block and end events
		blockIndex := int(e.Index)

		dbg.Debug("content_block_stop",
			"index", e.Index,
		)

		// Validate block index
		if blockIndex < 0 || blockIndex >= len(message.Content) {
			p.logger.Error("invalid block index in stream", "block_index", blockIndex, "message_blocks", len(message.Content))
			emitter.Error(fmt.Errorf("invalid block index %d, message has %d blocks", blockIndex, len(message.Content)))
			return
		}

		// Get the accumulated block from SDK
		anthropicBlock := message.Content[blockIndex]
		messageID := string(message.ID)

		// Emit AG-UI end events based on block type
		switch anthropicBlock.Type {
		case "text":
			emitter.TextMessageEnd(messageID)

		case "thinking":
			// Thinking phase end: ThinkingTextMessageEnd + ThinkingEnd
			emitter.ThinkingTextMessageEnd()
			emitter.ThinkingEnd()

		case "tool_use", "server_tool_use":
			if anthropicBlock.ID != "" {
				emitter.ToolCallEnd(string(anthropicBlock.ID))
			}

		case "web_search_tool_result":
			// Emit tool call result for web search
			if anthropicBlock.ToolUseID != "" {
				// Serialize the search results as JSON content
				resultContent, _ := json.Marshal(anthropicBlock.Content)
				emitter.ToolCallResult(messageID, anthropicBlock.ToolUseID, string(resultContent))
			}
		}

		// Convert the complete Anthropic block to library format using shared logic
		// This handles normalization of provider-specific types
		block, err := p.convertAnthropicBlock(anthropicBlock, blockIndex, tools)
		if err != nil {
			p.logger.Error("failed to convert block in stream", "block_index", blockIndex, "error", err)
			emitter.Error(fmt.Errorf("convert block %d: %w", blockIndex, err))
			return
		}

		// Log finalized block metadata
		logArgs := []any{"index", blockIndex, "block_type", block.BlockType}
		if block.BlockType == llmprovider.BlockTypeToolUse {
			if id, ok := block.Content["id"].(string); ok {
				logArgs = append(logArgs, "id", id)
			}
			if name, ok := block.Content["tool_name"].(string); ok {
				logArgs = append(logArgs, "name", name)
			}
			if input, ok := block.Content["input"]; ok {
				inputJSON, _ := json.Marshal(input)
				logArgs = append(logArgs, "args_len", len(inputJSON))
			}
		}
		dbg.Debug("block_finalized", logArgs...)

		// Emit complete block for persistence
		emitter.Block(block)

	case anthropic.MessageDeltaEvent:
		// MessageDelta - contains stop_reason, handled in FinalMessage
		return

	case anthropic.MessageStopEvent:
		// MessageStop - final metadata sent after stream.Next() completes
		return

	default:
		// Unknown event type - log warning but don't fail
		return
	}
}
