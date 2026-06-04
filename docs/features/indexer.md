# Feature: Workspace file indexer (`chimera-indexer`)

| Field | Value |
|-------|-------|
| **Doc kind** | `feature-record` |
| **Areas** | `chimera-indexer`, gateway RAG ingest, supervised stack, operator settings UI |
| **Status** | `current` |
| **Introduced** | Gateway v0.2+ (Phases 2–6); supervised workspaces and queue refactor in later minors |
| **Originated from** | [`plans/indexer.md`](../plans/indexer.md) |
| **Related features** | [Indexer workspaces](indexer-workspaces.md), [Indexer ingest pipeline](indexer-ingest-pipeline.md), [Indexer health and operator logs](indexer-health-and-operator-logs.md) |
| **Depends on** | [Gateway RAG ingest and retrieval](gateway-rag-ingest-and-retrieval.md), Bearer token auth, supervised stack |
| **Last updated** | See git history |

## At a glance

`chimera-indexer` is a portable Go binary that watches configured directory roots, applies ignore rules, hashes files, and sends content to the **Chimera gateway** for server-side chunking and embedding. The indexer never embeds locally. In supervised mode (`chimera-supervisor` / desktop), the gateway starts the indexer as a child process, tees its JSON logs into the operator log buffer, and supplies **watch directories from operator SQLite** (not YAML `roots:`). Standalone runs still merge layered YAML config and optional `--root` flags. Large files use a chunked session API; smaller files use whole-body `POST /v1/ingest`. Absolute host paths never leave the machine on the wire—only root-relative `source` paths are transmitted.

## Operator-visible behavior

- **Workspaces** — On `/ui/settings`, operators create workspaces (project + flavor + one or more folder paths). The desktop shell exposes a native folder picker (`chimeraPickFolder`). Saved rows live in operator SQLite; CRUD does not rewrite `indexer.supervised.yaml`.
- **Supervised tuning** — `indexer.supervised.yaml` holds timeouts, workers, ignore extras, log level, and similar tuning. Editing that file hot-reloads the indexer session without restarting the whole desktop stack (unless the binary itself is stale).
- **Logs** — Filter source `indexer` on `/ui/settings` to see structured progress: run lifecycle, per-scope status, ingest summaries, recovery when embedding or vector storage is down.
- **Standalone** — Run `chimera-indexer` with layered YAML (`~/.locus/indexer.config.yaml`, project-local, optional `--config`) and `CHIMERA_GATEWAY_URL` / `CHIMERA_GATEWAY_TOKEN` in the environment.

Install, env vars, YAML keys, and the full structured log slug table remain in the operator guide [`docs/indexer.md`](../indexer.md).

## System behavior and contracts

**Invariants**

- **Gateway owns chunking and embedding** — one logical file per ingest; chunk boundaries can change without an indexer upgrade.
- **Relative paths only on the wire** — `source` is always relative to the configured root; no absolute host paths in HTTP bodies.
- **Symlinks are not followed** — YAML may expose `follow_symlinks`, but `Resolve` forces it off.
- **Tokens never in YAML** — `CHIMERA_GATEWAY_TOKEN` comes from the environment only.
- **Tenant scope from Bearer token** — same token model as chat ingest; optional `X-Chimera-Project` / `X-Chimera-Flavor-Id` per root or glob override.
- **Supervised roots are API-only** — with `--config` under supervision, effective watch roots come **only** from `GET /v1/indexer/workspaces`; YAML `roots:` is ignored.
- **Indexer does not open operator SQLite** — workspace data is always fetched over HTTP from the gateway.

**Decisions**

| Topic | Decision |
|-------|----------|
| Ingest unit | Whole file (Mode A) under `max_whole_file_bytes`; session + ordered chunks (Mode B) above threshold |
| Content hash | Client SHA-256 for skip detection; gateway returns authoritative `content_sha256` over ingested UTF-8 bytes; local sync state prefers server digest when present |
| Startup skip | Paginated `GET /v1/indexer/corpus/inventory` per distinct `(project_id, flavor_id)` scope plus local sync-state file |
| Initial scan | Queued `ScanJob` → chunked `FanoutListJob`s → tier-1 ingest jobs (no synchronous queue flood) |
| File changes | fsnotify with debounce; Create events tier 3, Write tier 2, bulk scan/fan-out tier 1 |
| Failure handling | Bounded retries on transient errors; then recovery poll; global **ingest gate** when health reports not ingest-ready |
| Deletes / renames | **Deferred** — add/update only; no corpus tombstone or delete-by-source in shipped code |
| Sync state store | **SQLite** at `sync-state.sqlite` (resolved from `sync_state_path`; legacy JSON migrates once) |
| Model-assisted strategy | **Not shipped** (Phase 7 in master plan) |

**Identity / auth / scoping**

- `tenant_id`, `principal_id`, and `user_label` attach to structured logs after `GET /v1/indexer/config`.
- Per-file ingest sends scope headers from merged YAML defaults, per-root scope, and glob overrides (`defaults` → root → `overrides[]`).
- Supervised workspaces bind each path to `project_id`, `flavor_id`, and `workspace_id` from operator SQLite rows.

**Persistence**

| Store | Role |
|-------|------|
| Operator SQLite (`operator.sqlite`) | Workspace definitions (gateway-owned) |
| `sync-state.sqlite` (from `sync_state_path`) | Per-file client/server SHA checkpoints; optional `chunk_count` / `chunk_schema` |
| In-memory work queue | Scan, fan-out, and ingest jobs; **lost on process restart** |

## Interfaces

| Surface | Detail |
|---------|--------|
| `GET /v1/indexer/config` | Embedding model, chunk settings, ingest paths, size limits, corpus inventory path |
| `GET /v1/indexer/workspaces` | All workspaces for token tenant (supervised watch roots) |
| `GET /v1/indexer/corpus/inventory` | Paginated remote file hashes for startup reconciliation |
| `GET /v1/indexer/storage/health` | Vector store + embedding readiness (`checks.*`, stable `reason_code`) |
| `GET /v1/indexer/storage/stats` | Collection point counts (scoped) |
| `POST /v1/ingest` | Whole-file ingest |
| `POST /v1/ingest/session` + chunk PUT + complete | Large-file transport |
| `GET /api/ui/indexer/config` | UI: supervised YAML text + nested workspaces from SQLite |
| `GET/POST/PUT/DELETE /api/ui/indexer/workspaces…` | UI workspace CRUD |
| Env | `CHIMERA_GATEWAY_URL`, `CHIMERA_GATEWAY_TOKEN` |
| Gateway config | `indexer.supervised.enabled`, `config_path`, `bin`, `log_json`, `start_when_rag_disabled` |

## Code map

| Concern | Location |
|---------|----------|
| Binary entry | `chimera/chimera-indexer/main.go` — supervised loop, workspace poll, config hot-reload |
| Core indexer | `chimera/chimera-indexer/internal/indexer/` — walk, queue, ingest, watch, scope |
| Gateway client | `internal/indexer/client.go` |
| Gateway ingest + indexer API | `chimera/chimera-gateway/internal/server/indexerapi/` |
| Supervised child spawn | `chimera/internal/supervisor/indexer.go`, `cmd/chimera/serve.go` |
| Operator workspaces store | `chimera/chimera-gateway/internal/operatorstore/store.go` |
| UI workspaces + settings | `internal/server/adminui/api/indexer/`, `embed/embedui/settings/` |
| Makefile | `make chimera-indexer-build`, `chimera-indexer-install` |

## Verification

```bash
go test ./chimera/chimera-indexer/internal/indexer/...
go test ./chimera/chimera-gateway/internal/server/indexerapi/...
go test ./chimera/chimera-gateway/internal/operatorstore/...
```

Manual: enable supervised indexer in `gateway.yaml`, add a workspace path on `/ui/settings`, confirm `indexer.run.start` and scoped ingest lines; stop embedding provider and confirm ingest gate closes with a stable `reason_code`.

## Memory and Windows resources

**Targets (desktop, supervised):** After bulk ingest settles, idle private working set should stay **≤300 MB** per ~1k indexed files across watched roots (validated ~**58 MB** with three warm workspaces post-fix). Peak RAM during a **first-time** full ingest of a large monorepo may still reach **hundreds of MB to ~1 GB** depending on `workers`, file sizes, and manifest chunk count.

**Drivers**

| Driver | Idle retained? | Mitigation |
|--------|----------------|------------|
| Manifest ingest spike (4 workers × file + JSON) | Until GC / OS release | `debug.FreeOSMemory()` on transition to `watch_idle`; tune `workers` |
| fsnotify per directory | Yes | Watches respect the same ignore rules as `Walk` (not raw full tree) |
| Sync-state SQLite | Yes | Small vs heap; grows with indexed file count |
| Spurious full re-index on restart | Was yes | `ApplyWorkspacesReindex` baselines generation on first poll without clearing sync |

**Diagnostics:** Task Manager **active private working set** on `chimera-indexer.exe`; correlate with `indexer.state` (`watch_idle`, `queue_depth: 0`) and `indexer.queue.snapshot` in supervised logs. High **handle** counts (>5k) usually mean unignored directory watches—check ignore rules and `ignore_extra`.

**`conhost.exe` (Windows):** Not created by Chimera explicitly. The supervisor attaches **stdout/stderr pipes** to every supervised child (`chimera-gateway`, `chimera-gateway --gateway-backend`, `chimera-indexer`, `chimera-broker`, `chimera-vectorstore`, and the supervisor itself) with `CREATE_NO_WINDOW` (no visible console). Windows often spawns one **Console Window Host** (~800 K) per piped child. Task Manager filters may show only a subset (e.g. gateway + indexer); broker and vectorstore use the same pattern. Harmless for RAM; unrelated to indexer footprint.

**Plan:** [`plans/indexer-memory-usage-analysis.md`](../plans/indexer-memory-usage-analysis.md).

## Out of scope and known gaps

- **Re-index all workspaces** — no single settings button; use `POST /api/ui/indexer/reindex-all` or per-workspace **Re-index** on managed workspace cards ([`plans/indexer-sync-state-sqlite-and-force-reindex.md`](../plans/indexer-sync-state-sqlite-and-force-reindex.md) shipped).
- **Incremental fsnotify root add/remove without session reload** — workspace changes trigger **full watch-session reload** after queue idle (up to ~10 minutes), not in-process `AddRoot`/`RemoveRoot`.
- **Partial path materialize** — one bad path in a workspace can fail entire `RootsFromWorkspacesResponse` until fixed.
- **Corpus purge on workspace/path delete** — watches stop after reload; vectors may remain in Qdrant.
- **Durable offline queue** — paused work is not persisted across crashes.
- **Delete/rename lifecycle** — undefined beyond best-effort add/update.
- **Phase 7 model-assisted indexing strategy** — not implemented.

## References

- Operator runbook: [`docs/indexer.md`](../indexer.md)
- Configuration: [`docs/configuration.md`](../configuration.md), [`config/indexer.example.yaml`](../../config/indexer.example.yaml)
- Delivery plans: [`plans/indexer.md`](../plans/indexer.md), [`plans/indexer-workspaces-sqlite-gateway-api.md`](../plans/indexer-workspaces-sqlite-gateway-api.md), [`plans/indexer-scan-and-fanout-jobs.md`](../plans/indexer-scan-and-fanout-jobs.md), [`plans/indexer-health-and-quiet-logs.md`](../plans/indexer-health-and-quiet-logs.md), [`plans/indexer-workspaces-accurate-reporting.md`](../plans/indexer-workspaces-accurate-reporting.md)
