package claw

import (
	"testing"

	"github.com/dreamwaver/dreamwaver/services/orchestrator/internal/harness"
)

func TestHarnessManifestsAreValidAndAligned(t *testing.T) {
	t.Parallel()

	manifests := HarnessManifests()
	roles := Roles()
	if len(manifests) != len(roles) {
		t.Fatalf("got %d manifests for %d roles", len(manifests), len(roles))
	}
	for i, manifest := range manifests {
		if err := manifest.Validate(); err != nil {
			t.Fatalf("manifest %q is invalid: %v", manifest.ID, err)
		}
		if manifest.ID != roles[i].Key {
			t.Fatalf("manifest %d id = %q, want %q", i, manifest.ID, roles[i].Key)
		}
	}
	if _, err := harness.NewRegistry(manifests...); err != nil {
		t.Fatalf("NewRegistry(HarnessManifests()) error = %v", err)
	}
}
