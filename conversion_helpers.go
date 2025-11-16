package llmprovider

import (
	"fmt"
	"strings"
)

// SplitMessagesAtCrossProviderTool handles server-side tool blocks from other providers
// by converting them into a normalized custom tool pattern that works across all providers.
//
// This is provider-agnostic shared logic used by all adapters during message conversion.
//
// Strategy:
//   1. Find server_tool_use blocks from different providers in assistant messages
//   2. Split the assistant message at each cross-provider tool
//   3. Convert tool_use → synthetic assistant text: "I used the {tool_name} tool"
//   4. Find following result blocks (text blocks after tool_use)
//   5. Inject synthetic user message with tool results
//   6. Continue with remaining blocks in new assistant message
//
// Returns: modified messages with injected synthetic user turns
func SplitMessagesAtCrossProviderTool(messages []Message, currentProvider ProviderID) ([]Message, error) {
	result := make([]Message, 0, len(messages))

	for _, msg := range messages {
		// Only process assistant messages
		if msg.Role != "assistant" {
			result = append(result, msg)
			continue
		}

		// Scan for cross-provider server tools in assistant message
		needsSplit := false
		for _, block := range msg.Blocks {
			if block.IsToolUseBlock() &&
				block.IsServerSideTool() &&
				block.IsFromDifferentProvider(currentProvider) {
				needsSplit = true
				break
			}
		}

		if !needsSplit {
			result = append(result, msg)
			continue
		}

		// Split assistant message at each cross-provider server tool
		currentBlocks := []*Block{}

		for i := 0; i < len(msg.Blocks); i++ {
			block := msg.Blocks[i]

			// Check if this is a cross-provider server tool
			if block.IsToolUseBlock() &&
				block.IsServerSideTool() &&
				block.IsFromDifferentProvider(currentProvider) {

				// Close current assistant message (if any blocks accumulated)
				if len(currentBlocks) > 0 {
					result = append(result, Message{
						Role:   "assistant",
						Blocks: currentBlocks,
					})
					currentBlocks = []*Block{}
				}

				// Get tool name
				toolName, _ := block.GetToolName()
				if toolName == "" {
					toolName = "search" // Fallback
				}

				// Add synthetic assistant text: "I used the X tool"
				syntheticText := fmt.Sprintf("I used the %s tool to help answer your question.", toolName)
				result = append(result, Message{
					Role: "assistant",
					Blocks: []*Block{
						{
							BlockType:   BlockTypeText,
							Sequence:    0,
							TextContent: &syntheticText,
						},
					},
				})

				// Find corresponding result blocks (next text blocks after tool_use)
				resultBlocks, consumed := FindToolResultBlocks(msg.Blocks, i)

				// Add synthetic user message with tool results
				userText := FormatToolResults(resultBlocks)
				result = append(result, Message{
					Role: "user",
					Blocks: []*Block{
						{
							BlockType:   BlockTypeText,
							Sequence:    0,
							TextContent: &userText,
						},
					},
				})

				// Skip the result blocks (already processed)
				i += consumed
				continue
			}

			// Regular block - accumulate for current assistant message
			currentBlocks = append(currentBlocks, block)
		}

		// Add any remaining blocks
		if len(currentBlocks) > 0 {
			result = append(result, Message{
				Role:   "assistant",
				Blocks: currentBlocks,
			})
		}
	}

	return result, nil
}

// FindToolResultBlocks finds text blocks that follow a tool_use block.
// These are assumed to be the tool's results (converted from web_search_tool_result, etc.).
// Returns the result blocks and the number of blocks consumed.
//
// Strategy: Only consume the FIRST text block after a tool_use (the immediate result).
// Subsequent text blocks are part of the assistant's continuation and should remain.
func FindToolResultBlocks(blocks []*Block, toolUseIndex int) ([]*Block, int) {
	results := []*Block{}

	// Look ahead for the first text block (the tool result)
	if toolUseIndex+1 < len(blocks) {
		nextBlock := blocks[toolUseIndex+1]

		// Only take the first text block as the result
		if nextBlock.BlockType == BlockTypeText {
			results = append(results, nextBlock)
			return results, 1
		}
	}

	return results, 0
}

// FormatToolResults formats tool result blocks into user-friendly text for synthetic user message.
func FormatToolResults(blocks []*Block) string {
	if len(blocks) == 0 {
		return "No results found."
	}

	var sb strings.Builder
	sb.WriteString("Tool results:\n\n")

	for _, block := range blocks {
		if block.TextContent != nil {
			sb.WriteString(*block.TextContent)
			sb.WriteString("\n\n")
		}
	}

	return strings.TrimSpace(sb.String())
}
