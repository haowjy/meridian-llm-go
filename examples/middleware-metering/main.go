// Example: middleware-metering demonstrates the ProviderMiddleware system
// with the built-in UsageMeteringMiddleware wrapping a lorem provider.
//
// No API keys required — uses the mock lorem provider.
//
// Usage: make run-middleware-metering
package main

import (
	"context"
	"fmt"
	"log"

	llmprovider "github.com/haowjy/meridian-llm-go"
	"github.com/haowjy/meridian-llm-go/providers/lorem"
)

// printGate is a UsageGate that always allows and prints the check.
type printGate struct{}

func (g *printGate) CheckUsage(_ context.Context, req llmprovider.UsageGateRequest) (*llmprovider.UsageDecision, error) {
	fmt.Printf("[gate] CheckUsage: provider=%s model=%s op=%s seq=%d\n",
		req.Provider, req.Model, req.Scope.Operation, req.Scope.Sequence)
	return &llmprovider.UsageDecision{Allowed: true}, nil
}

// printReporter is a UsageReporter that prints the report.
type printReporter struct{}

func (r *printReporter) ReportUsage(_ context.Context, report llmprovider.UsageReport) error {
	fmt.Printf("[reporter] Usage: provider=%s model=%s input=%d output=%d stop=%s\n",
		report.Provider, report.Model, report.InputTokens, report.OutputTokens, report.StopReason)
	if report.GenerationID != "" {
		fmt.Printf("[reporter]   generation_id=%s\n", report.GenerationID)
	}
	if report.Scope.Operation != "" {
		fmt.Printf("[reporter]   scope: op=%s seq=%d phase=%s\n",
			report.Scope.Operation, report.Scope.Sequence, report.Scope.Phase)
	}
	return nil
}

func textBlock(text string) *llmprovider.Block {
	t := text
	return &llmprovider.Block{BlockType: "text", TextContent: &t}
}

func intPtr(n int) *int { return &n }

func main() {
	// Create a lorem provider (mock, no API key needed)
	base := lorem.NewProvider()

	// Wrap with usage metering middleware
	metered := llmprovider.WrapProvider(base,
		llmprovider.NewUsageMeteringMiddleware(&printGate{}, &printReporter{}),
	)

	fmt.Printf("Provider: %s (wrapped: %T)\n\n", metered.Name(), metered)

	// --- Blocking Generate ---
	fmt.Println("=== GenerateResponse ===")
	ctx := llmprovider.WithUsageScope(context.Background(), llmprovider.UsageScope{
		Operation: "generate",
		Sequence:  1,
		Phase:     "initial",
	})

	resp, err := metered.GenerateResponse(ctx, &llmprovider.GenerateRequest{
		Model:    "lorem-fast",
		Messages: []llmprovider.Message{{Role: "user", Blocks: []*llmprovider.Block{textBlock("Hello")}}},
	})
	if err != nil {
		log.Fatalf("GenerateResponse error: %v", err)
	}
	fmt.Printf("Response: %d blocks, model=%s, tokens=%d/%d\n\n",
		len(resp.Blocks), resp.Model, resp.InputTokens, resp.OutputTokens)

	// --- Streaming ---
	fmt.Println("=== StreamResponse ===")
	ctx = llmprovider.WithUsageScope(context.Background(), llmprovider.UsageScope{
		Operation: "stream",
		Sequence:  2,
		Phase:     "continuation",
	})

	ch, err := metered.StreamResponse(ctx, &llmprovider.GenerateRequest{
		Model:    "lorem-fast",
		Messages: []llmprovider.Message{{Role: "user", Blocks: []*llmprovider.Block{textBlock("Tell me a story")}}},
		Params:   &llmprovider.RequestParams{MaxTokens: intPtr(50)},
	})
	if err != nil {
		log.Fatalf("StreamResponse error: %v", err)
	}

	eventCount := 0
	for event := range ch {
		eventCount++
		if event.Error != nil {
			log.Printf("Stream error: %v", event.Error)
			break
		}
		if event.Metadata != nil {
			fmt.Printf("Stream complete: model=%s tokens=%d/%d stop=%s\n",
				event.Metadata.Model, event.Metadata.InputTokens, event.Metadata.OutputTokens, event.Metadata.StopReason)
		}
	}
	fmt.Printf("Total events: %d\n\n", eventCount)

	// --- Demonstrate Unwrap ---
	fmt.Println("=== Unwrap ===")
	if wp, ok := metered.(llmprovider.WrappedProvider); ok {
		inner := wp.Unwrap()
		fmt.Printf("Unwrapped: %T (name=%s)\n", inner, inner.Name())
	}

	fmt.Println("\nDone!")
}
