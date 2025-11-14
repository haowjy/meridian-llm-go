package llmprovider

// Block type constants
const (
	BlockTypeText       = "text"
	BlockTypeThinking   = "thinking" // Claude extended thinking
	BlockTypeToolUse    = "tool_use"
	BlockTypeToolResult = "tool_result" // Result is the result sent back from the tool call
	BlockTypeImage      = "image"
	BlockTypeDocument   = "document" // Provider file uploads (Anthropic/Gemini)
)

// Block represents a multimodal content block.
// This is a content-only type with no database fields.
//
// User blocks: text, image, tool_result, document
// Assistant blocks: text, thinking, tool_use
//
// The Content field stores block-type-specific structured data as a map:
// - text: empty (text in TextContent field)
// - thinking: {"signature": "4k_a"} (optional, text in TextContent)
// - tool_use: {"tool_use_id": "toolu_...", "tool_name": "...", "input": {...}}
// - tool_result: {"tool_use_id": "toolu_...", "is_error": false}
// - image: {"url": "...", "mime_type": "...", "alt_text": "..."}
// - document: {"file_id": "...", "file_uri": "...", "mime_type": "...", "title": "...", "context": "..."}
type Block struct {
	// BlockType indicates the type of block
	// Values: "text", "thinking", "tool_use", "tool_result", "image", "document"
	BlockType string `json:"block_type"`

	// Sequence indicates the position of this block in the turn (0-indexed)
	Sequence int `json:"sequence"`

	// TextContent contains the text for text/thinking blocks
	TextContent *string `json:"text_content,omitempty"`

	// Content contains type-specific structured data
	Content map[string]interface{} `json:"content,omitempty"`
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
		b.BlockType == BlockTypeToolUse
}

// IsToolBlock returns true if this is a tool-related block
func (b *Block) IsToolBlock() bool {
	return b.BlockType == BlockTypeToolUse || b.BlockType == BlockTypeToolResult
}

// Delta type constants for streaming events
const (
	DeltaTypeText          = "text_delta"       // Regular text content
	DeltaTypeThinking      = "thinking_delta"   // Thinking/reasoning text
	DeltaTypeSignature     = "signature_delta"  // Cryptographic signature (Anthropic/Gemini Extended Thinking)
	DeltaTypeToolCallStart = "tool_call_start"  // Tool call initiated (name, id)
	DeltaTypeInputJSON     = "input_json_delta" // Incremental tool input JSON
	DeltaTypeUsage         = "usage_delta"      // Token usage updates

	// Legacy aliases for backwards compatibility
	DeltaTypeTextDelta      = DeltaTypeText
	DeltaTypeInputJSONDelta = DeltaTypeInputJSON
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

	// InputJSONDelta contains incremental JSON for tool input (tool_use blocks)
	// Accumulated into Block.Content["input"]
	InputJSONDelta *string `json:"input_json_delta,omitempty"`

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

// IsInputJSONDelta returns true if this delta contains tool input JSON
func (d *BlockDelta) IsInputJSONDelta() bool {
	return d.DeltaType == DeltaTypeInputJSONDelta && d.InputJSONDelta != nil
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
