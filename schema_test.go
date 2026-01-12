package llmprovider

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestToolInputSchema_MarshalJSON(t *testing.T) {
	schema := NewToolInputSchema()
	schema.AddProperty("path", PropertySchema{
		Type:        "string",
		Description: "File path",
	}, -1)
	schema.AddProperty("content", PropertySchema{
		Type:        "string",
		Description: "File content",
	}, -1)
	schema.AddRequired("path")

	data, err := json.Marshal(schema)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	// Verify structure
	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if result["type"] != "object" {
		t.Errorf("type = %v, want object", result["type"])
	}

	props, ok := result["properties"].(map[string]interface{})
	if !ok {
		t.Fatalf("properties not a map")
	}
	if len(props) != 2 {
		t.Errorf("properties count = %d, want 2", len(props))
	}

	required, ok := result["required"].([]interface{})
	if !ok {
		t.Fatalf("required not an array")
	}
	if len(required) != 1 || required[0] != "path" {
		t.Errorf("required = %v, want [path]", required)
	}
}

func TestOrderedProperties_MarshalJSON_Order(t *testing.T) {
	// Test that properties are emitted in Order slice order
	props := OrderedProperties{
		Order: []string{"command", "path", "file_text"},
		Values: map[string]PropertySchema{
			"file_text": {Type: "string", Description: "Content"},
			"path":      {Type: "string", Description: "Path"},
			"command":   {Type: "string", Description: "Command"},
		},
	}

	data, err := json.Marshal(props)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	jsonStr := string(data)

	// Verify order: command should appear before path, path before file_text
	cmdIdx := strings.Index(jsonStr, `"command"`)
	pathIdx := strings.Index(jsonStr, `"path"`)
	fileTextIdx := strings.Index(jsonStr, `"file_text"`)

	if cmdIdx < 0 || pathIdx < 0 || fileTextIdx < 0 {
		t.Fatalf("missing expected keys in JSON: %s", jsonStr)
	}

	if cmdIdx > pathIdx {
		t.Errorf("command should appear before path: %s", jsonStr)
	}
	if pathIdx > fileTextIdx {
		t.Errorf("path should appear before file_text: %s", jsonStr)
	}
}

func TestOrderedProperties_MarshalJSON_RemainingAlphabetical(t *testing.T) {
	// Test that keys not in Order are emitted alphabetically
	props := OrderedProperties{
		Order: []string{"first"},
		Values: map[string]PropertySchema{
			"first":  {Type: "string"},
			"zebra":  {Type: "string"},
			"apple":  {Type: "string"},
			"middle": {Type: "string"},
		},
	}

	data, err := json.Marshal(props)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	jsonStr := string(data)

	// first should be... first
	firstIdx := strings.Index(jsonStr, `"first"`)
	appleIdx := strings.Index(jsonStr, `"apple"`)
	middleIdx := strings.Index(jsonStr, `"middle"`)
	zebraIdx := strings.Index(jsonStr, `"zebra"`)

	if firstIdx > appleIdx {
		t.Errorf("first should appear before apple: %s", jsonStr)
	}

	// Remaining should be alphabetical: apple < middle < zebra
	if appleIdx > middleIdx {
		t.Errorf("apple should appear before middle: %s", jsonStr)
	}
	if middleIdx > zebraIdx {
		t.Errorf("middle should appear before zebra: %s", jsonStr)
	}
}

func TestOrderedProperties_MarshalJSON_Empty(t *testing.T) {
	props := OrderedProperties{}

	data, err := json.Marshal(props)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	if string(data) != "{}" {
		t.Errorf("empty properties = %s, want {}", string(data))
	}
}

func TestPropertySchema_MarshalJSON(t *testing.T) {
	min := 1
	max := 100
	prop := PropertySchema{
		Type:        "integer",
		Description: "A number",
		Minimum:     &min,
		Maximum:     &max,
	}

	data, err := json.Marshal(prop)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	if result["type"] != "integer" {
		t.Errorf("type = %v, want integer", result["type"])
	}
	if result["minimum"] != float64(1) {
		t.Errorf("minimum = %v, want 1", result["minimum"])
	}
	if result["maximum"] != float64(100) {
		t.Errorf("maximum = %v, want 100", result["maximum"])
	}
}

func TestPropertySchema_Enum(t *testing.T) {
	prop := PropertySchema{
		Type:        "string",
		Description: "Command to execute",
		Enum:        []string{"create", "update", "delete"},
	}

	data, err := json.Marshal(prop)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}

	enumVals, ok := result["enum"].([]interface{})
	if !ok {
		t.Fatalf("enum not an array")
	}
	if len(enumVals) != 3 {
		t.Errorf("enum count = %d, want 3", len(enumVals))
	}
}

func TestToolInputSchema_AddProperty(t *testing.T) {
	schema := NewToolInputSchema()

	// Add at end (default)
	schema.AddProperty("first", PropertySchema{Type: "string"}, -1)
	schema.AddProperty("second", PropertySchema{Type: "string"}, -1)

	// Add at specific position
	schema.AddProperty("inserted", PropertySchema{Type: "string"}, 1)

	expected := []string{"first", "inserted", "second"}
	if len(schema.Properties.Order) != len(expected) {
		t.Fatalf("order length = %d, want %d", len(schema.Properties.Order), len(expected))
	}
	for i, name := range expected {
		if schema.Properties.Order[i] != name {
			t.Errorf("order[%d] = %s, want %s", i, schema.Properties.Order[i], name)
		}
	}
}

func TestIntPtr(t *testing.T) {
	ptr := IntPtr(42)
	if ptr == nil {
		t.Fatal("IntPtr returned nil")
	}
	if *ptr != 42 {
		t.Errorf("*IntPtr(42) = %d, want 42", *ptr)
	}
}
