package llmprovider

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
)

// mockProvider is a minimal Provider for testing middleware composition.
type mockProvider struct {
	name              ProviderID
	generateResp      *GenerateResponse
	generateErr       error
	streamEvents      []StreamEvent
	streamTerminalErr error
	streamErr         error
	generateCalls     atomic.Int32
	streamCalls       atomic.Int32
}

func newMockProvider(name ProviderID) *mockProvider {
	return &mockProvider{
		name: name,
		generateResp: &GenerateResponse{
			Model:        "test-model",
			InputTokens:  10,
			OutputTokens: 20,
			StopReason:   "end_turn",
		},
		streamEvents: []StreamEvent{
			{Metadata: &StreamMetadata{
				Model:        "test-model",
				InputTokens:  10,
				OutputTokens: 20,
				StopReason:   "end_turn",
			}},
		},
	}
}

func (p *mockProvider) GenerateResponse(_ context.Context, _ *GenerateRequest) (*GenerateResponse, error) {
	p.generateCalls.Add(1)
	if p.generateErr != nil {
		return nil, p.generateErr
	}
	return p.generateResp, nil
}

func (p *mockProvider) StreamResponse(_ context.Context, _ *GenerateRequest) (*Stream, error) {
	p.streamCalls.Add(1)
	if p.streamErr != nil {
		return nil, p.streamErr
	}
	return StreamFromSlice(p.streamEvents, p.streamTerminalErr), nil
}

func (p *mockProvider) Name() ProviderID            { return p.name }
func (p *mockProvider) SupportsModel(m string) bool { return m == "test-model" }

// orderTrackingMiddleware records when it is called to verify composition order.
type orderTrackingMiddleware struct {
	id    string
	order *[]string
}

func (m *orderTrackingMiddleware) WrapGenerate(_ ProviderCallInfo, next GenerateFunc) GenerateFunc {
	return func(ctx context.Context, req *GenerateRequest) (*GenerateResponse, error) {
		*m.order = append(*m.order, m.id)
		return next(ctx, req)
	}
}

func (m *orderTrackingMiddleware) WrapStream(_ ProviderCallInfo, next StreamFunc) StreamFunc {
	return func(ctx context.Context, req *GenerateRequest) (*Stream, error) {
		*m.order = append(*m.order, m.id)
		return next(ctx, req)
	}
}

// noopMiddleware is a stateless middleware for concurrent safety testing.
type noopMiddleware struct{}

func (m *noopMiddleware) WrapGenerate(_ ProviderCallInfo, next GenerateFunc) GenerateFunc {
	return next
}

func (m *noopMiddleware) WrapStream(_ ProviderCallInfo, next StreamFunc) StreamFunc {
	return next
}

// denyMiddleware always returns an error without calling next.
type denyMiddleware struct {
	err error
}

func (m *denyMiddleware) WrapGenerate(_ ProviderCallInfo, _ GenerateFunc) GenerateFunc {
	return func(_ context.Context, _ *GenerateRequest) (*GenerateResponse, error) {
		return nil, m.err
	}
}

func (m *denyMiddleware) WrapStream(_ ProviderCallInfo, _ StreamFunc) StreamFunc {
	return func(_ context.Context, _ *GenerateRequest) (*Stream, error) {
		return nil, m.err
	}
}

func TestWrapProvider_NoMiddleware(t *testing.T) {
	base := newMockProvider("test")
	wrapped := WrapProvider(base)

	// Should return the exact same provider instance
	if wrapped != base {
		t.Error("WrapProvider with no middleware should return the same provider instance")
	}
}

func TestWrapProvider_CompositionOrder_Generate(t *testing.T) {
	base := newMockProvider("test")
	var order []string

	m1 := &orderTrackingMiddleware{id: "first", order: &order}
	m2 := &orderTrackingMiddleware{id: "second", order: &order}

	wrapped := WrapProvider(base, m1, m2)
	_, err := wrapped.GenerateResponse(context.Background(), &GenerateRequest{Model: "test-model"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// First middleware listed should be outermost (runs first)
	if len(order) != 2 || order[0] != "first" || order[1] != "second" {
		t.Errorf("expected order [first, second], got %v", order)
	}
}

func TestWrapProvider_CompositionOrder_Stream(t *testing.T) {
	base := newMockProvider("test")
	var order []string

	m1 := &orderTrackingMiddleware{id: "first", order: &order}
	m2 := &orderTrackingMiddleware{id: "second", order: &order}

	wrapped := WrapProvider(base, m1, m2)
	stream, err := wrapped.StreamResponse(context.Background(), &GenerateRequest{Model: "test-model"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer stream.Close()

	// Drain the stream
	for stream.Next() {
	}
	if err := stream.Err(); err != nil {
		t.Fatalf("unexpected stream err: %v", err)
	}

	if len(order) != 2 || order[0] != "first" || order[1] != "second" {
		t.Errorf("expected order [first, second], got %v", order)
	}
}

func TestWrapProvider_NameAndSupportsModel(t *testing.T) {
	base := newMockProvider("test-provider")
	wrapped := WrapProvider(base, &orderTrackingMiddleware{id: "m1", order: &[]string{}})

	if wrapped.Name() != "test-provider" {
		t.Errorf("expected Name() = test-provider, got %s", wrapped.Name())
	}

	if !wrapped.SupportsModel("test-model") {
		t.Error("expected SupportsModel(test-model) = true")
	}

	if wrapped.SupportsModel("unknown-model") {
		t.Error("expected SupportsModel(unknown-model) = false")
	}
}

func TestWrapProvider_Unwrap(t *testing.T) {
	base := newMockProvider("test")
	wrapped := WrapProvider(base, &orderTrackingMiddleware{id: "m1", order: &[]string{}})

	wp, ok := wrapped.(WrappedProvider)
	if !ok {
		t.Fatal("wrapped provider should implement WrappedProvider")
	}

	if wp.Unwrap() != base {
		t.Error("Unwrap() should return the base provider")
	}
}

func TestWrapProvider_UnwrapChain(t *testing.T) {
	base := newMockProvider("test")
	noop := &orderTrackingMiddleware{id: "m1", order: &[]string{}}

	// Double wrap
	wrapped1 := WrapProvider(base, noop)
	wrapped2 := WrapProvider(wrapped1, noop)

	// Unwrap chain: wrapped2 -> wrapped1 -> base
	wp2, ok := wrapped2.(WrappedProvider)
	if !ok {
		t.Fatal("outer wrap should implement WrappedProvider")
	}

	inner := wp2.Unwrap()
	wp1, ok := inner.(WrappedProvider)
	if !ok {
		t.Fatal("inner wrap should implement WrappedProvider")
	}

	if wp1.Unwrap() != base {
		t.Error("double unwrap should reach the base provider")
	}
}

func TestWrapProvider_GenerateDenial(t *testing.T) {
	base := newMockProvider("test")
	deny := &denyMiddleware{err: errors.New("denied")}

	wrapped := WrapProvider(base, deny)
	_, err := wrapped.GenerateResponse(context.Background(), &GenerateRequest{Model: "test-model"})

	if err == nil || err.Error() != "denied" {
		t.Errorf("expected denial error, got: %v", err)
	}

	if base.generateCalls.Load() != 0 {
		t.Error("base provider should not have been called when middleware denies")
	}
}

func TestWrapProvider_StreamDenial(t *testing.T) {
	base := newMockProvider("test")
	deny := &denyMiddleware{err: errors.New("denied")}

	wrapped := WrapProvider(base, deny)
	_, err := wrapped.StreamResponse(context.Background(), &GenerateRequest{Model: "test-model"})

	if err == nil || err.Error() != "denied" {
		t.Errorf("expected denial error, got: %v", err)
	}

	if base.streamCalls.Load() != 0 {
		t.Error("base provider should not have been called when middleware denies")
	}
}

func TestWrapProvider_GeneratePassthrough(t *testing.T) {
	base := newMockProvider("test")
	noop := &orderTrackingMiddleware{id: "noop", order: &[]string{}}

	wrapped := WrapProvider(base, noop)
	resp, err := wrapped.GenerateResponse(context.Background(), &GenerateRequest{Model: "test-model"})

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Model != "test-model" {
		t.Errorf("expected model test-model, got %s", resp.Model)
	}
	if resp.InputTokens != 10 || resp.OutputTokens != 20 {
		t.Errorf("expected tokens 10/20, got %d/%d", resp.InputTokens, resp.OutputTokens)
	}
}

func TestWrapProvider_StreamPassthrough(t *testing.T) {
	base := newMockProvider("test")
	noop := &orderTrackingMiddleware{id: "noop", order: &[]string{}}

	wrapped := WrapProvider(base, noop)
	stream, err := wrapped.StreamResponse(context.Background(), &GenerateRequest{Model: "test-model"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer stream.Close()

	var metadataReceived bool
	for stream.Next() {
		event := stream.Event()
		if event.Metadata != nil {
			metadataReceived = true
			if event.Metadata.Model != "test-model" {
				t.Errorf("expected model test-model, got %s", event.Metadata.Model)
			}
		}
	}
	if err := stream.Err(); err != nil {
		t.Fatalf("unexpected stream err: %v", err)
	}
	if !metadataReceived {
		t.Error("expected to receive metadata event")
	}
}

func TestWrapProvider_RequestMutationIsolation(t *testing.T) {
	base := newMockProvider("test")

	// Middleware that mutates the model field
	mutator := &mutatingMiddleware{newModel: "mutated-model"}
	wrapped := WrapProvider(base, mutator)

	req := &GenerateRequest{Model: "original-model"}
	_, err := wrapped.GenerateResponse(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// The caller's original request should be unchanged
	if req.Model != "original-model" {
		t.Errorf("original request was mutated: Model = %s", req.Model)
	}
}

// mutatingMiddleware clones the request and modifies it.
type mutatingMiddleware struct {
	newModel string
}

func (m *mutatingMiddleware) WrapGenerate(_ ProviderCallInfo, next GenerateFunc) GenerateFunc {
	return func(ctx context.Context, req *GenerateRequest) (*GenerateResponse, error) {
		// Clone the request before mutating (proper middleware behavior)
		cloned := *req
		cloned.Model = m.newModel
		return next(ctx, &cloned)
	}
}

func (m *mutatingMiddleware) WrapStream(_ ProviderCallInfo, next StreamFunc) StreamFunc {
	return func(ctx context.Context, req *GenerateRequest) (*Stream, error) {
		cloned := *req
		cloned.Model = m.newModel
		return next(ctx, &cloned)
	}
}

func TestWrapProvider_ConcurrentSafety(t *testing.T) {
	base := newMockProvider("test")
	wrapped := WrapProvider(base, &noopMiddleware{})

	const goroutines = 50
	var wg sync.WaitGroup
	errs := make(chan error, goroutines*2)

	for i := 0; i < goroutines; i++ {
		wg.Add(2)

		// Generate goroutine
		go func() {
			defer wg.Done()
			_, err := wrapped.GenerateResponse(context.Background(), &GenerateRequest{Model: "test-model"})
			if err != nil {
				errs <- err
			}
		}()

		// Stream goroutine
		go func() {
			defer wg.Done()
			stream, err := wrapped.StreamResponse(context.Background(), &GenerateRequest{Model: "test-model"})
			if err != nil {
				errs <- err
				return
			}
			defer stream.Close()
			for stream.Next() {
			}
			if err := stream.Err(); err != nil {
				errs <- err
			}
		}()
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		t.Errorf("concurrent call failed: %v", err)
	}
}
