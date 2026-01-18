package openrouter

import (
	"testing"
	"time"
)

func TestResponsesStreamState_FindStalledToolArgs_AnyArgsButNeverMeaningful(t *testing.T) {
	timeout := 2 * time.Minute
	now := time.Now()

	state := newResponsesStreamState()
	state.callArgsByCallID["call_1"] = &responsesToolCallAccumulator{
		CallID:      "call_1",
		Name:        "doc_edit",
		seenAnyArgs: true,
		firstArgsAt: now.Add(-timeout - time.Second),
	}

	callID, acc := state.findStalledToolArgs(now, timeout)
	if acc == nil || callID != "call_1" {
		t.Fatalf("got (%q, %#v), want stalled call_1", callID, acc)
	}
}
