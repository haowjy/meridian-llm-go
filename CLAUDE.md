# meridian-llm-go — Agent Instructions

## Public Library Notice

This is a **public Go module** (`github.com/haowjy/meridian-llm-go`). External consumers depend on it. Be mindful of breaking changes to exported types, interfaces, and function signatures. Claude should actively help with core infrastructure changes — just ensure backward compatibility when possible.

## Development Commands

```bash
make test                    # Run all tests
make examples                # Build all example binaries
make clean                   # Remove example binaries
make run-lorem-streaming     # Mock provider (no API key needed)
make run-anthropic-streaming # Real Claude API (requires ANTHROPIC_API_KEY)
make run-openrouter-streaming # OpenRouter (requires OPENROUTER_API_KEY)
```

## Architecture

```
provider.go              → Provider interface (GenerateResponse, StreamResponse)
streaming.go             → StreamEvent, stream types
request.go / response.go → GenerateRequest, GenerateResponse
types.go                 → Block, Message, content types
tools.go                 → Tool types + ToolRegistry
schema.go                → JSON Schema helpers
errors.go                → Typed error handling (rate limits, auth, etc.)
providers/
  anthropic/             → Anthropic (Claude) adapter
  lorem/                 → Mock provider for testing
  openrouter/            → OpenRouter multi-provider adapter
internal/                → Internal helpers (not exported)
docs/                    → Architecture docs (blocks, streaming, tools, errors)
examples/                → Runnable examples per provider
```

### Key Design Principles

- **Content-only types** — No database fields, no ORM tags. Pure content models.
- **Streaming-first** — `StreamResponse()` returns `<-chan StreamEvent`. Blocking `GenerateResponse()` exists but streaming is primary.
- **Provider interface** — All providers implement `Provider` (see `provider.go`). Four methods: `GenerateResponse`, `StreamResponse`, `Name`, `SupportsModel`.
- **Typed errors** — `LLMError` with `ErrorType` enum for programmatic handling (rate limits, auth, overloaded, etc.)

## Adding a New Provider

1. Create `providers/<name>/` directory
2. Implement the `Provider` interface (`provider.go`)
3. Add model prefix detection in `SupportsModel()`
4. Register in `provider_registry.go` if using auto-detection
5. Add example in `examples/<name>-streaming/`
6. Add Makefile target: `make run-<name>-streaming`

## Conventions

- Exported types live at package root (`llmprovider`)
- Provider adapters convert between provider SDK types and `llmprovider` types
- Tests use `_test.go` suffix at package root
- `test_helpers.go` has shared test utilities

## Versioning

Patch versions are auto-bumped by a **post-commit hook** in the parent `meridian/` repo. After committing here, the hook:
1. Tags the commit (v0.0.X+1)
2. Pushes commit + tag to origin
3. Updates `backend/go.mod` with the new version

For manual/interactive version management, use `scripts/update-libraries.sh` from the parent repo.

## Documentation

- `README.md` — Quick start, installation, usage examples
- `docs/` — Architecture docs (blocks, streaming, tools, errors, providers)
