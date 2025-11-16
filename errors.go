package llmprovider

import (
	"context"
	"errors"
	"fmt"
)

// ErrorCode is a machine-readable error identifier
type ErrorCode string

const (
	ErrorCodeInvalidModel        ErrorCode = "INVALID_MODEL"
	ErrorCodeInvalidAPIKey       ErrorCode = "INVALID_API_KEY"
	ErrorCodeRateLimited         ErrorCode = "RATE_LIMITED"
	ErrorCodeUnsupportedFeature  ErrorCode = "UNSUPPORTED_FEATURE"
	ErrorCodeUnsupportedTool     ErrorCode = "UNSUPPORTED_TOOL"
	ErrorCodeToolUnavailable     ErrorCode = "TOOL_UNAVAILABLE"
	ErrorCodeToolExecution       ErrorCode = "TOOL_EXECUTION_FAILED"
	ErrorCodeInvalidRequest      ErrorCode = "INVALID_REQUEST"
	ErrorCodeProviderUnavailable ErrorCode = "PROVIDER_UNAVAILABLE"
	ErrorCodeTimeout             ErrorCode = "TIMEOUT"
)

// Sentinel errors for common failure modes.
// These can be checked with errors.Is().
var (
	// ErrInvalidModel indicates the requested model is not supported by the provider.
	ErrInvalidModel = errors.New("llmprovider: invalid or unsupported model")

	// ErrInvalidAPIKey indicates the API key is missing, malformed, or unauthorized.
	ErrInvalidAPIKey = errors.New("llmprovider: invalid API key")

	// ErrRateLimited indicates the provider's rate limit has been exceeded.
	ErrRateLimited = errors.New("llmprovider: rate limit exceeded")

	// ErrUnsupportedFeature indicates the requested feature is not available.
	// Examples: extended thinking on models that don't support it, vision on text-only models.
	ErrUnsupportedFeature = errors.New("llmprovider: unsupported feature")

	// ErrUnsupportedTool indicates the requested tool is not supported by the model.
	ErrUnsupportedTool = errors.New("llmprovider: tool not supported by model")

	// ErrToolUnavailable indicates a tool temporarily unavailable (e.g., search service down).
	ErrToolUnavailable = errors.New("llmprovider: tool temporarily unavailable")

	// ErrInvalidRequest indicates the request parameters are invalid.
	ErrInvalidRequest = errors.New("llmprovider: invalid request")

	// ErrProviderUnavailable indicates the provider service is down or unreachable.
	ErrProviderUnavailable = errors.New("llmprovider: provider unavailable")

	// ErrTimeout indicates the request timed out.
	ErrTimeout = errors.New("llmprovider: request timeout")
)

// ModelError represents an error related to model validation or availability.
type ModelError struct {
	Code     ErrorCode // Machine-readable error code
	Model    string    // The model that was requested
	Provider string    // The provider name
	Reason   string    // Human-readable explanation
	Err      error     // Wrapped error (usually ErrInvalidModel or ErrUnsupportedFeature)
}

func (e *ModelError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("model '%s' for provider '%s': %s (%v)", e.Model, e.Provider, e.Reason, e.Err)
	}
	return fmt.Sprintf("model '%s' for provider '%s': %s", e.Model, e.Provider, e.Reason)
}

func (e *ModelError) Unwrap() error {
	return e.Err
}

// ValidationError represents an error in request parameter validation.
type ValidationError struct {
	Code   ErrorCode // Machine-readable error code
	Field  string    // The parameter field that failed validation
	Value  any       // The invalid value
	Reason string    // Human-readable explanation
	Err    error     // Wrapped error (usually ErrInvalidRequest)
}

func (e *ValidationError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("validation failed for '%s' (value: %v): %s (%v)", e.Field, e.Value, e.Reason, e.Err)
	}
	return fmt.Sprintf("validation failed for '%s' (value: %v): %s", e.Field, e.Value, e.Reason)
}

func (e *ValidationError) Unwrap() error {
	return e.Err
}

// ToolError represents an error related to tool execution or availability.
type ToolError struct {
	Code      ErrorCode // Machine-readable error code
	Tool      string    // Tool name
	Provider  string    // Provider name
	Model     string    // Model name
	Reason    string    // Human-readable explanation
	Err       error     // Wrapped sentinel error
	Retryable bool      // Whether this error can be retried
}

func (e *ToolError) Error() string {
	return fmt.Sprintf("tool '%s' error for model '%s' (%s): %s", e.Tool, e.Model, e.Provider, e.Reason)
}

func (e *ToolError) Unwrap() error {
	return e.Err
}

// ProviderError represents an error from the underlying provider API.
type ProviderError struct {
	Code       ErrorCode // Machine-readable error code
	Provider   string    // The provider name
	StatusCode int       // HTTP status code (if applicable)
	Message    string    // Error message from provider
	Retryable  bool      // Whether this error is potentially retryable
	Err        error     // Wrapped sentinel error (ErrRateLimited, ErrProviderUnavailable, etc.)
}

func (e *ProviderError) Error() string {
	if e.StatusCode > 0 {
		return fmt.Sprintf("provider '%s' error (status %d): %s", e.Provider, e.StatusCode, e.Message)
	}
	return fmt.Sprintf("provider '%s' error: %s", e.Provider, e.Message)
}

func (e *ProviderError) Unwrap() error {
	return e.Err
}

// NewProviderError creates a ProviderError and automatically determines retryability
func NewProviderError(provider string, statusCode int, message string, err error) *ProviderError {
	// Auto-determine retryability from status code
	retryable := statusCode == 429 || statusCode == 502 || statusCode == 503 || statusCode == 504

	// Infer error code from status
	var code ErrorCode
	switch statusCode {
	case 401, 403:
		code = ErrorCodeInvalidAPIKey
	case 429:
		code = ErrorCodeRateLimited
	case 502, 503, 504:
		code = ErrorCodeProviderUnavailable
	default:
		code = ErrorCodeProviderUnavailable
	}

	return &ProviderError{
		Code:       code,
		Provider:   provider,
		StatusCode: statusCode,
		Message:    message,
		Retryable:  retryable,
		Err:        err,
	}
}

// IsRetryable checks if an error is potentially retryable.
// Returns true for rate limits, temporary unavailability, network errors, timeouts, etc.
func IsRetryable(err error) bool {
	if err == nil {
		return false
	}

	// Check for timeout (including context.DeadlineExceeded)
	if errors.Is(err, ErrTimeout) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}

	// Check for ProviderError with Retryable flag
	var providerErr *ProviderError
	if errors.As(err, &providerErr) {
		return providerErr.Retryable
	}

	// Check for ToolError with Retryable flag
	var toolErr *ToolError
	if errors.As(err, &toolErr) {
		return toolErr.Retryable
	}

	// Rate limits are always retryable
	if errors.Is(err, ErrRateLimited) {
		return true
	}

	// Provider unavailable is retryable
	if errors.Is(err, ErrProviderUnavailable) {
		return true
	}

	// Tool temporarily unavailable is retryable
	if errors.Is(err, ErrToolUnavailable) {
		return true
	}

	return false
}

// IsInvalidRequest checks if an error indicates invalid request parameters.
// These errors are not retryable and require request changes.
func IsInvalidRequest(err error) bool {
	if err == nil {
		return false
	}

	if errors.Is(err, ErrInvalidRequest) {
		return true
	}

	if errors.Is(err, ErrInvalidModel) {
		return true
	}

	if errors.Is(err, ErrUnsupportedFeature) {
		return true
	}

	if errors.Is(err, ErrUnsupportedTool) {
		return true
	}

	var validationErr *ValidationError
	if errors.As(err, &validationErr) {
		return true
	}

	return false
}

// IsAuthError checks if an error is related to authentication.
func IsAuthError(err error) bool {
	if err == nil {
		return false
	}

	if errors.Is(err, ErrInvalidAPIKey) {
		return true
	}

	var providerErr *ProviderError
	if errors.As(err, &providerErr) {
		// HTTP 401/403 indicate auth issues
		return providerErr.StatusCode == 401 || providerErr.StatusCode == 403
	}

	return false
}
