-- Operator SQLite: indexer workspaces (Phase 1). Applied by internal/operatorstore; add new numbered files only.

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
