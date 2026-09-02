package harness

import "context"

// RunScope identifies one activation throughout model, tool, event, and
// persistence calls. A Thread contains turns; a turn may contain many tasks
// and agent activations.
type RunScope struct {
	WorkspaceID  string `json:"workspace_id,omitempty"`
	ThreadID     string `json:"thread_id"`
	TurnID       string `json:"turn_id"`
	TaskID       string `json:"task_id,omitempty"`
	AgentID      string `json:"agent_id,omitempty"`
	ActivationID string `json:"activation_id,omitempty"`
}

type runScopeKey struct{}

func WithRunScope(ctx context.Context, scope RunScope) context.Context {
	return context.WithValue(ctx, runScopeKey{}, scope)
}

func RunScopeFromContext(ctx context.Context) (RunScope, bool) {
	scope, ok := ctx.Value(runScopeKey{}).(RunScope)
	return scope, ok && scope.ThreadID != "" && scope.TurnID != ""
}

func WithAgent(ctx context.Context, agentID, activationID string) context.Context {
	scope, _ := ctx.Value(runScopeKey{}).(RunScope)
	scope.AgentID = agentID
	scope.ActivationID = activationID
	return context.WithValue(ctx, runScopeKey{}, scope)
}

func WithTask(ctx context.Context, taskID string) context.Context {
	scope, _ := ctx.Value(runScopeKey{}).(RunScope)
	scope.TaskID = taskID
	return context.WithValue(ctx, runScopeKey{}, scope)
}
