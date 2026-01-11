package llmprovider

import (
	"testing"
)

// Helper to create a text block
func textBlock(text string, seq int) *Block {
	return &Block{
		BlockType:   BlockTypeText,
		Sequence:    seq,
		TextContent: &text,
	}
}

// Helper to create a cross-provider tool_use block
func crossProviderToolBlock(toolName string, originProvider string, seq int) *Block {
	side := ExecutionSideProvider
	return &Block{
		BlockType:     BlockTypeToolUse,
		Sequence:      seq,
		ExecutionSide: &side,
		Provider:      &originProvider,
		Content: map[string]interface{}{
			"tool_use_id": "toolu_123",
			"tool_name":   toolName,
			"input":       map[string]interface{}{},
		},
	}
}

// Helper to create a same-provider tool_use block
func sameProviderToolBlock(toolName string, currentProvider string, seq int) *Block {
	side := ExecutionSideProvider
	return &Block{
		BlockType:     BlockTypeToolUse,
		Sequence:      seq,
		ExecutionSide: &side,
		Provider:      &currentProvider,
		Content: map[string]interface{}{
			"tool_use_id": "toolu_456",
			"tool_name":   toolName,
			"input":       map[string]interface{}{},
		},
	}
}

// Helper to create a backend-side tool_use block (replayable across providers)
func backendToolBlock(toolName string, seq int) *Block {
	side := ExecutionSideServer
	return &Block{
		BlockType:     BlockTypeToolUse,
		Sequence:      seq,
		ExecutionSide: &side,
		Content: map[string]interface{}{
			"tool_use_id": "toolu_789",
			"tool_name":   toolName,
			"input":       map[string]interface{}{},
		},
	}
}

func TestSplitMessagesAtCrossProviderTool(t *testing.T) {
	t.Run("user messages pass through unchanged", func(t *testing.T) {
		messages := []Message{
			{Role: "user", Blocks: []*Block{textBlock("Hello", 0)}},
		}

		result, err := SplitMessagesAtCrossProviderTool(messages, ProviderOpenAI)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(result) != 1 {
			t.Fatalf("expected 1 message, got %d", len(result))
		}
		if result[0].Role != "user" {
			t.Errorf("expected role 'user', got %s", result[0].Role)
		}
	})

	t.Run("assistant messages without cross-provider tools pass through unchanged", func(t *testing.T) {
		messages := []Message{
			{
				Role: "assistant",
				Blocks: []*Block{
					textBlock("Here's my response", 0),
					backendToolBlock("bash", 1), // Backend tool, not provider-side
				},
			},
		}

		result, err := SplitMessagesAtCrossProviderTool(messages, ProviderAnthropic)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(result) != 1 {
			t.Fatalf("expected 1 message, got %d", len(result))
		}
		if len(result[0].Blocks) != 2 {
			t.Errorf("expected 2 blocks, got %d", len(result[0].Blocks))
		}
	})

	t.Run("assistant messages with same-provider tools pass through unchanged", func(t *testing.T) {
		messages := []Message{
			{
				Role: "assistant",
				Blocks: []*Block{
					textBlock("Searching...", 0),
					sameProviderToolBlock("web_search", "anthropic", 1),
					textBlock("Found results", 2),
				},
			},
		}

		result, err := SplitMessagesAtCrossProviderTool(messages, ProviderAnthropic)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(result) != 1 {
			t.Fatalf("expected 1 message, got %d", len(result))
		}
		if len(result[0].Blocks) != 3 {
			t.Errorf("expected 3 blocks, got %d", len(result[0].Blocks))
		}
	})

	t.Run("splits at cross-provider tool with preceding text", func(t *testing.T) {
		messages := []Message{
			{
				Role: "assistant",
				Blocks: []*Block{
					textBlock("Let me search for that", 0),
					crossProviderToolBlock("web_search", "anthropic", 1),
					textBlock("Search results here", 2),
					textBlock("Based on these results...", 3),
				},
			},
		}

		result, err := SplitMessagesAtCrossProviderTool(messages, ProviderOpenAI)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Expected: 4 messages
		// 1. Assistant: "Let me search for that"
		// 2. Assistant: "I used the web_search tool..."
		// 3. User: "Tool results:\n\nSearch results here"
		// 4. Assistant: "Based on these results..."
		if len(result) != 4 {
			t.Fatalf("expected 4 messages, got %d", len(result))
		}

		// First message: original text before tool
		if result[0].Role != "assistant" {
			t.Errorf("msg 0: expected role 'assistant', got %s", result[0].Role)
		}
		if *result[0].Blocks[0].TextContent != "Let me search for that" {
			t.Errorf("msg 0: unexpected text content")
		}

		// Second message: synthetic assistant "I used the X tool"
		if result[1].Role != "assistant" {
			t.Errorf("msg 1: expected role 'assistant', got %s", result[1].Role)
		}
		if result[1].Blocks[0].TextContent == nil {
			t.Errorf("msg 1: expected text content")
		} else if *result[1].Blocks[0].TextContent != "I used the web_search tool to help answer your question." {
			t.Errorf("msg 1: unexpected synthetic text: %s", *result[1].Blocks[0].TextContent)
		}

		// Third message: synthetic user with tool results
		if result[2].Role != "user" {
			t.Errorf("msg 2: expected role 'user', got %s", result[2].Role)
		}

		// Fourth message: remaining assistant blocks
		if result[3].Role != "assistant" {
			t.Errorf("msg 3: expected role 'assistant', got %s", result[3].Role)
		}
		if *result[3].Blocks[0].TextContent != "Based on these results..." {
			t.Errorf("msg 3: unexpected text content")
		}
	})

	t.Run("splits at cross-provider tool without preceding text", func(t *testing.T) {
		messages := []Message{
			{
				Role: "assistant",
				Blocks: []*Block{
					crossProviderToolBlock("web_search", "anthropic", 0),
					textBlock("Search results here", 1),
				},
			},
		}

		result, err := SplitMessagesAtCrossProviderTool(messages, ProviderOpenAI)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Expected: 2 messages (no preceding blocks, so no first assistant message)
		// 1. Assistant: "I used the web_search tool..."
		// 2. User: "Tool results:\n\nSearch results here"
		if len(result) != 2 {
			t.Fatalf("expected 2 messages, got %d", len(result))
		}

		if result[0].Role != "assistant" {
			t.Errorf("msg 0: expected role 'assistant', got %s", result[0].Role)
		}
		if result[1].Role != "user" {
			t.Errorf("msg 1: expected role 'user', got %s", result[1].Role)
		}
	})

	t.Run("handles tool with no result block", func(t *testing.T) {
		messages := []Message{
			{
				Role: "assistant",
				Blocks: []*Block{
					crossProviderToolBlock("web_search", "anthropic", 0),
				},
			},
		}

		result, err := SplitMessagesAtCrossProviderTool(messages, ProviderOpenAI)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// Expected: 2 messages
		// 1. Assistant: "I used the web_search tool..."
		// 2. User: "No results found."
		if len(result) != 2 {
			t.Fatalf("expected 2 messages, got %d", len(result))
		}
	})

	t.Run("falls back to 'search' when tool name is empty", func(t *testing.T) {
		side := ExecutionSideProvider
		originProvider := "anthropic"
		messages := []Message{
			{
				Role: "assistant",
				Blocks: []*Block{
					{
						BlockType:     BlockTypeToolUse,
						Sequence:      0,
						ExecutionSide: &side,
						Provider:      &originProvider,
						Content:       map[string]interface{}{}, // No tool_name
					},
				},
			},
		}

		result, err := SplitMessagesAtCrossProviderTool(messages, ProviderOpenAI)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(result) != 2 {
			t.Fatalf("expected 2 messages, got %d", len(result))
		}

		// Should fallback to "search"
		if *result[0].Blocks[0].TextContent != "I used the search tool to help answer your question." {
			t.Errorf("unexpected fallback text: %s", *result[0].Blocks[0].TextContent)
		}
	})

	t.Run("preserves multiple messages in sequence", func(t *testing.T) {
		messages := []Message{
			{Role: "user", Blocks: []*Block{textBlock("Question 1", 0)}},
			{
				Role: "assistant",
				Blocks: []*Block{
					crossProviderToolBlock("web_search", "anthropic", 0),
					textBlock("Answer 1", 1),
				},
			},
			{Role: "user", Blocks: []*Block{textBlock("Question 2", 0)}},
			{Role: "assistant", Blocks: []*Block{textBlock("Answer 2", 0)}},
		}

		result, err := SplitMessagesAtCrossProviderTool(messages, ProviderOpenAI)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// msg 0: user question 1 (unchanged)
		// msg 1: synthetic assistant "I used..."
		// msg 2: synthetic user tool results (contains "Answer 1" as tool result)
		// msg 3: user question 2 (unchanged)
		// msg 4: assistant answer 2 (unchanged)
		// Note: "Answer 1" is consumed as tool result, no separate assistant message for it
		if len(result) != 5 {
			t.Fatalf("expected 5 messages, got %d", len(result))
		}

		// Verify sequence
		roles := make([]string, len(result))
		for i, m := range result {
			roles[i] = m.Role
		}
		expected := []string{"user", "assistant", "user", "user", "assistant"}
		for i, r := range roles {
			if r != expected[i] {
				t.Errorf("msg %d: expected role %s, got %s", i, expected[i], r)
			}
		}
	})
}

func TestFindToolResultBlocks(t *testing.T) {
	t.Run("finds first text block after tool", func(t *testing.T) {
		blocks := []*Block{
			crossProviderToolBlock("web_search", "anthropic", 0),
			textBlock("Result text", 1),
			textBlock("More text", 2),
		}

		results, consumed := FindToolResultBlocks(blocks, 0)

		if len(results) != 1 {
			t.Fatalf("expected 1 result, got %d", len(results))
		}
		if consumed != 1 {
			t.Errorf("expected consumed=1, got %d", consumed)
		}
		if *results[0].TextContent != "Result text" {
			t.Errorf("unexpected text content")
		}
	})

	t.Run("returns empty when no text block follows", func(t *testing.T) {
		blocks := []*Block{
			crossProviderToolBlock("web_search", "anthropic", 0),
			backendToolBlock("bash", 1), // Not a text block
		}

		results, consumed := FindToolResultBlocks(blocks, 0)

		if len(results) != 0 {
			t.Errorf("expected 0 results, got %d", len(results))
		}
		if consumed != 0 {
			t.Errorf("expected consumed=0, got %d", consumed)
		}
	})

	t.Run("returns empty when tool is last block", func(t *testing.T) {
		blocks := []*Block{
			textBlock("Some text", 0),
			crossProviderToolBlock("web_search", "anthropic", 1),
		}

		results, consumed := FindToolResultBlocks(blocks, 1)

		if len(results) != 0 {
			t.Errorf("expected 0 results, got %d", len(results))
		}
		if consumed != 0 {
			t.Errorf("expected consumed=0, got %d", consumed)
		}
	})

	t.Run("handles index out of bounds gracefully", func(t *testing.T) {
		blocks := []*Block{
			textBlock("Only block", 0),
		}

		results, consumed := FindToolResultBlocks(blocks, 5)

		if len(results) != 0 {
			t.Errorf("expected 0 results, got %d", len(results))
		}
		if consumed != 0 {
			t.Errorf("expected consumed=0, got %d", consumed)
		}
	})
}

func TestFormatToolResults(t *testing.T) {
	t.Run("empty blocks returns 'No results found'", func(t *testing.T) {
		result := FormatToolResults([]*Block{})

		if result != "No results found." {
			t.Errorf("expected 'No results found.', got %s", result)
		}
	})

	t.Run("formats single block", func(t *testing.T) {
		blocks := []*Block{
			textBlock("Search result content", 0),
		}

		result := FormatToolResults(blocks)

		expected := "Tool results:\n\nSearch result content"
		if result != expected {
			t.Errorf("expected %q, got %q", expected, result)
		}
	})

	t.Run("formats multiple blocks", func(t *testing.T) {
		blocks := []*Block{
			textBlock("Result 1", 0),
			textBlock("Result 2", 1),
		}

		result := FormatToolResults(blocks)

		expected := "Tool results:\n\nResult 1\n\nResult 2"
		if result != expected {
			t.Errorf("expected %q, got %q", expected, result)
		}
	})

	t.Run("skips blocks without text content", func(t *testing.T) {
		blocks := []*Block{
			textBlock("Has text", 0),
			{BlockType: BlockTypeToolUse, Sequence: 1}, // No TextContent
		}

		result := FormatToolResults(blocks)

		expected := "Tool results:\n\nHas text"
		if result != expected {
			t.Errorf("expected %q, got %q", expected, result)
		}
	})
}
