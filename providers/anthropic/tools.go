package anthropic

import (
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"

	"github.com/haowjy/meridian-llm-go"
)

// convertToolsToAnthropicTools converts library Tool format to Anthropic SDK format.
// This function knows the Anthropic API format and hardcodes the mappings.
func (p *Provider) convertToolsToAnthropicTools(tools []llmprovider.Tool) ([]anthropic.ToolUnionParam, error) {
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
			anthropicTool, err = p.convertSearchTool(&tool)

		case "text_editor":
			anthropicTool, err = p.convertTextEditorTool(&tool)

		case "bash":
			anthropicTool, err = p.convertBashTool(&tool)

		default:
			// All other tools are custom function tools
			anthropicTool, err = p.convertCustomTool(&tool)
		}

		if err != nil {
			p.logger.Error("failed to convert tool", "tool_index", i, "tool_name", tool.Function.Name, "error", err)
			return nil, fmt.Errorf("tool %d (%s): %w", i, tool.Function.Name, err)
		}

		result = append(result, anthropicTool)
	}

	return result, nil
}

// convertSearchTool converts search tool to Anthropic web_search format.
// Anthropic search is provider-executed.
func (p *Provider) convertSearchTool(tool *llmprovider.Tool) (anthropic.ToolUnionParam, error) {
	// Validate tool name (OpenAI format uses tool.Function.Name)
	if tool.Function.Name != "search" {
		p.logger.Error("unexpected tool name for search tool", "tool_name", tool.Function.Name)
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
func (p *Provider) convertTextEditorTool(tool *llmprovider.Tool) (anthropic.ToolUnionParam, error) {
	// Validate tool name (OpenAI format uses tool.Function.Name)
	if tool.Function.Name != "text_editor" {
		p.logger.Error("unexpected tool name for text_editor tool", "tool_name", tool.Function.Name)
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
func (p *Provider) convertBashTool(tool *llmprovider.Tool) (anthropic.ToolUnionParam, error) {
	// Validate tool name (OpenAI format uses tool.Function.Name)
	if tool.Function.Name != "bash" {
		p.logger.Error("unexpected tool name for bash tool", "tool_name", tool.Function.Name)
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
// Converts typed ToolInputSchema → Anthropic format (input_schema).
func (p *Provider) convertCustomTool(tool *llmprovider.Tool) (anthropic.ToolUnionParam, error) {
	// With typed ToolInputSchema, we have direct access to all fields.
	// No more type assertions needed - the schema is already properly typed.
	params := tool.Function.Parameters

	// Build Anthropic schema with correct structure.
	// Properties field uses OrderedProperties which implements MarshalJSON for ordering.
	schema := anthropic.ToolInputSchemaParam{
		Properties: params.Properties, // OrderedProperties marshals with correct order
		Required:   params.Required,   // Already []string - no type assertion needed
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
func (p *Provider) convertToolChoice(choice *llmprovider.ToolChoice) (*anthropic.ToolChoiceUnionParam, error) {
	if choice == nil {
		return nil, nil
	}

	// Validate tool choice
	if err := choice.Validate(); err != nil {
		p.logger.Error("invalid tool choice", "error", err)
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
			p.logger.Error("tool_name required for specific mode")
			return nil, fmt.Errorf("tool_name required for specific mode")
		}

		// Use ToolChoiceParamOfTool helper which returns ToolChoiceUnionParam
		unionParam := anthropic.ToolChoiceParamOfTool(*choice.ToolName)
		return &unionParam, nil

	default:
		p.logger.Error("unsupported tool choice mode", "mode", choice.Mode)
		return nil, fmt.Errorf("unsupported tool choice mode: %s", choice.Mode)
	}
}
