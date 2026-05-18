# Chimera configuration reference

Chimera (the gateway runtime in Porcelain) reads **YAML files** and **environment variables**.

## Operator vocabulary (Chimera products)

In UI, logs, supervisor output, and this doc set, use **Chimera product names**:

| Product | Role |
|---------|------|
| **chimera-gateway** | Client-facing API, routing, RAG orchestration, operator UI |
| **chimera-broker** | LLM relay (supervised wrapper around the OpenAI-compatible backend) |
| **chimera-vectorstore** | Vector retrieval for RAG (supervised wrapper around the storage backend) |
| **chimera-indexer** | File ingest into collections scoped by workspace |

**Config file keys** may still say `upstream.*` or `rag.qdrant.*` (wire names unchanged until a dedicated env/YAML hard-cut). **`GET /status`** exposes `broker` and `vectorstore` blocks; nested `broker.upstream` / `vectorstore` debug fields name the wrapped implementation only.

Naming refactor status: [plans/chimera-gateway-refactor.md](plans/chimera-gateway-refactor.md). Vocabulary audit: `make chimera-gateway-audit` or `pwsh -File scripts/chimera-gateway-vocab-audit.ps1`. `gateway.yaml`, `api-keys.yaml`, `routing-policy.yaml`, and `provider-free-tier.yaml` (when configured) are picked up when their file **modification time** changes (`gateway.yaml` reload also runs when `provider-free-tier.yaml` alone changes). **Gateway metrics** (the `metrics` block in `gateway.yaml`) are applied **at process start** only (changing paths requires a **restart**).

## Go gateway binary

The `chimera` program (`go build -o chimera ./cmd/chimera`) reads:

- **Config path:** `CHIMERA_GATEWAY_CONFIG`, or `-config /path/to/gateway.yaml`, or default `./config/gateway.yaml` (relative to the process working directory).
- **Listen address:** from `gateway.listen_host` and `gateway.listen_port`, unless overridden with `-listen` (e.g. `:3001` or `host:port`).
- **Log level:** `gateway.log_level` unless `LOG_LEVEL` is set (`debug`, `info`, `warn`, `error`); Go uses `log/slog` text logs on stdout.
- **Broker endpoint (YAML `upstream.*`):** `upstream.base_url`, `upstream.api_key_env`, `health.*`, `routing.fallback_chain`, `paths.*` — see tables below. Points at **chimera-broker** (or a standalone OpenAI-compatible proxy such as BiFrost during local dev).
- **`.env`:** At startup, the runtime loads an optional `.env` in the **process working directory** (via `github.com/joho/godotenv`). Missing file is normal when the environment is injected by your shell or service manager.

`GET /health` returns JSON including `checks.vectorstore` when RAG is enabled and `checks.upstream` (broker/backend probe). `GET /v1/models` prepends the virtual model id (`Chimera-<semver>`), then merges the **chimera-broker** catalog when available. `POST /v1/chat/completions` validates the gateway Bearer token, applies routing for the virtual model, and walks the fallback chain on 429/selected 5xx.

To run **chimera-broker** and **chimera-vectorstore** as supervised wrappers, use `chimera serve` or make target `chimera-supervisor-run` — see [supervisor.md](supervisor.md). BiFrost/Qdrant remain the typical local backends behind those wrappers.

## Environment variables

| Variable | Required | Description |
|----------|----------|-------------|
| `CHIMERA_BROKER_API_KEY` | Yes for typical BiFrost setups | Bearer token the gateway sends to its OpenAI-compatible upstream (`upstream.base_url`). Name is configurable via `upstream.api_key_env` in `gateway.yaml` (default `CHIMERA_BROKER_API_KEY`). For BiFrost without governance virtual keys, any non-empty placeholder works. |
| `LOG_LEVEL` | No | Log level for `log/slog`: `debug`, `info`, `warn`, `error`. Overrides `gateway.log_level` when set. |
| `CHIMERA_GATEWAY_CONFIG` | No | Path to `gateway.yaml`. Default `./config/gateway.yaml` on the host. |

Provider keys (`GROQ_API_KEY`, `GEMINI_API_KEY`, `OPENAI_API_KEY`, etc.) are **not** read by the gateway; **BiFrost** (`config/bifrost.config.json`) consumes them.

**Model listing (BiFrost):** `GET /v1/models` on BiFrost alone may return entries like `groq/*`. The gateway first calls BiFrost’s `GET /api/models?unfiltered=true&limit=500`, maps each `{ provider, name }` to an OpenAI-style id `provider/name`, then prepends the virtual model (`Chimera-<semver>`). If that route is missing, the gateway uses `GET /v1/models` only. See `scripts/list-bifrost-models.sh`.

## `config/gateway.yaml`

| Field | Description |
|-------|-------------|
| `gateway.semver` | Semantic version string used to build the virtual model id (`Chimera-<semver>`). |
| **`gateway.listen_port` / `listen_host`** | HTTP bind address. |
| `gateway.log_level` | Suggested log level (use `LOG_LEVEL` env for a simple override). |
| `upstream.base_url` | OpenAI-compatible upstream root (no trailing slash required), e.g. `http://127.0.0.1:8080` for local BiFrost. |
| `upstream.api_key_env` | Name of the process env var holding the upstream Bearer token (e.g. `CHIMERA_BROKER_API_KEY`; default `CHIMERA_BROKER_API_KEY`). |
| `health.upstream_url` | Optional explicit URL for `GET /health` upstream probe; default `{upstream.base_url}/health`. The probe sends `Authorization: Bearer` + that token when set (BiFrost’s `/health` is typically unauthenticated). Deprecated alias: `health.litellm_url`. |
| `health.timeout_ms` | Timeout for the upstream health request and for `GET /v1/models` upstream list (default **5000**). |
| `health.chat_timeout_ms` | Timeout for each upstream `POST /v1/chat/completions` attempt (default **300000**). |
| `paths.api_keys` | Path to `api-keys.yaml` (relative to `gateway.yaml`’s directory unless absolute). |
| `paths.routing_policy` | Path to `routing-policy.yaml`. |
| `paths.provider_free_tier` | Path to `provider-free-tier.yaml` (default `./provider-free-tier.yaml` next to `gateway.yaml`). |
| `paths.provider_model_limits` | Path to `provider-model-limits.yaml` (default `./provider-model-limits.yaml` next to `gateway.yaml`). Missing or empty file means **no enforcement**; invalid file is logged and the gateway starts with an empty spec. See `docs/plans/version-v0.1.1.md` §3.7. **Quota enforcement on chat** compares limits to live usage in the metrics DB — keep `metrics.enabled: true` (default) or limits are not applied even when the YAML is populated. |
| `routing.filter_free_tier_models` | When **true** and the allowlist file loads successfully, merged `GET /v1/models` lists only ids in both the upstream catalog and the allowlist; `POST /api/ui/routing/generate` (operator UI session) uses the same intersection. |
| `routing.fallback_chain` | Ordered upstream **model ids** for virtual-model requests (BiFrost: `provider/model`). On **429** / selected **5xx**, the gateway tries the next entry. |
| `metrics.enabled` | Default **true**. When **false**, the gateway does **not** open SQLite metrics or record upstream chat outcomes. |
| `metrics.sqlite_path` | SQLite database file for gateway metrics (relative to **`gateway.yaml`’s directory** unless absolute). Default `../data/gateway/metrics.sqlite`. |
| `metrics.migrations_dir` | Directory containing `NNNNNN_description.sql` migration files (default `../migrations/chimera-gateway/metrics`). Migrations run **once at startup**; see `docs/plans/version-v0.1.1.md` §3.6. |

Reload: change file and **save** (mtime update). On reload, if token or policy **paths** change, those stores are re-opened.

### Supervised file indexer (`indexer.supervised`)

Used by `chimera serve` and `locus-desktop`: optional supervision of `chimera-indexer` as a child process after BiFrost is healthy. The child gets `CHIMERA_GATEWAY_URL` and a single merged `--config` file; set `CHIMERA_GATEWAY_TOKEN` in the environment for `POST /v1/ingest`. Operator UI: `/ui/indexer` (GET/PUT config, append roots). Behavior and log slugs: **[indexer.md](indexer.md)** (supervised mode); process tree: **[supervisor.md](supervisor.md)**.

| Field | Description |
|-------|-------------|
| `indexer.supervised.enabled` | **true** → start `chimera-indexer` beside the gateway binary (or `indexer.supervised.bin` / `PATH`). Ignored unless `rag.enabled` is **true** or `start_when_rag_disabled` is **true**. |
| `indexer.supervised.log_json` | **true** → pass `--log-json` so the indexer writes structured JSON logs on stderr (same ring buffer as other supervised services; filter `/ui/logs` by source `indexer`). |
| `indexer.supervised.bin` | Optional explicit path to the `chimera-indexer` executable. Empty → resolve next to the gateway binary or `PATH`. |
| `indexer.supervised.config_path` | Path to the single merged config passed as `--config` (default `../data/gateway/indexer.supervised.yaml` relative to `gateway.yaml`’s directory). |
| `indexer.supervised.start_when_rag_disabled` | **true** → allow starting the supervised indexer when `rag.enabled` is **false** (default **false**). |

## `config/api-keys.yaml`

```yaml
api_keys:
  - label: optional-human-name
    secret: "secret-bearer-value"
    tenant_id: "tenant-slug"
```

- `secret` — must match the client’s `Authorization: Bearer` value exactly.
- `tenant_id` — carried in logs today; **v0.2+** RAG scopes by tenant.

## `config/provider-model-limits.yaml`

Operator-maintained ceilings (`rpm`, `rpd`, `tpm`, `tpd`) compared against live metrics by the gateway's admission guard (`internal/providerlimits`). See `docs/plans/version-v0.1.1.md` §3.7 for the full schema; short version:

```yaml
schema_version: 1
defaults:
  usage_day_timezone: UTC
providers:
  groq:
    usage_day_timezone: UTC  # IANA tz for rpd/tpd day boundaries (e.g. America/Los_Angeles for Gemini)
    rpm: 30
    rpd: 14400
    tpm: 6000
    models:
      groq/llama-3.3-70b-versatile:
        tpm: 12000
```

- Merge: **model > provider > defaults**. Unset fields mean **no enforcement** for that dimension at that layer; if every layer leaves a field unset, the gateway does not cap it.
- When a provider (or any of its models) sets `rpd` or `tpd`, the provider must be able to resolve an `usage_day_timezone` from its own block or `defaults` — otherwise startup rejects the file.
- Unknown fields are rejected at parse time; negative numbers and invalid IANA names are rejected.
- Config-only: **reloading requires a restart** today. A copy-paste starter lives at `config/provider-model-limits.example.yaml`.

## `config/provider-free-tier.yaml`

Operator-maintained allowlist of BiFrost `provider/model` ids (and optional `patterns`). See comments inside the file for `format_version`, `effective_date`, and editing rules.

**Reference snapshot (optional):** `make catalog-free` fetches [Groq rate limits](https://console.groq.com/docs/rate-limits) and [Gemini API pricing](https://ai.google.dev/gemini-api/docs/pricing), derives BiFrost-style ids, and writes `config/free-tier-catalog.snapshot.yaml` (gitignored by default). Use `INTERSECT=`_path_ to restrict lines to ids that fuzzy-match a catalog file: JSON or YAML with `data`.`id` (same shape as `GET /v1/models`, e.g. `config/catalog-available.snapshot.yaml` from `make catalog-available`). `make catalog-available` calls `GET /v1/models` on BiFrost (defaults `BIFROST_BASE_URL=http://127.0.0.1:8080`, optional `CHIMERA_BROKER_API_KEY`) and writes `config/catalog-available.snapshot.yaml`. These snapshots are **not** loaded by the gateway automatically; merge entries into `provider-free-tier.yaml` by hand if you want them enforced.

## `config/routing-policy.yaml`

| Field | Description |
|-------|-------------|
| `ambiguous_default_model` | Upstream model id used when **no rule** matches (*Model selection and routing policy · 2*). |
| `rules` | Ordered list. Each rule may set `when.min_message_chars` (compared to the **last user** message length). First match wins; `models[0]` is the **initial** upstream model. Every id should appear in `routing.fallback_chain`. |

**Operator UI:** with a valid session, `POST /api/ui/routing/generate` fetches the upstream model list, optionally applies the free-tier filter, then writes `routing-policy.yaml` and `routing.fallback_chain` in `gateway.yaml` only if both outputs validate.

## `config/bifrost.config.json`

BiFrost bootstrap file. Provider keys use `env.VAR` for secrets.

**Per-key `models`:** In BiFrost, an **empty** or **omitted** `models` list means the key may be used for **any** model for that provider (minus `blacklisted_models` if set). **`"models": ["*"]` is not a wildcard** — it is treated as the literal model name `*`, so chat requests for real model ids will fail with *no keys found that support model*. Use no `models` field (or `[]`) when you want full catalog access without enumerating models.

## Logging semantics

### Baseline (v0.1)

- **INFO**: each HTTP response (method, path, status, duration, redacted `Authorization` prefix).
- **INFO**: upstream chat probe summary (status, model, stream flag).
- **DEBUG**: routing rule match, config path resolution, reload events, upstream relay details.

### Correlation (v0.2.1+)

Structured logs may include `request_id` (middleware), `service`, chat `conversation_id` (header `X-Chimera-Conversation-Id` or gateway-generated), `principal_id`, and stable `msg` slugs on ingest/RAG/indexer paths. Ingest/indexer flows may carry `index_run_id`. Operator-facing breakdown: [plans/log-presentation-layer.plan.md](plans/log-presentation-layer.plan.md).
