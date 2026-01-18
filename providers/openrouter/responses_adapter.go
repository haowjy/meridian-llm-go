package openrouter

import (
	"encoding/json"
	"fmt"
	"strings"

	llmprovider "github.com/haowjy/meridian-llm-go"
)

// ===== Responses API Request Conversion =====

// convertToResponsesInput converts library messages to Responses API input format.
// Messages become {type: "message", role, content: [{type: "input_text", text}]}
// Tool results become {type: "function_call_output", call_id, output}
func (p *Provider) convertToResponsesInput(messages []llmprovider.Message) ([]ResponsesInput, error) {
	// Phase 1: Handle cross-provider server tools by splitting messages
	processedMessages, err := llmprovider.SplitMessagesAtCrossProviderTool(messages, llmprovider.ProviderOpenRouter)
	if err != nil {
		p.logger.Error("failed to process cross-provider tools for responses API", "error", err)
		return nil, fmt.Errorf("failed to process cross-provider tools: %w", err)
	}

	// Phase 2: Split assistant messages at tool_result boundaries
	splitMessages := splitMessagesAtToolResults(processedMessages)

	// Phase 3: Merge consecutive same-role messages
	mergedMessages := mergeConsecutiveSameRoleMessages(splitMessages)

	result := make([]ResponsesInput, 0, len(mergedMessages)*2)

	for i, msg := range mergedMessages {
		inputs, err := p.convertMessageToResponsesInput(msg, i)
		if err != nil {
			return nil, err
		}
		result = append(result, inputs...)
	}

	return result, nil
}

// convertMessageToResponsesInput converts a single library message to Responses API input(s).
// May return multiple inputs (e.g., message + function_call_output separately).
func (p *Provider) convertMessageToResponsesInput(msg llmprovider.Message, msgIndex int) ([]ResponsesInput, error) {
	var result []ResponsesInput

	// Separate blocks by type
	var textBlocks []*llmprovider.Block
	var thinkingBlocks []*llmprovider.Block
	var toolUseBlocks []*llmprovider.Block
	var toolResultBlocks []*llmprovider.Block

	for _, block := range msg.Blocks {
		switch block.BlockType {
		case llmprovider.BlockTypeText:
			textBlocks = append(textBlocks, block)
		case llmprovider.BlockTypeThinking:
			thinkingBlocks = append(thinkingBlocks, block)
		case llmprovider.BlockTypeToolUse:
			toolUseBlocks = append(toolUseBlocks, block)
		case llmprovider.BlockTypeToolResult:
			toolResultBlocks = append(toolResultBlocks, block)
		}
	}

	// Handle tool_result blocks as function_call_output inputs
	for j, block := range toolResultBlocks {
		callID, ok := block.GetToolUseID()
		if !ok || callID == "" {
			p.logger.Error("tool_result block missing tool_use_id for responses API", "message_index", msgIndex, "block_index", j)
			return nil, fmt.Errorf("message %d, block %d: tool_result block missing tool_use_id", msgIndex, j)
		}

		// Extract output content
		output := extractToolResultOutput(block)

		result = append(result, ResponsesInput{
			Type:   "function_call_output",
			CallID: callID,
			Output: output,
		})
	}

	// Handle user/assistant/system messages
	if msg.Role == "user" || msg.Role == "assistant" || msg.Role == "system" {
		var content []ResponsesContent

		// Add text content
		for _, block := range textBlocks {
			if block.TextContent != nil && *block.TextContent != "" {
				content = append(content, ResponsesContent{
					Type: "input_text",
					Text: *block.TextContent,
				})
			}
		}

		// Add thinking content (flattened to text for input replay)
		// Note: For responses API, we include thinking as text in the input
		for _, block := range thinkingBlocks {
			if block.TextContent != nil && *block.TextContent != "" {
				content = append(content, ResponsesContent{
					Type: "input_text",
					Text: *block.TextContent,
				})
			}
		}

		// Only add message if it has content
		if len(content) > 0 {
			input := ResponsesInput{
				Type:    "message",
				Role:    msg.Role,
				Content: content,
			}
			result = append(result, input)
		}

		// Tool use blocks in assistant messages need to be handled specially
		// In Responses API, previous tool calls are typically represented via previous_response_id
		// For explicit replay, we would need to include them differently
		// For now, tool_use blocks are not directly convertible to ResponsesInput
		// They're handled via the conversation history pattern
		_ = toolUseBlocks // Acknowledge but don't process directly
	}

	return result, nil
}

// extractToolResultOutput extracts the output string from a tool_result block.
// Priority: TextContent > Content["content"] > Content["result"] > Content["error"]
func extractToolResultOutput(block *llmprovider.Block) string {
	if block.TextContent != nil {
		return *block.TextContent
	}
	if contentStr, ok := block.Content["content"].(string); ok {
		return contentStr
	}
	if resultStr, ok := block.Content["result"].(string); ok {
		isError := false
		if errFlag, ok := block.Content["is_error"].(bool); ok {
			isError = errFlag
		}
		if !isError {
			return resultStr
		}
	}
	if errMsg, ok := block.Content["error"].(string); ok {
		return errMsg
	}
	return ""
}

// ===== Responses API Tool Conversion =====

// convertToResponsesTools converts library tools to Responses API FLAT format.
// Output: {type: "function", name, description, parameters}
func (p *Provider) convertToResponsesTools(tools []llmprovider.Tool) ([]ResponsesTool, error) {
	if len(tools) == 0 {
		return nil, nil
	}

	result := make([]ResponsesTool, 0, len(tools))

	for i, tool := range tools {
		responsesTool, err := p.convertToolToResponsesFormat(tool, i)
		if err != nil {
			return nil, err
		}
		result = append(result, responsesTool)
	}

	return result, nil
}

// convertToolToResponsesFormat converts a single library tool to Responses API format.
func (p *Provider) convertToolToResponsesFormat(tool llmprovider.Tool, index int) (ResponsesTool, error) {
	// Extract parameters as map[string]interface{}
	params, err := extractToolParameters(tool)
	if err != nil {
		p.logger.Error("failed to extract tool parameters for responses API", "tool_index", index, "tool_name", tool.Function.Name, "error", err)
		return ResponsesTool{}, fmt.Errorf("tool %d (%s): %w", index, tool.Function.Name, err)
	}

	return ResponsesTool{
		Type:        "function",
		Name:        tool.Function.Name,
		Description: tool.Function.Description,
		Parameters:  params,
	}, nil
}

// extractToolParameters extracts parameters from a library tool as map[string]interface{}.
// ToolInputSchema is a struct type, so we use JSON roundtrip for clean conversion.
func extractToolParameters(tool llmprovider.Tool) (map[string]interface{}, error) {
	// ToolInputSchema is a struct (not pointer/interface), convert via JSON roundtrip
	// This handles OrderedProperties properly via its MarshalJSON method
	jsonBytes, err := json.Marshal(tool.Function.Parameters)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal parameters: %w", err)
	}

	var params map[string]interface{}
	if err := json.Unmarshal(jsonBytes, &params); err != nil {
		return nil, fmt.Errorf("failed to unmarshal parameters: %w", err)
	}

	// Ensure we have a valid object schema
	if params == nil {
		params = map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{},
		}
	}

	return params, nil
}

// ===== Responses API Request Building =====

// buildResponsesRequest constructs an OpenRouter Responses API request from a GenerateRequest.
func (p *Provider) buildResponsesRequest(req *llmprovider.GenerateRequest) (*ResponsesRequest, error) {
	// Convert library messages to Responses API input format
	inputs, err := p.convertToResponsesInput(req.Messages)
	if err != nil {
		p.logger.Error("failed to convert messages for responses API", "model", req.Model, "error", err)
		return nil, fmt.Errorf("failed to convert messages: %w", err)
	}

	params := req.Params
	if params == nil {
		params = &llmprovider.RequestParams{}
	}

	responsesReq := &ResponsesRequest{
		Model:  req.Model,
		Input:  inputs,
		Stream: false, // Caller sets this
	}

	// MaxOutputTokens (Responses API uses max_output_tokens instead of max_tokens)
	if params.MaxTokens != nil {
		responsesReq.MaxOutputTokens = params.MaxTokens
	}

	// Temperature
	if params.Temperature != nil {
		responsesReq.Temperature = params.Temperature
	}

	// Top-P
	if params.TopP != nil {
		responsesReq.TopP = params.TopP
	}

	// Tools
	if len(params.Tools) > 0 {
		tools, err := p.convertToResponsesTools(params.Tools)
		if err != nil {
			p.logger.Error("failed to convert tools for responses API", "model", req.Model, "error", err)
			return nil, fmt.Errorf("failed to convert tools: %w", err)
		}
		responsesReq.Tools = tools
	}

	// Tool choice
	if params.ToolChoice != nil {
		toolChoice, err := p.convertToolChoice(params.ToolChoice)
		if err != nil {
			p.logger.Error("failed to convert tool choice for responses API", "model", req.Model, "error", err)
			return nil, fmt.Errorf("failed to convert tool choice: %w", err)
		}
		responsesReq.ToolChoice = toolChoice
	}

	// Reasoning/Thinking
	if params.ThinkingEnabled != nil {
		if *params.ThinkingEnabled && params.ThinkingLevel != nil {
			responsesReq.Reasoning = &ReasoningConfig{
				Effort: *params.ThinkingLevel,
			}
		} else if !*params.ThinkingEnabled {
			responsesReq.Reasoning = &ReasoningConfig{
				Effort: "none",
			}
		}
	}

	return responsesReq, nil
}

// ===== Responses API Response Conversion =====

// convertFromResponsesResponse converts a Responses API response to library format.
// Used for non-streaming responses.
func (p *Provider) convertFromResponsesResponse(resp *ResponseObject) (*llmprovider.GenerateResponse, error) {
	if resp == nil {
		return nil, fmt.Errorf("nil response object")
	}

	blocks := make([]*llmprovider.Block, 0)
	providerIDStr := llmprovider.ProviderOpenRouter.String()
	sequence := 0

	// Process output items in order
	for _, item := range resp.Output {
		switch item.Type {
		case "message":
			// Extract text from message content
			var textParts []string
			for _, content := range item.Content {
				if content.Type == "output_text" && content.Text != "" {
					textParts = append(textParts, content.Text)
				}
			}
			if len(textParts) > 0 {
				text := strings.Join(textParts, "")
				blocks = append(blocks, &llmprovider.Block{
					BlockType:   llmprovider.BlockTypeText,
					Sequence:    sequence,
					TextContent: &text,
					Provider:    &providerIDStr,
				})
				sequence++
			}

		case "function_call":
			// Parse arguments JSON
			input := make(map[string]interface{})
			if item.Arguments != "" {
				if err := json.Unmarshal([]byte(item.Arguments), &input); err != nil {
					p.logger.Warn("failed to parse function call arguments in responses API", "item_id", item.ID, "error", err)
					// Continue with empty input rather than failing
				}
			}

			content := map[string]interface{}{
				"tool_use_id": item.CallID,
				"tool_name":   item.Name,
				"input":       input,
			}

			executionSide := llmprovider.ExecutionSideServer
			blocks = append(blocks, &llmprovider.Block{
				BlockType:     llmprovider.BlockTypeToolUse,
				Sequence:      sequence,
				Content:       content,
				ExecutionSide: &executionSide,
				Provider:      &providerIDStr,
			})
			sequence++

		case "reasoning":
			// Reasoning/thinking block
			var thinkingText string
			for _, content := range item.Content {
				if content.Text != "" {
					thinkingText += content.Text
				}
			}
			if thinkingText != "" {
				blocks = append(blocks, &llmprovider.Block{
					BlockType:   llmprovider.BlockTypeThinking,
					Sequence:    sequence,
					TextContent: &thinkingText,
					Provider:    &providerIDStr,
				})
				sequence++
			}
		}
	}

	// Map status to stop reason
	stopReason := mapResponseStatusToStopReason(resp.Status, resp.Output)

	// Extract usage
	var inputTokens, outputTokens int
	if resp.Usage != nil {
		inputTokens = resp.Usage.InputTokens
		outputTokens = resp.Usage.OutputTokens
	}

	// Build response metadata
	responseMetadata := make(map[string]interface{})
	responseMetadata["response_id"] = resp.ID
	if resp.Usage != nil {
		responseMetadata["total_tokens"] = resp.Usage.TotalTokens
	}

	return &llmprovider.GenerateResponse{
		Blocks:           blocks,
		Model:            resp.Model,
		InputTokens:      inputTokens,
		OutputTokens:     outputTokens,
		StopReason:       stopReason,
		ResponseMetadata: responseMetadata,
	}, nil
}

// mapResponseStatusToStopReason maps Responses API status to library stop reason.
func mapResponseStatusToStopReason(status string, output []OutputItem) string {
	switch status {
	case "completed":
		// Check if there are function calls - that indicates tool_use stop reason
		for _, item := range output {
			if item.Type == "function_call" {
				return "tool_use"
			}
		}
		return "end_turn"
	case "failed":
		return "error"
	case "cancelled":
		return "cancelled"
	default:
		return status
	}
}
