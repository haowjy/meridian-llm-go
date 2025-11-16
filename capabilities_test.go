package llmprovider

import (
	"testing"
)

func TestConvertEffortToBudget_KnownModel(t *testing.T) {
	registry := GetCapabilityRegistry()

	tests := []struct {
		name     string
		provider string
		model    string
		effort   string
		expected int
	}{
		{
			name:     "Claude Haiku 4.5 - low effort",
			provider: "anthropic",
			model:    "claude-haiku-4-5",
			effort:   "low",
			expected: 2000,
		},
		{
			name:     "Claude Haiku 4.5 - medium effort",
			provider: "anthropic",
			model:    "claude-haiku-4-5",
			effort:   "medium",
			expected: 5000,
		},
		{
			name:     "Claude Haiku 4.5 - high effort",
			provider: "anthropic",
			model:    "claude-haiku-4-5",
			effort:   "high",
			expected: 12000,
		},
		{
			name:     "Claude Sonnet 4.5 - low effort",
			provider: "anthropic",
			model:    "claude-sonnet-4-5",
			effort:   "low",
			expected: 2000,
		},
		{
			name:     "Claude Opus 4.1 - medium effort",
			provider: "anthropic",
			model:    "claude-opus-4-1",
			effort:   "medium",
			expected: 5000,
		},
		{
			name:     "Claude 3.7 Sonnet - high effort",
			provider: "anthropic",
			model:    "claude-3-7-sonnet",
			effort:   "high",
			expected: 12000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			budget, err := registry.ConvertEffortToBudget(tt.provider, tt.model, tt.effort)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if budget != tt.expected {
				t.Errorf("expected budget %d, got %d", tt.expected, budget)
			}
		})
	}
}

func TestConvertEffortToBudget_UnknownModel_FallsBackToDefaults(t *testing.T) {
	registry := GetCapabilityRegistry()

	tests := []struct {
		name     string
		provider string
		model    string
		effort   string
		expected int
	}{
		{
			name:     "Unknown model - low effort uses default",
			provider: "anthropic",
			model:    "claude-unknown-model-99",
			effort:   "low",
			expected: 2000,
		},
		{
			name:     "Unknown model - medium effort uses default",
			provider: "anthropic",
			model:    "claude-future-model",
			effort:   "medium",
			expected: 5000,
		},
		{
			name:     "Unknown model - high effort uses default",
			provider: "anthropic",
			model:    "claude-brand-new",
			effort:   "high",
			expected: 12000,
		},
		{
			name:     "Unknown provider - low effort uses default",
			provider: "openrouter",
			model:    "some-new-model",
			effort:   "low",
			expected: 2000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			budget, err := registry.ConvertEffortToBudget(tt.provider, tt.model, tt.effort)
			if err != nil {
				t.Fatalf("expected fallback to succeed, got error: %v", err)
			}
			if budget != tt.expected {
				t.Errorf("expected default budget %d, got %d", tt.expected, budget)
			}
		})
	}
}

func TestConvertEffortToBudget_InvalidEffortLevel(t *testing.T) {
	registry := GetCapabilityRegistry()

	tests := []struct {
		name     string
		provider string
		model    string
		effort   string
	}{
		{
			name:     "Invalid effort level for known model",
			provider: "anthropic",
			model:    "claude-haiku-4-5",
			effort:   "ultra",
		},
		{
			name:     "Invalid effort level for unknown model",
			provider: "anthropic",
			model:    "claude-unknown",
			effort:   "super-high",
		},
		{
			name:     "Empty effort level",
			provider: "anthropic",
			model:    "claude-haiku-4-5",
			effort:   "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := registry.ConvertEffortToBudget(tt.provider, tt.model, tt.effort)
			if err == nil {
				t.Fatal("expected error for invalid effort level, got nil")
			}
		})
	}
}

func TestSupportsThinking_KnownModel(t *testing.T) {
	registry := GetCapabilityRegistry()

	tests := []struct {
		name     string
		provider string
		model    string
		expected bool
	}{
		{
			name:     "Claude Haiku 4.5 supports thinking",
			provider: "anthropic",
			model:    "claude-haiku-4-5",
			expected: true,
		},
		{
			name:     "Claude Sonnet 4.5 supports thinking",
			provider: "anthropic",
			model:    "claude-sonnet-4-5",
			expected: true,
		},
		{
			name:     "Claude Opus 4.1 supports thinking",
			provider: "anthropic",
			model:    "claude-opus-4-1",
			expected: true,
		},
		{
			name:     "Claude 3.7 Sonnet supports thinking",
			provider: "anthropic",
			model:    "claude-3-7-sonnet",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			supports := registry.SupportsThinking(tt.provider, tt.model)
			if supports != tt.expected {
				t.Errorf("expected SupportsThinking=%v, got %v", tt.expected, supports)
			}
		})
	}
}

func TestSupportsThinking_UnknownModel_ReturnsFalse(t *testing.T) {
	registry := GetCapabilityRegistry()

	supports := registry.SupportsThinking("anthropic", "claude-unknown-model")
	if supports {
		t.Error("expected SupportsThinking=false for unknown model, got true")
	}
}

func TestGetModelCapability_KnownModel(t *testing.T) {
	registry := GetCapabilityRegistry()

	modelCap, err := registry.GetModelCapability("anthropic", "claude-haiku-4-5")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if modelCap.ContextWindow != 200000 {
		t.Errorf("expected context window 200000, got %d", modelCap.ContextWindow)
	}

	if modelCap.MaxOutputTokens != 64000 {
		t.Errorf("expected max output tokens 64000, got %d", modelCap.MaxOutputTokens)
	}

	if !modelCap.Features.Thinking {
		t.Error("expected thinking feature to be enabled")
	}

	if !modelCap.Features.Tools {
		t.Error("expected tools feature to be enabled")
	}
}

func TestGetModelCapability_UnknownModel(t *testing.T) {
	registry := GetCapabilityRegistry()

	_, err := registry.GetModelCapability("anthropic", "claude-unknown-model")
	if err == nil {
		t.Fatal("expected error for unknown model, got nil")
	}
}
