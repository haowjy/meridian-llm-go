package llmprovider

import (
	"errors"
	"testing"
)

func TestValidateRequestParams_Temperature(t *testing.T) {
	tests := []struct {
		name        string
		temperature *float64
		wantErr     bool
	}{
		{"nil temperature is valid", nil, false},
		{"temperature 0.0", float64Ptr(0.0), false},
		{"temperature 1.0", float64Ptr(1.0), false},
		{"temperature 0.5", float64Ptr(0.5), false},
		{"temperature -0.1 is invalid", float64Ptr(-0.1), true},
		{"temperature 1.1 is invalid", float64Ptr(1.1), true},
		{"temperature 2.0 is invalid", float64Ptr(2.0), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params := &RequestParams{
				Temperature: tt.temperature,
			}
			err := ValidateRequestParams(params)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateRequestParams() error = %v, wantErr %v", err, tt.wantErr)
			}

			if err != nil && !IsInvalidRequest(err) {
				t.Error("validation error should be classified as invalid request")
			}
		})
	}
}

func TestValidateRequestParams_TopP(t *testing.T) {
	tests := []struct {
		name    string
		topP    *float64
		wantErr bool
	}{
		{"nil topP is valid", nil, false},
		{"topP 0.0", float64Ptr(0.0), false},
		{"topP 1.0", float64Ptr(1.0), false},
		{"topP 0.5", float64Ptr(0.5), false},
		{"topP -0.1 is invalid", float64Ptr(-0.1), true},
		{"topP 1.1 is invalid", float64Ptr(1.1), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params := &RequestParams{
				TopP: tt.topP,
			}
			err := ValidateRequestParams(params)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateRequestParams() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateRequestParams_TopK(t *testing.T) {
	tests := []struct {
		name    string
		topK    *int
		wantErr bool
	}{
		{"nil topK is valid", nil, false},
		{"topK 0 is valid", intPtr(0), false},
		{"topK 1", intPtr(1), false},
		{"topK 100", intPtr(100), false},
		{"topK -1 is invalid", intPtr(-1), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params := &RequestParams{
				TopK: tt.topK,
			}
			err := ValidateRequestParams(params)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateRequestParams() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestValidateRequestParams_MaxTokens(t *testing.T) {
	tests := []struct {
		name      string
		maxTokens *int
		wantErr   bool
	}{
		{"nil maxTokens is valid", nil, false},
		{"maxTokens 1", intPtr(1), false},
		{"maxTokens 4096", intPtr(4096), false},
		{"maxTokens 0 is invalid", intPtr(0), true},
		{"maxTokens -1 is invalid", intPtr(-1), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params := &RequestParams{
				MaxTokens: tt.maxTokens,
			}
			err := ValidateRequestParams(params)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateRequestParams() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestRequestParams_GetMaxTokens(t *testing.T) {
	tests := []struct {
		name         string
		params       *RequestParams
		defaultValue int
		expected     int
	}{
		{
			name:         "nil params uses default",
			params:       nil,
			defaultValue: 1000,
			expected:     1000,
		},
		{
			name: "nil maxTokens uses default",
			params: &RequestParams{
				MaxTokens: nil,
			},
			defaultValue: 1000,
			expected:     1000,
		},
		{
			name: "zero maxTokens returns zero",
			params: &RequestParams{
				MaxTokens: intPtr(0),
			},
			defaultValue: 1000,
			expected:     0,
		},
		{
			name: "positive maxTokens is used",
			params: &RequestParams{
				MaxTokens: intPtr(500),
			},
			defaultValue: 1000,
			expected:     500,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var result int
			if tt.params == nil {
				// For nil params, just expect the default value
				result = tt.defaultValue
			} else {
				result = tt.params.GetMaxTokens(tt.defaultValue)
			}

			if result != tt.expected {
				t.Errorf("GetMaxTokens(%d) = %d, want %d", tt.defaultValue, result, tt.expected)
			}
		})
	}
}

func TestRequestParams_GetThinkingBudgetTokens(t *testing.T) {
	tests := []struct {
		name     string
		params   *RequestParams
		expected int
	}{
		{
			name:     "nil params returns 0",
			params:   nil,
			expected: 0,
		},
		{
			name: "thinking disabled returns 0",
			params: &RequestParams{
				ThinkingEnabled: boolPtr(false),
			},
			expected: 0,
		},
		{
			name: "thinking enabled with low level",
			params: &RequestParams{
				ThinkingEnabled: boolPtr(true),
				ThinkingLevel:   stringPtr("low"),
			},
			expected: 2000,
		},
		{
			name: "thinking enabled with medium level",
			params: &RequestParams{
				ThinkingEnabled: boolPtr(true),
				ThinkingLevel:   stringPtr("medium"),
			},
			expected: 5000,
		},
		{
			name: "thinking enabled with high level",
			params: &RequestParams{
				ThinkingEnabled: boolPtr(true),
				ThinkingLevel:   stringPtr("high"),
			},
			expected: 12000,
		},
		{
			name: "unknown thinking level returns 0",
			params: &RequestParams{
				ThinkingEnabled: boolPtr(true),
				ThinkingLevel:   stringPtr("unknown"),
			},
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var result int
			if tt.params == nil {
				// Nil params means no thinking
				result = 0
			} else {
				result = tt.params.GetThinkingBudgetTokens()
			}

			if result != tt.expected {
				t.Errorf("GetThinkingBudgetTokens() = %d, want %d", result, tt.expected)
			}
		})
	}
}

func TestValidationError_Error(t *testing.T) {
	err := &ValidationError{
		Field:  "temperature",
		Value:  1.5,
		Reason: "must be between 0 and 1",
		Err:    ErrInvalidRequest,
	}

	msg := err.Error()
	if msg == "" {
		t.Error("error message is empty")
	}

	// Check that error can be unwrapped
	if !errors.Is(err, ErrInvalidRequest) {
		t.Error("ValidationError should wrap ErrInvalidRequest")
	}
}

// Helper functions are in test_helpers.go
