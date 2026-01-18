package openrouter

import "testing"

func TestExtractThinkingInfo_WhitespaceOnly_ReturnsNil(t *testing.T) {
	newline := "\n"
	space := "   "

	tests := []struct {
		name    string
		details []ReasoningDetail
	}{
		{
			name: "reasoning.text newline",
			details: []ReasoningDetail{
				{Type: "reasoning.text", Text: &newline},
			},
		},
		{
			name: "reasoning.text spaces",
			details: []ReasoningDetail{
				{Type: "reasoning.text", Text: &space},
			},
		},
		{
			name: "reasoning.summary spaces",
			details: []ReasoningDetail{
				{Type: "reasoning.summary", Summary: &space},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractThinkingInfo(tt.details)
			if got != nil {
				t.Fatalf("expected nil, got %#v", got)
			}
		})
	}
}

func TestExtractThinkingInfo_NonEmpty_ReturnsThinking(t *testing.T) {
	newline := "\n"
	text := "Hello"

	got := extractThinkingInfo([]ReasoningDetail{
		{Type: "reasoning.text", Text: &newline},
		{Type: "reasoning.text", Text: &text},
	})

	if got == nil {
		t.Fatal("expected non-nil thinking info, got nil")
	}
	if got.Text == "" {
		t.Fatal("expected non-empty thinking text, got empty")
	}
}

func TestExtractThinkingInfo_ReasoningSummary_InsertsSeparatorBetweenSummaries(t *testing.T) {
	s1 := "content."
	s2 := "Evaluating tool usage limits"

	got := extractThinkingInfo([]ReasoningDetail{
		{Type: "reasoning.summary", Summary: &s1},
		{Type: "reasoning.summary", Summary: &s2},
	})

	if got == nil {
		t.Fatal("expected non-nil thinking info, got nil")
	}
	if got.Text != "content.\n\nEvaluating tool usage limits" {
		t.Fatalf("got.Text = %q, want %q", got.Text, "content.\\n\\nEvaluating tool usage limits")
	}
}
