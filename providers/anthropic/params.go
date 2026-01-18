package anthropic

import (
	"encoding/json"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"

	"github.com/haowjy/meridian-llm-go"
)

// buildMessageParams constructs Anthropic API parameters from a GenerateRequest.
// This function is shared between GenerateResponse and StreamResponse to avoid duplication.
func (p *Provider) buildMessageParams(req *llmprovider.GenerateRequest) (anthropic.MessageNewParams, error) {
	// Convert library messages to Anthropic format
	messages, err := p.convertToAnthropicMessages(req.Messages)
	if err != nil {
		p.logger.Error("failed to convert messages", "error", err)
		return anthropic.MessageNewParams{}, fmt.Errorf("failed to convert messages: %w", err)
	}

	// Extract params or use defaults
	params := req.Params
	if params == nil {
		params = &llmprovider.RequestParams{}
	}

	// Determine default max_tokens based on whether thinking is enabled.
	// Thinking models need higher max_tokens to accommodate both thinking and response.
	defaultMaxTokens := 4096
	if params.ThinkingEnabled != nil && *params.ThinkingEnabled {
		defaultMaxTokens = 16384 // Thinking models need more headroom
	}

	maxTokens := int64(params.GetMaxTokens(defaultMaxTokens))

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

	// Thinking mode - calculate budget as ratio of max_tokens.
	// This ensures the Anthropic constraint (max_tokens > budget_tokens) is satisfied by design.
	if params.ThinkingEnabled != nil && *params.ThinkingEnabled {
		budgetTokens, err := params.GetThinkingBudgetTokens(int(maxTokens))
		if err != nil {
			p.logger.Error("failed to get thinking budget", "error", err)
			return anthropic.MessageNewParams{}, fmt.Errorf("failed to get thinking budget: %w", err)
		}
		if budgetTokens > 0 {
			apiParams.Thinking = anthropic.ThinkingConfigParamOfEnabled(int64(budgetTokens))
		}
	}

	// Tools - convert tools to Anthropic format
	if len(params.Tools) > 0 {
		anthropicTools, err := p.convertToolsToAnthropicTools(params.Tools)
		if err != nil {
			p.logger.Error("failed to convert tools", "error", err)
			return anthropic.MessageNewParams{}, fmt.Errorf("failed to convert tools: %w", err)
		}
		apiParams.Tools = anthropicTools
	}

	// Tool choice - convert to Anthropic format
	if params.ToolChoice != nil {
		// Tool choice must be a *ToolChoice
		toolChoice, ok := params.ToolChoice.(*llmprovider.ToolChoice)
		if !ok {
			p.logger.Error("tool_choice must be *llmprovider.ToolChoice")
			return anthropic.MessageNewParams{}, fmt.Errorf("tool_choice must be *llmprovider.ToolChoice")
		}

		anthropicToolChoice, err := p.convertToolChoice(toolChoice)
		if err != nil {
			p.logger.Error("failed to convert tool choice", "error", err)
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
func (p *Provider) BuildMessageParamsDebug(req *llmprovider.GenerateRequest) (map[string]interface{}, error) {
	apiParams, err := p.buildMessageParams(req)
	if err != nil {
		return nil, err
	}

	// Marshal to JSON using the SDK's types, then back into a map
	jsonBytes, err := json.Marshal(apiParams)
	if err != nil {
		p.logger.Debug("failed to marshal anthropic params", "error", err)
		return nil, fmt.Errorf("failed to marshal anthropic params: %w", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(jsonBytes, &result); err != nil {
		p.logger.Debug("failed to unmarshal anthropic params", "error", err)
		return nil, fmt.Errorf("failed to unmarshal anthropic params: %w", err)
	}

	return result, nil
}
