package llmprovider

// StreamEvent represents a single event in a streaming response.
// Each event contains either a delta, metadata (completion), or an error.
type StreamEvent struct {
	// Delta contains incremental block content (nil if metadata or error)
	Delta *BlockDelta

	// Metadata contains final response data when streaming completes (nil until end)
	Metadata *StreamMetadata

	// Error contains any error that occurred during streaming (nil if successful)
	Error error
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

	// ResponseMetadata contains provider-specific response data
	ResponseMetadata map[string]interface{}
}
