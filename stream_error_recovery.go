package llmprovider

import "fmt"

// MalformedToolRecovery contains blocks to emit when tool JSON parsing fails.
// This allows the LLM to see what went wrong and retry.
type MalformedToolRecovery struct {
	ToolUseBlock    *Block
	ToolResultBlock *Block
}

// NewMalformedToolRecovery creates recovery blocks for a malformed tool call.
// This is reusable across all providers (OpenRouter, future OpenAI, Gemini, etc.)
//
// When a provider receives a tool call with unparseable JSON arguments, instead of
// aborting the stream, it can use this helper to emit:
//  1. A tool_use block with the raw malformed input (truncated)
//  2. A tool_result block marked as error, explaining the parse failure
//
// This allows the LLM to see what went wrong and potentially retry with valid JSON.
//
// Parameters:
//   - toolID: The tool_use_id from the provider
//   - toolName: The tool name
//   - rawInput: The malformed JSON string (will be truncated to maxLen)
//   - parseErr: The JSON parse error
//   - sequence: Current block sequence number
//   - provider: Provider identifier (optional, for tracking)
//
// Returns:
//   - recovery: Struct containing both blocks to emit
//   - nextSequence: The next available sequence number after emitting both blocks
func NewMalformedToolRecovery(
	toolID, toolName, rawInput string,
	parseErr error,
	sequence int,
	provider *string,
) (*MalformedToolRecovery, int) {
	serverSide := ExecutionSideServer

	// Truncate raw input to avoid bloating the context
	const maxInputLen = 500
	truncatedInput := truncateString(rawInput, maxInputLen)

	toolUseBlock := &Block{
		BlockType: BlockTypeToolUse,
		Sequence:  sequence,
		Content: map[string]interface{}{
			"tool_use_id": toolID,
			"tool_name":   toolName,
			// Store malformed input under special key so it's clear this isn't valid input
			"input": map[string]interface{}{"_malformed": truncatedInput},
		},
		ExecutionSide: &serverSide,
		Provider:      provider,
	}

	toolResultBlock := &Block{
		BlockType: BlockTypeToolResult,
		Sequence:  sequence + 1,
		Content: map[string]interface{}{
			"tool_use_id": toolID,
			"tool_name":   toolName,
			"is_error":    true,
			"error":       fmt.Sprintf("MALFORMED_JSON: %v", parseErr),
		},
	}

	return &MalformedToolRecovery{
		ToolUseBlock:    toolUseBlock,
		ToolResultBlock: toolResultBlock,
	}, sequence + 2
}

// truncateString truncates a string to maxLen characters, adding "..." if truncated.
func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
