package harness

import (
	"context"
	"errors"
	"testing"
)

func TestMemoryJournalLifecycleAndPaging(t *testing.T) {
	t.Parallel()

	journal := NewMemoryJournal()
	started, err := journal.Append(context.Background(), Item{
		ID: "tool-1", ThreadID: "thread-1", TurnID: "turn-1",
		Type: ItemToolCall, Status: ItemStarted,
	})
	if err != nil {
		t.Fatalf("Append(started) error = %v", err)
	}
	if started.Sequence != 1 {
		t.Fatalf("started sequence = %d, want 1", started.Sequence)
	}

	completed, err := journal.Append(context.Background(), Item{
		ID: "tool-1", ThreadID: "thread-1", TurnID: "turn-1",
		Type: ItemToolCall, Status: ItemCompleted,
	})
	if err != nil {
		t.Fatalf("Append(completed) error = %v", err)
	}
	if completed.Sequence != 2 || completed.CompletedAt == nil {
		t.Fatalf("unexpected completed item: %#v", completed)
	}

	page, err := journal.ListSince(context.Background(), "", "thread-1", 1, 10)
	if err != nil {
		t.Fatalf("ListSince() error = %v", err)
	}
	if len(page) != 1 || page[0].Status != ItemCompleted {
		t.Fatalf("unexpected page: %#v", page)
	}
	latest, err := journal.Latest(context.Background(), "", "thread-1", "tool-1")
	if err != nil || latest.Status != ItemCompleted {
		t.Fatalf("Latest() = %#v, %v", latest, err)
	}
}

func TestMemoryJournalRejectsTerminalRewrite(t *testing.T) {
	t.Parallel()

	journal := NewMemoryJournal()
	base := Item{ID: "message-1", ThreadID: "thread-1", TurnID: "turn-1", Type: ItemAgentMessage}
	base.Status = ItemCompleted
	if _, err := journal.Append(context.Background(), base); err != nil {
		t.Fatalf("Append(completed) error = %v", err)
	}
	base.Status = ItemStreaming
	if _, err := journal.Append(context.Background(), base); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("Append(after terminal) error = %v, want ErrInvalidTransition", err)
	}
}

func TestMemoryJournalSeparatesThreadSequences(t *testing.T) {
	t.Parallel()

	journal := NewMemoryJournal()
	for _, threadID := range []string{"thread-a", "thread-b"} {
		item, err := journal.Append(context.Background(), Item{
			ID: "message", ThreadID: threadID, TurnID: "turn", Type: ItemAgentMessage, Status: ItemCompleted,
		})
		if err != nil {
			t.Fatalf("Append(%s) error = %v", threadID, err)
		}
		if item.Sequence != 1 {
			t.Fatalf("thread %s sequence = %d, want 1", threadID, item.Sequence)
		}
	}
}
