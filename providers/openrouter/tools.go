package openrouter

import (
	"fmt"

	"github.com/haowjy/meridian-llm-go"
)

// convertToOpenRouterTools converts library Tool format to OpenRouter format.
// OpenRouter uses OpenAI-compatible format, so most tools pass through directly.
func (p *Provider) convertToOpenRouterTools(tools []llmprovider.Tool) ([]Tool, error) {
	if len(tools) == 0 {
		return nil, nil
	}

	result := make([]Tool, 0, len(tools))

	for i, tool := range tools {
		var openrouterTool Tool
		var err error

		// Route based on function name (OpenAI format uses tool.Function.Name)
		switch tool.Function.Name {
		case "search":
			// OpenRouter has a web plugin for search
			// However, it's not universally supported across all models
			// For now, convert it as a custom tool with search semantics
			openrouterTool, err = p.convertSearchTool(&tool)

		default:
			// All other tools use standard OpenAI function format
			openrouterTool, err = convertCustomTool(&tool)
		}

		if err != nil {
			p.logger.Error("tool conversion failed", "tool_index", i, "tool_name", tool.Function.Name, "error", err)
			return nil, fmt.Errorf("tool %d (%s): %w", i, tool.Function.Name, err)
		}

		result = append(result, openrouterTool)
	}

	return result, nil
}

// convertSearchTool converts search tool to OpenRouter format.
// OpenRouter supports web search through some models, but format varies.
// We convert it as a standard function tool with search semantics.
func (p *Provider) convertSearchTool(tool *llmprovider.Tool) (Tool, error) {
	// Validate tool name
	if tool.Function.Name != "search" {
		p.logger.Error("expected search tool, got different name", "tool_name", tool.Function.Name)
		return Tool{}, fmt.Errorf("expected search tool, got %s", tool.Function.Name)
	}

	// Convert to standard function tool
	// OpenRouter's web plugin is model-dependent and not part of the standard API
	// So we treat search as a client-side tool for now
	return convertCustomTool(tool)
}

// convertCustomTool converts a custom function tool to OpenRouter format.
// OpenRouter uses OpenAI format, so this is a direct mapping.
// ToolInputSchema marshals to the correct JSON structure automatically.
func convertCustomTool(tool *llmprovider.Tool) (Tool, error) {
	// Build function definition
	// ToolInputSchema implements MarshalJSON for proper JSON output
	funcDef := FunctionDefinition{
		Name:       tool.Function.Name,
		Parameters: tool.Function.Parameters,
	}

	// Add description if present
	if tool.Function.Description != "" {
		funcDef.Description = &tool.Function.Description
	}

	return Tool{
		Type:     "function",
		Function: funcDef,
	}, nil
}
