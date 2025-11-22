package anthropic

import (
	"context"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"

	"github.com/haowjy/meridian-llm-go"
)

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

	// Build Anthropic API parameters (shared logic with GenerateResponse)
	apiParams, err := buildMessageParams(req)
	if err != nil {
		return nil, err
	}

	// Create streaming channel
	eventChan := make(chan llmprovider.StreamEvent, 10) // Buffered to prevent blocking

	// Start streaming goroutine
	go func() {
		defer close(eventChan)

		// Call Anthropic streaming API
		stream := p.client.Messages.NewStreaming(ctx, apiParams)

		// Accumulator for final message metadata
		message := anthropic.Message{}

		// Iterate through streaming events
		for stream.Next() {
			event := stream.Current()

			// Accumulate event into final message
			if err := message.Accumulate(event); err != nil {
				eventChan <- llmprovider.StreamEvent{
					Error: fmt.Errorf("failed to accumulate message: %w", err),
				}
				return
			}

			// Transform Anthropic event to library StreamEvent
			// Pass accumulated message so we can emit complete blocks on ContentBlockStop
			streamEvent := transformAnthropicStreamEvent(event, &message)

			// Send to channel if not empty (check context in case consumer cancelled)
			if streamEvent.Delta != nil || streamEvent.Block != nil || streamEvent.Error != nil {
				select {
				case <-ctx.Done():
					// Consumer cancelled, send error and exit
					eventChan <- llmprovider.StreamEvent{
						Error: ctx.Err(),
					}
					return
				case eventChan <- streamEvent:
					// Successfully sent
				}
			}
		}

		// Check for streaming errors
		if err := stream.Err(); err != nil {
			eventChan <- llmprovider.StreamEvent{
				Error: fmt.Errorf("anthropic streaming error: %w", err),
			}
			return
		}

		// Send final message metadata
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

// transformAnthropicStreamEvent converts an Anthropic streaming event to a library StreamEvent.
//
// The message parameter is the SDK's accumulated message, which contains complete ContentBlocks
// as they finish streaming. We use this to emit complete, normalized blocks when ContentBlockStop arrives.
//
// Anthropic stream events include:
// - MessageStart: Contains message metadata (id, model, role)
// - ContentBlockStart: New content block started (index, type)
// - ContentBlockDelta: Incremental content for current block (text_delta, input_json_delta)
// - ContentBlockStop: Current block finished → we emit complete block here
// - MessageDelta: Message-level delta (stop_reason, stop_sequence)
// - MessageStop: Streaming complete
func transformAnthropicStreamEvent(event anthropic.MessageStreamEventUnion, message *anthropic.Message) llmprovider.StreamEvent {
	switch e := event.AsAny().(type) {
	case anthropic.MessageStartEvent:
		// MessageStart event - not needed for deltas, metadata comes at the end
		return llmprovider.StreamEvent{} // Empty event, ignored by consumers

	case anthropic.ContentBlockStartEvent:
		// ContentBlockStart - emit block start delta with BlockType set.
		// For provider-specific types, we normalize BlockType to library block types
		// so downstream consumers see consistent values (e.g., "web_search_use", "web_search_result").
		var blockType string
		switch e.ContentBlock.Type {
		case "text":
			blockType = llmprovider.BlockTypeText
		case "thinking":
			blockType = llmprovider.BlockTypeThinking
		case "tool_use":
			blockType = llmprovider.BlockTypeToolUse
		case "server_tool_use":
			// Server-side tools: web_search_use vs other server tools
			if e.ContentBlock.Name == "web_search" {
				blockType = llmprovider.BlockTypeWebSearch
			} else {
				blockType = llmprovider.BlockTypeToolUse
			}
		case "web_search_tool_result":
			blockType = llmprovider.BlockTypeWebSearchResult
		default:
			// Fallback to raw provider type string
			blockType = string(e.ContentBlock.Type)
		}

		delta := &llmprovider.BlockDelta{
			BlockIndex: int(e.Index),
			BlockType:  &blockType, // Signals block start with normalized type
		}

		// Set appropriate DeltaType based on block type
		switch e.ContentBlock.Type {
		case "text":
			delta.DeltaType = llmprovider.DeltaTypeText

		case "thinking":
			delta.DeltaType = llmprovider.DeltaTypeThinking
			// Initial signature comes in signature_delta events, not here
			// (Anthropic sends empty signature:"" in content_block_start)

		case "tool_use":
			delta.DeltaType = llmprovider.DeltaTypeToolCallStart
			if e.ContentBlock.ID != "" {
				toolID := e.ContentBlock.ID
				delta.ToolCallID = &toolID
				delta.ToolUseID = &toolID // Legacy field
			}
			if e.ContentBlock.Name != "" {
				toolName := e.ContentBlock.Name
				delta.ToolCallName = &toolName
				delta.ToolName = &toolName // Legacy field
			}

		case "server_tool_use":
		// Server-side tools (web_search) arrive complete in ContentBlockStart
		// No input_json_delta events will follow - input is complete on arrival
		delta.DeltaType = llmprovider.DeltaTypeToolCallStart
		if e.ContentBlock.ID != "" {
			toolID := e.ContentBlock.ID
			delta.ToolCallID = &toolID
			delta.ToolUseID = &toolID // Legacy field
		}
		if e.ContentBlock.Name != "" {
			toolName := e.ContentBlock.Name
			delta.ToolCallName = &toolName
			delta.ToolName = &toolName // Legacy field
		}
		// Note: Input is complete but we don't send it in the delta
		// The complete block (with ProviderData) will be emitted on ContentBlockStop

		case "web_search_tool_result":
		// Web search results arrive complete in ContentBlockStart
		// No deltas will follow - this is a complete block
		// Emit a delta to signal block start, complete data comes in ContentBlockStop
		delta.DeltaType = llmprovider.DeltaTypeToolResult
		if e.ContentBlock.ToolUseID != "" {
			toolID := e.ContentBlock.ToolUseID
			delta.ToolCallID = &toolID
			delta.ToolUseID = &toolID // Legacy field
		}
		}

		return llmprovider.StreamEvent{Delta: delta}

	case anthropic.ContentBlockDeltaEvent:
		// ContentBlockDelta - emit content delta
		delta := &llmprovider.BlockDelta{
			BlockIndex: int(e.Index),
		}

		// Extract delta based on type
		switch e.Delta.Type {
		case "text_delta":
			delta.DeltaType = llmprovider.DeltaTypeText
			text := e.Delta.Text
			delta.TextDelta = &text

		case "thinking_delta":
			delta.DeltaType = llmprovider.DeltaTypeThinking
			text := e.Delta.Thinking
			delta.TextDelta = &text

		case "signature_delta":
			delta.DeltaType = llmprovider.DeltaTypeSignature
			sig := e.Delta.Signature
			delta.SignatureDelta = &sig

		case "input_json_delta":
			delta.DeltaType = llmprovider.DeltaTypeJSON
			jsonDelta := e.Delta.PartialJSON
			delta.JSONDelta = &jsonDelta
		}

		return llmprovider.StreamEvent{Delta: delta}

	case anthropic.ContentBlockStopEvent:
		// ContentBlockStop - emit complete normalized block using shared conversion logic
		// The SDK has accumulated the complete block in message.Content[index]
		blockIndex := int(e.Index)

		// Validate block index
		if blockIndex < 0 || blockIndex >= len(message.Content) {
			return llmprovider.StreamEvent{
				Error: fmt.Errorf("invalid block index %d, message has %d blocks", blockIndex, len(message.Content)),
			}
		}

		// Convert the complete Anthropic block to library format using shared logic
		// This handles normalization of provider-specific types (server_tool_use → web_search,
		// web_search_tool_result → web_search_result)
		block, err := convertAnthropicBlock(message.Content[blockIndex], blockIndex)
		if err != nil {
			return llmprovider.StreamEvent{
				Error: fmt.Errorf("convert block %d: %w", blockIndex, err),
			}
		}

		return llmprovider.StreamEvent{Block: block}

	case anthropic.MessageDeltaEvent:
		// MessageDelta - contains stop_reason, handled in FinalMessage
		return llmprovider.StreamEvent{} // Empty event

	case anthropic.MessageStopEvent:
		// MessageStop - final metadata sent after stream.Next() completes
		return llmprovider.StreamEvent{} // Empty event

	default:
		// Unknown event type - log warning but don't fail
		// TODO: Add structured logging
		return llmprovider.StreamEvent{} // Empty event
	}
}
