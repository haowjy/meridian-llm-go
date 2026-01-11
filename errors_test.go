package llmprovider

import (
	"context"
	"errors"
	"testing"
)

func TestModelError(t *testing.T) {
	t.Run("Error with wrapped error", func(t *testing.T) {
		err := &ModelError{
			Code:     ErrorCodeInvalidModel,
			Model:    "gpt-5",
			Provider: "openai",
			Reason:   "model not found",
			Err:      ErrInvalidModel,
		}

		msg := err.Error()
		if msg != "model 'gpt-5' for provider 'openai': model not found (llmprovider: invalid or unsupported model)" {
			t.Errorf("unexpected error message: %s", msg)
		}
	})

	t.Run("Error without wrapped error", func(t *testing.T) {
		err := &ModelError{
			Code:     ErrorCodeInvalidModel,
			Model:    "gpt-5",
			Provider: "openai",
			Reason:   "model not found",
		}

		msg := err.Error()
		if msg != "model 'gpt-5' for provider 'openai': model not found" {
			t.Errorf("unexpected error message: %s", msg)
		}
	})

	t.Run("Unwrap returns wrapped error", func(t *testing.T) {
		err := &ModelError{
			Err: ErrInvalidModel,
		}

		if !errors.Is(err, ErrInvalidModel) {
			t.Error("expected Unwrap to return ErrInvalidModel")
		}
	})
}

func TestValidationError(t *testing.T) {
	t.Run("Error with wrapped error", func(t *testing.T) {
		err := &ValidationError{
			Code:   ErrorCodeInvalidRequest,
			Field:  "temperature",
			Value:  2.5,
			Reason: "must be between 0 and 2",
			Err:    ErrInvalidRequest,
		}

		msg := err.Error()
		expected := "validation failed for 'temperature' (value: 2.5): must be between 0 and 2 (llmprovider: invalid request)"
		if msg != expected {
			t.Errorf("unexpected error message:\ngot:  %s\nwant: %s", msg, expected)
		}
	})

	t.Run("Error without wrapped error", func(t *testing.T) {
		err := &ValidationError{
			Field:  "max_tokens",
			Value:  -1,
			Reason: "must be positive",
		}

		msg := err.Error()
		expected := "validation failed for 'max_tokens' (value: -1): must be positive"
		if msg != expected {
			t.Errorf("unexpected error message:\ngot:  %s\nwant: %s", msg, expected)
		}
	})

	t.Run("Unwrap returns wrapped error", func(t *testing.T) {
		err := &ValidationError{
			Err: ErrInvalidRequest,
		}

		if !errors.Is(err, ErrInvalidRequest) {
			t.Error("expected Unwrap to return ErrInvalidRequest")
		}
	})
}

func TestToolError(t *testing.T) {
	t.Run("Error message format", func(t *testing.T) {
		err := &ToolError{
			Code:      ErrorCodeToolExecution,
			Tool:      "web_search",
			Provider:  "anthropic",
			Model:     "claude-3-opus",
			Reason:    "search API returned 500",
			Retryable: true,
		}

		msg := err.Error()
		expected := "tool 'web_search' error for model 'claude-3-opus' (anthropic): search API returned 500"
		if msg != expected {
			t.Errorf("unexpected error message:\ngot:  %s\nwant: %s", msg, expected)
		}
	})

	t.Run("Unwrap returns wrapped error", func(t *testing.T) {
		err := &ToolError{
			Err: ErrToolUnavailable,
		}

		if !errors.Is(err, ErrToolUnavailable) {
			t.Error("expected Unwrap to return ErrToolUnavailable")
		}
	})
}

func TestProviderError(t *testing.T) {
	t.Run("Error with status code", func(t *testing.T) {
		err := &ProviderError{
			Provider:   "openai",
			StatusCode: 429,
			Message:    "rate limit exceeded",
		}

		msg := err.Error()
		expected := "provider 'openai' error (status 429): rate limit exceeded"
		if msg != expected {
			t.Errorf("unexpected error message:\ngot:  %s\nwant: %s", msg, expected)
		}
	})

	t.Run("Error without status code", func(t *testing.T) {
		err := &ProviderError{
			Provider: "anthropic",
			Message:  "connection failed",
		}

		msg := err.Error()
		expected := "provider 'anthropic' error: connection failed"
		if msg != expected {
			t.Errorf("unexpected error message:\ngot:  %s\nwant: %s", msg, expected)
		}
	})

	t.Run("Unwrap returns wrapped error", func(t *testing.T) {
		err := &ProviderError{
			Err: ErrRateLimited,
		}

		if !errors.Is(err, ErrRateLimited) {
			t.Error("expected Unwrap to return ErrRateLimited")
		}
	})
}

func TestNewProviderError(t *testing.T) {
	tests := []struct {
		name           string
		statusCode     int
		expectedCode   ErrorCode
		expectedRetry  bool
	}{
		{"401 unauthorized", 401, ErrorCodeInvalidAPIKey, false},
		{"403 forbidden", 403, ErrorCodeInvalidAPIKey, false},
		{"429 rate limited", 429, ErrorCodeRateLimited, true},
		{"502 bad gateway", 502, ErrorCodeProviderUnavailable, true},
		{"503 service unavailable", 503, ErrorCodeProviderUnavailable, true},
		{"504 gateway timeout", 504, ErrorCodeProviderUnavailable, true},
		{"500 internal error", 500, ErrorCodeProviderUnavailable, false},
		{"400 bad request", 400, ErrorCodeProviderUnavailable, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := NewProviderError("test-provider", tt.statusCode, "test message", nil)

			if err.Code != tt.expectedCode {
				t.Errorf("expected code %s, got %s", tt.expectedCode, err.Code)
			}
			if err.Retryable != tt.expectedRetry {
				t.Errorf("expected retryable=%v, got %v", tt.expectedRetry, err.Retryable)
			}
			if err.Provider != "test-provider" {
				t.Errorf("expected provider 'test-provider', got %s", err.Provider)
			}
			if err.StatusCode != tt.statusCode {
				t.Errorf("expected status code %d, got %d", tt.statusCode, err.StatusCode)
			}
		})
	}
}

func TestIsRetryable(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{"nil error", nil, false},
		{"ErrTimeout", ErrTimeout, true},
		{"ErrStreamingIdleTimeout", ErrStreamingIdleTimeout, true},
		{"context.DeadlineExceeded", context.DeadlineExceeded, true},
		{"ErrRateLimited", ErrRateLimited, true},
		{"ErrProviderUnavailable", ErrProviderUnavailable, true},
		{"ErrToolUnavailable", ErrToolUnavailable, true},
		{"ErrInvalidModel", ErrInvalidModel, false},
		{"ErrInvalidAPIKey", ErrInvalidAPIKey, false},
		{"ErrInvalidRequest", ErrInvalidRequest, false},
		{"ProviderError retryable", &ProviderError{Retryable: true}, true},
		{"ProviderError not retryable", &ProviderError{Retryable: false}, false},
		{"ToolError retryable", &ToolError{Retryable: true}, true},
		{"ToolError not retryable", &ToolError{Retryable: false}, false},
		{"wrapped ErrRateLimited", &ProviderError{Err: ErrRateLimited, Retryable: true}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsRetryable(tt.err)
			if result != tt.expected {
				t.Errorf("IsRetryable(%v) = %v, want %v", tt.err, result, tt.expected)
			}
		})
	}
}

func TestIsInvalidRequest(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{"nil error", nil, false},
		{"ErrInvalidRequest", ErrInvalidRequest, true},
		{"ErrInvalidModel", ErrInvalidModel, true},
		{"ErrUnsupportedFeature", ErrUnsupportedFeature, true},
		{"ErrUnsupportedTool", ErrUnsupportedTool, true},
		{"ValidationError", &ValidationError{Err: ErrInvalidRequest}, true},
		{"ErrRateLimited", ErrRateLimited, false},
		{"ErrProviderUnavailable", ErrProviderUnavailable, false},
		{"ErrTimeout", ErrTimeout, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsInvalidRequest(tt.err)
			if result != tt.expected {
				t.Errorf("IsInvalidRequest(%v) = %v, want %v", tt.err, result, tt.expected)
			}
		})
	}
}

func TestIsAuthError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{"nil error", nil, false},
		{"ErrInvalidAPIKey", ErrInvalidAPIKey, true},
		{"ProviderError 401", &ProviderError{StatusCode: 401}, true},
		{"ProviderError 403", &ProviderError{StatusCode: 403}, true},
		{"ProviderError 429", &ProviderError{StatusCode: 429}, false},
		{"ProviderError 500", &ProviderError{StatusCode: 500}, false},
		{"ErrRateLimited", ErrRateLimited, false},
		{"ErrInvalidRequest", ErrInvalidRequest, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsAuthError(tt.err)
			if result != tt.expected {
				t.Errorf("IsAuthError(%v) = %v, want %v", tt.err, result, tt.expected)
			}
		})
	}
}
