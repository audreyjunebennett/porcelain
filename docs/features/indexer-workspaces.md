# Feature: Indexer workspaces (operator SQLite)

| Field | Value |
|-------|-------|
| **Doc kind** | `feature-record` |
| **Areas** | Operator SQLite, gateway indexer API, supervised `chimera-indexer`, settings embed UI |
| **Status** | `current` |
| **Introduced** | Gateway + indexer minor after v0.2 supervised stack |
| **Originated from** | [`plans/indexer-workspaces-sqlite-gateway-api.md`](../plans/indexer-workspaces-sqlite-gateway-api.md), [`plans/indexer-workspaces-accurate-reporting.md`](../plans/indexer-workspaces-accurate-reporting.md) |
| **Related features** | [Workspace file indexer](indexer.md), [Indexer ingest pipeline](indexer-ingest-pipeline.md) |
| **Depends on** | [Operator UI session auth](operator-ui-session-auth.md), operator SQLite migrations, UI session tenant |
| **Last updated** | See git history |

## At a glance

Operators define **indexer workspaces**—a project, flavor, and one or more absolute directory paths—in operator SQLite via `/ui/settings`. The gateway exposes CRUD to the UI and a read-only list to the supervised indexer at `GET /v1/indexer/workspaces`. The indexer binary **never opens** `operator.sqlite`. Supervised `indexer.supervised.yaml` holds tuning only; saving a workspace does not rewrite YAML or rely on the config file watcher. The settings log view shows **one card per database workspace row**, with titles and paths from SQLite and live progress from structured indexer logs.

## Operator-visible behavior

- **Workspaces section** on `/ui/settings` — create, edit, delete workspaces; add/remove watched paths; native folder picker in desktop webview (`window.chimeraPickFolder` / `window.top.chimeraPickFolder` in iframe).
- **Card identity** — Titles use **USER:PROJECT[:FLAVOR]** from the workspace row, not inferred from noisy log lines. Multiple paths on one workspace render as **one** card.
- **Expectations on change** — Adding or removing a path updates SQLite immediately; the indexer picks up changes within `workspaces_poll_interval_ms` (default **30s**) after the work queue drains (up to **10 minutes** wait). **`index_run_id` stays stable** for the process; roots are added/removed in-process without restarting `RunWatchers`.
- **YAML tuning** — Advanced indexer settings remain in `indexer.supervised.yaml`; edits hot-reload without touching workspace rows.
- **Orphan log lines** — Process-level indexer messages with no matching DB row appear under the **chimera-indexer service** summary, not as extra workspace cards.

## System behavior and contracts

**Invariants**

- **`operator.sqlite` is separate from `metrics.sqlite`** — distinct path, migrations, and retention concerns.
- **Schema** — `workspaces` (auto-increment id, tenant, project, flavor) and `workspace_paths` (FK, absolute path, CASCADE on workspace delete). Paths are **not** globally unique.
- **Supervised watch list** — Effective roots come **only** from the workspaces API when running with supervised `--config`; YAML `roots:` and `--root` are not used.
- **Standalone indexer** — Unchanged: roots from merged YAML layers and optional CLI `--root`.
- **UI card list is DB-first** — Logs enrich progress; they do not create workspace cards for unmatched partitions.

**Decisions**

| Topic | Decision |
|-------|----------|
| Workspace id | Integer `AUTOINCREMENT`; operators do not supply ids |
| Delivery to indexer | **Pull only** — periodic poll + session start fetch; no gateway push |
| Fingerprint | `WorkspacesRootsFingerprint` hashes sorted `(workspace_id, path, project_id, flavor_id)` tuples — path add/remove/edit detected |
| Reload strategy | **Incremental** add/remove of fsnotify roots and targeted scan for new paths; YAML tuning reloads in-process |
| Materialize errors | `RootsFromWorkspacesResponse` is **all-or-nothing** — one missing/invalid path fails the whole materialize |
| Tenant for UI CRUD | `operatorIndexerTenantID()` (empty string for single-user desktop today) |
| Corpus on delete | Stop watching after reload; **purge** scoped Qdrant collection on `DELETE /api/ui/indexer/workspaces/{id}` when RAG is enabled (uses UI session principal as ingest tenant + workspace project/flavor). Purge failure **blocks** SQLite delete (502). |

**Persistence**

- Migrations under `migrations/chimera-gateway/operator/` (workspaces tables in early operator migrations).
- Gateway config: `operator.sqlite_path` (default under `data/gateway/` relative to `gateway.yaml`).

## Interfaces

| Surface | Detail |
|---------|--------|
| `GET /v1/indexer/workspaces` | Bearer auth; nested workspaces with `workspace_id`, `project_id`, `flavor_id`, `paths[]` |
| `GET /api/ui/indexer/config` | Supervised YAML + nested workspaces for settings UI |
| `GET /api/ui/indexer/workspaces` | List workspaces (session auth) |
| `POST /api/ui/indexer/workspaces` | Create workspace + paths |
| `PUT /api/ui/indexer/workspaces/{id}` | Update project/flavor/paths |
| `DELETE /api/ui/indexer/workspaces/{id}` | Delete workspace and paths |
| Indexer poll | `workspaces_poll_interval_ms` in supervised YAML (default 30s); logs `indexer.supervised.workspaces_changed` then `indexer.supervised.workspaces_applied` |
| Structured logs | Scope fields include `workspace_id`, `ingest_project`, `flavor_id`, `indexer_target_key` on job and status lines |

## Code map

| Concern | Location |
|---------|----------|
| Operator store | `chimera/chimera-gateway/internal/operatorstore/store.go` |
| Indexer-facing API | `internal/server/indexerapi/indexer.go` (`HandleWorkspaces`) |
| UI handlers + purge | `internal/server/adminui/api/indexer/handlers.go`, `workspace_purge.go` |
| Vector purge | `internal/rag/service.go` (`PurgeWorkspaceCorpus`), `internal/vectorstore/` (`DeleteCollection`) |
| Indexer client + materialize | `chimera/chimera-indexer/internal/indexer/workspaces.go` |
| Supervised poll + dynamic roots | `chimera/chimera-indexer/main.go`, `internal/indexer/dynamic_roots.go`, `internal/indexer/watch.go` |
| Legacy YAML roots import | `internal/operatorstore/import_supervised_roots.go` (gateway startup when DB empty) |
| Settings UI workspaces | `embed/embedui/settings/` — `summarizedFeed.js`, workspace draft/card components |
| Log pinning / scope registry | `embed/embedui/settings/derive/indexerPartition.js`, `servicelogs/pin_indexer.go` |
| Tests | `workspaces_test.go`, `operatorstore/store_test.go`, `embedui_test/settings_*` |

## Verification

```bash
go test ./chimera/chimera-indexer/internal/indexer/ -run Workspaces
go test ./chimera/chimera-gateway/internal/operatorstore/...
go test ./chimera/chimera-gateway/internal/server/adminui/api/indexer/... -run WorkspaceDELETE
```

Manual: create a workspace with two paths on `/ui/settings`; confirm one card; add a third path; within poll interval (+ queue drain) confirm `indexer.supervised.workspaces_changed` and `workspaces_applied` with **unchanged** `index_run_id`; delete workspace and confirm indexing stops **and** the scoped Qdrant collection is removed (`gateway.operator.workspace.purged`).

## Out of scope and known gaps

- **Re-index all** — no dedicated UI control; per-workspace **Re-index** on managed cards calls `POST /api/ui/indexer/workspaces/{id}/reindex` ([`plans/indexer-sync-state-sqlite-and-force-reindex.md`](../plans/indexer-sync-state-sqlite-and-force-reindex.md) shipped).
- **Best-effort per-path materialize** — planned in accurate-reporting Phase 4D; **not** implemented.
- **Corpus purge on workspace delete** — `DELETE /api/ui/indexer/workspaces/{id}` drops the vector collection for `(ingest tenant, project_id, flavor_id)` before removing the SQLite row. Ingest tenant is the authenticated UI session principal (same tenant the indexer uses via API key). If RAG is enabled but purge fails, the workspace row is kept and the API returns 502. Structured log: `gateway.operator.workspace.purged` (success) / `gateway.operator.workspace.purge_failed` (blocked delete).
- **Removed watch path** — fsnotify stops; sync checkpoints cleared; stale sources pushed via `PUT /v1/indexer/corpus/stale` when possible (not a full collection purge unless the whole workspace is deleted).
- **ETag / revision** on workspaces response — not implemented.

## References

- Plans: [`plans/indexer-workspaces-sqlite-gateway-api.md`](../plans/indexer-workspaces-sqlite-gateway-api.md), [`plans/indexer-workspaces-accurate-reporting.md`](../plans/indexer-workspaces-accurate-reporting.md)
- Parent feature: [`indexer.md`](indexer.md)
- Operator guide: [`docs/indexer.md`](../indexer.md) (supervised mode section)
