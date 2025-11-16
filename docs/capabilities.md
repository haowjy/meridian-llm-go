---
detail: minimal
audience: library users
---

# Capabilities

Provider capability configuration (models, pricing, features, tools) via YAML files.

## Overview

Capability configs define what each provider supports:
- Models and pricing
- Feature flags (vision, thinking, streaming)
- Tool support
- Context limits

Loaded once at startup, cached in memory.

## Loading Capabilities

```go
import llm "github.com/yourusername/meridian-llm-go"

// Load from embedded configs (default)
client, err := llm.NewLLMClient("")
// Uses built-in configs from config/capabilities/*.yaml

// Load from custom directory
client, err := llm.NewLLMClient("/path/to/configs")
// Loads from /path/to/configs/anthropic.yaml, openai.yaml, etc.
```

## Capability Structure

```yaml
provider: anthropic
display_name: Anthropic Claude
last_updated: "2025-01-14T00:00:00Z"

models:
  claude-sonnet-4-5-20250929:
    display_name: "Claude Sonnet 4.5"
    context_window: 200000
    max_output_tokens: 64000

    pricing:
      input_per_mtok: 3.0
      output_per_mtok: 15.0

    features:
      vision: true
      tool_calling: true
      thinking: true
      streaming: true

built_in_tools:
  search:
    native_support: true
    execution_side: server

  bash:
    native_support: true
    execution_side: client
```

## Model Capabilities

### Model Info

```go
// Get model capabilities
caps, err := client.GetModelCapabilities("anthropic", "claude-sonnet-4-5")

fmt.Println(caps.DisplayName)      // "Claude Sonnet 4.5"
fmt.Println(caps.ContextWindow)    // 200000
fmt.Println(caps.MaxOutputTokens)  // 64000
```

### Pricing

```go
// Calculate cost
inputTokens := 1000
outputTokens := 500

inputCost := float64(inputTokens) / 1_000_000 * caps.Pricing.InputPerMTok
outputCost := float64(outputTokens) / 1_000_000 * caps.Pricing.OutputPerMTok

totalCost := inputCost + outputCost
fmt.Printf("Cost: $%.4f\n", totalCost)
```

Pricing fields:
- `input_per_mtok` - Input price per million tokens
- `output_per_mtok` - Output price per million tokens
- `thinking_input_per_mtok` - Thinking tokens input price (optional)
- `thinking_output_per_mtok` - Thinking tokens output price (optional)
- `cached_input_per_mtok` - Prompt caching price (optional)

### Features

```go
if caps.Features.Vision {
    // Model supports image input
}

if caps.Features.ToolCalling {
    // Model supports function calling
}

if caps.Features.Thinking {
    // Model supports extended thinking
}

if caps.Features.Streaming {
    // Model supports SSE streaming
}
```

Feature flags:
- `vision` - Supports image input
- `tool_calling` - Supports function calling
- `thinking` - Supports extended thinking/reasoning
- `streaming` - Supports SSE streaming
- `prompt_caching` - Supports prompt caching
- `batch_api` - Supports batch processing
- `json_mode` - Supports structured JSON output

## Tool Capabilities

### Built-in Tool Support

```go
// Check if provider natively supports a tool
toolCaps, exists := client.GetToolCapabilities("anthropic", "search")

if toolCaps.NativeSupport {
    fmt.Println("Provider has built-in search")
    fmt.Println("Execution:", toolCaps.ExecutionSide) // "server" or "client"
} else {
    fmt.Println("Implement as custom tool")
}
```

Tool fields:
- `native_support` - Provider has built-in implementation
- `execution_side` - Who executes: `server` (provider) or `client` (you)
- `pricing_per_1k_requests` - Cost per 1000 tool invocations (optional)

### Tool Examples

**Anthropic:**
```yaml
built_in_tools:
  search:
    native_support: true
    execution_side: server
    pricing_per_1k_requests: 10.0

  bash:
    native_support: true
    execution_side: client
```

**OpenAI:**
```yaml
built_in_tools:
  search:
    native_support: true
    execution_side: server

  code_exec:
    native_support: true
    execution_side: server
    pricing_per_1k_requests: 30.0
```

## Thinking Configuration

For providers that support thinking:

```yaml
thinking:
  min_budget: 1024        # Minimum tokens
  max_budget: 200000      # Maximum tokens
  default_budget: 2000    # Default if not specified

  effort_to_budget:       # UX helper for effort levels
    minimal: 1024
    low: 2000
    medium: 6000
    high: 15000
```

Usage:
```go
// Use effort level (library converts to tokens)
req.Params = &llm.RequestParams{
    ThinkingEffort: llm.ThinkingEffortMedium, // → 6000 tokens
}

// Or specify budget directly
req.Params = &llm.RequestParams{
    ThinkingBudget: 10000, // Exact token count
}
```

## Configuration Files

### Embedded Configs

Library ships with embedded YAML configs for all providers:

```
meridian-llm-go/
└── config/
    └── capabilities/
        ├── anthropic.yaml
        ├── openai.yaml
        ├── gemini.yaml
        └── openrouter.yaml
```

### Custom Configs

Override with custom directory:

```go
client, err := llm.NewLLMClient("/my/configs")
```

Your directory structure:
```
/my/configs/
├── anthropic.yaml   # Override Anthropic config
└── custom.yaml      # Add custom provider
```

## YAML Schema

### Model Schema

```yaml
models:
  <model_id>:
    display_name: string
    family: string
    version: string

    context_window: int
    max_output_tokens: int

    pricing:
      input_per_mtok: float
      output_per_mtok: float

    features:
      vision: bool
      tool_calling: bool
      thinking: bool
      streaming: bool

    modalities: [text, image]
    status: current | legacy | deprecated
```

### Tool Schema

```yaml
built_in_tools:
  <unified_name>:
    native_support: bool
    execution_side: server | client
    pricing_per_1k_requests: float  # optional
    language: string                # optional (for code_exec)
```

## Validation

Configs are validated at load time:

```go
client, err := llm.NewLLMClient(configDir)
if err != nil {
    // Validation error: missing required fields, invalid ranges, etc.
    log.Fatal(err)
}
```

Validation checks:
- Required fields present
- Positive values for context/tokens/pricing
- Valid status values
- Valid thinking budget ranges

## Best Practices

### 1. Use Embedded Configs

```go
// Prefer embedded configs for production
client, err := llm.NewLLMClient("")
```

Embedded configs are:
- Version-controlled with library
- Tested and validated
- Updated with library releases

### 2. Check Feature Support

```go
caps, _ := client.GetModelCapabilities(provider, model)

if !caps.Features.ToolCalling {
    return errors.New("model doesn't support tools")
}
```

### 3. Estimate Costs

```go
estimatedCost := calculateCost(caps.Pricing, inputTokens, outputTokens)

if estimatedCost > budget {
    return errors.New("request exceeds budget")
}
```

### 4. Use Thinking Effort Levels

```go
// Prefer effort levels (more portable)
ThinkingEffort: llm.ThinkingEffortMedium

// Over exact budgets (provider-specific)
ThinkingBudget: 6000
```

## API Reference

**Types:**
- `ProviderCapabilities` - Full provider config
- `ModelCapability` - Model-specific capabilities
- `BuiltInToolSpec` - Tool support info
- `ThinkingCapability` - Thinking configuration

**Methods:**
- `NewLLMClient(configDir string) (*Client, error)` - Load capabilities
- `client.GetModelCapabilities(provider, model) (*ModelCapability, error)`
- `client.GetToolCapabilities(provider, tool) (*BuiltInToolSpec, bool)`

**See:** `capabilities.go`

## Related

- [providers.md](providers.md) - Supported providers
- [tools.md](tools.md) - Tool usage
- [errors.md](errors.md) - Validation errors

For **full schema reference**, see:
- `../../_docs/technical/llm/capability-config-schema.md`
