# claudia-index (v0.5)

`claudia-index` is the workspace file indexer that ships alongside the Claudia
Gateway v0.2+. It walks configured directory roots, applies ignore rules,
hashes each file, and sends bytes via `POST /v1/ingest` (small/medium files)
or the **chunked session API** when the file is larger than `max_whole_file_bytes`
from `GET /v1/indexer/config` (see gateway `rag.ingest.max_whole_file_bytes`).
The gateway
chunks, embeds, and writes vectors to Qdrant, so the indexer never embeds or
chunks locally.

See [`plans/indexer.plan.md`](plans/indexer.plan.md) for the full product plan and
non-goals; this document is the operator-facing quick start.

## Supervised mode (`claudia serve` / desktop) — indexer plan v0.5 / gateway v0.2.2+

When `indexer.supervised.enabled: true` is set in `config/gateway.yaml`
(and RAG is enabled, or `start_when_rag_disabled: true`), `claudia serve`
starts `claudia-index` as a child process after BiFrost is healthy. The child
inherits the parent environment and receives `CLAUDIA_GATEWAY_URL` pointing
at this gateway instance; set `CLAUDIA_GATEWAY_TOKEN` in the environment so
ingest can authenticate.

- **Single config file:** `indexer.supervised.config_path` (default:
  `../data/gateway/indexer.supervised.yaml` relative to `gateway.yaml`) is
  passed as `--config` (highest merge layer). The operator UI exposes it at
  `/ui/indexer` (session cookie) with **GET/PUT `/api/ui/indexer/config`** and
  **POST `/api/ui/indexer/append-root`**. When that file changes on disk, the
  supervised **`claudia-index` process detects the edit** (debounced), stops the
  current watcher session, and **starts a new session** loaded from YAML—**no full
  desktop restart**. If the indexer binary is stale, or other gateway settings
  change, you still restart **`claudia serve`/desktop**.
- **Logs:** stderr/stdout are teed into the same ring buffer as BiFrost/Qdrant;
  open `/ui/logs` and filter source `indexer`.
- **Structured stderr:** enable `indexer.supervised.log_json: true` to add
  `--log-json` (JSON **slog** on stderr).

### Structured operator logs (`--log-json`)

After a successful `GET /v1/indexer/config`, the logger adds `tenant_id`, `principal_id` (same as tenant), and `user_label` to **all** structured lines via `log.With`, plus `indexer_key` when **every watched root resolves to the same** ingest `indexer_target_key`; if roots map to **multiple** projects/flavors, `indexer_multi_target` is set instead (**no** single `indexer_key`) so `/ui/logs` can split cards using `root_scopes` / job lines. Every line carries `index_run_id` and `service":"indexer"`. Stable slug `msg` for grouping: see [`plans/log-presentation-layer.plan.md`](plans/log-presentation-layer.plan.md).

| `msg` slug | When | Notable fields |
|------------|------|----------------|
| `indexer.run.start` | Process start (after config fetch when possible) | `roots`, `root_ids`, `watch_root_paths`, `root_scopes` (JSON array per root: `path`, `ingest_project`, `flavor_id`, `indexer_target_key`, workspace); legacy top-level `ingest_project` / scope fields mirror **YAML default scope** only (`defaults:` null → often empty — use `root_scopes` in `/ui/logs`) |
| `gateway.indexer.config` | After successful `GET /v1/indexer/config` | `gateway_version`, `embedding_model`, `chunk_size`, …; optional `ingest_project` / `flavor_id` from request headers; `defaults_project_id` / `defaults_flavor_id` from gateway `defaults` (helps `/ui/logs` when YAML scope is empty) |
| `indexer.storage.stats` | Periodic or one-shot (`GET /v1/indexer/storage/stats`) | `collection`, `qdrant_points`, `vector_dim`, `available`, optional `detail` / `err` |
| `indexer.state` | Same cadence as storage stats polling (watch mode) or one-shot at exit | `state` (`watch_idle`, `backlog`, `uploading`, `recovery`, `initial_scanning`, `idle`), `queue_depth`, `ingest_inflight`, `initial_scan_complete`, `watch_mode`, `recovery`, `qdrant_points_reported` |
| `indexer.reconcile.summary` | Corpus inventory loaded | `phase`=`inventory_loaded`, `remote_source_paths` |
| `indexer.discovery.summary` | After initial walk of all roots | `candidates_*`, `files_excluded_by_ignore_rules` (same count as `skipped_ignored`), `skipped_ignored_files`, `skipped_ignored_dirs`, other `skipped_*` |
| `indexer.queue.snapshot` | Run workers start/exit, after initial scan, pause/resume, **`phase`=`worker_drain_tick`** (immediate once + every 30s while draining) | `queue_depth`, `queue_cap`, `workers`, counters below |
| `indexer.run.progress` | Milestone (unchanged) | e.g. `phase`=`initial_scan`, `candidates_enqueued` |
| `indexer.retry.scheduled` | Before backoff sleep | `rel`, `attempt`, `max_attempts`, `delay_ms`, `err`, plus scope: `tenant_id`, `project_id`, `ingest_project`, `flavor_id`, `indexer_target_key`, `root` |
| `indexer.recovery.poll` | Each recovery poll tick | `poll_n`, `interval_ms`, `storage_ok`, `rag_disabled`, optional `root_health_ok` |
| `indexer.recovery.resumed` | Storage (and root health if enabled) OK again | (human line; pairs with recovery polls) |
| `indexer.worker.paused` | Worker entering recovery | `worker`, `rel`, `work_kind`, plus scope: `tenant_id`, `project_id`, `ingest_project`, `flavor_id`, `indexer_target_key`, `root` (ingest only) |
| `indexer.job.skipped` | Skipped before upload (unchanged / empty text) | `rel`, `skip_reason` — one of `empty_or_whitespace`, `unchanged_corpus_client_hash`, `unchanged_corpus_sync`, `unchanged_local_sync`, plus the same **scope** fields as `indexer.retry.scheduled` |
| `indexer.job.upload` | About to call gateway ingest (whole or chunked) | `rel`, `bytes`, `transport` (`whole` \| `chunked`), plus the same **scope** fields (includes `ingest_project`, `flavor_id`, `root`, …) |
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
| `indexer.run.done` | One-shot or watch exit | `mode`, `ingest_completed`, `ingest_failed_dropped`, `retry_events`, `jobs_dequeued`, `skip_unchanged_*` |

### Stderr level (`log_level`)

Indexer YAML may set `log_level` to `debug`, `info`, `warn`, or `error`
(minimum level for **stderr** `slog` output from `claudia-index`). When unset, it
defaults to `info`. `claudia-index --log-level …` overrides YAML for the
process (same values).

### Per-file skip / upload lines (`job_skip_log`)

Indexer YAML may set `job_skip_log` to `info`, `debug`, or `off`:

| Value | Effect |
|-------|--------|
| `info` (default) | `indexer.job.skipped` (INFO) for skips; `indexer.job.upload` (INFO) before ingest; DEBUG `indexer.skip.*` lines still apply when the process log level is DEBUG. |
| `debug` | No INFO `indexer.job.skipped` / `indexer.job.upload`; DEBUG `indexer.skip.*` only (same as legacy `verbose_job_logs: false`). |
| `off` | No per-file skip or pre-upload INFO lines (queue snapshots and `indexer.job.ingested` / errors unchanged). |

**Legacy:** `verbose_job_logs` (bool) is deprecated. When `job_skip_log` is
unset after merge: **`true` → info**, **`false` → debug** (matching the old default
where absent meant verbose INFO lines).

- **Desktop folder picker:** the native shell binds `window.claudiaPickFolder`
  (WebView + `dlgs` folder dialog); the Indexer tab calls it from `/ui/indexer`
  (iframe uses `window.top.claudiaPickFolder`).

**Binary:** place `claudia-index` next to the **same executable** that runs
supervision (`claudia`, `claudia-desktop`, etc.—see `executableDir` in
`cmd/claudia/serve_defaults.go`), or set `indexer.supervised.bin`, or ensure
it is on `PATH`. After `make indexer-build`, restart `claudia serve`
or desktop so the child process is started again; otherwise you may still be
running an older `claudia-index` on disk.

## Install / build

```sh
make indexer-build   # produces ./claudia-index[.exe]
make indexer-install # go install into $GOBIN
```

## Environment

| Variable                | Purpose                                    |
|-------------------------|--------------------------------------------|
| `CLAUDIA_GATEWAY_URL`   | Base URL of a running Claudia Gateway      |
| `CLAUDIA_GATEWAY_TOKEN` | Bearer token (required; never store in YAML) |

## Configuration

`claudia-index` loads **YAML config in layers** (each file optional except when
you pass `--config`, which must exist). Merge order (lowest → highest):

1. `~/.claudia/indexer.config.yaml` (user-wide; `os.UserHomeDir()` / Windows `%USERPROFILE%`)
2. `<cwd>/.claudia/indexer.config.yaml` (project-local)
3. `--config path` when set (highest among files)

Later files override earlier ones for the same keys (see `MergeFileConfig` in
`internal/indexer/config.go`). You can run with **only** layers (1)+(2) and no
`--config`, or add `--config` for an extra overlay. A starter overlay lives
at [`config/indexer.example.yaml`](../config/indexer.example.yaml).

After merged YAML: **environment** (`CLAUDIA_GATEWAY_URL`) overrides
`gateway_url`; **CLI** `--gateway-url` and `--root` override merged YAML for
those fields; `--log-level` overrides `log_level`. `CLAUDIA_GATEWAY_TOKEN` is always from the environment (never
YAML).

```yaml
# Claudia Gateway base URL (default listen_port is 3000 in config/gateway.yaml).
# Do not point this at BiFrost (8080) — claudia-index talks to the gateway.
gateway_url: "http://127.0.0.1:3000"
roots:
  - "."
ignore_extra:
  - "tmp/"
  - "*.snapshot"
```

`tenant_id` is implied by the bearer token JSON response also returns `tenant_id`, `user_label` (from `tokens.yaml` / UI label), `principal_id` (same id as `tenant_id`) for indexer operator logs. **v0.3** adds optional
`defaults`, per-root, and per-glob `project_id` / `flavor_id` / `workspace_id`
in YAML. They are merged in order **defaults → root → overrides** (each
`overrides[]` glob that matches the file’s root-relative path applies on top;
later list entries win for the same field). Values are sent on every ingest as
`X-Claudia-Project` and `X-Claudia-Flavor-Id`, and the merged **defaults** are
also sent on `GET /v1/indexer/config` at startup. Match the same headers (or
Continue `config.yaml` project/flavor fields) as chat so RAG queries the same
Qdrant collection the indexer wrote to.

**v0.4:** Successful ingests record **client** and **server** SHA-256 digests under
`sync_state_path`. When omitted: **`indexer.sync-state.json` next to the `--config`
file** if you pass `--config` (e.g. supervised `data/gateway/indexer.sync-state.json`
alongside `indexer.supervised.yaml`), otherwise `.claudia/indexer.sync-state.json`
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

`GET /v1/indexer/corpus/inventory` (Bearer token; same `X-Claudia-Project`
/`X-Claudia-Flavor-Id` headers as ingest) returns **deduplicated** sources
for the scoped Qdrant collection. Query params:

- `limit` — max points to scan per page (default **256**, max **2000**).
- `cursor` — opaque value from the previous response’s `next_cursor`
  (omit on the first page).

Response JSON includes `entries[]` with `source`, `content_sha256`
(server digest over UTF-8 file bytes), optional `client_content_hash`
(indexer-supplied `content_hash` when present), plus `has_more` and
`next_cursor`. The gateway advertises the path on `GET /v1/indexer/config`
as `corpus_inventory_path`.

`claudia-index` loads all pages during the initial scan (after
`GET /v1/indexer/config`) and skips files whose **client** hash matches the
inventory when `client_content_hash` is set, or falls back to **sync state +
server SHA** when only server digests exist on older points.

## Ignore rules

The matcher is a layered gitignore-style engine that combines:

1. Built-in defaults (`.git/`, `node_modules/`, `*.bin`, `*.png`, secrets,
   etc.).
2. `ignore_extra` from the YAML config.
3. `.claudiaignore` at each root (created by you).
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

## Modes

```sh
claudia-index --config .claudia/indexer.config.yaml          # watch + ingest
claudia-index --config .claudia/indexer.config.yaml --one-shot  # scan + exit
claudia-index --root ./apps/web --gateway-url http://x:8080  # flag-only
```

In watch mode the indexer drains an initial scan, then incrementally ingests
files reported by `fsnotify` (debounced to coalesce save bursts; default
debounce 750 ms).

## Security notes

- `source` paths sent on the wire are always **relative to the configured
  root**. Absolute host paths are never transmitted.
- Symlinks are not followed (no toggle in v0.2).
- Tokens stay in the environment; YAML never contains secrets in supported releases.
