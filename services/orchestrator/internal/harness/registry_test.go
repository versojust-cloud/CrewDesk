package harness

import "testing"

func validManifest(id string) AgentManifest {
	return AgentManifest{
		ID:           id,
		DisplayName:  id,
		Instructions: "Complete the assigned task.",
		Model:        ModelPolicy{Tier: "worker"},
	}
}

func TestManifestValidate(t *testing.T) {
	t.Parallel()

	manifest := validManifest("researcher")
	manifest.Capabilities = []string{"web_search", "web_search"}
	if err := manifest.Validate(); err == nil {
		t.Fatal("expected duplicate capability to fail validation")
	}

	manifest = validManifest("researcher")
	manifest.Handoffs = []string{"researcher"}
	if err := manifest.Validate(); err == nil {
		t.Fatal("expected self handoff to fail validation")
	}
}

func TestRegistryPreservesOrderAndClones(t *testing.T) {
	t.Parallel()

	first := validManifest("coordinator")
	first.Capabilities = []string{"plan_tasks"}
	second := validManifest("researcher")
	second.Capabilities = []string{"web_search"}

	registry, err := NewRegistry(first, second)
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}
	got := registry.List()
	if len(got) != 2 || got[0].ID != "coordinator" || got[1].ID != "researcher" {
		t.Fatalf("unexpected registry order: %#v", got)
	}

	got[0].Capabilities[0] = "changed"
	again, ok := registry.Get("coordinator")
	if !ok || again.Capabilities[0] != "plan_tasks" {
		t.Fatalf("registry leaked mutable manifest: %#v", again)
	}
}

func TestRegistryCapabilityAndHandoff(t *testing.T) {
	t.Parallel()

	manager := validManifest("manager")
	manager.Handoffs = []string{"researcher"}
	researcher := validManifest("researcher")
	researcher.Capabilities = []string{"web_search"}
	researcher.Priority = 100
	backup := validManifest("backup")
	backup.Capabilities = []string{"web_search"}
	backup.Priority = 10

	registry, err := NewRegistry(manager, backup, researcher)
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}
	resolved := registry.ResolveCapability("web_search")
	if len(resolved) != 2 || resolved[0].ID != "researcher" {
		t.Fatalf("unexpected capability resolution: %#v", resolved)
	}
	if !registry.CanHandoff("manager", "researcher") {
		t.Fatal("expected declared handoff to be allowed")
	}
	if registry.CanHandoff("manager", "missing") {
		t.Fatal("expected handoff to missing target to be denied")
	}
}
