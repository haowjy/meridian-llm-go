package anthropic

import (
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
		budgetTokens := params.GetThinkingBudgetTokens()
		if budgetTokens > 0 {
			apiParams.Thinking = anthropic.ThinkingConfigParamOfEnabled(int64(budgetTokens))
		}
	}

	return apiParams, nil
}
