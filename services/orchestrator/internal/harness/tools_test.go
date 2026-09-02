package harness

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/dreamwaver/dreamwaver/services/orchestrator/internal/schema"
)

type fakeTool struct{ name string }

func (t fakeTool) Name() string              { return t.name }
func (fakeTool) Description() string         { return "test tool" }
func (fakeTool) Parameters() json.RawMessage { return json.RawMessage(`{"type":"object"}`) }
func (fakeTool) Execute(context.Context, json.RawMessage) (schema.ToolResult, error) {
	return schema.ToolResult{Output: "ok"}, nil
}

type fakeFactory struct {
	name         string
	availability Availability
	policy       ToolPolicy
	buildCount   *int
}

func (f fakeFactory) Name() string { return f.name }
func (f fakeFactory) Available(context.Context, ToolScope) Availability {
	return f.availability
}
func (f fakeFactory) Build(context.Context, ToolScope) (ToolInstance, error) {
	if f.buildCount != nil {
		*f.buildCount++
	}
	return fakeTool{name: f.name}, nil
}
func (f fakeFactory) Policy() ToolPolicy { return f.policy }

func TestToolCatalogBuildsDeclaredAvailableTools(t *testing.T) {
	t.Parallel()

	catalog, err := NewToolCatalog(
		fakeFactory{name: "web_search", availability: Availability{Available: true}, policy: ToolPolicy{RequiresNetwork: true}},
		fakeFactory{name: "code_execute", availability: Availability{Available: false, Reason: "sandbox offline"}},
	)
	if err != nil {
		t.Fatalf("NewToolCatalog() error = %v", err)
	}
	agent := validManifest("worker")
	agent.Capabilities = []string{"web_search", "code_execute", "missing"}
	agent.Permissions.AllowNetwork = true
	instances, resolutions, err := catalog.Build(context.Background(), ToolScope{Agent: agent})
	if err != nil {
		t.Fatalf("BuildRegistry() error = %v", err)
	}
	if !hasTool(instances, "web_search") {
		t.Fatal("expected web_search to be built")
	}
	if hasTool(instances, "code_execute") {
		t.Fatal("did not expect unavailable code_execute")
	}
	if len(resolutions) != 3 || resolutions[2].Availability.Reason != "factory not registered" {
		t.Fatalf("unexpected resolutions: %#v", resolutions)
	}
}

func TestToolCatalogEnforcesNetworkPermission(t *testing.T) {
	t.Parallel()

	catalog, err := NewToolCatalog(fakeFactory{
		name: "web_search", availability: Availability{Available: true}, policy: ToolPolicy{RequiresNetwork: true},
	})
	if err != nil {
		t.Fatalf("NewToolCatalog() error = %v", err)
	}
	agent := validManifest("worker")
	agent.Capabilities = []string{"web_search"}
	instances, resolutions, err := catalog.Build(context.Background(), ToolScope{Agent: agent})
	if err != nil {
		t.Fatalf("BuildRegistry() error = %v", err)
	}
	if hasTool(instances, "web_search") {
		t.Fatal("network tool should be denied")
	}
	if resolutions[0].Availability.Available || resolutions[0].Availability.Reason == "" {
		t.Fatalf("unexpected resolution: %#v", resolutions[0])
	}
}

func TestToolCatalogResolveDoesNotBuildInstances(t *testing.T) {
	t.Parallel()

	built := 0
	catalog, err := NewToolCatalog(fakeFactory{
		name: "web_search", availability: Availability{Available: true}, buildCount: &built,
	})
	if err != nil {
		t.Fatalf("NewToolCatalog() error = %v", err)
	}
	agent := validManifest("worker")
	agent.Capabilities = []string{"web_search", "missing"}

	resolutions, err := catalog.Resolve(context.Background(), ToolScope{Agent: agent})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if built != 0 {
		t.Fatalf("Resolve() built %d instances, want 0", built)
	}
	if len(resolutions) != 2 || !resolutions[0].Availability.Available || resolutions[1].Availability.Available {
		t.Fatalf("unexpected resolutions: %#v", resolutions)
	}
}

func hasTool(instances []ToolInstance, name string) bool {
	for _, instance := range instances {
		if instance.Name() == name {
			return true
		}
	}
	return false
}
