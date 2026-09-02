-- Allow active Harness items to terminate through explicit cancellation.

alter table harness_item_events
  drop constraint if exists harness_item_status_check;

alter table harness_item_events
  add constraint harness_item_status_check
  check (status in ('started', 'streaming', 'completed', 'failed', 'cancelled'));
