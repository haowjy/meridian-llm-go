package openrouter

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// GenerationResponse wraps the OpenRouter generation stats API response.
// OpenRouter returns generation data wrapped in a "data" field.
type GenerationResponse struct {
	Data GenerationStats `json:"data"`
}

// GenerationStats represents the generation statistics from OpenRouter.
// This provides native token counts (from the model's actual tokenizer) and cost information.
// API: GET https://openrouter.ai/api/v1/generation?id={generationID}
type GenerationStats struct {
	// ID is the unique identifier for this generation (gen-...)
	ID string `json:"id"`

	// Model is the model that was used
	Model string `json:"model"`

	// ProviderName is the upstream provider that actually served the request
	// (e.g., "DeepInfra", "OpenAI", "Together")
	ProviderName string `json:"provider_name,omitempty"`

	// NativeTokensPrompt is the number of input tokens (native tokenizer)
	NativeTokensPrompt int `json:"native_tokens_prompt"`

	// NativeTokensCompletion is the number of output tokens (native tokenizer)
	NativeTokensCompletion int `json:"native_tokens_completion"`

	// NativeTokensReasoning is the number of reasoning tokens (o1, DeepSeek-R1, MiniMax)
	// These are separate from completion tokens and essential for accurate cost tracking
	NativeTokensReasoning int `json:"native_tokens_reasoning,omitempty"`

	// NativeTokensCached is the number of cached tokens (cache hits)
	NativeTokensCached int `json:"native_tokens_cached,omitempty"`

	// TotalCost is the total cost in USD for this generation
	TotalCost float64 `json:"total_cost"`

	// FinishReason indicates why generation stopped
	// (e.g., "stop", "length", "tool_use", "content_filter")
	FinishReason string `json:"finish_reason,omitempty"`

	// GenerationTime is the time taken to generate in milliseconds
	GenerationTime int64 `json:"generation_time,omitempty"`

	// CreatedAt is the timestamp when the generation was created
	CreatedAt time.Time `json:"created_at,omitempty"`

	// UpstreamID is the provider's request ID (e.g., OpenAI's request ID)
	UpstreamID string `json:"upstream_id,omitempty"`

	// Latency is the request latency in milliseconds
	Latency int64 `json:"latency,omitempty"`

	// Cancelled indicates whether this generation was cancelled
	Cancelled bool `json:"cancelled,omitempty"`

	// AdditionalFields preserves unknown JSON fields from OpenRouter API
	// This provides forward compatibility when OpenRouter adds new fields
	AdditionalFields map[string]interface{} `json:"-"`
}

// UnmarshalJSON custom implementation to preserve unknown JSON fields
func (gs *GenerationStats) UnmarshalJSON(data []byte) error {
	// Create alias to avoid recursion
	type Alias GenerationStats
	aux := &struct {
		*Alias
	}{
		Alias: (*Alias)(gs),
	}

	// Unmarshal into known fields
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	// Unmarshal into map to capture all fields
	var allFields map[string]interface{}
	if err := json.Unmarshal(data, &allFields); err != nil {
		return err
	}

	// Remove known fields from map
	knownFields := []string{
		"id", "model", "provider_name",
		"native_tokens_prompt", "native_tokens_completion",
		"native_tokens_reasoning", "native_tokens_cached",
		"total_cost", "finish_reason", "generation_time",
		"created_at", "upstream_id", "latency", "cancelled",
	}

	for _, field := range knownFields {
		delete(allFields, field)
	}

	// Store remaining unknown fields
	if len(allFields) > 0 {
		gs.AdditionalFields = allFields
	}

	return nil
}

// MarshalJSON custom implementation to merge known + unknown fields.
// Value receiver so `json.Marshal(stats)` (non-pointer) preserves AdditionalFields.
func (gs GenerationStats) MarshalJSON() ([]byte, error) {
	// Create alias to avoid recursion
	type Alias GenerationStats
	aux := Alias(gs)

	// Marshal known fields
	knownBytes, err := json.Marshal(aux)
	if err != nil {
		return nil, err
	}

	// If no additional fields, return known fields only
	if len(gs.AdditionalFields) == 0 {
		return knownBytes, nil
	}

	// Merge known + additional fields
	var knownMap map[string]interface{}
	if err := json.Unmarshal(knownBytes, &knownMap); err != nil {
		return nil, err
	}

	for k, v := range gs.AdditionalFields {
		knownMap[k] = v
	}

	return json.Marshal(knownMap)
}

// GetGenerationStats queries OpenRouter for generation statistics.
// This is useful for getting accurate native token counts after a stream is cancelled,
// since streaming responses may not include usage data.
//
// The generationID is obtained from StreamMetadata.GenerationID during streaming.
//
// Returns an error if:
// - The generation ID is empty
// - The API request fails
// - The generation is not found (404)
func (p *Provider) GetGenerationStats(ctx context.Context, generationID string) (*GenerationStats, error) {
	if generationID == "" {
		p.logger.Error("generation ID is required", "generation_id", generationID)
		return nil, fmt.Errorf("generation ID is required")
	}

	// Build the request URL
	url := fmt.Sprintf("%s/generation?id=%s", p.baseURL, generationID)

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		p.logger.Error("failed to create request", "generation_id", generationID, "error", err)
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Authorization", "Bearer "+p.apiKey)
	req.Header.Set("Content-Type", "application/json")

	// Execute request
	resp, err := p.httpClient.Do(req)
	if err != nil {
		p.logger.Error("failed to query generation stats", "generation_id", generationID, "error", err)
		return nil, fmt.Errorf("failed to query generation stats: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		p.logger.Error("failed to read response", "generation_id", generationID, "error", err)
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Handle error responses
	if resp.StatusCode != http.StatusOK {
		// Try to parse error message
		var errResp struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if json.Unmarshal(body, &errResp) == nil && errResp.Error.Message != "" {
			p.logger.Error("generation stats error", "generation_id", generationID, "status_code", resp.StatusCode, "error", errResp.Error.Message)
			return nil, fmt.Errorf("generation stats error (HTTP %d): %s", resp.StatusCode, errResp.Error.Message)
		}
		p.logger.Error("generation stats error", "generation_id", generationID, "status_code", resp.StatusCode, "error", string(body))
		return nil, fmt.Errorf("generation stats error (HTTP %d): %s", resp.StatusCode, string(body))
	}

	// Parse response (unwrap from {data:{...}} wrapper)
	var response GenerationResponse
	if err := json.Unmarshal(body, &response); err != nil {
		p.logger.Error("failed to parse generation stats", "generation_id", generationID, "error", err)
		return nil, fmt.Errorf("failed to parse generation stats: %w", err)
	}

	return &response.Data, nil
}

// CancelGeneration attempts to cancel an ongoing OpenRouter generation via API.
// This sends a DELETE request to the OpenRouter generation endpoint.
//
// Note: Success depends on whether the upstream provider supports cancellation.
// Many providers support cancellation (e.g., DeepInfra, Together), but some don't.
// If the upstream doesn't support cancel, the API will return an error, but the
// generation will continue to completion and you'll be billed for the full request.
//
// The caller should always continue with normal flow (query /generation for actual usage)
// regardless of whether this cancel succeeds or fails.
//
// Returns an error if:
// - The generation ID is empty
// - The API request fails
// - The upstream provider doesn't support cancellation
func (p *Provider) CancelGeneration(ctx context.Context, generationID string) error {
	if generationID == "" {
		p.logger.Error("generation ID is required for cancellation", "generation_id", generationID)
		return fmt.Errorf("generation ID is required")
	}

	// Build the request URL (DELETE to same endpoint as GET)
	url := fmt.Sprintf("%s/generation?id=%s", p.baseURL, generationID)

	// Create HTTP DELETE request
	req, err := http.NewRequestWithContext(ctx, "DELETE", url, nil)
	if err != nil {
		p.logger.Error("failed to create cancel request", "generation_id", generationID, "error", err)
		return fmt.Errorf("failed to create cancel request: %w", err)
	}

	// Set headers
	req.Header.Set("Authorization", "Bearer "+p.apiKey)

	// Execute request
	resp, err := p.httpClient.Do(req)
	if err != nil {
		p.logger.Error("cancel API call failed", "generation_id", generationID, "error", err)
		return fmt.Errorf("cancel API call failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	// Read response body for error details
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		p.logger.Error("failed to read cancel response", "generation_id", generationID, "error", err)
		return fmt.Errorf("failed to read cancel response: %w", err)
	}

	// Check response status
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		// Try to parse error message
		var errResp struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if json.Unmarshal(body, &errResp) == nil && errResp.Error.Message != "" {
			p.logger.Error("cancel failed", "generation_id", generationID, "status_code", resp.StatusCode, "error", errResp.Error.Message)
			return fmt.Errorf("cancel failed (HTTP %d): %s", resp.StatusCode, errResp.Error.Message)
		}
		p.logger.Error("cancel failed", "generation_id", generationID, "status_code", resp.StatusCode, "error", string(body))
		return fmt.Errorf("cancel failed (HTTP %d): %s", resp.StatusCode, string(body))
	}

	return nil
}
