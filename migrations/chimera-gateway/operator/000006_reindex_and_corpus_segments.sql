-- Operator SQLite: workspace re-index intent + corpus segment index for manifest ingest.

ALTER TABLE workspaces ADD COLUMN reindex_generation INTEGER NOT NULL DEFAULT 0;

CREATE TABLE IF NOT EXISTS corpus_segments (
	segment_id TEXT PRIMARY KEY,
	tenant_id TEXT NOT NULL,
	project_id TEXT NOT NULL,
	flavor_id TEXT NOT NULL,
	source TEXT NOT NULL,
	content_sha256 TEXT NOT NULL,
	chunk_index INTEGER NOT NULL,
	chunk_count INTEGER NOT NULL,
	start_line INTEGER NOT NULL,
	end_line INTEGER NOT NULL,
	start_byte INTEGER NOT NULL,
	end_byte INTEGER NOT NULL,
	start_ch INTEGER NOT NULL,
	end_ch INTEGER NOT NULL,
	starts_mid_line INTEGER NOT NULL DEFAULT 0,
	vector_point_id TEXT NOT NULL,
	language TEXT,
	created_at TEXT NOT NULL,
	UNIQUE (tenant_id, project_id, flavor_id, source, content_sha256, chunk_index)
);

CREATE INDEX IF NOT EXISTS idx_corpus_segments_lookup
	ON corpus_segments (tenant_id, project_id, flavor_id, source, content_sha256);

CREATE INDEX IF NOT EXISTS idx_corpus_segments_line
	ON corpus_segments (tenant_id, project_id, flavor_id, source, start_line);
