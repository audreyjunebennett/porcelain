-- Operator conversation history (transcripts, not lifecycle logs).

CREATE TABLE IF NOT EXISTS conversations (
	conversation_id TEXT NOT NULL PRIMARY KEY,
	principal_id TEXT NOT NULL,
	title TEXT NULL,
	preview_text TEXT NOT NULL DEFAULT '',
	flagged INTEGER NOT NULL DEFAULT 0 CHECK (flagged IN (0, 1)),
	workspace_project_id TEXT NOT NULL DEFAULT '',
	workspace_flavor_id TEXT NOT NULL DEFAULT '',
	workspace_row_id INTEGER NULL,
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS conversation_turns (
	turn_id TEXT NOT NULL PRIMARY KEY,
	conversation_id TEXT NOT NULL REFERENCES conversations (conversation_id) ON DELETE CASCADE,
	turn_index INTEGER NOT NULL,
	role TEXT NOT NULL CHECK (role IN ('user', 'assistant', 'error')),
	content TEXT NOT NULL,
	selected_model TEXT NOT NULL DEFAULT '',
	resolved_model TEXT NOT NULL DEFAULT '',
	error_detail TEXT NOT NULL DEFAULT '',
	retry_user_text TEXT NOT NULL DEFAULT '',
	prompt_tokens INTEGER NULL,
	completion_tokens INTEGER NULL,
	total_tokens INTEGER NULL,
	created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS conversation_retrievals (
	retrieval_id TEXT NOT NULL PRIMARY KEY,
	turn_id TEXT NOT NULL REFERENCES conversation_turns (turn_id) ON DELETE CASCADE,
	sort_order INTEGER NOT NULL,
	file_path TEXT NOT NULL DEFAULT '',
	score REAL NOT NULL DEFAULT 0,
	snippet_text TEXT NOT NULL DEFAULT '',
	language TEXT NOT NULL DEFAULT '',
	vector_point_id TEXT NOT NULL DEFAULT '',
	content_sha256 TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_conversations_principal_updated
	ON conversations (principal_id, updated_at DESC);

CREATE INDEX IF NOT EXISTS idx_conversations_principal_flagged
	ON conversations (principal_id, flagged, updated_at DESC);

CREATE INDEX IF NOT EXISTS idx_conversation_turns_conversation
	ON conversation_turns (conversation_id, turn_index, role);

CREATE INDEX IF NOT EXISTS idx_conversation_retrievals_turn
	ON conversation_retrievals (turn_id, sort_order);
