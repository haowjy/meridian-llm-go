package llmprovider

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
)

// ToolInputSchema represents a JSON Schema for tool parameters.
// This typed struct eliminates the []string vs []interface{} bugs that occur
// when using map[string]interface{} with native Go types.
type ToolInputSchema struct {
	Type       string            `json:"type"`                // Always "object"
	Properties OrderedProperties `json:"properties"`          // Ordered for streaming UX
	Required   []string          `json:"required,omitempty"`
}

// NewToolInputSchema creates a new ToolInputSchema with type "object".
func NewToolInputSchema() ToolInputSchema {
	return ToolInputSchema{
		Type: "object",
		Properties: OrderedProperties{
			Values: make(map[string]PropertySchema),
		},
	}
}

// AddProperty adds a property to the schema.
// If atPosition >= 0, the property is inserted at that position in the Order slice.
// If atPosition < 0, the property is appended to the Order slice.
func (s *ToolInputSchema) AddProperty(name string, prop PropertySchema, atPosition int) {
	if s.Properties.Values == nil {
		s.Properties.Values = make(map[string]PropertySchema)
	}
	s.Properties.Values[name] = prop

	if atPosition < 0 {
		s.Properties.Order = append(s.Properties.Order, name)
	} else {
		// Insert at position
		if atPosition > len(s.Properties.Order) {
			atPosition = len(s.Properties.Order)
		}
		s.Properties.Order = append(s.Properties.Order[:atPosition],
			append([]string{name}, s.Properties.Order[atPosition:]...)...)
	}
}

// AddRequired adds a field to the required list.
func (s *ToolInputSchema) AddRequired(name string) {
	s.Required = append(s.Required, name)
}

// OrderedProperties maintains property order for JSON marshaling.
// This is critical for streaming UX: some LLMs stream tool input keys following
// the schema's property order. For example, doc_edit needs "path" to appear
// before "file_text" so the UI can show the destination early while content streams.
type OrderedProperties struct {
	Order  []string                   // Explicit order for these keys
	Values map[string]PropertySchema  // The actual property definitions
}

// MarshalJSON emits properties in Order slice order, then remaining keys alphabetically.
func (o OrderedProperties) MarshalJSON() ([]byte, error) {
	if len(o.Values) == 0 {
		return []byte("{}"), nil
	}

	seen := make(map[string]bool, len(o.Values))
	var buf bytes.Buffer
	buf.WriteByte('{')

	first := true
	writeKV := func(k string, v PropertySchema) error {
		keyBytes, err := json.Marshal(k)
		if err != nil {
			return fmt.Errorf("marshal key: %w", err)
		}
		valBytes, err := json.Marshal(v)
		if err != nil {
			return fmt.Errorf("marshal value for %q: %w", k, err)
		}

		if !first {
			buf.WriteByte(',')
		}
		first = false
		buf.Write(keyBytes)
		buf.WriteByte(':')
		buf.Write(valBytes)
		return nil
	}

	// 1) Emit configured keys in order (if present in Values).
	for _, k := range o.Order {
		v, ok := o.Values[k]
		if !ok {
			continue
		}
		if err := writeKV(k, v); err != nil {
			return nil, err
		}
		seen[k] = true
	}

	// 2) Emit remaining keys in lexicographic order (deterministic).
	remaining := make([]string, 0, len(o.Values))
	for k := range o.Values {
		if !seen[k] {
			remaining = append(remaining, k)
		}
	}
	sort.Strings(remaining)
	for _, k := range remaining {
		if err := writeKV(k, o.Values[k]); err != nil {
			return nil, err
		}
	}

	buf.WriteByte('}')
	return buf.Bytes(), nil
}

// PropertySchema represents a single property definition in a JSON Schema.
// Common fields are typed; extend as needed for additional JSON Schema keywords.
type PropertySchema struct {
	Type        string   `json:"type"`
	Description string   `json:"description,omitempty"`
	Enum        []string `json:"enum,omitempty"`
	Minimum     *int     `json:"minimum,omitempty"`
	Maximum     *int     `json:"maximum,omitempty"`
}

// IntPtr returns a pointer to an int (helper for Minimum/Maximum fields).
func IntPtr(i int) *int {
	return &i
}
