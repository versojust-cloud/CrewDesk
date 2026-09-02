package api

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// ListCrewAgents returns public manifest fields for dynamic clients. System
// instructions and permission internals stay server-side.
func (h *handlers) ListCrewAgents(w http.ResponseWriter, _ *http.Request) {
	if h.deps.Harness == nil || h.deps.Harness.Agents == nil {
		writeJSON(w, http.StatusOK, map[string]any{"agents": []any{}})
		return
	}
	type agentView struct {
		ID           string            `json:"id"`
		DisplayName  string            `json:"display_name"`
		Description  string            `json:"description,omitempty"`
		ModelTier    string            `json:"model_tier,omitempty"`
		Capabilities []string          `json:"capabilities,omitempty"`
		Handoffs     []string          `json:"handoffs,omitempty"`
		Metadata     map[string]string `json:"metadata,omitempty"`
	}
	manifests := h.deps.Harness.Agents.List()
	agents := make([]agentView, 0, len(manifests))
	for _, manifest := range manifests {
		agents = append(agents, agentView{
			ID: manifest.ID, DisplayName: manifest.DisplayName, Description: manifest.Description,
			ModelTier: manifest.Model.Tier, Capabilities: manifest.Capabilities,
			Handoffs: manifest.Handoffs, Metadata: manifest.Metadata,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"agents": agents})
}

// ListHarnessItems pages the authoritative execution stream for replay,
// diagnostics, and evaluation.
//
// GET /api/v1/crew/threads/{id}/items?after=<sequence>&limit=<n>
func (h *handlers) ListHarnessItems(w http.ResponseWriter, r *http.Request) {
	threadID := chi.URLParam(r, "id")
	if threadID == "" {
		errorJSON(w, http.StatusBadRequest, "missing thread id")
		return
	}
	if h.deps.Store == nil || h.deps.Store.HarnessItems == nil {
		writeJSON(w, http.StatusOK, map[string]any{"items": []any{}})
		return
	}

	var after int64
	if raw := r.URL.Query().Get("after"); raw != "" {
		if value, err := strconv.ParseInt(raw, 10, 64); err == nil && value >= 0 {
			after = value
		}
	}
	limit := 500
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if value, err := strconv.Atoi(raw); err == nil && value > 0 {
			limit = value
		}
	}
	workspaceID := workspaceIDFromCtx(r.Context())
	workspaceKey := ""
	if workspaceID != uuid.Nil {
		workspaceKey = workspaceID.String()
	}
	items, err := h.deps.Store.HarnessItems.ListSince(r.Context(), workspaceKey, threadID, after, limit)
	if err != nil {
		errorJSON(w, http.StatusInternalServerError, "list harness items: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}
