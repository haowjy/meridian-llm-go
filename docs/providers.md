---
detail: minimal
audience: library users
---

# Providers

Supported LLM providers and their notable features.

## Supported Providers

| Provider | Status | Models | Special Features |
|----------|--------|--------|------------------|
| **Anthropic** | ✅ Current | Claude Sonnet/Opus/Haiku 4.x | web_search, bash, text_editor, thinking |
| **OpenAI** | 🚧 Planned | GPT-4o, GPT-4o-mini | code_interpreter, model-based search |
| **Gemini** | 🚧 Planned | Gemini 2.5 Pro/Flash | google_search, url_context, code_execution |
| **OpenRouter** | 🚧 Planned | All proxied models | Plugin system, model routing |

## Anthropic

**Status:** ✅ Fully supported

**Models:**
- `claude-sonnet-4-5-20250929` (recommended)
- `claude-opus-4-1-20250805`
- `claude-haiku-4-5-20251001`

**Features:**
- Extended thinking (with signature verification)
- Server-side web search (`web_search_20250305`)
- Client-side bash execution (`BashTool20250124`)
- Client-side text editor (`TextEditor20250728`)
- Vision support (images up to 1568px)
- Streaming with SSE
- Prompt caching

**Notable:**
- Web search results are encrypted (not portable across providers)
- Thinking budget: 1024-200000 tokens

**Example:**
```go
provider, err := llm.NewAnthropicProvider(apiKey)

req := &llm.GenerateRequest{
    Model: "claude-sonnet-4-5",
    Messages: messages,
    Params: &llm.RequestParams{
        ThinkingEffort: llm.ThinkingEffortMedium,
        Tools: []*llm.Tool{
            llm.NewSearchTool(),
            llm.NewBashTool(),
        },
    },
}

resp, err := provider.GenerateResponse(ctx, req)
```

**Docs:** https://docs.anthropic.com/

---

## OpenAI

**Status:** 🚧 Planned

**Models (planned):**
- `gpt-4o`
- `gpt-4o-mini`
- `gpt-4o-search-preview` (model-based search)

**Features (planned):**
- Code interpreter (Python execution, server-side)
- Model-based search (specific models only)
- Vision support
- Streaming with SSE
- Structured outputs (JSON mode)

**Notable:**
- Search is model-based (use search-preview models)
- Code interpreter: $0.03 per session

**Docs:** https://platform.openai.com/docs

---

## Gemini

**Status:** 🚧 Planned

**Models (planned):**
- `gemini-2.5-pro`
- `gemini-2.5-flash`

**Features (planned):**
- Google Search (server-side, grounding support)
- URL context (fetch and parse web pages, server-side)
- Code execution (Python, server-side, 65+ libraries)
- Vision support
- Streaming
- Grounding with citations

**Notable:**
- URL context: Up to 20 URLs, 34MB per URL
- Code execution: 30s timeout, 2MB file limit
- Dynamic thinking budgets

**Docs:** https://ai.google.dev/gemini-api/docs

---

## OpenRouter

**Status:** 🚧 Planned

**Models (planned):**
- All proxied models from Anthropic, OpenAI, Google, etc.

**Features (planned):**
- Plugin system for tools (Exa search, native search)
- Model routing (fallback, load balancing)
- Unified API across all models

**Notable:**
- Acts as proxy to other providers
- Plugin-based tool support
- Variable pricing based on underlying provider

**Docs:** https://openrouter.ai/docs

---

## Provider Comparison

### Tool Support

| Tool | Anthropic | OpenAI | Gemini | OpenRouter |
|------|-----------|--------|--------|------------|
| **Web Search** | ✅ `web_search` (encrypted) | ✅ Model-based | ✅ `google_search` | ✅ Plugin |
| **Code Execution** | ✅ Bash (client) | ✅ Python (server) | ✅ Python (server) | Varies |
| **URL Fetching** | ❌ | ❌ | ✅ `url_context` | Varies |
| **Custom Tools** | ✅ | ✅ | ✅ | ✅ |

### Thinking Support

| Provider | Type | Budget Range |
|----------|------|--------------|
| **Anthropic** | Token budget | 1024-200000 |
| **OpenAI** | Effort level | low/medium/high |
| **Gemini** | Token budget | 0-32768 (dynamic) |

### Streaming

All providers support streaming with the library's unified `StreamEvent` format:

```go
stream, _ := provider.GenerateStream(ctx, req)

for event := range stream.Events() {
    // Same handling for all providers
    if event.Delta != nil { /* ... */ }
    if event.Block != nil { /* ... */ }
}
```

---

## Creating Providers

### Anthropic

```go
import llm "github.com/yourusername/meridian-llm-go"

provider, err := llm.NewAnthropicProvider(apiKey)
if err != nil {
    log.Fatal(err)
}
```

### OpenAI (planned)

```go
provider, err := llm.NewOpenAIProvider(apiKey)
```

### Gemini (planned)

```go
provider, err := llm.NewGeminiProvider(apiKey)
```

### OpenRouter (planned)

```go
provider, err := llm.NewOpenRouterProvider(apiKey)
```

---

## Provider Switching

### Portable Blocks

Most blocks work across providers:

```go
// These blocks are portable
blocks := []*llm.Block{
    {BlockType: llm.BlockTypeText, TextContent: ptr("Hello")},
    {BlockType: llm.BlockTypeThinking, TextContent: ptr("...")},
    {BlockType: llm.BlockTypeToolUse, /* custom tool */},
    {BlockType: llm.BlockTypeToolResult, /* client tool result */},
}

// Can replay with any provider
resp, _ := newProvider.GenerateResponse(ctx, &llm.GenerateRequest{
    Messages: []llm.Message{{Role: llm.RoleAssistant, Blocks: blocks}},
})
```

### Non-Portable Blocks

Server-side tool results are NOT portable:

```go
// Check for provider-specific blocks
for _, block := range blocks {
    if block.Provider != nil {
        log.Warn("Block contains provider-specific data",
            "type", block.BlockType,
            "provider", *block.Provider)
        // Don't replay with different provider
    }
}
```

Examples of non-portable blocks:
- `web_search_result` (encrypted citations)
- `code_exec_result` (server-side execution state)

---

## Provider-Specific Features

### Anthropic Extended Thinking

```go
req.Params = &llm.RequestParams{
    ThinkingEffort: llm.ThinkingEffortHigh, // 15000 tokens
}

// Response includes signature
for _, block := range resp.Blocks {
    if block.BlockType == llm.BlockTypeThinking {
        if sig, ok := block.Content["signature"]; ok {
            log.Printf("Thinking signature: %s", sig)
        }
    }
}
```

### Anthropic Prompt Caching

```go
// Mark messages for caching (Anthropic-specific)
req.Params = &llm.RequestParams{
    CacheControl: &llm.CacheControl{
        Type: "ephemeral",
    },
}
```

### OpenAI JSON Mode (planned)

```go
req.Params = &llm.RequestParams{
    ResponseFormat: &llm.ResponseFormat{
        Type: "json_object",
    },
}
```

---

## API Reference

**Provider interface:**
```go
type Provider interface {
    GenerateResponse(ctx context.Context, req *GenerateRequest) (*Response, error)
    GenerateStream(ctx context.Context, req *GenerateRequest) (*Stream, error)
}
```

**Factory functions:**
- `NewAnthropicProvider(apiKey string) (Provider, error)`
- `NewOpenAIProvider(apiKey string) (Provider, error)` (planned)
- `NewGeminiProvider(apiKey string) (Provider, error)` (planned)
- `NewOpenRouterProvider(apiKey string) (Provider, error)` (planned)

**See:** `provider.go`, `providers/*/provider.go`

---

## Related

- [capabilities.md](capabilities.md) - Provider capability configs
- [tools.md](tools.md) - Tool support per provider
- [errors.md](errors.md) - Provider error mappings

For **provider adapter architecture**, see:
- `../../_docs/technical/llm/architecture.md`
