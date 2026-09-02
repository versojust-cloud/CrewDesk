package harness

import (
	"encoding/json"
	"time"
)

type OrchestrationPolicy string

const (
	PolicyManager    OrchestrationPolicy = "manager"
	PolicySequential OrchestrationPolicy = "sequential"
	PolicyParallel   OrchestrationPolicy = "parallel"
	PolicyGraph      OrchestrationPolicy = "graph"
	PolicyHandoff    OrchestrationPolicy = "handoff"
	PolicyRoundtable OrchestrationPolicy = "roundtable"
	PolicyReviewLoop OrchestrationPolicy = "review_loop"
)

type ThreadStatus string

const (
	ThreadActive   ThreadStatus = "active"
	ThreadArchived ThreadStatus = "archived"
)

// Thread is the durable conversation and workspace boundary.
type Thread struct {
	ID          string            `json:"id"`
	WorkspaceID string            `json:"workspace_id,omitempty"`
	Title       string            `json:"title,omitempty"`
	Status      ThreadStatus      `json:"status"`
	Metadata    map[string]string `json:"metadata,omitempty"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
}

type TurnStatus string

const (
	TurnQueued        TurnStatus = "queued"
	TurnRunning       TurnStatus = "running"
	TurnInputRequired TurnStatus = "input_required"
	TurnCompleted     TurnStatus = "completed"
	TurnFailed        TurnStatus = "failed"
	TurnCancelled     TurnStatus = "cancelled"
)

// Turn is one user request and its complete execution lifecycle.
type Turn struct {
	ID          string              `json:"id"`
	ThreadID    string              `json:"thread_id"`
	Status      TurnStatus          `json:"status"`
	Policy      OrchestrationPolicy `json:"policy"`
	Input       string              `json:"input"`
	RootAgentID string              `json:"root_agent_id,omitempty"`
	CreatedAt   time.Time           `json:"created_at"`
	UpdatedAt   time.Time           `json:"updated_at"`
	CompletedAt *time.Time          `json:"completed_at,omitempty"`
	Error       string              `json:"error,omitempty"`
}

type TaskStatus string

const (
	TaskQueued        TaskStatus = "queued"
	TaskRunning       TaskStatus = "running"
	TaskInputRequired TaskStatus = "input_required"
	TaskCompleted     TaskStatus = "completed"
	TaskFailed        TaskStatus = "failed"
	TaskCancelled     TaskStatus = "cancelled"
	TaskSkipped       TaskStatus = "skipped"
)

// TaskSpec is the structured brief passed between agents. Dependencies turn
// a flat plan into a DAG without coupling the runtime to a single planner.
type TaskSpec struct {
	ID             string            `json:"id"`
	ParentID       string            `json:"parent_id,omitempty"`
	Goal           string            `json:"goal"`
	AssigneeID     string            `json:"assignee_id,omitempty"`
	Dependencies   []string          `json:"dependencies,omitempty"`
	Constraints    []string          `json:"constraints,omitempty"`
	ExpectedOutput string            `json:"expected_output,omitempty"`
	Input          json.RawMessage   `json:"input,omitempty"`
	Metadata       map[string]string `json:"metadata,omitempty"`
	Limits         RunLimits         `json:"limits"`
	Retry          RetryPolicy       `json:"retry"`
}

// RetryPolicy bounds transient retries for one task. Zero MaxAttempts means
// one attempt. Backoff values are milliseconds to keep the API JSON stable.
type RetryPolicy struct {
	MaxAttempts      int `json:"max_attempts,omitempty"`
	InitialBackoffMs int `json:"initial_backoff_ms,omitempty"`
	MaxBackoffMs     int `json:"max_backoff_ms,omitempty"`
}

type Task struct {
	Spec      TaskSpec   `json:"spec"`
	Status    TaskStatus `json:"status"`
	Attempt   int        `json:"attempt"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	Error     string     `json:"error,omitempty"`
}

type ItemType string

const (
	ItemUserMessage     ItemType = "user_message"
	ItemAgentMessage    ItemType = "agent_message"
	ItemToolCall        ItemType = "tool_call"
	ItemToolResult      ItemType = "tool_result"
	ItemTaskStatus      ItemType = "task_status"
	ItemArtifact        ItemType = "artifact"
	ItemHandoff         ItemType = "handoff"
	ItemApprovalRequest ItemType = "approval_request"
	ItemError           ItemType = "error"
)

type ItemStatus string

const (
	ItemStarted   ItemStatus = "started"
	ItemStreaming ItemStatus = "streaming"
	ItemCompleted ItemStatus = "completed"
	ItemFailed    ItemStatus = "failed"
	ItemCancelled ItemStatus = "cancelled"
)

// Item is the append-only execution record used by persistence, streaming,
// replay, evaluation, and frontend projections.
type Item struct {
	ID           string          `json:"id"`
	WorkspaceID  string          `json:"workspace_id,omitempty"`
	ThreadID     string          `json:"thread_id"`
	TurnID       string          `json:"turn_id"`
	TaskID       string          `json:"task_id,omitempty"`
	AgentID      string          `json:"agent_id,omitempty"`
	ActivationID string          `json:"activation_id,omitempty"`
	Type         ItemType        `json:"type"`
	Status       ItemStatus      `json:"status"`
	Sequence     int64           `json:"sequence"`
	Payload      json.RawMessage `json:"payload,omitempty"`
	CreatedAt    time.Time       `json:"created_at"`
	CompletedAt  *time.Time      `json:"completed_at,omitempty"`
}

// Artifact provides one representation for documents, images, datasets,
// source archives, decks, videos, and other task outputs.
type Artifact struct {
	ID        string            `json:"id"`
	ThreadID  string            `json:"thread_id"`
	TurnID    string            `json:"turn_id"`
	TaskID    string            `json:"task_id,omitempty"`
	AgentID   string            `json:"agent_id,omitempty"`
	Kind      string            `json:"kind"`
	Name      string            `json:"name"`
	MIMEType  string            `json:"mime_type,omitempty"`
	URI       string            `json:"uri,omitempty"`
	Content   json.RawMessage   `json:"content,omitempty"`
	Version   int               `json:"version"`
	Metadata  map[string]string `json:"metadata,omitempty"`
	CreatedAt time.Time         `json:"created_at"`
}
