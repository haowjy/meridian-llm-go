package openrouter

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/haowjy/meridian-llm-go"
)

// ===== Data Structures for SOLID-compliant block parsing =====

// ParsedDelta represents structured information extracted from a delta/message.
// Separates parsing from emission/building (Single Responsibility Principle).
type ParsedDelta struct {
	WebSearch *WebSearchInfo // nil if no web search in this delta
	Thinking  *ThinkingInfo  // nil if no thinking in this delta
	Text      *TextInfo      // nil if no text in this delta
}

// WebSearchInfo contains web search data extracted from annotations.
type WebSearchInfo struct {
	Annotations []Annotation
}

// ThinkingInfo contains thinking/reasoning text extracted from reasoning_details.
type ThinkingInfo struct {
	Text string // Combined text from all reasoning_details
}

// TextInfo contains text content extracted from content field.
type TextInfo struct {
	Text string
}

// BlockState tracks the current block emission state during streaming/parsing.
type BlockState struct {
	CurrentType   string // "thinking", "text", "" (empty = no block started)
	CurrentIndex  int    // Current block sequence number
	WebSearchDone bool   // Have we emitted web search blocks?
}

// BlockTransition describes state changes when processing a delta.
type BlockTransition struct {
	ClosePrevious bool   // Should we close the previous block?
	StartNew      bool   // Should we start a new block?
	NewType       string // "thinking", "text" (if StartNew=true)
	NewIndex      int    // Updated block index
}

// ===== End of data structures =====

// ===== Pure Parsing Functions (Single Responsibility) =====

// extractWebSearchInfo extracts web search information from annotations.
// Returns nil if no annotations present.
func extractWebSearchInfo(annotations []Annotation) *WebSearchInfo {
	if len(annotations) == 0 {
		return nil
	}
	return &WebSearchInfo{Annotations: annotations}
}

// extractThinkingInfo extracts thinking text from reasoning_details array.
// Returns nil if no reasoning details present or all are empty.
func extractThinkingInfo(details []ReasoningDetail) *ThinkingInfo {
	if len(details) == 0 {
		return nil
	}

	var text strings.Builder
	for _, detail := range details {
		// Extract text based on detail type
		switch detail.Type {
		case "reasoning.text":
			if detail.Text != nil && *detail.Text != "" {
				text.WriteString(*detail.Text)
			}
		case "reasoning.summary":
			if detail.Summary != nil && *detail.Summary != "" {
				text.WriteString(*detail.Summary)
			}
		// Skip "reasoning.encrypted" - we can't use encrypted data
		}
	}

	if text.Len() == 0 {
		return nil
	}

	result := text.String()
	return &ThinkingInfo{Text: result}
}

// extractTextInfo extracts text content from content field.
// Returns nil if content is nil or empty.
func extractTextInfo(content *string) *TextInfo {
	if content == nil || *content == "" {
		return nil
	}
	return &TextInfo{Text: *content}
}

// parseDelta parses annotations, reasoning_details, and content into structured info.
// This function only extracts data - it doesn't emit blocks or manage state.
func parseDelta(
	annotations []Annotation,
	reasoningDetails []ReasoningDetail,
	content *string,
) *ParsedDelta {
	return &ParsedDelta{
		WebSearch: extractWebSearchInfo(annotations),
		Thinking:  extractThinkingInfo(reasoningDetails),
		Text:      extractTextInfo(content),
	}
}

// ===== End of pure parsing functions =====

// ===== State Transition Logic =====

// determineTransition determines block transitions based on current state and parsed delta.
// This function only decides what to do - it doesn't emit blocks or build them.
func determineTransition(state BlockState, parsed *ParsedDelta) BlockTransition {
	transition := BlockTransition{
		NewIndex: state.CurrentIndex,
	}

	// Thinking → Text transition
	// (had reasoning before, now have text without reasoning)
	if state.CurrentType == "thinking" && parsed.Text != nil && parsed.Thinking == nil {
		transition.ClosePrevious = true
		transition.StartNew = true
		transition.NewType = "text"
		transition.NewIndex = state.CurrentIndex + 1
		return transition
	}

	// Start thinking block
	if parsed.Thinking != nil && state.CurrentType != "thinking" {
		transition.StartNew = true
		transition.NewType = "thinking"
		return transition
	}

	// Start text block (no thinking before)
	if parsed.Text != nil && state.CurrentType != "text" {
		transition.StartNew = true
		transition.NewType = "text"
		return transition
	}

	// Continue current block (no transition)
	return transition
}

// ===== End of state transition logic =====

// ===== Non-Streaming Block Builder =====

// buildNonStreamingBlocks builds complete blocks from parsed delta data.
// This function only builds blocks - it doesn't parse or manage state transitions.
func buildNonStreamingBlocks(parsed *ParsedDelta, state *BlockState) ([]*llmprovider.Block, error) {
	blocks := []*llmprovider.Block{}
	providerIDStr := llmprovider.ProviderOpenRouter.String()

	// 1. Web search blocks (if present and not done)
	if parsed.WebSearch != nil && !state.WebSearchDone {
		wsBlocks, err := convertAnnotationsToWebSearchBlocks(
			parsed.WebSearch.Annotations,
			state.CurrentIndex,
		)
		if err != nil {
			return nil, err
		}
		blocks = append(blocks, wsBlocks...)
		state.CurrentIndex += len(wsBlocks)
		state.WebSearchDone = true
	}

	// 2. Thinking block (if present)
	if parsed.Thinking != nil {
		blocks = append(blocks, &llmprovider.Block{
			BlockType:   llmprovider.BlockTypeThinking,
			Sequence:    state.CurrentIndex,
			TextContent: &parsed.Thinking.Text,
			Provider:    &providerIDStr,
		})
		state.CurrentIndex++
		state.CurrentType = "thinking"
	}

	// 3. Text block (if present)
	if parsed.Text != nil {
		blocks = append(blocks, &llmprovider.Block{
			BlockType:   llmprovider.BlockTypeText,
			Sequence:    state.CurrentIndex,
			TextContent: &parsed.Text.Text,
			Provider:    &providerIDStr,
		})
		state.CurrentIndex++
		state.CurrentType = "text"
	}

	return blocks, nil
}

// ===== End of non-streaming block builder =====

// convertToOpenRouterMessages converts library messages to OpenRouter/OpenAI format.
func convertToOpenRouterMessages(messages []llmprovider.Message) ([]Message, error) {
	// Phase 1: Handle cross-provider server tools by splitting messages
	// This converts server tools from other providers into synthetic conversation turns
	processedMessages, err := llmprovider.SplitMessagesAtCrossProviderTool(messages, llmprovider.ProviderOpenRouter)
	if err != nil {
		return nil, fmt.Errorf("failed to process cross-provider tools: %w", err)
	}

	result := make([]Message, 0, len(processedMessages))

	for i, msg := range processedMessages {
		// Convert blocks to OpenRouter format
		// OpenRouter uses OpenAI format: single content string + tool_calls array
		openrouterMsg, err := convertMessageToOpenRouter(msg, i)
		if err != nil {
			return nil, err
		}

		// System messages are handled in messages array (unlike Anthropic's separate system param)
		result = append(result, openrouterMsg...)
	}

	return result, nil
}

// convertMessageToOpenRouter converts a single library message to OpenRouter format.
// May return multiple messages (e.g., when splitting tool results).
func convertMessageToOpenRouter(msg llmprovider.Message, msgIndex int) ([]Message, error) {
	var result []Message

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
		// Skip web_search blocks - they're provider-specific and will be replayed from ProviderData if needed
		}
	}

	// Handle tool_result blocks separately (they become role:"tool" messages in OpenRouter)
	for j, block := range toolResultBlocks {
		toolUseID, ok := block.GetToolUseID()
		if !ok || toolUseID == "" {
			return nil, fmt.Errorf("message %d, block %d: tool_result block missing tool_use_id", msgIndex, j)
		}

		// Extract result content
		var resultContent string
		if block.TextContent != nil {
			resultContent = *block.TextContent
		} else if contentStr, ok := block.Content["content"].(string); ok {
			resultContent = contentStr
		}

		// Create tool message
		result = append(result, Message{
			Role:       "tool",
			Content:    resultContent,
			ToolCallID: &toolUseID,
		})
	}

	// Handle user/assistant messages
	if msg.Role == "user" || msg.Role == "assistant" {
		openrouterMsg := Message{
			Role: msg.Role,
		}

		// Flatten text and thinking blocks into single content string
		var contentParts []string
		for _, block := range textBlocks {
			if block.TextContent != nil {
				contentParts = append(contentParts, *block.TextContent)
			}
		}
		for _, block := range thinkingBlocks {
			if block.TextContent != nil {
				// Include thinking as text with marker
				// OpenRouter doesn't have separate thinking blocks
				contentParts = append(contentParts, fmt.Sprintf("[Thinking: %s]", *block.TextContent))
			}
		}

		// Set content if we have any
		if len(contentParts) > 0 {
			content := strings.Join(contentParts, "\n\n")
			openrouterMsg.Content = content
		}

		// Convert tool_use blocks to tool_calls array (assistant messages only)
		if msg.Role == "assistant" && len(toolUseBlocks) > 0 {
			toolCalls := make([]ToolCall, 0, len(toolUseBlocks))
			for j, block := range toolUseBlocks {
				toolCall, err := convertToolUseToToolCall(block, msgIndex, j)
				if err != nil {
					return nil, err
				}
				toolCalls = append(toolCalls, toolCall)
			}
			openrouterMsg.ToolCalls = toolCalls
		}

		// Only add message if it has content or tool calls
		if openrouterMsg.Content != nil || len(openrouterMsg.ToolCalls) > 0 {
			result = append(result, openrouterMsg)
		}
	}

	return result, nil
}

// convertToolUseToToolCall converts a tool_use block to OpenRouter ToolCall format.
func convertToolUseToToolCall(block *llmprovider.Block, msgIndex, blockIndex int) (ToolCall, error) {
	if block.Content == nil {
		return ToolCall{}, fmt.Errorf("message %d, block %d: tool_use block missing content", msgIndex, blockIndex)
	}

	toolUseID, ok := block.Content["tool_use_id"].(string)
	if !ok || toolUseID == "" {
		return ToolCall{}, fmt.Errorf("message %d, block %d: tool_use block missing tool_use_id", msgIndex, blockIndex)
	}

	toolName, ok := block.Content["tool_name"].(string)
	if !ok || toolName == "" {
		return ToolCall{}, fmt.Errorf("message %d, block %d: tool_use block missing tool_name", msgIndex, blockIndex)
	}

	input, ok := block.Content["input"]
	if !ok {
		return ToolCall{}, fmt.Errorf("message %d, block %d: tool_use block missing input", msgIndex, blockIndex)
	}

	// Marshal input to JSON string
	inputJSON, err := json.Marshal(input)
	if err != nil {
		return ToolCall{}, fmt.Errorf("message %d, block %d: failed to marshal tool input: %w", msgIndex, blockIndex, err)
	}

	return ToolCall{
		ID:   toolUseID,
		Type: "function",
		Function: FunctionCall{
			Name:      toolName,
			Arguments: string(inputJSON),
		},
	}, nil
}

// convertFromChatCompletionResponse converts OpenRouter response to library format.
func convertFromChatCompletionResponse(resp *ChatCompletionResponse) (*llmprovider.GenerateResponse, error) {
	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("no choices in response")
	}

	choice := resp.Choices[0]
	blocks := make([]*llmprovider.Block, 0)

	// Initialize block state
	state := BlockState{CurrentIndex: 0}

	// Convert Message.Content (interface{}) to *string for parsing
	var contentPtr *string
	if contentStr, ok := choice.Message.Content.(string); ok && contentStr != "" {
		contentPtr = &contentStr
	}

	// Parse message fields using SOLID-compliant functions
	parsed := parseDelta(
		choice.Message.Annotations,
		choice.Message.ReasoningDetails,
		contentPtr,
	)

	// Build blocks using non-streaming builder
	messageBlocks, err := buildNonStreamingBlocks(parsed, &state)
	if err != nil {
		return nil, err
	}
	blocks = append(blocks, messageBlocks...)

	// Add citations to text block if annotations present
	if parsed.WebSearch != nil {
		// Find text block and add citations
		for _, block := range blocks {
			if block.BlockType == llmprovider.BlockTypeText {
				block.Citations = convertAnnotationsToCitations(choice.Message.Annotations)
				break
			}
		}
	}

	// Convert tool_calls to tool_use blocks
	providerIDStr := llmprovider.ProviderOpenRouter.String()
	for _, toolCall := range choice.Message.ToolCalls {
		block, err := convertToolCallToBlock(toolCall, state.CurrentIndex)
		if err != nil {
			// Continue on error (don't fail entire response)
			continue
		}
		block.Provider = &providerIDStr
		blocks = append(blocks, block)
		state.CurrentIndex++
	}

	// Map finish_reason to library stop_reason
	stopReason := ""
	if choice.FinishReason != nil {
		stopReason = mapFinishReason(*choice.FinishReason)
	}

	// Build response metadata
	responseMetadata := make(map[string]interface{})
	responseMetadata["total_tokens"] = resp.Usage.TotalTokens
	responseMetadata["response_id"] = resp.ID

	return &llmprovider.GenerateResponse{
		Blocks:           blocks,
		Model:            resp.Model,
		InputTokens:      resp.Usage.PromptTokens,
		OutputTokens:     resp.Usage.CompletionTokens,
		StopReason:       stopReason,
		ResponseMetadata: responseMetadata,
	}, nil
}

// convertToolCallToBlock converts an OpenRouter ToolCall to a library Block.
func convertToolCallToBlock(toolCall ToolCall, sequence int) (*llmprovider.Block, error) {
	// Parse arguments JSON
	var input map[string]interface{}
	if err := json.Unmarshal([]byte(toolCall.Function.Arguments), &input); err != nil {
		return nil, fmt.Errorf("invalid tool call arguments: %w", err)
	}

	content := map[string]interface{}{
		"tool_use_id": toolCall.ID,
		"tool_name":   toolCall.Function.Name,
		"input":       input,
	}

	return &llmprovider.Block{
		BlockType: llmprovider.BlockTypeToolUse,
		Sequence:  sequence,
		Content:   content,
	}, nil
}

// convertAnnotationsToWebSearchBlocks creates synthetic web_search blocks from OpenRouter annotations.
// OpenRouter models with :online suffix automatically invoke web search and return results as annotations.
//
// TODO: OpenRouter's :online models auto-trigger web search which may be unexpected.
// Consider adding configuration to:
// - Warn users about auto-search behavior
// - Provide opt-out mechanism
// - Document which models have this behavior
func convertAnnotationsToWebSearchBlocks(annotations []Annotation, startSequence int) ([]*llmprovider.Block, error) {
	if len(annotations) == 0 {
		return nil, nil
	}

	blocks := []*llmprovider.Block{}
	providerIDStr := string(llmprovider.ProviderOpenRouter)

	// Generate synthetic tool_use_id for web search
	toolUseID := fmt.Sprintf("or_websearch_%d", time.Now().UnixNano())

	// Create web_search_use block (synthetic - indicates search was performed)
	searchUseBlock := &llmprovider.Block{
		BlockType: llmprovider.BlockTypeWebSearch,
		Sequence:  startSequence,
		Content: map[string]interface{}{
			"tool_use_id": toolUseID,
			"tool_name":   "web_search",
			"input": map[string]interface{}{
				"query": "(auto-invoked by :online model)",
			},
		},
		Provider: &providerIDStr,
	}
	blocks = append(blocks, searchUseBlock)

	// Create web_search_result block
	results := []map[string]interface{}{}
	for _, annotation := range annotations {
		if annotation.URLCitation != nil {
			result := map[string]interface{}{
				"url": annotation.URLCitation.URL,
			}
			if annotation.URLCitation.Title != nil {
				result["title"] = *annotation.URLCitation.Title
			}
			if annotation.URLCitation.Content != nil {
				result["content"] = *annotation.URLCitation.Content
			}
			results = append(results, result)
		}
	}

	searchResultBlock := &llmprovider.Block{
		BlockType: llmprovider.BlockTypeWebSearchResult,
		Sequence:  startSequence + 1,
		Content: map[string]interface{}{
			"tool_use_id": toolUseID,
			"results":     results,
		},
		Provider: &providerIDStr,
	}
	blocks = append(blocks, searchResultBlock)

	return blocks, nil
}

// convertAnnotationsToCitations converts OpenRouter annotations to library Citation format.
func convertAnnotationsToCitations(annotations []Annotation) []llmprovider.Citation {
	citations := []llmprovider.Citation{}

	for _, annotation := range annotations {
		if annotation.URLCitation != nil {
			citation := llmprovider.Citation{
				Type:       "url_citation",
				URL:        annotation.URLCitation.URL,
				StartIndex: &annotation.URLCitation.StartIndex,
				EndIndex:   &annotation.URLCitation.EndIndex,
			}
			if annotation.URLCitation.Title != nil {
				citation.Title = *annotation.URLCitation.Title
			}
			if annotation.URLCitation.Content != nil {
				citation.CitedText = annotation.URLCitation.Content
			}
			citations = append(citations, citation)
		}
	}

	return citations
}

// mapFinishReason maps OpenRouter finish_reason to library stop_reason.
func mapFinishReason(finishReason string) string {
	switch finishReason {
	case "stop":
		return "end_turn"
	case "length":
		return "max_tokens"
	case "tool_calls":
		return "tool_use"
	case "content_filter":
		return "stop_sequence"
	default:
		return finishReason
	}
}
