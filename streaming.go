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

	// GenerationIDDiscovered is a non-terminal metadata event emitted when generation ID is discovered
	// Emitted once per generation (on first chunk), allows early persistence
	// This is separate from Metadata which is the final event
	GenerationIDDiscovered *GenerationIDEvent

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
