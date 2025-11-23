package llmprovider

import (
	"encoding/json"
	"fmt"
)

// RequestParams represents all possible LLM request parameters across providers.
// All fields are optional pointers to distinguish "not set" from "set to zero value".
type RequestParams struct {
	// ===== Core Parameters (Most Providers) =====

	// Model specifies the LLM model to use (e.g., "claude-haiku-4-5-20251001")
	// Can be overridden at request time
	Model *string `json:"model,omitempty"`

	// MaxTokens sets the maximum number of tokens to generate
	MaxTokens *int `json:"max_tokens,omitempty"`

	// Temperature controls randomness (0.0-1.0)
	// 0.0 = deterministic, 1.0 = maximum randomness
	Temperature *float64 `json:"temperature,omitempty"`

	// TopP (nucleus sampling) - cumulative probability cutoff (0.0-1.0)
	TopP *float64 `json:"top_p,omitempty"`

	// TopK limits sampling to top K tokens
	TopK *int `json:"top_k,omitempty"`

	// Stop sequences - generation stops if any of these are generated
	Stop []string `json:"stop,omitempty"`

	// Seed for deterministic sampling (if supported by provider)
	Seed *int `json:"seed,omitempty"`

	// ===== Anthropic-Specific Parameters =====

	// ThinkingEnabled enables extended thinking mode (Claude only)
	ThinkingEnabled *bool `json:"thinking_enabled,omitempty"`

	// ThinkingLevel sets the thinking budget: "low", "medium", "high"
	// Maps to token budgets: low=2000, medium=5000, high=12000
	ThinkingLevel *string `json:"thinking_level,omitempty"`

	// ThinkingBudget explicitly sets thinking token budget (overrides ThinkingLevel)
	// Valid range depends on model (e.g., Claude: 1024-200000)
	ThinkingBudget *int `json:"thinking_budget,omitempty"`

	// System prompt override (can also be set per turn)
	System *string `json:"system,omitempty"`

	// ===== OpenAI-Specific Parameters =====

	// FrequencyPenalty reduces repetition of token sequences (-2.0 to 2.0)
	FrequencyPenalty *float64 `json:"frequency_penalty,omitempty"`

	// PresencePenalty reduces repetition of topics (-2.0 to 2.0)
	PresencePenalty *float64 `json:"presence_penalty,omitempty"`

	// RepetitionPenalty reduces token repetition (some providers)
	RepetitionPenalty *float64 `json:"repetition_penalty,omitempty"`

	// MinP - minimum probability threshold for sampling
	MinP *float64 `json:"min_p,omitempty"`

	// TopA - top-a sampling parameter
	TopA *float64 `json:"top_a,omitempty"`

	// LogitBias adjusts likelihood of specific tokens
	LogitBias map[string]float64 `json:"logit_bias,omitempty"`

	// LogProbs returns log probabilities of output tokens
	LogProbs *bool `json:"logprobs,omitempty"`

	// TopLogProbs specifies how many top logprobs to return per token
	TopLogProbs *int `json:"top_logprobs,omitempty"`

	// ResponseFormat for structured outputs (JSON mode, etc.)
	ResponseFormat *ResponseFormat `json:"response_format,omitempty"`

	// ===== Tool Parameters =====

	// LegacyTools - legacy OpenAI format (for backward compatibility)
	// DEPRECATED: Use Tools field for new code
	LegacyTools []LegacyTool `json:"legacy_tools,omitempty"`

	// Tools available for the model to use (unified format)
	// Includes built-in tools (search, text_editor, bash) and custom tools
	// Use NewSearchTool(), NewTextEditorTool(), NewCustomTool() to create
	Tools []Tool `json:"tools,omitempty"`

	// ToolChoice controls whether/which tools to use
	// Can be interface{} for provider-specific formats or use NewToolChoice()
	ToolChoice interface{} `json:"tool_choice,omitempty"`

	// ParallelToolCalls allows model to use multiple tools simultaneously
	ParallelToolCalls *bool `json:"parallel_tool_calls,omitempty"`

	// ===== Provider Routing (OpenRouter) =====

	// Provider specifies which provider to use (OpenRouter)
	// Format: "anthropic/claude-haiku-4-5", "openai/gpt-4", etc.
	Provider *string `json:"provider,omitempty"`

	// FallbackModels lists alternative models if primary fails
	FallbackModels []string `json:"fallback_models,omitempty"`
}

// ResponseFormat specifies the format for structured outputs
type ResponseFormat struct {
	Type       string      `json:"type"`                  // "text", "json_object", "json_schema"
	JSONSchema interface{} `json:"json_schema,omitempty"` // Schema for structured output
}

// LegacyTool represents a function the model can call (OpenAI format)
// DEPRECATED: Use Tool from tools.go for new code
type LegacyTool struct {
	Type     string       `json:"type"` // "function"
	Function ToolFunction `json:"function"`
}

// ToolFunction defines a callable function (legacy OpenAI format)
type ToolFunction struct {
	Name        string      `json:"name"`
	Description string      `json:"description,omitempty"`
	Parameters  interface{} `json:"parameters,omitempty"` // JSON schema for parameters
}

// ValidateRequestParams validates request parameters
func ValidateRequestParams(params *RequestParams) error {
	if params == nil {
		return nil // nil params is valid
	}

	// Validate ranges
	if params.Temperature != nil {
		if *params.Temperature < 0.0 || *params.Temperature > 2.0 {
			return &ValidationError{
				Field:  "temperature",
				Value:  *params.Temperature,
				Reason: "must be between 0.0 and 2.0",
				Err:    ErrInvalidRequest,
			}
		}
	}

	if params.TopP != nil {
		if *params.TopP < 0.0 || *params.TopP > 1.0 {
			return &ValidationError{
				Field:  "top_p",
				Value:  *params.TopP,
				Reason: "must be between 0.0 and 1.0",
				Err:    ErrInvalidRequest,
			}
		}
	}

	if params.TopK != nil {
		if *params.TopK < 0 {
			return &ValidationError{
				Field:  "top_k",
				Value:  *params.TopK,
				Reason: "must be non-negative",
				Err:    ErrInvalidRequest,
			}
		}
	}

	if params.MaxTokens != nil {
		if *params.MaxTokens < 1 {
			return &ValidationError{
				Field:  "max_tokens",
				Value:  *params.MaxTokens,
				Reason: "must be positive",
				Err:    ErrInvalidRequest,
			}
		}
	}

	if params.ThinkingLevel != nil {
		validLevels := map[string]bool{"low": true, "medium": true, "high": true}
		if !validLevels[*params.ThinkingLevel] {
			return &ValidationError{
				Field:  "thinking_level",
				Value:  *params.ThinkingLevel,
				Reason: "must be 'low', 'medium', or 'high'",
				Err:    ErrInvalidRequest,
			}
		}
	}

	if params.FrequencyPenalty != nil {
		if *params.FrequencyPenalty < -2.0 || *params.FrequencyPenalty > 2.0 {
			return &ValidationError{
				Field:  "frequency_penalty",
				Value:  *params.FrequencyPenalty,
				Reason: "must be between -2.0 and 2.0",
				Err:    ErrInvalidRequest,
			}
		}
	}

	if params.PresencePenalty != nil {
		if *params.PresencePenalty < -2.0 || *params.PresencePenalty > 2.0 {
			return &ValidationError{
				Field:  "presence_penalty",
				Value:  *params.PresencePenalty,
				Reason: "must be between -2.0 and 2.0",
				Err:    ErrInvalidRequest,
			}
		}
	}

	return nil
}

// GetRequestParamStruct unmarshals a JSONB map into a typed RequestParams struct
func GetRequestParamStruct(params map[string]interface{}) (*RequestParams, error) {
	if params == nil {
		return &RequestParams{}, nil
	}

	jsonBytes, err := json.Marshal(params)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal params: %w", err)
	}

	var rp RequestParams
	if err := json.Unmarshal(jsonBytes, &rp); err != nil {
		return nil, fmt.Errorf("failed to unmarshal params: %w", err)
	}

	return &rp, nil
}

// GetMaxTokens returns max_tokens with default fallback
func (rp *RequestParams) GetMaxTokens(defaultValue int) int {
	if rp.MaxTokens != nil {
		return *rp.MaxTokens
	}
	return defaultValue
}

// GetTemperature returns temperature with default fallback
func (rp *RequestParams) GetTemperature(defaultValue float64) float64 {
	if rp.Temperature != nil {
		return *rp.Temperature
	}
	return defaultValue
}

// GetThinkingBudgetTokens converts thinking_level to token budget using default budgets
func (rp *RequestParams) GetThinkingBudgetTokens() (int, error) {
	if rp.ThinkingLevel == nil {
		return 0, nil // Thinking not enabled
	}

	return ConvertEffortToBudget(*rp.ThinkingLevel)
}

// ConvertEffortToBudget converts effort level to token budget
// Uses standard default budgets across all providers
func ConvertEffortToBudget(effort string) (int, error) {
	budgets := map[string]int{
		"low":    2000,
		"medium": 5000,
		"high":   12000,
	}

	budget, ok := budgets[effort]
	if !ok {
		return 0, fmt.Errorf("unknown effort level: %s (valid: low, medium, high)", effort)
	}

	return budget, nil
}
