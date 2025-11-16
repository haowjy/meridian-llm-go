package llmprovider

// Severity indicates how serious a validation warning is
type Severity string

const (
	SeverityInfo    Severity = "info"    // Informational (might be expected)
	SeverityWarning Severity = "warning" // Potentially problematic
	SeverityError   Severity = "error"   // Likely to cause API failure
)

// WarningCode is a machine-readable identifier for validation warnings
type WarningCode string

const (
	// Model warnings
	WarningCodeModelUnknown      WarningCode = "MODEL_UNKNOWN"
	WarningCodeCapabilityMissing WarningCode = "CAPABILITY_MISSING"

	// Tool warnings
	WarningCodeToolUnsupported        WarningCode = "TOOL_UNSUPPORTED"
	WarningCodeToolNotInCapabilities  WarningCode = "TOOL_NOT_IN_CAPABILITIES"
	WarningCodeModelDoesNotSupportTools WarningCode = "MODEL_DOES_NOT_SUPPORT_TOOLS"

	// Thinking warnings
	WarningCodeThinkingUnsupported   WarningCode = "THINKING_UNSUPPORTED"
	WarningCodeThinkingBudgetTooLow  WarningCode = "THINKING_BUDGET_TOO_LOW"
	WarningCodeThinkingBudgetTooHigh WarningCode = "THINKING_BUDGET_TOO_HIGH"
	WarningCodeThinkingLevelInvalid  WarningCode = "THINKING_LEVEL_INVALID"

	// Vision warnings
	WarningCodeVisionUnsupported WarningCode = "VISION_UNSUPPORTED"

	// Parameter warnings
	WarningCodeTemperatureOutOfRange WarningCode = "TEMPERATURE_OUT_OF_RANGE"
	WarningCodeTopPOutOfRange        WarningCode = "TOP_P_OUT_OF_RANGE"
	WarningCodeTopKOutOfRange        WarningCode = "TOP_K_OUT_OF_RANGE"
)

// ValidationWarning represents a potential issue that might cause API failure.
// These are informational - the library doesn't block requests based on warnings.
// Provider APIs are the source of truth for validation.
type ValidationWarning struct {
	Code     WarningCode // Machine-readable code
	Category string      // "model", "tool", "thinking", "parameter", "vision"
	Field    string      // Field that might cause issues
	Value    any         // The potentially problematic value
	Message  string      // Human-readable warning
	Severity Severity    // How serious this warning is
}

// ValidationRule interface allows adding custom validation logic
type ValidationRule interface {
	// Name returns a human-readable name for this rule
	Name() string

	// Check validates a request and returns warnings
	Check(provider string, req *GenerateRequest) []ValidationWarning
}
