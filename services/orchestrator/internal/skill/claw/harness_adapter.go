package claw

import (
	"slices"

	"github.com/dreamwaver/dreamwaver/services/orchestrator/internal/harness"
)

// HarnessManifests exposes the existing Claw role pack through CrewDesk's
// generic runtime contract. The current coordinator can keep using Role while
// the new runtime and API are introduced incrementally.
func HarnessManifests() []harness.AgentManifest {
	roles := Roles()
	manifests := make([]harness.AgentManifest, 0, len(roles))
	for _, role := range roles {
		manifests = append(manifests, harness.AgentManifest{
			ID:           role.Key,
			DisplayName:  role.DisplayName,
			Description:  roleDescription(role.Key),
			Instructions: role.SystemPrompt,
			Model:        harness.ModelPolicy{Tier: role.ModelTier},
			Capabilities: slices.Clone(role.ToolNames),
			Handoffs:     roleHandoffs(role.Key),
			Limits: harness.RunLimits{
				MaxSteps:       role.MaxSteps,
				TimeoutSeconds: roleTimeoutSeconds(role.Key),
			},
			Context: harness.ContextPolicy{
				IncludeThreadHistory: role.Key == RoleCoordinator || role.Key == RoleWriter || role.Key == RoleCritic,
				MaxHistoryItems:      40,
				ShareToolResults:     true,
			},
			Permissions: harness.PermissionPolicy{
				ApprovalMode:   harness.ApprovalOnRequest,
				SandboxProfile: roleSandboxProfile(role.Key),
				AllowNetwork:   role.Key == RoleResearcher,
			},
			Metadata: map[string]string{
				"pack":        "claw",
				"ui_surface":  "virtual_office",
				"legacy_role": role.Key,
			},
		})
	}
	return manifests
}

func roleTimeoutSeconds(role string) int {
	switch role {
	case RoleProducer:
		return 8 * 60
	case RoleDesigner:
		return 6 * 60
	default:
		return 4 * 60
	}
}

func roleDescription(role string) string {
	switch role {
	case RoleCoordinator:
		return "Decomposes goals, builds task graphs, and coordinates execution."
	case RoleResearcher:
		return "Finds current sources, facts, and market evidence."
	case RoleEngineer:
		return "Executes code, analyzes data, and produces verifiable technical results."
	case RoleDesigner:
		return "Creates and edits visual assets."
	case RoleWriter:
		return "Synthesizes task outputs into a coherent deliverable."
	case RoleCritic:
		return "Evaluates deliverables against a rubric and corrects quality issues."
	case RoleProducer:
		return "Packages results as presentations or interactive deliverables."
	case RoleVideographer:
		return "Creates short video assets from approved visual inputs."
	default:
		return "Executes an assigned CrewDesk task."
	}
}

func roleHandoffs(role string) []string {
	switch role {
	case RoleCoordinator:
		return []string{RoleResearcher, RoleEngineer, RoleDesigner, RoleWriter, RoleProducer, RoleVideographer}
	case RoleResearcher, RoleEngineer, RoleDesigner:
		return []string{RoleWriter}
	case RoleWriter:
		return []string{RoleCritic, RoleProducer}
	case RoleCritic:
		return []string{RoleWriter, RoleProducer}
	default:
		return nil
	}
}

func roleSandboxProfile(role string) string {
	if role == RoleEngineer {
		return "isolated-code"
	}
	return "tool-only"
}
