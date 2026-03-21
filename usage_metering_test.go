package llmprovider

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

// mockGate is a configurable UsageGate for testing.
type mockGate struct {
	decision  *UsageDecision
	err       error
	calls     atomic.Int32
	lastScope UsageScope
}

func (g *mockGate) CheckUsage(_ context.Context, req UsageGateRequest) (*UsageDecision, error) {
	g.calls.Add(1)
	g.lastScope = req.Scope
	if g.err != nil {
		return nil, g.err
	}
	return g.decision, nil
}

func allowGate() *mockGate {
	return &mockGate{decision: &UsageDecision{Allowed: true}}
}

func denyGate(code, reason string) *mockGate {
	return &mockGate{decision: &UsageDecision{
		Allowed:  false,
		Code:     code,
		Reason:   reason,
		Metadata: map[string]any{"limit": 100},
	}}
}

// mockReporter is a configurable UsageReporter for testing.
type mockReporter struct {
	err        error
	calls      atomic.Int32
	lastReport UsageReport
}

func (r *mockReporter) ReportUsage(_ context.Context, report UsageReport) error {
	r.calls.Add(1)
	r.lastReport = report
	if r.err != nil {
		return r.err
	}
	return nil
}

func TestUsageMeteringMiddleware_NilGateAndReporter(t *testing.T) {
	base := newMockProvider("test")
	mw := NewUsageMeteringMiddleware(nil, nil)
	wrapped := WrapProvider(base, mw)

	// Generate should pass through
	resp, err := wrapped.GenerateResponse(context.Background(), &GenerateRequest{Model: "test-model"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Model != "test-model" {
		t.Errorf("expected model test-model, got %s", resp.Model)
	}

	// Stream should pass through
	ch, err := wrapped.StreamResponse(context.Background(), &GenerateRequest{Model: "test-model"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for range ch {
	}
}

func TestUsageScopeContext_RoundTrip(t *testing.T) {
	scope := UsageScope{
		Operation:      "inference",
		Sequence:       3,
		Phase:          "continuation",
		IdempotencyKey: "key-123",
		Attributes:     map[string]string{"thread": "abc"},
	}

	ctx := WithUsageScope(context.Background(), scope)
	got, ok := UsageScopeFromContext(ctx)

	if !ok {
		t.Fatal("expected scope in context")
	}
	if got.Operation != "inference" || got.Sequence != 3 || got.Phase != "continuation" {
		t.Errorf("scope mismatch: %+v", got)
	}
	if got.IdempotencyKey != "key-123" {
		t.Errorf("expected idempotency key key-123, got %s", got.IdempotencyKey)
	}
	if got.Attributes["thread"] != "abc" {
		t.Errorf("expected attribute thread=abc, got %s", got.Attributes["thread"])
	}
}

func TestUsageScopeContext_Missing(t *testing.T) {
	_, ok := UsageScopeFromContext(context.Background())
	if ok {
		t.Error("expected no scope in empty context")
	}
}

func TestUsageDeniedError_Format(t *testing.T) {
	tests := []struct {
		name     string
		err      UsageDeniedError
		expected string
	}{
		{
			name:     "with code",
			err:      UsageDeniedError{Code: "CREDITS_EXHAUSTED", Reason: "no credits remaining"},
			expected: "usage denied [CREDITS_EXHAUSTED]: no credits remaining",
		},
		{
			name:     "without code",
			err:      UsageDeniedError{Reason: "rate limit exceeded"},
			expected: "usage denied: rate limit exceeded",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.err.Error() != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, tt.err.Error())
			}
		})
	}
}

func TestUsageDeniedError_ErrorsAs(t *testing.T) {
	err := error(&UsageDeniedError{Code: "TEST", Reason: "test"})
	var denied *UsageDeniedError
	if !errors.As(err, &denied) {
		t.Error("expected errors.As to match UsageDeniedError")
	}
	if denied.Code != "TEST" {
		t.Errorf("expected code TEST, got %s", denied.Code)
	}
}

func TestUsageMeteringMiddleware_GateOnly(t *testing.T) {
	base := newMockProvider("test")
	gate := allowGate()
	mw := NewUsageMeteringMiddleware(gate, nil)
	wrapped := WrapProvider(base, mw)

	// Generate: gate allows, no reporter, no panic
	resp, err := wrapped.GenerateResponse(context.Background(), &GenerateRequest{Model: "test-model"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Model != "test-model" {
		t.Errorf("expected model test-model, got %s", resp.Model)
	}
	if gate.calls.Load() != 1 {
		t.Errorf("expected 1 gate call, got %d", gate.calls.Load())
	}

	// Stream: gate allows, no reporter, no panic
	ch, err := wrapped.StreamResponse(context.Background(), &GenerateRequest{Model: "test-model"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for range ch {
	}
	if gate.calls.Load() != 2 {
		t.Errorf("expected 2 gate calls, got %d", gate.calls.Load())
	}
}

func TestUsageMeteringMiddleware_ReporterOnly(t *testing.T) {
	base := newMockProvider("test")
	reporter := &mockReporter{}
	mw := NewUsageMeteringMiddleware(nil, reporter)
	wrapped := WrapProvider(base, mw)

	// Generate: no gate, reporter called with correct data
	resp, err := wrapped.GenerateResponse(context.Background(), &GenerateRequest{Model: "test-model"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Model != "test-model" {
		t.Errorf("expected model test-model, got %s", resp.Model)
	}
	if reporter.calls.Load() != 1 {
		t.Errorf("expected 1 reporter call, got %d", reporter.calls.Load())
	}
	if reporter.lastReport.InputTokens != 10 || reporter.lastReport.OutputTokens != 20 {
		t.Errorf("expected tokens 10/20, got %d/%d", reporter.lastReport.InputTokens, reporter.lastReport.OutputTokens)
	}
}

func TestUsageMeteringMiddleware_GateDeniesGenerate(t *testing.T) {
	base := newMockProvider("test")
	gate := denyGate("CREDITS_EXHAUSTED", "no credits")
	mw := NewUsageMeteringMiddleware(gate, &mockReporter{})
	wrapped := WrapProvider(base, mw)

	_, err := wrapped.GenerateResponse(context.Background(), &GenerateRequest{Model: "test-model"})

	if err == nil {
		t.Fatal("expected denial error")
	}

	var denied *UsageDeniedError
	if !errors.As(err, &denied) {
		t.Fatalf("expected UsageDeniedError, got %T: %v", err, err)
	}
	if denied.Code != "CREDITS_EXHAUSTED" {
		t.Errorf("expected code CREDITS_EXHAUSTED, got %s", denied.Code)
	}
	if denied.Metadata["limit"] != 100 {
		t.Errorf("expected metadata limit=100, got %v", denied.Metadata["limit"])
	}

	// Base provider should not have been called
	if base.generateCalls.Load() != 0 {
		t.Error("base provider should not have been called on denial")
	}
}

func TestUsageMeteringMiddleware_GateDeniesStream(t *testing.T) {
	base := newMockProvider("test")
	gate := denyGate("RATE_LIMITED", "too many requests")
	mw := NewUsageMeteringMiddleware(gate, &mockReporter{})
	wrapped := WrapProvider(base, mw)

	_, err := wrapped.StreamResponse(context.Background(), &GenerateRequest{Model: "test-model"})

	if err == nil {
		t.Fatal("expected denial error")
	}

	var denied *UsageDeniedError
	if !errors.As(err, &denied) {
		t.Fatalf("expected UsageDeniedError, got %T: %v", err, err)
	}
	if denied.Code != "RATE_LIMITED" {
		t.Errorf("expected code RATE_LIMITED, got %s", denied.Code)
	}

	if base.streamCalls.Load() != 0 {
		t.Error("base provider should not have been called on denial")
	}
}

func TestUsageMeteringMiddleware_GateAllowsReporterSucceeds_Generate(t *testing.T) {
	base := newMockProvider("test")
	gate := allowGate()
	reporter := &mockReporter{}

	scope := UsageScope{Operation: "inference", Sequence: 1}
	ctx := WithUsageScope(context.Background(), scope)

	mw := NewUsageMeteringMiddleware(gate, reporter)
	wrapped := WrapProvider(base, mw)

	resp, err := wrapped.GenerateResponse(ctx, &GenerateRequest{Model: "test-model"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Model != "test-model" {
		t.Errorf("expected model test-model, got %s", resp.Model)
	}

	// Gate was called with the scope
	if gate.lastScope.Operation != "inference" {
		t.Errorf("expected scope operation inference, got %s", gate.lastScope.Operation)
	}

	// Reporter was called with correct data
	if reporter.calls.Load() != 1 {
		t.Fatalf("expected 1 reporter call, got %d", reporter.calls.Load())
	}
	if reporter.lastReport.Provider != "test" {
		t.Errorf("expected provider test, got %s", reporter.lastReport.Provider)
	}
	if reporter.lastReport.Scope.Operation != "inference" {
		t.Errorf("expected scope operation inference, got %s", reporter.lastReport.Scope.Operation)
	}
	if reporter.lastReport.InputTokens != 10 || reporter.lastReport.OutputTokens != 20 {
		t.Errorf("expected tokens 10/20, got %d/%d", reporter.lastReport.InputTokens, reporter.lastReport.OutputTokens)
	}
}

func TestUsageMeteringMiddleware_GateAllowsReporterSucceeds_Stream(t *testing.T) {
	base := newMockProvider("test")
	gate := allowGate()
	reporter := &mockReporter{}

	scope := UsageScope{Operation: "stream", Sequence: 2}
	ctx := WithUsageScope(context.Background(), scope)

	mw := NewUsageMeteringMiddleware(gate, reporter)
	wrapped := WrapProvider(base, mw)

	ch, err := wrapped.StreamResponse(ctx, &GenerateRequest{Model: "test-model"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var metadataReceived bool
	for event := range ch {
		if event.Metadata != nil {
			metadataReceived = true
		}
	}
	if !metadataReceived {
		t.Error("expected metadata event")
	}

	if reporter.calls.Load() != 1 {
		t.Fatalf("expected 1 reporter call, got %d", reporter.calls.Load())
	}
	if reporter.lastReport.Scope.Operation != "stream" || reporter.lastReport.Scope.Sequence != 2 {
		t.Errorf("unexpected scope: %+v", reporter.lastReport.Scope)
	}
}

func TestUsageMeteringMiddleware_ReporterFailure_Generate(t *testing.T) {
	base := newMockProvider("test")
	reportErr := errors.New("settlement failed")
	reporter := &mockReporter{err: reportErr}
	mw := NewUsageMeteringMiddleware(nil, reporter)
	wrapped := WrapProvider(base, mw)

	_, err := wrapped.GenerateResponse(context.Background(), &GenerateRequest{Model: "test-model"})

	if err == nil {
		t.Fatal("expected reporter error to propagate")
	}
	if err.Error() != "settlement failed" {
		t.Errorf("expected 'settlement failed', got %v", err)
	}
}

func TestUsageMeteringMiddleware_ReporterFailure_Stream(t *testing.T) {
	base := newMockProvider("test")
	reportErr := errors.New("settlement failed")
	reporter := &mockReporter{err: reportErr}
	mw := NewUsageMeteringMiddleware(nil, reporter)
	wrapped := WrapProvider(base, mw)

	ch, err := wrapped.StreamResponse(context.Background(), &GenerateRequest{Model: "test-model"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var lastEvent StreamEvent
	for event := range ch {
		lastEvent = event
	}

	// Reporter failure should produce terminal error event
	if lastEvent.Error == nil {
		t.Fatal("expected terminal error event from reporter failure")
	}
	if lastEvent.Error.Error() != "settlement failed" {
		t.Errorf("expected 'settlement failed', got %v", lastEvent.Error)
	}
}

func TestUsageMeteringMiddleware_UpstreamStreamError(t *testing.T) {
	base := newMockProvider("test")
	base.streamEvents = []StreamEvent{
		{Error: errors.New("upstream error")},
	}

	reporter := &mockReporter{}
	mw := NewUsageMeteringMiddleware(nil, reporter)
	wrapped := WrapProvider(base, mw)

	ch, err := wrapped.StreamResponse(context.Background(), &GenerateRequest{Model: "test-model"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var lastEvent StreamEvent
	for event := range ch {
		lastEvent = event
	}

	// Error should pass through
	if lastEvent.Error == nil || lastEvent.Error.Error() != "upstream error" {
		t.Errorf("expected upstream error, got %v", lastEvent.Error)
	}

	// Reporter should NOT be called on upstream error
	if reporter.calls.Load() != 0 {
		t.Errorf("reporter should not be called on upstream error, got %d calls", reporter.calls.Load())
	}
}

func TestUsageMeteringMiddleware_GenerationIDPassthrough(t *testing.T) {
	base := newMockProvider("test")
	base.streamEvents = []StreamEvent{
		{GenerationIDDiscovered: &GenerationIDEvent{
			GenerationID: "gen-abc123",
			Model:        "test-model",
			Provider:     "test",
		}},
		{Metadata: &StreamMetadata{
			Model:        "test-model",
			InputTokens:  10,
			OutputTokens: 20,
			StopReason:   "end_turn",
			GenerationID: "gen-abc123",
		}},
	}

	reporter := &mockReporter{}
	mw := NewUsageMeteringMiddleware(nil, reporter)
	wrapped := WrapProvider(base, mw)

	ch, err := wrapped.StreamResponse(context.Background(), &GenerateRequest{Model: "test-model"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var genIDReceived bool
	var metadataReceived bool
	for event := range ch {
		if event.GenerationIDDiscovered != nil {
			genIDReceived = true
			if event.GenerationIDDiscovered.GenerationID != "gen-abc123" {
				t.Errorf("expected gen-abc123, got %s", event.GenerationIDDiscovered.GenerationID)
			}
		}
		if event.Metadata != nil {
			metadataReceived = true
		}
	}

	if !genIDReceived {
		t.Error("expected GenerationIDDiscovered to pass through")
	}
	if !metadataReceived {
		t.Error("expected metadata event")
	}

	// Reporter should include generation ID
	if reporter.lastReport.GenerationID != "gen-abc123" {
		t.Errorf("expected reporter to receive generation ID gen-abc123, got %s", reporter.lastReport.GenerationID)
	}
}

func TestUsageMeteringMiddleware_ScopePassedToGateAndReporter(t *testing.T) {
	base := newMockProvider("test")
	gate := allowGate()
	reporter := &mockReporter{}

	scope := UsageScope{
		Operation:      "tool_continuation",
		Sequence:       5,
		Phase:          "continuation",
		IdempotencyKey: "idem-xyz",
		Attributes:     map[string]string{"thread_id": "t1"},
	}

	ctx := WithUsageScope(context.Background(), scope)
	mw := NewUsageMeteringMiddleware(gate, reporter)
	wrapped := WrapProvider(base, mw)

	_, err := wrapped.GenerateResponse(ctx, &GenerateRequest{Model: "test-model"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify gate received the scope
	if gate.lastScope.IdempotencyKey != "idem-xyz" {
		t.Errorf("gate didn't receive idempotency key: %s", gate.lastScope.IdempotencyKey)
	}
	if gate.lastScope.Sequence != 5 {
		t.Errorf("gate didn't receive sequence: %d", gate.lastScope.Sequence)
	}

	// Verify reporter received the scope
	if reporter.lastReport.Scope.Operation != "tool_continuation" {
		t.Errorf("reporter didn't receive operation: %s", reporter.lastReport.Scope.Operation)
	}
	if reporter.lastReport.Scope.Attributes["thread_id"] != "t1" {
		t.Errorf("reporter didn't receive attributes: %v", reporter.lastReport.Scope.Attributes)
	}
}

func TestUsageMeteringMiddleware_GateError_Generate(t *testing.T) {
	base := newMockProvider("test")
	gate := &mockGate{err: errors.New("gate unavailable")}
	mw := NewUsageMeteringMiddleware(gate, &mockReporter{})
	wrapped := WrapProvider(base, mw)

	_, err := wrapped.GenerateResponse(context.Background(), &GenerateRequest{Model: "test-model"})
	if err == nil || err.Error() != "gate unavailable" {
		t.Errorf("expected gate error, got: %v", err)
	}
	// Should NOT be a UsageDeniedError
	var denied *UsageDeniedError
	if errors.As(err, &denied) {
		t.Error("gate error should not be UsageDeniedError")
	}
	if base.generateCalls.Load() != 0 {
		t.Error("base should not be called on gate error")
	}
}

func TestUsageMeteringMiddleware_GateError_Stream(t *testing.T) {
	base := newMockProvider("test")
	gate := &mockGate{err: errors.New("gate unavailable")}
	mw := NewUsageMeteringMiddleware(gate, &mockReporter{})
	wrapped := WrapProvider(base, mw)

	_, err := wrapped.StreamResponse(context.Background(), &GenerateRequest{Model: "test-model"})
	if err == nil || err.Error() != "gate unavailable" {
		t.Errorf("expected gate error, got: %v", err)
	}
	// Should NOT be a UsageDeniedError
	var denied *UsageDeniedError
	if errors.As(err, &denied) {
		t.Error("gate error should not be UsageDeniedError")
	}
	if base.streamCalls.Load() != 0 {
		t.Error("base should not be called on gate error")
	}
}

func TestUsageMeteringMiddleware_ContextCancellation(t *testing.T) {
	// Create a provider that sends many events slowly
	base := newMockProvider("test")
	events := make([]StreamEvent, 100)
	for i := range events {
		events[i] = StreamEvent{} // non-terminal events
	}
	events = append(events, StreamEvent{Metadata: &StreamMetadata{Model: "test"}})
	base.streamEvents = events

	reporter := &mockReporter{}
	mw := NewUsageMeteringMiddleware(nil, reporter)
	wrapped := WrapProvider(base, mw)

	ctx, cancel := context.WithCancel(context.Background())
	ch, err := wrapped.StreamResponse(ctx, &GenerateRequest{Model: "test-model"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Read one event then cancel
	<-ch
	cancel()

	// Verify channel eventually closes (goroutine doesn't leak)
	timeout := time.After(2 * time.Second)
	for {
		select {
		case _, ok := <-ch:
			if !ok {
				return // channel closed, goroutine cleaned up
			}
		case <-timeout:
			t.Fatal("proxy channel not closed after context cancellation — goroutine leaked")
		}
	}
}

func TestUsageMeteringMiddleware_EmptyStream(t *testing.T) {
	base := newMockProvider("test")
	base.streamEvents = nil // empty stream

	reporter := &mockReporter{}
	mw := NewUsageMeteringMiddleware(nil, reporter)
	wrapped := WrapProvider(base, mw)

	ch, err := wrapped.StreamResponse(context.Background(), &GenerateRequest{Model: "test-model"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Channel should close without any events
	count := 0
	for range ch {
		count++
	}

	if reporter.calls.Load() != 0 {
		t.Error("reporter should not be called on empty stream")
	}
}
