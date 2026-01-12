package llmprovider

import (
	"sync"
	"testing"
)

// newTestRegistry creates a fresh registry for testing (not the global singleton)
func newTestRegistry() *ToolRegistry {
	return &ToolRegistry{
		tools: make(map[string]ToolDefinition),
	}
}

// mockToolFactory creates a simple mock tool factory for testing
func mockToolFactory(name string) func() (*Tool, error) {
	return func() (*Tool, error) {
		return &Tool{
			Type: "function",
			Function: FunctionDetails{
				Name:        name,
				Description: "Mock tool: " + name,
				Parameters:  NewToolInputSchema(),
			},
		}, nil
	}
}

func TestToolRegistry_Register(t *testing.T) {
	t.Run("successful registration", func(t *testing.T) {
		registry := newTestRegistry()

		err := registry.Register(ToolDefinition{
			Name:        "test_tool",
			Description: "A test tool",
			Factory:     mockToolFactory("test_tool"),
		})

		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		if !registry.IsRegistered("test_tool") {
			t.Error("expected tool to be registered")
		}
	})

	t.Run("empty name error", func(t *testing.T) {
		registry := newTestRegistry()

		err := registry.Register(ToolDefinition{
			Name:    "",
			Factory: mockToolFactory(""),
		})

		if err == nil {
			t.Error("expected error for empty name")
		}
	})

	t.Run("nil factory error", func(t *testing.T) {
		registry := newTestRegistry()

		err := registry.Register(ToolDefinition{
			Name:    "test_tool",
			Factory: nil,
		})

		if err == nil {
			t.Error("expected error for nil factory")
		}
	})

	t.Run("duplicate registration error", func(t *testing.T) {
		registry := newTestRegistry()

		def := ToolDefinition{
			Name:    "duplicate_tool",
			Factory: mockToolFactory("duplicate_tool"),
		}

		// First registration should succeed
		err := registry.Register(def)
		if err != nil {
			t.Fatalf("first registration failed: %v", err)
		}

		// Second registration should fail
		err = registry.Register(def)
		if err == nil {
			t.Error("expected error for duplicate registration")
		}
	})
}

func TestToolRegistry_Unregister(t *testing.T) {
	t.Run("successful unregister", func(t *testing.T) {
		registry := newTestRegistry()

		_ = registry.Register(ToolDefinition{
			Name:    "to_remove",
			Factory: mockToolFactory("to_remove"),
		})

		err := registry.Unregister("to_remove")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}

		if registry.IsRegistered("to_remove") {
			t.Error("expected tool to be unregistered")
		}
	})

	t.Run("unregister non-existent tool", func(t *testing.T) {
		registry := newTestRegistry()

		err := registry.Unregister("non_existent")
		if err == nil {
			t.Error("expected error for non-existent tool")
		}
	})
}

func TestToolRegistry_Get(t *testing.T) {
	t.Run("get existing tool", func(t *testing.T) {
		registry := newTestRegistry()

		_ = registry.Register(ToolDefinition{
			Name:        "my_tool",
			Description: "My custom tool",
			Factory:     mockToolFactory("my_tool"),
		})

		def, err := registry.Get("my_tool")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if def.Name != "my_tool" {
			t.Errorf("expected name 'my_tool', got %s", def.Name)
		}
		if def.Description != "My custom tool" {
			t.Errorf("expected description 'My custom tool', got %s", def.Description)
		}
	})

	t.Run("get non-existent tool", func(t *testing.T) {
		registry := newTestRegistry()

		_, err := registry.Get("unknown")
		if err == nil {
			t.Error("expected error for unknown tool")
		}
	})
}

func TestToolRegistry_IsRegistered(t *testing.T) {
	registry := newTestRegistry()

	_ = registry.Register(ToolDefinition{
		Name:    "registered_tool",
		Factory: mockToolFactory("registered_tool"),
	})

	if !registry.IsRegistered("registered_tool") {
		t.Error("expected IsRegistered to return true for registered tool")
	}

	if registry.IsRegistered("not_registered") {
		t.Error("expected IsRegistered to return false for unregistered tool")
	}
}

func TestToolRegistry_List(t *testing.T) {
	registry := newTestRegistry()

	// Register multiple tools
	for _, name := range []string{"tool_a", "tool_b", "tool_c"} {
		n := name // capture loop variable
		_ = registry.Register(ToolDefinition{
			Name:    n,
			Factory: mockToolFactory(n),
		})
	}

	names := registry.List()
	if len(names) != 3 {
		t.Errorf("expected 3 tools, got %d", len(names))
	}

	// Check all names are present (order not guaranteed)
	nameSet := make(map[string]bool)
	for _, n := range names {
		nameSet[n] = true
	}

	for _, expected := range []string{"tool_a", "tool_b", "tool_c"} {
		if !nameSet[expected] {
			t.Errorf("expected %s in list", expected)
		}
	}
}

func TestToolRegistry_Create(t *testing.T) {
	t.Run("create existing tool", func(t *testing.T) {
		registry := newTestRegistry()

		_ = registry.Register(ToolDefinition{
			Name:        "creatable",
			Description: "A creatable tool definition",
			Factory:     mockToolFactory("creatable"),
		})

		tool, err := registry.Create("creatable")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if tool.Function.Name != "creatable" {
			t.Errorf("expected function name 'creatable', got %s", tool.Function.Name)
		}
		if tool.Type != "function" {
			t.Errorf("expected type 'function', got %s", tool.Type)
		}
	})

	t.Run("create non-existent tool", func(t *testing.T) {
		registry := newTestRegistry()

		_, err := registry.Create("unknown")
		if err == nil {
			t.Error("expected error for unknown tool")
		}
	})
}

func TestToolRegistry_ConcurrentAccess(t *testing.T) {
	registry := newTestRegistry()
	var wg sync.WaitGroup
	numGoroutines := 100

	// Concurrent registrations (each with unique name)
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			name := string(rune('a'+id%26)) + string(rune('0'+id/26))
			_ = registry.Register(ToolDefinition{
				Name:    name,
				Factory: mockToolFactory(name),
			})
		}(i)
	}

	// Concurrent reads
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = registry.List()
		}()
	}

	wg.Wait()

	// Should not panic and should have some registered tools
	names := registry.List()
	if len(names) == 0 {
		t.Error("expected some tools to be registered")
	}
}

func TestGetToolRegistry_Singleton(t *testing.T) {
	// GetToolRegistry should always return the same instance
	registry1 := GetToolRegistry()
	registry2 := GetToolRegistry()

	if registry1 != registry2 {
		t.Error("expected GetToolRegistry to return the same instance")
	}

	// Should have built-in tools registered
	if !registry1.IsRegistered(ToolTypeSearch) {
		t.Error("expected search tool to be registered")
	}
	if !registry1.IsRegistered(ToolTypeTextEditor) {
		t.Error("expected text_editor tool to be registered")
	}
	if !registry1.IsRegistered(ToolTypeBash) {
		t.Error("expected bash tool to be registered")
	}
}
