---
detail: minimal
audience: library users
---

# Tools

OpenAI-style function calling with built-in and custom tools.

## Quick Start

**Built-in tools:**
```go
searchTool, _ := llm.NewSearchTool()
bashTool, _ := llm.NewBashTool()
editorTool, _ := llm.NewTextEditorTool()

req := &llm.GenerateRequest{
    Model: "claude-sonnet-4-5",
    Messages: messages,
    Params: &llm.RequestParams{
        Tools: []*llm.Tool{searchTool, bashTool},
    },
}
```

**Custom tools:**
```go
weatherTool, _ := llm.NewCustomTool(
    "get_weather",
    "Get current weather for a location",
    map[string]interface{}{
        "type": "object",
        "properties": map[string]interface{}{
            "location": map[string]interface{}{
                "type":        "string",
                "description": "City name",
            },
        },
        "required": []string{"location"},
    },
)
```

## Tool Structure

All tools use OpenAI-style function format:

```go
type Tool struct {
    Type     string           // Always "function"
    Function FunctionDetails
}

type FunctionDetails struct {
    Name        string
    Description string
    Parameters  map[string]interface{} // JSON Schema object
}
```

Providers automatically convert this to their native format:
- **Anthropic**: `tools: [{type: "computer_20241022", ...}]` or `{name: "web_search_20250305"}`
- **OpenAI**: Uses tools as-is
- **Gemini**: Converts to `FunctionDeclaration`

## Built-in Tools

### Web Search

```go
searchTool, err := llm.NewSearchTool()
```

Provider mapping:
- **Anthropic**: `web_search_20250305` (server-side, encrypted results)
- **Gemini**: `google_search` (server-side)
- **OpenAI**: Model-based search (only certain models)

**Execution:** Server-side (provider executes)

**Portability:** ❌ Results are provider-specific and not portable

### Text Editor

```go
editorTool, err := llm.NewTextEditorTool()
```

Provider mapping:
- **Anthropic**: `TextEditor20250728`

**Execution:** Client-side (you implement)

**Portability:** ✅ Results are portable

### Bash

```go
bashTool, err := llm.NewBashTool()
```

Provider mapping:
- **Anthropic**: `BashTool20250124`

**Execution:** Client-side (you implement)

**Portability:** ✅ Results are portable

## Custom Tools

### Basic Custom Tool

```go
tool, err := llm.NewCustomTool(
    "get_weather",
    "Get current weather for a location",
    map[string]interface{}{
        "type": "object",
        "properties": map[string]interface{}{
            "location": map[string]interface{}{
                "type":        "string",
                "description": "City name",
            },
            "unit": map[string]interface{}{
                "type": "string",
                "enum": []interface{}{"celsius", "fahrenheit"},
            },
        },
        "required": []string{"location"},
    },
)
```

**Execution:** Client-side (you implement execution loop)

**Portability:** ✅ Fully portable across all providers

### Parameters Schema

The `parameters` field follows [JSON Schema](https://json-schema.org/):

```go
map[string]interface{}{
    "type": "object",
    "properties": map[string]interface{}{
        "field_name": map[string]interface{}{
            "type":        "string",                    // string, number, boolean, array, object
            "description": "What this field does",
            "enum":        []interface{}{"a", "b"},     // Optional: limit to specific values
        },
    },
    "required": []string{"field_name"},                 // Optional: required fields
}
```

## Tool Execution

### Server-Side Tools (web_search)

Provider executes, returns results automatically:

```go
// 1. Send request with search tool
resp, err := provider.GenerateResponse(ctx, req)

// 2. Provider handles execution internally

// 3. Response includes tool results
for _, block := range resp.Blocks {
    if block.BlockType == llm.BlockTypeWebSearchResult {
        // Process search results
        results := block.Content["results"]
    }
}
```

You don't execute anything - the provider handles it.

### Client-Side Tools (bash, text_editor, custom)

You must execute and send results back:

```go
// 1. Send request with tools
resp1, _ := provider.GenerateResponse(ctx, req)

// 2. Detect tool_use blocks
for _, block := range resp1.Blocks {
    if block.BlockType == llm.BlockTypeToolUse {
        toolName, _ := block.GetToolName()
        toolInput, _ := block.GetToolInput()
        toolUseID, _ := block.GetToolUseID()

        // 3. Execute tool in your code
        result := executeTool(toolName, toolInput)

        // 4. Create tool_result block
        resultBlock := &llm.Block{
            BlockType: llm.BlockTypeToolResult,
            Content: map[string]interface{}{
                "tool_use_id": toolUseID,
                "is_error":    false,
            },
            TextContent: &result,
        }

        // 5. Send result back in next request
        req2 := &llm.GenerateRequest{
            Model: "claude-sonnet-4-5",
            Messages: append(req.Messages,
                llm.Message{Role: llm.RoleAssistant, Blocks: resp1.Blocks},
                llm.Message{Role: llm.RoleUser, Blocks: []*llm.Block{resultBlock}},
            ),
        }

        // 6. Get final response
        resp2, _ := provider.GenerateResponse(ctx, req2)
    }
}
```

See [streaming.md](streaming.md) for streaming tool execution.

## Tool Choice

Control whether model must use tools:

```go
// Auto (default): Model decides
Params: &llm.RequestParams{
    Tools:      []*llm.Tool{searchTool},
    ToolChoice: &llm.ToolChoice{Mode: llm.ToolChoiceModeAuto},
}

// Required: Must use a tool
ToolChoice: &llm.ToolChoice{Mode: llm.ToolChoiceModeRequired}

// None: Cannot use tools
ToolChoice: &llm.ToolChoice{Mode: llm.ToolChoiceModeNone}

// Specific: Must use this tool
ToolChoice: &llm.ToolChoice{
    Mode:     llm.ToolChoiceModeSpecific,
    ToolName: ptr("get_weather"),
}
```

## Provider Portability

### Portable Tools (✅)

**Client-side built-in tools:**
```go
[]*llm.Tool{llm.NewBashTool(), llm.NewTextEditorTool()}
```
- You execute in your application
- Results are plain text
- Portable across all providers

**Custom tools:**
```go
[]*llm.Tool{customTool}
```
- You execute in your application
- Fully portable

### Non-Portable Tools (❌)

**Server-side built-in tools:**
```go
[]*llm.Tool{llm.NewSearchTool()}
```
- Provider executes and returns encrypted/proprietary data
- **NOT portable** - conversation is locked to that provider
- Switching providers mid-conversation breaks citations

**Why not portable?**

Providers return encrypted content only they can decrypt:

```json
// Anthropic web_search response
{
  "content": [{
    "encrypted_content": "EqgfCioIARgBIiQ3YTAwMjY1...",
    "title": "Result Title"
  }]
}
```

OpenAI/Gemini cannot decrypt Anthropic's encrypted content.

### Provider Data Preservation

The library preserves provider-specific data for debugging:

```go
block := &llm.Block{
    BlockType:    llm.BlockTypeWebSearchResult,
    Provider:     ptr("anthropic"),           // Source provider
    ProviderData: json.RawMessage{...},      // Raw provider response
    // ... normalized fields
}
```

**Check before switching providers:**
```go
for _, block := range turn.Blocks {
    if block.Provider != nil && *block.Provider != newProvider {
        log.Warn("Block contains provider-specific data",
            "block_type", block.BlockType,
            "provider", *block.Provider)
    }
}
```

## Streaming Tool Execution

In streaming mode, tool calls appear as deltas:

```go
stream, _ := provider.GenerateStream(ctx, req)

for event := range stream.Events() {
    if event.Delta != nil {
        switch event.Delta.DeltaType {
        case "tool_call_start":
            // Tool call begins
            toolID := event.Delta.ToolCallID
            toolName := event.Delta.ToolCallName

        case "input_json_delta":
            // Incremental tool input
            jsonDelta := event.Delta.InputJSONDelta
        }
    }

    if event.Block != nil && event.Block.BlockType == llm.BlockTypeToolUse {
        // Complete tool_use block available
        // Execute tool and send result
    }
}
```

See [streaming.md](streaming.md) for complete streaming patterns.

## API Reference

**Factory functions:**
- `NewSearchTool() (*Tool, error)` - Web search tool
- `NewTextEditorTool() (*Tool, error)` - Text editor tool
- `NewBashTool() (*Tool, error)` - Bash execution tool
- `NewCustomTool(name, description, parameters) (*Tool, error)` - Custom tool

**Block helpers:**
- `block.GetToolUseID() (string, bool)` - Extract tool_use_id
- `block.GetToolName() (string, bool)` - Extract tool_name
- `block.GetToolInput() (map[string]interface{}, bool)` - Extract input

**See:** `tools.go`, `tool_types.go`, `types.go`

## Examples

See `examples/` directory:
- `examples/anthropic-basic/` - Basic tool calling
- `examples/anthropic-streaming/` - Streaming with tools
