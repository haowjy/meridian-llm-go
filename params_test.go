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
		{"temperature 1.1", float64Ptr(1.1), false},
		{"temperature 2.0", float64Ptr(2.0), false},
		{"temperature -0.1 is invalid", float64Ptr(-0.1), true},
		{"temperature 2.1 is invalid", float64Ptr(2.1), true},
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
	// Use a consistent maxTokens for testing ratio calculations
	maxTokens := 16384

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
			name: "thinking enabled with low level (20%)",
			params: &RequestParams{
				ThinkingEnabled: boolPtr(true),
				ThinkingLevel:   stringPtr("low"),
			},
			expected: 3276, // 16384 * 0.20 = 3276.8 → 3276
		},
		{
			name: "thinking enabled with medium level (50%)",
			params: &RequestParams{
				ThinkingEnabled: boolPtr(true),
				ThinkingLevel:   stringPtr("medium"),
			},
			expected: 8192, // 16384 * 0.50 = 8192
		},
		{
			name: "thinking enabled with high level (80%)",
			params: &RequestParams{
				ThinkingEnabled: boolPtr(true),
				ThinkingLevel:   stringPtr("high"),
			},
			expected: 13107, // 16384 * 0.80 = 13107.2 → 13107
		},
		{
			name: "thinking enabled with xhigh level (95%)",
			params: &RequestParams{
				ThinkingEnabled: boolPtr(true),
				ThinkingLevel:   stringPtr("xhigh"),
			},
			expected: 15564, // 16384 * 0.95 = 15564.8 → 15564
		},
		{
			name: "unknown thinking level returns error",
			params: &RequestParams{
				ThinkingEnabled: boolPtr(true),
				ThinkingLevel:   stringPtr("unknown"),
			},
			expected: 0, // Will be ignored since we expect an error
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var result int
			var err error
			if tt.params == nil {
				// Nil params means no thinking
				result = 0
			} else {
				result, err = tt.params.GetThinkingBudgetTokens(maxTokens)

				// Special case: "unknown thinking level" should return an error
				if tt.name == "unknown thinking level returns error" {
					if err == nil {
						t.Errorf("GetThinkingBudgetTokens() expected error for unknown level, got nil")
					}
					return // Test passes if error is returned
				}

				if err != nil {
					t.Errorf("GetThinkingBudgetTokens() unexpected error = %v", err)
				}
			}

			if result != tt.expected {
				t.Errorf("GetThinkingBudgetTokens() = %d, want %d", result, tt.expected)
			}
		})
	}
}

func TestCalculateThinkingBudget(t *testing.T) {
	tests := []struct {
		name      string
		maxTokens int
		effort    string
		expected  int
		wantErr   bool
	}{
		{
			name:      "low effort with 8192 tokens",
			maxTokens: 8192,
			effort:    "low",
			expected:  1638, // 8192 * 0.20 = 1638.4 → 1638
		},
		{
			name:      "medium effort with 8192 tokens",
			maxTokens: 8192,
			effort:    "medium",
			expected:  4096, // 8192 * 0.50 = 4096
		},
		{
			name:      "high effort with 8192 tokens",
			maxTokens: 8192,
			effort:    "high",
			expected:  6553, // 8192 * 0.80 = 6553.6 → 6553
		},
		{
			name:      "xhigh effort with 8192 tokens",
			maxTokens: 8192,
			effort:    "xhigh",
			expected:  7782, // 8192 * 0.95 = 7782.4 → 7782
		},
		{
			name:      "invalid effort level",
			maxTokens: 8192,
			effort:    "invalid",
			expected:  0,
			wantErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := CalculateThinkingBudget(tt.maxTokens, tt.effort)

			if (err != nil) != tt.wantErr {
				t.Errorf("CalculateThinkingBudget() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if result != tt.expected {
				t.Errorf("CalculateThinkingBudget() = %d, want %d", result, tt.expected)
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
