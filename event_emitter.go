package llmprovider

import (
	"github.com/ag-ui-protocol/ag-ui/sdks/community/go/pkg/core/events"
)

// EventEmitter provides convenient methods for emitting AG-UI events to a StreamEvent channel.
// It centralizes event creation and emission, ensuring consistent event handling across providers.
//
// Usage:
//
//	eventChan := make(chan StreamEvent, 10)
//	emitter := NewEventEmitter(eventChan)
//	emitter.TextMessageStart("msg-123", "assistant")
//	emitter.TextMessageContent("msg-123", "Hello, ")
//	emitter.TextMessageContent("msg-123", "world!")
//	emitter.TextMessageEnd("msg-123")
type EventEmitter struct {
	eventChan chan<- StreamEvent
}

// NewEventEmitter creates a new EventEmitter that sends events to the given channel.
func NewEventEmitter(ch chan<- StreamEvent) *EventEmitter {
	return &EventEmitter{eventChan: ch}
}

// =============================================================================
// Text Message Events
// =============================================================================

// TextMessageStart emits a TEXT_MESSAGE_START event signaling the beginning of a text message.
func (e *EventEmitter) TextMessageStart(messageID, role string) {
	e.eventChan <- StreamEvent{
		Event: events.NewTextMessageStartEvent(messageID, events.WithRole(role)),
	}
}

// TextMessageContent emits a TEXT_MESSAGE_CONTENT event with incremental text content.
func (e *EventEmitter) TextMessageContent(messageID, delta string) {
	if delta == "" {
		return // Don't emit empty deltas
	}
	e.eventChan <- StreamEvent{
		Event: events.NewTextMessageContentEvent(messageID, delta),
	}
}

// TextMessageEnd emits a TEXT_MESSAGE_END event signaling completion of a text message.
func (e *EventEmitter) TextMessageEnd(messageID string) {
	e.eventChan <- StreamEvent{
		Event: events.NewTextMessageEndEvent(messageID),
	}
}

// =============================================================================
// Thinking Events (Extended Thinking / Chain-of-Thought)
// =============================================================================

// ThinkingStart emits a THINKING_START event signaling the beginning of a thinking phase.
func (e *EventEmitter) ThinkingStart(title *string) {
	evt := events.NewThinkingStartEvent()
	if title != nil {
		evt = evt.WithTitle(*title)
	}
	e.eventChan <- StreamEvent{Event: evt}
}

// ThinkingTextMessageStart emits a THINKING_TEXT_MESSAGE_START event.
func (e *EventEmitter) ThinkingTextMessageStart() {
	e.eventChan <- StreamEvent{
		Event: events.NewThinkingTextMessageStartEvent(),
	}
}

// ThinkingTextMessageContent emits a THINKING_TEXT_MESSAGE_CONTENT event with thinking text.
func (e *EventEmitter) ThinkingTextMessageContent(delta string) {
	if delta == "" {
		return // Don't emit empty deltas
	}
	e.eventChan <- StreamEvent{
		Event: events.NewThinkingTextMessageContentEvent(delta),
	}
}

// ThinkingTextMessageEnd emits a THINKING_TEXT_MESSAGE_END event.
func (e *EventEmitter) ThinkingTextMessageEnd() {
	e.eventChan <- StreamEvent{
		Event: events.NewThinkingTextMessageEndEvent(),
	}
}

// ThinkingEnd emits a THINKING_END event signaling the end of a thinking phase.
func (e *EventEmitter) ThinkingEnd() {
	e.eventChan <- StreamEvent{
		Event: events.NewThinkingEndEvent(),
	}
}

// =============================================================================
// Tool Call Events
// =============================================================================

// ToolCallStart emits a TOOL_CALL_START event signaling the beginning of a tool call.
func (e *EventEmitter) ToolCallStart(toolCallID, toolCallName string, parentMessageID *string) {
	var opts []events.ToolCallStartOption
	if parentMessageID != nil {
		opts = append(opts, events.WithParentMessageID(*parentMessageID))
	}
	e.eventChan <- StreamEvent{
		Event: events.NewToolCallStartEvent(toolCallID, toolCallName, opts...),
	}
}

// ToolCallArgs emits a TOOL_CALL_ARGS event with incremental JSON argument chunks.
func (e *EventEmitter) ToolCallArgs(toolCallID, delta string) {
	if delta == "" {
		return // Don't emit empty deltas
	}
	e.eventChan <- StreamEvent{
		Event: events.NewToolCallArgsEvent(toolCallID, delta),
	}
}

// ToolCallEnd emits a TOOL_CALL_END event signaling completion of tool call arguments.
func (e *EventEmitter) ToolCallEnd(toolCallID string) {
	e.eventChan <- StreamEvent{
		Event: events.NewToolCallEndEvent(toolCallID),
	}
}

// ToolCallResult emits a TOOL_CALL_RESULT event with the result of tool execution.
func (e *EventEmitter) ToolCallResult(messageID, toolCallID, content string) {
	e.eventChan <- StreamEvent{
		Event: events.NewToolCallResultEvent(messageID, toolCallID, content),
	}
}

// =============================================================================
// Lifecycle Events
// =============================================================================

// RunStarted emits a RUN_STARTED event signaling the beginning of an agent run.
func (e *EventEmitter) RunStarted(threadID, runID string) {
	e.eventChan <- StreamEvent{
		Event: events.NewRunStartedEvent(threadID, runID),
	}
}

// RunFinished emits a RUN_FINISHED event signaling successful completion of an agent run.
func (e *EventEmitter) RunFinished(threadID, runID string) {
	e.eventChan <- StreamEvent{
		Event: events.NewRunFinishedEvent(threadID, runID),
	}
}

// RunError emits a RUN_ERROR event signaling failure of an agent run.
func (e *EventEmitter) RunError(message string, runID *string) {
	var opts []events.RunErrorOption
	if runID != nil {
		opts = append(opts, events.WithRunID(*runID))
	}
	e.eventChan <- StreamEvent{
		Event: events.NewRunErrorEvent(message, opts...),
	}
}

// StepStarted emits a STEP_STARTED event signaling the beginning of a step.
func (e *EventEmitter) StepStarted(stepName string) {
	e.eventChan <- StreamEvent{
		Event: events.NewStepStartedEvent(stepName),
	}
}

// StepFinished emits a STEP_FINISHED event signaling completion of a step.
func (e *EventEmitter) StepFinished(stepName string) {
	e.eventChan <- StreamEvent{
		Event: events.NewStepFinishedEvent(stepName),
	}
}

// =============================================================================
// Non-AG-UI Events (Meridian-specific)
// =============================================================================

// Block emits a complete block for persistence.
// This is NOT an AG-UI event - it's used internally for block reconstruction.
func (e *EventEmitter) Block(block *Block) {
	e.eventChan <- StreamEvent{Block: block}
}

// Metadata emits final response metadata when streaming completes.
// This is NOT an AG-UI event - it's used internally for completion data.
func (e *EventEmitter) Metadata(metadata *StreamMetadata) {
	e.eventChan <- StreamEvent{Metadata: metadata}
}

// GenerationIDDiscovered emits early generation ID metadata.
// This is NOT an AG-UI event - it's used for cancel enrichment.
func (e *EventEmitter) GenerationIDDiscovered(generationID, model, provider string) {
	e.eventChan <- StreamEvent{
		GenerationIDDiscovered: &GenerationIDEvent{
			GenerationID: generationID,
			Model:        model,
			Provider:     provider,
		},
	}
}

// Error emits an error that occurred during streaming.
func (e *EventEmitter) Error(err error) {
	e.eventChan <- StreamEvent{Error: err}
}

// =============================================================================
// Raw Event Emission
// =============================================================================

// EmitEvent emits any AG-UI event directly.
// Use this for events not covered by the convenience methods above.
func (e *EventEmitter) EmitEvent(event events.Event) {
	e.eventChan <- StreamEvent{Event: event}
}
