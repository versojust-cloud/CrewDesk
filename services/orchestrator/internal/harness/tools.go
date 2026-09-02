package harness

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sync"

	"github.com/dreamwaver/dreamwaver/services/orchestrator/internal/schema"
)

type Availability struct {
	Available bool   `json:"available"`
	Reason    string `json:"reason,omitempty"`
}

type ToolPolicy struct {
	SideEffect      bool   `json:"side_effect"`
	Destructive     bool   `json:"destructive"`
	RequiresNetwork bool   `json:"requires_network"`
	ApprovalDefault bool   `json:"approval_default"`
	SandboxProfile  string `json:"sandbox_profile,omitempty"`
}

type ToolScope struct {
	Run          RunScope
	Agent        AgentManifest
	Journal      ItemJournal
	Dependencies map[string]any
}

// ToolInstance mirrors the executable contract used by CrewDesk's current
// agent loop while keeping the Harness independent from its legacy registry.
type ToolInstance interface {
	Name() string
	Description() string
	Parameters() json.RawMessage
	Execute(ctx context.Context, args json.RawMessage) (schema.ToolResult, error)
}

// ToolFactory constructs one scoped tool and reports whether its backing
// service is currently usable. Factories are registered once at boot.
type ToolFactory interface {
	Name() string
	Available(ctx context.Context, scope ToolScope) Availability
	Build(ctx context.Context, scope ToolScope) (ToolInstance, error)
	Policy() ToolPolicy
}

type ToolResolution struct {
	Name         string       `json:"name"`
	Availability Availability `json:"availability"`
	Policy       ToolPolicy   `json:"policy"`
}

type ToolCatalog struct {
	mu        sync.RWMutex
	factories map[string]ToolFactory
}

func NewToolCatalog(factories ...ToolFactory) (*ToolCatalog, error) {
	catalog := &ToolCatalog{factories: make(map[string]ToolFactory, len(factories))}
	for _, factory := range factories {
		if err := catalog.Register(factory); err != nil {
			return nil, err
		}
	}
	return catalog, nil
}

func (c *ToolCatalog) Register(factory ToolFactory) error {
	if factory == nil || factory.Name() == "" {
		return errors.New("harness: tool factory and name are required")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, exists := c.factories[factory.Name()]; exists {
		return fmt.Errorf("harness: tool factory %q is already registered", factory.Name())
	}
	c.factories[factory.Name()] = factory
	return nil
}

// BuildRegistry resolves only capabilities declared by the manifest. Missing
// or disabled tools are returned in the resolution report, not silently added.
func (c *ToolCatalog) Build(ctx context.Context, scope ToolScope) ([]ToolInstance, []ToolResolution, error) {
	if err := scope.Agent.Validate(); err != nil {
		return nil, nil, err
	}
	c.mu.RLock()
	factories := make(map[string]ToolFactory, len(c.factories))
	for name, factory := range c.factories {
		factories[name] = factory
	}
	c.mu.RUnlock()

	built := make([]ToolInstance, 0, len(scope.Agent.Capabilities))
	resolutions := make([]ToolResolution, 0, len(scope.Agent.Capabilities))
	for _, capability := range scope.Agent.Capabilities {
		factory, ok := factories[capability]
		if !ok {
			resolutions = append(resolutions, ToolResolution{
				Name:         capability,
				Availability: Availability{Available: false, Reason: "factory not registered"},
			})
			continue
		}
		policy := factory.Policy()
		availability := factory.Available(ctx, scope)
		if policy.RequiresNetwork && !scope.Agent.Permissions.AllowNetwork {
			availability = Availability{Available: false, Reason: "agent network permission denied"}
		}
		resolution := ToolResolution{Name: capability, Availability: availability, Policy: policy}
		resolutions = append(resolutions, resolution)
		if !availability.Available {
			continue
		}
		instance, err := factory.Build(ctx, scope)
		if err != nil {
			return nil, resolutions, fmt.Errorf("build tool %q: %w", capability, err)
		}
		if instance == nil || instance.Name() != capability {
			return nil, resolutions, fmt.Errorf("build tool %q: factory returned mismatched tool", capability)
		}
		built = append(built, instance)
	}
	return slices.Clone(built), slices.Clone(resolutions), nil
}

// Dependency returns a typed runtime dependency without requiring every
// factory to define its own unchecked type assertion.
func Dependency[T any](scope ToolScope, name string) (T, bool) {
	var zero T
	value, ok := scope.Dependencies[name]
	if !ok {
		return zero, false
	}
	typed, ok := value.(T)
	return typed, ok
}
