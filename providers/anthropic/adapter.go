package anthropic

import (
	"encoding/json"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"

	"github.com/haowjy/meridian-llm-go"
)

// convertToAnthropicMessages converts library messages to Anthropic SDK format.
func convertToAnthropicMessages(messages []llmprovider.Message) ([]anthropic.MessageParam, error) {
	// Phase 1: Handle cross-provider server tools by splitting messages
	// This converts server tools from other providers into synthetic conversation turns
	processedMessages, err := llmprovider.SplitMessagesAtCrossProviderTool(messages, llmprovider.ProviderAnthropic)
	if err != nil {
		return nil, fmt.Errorf("failed to process cross-provider tools: %w", err)
	}

	result := make([]anthropic.MessageParam, 0, len(processedMessages))

	for i, msg := range processedMessages {
		// Convert blocks to Anthropic ContentBlockParamUnion
		blocks := make([]anthropic.ContentBlockParamUnion, 0, len(msg.Blocks))

		for j, block := range msg.Blocks {
			// Same-provider optimization: Replay original Anthropic blocks from ProviderData
			// This preserves provider-specific data (encrypted_content, etc.) for perfect replay
			if block.IsFromProvider(llmprovider.ProviderAnthropic) && block.HasProviderData() {
				if originalBlock, err := replayAnthropicBlock(block); err == nil {
					blocks = append(blocks, originalBlock)
					continue
				}
				// Fall through to normalized conversion if replay fails
			}

			// Cross-provider check: Server tools from other providers should have been split
			if block.BlockType == llmprovider.BlockTypeToolUse &&
				block.ExecutionSide != nil &&
				*block.ExecutionSide == llmprovider.ExecutionSideServer &&
				block.IsFromDifferentProvider(llmprovider.ProviderAnthropic) {
				// Cross-provider server tools should have been handled by SplitMessagesAtCrossProviderTool
				return nil, fmt.Errorf("message %d, block %d: unexpected cross-provider server tool (should have been split)", i, j)
			}

			switch block.BlockType {
			case llmprovider.BlockTypeText:
				// Text block: use TextContent field
				if block.TextContent == nil {
					return nil, fmt.Errorf("message %d, block %d: text block missing text_content", i, j)
				}
				blocks = append(blocks, anthropic.NewTextBlock(*block.TextContent))

			case llmprovider.BlockTypeToolUse:
				// Tool use block: extract tool_use_id, tool_name, and input
				if block.Content == nil {
					return nil, fmt.Errorf("message %d, block %d: tool_use block missing content", i, j)
				}

				toolUseID, ok := block.Content["tool_use_id"].(string)
				if !ok || toolUseID == "" {
					return nil, fmt.Errorf("message %d, block %d: tool_use block missing tool_use_id", i, j)
				}

				toolName, ok := block.Content["tool_name"].(string)
				if !ok || toolName == "" {
					return nil, fmt.Errorf("message %d, block %d: tool_use block missing tool_name", i, j)
				}

				input, ok := block.Content["input"]
				if !ok {
					return nil, fmt.Errorf("message %d, block %d: tool_use block missing input", i, j)
				}

				// Create Anthropic tool use block using SDK helper
				blocks = append(blocks, anthropic.NewToolUseBlock(toolUseID, input, toolName))

			case llmprovider.BlockTypeToolResult:
				// Tool result block: extract tool_use_id and content
				if block.Content == nil {
					return nil, fmt.Errorf("message %d, block %d: tool_result block missing content", i, j)
				}

				toolUseID, ok := block.Content["tool_use_id"].(string)
				if !ok || toolUseID == "" {
					return nil, fmt.Errorf("message %d, block %d: tool_result block missing tool_use_id", i, j)
				}

				// Check if this is an error result
				isError := false
				if errFlag, ok := block.Content["is_error"].(bool); ok {
					isError = errFlag
				}

				// Tool result content can be in TextContent or Content["content"]
				var resultContent string
				if block.TextContent != nil {
					resultContent = *block.TextContent
				} else if contentStr, ok := block.Content["content"].(string); ok {
					resultContent = contentStr
				}

				// Create Anthropic tool result block using SDK helper
				blocks = append(blocks, anthropic.NewToolResultBlock(toolUseID, resultContent, isError))

			case llmprovider.BlockTypeThinking:
				// Thinking block: extract thinking text and signature
				if block.TextContent == nil {
					return nil, fmt.Errorf("message %d, block %d: thinking block missing text_content", i, j)
				}

				// Extract signature from ProviderData (where we store it during conversion)
				var signature string
				if len(block.ProviderData) > 0 {
					var providerData map[string]interface{}
					if err := json.Unmarshal(block.ProviderData, &providerData); err == nil {
						if sig, ok := providerData["signature"].(string); ok {
							signature = sig
						}
					}
				}

				// Create Anthropic thinking block using SDK helper
				blocks = append(blocks, anthropic.NewThinkingBlock(signature, *block.TextContent))

			case llmprovider.BlockTypeWebSearch, llmprovider.BlockTypeWebSearchResult:
				// Web search block (invocation or result)
				// Same-provider replay: Use ProviderData if available
				// Cross-provider replay: Not yet supported (future work)

				if block.IsFromProvider(llmprovider.ProviderAnthropic) && block.HasProviderData() {
					// Replay original Anthropic block from ProviderData
					// This preserves provider-specific fields like EncryptedContent
					originalBlock, err := replayAnthropicBlock(block)
					if err == nil {
						blocks = append(blocks, originalBlock)
						continue
					}
					// If replay fails, fall through to error
					return nil, fmt.Errorf("message %d, block %d: failed to replay web_search block: %w", i, j, err)
				}

				// Cross-provider web_search replay not yet implemented
				// Design: Convert to synthetic tool_use + tool_result (see design doc)
				return nil, fmt.Errorf("message %d, block %d: cross-provider web_search replay not yet supported", i, j)

			default:
				// Skip unsupported block types (image, document, etc.)
				// These will be added as needed in future iterations
			}
		}

		// Create message based on role
		var message anthropic.MessageParam
		switch msg.Role {
		case "user":
			message = anthropic.NewUserMessage(blocks...)
		case "assistant":
			message = anthropic.NewAssistantMessage(blocks...)
		default:
			return nil, fmt.Errorf("message %d: unsupported role '%s'", i, msg.Role)
		}

		result = append(result, message)
	}

	return result, nil
}

// replayAnthropicBlock attempts to deserialize ProviderData and reconstruct the exact
// Anthropic SDK block for same-provider replay. This preserves provider-specific data
// like encrypted_content that would be lost in normalization.
func replayAnthropicBlock(block *llmprovider.Block) (anthropic.ContentBlockParamUnion, error) {
	if !block.HasProviderData() {
		return anthropic.ContentBlockParamUnion{}, fmt.Errorf("block has no provider data")
	}

	// Unmarshal to determine block type
	var rawBlock struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(block.ProviderData, &rawBlock); err != nil {
		return anthropic.ContentBlockParamUnion{}, fmt.Errorf("failed to unmarshal provider data: %w", err)
	}

	switch rawBlock.Type {
	case "server_tool_use":
		// Deserialize server_tool_use block
		var serverToolUse struct {
			ID    string                 `json:"id"`
			Name  string                 `json:"name"`
			Input map[string]interface{} `json:"input"`
		}
		if err := json.Unmarshal(block.ProviderData, &serverToolUse); err != nil {
			return anthropic.ContentBlockParamUnion{}, fmt.Errorf("failed to unmarshal server_tool_use: %w", err)
		}
		// Use SDK constructor to rebuild block
		return anthropic.NewServerToolUseBlock(serverToolUse.ID, serverToolUse.Input), nil

	case "web_search_tool_result":
		// First try to deserialize using the new sparse provider_data shape that we
		// construct in convertAnthropicBlock (type/tool_use_id/content{type,results|error_code}).
		var replayPD struct {
			ToolUseID string `json:"tool_use_id"`
			Content   struct {
				Type    string `json:"type"`
				Results []struct {
					URL              string `json:"url"`
					Title            string `json:"title"`
					PageAge          string `json:"page_age"`
					EncryptedContent string `json:"encrypted_content"`
				} `json:"results"`
				ErrorCode string `json:"error_code"`
			} `json:"content"`
		}

		if err := json.Unmarshal(block.ProviderData, &replayPD); err == nil && replayPD.ToolUseID != "" {
			// New-format provider_data path
			if len(replayPD.Content.Results) > 0 {
				searchResults := make([]anthropic.WebSearchResultBlockParam, len(replayPD.Content.Results))
				for i, result := range replayPD.Content.Results {
					searchResults[i] = anthropic.WebSearchResultBlockParam{
						EncryptedContent: result.EncryptedContent,
						Title:            result.Title,
						URL:              result.URL,
						PageAge:          anthropic.Opt(result.PageAge),
						Type:             "web_search_result",
					}
				}
				return anthropic.NewWebSearchToolResultBlock(searchResults, replayPD.ToolUseID), nil
			}

			if replayPD.Content.ErrorCode != "" {
				searchError := anthropic.WebSearchToolRequestErrorParam{
					ErrorCode: anthropic.WebSearchToolRequestErrorErrorCode(replayPD.Content.ErrorCode),
				}
				return anthropic.NewWebSearchToolResultBlock(searchError, replayPD.ToolUseID), nil
			}
			// If we got here, new-format provider_data was structurally valid but empty;
			// fall through to legacy path below for safety.
		}

		// Legacy path: provider_data was constructed from the SDK union directly.
		// Deserialize using Anthropic's WebSearchToolResultBlockContentUnion type.
		var legacy struct {
			ToolUseID string          `json:"tool_use_id"`
			Content   json.RawMessage `json:"content"`
		}
		if err := json.Unmarshal(block.ProviderData, &legacy); err != nil {
			return anthropic.ContentBlockParamUnion{}, fmt.Errorf("failed to unmarshal web_search_tool_result: %w", err)
		}

		var contentUnion anthropic.WebSearchToolResultBlockContentUnion
		if err := json.Unmarshal(legacy.Content, &contentUnion); err != nil {
			return anthropic.ContentBlockParamUnion{}, fmt.Errorf("failed to unmarshal web_search content: %w", err)
		}

		if len(contentUnion.OfWebSearchResultBlockArray) > 0 {
			searchResults := make([]anthropic.WebSearchResultBlockParam, len(contentUnion.OfWebSearchResultBlockArray))
			for i, result := range contentUnion.OfWebSearchResultBlockArray {
				searchResults[i] = anthropic.WebSearchResultBlockParam{
					EncryptedContent: result.EncryptedContent,
					Title:            result.Title,
					URL:              result.URL,
					PageAge:          anthropic.Opt(result.PageAge),
					Type:             result.Type,
				}
			}
			return anthropic.NewWebSearchToolResultBlock(searchResults, legacy.ToolUseID), nil
		}

		if contentUnion.ErrorCode != "" {
			searchError := anthropic.WebSearchToolRequestErrorParam{
				ErrorCode: anthropic.WebSearchToolRequestErrorErrorCode(contentUnion.ErrorCode),
			}
			return anthropic.NewWebSearchToolResultBlock(searchError, legacy.ToolUseID), nil
		}

		return anthropic.ContentBlockParamUnion{}, fmt.Errorf("web_search_tool_result has no results and no error")

	default:
		// Other block types not yet supported for raw replay
		// Fall back to normalized conversion
		return anthropic.ContentBlockParamUnion{}, fmt.Errorf("raw replay not implemented for type: %s", rawBlock.Type)
	}
}

// convertAnthropicBlock converts a single Anthropic ContentBlockUnion to library Block format.
// This is the shared conversion logic used by both streaming and non-streaming paths.
// It normalizes provider-specific block types (web_search_tool_result, server_tool_use)
// to standard library types (web_search, web_search_result, tool_use) while preserving raw data in ProviderData.
func convertAnthropicBlock(content anthropic.ContentBlockUnion, sequence int) (*llmprovider.Block, error) {
	providerID := llmprovider.ProviderAnthropic.String()
	provider := &providerID

	// Check content type and extract appropriate fields
	switch content.Type {
	case "text":
		text := content.Text

		// Convert citations if present
		var citations []llmprovider.Citation
		if len(content.Citations) > 0 {
			citations = make([]llmprovider.Citation, 0, len(content.Citations))
			for _, cite := range content.Citations {
				citation := llmprovider.Citation{
					Type: cite.Type, // "web_search_result_location", "char_location", etc.
				}

				// Common fields
				if cite.CitedText != "" {
					citation.CitedText = &cite.CitedText
				}

				// Web search result location fields
				if cite.Type == "web_search_result_location" {
					citation.URL = cite.URL
					citation.Title = cite.Title

					// Store encrypted_index in ProviderData
					if cite.EncryptedIndex != "" {
						providerData := map[string]interface{}{
							"encrypted_index": cite.EncryptedIndex,
						}
						if rawData, err := json.Marshal(providerData); err == nil {
							citation.ProviderData = rawData
						}
					}
				}

				// Search result location fields (for client-side search tools)
				if cite.Type == "search_result_location" {
					citation.Title = cite.Title
					citation.URL = cite.URL
					if cite.SearchResultIndex >= 0 {
						idx := int(cite.SearchResultIndex)
						citation.ResultIndex = &idx
					}
					if cite.Source != "" {
						citation.ProviderData, _ = json.Marshal(map[string]interface{}{
							"source": cite.Source,
						})
					}
				}

				// Char location fields (for document citations)
				if cite.Type == "char_location" {
					if cite.StartCharIndex >= 0 {
						idx := int(cite.StartCharIndex)
						citation.StartIndex = &idx
					}
					if cite.EndCharIndex >= 0 {
						idx := int(cite.EndCharIndex)
						citation.EndIndex = &idx
					}
					if cite.DocumentTitle != "" {
						citation.Title = cite.DocumentTitle
					}
				}

				citations = append(citations, citation)
			}
		}

		return &llmprovider.Block{
			BlockType:   llmprovider.BlockTypeText,
			Sequence:    sequence,
			TextContent: &text,
			Content:     nil,
			Provider:    provider,
			Citations:   citations,
		}, nil

	case "thinking":
		thinking := content.Thinking
		signature := content.Signature

		// Thinking blocks without signatures cannot be verified as extended thinking
		// Convert to regular text blocks (unverifiable thinking = regular text)
		if signature == "" {
			return &llmprovider.Block{
				BlockType:   llmprovider.BlockTypeText,
				Sequence:    sequence,
				TextContent: &thinking,
				Provider:    provider,
			}, nil
		}

		// Signature is provider-specific metadata (cryptographic verification)
		// Store in ProviderData, not Content (Content is for semantic data only)
		providerDataMap := map[string]interface{}{
			"signature": signature,
		}
		providerData, err := json.Marshal(providerDataMap)
		if err != nil {
			return nil, fmt.Errorf("marshal thinking signature: %w", err)
		}

		return &llmprovider.Block{
			BlockType:    llmprovider.BlockTypeThinking,
			Sequence:     sequence,
			TextContent:  &thinking,
			Content:      nil, // No semantic content for thinking blocks
			Provider:     provider,
			ProviderData: providerData, // Signature stored as provider-specific metadata
		}, nil

	case "tool_use":
		// Tool use block from Anthropic response
		contentMap := make(map[string]interface{})
		contentMap["tool_use_id"] = content.ID
		contentMap["tool_name"] = content.Name
		contentMap["input"] = content.Input

		// Determine execution side based on tool type
		// Server-side tools: web_search (results included automatically)
		// Client-side tools: bash, text_editor, custom (consumer must execute)
		executionSide := llmprovider.ExecutionSideClient
		if content.Name == "web_search" {
			executionSide = llmprovider.ExecutionSideServer
		}

		return &llmprovider.Block{
			BlockType:     llmprovider.BlockTypeToolUse,
			Sequence:      sequence,
			Content:       contentMap,
			ExecutionSide: &executionSide,
			Provider:      provider,
		}, nil

	// Provider-specific block types (web_search_tool_result, server_tool_use, etc.)
	default:
		// Handle known provider-specific types by extracting essential fields
		switch content.Type {
		case "server_tool_use":
			// Server-side tool use (e.g., web_search executed by Anthropic)
			// Build sparse JSON manually (SDK's RawJSON() includes inflated struct with zero-value fields)
			providerDataMap := map[string]interface{}{
				"type":  content.Type,
				"id":    content.ID,
				"name":  content.Name,
				"input": content.Input, // json.RawMessage
			}
			rawData, err := json.Marshal(providerDataMap)
			if err != nil {
				return nil, fmt.Errorf("marshal server_tool_use provider data: %w", err)
			}

			// Extract essential fields for tool result matching
			contentMap := make(map[string]interface{})
			contentMap["tool_use_id"] = content.ID
			contentMap["tool_name"] = content.Name
			contentMap["input"] = content.Input

			executionSide := llmprovider.ExecutionSideServer

			// Determine block type based on tool name.
			// web_search → BlockTypeWebSearch (invocation, LLM request, server-executed)
			// Other server tools use generic BlockTypeToolUse.
			blockType := llmprovider.BlockTypeToolUse // Default for server tools
			if content.Name == "web_search" {
				blockType = llmprovider.BlockTypeWebSearch
			}

			return &llmprovider.Block{
				BlockType:     blockType,
				Sequence:      sequence,
				Content:       contentMap,
				ExecutionSide: &executionSide,
				Provider:      provider,
				ProviderData:  rawData, // Sparse JSON for replay
			}, nil

		case "web_search_tool_result":
			// Web search tool result from Anthropic (server-executed search)
			// Normalized to web_search_result block type (not tool_result - this is not a client tool)
			// Can be either success (results array) or error
			contentMap := make(map[string]interface{})

			// Extract tool_use_id
			if content.ToolUseID != "" {
				contentMap["tool_use_id"] = content.ToolUseID
			}

			// Check if this is an error or success for normalized Content
			if content.Content.Type == "web_search_tool_result_error" {
				// Error case: store error information in normalized content
				contentMap["is_error"] = true
				contentMap["error_code"] = string(content.Content.ErrorCode)
			} else {
				// Success case: convert search results to normalized format
				sources := content.Content.OfWebSearchResultBlockArray
				results := make([]map[string]interface{}, 0, len(sources))

				for _, source := range sources {
					result := map[string]interface{}{
						"title": source.Title,
						"url":   source.URL,
					}
					// Add optional page_age field
					if source.PageAge != "" {
						result["page_age"] = source.PageAge
					}
					// Note: EncryptedContent cannot be decrypted, so we don't include snippet
					// The full raw block is preserved in ProviderData for replay
					results = append(results, result)
				}

				contentMap["results"] = results
			}

			// Build sparse JSON for ProviderData.
			// IMPORTANT: Do NOT use RawJSON() here; it re-marshals the entire union
			// struct and introduces internal fields (OfWebSearchResultBlockArray, etc.).
			// Instead, manually construct a minimal JSON object that matches Anthropic's
			// documented shape and preserves EncryptedContent for replay.

			providerDataContent := make(map[string]interface{})
			if content.Content.Type == "web_search_tool_result_error" {
				// Error case
				providerDataContent["type"] = "web_search_tool_result_error"
				providerDataContent["error_code"] = string(content.Content.ErrorCode)
			} else {
				// Success case
				providerDataContent["type"] = "web_search_tool_result_success"

				sources := content.Content.OfWebSearchResultBlockArray
				results := make([]map[string]interface{}, 0, len(sources))

				for _, source := range sources {
					result := map[string]interface{}{
						"type":  "web_search_result",
						"url":   source.URL,
						"title": source.Title,
					}
					if source.PageAge != "" {
						result["page_age"] = source.PageAge
					}
					if source.EncryptedContent != "" {
						result["encrypted_content"] = source.EncryptedContent
					}
					results = append(results, result)
				}

				providerDataContent["results"] = results
			}

			providerDataMap := map[string]interface{}{
				"type":        content.Type,
				"tool_use_id": content.ToolUseID,
				"content":     providerDataContent,
			}

			rawData, err := json.Marshal(providerDataMap)
			if err != nil {
				return nil, fmt.Errorf("marshal web_search_tool_result provider data: %w", err)
			}

			return &llmprovider.Block{
				BlockType:    llmprovider.BlockTypeWebSearchResult, // Server-executed search result, not client tool
				Sequence:     sequence,
				Content:      contentMap,
				Provider:     provider,
				ProviderData: rawData, // Sparse JSON that preserves encrypted_content for replay
			}, nil

		default:
			// Unknown provider-specific type - preserve raw data only using RawJSON()
			rawData := json.RawMessage([]byte(content.RawJSON()))

			// Guess block type based on naming convention
			blockType := llmprovider.BlockTypeToolResult
			if content.Type == "server_tool_use" {
				blockType = llmprovider.BlockTypeToolUse
			}

			return &llmprovider.Block{
				BlockType:    blockType,
				Sequence:     sequence,
				Provider:     provider,
				ProviderData: rawData, // Store entire raw block for replay/debugging
			}, nil
		}
	}
}

// convertFromAnthropicResponse converts an Anthropic response to library format.
func convertFromAnthropicResponse(msg *anthropic.Message) (*llmprovider.GenerateResponse, error) {
	// Convert content blocks using shared conversion logic
	blocks := make([]*llmprovider.Block, 0, len(msg.Content))

	for i, content := range msg.Content {
		block, err := convertAnthropicBlock(content, i)
		if err != nil {
			// Log error but continue (don't fail entire response)
			continue
		}
		if block != nil {
			blocks = append(blocks, block)
		}
	}

	// Build response metadata with provider-specific data
	responseMetadata := make(map[string]interface{})

	// Add stop sequence if present
	if msg.StopSequence != "" {
		responseMetadata["stop_sequence"] = msg.StopSequence
	}

	// Add cache token usage if present (Anthropic prompt caching)
	if msg.Usage.CacheCreationInputTokens > 0 {
		responseMetadata["cache_creation_input_tokens"] = int(msg.Usage.CacheCreationInputTokens)
	}
	if msg.Usage.CacheReadInputTokens > 0 {
		responseMetadata["cache_read_input_tokens"] = int(msg.Usage.CacheReadInputTokens)
	}

	return &llmprovider.GenerateResponse{
		Blocks:           blocks,
		Model:            string(msg.Model),
		InputTokens:      int(msg.Usage.InputTokens),
		OutputTokens:     int(msg.Usage.OutputTokens),
		StopReason:       string(msg.StopReason),
		ResponseMetadata: responseMetadata,
	}, nil
}
