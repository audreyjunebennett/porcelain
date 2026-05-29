-- Operator SQLite: per-tenant provider model availability (operator-curated catalog filter).

CREATE TABLE IF NOT EXISTS provider_model_config (
	tenant_id TEXT NOT NULL,
	provider_id TEXT NOT NULL,
	updated_at TEXT NOT NULL,
	metadata_json TEXT NOT NULL DEFAULT '{}',
	PRIMARY KEY (tenant_id, provider_id)
);

CREATE TABLE IF NOT EXISTS provider_model_availability (
	tenant_id TEXT NOT NULL,
	provider_id TEXT NOT NULL,
	model_id TEXT NOT NULL,
	available INTEGER NOT NULL,
	metadata_json TEXT NOT NULL DEFAULT '{}',
	updated_at TEXT NOT NULL,
	PRIMARY KEY (tenant_id, provider_id, model_id)
);

CREATE INDEX IF NOT EXISTS idx_provider_model_avail_tenant ON provider_model_availability (tenant_id);
CREATE INDEX IF NOT EXISTS idx_provider_model_avail_provider ON provider_model_availability (tenant_id, provider_id);
