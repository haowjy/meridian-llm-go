package llmprovider

// StreamEvent represents a single event in a streaming response.
// Each event contains either a delta, a complete block, metadata (completion), or an error.
type StreamEvent struct {
	// Delta contains incremental block content for real-time UI updates (nil if block/metadata/error)
	Delta *BlockDelta

	// Block contains a complete block when a block finishes streaming (nil if delta/metadata/error)
	// This is emitted once per block when streaming completes for that block.
	// The block is normalized and ready for database persistence.
	Block *Block

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
