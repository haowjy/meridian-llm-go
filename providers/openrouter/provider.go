package openrouter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/haowjy/meridian-llm-go"
)

// Provider implements the llmprovider.Provider interface for OpenRouter's unified API.
// OpenRouter proxies requests to multiple LLM providers (Anthropic, OpenAI, Google, etc.)
// using an OpenAI-compatible format.
//
// Web Search Support:
// The :online suffix enables web search for compatible models.
// Example: "moonshotai/kimi-k2-thinking:online"
// When using web_search tool, ensure model has :online suffix.
// Base models work fine without :online when web search is not needed.
//
// Common Issues:
// - 404 errors: Verify model name at https://openrouter.ai/models
// - Tool calling: Not all models support function calling - check OpenRouter docs
type Provider struct {
	apiKey     string
	httpClient *http.Client
	baseURL    string
}

// NewProvider creates a new OpenRouter provider with the given API key.
func NewProvider(apiKey string) (*Provider, error) {
	if apiKey == "" {
		return nil, llmprovider.ErrInvalidAPIKey
	}

	return &Provider{
		apiKey:     apiKey,
		httpClient: &http.Client{Timeout: 120 * time.Second},
		baseURL:    "https://openrouter.ai/api/v1",
	}, nil
}

// Name returns the provider identifier.
func (p *Provider) Name() llmprovider.ProviderID {
	return llmprovider.ProviderOpenRouter
}

// SupportsModel returns true if this provider supports the given model.
// OpenRouter supports models in "provider/model" format (e.g., "anthropic/claude-3.5-sonnet")
// or special models like "openrouter/auto"
func (p *Provider) SupportsModel(model string) bool {
	// OpenRouter uses provider/model format
	return strings.Contains(model, "/")
}

// validateWebSearchRequirements blocks web_search tool usage with OpenRouter.
// OpenRouter's built-in search is not suitable for our use case.
//
// TODO(search): Implement custom web search tool that works across all providers.
// Once implemented, remove this block and allow web_search with OpenRouter.
func (p *Provider) validateWebSearchRequirements(req *llmprovider.GenerateRequest) error {
	// Check if request includes web_search tool
	if req.Params == nil || len(req.Params.Tools) == 0 {
		return nil
	}

	for _, tool := range req.Params.Tools {
		if tool.Function.Name == "search" || tool.Function.Name == "web_search" {
			return &llmprovider.ModelError{
				Model:    req.Model,
				Provider: p.Name().String(),
				Reason:   "web_search is not yet supported with OpenRouter - custom implementation pending. Use Anthropic provider for web search, or use other tools (doc_search, doc_view, doc_tree).",
				Err:      llmprovider.ErrInvalidModel,
			}
		}
	}

	return nil
}

// GenerateResponse generates a non-streaming response from OpenRouter.
func (p *Provider) GenerateResponse(ctx context.Context, req *llmprovider.GenerateRequest) (*llmprovider.GenerateResponse, error) {
	// Validate model
	if !p.SupportsModel(req.Model) {
		return nil, &llmprovider.ModelError{
			Model:    req.Model,
			Provider: p.Name().String(),
			Reason:   "model not supported by OpenRouter (must be in 'provider/model' format)",
			Err:      llmprovider.ErrInvalidModel,
		}
	}

	// Validate web_search requires :online suffix
	if err := p.validateWebSearchRequirements(req); err != nil {
		return nil, err
	}

	// Build OpenRouter API request (shared logic)
	openrouterReq, err := buildChatCompletionRequest(req)
	if err != nil {
		return nil, err
	}

	// Ensure streaming is disabled for this call
	openrouterReq.Stream = false

	// Make HTTP request
	httpReq, err := p.buildHTTPRequest(ctx, openrouterReq)
	if err != nil {
		return nil, err
	}

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("openrouter HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	// Handle error responses
	if resp.StatusCode != http.StatusOK {
		return nil, p.handleErrorResponse(resp)
	}

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	// Parse response
	var chatResp ChatCompletionResponse
	if err := json.Unmarshal(body, &chatResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	// Convert to library format
	response, err := convertFromChatCompletionResponse(&chatResp)
	if err != nil {
		return nil, fmt.Errorf("failed to convert response: %w", err)
	}

	return response, nil
}

// buildHTTPRequest creates an HTTP request for OpenRouter API.
func (p *Provider) buildHTTPRequest(ctx context.Context, req *ChatCompletionRequest) (*http.Request, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	// Set headers
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	httpReq.Header.Set("Content-Type", "application/json")

	return httpReq, nil
}

// handleErrorResponse parses error responses from OpenRouter.
func (p *Provider) handleErrorResponse(resp *http.Response) error {
	body, _ := io.ReadAll(resp.Body)

	// Try to parse structured error
	var errResp struct {
		Error struct {
			Code     int                    `json:"code"`
			Message  string                 `json:"message"`
			Metadata map[string]interface{} `json:"metadata"`
		} `json:"error"`
	}

	if err := json.Unmarshal(body, &errResp); err != nil || errResp.Error.Message == "" {
		// Fallback to plain text error
		return fmt.Errorf("openrouter error (HTTP %d): %s", resp.StatusCode, string(body))
	}

	// Map HTTP status codes to library errors
	switch resp.StatusCode {
	case 401:
		return llmprovider.ErrInvalidAPIKey
	case 429:
		return &llmprovider.ProviderError{
			Code:       llmprovider.ErrorCodeRateLimited,
			Provider:   p.Name().String(),
			StatusCode: resp.StatusCode,
			Message:    errResp.Error.Message,
			Retryable:  true,
			Err:        llmprovider.ErrRateLimited,
		}
	case 402:
		return &llmprovider.ProviderError{
			Code:       llmprovider.ErrorCodeProviderUnavailable,
			Provider:   p.Name().String(),
			StatusCode: resp.StatusCode,
			Message:    "insufficient credits: " + errResp.Error.Message,
			Retryable:  false,
			Err:        llmprovider.ErrProviderUnavailable,
		}
	case 408:
		return &llmprovider.ProviderError{
			Code:       llmprovider.ErrorCodeTimeout,
			Provider:   p.Name().String(),
			StatusCode: resp.StatusCode,
			Message:    errResp.Error.Message,
			Retryable:  true,
			Err:        llmprovider.ErrTimeout,
		}
	case 404:
		// Model not found - provide helpful message
		message := errResp.Error.Message
		if message == "" {
			message = "model not found on OpenRouter - verify model name at https://openrouter.ai/models"
		}
		return &llmprovider.ModelError{
			Model:    "", // Model name not available here, will be in context
			Provider: p.Name().String(),
			Reason:   message,
			Err:      llmprovider.ErrInvalidModel,
		}
	default:
		return &llmprovider.ProviderError{
			Code:       llmprovider.ErrorCodeProviderUnavailable,
			Provider:   p.Name().String(),
			StatusCode: resp.StatusCode,
			Message:    errResp.Error.Message,
			Retryable:  resp.StatusCode >= 500,
			Err:        llmprovider.ErrProviderUnavailable,
		}
	}
}
