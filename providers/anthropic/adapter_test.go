package anthropic

import (
	"testing"

	"github.com/haowjy/meridian-llm-go"
)

func TestConvertToAnthropicMessages_Text(t *testing.T) {
	text := "Hello, world!"
	messages := []llmprovider.Message{
		{
			Role: "user",
			Blocks: []*llmprovider.Block{
				{
					BlockType:   llmprovider.BlockTypeText,
					Sequence:    0,
					TextContent: &text,
				},
			},
		},
	}

	result, err := convertToAnthropicMessages(messages)
	if err != nil {
		t.Fatalf("convertToAnthropicMessages() error = %v", err)
	}

	if len(result) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result))
	}
}

func TestConvertToAnthropicMessages_ToolUse(t *testing.T) {
	// Create assistant message with tool_use block
	messages := []llmprovider.Message{
		{
			Role: "assistant",
			Blocks: []*llmprovider.Block{
				{
					BlockType: llmprovider.BlockTypeToolUse,
					Sequence:  0,
					Content: map[string]interface{}{
						"tool_use_id": "toolu_123",
						"tool_name":   "web_search",
						"input": map[string]interface{}{
							"query": "weather in Paris",
						},
					},
				},
			},
		},
	}

	result, err := convertToAnthropicMessages(messages)
	if err != nil {
		t.Fatalf("convertToAnthropicMessages() error = %v", err)
	}

	if len(result) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result))
	}
}

func TestConvertToAnthropicMessages_ToolResult(t *testing.T) {
	content := "Weather is sunny, 25°C"
	messages := []llmprovider.Message{
		{
			Role: "user",
			Blocks: []*llmprovider.Block{
				{
					BlockType:   llmprovider.BlockTypeToolResult,
					Sequence:    0,
					TextContent: &content,
					Content: map[string]interface{}{
						"tool_use_id": "toolu_123",
						"is_error":    false,
					},
				},
			},
		},
	}

	result, err := convertToAnthropicMessages(messages)
	if err != nil {
		t.Fatalf("convertToAnthropicMessages() error = %v", err)
	}

	if len(result) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result))
	}
}

func TestConvertToAnthropicMessages_ToolUse_MissingID(t *testing.T) {
	// Tool use without tool_use_id should error
	messages := []llmprovider.Message{
		{
			Role: "assistant",
			Blocks: []*llmprovider.Block{
				{
					BlockType: llmprovider.BlockTypeToolUse,
					Content: map[string]interface{}{
						"tool_name": "web_search",
						"input":     map[string]interface{}{},
					},
				},
			},
		},
	}

	_, err := convertToAnthropicMessages(messages)
	if err == nil {
		t.Error("expected error for missing tool_use_id, got nil")
	}
}

func TestConvertToAnthropicMessages_ToolResult_MissingID(t *testing.T) {
	// Tool result without tool_use_id should error
	content := "result"
	messages := []llmprovider.Message{
		{
			Role: "user",
			Blocks: []*llmprovider.Block{
				{
					BlockType:   llmprovider.BlockTypeToolResult,
					TextContent: &content,
					Content: map[string]interface{}{
						"is_error": false,
					},
				},
			},
		},
	}

	_, err := convertToAnthropicMessages(messages)
	if err == nil {
		t.Error("expected error for missing tool_use_id, got nil")
	}
}

// Note: Tests for convertFromAnthropicResponse would require creating mock
// Anthropic SDK Message objects, which is complex due to SDK internals.
// These are better tested via integration tests with real API calls.

func TestConvertToAnthropicMessages_CrossProviderServerTool(t *testing.T) {
	// Simulate a conversation with Google web_search, now replaying to Anthropic
	googleProvider := "google"
	executionSide := llmprovider.ExecutionSideServer
	searchText := "I searched the web and found these sources:\n1. [Example](http://example.com)"
	responseText := "Based on the search results, the weather is sunny."

	messages := []llmprovider.Message{
		{
			Role: "user",
			Blocks: []*llmprovider.Block{
				{
					BlockType:   llmprovider.BlockTypeText,
					Sequence:    0,
					TextContent: strPtr("What's the weather?"),
				},
			},
		},
		{
			Role: "assistant",
			Blocks: []*llmprovider.Block{
				{
					BlockType: llmprovider.BlockTypeToolUse,
					Sequence:  0,
					Content: map[string]interface{}{
						"tool_use_id": "google_123",
						"tool_name":   "web_search",
						"input": map[string]interface{}{
							"query": "weather",
						},
					},
					Provider:      &googleProvider,
					ExecutionSide: &executionSide,
				},
				{
					BlockType:   llmprovider.BlockTypeText,
					Sequence:    1,
					TextContent: &searchText,
				},
				{
					BlockType:   llmprovider.BlockTypeText,
					Sequence:    2,
					TextContent: &responseText,
				},
			},
		},
	}

	result, err := convertToAnthropicMessages(messages)
	if err != nil {
		t.Fatalf("convertToAnthropicMessages() error = %v", err)
	}

	// Should have 4 messages after splitting:
	// 1. User: "What's the weather?"
	// 2. Assistant: "I used the web_search tool..."
	// 3. User: "Tool results: I searched the web..."
	// 4. Assistant: "Based on the search results..."
	if len(result) != 4 {
		t.Fatalf("expected 4 messages after split, got %d", len(result))
	}

	// Verify roles
	expectedRoles := []string{"user", "assistant", "user", "assistant"}
	for i, expected := range expectedRoles {
		if string(result[i].Role) != expected {
			t.Errorf("message %d: expected role %s, got %s", i, expected, result[i].Role)
		}
	}
}

func TestConvertToAnthropicMessages_SameProviderServerTool(t *testing.T) {
	// Server-side tool from Anthropic should be skipped during replay
	anthropicProvider := llmprovider.ProviderAnthropic.String()
	executionSide := llmprovider.ExecutionSideServer
	searchText := "I searched the web and found these sources:\n1. [Example](http://example.com)"

	messages := []llmprovider.Message{
		{
			Role: "assistant",
			Blocks: []*llmprovider.Block{
				{
					BlockType: llmprovider.BlockTypeToolUse,
					Sequence:  0,
					Content: map[string]interface{}{
						"tool_use_id": "toolu_123",
						"tool_name":   "web_search",
						"input": map[string]interface{}{
							"query": "weather",
						},
					},
					Provider:      &anthropicProvider,
					ExecutionSide: &executionSide,
				},
				{
					BlockType:   llmprovider.BlockTypeText,
					Sequence:    1,
					TextContent: &searchText,
				},
			},
		},
	}

	result, err := convertToAnthropicMessages(messages)
	if err != nil {
		t.Fatalf("convertToAnthropicMessages() error = %v", err)
	}

	// Should have 1 message (tool_use skipped, only text block remains)
	if len(result) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result))
	}

	// Should be assistant message
	if string(result[0].Role) != "assistant" {
		t.Errorf("expected role assistant, got %s", result[0].Role)
	}
}

func TestSplitMessagesAtCrossProviderTool(t *testing.T) {
	googleProvider := "google"
	executionSide := llmprovider.ExecutionSideServer
	searchText := "Search results here"
	responseText := "Final response"

	messages := []llmprovider.Message{
		{
			Role: "assistant",
			Blocks: []*llmprovider.Block{
				{
					BlockType: llmprovider.BlockTypeToolUse,
					Sequence:  0,
					Content: map[string]interface{}{
						"tool_use_id": "google_123",
						"tool_name":   "web_search",
						"input":       map[string]interface{}{},
					},
					Provider:      &googleProvider,
					ExecutionSide: &executionSide,
				},
				{
					BlockType:   llmprovider.BlockTypeText,
					Sequence:    1,
					TextContent: &searchText,
				},
				{
					BlockType:   llmprovider.BlockTypeText,
					Sequence:    2,
					TextContent: &responseText,
				},
			},
		},
	}

	result, err := llmprovider.SplitMessagesAtCrossProviderTool(messages, llmprovider.ProviderAnthropic)
	if err != nil {
		t.Fatalf("SplitMessagesAtCrossProviderTool() error = %v", err)
	}

	// Should have 3 messages:
	// 1. Assistant: "I used the web_search tool"
	// 2. User: "Tool results: ..."
	// 3. Assistant: "Final response"
	if len(result) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(result))
	}

	// Verify roles
	if result[0].Role != "assistant" || result[1].Role != "user" || result[2].Role != "assistant" {
		t.Errorf("unexpected role sequence: %s, %s, %s", result[0].Role, result[1].Role, result[2].Role)
	}
}

func TestFormatToolResults(t *testing.T) {
	text1 := "Result 1"
	text2 := "Result 2"
	blocks := []*llmprovider.Block{
		{
			BlockType:   llmprovider.BlockTypeText,
			TextContent: &text1,
		},
		{
			BlockType:   llmprovider.BlockTypeText,
			TextContent: &text2,
		},
	}

	result := llmprovider.FormatToolResults(blocks)

	expected := "Tool results:\n\nResult 1\n\nResult 2"
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestFormatToolResults_Empty(t *testing.T) {
	blocks := []*llmprovider.Block{}

	result := llmprovider.FormatToolResults(blocks)

	expected := "No results found."
	if result != expected {
		t.Errorf("expected %q, got %q", expected, result)
	}
}

func TestConvertToAnthropicMessages_ThinkingBlock_WithSignature(t *testing.T) {
	// Test native Anthropic thinking block with signature
	thinkingText := "Let me analyze this problem step by step..."
	anthropicProvider := llmprovider.ProviderAnthropic.String()

	// Simulate Anthropic thinking block with signature in ProviderData
	providerData := []byte(`{"signature": "4k_a"}`)

	messages := []llmprovider.Message{
		{
			Role: "assistant",
			Blocks: []*llmprovider.Block{
				{
					BlockType:    llmprovider.BlockTypeThinking,
					Sequence:     0,
					TextContent:  &thinkingText,
					Provider:     &anthropicProvider,
					ProviderData: providerData,
				},
			},
		},
	}

	result, err := convertToAnthropicMessages(messages)
	if err != nil {
		t.Fatalf("convertToAnthropicMessages() error = %v", err)
	}

	if len(result) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result))
	}

	// Should have 1 block (thinking block with signature)
	blocks := result[0].Content
	if len(blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(blocks))
	}
}

func TestConvertToAnthropicMessages_ThinkingBlock_WithoutSignature(t *testing.T) {
	// Test cross-provider thinking block without signature (e.g., from OpenRouter)
	thinkingText := "Let me think about this..."
	openrouterProvider := "openrouter"

	messages := []llmprovider.Message{
		{
			Role: "assistant",
			Blocks: []*llmprovider.Block{
				{
					BlockType:   llmprovider.BlockTypeThinking,
					Sequence:    0,
					TextContent: &thinkingText,
					Provider:    &openrouterProvider,
					// No ProviderData, no signature
				},
			},
		},
	}

	result, err := convertToAnthropicMessages(messages)
	if err != nil {
		t.Fatalf("convertToAnthropicMessages() error = %v", err)
	}

	if len(result) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result))
	}

	// Should have 1 block (converted to text block with <thinking> tags)
	blocks := result[0].Content
	if len(blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(blocks))
	}

	// Verify it's a text block (not a thinking block)
	// Note: We can't easily inspect the SDK type here, but the conversion should succeed
	// without a 400 error, which is the main goal
}

// Helper function to create string pointers
func strPtr(s string) *string {
	return &s
}

// Tests for splitMessagesAtToolResults

func TestSplitMessagesAtToolResults_SingleRound(t *testing.T) {
	// Assistant message with single tool round: thinking, tool_use, tool_result
	thinking := "I need to search"
	messages := []llmprovider.Message{
		{
			Role: "assistant",
			Blocks: []*llmprovider.Block{
				{
					BlockType:   llmprovider.BlockTypeThinking,
					Sequence:    0,
					TextContent: &thinking,
				},
				{
					BlockType: llmprovider.BlockTypeToolUse,
					Sequence:  1,
					Content: map[string]interface{}{
						"tool_use_id": "call-1",
						"tool_name":   "web_search",
						"input":       map[string]interface{}{"query": "test"},
					},
				},
				{
					BlockType: llmprovider.BlockTypeToolResult,
					Sequence:  2,
					Content: map[string]interface{}{
						"tool_use_id": "call-1",
						"result":      "Search result",
					},
				},
			},
		},
	}

	split := splitMessagesAtToolResults(messages)

	// Should have 2 messages: [assistant (thinking, tool_use), user (tool_result)]
	if len(split) != 2 {
		t.Fatalf("expected 2 split messages, got %d", len(split))
	}

	// First message: assistant with thinking + tool_use
	if split[0].Role != "assistant" {
		t.Errorf("expected first message role to be assistant, got %s", split[0].Role)
	}
	if len(split[0].Blocks) != 2 {
		t.Fatalf("expected first message to have 2 blocks, got %d", len(split[0].Blocks))
	}
	if split[0].Blocks[0].BlockType != llmprovider.BlockTypeThinking {
		t.Errorf("expected first block to be thinking, got %s", split[0].Blocks[0].BlockType)
	}
	if split[0].Blocks[1].BlockType != llmprovider.BlockTypeToolUse {
		t.Errorf("expected second block to be tool_use, got %s", split[0].Blocks[1].BlockType)
	}

	// Second message: user with tool_result
	if split[1].Role != "user" {
		t.Errorf("expected second message role to be user, got %s", split[1].Role)
	}
	if len(split[1].Blocks) != 1 {
		t.Fatalf("expected second message to have 1 block, got %d", len(split[1].Blocks))
	}
	if split[1].Blocks[0].BlockType != llmprovider.BlockTypeToolResult {
		t.Errorf("expected block to be tool_result, got %s", split[1].Blocks[0].BlockType)
	}
}

func TestSplitMessagesAtToolResults_MultipleRounds(t *testing.T) {
	// Assistant message with multiple tool rounds (simulating tool continuation)
	// Blocks: tool_use, tool_result, thinking, tool_use, tool_result
	thinking := "Need another search"
	messages := []llmprovider.Message{
		{
			Role: "assistant",
			Blocks: []*llmprovider.Block{
				{
					BlockType: llmprovider.BlockTypeToolUse,
					Sequence:  0,
					Content: map[string]interface{}{
						"tool_use_id": "call-1",
						"tool_name":   "web_search",
						"input":       map[string]interface{}{"query": "test"},
					},
				},
				{
					BlockType: llmprovider.BlockTypeToolResult,
					Sequence:  1,
					Content: map[string]interface{}{
						"tool_use_id": "call-1",
						"result":      "Result 1",
					},
				},
				{
					BlockType:   llmprovider.BlockTypeThinking,
					Sequence:    2,
					TextContent: &thinking,
				},
				{
					BlockType: llmprovider.BlockTypeToolUse,
					Sequence:  3,
					Content: map[string]interface{}{
						"tool_use_id": "call-2",
						"tool_name":   "web_search",
						"input":       map[string]interface{}{"query": "test2"},
					},
				},
				{
					BlockType: llmprovider.BlockTypeToolResult,
					Sequence:  4,
					Content: map[string]interface{}{
						"tool_use_id": "call-2",
						"result":      "Result 2",
					},
				},
			},
		},
	}

	split := splitMessagesAtToolResults(messages)

	// Should have 4 messages: [assistant, user, assistant, user]
	if len(split) != 4 {
		t.Fatalf("expected 4 split messages, got %d", len(split))
	}

	// Verify alternation pattern
	expectedRoles := []string{"assistant", "user", "assistant", "user"}
	for i, expected := range expectedRoles {
		if split[i].Role != expected {
			t.Errorf("expected message %d role to be %s, got %s", i, expected, split[i].Role)
		}
	}

	// Verify block counts
	expectedBlockCounts := []int{1, 1, 2, 1} // [tool_use], [tool_result], [thinking, tool_use], [tool_result]
	for i, expected := range expectedBlockCounts {
		if len(split[i].Blocks) != expected {
			t.Errorf("expected message %d to have %d blocks, got %d", i, expected, len(split[i].Blocks))
		}
	}

	// Verify block types
	if split[0].Blocks[0].BlockType != llmprovider.BlockTypeToolUse {
		t.Errorf("expected message 0 block 0 to be tool_use, got %s", split[0].Blocks[0].BlockType)
	}
	if split[1].Blocks[0].BlockType != llmprovider.BlockTypeToolResult {
		t.Errorf("expected message 1 block 0 to be tool_result, got %s", split[1].Blocks[0].BlockType)
	}
	if split[2].Blocks[0].BlockType != llmprovider.BlockTypeThinking {
		t.Errorf("expected message 2 block 0 to be thinking, got %s", split[2].Blocks[0].BlockType)
	}
	if split[2].Blocks[1].BlockType != llmprovider.BlockTypeToolUse {
		t.Errorf("expected message 2 block 1 to be tool_use, got %s", split[2].Blocks[1].BlockType)
	}
	if split[3].Blocks[0].BlockType != llmprovider.BlockTypeToolResult {
		t.Errorf("expected message 3 block 0 to be tool_result, got %s", split[3].Blocks[0].BlockType)
	}
}

func TestSplitMessagesAtToolResults_UserMessagePassthrough(t *testing.T) {
	// User messages should pass through unchanged
	text := "Hello"
	messages := []llmprovider.Message{
		{
			Role: "user",
			Blocks: []*llmprovider.Block{
				{
					BlockType:   llmprovider.BlockTypeText,
					Sequence:    0,
					TextContent: &text,
				},
			},
		},
	}

	split := splitMessagesAtToolResults(messages)

	// Should have 1 message (unchanged)
	if len(split) != 1 {
		t.Fatalf("expected 1 message, got %d", len(split))
	}

	// Verify unchanged
	if split[0].Role != "user" {
		t.Errorf("expected role to be user, got %s", split[0].Role)
	}
	if len(split[0].Blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(split[0].Blocks))
	}
	if split[0].Blocks[0].BlockType != llmprovider.BlockTypeText {
		t.Errorf("expected block to be text, got %s", split[0].Blocks[0].BlockType)
	}
}

func TestSplitMessagesAtToolResults_AssistantWithoutToolResults(t *testing.T) {
	// Assistant message without tool_result blocks should pass through unchanged
	thinking := "Let me think"
	text := "Here's my response"
	messages := []llmprovider.Message{
		{
			Role: "assistant",
			Blocks: []*llmprovider.Block{
				{
					BlockType:   llmprovider.BlockTypeThinking,
					Sequence:    0,
					TextContent: &thinking,
				},
				{
					BlockType:   llmprovider.BlockTypeText,
					Sequence:    1,
					TextContent: &text,
				},
			},
		},
	}

	split := splitMessagesAtToolResults(messages)

	// Should have 1 message (unchanged)
	if len(split) != 1 {
		t.Fatalf("expected 1 message, got %d", len(split))
	}

	// Verify unchanged
	if split[0].Role != "assistant" {
		t.Errorf("expected role to be assistant, got %s", split[0].Role)
	}
	if len(split[0].Blocks) != 2 {
		t.Fatalf("expected 2 blocks, got %d", len(split[0].Blocks))
	}
}

func TestSplitMessagesAtToolResults_OnlyToolResult(t *testing.T) {
	// Edge case: Assistant message with only tool_result block
	// (shouldn't happen in practice, but should handle gracefully)
	messages := []llmprovider.Message{
		{
			Role: "assistant",
			Blocks: []*llmprovider.Block{
				{
					BlockType: llmprovider.BlockTypeToolResult,
					Sequence:  0,
					Content: map[string]interface{}{
						"tool_use_id": "call-1",
						"result":      "Result",
					},
				},
			},
		},
	}

	split := splitMessagesAtToolResults(messages)

	// Should have 1 message: user with tool_result
	if len(split) != 1 {
		t.Fatalf("expected 1 message, got %d", len(split))
	}

	// Should be user message
	if split[0].Role != "user" {
		t.Errorf("expected role to be user, got %s", split[0].Role)
	}
	if len(split[0].Blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(split[0].Blocks))
	}
	if split[0].Blocks[0].BlockType != llmprovider.BlockTypeToolResult {
		t.Errorf("expected block to be tool_result, got %s", split[0].Blocks[0].BlockType)
	}
}

// Tests for mergeConsecutiveSameRoleMessages

func TestMergeConsecutiveSameRoleMessages_ConsecutiveUserMessages(t *testing.T) {
	// Simulate the bug scenario: tool_result block + new user query
	toolResultText := "Search completed successfully"
	newQueryText := "Who else is related to aria?"

	messages := []llmprovider.Message{
		{
			Role: "user",
			Blocks: []*llmprovider.Block{
				{
					BlockType:   llmprovider.BlockTypeToolResult,
					Sequence:    0,
					TextContent: &toolResultText,
					Content: map[string]interface{}{
						"tool_use_id": "toolu_123",
						"is_error":    false,
					},
				},
			},
		},
		{
			Role: "user",
			Blocks: []*llmprovider.Block{
				{
					BlockType:   llmprovider.BlockTypeText,
					Sequence:    0,
					TextContent: &newQueryText,
				},
			},
		},
	}

	merged := mergeConsecutiveSameRoleMessages(messages)

	// Should have 1 message (two consecutive user messages merged)
	if len(merged) != 1 {
		t.Fatalf("expected 1 merged message, got %d", len(merged))
	}

	// Should be user role
	if merged[0].Role != "user" {
		t.Errorf("expected role user, got %s", merged[0].Role)
	}

	// Should have 2 blocks (tool_result + text)
	if len(merged[0].Blocks) != 2 {
		t.Fatalf("expected 2 blocks, got %d", len(merged[0].Blocks))
	}

	// Verify block order preserved
	if merged[0].Blocks[0].BlockType != llmprovider.BlockTypeToolResult {
		t.Errorf("expected first block to be tool_result, got %s", merged[0].Blocks[0].BlockType)
	}
	if merged[0].Blocks[1].BlockType != llmprovider.BlockTypeText {
		t.Errorf("expected second block to be text, got %s", merged[0].Blocks[1].BlockType)
	}
}

func TestMergeConsecutiveSameRoleMessages_ConsecutiveAssistantMessages(t *testing.T) {
	text1 := "First response"
	text2 := "Second response"

	messages := []llmprovider.Message{
		{
			Role: "assistant",
			Blocks: []*llmprovider.Block{
				{
					BlockType:   llmprovider.BlockTypeText,
					Sequence:    0,
					TextContent: &text1,
				},
			},
		},
		{
			Role: "assistant",
			Blocks: []*llmprovider.Block{
				{
					BlockType:   llmprovider.BlockTypeText,
					Sequence:    0,
					TextContent: &text2,
				},
			},
		},
	}

	merged := mergeConsecutiveSameRoleMessages(messages)

	// Should have 1 message (two consecutive assistant messages merged)
	if len(merged) != 1 {
		t.Fatalf("expected 1 merged message, got %d", len(merged))
	}

	// Should be assistant role
	if merged[0].Role != "assistant" {
		t.Errorf("expected role assistant, got %s", merged[0].Role)
	}

	// Should have 2 blocks
	if len(merged[0].Blocks) != 2 {
		t.Fatalf("expected 2 blocks, got %d", len(merged[0].Blocks))
	}
}

func TestMergeConsecutiveSameRoleMessages_ProperlyAlternating(t *testing.T) {
	userText := "Hello"
	assistantText := "Hi there"

	messages := []llmprovider.Message{
		{
			Role: "user",
			Blocks: []*llmprovider.Block{
				{
					BlockType:   llmprovider.BlockTypeText,
					Sequence:    0,
					TextContent: &userText,
				},
			},
		},
		{
			Role: "assistant",
			Blocks: []*llmprovider.Block{
				{
					BlockType:   llmprovider.BlockTypeText,
					Sequence:    0,
					TextContent: &assistantText,
				},
			},
		},
	}

	merged := mergeConsecutiveSameRoleMessages(messages)

	// Should have 2 messages (no merging needed)
	if len(merged) != 2 {
		t.Fatalf("expected 2 messages (no merge), got %d", len(merged))
	}

	// Verify roles preserved
	if merged[0].Role != "user" || merged[1].Role != "assistant" {
		t.Errorf("expected user/assistant roles, got %s/%s", merged[0].Role, merged[1].Role)
	}
}

func TestMergeConsecutiveSameRoleMessages_MultipleConsecutiveGroups(t *testing.T) {
	// Test pattern: user, user, assistant, assistant, user
	// Should merge to: user, assistant, user

	messages := []llmprovider.Message{
		{Role: "user", Blocks: []*llmprovider.Block{{BlockType: llmprovider.BlockTypeText, TextContent: strPtr("u1")}}},
		{Role: "user", Blocks: []*llmprovider.Block{{BlockType: llmprovider.BlockTypeText, TextContent: strPtr("u2")}}},
		{Role: "assistant", Blocks: []*llmprovider.Block{{BlockType: llmprovider.BlockTypeText, TextContent: strPtr("a1")}}},
		{Role: "assistant", Blocks: []*llmprovider.Block{{BlockType: llmprovider.BlockTypeText, TextContent: strPtr("a2")}}},
		{Role: "user", Blocks: []*llmprovider.Block{{BlockType: llmprovider.BlockTypeText, TextContent: strPtr("u3")}}},
	}

	merged := mergeConsecutiveSameRoleMessages(messages)

	// Should have 3 messages
	if len(merged) != 3 {
		t.Fatalf("expected 3 merged messages, got %d", len(merged))
	}

	// Verify roles
	if merged[0].Role != "user" || merged[1].Role != "assistant" || merged[2].Role != "user" {
		t.Errorf("expected user/assistant/user, got %s/%s/%s", merged[0].Role, merged[1].Role, merged[2].Role)
	}

	// Verify block counts
	if len(merged[0].Blocks) != 2 || len(merged[1].Blocks) != 2 || len(merged[2].Blocks) != 1 {
		t.Errorf("expected block counts [2,2,1], got [%d,%d,%d]",
			len(merged[0].Blocks), len(merged[1].Blocks), len(merged[2].Blocks))
	}
}

func TestMergeConsecutiveSameRoleMessages_EmptyInput(t *testing.T) {
	messages := []llmprovider.Message{}
	merged := mergeConsecutiveSameRoleMessages(messages)

	if len(merged) != 0 {
		t.Fatalf("expected empty result, got %d messages", len(merged))
	}
}

func TestMergeConsecutiveSameRoleMessages_SingleMessage(t *testing.T) {
	messages := []llmprovider.Message{
		{
			Role: "user",
			Blocks: []*llmprovider.Block{
				{BlockType: llmprovider.BlockTypeText, TextContent: strPtr("Hello")},
			},
		},
	}

	merged := mergeConsecutiveSameRoleMessages(messages)

	// Should return same message unchanged
	if len(merged) != 1 {
		t.Fatalf("expected 1 message, got %d", len(merged))
	}

	if merged[0].Role != "user" {
		t.Errorf("expected role user, got %s", merged[0].Role)
	}
}

func TestConvertToAnthropicMessages_WithMerging_Integration(t *testing.T) {
	// Integration test: Verify full conversion flow with merging
	// Simulates real scenario from user's bug report
	toolResultText := "Search results found"
	newQueryText := "Who else is related to aria?"

	messages := []llmprovider.Message{
		{
			Role: "user",
			Blocks: []*llmprovider.Block{
				{BlockType: llmprovider.BlockTypeText, TextContent: strPtr("Initial query")},
			},
		},
		{
			Role: "assistant",
			Blocks: []*llmprovider.Block{
				{
					BlockType: llmprovider.BlockTypeToolUse,
					Content: map[string]interface{}{
						"tool_use_id": "toolu_123",
						"tool_name":   "doc_search",
						"input":       map[string]interface{}{"query": "aria"},
					},
				},
			},
		},
		{
			Role: "user",
			Blocks: []*llmprovider.Block{
				{
					BlockType:   llmprovider.BlockTypeToolResult,
					TextContent: &toolResultText,
					Content: map[string]interface{}{
						"tool_use_id": "toolu_123",
						"is_error":    false,
					},
				},
			},
		},
		{
			Role: "user",
			Blocks: []*llmprovider.Block{
				{BlockType: llmprovider.BlockTypeText, TextContent: &newQueryText},
			},
		},
	}

	result, err := convertToAnthropicMessages(messages)
	if err != nil {
		t.Fatalf("convertToAnthropicMessages() error = %v", err)
	}

	// Should have 3 messages after merging:
	// 1. User: "Initial query"
	// 2. Assistant: tool_use
	// 3. User: tool_result + "Who else..." (MERGED)
	if len(result) != 3 {
		t.Fatalf("expected 3 messages after merge, got %d", len(result))
	}

	// Verify roles alternate properly
	expectedRoles := []string{"user", "assistant", "user"}
	for i, expected := range expectedRoles {
		if string(result[i].Role) != expected {
			t.Errorf("message %d: expected role %s, got %s", i, expected, result[i].Role)
		}
	}

	// Verify last message has 2 blocks (tool_result + text)
	lastMessage := result[2]
	if len(lastMessage.Content) != 2 {
		t.Fatalf("expected last message to have 2 blocks, got %d", len(lastMessage.Content))
	}
}
