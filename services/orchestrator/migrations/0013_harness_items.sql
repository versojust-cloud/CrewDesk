-- CrewDesk Harness item lifecycle journal.
-- Each item may have multiple revisions (started, streaming, completed/failed).
-- Sequence is monotonic within a workspace/thread pair and is the replay cursor.

create table if not exists harness_item_events (
  workspace_id text        not null default '',
  thread_id    text        not null,
  sequence     bigint      not null,
  item_id      text        not null,
  turn_id      text        not null,
  task_id      text        not null default '',
  agent_id     text        not null default '',
  activation_id text       not null default '',
  item_type    text        not null,
  status       text        not null,
  payload      jsonb       not null default 'null'::jsonb,
  created_at   timestamptz not null,
  completed_at timestamptz,
  primary key (workspace_id, thread_id, sequence),
  constraint harness_item_status_check
    check (status in ('started', 'streaming', 'completed', 'failed'))
);

create index if not exists harness_item_latest_idx
  on harness_item_events (workspace_id, thread_id, item_id, sequence desc);

create index if not exists harness_item_turn_idx
  on harness_item_events (workspace_id, thread_id, turn_id, sequence);

create index if not exists harness_item_created_at_idx
  on harness_item_events (created_at);
