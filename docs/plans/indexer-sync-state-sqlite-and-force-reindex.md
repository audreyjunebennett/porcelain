# Plan: Indexer sync-state SQLite and force re-index

| Field                          | Value                                                                                                                                                                                                                                                                                                                            |
|--------------------------------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| **Doc kind**                   | `feature-plan`                                                                                                                                                                                                                                                                                                                   |
| **Owners / areas**             | `chimera-indexer`, gateway (`operatorstore`, indexer API, embed UI settings), operator docs                                                                                                                                                                                                                                      |
| **Status**                     | `draft`                                                                                                                                                                                                                                                                                                                          |
| **Targets**                    | locus-desktop supervised stack; gateway + indexer next minor                                                                                                                                                                                                                                                                     |
| **Last updated**               | 2026-05-25                                                                                                                                                                                                                                                                                                                       |
| **Supersedes / superseded by** | Complements [`indexer-workspaces-sqlite-gateway-api.md`](indexer-workspaces-sqlite-gateway-api.md) (workspace list polling). Coordinates with [`indexer-manifest-ingest.md`](indexer-manifest-ingest.md) (segment index vs sync checkpoints). Does not replace corpus inventory reconciliation in [`indexer.md`](../indexer.md). |

## At a glance

Operators who run several large workspaces need a reliable way to **re-upload embedded content** when the local skip cache and Qdrant drift apart—for example after a vectorstore reset, a missing collection, or stale `indexer.sync-state.json` entries. Today that file grows without bound and rewrites entirely on every successful ingest, which does not scale past a few thousand files. This plan moves per-file SHA checkpoints into **indexer-local SQLite**, adds a **Re-index workspace** control on the settings screen, and uses a small **reindex generation counter** in operator SQLite (read via the existing workspace poll) so the gateway never has to push commands to the indexer.

| Phase                                                                                                  | Outcome                                                                                         | Status |
|--------------------------------------------------------------------------------------------------------|-------------------------------------------------------------------------------------------------|--------|
| [Phase 1 — Indexer-local sync-state SQLite](#phase-1--indexer-local-sync-state-sqlite)                 | Checkpoints stored incrementally; JSON migrates once; per-root delete is cheap                  | `todo` |
| [Phase 2 — Operator reindex intent and API](#phase-2--operator-reindex-intent-and-api)                 | Settings can bump a workspace reindex counter; indexer API exposes it                           | `todo` |
| [Phase 3 — Indexer applies reindex and rescans](#phase-3--indexer-applies-reindex-and-rescans)         | Poll detects generation change, clears local checkpoints, re-enqueues work                      | `todo` |
| [Phase 4 — Settings UI and operator copy](#phase-4--settings-ui-and-operator-copy)                     | Force re-index button, confirmation, and log messages operators understand                      | `todo` |
| [Phase 5 — Auto-repair and storage-stats messaging](#phase-5--auto-repair-and-storage-stats-messaging) | Missing Qdrant collection clears stale skips; UI stops sounding like a workspace lookup failure | `todo` |

---

## Background

### Observed problems

1. **Huge sync-state file.** `indexer.sync-state.json` holds one entry per indexed file (`client_sha256`, `server_sha256`). At ~4k+ files the file is tens of thousands of lines. Every successful ingest calls `MarshalIndent` on the **entire** map and rewrites the file (`chimera/chimera-indexer/internal/indexer/syncstate.go`), which is O(n) per upload during bulk indexing.

2. **Stale skip cache vs empty vectorstore.** Skip logic is local-only: if sync state says a file hash matches, ingest is skipped without checking Qdrant. If the collection was never created or was wiped, the indexer can believe thousands of files are "done" while `GET /v1/indexer/storage/stats` returns 404 for that scope.

3. **No operator-facing force re-index.** Recovery today is manual: delete or edit `indexer.sync-state.json`, or touch files on disk. There is no settings-screen action.

4. **Corpus inventory not always available.** `GET /v1/indexer/corpus/inventory` is the intended remote reconciliation path at scan time, but when it 404s or is disabled the indexer falls back to sync state alone.

### Design constraints (from existing architecture)

- **Indexer stays an HTTP client for operator workspace data** — it does not open `operator.sqlite` directly ([`indexer-workspaces-sqlite-gateway-api.md`](indexer-workspaces-sqlite-gateway-api.md)).
- **No gateway → indexer push.** Supervised mode already polls `GET /v1/indexer/workspaces` every ~30s and reloads on fingerprint change (`chimera/chimera-indexer/main.go`). Re-index intent should ride the same pull channel.
- **Sync checks stay local during ingest.** Per-file skip must remain an in-process lookup (memory or SQLite). Storing all SHAs in operator SQLite with per-file HTTP would not scale; batch-at-scan-time belongs to corpus inventory, not this plan.
- **Separate persistence files.** Operator DB (`operator.sqlite`), metrics (`metrics.sqlite`), and indexer checkpoints (`sync-state.sqlite`) stay isolated for backups and retention.

### Split of responsibility

| Concern                     | Owner                     | Mechanism                                                                       |
|-----------------------------|---------------------------|---------------------------------------------------------------------------------|
| Per-file SHA checkpoints    | Indexer process           | Local SQLite at `sync_state_path` (default beside config YAML)                  |
| Operator intent to re-index | Gateway / operator SQLite | `reindex_generation` (or equivalent) per workspace row                          |
| Delivery of intent          | Existing poll             | `GET /v1/indexer/workspaces` includes generation; indexer compares to last seen |
| Remote corpus truth         | Gateway / Qdrant          | Corpus inventory (separate fix if 404); optional auto-clear on collection 404   |

**Related docs:** [`docs/indexer.md`](../indexer.md), [`supervisor-info-log-trim.md`](supervisor-info-log-trim.md), [`indexer-workspaces-accurate-reporting.md`](indexer-workspaces-accurate-reporting.md).

**Non-goals**

- Moving all SHA rows into operator SQLite with batch sync HTTP (too heavy for v1).
- Gateway-initiated WebSocket or supervisor signals to the indexer.
- Full corpus purge API on workspace delete (documented follow-up in workspace plans).
- Changing embedding or chunking rules (separate "full reindex on gateway version bump" idea in [`indexer.md` plan](../plans/indexer.md)).

---

## Phase 1 — Indexer-local sync-state SQLite

**Goal.** Checkpoint reads and writes scale to tens of thousands of files without rewriting a monolithic JSON file on every ingest.

**Deliverables**

- Replace or wrap `SyncState` in `chimera/chimera-indexer/internal/indexer/syncstate.go` with a SQLite backend using `modernc.org/sqlite` (same driver family as gateway `operatorstore`).
- Default path: derive from existing `sync_state_path` — when extension is `.json`, use sibling `sync-state.sqlite` (or honor explicit `.sqlite` path). Document in `config/indexer.example.yaml` and `docs/indexer.md`.
- Schema (illustrative):

  ```sql
  CREATE TABLE sync_entries (
    job_key TEXT PRIMARY KEY,
    root_id TEXT NOT NULL,
    rel_path TEXT NOT NULL,
    client_sha256 TEXT NOT NULL,
    server_sha256 TEXT NOT NULL,
    updated_at TEXT NOT NULL
  );
  CREATE INDEX idx_sync_entries_root ON sync_entries(root_id);
  ```

- API surface unchanged for ingest: `Get(key)`, `Put(key, entry)`, plus new `DeleteByRoot(rootID string)` and `DeleteAll()` for re-index.
- **One-time migration:** on open, if legacy JSON exists and SQLite is empty, import entries and rename JSON to `indexer.sync-state.json.bak` (or leave in place with log line — decide in implementation).
- WAL mode + single connection (`SetMaxOpenConns(1)`) matching gateway SQLite discipline.
- Tests: round-trip Get/Put, migration from sample JSON, `DeleteByRoot` scope isolation, concurrent ingest simulation (single writer).

**Acceptance**

- Indexing 5k files produces incremental SQLite writes only; no full-file rewrite per ingest.
- Startup opens SQLite in bounded time (no loading 20k-line JSON into a map).
- Existing supervised installs migrate automatically on first run after upgrade.
- Deleting checkpoints for one `root_id` does not affect other roots.

**Status:** `todo`

---

## Phase 2 — Operator reindex intent and API

**Goal.** The gateway records when an operator requests a workspace re-index, in a field the indexer can observe without opening operator SQLite.

**Deliverables**

- Operator SQLite migration: add `reindex_generation INTEGER NOT NULL DEFAULT 0` (or `reindex_requested_at`) to `workspaces` table in `chimera/chimera-gateway/internal/operatorstore/`.
- `operatorstore` methods: `BumpReindexGeneration(ctx, workspaceID)`, `ListWorkspaces` includes generation in `Workspace` struct.
- **Indexer-facing API:** extend `GET /v1/indexer/workspaces` payload (`chimera/chimera-gateway/internal/server/indexerapi/indexer.go`) with `reindex_generation` per workspace (mirror in `WorkspacesAPIResponse` / `WorkspaceAPIEntry` in indexer client).
- **UI-facing API:** `POST /api/ui/indexer/workspaces/{id}/reindex` in `adminui/api/indexer/handlers.go` — auth required, bumps generation, logs `gateway.operator.workspace.reindex_requested` (slug TBD in `internal/naming/log_messages.go` and operator copy).
- Response includes new generation and a short note (`indexer_poll_seconds`, e.g. 30) so the UI can set expectations.

**Acceptance**

- API test: POST reindex increments generation; GET `/v1/indexer/workspaces` returns updated value for that workspace only.
- Generation survives gateway restart.
- Indexer binary still does not import `operatorstore` or open `operator.sqlite`.

**Status:** `todo`

---

## Phase 3 — Indexer applies reindex and rescans

**Goal.** Within one workspace poll cycle after the operator clicks re-index, the indexer clears local checkpoints for that workspace and re-uploads changed files.

**Deliverables**

- Indexer process state: remember last seen `reindex_generation` per workspace key (in memory + optional small sidecar file under `.locus/` so restarts do not re-trigger spurious rescans — open question below).
- On `FetchWorkspaces` (session start and poll goroutine in `main.go`): for each workspace where `reindex_generation` increased, call `syncState.DeleteByRoot(matchingRootIDs…)` and log `indexer.reindex.requested` with `workspace_id`, `project_id`, `generation`.
- **Root mapping:** map gateway `workspace_id` + paths to indexer `Root.ID` values used in `Job.Key()` (`root_id + "\x00" + rel_path`). Document whether root id is workspace row id, project id, or configured root id from `RootsFromWorkspacesResponse`.
- **Rescan trigger:** after clearing checkpoints, enqueue initial scan for affected roots (prefer targeted rescan over full session reload when only generation changed and paths unchanged). If simpler v1: treat generation bump like workspace fingerprint change → idle wait → session reload (reuse existing reload machinery).
- Do **not** require gateway push; poll interval remains default 30s unless config exposes `workspaces_poll_interval_ms` (future; optional in this phase).

**Acceptance**

- Integration test: bump generation via mock gateway → indexer clears sync rows for that scope → files ingest again (mock ingest counts increase).
- Other workspaces' sync entries untouched.
- Re-index with unchanged file content still runs ingest (checkpoints cleared); idempotent upsert on gateway side is acceptable.

**Status:** `todo`

---

## Phase 4 — Settings UI and operator copy

**Goal.** Operators can force re-index from the settings screen without editing files on disk.

**Deliverables**

- Workspace card or draft panel: **Re-index workspace** button with confirmation ("Re-upload all files in this workspace; may take several minutes").
- Wire to `POST /api/ui/indexer/workspaces/{id}/reindex`; show toast or status line with poll hint.
- Operator log formatter for `gateway.operator.workspace.reindex_requested` and `indexer.reindex.requested` (embed UI `operator_copy.js` / `operatorMessageIndexer.js`).
- Gallery or component test stub in `settings_components_test.go` if applicable.

**Acceptance**

- Manual: click re-index on `assistants` → within ~30s indexer logs reindex applied → upsert summaries appear → storage stats show collection reachable with points > 0 (when ingest succeeds).
- Button disabled when operator store unavailable (same gating as workspace CRUD).

**Status:** `todo`

---

## Phase 5 — Auto-repair and storage-stats messaging

**Goal.** Reduce silent drift and confusing operator messages when Qdrant has no collection for a scope.

**Deliverables**

- **Auto-repair (indexer):** when `indexer.storage.stats` reports `available: false` and `detail` indicates Qdrant collection 404 for a scope, clear sync checkpoints for the matching root(s) once per run (log `indexer.reindex.auto_collection_missing` at INFO). Optionally enqueue rescan without waiting for operator action.
- **Operator message (embed UI):** in `indexer_storage_stats` formatter, branch 404 / "doesn't exist" to copy like "No search index yet for workspace **assistants** — files will upload on the next ingest" instead of "Could not verify the stored search index — qdrant GET …".
- **Severity:** `sumEvlog.js` — missing collection 404 should not display as ERROR when it is an expected empty state.
- Tests for formatter and observation hook; align with [`supervisor-info-log-trim.md`](supervisor-info-log-trim.md) (first auto-repair event INFO/WARN; avoid spam every 2 min unless generation unchanged).

**Acceptance**

- Scenario: sync state populated, Qdrant collection missing → one repair action → ingest resumes without manual JSON delete.
- Settings event log shows WARN/INFO, not ERROR, for empty collection on idle system.

**Status:** `todo`

---

## Open questions

1. **Sidecar for last-seen generation** — Should the indexer persist `workspace_id → reindex_generation` locally so a process restart does not mis-handle generation 5 as "new" if it already applied 5 before exit? Prefer yes: small JSON or SQLite meta table.

2. **Rescan vs full session reload** — v1 may reuse fingerprint reload (simpler). Targeted rescan without new `index_run_id` is nicer for operators; defer if reload path is sufficient.

3. **JSON deprecation timeline** — After migration, delete JSON backup automatically after N days, or leave forever?

4. **Standalone indexer** — Force re-index API is UI/gateway-centric; should CLI expose `--clear-sync-state` / `--reindex-root` for non-supervised use?

5. **Corpus inventory 404** — Fix gateway route separately or bundle into Phase 5? Remote reconciliation reduces reliance on local checkpoints but is not a substitute for SQLite scale fix.

---

## References

- Sync state (today): `chimera/chimera-indexer/internal/indexer/syncstate.go`, `ingest.go` (`skip unchanged (sync state)`)
- Workspace poll: `chimera/chimera-indexer/main.go` (`defaultWorkspacesPollInterval`, `WorkspacesRootsFingerprint`)
- Operator store: `chimera/chimera-gateway/internal/operatorstore/store.go`
- Indexer workspaces API: `chimera/chimera-gateway/internal/server/indexerapi/indexer.go` (`HandleWorkspaces`)
- UI workspaces CRUD: `chimera/chimera-gateway/internal/server/adminui/api/indexer/handlers.go`
- Storage observation: `chimera/chimera-indexer/internal/indexer/observation.go`
- Operator storage-stats copy: `embed/embedui/settings/render/operatorMessageIndexer.js` (`indexer_storage_stats`)
- Config: `config/indexer.example.yaml` (`sync_state_path`)
