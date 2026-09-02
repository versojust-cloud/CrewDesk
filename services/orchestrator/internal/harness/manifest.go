// Package harness defines CrewDesk's domain-neutral agent runtime contracts.
// It deliberately has no dependency on the virtual-office UI or a specific
// skill pack, so the same runtime can serve research, coding, content, and
// operations workflows.
package harness

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

// ModelPolicy describes how the router should select a model for an agent.
// Tier is the stable policy name; Model may pin a deployment when required.
type ModelPolicy struct {
	Tier  string `json:"tier"`
	Model string `json:"model,omitempty"`
}

// RunLimits bound a single agent activation. Zero-valued optional limits are
// left to the runtime defaults.
type RunLimits struct {
	MaxSteps       int   `json:"max_steps,omitempty"`
	TimeoutSeconds int   `json:"timeout_seconds,omitempty"`
	MaxToolCalls   int   `json:"max_tool_calls,omitempty"`
	MaxCostMicros  int64 `json:"max_cost_micros,omitempty"`
}

// ContextPolicy controls what is exposed when an agent starts or receives a
// handoff. Structured task briefs and artifacts remain available separately.
type ContextPolicy struct {
	IncludeThreadHistory bool `json:"include_thread_history"`
	MaxHistoryItems      int  `json:"max_history_items,omitempty"`
	ShareToolResults     bool `json:"share_tool_results"`
}

// ApprovalMode controls whether side-effecting tools require confirmation.
type ApprovalMode string

const (
	ApprovalNever     ApprovalMode = "never"
	ApprovalOnRequest ApprovalMode = "on_request"
	ApprovalAlways    ApprovalMode = "always"
)

// PermissionPolicy is evaluated before a tool call, not merely described to
// the model. SandboxProfile names a deployment-defined execution profile.
type PermissionPolicy struct {
	ApprovalMode   ApprovalMode `json:"approval_mode"`
	SandboxProfile string       `json:"sandbox_profile,omitempty"`
	AllowNetwork   bool         `json:"allow_network"`
}

// AgentManifest is the server-owned declaration of one executable agent.
// Capabilities are logical names resolved by the tool registry at runtime.
type AgentManifest struct {
	ID           string            `json:"id"`
	DisplayName  string            `json:"display_name"`
	Description  string            `json:"description,omitempty"`
	Priority     int               `json:"priority,omitempty"`
	Instructions string            `json:"instructions"`
	Model        ModelPolicy       `json:"model"`
	Capabilities []string          `json:"capabilities,omitempty"`
	Handoffs     []string          `json:"handoffs,omitempty"`
	Limits       RunLimits         `json:"limits"`
	Context      ContextPolicy     `json:"context"`
	Permissions  PermissionPolicy  `json:"permissions"`
	Metadata     map[string]string `json:"metadata,omitempty"`
}

// Validate rejects manifests that cannot be executed deterministically.
func (m AgentManifest) Validate() error {
	if strings.TrimSpace(m.ID) == "" {
		return errors.New("agent id is required")
	}
	if strings.TrimSpace(m.DisplayName) == "" {
		return fmt.Errorf("agent %q: display name is required", m.ID)
	}
	if strings.TrimSpace(m.Instructions) == "" {
		return fmt.Errorf("agent %q: instructions are required", m.ID)
	}
	if strings.TrimSpace(m.Model.Tier) == "" && strings.TrimSpace(m.Model.Model) == "" {
		return fmt.Errorf("agent %q: model tier or pinned model is required", m.ID)
	}
	if m.Limits.MaxSteps < 0 || m.Limits.TimeoutSeconds < 0 || m.Limits.MaxToolCalls < 0 || m.Limits.MaxCostMicros < 0 {
		return fmt.Errorf("agent %q: run limits cannot be negative", m.ID)
	}
	if m.Context.MaxHistoryItems < 0 {
		return fmt.Errorf("agent %q: max history items cannot be negative", m.ID)
	}
	if err := validateUniqueNames("capability", m.ID, m.Capabilities); err != nil {
		return err
	}
	if err := validateUniqueNames("handoff", m.ID, m.Handoffs); err != nil {
		return err
	}
	if slices.Contains(m.Handoffs, m.ID) {
		return fmt.Errorf("agent %q: cannot hand off to itself", m.ID)
	}
	switch m.Permissions.ApprovalMode {
	case "", ApprovalNever, ApprovalOnRequest, ApprovalAlways:
	default:
		return fmt.Errorf("agent %q: unsupported approval mode %q", m.ID, m.Permissions.ApprovalMode)
	}
	return nil
}

func validateUniqueNames(kind, agentID string, names []string) error {
	seen := make(map[string]struct{}, len(names))
	for _, name := range names {
		trimmed := strings.TrimSpace(name)
		if trimmed == "" {
			return fmt.Errorf("agent %q: %s name cannot be empty", agentID, kind)
		}
		if trimmed != name {
			return fmt.Errorf("agent %q: %s %q has surrounding whitespace", agentID, kind, name)
		}
		if _, ok := seen[trimmed]; ok {
			return fmt.Errorf("agent %q: duplicate %s %q", agentID, kind, name)
		}
		seen[trimmed] = struct{}{}
	}
	return nil
}

// Clone returns a copy safe for callers to mutate.
func (m AgentManifest) Clone() AgentManifest {
	m.Capabilities = slices.Clone(m.Capabilities)
	m.Handoffs = slices.Clone(m.Handoffs)
	m.Metadata = cloneStringMap(m.Metadata)
	return m
}

func cloneStringMap(src map[string]string) map[string]string {
	if src == nil {
		return nil
	}
	dst := make(map[string]string, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
}
