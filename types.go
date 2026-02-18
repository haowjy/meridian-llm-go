package llmprovider

import "encoding/json"

// Block type constants
const (
	BlockTypeText            = "text"
	BlockTypeThinking        = "thinking"         // Claude extended thinking
	BlockTypeToolUse         = "tool_use"
	BlockTypeToolResult      = "tool_result"      // Result sent back from client-executed tool call
	BlockTypeImage           = "image"
	BlockTypeDocument        = "document"          // Provider file uploads (Anthropic/Gemini)
	BlockTypeWebSearch       = "web_search_use"    // Provider-executed web search invocation (LLM request)
	BlockTypeWebSearchResult = "web_search_result" // Provider-executed web search result (provider response)
)

// Citation represents a reference from text content to external sources.
// Used primarily for web search results, but can represent any citation type.
//
// Provider mappings:
// - Anthropic: text.citations[] -> Citation (web_search_result_location)
// - Google: groundingSupports[] -> Citation (grounding_support)
// - OpenAI/OpenRouter: annotations[] -> Citation (url_citation)
type Citation struct {
	// Type indicates the citation type
	// Values: "web_search_result", "url_citation", "grounding_support"
	Type string `json:"type"`

	// URL is the cited resource URL
	URL string `json:"url"`

	// Title is the page/resource title
	Title string `json:"title"`

	// StartIndex is the character position in TextContent where citation starts (optional)
	StartIndex *int `json:"start_index,omitempty"`

	// EndIndex is the character position in TextContent where citation ends (optional)
	EndIndex *int `json:"end_index,omitempty"`

	// CitedText is the exact text that was cited (optional)
	CitedText *string `json:"cited_text,omitempty"`

	// ResultIndex points to the index in the tool_result.Content["results"] array (optional)
	// Used to link citations back to search results
	ResultIndex *int `json:"result_index,omitempty"`

	// Snippet is a preview/excerpt from the cited source (optional)
	Snippet *string `json:"snippet,omitempty"`

	// ProviderData stores provider-specific citation data
	// Examples: Anthropic's encrypted_index, Google's grounding confidence scores
	ProviderData json.RawMessage `json:"provider_data,omitempty"`
}

// Block represents a multimodal content block.
// This is a content-only type with no database fields.
//
// User blocks: text, image, tool_result, document
// Assistant blocks: text, thinking, tool_use, web_search, web_search_result
//
// The Content field stores block-type-specific structured data as a map:
// - text: empty (text in TextContent field)
// - thinking: {"signature": "4k_a"} (optional, text in TextContent)
// - tool_use: {"tool_use_id": "toolu_...", "tool_name": "...", "input": {...}}
// - tool_result: {"tool_use_id": "toolu_...", "is_error": false}
// - web_search: {"tool_use_id": "toolu_...", "tool_name": "web_search", "input": {...}}
// - web_search_result: {"tool_use_id": "toolu_...", "results": [{title, url, page_age}]} or {"tool_use_id": "...", "is_error": true, "error_code": "..."}
// - image: {"url": "...", "mime_type": "...", "alt_text": "..."}
// - document: {"file_id": "...", "file_uri": "...", "mime_type": "...", "title": "...", "context": "..."}
type Block struct {
	// BlockType indicates the type of block
	// Values: "text", "thinking", "tool_use", "tool_result", "image", "document", "web_search", "web_search_result"
	BlockType string `json:"block_type"`

	// Sequence indicates the position of this block in the turn (0-indexed)
	Sequence int `json:"sequence"`

	// TextContent contains the text for text/thinking blocks
	TextContent *string `json:"text_content,omitempty"`

	// Content contains type-specific structured data
	Content map[string]interface{} `json:"content,omitempty"`

	// ExecutionSide indicates where tool execution happens (for tool_use blocks)
	// Values: ExecutionSideProvider (LLM provider), ExecutionSideLocal (non-provider)
	// Defaults to ExecutionSideLocal if empty
	// Only relevant for tool_use blocks
	ExecutionSide *ExecutionSide `json:"execution_side,omitempty"`

	// Provider identifies which LLM provider generated this block
	// Values: "anthropic", "openai", "gemini", etc.
	// Only populated when block contains provider-specific data that can't be converted
	Provider *string `json:"provider,omitempty"`

	// ProviderData stores the raw provider-specific response for this block
	// Only populated when our normalized format loses information (lossy conversion)
	// Examples:
	// - Anthropic's encrypted web_search results (can't be decrypted by other providers)
	// - Provider-specific metadata not in our standard schema
	// - Special block types that don't map cleanly to our normalized types
	//
	// Standard portable data (text, tool_use_id, tool_name, input) stays in normalized fields.
	// This field is for preservation, not primary access.
	ProviderData json.RawMessage `json:"provider_data,omitempty"`

	// Citations contains references to external sources (primarily for text blocks)
	// Used when text content references web search results or other sources
	// Examples:
	// - Anthropic: text.citations[] for web_search grounding
	// - Google: groundingSupports for Gemini grounding
	// - OpenAI/OpenRouter: annotations for cited sources
	Citations []Citation `json:"citations,omitempty"`
}

// GetExecutionSide returns the execution side, or empty string if not set
func (b *Block) GetExecutionSide() ExecutionSide {
	if b.ExecutionSide == nil {
		return ""
	}
	return *b.ExecutionSide
}

// SetExecutionSide sets the execution side for this block
func (b *Block) SetExecutionSide(side ExecutionSide) {
	b.ExecutionSide = &side
}

// IsUserBlock returns true if this is a user turn block
func (b *Block) IsUserBlock() bool {
	return b.BlockType == BlockTypeText ||
		b.BlockType == BlockTypeImage ||
		b.BlockType == BlockTypeDocument ||
		b.BlockType == BlockTypeToolResult
}

// IsAssistantBlock returns true if this is an assistant turn block
func (b *Block) IsAssistantBlock() bool {
	return b.BlockType == BlockTypeText ||
		b.BlockType == BlockTypeThinking ||
		b.BlockType == BlockTypeToolUse ||
		b.BlockType == BlockTypeWebSearch ||
		b.BlockType == BlockTypeWebSearchResult
}

// IsToolBlock returns true if this is a tool-related block
func (b *Block) IsToolBlock() bool {
	return b.BlockType == BlockTypeToolUse ||
		b.BlockType == BlockTypeToolResult ||
		b.BlockType == BlockTypeWebSearch ||
		b.BlockType == BlockTypeWebSearchResult
}

// IsToolUseBlock returns true if this is a tool_use block
func (b *Block) IsToolUseBlock() bool {
	return b.BlockType == BlockTypeToolUse
}

// IsToolResultBlock returns true if this is a tool_result block
func (b *Block) IsToolResultBlock() bool {
	return b.BlockType == BlockTypeToolResult
}

// IsProviderSideTool returns true if this tool is executed provider-side (e.g., Anthropic's web_search)
func (b *Block) IsProviderSideTool() bool {
	return b.GetExecutionSide() == ExecutionSideProvider
}

// IsLocalTool returns true if this tool requires non-provider execution (stop/execute/resume cycle)
// Treats empty ExecutionSide as local (default)
func (b *Block) IsLocalTool() bool {
	side := b.GetExecutionSide()
	return side == ExecutionSideLocal || side == ""
}

// GetToolUseID returns the tool_use_id from a tool_use or tool_result block
func (b *Block) GetToolUseID() (string, bool) {
	if !b.IsToolBlock() {
		return "", false
	}
	id, ok := b.Content["tool_use_id"].(string)
	return id, ok
}

// GetToolName returns the tool_name from a tool_use block
func (b *Block) GetToolName() (string, bool) {
	if !b.IsToolUseBlock() {
		return "", false
	}
	name, ok := b.Content["tool_name"].(string)
	return name, ok
}

// GetToolInput returns the input from a tool_use block
func (b *Block) GetToolInput() (map[string]interface{}, bool) {
	if !b.IsToolUseBlock() {
		return nil, false
	}
	input, ok := b.Content["input"].(map[string]interface{})
	return input, ok
}

// IsFromDifferentProvider returns true if this block was created by a different provider
func (b *Block) IsFromDifferentProvider(currentProvider ProviderID) bool {
	return b.Provider != nil && *b.Provider != "" && *b.Provider != currentProvider.String()
}

// IsFromProvider returns true if this block was created by the specified provider
func (b *Block) IsFromProvider(provider ProviderID) bool {
	return b.Provider != nil && *b.Provider == provider.String()
}

// HasProviderData returns true if this block has raw provider-specific data
func (b *Block) HasProviderData() bool {
	return len(b.ProviderData) > 0
}

// CanReplayToProvider returns true if this block can be safely replayed to the given provider.
// Provider-side tool blocks can only be replayed to their original provider.
// Local tools (backend/client executed) are replayable across providers.
func (b *Block) CanReplayToProvider(targetProvider ProviderID) bool {
	// Non-tool blocks are always replayable
	if b.BlockType != BlockTypeToolUse {
		return true
	}

	// Local tools are replayable across providers
	side := b.GetExecutionSide()
	if side == ExecutionSideLocal || side == "" {
		return true
	}

	// Provider-side tools can only replay to same provider
	return b.IsFromProvider(targetProvider)
}

