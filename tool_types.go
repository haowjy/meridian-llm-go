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
	tool := &Tool{
		Type: "function",
		Function: FunctionDetails{
			Name:        "search",
			Description: "Search the web for current information",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"query": map[string]interface{}{
						"type":        "string",
						"description": "The search query",
					},
				},
				"required": []string{"query"},
			},
		},
	}

	if err := tool.Validate(); err != nil {
		return nil, fmt.Errorf("failed to create search tool: %w", err)
	}

	return tool, nil
}

// NewTextEditorTool creates a text editor tool (OpenAI format).
// This is a client-side tool for editing files.
func NewTextEditorTool() (*Tool, error) {
	tool := &Tool{
		Type: "function",
		Function: FunctionDetails{
			Name:        "text_editor",
			Description: "Edit text files (client-side execution)",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"path": map[string]interface{}{
						"type":        "string",
						"description": "Path to the file to edit",
					},
					"command": map[string]interface{}{
						"type":        "string",
						"description": "Editor command to execute",
					},
				},
				"required": []string{"path", "command"},
			},
		},
	}

	if err := tool.Validate(); err != nil {
		return nil, fmt.Errorf("failed to create text editor tool: %w", err)
	}

	return tool, nil
}

// NewBashTool creates a bash command execution tool (OpenAI format).
// This is a client-side tool for executing shell commands.
func NewBashTool() (*Tool, error) {
	tool := &Tool{
		Type: "function",
		Function: FunctionDetails{
			Name:        "bash",
			Description: "Execute bash commands (client-side execution)",
			Parameters: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"command": map[string]interface{}{
						"type":        "string",
						"description": "The bash command to execute",
					},
				},
				"required": []string{"command"},
			},
		},
	}

	if err := tool.Validate(); err != nil {
		return nil, fmt.Errorf("failed to create bash tool: %w", err)
	}

	return tool, nil
}

// NewCustomTool creates a custom function tool (OpenAI format).
// This follows the universal function calling standard used by OpenAI, Anthropic, Gemini, and OpenRouter.
//
// Parameters:
//   - name: Function name (required)
//   - description: What the function does (required)
//   - parameters: JSON Schema object defining function parameters (required)
//
// Example parameters:
//
//	map[string]interface{}{
//	  "type": "object",
//	  "properties": map[string]interface{}{
//	    "location": map[string]interface{}{
//	      "type": "string",
//	      "description": "The city and state, e.g. San Francisco, CA",
//	    },
//	    "unit": map[string]interface{}{
//	      "type": "string",
//	      "enum": []string{"celsius", "fahrenheit"},
//	    },
//	  },
//	  "required": []string{"location"},
//	}
func NewCustomTool(name string, description string, parameters map[string]interface{}) (*Tool, error) {
	if name == "" {
		return nil, errors.New("tool name is required")
	}

	if description == "" {
		return nil, errors.New("tool description is required")
	}

	if parameters == nil {
		return nil, errors.New("parameters are required")
	}

	tool := &Tool{
		Type: "function",
		Function: FunctionDetails{
			Name:        name,
			Description: description,
			Parameters:  parameters,
		},
	}

	if err := tool.Validate(); err != nil {
		return nil, fmt.Errorf("failed to create custom tool: %w", err)
	}

	return tool, nil
}
