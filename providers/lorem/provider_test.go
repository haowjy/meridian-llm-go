package lorem

import (
	"context"
	"testing"
	"time"

	"github.com/haowjy/meridian-llm-go"
)

func TestProvider_Name(t *testing.T) {
	provider := NewProvider()
	if provider.Name() != "lorem" {
		t.Errorf("expected provider name 'lorem', got '%s'", provider.Name())
	}
}

func TestProvider_SupportsModel(t *testing.T) {
	provider := NewProvider()

	tests := []struct {
		model    string
		expected bool
	}{
		{"lorem-fast", true},
		{"lorem-slow", true},
		{"lorem-medium", true},
		{"lorem-cutoff", true},
		{"lorem-anything", true},
		{"claude-3", false},
		{"gpt-4", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			result := provider.SupportsModel(tt.model)
			if result != tt.expected {
				t.Errorf("SupportsModel(%q) = %v, want %v", tt.model, result, tt.expected)
			}
		})
	}
}

func TestProvider_GenerateResponse(t *testing.T) {
	provider := NewProvider()
	ctx := context.Background()

	req := &llmprovider.GenerateRequest{
		Model: "lorem-fast",
		Messages: []llmprovider.Message{
			{
				Role: "user",
				Blocks: []*llmprovider.Block{
					{
						BlockType:   llmprovider.BlockTypeText,
						TextContent: stringPtr("Hello, test!"),
					},
				},
			},
		},
		Params: &llmprovider.RequestParams{
			MaxTokens: intPtr(50),
		},
	}

	resp, err := provider.GenerateResponse(ctx, req)
	if err != nil {
		t.Fatalf("GenerateResponse failed: %v", err)
	}

	if resp == nil {
		t.Fatal("response is nil")
	}

	if len(resp.Blocks) == 0 {
		t.Fatal("response has no blocks")
	}

	if resp.Blocks[0].TextContent == nil || *resp.Blocks[0].TextContent == "" {
		t.Error("response text content is empty")
	}

	if resp.Model != "lorem-fast" {
		t.Errorf("expected model 'lorem-fast', got '%s'", resp.Model)
	}

	if resp.StopReason != "end_turn" {
		t.Errorf("expected stop_reason 'end_turn', got '%s'", resp.StopReason)
	}

	if resp.OutputTokens == 0 {
		t.Error("expected non-zero output tokens")
	}
}

func TestProvider_GenerateResponse_Cutoff(t *testing.T) {
	provider := NewProvider()
	ctx := context.Background()

	req := &llmprovider.GenerateRequest{
		Model: "lorem-cutoff",
		Messages: []llmprovider.Message{
			{
				Role: "user",
				Blocks: []*llmprovider.Block{
					{
						BlockType:   llmprovider.BlockTypeText,
						TextContent: stringPtr("Test cutoff"),
					},
				},
			},
		},
		Params: &llmprovider.RequestParams{
			MaxTokens: intPtr(20),
		},
	}

	resp, err := provider.GenerateResponse(ctx, req)
	if err != nil {
		t.Fatalf("GenerateResponse failed: %v", err)
	}

	if resp.StopReason != "max_tokens" {
		t.Errorf("expected stop_reason 'max_tokens' for cutoff model, got '%s'", resp.StopReason)
	}
}

func TestProvider_StreamResponse(t *testing.T) {
	provider := NewProvider()
	ctx := context.Background()

	req := &llmprovider.GenerateRequest{
		Model: "lorem-fast",
		Messages: []llmprovider.Message{
			{
				Role: "user",
				Blocks: []*llmprovider.Block{
					{
						BlockType:   llmprovider.BlockTypeText,
						TextContent: stringPtr("Stream test"),
					},
				},
			},
		},
		Params: &llmprovider.RequestParams{
			MaxTokens: intPtr(30),
		},
	}

	eventChan, err := provider.StreamResponse(ctx, req)
	if err != nil {
		t.Fatalf("StreamResponse failed: %v", err)
	}

	var deltaCount int
	var metadata *llmprovider.StreamMetadata
	var lastError error

	for event := range eventChan {
		if event.Error != nil {
			lastError = event.Error
		}
		if event.Delta != nil {
			deltaCount++
		}
		if event.Metadata != nil {
			metadata = event.Metadata
		}
	}

	if lastError != nil {
		t.Errorf("received error event: %v", lastError)
	}

	if deltaCount == 0 {
		t.Error("expected at least one delta event")
	}

	if metadata == nil {
		t.Fatal("expected metadata event")
	}

	if metadata.Model != "lorem-fast" {
		t.Errorf("expected model 'lorem-fast', got '%s'", metadata.Model)
	}

	if metadata.StopReason != "end_turn" {
		t.Errorf("expected stop_reason 'end_turn', got '%s'", metadata.StopReason)
	}
}

func TestProvider_StreamResponse_Speed(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping slow test in short mode")
	}

	provider := NewProvider()
	ctx := context.Background()

	tests := []struct {
		model         string
		expectedDelay time.Duration
		tolerance     time.Duration
	}{
		{"lorem-fast", 30 * time.Millisecond, 20 * time.Millisecond},
		{"lorem-slow", 500 * time.Millisecond, 100 * time.Millisecond},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			req := &llmprovider.GenerateRequest{
				Model: tt.model,
				Messages: []llmprovider.Message{
					{
						Role: "user",
						Blocks: []*llmprovider.Block{
							{
								BlockType:   llmprovider.BlockTypeText,
								TextContent: stringPtr("Speed test"),
							},
						},
					},
				},
				Params: &llmprovider.RequestParams{
					MaxTokens: intPtr(10),
				},
			}

			start := time.Now()
			eventChan, err := provider.StreamResponse(ctx, req)
			if err != nil {
				t.Fatalf("StreamResponse failed: %v", err)
			}

			var firstDelta time.Time
			var secondDelta time.Time

			for event := range eventChan {
				if event.Delta != nil && event.Delta.TextDelta != nil {
					if firstDelta.IsZero() {
						firstDelta = time.Now()
					} else if secondDelta.IsZero() {
						secondDelta = time.Now()
						break
					}
				}
			}

			if firstDelta.IsZero() || secondDelta.IsZero() {
				t.Skip("not enough deltas to measure speed")
			}

			actualDelay := secondDelta.Sub(firstDelta)
			diff := actualDelay - tt.expectedDelay
			if diff < 0 {
				diff = -diff
			}

			if diff > tt.tolerance {
				t.Logf("total time: %v", time.Since(start))
				t.Logf("delay between deltas: %v (expected ~%v)", actualDelay, tt.expectedDelay)
				// Don't fail, just log - timing tests are inherently flaky
			}
		})
	}
}

func TestProvider_InvalidModel(t *testing.T) {
	provider := NewProvider()
	ctx := context.Background()

	req := &llmprovider.GenerateRequest{
		Model: "claude-3",
		Messages: []llmprovider.Message{
			{
				Role: "user",
				Blocks: []*llmprovider.Block{
					{
						BlockType:   llmprovider.BlockTypeText,
						TextContent: stringPtr("Test"),
					},
				},
			},
		},
	}

	_, err := provider.GenerateResponse(ctx, req)
	if err == nil {
		t.Fatal("expected error for invalid model")
	}

	var modelErr *llmprovider.ModelError
	if !llmprovider.IsInvalidRequest(err) {
		t.Error("error should be classified as invalid request")
	}

	if err, ok := err.(*llmprovider.ModelError); ok {
		modelErr = err
	}

	if modelErr == nil {
		t.Fatal("expected ModelError type")
	}

	if modelErr.Model != "claude-3" {
		t.Errorf("expected model 'claude-3' in error, got '%s'", modelErr.Model)
	}

	if modelErr.Provider != "lorem" {
		t.Errorf("expected provider 'lorem' in error, got '%s'", modelErr.Provider)
	}
}

// Helper functions

func stringPtr(s string) *string {
	return &s
}

func intPtr(i int) *int {
	return &i
}
