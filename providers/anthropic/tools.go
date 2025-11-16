package anthropic

import (
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"

	"github.com/haowjy/meridian-llm-go"
)

// convertToolsToAnthropicTools converts library Tool format to Anthropic SDK format.
// This function knows the Anthropic API format and hardcodes the mappings.
func convertToolsToAnthropicTools(tools []llmprovider.Tool) ([]anthropic.ToolUnionParam, error) {
	if len(tools) == 0 {
		return nil, nil
	}

	result := make([]anthropic.ToolUnionParam, 0, len(tools))

	for i, tool := range tools {
		var anthropicTool anthropic.ToolUnionParam
		var err error

		// Route based on function name (OpenAI format uses tool.Function.Name)
		switch tool.Function.Name {
		case "search":
			anthropicTool, err = convertSearchTool(&tool)

		case "text_editor":
			anthropicTool, err = convertTextEditorTool(&tool)

		case "bash":
			anthropicTool, err = convertBashTool(&tool)

		default:
			// All other tools are custom function tools
			anthropicTool, err = convertCustomTool(&tool)
		}

		if err != nil {
			return nil, fmt.Errorf("tool %d (%s): %w", i, tool.Function.Name, err)
		}

		result = append(result, anthropicTool)
	}

	return result, nil
}

// convertSearchTool converts search tool to Anthropic web_search format.
// Anthropic search is server-side executed.
func convertSearchTool(tool *llmprovider.Tool) (anthropic.ToolUnionParam, error) {
	// Validate tool name (OpenAI format uses tool.Function.Name)
	if tool.Function.Name != "search" {
		return anthropic.ToolUnionParam{}, fmt.Errorf("expected search tool, got %s", tool.Function.Name)
	}

	// Anthropic web search format
	// https://docs.anthropic.com/en/docs/build-with-claude/web-search
	return anthropic.ToolUnionParam{
		OfWebSearchTool20250305: &anthropic.WebSearchTool20250305Param{
			// Name and Type have default values and will auto-marshal
			// Future: Add MaxUses, AllowedDomains, BlockedDomains, UserLocation when supported
		},
	}, nil
}

// convertTextEditorTool converts text_editor tool to Anthropic text editor format.
// Anthropic text editor is client-side executed.
func convertTextEditorTool(tool *llmprovider.Tool) (anthropic.ToolUnionParam, error) {
	// Validate tool name (OpenAI format uses tool.Function.Name)
	if tool.Function.Name != "text_editor" {
		return anthropic.ToolUnionParam{}, fmt.Errorf("expected text_editor tool, got %s", tool.Function.Name)
	}

	// Anthropic text editor format
	// https://docs.anthropic.com/en/docs/build-with-claude/edit-code
	return anthropic.ToolUnionParam{
		OfTextEditor20250728: &anthropic.ToolTextEditor20250728Param{
			// Name and Type have default values and will auto-marshal
			// Future: Add MaxCharacters when supported
		},
	}, nil
}

// convertBashTool converts bash/code execution tool to Anthropic bash format.
// Anthropic bash is client-side executed.
func convertBashTool(tool *llmprovider.Tool) (anthropic.ToolUnionParam, error) {
	// Validate tool name (OpenAI format uses tool.Function.Name)
	if tool.Function.Name != "bash" {
		return anthropic.ToolUnionParam{}, fmt.Errorf("expected bash tool, got %s", tool.Function.Name)
	}

	// Anthropic bash format
	// https://docs.anthropic.com/en/docs/build-with-claude/computer-use
	return anthropic.ToolUnionParam{
		OfBashTool20250124: &anthropic.ToolBash20250124Param{
			// Name and Type have default values and will auto-marshal
		},
	}, nil
}

// convertCustomTool converts custom function tool to Anthropic custom tool format.
// Custom tools are client-side executed.
// Converts OpenAI format (tool.Function.Parameters) → Anthropic format (input_schema).
func convertCustomTool(tool *llmprovider.Tool) (anthropic.ToolUnionParam, error) {
	// OpenAI format: tool.Function.Parameters contains full JSON schema
	// Example: {"type": "object", "properties": {...}, "required": [...]}
	//
	// Anthropic format needs:
	// - Type: "object"
	// - Properties: just the properties object (not full schema)
	// - ExtraFields: other schema fields like "required"

	// Extract fields from OpenAI schema
	properties, _ := tool.Function.Parameters["properties"]

	// Build Anthropic schema with correct structure
	// Type can be elided (zero value) - it will marshal as "object"
	schema := anthropic.ToolInputSchemaParam{
		Properties:  properties,
		ExtraFields: make(map[string]any),
	}

	// Extract required field if present (it's a direct field in v1.17.0)
	if required, ok := tool.Function.Parameters["required"].([]interface{}); ok {
		schema.Required = make([]string, len(required))
		for i, v := range required {
			if str, ok := v.(string); ok {
				schema.Required[i] = str
			}
		}
	}

	// Copy other fields (additionalProperties, etc.) to ExtraFields
	for key, value := range tool.Function.Parameters {
		if key != "type" && key != "properties" && key != "required" {
			schema.ExtraFields[key] = value
		}
	}

	// Anthropic custom tool format (standard function calling)
	// Use ToolUnionParamOfTool helper with function name
	toolParam := anthropic.ToolUnionParamOfTool(schema, tool.Function.Name)

	// Add description if provided
	if tool.Function.Description != "" {
		// Need to set description through the OfTool field
		if toolParam.OfTool == nil {
			toolParam.OfTool = &anthropic.ToolParam{}
		}
		toolParam.OfTool.Description = anthropic.String(tool.Function.Description)
	}

	return toolParam, nil
}

// convertToolChoice converts library ToolChoice to Anthropic format.
// Returns nil if no tool choice specified (lets provider decide).
func convertToolChoice(choice *llmprovider.ToolChoice) (*anthropic.ToolChoiceUnionParam, error) {
	if choice == nil {
		return nil, nil
	}

	// Validate tool choice
	if err := choice.Validate(); err != nil {
		return nil, fmt.Errorf("invalid tool choice: %w", err)
	}

	// Map tool choice modes to Anthropic format
	switch choice.Mode {
	case llmprovider.ToolChoiceModeAuto:
		// Auto mode: model decides whether to use tools
		return &anthropic.ToolChoiceUnionParam{
			OfAuto: &anthropic.ToolChoiceAutoParam{},
		}, nil

	case llmprovider.ToolChoiceModeRequired:
		// Required mode: model must use a tool (Anthropic calls this "any")
		return &anthropic.ToolChoiceUnionParam{
			OfAny: &anthropic.ToolChoiceAnyParam{},
		}, nil

	case llmprovider.ToolChoiceModeNone:
		// None mode: don't use tools
		noneParam := anthropic.NewToolChoiceNoneParam()
		return &anthropic.ToolChoiceUnionParam{
			OfNone: &noneParam,
		}, nil

	case llmprovider.ToolChoiceModeSpecific:
		// Specific mode: must use specific tool
		if choice.ToolName == nil || *choice.ToolName == "" {
			return nil, fmt.Errorf("tool_name required for specific mode")
		}

		// Use ToolChoiceParamOfTool helper which returns ToolChoiceUnionParam
		unionParam := anthropic.ToolChoiceParamOfTool(*choice.ToolName)
		return &unionParam, nil

	default:
		return nil, fmt.Errorf("unsupported tool choice mode: %s", choice.Mode)
	}
}
