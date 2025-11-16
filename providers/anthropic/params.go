package anthropic

import (
	"encoding/json"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"

	"github.com/haowjy/meridian-llm-go"
)

// buildMessageParams constructs Anthropic API parameters from a GenerateRequest.
// This function is shared between GenerateResponse and StreamResponse to avoid duplication.
func buildMessageParams(req *llmprovider.GenerateRequest) (anthropic.MessageNewParams, error) {
	// Convert library messages to Anthropic format
	messages, err := convertToAnthropicMessages(req.Messages)
	if err != nil {
		return anthropic.MessageNewParams{}, fmt.Errorf("failed to convert messages: %w", err)
	}

	// Extract params or use defaults
	params := req.Params
	if params == nil {
		params = &llmprovider.RequestParams{}
	}

	// Build request parameters with defaults
	maxTokens := int64(params.GetMaxTokens(4096))

	apiParams := anthropic.MessageNewParams{
		Model:     anthropic.Model(req.Model),
		Messages:  messages,
		MaxTokens: maxTokens,
	}

	// Temperature
	if params.Temperature != nil {
		apiParams.Temperature = anthropic.Float(*params.Temperature)
	}

	// Top-P
	if params.TopP != nil {
		apiParams.TopP = anthropic.Float(*params.TopP)
	}

	// Top-K
	if params.TopK != nil {
		apiParams.TopK = anthropic.Int(int64(*params.TopK))
	}

	// Stop sequences
	if len(params.Stop) > 0 {
		apiParams.StopSequences = params.Stop
	}

	// System prompt
	if params.System != nil {
		apiParams.System = []anthropic.TextBlockParam{
			{
				Type: "text",
				Text: *params.System,
			},
		}
	}

	// Thinking mode - convert user-friendly level to token budget
	if params.ThinkingEnabled != nil && *params.ThinkingEnabled {
		budgetTokens, err := params.GetThinkingBudgetTokens("anthropic", req.Model)
		if err != nil {
			return anthropic.MessageNewParams{}, fmt.Errorf("failed to get thinking budget: %w", err)
		}
		if budgetTokens > 0 {
			apiParams.Thinking = anthropic.ThinkingConfigParamOfEnabled(int64(budgetTokens))
		}
	}

	// Tools - convert tools to Anthropic format
	if len(params.Tools) > 0 {
		anthropicTools, err := convertToolsToAnthropicTools(params.Tools)
		if err != nil {
			return anthropic.MessageNewParams{}, fmt.Errorf("failed to convert tools: %w", err)
		}
		apiParams.Tools = anthropicTools
	}

	// Tool choice - convert to Anthropic format
	if params.ToolChoice != nil {
		// Tool choice must be a *ToolChoice
		toolChoice, ok := params.ToolChoice.(*llmprovider.ToolChoice)
		if !ok {
			return anthropic.MessageNewParams{}, fmt.Errorf("tool_choice must be *llmprovider.ToolChoice")
		}

		anthropicToolChoice, err := convertToolChoice(toolChoice)
		if err != nil {
			return anthropic.MessageNewParams{}, fmt.Errorf("failed to convert tool choice: %w", err)
		}

		// Only set if not nil (nil means auto mode)
		if anthropicToolChoice != nil {
			apiParams.ToolChoice = *anthropicToolChoice
		}
	}

	return apiParams, nil
}

// BuildMessageParamsDebug builds the Anthropic MessageNewParams for a GenerateRequest
// and returns it as a generic JSON map for debugging/inspection. This does not perform
// any network calls and is safe to use in debug-only tooling.
func BuildMessageParamsDebug(req *llmprovider.GenerateRequest) (map[string]interface{}, error) {
	apiParams, err := buildMessageParams(req)
	if err != nil {
		return nil, err
	}

	// Marshal to JSON using the SDK's types, then back into a map
	jsonBytes, err := json.Marshal(apiParams)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal anthropic params: %w", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(jsonBytes, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal anthropic params: %w", err)
	}

	return result, nil
}
