# Plan: Log view - Indexer UX

| Field                          | Value                                                                        |
|--------------------------------|------------------------------------------------------------------------------|
| **Doc kind**                   | `feature-plan`                                                               |
| **Owners / areas**             | Indexer, gateway embed UI, operator logs                                     |
| **Status**                     | `done`                                                                       |
| **Targets**                    | Indexer log UX P0-P2; P3 deferred                                            |
| **Last updated**               | See git history                                                              |
| **Supersedes / superseded by** | Builds on [`log-presentation-layer.plan.md`](log-presentation-layer.plan.md) |

## At a glance

Make it easy to see what the indexer is doing in the operator log view. One card per indexer shows which workspace it is working on, whether it's idle or backlogged, how many vectors are stored, and the most recent file it touched — without scanning raw JSON lines.

| Phase                                                                   | Outcome                                                                  | Status     |
|-------------------------------------------------------------------------|--------------------------------------------------------------------------|------------|
| [P0 — Backend instrumentation](#phase-p0--backend-instrumentation)      | Stable indexer identity, declared state, Qdrant stats, ignore metrics    | `done`     |
| [P1 — Log UI summary feed](#phase-p1--log-ui-summary-feed)              | Per-indexer cards with title, idle hint, watched paths                   | `done`     |
| [P2 — UI polish](#phase-p2--ui-polish)                                  | Event-mix chart, queue bar, jobs rollup, plain-language summary          | `done`     |
| [P3 — Multi-collection futures](#phase-p3--multi-key--futures-deferred) | Separate Qdrant stats per project + flavor; optional human summary field | `deferred` |

---

**Purpose:** Make indexer information in `/ui/logs` understandable to operators, with stable backend signals and front-end rollups.

**Related docs:** [`indexer.md`](../indexer.md), [`log-presentation-layer.plan.md`](log-presentation-layer.plan.md), [`indexer.plan.md`](indexer.plan.md).

**Document control**

| Field              | Value                                                    |
|--------------------|----------------------------------------------------------|
| Primary code owner | Gateway + embed UI in `porcelain`                        |
| Last plan update   | See git history for this file                            |
| Log JSON contract  | [`indexer.md`](../indexer.md) § Structured operator logs |

---

## How agents should update this document

1. **When starting work:** Set the relevant **step** status to `in_progress` and add your name or session note under **Activity log** (append-only at bottom).
2. **When merging a change:** Set step to `done`; add one short **Implementation notes** bullet (file paths, gotchas, env flags).
3. **If scope changes:** Add or split steps; do not delete history — move cancelled items to **Deferred / won’t do** with reason.
4. **After touching Go or embed UI:** Note which **Tests** you ran (`go test ./...`, targeted package, manual `/ui/logs`).
5. **Keep §1 slug table** aligned with [`indexer.md`](../indexer.md) — one source of truth for field names.

**Status legend**

| Status        | Meaning                                          |
|---------------|--------------------------------------------------|
| `todo`        | Not started                                      |
| `in_progress` | Active work                                      |
| `done`        | Shipped in tree (main or branch per team policy) |
| `deferred`    | Explicitly postponed                             |

---

## Phases, steps, and status

### Phase P0 — Backend instrumentation

| Step  | Description                                                                                                                                                                                | Status |
|-------|--------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|--------|
| P0.1  | `GET /v1/indexer/config` returns `tenant_id`, `user_label`, `principal_id` (`internal/server/indexer.go`)                                                                                  | done   |
| P0.2  | `IndexerKey(...)` stable id; `log.With` attaches `indexer_key`, `tenant_id`, `principal_id`, `user_label` after config fetch (`internal/indexer/key.go`, `indexer.go` `FetchAndLogConfig`) | done   |
| P0.3  | Startup order: `FetchAndLogConfig` → `LogIndexerRunStart` (`cmd/chimera-indexer/main.go`)                                                                                                  | done   |
| P0.4  | `indexer.run.start` includes `watch_root_paths` (absolute local paths) (`internal/indexer/observation.go`)                                                                                 | done   |
| P0.5  | Slug `gateway.indexer.config` on config line (was human-only string before)                                                                                                                | done   |
| P0.6  | `GatewayClient.FetchStorageStats`; `EmitStorageStatsAndState` logs `indexer.storage.stats` + `indexer.state` (`internal/indexer/client.go`, `observation.go`)                              | done   |
| P0.7  | `RunObservationLoop` in watch mode; default poll **2m** via `storage_stats_poll_ms` in YAML (`internal/indexer/config.go`); **&lt; 0** disables                                            | done   |
| P0.8  | One-shot: single `EmitStorageStatsAndState` before `indexer.run.done`                                                                                                                      | done   |
| P0.9  | `indexer.state` values: `initial_scanning`, `uploading`, `backlog`, `watch_idle`, `idle`, `recovery` (derived from queue, inflight, recovery flag)                                         | done   |
| P0.10 | `ingestInflight` wrapper in `processJob`; `inRecovery` in `waitForRecovery`                                                                                                                | done   |
| P0.11 | Discovery: `files_excluded_by_ignore_rules`, `skipped_ignored_files`, `skipped_ignored_dirs` (`internal/indexer/ops_events.go`)                                                            | done   |
| P0.12 | Tests: `internal/indexer/key_test.go`; existing `go test ./internal/indexer/...`                                                                                                           | done   |

**Implementation notes (P0)**

- **Stats scope:** `FetchStorageStats` uses **default** indexer headers only. Per-file flavor overrides in YAML do not get separate Qdrant polls in one process (see §Limitations).
- **JSON logs required** for UI to parse nested fields; supervised mode uses `--log-json`.
- **Config default:** `Resolved.StorageStatsPoll` = 2 minutes when YAML omits key; `storage_stats_poll_ms: -1` disables polling.

---

### Phase P1 — Log UI (summary feed)

| Step  | Description                                                                                                                                                                                     | Status |
|-------|-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|--------|
| P1.1  | Section title **Indexers** (`renderSummarizedUnified` in `internal/server/embedui/logs.js`)                                                                                                     | done   |
| P1.2  | Group cards by `indexer_key`, fallback `index_run_id`                                                                                                                                           | done   |
| P1.3  | Card title: `user_label — workspace / project · flavor`                                                                                                                                         | done   |
| P1.4  | Subtitle from latest `indexer.state` + optional **last `rel`** when file activity ≤ **120s** (`INDEXER_IDLE_RECENCY_MS`)                                                                        | done   |
| P1.5  | Status pill mapping: `watch_idle`/`idle`→waiting, `recovery`→monitor style, else indexing/complete/error                                                                                        | done   |
| P1.6  | Metrics: prefer `qdrant_points` from `indexer.storage.stats`, else chunk rollup                                                                                                                 | done   |
| P1.7  | Expanded summary: User, indexer key, IDs, **paths excluded by ignore rules**, **watched paths** `<pre>` (`logs.css` `.indexer-paths-pre`)                                                       | done   |
| P1.8  | `collectIndexerRunMeta` extensions (`internal/server/embedui/logs/derive/indexerMetrics.js`): `user_label`, `lastDeclaredState`, `qdrantPointsLive`, `filesExcludedByIgnores`, `watchRootPaths` | done   |
| P1.9  | `flatLooksLikeIndexerRunStart` accepts `watch_root_paths` array                                                                                                                                 | done   |
| P1.10 | Tests: `internal/server/logs_components_test.go` derives `collectIndexerRunMeta`                                                                                                                | done   |

**Implementation notes (P1)**

- **`lastDeclaredState` / `qdrantPointsLive`:** Scanned from **newest** matching events (reverse walk in `indexerMetrics.js`).
- **Durability pill:** Replaced misleading **ongoing** with session **duration** (`humanDurationMs`); Qdrant/session vector counts in second pill.
- **Embed layout:** `logs_bootstrap.js` loads modules; **authoritative bundle** for production is `embedui/logs.js` (see `internal/server/ui_handlers.go` `go:embed`).

---

### Phase P2 — UI polish

| Step | Description                                                                                                                                                                                                                                                   | Status |
|------|---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|--------|
| P2.1 | Replace **Request timeline** with **Event mix** stacked bar (slug categories via `indexerSlugHistogramBucket`) + color legend; add **Latest queue utilization** (depth/cap bar + caption from last `indexer.queue.snapshot` + optional `candidates_enqueued`) | done   |
| P2.2 | Jobs rollup: labels **Started upload · successfully ingested · skipped (before upload)**; sub **distinct relative paths**; **Advanced** `<details>` for workers · queue · snapshot line count                                                                 | done   |
| P2.3 | **`embedui/logs/derive/indexerPresent.js`:** `indexerProseSummary(flat)`, `indexerDeclaredStateLabel`, `indexerSlugHistogramBucket`, `indexerGroupKeyFromFlat`, `indexerFlatMsgForPresent`; `primaryLogMessage` uses prose when present (`logs.js`)           | done   |
| P2.4 | `logSummaryHtml(ev, badge, { suppressIndexerBadge: true })` for indexer expanded **last 3** + **full log** (hides duplicate **indexer** pill; **qdrant** lines keep badge)                                                                                    | done   |
| P2.5 | Goja tests: `TestLogsDerive_indexerPresent_histogramBucket`, `_proseStateAndStats`, `_groupKey` in `internal/server/logs_components_test.go`                                                                                                                  | done   |

**Implementation notes (P2)**

- **Load order:** `logs.html` includes `derive/indexerPresent.js` **after** `indexerMetrics.js`, **before** `gatewayUsageMetrics.js` and `logs.js`.
- **Histogram:** Categories are **approximate** (message slug only); “other” catches non-indexer lines if any appear in the card’s loaded window.
- **Prose fallback:** If `indexerProseSummary` returns `null`, `primaryLogMessage` keeps the legacy field-join line for that entry.
- **Tests run for this phase:** `go test ./internal/server/... -run Logs`, then `go test ./...`.

---

### Phase P3 — Multi-key / futures (deferred)

| Step | Description                                                                              | Status   |
|------|------------------------------------------------------------------------------------------|----------|
| P3.1 | Separate **Qdrant stats** polls per `(project, flavor)` when YAML overrides vary by path | deferred |
| P3.2 | Optional `operator_summary` human string alongside `msg` slug                            | deferred |

---

## Test definitions

### Automated (required when touching linked code)

| Test            | Command / location                        | Covers                                                                 |
|-----------------|-------------------------------------------|------------------------------------------------------------------------|
| Indexer package | `go test ./internal/indexer/...`          | Key stability, indexer behavior, client                                |
| Server + derive | `go test ./internal/server/... -run Logs` | Goja `collectIndexerRunMeta`, `indexerPresent`, gateway indexer routes |
| Full gate       | `go test ./...`                           | Repo-wide regressions                                                  |

**Future (optional)**

- End-to-end Goja eval of a **minimal `buildIndexerCard` HTML** fixture (heavier; currently covered by `indexerPresent` + `collectIndexerRunMeta` unit tests).

### Manual (operator)

| #  | Scenario                                              | Pass criteria                                                                                           |
|----|-------------------------------------------------------|---------------------------------------------------------------------------------------------------------|
| M1 | Supervised indexer, `log_json: true`, load `/ui/logs` | **Indexers** section; card title shows **token label**; expanded shows **watched paths**                |
| M2 | Wait >2 min idle after ingest                         | `indexer.state` with `watch_idle`; UI **waiting** pill; subtitle plain language                         |
| M3 | Touch a file under a root                             | Within **2 min**, subtitle includes **relative path**                                                   |
| M4 | `storage_stats_poll_ms: -1` in indexer YAML           | No periodic `indexer.storage.stats`/`indexer.state` spam (watch mode); one-shot still emits once at end |
| M5 | Large ignore set                                      | Expanded summary shows **paths excluded by ignore rules** matching discovery                            |

---

## 1. Canonical `msg` slugs (rollup + UI)

*Keep aligned with [`indexer.md`](../indexer.md). Implemented slugs emphasized.*

| `msg` slug                                           | Typical level | Purpose                                                                                                                |
|------------------------------------------------------|---------------|------------------------------------------------------------------------------------------------------------------------|
| `indexer.run.start`                                  | INFO          | `roots`, `root_ids`, `watch_root_paths`, scope / ingest ids                                                            |
| `gateway.indexer.config`                             | INFO          | Gateway RAG/embed settings (**replaces** undocumented human-only grouping for config line)                             |
| `indexer.storage.stats`                              | INFO / WARN   | `qdrant_points`, `collection`, `vector_dim`, `available`, errors as warn + `err`                                       |
| `indexer.state`                                      | INFO          | `state`, `queue_depth`, `ingest_inflight`, `initial_scan_complete`, `watch_mode`, `recovery`, `qdrant_points_reported` |
| `indexer.reconcile.summary`                          | INFO          | Corpus inventory loaded                                                                                                |
| `indexer.discovery.summary`                          | INFO          | `files_excluded_by_ignore_rules`, `skipped_ignored_files`, `skipped_ignored_dirs`, other `skipped_*`, `candidates_*`   |
| `indexer.queue.snapshot`                             | INFO          | Worker / queue / ingest counters                                                                                       |
| `indexer.run.progress`                               | INFO          | Milestones (e.g. `initial_scan`)                                                                                       |
| `indexer.retry.scheduled`                            | WARN          | Backoff                                                                                                                |
| `indexer.recovery.poll` / `indexer.recovery.resumed` | INFO          | Recovery lifecycle                                                                                                     |
| `indexer.worker.paused`                              | WARN          | Worker paused                                                                                                          |
| `indexer.job.*`                                      | INFO/ERROR    | Per-file lifecycle                                                                                                     |
| `indexer.run.done` / stop                            | INFO          | Final counters                                                                                                         |

**Structured fields commonly repeated (after successful config fetch):** `index_run_id`, `service`, `indexer_key`, `tenant_id`, `principal_id`, `user_label`.

---

## 2. Resolved product decisions (historical)

| Decision               | Resolution                                                                                                                                                                                                                                                                                                                                                                                                                  |
|------------------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| Grouping key           | **Cards per `indexer_target_key`** (`ik_…` fingerprint of tenant + project + flavor): multi-root YAML with **distinct** ingest targets sets `indexer_multi_target` (no shared `log.With indexer_key`) and `root_scopes` on `indexer.run.start`; UI partitions lines by target (job `root`, `ingest_project`, fan-out shared lines). Single-target runs still attach one `indexer_key`. Fallback `index_run_id` when needed. |
| Qdrant truth           | **Poll `GET /v1/indexer/storage/stats`** on a timer (default **2 min**); one-shot emits once at end.                                                                                                                                                                                                                                                                                                                        |
| Declarative state      | `indexer.state` every poll tick + derived fields (`uploading`, `recovery`, …).                                                                                                                                                                                                                                                                                                                                              |
| Idle / file subtitle   | `INDEXER_IDLE_RECENCY_MS = 120000` in `logs.js`.                                                                                                                                                                                                                                                                                                                                                                            |
| User in title/summary  | `user_label` from gateway config response + rollup.                                                                                                                                                                                                                                                                                                                                                                         |
| Watched paths          | **Full absolute paths** acceptable in logs and expanded summary.                                                                                                                                                                                                                                                                                                                                                            |
| Ignore rule visibility | `files_excluded_by_ignore_rules` (+ file/dir breakdown in discovery summary).                                                                                                                                                                                                                                                                                                                                               |

---

## 3. Limitations / known gaps

1. **Single stats scope:** One **default header** poll per process; multiple flavors/globs ≠ multiple live Qdrant counts without **P3.1**.
2. **Grouped history:** Cards keyed by `indexer_key` **merge events** across restarts sharing the same key (usually desired).
3. **Pre-config lines:** If `FetchAndLogConfig` fails permanently, logs may lack `indexer_key` → UI falls back to `index_run_id` only.
4. **Progress semantics:** Histogram is **volume by message type**, not time-ordered lifecycle; fine-grained backlog vs uploading still via `indexer.state` + queue snapshots.

---

## 4. File map (for future agents)

| Concern                             | Path                                                                                                 |
|-------------------------------------|------------------------------------------------------------------------------------------------------|
| Indexer binary entry                | `cmd/chimera-indexer/main.go`                                                                        |
| Key, observation, ops, indexer core | `internal/indexer/key.go`, `observation.go`, `ops_events.go`, `indexer.go`, `client.go`, `config.go` |
| Gateway indexer config/stat routes  | `internal/server/indexer.go`                                                                         |
| Log cards + grouping                | `internal/server/embedui/logs.js`                                                                    |
| Derived meta                        | `internal/server/embedui/logs/derive/indexerMetrics.js`                                              |
| Operator prose + histogram buckets  | `internal/server/embedui/logs/derive/indexerPresent.js`                                              |
| Logs page `<script>` order          | `internal/server/embedui/logs.html`                                                                  |
| Summary styles                      | `internal/server/embedui/logs.css`                                                                   |
| Derive tests                        | `internal/server/logs_components_test.go`                                                            |
| Operator docs                       | `docs/indexer.md`, `config/indexer.example.yaml`                                                     |

---

## 5. Acceptance criteria (rollup)

### Done (P0+P1 core)

- [x] Structured `indexer_key` and identity fields on indexer JSON logs after config fetch.
- [x] `indexer.state` and `indexer.storage.stats` on a documented cadence.
- [x] UI section **Indexers**; grouping by `indexer_key`; operator-facing title + summary fields.
- [x] **2 minute** idle semantics for subtitle file hint.

### P2 (complete)

- [x] Event mix histogram + queue utilization block.
- [x] Jobs rollup + advanced disclosure.
- [x] Shared **slug→prose** via `indexerPresent.js` + `primaryLogMessage`.
- [x] Goja tests for `indexerPresent` exports.

---

## 6. Activity log (agents append here)

_Format: `YYYY-MM-DD — short note — optional @branch`_

- 2026-05-05 — **P0/P1 landed:** gateway config adds `tenant_id` / `user_label` / `principal_id`; indexer emits `indexer_key` via `FetchAndLogConfig` `log.With`; `gateway.indexer.config`, `indexer.storage.stats`, `indexer.state`, discovery ignore breakdown; `watch_root_paths` on start; `RunObservationLoop` + default 2m `storage_stats_poll_ms`; `/ui/logs` Indexers grouping, titles, idle 120s hint, expanded paths + ignores; docs `indexer.md` + `log-view-indexer.plan.md` restructure.
- 2026-05-05 — **P2 landed:** `indexerPresent.js` (prose + histogram bucket + group key); `logs.js` event-mix bar + legend + queue mini-bar; jobs rollup + `<details>` advanced; `logSummaryHtml` optional `suppressIndexerBadge`; `logs.css` legend/advanced styles; goja tests `TestLogsDerive_indexerPresent_*`; ran `go test ./internal/server/... -run Logs` and `go test ./...`.
