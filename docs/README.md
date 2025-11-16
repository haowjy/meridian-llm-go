---
detail: minimal
audience: library users
---

# meridian-llm-go

Multi-provider LLM library with unified API, streaming support, and OpenAI-style function tools.

## Quick Start

```go
import llm "github.com/yourusername/meridian-llm-go"

// Create provider
provider, err := llm.NewAnthropicProvider(apiKey)

// Create request
req := &llm.GenerateRequest{
    Model: "claude-sonnet-4-5",
    Messages: []llm.Message{{
        Role: llm.RoleUser,
        Blocks: []*llm.Block{{
            BlockType: llm.BlockTypeText,
            TextContent: ptr("Hello!"),
        }},
    }},
}

// Get response
resp, err := provider.GenerateResponse(ctx, req)

// Process blocks
for _, block := range resp.Blocks {
    if block.BlockType == llm.BlockTypeText {
        fmt.Println(*block.TextContent)
    }
}
```

## Documentation

### Core Concepts

**[Blocks & Content Model](blocks.md)** - Message/Block structure, block types, content schemas

**[Streaming](streaming.md)** - Real-time SSE streaming with deltas and events

**[Tools](tools.md)** - Function calling with OpenAI-style tools (search, bash, custom tools)

**[Errors](errors.md)** - Unified error handling across providers

**[Capabilities](capabilities.md)** - Provider capability configs (models, pricing, features)

**[Providers](providers.md)** - Supported providers and notable features

### Library Features

- **Unified API** - Same interface across Anthropic, OpenAI, Gemini, OpenRouter
- **Block-centric** - All content (text, images, tools) uses `Block` type
- **Streaming first** - Real-time SSE with deltas and complete blocks
- **OpenAI-style tools** - Universal function calling format
- **Provider portability** - Switch providers without code changes (with caveats)
- **Error normalization** - Consistent error categories and retryability

## Integration

For **Meridian backend integration**, see:
- [Backend integration overview](../../_docs/technical/llm/llm-integration.md)
- [Backend streaming architecture](../../_docs/technical/backend/architecture/streaming-architecture.md)

## Examples

```go
// Streaming
stream, err := provider.GenerateStream(ctx, req)
for event := range stream.Events() {
    if event.Delta != nil {
        // Handle incremental delta
        fmt.Printf("Delta: %+v\n", event.Delta)
    }
    if event.Block != nil {
        // Handle complete block
        fmt.Printf("Block complete: %+v\n", event.Block)
    }
}

// Tools
searchTool, _ := llm.NewSearchTool()
bashTool, _ := llm.NewBashTool()
customTool, _ := llm.NewCustomTool("get_weather",
    "Get weather for a location",
    map[string]interface{}{
        "type": "object",
        "properties": map[string]interface{}{
            "location": map[string]string{"type": "string"},
        },
        "required": []string{"location"},
    })

req.Params = &llm.RequestParams{
    Tools: []*llm.Tool{searchTool, bashTool, customTool},
}
```

## Design Philosophy

1. **Trust providers** - Don't duplicate their validation logic
2. **Fail explicitly** - Clear errors with mitigation strategies
3. **Simple adapters** - Adapters translate formats, not business logic
4. **Portable by default** - Standard blocks work everywhere
5. **Preserve when needed** - Provider-specific data available via ProviderData field

## Supported Providers

| Provider | Status | Special Features |
|----------|--------|------------------|
| **Anthropic** | ✅ Current | web_search, bash, text_editor, thinking |
| **OpenAI** | 🚧 Planned | code_interpreter, model-based search |
| **Gemini** | 🚧 Planned | google_search, url_context, code_execution |
| **OpenRouter** | 🚧 Planned | Plugin system, model routing |

See [providers.md](providers.md) for details.

## Related Documentation

- **Backend integration**: `../../_docs/technical/llm/`
- **Product docs**: `../../_docs/high-level/`
- **Implementation plan**: `../../_docs/hidden/handoffs/llm-provider-unification-plan-v5.md`
