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
	BlockTypeWebSearch       = "web_search_use"    // Server-executed web search invocation (LLM request)
	BlockTypeWebSearchResult = "web_search_result" // Server-executed web search result (provider response)
)

// Citation represents a reference from text content to external sources.
// Used primarily for web search results, but can represent any citation type.
//
// Provider mappings:
// - Anthropic: text.citations[] → Citation (web_search_result_location)
// - Google: groundingSupports[] → Citation (grounding_support)
// - OpenAI/OpenRouter: annotations[] → Citation (url_citation)
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
	// Values: ExecutionSideServer (provider executes), ExecutionSideClient (consumer executes)
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

// IsServerSideTool returns true if this tool is executed server-side
func (b *Block) IsServerSideTool() bool {
	return b.GetExecutionSide() == ExecutionSideServer
}

// IsClientSideTool returns true if this tool is executed client-side
func (b *Block) IsClientSideTool() bool {
	return b.GetExecutionSide() == ExecutionSideClient
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
// Server-side tool blocks can only be replayed to their original provider.
func (b *Block) CanReplayToProvider(targetProvider ProviderID) bool {
	// Non-tool blocks are always replayable
	if b.BlockType != BlockTypeToolUse {
		return true
	}

	// Client-side tools are replayable across providers
	if b.GetExecutionSide() == ExecutionSideClient {
		return true
	}

	// Server-side tools can only replay to same provider
	return b.IsFromProvider(targetProvider)
}

// Delta type constants for streaming events
const (
	DeltaTypeText          = "text_delta"       // Regular text content
	DeltaTypeThinking      = "thinking_delta"   // Thinking/reasoning text
	DeltaTypeSignature     = "signature_delta"  // Cryptographic signature (Anthropic/Gemini Extended Thinking)
	DeltaTypeToolCallStart = "tool_call_start"  // Tool call initiated (name, id)
	DeltaTypeToolResult    = "tool_result_start" // Tool result arriving (server or client-side)
	DeltaTypeJSON          = "json_delta"       // Incremental JSON content (tool input, tool results, etc.)
	DeltaTypeUsage         = "usage_delta"      // Token usage updates

	// Legacy aliases for backwards compatibility
	DeltaTypeTextDelta      = DeltaTypeText
	DeltaTypeInputJSONDelta = DeltaTypeJSON // DEPRECATED: use DeltaTypeJSON
)

// BlockDelta represents an incremental update to a block during streaming.
// This is an ephemeral type - deltas are accumulated in memory and never persisted.
//
// Delta flow:
//  1. Provider streams deltas (e.g., Anthropic content_block_delta events)
//  2. Deltas transformed to BlockDelta
//  3. Consumer accumulates deltas in memory
//  4. On block completion, accumulated content becomes a complete Block
//
// BlockType is optional and signals block starts:
//   - Set on first delta for a block (acts as block_start signal)
//   - Nil on subsequent deltas for the same block
//   - Allows consumer to detect new blocks without separate events
type BlockDelta struct {
	// BlockIndex identifies which block this delta belongs to (0-indexed)
	// Matches the Sequence field in Block
	BlockIndex int `json:"block_index"`

	// BlockType indicates the type of block being accumulated
	// Values: "text", "thinking", "tool_use"
	// OPTIONAL: Only set on first delta for a block (signals block start)
	BlockType *string `json:"block_type,omitempty"`

	// DeltaType indicates what kind of delta this is
	// Values: "text_delta", "thinking_delta", "signature_delta",
	//         "tool_call_start", "input_json_delta", "usage_delta"
	DeltaType string `json:"delta_type"`

	// === Content Deltas ===

	// TextDelta contains incremental text content (text or thinking blocks)
	// Accumulated into Block.TextContent
	TextDelta *string `json:"text_delta,omitempty"`

	// SignatureDelta contains incremental cryptographic signature
	// (Anthropic/Gemini Extended Thinking only)
	// Accumulated into Block.Content["signature"]
	SignatureDelta *string `json:"signature_delta,omitempty"`

	// JSONDelta contains incremental JSON content
	// For tool_use blocks: accumulated into Block.Content["input"]
	// For tool_result blocks: accumulated into Block.Content["result"]
	// For other structured blocks: accumulated into appropriate Content field
	JSONDelta *string `json:"json_delta,omitempty"`

	// === Tool Call Metadata ===

	// ToolCallID identifies the tool call (set on tool_call_start)
	// Stored in Block.Content["id"]
	ToolCallID *string `json:"tool_call_id,omitempty"`

	// ToolCallName is the function name (set on tool_call_start)
	// Stored in Block.Content["name"]
	ToolCallName *string `json:"tool_call_name,omitempty"`

	// === Legacy Fields (for backwards compatibility) ===

	// ToolUseID is DEPRECATED, use ToolCallID instead
	// Stored in Block.Content["tool_use_id"]
	ToolUseID *string `json:"tool_use_id,omitempty"`

	// ToolName is DEPRECATED, use ToolCallName instead
	// Stored in Block.Content["tool_name"]
	ToolName *string `json:"tool_name,omitempty"`

	// ThinkingSignature is DEPRECATED, use SignatureDelta with DeltaTypeSignature
	// Stored in Block.Content["signature"]
	ThinkingSignature *string `json:"thinking_signature,omitempty"`

	// === Usage Metadata ===

	// InputTokens contains input/prompt token count
	// Accumulated at Turn level (not Block level)
	InputTokens *int `json:"input_tokens,omitempty"`

	// OutputTokens contains output/completion token count
	// Accumulated at Turn level (not Block level)
	OutputTokens *int `json:"output_tokens,omitempty"`

	// ThinkingTokens contains thinking-specific token count (Gemini)
	// Stored in Turn.ResponseMetadata["thinking_tokens"]
	ThinkingTokens *int `json:"thinking_tokens,omitempty"`
}

// IsTextDelta returns true if this delta contains text content
func (d *BlockDelta) IsTextDelta() bool {
	return d.DeltaType == DeltaTypeTextDelta && d.TextDelta != nil
}

// IsJSONDelta returns true if this delta contains JSON content
func (d *BlockDelta) IsJSONDelta() bool {
	return d.DeltaType == DeltaTypeJSON && d.JSONDelta != nil
}

// IsInputJSONDelta is DEPRECATED, use IsJSONDelta instead
func (d *BlockDelta) IsInputJSONDelta() bool {
	return d.IsJSONDelta()
}

// IsBlockStart returns true if this delta signals the start of a new block
// Detected by BlockType field being set (non-nil)
func (d *BlockDelta) IsBlockStart() bool {
	return d.BlockType != nil
}

// IsSignatureDelta returns true if this delta contains signature content
func (d *BlockDelta) IsSignatureDelta() bool {
	return d.DeltaType == DeltaTypeSignature && d.SignatureDelta != nil
}

// IsUsageDelta returns true if this delta contains token usage updates
func (d *BlockDelta) IsUsageDelta() bool {
	return d.DeltaType == DeltaTypeUsage &&
		(d.InputTokens != nil || d.OutputTokens != nil || d.ThinkingTokens != nil)
}
