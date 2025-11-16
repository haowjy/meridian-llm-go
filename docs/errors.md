---
detail: minimal
audience: library users
---

# Error Handling

Unified error handling across LLM providers with normalized categories and retryability.

## Quick Start

```go
resp, err := provider.GenerateResponse(ctx, req)
if err != nil {
    var llmErr *llm.LLMError
    if errors.As(err, &llmErr) {
        // Normalized LLM error
        log.Printf("Error: %s (category: %s, retryable: %t)",
            llmErr.Message,
            llmErr.Category,
            llmErr.Retryable)

        if llmErr.Retryable {
            // Retry with exponential backoff
            time.Sleep(llmErr.RetryAfter())
            return retry(ctx, req)
        }
    }
    return err
}
```

## LLMError Structure

```go
type LLMError struct {
    Category     ErrorCategory          // Normalized error category
    ProviderCode string                 // Original provider error code
    Message      string                 // Error message
    HTTPStatus   int                    // HTTP status code
    Retryable    bool                   // Whether error is retryable
    Provider     string                 // Provider name ("anthropic", "openai", etc.)
    Metadata     map[string]interface{} // Additional error context
}
```

## Error Categories

### Client Errors (4xx)

| Category | HTTP | Retryable | Meaning |
|----------|------|-----------|---------|
| `invalid_request` | 400 | ❌ | Malformed request, missing required fields |
| `authentication` | 401 | ❌ | Invalid API key |
| `permission_denied` | 403 | ❌ | API key doesn't have access |
| `not_found` | 404 | ❌ | Resource not found (model, endpoint) |
| `rate_limit` | 429 | ✅ | Too many requests |

### Server Errors (5xx)

| Category | HTTP | Retryable | Meaning |
|----------|------|-----------|---------|
| `provider_overloaded` | 503 | ✅ | Provider temporarily overloaded |
| `provider_error` | 500 | ❌ | Internal provider error |
| `provider_timeout` | 504 | ✅ | Provider request timeout |

### Tool Errors

| Category | Retryable | Meaning |
|----------|-----------|---------|
| `tool_unavailable` | ✅ | Tool temporarily unavailable |
| `tool_execution_failed` | ❌ | Tool ran but failed |
| `tool_invalid_input` | ❌ | Bad tool parameters |

### Streaming Errors

| Category | Retryable | Meaning |
|----------|-----------|---------|
| `stream_interrupted` | ❌ | Mid-stream failure |
| `stream_timeout` | ✅ | Stream took too long |

## Handling Errors

### Basic Error Handling

```go
resp, err := provider.GenerateResponse(ctx, req)
if err != nil {
    var llmErr *llm.LLMError
    if !errors.As(err, &llmErr) {
        // Unknown error, not from LLM provider
        return fmt.Errorf("unexpected error: %w", err)
    }

    // Handle by category
    switch llmErr.Category {
    case llm.ErrorRateLimit:
        log.Warn("Rate limited, retrying...")
        time.Sleep(10 * time.Second)
        return retry(ctx, req)

    case llm.ErrorAuthentication:
        return fmt.Errorf("invalid API key: %w", err)

    case llm.ErrorInvalidRequest:
        return fmt.Errorf("bad request: %s", llmErr.Message)

    default:
        return fmt.Errorf("provider error: %w", err)
    }
}
```

### Retryable Errors

```go
func callWithRetry(ctx context.Context, provider llm.Provider, req *llm.GenerateRequest) (*llm.Response, error) {
    maxRetries := 3
    baseDelay := 1 * time.Second

    for attempt := 0; attempt <= maxRetries; attempt++ {
        resp, err := provider.GenerateResponse(ctx, req)
        if err == nil {
            return resp, nil
        }

        var llmErr *llm.LLMError
        if !errors.As(err, &llmErr) || !llmErr.Retryable {
            // Not retryable
            return nil, err
        }

        if attempt == maxRetries {
            return nil, fmt.Errorf("max retries exceeded: %w", err)
        }

        // Exponential backoff
        delay := baseDelay * time.Duration(1<<attempt)
        if llmErr.Category == llm.ErrorRateLimit {
            delay = 10 * time.Second * time.Duration(1<<attempt)
        }

        log.Printf("Retry %d/%d after %v (error: %s)",
            attempt+1, maxRetries, delay, llmErr.Category)

        select {
        case <-time.After(delay):
            continue
        case <-ctx.Done():
            return nil, ctx.Err()
        }
    }

    return nil, fmt.Errorf("unreachable")
}
```

### Streaming Errors

Errors in streaming can occur at any point:

```go
stream, err := provider.GenerateStream(ctx, req)
if err != nil {
    // Pre-stream error (before streaming starts)
    return handleError(err)
}

for event := range stream.Events() {
    if event.Error != nil {
        // Mid-stream error
        var llmErr *llm.LLMError
        if errors.As(event.Error, &llmErr) {
            log.Printf("Stream error: %s", llmErr.Category)
        }
        return event.Error
    }

    // Process event...
}
```

## Provider Error Mappings

The library normalizes provider-specific errors:

### Anthropic Errors

| Provider Code | Category | Retryable |
|--------------|----------|-----------|
| `invalid_request_error` | `invalid_request` | ❌ |
| `authentication_error` | `authentication` | ❌ |
| `permission_error` | `permission_denied` | ❌ |
| `rate_limit_error` | `rate_limit` | ✅ |
| `overloaded_error` | `provider_overloaded` | ✅ |
| `api_error` | `provider_error` | ❌ |

### OpenAI Errors

| HTTP Status | Category | Retryable |
|------------|----------|-----------|
| 400 | `invalid_request` | ❌ |
| 401 | `authentication` | ❌ |
| 403 | `permission_denied` | ❌ |
| 404 | `not_found` | ❌ |
| 429 | `rate_limit` | ✅ |
| 500 | `provider_error` | ❌ |
| 503 | `provider_overloaded` | ✅ |
| 504 | `provider_timeout` | ✅ |

### Gemini Errors

Similar to OpenAI (uses standard HTTP status codes).

## Error Context

`LLMError.Metadata` contains additional context:

```go
if llmErr.Metadata != nil {
    // Anthropic
    requestID := llmErr.Metadata["request_id"]

    // OpenAI
    errorType := llmErr.Metadata["error_type"]

    // Tool errors
    toolName := llmErr.Metadata["tool_name"]
}
```

## Best Practices

### 1. Always Check Retryable Flag

```go
if llmErr.Retryable {
    // Safe to retry
} else {
    // Don't retry - fix the request
}
```

### 2. Use Exponential Backoff

```go
delay := baseDelay * time.Duration(1 << attempt)
```

### 3. Respect Rate Limits

```go
if llmErr.Category == llm.ErrorRateLimit {
    // Longer backoff for rate limits
    delay = 10 * time.Second * time.Duration(1<<attempt)
}
```

### 4. Preserve Original Error Info

```go
log.Printf("Provider error: %s (code: %s, status: %d)",
    llmErr.Message,
    llmErr.ProviderCode,
    llmErr.HTTPStatus)
```

### 5. Handle Streaming Errors Separately

```go
// Pre-stream errors
stream, err := provider.GenerateStream(ctx, req)
if err != nil {
    return handlePreStreamError(err)
}

// Mid-stream errors
for event := range stream.Events() {
    if event.Error != nil {
        return handleMidStreamError(event.Error)
    }
}
```

## Error Constants

```go
const (
    // Client errors (4xx)
    ErrorInvalidRequest   ErrorCategory = "invalid_request"
    ErrorAuthentication   ErrorCategory = "authentication"
    ErrorPermissionDenied ErrorCategory = "permission_denied"
    ErrorNotFound         ErrorCategory = "not_found"
    ErrorRateLimit        ErrorCategory = "rate_limit"

    // Server errors (5xx)
    ErrorProviderOverloaded ErrorCategory = "provider_overloaded"
    ErrorProviderError      ErrorCategory = "provider_error"
    ErrorProviderTimeout    ErrorCategory = "provider_timeout"

    // Tool errors
    ErrorToolUnavailable    ErrorCategory = "tool_unavailable"
    ErrorToolExecutionFailed ErrorCategory = "tool_execution_failed"
    ErrorToolInvalidInput   ErrorCategory = "tool_invalid_input"

    // Streaming errors
    ErrorStreamInterrupted  ErrorCategory = "stream_interrupted"
    ErrorStreamTimeout      ErrorCategory = "stream_timeout"
)
```

## API Reference

**Types:**
- `LLMError` - Normalized error with category and retryability
- `ErrorCategory` - Error category constant

**Methods:**
- `error.Error() string` - Error message
- `errors.As(err, &llmErr)` - Check if error is LLMError

**See:** `errors.go`

## Related

- [streaming.md](streaming.md) - Streaming error handling
- [providers.md](providers.md) - Provider-specific error behaviors

For **backend error handling** (retry strategies, user messages), see:
- `../../_docs/technical/llm/error-normalization.md`
