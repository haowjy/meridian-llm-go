package llmprovider

import (
	"errors"
	"fmt"
)

// NewSearchTool creates a web search tool (OpenAI format).
// Providers will convert this to their specific format:
//   - Anthropic: Uses web_search_20250305
//   - OpenAI: Uses function calling with this schema
//   - Gemini: Uses FunctionDeclaration
func NewSearchTool() (*Tool, error) {
	schema := NewToolInputSchema()
	schema.AddProperty("query", PropertySchema{
		Type:        "string",
		Description: "The search query",
	}, -1)
	schema.AddRequired("query")

	tool := &Tool{
		Type: "function",
		Function: FunctionDetails{
			Name:        "search",
			Description: "Search the web for current information",
			Parameters:  schema,
		},
		ExecutionSide: ExecutionSideProvider, // Provider executes (e.g., Anthropic's built-in web_search)
	}

	if err := tool.Validate(); err != nil {
		globalLogger.Error("failed to create search tool", "error", err)
		return nil, fmt.Errorf("failed to create search tool: %w", err)
	}

	return tool, nil
}

// NewTextEditorTool creates a text editor tool (OpenAI format).
// This is a locally-executed tool for editing files (executed by backend/client).
func NewTextEditorTool() (*Tool, error) {
	schema := NewToolInputSchema()
	schema.AddProperty("path", PropertySchema{
		Type:        "string",
		Description: "Path to the file to edit",
	}, -1)
	schema.AddProperty("command", PropertySchema{
		Type:        "string",
		Description: "Editor command to execute",
	}, -1)
	schema.AddRequired("path")
	schema.AddRequired("command")

	tool := &Tool{
		Type: "function",
		Function: FunctionDetails{
			Name:        "text_editor",
			Description: "Edit text files (local execution)",
			Parameters:  schema,
		},
		ExecutionSide: ExecutionSideLocal, // Local execution (stop/execute/resume)
	}

	if err := tool.Validate(); err != nil {
		globalLogger.Error("failed to create text editor tool", "error", err)
		return nil, fmt.Errorf("failed to create text editor tool: %w", err)
	}

	return tool, nil
}

// NewBashTool creates a bash command execution tool (OpenAI format).
// This is a locally-executed tool for executing shell commands (executed by backend/client).
func NewBashTool() (*Tool, error) {
	schema := NewToolInputSchema()
	schema.AddProperty("command", PropertySchema{
		Type:        "string",
		Description: "The bash command to execute",
	}, -1)
	schema.AddRequired("command")

	tool := &Tool{
		Type: "function",
		Function: FunctionDetails{
			Name:        "bash",
			Description: "Execute bash commands (local execution)",
			Parameters:  schema,
		},
		ExecutionSide: ExecutionSideLocal, // Local execution (stop/execute/resume)
	}

	if err := tool.Validate(); err != nil {
		globalLogger.Error("failed to create bash tool", "error", err)
		return nil, fmt.Errorf("failed to create bash tool: %w", err)
	}

	return tool, nil
}

// NewCustomTool creates a custom function tool (OpenAI format) with default local execution.
// This follows the universal function calling standard used by OpenAI, Anthropic, Gemini, and OpenRouter.
// ExecutionSide defaults to Local (non-provider execution).
//
// Parameters:
//   - name: Function name (required)
//   - description: What the function does (required)
//   - schema: ToolInputSchema defining function parameters (required)
//
// Example:
//
//	schema := NewToolInputSchema()
//	schema.AddProperty("location", PropertySchema{
//	    Type:        "string",
//	    Description: "The city and state, e.g. San Francisco, CA",
//	}, -1)
//	schema.AddProperty("unit", PropertySchema{
//	    Type: "string",
//	    Enum: []string{"celsius", "fahrenheit"},
//	}, -1)
//	schema.AddRequired("location")
//	tool, err := NewCustomTool("get_weather", "Get the weather", schema)
func NewCustomTool(name string, description string, schema ToolInputSchema) (*Tool, error) {
	return NewCustomToolWithSide(name, description, schema, ExecutionSideLocal)
}

// NewCustomToolWithSide creates a custom function tool with explicit execution side.
// Use this when you need to specify where the tool is executed (Provider, Server, or Client).
func NewCustomToolWithSide(name string, description string, schema ToolInputSchema, executionSide ExecutionSide) (*Tool, error) {
	if name == "" {
		return nil, errors.New("tool name is required")
	}

	if description == "" {
		return nil, errors.New("tool description is required")
	}

	tool := &Tool{
		Type: "function",
		Function: FunctionDetails{
			Name:        name,
			Description: description,
			Parameters:  schema,
		},
		ExecutionSide: executionSide,
	}

	if err := tool.Validate(); err != nil {
		globalLogger.Error("failed to create custom tool", "error", err)
		return nil, fmt.Errorf("failed to create custom tool: %w", err)
	}

	return tool, nil
}
