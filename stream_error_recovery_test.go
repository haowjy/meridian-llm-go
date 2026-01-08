package llmprovider

import (
	"errors"
	"testing"
)

func TestNewMalformedToolRecovery(t *testing.T) {
	providerStr := "openrouter"

	t.Run("creates both blocks with correct content", func(t *testing.T) {
		recovery, nextSeq := NewMalformedToolRecovery(
			"tool_123",
			"doc_edit",
			`{"command": "str_replace", "path": "/test.md`, // malformed - missing closing brace
			errors.New("unexpected end of JSON input"),
			10,
			&providerStr,
		)

		// Verify tool_use block
		if recovery.ToolUseBlock.BlockType != BlockTypeToolUse {
			t.Errorf("ToolUseBlock.BlockType = %v, want %v", recovery.ToolUseBlock.BlockType, BlockTypeToolUse)
		}
		if recovery.ToolUseBlock.Sequence != 10 {
			t.Errorf("ToolUseBlock.Sequence = %v, want 10", recovery.ToolUseBlock.Sequence)
		}
		if recovery.ToolUseBlock.Content["tool_use_id"] != "tool_123" {
			t.Errorf("ToolUseBlock tool_use_id = %v, want tool_123", recovery.ToolUseBlock.Content["tool_use_id"])
		}
		if recovery.ToolUseBlock.Content["tool_name"] != "doc_edit" {
			t.Errorf("ToolUseBlock tool_name = %v, want doc_edit", recovery.ToolUseBlock.Content["tool_name"])
		}
		// Check _malformed key exists in input
		inputMap, ok := recovery.ToolUseBlock.Content["input"].(map[string]interface{})
		if !ok {
			t.Fatal("ToolUseBlock input is not a map")
		}
		if _, exists := inputMap["_malformed"]; !exists {
			t.Error("ToolUseBlock input should have _malformed key")
		}

		// Verify tool_result block
		if recovery.ToolResultBlock.BlockType != BlockTypeToolResult {
			t.Errorf("ToolResultBlock.BlockType = %v, want %v", recovery.ToolResultBlock.BlockType, BlockTypeToolResult)
		}
		if recovery.ToolResultBlock.Sequence != 11 {
			t.Errorf("ToolResultBlock.Sequence = %v, want 11", recovery.ToolResultBlock.Sequence)
		}
		if recovery.ToolResultBlock.Content["is_error"] != true {
			t.Error("ToolResultBlock should have is_error: true")
		}
		errorStr, _ := recovery.ToolResultBlock.Content["error"].(string)
		if errorStr == "" {
			t.Error("ToolResultBlock should have error message")
		}

		// Verify next sequence
		if nextSeq != 12 {
			t.Errorf("nextSeq = %v, want 12", nextSeq)
		}
	})

	t.Run("truncates long input", func(t *testing.T) {
		// Create input longer than 500 chars
		longInput := ""
		for i := 0; i < 600; i++ {
			longInput += "a"
		}

		recovery, _ := NewMalformedToolRecovery(
			"tool_123",
			"doc_edit",
			longInput,
			errors.New("parse error"),
			0,
			nil,
		)

		inputMap := recovery.ToolUseBlock.Content["input"].(map[string]interface{})
		malformedStr := inputMap["_malformed"].(string)

		// Should be truncated to 500 + "..."
		if len(malformedStr) != 503 {
			t.Errorf("malformed string length = %v, want 503 (500 + ...)", len(malformedStr))
		}
		if malformedStr[500:] != "..." {
			t.Error("truncated string should end with ...")
		}
	})
}

func TestTruncateString(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		maxLen   int
		expected string
	}{
		{
			name:     "short string unchanged",
			input:    "hello",
			maxLen:   10,
			expected: "hello",
		},
		{
			name:     "exact length unchanged",
			input:    "hello",
			maxLen:   5,
			expected: "hello",
		},
		{
			name:     "long string truncated",
			input:    "hello world",
			maxLen:   5,
			expected: "hello...",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := truncateString(tt.input, tt.maxLen)
			if result != tt.expected {
				t.Errorf("truncateString(%q, %d) = %q, want %q", tt.input, tt.maxLen, result, tt.expected)
			}
		})
	}
}
