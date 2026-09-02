package harness

import (
	"context"
	"testing"

	"github.com/dreamwaver/dreamwaver/services/orchestrator/internal/event"
)

type captureEmitter struct{ events []event.Event }

func (e *captureEmitter) Emit(_ context.Context, ev event.Event) {
	e.events = append(e.events, ev)
}

func TestRecordingEmitterPreservesLegacyAndRecordsMessageLifecycle(t *testing.T) {
	t.Parallel()

	next := &captureEmitter{}
	journal := NewMemoryJournal()
	recorder := NewRecordingEmitter(next, journal)
	ctx := WithRunScope(context.Background(), RunScope{ThreadID: "thread-1", TurnID: "turn-1", AgentID: "writer"})

	recorder.Emit(ctx, event.NewLLMToken("writer", "hello"))
	recorder.Emit(ctx, event.NewLLMToken("writer", " world"))
	recorder.Emit(ctx, event.NewLLMThought("writer", "hello world", nil, event.Tokens{}))
	if err := recorder.Flush(ctx); err != nil {
		t.Fatalf("Flush() error = %v", err)
	}

	if len(next.events) != 3 {
		t.Fatalf("legacy emitter got %d events, want 3", len(next.events))
	}
	items, err := journal.ListSince(ctx, "", "thread-1", 0, 10)
	if err != nil {
		t.Fatalf("ListSince() error = %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("journal got %d items, want 2", len(items))
	}
	if items[0].ID != items[1].ID {
		t.Fatalf("message lifecycle used different ids: %#v", items)
	}
	if items[0].Status != ItemStarted || items[1].Status != ItemCompleted {
		t.Fatalf("unexpected lifecycle: %#v", items)
	}
}

func TestRecordingEmitterRecordsToolLifecycle(t *testing.T) {
	t.Parallel()

	journal := NewMemoryJournal()
	recorder := NewRecordingEmitter(event.NoopEmitter{}, journal)
	ctx := WithRunScope(context.Background(), RunScope{ThreadID: "thread-1", TurnID: "turn-1", TaskID: "task-1"})

	recorder.Emit(ctx, event.NewToolStart("engineer", "code_execute", "call-1", `{}`))
	recorder.Emit(ctx, event.NewToolEnd("engineer", "code_execute", "call-1", "ok", "", 12))
	if err := recorder.Flush(ctx); err != nil {
		t.Fatalf("Flush() error = %v", err)
	}

	items, err := journal.ListSince(ctx, "", "thread-1", 0, 10)
	if err != nil {
		t.Fatalf("ListSince() error = %v", err)
	}
	if len(items) != 2 || items[0].ID != items[1].ID || items[1].Status != ItemCompleted {
		t.Fatalf("unexpected tool lifecycle: %#v", items)
	}
	if items[0].TaskID != "task-1" {
		t.Fatalf("tool task id = %q, want task-1", items[0].TaskID)
	}
}
