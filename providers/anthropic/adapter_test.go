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

// Helper function to create string pointers
func strPtr(s string) *string {
	return &s
}
