package llmprovider

import (
	"context"
	"fmt"
)

// usageScopeKey is the unexported context key for UsageScope.
type usageScopeKey struct{}

// UsageScope provides context about the current operation for metering.
// Consumers attach this to the context via WithUsageScope so the metering
// middleware can include it in gate requests and usage reports.
type UsageScope struct {
	// Operation identifies the logical operation (e.g., "inference", "tool_continuation").
	Operation string

	// Sequence is the step number within a multi-step operation (e.g., tool round index).
	Sequence int

	// Phase identifies the operation phase (e.g., "initial", "continuation").
	Phase string

	// IdempotencyKey is used by the gate/reporter for deduplication.
	IdempotencyKey string

	// Attributes holds additional key-value pairs for consumer-specific metadata.
	Attributes map[string]string
}

// UsageGateRequest contains the information sent to the gate for authorization.
type UsageGateRequest struct {
	Provider ProviderID
	Model    string
	Request  *GenerateRequest
	Scope    UsageScope
}

// UsageDecision is the result of a gate check.
type UsageDecision struct {
	Allowed  bool
	Code     string
	Reason   string
	Metadata map[string]any
}

// UsageGate checks whether a provider call should proceed.
// Implementations must be safe for concurrent use.
type UsageGate interface {
	// CheckUsage decides whether the call is allowed.
	// Return a decision with Allowed=false to deny, or an error for gate failures.
	CheckUsage(ctx context.Context, req UsageGateRequest) (*UsageDecision, error)
}

// UsageReport contains usage data sent after a provider call completes.
type UsageReport struct {
	Provider         ProviderID
	Model            string
	Scope            UsageScope
	InputTokens      int
	OutputTokens     int
	StopReason string
	// GenerationID is the provider-specific generation identifier.
	// Only populated for streaming calls; empty for GenerateResponse.
	GenerationID     string
	ResponseMetadata map[string]any
}

// UsageReporter receives usage data after provider calls complete.
// Implementations must be safe for concurrent use.
//
// If ReportUsage returns an error, the metering middleware surfaces it to the caller:
//   - For GenerateResponse: the error is returned directly
//   - For StreamResponse: a terminal StreamEvent{Error: err} is emitted
//
// Consumers that want non-blocking settlement should make their reporter
// durable first (e.g., enqueue to a persistent store) then return nil.
type UsageReporter interface {
	ReportUsage(ctx context.Context, report UsageReport) error
}

// UsageDeniedError indicates a provider call was denied by the usage gate.
// Consumers can check for this with errors.As to handle denial programmatically.
type UsageDeniedError struct {
	Code     string
	Reason   string
	Scope    UsageScope
	Metadata map[string]any
}

func (e *UsageDeniedError) Error() string {
	if e.Code != "" {
		return fmt.Sprintf("usage denied [%s]: %s", e.Code, e.Reason)
	}
	return fmt.Sprintf("usage denied: %s", e.Reason)
}

// WithUsageScope attaches a UsageScope to the context.
func WithUsageScope(ctx context.Context, scope UsageScope) context.Context {
	return context.WithValue(ctx, usageScopeKey{}, scope)
}

// UsageScopeFromContext retrieves the UsageScope from the context.
// Returns the zero-value UsageScope and false if not present.
func UsageScopeFromContext(ctx context.Context) (UsageScope, bool) {
	scope, ok := ctx.Value(usageScopeKey{}).(UsageScope)
	return scope, ok
}

// usageMeteringMiddleware implements ProviderMiddleware for usage metering.
type usageMeteringMiddleware struct {
	gate     UsageGate
	reporter UsageReporter
}

// NewUsageMeteringMiddleware creates a middleware that checks usage gates before
// provider calls and reports usage after completion.
//
// Both gate and reporter are optional (nil-safe):
//   - nil gate: all calls are allowed
//   - nil reporter: usage is not reported
//   - both nil: middleware is a no-op (returns next directly, zero overhead)
func NewUsageMeteringMiddleware(gate UsageGate, reporter UsageReporter) ProviderMiddleware {
	return &usageMeteringMiddleware{
		gate:     gate,
		reporter: reporter,
	}
}

func (m *usageMeteringMiddleware) WrapGenerate(info ProviderCallInfo, next GenerateFunc) GenerateFunc {
	// No-op optimization: if both gate and reporter are nil, skip wrapping entirely
	if m.gate == nil && m.reporter == nil {
		return next
	}

	return func(ctx context.Context, req *GenerateRequest) (*GenerateResponse, error) {
		scope, _ := UsageScopeFromContext(ctx)

		// Gate check
		if m.gate != nil {
			decision, err := m.gate.CheckUsage(ctx, UsageGateRequest{
				Provider: info.Provider,
				Model:    req.Model,
				Request:  req,
				Scope:    scope,
			})
			if err != nil {
				return nil, err
			}
			if !decision.Allowed {
				return nil, &UsageDeniedError{
					Code:     decision.Code,
					Reason:   decision.Reason,
					Scope:    scope,
					Metadata: decision.Metadata,
				}
			}
		}

		// Call the next handler
		resp, err := next(ctx, req)
		if err != nil {
			return nil, err
		}

		// Report usage
		if m.reporter != nil {
			report := UsageReport{
				Provider:         info.Provider,
				Model:            resp.Model,
				Scope:            scope,
				InputTokens:      resp.InputTokens,
				OutputTokens:     resp.OutputTokens,
				StopReason:       resp.StopReason,
				ResponseMetadata: resp.ResponseMetadata,
			}
			if err := m.reporter.ReportUsage(ctx, report); err != nil {
				return nil, err
			}
		}

		return resp, nil
	}
}

func (m *usageMeteringMiddleware) WrapStream(info ProviderCallInfo, next StreamFunc) StreamFunc {
	// No-op optimization: if both gate and reporter are nil, skip wrapping entirely
	if m.gate == nil && m.reporter == nil {
		return next
	}

	return func(ctx context.Context, req *GenerateRequest) (<-chan StreamEvent, error) {
		scope, _ := UsageScopeFromContext(ctx)

		// Gate check (synchronous, before stream starts)
		if m.gate != nil {
			decision, err := m.gate.CheckUsage(ctx, UsageGateRequest{
				Provider: info.Provider,
				Model:    req.Model,
				Request:  req,
				Scope:    scope,
			})
			if err != nil {
				return nil, err
			}
			if !decision.Allowed {
				return nil, &UsageDeniedError{
					Code:     decision.Code,
					Reason:   decision.Reason,
					Scope:    scope,
					Metadata: decision.Metadata,
				}
			}
		}

		// Get the upstream stream
		upstream, err := next(ctx, req)
		if err != nil {
			return nil, err
		}

		// If no reporter, pass through the stream unchanged
		if m.reporter == nil {
			return upstream, nil
		}

		// Proxy the stream to intercept terminal metadata for reporting.
		// All sends use select with ctx.Done to prevent goroutine leaks
		// when the caller stops reading (e.g., HTTP disconnect, context cancel).
		proxy := make(chan StreamEvent)
		go func() {
			defer close(proxy)
			var reported bool
			for event := range upstream {
				// If upstream sends an error, forward it without reporting
				if event.Error != nil {
					select {
					case proxy <- event:
					case <-ctx.Done():
						return
					}
					continue
				}

				// Terminal metadata: report usage before forwarding
				if event.Metadata != nil {
					if !reported && m.reporter != nil {
						reported = true
						report := UsageReport{
							Provider:         info.Provider,
							Model:            event.Metadata.Model,
							Scope:            scope,
							InputTokens:      event.Metadata.InputTokens,
							OutputTokens:     event.Metadata.OutputTokens,
							StopReason:       event.Metadata.StopReason,
							GenerationID:     event.Metadata.GenerationID,
							ResponseMetadata: event.Metadata.ResponseMetadata,
						}
						if err := m.reporter.ReportUsage(ctx, report); err != nil {
							// Reporter failed: emit terminal error instead of metadata
							select {
							case proxy <- StreamEvent{Error: err}:
							case <-ctx.Done():
							}
							return
						}
					}
					// Forward the metadata event regardless
					select {
					case proxy <- event:
					case <-ctx.Done():
						return
					}
					continue
				}

				// All other events (deltas, blocks, GenerationIDDiscovered) pass through
				select {
				case proxy <- event:
				case <-ctx.Done():
					return
				}
			}
		}()

		return proxy, nil
	}
}
