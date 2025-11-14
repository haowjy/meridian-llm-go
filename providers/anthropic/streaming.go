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
			Provider: p.Name(),
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
			streamEvent := transformAnthropicStreamEvent(event)

			// Send to channel (check context in case consumer cancelled)
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
// Anthropic stream events include:
// - MessageStart: Contains message metadata (id, model, role)
// - ContentBlockStart: New content block started (index, type)
// - ContentBlockDelta: Incremental content for current block (text_delta, input_json_delta)
// - ContentBlockStop: Current block finished
// - MessageDelta: Message-level delta (stop_reason, stop_sequence)
// - MessageStop: Streaming complete
func transformAnthropicStreamEvent(event anthropic.MessageStreamEventUnion) llmprovider.StreamEvent {
	switch e := event.AsAny().(type) {
	case anthropic.MessageStartEvent:
		// MessageStart event - not needed for deltas, metadata comes at the end
		return llmprovider.StreamEvent{} // Empty event, ignored by consumers

	case anthropic.ContentBlockStartEvent:
		// ContentBlockStart - emit block start delta with BlockType set
		blockType := string(e.ContentBlock.Type)
		delta := &llmprovider.BlockDelta{
			BlockIndex: int(e.Index),
			BlockType:  &blockType, // Set BlockType pointer (signals block start)
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
			delta.DeltaType = llmprovider.DeltaTypeInputJSON
			jsonDelta := e.Delta.PartialJSON
			delta.InputJSONDelta = &jsonDelta
		}

		return llmprovider.StreamEvent{Delta: delta}

	case anthropic.ContentBlockStopEvent:
		// ContentBlockStop - not needed, block completion handled by consumer
		return llmprovider.StreamEvent{} // Empty event

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
