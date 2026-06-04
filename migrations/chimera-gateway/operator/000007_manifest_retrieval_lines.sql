-- Manifest ingest: line ranges on saved RAG hits (no backfill for legacy rows).

ALTER TABLE conversation_retrievals ADD COLUMN start_line INTEGER NOT NULL DEFAULT 0;
ALTER TABLE conversation_retrievals ADD COLUMN end_line INTEGER NOT NULL DEFAULT 0;
ALTER TABLE conversation_retrievals ADD COLUMN starts_mid_line INTEGER NOT NULL DEFAULT 0 CHECK (starts_mid_line IN (0, 1));
