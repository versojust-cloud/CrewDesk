package harness

import (
	"fmt"
	"sort"
	"sync"
)

// Registry owns the manifests available to one CrewDesk deployment.
// Registration order is preserved for deterministic planning and UI output.
type Registry struct {
	mu     sync.RWMutex
	agents map[string]AgentManifest
	order  []string
}

func NewRegistry(manifests ...AgentManifest) (*Registry, error) {
	r := &Registry{agents: make(map[string]AgentManifest, len(manifests))}
	for _, manifest := range manifests {
		if err := r.Register(manifest); err != nil {
			return nil, err
		}
	}
	return r, nil
}

// Register adds an agent and rejects duplicate IDs.
func (r *Registry) Register(manifest AgentManifest) error {
	if err := manifest.Validate(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.agents[manifest.ID]; exists {
		return fmt.Errorf("agent %q is already registered", manifest.ID)
	}
	r.agents[manifest.ID] = manifest.Clone()
	r.order = append(r.order, manifest.ID)
	return nil
}

// Upsert replaces a manifest without changing its display order, or appends
// it when the ID is new. It is intended for deployment-time configuration.
func (r *Registry) Upsert(manifest AgentManifest) error {
	if err := manifest.Validate(); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.agents[manifest.ID]; !exists {
		r.order = append(r.order, manifest.ID)
	}
	r.agents[manifest.ID] = manifest.Clone()
	return nil
}

func (r *Registry) Get(id string) (AgentManifest, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	manifest, ok := r.agents[id]
	return manifest.Clone(), ok
}

func (r *Registry) List() []AgentManifest {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]AgentManifest, 0, len(r.order))
	for _, id := range r.order {
		out = append(out, r.agents[id].Clone())
	}
	return out
}

// ResolveCapability returns agents that declare a capability. Higher
// priority metadata wins; IDs break ties to keep selection deterministic.
func (r *Registry) ResolveCapability(capability string) []AgentManifest {
	r.mu.RLock()
	defer r.mu.RUnlock()
	var out []AgentManifest
	for _, id := range r.order {
		manifest := r.agents[id]
		for _, name := range manifest.Capabilities {
			if name == capability {
				out = append(out, manifest.Clone())
				break
			}
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Priority > out[j].Priority
	})
	return out
}

// CanHandoff verifies the source declaration and target existence.
func (r *Registry) CanHandoff(fromID, toID string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	from, sourceExists := r.agents[fromID]
	_, targetExists := r.agents[toID]
	if !sourceExists || !targetExists {
		return false
	}
	for _, id := range from.Handoffs {
		if id == toID {
			return true
		}
	}
	return false
}
