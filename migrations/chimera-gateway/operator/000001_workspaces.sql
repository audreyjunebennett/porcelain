-- Operator SQLite: indexer workspaces + conversation merge state. Applied by internal/operatorstore; add new numbered files only.

CREATE TABLE IF NOT EXISTS workspaces (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	tenant_id TEXT NOT NULL DEFAULT '',
	project_id TEXT NOT NULL,
	flavor_id TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_workspaces_tenant ON workspaces (tenant_id);

CREATE TABLE IF NOT EXISTS workspace_paths (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	workspace_row_id INTEGER NOT NULL REFERENCES workspaces (id) ON DELETE CASCADE,
	path TEXT NOT NULL,
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_workspace_paths_workspace ON workspace_paths (workspace_row_id);

-- Semantic conversation merge + rolling fingerprint state (moved from metrics DB).
CREATE TABLE IF NOT EXISTS conversation_context (
	conversation_id TEXT NOT NULL PRIMARY KEY,
	tenant_id TEXT NOT NULL,
	project_id TEXT NOT NULL DEFAULT '',
	flavor_id TEXT NOT NULL DEFAULT '',
	last_user_embedding BLOB NOT NULL,
	embedding_dim INTEGER NOT NULL,
	last_user_text_normalized TEXT NOT NULL DEFAULT '',
	last_model_text_normalized TEXT NOT NULL DEFAULT '',
	last_updated_unix REAL NOT NULL,
	rolling_fingerprint TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_conversation_context_scope_time
	ON conversation_context (tenant_id, project_id, flavor_id, last_updated_unix DESC);

-- Short-lived JSON completion cache for duplicate HTTP retries (same scope + fingerprint + user text).
CREATE TABLE IF NOT EXISTS conversation_dedup_cache (
	dedup_key TEXT NOT NULL PRIMARY KEY,
	response_body BLOB NOT NULL,
	created_unix REAL NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_conversation_dedup_created ON conversation_dedup_cache (created_unix);
