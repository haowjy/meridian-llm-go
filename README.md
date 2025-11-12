# meridian-llm-go

A minimal, provider-agnostic Go library for multi-LLM support with streaming-first design.

## Features

- **Universal Provider Interface**: Support for multiple LLM providers (Anthropic, OpenAI, Gemini, etc.)
- **Streaming-First**: Native SSE streaming support for real-time responses
- **Type-Safe**: Strongly typed content blocks and parameters
- **Typed Error Handling**: Programmatic error detection (rate limits, auth errors, invalid requests)
- **Minimal Dependencies**: Only provider SDKs required
- **Extensible**: Easy to add custom providers
- **Content-Only Types**: No database fields - pure content models

## Supported Providers

- ✅ **Anthropic** (Claude) - Full support with thinking mode
- ✅ **Lorem** - Mock provider for testing
- 🚧 **OpenAI** - Coming soon
- 🚧 **Gemini** - Coming soon
- 🚧 **OpenRouter** - Coming soon

## Installation

```bash
go get github.com/haowjy/meridian-llm-go
```

## Quick Start

### Non-Streaming (Blocking)

```go
package main

import (
    "context"
    "fmt"

    "github.com/haowjy/meridian-llm-go"
    "github.com/haowjy/meridian-llm-go/providers/anthropic"
)

func main() {
    // Create provider
    provider, err := anthropic.NewProvider("your-api-key")
    if err != nil {
        panic(err)
    }

    // Build request
    req := &llmprovider.GenerateRequest{
        Model: "claude-haiku-4-5-20251001",
        Messages: []llmprovider.Message{
            {
                Role: "user",
                Blocks: []*llmprovider.Block{
                    {
                        BlockType:   llmprovider.BlockTypeText,
                        Sequence:    0,
                        TextContent: strPtr("Hello, Claude!"),
                    },
                },
            },
        },
        Params: &llmprovider.RequestParams{
            MaxTokens:   intPtr(1024),
            Temperature: floatPtr(0.7),
        },
    }

    // Generate response
    resp, err := provider.GenerateResponse(context.Background(), req)
    if err != nil {
        panic(err)
    }

    // Print response
    for _, block := range resp.Blocks {
        if block.TextContent != nil {
            fmt.Println(*block.TextContent)
        }
    }
}

func strPtr(s string) *string    { return &s }
func intPtr(i int) *int          { return &i }
func floatPtr(f float64) *float64 { return &f }
```

### Streaming

```go
package main

import (
    "context"
    "fmt"

    "github.com/haowjy/meridian-llm-go"
    "github.com/haowjy/meridian-llm-go/providers/anthropic"
)

func main() {
    provider, _ := anthropic.NewProvider("your-api-key")

    req := &llmprovider.GenerateRequest{
        Model: "claude-haiku-4-5-20251001",
        Messages: []llmprovider.Message{ /* same as above */ },
        Params: &llmprovider.RequestParams{
            MaxTokens: intPtr(1024),
        },
    }

    // Start streaming
    eventChan, err := provider.StreamResponse(context.Background(), req)
    if err != nil {
        panic(err)
    }

    // Read events
    for event := range eventChan {
        if event.Error != nil {
            fmt.Println("Error:", event.Error)
            break
        }

        if event.Delta != nil {
            // Print text deltas as they arrive
            if event.Delta.TextDelta != nil {
                fmt.Print(*event.Delta.TextDelta)
            }
        }

        if event.Metadata != nil {
            // Streaming complete
            fmt.Printf("\n\nTokens: %d in, %d out\n",
                event.Metadata.InputTokens,
                event.Metadata.OutputTokens)
        }
    }
}
```

### Mock Provider (Lorem)

Perfect for testing without API keys:

```go
import "github.com/haowjy/meridian-llm-go/providers/lorem"

provider := lorem.NewProvider()

req := &llmprovider.GenerateRequest{
    Model: "lorem-fast",  // Options: lorem-fast, lorem-slow, lorem-cutoff
    Messages: /* ... */,
}

resp, _ := provider.GenerateResponse(context.Background(), req)
```

## Core Types

### Provider Interface

```go
type Provider interface {
    GenerateResponse(ctx context.Context, req *GenerateRequest) (*GenerateResponse, error)
    StreamResponse(ctx context.Context, req *GenerateRequest) (<-chan StreamEvent, error)
    Name() string
    SupportsModel(model string) bool
}
```

### Request/Response

```go
type GenerateRequest struct {
    Messages []Message      // Conversation history
    Model    string         // Model identifier
    Params   *RequestParams // All provider params
}

type GenerateResponse struct {
    Blocks           []*Block
    Model            string
    InputTokens      int
    OutputTokens     int
    StopReason       string
    ResponseMetadata map[string]interface{}
}
```

### Content Blocks

```go
type Block struct {
    BlockType   string                 // "text", "thinking", "tool_use", etc.
    Sequence    int                    // Position in turn
    TextContent *string                // For text/thinking blocks
    Content     map[string]interface{} // Type-specific data
}
```

Supported block types:
- `text` - Plain text content
- `thinking` - Claude extended thinking (signature optional)
- `tool_use` - Tool/function calls
- `tool_result` - Tool execution results
- `image` - Images (URL or base64)
- `document` - Provider file uploads

### Streaming

```go
type StreamEvent struct {
    Delta    *BlockDelta      // Incremental content
    Metadata *StreamMetadata  // Final metadata (when complete)
    Error    error            // Any errors
}

type BlockDelta struct {
    BlockIndex        int
    BlockType         string
    DeltaType         string  // "text_delta", "input_json_delta"
    TextDelta         *string
    InputJSONDelta    *string
    ToolUseID         *string
    ToolName          *string
    ThinkingSignature *string
}
```

## RequestParams (Unified)

The library provides a unified `RequestParams` struct that supports parameters from all providers:

```go
type RequestParams struct {
    // Core (most providers)
    MaxTokens   *int
    Temperature *float64
    TopP        *float64
    TopK        *int
    Stop        []string
    System      *string

    // Anthropic-specific
    ThinkingEnabled *bool
    ThinkingLevel   *string  // "low", "medium", "high"

    // OpenAI-specific
    FrequencyPenalty *float64
    PresencePenalty  *float64
    ResponseFormat   *ResponseFormat

    // Tools
    Tools             []Tool
    ToolChoice        interface{}
    ParallelToolCalls *bool
}
```

Each provider extracts what it supports from this unified struct.

## Advanced: Thinking Mode (Claude)

```go
params := &llmprovider.RequestParams{
    ThinkingEnabled: boolPtr(true),
    ThinkingLevel:   strPtr("high"),  // Token budgets: low=2k, medium=5k, high=12k
}
```

Thinking blocks are emitted as separate content blocks during streaming.

## Error Handling

The library provides typed errors for programmatic handling:

### Checking Error Types

```go
import "errors"

resp, err := provider.GenerateResponse(ctx, req)
if err != nil {
    // Check if error is retryable (rate limits, temporary outages)
    if llmprovider.IsRetryable(err) {
        // Implement retry logic with backoff
    }

    // Check for auth issues
    if llmprovider.IsAuthError(err) {
        // Refresh or validate API key
    }

    // Check for invalid request parameters
    if llmprovider.IsInvalidRequest(err) {
        // Fix request parameters
    }

    // Get detailed error information
    var modelErr *llmprovider.ModelError
    if errors.As(err, &modelErr) {
        fmt.Printf("Model %s not supported by %s: %s\n",
            modelErr.Model, modelErr.Provider, modelErr.Reason)
    }
}
```

### Sentinel Errors

Available for use with `errors.Is()`:

- `ErrInvalidModel` - Model not supported by provider
- `ErrInvalidAPIKey` - Missing or invalid API key
- `ErrRateLimited` - Rate limit exceeded (retryable)
- `ErrUnsupportedFeature` - Feature not available for this provider/model
- `ErrInvalidRequest` - Invalid request parameters
- `ErrProviderUnavailable` - Provider service temporarily down (retryable)

### Error Types

- `ModelError` - Model validation failures (includes model name, provider, reason)
- `ValidationError` - Request parameter validation (includes field, value, reason)
- `ProviderError` - Provider API errors (includes status code, message, retryable flag)

## Testing

Run tests:
```bash
go test ./...
```

Run examples (no API key needed):
```bash
go run examples/lorem-streaming/main.go
go run examples/lorem-basic/main.go
```

Run with Anthropic API key:
```bash
export ANTHROPIC_API_KEY="sk-ant-..."
go run examples/anthropic-basic/main.go
go run examples/anthropic-streaming/main.go
go run examples/anthropic-thinking/main.go
```

See `examples/README.md` for detailed setup instructions and .env configuration.

## Architecture

```
meridian-llm-go/
├── provider.go          # Provider interface only
├── request.go           # GenerateRequest, Message
├── response.go          # GenerateResponse
├── streaming.go         # StreamEvent, StreamMetadata
├── types.go             # Block, BlockDelta
├── params.go            # RequestParams + validation
├── errors.go            # Typed errors
├── test_helpers.go      # Test utilities
├── providers/
│   ├── anthropic/       # Claude provider
│   │   ├── provider.go
│   │   ├── adapter.go   # SDK conversions
│   │   ├── streaming.go
│   │   └── params.go    # Shared param building
│   └── lorem/           # Mock provider
│       └── provider.go
└── examples/
    ├── helpers/         # Example helper functions
    ├── lorem-streaming/
    ├── lorem-basic/
    ├── anthropic-basic/
    ├── anthropic-streaming/
    ├── anthropic-thinking/
    └── README.md        # Comprehensive setup guide
```

## Design Philosophy

1. **Content-Only Types**: No database fields (IDs, timestamps) - pure content models
2. **Provider-Agnostic**: Unified interface works across all providers
3. **Streaming-First**: Real-time SSE support as a first-class citizen
4. **Typed Errors**: Programmatic error detection for better error recovery
5. **Minimal Dependencies**: Only official provider SDKs
6. **Extensible**: Easy to add custom providers

## Comparison with Meridian Backend

| Feature | meridian-llm-go | Meridian Backend |
|---------|---------------------|------------------|
| Content Types | ✅ Block, BlockDelta | Turn, TurnBlock (with DB fields) |
| Providers | ✅ Anthropic, Lorem | Same, but tightly coupled |
| Streaming | ✅ Provider.StreamResponse() | TurnExecutor + custom SSE |
| Error Handling | ✅ Typed errors | Generic fmt.Errorf |
| Dependencies | Only provider SDKs | Full backend stack |
| Reusability | ✅ Any Go project | Meridian-specific |

## Contributing

This library was extracted from [Meridian](https://github.com/yourusername/meridian) to enable reuse across projects.

Contributions welcome:
- Add new providers (OpenAI, Gemini, etc.)
- Improve type safety
- Add validation helpers
- Expand test coverage

## License

MIT
