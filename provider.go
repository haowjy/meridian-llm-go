package llmprovider

import (
	"context"
)

// Provider defines the interface that all LLM providers must implement.
// This abstraction allows supporting multiple providers (Anthropic, OpenAI, Gemini, etc.)
// while maintaining a consistent interface.
//
// Types used by this interface:
//   - GenerateRequest, Message: defined in request.go
//   - GenerateResponse: defined in response.go
//   - StreamEvent: defined in streaming.go
type Provider interface {
	// GenerateResponse generates a complete response from the LLM provider (blocking).
	// It takes conversation context (messages) and returns content blocks.
	// Used for non-streaming scenarios or as fallback.
	GenerateResponse(ctx context.Context, req *GenerateRequest) (*GenerateResponse, error)

	// StreamResponse generates a streaming response from the LLM provider (non-blocking).
	// Returns a channel that emits StreamEvent as they arrive.
	// The channel is closed when streaming completes or encounters an error.
	// Metadata (tokens, stop_reason) is sent in the final StreamMetadata event.
	//
	// Usage:
	//   eventChan, err := provider.StreamResponse(ctx, req)
	//   if err != nil { return err }
	//   for event := range eventChan {
	//     if event.Error != nil { handle error }
	//     if event.Delta != nil { process delta }
	//     if event.Metadata != nil { streaming complete }
	//   }
	StreamResponse(ctx context.Context, req *GenerateRequest) (<-chan StreamEvent, error)

	// Name returns the provider name (e.g., "anthropic", "openai", "lorem")
	Name() string

	// SupportsModel returns true if the provider supports the given model.
	SupportsModel(model string) bool
}
