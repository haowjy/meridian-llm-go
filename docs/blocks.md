---
detail: minimal
audience: library users
---

# Blocks & Content Model

All LLM content (text, images, tool calls, results) is represented as `Block` objects.

## Message & Block Structure

```go
type Message struct {
    Role   Role      // "user" or "assistant"
    Blocks []*Block  // Content blocks
}

type Block struct {
    BlockType     string                 // Block type (see table below)
    Sequence      int                    // Position in message (0-indexed)
    TextContent   *string                // Text for text/thinking blocks
    Content       map[string]interface{} // Type-specific structured data
    ExecutionSide *ExecutionSide         // For tool_use: "server" or "client"
    Provider      *string                // Provider that created this block
    ProviderData  json.RawMessage        // Raw provider response (if lossy conversion)
    Citations     []Citation             // References to external sources
}
```

## Block Types

### User Blocks

| BlockType | TextContent | Content | Purpose |
|-----------|-------------|---------|---------|
| `text` | Required | null | User's text message |
| `image` | null | Image metadata | User uploads image |
| `document` | null | File metadata | User attaches file |
| `tool_result` | Optional | Result metadata | Tool execution result |

### Assistant Blocks

| BlockType | TextContent | Content | Purpose |
|-----------|-------------|---------|---------|
| `text` | Required | null | Assistant's text response |
| `thinking` | Required | Signature (optional) | Extended thinking/reasoning |
| `tool_use` | null | Tool metadata | Function call |
| `web_search_use` | null | Search metadata | Server-executed web search request |
| `web_search_result` | null | Results or error | Server-executed web search response |

## Block Type Details

### text

Simple text content:

```go
block := &llm.Block{
    BlockType:   llm.BlockTypeText,
    Sequence:    0,
    TextContent: ptr("Hello, world!"),
    Content:     nil,
}
```

### thinking

Extended reasoning with optional signature:

```go
block := &llm.Block{
    BlockType:   llm.BlockTypeThinking,
    Sequence:    0,
    TextContent: ptr("Let me analyze this problem..."),
    Content: map[string]interface{}{
        "signature": "4k_a", // Optional: Extended Thinking signature
    },
}
```

### tool_use

Client-side or server-side tool call:

```go
block := &llm.Block{
    BlockType: llm.BlockTypeToolUse,
    Sequence:  1,
    Content: map[string]interface{}{
        "tool_use_id": "toolu_abc123",
        "tool_name":   "get_weather",
        "input": map[string]interface{}{
            "location": "San Francisco",
            "unit":     "celsius",
        },
    },
    ExecutionSide: ptr(llm.ExecutionSideClient), // Client executes
}
```

Helper methods:
```go
toolUseID, _ := block.GetToolUseID()   // "toolu_abc123"
toolName, _ := block.GetToolName()     // "get_weather"
toolInput, _ := block.GetToolInput()   // {"location": "San Francisco", ...}
```

### tool_result

Result from client-executed tool:

```go
block := &llm.Block{
    BlockType:   llm.BlockTypeToolResult,
    Sequence:    0,
    TextContent: ptr("Temperature: 18°C, Partly cloudy"),
    Content: map[string]interface{}{
        "tool_use_id": "toolu_abc123", // Matches tool_use block
        "is_error":    false,
    },
}
```

Error result:
```go
Content: map[string]interface{}{
    "tool_use_id": "toolu_abc123",
    "is_error":    true,
},
TextContent: ptr("Error: API key invalid"),
```

### web_search_use

Server-executed web search request (LLM → provider):

```go
block := &llm.Block{
    BlockType: llm.BlockTypeWebSearch,
    Sequence:  1,
    Content: map[string]interface{}{
        "tool_use_id": "srvtoolu_abc123",
        "tool_name":   "web_search",
        "input": map[string]interface{}{
            "query": "public domain short poems",
        },
    },
    ExecutionSide: ptr(llm.ExecutionSideServer), // Provider executes
}
```

### web_search_result

Server-executed web search response (provider → LLM):

Success:
```go
block := &llm.Block{
    BlockType: llm.BlockTypeWebSearchResult,
    Sequence:  2,
    Content: map[string]interface{}{
        "tool_use_id": "srvtoolu_abc123",
        "results": []map[string]interface{}{
            {
                "title":    "Public Domain Poetry",
                "url":      "https://www.public-domain-poetry.com/",
                "page_age": "2024-01-15",
            },
        },
        "is_error": false,
    },
}
```

Error:
```go
Content: map[string]interface{}{
    "tool_use_id": "srvtoolu_abc123",
    "is_error":    true,
    "error_code":  "API_ERROR",
}
```

### image

User-uploaded image:

```go
block := &llm.Block{
    BlockType: llm.BlockTypeImage,
    Sequence:  1,
    Content: map[string]interface{}{
        "url":       "https://example.com/image.jpg",
        "mime_type": "image/jpeg",
        "alt_text":  "A beautiful sunset",
    },
}
```

### document

User-attached file:

```go
block := &llm.Block{
    BlockType: llm.BlockTypeDocument,
    Sequence:  1,
    Content: map[string]interface{}{
        "file_id":   "file_abc123",
        "file_uri":  "https://example.com/doc.pdf",
        "mime_type": "application/pdf",
        "title":     "Research Paper",
    },
}
```

## Citations

Text blocks can include citations to sources (e.g., web search results):

```go
type Citation struct {
    Type        string  // "web_search_result", "url_citation", "grounding_support"
    URL         string  // Cited resource URL
    Title       string  // Page title
    StartIndex  *int    // Character position in TextContent where citation starts
    EndIndex    *int    // Character position where citation ends
    CitedText   *string // Exact text that was cited
    ResultIndex *int    // Index in tool_result.Content["results"] array
    Snippet     *string // Preview from source
    ProviderData json.RawMessage // Provider-specific data
}
```

Example with citations:
```go
block := &llm.Block{
    BlockType:   llm.BlockTypeText,
    TextContent: ptr("The sky is blue because of Rayleigh scattering."),
    Citations: []llm.Citation{
        {
            Type:       "web_search_result",
            URL:        "https://example.com/why-sky-blue",
            Title:      "Why Is the Sky Blue?",
            StartIndex: ptr(12),
            EndIndex:   ptr(46),
            CitedText:  ptr("blue because of Rayleigh scattering"),
        },
    },
}
```

## Provider Data Preservation

Blocks may preserve provider-specific data:

```go
block := &llm.Block{
    BlockType: llm.BlockTypeWebSearchResult,
    Provider:  ptr("anthropic"),
    ProviderData: json.RawMessage(`{
        "type": "web_search_tool_result",
        "content": {
            "results": [{
                "encrypted_content": "EqgfCioIARgBIiQ3YTAwMjY1...",
                "title": "Result Title"
            }]
        }
    }`),
    // ... normalized fields
}
```

**When preserved:**
- Provider-specific block types
- Encrypted content that can't be converted
- Special metadata not in normalized schema

**When NOT preserved:**
- Standard blocks (text, thinking, tool_use with normal IDs)
- Data that maps cleanly to normalized format
- Portable client-side tool results

## Block Validation

```go
// Check if block is for user or assistant
isUserBlock := block.IsUserBlock()       // text, image, document, tool_result
isAssistantBlock := block.IsAssistantBlock() // text, thinking, tool_use, web_search*

// Check tool-related blocks
isToolBlock := block.IsToolBlock()       // Any tool-related block
isToolUse := block.IsToolUseBlock()      // Specifically tool_use
isToolResult := block.IsToolResultBlock() // Specifically tool_result

// Check execution side
isServerSide := block.IsServerSideTool()  // Provider executes
isClientSide := block.IsClientSideTool()  // You execute
```

## Creating Blocks

### Text Block

```go
textBlock := &llm.Block{
    BlockType:   llm.BlockTypeText,
    Sequence:    0,
    TextContent: ptr("Hello!"),
}
```

### Tool Use Block

```go
toolUseBlock := &llm.Block{
    BlockType: llm.BlockTypeToolUse,
    Sequence:  0,
    Content: map[string]interface{}{
        "tool_use_id": "toolu_123",
        "tool_name":   "search",
        "input":       map[string]interface{}{"query": "weather"},
    },
}
```

### Tool Result Block

```go
toolResultBlock := &llm.Block{
    BlockType:   llm.BlockTypeToolResult,
    Sequence:    0,
    TextContent: ptr("Sunny, 72°F"),
    Content: map[string]interface{}{
        "tool_use_id": "toolu_123",
        "is_error":    false,
    },
}
```

## Block Constants

```go
const (
    BlockTypeText            = "text"
    BlockTypeThinking        = "thinking"
    BlockTypeToolUse         = "tool_use"
    BlockTypeToolResult      = "tool_result"
    BlockTypeImage           = "image"
    BlockTypeDocument        = "document"
    BlockTypeWebSearch       = "web_search_use"
    BlockTypeWebSearchResult = "web_search_result"
)

const (
    ExecutionSideServer = "server"  // Provider executes
    ExecutionSideClient = "client"  // Consumer executes
)

const (
    RoleUser      = "user"
    RoleAssistant = "assistant"
)
```

## API Reference

**Types:**
- `Message` - Container for role + blocks
- `Block` - Multimodal content unit
- `Citation` - Source reference

**Block helpers:**
- `block.IsUserBlock() bool`
- `block.IsAssistantBlock() bool`
- `block.IsToolBlock() bool`
- `block.IsServerSideTool() bool`
- `block.GetToolUseID() (string, bool)`
- `block.GetToolName() (string, bool)`
- `block.GetToolInput() (map[string]interface{}, bool)`

**See:** `types.go`

## Related

- [streaming.md](streaming.md) - How blocks stream incrementally
- [tools.md](tools.md) - Tool execution patterns
- [providers.md](providers.md) - Provider-specific block mappings

For **backend block extensions** (reference, partial_reference), see:
- `../../_docs/technical/llm/streaming/block-types-reference.md`
