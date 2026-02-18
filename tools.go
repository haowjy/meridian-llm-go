package llmprovider

import (
	"errors"
	"fmt"
)

// Tool type constants (for unified tools array)
const (
	ToolTypeSearch     = "search"
	ToolTypeTextEditor = "text_editor"
	ToolTypeBash       = "bash"
	ToolTypeCustom     = "custom"
)

// ToolCategory represents the type of tool functionality
type ToolCategory string

const (
	ToolCategorySearch   ToolCategory = "search"
	ToolCategoryCodeExec ToolCategory = "code_exec"
	ToolCategoryFileEdit ToolCategory = "file_edit"
	ToolCategoryCustom   ToolCategory = "custom"
)

// ExecutionSide indicates where tool execution happens
type ExecutionSide string

const (
	ExecutionSideProvider ExecutionSide = "provider" // LLM provider executes (e.g., Anthropic's built-in web_search)
	ExecutionSideLocal    ExecutionSide = "local"    // Non-provider execution (e.g., backend/client handles stop/execute/resume)
)

// ToolChoiceMode controls tool selection behavior
type ToolChoiceMode string

const (
	ToolChoiceModeAuto     ToolChoiceMode = "auto"     // Model decides whether to use tools
	ToolChoiceModeRequired ToolChoiceMode = "required" // Model must use a tool
	ToolChoiceModeNone     ToolChoiceMode = "none"     // Model cannot use tools
	ToolChoiceModeSpecific ToolChoiceMode = "specific" // Model must use specific tool
)

// FunctionDetails represents the function definition within a tool (OpenAI format).
// This matches the universal standard used by OpenAI, OpenRouter, and easily converts to Anthropic/Gemini.
type FunctionDetails struct {
	Name        string          `json:"name"`                  // Function name (required)
	Description string          `json:"description,omitempty"` // What the function does
	Parameters  ToolInputSchema `json:"parameters"`            // Typed JSON Schema for parameters
}

// Tool represents a function tool (OpenAI universal format).
// This format is the industry standard and cleanly converts to all providers:
//   - OpenAI: Use directly (native format)
//   - Anthropic: Flatten and rename (parameters -> input_schema)
//   - Gemini: Flatten and rename (parameters -> parameters_json_schema)
type Tool struct {
	Type          string           `json:"type"`     // Always "function" for function tools
	Function      FunctionDetails  `json:"function"` // Function definition
	ExecutionSide ExecutionSide    `json:"-"`        // Where tool is executed (not sent to API), defaults to Local (non-provider)
}

// Validate checks if the Tool is properly configured
func (t *Tool) Validate() error {
	if t.Type == "" {
		return errors.New("tool type is required")
	}

	if t.Type != "function" {
		globalLogger.Error("unsupported tool type", "tool_type", t.Type)
		return fmt.Errorf("unsupported tool type: %s (only 'function' is supported)", t.Type)
	}

	if t.Function.Name == "" {
		return errors.New("function name is required")
	}

	// Validate that parameters has type "object" (JSON Schema requirement)
	if t.Function.Parameters.Type != "object" {
		return errors.New("function parameters must be a JSON schema with type 'object'")
	}

	return nil
}

// ToolChoice specifies tool selection behavior
type ToolChoice struct {
	Mode     ToolChoiceMode // Selection mode
	ToolName *string        // Required when Mode is ToolChoiceModeSpecific
}

// Validate checks if the ToolChoice is properly configured
func (tc *ToolChoice) Validate() error {
	if tc.Mode == ToolChoiceModeSpecific && tc.ToolName == nil {
		return errors.New("tool_name is required when mode is 'specific'")
	}

	if tc.Mode == ToolChoiceModeSpecific && *tc.ToolName == "" {
		return errors.New("tool_name cannot be empty when mode is 'specific'")
	}

	// Validate mode is one of the known values
	switch tc.Mode {
	case ToolChoiceModeAuto, ToolChoiceModeRequired, ToolChoiceModeNone, ToolChoiceModeSpecific:
		// Valid mode
	default:
		globalLogger.Error("invalid tool choice mode", "mode", tc.Mode)
		return fmt.Errorf("invalid tool choice mode: %s", tc.Mode)
	}

	return nil
}

// NewToolChoice creates a new ToolChoice with the specified mode
func NewToolChoice(mode ToolChoiceMode) (*ToolChoice, error) {
	tc := &ToolChoice{
		Mode: mode,
	}

	if err := tc.Validate(); err != nil {
		globalLogger.Error("invalid tool choice", "error", err)
		return nil, fmt.Errorf("invalid tool choice: %w", err)
	}

	return tc, nil
}

// NewSpecificToolChoice creates a ToolChoice for a specific tool
func NewSpecificToolChoice(toolName string) (*ToolChoice, error) {
	tc := &ToolChoice{
		Mode:     ToolChoiceModeSpecific,
		ToolName: &toolName,
	}

	if err := tc.Validate(); err != nil {
		globalLogger.Error("invalid specific tool choice", "error", err)
		return nil, fmt.Errorf("invalid specific tool choice: %w", err)
	}

	return tc, nil
}

// MapToolByName creates a built-in tool from a user-friendly name.
// Supports multiple aliases for convenience.
//
// Supported names:
//   - "web_search", "search" -> Search tool
//   - "text_editor", "file_edit" -> Text editor tool
//   - "bash", "code_exec" -> Bash tool
//
// Returns error if the name doesn't match any built-in tool.
func MapToolByName(name string) (*Tool, error) {
	switch name {
	case "web_search", "search":
		return NewSearchTool()
	case "text_editor", "file_edit":
		return NewTextEditorTool()
	case "bash", "code_exec":
		return NewBashTool()
	default:
		globalLogger.Error("unknown built-in tool", "tool_name", name)
		return nil, fmt.Errorf("unknown built-in tool: %s", name)
	}
}
