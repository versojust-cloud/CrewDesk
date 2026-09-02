package harness

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"
)

var ErrInvalidTaskGraph = errors.New("harness: invalid task graph")

// TaskResult is the addressable output of one graph node. Dependent handlers
// receive cloned results so concurrent execution cannot mutate shared state.
type TaskResult struct {
	Output    json.RawMessage   `json:"output,omitempty"`
	Artifacts []Artifact        `json:"artifacts,omitempty"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

type TaskInvocation struct {
	Task         Task                  `json:"task"`
	Dependencies map[string]TaskResult `json:"dependencies,omitempty"`
}

type TaskHandler interface {
	ExecuteTask(ctx context.Context, invocation TaskInvocation) (TaskResult, error)
}

type TaskHandlerFunc func(context.Context, TaskInvocation) (TaskResult, error)

func (f TaskHandlerFunc) ExecuteTask(ctx context.Context, invocation TaskInvocation) (TaskResult, error) {
	return f(ctx, invocation)
}

type GraphResult struct {
	Status  TurnStatus            `json:"status"`
	Tasks   []Task                `json:"tasks"`
	Results map[string]TaskResult `json:"results,omitempty"`
}

type GraphExecutor struct {
	Journal        ItemJournal
	MaxConcurrency int
}

func NewGraphExecutor(journal ItemJournal, maxConcurrency int) *GraphExecutor {
	if maxConcurrency <= 0 {
		maxConcurrency = 4
	}
	return &GraphExecutor{Journal: journal, MaxConcurrency: maxConcurrency}
}

type graphNode struct {
	task   Task
	result TaskResult
}

type taskCompletion struct {
	id        string
	task      Task
	result    TaskResult
	recordErr error
}

// Execute runs a validated dependency graph. Task failures are represented in
// GraphResult rather than returned as infrastructure errors, allowing callers
// to use successful independent outputs. Validation, journal, and cancellation
// failures are returned directly.
func (e *GraphExecutor) Execute(ctx context.Context, scope RunScope, specs []TaskSpec, handler TaskHandler) (GraphResult, error) {
	if e == nil || e.Journal == nil {
		return GraphResult{}, errors.New("harness: graph executor journal is required")
	}
	if handler == nil {
		return GraphResult{}, errors.New("harness: task handler is required")
	}
	if strings.TrimSpace(scope.ThreadID) == "" || strings.TrimSpace(scope.TurnID) == "" {
		return GraphResult{}, errors.New("harness: graph thread and turn ids are required")
	}
	if err := ValidateTaskGraph(specs); err != nil {
		return GraphResult{}, err
	}

	maxConcurrency := e.MaxConcurrency
	if maxConcurrency <= 0 {
		maxConcurrency = 4
	}
	if maxConcurrency > len(specs) {
		maxConcurrency = len(specs)
	}

	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	nodes := make(map[string]*graphNode, len(specs))
	order := make([]string, 0, len(specs))
	now := time.Now().UTC()
	for _, spec := range specs {
		task := Task{Spec: cloneTaskSpec(spec), Status: TaskQueued, CreatedAt: now, UpdatedAt: now}
		nodes[spec.ID] = &graphNode{task: task}
		order = append(order, spec.ID)
	}

	remaining := len(nodes)
	active := 0
	completions := make(chan taskCompletion, maxConcurrency)
	var infrastructureErr error
	cancellationObserved := false

	for remaining > 0 {
		if runCtx.Err() != nil && !cancellationObserved {
			cancellationObserved = true
			for _, id := range order {
				node := nodes[id]
				if node.task.Status != TaskQueued {
					continue
				}
				node.task.Status = TaskCancelled
				node.task.Error = runCtx.Err().Error()
				node.task.UpdatedAt = time.Now().UTC()
				if err := e.recordTask(ctx, scope, node.task, TaskResult{}, ItemCancelled); err != nil && infrastructureErr == nil {
					infrastructureErr = err
				}
				remaining--
			}
		}

		if runCtx.Err() == nil {
			for _, id := range order {
				if active >= maxConcurrency {
					break
				}
				node := nodes[id]
				if node.task.Status != TaskQueued {
					continue
				}
				ready, blocked := dependencyState(node.task.Spec.Dependencies, nodes)
				if blocked {
					node.task.Status = TaskSkipped
					node.task.Error = "dependency did not complete"
					node.task.UpdatedAt = time.Now().UTC()
					remaining--
					if err := e.recordTask(ctx, scope, node.task, TaskResult{}, ItemCompleted); err != nil {
						infrastructureErr = err
						cancel()
						break
					}
					continue
				}
				if !ready {
					continue
				}

				node.task.Status = TaskRunning
				node.task.Attempt = 1
				node.task.UpdatedAt = time.Now().UTC()
				if err := e.recordTask(ctx, scope, node.task, TaskResult{}, ItemStarted); err != nil {
					node.task.Status = TaskFailed
					node.task.Error = err.Error()
					node.task.UpdatedAt = time.Now().UTC()
					remaining--
					infrastructureErr = err
					cancel()
					break
				}
				invocation := TaskInvocation{
					Task:         cloneTask(node.task),
					Dependencies: dependencyResults(node.task.Spec.Dependencies, nodes),
				}
				active++
				go func(taskID string, invocation TaskInvocation) {
					task, result, recordErr := e.executeTask(runCtx, scope, invocation, handler)
					completions <- taskCompletion{id: taskID, task: task, result: result, recordErr: recordErr}
				}(id, invocation)
			}
		}

		if active == 0 {
			if remaining == 0 {
				break
			}
			if infrastructureErr != nil {
				continue
			}
			return graphResult(order, nodes), fmt.Errorf("%w: scheduler made no progress", ErrInvalidTaskGraph)
		}

		var completion taskCompletion
		if cancellationObserved {
			completion = <-completions
		} else {
			select {
			case completion = <-completions:
			case <-runCtx.Done():
				continue
			}
		}
		active--
		remaining--
		nodes[completion.id].task = completion.task
		nodes[completion.id].result = cloneTaskResult(completion.result)
		if completion.recordErr != nil && infrastructureErr == nil {
			infrastructureErr = completion.recordErr
			cancel()
		}
	}

	result := graphResult(order, nodes)
	if infrastructureErr != nil {
		return result, infrastructureErr
	}
	if ctx.Err() != nil {
		return result, ctx.Err()
	}
	return result, nil
}

func (e *GraphExecutor) executeTask(ctx context.Context, scope RunScope, invocation TaskInvocation, handler TaskHandler) (Task, TaskResult, error) {
	task := cloneTask(invocation.Task)
	attempts := normalizedAttempts(task.Spec.Retry)
	var lastResult TaskResult
	var lastErr error

	for attempt := 1; attempt <= attempts; attempt++ {
		task.Attempt = attempt
		task.UpdatedAt = time.Now().UTC()
		if attempt > 1 {
			if err := e.recordTask(ctx, scope, task, lastResult, ItemStreaming); err != nil {
				return task, lastResult, err
			}
		}
		invocation.Task = cloneTask(task)
		taskCtx := WithTask(ctx, task.Spec.ID)
		if task.Spec.AssigneeID != "" {
			activationID := scope.TurnID + ":" + task.Spec.ID + ":" + fmt.Sprint(attempt)
			taskCtx = WithAgent(taskCtx, task.Spec.AssigneeID, activationID)
		}
		result, err := handler.ExecuteTask(taskCtx, invocation)
		lastResult = cloneTaskResult(result)
		lastErr = err
		if err == nil {
			task.Status = TaskCompleted
			task.Error = ""
			task.UpdatedAt = time.Now().UTC()
			return task, lastResult, e.recordTask(ctx, scope, task, lastResult, ItemCompleted)
		}
		if ctx.Err() != nil {
			task.Status = TaskCancelled
			task.Error = ctx.Err().Error()
			task.UpdatedAt = time.Now().UTC()
			return task, lastResult, e.recordTask(ctx, scope, task, lastResult, ItemCancelled)
		}
		if attempt < attempts {
			if err := waitForRetry(ctx, retryBackoff(task.Spec.Retry, attempt)); err != nil {
				task.Status = TaskCancelled
				task.Error = err.Error()
				task.UpdatedAt = time.Now().UTC()
				return task, lastResult, e.recordTask(ctx, scope, task, lastResult, ItemCancelled)
			}
		}
	}

	task.Status = TaskFailed
	task.Error = lastErr.Error()
	task.UpdatedAt = time.Now().UTC()
	return task, lastResult, e.recordTask(ctx, scope, task, lastResult, ItemFailed)
}

func (e *GraphExecutor) recordTask(ctx context.Context, scope RunScope, task Task, result TaskResult, status ItemStatus) error {
	payload, err := json.Marshal(struct {
		Task   Task       `json:"task"`
		Result TaskResult `json:"result,omitempty"`
	}{Task: task, Result: result})
	if err != nil {
		return fmt.Errorf("harness: encode task %q: %w", task.Spec.ID, err)
	}
	writeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	_, err = e.Journal.Append(writeCtx, Item{
		ID:          scope.TurnID + ":task:" + task.Spec.ID,
		WorkspaceID: scope.WorkspaceID,
		ThreadID:    scope.ThreadID,
		TurnID:      scope.TurnID,
		TaskID:      task.Spec.ID,
		AgentID:     task.Spec.AssigneeID,
		Type:        ItemTaskStatus,
		Status:      status,
		Payload:     payload,
		CreatedAt:   task.CreatedAt,
	})
	if err != nil {
		return fmt.Errorf("harness: record task %q: %w", task.Spec.ID, err)
	}
	return nil
}

func ValidateTaskGraph(specs []TaskSpec) error {
	if len(specs) == 0 {
		return fmt.Errorf("%w: at least one task is required", ErrInvalidTaskGraph)
	}
	byID := make(map[string]TaskSpec, len(specs))
	for _, spec := range specs {
		if strings.TrimSpace(spec.ID) == "" || strings.TrimSpace(spec.Goal) == "" {
			return fmt.Errorf("%w: task id and goal are required", ErrInvalidTaskGraph)
		}
		if _, exists := byID[spec.ID]; exists {
			return fmt.Errorf("%w: duplicate task %q", ErrInvalidTaskGraph, spec.ID)
		}
		if spec.Retry.MaxAttempts < 0 || spec.Retry.MaxAttempts > 10 || spec.Retry.InitialBackoffMs < 0 || spec.Retry.MaxBackoffMs < 0 {
			return fmt.Errorf("%w: task %q has invalid retry policy", ErrInvalidTaskGraph, spec.ID)
		}
		byID[spec.ID] = spec
	}

	indegree := make(map[string]int, len(specs))
	dependents := make(map[string][]string, len(specs))
	for _, spec := range specs {
		seen := make(map[string]struct{}, len(spec.Dependencies))
		for _, dependency := range spec.Dependencies {
			if dependency == spec.ID {
				return fmt.Errorf("%w: task %q depends on itself", ErrInvalidTaskGraph, spec.ID)
			}
			if _, exists := byID[dependency]; !exists {
				return fmt.Errorf("%w: task %q depends on missing task %q", ErrInvalidTaskGraph, spec.ID, dependency)
			}
			if _, exists := seen[dependency]; exists {
				return fmt.Errorf("%w: task %q repeats dependency %q", ErrInvalidTaskGraph, spec.ID, dependency)
			}
			seen[dependency] = struct{}{}
			indegree[spec.ID]++
			dependents[dependency] = append(dependents[dependency], spec.ID)
		}
	}

	queue := make([]string, 0, len(specs))
	for _, spec := range specs {
		if indegree[spec.ID] == 0 {
			queue = append(queue, spec.ID)
		}
	}
	visited := 0
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		visited++
		for _, dependent := range dependents[id] {
			indegree[dependent]--
			if indegree[dependent] == 0 {
				queue = append(queue, dependent)
			}
		}
	}
	if visited != len(specs) {
		return fmt.Errorf("%w: dependency cycle detected", ErrInvalidTaskGraph)
	}
	return nil
}

func dependencyState(dependencies []string, nodes map[string]*graphNode) (ready, blocked bool) {
	for _, dependency := range dependencies {
		switch nodes[dependency].task.Status {
		case TaskCompleted:
		case TaskFailed, TaskCancelled, TaskSkipped:
			return false, true
		default:
			ready = false
			return ready, false
		}
	}
	return true, false
}

func dependencyResults(dependencies []string, nodes map[string]*graphNode) map[string]TaskResult {
	if len(dependencies) == 0 {
		return nil
	}
	out := make(map[string]TaskResult, len(dependencies))
	for _, dependency := range dependencies {
		out[dependency] = cloneTaskResult(nodes[dependency].result)
	}
	return out
}

func normalizedAttempts(policy RetryPolicy) int {
	if policy.MaxAttempts <= 0 {
		return 1
	}
	return policy.MaxAttempts
}

func retryBackoff(policy RetryPolicy, failedAttempt int) time.Duration {
	backoff := policy.InitialBackoffMs
	if backoff <= 0 {
		return 0
	}
	if policy.MaxBackoffMs > 0 && backoff > policy.MaxBackoffMs {
		backoff = policy.MaxBackoffMs
	}
	for i := 1; i < failedAttempt; i++ {
		backoff *= 2
		if policy.MaxBackoffMs > 0 && backoff >= policy.MaxBackoffMs {
			backoff = policy.MaxBackoffMs
			break
		}
	}
	return time.Duration(backoff) * time.Millisecond
}

func waitForRetry(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func graphResult(order []string, nodes map[string]*graphNode) GraphResult {
	result := GraphResult{Status: TurnCompleted, Tasks: make([]Task, 0, len(order)), Results: make(map[string]TaskResult)}
	for _, id := range order {
		node := nodes[id]
		result.Tasks = append(result.Tasks, cloneTask(node.task))
		if node.task.Status == TaskCompleted {
			result.Results[id] = cloneTaskResult(node.result)
		}
		if node.task.Status == TaskFailed {
			result.Status = TurnFailed
		} else if node.task.Status == TaskCancelled && result.Status != TurnFailed {
			result.Status = TurnCancelled
		}
	}
	return result
}

func cloneTask(task Task) Task {
	task.Spec = cloneTaskSpec(task.Spec)
	return task
}

func cloneTaskSpec(spec TaskSpec) TaskSpec {
	spec.Dependencies = slices.Clone(spec.Dependencies)
	spec.Constraints = slices.Clone(spec.Constraints)
	spec.Input = slices.Clone(spec.Input)
	spec.Metadata = maps.Clone(spec.Metadata)
	return spec
}

func cloneTaskResult(result TaskResult) TaskResult {
	result.Output = slices.Clone(result.Output)
	result.Metadata = maps.Clone(result.Metadata)
	result.Artifacts = slices.Clone(result.Artifacts)
	for i := range result.Artifacts {
		result.Artifacts[i].Content = slices.Clone(result.Artifacts[i].Content)
		result.Artifacts[i].Metadata = maps.Clone(result.Artifacts[i].Metadata)
	}
	return result
}
