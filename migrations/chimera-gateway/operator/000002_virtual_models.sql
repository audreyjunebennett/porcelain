-- Operator SQLite: virtual models and per-model routing stacks.

CREATE TABLE IF NOT EXISTS virtual_models (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	model_id TEXT NOT NULL UNIQUE,
	name TEXT NOT NULL,
	version TEXT NOT NULL,
	description TEXT NOT NULL DEFAULT '',
	enabled INTEGER NOT NULL DEFAULT 1,
	visibility TEXT NOT NULL DEFAULT 'public',
	created_by_principal_id TEXT NOT NULL DEFAULT '',
	tenant_id TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_virtual_models_tenant ON virtual_models (tenant_id);
CREATE INDEX IF NOT EXISTS idx_virtual_models_enabled ON virtual_models (enabled);

CREATE TABLE IF NOT EXISTS routing_rule_definitions (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL,
	slug TEXT NOT NULL UNIQUE,
	default_config_json TEXT NOT NULL DEFAULT '{}',
	description TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS virtual_model_fallback (
	virtual_model_id INTEGER NOT NULL PRIMARY KEY REFERENCES virtual_models (id) ON DELETE CASCADE,
	chain_json TEXT NOT NULL,
	updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS virtual_model_routing_policy (
	virtual_model_id INTEGER NOT NULL PRIMARY KEY REFERENCES virtual_models (id) ON DELETE CASCADE,
	enabled INTEGER NOT NULL DEFAULT 1,
	policy_yaml TEXT NOT NULL DEFAULT '',
	updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS virtual_model_tool_router (
	virtual_model_id INTEGER NOT NULL PRIMARY KEY REFERENCES virtual_models (id) ON DELETE CASCADE,
	enabled INTEGER NOT NULL DEFAULT 0,
	router_models_json TEXT NOT NULL DEFAULT '[]',
	confidence_threshold REAL NOT NULL DEFAULT 0.5,
	updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS virtual_model_rule_bindings (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	virtual_model_id INTEGER NOT NULL REFERENCES virtual_models (id) ON DELETE CASCADE,
	routing_rule_definition_id INTEGER NOT NULL REFERENCES routing_rule_definitions (id) ON DELETE CASCADE,
	enabled INTEGER NOT NULL DEFAULT 1,
	override_config_json TEXT NOT NULL DEFAULT '{}',
	sort_order INTEGER NOT NULL DEFAULT 0,
	UNIQUE (virtual_model_id, routing_rule_definition_id)
);

CREATE INDEX IF NOT EXISTS idx_vm_rule_bindings_vm ON virtual_model_rule_bindings (virtual_model_id);
