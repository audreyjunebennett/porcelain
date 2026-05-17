-- Gateway metrics schema (G6). Applied by internal/gatewaymetrics migrator; do not edit history in place — add new numbered files.

CREATE TABLE IF NOT EXISTS upstream_rollup_minute (
  provider TEXT NOT NULL,
  model_id TEXT NOT NULL,
  minute_utc TEXT NOT NULL,
  status INTEGER NOT NULL,
  calls INTEGER NOT NULL DEFAULT 0,
  est_tokens INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (provider, model_id, minute_utc, status)
);

CREATE TABLE IF NOT EXISTS upstream_rollup_day (
  provider TEXT NOT NULL,
  model_id TEXT NOT NULL,
  day_utc TEXT NOT NULL,
  status INTEGER NOT NULL,
  calls INTEGER NOT NULL DEFAULT 0,
  est_tokens INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (provider, model_id, day_utc, status)
);

CREATE TABLE IF NOT EXISTS upstream_call_events (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  occurred_at TEXT NOT NULL,
  provider TEXT NOT NULL,
  model_id TEXT NOT NULL,
  status INTEGER NOT NULL,
  est_tokens INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_upstream_call_events_time ON upstream_call_events (occurred_at);
