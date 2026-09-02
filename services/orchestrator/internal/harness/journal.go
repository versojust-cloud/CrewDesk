package harness

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"time"
)

var (
	ErrItemNotFound      = errors.New("harness: item not found")
	ErrInvalidTransition = errors.New("harness: invalid item transition")
)

// ItemJournal stores every lifecycle revision. ListSince returns the event
// stream, while Latest returns the authoritative state for one item ID.
type ItemJournal interface {
	Append(ctx context.Context, item Item) (Item, error)
	Latest(ctx context.Context, workspaceID, threadID, itemID string) (Item, error)
	ListSince(ctx context.Context, workspaceID, threadID string, afterSequence int64, limit int) ([]Item, error)
}

type MemoryJournal struct {
	mu       sync.RWMutex
	byThread map[string][]Item
	latest   map[string]map[string]Item
}

func NewMemoryJournal() *MemoryJournal {
	return &MemoryJournal{
		byThread: make(map[string][]Item),
		latest:   make(map[string]map[string]Item),
	}
}

func (j *MemoryJournal) Append(_ context.Context, item Item) (Item, error) {
	if err := ValidateItem(item); err != nil {
		return Item{}, err
	}

	j.mu.Lock()
	defer j.mu.Unlock()

	key := journalThreadKey(item.WorkspaceID, item.ThreadID)
	threadLatest := j.latest[key]
	if threadLatest == nil {
		threadLatest = make(map[string]Item)
		j.latest[key] = threadLatest
	}
	previous, exists := threadLatest[item.ID]
	if exists {
		if err := ValidateItemTransition(previous, item); err != nil {
			return Item{}, err
		}
		if item.CreatedAt.IsZero() {
			item.CreatedAt = previous.CreatedAt
		}
	}

	now := time.Now().UTC()
	if item.CreatedAt.IsZero() {
		item.CreatedAt = now
	}
	if isTerminalItemStatus(item.Status) && item.CompletedAt == nil {
		item.CompletedAt = &now
	}
	item.Sequence = int64(len(j.byThread[key])) + 1
	item.Payload = slices.Clone(item.Payload)

	j.byThread[key] = append(j.byThread[key], item)
	threadLatest[item.ID] = item
	return cloneItem(item), nil
}

func (j *MemoryJournal) Latest(_ context.Context, workspaceID, threadID, itemID string) (Item, error) {
	j.mu.RLock()
	defer j.mu.RUnlock()
	item, ok := j.latest[journalThreadKey(workspaceID, threadID)][itemID]
	if !ok {
		return Item{}, ErrItemNotFound
	}
	return cloneItem(item), nil
}

func (j *MemoryJournal) ListSince(_ context.Context, workspaceID, threadID string, afterSequence int64, limit int) ([]Item, error) {
	if limit <= 0 || limit > 2000 {
		limit = 500
	}
	j.mu.RLock()
	defer j.mu.RUnlock()
	all := j.byThread[journalThreadKey(workspaceID, threadID)]
	out := make([]Item, 0, min(limit, len(all)))
	for _, item := range all {
		if item.Sequence <= afterSequence {
			continue
		}
		out = append(out, cloneItem(item))
		if len(out) == limit {
			break
		}
	}
	return out, nil
}

func ValidateItem(item Item) error {
	if item.ID == "" || item.ThreadID == "" || item.TurnID == "" {
		return errors.New("harness: item id, thread id, and turn id are required")
	}
	if item.Type == "" || item.Status == "" {
		return errors.New("harness: item type and status are required")
	}
	switch item.Status {
	case ItemStarted, ItemStreaming, ItemCompleted, ItemFailed:
		return nil
	default:
		return fmt.Errorf("harness: unsupported item status %q", item.Status)
	}
}

func ValidateItemTransition(previous, next Item) error {
	if err := ValidateItem(next); err != nil {
		return err
	}
	if err := validateIdentity(previous, next); err != nil {
		return err
	}
	if !allowedItemTransition(previous.Status, next.Status) {
		return fmt.Errorf("%w: item %q cannot move from %q to %q", ErrInvalidTransition, next.ID, previous.Status, next.Status)
	}
	return nil
}

func validateIdentity(previous, next Item) error {
	if previous.WorkspaceID != next.WorkspaceID || previous.ThreadID != next.ThreadID || previous.TurnID != next.TurnID || previous.Type != next.Type || previous.TaskID != next.TaskID || previous.ActivationID != next.ActivationID {
		return fmt.Errorf("%w: item %q identity changed", ErrInvalidTransition, next.ID)
	}
	if previous.AgentID != "" && next.AgentID != "" && previous.AgentID != next.AgentID {
		return fmt.Errorf("%w: item %q agent changed", ErrInvalidTransition, next.ID)
	}
	return nil
}

func allowedItemTransition(previous, next ItemStatus) bool {
	switch previous {
	case ItemStarted:
		return next == ItemStreaming || next == ItemCompleted || next == ItemFailed
	case ItemStreaming:
		return next == ItemStreaming || next == ItemCompleted || next == ItemFailed
	default:
		return false
	}
}

func isTerminalItemStatus(status ItemStatus) bool {
	return status == ItemCompleted || status == ItemFailed
}

func cloneItem(item Item) Item {
	item.Payload = slices.Clone(item.Payload)
	if item.CompletedAt != nil {
		completedAt := *item.CompletedAt
		item.CompletedAt = &completedAt
	}
	return item
}

func journalThreadKey(workspaceID, threadID string) string {
	return workspaceID + "\x00" + threadID
}
