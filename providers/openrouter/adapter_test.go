package openrouter

import (
	"testing"

	"github.com/haowjy/meridian-llm-go"
)

// TestConvertToOpenRouterMessages_SimpleText tests basic text message conversion
func TestConvertToOpenRouterMessages_SimpleText(t *testing.T) {
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

	result, err := convertToOpenRouterMessages(messages)
	if err != nil {
		t.Fatalf("error = %v", err)
	}

	if len(result) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result))
	}

	if result[0].Role != "user" {
		t.Errorf("expected role 'user', got '%s'", result[0].Role)
	}

	if content, ok := result[0].Content.(string); !ok || content != text {
		t.Errorf("expected content '%s', got %v", text, result[0].Content)
	}
}

// TestConvertToOpenRouterMessages_ToolUse tests tool_use block conversion to tool_calls
func TestConvertToOpenRouterMessages_ToolUse(t *testing.T) {
	messages := []llmprovider.Message{
		{
			Role: "assistant",
			Blocks: []*llmprovider.Block{
				{
					BlockType: llmprovider.BlockTypeToolUse,
					Sequence:  0,
					Content: map[string]interface{}{
						"tool_use_id": "call_123",
						"tool_name":   "search",
						"input": map[string]interface{}{
							"query": "test query",
						},
					},
				},
			},
		},
	}

	result, err := convertToOpenRouterMessages(messages)
	if err != nil {
		t.Fatalf("error = %v", err)
	}

	if len(result) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result))
	}

	if len(result[0].ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(result[0].ToolCalls))
	}

	toolCall := result[0].ToolCalls[0]
	if toolCall.ID != "call_123" {
		t.Errorf("expected ID 'call_123', got '%s'", toolCall.ID)
	}
	if toolCall.Function.Name != "search" {
		t.Errorf("expected name 'search', got '%s'", toolCall.Function.Name)
	}
}

// TestConvertToOpenRouterMessages_MissingToolUseID tests error handling for missing tool_use_id
func TestConvertToOpenRouterMessages_MissingToolUseID(t *testing.T) {
	messages := []llmprovider.Message{
		{
			Role: "assistant",
			Blocks: []*llmprovider.Block{
				{
					BlockType: llmprovider.BlockTypeToolUse,
					Sequence:  0,
					Content: map[string]interface{}{
						// Missing tool_use_id
						"tool_name": "search",
					},
				},
			},
		},
	}

	_, err := convertToOpenRouterMessages(messages)
	if err == nil {
		t.Error("expected error for missing tool_use_id, got nil")
	}
}

// TestConvertFromChatCompletionResponse tests response conversion
func TestConvertFromChatCompletionResponse(t *testing.T) {
	finishReason := "stop"
	content := "Hello from OpenRouter"

	resp := &ChatCompletionResponse{
		ID:      "chatcmpl-123",
		Model:   "anthropic/claude-3.5-sonnet",
		Created: 1234567890,
		Choices: []Choice{
			{
				Index:        0,
				Message:      Message{Content: content},
				FinishReason: &finishReason,
			},
		},
		Usage: Usage{
			PromptTokens:     10,
			CompletionTokens: 15,
			TotalTokens:      25,
		},
	}

	result, err := convertFromChatCompletionResponse(resp)
	if err != nil {
		t.Fatalf("error = %v", err)
	}

	if len(result.Blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(result.Blocks))
	}

	block := result.Blocks[0]
	if block.BlockType != llmprovider.BlockTypeText {
		t.Errorf("expected BlockTypeText, got %s", block.BlockType)
	}

	if block.TextContent == nil || *block.TextContent != content {
		t.Errorf("expected text content '%s', got %v", content, block.TextContent)
	}

	if result.InputTokens != 10 {
		t.Errorf("expected InputTokens 10, got %d", result.InputTokens)
	}

	if result.OutputTokens != 15 {
		t.Errorf("expected OutputTokens 15, got %d", result.OutputTokens)
	}

	if result.StopReason != "end_turn" {
		t.Errorf("expected StopReason 'end_turn', got '%s'", result.StopReason)
	}
}

// TestMapFinishReason tests finish_reason mapping
func TestMapFinishReason(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"stop", "end_turn"},
		{"length", "max_tokens"},
		{"tool_calls", "tool_use"},
		{"content_filter", "stop_sequence"},
		{"unknown", "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := mapFinishReason(tt.input)
			if result != tt.expected {
				t.Errorf("mapFinishReason(%s) = %s, expected %s", tt.input, result, tt.expected)
			}
		})
	}
}
