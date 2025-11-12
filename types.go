package llmprovider

// Block type constants
const (
	BlockTypeText       = "text"
	BlockTypeThinking   = "thinking"   // Claude extended thinking
	BlockTypeToolUse    = "tool_use"
	BlockTypeToolResult = "tool_result"
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
	DeltaTypeTextDelta      = "text_delta"       // Text content delta
	DeltaTypeInputJSONDelta = "input_json_delta" // Tool input JSON delta
)

// BlockDelta represents an incremental update to a block during streaming.
// This is an ephemeral type - deltas are accumulated in memory and never persisted.
//
// Delta flow:
//   1. Provider streams deltas (e.g., Anthropic content_block_delta events)
//   2. Deltas transformed to BlockDelta
//   3. Consumer accumulates deltas in memory
//   4. On block completion, accumulated content becomes a complete Block
type BlockDelta struct {
	// BlockIndex identifies which block this delta belongs to (0-indexed)
	// Matches the Sequence field in Block
	BlockIndex int `json:"block_index"`

	// BlockType indicates the type of block being accumulated
	// Values: "text", "thinking", "tool_use"
	BlockType string `json:"block_type"`

	// DeltaType indicates what kind of delta this is
	// Values: "text_delta", "input_json_delta"
	DeltaType string `json:"delta_type"`

	// TextDelta contains the incremental text content (for text/thinking blocks)
	// Accumulated into Block.TextContent
	TextDelta *string `json:"text_delta,omitempty"`

	// InputJSONDelta contains incremental JSON for tool input (for tool_use blocks)
	// Accumulated into Block.Content["input"]
	InputJSONDelta *string `json:"input_json_delta,omitempty"`

	// ToolUseID is set when a tool_use block starts
	// Stored in Block.Content["tool_use_id"]
	ToolUseID *string `json:"tool_use_id,omitempty"`

	// ToolName is set when a tool_use block starts
	// Stored in Block.Content["tool_name"]
	ToolName *string `json:"tool_name,omitempty"`

	// ThinkingSignature is set when a thinking block starts (e.g., "4k_a")
	// Stored in Block.Content["signature"]
	ThinkingSignature *string `json:"thinking_signature,omitempty"`
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
// Detected by presence of block-specific initialization fields
func (d *BlockDelta) IsBlockStart() bool {
	return d.ToolUseID != nil || d.ToolName != nil || d.ThinkingSignature != nil
}
