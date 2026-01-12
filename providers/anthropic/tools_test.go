package anthropic

import (
	"encoding/json"
	"testing"

	llmprovider "github.com/haowjy/meridian-llm-go"
)

func findRequiredInInputSchema(t *testing.T, v any) []any {
	t.Helper()

	switch node := v.(type) {
	case map[string]any:
		// Direct hit
		if schema, ok := node["input_schema"].(map[string]any); ok {
			if req, ok := schema["required"].([]any); ok {
				return req
			}
		}

		// Recurse
		for _, child := range node {
			if req := findRequiredInInputSchema(t, child); req != nil {
				return req
			}
		}
	case []any:
		for _, child := range node {
			if req := findRequiredInInputSchema(t, child); req != nil {
				return req
			}
		}
	}

	return nil
}

func TestConvertCustomTool_RequiredCopiedFromStringSlice(t *testing.T) {
	// Build schema using typed ToolInputSchema
	schema := llmprovider.NewToolInputSchema()
	schema.AddProperty("command", llmprovider.PropertySchema{Type: "string"}, -1)
	schema.AddProperty("path", llmprovider.PropertySchema{Type: "string"}, -1)
	schema.AddRequired("command")
	schema.AddRequired("path")

	tool := llmprovider.Tool{
		Type: "function",
		Function: llmprovider.FunctionDetails{
			Name:        "doc_edit",
			Description: "test tool",
			Parameters:  schema,
		},
	}

	converted, err := convertCustomTool(&tool)
	if err != nil {
		t.Fatalf("convertCustomTool() error = %v", err)
	}

	raw, err := json.Marshal(converted)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	var payload any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}

	required := findRequiredInInputSchema(t, payload)
	if required == nil {
		t.Fatalf("expected required in input_schema, got none. payload=%s", string(raw))
	}

	seen := map[string]bool{}
	for _, v := range required {
		if s, ok := v.(string); ok {
			seen[s] = true
		}
	}

	if !seen["command"] || !seen["path"] {
		t.Fatalf("expected required to include command+path, got %v. payload=%s", required, string(raw))
	}
}

