package claw

import (
	"context"
	"testing"

	"github.com/dreamwaver/dreamwaver/services/orchestrator/internal/event"
	"github.com/dreamwaver/dreamwaver/services/orchestrator/internal/harness"
)

func TestRegisterHarnessToolsReportsRuntimeAvailability(t *testing.T) {
	t.Parallel()

	runner := &Runner{TavilyKey: "configured", Emitter: event.NoopEmitter{}}
	catalog, err := harness.NewToolCatalog()
	if err != nil {
		t.Fatalf("NewToolCatalog() error = %v", err)
	}
	if err := RegisterHarnessTools(catalog, runner); err != nil {
		t.Fatalf("RegisterHarnessTools() error = %v", err)
	}
	agent := manifestByID(t, RoleResearcher)
	agent.Capabilities = []string{"web_search", "code_execute"}

	resolutions, err := catalog.Resolve(context.Background(), harness.ToolScope{Agent: agent})
	if err != nil {
		t.Fatalf("Resolve() error = %v", err)
	}
	if len(resolutions) != 2 {
		t.Fatalf("Resolve() returned %d entries, want 2", len(resolutions))
	}
	if !resolutions[0].Availability.Available {
		t.Fatalf("web_search unavailable: %#v", resolutions[0])
	}
	if resolutions[1].Availability.Available || resolutions[1].Availability.Reason == "" {
		t.Fatalf("code_execute should report its missing backend: %#v", resolutions[1])
	}
}

func TestHarnessToolFactoryBuildsSessionScopedTool(t *testing.T) {
	t.Parallel()

	runner := &Runner{Emitter: event.NoopEmitter{}}
	catalog, err := harness.NewToolCatalog()
	if err != nil {
		t.Fatalf("NewToolCatalog() error = %v", err)
	}
	if err := RegisterHarnessTools(catalog, runner); err != nil {
		t.Fatalf("RegisterHarnessTools() error = %v", err)
	}
	agent := manifestByID(t, RoleWriter)
	agent.Capabilities = []string{"write_document"}
	scope := harness.ToolScope{
		Agent: agent,
		Dependencies: map[string]any{
			clawSessionDependency: &Session{},
		},
	}

	instances, resolutions, err := catalog.Build(context.Background(), scope)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if len(instances) != 1 || instances[0].Name() != "write_document" {
		t.Fatalf("unexpected instances: %#v", instances)
	}
	if len(resolutions) != 1 || !resolutions[0].Availability.Available {
		t.Fatalf("unexpected resolutions: %#v", resolutions)
	}
}

func manifestByID(t *testing.T, id string) harness.AgentManifest {
	t.Helper()
	for _, manifest := range HarnessManifests() {
		if manifest.ID == id {
			return manifest
		}
	}
	t.Fatalf("manifest %q not found", id)
	return harness.AgentManifest{}
}
