package llmprovider

import "testing"

func TestBlock_IsUserBlock(t *testing.T) {
	tests := []struct {
		name     string
		block    *Block
		expected bool
	}{
		{
			name: "user block",
			block: &Block{
				BlockType: BlockTypeText,
			},
			expected: true,
		},
		{
			name: "assistant text block",
			block: &Block{
				BlockType: BlockTypeText,
			},
			expected: true, // BlockTypeText can be both user and assistant
		},
		{
			name: "thinking block",
			block: &Block{
				BlockType: BlockTypeThinking,
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Note: IsUserBlock() checks if block CAN BE a user block
			// It returns true for text blocks
			result := tt.block.IsUserBlock()
			if result != tt.expected {
				t.Errorf("IsUserBlock() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestBlock_IsAssistantBlock(t *testing.T) {
	tests := []struct {
		name     string
		block    *Block
		expected bool
	}{
		{
			name: "text block",
			block: &Block{
				BlockType: BlockTypeText,
			},
			expected: true,
		},
		{
			name: "thinking block",
			block: &Block{
				BlockType: BlockTypeThinking,
			},
			expected: true,
		},
		{
			name: "tool_use block",
			block: &Block{
				BlockType: BlockTypeToolUse,
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.block.IsAssistantBlock()
			if result != tt.expected {
				t.Errorf("IsAssistantBlock() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestBlockTypes_Constants(t *testing.T) {
	// Verify block type constants are defined correctly
	expectedTypes := map[string]string{
		"BlockTypeText":       "text",
		"BlockTypeThinking":   "thinking",
		"BlockTypeToolUse":    "tool_use",
		"BlockTypeToolResult": "tool_result",
		"BlockTypeImage":      "image",
		"BlockTypeDocument":   "document",
	}

	actualTypes := map[string]string{
		"BlockTypeText":       BlockTypeText,
		"BlockTypeThinking":   BlockTypeThinking,
		"BlockTypeToolUse":    BlockTypeToolUse,
		"BlockTypeToolResult": BlockTypeToolResult,
		"BlockTypeImage":      BlockTypeImage,
		"BlockTypeDocument":   BlockTypeDocument,
	}

	for name, expected := range expectedTypes {
		actual, ok := actualTypes[name]
		if !ok {
			t.Errorf("constant %s not found", name)
			continue
		}
		if actual != expected {
			t.Errorf("%s = %q, want %q", name, actual, expected)
		}
	}
}

