# chimera-indexer

`chimera-indexer` is the workspace file indexer that ships alongside the Chimera
Gateway v0.2+. It walks configured directory roots, applies ignore rules,
hashes each file, and sends bytes via `POST /v1/ingest` (small/medium files)
or the **chunked session API** when the file is larger than `max_whole_file_bytes`
from `GET /v1/indexer/config` (see gateway `rag.ingest.max_whole_file_bytes`).
The gateway
chunks, embeds, and writes vectors to Qdrant, so the indexer never embeds or
chunks locally.

**As-built contracts:** [features/indexer.md](features/indexer.md), [indexer-workspaces](features/indexer-workspaces.md), [indexer-ingest-pipeline](features/indexer-ingest-pipeline.md). Delivery history: [plans/indexer.md](plans/indexer.md). This document is the operator quick start.

## Supervised mode (`chimera-supervisor` / desktop)

When `indexer.supervised.enabled: true` is set in `config/gateway.yaml`
(and RAG is enabled, or `start_when_rag_disabled: true`), `chimera-supervisor`
starts `chimera-indexer` as a child process after the broker is healthy. The child
inherits the parent environment and receives `CHIMERA_GATEWAY_URL` pointing
at this gateway instance; set `CHIMERA_GATEWAY_TOKEN` in the environment so
ingest can authenticate.

- **Single config file:** `indexer.supervised.config_path` (default:
  `../data/gateway/indexer.supervised.yaml` relative to `gateway.yaml`) is
  passed as `--config` (highest merge layer). The file holds **indexer tuning
  only** (timeouts, workers, ignores, and so on). **Watch directories** are
  **not** read from YAML `roots:` in supervised mode; they come from the
  gateway **`GET /v1/indexer/workspaces`** (operator SQLite), managed in the
  settings UI (`/ui/settings`). When the supervised file changes on disk, the supervised
  **`chimera-indexer` process detects the edit** (debounced), stops the
  current watcher session, and **starts a new session**—**no full desktop
  restart**. If the indexer binary is stale, or other gateway settings
  change, you still restart **`chimera-supervisor`/desktop**.
- **Standalone `chimera-indexer`** (no `--config`): unchanged—roots come from
  merged YAML and optional `--root` as before.
- **Logs:** stderr/stdout are teed into the same ring buffer as BiFrost/Qdrant;
  open `/ui/settings` and filter source `indexer`.
- **Structured stderr:** supervised indexer passes `--log-json` by default (JSON **slog** on stderr). Set `indexer.supervised.log_json: false` to opt out.

### Structured operator logs (`--log-json`)

After a successful `GET /v1/indexer/config`, the logger adds `tenant_id`, `principal_id` (same as tenant), and `user_label` to **all** structured lines via `log.With`, plus `indexer_key` when **every watched root resolves to the same** ingest `indexer_target_key`; if roots map to **multiple** projects/flavors, `indexer_multi_target` is set instead (**no** single `indexer_key`) so `/ui/settings` can split cards using `root_scopes` / job lines. Every line carries `index_run_id` and `service":"indexer"`. Stable slug `msg` for grouping: see [structured-operator-log-lines](features/structured-operator-log-lines.md).

| `msg` slug | When | Notable fields |
|------------|------|----------------|
| `indexer.run.start` | Process start (after config fetch when possible) | `roots`, `root_ids`, `watch_root_paths`, `root_scopes` (JSON array per root: `path`, `ingest_project`, `flavor_id`, `indexer_target_key`, workspace); legacy top-level `ingest_project` / scope fields mirror **YAML default scope** only (`defaults:` null → often empty — use `root_scopes` in `/ui/settings`) |
| `gateway.indexer.config` | After successful `GET /v1/indexer/config` | `gateway_version`, `embedding_model`, `chunk_size`, …; optional `ingest_project` / `flavor_id` from request headers; `defaults_project_id` / `defaults_flavor_id` from gateway `defaults` (helps `/ui/settings` when YAML scope is empty) |
| `indexer.storage.stats` | Periodic or one-shot (`GET /v1/indexer/storage/stats`) | `collection`, `qdrant_points`, `vector_dim`, `available`, optional `detail` / `err` |
| `indexer.state` | Same cadence as storage stats polling (watch mode) or one-shot at exit | `state` (`watch_idle`, `backlog`, `uploading`, `recovery`, `initial_scanning`, `idle`), `queue_depth`, `ingest_inflight`, `initial_scan_complete`, `watch_mode`, `recovery`, `qdrant_points_reported` |
| `indexer.reconcile.summary` | Corpus inventory loaded | `phase`=`inventory_loaded`, `remote_source_paths` |
| `indexer.discovery.summary` | After initial walk of all roots | `candidates_*`, `files_excluded_by_ignore_rules` (same count as `skipped_ignored`), `skipped_ignored_files`, `skipped_ignored_dirs`, other `skipped_*` |
| `indexer.queue.snapshot` | Run workers start/exit, after initial scan, pause/resume, **`phase`=`worker_drain_tick`** (immediate once + every 30s while draining) | `queue_depth`, `queue_cap`, `workers`, counters below |
| `indexer.run.progress` | Milestone (unchanged) | e.g. `phase`=`initial_scan`, `candidates_enqueued` |
| `indexer.retry.scheduled` | Before backoff sleep | `rel`, `attempt`, `max_attempts`, `delay_ms`, `err`, plus scope: `tenant_id`, `project_id`, `ingest_project`, `flavor_id`, `indexer_target_key`, `root` |
| `indexer.recovery.poll` | Each recovery poll while waiting (WARN; `poll_n` at DEBUG) | `embed_ok`, `embed_reason_code`, `storage_ok`, optional health detail fields |
| `indexer.recovery.resumed` | Ingest ready (vector store + embedding) again | (human line; pairs with recovery polls) |
| `indexer.ingest.gate.closed` | Global ingest pause opened | `reason_code`, `detail`, `queue_depth` |
| `indexer.ingest.gate.open` | Global ingest pause cleared | `embed_model`, `queue_depth` |
| `indexer.worker.paused` | Worker entering recovery | `worker`, `rel`, `work_kind`, plus scope: `tenant_id`, `project_id`, `ingest_project`, `flavor_id`, `indexer_target_key`, `root` (ingest only) |
| `indexer.scope.status` | Per-scope progress (edge-triggered on change + ~45s heartbeat fallback) | `change_reason` (`heartbeat` on periodic lines), `declarative_state`, queue/workspace counts, optional `current_rel`, `ingest_gate_closed`, `ingest_gate_reason_code`, `embed_reason_code`, `in_recovery`, `ingest_completed`, plus scope fields |
| `indexer.scope.active_file` | DEBUG — file entering upload path (after skip checks) | `rel`, `worker`, plus scope fields |
| `indexer.job.skipped` | Skipped before upload (unchanged / empty text) | `rel`, `skip_reason`, plus scope fields (DEBUG when `job_skip_log: debug`) |
| `indexer.job.skipped.summary` | Batched skip/ingest rollup per scope (default ~every 5s while active) | `window_ms`, skip/ingest deltas, `queue_depth`, plus scope fields |
| `indexer.job.upload` | About to call gateway ingest (whole or chunked) | `rel`, `bytes`, `transport` (`whole` \| `chunked`), plus scope fields |
| `indexer.job.ingested` | Successful ingest | `rel`, `mode`, `chunks`, `collection`, `content_sha256`, plus the same **scope** fields |
| `indexer.job.failed` | Dropped after non-pause failure | `worker`, `rel`, `err`, plus the same **scope** fields |
| `indexer.sync_state.write_failed` | Local sync-state DB write failed after ingest | `rel`, `err`, plus the same **scope** fields |
| `indexer.skip.empty_or_whitespace` | DEBUG only when `job_skip_log: debug` (or legacy `verbose_job_logs: false`) — empty file skipped | `rel`, plus scope fields (same as `indexer.job.skipped` with `skip_reason`=`empty_or_whitespace`) |
| `indexer.skip.unchanged_corpus_client_hash` | DEBUG, unchanged vs corpus client hash | `rel`, plus scope fields |
| `indexer.skip.unchanged_corpus_sync` | DEBUG, unchanged vs corpus + local sync | `rel`, plus scope fields |
| `indexer.skip.unchanged_local_sync` | DEBUG, unchanged vs local sync state only | `rel`, plus scope fields |
| `indexer.fanout.enqueue_failed` | Fan-out chunk could not be queued | `candidates` (chunk size); one scope: same **scope** fields; **multi-scope** chunk: `indexer_multi_scope_chunk`, `distinct_scope_count`, `indexer_target_keys` (comma-separated) |
| `indexer.fanout.remainder_blocked` | Queue full requeueing fan-out remainder | `remainder_size`; scope fields as for `indexer.fanout.enqueue_failed` |
| `indexer.work.failed` | Non-ingest work dropped (e.g. fan-out) | `worker`, `kind`, `err`; for **fan-out** list items, same multi-scope **scope** fields as `indexer.fanout.enqueue_failed` when candidates are present |
| `indexer.run.done` | One-shot or watch exit | `mode`, `ingest_completed`, `ingest_failed_dropped`, `retry_events`, `jobs_dequeued`, `skip_unchanged_*`, `skip_empty_or_whitespace` |

### Log profiles (operator vs trace)

| Profile | Settings | INFO lines |
|---------|----------|------------|
| **Operator (supervised default)** | `log_level: info`, `job_skip_log: debug` | Run lifecycle, errors, `indexer.job.ingested`, `indexer.job.skipped.summary` rollups (~5s while draining), gate/recovery — not per-file skips |
| **Trace** | `log_level: debug`, `job_skip_log: info` | Above plus DEBUG `indexer.scope.active_file`, per-file `indexer.job.skipped` / `indexer.job.upload` |

Disable batched summaries with `skip_summary_min_interval_ms: -1` in indexer YAML.

### Stderr level (`log_level`)

Indexer YAML may set `log_level` to `debug`, `info`, `warn`, or `error`
(minimum level for **stderr** `slog` output from `chimera-indexer`). When unset, it
defaults to `info`. `chimera-indexer --log-level …` overrides YAML for the
process (same values).

### Per-file skip / upload lines (`job_skip_log`)

Indexer YAML may set `job_skip_log` to `info`, `debug`, or `off`:

| Value | Effect |
|-------|--------|
| `info` | `indexer.job.skipped` (INFO) for skips; `indexer.job.upload` (INFO) before ingest; DEBUG `indexer.skip.*` lines still apply when the process log level is DEBUG. |
| `debug` (supervised default) | No INFO `indexer.job.skipped` / `indexer.job.upload`; DEBUG `indexer.skip.*` and `indexer.scope.active_file` when log level is DEBUG; use `indexer.job.skipped.summary` at INFO for rollups. |
| `off` | No per-file skip or pre-upload INFO lines (queue snapshots and `indexer.job.ingested` / errors unchanged). |

**Legacy:** `verbose_job_logs` (bool) is deprecated. When `job_skip_log` is
unset after merge: **`true` → info**, **`false` → debug** (matching the old default
where absent meant verbose INFO lines).

- **Desktop folder picker:** the native shell binds `window.chimeraPickFolder`
  (WebView + `dlgs` folder dialog); the Workspaces section on `/ui/settings` calls it
  (iframe uses `window.top.chimeraPickFolder`).

**Binary:** place `chimera-indexer` next to the **same executable** that runs
supervision (`chimera`, `locus-desktop`, etc.—see `executableDir` in
`cmd/chimera/serve_defaults.go`), or set `indexer.supervised.bin`, or ensure
it is on `PATH`. After `make chimera-indexer-build`, restart `chimera-supervisor`
or desktop so the child process is started again; otherwise you may still be
running an older `chimera-indexer` on disk.

## Install / build

```sh
make chimera-indexer-build   # produces ./chimera-indexer[.exe]
make chimera-indexer-install # go install into $GOBIN
```

## Environment

| Variable                | Purpose                                    |
|-------------------------|--------------------------------------------|
| `CHIMERA_GATEWAY_URL`   | Base URL of a running Chimera Gateway      |
| `CHIMERA_GATEWAY_TOKEN` | Bearer token (required; never store in YAML) |

## Configuration

`chimera-indexer` loads **YAML config in layers** (each file optional except when
you pass `--config`, which must exist). Merge order (lowest → highest):

1. `~/.locus/indexer.config.yaml` (user-wide; `os.UserHomeDir()` / Windows `%USERPROFILE%`)
2. `<cwd>/.locus/indexer.config.yaml` (project-local)
3. `--config path` when set (highest among files)

Later files override earlier ones for the same keys (see `MergeFileConfig` in
`internal/indexer/config.go`). You can run with **only** layers (1)+(2) and no
`--config`, or add `--config` for an extra overlay. A starter overlay lives
at [`config/indexer.example.yaml`](../config/indexer.example.yaml).

After merged YAML: **environment** (`CHIMERA_GATEWAY_URL`) overrides
`gateway_url`; **CLI** `--gateway-url` and `--root` override merged YAML for
those fields; `--log-level` overrides `log_level`. `CHIMERA_GATEWAY_TOKEN` is always from the environment (never
YAML).

```yaml
# Chimera Gateway base URL (default listen_port is 3000 in config/gateway.yaml).
# Do not point this at BiFrost (8080) — chimera-indexer talks to the gateway.
gateway_url: "http://127.0.0.1:3000"
roots:
  - "."
ignore_extra:
  - "tmp/"
  - "*.snapshot"
```

`tenant_id` is implied by the bearer token JSON response also returns `tenant_id`, `user_label` (from `api-keys.yaml` / UI label), `principal_id` (same id as `tenant_id`) for indexer operator logs. **Phase 3** adds optional
`defaults`, per-root, and per-glob `project_id` / `flavor_id` / `workspace_id`
in YAML. They are merged in order **defaults → root → overrides** (each
`overrides[]` glob that matches the file’s root-relative path applies on top;
later list entries win for the same field). Values are sent on every ingest as
`X-Chimera-Project` and `X-Chimera-Flavor-Id`, and the merged **defaults** are
also sent on `GET /v1/indexer/config` at startup. Match the same headers (or
Continue `config.yaml` project/flavor fields) as chat so RAG queries the same
Qdrant collection the indexer wrote to.

**Phase 4:** Successful ingests record **client** and **server** SHA-256 digests under
`sync_state_path`. When omitted: **`indexer.sync-state.json` next to the `--config`
file** if you pass `--config` (e.g. supervised `data/gateway/indexer.sync-state.json`
alongside `indexer.supervised.yaml`), otherwise `.locus/indexer.sync-state.json`
under the process working directory. If a file’s client hash matches the last
recorded value, the indexer **skips** re-upload.
Gateway responses include `content_sha256` (authoritative over UTF-8 text
bytes ingested). Optional YAML: `max_whole_file_bytes` (caps whole-body mode
when lower than the gateway), `sync_state_path`.

**Chunked ingest (large files):** each session step (**start session**, **PUT
chunk**, **complete**) retries transient errors with the same backoff settings
as whole-file ingest (`retry_max_attempts`, `retry_base_delay_ms`,
`retry_max_delay_ms`).

## Corpus inventory (reconciliation)

`GET /v1/indexer/corpus/inventory` (Bearer token; same `X-Chimera-Project`
/`X-Chimera-Flavor-Id` headers as ingest) returns **deduplicated** sources
for the scoped Qdrant collection. Query params:

- `limit` — max points to scan per page (default **256**, max **2000**).
- `cursor` — opaque value from the previous response’s `next_cursor`
  (omit on the first page).

Response JSON includes `entries[]` with `source`, `content_sha256`
(server digest over UTF-8 file bytes), optional `client_content_hash`
(indexer-supplied `content_hash` when present), plus `has_more` and
`next_cursor`. The gateway advertises the path on `GET /v1/indexer/config`
as `corpus_inventory_path`.

`chimera-indexer` loads all pages during the initial scan (after
`GET /v1/indexer/config`) and skips files whose **client** hash matches the
inventory when `client_content_hash` is set, or falls back to **sync state +
server SHA** when only server digests exist on older points.

## Ignore rules

The matcher is a layered gitignore-style engine that combines:

1. Built-in defaults (`.git/`, `node_modules/`, `*.bin`, `*.png`, secrets,
   etc.).
2. `ignore_extra` from the YAML config.
3. `.locusignore` at each root (created by you).
4. `.gitignore` at each root.

Binary files are also excluded via a NUL-byte sniff over the first ~8 KB.

## Failure handling

Per [§ Failure handling](plans/indexer.plan.md#failure-handling-normative):

- Retry transient failures (`5xx`, `408`, `425`, `429`, network errors) with
  bounded exponential backoff (`retry_max_attempts`, `retry_base_delay_ms`,
  `retry_max_delay_ms`; defaults 5 / 500 ms / 30 s).
- After the last retry, the worker pauses and polls
  `GET /v1/indexer/storage/health` every `recovery_poll_interval_ms`
  (default 30 s). By default it **also** requires `GET /health` to report
  readiness (non-`503`, no `degraded` in JSON). Set `recovery_include_root_health: false`
  in YAML to only use storage health.
- `401`/`403` responses are treated as fatal and surfaced in logs without
  retry.

### Gateway storage health (`GET /v1/indexer/storage/health`)

The gateway returns HTTP **200** for degraded dependency checks (vector store
and/or embedding unavailable) so the indexer can poll without treating the JSON
body as a transport error. Auth failures and RAG-disabled responses remain **503**
with the structured `{"error":{...}}` shape.

Top-level `ok` is `true` only when **both** checks required for ingest succeed:
vector store reachability and configured embedding model availability.

| Field | Meaning |
|-------|---------|
| `checks.vectorstore.ok` | Qdrant / vector store probe (`StoreHealth`) |
| `checks.embedding.ok` | Configured embedding model is present in a fresh chimera-broker `/v1/models` snapshot and its provider is usable |
| `checks.embedding.model` | Configured embedding model id from gateway RAG config |
| `checks.embedding.model_in_catalog` | Whether the model appears in the live catalog |
| `checks.embedding.provider` | Provider prefix from the model id (e.g. `ollama`) |
| `checks.embedding.provider_state` | Classified provider liveness (`up`, `down`, `key_missing`, …) |
| `checks.embedding.reason_code` | Stable machine-readable reason when `checks.embedding.ok` is false |
| `checks.embedding.detail` | Human-readable detail (transport error, catalog miss, etc.) |

**Embedding `reason_code` values:**

| Code | When |
|------|------|
| `vectorstore_unreachable` | Vector store probe failed (`checks.vectorstore`) |
| `embed_model_not_in_catalog` | Configured model absent from fresh catalog |
| `embed_provider_down` | Provider classified unavailable |
| `embed_provider_key_missing` | Provider missing required API key / config |
| `embed_catalog_stale` | Catalog snapshot missing or older than freshness window |

The indexer client exposes helpers on `HealthStatus`: `IngestReady()`,
`VectorstoreOK()`, `EmbedOK()`, `ReasonCode()`, and `HealthDetail()`. Recovery
poll logs (`indexer.recovery.poll`) include embed fields.

**Phase 2 — ingest gate.** When health reports not ingest-ready, the indexer
opens a process-wide **ingest gate** so workers block before dequeuing ingest
jobs. Gate transitions log once at INFO:

| `msg` | When |
|-------|------|
| `indexer.ingest.gate.closed` | Ingest paused (`reason_code`, `detail`, `queue_depth`) |
| `indexer.ingest.gate.open` | Ingest resumed (`embed_model`, `queue_depth`) |

Recovery waits for `IngestReady()` (vector store **and** embedding). After the
first **502/503 embed-classified** ingest failure, `retry_short_circuit_on_embed`
(default **true**) skips remaining per-file retries and closes the gate instead
of burning `retry_max_attempts`.

## Modes

```sh
chimera-indexer --config .locus/indexer.config.yaml               # watch + ingest
chimera-indexer --config .locus/indexer.config.yaml --one-shot    # scan + exit
chimera-indexer --root ./apps/web --gateway-url http://x:8080     # flag-only
```

In watch mode the indexer drains an initial scan, then incrementally ingests
files reported by `fsnotify` (debounced to coalesce save bursts; default
debounce 750 ms).

## Security notes

- `source` paths sent on the wire are always **relative to the configured
  root**. Absolute host paths are never transmitted.
- Symlinks are not followed (no toggle in Phase 2).
- Tokens stay in the environment; YAML never contains secrets in supported releases.
