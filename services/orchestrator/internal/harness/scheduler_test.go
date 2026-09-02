package harness

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
)

func TestValidateTaskGraphRejectsMissingDependencyAndCycle(t *testing.T) {
	t.Parallel()

	missing := []TaskSpec{{ID: "write", Goal: "write", Dependencies: []string{"research"}}}
	if err := ValidateTaskGraph(missing); !errors.Is(err, ErrInvalidTaskGraph) {
		t.Fatalf("missing dependency error = %v", err)
	}
	cycle := []TaskSpec{
		{ID: "a", Goal: "a", Dependencies: []string{"b"}},
		{ID: "b", Goal: "b", Dependencies: []string{"a"}},
	}
	if err := ValidateTaskGraph(cycle); !errors.Is(err, ErrInvalidTaskGraph) {
		t.Fatalf("cycle error = %v", err)
	}
}

func TestGraphExecutorPassesDependencyResultsAndRecordsLifecycle(t *testing.T) {
	t.Parallel()

	journal := NewMemoryJournal()
	executor := NewGraphExecutor(journal, 2)
	scope := RunScope{WorkspaceID: "workspace", ThreadID: "thread", TurnID: "turn"}
	specs := []TaskSpec{
		{ID: "research", Goal: "research", AssigneeID: "researcher"},
		{ID: "write", Goal: "write", AssigneeID: "writer", Dependencies: []string{"research"}},
	}
	handler := TaskHandlerFunc(func(_ context.Context, invocation TaskInvocation) (TaskResult, error) {
		if invocation.Task.Spec.ID == "write" {
			dependency, ok := invocation.Dependencies["research"]
			if !ok || string(dependency.Output) != `"facts"` {
				t.Fatalf("write dependency = %#v", invocation.Dependencies)
			}
			return TaskResult{Output: json.RawMessage(`"report"`)}, nil
		}
		return TaskResult{Output: json.RawMessage(`"facts"`)}, nil
	})

	result, err := executor.Execute(context.Background(), scope, specs, handler)
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if result.Status != TurnCompleted || len(result.Tasks) != 2 {
		t.Fatalf("unexpected graph result: %#v", result)
	}
	for _, task := range result.Tasks {
		if task.Status != TaskCompleted || task.Attempt != 1 {
			t.Fatalf("unexpected task: %#v", task)
		}
	}
	items, err := journal.ListSince(context.Background(), "workspace", "thread", 0, 20)
	if err != nil {
		t.Fatalf("ListSince() error = %v", err)
	}
	if len(items) != 4 {
		t.Fatalf("journal items = %d, want 4: %#v", len(items), items)
	}
	if items[0].Status != ItemStarted || items[1].Status != ItemCompleted || items[2].Status != ItemStarted || items[3].Status != ItemCompleted {
		t.Fatalf("unexpected lifecycle: %#v", items)
	}
}

func TestGraphExecutorRetriesTransientFailure(t *testing.T) {
	t.Parallel()

	journal := NewMemoryJournal()
	executor := NewGraphExecutor(journal, 1)
	scope := RunScope{ThreadID: "thread", TurnID: "turn"}
	attempts := 0
	specs := []TaskSpec{{
		ID: "flaky", Goal: "retry", Retry: RetryPolicy{MaxAttempts: 2},
	}}
	result, err := executor.Execute(context.Background(), scope, specs, TaskHandlerFunc(func(_ context.Context, _ TaskInvocation) (TaskResult, error) {
		attempts++
		if attempts == 1 {
			return TaskResult{}, errors.New("temporary")
		}
		return TaskResult{Output: json.RawMessage(`"ok"`)}, nil
	}))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if attempts != 2 || result.Tasks[0].Status != TaskCompleted || result.Tasks[0].Attempt != 2 {
		t.Fatalf("unexpected retry result: attempts=%d result=%#v", attempts, result)
	}
	items, _ := journal.ListSince(context.Background(), "", "thread", 0, 10)
	if len(items) != 3 || items[1].Status != ItemStreaming || items[2].Status != ItemCompleted {
		t.Fatalf("unexpected retry lifecycle: %#v", items)
	}
}

func TestGraphExecutorSkipsDependentsButRunsIndependentBranch(t *testing.T) {
	t.Parallel()

	executor := NewGraphExecutor(NewMemoryJournal(), 2)
	scope := RunScope{ThreadID: "thread", TurnID: "turn"}
	specs := []TaskSpec{
		{ID: "failed", Goal: "fail"},
		{ID: "blocked", Goal: "blocked", Dependencies: []string{"failed"}},
		{ID: "independent", Goal: "continue"},
	}
	var mu sync.Mutex
	executed := map[string]bool{}
	result, err := executor.Execute(context.Background(), scope, specs, TaskHandlerFunc(func(_ context.Context, invocation TaskInvocation) (TaskResult, error) {
		mu.Lock()
		executed[invocation.Task.Spec.ID] = true
		mu.Unlock()
		if invocation.Task.Spec.ID == "failed" {
			return TaskResult{}, errors.New("failed")
		}
		return TaskResult{}, nil
	}))
	if err != nil {
		t.Fatalf("Execute() infrastructure error = %v", err)
	}
	if result.Status != TurnFailed {
		t.Fatalf("graph status = %q, want failed", result.Status)
	}
	statuses := taskStatuses(result.Tasks)
	if statuses["failed"] != TaskFailed || statuses["blocked"] != TaskSkipped || statuses["independent"] != TaskCompleted {
		t.Fatalf("unexpected statuses: %#v", statuses)
	}
	if executed["blocked"] || !executed["independent"] {
		t.Fatalf("unexpected executed tasks: %#v", executed)
	}
}

func TestGraphExecutorCancelsActiveAndQueuedTasks(t *testing.T) {
	t.Parallel()

	journal := NewMemoryJournal()
	executor := NewGraphExecutor(journal, 1)
	scope := RunScope{ThreadID: "thread", TurnID: "turn"}
	specs := []TaskSpec{
		{ID: "active", Goal: "wait"},
		{ID: "queued", Goal: "never starts", Dependencies: []string{"active"}},
	}
	ctx, cancel := context.WithCancel(context.Background())
	started := make(chan struct{})
	done := make(chan struct{})
	var result GraphResult
	var runErr error
	go func() {
		result, runErr = executor.Execute(ctx, scope, specs, TaskHandlerFunc(func(ctx context.Context, _ TaskInvocation) (TaskResult, error) {
			close(started)
			<-ctx.Done()
			return TaskResult{}, ctx.Err()
		}))
		close(done)
	}()
	<-started
	cancel()
	<-done

	if !errors.Is(runErr, context.Canceled) || result.Status != TurnCancelled {
		t.Fatalf("cancel result = %#v, error = %v", result, runErr)
	}
	statuses := taskStatuses(result.Tasks)
	if statuses["active"] != TaskCancelled || statuses["queued"] != TaskCancelled {
		t.Fatalf("unexpected statuses: %#v", statuses)
	}
	for _, id := range []string{"active", "queued"} {
		item, err := journal.Latest(context.Background(), "", "thread", "turn:task:"+id)
		if err != nil || item.Status != ItemCancelled {
			t.Fatalf("latest item %q = %#v, %v", id, item, err)
		}
	}
}

func taskStatuses(tasks []Task) map[string]TaskStatus {
	out := make(map[string]TaskStatus, len(tasks))
	for _, task := range tasks {
		out[task.Spec.ID] = task.Status
	}
	return out
}
