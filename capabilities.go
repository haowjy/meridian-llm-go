package llmprovider

import (
	_ "embed"
	"fmt"
	"os"
	"sync"

	"gopkg.in/yaml.v3"
)

//go:embed config/capabilities/anthropic.yaml
var anthropicCapabilitiesYAML []byte

// Capabilities Philosophy:
//
// This package provides MODEL METADATA for UX, pricing calculations, and informational purposes.
// It does NOT enforce validation - provider APIs are the source of truth.
//
// Use cases:
//  - Display model limits/features in UI
//  - Calculate pricing estimates
//  - Show tool availability
//  - Provide warnings (not errors)
//
// Capabilities may be outdated as providers release new models/features.
// Library users can override embedded capabilities by:
//  1. Calling LoadCapabilitiesFromFile() with custom YAML
//  2. Calling RegisterProviderCapabilities() programmatically
//
// The library trusts provider APIs to validate requests.

// ProviderCapabilities represents the full capability configuration for a provider
type ProviderCapabilities struct {
	Version     string                     `yaml:"version"`      // Semantic version (e.g., "1.0.0")
	LastUpdated string                     `yaml:"last_updated"` // ISO 8601 date (e.g., "2025-01-15")
	Provider    string                     `yaml:"provider"`
	Models      map[string]ModelCapability `yaml:"models"`
	Constraints ProviderConstraints        `yaml:"constraints"`
}

// ModelCapability represents the capabilities of a specific model
type ModelCapability struct {
	ContextWindow    int              `yaml:"context_window"`
	MaxOutputTokens  int              `yaml:"max_output_tokens"`
	Features         ModelFeatures    `yaml:"features"`
	Thinking         ThinkingCapability `yaml:"thinking"`
	Pricing          PricingInfo      `yaml:"pricing"`
	Tools            []ToolCapability `yaml:"tools"`
}

// ModelFeatures indicates which features a model supports
type ModelFeatures struct {
	Vision    bool `yaml:"vision"`
	Tools     bool `yaml:"tools"`
	Thinking  bool `yaml:"thinking"`
	Streaming bool `yaml:"streaming"`
}

// ThinkingCapability defines thinking/reasoning constraints
type ThinkingCapability struct {
	MinBudget      int            `yaml:"min_budget"`
	MaxBudget      int            `yaml:"max_budget"`
	EffortToBudget map[string]int `yaml:"effort_to_budget"` // "low" -> 2000, etc.
}

// PricingInfo contains model pricing information
type PricingInfo struct {
	InputPer1M      float64 `yaml:"input_per_1m"`
	OutputPer1M     float64 `yaml:"output_per_1m"`
	CacheWritePer1M float64 `yaml:"cache_write_per_1m"`
	CacheReadPer1M  float64 `yaml:"cache_read_per_1m"`
}

// ToolCapability represents tool support for a model
type ToolCapability struct {
	Name                string  `yaml:"name"`
	NativeSupport       bool    `yaml:"native_support"`
	ExecutionSide       string  `yaml:"execution_side"`
	PricingPer1KRequests float64 `yaml:"pricing_per_1k_requests"`
	Description         string  `yaml:"description"`
}

// ProviderConstraints defines provider-wide parameter limits
type ProviderConstraints struct {
	TemperatureMin float64 `yaml:"temperature_min"`
	TemperatureMax float64 `yaml:"temperature_max"`
	TopPMin        float64 `yaml:"top_p_min"`
	TopPMax        float64 `yaml:"top_p_max"`
	TopKMin        int     `yaml:"top_k_min"`
	TopKMax        int     `yaml:"top_k_max"`
}

// CapabilityRegistry manages provider capabilities
type CapabilityRegistry struct {
	capabilities map[string]*ProviderCapabilities
	mu           sync.RWMutex
}

var (
	globalRegistry     *CapabilityRegistry
	globalRegistryOnce sync.Once
)

// GetCapabilityRegistry returns the global capability registry (singleton)
func GetCapabilityRegistry() *CapabilityRegistry {
	globalRegistryOnce.Do(func() {
		globalRegistry = &CapabilityRegistry{
			capabilities: make(map[string]*ProviderCapabilities),
		}
		// Load embedded Anthropic capabilities
		if err := globalRegistry.loadAnthropicCapabilities(); err != nil {
			// Log error but don't panic - validation will catch missing capabilities
			fmt.Printf("Warning: failed to load Anthropic capabilities: %v\n", err)
		}
	})
	return globalRegistry
}

// loadAnthropicCapabilities loads the embedded Anthropic YAML
func (r *CapabilityRegistry) loadAnthropicCapabilities() error {
	var caps ProviderCapabilities
	if err := yaml.Unmarshal(anthropicCapabilitiesYAML, &caps); err != nil {
		return fmt.Errorf("failed to unmarshal Anthropic capabilities: %w", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.capabilities["anthropic"] = &caps

	return nil
}

// GetProviderCapabilities returns capabilities for a provider
func (r *CapabilityRegistry) GetProviderCapabilities(provider string) (*ProviderCapabilities, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	caps, ok := r.capabilities[provider]
	if !ok {
		return nil, fmt.Errorf("no capabilities found for provider: %s", provider)
	}
	return caps, nil
}

// GetModelCapability returns capabilities for a specific model
func (r *CapabilityRegistry) GetModelCapability(provider, model string) (*ModelCapability, error) {
	providerCaps, err := r.GetProviderCapabilities(provider)
	if err != nil {
		return nil, err
	}

	modelCap, ok := providerCaps.Models[model]
	if !ok {
		return nil, fmt.Errorf("model %s not found for provider %s", model, provider)
	}
	return &modelCap, nil
}

// SupportsModel checks if a provider supports a specific model
func (r *CapabilityRegistry) SupportsModel(provider, model string) bool {
	_, err := r.GetModelCapability(provider, model)
	return err == nil
}

// SupportsTools checks if a model supports tools
func (r *CapabilityRegistry) SupportsTools(provider, model string) bool {
	modelCap, err := r.GetModelCapability(provider, model)
	if err != nil {
		return false
	}
	return modelCap.Features.Tools
}

// SupportsThinking checks if a model supports extended thinking
func (r *CapabilityRegistry) SupportsThinking(provider, model string) bool {
	modelCap, err := r.GetModelCapability(provider, model)
	if err != nil {
		return false
	}
	return modelCap.Features.Thinking
}

// GetToolCapability returns tool capability for a specific tool
func (r *CapabilityRegistry) GetToolCapability(provider, model, toolName string) (*ToolCapability, error) {
	modelCap, err := r.GetModelCapability(provider, model)
	if err != nil {
		return nil, err
	}

	for _, tool := range modelCap.Tools {
		if tool.Name == toolName {
			return &tool, nil
		}
	}
	return nil, fmt.Errorf("tool %s not supported by model %s", toolName, model)
}

// GetThinkingBudgetRange returns the valid thinking budget range for a model
func (r *CapabilityRegistry) GetThinkingBudgetRange(provider, model string) (min int, max int, err error) {
	modelCap, err := r.GetModelCapability(provider, model)
	if err != nil {
		return 0, 0, err
	}
	return modelCap.Thinking.MinBudget, modelCap.Thinking.MaxBudget, nil
}

// ConvertEffortToBudget converts effort level to token budget
// Falls back to default budgets if model not found in registry
func (r *CapabilityRegistry) ConvertEffortToBudget(provider, model, effort string) (int, error) {
	// Default thinking budgets (used when model not in registry)
	defaultBudgets := map[string]int{
		"low":    2000,
		"medium": 5000,
		"high":   12000,
	}

	// Try to get model-specific budget from registry
	modelCap, err := r.GetModelCapability(provider, model)
	if err != nil {
		// Model not in registry - use defaults
		budget, ok := defaultBudgets[effort]
		if !ok {
			return 0, fmt.Errorf("unknown effort level: %s (valid: low, medium, high)", effort)
		}
		fmt.Printf("Warning: model %s not found in capability registry for provider %s, using default thinking budget: %d tokens\n", model, provider, budget)
		return budget, nil
	}

	// Use model-specific budget if available
	budget, ok := modelCap.Thinking.EffortToBudget[effort]
	if !ok {
		// Model found but effort level not defined - use defaults
		defaultBudget, defaultOk := defaultBudgets[effort]
		if !defaultOk {
			return 0, fmt.Errorf("unknown effort level: %s (valid: low, medium, high)", effort)
		}
		fmt.Printf("Warning: effort level %s not defined for model %s, using default thinking budget: %d tokens\n", effort, model, defaultBudget)
		return defaultBudget, nil
	}
	return budget, nil
}

// LoadCapabilitiesFromFile loads provider capabilities from a YAML file.
// This allows library users to override embedded capabilities with custom data.
// The file format should match the embedded YAML structure.
func (r *CapabilityRegistry) LoadCapabilitiesFromFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("failed to read capabilities file: %w", err)
	}

	var caps ProviderCapabilities
	if err := yaml.Unmarshal(data, &caps); err != nil {
		return fmt.Errorf("failed to unmarshal capabilities: %w", err)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.capabilities[caps.Provider] = &caps

	return nil
}

// RegisterProviderCapabilities programmatically registers provider capabilities.
// This allows library users to define capabilities in code rather than YAML.
func (r *CapabilityRegistry) RegisterProviderCapabilities(provider string, caps *ProviderCapabilities) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.capabilities[provider] = caps
}

// LoadCapabilitiesFromFile is a convenience function that calls the global registry's LoadCapabilitiesFromFile.
func LoadCapabilitiesFromFile(path string) error {
	return GetCapabilityRegistry().LoadCapabilitiesFromFile(path)
}

// RegisterProviderCapabilities is a convenience function that calls the global registry's RegisterProviderCapabilities.
func RegisterProviderCapabilities(provider string, caps *ProviderCapabilities) {
	GetCapabilityRegistry().RegisterProviderCapabilities(provider, caps)
}
