package openrouter

import (
	"encoding/json"
	"testing"
	"time"
)

// TestGenerationResponse_UnwrapDataWrapper tests that the GenerationResponse
// correctly unwraps the {data:{...}} wrapper that OpenRouter returns.
func TestGenerationResponse_UnwrapDataWrapper(t *testing.T) {
	// Real OpenRouter response format with data wrapper (including new fields)
	jsonResponse := `{
		"data": {
			"id": "gen-abc123xyz",
			"model": "x-ai/grok-beta",
			"provider_name": "DeepInfra",
			"native_tokens_prompt": 51,
			"native_tokens_completion": 199,
			"native_tokens_reasoning": 132,
			"native_tokens_cached": 0,
			"total_cost": 0.00025308,
			"finish_reason": "tool_use",
			"generation_time": 1234,
			"created_at": "2026-01-13T02:11:11Z",
			"upstream_id": "743211e8ac9441d2aa5137e9869cc5e3",
			"latency": 278,
			"cancelled": false
		}
	}`

	var response GenerationResponse
	err := json.Unmarshal([]byte(jsonResponse), &response)
	if err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	// Verify all fields are populated correctly
	stats := response.Data

	if stats.ID != "gen-abc123xyz" {
		t.Errorf("expected ID 'gen-abc123xyz', got '%s'", stats.ID)
	}

	if stats.Model != "x-ai/grok-beta" {
		t.Errorf("expected Model 'x-ai/grok-beta', got '%s'", stats.Model)
	}

	if stats.ProviderName != "DeepInfra" {
		t.Errorf("expected ProviderName 'DeepInfra', got '%s'", stats.ProviderName)
	}

	if stats.NativeTokensPrompt != 51 {
		t.Errorf("expected NativeTokensPrompt 51, got %d", stats.NativeTokensPrompt)
	}

	if stats.NativeTokensCompletion != 199 {
		t.Errorf("expected NativeTokensCompletion 199, got %d", stats.NativeTokensCompletion)
	}

	if stats.NativeTokensReasoning != 132 {
		t.Errorf("expected NativeTokensReasoning 132, got %d", stats.NativeTokensReasoning)
	}

	if stats.NativeTokensCached != 0 {
		t.Errorf("expected NativeTokensCached 0, got %d", stats.NativeTokensCached)
	}

	if stats.TotalCost != 0.00025308 {
		t.Errorf("expected TotalCost 0.00025308, got %f", stats.TotalCost)
	}

	if stats.FinishReason != "tool_use" {
		t.Errorf("expected FinishReason 'tool_use', got '%s'", stats.FinishReason)
	}

	if stats.GenerationTime != 1234 {
		t.Errorf("expected GenerationTime 1234, got %d", stats.GenerationTime)
	}

	expectedTime, _ := time.Parse(time.RFC3339, "2026-01-13T02:11:11Z")
	if !stats.CreatedAt.Equal(expectedTime) {
		t.Errorf("expected CreatedAt '%v', got '%v'", expectedTime, stats.CreatedAt)
	}

	if stats.UpstreamID != "743211e8ac9441d2aa5137e9869cc5e3" {
		t.Errorf("expected UpstreamID '743211e8ac9441d2aa5137e9869cc5e3', got '%s'", stats.UpstreamID)
	}

	if stats.Latency != 278 {
		t.Errorf("expected Latency 278, got %d", stats.Latency)
	}

	if stats.Cancelled != false {
		t.Errorf("expected Cancelled false, got %v", stats.Cancelled)
	}
}

// TestGenerationResponse_MinimalFields tests parsing with only required fields.
func TestGenerationResponse_MinimalFields(t *testing.T) {
	// Minimal response with only required fields
	jsonResponse := `{
		"data": {
			"id": "gen-minimal",
			"model": "anthropic/claude-3.5-sonnet",
			"native_tokens_prompt": 100,
			"native_tokens_completion": 200,
			"total_cost": 0.001
		}
	}`

	var response GenerationResponse
	err := json.Unmarshal([]byte(jsonResponse), &response)
	if err != nil {
		t.Fatalf("failed to parse minimal response: %v", err)
	}

	stats := response.Data

	if stats.ID != "gen-minimal" {
		t.Errorf("expected ID 'gen-minimal', got '%s'", stats.ID)
	}

	if stats.Model != "anthropic/claude-3.5-sonnet" {
		t.Errorf("expected Model 'anthropic/claude-3.5-sonnet', got '%s'", stats.Model)
	}

	if stats.NativeTokensPrompt != 100 {
		t.Errorf("expected NativeTokensPrompt 100, got %d", stats.NativeTokensPrompt)
	}

	if stats.NativeTokensCompletion != 200 {
		t.Errorf("expected NativeTokensCompletion 200, got %d", stats.NativeTokensCompletion)
	}

	if stats.TotalCost != 0.001 {
		t.Errorf("expected TotalCost 0.001, got %f", stats.TotalCost)
	}

	// Optional fields should be empty/zero
	if stats.ProviderName != "" {
		t.Errorf("expected ProviderName to be empty, got '%s'", stats.ProviderName)
	}

	if stats.FinishReason != "" {
		t.Errorf("expected FinishReason to be empty, got '%s'", stats.FinishReason)
	}

	if stats.GenerationTime != 0 {
		t.Errorf("expected GenerationTime to be 0, got %d", stats.GenerationTime)
	}

	if !stats.CreatedAt.IsZero() {
		t.Errorf("expected CreatedAt to be zero, got %v", stats.CreatedAt)
	}

	if stats.Cancelled != false {
		t.Errorf("expected Cancelled to be false, got %v", stats.Cancelled)
	}
}

// TestGenerationResponse_CancelledGeneration tests parsing a cancelled generation.
func TestGenerationResponse_CancelledGeneration(t *testing.T) {
	jsonResponse := `{
		"data": {
			"id": "gen-cancelled",
			"model": "openai/gpt-4",
			"provider_name": "OpenAI",
			"native_tokens_prompt": 25,
			"native_tokens_completion": 50,
			"total_cost": 0.0005,
			"finish_reason": "cancelled",
			"cancelled": true
		}
	}`

	var response GenerationResponse
	err := json.Unmarshal([]byte(jsonResponse), &response)
	if err != nil {
		t.Fatalf("failed to parse cancelled response: %v", err)
	}

	stats := response.Data

	if stats.Cancelled != true {
		t.Errorf("expected Cancelled true, got %v", stats.Cancelled)
	}

	if stats.FinishReason != "cancelled" {
		t.Errorf("expected FinishReason 'cancelled', got '%s'", stats.FinishReason)
	}

	// Tokens should still be present even if cancelled
	if stats.NativeTokensPrompt != 25 {
		t.Errorf("expected NativeTokensPrompt 25, got %d", stats.NativeTokensPrompt)
	}

	if stats.NativeTokensCompletion != 50 {
		t.Errorf("expected NativeTokensCompletion 50, got %d", stats.NativeTokensCompletion)
	}
}

// TestGenerationStats_DirectParsing_ShouldFail tests that parsing without the wrapper fails.
// This verifies we're actually testing the fix for the {data:{}} bug.
func TestGenerationStats_DirectParsing_ShouldFail(t *testing.T) {
	// Response without data wrapper (old buggy assumption)
	jsonResponse := `{
		"id": "gen-direct",
		"model": "test-model",
		"native_tokens_prompt": 100,
		"native_tokens_completion": 200,
		"total_cost": 0.001
	}`

	// Try parsing directly into GenerationStats (should work for unwrapped)
	var stats GenerationStats
	err := json.Unmarshal([]byte(jsonResponse), &stats)
	if err != nil {
		t.Fatalf("failed to parse direct response: %v", err)
	}

	// This works, but OpenRouter doesn't return this format
	if stats.ID != "gen-direct" {
		t.Errorf("expected ID 'gen-direct', got '%s'", stats.ID)
	}

	// Now try parsing wrapped response into GenerationStats directly (should fail to populate)
	wrappedResponse := `{
		"data": {
			"id": "gen-wrapped",
			"model": "test-model",
			"native_tokens_prompt": 100,
			"native_tokens_completion": 200,
			"total_cost": 0.001
		}
	}`

	var stats2 GenerationStats
	err = json.Unmarshal([]byte(wrappedResponse), &stats2)
	if err != nil {
		t.Fatalf("failed to parse wrapped response: %v", err)
	}

	// Fields should be empty/zero because they're inside "data"
	if stats2.ID != "" {
		t.Errorf("expected ID to be empty (wrapped data not parsed), got '%s'", stats2.ID)
	}

	if stats2.NativeTokensPrompt != 0 {
		t.Errorf("expected NativeTokensPrompt to be 0 (wrapped data not parsed), got %d", stats2.NativeTokensPrompt)
	}
}

// TestGenerationStats_PreservesUnknownFields tests forward compatibility with unknown JSON fields.
func TestGenerationStats_PreservesUnknownFields(t *testing.T) {
	// Response with known fields + future unknown fields
	jsonResponse := `{
		"data": {
			"id": "gen-123",
			"model": "test-model",
			"native_tokens_prompt": 100,
			"native_tokens_completion": 200,
			"total_cost": 0.001,
			"future_field_1": "some_value",
			"future_nested": {"key": "value"},
			"future_number": 42
		}
	}`

	var response GenerationResponse
	err := json.Unmarshal([]byte(jsonResponse), &response)
	if err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	stats := response.Data

	// Verify known fields work
	if stats.ID != "gen-123" {
		t.Errorf("expected ID 'gen-123', got '%s'", stats.ID)
	}

	if stats.NativeTokensPrompt != 100 {
		t.Errorf("expected NativeTokensPrompt 100, got %d", stats.NativeTokensPrompt)
	}

	// Verify unknown fields preserved
	if stats.AdditionalFields == nil {
		t.Fatalf("expected AdditionalFields to be non-nil")
	}

	if len(stats.AdditionalFields) != 3 {
		t.Errorf("expected 3 additional fields, got %d", len(stats.AdditionalFields))
	}

	if stats.AdditionalFields["future_field_1"] != "some_value" {
		t.Errorf("expected future_field_1 'some_value', got '%v'", stats.AdditionalFields["future_field_1"])
	}

	if stats.AdditionalFields["future_number"] != float64(42) {
		t.Errorf("expected future_number 42, got '%v'", stats.AdditionalFields["future_number"])
	}

	// Verify round-trip preserves unknown fields
	marshaled, err := json.Marshal(stats)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var roundtrip GenerationStats
	err = json.Unmarshal(marshaled, &roundtrip)
	if err != nil {
		t.Fatalf("roundtrip unmarshal failed: %v", err)
	}

	if roundtrip.AdditionalFields["future_field_1"] != "some_value" {
		t.Errorf("roundtrip: expected future_field_1 'some_value', got '%v'", roundtrip.AdditionalFields["future_field_1"])
	}

	// Verify known fields still work after roundtrip
	if roundtrip.ID != "gen-123" {
		t.Errorf("roundtrip: expected ID 'gen-123', got '%s'", roundtrip.ID)
	}
}
