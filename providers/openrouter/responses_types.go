package openrouter

import "time"

// ===== OpenRouter Responses API Types =====
// POST /api/v1/responses
// See: https://openrouter.ai/docs/api/api-reference/responses/create-responses
//      https://openrouter.ai/docs/api/reference/responses/basic-usage
//      https://openrouter.ai/docs/api/reference/responses/tool-calling

// ResponsesRequest represents an OpenRouter Responses API request.
// Unlike ChatCompletionRequest, this uses a structured input format
// and returns item-based streaming events.
type ResponsesRequest struct {
	Model              string           `json:"model"`
	Input              []ResponsesInput `json:"input"`
	Stream             bool             `json:"stream,omitempty"`
	MaxOutputTokens    *int             `json:"max_output_tokens,omitempty"`
	Temperature        *float64         `json:"temperature,omitempty"`
	TopP               *float64         `json:"top_p,omitempty"`
	Tools              []ResponsesTool  `json:"tools,omitempty"`
	ToolChoice         interface{}      `json:"tool_choice,omitempty"`
	Reasoning          *ReasoningConfig `json:"reasoning,omitempty"`
	PreviousResponseID *string          `json:"previous_response_id,omitempty"` // For tool-loop continuation
}

// ResponsesInput represents an input item in the Responses API.
// Can be a "message" or "function_call_output".
type ResponsesInput struct {
	Type    string             `json:"type"`              // "message" | "function_call_output"
	Role    string             `json:"role,omitempty"`    // For "message": "user" | "assistant" | "system"
	Content []ResponsesContent `json:"content,omitempty"` // For "message"
	CallID  string             `json:"call_id,omitempty"` // For "function_call_output"
	Output  string             `json:"output,omitempty"`  // For "function_call_output"
}

// ResponsesContent represents content within a message input.
type ResponsesContent struct {
	Type string `json:"type"` // "input_text" | "input_image"
	Text string `json:"text,omitempty"`
	// For images, would add URL/base64 fields
}

// ResponsesTool represents a tool definition in Responses API format.
// Uses FLAT schema: {type, name, description, parameters}
type ResponsesTool struct {
	Type        string                 `json:"type"` // "function"
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	Strict      *bool                  `json:"strict,omitempty"` // null per OpenRouter docs
	Parameters  map[string]interface{} `json:"parameters"`
}

// ===== SSE Event Types for Responses API =====

// ResponsesSSEEvent is a tolerant union type for all Responses API SSE events.
// Uses omitempty everywhere to handle partial/missing fields gracefully.
type ResponsesSSEEvent struct {
	// Event type discriminator
	Type           string `json:"type,omitempty"`
	SequenceNumber int    `json:"sequence_number,omitempty"`

	// Response-level fields
	ResponseID string          `json:"response_id,omitempty"`
	Response   *ResponseObject `json:"response,omitempty"`

	// Output item fields
	OutputIndex  int          `json:"output_index,omitempty"`
	ContentIndex int          `json:"content_index,omitempty"`
	ItemID       string       `json:"item_id,omitempty"`
	Item         *OutputItem  `json:"item,omitempty"`
	Part         *ContentPart `json:"part,omitempty"`

	// Delta fields (text and function call arguments)
	Delta     string `json:"delta,omitempty"`
	Arguments string `json:"arguments,omitempty"`

	// Error handling
	Error *ResponseError `json:"error,omitempty"`
}

// ResponseObject represents the response object in response.created/done events.
type ResponseObject struct {
	ID        string         `json:"id,omitempty"`
	Object    string         `json:"object,omitempty"` // "response"
	CreatedAt int64          `json:"created_at,omitempty"`
	Status    string         `json:"status,omitempty"` // "in_progress" | "completed" | "failed"
	Model     string         `json:"model,omitempty"`
	Output    []OutputItem   `json:"output,omitempty"`
	Usage     *ResponseUsage `json:"usage,omitempty"`
	Error     *ResponseError `json:"error,omitempty"`
}

// OutputItem represents an item in the response output array.
// Can be: message, function_call, or reasoning
type OutputItem struct {
	Type      string                   `json:"type,omitempty"`      // "message" | "function_call" | "reasoning"
	ID        string                   `json:"id,omitempty"`        // Item ID
	CallID    string                   `json:"call_id,omitempty"`   // For function_call: the call identifier used for continuation
	Name      string                   `json:"name,omitempty"`      // For function_call: function name
	Arguments string                   `json:"arguments,omitempty"` // For function_call: may arrive here or in *.done
	Role      string                   `json:"role,omitempty"`      // For message: "assistant"
	Status    string                   `json:"status,omitempty"`    // "in_progress" | "completed"
	Content   []ResponsesOutputContent `json:"content,omitempty"`   // For message: array of content parts
}

// ResponsesOutputContent represents a content part in the output message.
type ResponsesOutputContent struct {
	Type string `json:"type,omitempty"` // "output_text"
	Text string `json:"text,omitempty"`
}

// ResponseUsage represents token usage in the response.
type ResponseUsage struct {
	InputTokens         int           `json:"input_tokens,omitempty"`
	OutputTokens        int           `json:"output_tokens,omitempty"`
	TotalTokens         int           `json:"total_tokens,omitempty"`
	InputTokensDetails  *TokenDetails `json:"input_tokens_details,omitempty"`
	OutputTokensDetails *TokenDetails `json:"output_tokens_details,omitempty"`
}

// TokenDetails provides breakdown of token usage.
type TokenDetails struct {
	CachedTokens    int `json:"cached_tokens,omitempty"`
	ReasoningTokens int `json:"reasoning_tokens,omitempty"`
}

// ResponseError represents an error in the response.
type ResponseError struct {
	Type    string `json:"type,omitempty"`
	Code    string `json:"code,omitempty"`
	Message string `json:"message,omitempty"`
	Param   string `json:"param,omitempty"`
}

// ===== SSE Event Type Constants =====
// These match the event types from OpenRouter Responses API streaming

const (
	// Lifecycle events
	ResponsesEventResponseCreated    = "response.created"
	ResponsesEventResponseInProgress = "response.in_progress"
	ResponsesEventResponseCompleted  = "response.completed"
	ResponsesEventResponseFailed     = "response.failed"
	ResponsesEventResponseDone       = "response.done"

	// Output item events
	ResponsesEventOutputItemAdded = "response.output_item.added"
	ResponsesEventOutputItemDone  = "response.output_item.done"

	// Content part events (for message items)
	ResponsesEventContentPartAdded = "response.content_part.added"
	ResponsesEventContentPartDelta = "response.content_part.delta"
	ResponsesEventContentPartDone  = "response.content_part.done"

	// Function call events
	ResponsesEventFunctionCallArgsDelta = "response.function_call_arguments.delta"
	ResponsesEventFunctionCallArgsDone  = "response.function_call_arguments.done"

	// Reasoning events (for thinking/extended thinking)
	ResponsesEventReasoningDelta        = "response.reasoning.delta"
	ResponsesEventReasoningSummaryDelta = "response.reasoning_summary.delta"
	ResponsesEventReasoningDone         = "response.reasoning.done"

	// Error event
	ResponsesEventError = "error"
)

// ===== Streaming State Tracking =====

// responsesStreamState tracks state during Responses API streaming.
// Uses maps keyed by item/call IDs (not single "current" item) for robust handling.
type responsesStreamState struct {
	// Item tracking (item id -> item info)
	outputItemByID map[string]*OutputItem

	// Tool call args buffer (keyed by call_id for proper continuation)
	callArgsByCallID map[string]*responsesToolCallAccumulator

	// Active items for routing deltas when output_index is used
	// These map output_index -> item_id for content/text routing
	activeMessageItemID   string
	activeReasoningItemID string

	// Response metadata
	responseID string
	model      string
}

// responsesToolCallAccumulator accumulates tool call data during streaming.
type responsesToolCallAccumulator struct {
	ItemID    string // The item ID for this function_call
	CallID    string // The call_id used for continuation (key for this map)
	Name      string // Function name
	Arguments string // Accumulated arguments JSON

	argsProgress         toolArgsProgress
	seenAnyArgs          bool
	firstArgsAt          time.Time
	lastAnyArgsAt        time.Time
	seenMeaningfulArgs   bool
	lastMeaningfulArgsAt time.Time
}

// newResponsesStreamState creates a new streaming state.
func newResponsesStreamState() *responsesStreamState {
	return &responsesStreamState{
		outputItemByID:   make(map[string]*OutputItem),
		callArgsByCallID: make(map[string]*responsesToolCallAccumulator),
	}
}

func (s *responsesStreamState) findStalledToolArgs(now time.Time, timeout time.Duration) (string, *responsesToolCallAccumulator) {
	for callID, acc := range s.callArgsByCallID {
		if acc == nil || !acc.seenAnyArgs || acc.firstArgsAt.IsZero() {
			continue
		}

		if !acc.seenMeaningfulArgs || acc.lastMeaningfulArgsAt.IsZero() {
			if now.Sub(acc.firstArgsAt) >= timeout {
				return callID, acc
			}
			continue
		}

		if now.Sub(acc.lastMeaningfulArgsAt) >= timeout {
			return callID, acc
		}
	}
	return "", nil
}
