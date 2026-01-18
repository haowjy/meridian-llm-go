package streamutil

import (
	"strings"
	"testing"
)

func TestAppendWithOptionalSeparator_EmptyBuilder_NoLeadingSeparator(t *testing.T) {
	var b strings.Builder
	AppendWithOptionalSeparator(&b, "hello", "\n\n")
	if b.String() != "hello" {
		t.Fatalf("got %q, want %q", b.String(), "hello")
	}
}

func TestAppendWithOptionalSeparator_InsertsSeparatorWhenNeeded(t *testing.T) {
	var b strings.Builder
	AppendWithOptionalSeparator(&b, "content.", "\n\n")
	AppendWithOptionalSeparator(&b, "Evaluating tool usage limits", "\n\n")
	if b.String() != "content.\n\nEvaluating tool usage limits" {
		t.Fatalf("got %q, want %q", b.String(), "content.\\n\\nEvaluating tool usage limits")
	}
}

func TestAppendWithOptionalSeparator_DoesNotInsertIfExistingEndsWithWhitespace(t *testing.T) {
	var b strings.Builder
	AppendWithOptionalSeparator(&b, "content.\n", "\n\n")
	AppendWithOptionalSeparator(&b, "Evaluating", "\n\n")
	if b.String() != "content.\nEvaluating" {
		t.Fatalf("got %q, want %q", b.String(), "content.\\nEvaluating")
	}
}

func TestAppendWithOptionalSeparator_DoesNotInsertIfNextStartsWithWhitespace(t *testing.T) {
	var b strings.Builder
	AppendWithOptionalSeparator(&b, "content.", "\n\n")
	AppendWithOptionalSeparator(&b, "\nEvaluating", "\n\n")
	if b.String() != "content.\nEvaluating" {
		t.Fatalf("got %q, want %q", b.String(), "content.\\nEvaluating")
	}
}
