package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/dreamwaver/dreamwaver/services/orchestrator/internal/harness"
)

type pgxHarnessItems struct {
	pool *pgxpool.Pool
}

func newPgxHarnessItems(pool *pgxpool.Pool) *pgxHarnessItems {
	return &pgxHarnessItems{pool: pool}
}

func (s *pgxHarnessItems) Append(ctx context.Context, item harness.Item) (harness.Item, error) {
	if err := harness.ValidateItem(item); err != nil {
		return harness.Item{}, err
	}

	// The transaction-level advisory lock coordinates sequence allocation and
	// lifecycle validation for this workspace/thread across all processes.
	// Independent threads remain concurrent.
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return harness.Item{}, fmt.Errorf("harness items begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx,
		`select pg_advisory_xact_lock(hashtextextended($1 || chr(31) || $2, 0))`,
		item.WorkspaceID, item.ThreadID,
	); err != nil {
		return harness.Item{}, fmt.Errorf("harness items lock: %w", err)
	}

	previous, err := latestHarnessItem(ctx, tx, item.WorkspaceID, item.ThreadID, item.ID)
	if err == nil {
		if err := harness.ValidateItemTransition(previous, item); err != nil {
			return harness.Item{}, err
		}
		if item.CreatedAt.IsZero() {
			item.CreatedAt = previous.CreatedAt
		}
	} else if !errors.Is(err, harness.ErrItemNotFound) {
		return harness.Item{}, err
	}

	var maxSequence int64
	if err := tx.QueryRow(ctx, `
		select coalesce(max(sequence), 0)
		from harness_item_events
		where workspace_id = $1 and thread_id = $2
	`, item.WorkspaceID, item.ThreadID).Scan(&maxSequence); err != nil {
		return harness.Item{}, fmt.Errorf("harness items sequence: %w", err)
	}
	item.Sequence = maxSequence + 1
	now := time.Now().UTC()
	if item.CreatedAt.IsZero() {
		item.CreatedAt = now
	}
	if (item.Status == harness.ItemCompleted || item.Status == harness.ItemFailed) && item.CompletedAt == nil {
		item.CompletedAt = &now
	}

	if _, err := tx.Exec(ctx, `
		insert into harness_item_events (
			workspace_id, thread_id, sequence, item_id, turn_id,
			task_id, agent_id, activation_id, item_type, status, payload, created_at, completed_at
		) values ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
	`, item.WorkspaceID, item.ThreadID, item.Sequence, item.ID, item.TurnID,
		item.TaskID, item.AgentID, item.ActivationID, string(item.Type), string(item.Status), harnessPayload(item.Payload), item.CreatedAt, item.CompletedAt,
	); err != nil {
		return harness.Item{}, fmt.Errorf("harness items append: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return harness.Item{}, fmt.Errorf("harness items commit: %w", err)
	}
	return item, nil
}

func (s *pgxHarnessItems) Latest(ctx context.Context, workspaceID, threadID, itemID string) (harness.Item, error) {
	return latestHarnessItem(ctx, s.pool, workspaceID, threadID, itemID)
}

func (s *pgxHarnessItems) ListSince(ctx context.Context, workspaceID, threadID string, afterSequence int64, limit int) ([]harness.Item, error) {
	if limit <= 0 || limit > 2000 {
		limit = 500
	}
	rows, err := s.pool.Query(ctx, `
		select workspace_id, thread_id, sequence, item_id, turn_id,
		       task_id, agent_id, activation_id, item_type, status, coalesce(payload, 'null'::jsonb), created_at, completed_at
		from harness_item_events
		where workspace_id = $1 and thread_id = $2 and sequence > $3
		order by sequence asc
		limit $4
	`, workspaceID, threadID, afterSequence, limit)
	if err != nil {
		return nil, fmt.Errorf("harness items list: %w", err)
	}
	defer rows.Close()

	items := make([]harness.Item, 0, limit)
	for rows.Next() {
		item, err := scanHarnessItem(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

type harnessItemScanner interface {
	Scan(dest ...any) error
}

func latestHarnessItem(ctx context.Context, queryer interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, workspaceID, threadID, itemID string) (harness.Item, error) {
	row := queryer.QueryRow(ctx, `
		select workspace_id, thread_id, sequence, item_id, turn_id,
		       task_id, agent_id, activation_id, item_type, status, coalesce(payload, 'null'::jsonb), created_at, completed_at
		from harness_item_events
		where workspace_id = $1 and thread_id = $2 and item_id = $3
		order by sequence desc
		limit 1
	`, workspaceID, threadID, itemID)
	item, err := scanHarnessItem(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return harness.Item{}, harness.ErrItemNotFound
	}
	if err != nil {
		return harness.Item{}, fmt.Errorf("harness items latest: %w", err)
	}
	return item, nil
}

func scanHarnessItem(row harnessItemScanner) (harness.Item, error) {
	var item harness.Item
	var itemType, status string
	if err := row.Scan(
		&item.WorkspaceID, &item.ThreadID, &item.Sequence, &item.ID, &item.TurnID,
		&item.TaskID, &item.AgentID, &item.ActivationID, &itemType, &status, &item.Payload, &item.CreatedAt, &item.CompletedAt,
	); err != nil {
		return harness.Item{}, err
	}
	item.Type = harness.ItemType(itemType)
	item.Status = harness.ItemStatus(status)
	return item, nil
}

func harnessPayload(payload json.RawMessage) json.RawMessage {
	if len(payload) == 0 {
		return json.RawMessage("null")
	}
	return payload
}

var _ harness.ItemJournal = (*pgxHarnessItems)(nil)
