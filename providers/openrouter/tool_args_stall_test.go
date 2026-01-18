package openrouter

import (
	"testing"
	"time"
)

func TestFindStalledToolArgs_AnyArgsButNeverMeaningful_TimesOutFromFirstArgs(t *testing.T) {
	timeout := 2 * time.Minute
	now := time.Now()

	acc := &accumulatedToolCall{
		ID:          "call_1",
		Name:        "doc_edit",
		seenAnyArgs: true,
		firstArgsAt: now.Add(-timeout - time.Second),
	}

	m := map[int]*accumulatedToolCall{0: acc}

	got := findStalledToolArgs(m, now, timeout)
	if got == nil || got.ID != "call_1" {
		t.Fatalf("got %#v, want stalled call_1", got)
	}
}

func TestFindStalledToolArgs_MeaningfulArgsThenNoProgress_TimesOutFromLastMeaningful(t *testing.T) {
	timeout := 2 * time.Minute
	now := time.Now()

	acc := &accumulatedToolCall{
		ID:                   "call_1",
		Name:                 "doc_edit",
		seenAnyArgs:          true,
		firstArgsAt:          now.Add(-timeout),
		seenMeaningfulArgs:   true,
		lastMeaningfulArgsAt: now.Add(-timeout - time.Second),
	}

	m := map[int]*accumulatedToolCall{0: acc}

	got := findStalledToolArgs(m, now, timeout)
	if got == nil || got.ID != "call_1" {
		t.Fatalf("got %#v, want stalled call_1", got)
	}
}

func TestFindStalledToolArgs_AnyArgsRecent_NoStall(t *testing.T) {
	timeout := 2 * time.Minute
	now := time.Now()

	acc := &accumulatedToolCall{
		ID:          "call_1",
		Name:        "doc_edit",
		seenAnyArgs: true,
		firstArgsAt: now.Add(-time.Second),
	}

	m := map[int]*accumulatedToolCall{0: acc}

	got := findStalledToolArgs(m, now, timeout)
	if got != nil {
		t.Fatalf("got %#v, want nil", got)
	}
}
