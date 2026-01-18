package openrouter

import (
	"encoding/json"
	"io"
	"log/slog"
	"testing"

	llmprovider "github.com/haowjy/meridian-llm-go"
)

// ===== Responses API Type Tests =====

func TestResponsesSSEEvent_ParseTextStream(t *testing.T) {
	// Test parsing response.created event
	createdJSON := `{
		"type": "response.created",
		"response": {
			"id": "resp_123",
			"object": "response",
			"model": "anthropic/claude-3-sonnet",
			"status": "in_progress"
		}
	}`

	var event ResponsesSSEEvent
	if err := json.Unmarshal([]byte(createdJSON), &event); err != nil {
		t.Fatalf("failed to parse response.created: %v", err)
	}

	if event.Type != ResponsesEventResponseCreated {
		t.Errorf("expected type %q, got %q", ResponsesEventResponseCreated, event.Type)
	}
	if event.Response == nil {
		t.Fatal("expected response object, got nil")
	}
	if event.Response.ID != "resp_123" {
		t.Errorf("expected response ID %q, got %q", "resp_123", event.Response.ID)
	}
}

func TestResponsesSSEEvent_ParseOutputItemAdded(t *testing.T) {
	tests := []struct {
		name     string
		json     string
		itemType string
		itemID   string
	}{
		{
			name: "message item",
			json: `{
				"type": "response.output_item.added",
				"output_index": 0,
				"item": {
					"type": "message",
					"id": "msg_001",
					"role": "assistant",
					"status": "in_progress"
				}
			}`,
			itemType: "message",
			itemID:   "msg_001",
		},
		{
			name: "function_call item",
			json: `{
				"type": "response.output_item.added",
				"output_index": 1,
				"item": {
					"type": "function_call",
					"id": "fc_001",
					"call_id": "call_abc123",
					"name": "get_weather",
					"status": "in_progress"
				}
			}`,
			itemType: "function_call",
			itemID:   "fc_001",
		},
		{
			name: "reasoning item",
			json: `{
				"type": "response.output_item.added",
				"output_index": 0,
				"item": {
					"type": "reasoning",
					"id": "rs_001",
					"status": "in_progress"
				}
			}`,
			itemType: "reasoning",
			itemID:   "rs_001",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var event ResponsesSSEEvent
			if err := json.Unmarshal([]byte(tt.json), &event); err != nil {
				t.Fatalf("failed to parse: %v", err)
			}

			if event.Type != ResponsesEventOutputItemAdded {
				t.Errorf("expected type %q, got %q", ResponsesEventOutputItemAdded, event.Type)
			}
			if event.Item == nil {
				t.Fatal("expected item, got nil")
			}
			if event.Item.Type != tt.itemType {
				t.Errorf("expected item type %q, got %q", tt.itemType, event.Item.Type)
			}
			if event.Item.ID != tt.itemID {
				t.Errorf("expected item ID %q, got %q", tt.itemID, event.Item.ID)
			}
		})
	}
}

func TestResponsesSSEEvent_ParseFunctionCallArgsDelta(t *testing.T) {
	jsonStr := `{
		"type": "response.function_call_arguments.delta",
		"output_index": 1,
		"item_id": "fc_001",
		"arguments": "{\"city\":"
	}`

	var event ResponsesSSEEvent
	if err := json.Unmarshal([]byte(jsonStr), &event); err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	if event.Type != ResponsesEventFunctionCallArgsDelta {
		t.Errorf("expected type %q, got %q", ResponsesEventFunctionCallArgsDelta, event.Type)
	}
	if event.Arguments != `{"city":` {
		t.Errorf("expected arguments %q, got %q", `{"city":`, event.Arguments)
	}
	if event.ItemID != "fc_001" {
		t.Errorf("expected item_id %q, got %q", "fc_001", event.ItemID)
	}
}

func TestResponsesSSEEvent_ParseContentPartDelta(t *testing.T) {
	jsonStr := `{
		"type": "response.content_part.delta",
		"output_index": 0,
		"content_index": 0,
		"delta": "Hello, "
	}`

	var event ResponsesSSEEvent
	if err := json.Unmarshal([]byte(jsonStr), &event); err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	if event.Type != ResponsesEventContentPartDelta {
		t.Errorf("expected type %q, got %q", ResponsesEventContentPartDelta, event.Type)
	}
	if event.Delta != "Hello, " {
		t.Errorf("expected delta %q, got %q", "Hello, ", event.Delta)
	}
}

// ===== Responses API Adapter Tests =====

func TestConvertToResponsesInput_SimpleText(t *testing.T) {
	provider := &Provider{logger: testResponsesLogger()}

	text := "Hello, world!"
	messages := []llmprovider.Message{
		{
			Role: "user",
			Blocks: []*llmprovider.Block{
				{
					BlockType:   llmprovider.BlockTypeText,
					TextContent: &text,
				},
			},
		},
	}

	inputs, err := provider.convertToResponsesInput(messages)
	if err != nil {
		t.Fatalf("conversion failed: %v", err)
	}

	if len(inputs) != 1 {
		t.Fatalf("expected 1 input, got %d", len(inputs))
	}

	input := inputs[0]
	if input.Type != "message" {
		t.Errorf("expected type %q, got %q", "message", input.Type)
	}
	if input.Role != "user" {
		t.Errorf("expected role %q, got %q", "user", input.Role)
	}
	if len(input.Content) != 1 {
		t.Fatalf("expected 1 content part, got %d", len(input.Content))
	}
	if input.Content[0].Type != "input_text" {
		t.Errorf("expected content type %q, got %q", "input_text", input.Content[0].Type)
	}
	if input.Content[0].Text != "Hello, world!" {
		t.Errorf("expected text %q, got %q", "Hello, world!", input.Content[0].Text)
	}
}

func TestConvertToResponsesInput_ToolResult(t *testing.T) {
	provider := &Provider{logger: testResponsesLogger()}

	resultText := `{"temp": 72}`
	messages := []llmprovider.Message{
		{
			Role: "tool",
			Blocks: []*llmprovider.Block{
				{
					BlockType:   llmprovider.BlockTypeToolResult,
					TextContent: &resultText,
					Content: map[string]interface{}{
						"tool_use_id": "call_abc123",
					},
				},
			},
		},
	}

	inputs, err := provider.convertToResponsesInput(messages)
	if err != nil {
		t.Fatalf("conversion failed: %v", err)
	}

	if len(inputs) != 1 {
		t.Fatalf("expected 1 input, got %d", len(inputs))
	}

	input := inputs[0]
	if input.Type != "function_call_output" {
		t.Errorf("expected type %q, got %q", "function_call_output", input.Type)
	}
	if input.CallID != "call_abc123" {
		t.Errorf("expected call_id %q, got %q", "call_abc123", input.CallID)
	}
	if input.Output != `{"temp": 72}` {
		t.Errorf("expected output %q, got %q", `{"temp": 72}`, input.Output)
	}
}

func TestConvertToResponsesTools(t *testing.T) {
	provider := &Provider{logger: testResponsesLogger()}

	schema := llmprovider.NewToolInputSchema()
	schema.AddProperty("query", llmprovider.PropertySchema{
		Type:        "string",
		Description: "The search query",
	}, -1)
	schema.AddRequired("query")

	tools := []llmprovider.Tool{
		{
			Type: "function",
			Function: llmprovider.FunctionDetails{
				Name:        "web_search",
				Description: "Search the web",
				Parameters:  schema,
			},
		},
	}

	responsesTools, err := provider.convertToResponsesTools(tools)
	if err != nil {
		t.Fatalf("conversion failed: %v", err)
	}

	if len(responsesTools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(responsesTools))
	}

	tool := responsesTools[0]
	if tool.Type != "function" {
		t.Errorf("expected type %q, got %q", "function", tool.Type)
	}
	if tool.Name != "web_search" {
		t.Errorf("expected name %q, got %q", "web_search", tool.Name)
	}
	if tool.Description != "Search the web" {
		t.Errorf("expected description %q, got %q", "Search the web", tool.Description)
	}

	// Check parameters structure
	params := tool.Parameters
	if params == nil {
		t.Fatal("expected parameters, got nil")
	}
	if params["type"] != "object" {
		t.Errorf("expected parameters type %q, got %v", "object", params["type"])
	}
}

// ===== Responses API Streaming State Tests =====

func TestResponsesStreamState_TrackItems(t *testing.T) {
	state := newResponsesStreamState()

	// Add message item
	msgItem := &OutputItem{
		Type:   "message",
		ID:     "msg_001",
		Role:   "assistant",
		Status: "in_progress",
	}
	state.outputItemByID["msg_001"] = msgItem
	state.activeMessageItemID = "msg_001"

	// Add function_call item
	fcItem := &OutputItem{
		Type:   "function_call",
		ID:     "fc_001",
		CallID: "call_abc123",
		Name:   "get_weather",
		Status: "in_progress",
	}
	state.outputItemByID["fc_001"] = fcItem
	state.callArgsByCallID["call_abc123"] = &responsesToolCallAccumulator{
		ItemID: "fc_001",
		CallID: "call_abc123",
		Name:   "get_weather",
	}

	// Verify tracking
	if state.activeMessageItemID != "msg_001" {
		t.Errorf("expected active message ID %q, got %q", "msg_001", state.activeMessageItemID)
	}

	if _, ok := state.outputItemByID["msg_001"]; !ok {
		t.Error("expected message item to be tracked")
	}

	if acc, ok := state.callArgsByCallID["call_abc123"]; !ok {
		t.Error("expected tool call to be tracked")
	} else {
		if acc.Name != "get_weather" {
			t.Errorf("expected tool name %q, got %q", "get_weather", acc.Name)
		}
	}
}

func TestResponsesStreamState_AccumulateToolArgs(t *testing.T) {
	state := newResponsesStreamState()

	// Create accumulator
	state.callArgsByCallID["call_abc123"] = &responsesToolCallAccumulator{
		ItemID: "fc_001",
		CallID: "call_abc123",
		Name:   "get_weather",
	}

	// Simulate argument streaming
	acc := state.callArgsByCallID["call_abc123"]
	acc.Arguments += `{"city":`
	acc.Arguments += `"San Francisco"`
	acc.Arguments += `}`

	// Verify accumulation
	expected := `{"city":"San Francisco"}`
	if acc.Arguments != expected {
		t.Errorf("expected arguments %q, got %q", expected, acc.Arguments)
	}
}

// ===== Helper Functions =====

// testLogger returns a no-op logger for testing
func testResponsesLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
