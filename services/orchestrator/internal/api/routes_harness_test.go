package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/dreamwaver/dreamwaver/services/orchestrator/internal/harness"
)

type discoveryFactory struct{}

func (discoveryFactory) Name() string { return "search" }
func (discoveryFactory) Available(context.Context, harness.ToolScope) harness.Availability {
	return harness.Availability{Available: false, Reason: "provider not configured"}
}
func (discoveryFactory) Build(context.Context, harness.ToolScope) (harness.ToolInstance, error) {
	return nil, nil
}
func (discoveryFactory) Policy() harness.ToolPolicy {
	return harness.ToolPolicy{RequiresNetwork: true}
}

func TestListCrewAgentsIncludesToolAvailabilityWithoutPrivatePrompt(t *testing.T) {
	t.Parallel()

	agents, err := harness.NewRegistry(harness.AgentManifest{
		ID:           "researcher",
		DisplayName:  "Researcher",
		Instructions: "private system prompt",
		Model:        harness.ModelPolicy{Tier: "worker"},
		Capabilities: []string{"search"},
		Permissions:  harness.PermissionPolicy{AllowNetwork: true},
	})
	if err != nil {
		t.Fatalf("NewRegistry() error = %v", err)
	}
	tools, err := harness.NewToolCatalog(discoveryFactory{})
	if err != nil {
		t.Fatalf("NewToolCatalog() error = %v", err)
	}
	runtime, err := harness.NewRuntime(agents, tools, harness.NewMemoryJournal())
	if err != nil {
		t.Fatalf("NewRuntime() error = %v", err)
	}
	handler := handlers{deps: Dependencies{Harness: runtime}}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/crew/agents", nil)

	handler.ListCrewAgents(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", recorder.Code, recorder.Body.String())
	}
	var response struct {
		Data struct {
			Agents []struct {
				ID    string                   `json:"id"`
				Tools []harness.ToolResolution `json:"tools"`
			} `json:"agents"`
		} `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(response.Data.Agents) != 1 || len(response.Data.Agents[0].Tools) != 1 {
		t.Fatalf("unexpected response: %s", recorder.Body.String())
	}
	tool := response.Data.Agents[0].Tools[0]
	if tool.Availability.Available || tool.Availability.Reason == "" || !tool.Policy.RequiresNetwork {
		t.Fatalf("unexpected tool resolution: %#v", tool)
	}
	if containsJSONField(recorder.Body.Bytes(), "instructions") || containsJSONField(recorder.Body.Bytes(), "permissions") {
		t.Fatal("private agent configuration leaked into discovery response")
	}
}

func containsJSONField(body []byte, field string) bool {
	var value any
	if json.Unmarshal(body, &value) != nil {
		return false
	}
	return findJSONField(value, field)
}

func findJSONField(value any, field string) bool {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if key == field || findJSONField(child, field) {
				return true
			}
		}
	case []any:
		for _, child := range typed {
			if findJSONField(child, field) {
				return true
			}
		}
	}
	return false
}
