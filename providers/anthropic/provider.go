package anthropic

import (
	"context"
	"fmt"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"github.com/haowjy/meridian-llm-go"
)

// Provider implements the llmprovider.Provider interface for Anthropic (Claude) models.
type Provider struct {
	client *anthropic.Client
}

// NewProvider creates a new Anthropic provider with the given API key.
func NewProvider(apiKey string) (*Provider, error) {
	if apiKey == "" {
		return nil, llmprovider.ErrInvalidAPIKey
	}

	client := anthropic.NewClient(option.WithAPIKey(apiKey))

	return &Provider{
		client: &client,
	}, nil
}

// Name returns the provider identifier.
func (p *Provider) Name() llmprovider.ProviderID {
	return llmprovider.ProviderAnthropic
}

// SupportsModel returns true if this provider supports the given model.
// Anthropic models start with "claude-"
func (p *Provider) SupportsModel(model string) bool {
	return strings.HasPrefix(model, "claude-")
}

// GenerateResponse generates a response from Claude.
func (p *Provider) GenerateResponse(ctx context.Context, req *llmprovider.GenerateRequest) (*llmprovider.GenerateResponse, error) {
	// Validate model
	if !p.SupportsModel(req.Model) {
		return nil, &llmprovider.ModelError{
			Model:    req.Model,
			Provider: p.Name().String(),
			Reason:   "model not supported by Anthropic (must start with 'claude-')",
			Err:      llmprovider.ErrInvalidModel,
		}
	}

	// Build Anthropic API parameters (shared logic with StreamResponse)
	apiParams, err := buildMessageParams(req)
	if err != nil {
		return nil, err
	}

	// Call Anthropic API
	message, err := p.client.Messages.New(ctx, apiParams)
	if err != nil {
		return nil, fmt.Errorf("anthropic API call failed: %w", err)
	}

	// Convert response to library format with metadata
	response, err := convertFromAnthropicResponse(message)
	if err != nil {
		return nil, fmt.Errorf("failed to convert response: %w", err)
	}

	return response, nil
}
