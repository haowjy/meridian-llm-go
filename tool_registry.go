package llmprovider

import (
	"fmt"
	"sync"
)

// ToolDefinition describes how to create a tool
type ToolDefinition struct {
	Name        string           // Unique tool name
	Description string           // Human-readable description
	Factory     func() (*Tool, error) // Factory function to create tool
}

// ToolRegistry manages runtime registration of custom tools
// This allows library users to register their own tool types beyond the built-in ones
type ToolRegistry struct {
	tools map[string]ToolDefinition
	mu    sync.RWMutex
}

var (
	globalToolRegistry     *ToolRegistry
	globalToolRegistryOnce sync.Once
)

// GetToolRegistry returns the global tool registry (singleton)
func GetToolRegistry() *ToolRegistry {
	globalToolRegistryOnce.Do(func() {
		globalToolRegistry = &ToolRegistry{
			tools: make(map[string]ToolDefinition),
		}
		// Register built-in tools
		globalToolRegistry.registerBuiltInTools()
	})
	return globalToolRegistry
}

// registerBuiltInTools registers the built-in tool types
func (r *ToolRegistry) registerBuiltInTools() {
	// Register search tool
	_ = r.Register(ToolDefinition{
		Name:        ToolTypeSearch,
		Description: "Web search tool (server-executed)",
		Factory:     NewSearchTool,
	})

	// Register text editor tool
	_ = r.Register(ToolDefinition{
		Name:        ToolTypeTextEditor,
		Description: "Text editor tool (client-executed)",
		Factory:     NewTextEditorTool,
	})

	// Register bash tool
	_ = r.Register(ToolDefinition{
		Name:        ToolTypeBash,
		Description: "Bash command execution tool (client-executed)",
		Factory:     NewBashTool,
	})
}

// Register adds a tool definition to the registry
func (r *ToolRegistry) Register(def ToolDefinition) error {
	if def.Name == "" {
		return fmt.Errorf("tool name is required")
	}

	if def.Factory == nil {
		return fmt.Errorf("factory function is required for tool %s", def.Name)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.tools[def.Name]; exists {
		return fmt.Errorf("tool %s is already registered", def.Name)
	}

	r.tools[def.Name] = def
	return nil
}

// Unregister removes a tool definition from the registry
// This is useful for testing or replacing tool implementations
func (r *ToolRegistry) Unregister(name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.tools[name]; !exists {
		return fmt.Errorf("tool %s is not registered", name)
	}

	delete(r.tools, name)
	return nil
}

// Get retrieves a tool definition by name
func (r *ToolRegistry) Get(name string) (ToolDefinition, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	def, exists := r.tools[name]
	if !exists {
		return ToolDefinition{}, fmt.Errorf("unknown tool: %s", name)
	}

	return def, nil
}

// IsRegistered checks if a tool is registered
func (r *ToolRegistry) IsRegistered(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	_, exists := r.tools[name]
	return exists
}

// List returns all registered tool names
func (r *ToolRegistry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.tools))
	for name := range r.tools {
		names = append(names, name)
	}
	return names
}

// Create creates a tool instance using the registered factory
func (r *ToolRegistry) Create(name string) (*Tool, error) {
	def, err := r.Get(name)
	if err != nil {
		return nil, err
	}

	return def.Factory()
}

// RegisterTool is a convenience function that registers a tool with the global registry
func RegisterTool(def ToolDefinition) error {
	return GetToolRegistry().Register(def)
}

// CreateTool is a convenience function that creates a tool using the global registry
func CreateTool(name string) (*Tool, error) {
	return GetToolRegistry().Create(name)
}
