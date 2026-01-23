package anthropic

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"github.com/haowjy/meridian-llm-go"
)

// Provider implements the llmprovider.Provider interface for Anthropic (Claude) models.
type Provider struct {
	client          *anthropic.Client
	httpClient      *http.Client // Custom HTTP client (nil = SDK default)
	logger          *slog.Logger
	debugStreamLogs bool
}

type Option func(*Provider)

func WithLogger(logger *slog.Logger) Option {
	return func(p *Provider) {
		if logger != nil {
			p.logger = logger
		}
	}
}

func WithDebugStreamLogs(enabled bool) Option {
	return func(p *Provider) {
		p.debugStreamLogs = enabled
	}
}

// WithHTTPClient sets a custom HTTP client for the provider.
// Use this to configure timeouts, transport settings, or other HTTP options.
// For streaming, consider setting Timeout: 0 and using streaming-level idle timeouts.
func WithHTTPClient(client *http.Client) Option {
	return func(p *Provider) {
		if client != nil {
			p.httpClient = client
		}
	}
}

// NewProvider creates a new Anthropic provider with the given API key.
func NewProvider(apiKey string, opts ...Option) (*Provider, error) {
	if apiKey == "" {
		return nil, llmprovider.ErrInvalidAPIKey
	}

	// Create provider struct first to apply options
	p := &Provider{
		logger: slog.Default(),
	}
	for _, opt := range opts {
		if opt != nil {
			opt(p)
		}
	}

	// Build client options
	clientOpts := []option.RequestOption{option.WithAPIKey(apiKey)}
	if p.httpClient != nil {
		clientOpts = append(clientOpts, option.WithHTTPClient(p.httpClient))
	}

	client := anthropic.NewClient(clientOpts...)
	p.client = &client

	return p, nil
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
	apiParams, err := p.buildMessageParams(req)
	if err != nil {
		return nil, err
	}

	// Call Anthropic API
	message, err := p.client.Messages.New(ctx, apiParams)
	if err != nil {
		p.logger.Error("anthropic API call failed", "model", req.Model, "error", err)
		return nil, fmt.Errorf("anthropic API call failed: %w", err)
	}

	// Extract tools from request params (pass to conversion to preserve ExecutionSide)
	var tools []llmprovider.Tool
	if req.Params != nil {
		tools = req.Params.Tools
	}

	// Convert response to library format with metadata
	response, err := p.convertFromAnthropicResponse(message, tools)
	if err != nil {
		p.logger.Error("failed to convert response", "error", err)
		return nil, fmt.Errorf("failed to convert response: %w", err)
	}

	return response, nil
}
