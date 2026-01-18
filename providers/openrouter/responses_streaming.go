package openrouter

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	llmprovider "github.com/haowjy/meridian-llm-go"
)

// StreamResponsesAPI generates a streaming response using the OpenRouter Responses API.
// This is an alternative to the Chat Completions API with better tool call streaming.
func (p *Provider) StreamResponsesAPI(ctx context.Context, req *llmprovider.GenerateRequest) (<-chan llmprovider.StreamEvent, error) {
	// Validate model
	if !p.SupportsModel(req.Model) {
		return nil, &llmprovider.ModelError{
			Model:    req.Model,
			Provider: p.Name().String(),
			Reason:   "model not supported by OpenRouter (must be in 'provider/model' format)",
			Err:      llmprovider.ErrInvalidModel,
		}
	}

	// Validate web_search requirements
	if err := p.validateWebSearchRequirements(req); err != nil {
		return nil, err
	}

	// Build Responses API request
	responsesReq, err := p.buildResponsesRequest(req)
	if err != nil {
		return nil, err
	}

	// Enable streaming
	responsesReq.Stream = true

	// Make HTTP request
	httpReq, err := p.buildHTTPRequestResponses(ctx, responsesReq)
	if err != nil {
		return nil, err
	}

	// Set Accept header for SSE
	httpReq.Header.Set("Accept", "text/event-stream")

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		p.logger.Error("openrouter responses API HTTP request failed", "model", req.Model, "error", err)
		return nil, fmt.Errorf("openrouter responses API HTTP request failed: %w", err)
	}

	// Check for immediate errors
	if resp.StatusCode != http.StatusOK {
		defer func() { _ = resp.Body.Close() }()
		return nil, p.handleErrorResponse(resp, req.Model)
	}

	// Create streaming channel
	eventChan := make(chan llmprovider.StreamEvent, 10)

	// Start streaming goroutine
	go func() {
		defer close(eventChan)
		defer func() { _ = resp.Body.Close() }()

		if err := p.streamResponsesEvents(ctx, resp.Body, eventChan); err != nil {
			eventChan <- llmprovider.StreamEvent{Error: err}
		}
	}()

	return eventChan, nil
}

// buildHTTPRequestResponses creates an HTTP request for the Responses API.
func (p *Provider) buildHTTPRequestResponses(ctx context.Context, req *ResponsesRequest) (*http.Request, error) {
	body, err := json.Marshal(req)
	if err != nil {
		p.logger.Error("failed to marshal responses request", "model", req.Model, "error", err)
		return nil, fmt.Errorf("failed to marshal responses request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/responses", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	// Set headers
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	httpReq.Header.Set("Content-Type", "application/json")

	return httpReq, nil
}

// streamResponsesEvents reads SSE events from the Responses API and emits library StreamEvents.
func (p *Provider) streamResponsesEvents(ctx context.Context, body io.ReadCloser, eventChan chan<- llmprovider.StreamEvent) error {
	dbg := streamDebug{enabled: p.debugStreamLogs, logger: p.logger}
	emitter := llmprovider.NewEventEmitter(eventChan)

	// Close body when context is cancelled to unblock scanner.
	// scanner.Scan() blocks waiting for data and only checks ctx.Done() after reading.
	// Closing the body forces Scan() to return with an error, enabling immediate cancellation.
	go func() {
		<-ctx.Done()
		body.Close()
	}()

	// Start line reader goroutine
	lineChan := make(chan string, 10)
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

	// Initialize streaming state
	state := newResponsesStreamState()

	// Accumulators for content
	var textContent strings.Builder
	var thinkingContent strings.Builder

	// Token usage
	var usage *ResponseUsage

	// Current message ID for AG-UI events
	var messageID string

	// Idle timeout timer
	idleTimer := time.NewTimer(DefaultStreamingIdleTimeout)
	defer idleTimer.Stop()

	resetIdleTimer := func() {
		if !idleTimer.Stop() {
			select {
			case <-idleTimer.C:
			default:
			}
		}
		idleTimer.Reset(DefaultStreamingIdleTimeout)
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case <-idleTimer.C:
			p.logger.Warn("responses API streaming idle timeout", "timeout", DefaultStreamingIdleTimeout, "model", state.model)
			return &llmprovider.ProviderError{
				Code:      llmprovider.ErrorCodeStreamingIdleTimeout,
				Provider:  llmprovider.ProviderOpenRouter.String(),
				Message:   fmt.Sprintf("no data received for %v during streaming", DefaultStreamingIdleTimeout),
				Retryable: true,
				Err:       llmprovider.ErrStreamingIdleTimeout,
			}

		case err := <-scanErrChan:
			p.logger.Error("error reading responses API stream", "model", state.model, "error", err)
			return fmt.Errorf("error reading stream: %w", err)

		case line, ok := <-lineChan:
			if !ok {
				// Channel closed - scanner finished
				goto finalize
			}

			// Reset idle timer
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

			// Parse event
			var event ResponsesSSEEvent
			if err := json.Unmarshal([]byte(data), &event); err != nil {
				// Try to parse as error
				var errResp struct {
					Error *ResponseError `json:"error"`
				}
				if json.Unmarshal([]byte(data), &errResp) == nil && errResp.Error != nil {
					p.logger.Error("openrouter responses API error", "error", errResp.Error.Message)
					return fmt.Errorf("openrouter responses API error: %s", errResp.Error.Message)
				}
				// Ignore unparseable chunks
				dbg.Warn("failed to parse responses API event", "data", data)
				continue
			}

			// Handle event by type
			if err := p.handleResponsesEvent(
				&event,
				state,
				emitter,
				&textContent,
				&thinkingContent,
				&messageID,
				&usage,
				dbg,
				eventChan,
			); err != nil {
				return err
			}

			// Guardrail: treat tool-arg streams as stalled if they stop making meaningful
			// progress for too long, even if the provider continues to send whitespace.
			if callID, acc := state.findStalledToolArgs(time.Now(), DefaultStreamingIdleTimeout); acc != nil {
				dbg.Warn("responses API tool call args stalled",
					"call_id", callID,
					"name", acc.Name,
					"timeout", DefaultStreamingIdleTimeout,
				)
				return &llmprovider.ProviderError{
					Code:      llmprovider.ErrorCodeStreamingIdleTimeout,
					Provider:  llmprovider.ProviderOpenRouter.String(),
					Message:   fmt.Sprintf("no meaningful tool call args progress for %v during streaming", DefaultStreamingIdleTimeout),
					Retryable: true,
					Err:       llmprovider.ErrStreamingIdleTimeout,
				}
			}
		}
	}

finalize:
	return p.finalizeResponsesStream(
		state,
		emitter,
		&textContent,
		&thinkingContent,
		messageID,
		usage,
		dbg,
		eventChan,
	)
}

// handleResponsesEvent processes a single Responses API SSE event.
func (p *Provider) handleResponsesEvent(
	event *ResponsesSSEEvent,
	state *responsesStreamState,
	emitter *llmprovider.EventEmitter,
	textContent *strings.Builder,
	thinkingContent *strings.Builder,
	messageID *string,
	usage **ResponseUsage,
	dbg streamDebug,
	eventChan chan<- llmprovider.StreamEvent,
) error {
	providerIDStr := llmprovider.ProviderOpenRouter.String()

	switch event.Type {
	case ResponsesEventResponseCreated:
		// Capture response ID and model
		if event.Response != nil {
			state.responseID = event.Response.ID
			state.model = event.Response.Model

			// Emit generation ID event for early persistence
			if state.responseID != "" {
				eventChan <- llmprovider.StreamEvent{
					GenerationIDDiscovered: &llmprovider.GenerationIDEvent{
						GenerationID: state.responseID,
						Model:        state.model,
						Provider:     "openrouter",
					},
				}
			}
		}
		dbg.Debug("responses API response created", "response_id", state.responseID)

	case ResponsesEventOutputItemAdded:
		// New output item added - track it and emit appropriate start event
		if event.Item != nil {
			state.outputItemByID[event.Item.ID] = event.Item

			switch event.Item.Type {
			case "message":
				// Start text message
				*messageID = event.Item.ID
				state.activeMessageItemID = event.Item.ID
				emitter.TextMessageStart(event.Item.ID, "assistant")
				dbg.Debug("responses API message item added", "item_id", event.Item.ID)

			case "function_call":
				// Start tool call - use CallID for tracking (not item ID)
				callID := event.Item.CallID
				if callID == "" {
					// Fallback to item ID if no call_id
					callID = event.Item.ID
				}
				state.callArgsByCallID[callID] = &responsesToolCallAccumulator{
					ItemID: event.Item.ID,
					CallID: callID,
					Name:   event.Item.Name,
				}
				// Emit ToolCallStart with call_id and parent messageID
				emitter.ToolCallStart(callID, event.Item.Name, messageID)
				dbg.Debug("responses API function_call item added", "item_id", event.Item.ID, "call_id", callID, "name", event.Item.Name)

			case "reasoning":
				// Start thinking
				state.activeReasoningItemID = event.Item.ID
				emitter.ThinkingStart(nil)
				emitter.ThinkingTextMessageStart()
				dbg.Debug("responses API reasoning item added", "item_id", event.Item.ID)
			}
		}

	case ResponsesEventContentPartDelta:
		// Text content delta - route to active message
		if event.Delta != "" && state.activeMessageItemID != "" {
			textContent.WriteString(event.Delta)
			emitter.TextMessageContent(*messageID, event.Delta)
			dbg.Debug("responses API text delta", "len", len(event.Delta))
		}

	case ResponsesEventFunctionCallArgsDelta:
		// Tool call arguments delta - stream to client and accumulate
		// Find the tool call accumulator
		if event.Arguments == "" {
			break
		}
		for callID, acc := range state.callArgsByCallID {
			// Match by item_id from the event
			if event.ItemID == acc.ItemID || callID == event.ItemID {
				now := time.Now()
				if !acc.seenAnyArgs {
					acc.seenAnyArgs = true
					acc.firstArgsAt = now
				}
				acc.lastAnyArgsAt = now

				// Treat whitespace-only deltas outside JSON strings as non-progress.
				meaningful := acc.argsProgress.Apply(event.Arguments)
				if meaningful {
					// Accumulate arguments
					prevLen := len(acc.Arguments)
					acc.Arguments += event.Arguments
					dbg.Debug("responses API function args delta",
						"call_id", callID,
						"prev_len", prevLen,
						"chunk_len", len(event.Arguments),
						"new_total", len(acc.Arguments),
					)

					// Stream delta to client via AG-UI event
					emitter.ToolCallArgs(callID, event.Arguments)

					acc.seenMeaningfulArgs = true
					acc.lastMeaningfulArgsAt = now
				} else {
					dbg.Debug("responses API function args delta ignored (non-meaningful whitespace outside string)",
						"call_id", callID,
						"chunk_len", len(event.Arguments),
					)
				}
				break
			}
		}

	case ResponsesEventFunctionCallArgsDone:
		// Tool call arguments complete - emit block
		for callID, acc := range state.callArgsByCallID {
			if event.ItemID == acc.ItemID || callID == event.ItemID {
				// Use arguments from event if present, otherwise from accumulator
				args := event.Arguments
				if args == "" {
					args = acc.Arguments
				}

				// Parse arguments
				input := make(map[string]interface{})
				if args != "" {
					if err := json.Unmarshal([]byte(args), &input); err != nil {
						dbg.Warn("responses API malformed tool JSON",
							"call_id", callID,
							"name", acc.Name,
							"error", err,
						)
						// Emit recovery blocks for malformed JSON
						recovery, _ := llmprovider.NewMalformedToolRecovery(
							callID, acc.Name, args, err,
							0, &providerIDStr, // Sequence managed by caller
						)
						eventChan <- llmprovider.StreamEvent{Block: recovery.ToolUseBlock}
						eventChan <- llmprovider.StreamEvent{Block: recovery.ToolResultBlock}
						// Remove from tracking
						delete(state.callArgsByCallID, callID)
						break
					}
				}

				// Build tool_use block
				content := map[string]interface{}{
					"tool_use_id": callID,
					"tool_name":   acc.Name,
					"input":       input,
				}
				executionSide := llmprovider.ExecutionSideServer

				// Emit ToolCallEnd and Block
				emitter.ToolCallEnd(callID)
				emitter.Block(&llmprovider.Block{
					BlockType:     llmprovider.BlockTypeToolUse,
					Sequence:      0, // Sequence managed by caller
					Content:       content,
					ExecutionSide: &executionSide,
					Provider:      &providerIDStr,
				})

				dbg.Debug("responses API function_call complete", "call_id", callID, "name", acc.Name)

				// Remove from tracking (complete)
				delete(state.callArgsByCallID, callID)
				break
			}
		}

	case ResponsesEventReasoningDelta:
		// Thinking/reasoning content delta
		if event.Delta != "" && state.activeReasoningItemID != "" {
			thinkingContent.WriteString(event.Delta)
			emitter.ThinkingTextMessageContent(event.Delta)
			dbg.Debug("responses API reasoning delta", "len", len(event.Delta))
		}

	case ResponsesEventReasoningSummaryDelta:
		// Reasoning summary delta (treat same as reasoning)
		if event.Delta != "" && state.activeReasoningItemID != "" {
			thinkingContent.WriteString(event.Delta)
			emitter.ThinkingTextMessageContent(event.Delta)
		}

	case ResponsesEventOutputItemDone:
		// Output item complete
		if event.Item != nil {
			switch event.Item.Type {
			case "message":
				// Message complete - will emit block in finalize
				dbg.Debug("responses API message item done", "item_id", event.Item.ID)

			case "reasoning":
				// Reasoning complete - emit thinking end events
				state.activeReasoningItemID = ""
				// Block will be emitted in finalize
				dbg.Debug("responses API reasoning item done", "item_id", event.Item.ID)
			}
		}

	case ResponsesEventResponseDone:
		// Response complete - capture final usage
		if event.Response != nil && event.Response.Usage != nil {
			*usage = event.Response.Usage
		}
		dbg.Debug("responses API response done")

	case ResponsesEventError:
		// Error event
		if event.Error != nil {
			p.logger.Error("responses API error event", "code", event.Error.Code, "message", event.Error.Message)
			return fmt.Errorf("responses API error: %s", event.Error.Message)
		}
	}

	return nil
}

// finalizeResponsesStream emits final blocks and metadata.
func (p *Provider) finalizeResponsesStream(
	state *responsesStreamState,
	emitter *llmprovider.EventEmitter,
	textContent *strings.Builder,
	thinkingContent *strings.Builder,
	messageID string,
	usage *ResponseUsage,
	dbg streamDebug,
	eventChan chan<- llmprovider.StreamEvent,
) error {
	providerIDStr := llmprovider.ProviderOpenRouter.String()
	sequence := 0

	// Emit thinking block if we have content
	if thinkingContent.Len() > 0 {
		text := thinkingContent.String()
		if strings.TrimSpace(text) != "" {
			emitter.ThinkingTextMessageEnd()
			emitter.ThinkingEnd()
			emitter.Block(&llmprovider.Block{
				BlockType:   llmprovider.BlockTypeThinking,
				Sequence:    sequence,
				TextContent: &text,
				Provider:    &providerIDStr,
			})
			sequence++
		}
	}

	// Emit text block if we have content
	if textContent.Len() > 0 {
		text := textContent.String()
		emitter.TextMessageEnd(messageID)
		emitter.Block(&llmprovider.Block{
			BlockType:   llmprovider.BlockTypeText,
			Sequence:    sequence,
			TextContent: &text,
			Provider:    &providerIDStr,
		})
		sequence++
	}

	// Emit any remaining incomplete tool calls (shouldn't happen normally)
	for callID, acc := range state.callArgsByCallID {
		dbg.Warn("responses API incomplete tool call at finalize", "call_id", callID, "name", acc.Name)

		// Parse whatever arguments we have
		input := make(map[string]interface{})
		if acc.Arguments != "" {
			_ = json.Unmarshal([]byte(acc.Arguments), &input)
		}

		content := map[string]interface{}{
			"tool_use_id": callID,
			"tool_name":   acc.Name,
			"input":       input,
		}
		executionSide := llmprovider.ExecutionSideServer

		emitter.ToolCallEnd(callID)
		emitter.Block(&llmprovider.Block{
			BlockType:     llmprovider.BlockTypeToolUse,
			Sequence:      sequence,
			Content:       content,
			ExecutionSide: &executionSide,
			Provider:      &providerIDStr,
		})
		sequence++
	}

	// Determine stop reason
	stopReason := "end_turn"
	if len(state.callArgsByCallID) > 0 {
		stopReason = "tool_use"
	}

	// Emit final metadata
	metadata := &llmprovider.StreamMetadata{
		Model:        state.model,
		StopReason:   stopReason,
		GenerationID: state.responseID,
	}

	if usage != nil {
		metadata.InputTokens = usage.InputTokens
		metadata.OutputTokens = usage.OutputTokens
	}

	emitter.Metadata(metadata)

	dbg.Debug("responses API stream finalized", "response_id", state.responseID, "model", state.model)
	return nil
}
