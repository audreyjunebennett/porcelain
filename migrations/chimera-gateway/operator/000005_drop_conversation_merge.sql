-- Remove semantic conversation merge state (feature removed).

DROP INDEX IF EXISTS idx_conversation_dedup_created;
DROP TABLE IF EXISTS conversation_dedup_cache;
DROP INDEX IF EXISTS idx_conversation_context_scope_time;
DROP TABLE IF EXISTS conversation_context;
