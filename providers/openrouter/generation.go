package openrouter

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// GenerationStats represents the response from OpenRouter's generation stats endpoint.
// This provides native token counts (from the model's actual tokenizer) and cost information.
// API: GET https://openrouter.ai/api/v1/generation?id={generationID}
type GenerationStats struct {
	// ID is the unique identifier for this generation
	ID string `json:"id"`

	// Model is the model that was used
	Model string `json:"model"`

	// NativeTokensPrompt is the number of input tokens (native tokenizer)
	NativeTokensPrompt int `json:"native_tokens_prompt"`

	// NativeTokensCompletion is the number of output tokens (native tokenizer)
	NativeTokensCompletion int `json:"native_tokens_completion"`

	// TotalCost is the total cost in USD for this generation
	TotalCost float64 `json:"total_cost"`

	// FinishReason indicates why generation stopped
	FinishReason string `json:"finish_reason,omitempty"`

	// GenerationTime is the time taken to generate in milliseconds
	GenerationTime int64 `json:"generation_time,omitempty"`
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
		return nil, fmt.Errorf("generation ID is required")
	}

	// Build the request URL
	url := fmt.Sprintf("%s/generation?id=%s", p.baseURL, generationID)

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Authorization", "Bearer "+p.apiKey)
	req.Header.Set("Content-Type", "application/json")

	// Execute request
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to query generation stats: %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
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
			return nil, fmt.Errorf("generation stats error (HTTP %d): %s", resp.StatusCode, errResp.Error.Message)
		}
		return nil, fmt.Errorf("generation stats error (HTTP %d): %s", resp.StatusCode, string(body))
	}

	// Parse response
	var stats GenerationStats
	if err := json.Unmarshal(body, &stats); err != nil {
		return nil, fmt.Errorf("failed to parse generation stats: %w", err)
	}

	return &stats, nil
}
