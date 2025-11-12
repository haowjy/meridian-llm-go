package llmprovider

// GenerateRequest contains the parameters for an LLM generation request.
type GenerateRequest struct {
	// Messages contains the conversation history.
	// Each message has a Role (user/assistant) and Blocks.
	Messages []Message

	// Model is the model identifier (e.g., "claude-haiku-4-5-20251001")
	Model string

	// Params contains all request parameters (temperature, max_tokens, thinking settings, etc.)
	// Provider adapters extract what they support from this unified struct.
	Params *RequestParams
}

// Message represents a single message in the conversation.
type Message struct {
	// Role is either "user" or "assistant"
	Role string

	// Blocks is the list of content blocks for this message
	Blocks []*Block
}
