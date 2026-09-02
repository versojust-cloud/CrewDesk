package harness

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dreamwaver/dreamwaver/services/orchestrator/internal/event"
)

// RecordingEmitter dual-writes existing progress events into the Harness
// journal. Legacy WebSocket consumers keep receiving the unchanged event.
type RecordingEmitter struct {
	Next    event.Emitter
	Journal ItemJournal

	counter        atomic.Uint64
	mu             sync.Mutex
	activeMessages map[string]string
	queue          chan recordRequest
}

type recordRequest struct {
	item    Item
	flushed chan struct{}
}

func NewRecordingEmitter(next event.Emitter, journal ItemJournal) *RecordingEmitter {
	recorder := &RecordingEmitter{
		Next:           next,
		Journal:        journal,
		activeMessages: make(map[string]string),
	}
	if journal != nil {
		recorder.queue = make(chan recordRequest, 1024)
		go recorder.recordLoop()
	}
	return recorder
}

func (r *RecordingEmitter) Emit(ctx context.Context, ev event.Event) {
	if r.Next != nil {
		r.Next.Emit(ctx, ev)
	}
	if r.Journal == nil {
		return
	}
	item, ok := r.project(ctx, ev)
	if !ok {
		return
	}
	select {
	case r.queue <- recordRequest{item: item}:
	case <-ctx.Done():
	}
}

func (r *RecordingEmitter) recordLoop() {
	for request := range r.queue {
		if request.flushed != nil {
			close(request.flushed)
			continue
		}
		item := request.item
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_, err := r.Journal.Append(ctx, item)
		cancel()
		if err != nil {
			slog.Warn("harness event recording failed", "item_id", item.ID, "err", err)
		}
	}
}

// Flush waits for events already accepted by the recorder. It is used by
// tests and graceful shutdown paths; normal request handling never waits.
func (r *RecordingEmitter) Flush(ctx context.Context) error {
	if r.queue == nil {
		return nil
	}
	done := make(chan struct{})
	select {
	case r.queue <- recordRequest{flushed: done}:
	case <-ctx.Done():
		return ctx.Err()
	}
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (r *RecordingEmitter) project(ctx context.Context, ev event.Event) (Item, bool) {
	scope, scoped := RunScopeFromContext(ctx)
	threadID := scope.ThreadID
	turnID := scope.TurnID
	if threadID == "" {
		threadID = ev.SessionID
	}
	if threadID == "" {
		threadID = event.SessionIDFromContext(ctx)
	}
	if threadID == "" {
		return Item{}, false
	}
	if turnID == "" {
		turnID = threadID + ":legacy"
	}
	agentID := ev.Data.Agent
	if agentID == "" && scoped {
		agentID = scope.AgentID
	}

	payload, err := json.Marshal(struct {
		LegacyKind event.Kind      `json:"legacy_kind"`
		Data       event.EventData `json:"data"`
	}{LegacyKind: ev.Kind, Data: ev.Data})
	if err != nil {
		return Item{}, false
	}

	item := Item{
		WorkspaceID:  scope.WorkspaceID,
		ThreadID:     threadID,
		TurnID:       turnID,
		TaskID:       scope.TaskID,
		AgentID:      agentID,
		ActivationID: scope.ActivationID,
		Payload:      payload,
		CreatedAt:    ev.At,
	}

	switch ev.Kind {
	case event.KindLLMToken:
		item.Type = ItemAgentMessage
		key := messageKey(threadID, turnID, scope.ActivationID, agentID)
		r.mu.Lock()
		item.ID = r.activeMessages[key]
		if item.ID == "" {
			item.ID = r.nextID(turnID, "message")
			r.activeMessages[key] = item.ID
			item.Status = ItemStarted
		} else {
			r.mu.Unlock()
			return Item{}, false
		}
		r.mu.Unlock()
	case event.KindLLMThought:
		item.Type = ItemAgentMessage
		key := messageKey(threadID, turnID, scope.ActivationID, agentID)
		r.mu.Lock()
		item.ID = r.activeMessages[key]
		delete(r.activeMessages, key)
		r.mu.Unlock()
		if item.ID == "" {
			item.ID = r.nextID(turnID, "message")
		}
		item.Status = ItemCompleted
	case event.KindToolStart:
		item.ID = toolItemID(turnID, ev.Data.ToolID, ev.Data.ToolName)
		item.Type = ItemToolCall
		item.Status = ItemStarted
	case event.KindToolEnd:
		item.ID = toolItemID(turnID, ev.Data.ToolID, ev.Data.ToolName)
		item.Type = ItemToolCall
		if ev.Data.Error != "" {
			item.Status = ItemFailed
		} else {
			item.Status = ItemCompleted
		}
	case event.KindStepStart:
		item.ID = fmt.Sprintf("%s:step:%s:%d", turnID, agentID, ev.Data.Step)
		item.Type = ItemTaskStatus
		item.Status = ItemStarted
	case event.KindStepEnd:
		item.ID = fmt.Sprintf("%s:step:%s:%d", turnID, agentID, ev.Data.Step)
		item.Type = ItemTaskStatus
		item.Status = ItemCompleted
	case event.KindError:
		item.ID = r.nextID(turnID, "error")
		item.Type = ItemError
		item.Status = ItemFailed
	case event.KindUserAnswer:
		item.ID = r.nextID(turnID, "user-message")
		item.Type = ItemUserMessage
		item.Status = ItemCompleted
	case event.KindClarificationRequired, event.KindOutlineReviewRequired, event.KindWizardStep, event.KindClawClarify:
		item.ID = r.nextID(turnID, "approval")
		item.Type = ItemApprovalRequest
		item.Status = ItemStarted
	case event.KindClawArtifactUpdated, event.KindRenderEnd, event.KindSlideUpdated:
		item.ID = r.nextID(turnID, "artifact")
		item.Type = ItemArtifact
		item.Status = ItemCompleted
	default:
		item.ID = r.nextID(turnID, "event")
		item.Type = ItemTaskStatus
		item.Status = ItemCompleted
	}
	return item, true
}

func (r *RecordingEmitter) nextID(turnID, prefix string) string {
	return fmt.Sprintf("%s:%s-%d", turnID, prefix, r.counter.Add(1))
}

func messageKey(threadID, turnID, activationID, agentID string) string {
	return threadID + "\x00" + turnID + "\x00" + activationID + "\x00" + agentID
}

func toolItemID(turnID, toolID, toolName string) string {
	if toolID == "" {
		toolID = toolName
	}
	return turnID + ":tool:" + toolID
}
