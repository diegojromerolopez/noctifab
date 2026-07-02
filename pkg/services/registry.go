package services

import (
	"context"
	"slices"
	"sync"

	"github.com/diegojromerolopez/noctifab/pkg/domain"
)

// Tool represents a single action interface that can be executed by an agent.
type Tool interface {
	// Name returns the unique identifier string of the tool (e.g., "write_file").
	Name() string
	// Description returns the LLM-facing usage documentation of the tool.
	Description() string
	// Execute performs the tool action on the state and workspace with the given arguments.
	Execute(ctx context.Context, state *domain.State, args map[string]any) (string, error)
}

// Registry manages the set of available tools for LLM agent routing.
type Registry interface {
	// Register inserts a tool implementation into the registry.
	Register(t Tool)
	// Get retrieves a registered tool implementation by its unique name.
	Get(name string) (Tool, bool)
	// List returns all registered tools, sorted deterministically by name.
	List() []Tool
}

// ToolRegistry is the default concurrent-safe memory map implementation of Registry.
type ToolRegistry struct {
	mu    sync.RWMutex
	tools map[string]Tool
}

// Compile-time interface compliance check.
var _ Registry = (*ToolRegistry)(nil)

// NewToolRegistry instantiates an empty ToolRegistry.
func NewToolRegistry() *ToolRegistry {
	return &ToolRegistry{
		tools: make(map[string]Tool),
	}
}

// Register adds a tool to the registry.
func (r *ToolRegistry) Register(t Tool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[t.Name()] = t
}

// Get finds a tool by name in the concurrent-safe registry.
func (r *ToolRegistry) Get(name string) (Tool, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, exists := r.tools[name]
	return t, exists
}

// List returns all registered tools sorted alphabetically by name to ensure
// deterministic LLM prompt compilation and test stability.
func (r *ToolRegistry) List() []Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.tools))
	for name := range r.tools {
		names = append(names, name)
	}
	slices.Sort(names)

	list := make([]Tool, 0, len(r.tools))
	for _, name := range names {
		list = append(list, r.tools[name])
	}
	return list
}
