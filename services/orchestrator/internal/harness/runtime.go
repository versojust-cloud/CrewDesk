package harness

import "errors"

// Runtime groups the registries and journal shared by every skill pack.
// Orchestration policies operate on this object instead of skill-specific
// Runner fields.
type Runtime struct {
	Agents *Registry
	Tools  *ToolCatalog
	Items  ItemJournal
	Graph  *GraphExecutor
}

func NewRuntime(agents *Registry, tools *ToolCatalog, items ItemJournal) (*Runtime, error) {
	if agents == nil {
		return nil, errors.New("harness: agent registry is required")
	}
	if tools == nil {
		return nil, errors.New("harness: tool catalog is required")
	}
	if items == nil {
		return nil, errors.New("harness: item journal is required")
	}
	return &Runtime{
		Agents: agents,
		Tools:  tools,
		Items:  items,
		Graph:  NewGraphExecutor(items, 4),
	}, nil
}
