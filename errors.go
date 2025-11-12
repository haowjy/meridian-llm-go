package llmprovider

import (
	"errors"
	"fmt"
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

	// ErrInvalidRequest indicates the request parameters are invalid.
	ErrInvalidRequest = errors.New("llmprovider: invalid request")

	// ErrProviderUnavailable indicates the provider service is down or unreachable.
	ErrProviderUnavailable = errors.New("llmprovider: provider unavailable")
)

// ModelError represents an error related to model validation or availability.
type ModelError struct {
	Model    string // The model that was requested
	Provider string // The provider name
	Reason   string // Human-readable explanation
	Err      error  // Wrapped error (usually ErrInvalidModel or ErrUnsupportedFeature)
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
	Field  string // The parameter field that failed validation
	Value  any    // The invalid value
	Reason string // Human-readable explanation
	Err    error  // Wrapped error (usually ErrInvalidRequest)
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

// ProviderError represents an error from the underlying provider API.
type ProviderError struct {
	Provider   string // The provider name
	StatusCode int    // HTTP status code (if applicable)
	Message    string // Error message from provider
	Retryable  bool   // Whether this error is potentially retryable
	Err        error  // Wrapped sentinel error (ErrRateLimited, ErrProviderUnavailable, etc.)
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

// IsRetryable checks if an error is potentially retryable.
// Returns true for rate limits, temporary unavailability, network errors, etc.
func IsRetryable(err error) bool {
	if err == nil {
		return false
	}

	// Check for ProviderError with Retryable flag
	var providerErr *ProviderError
	if errors.As(err, &providerErr) {
		return providerErr.Retryable
	}

	// Rate limits are always retryable
	if errors.Is(err, ErrRateLimited) {
		return true
	}

	// Provider unavailable is retryable
	if errors.Is(err, ErrProviderUnavailable) {
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
