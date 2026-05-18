-- Rename upstream_* metrics tables to broker_* (hard-cut vocabulary).

ALTER TABLE upstream_rollup_minute RENAME TO broker_rollup_minute;
ALTER TABLE upstream_rollup_day RENAME TO broker_rollup_day;
ALTER TABLE upstream_call_events RENAME TO broker_call_events;
DROP INDEX IF EXISTS idx_upstream_call_events_time;
CREATE INDEX IF NOT EXISTS idx_broker_call_events_time ON broker_call_events (occurred_at);
