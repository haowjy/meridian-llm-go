package llmprovider

// GenerateResponse contains the LLM provider's response.
type GenerateResponse struct {
	// Blocks is the list of content blocks returned by the provider
	Blocks []*Block

	// Model is the model that was used (may differ from request if aliased)
	Model string

	// InputTokens is the number of tokens in the input
	InputTokens int

	// OutputTokens is the number of tokens in the output
	OutputTokens int

	// StopReason indicates why generation stopped (e.g., "end_turn", "max_tokens")
	StopReason string

	// ResponseMetadata contains provider-specific response data
	// Examples: stop_sequence, cache_creation_input_tokens, cache_read_input_tokens, etc.
	ResponseMetadata map[string]interface{}
}
