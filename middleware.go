package llmprovider

import "context"

// StreamFunc is the function signature for streaming provider operations.
type StreamFunc func(ctx context.Context, req *GenerateRequest) (<-chan StreamEvent, error)

// GenerateFunc is the function signature for blocking provider operations.
type GenerateFunc func(ctx context.Context, req *GenerateRequest) (*GenerateResponse, error)

// ProviderCallInfo contains immutable provider metadata passed to middleware.
// Middleware receives this to make decisions without accessing the full provider.
type ProviderCallInfo struct {
	Provider ProviderID
}

// ProviderMiddleware wraps provider operations for cross-cutting concerns.
// Implementations must be safe for concurrent use.
//
// Middleware may:
//   - deny by returning an error without calling next
//   - short-circuit by returning its own response/stream without calling next
//   - modify the request, but must clone any fields it mutates before calling next
//
// Example middleware: usage metering, logging, rate limiting, caching, guardrails.
type ProviderMiddleware interface {
	// WrapStream wraps the streaming operation.
	WrapStream(info ProviderCallInfo, next StreamFunc) StreamFunc

	// WrapGenerate wraps the blocking generation operation.
	WrapGenerate(info ProviderCallInfo, next GenerateFunc) GenerateFunc
}

// WrappedProvider extends Provider with Unwrap for accessing the base provider.
// Consumers use Unwrap to type-assert optional capabilities on the underlying provider
// (e.g., CancelGeneration, QueryGenerationStats).
type WrappedProvider interface {
	Provider
	// Unwrap returns the directly wrapped provider.
	// Repeated WrapProvider calls form a simple unwrap chain.
	Unwrap() Provider
}

// WrapProvider composes middleware around a provider.
// Returns the provider unchanged when no middleware are supplied.
// First middleware listed is outermost (wraps around all others).
//
// Composition builds once at wrap time, not per call:
//
//	wrapped := WrapProvider(base, logging, metering)
//	// logging.WrapStream wraps metering.WrapStream wraps base.StreamResponse
//
// The returned provider delegates Name() and SupportsModel() to the base.
// Use Unwrap() on the returned WrappedProvider to access the base provider's
// optional interfaces.
func WrapProvider(provider Provider, middleware ...ProviderMiddleware) Provider {
	if len(middleware) == 0 {
		return provider
	}

	info := ProviderCallInfo{
		Provider: provider.Name(),
	}

	// Start with the base provider's functions
	streamFn := StreamFunc(provider.StreamResponse)
	generateFn := GenerateFunc(provider.GenerateResponse)

	// Apply middleware from last to first so first-listed is outermost
	for i := len(middleware) - 1; i >= 0; i-- {
		streamFn = middleware[i].WrapStream(info, streamFn)
		generateFn = middleware[i].WrapGenerate(info, generateFn)
	}

	return &wrappedProvider{
		base:       provider,
		streamFn:   streamFn,
		generateFn: generateFn,
	}
}

// wrappedProvider is the single wrapper struct produced by WrapProvider.
// It stores composed functions built once at wrap time.
type wrappedProvider struct {
	base       Provider
	streamFn   StreamFunc
	generateFn GenerateFunc
}

func (w *wrappedProvider) GenerateResponse(ctx context.Context, req *GenerateRequest) (*GenerateResponse, error) {
	return w.generateFn(ctx, req)
}

func (w *wrappedProvider) StreamResponse(ctx context.Context, req *GenerateRequest) (<-chan StreamEvent, error) {
	return w.streamFn(ctx, req)
}

// Name delegates to the base provider.
func (w *wrappedProvider) Name() ProviderID {
	return w.base.Name()
}

// SupportsModel delegates to the base provider.
func (w *wrappedProvider) SupportsModel(model string) bool {
	return w.base.SupportsModel(model)
}

// Unwrap returns the directly wrapped provider.
func (w *wrappedProvider) Unwrap() Provider {
	return w.base
}
