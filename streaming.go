package llmprovider

import (
	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/events"
)

// StreamEvent represents a single event in a streaming response.
// Uses the AG-UI SDK Event interface for protocol compliance.
//
// AG-UI Events (via Event field):
//   - Text Message Events: TEXT_MESSAGE_START, TEXT_MESSAGE_CONTENT, TEXT_MESSAGE_END
//   - Thinking Events: THINKING_START, THINKING_TEXT_MESSAGE_*, THINKING_END
//   - Tool Call Events: TOOL_CALL_START, TOOL_CALL_ARGS, TOOL_CALL_END, TOOL_CALL_RESULT
//   - Lifecycle Events: RUN_STARTED, RUN_FINISHED, RUN_ERROR, STEP_STARTED, STEP_FINISHED
//
// Non-AG-UI fields (Meridian-specific):
//   - Block: Complete block for persistence
//   - Metadata: Final response metadata
//   - GenerationIDDiscovered: Early generation ID for cancel enrichment
//   - Error: Streaming error
type StreamEvent struct {
	// =============================================================================
	// AG-UI Event (SDK interface)
	// =============================================================================

	// Event is the AG-UI protocol event implementing the events.Event interface.
	// Type assert to specific event types:
	//   - *events.TextMessageStartEvent, *events.TextMessageContentEvent, *events.TextMessageEndEvent
	//   - *events.ThinkingStartEvent, *events.ThinkingTextMessageContentEvent, etc.
	//   - *events.ToolCallStartEvent, *events.ToolCallArgsEvent, *events.ToolCallEndEvent
	//   - *events.RunStartedEvent, *events.RunFinishedEvent, *events.RunErrorEvent
	//   - *events.StepStartedEvent, *events.StepFinishedEvent
	Event events.Event `json:"event,omitempty"`

	// =============================================================================
	// Non-AG-UI Fields (Meridian-specific)
	// =============================================================================

	// Block contains a complete block when a block finishes streaming.
	// This is emitted once per block when streaming completes for that block.
	// The block is normalized and ready for database persistence.
	// INTERNAL: Used for block reconstruction, not part of AG-UI spec
	Block *Block `json:"block,omitempty"`

	// Metadata contains final response data when streaming completes (nil until end)
	Metadata *StreamMetadata `json:"metadata,omitempty"`

	// GenerationIDDiscovered is a non-terminal metadata event emitted when generation ID is discovered
	// Emitted once per generation (on first chunk), allows early persistence
	// This is separate from Metadata which is the final event
	GenerationIDDiscovered *GenerationIDEvent `json:"generationIDDiscovered,omitempty"`

	// Error contains any error that occurred during streaming (nil if successful)
	Error error `json:"error,omitempty"`
}

// =============================================================================
// Type-safe accessors for AG-UI events
// =============================================================================

// IsAGUIEvent returns true if this StreamEvent contains an AG-UI event.
func (e *StreamEvent) IsAGUIEvent() bool {
	return e.Event != nil
}

// GetEventType returns the AG-UI event type, or empty string if not an AG-UI event.
func (e *StreamEvent) GetEventType() events.EventType {
	if e.Event == nil {
		return ""
	}
	return e.Event.Type()
}

// GetTextMessageStart returns the TextMessageStartEvent if present.
func (e *StreamEvent) GetTextMessageStart() (*events.TextMessageStartEvent, bool) {
	evt, ok := e.Event.(*events.TextMessageStartEvent)
	return evt, ok
}

// GetTextMessageContent returns the TextMessageContentEvent if present.
func (e *StreamEvent) GetTextMessageContent() (*events.TextMessageContentEvent, bool) {
	evt, ok := e.Event.(*events.TextMessageContentEvent)
	return evt, ok
}

// GetTextMessageEnd returns the TextMessageEndEvent if present.
func (e *StreamEvent) GetTextMessageEnd() (*events.TextMessageEndEvent, bool) {
	evt, ok := e.Event.(*events.TextMessageEndEvent)
	return evt, ok
}

// GetThinkingStart returns the ThinkingStartEvent if present.
func (e *StreamEvent) GetThinkingStart() (*events.ThinkingStartEvent, bool) {
	evt, ok := e.Event.(*events.ThinkingStartEvent)
	return evt, ok
}

// GetThinkingTextMessageStart returns the ThinkingTextMessageStartEvent if present.
func (e *StreamEvent) GetThinkingTextMessageStart() (*events.ThinkingTextMessageStartEvent, bool) {
	evt, ok := e.Event.(*events.ThinkingTextMessageStartEvent)
	return evt, ok
}

// GetThinkingTextMessageContent returns the ThinkingTextMessageContentEvent if present.
func (e *StreamEvent) GetThinkingTextMessageContent() (*events.ThinkingTextMessageContentEvent, bool) {
	evt, ok := e.Event.(*events.ThinkingTextMessageContentEvent)
	return evt, ok
}

// GetThinkingTextMessageEnd returns the ThinkingTextMessageEndEvent if present.
func (e *StreamEvent) GetThinkingTextMessageEnd() (*events.ThinkingTextMessageEndEvent, bool) {
	evt, ok := e.Event.(*events.ThinkingTextMessageEndEvent)
	return evt, ok
}

// GetThinkingEnd returns the ThinkingEndEvent if present.
func (e *StreamEvent) GetThinkingEnd() (*events.ThinkingEndEvent, bool) {
	evt, ok := e.Event.(*events.ThinkingEndEvent)
	return evt, ok
}

// GetToolCallStart returns the ToolCallStartEvent if present.
func (e *StreamEvent) GetToolCallStart() (*events.ToolCallStartEvent, bool) {
	evt, ok := e.Event.(*events.ToolCallStartEvent)
	return evt, ok
}

// GetToolCallArgs returns the ToolCallArgsEvent if present.
func (e *StreamEvent) GetToolCallArgs() (*events.ToolCallArgsEvent, bool) {
	evt, ok := e.Event.(*events.ToolCallArgsEvent)
	return evt, ok
}

// GetToolCallEnd returns the ToolCallEndEvent if present.
func (e *StreamEvent) GetToolCallEnd() (*events.ToolCallEndEvent, bool) {
	evt, ok := e.Event.(*events.ToolCallEndEvent)
	return evt, ok
}

// GetToolCallResult returns the ToolCallResultEvent if present.
func (e *StreamEvent) GetToolCallResult() (*events.ToolCallResultEvent, bool) {
	evt, ok := e.Event.(*events.ToolCallResultEvent)
	return evt, ok
}

// GetRunStarted returns the RunStartedEvent if present.
func (e *StreamEvent) GetRunStarted() (*events.RunStartedEvent, bool) {
	evt, ok := e.Event.(*events.RunStartedEvent)
	return evt, ok
}

// GetRunFinished returns the RunFinishedEvent if present.
func (e *StreamEvent) GetRunFinished() (*events.RunFinishedEvent, bool) {
	evt, ok := e.Event.(*events.RunFinishedEvent)
	return evt, ok
}

// GetRunError returns the RunErrorEvent if present.
func (e *StreamEvent) GetRunError() (*events.RunErrorEvent, bool) {
	evt, ok := e.Event.(*events.RunErrorEvent)
	return evt, ok
}

// GetStepStarted returns the StepStartedEvent if present.
func (e *StreamEvent) GetStepStarted() (*events.StepStartedEvent, bool) {
	evt, ok := e.Event.(*events.StepStartedEvent)
	return evt, ok
}

// GetStepFinished returns the StepFinishedEvent if present.
func (e *StreamEvent) GetStepFinished() (*events.StepFinishedEvent, bool) {
	evt, ok := e.Event.(*events.StepFinishedEvent)
	return evt, ok
}

// StreamMetadata contains completion information sent when streaming finishes.
// This is sent as the final event before the channel closes.
type StreamMetadata struct {
	// Model is the model that was used (may differ from request if aliased)
	Model string

	// InputTokens is the number of tokens in the input
	InputTokens int

	// OutputTokens is the number of tokens in the output
	OutputTokens int

	// StopReason indicates why generation stopped (e.g., "end_turn", "max_tokens", "tool_use")
	StopReason string

	// GenerationID is the unique identifier for this generation (provider-specific)
	// Used for querying generation stats after cancel/timeout.
	// OpenRouter: Can be used with GET /api/v1/generation?id={GenerationID} to get native token counts.
	GenerationID string

	// ResponseMetadata contains provider-specific response data
	ResponseMetadata map[string]interface{}
}

// GenerationIDEvent contains generation metadata discovered early in the stream.
// This is emitted as soon as the provider sends the generation ID (typically first chunk),
// not at stream completion like StreamMetadata.
// Allows early persistence for cancel-via-generation enrichment.
type GenerationIDEvent struct {
	// GenerationID is the unique identifier for this generation (provider-specific)
	// OpenRouter: e.g., "gen-abc123xyz"
	GenerationID string

	// Model is the model identifier (e.g., "x-ai/grok-beta")
	Model string

	// Provider is the provider name (e.g., "openrouter")
	Provider string
}
