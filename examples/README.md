# llmprovider Examples

This directory contains runnable examples demonstrating the llmprovider library with both mock (Lorem) and real (Anthropic) providers.

## Quick Start

### Using Makefile (Recommended)

The fastest way to test examples - builds and runs in one command:

```bash
# Test without API key (Lorem provider)
make run-lorem-streaming  # Mock streaming (30 words/second)
make run-lorem-basic      # Mock non-streaming (10s delay)

# Test with Anthropic API (requires ANTHROPIC_API_KEY)
make run-anthropic-basic      # Simple Claude response
make run-anthropic-streaming  # Real-time streaming
make run-anthropic-thinking   # Extended thinking mode

# Build all examples as binaries
make examples

# Clean up built binaries
make clean
```

### Manual Testing with `go run`

#### 1. Test Without API Key (Lorem Provider)

The Lorem provider generates mock responses and requires no API key - perfect for testing!

```bash
# Streaming mock response (30 words/second)
go run examples/lorem-streaming/main.go

# Non-streaming mock response (10 second delay)
go run examples/lorem-basic/main.go
```

#### 2. Test With Anthropic API Key

Get your API key from [console.anthropic.com](https://console.anthropic.com/).

**Option 1: .env file** (recommended - auto-discovered)

Create a `.env` file anywhere in your workspace (e.g., project root or `backend/.env`):
```bash
# .env
ANTHROPIC_API_KEY=sk-ant-your-key-here
```

Then run examples from any directory - the .env will be automatically found:
```bash
# From workspace root
go run llmprovider/examples/anthropic-basic/main.go

# From llmprovider/
go run examples/anthropic-streaming/main.go

# From backend/ (if .env is there)
go run ../llmprovider/examples/anthropic-thinking/main.go
```

**Option 2: Environment variable**
```bash
export ANTHROPIC_API_KEY="sk-ant-..."
go run examples/anthropic-basic/main.go
```

## Examples Overview

| Example | Provider | Streaming | API Key | Description |
|---------|----------|-----------|---------|-------------|
| `lorem-streaming` | Lorem | ✅ | ❌ | Mock streaming at 30 words/sec |
| `lorem-basic` | Lorem | ❌ | ❌ | Mock non-streaming with 10s delay |
| `anthropic-basic` | Anthropic | ❌ | ✅ | Simple Claude response |
| `anthropic-streaming` | Anthropic | ✅ | ✅ | Real-time Claude streaming |
| `anthropic-thinking` | Anthropic | ✅ | ✅ | Extended thinking mode demo |

## Example Details

### lorem-streaming

**Purpose:** Test streaming without API costs
**Model:** `lorem-fast` (30 words/second)
**Shows:** Real-time text streaming, delta handling, final metadata

```bash
go run examples/lorem-streaming/main.go
```

**Expected output:**
```
=== Lorem Streaming Example ===
Provider: lorem

Streaming response:
---
Lorem ipsum dolor sit amet consectetur adipiscing elit sed do...
---

✓ Streaming complete
  Model: lorem-fast
  Input tokens: 6
  Output tokens: 100
  Stop reason: end_turn
```

### lorem-basic

**Purpose:** Test non-streaming (blocking) pattern
**Model:** `lorem-fast`
**Shows:** 10-second delay, complete response

```bash
go run examples/lorem-basic/main.go
```

### anthropic-basic

**Purpose:** Simple Claude integration test
**Model:** `claude-haiku-4-5-20251001` (fast, cheap)
**Shows:** API key handling, complete response, token counting

```bash
export ANTHROPIC_API_KEY="sk-ant-..."
go run examples/anthropic-basic/main.go
```

**Expected output:**
```
=== Anthropic Basic (Non-Streaming) Example ===
Provider: anthropic

Response:
---
A variable is a named container in programming that stores data...
---

✓ Response complete
  Model: claude-haiku-4-5-20251001
  Input tokens: 25
  Output tokens: 48
  Stop reason: end_turn
```

### anthropic-streaming

**Purpose:** Real-time streaming with Claude
**Model:** `claude-haiku-4-5-20251001`
**Shows:** Character-by-character streaming, deltas, metadata

```bash
export ANTHROPIC_API_KEY="sk-ant-..."
go run examples/anthropic-streaming/main.go
```

### anthropic-thinking

**Purpose:** Demonstrate extended thinking mode
**Model:** `claude-sonnet-4-5-20250929` (better reasoning)
**Shows:** Separate thinking/answer blocks, high-level reasoning

```bash
export ANTHROPIC_API_KEY="sk-ant-..."
go run examples/anthropic-thinking/main.go
```

**Expected output:**
```
🧠 THINKING BLOCK:
---
Let me work through this logic puzzle step by step...
---

💬 ANSWER BLOCK:
---
Based on the clues:
- Charlie's favorite is green
- Alice doesn't like red, so Alice likes blue
- Bob's favorite is not blue, so Bob likes red
---
```

## Troubleshooting

### API Key Issues

**Error:** `ANTHROPIC_API_KEY environment variable is required`

**Solution (Option 1 - .env file):**

Create a `.env` file in your workspace (it will be auto-discovered):
```bash
# .env
ANTHROPIC_API_KEY=sk-ant-your-key-here
```

The examples automatically search for `.env` by walking up the directory tree from where you run them.

**Solution (Option 2 - export):**
```bash
export ANTHROPIC_API_KEY="sk-ant-your-key-here"
```

**Error:** `401 Unauthorized`

**Solution:** Check that your API key is valid and has credits. Get a new key from [console.anthropic.com](https://console.anthropic.com/).

### Import Errors

**Error:** `cannot find package "github.com/yourusername/llmprovider"`

**Solution:** Make sure you're running from the workspace root and run:
```bash
go work sync
go mod tidy
```

### Rate Limiting

**Error:** `429 Too Many Requests`

**Solution:** You've hit Anthropic's rate limit. Wait a minute and try again, or use the Lorem provider for testing.

## Lorem Provider Models

The Lorem provider supports different models for various test scenarios:

- `lorem-fast` - 30 words/second (default for quick testing)
- `lorem-medium` - 10 words/second (realistic streaming speed)
- `lorem-slow` - 2 words/second (slow connection simulation)
- `lorem-cutoff` - Simulates hitting `max_tokens` limit

## API Costs

Using Anthropic examples will consume API credits:

- **Haiku:** ~$0.25 per million input tokens, ~$1.25 per million output tokens
- **Sonnet:** ~$3 per million input tokens, ~$15 per million output tokens

These examples use minimal tokens (~50-200 per run) and cost fractions of a cent.

## Next Steps

After running these examples:

1. **Modify examples** - Change prompts, models, parameters
2. **Create your own** - Use these as templates for your use cases
3. **Integrate** - Import llmprovider into your own projects
4. **Explore providers** - Try different models and settings

## Additional Resources

- Main README: `../README.md`
- Provider docs: `../providers/*/README.md`
- Anthropic docs: https://docs.anthropic.com
- Go workspaces: https://go.dev/doc/tutorial/workspaces
