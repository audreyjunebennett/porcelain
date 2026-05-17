# Plan: Supervised indexer workspaces in SQLite via gateway API

| Field | Value |
|-------|-------|
| **Doc kind** | `feature-plan` |
| **Owners / areas** | Gateway, `chimera-indexer`, desktop supervisor, embed UI (`/ui/logs` workspaces), persistence |
| **Status** | `draft` |
| **Targets** | Gateway + indexer next minor; desktop inherits supervised behavior |
| **Last updated** | See git history |
| **Supersedes / superseded by** | None |

## At a glance

Operators still manage **workspaces** from the logs UI, but those definitions live in a **small SQLite database** shipped with the app data directory, **not** in `indexer.supervised.yaml`. The supervised YAML keeps **indexer tuning** (timeouts, workers, ignore lists, and so on) and continues to **hot-reload** when that file changes. The **indexer process never opens that database**; it learns workspace roots by calling the **gateway** on a **periodic schedule** (and at session start), so edits to workspaces no longer rewrite the YAML file on every save—which today triggers a full watch-session recycle and contributes to unstable `index_run_id` / operator-visible identity churn and to failure modes when the file is briefly invalid mid-save.

| Phase | Outcome | Status |
|-------|---------|--------|
| [Phase 1 — Operator data store and gateway CRUD](#phase-1--operator-data-store-and-gateway-crud) | Workspaces persist in a dedicated SQLite file; gateway exposes create/read/update/delete used by the UI | `done` |
| [Phase 2 — Indexer client and merge rules](#phase-2--indexer-client-and-merge-rules) | `chimera-indexer` fetches workspace list over HTTP; supervised YAML supplies **tuning only**; no DB dependency in the indexer binary | `todo` |
| [Phase 3 — Dynamic roots, polling, and docs](#phase-3--dynamic-roots-polling-and-docs) | Configurable poll refetches workspaces; indexer **adds/removes watch roots without** recycling the whole watch session; docs and tests | `todo` |

---

## Background

Today, `indexer.supervised.yaml` is both the **merged `--config` layer** for the supervised child (`cmd/chimera-indexer` with explicit `--config`) and the **mutable store** for workspace roots: UI handlers in `internal/server/ui_indexer.go` read and write the whole file (`GET/PUT /api/ui/indexer/config`, append/update/remove root). The indexer watches that path (`indexer.WatchConfigPathForReload` from `cmd/chimera-indexer/main.go`); **any** save bumps mtime and cancels the current watch session in favor of a new one (`errSupervisedReload`), which assigns a new `index_run_id` per session. That matches the “ids change on reload” behavior operators see.

Putting **roots** and **tuning** in the same file means every workspace add/remove rewrites the same artifact the watcher observes. Partial writes or parse errors during save can also surface as a failed reload or a stuck process depending on timing.

**Constraints from product intent**

- **Separate from metrics:** Gateway metrics already use `metrics.sqlite` (see `internal/config/config.go`, `MetricsSQLitePath`). The new store must be a **different file** and **different migration set** so operational concerns (metrics retention, backups) stay isolated.
- **Indexer stays a pure HTTP client** for workspace data: no `database/sql` or shared DB path in `chimera-indexer`.
- **YAML remains** for supervised **non-workspace** settings and **hot reload** of those settings.
- **Workspaces** are the only scope for v1 of this store; future rows (conversations, turns, orchestration) are out of scope but the choice of DB should not block adding tables later.

**Related docs:** [`docs/configuration.md`](../configuration.md) (supervised indexer), [`docs/indexer.md`](../indexer.md), [`embedui-logs-workspaces-merge.md`](embedui-logs-workspaces-merge.md), [`internal/server/ui_indexer.go`](../../internal/server/ui_indexer.go), [`cmd/chimera-indexer/main.go`](../../cmd/chimera-indexer/main.go).

---

## Phase 1 — Operator data store and gateway CRUD

**Goal.** Workspace rows live in application SQLite; the gateway owns migrations and all reads/writes; the embed UI continues to manage workspaces but persists through new or adapted HTTP handlers instead of mutating `roots:` inside the supervised YAML.

**Deliverables**

- New gateway config fields (names illustrative; finalize in implementation): e.g. `operator.sqlite_path` defaulting to something like `../data/gateway/operator.sqlite` relative to `gateway.yaml` (same path-resolution style as `metrics.sqlite_path`), **distinct** from `metrics.sqlite_path`.
- SQL migrations under a gateway-owned directory (e.g. `porcelain/migrations/chimera-gateway/operator/`) with versioned `NNNNNN_*.sql` files; startup open + migrate pattern aligned with existing metrics migration discipline.
- **Normalized schema (decided):**
  - **`workspaces`** — one row per logical workspace: **`id INTEGER PRIMARY KEY AUTOINCREMENT`** (no operator-supplied workspace id), `tenant_id`, `project_id`, `flavor_id`, timestamps.
  - **`workspace_paths`** — one row per watched directory: primary key `id`, **`workspace_row_id` FK → `workspaces.id` ON DELETE CASCADE**, absolute `path`, timestamps. **Paths are not unique** across the DB (same disk path may appear under different workspaces).
  - Constraints: a workspace has **zero or many** paths; each path row belongs to exactly one workspace.
- Server-side repository (small package or `internal/server` helpers) used only from gateway process.
- HTTP API for the **session-authenticated operator UI** (reuse existing admin UI auth patterns used by `/api/ui/indexer/*`): list workspaces, create, update, delete—mirroring capabilities of `handleIndexerAppendRootPOST`, `handleIndexerUpdateRootPUT`, `handleIndexerRemoveRootPOST` without rewriting full YAML.
- Adjust `handleIndexerConfigGET` (and related) so the response the UI consumes includes **workspace list from SQLite** while still returning **supervised YAML text** for advanced editing of non-root settings if that remains exposed—or split “settings yaml” vs “workspaces” clearly in JSON to avoid ambiguity.

**Acceptance**

- With a fresh data directory, starting the gateway creates the operator DB and applies migrations; creating a workspace from the UI (or equivalent API test) stores a row and survives gateway restart.
- `metrics.sqlite` and `operator.sqlite` are **two files** when both enabled; no shared migration directory between them.

**Status:** `done`

**Goal.** The supervised indexer obtains workspaces from the gateway using the same **Bearer token** model as ingest (`CHIMERA_GATEWAY_TOKEN`), without reading SQLite.

**Deliverables**

- New **authenticated indexer** route, e.g. `GET /v1/indexer/workspaces`, returning **all workspaces for the token’s tenant** (nested or flat JSON: each workspace with `workspace_id`, `project_id`, `flavor_id`, and `paths: []`). Reuse existing indexer auth middleware / token checks used by `GET /v1/indexer/config` and ingest (see `internal/server/server.go` and indexer client).
- `internal/indexer` HTTP client method to fetch and parse that response; robust error handling (backoff, log, retry) consistent with `GatewayClient` patterns.
- **Merge semantics** (documented in `docs/indexer.md`) — **no transition period:**
  - **Supervised `--config` mode:** effective watch roots come **only** from `GET /v1/indexer/workspaces`. Supervised YAML **`roots:` is not used**; the supervisor does not pass `--root`.
  - **Standalone `chimera-indexer`** (no explicit supervised `--config`): unchanged — roots from layered YAML and optional `--root` overrides (`internal/indexer/config.go`).

**Acceptance**

- Integration-style test: gateway serves tenant workspaces; indexer client builds the correct flat list of `(abs path, scope)` jobs.
- Indexer binary does not gain a dependency on SQLite drivers for this feature.

**Status:** `todo`

---

## Phase 3 — Dynamic roots, polling, and docs

**Goal.** Workspace changes propagate on a **configurable poll interval** without rewriting supervised YAML for CRUD. **YAML tuning** continues to hot-reload via the existing supervised file watcher. The indexer **does not** tear down the entire watch session when the workspace set changes: it **incrementally** adds or removes filesystem watches and reconciles state.

**Implementation note (current code).** Today `RunWatchers` creates one `fsnotify.Watcher`, registers **all** `ix.cfg.Roots` once at entry, and never adds or removes top-level roots for the lifetime of that call (see `internal/indexer/indexer.go`). **Delivering “no tear down” is explicit indexer work:** e.g. a long-lived watcher loop that owns the `fsnotify.Watcher`, exposes `AddRoot` / `RemoveRoot` (recursive add/remove consistent with `addRecursive`), updates `matchers` and in-memory maps under mutex or safe publication, triggers **targeted** initial scan / queue drain for new paths only, and defines behavior for removed paths (stop enqueueing; optional tombstone / file-count bookkeeping). This phase is **not** a documentation-only tweak.

**Deliverables**

- **Periodic poll:** configurable interval (e.g. `workspaces_poll_interval_ms` in supervised YAML, with a documented default). On each tick, `GET /v1/indexer/workspaces`, compare to last applied snapshot (by stable ids or normalized path set), apply diff incrementally.
- **Optional:** `If-None-Match` / `ETag` or a monotonic `revision` field on the gateway response so the indexer can skip work when nothing changed (nice-to-have, not required for v1).
- **Decouple hot reload:** UI workspace CRUD never writes `roots:` to `indexer.supervised.yaml`; only tuning fields use the file watcher.
- **Operator migration:** one-time tool or gateway startup step: import legacy `roots:` from an existing `indexer.supervised.yaml` into normalized tables **before** cutting over (no long-lived dual-write). Document manual steps for anyone who only has the YAML file.
- Update **`docs/configuration.md`**, **`docs/indexer.md`**, and embed logs copy: **YAML = indexer tuning**, **SQLite + API = workspaces**, **poll = workspace refresh**.
- Tests: persistence + API; indexer poll + diff with fake HTTP; unit tests for dynamic add/remove watcher behavior.

**Acceptance**

- Adding or removing a path in the UI is picked up within the poll window **without** a new `index_run_id` **and without** restarting `RunWatchers` from scratch (same process session).
- Editing supervised YAML tuning still hot-reloads **without** requiring workspace poll to fire.
- Documented one-shot migration from old YAML-only roots to DB.

**Status:** `todo`

---

## Resolved decisions

| Topic | Decision |
|-------|----------|
| **Schema** | Normalized: `workspaces` (identity + `project_id` / `flavor_id`) and `workspace_paths` (absolute paths, FK to workspace). |
| **YAML roots** | **No transition:** supervised mode does not combine YAML `roots` with API; **API-only** for watch paths (no `--root` in supervised). |
| **`GET /v1/indexer/workspaces`** | Returns **all** workspaces for the **token’s tenant**. |
| **Push vs poll** | **Periodic polling only** for v1; interval **configurable** in supervised YAML. |
| **Root set changes** | **Requirement:** incremental add/remove of watches and reconciled indexer state **without** tearing down the full watcher session (requires indexer refactor as above). |

---

## Additional resolved decisions (follow-ups)

| Topic | Decision |
|-------|----------|
| **Supervised + `--root`** | **API-only:** supervised `chimera-indexer` does not use `--root`; watch list comes only from gateway workspaces API (Phase 2). |
| **Path uniqueness** | **Not enforced** — the same directory may be indexed under two different workspace rows intentionally. |
| **Removed paths / workspace** | **Stop watching** and **enqueue work** to delete or tombstone corpus vectors for that scope (Phase 3 indexer + gateway job). |
| **Workspace identity** | **`workspaces.id` is `INTEGER PRIMARY KEY AUTOINCREMENT`**; operators do **not** enter a workspace id in the UI (Phase 1). |
| **`index_run_id` on YAML reload** | **Unchanged for the process lifetime** when only supervised YAML tuning reloads; new id only on process restart (Phase 3 `cmd/chimera-indexer`). |

---

## References

- Code: `cmd/chimera-indexer/main.go`, `internal/indexer/config.go`, `internal/indexer/config_watch.go`, `internal/server/ui_indexer.go`, `internal/supervisor/indexer.go`, `internal/config/config.go`
- Docs: [`docs/indexer.md`](../indexer.md), [`docs/configuration.md`](../configuration.md), [`docs/supervisor.md`](../supervisor.md)
- Related plans: [`embedui-logs-workspaces-merge.md`](embedui-logs-workspaces-merge.md), [`log-view-indexer.md`](log-view-indexer.md)
