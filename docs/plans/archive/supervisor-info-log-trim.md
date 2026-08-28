# Plan: Trim supervisor INFO logs without losing operator signal

| Field | Value |
|-------|-------|
| **Doc kind** | `feature-plan` |
| **Owners / areas** | Gateway (`server.go`, embed UI derive), vectorstore normalizer (`vectorstoreline`), indexer (`workers.go`, `observation.go`, `scope_live_status.go`), broker normalizer (`brokerline`), supervisor log sink |
| **Status** | `done` |
| **Targets** | locus-desktop supervised stack at default INFO (`locus-desktop-supervisor.log`, `/api/ui/logs`) |
| **Last updated** | 2026-05-25 |
| **Supersedes / superseded by** | Extends [`indexer-health-and-quiet-logs.md`](indexer-health-and-quiet-logs.md) (indexer quiet-logs work shipped); complements [`log-gateway.md`](log-gateway.md), [`log-qdrant.md`](log-qdrant.md), [`log-bifrost.md`](log-bifrost.md) |

## At a glance

A typical 30-minute locus-desktop run at INFO produces hundreds of lines that repeat the same fact: settings-page polling, idle indexer heartbeats, unchanged storage snapshots, and ASCII startup banners. Operators still need errors, state transitions, indexing progress, and chat/routing events — but not 80 copies of `GET /api/ui/chimera-broker/providers → 200`. This plan demotes or collapses low-signal lines at the source (so the supervisor level filter drops them) while keeping everything the settings UI cards and event log actually render.

| Phase | Outcome | Status |
|-------|---------|--------|
| [Phase 1 — Gateway UI polling demotion](#phase-1--gateway-ui-polling-demotion) | Successful settings-page polls no longer fill the supervisor log at INFO | `done` |
| [Phase 2 — Vectorstore error storm dedupe](#phase-2--vectorstore-error-storm-dedupe) | Repeated Qdrant panics collapse to one operator-visible line per incident | `done` |
| [Phase 3 — Idle indexer heartbeat suppression](#phase-3--idle-indexer-heartbeat-suppression) | Unchanged `indexer.queue.snapshot` lines stop repeating every 30s while idle | `done` |
| [Phase 4 — Unchanged storage observation collapse](#phase-4--unchanged-storage-observation-collapse) | Stable `indexer.storage.stats` and `indexer.state` stop re-logging the same numbers every poll | `done` |
| [Phase 5 — Startup banner and routine chatter](#phase-5--startup-banner-and-routine-chatter) | One-shot startup noise (banners, broker housekeeping) demoted without hiding readiness or config errors | `done` |

---

## Background

### Observed volume (reference capture)

Analysis of `data/locus-desktop-supervisor.log` (~39 minutes at INFO, 1,041 JSON lines):

| Category | Lines | Notes |
|----------|------:|-------|
| Qdrant optimization panic storm | 433 | 42% of total; 3 lines per repeat (trace + backtrace + message) |
| `gateway.http.access` (UI polling) | 264 | Dominated by `/api/ui/*` and `/v1/indexer/storage/stats` at 2xx |
| `indexer.queue.snapshot` | 82 | Every ~30s while workers run, even when queue empty |
| `indexer.storage.stats` + `indexer.state` | 76 | ~2 min poll × 3 scopes + 1 state line per cycle |
| Startup banners (broker + vectorstore) | 26 | ASCII art emitted as separate INFO lines |
| Indexing / lifecycle (keep) | ~50 | Upsert summaries, discovery, run progress, failures |

Excluding the Qdrant storm, steady INFO is still ~470 lines — mostly polling and heartbeats the UI already hides or reads from the **latest** matching line only.

### Design principles

1. **INFO = something changed or failed** — periodic "still fine" belongs at DEBUG unless the operator UI has no other source for that fact.
2. **Demote at source** — supervisor applies `NewLevelFilterWriter` at INFO; lines must be DEBUG (or absent) before they reach `locus-desktop-supervisor.log`.
3. **UI contract first** — trim only after confirming `summarizedFeed.js`, `summarizedDirtyRouting.js`, and `operator_copy.js` derive state from latest structured events, not from line count.
4. **Failures stay loud** — non-2xx HTTP, ERROR/WARN slugs, state transitions, and first-seen degraded conditions remain INFO (or higher).

**Related docs:** [`locus-desktop-supervisor-contract.md`](locus-desktop-supervisor-contract.md), [`log-supervisor-normalization-fidelity.md`](log-supervisor-normalization-fidelity.md), [`indexer-health-and-quiet-logs.md`](indexer-health-and-quiet-logs.md), [`log-view-refactor.md`](log-view-refactor.md).

**Non-goals**

- Changing default log level to WARN globally.
- Removing lines from the in-process DEBUG buffer when operators enable trace mode.
- Fixing the underlying Qdrant "Access is denied" optimization bug (separate issue; this plan only addresses log amplification).

---

## Phase 1 — Gateway UI polling demotion

**Goal.** Successful settings-page and observation polls no longer appear in the default supervisor log stream.

**Deliverables**

- Extend `httpAccessLogLevel` in `chimera/chimera-gateway/internal/server/server.go` to return `slog.LevelDebug` for **2xx** requests to:
  - `/api/ui/tokens`
  - `/api/ui/state`
  - `/api/ui/chimera-broker/providers`
  - `/api/ui/indexer/config`
  - `/api/ui/indexer/workspaces`
  - `/v1/indexer/storage/stats`
- Mirror the same path list in `gatewayPanelHideRow` (`embed/embedui/settings/derive/gatewayCardModel.js`) so the gateway card full log and warn/fail counters stay consistent with what is stored at INFO.
- Tests in `chimera/internal/gatewayline/normalize_test.go` and/or `server` middleware tests asserting 2xx on these paths normalize or emit at DEBUG; non-2xx remains INFO.

**Acceptance**

- A 30-minute idle settings session produces **zero** INFO `gateway.http.access` lines for the paths above when all responses are 2xx.
- A failed poll (4xx/5xx) on any of these paths still appears at INFO in `locus-desktop-supervisor.log`.
- Gateway service card subtitle/KV and HTTP ok/fail counters in the UI are unchanged (cards poll HTTP APIs directly; log lines are not the primary data source).

**Status:** `done`

---

## Phase 2 — Vectorstore error storm dedupe

**Goal.** When Qdrant repeats the same optimization/panic error, operators see one clear ERROR (or WARN) line per incident window — not hundreds of identical backtraces.

**Deliverables**

- In `chimera/chimera-vectorstore/internal/vectorstoreline/normalize.go` (and/or a small rate-limit helper used by the line writer):
  - **`vectorstore.runtime.panic`**: emit full backtrace at DEBUG; at INFO emit a single truncated line (file + message, no stack) or suppress duplicate backtraces within a configurable window (default 30s) keyed by `(collection, error text prefix)`.
  - **`vectorstore.trace.other`**: rate-limit identical `progress_detail` (same normalization as panic root cause) to one line per window per target.
  - Prefer collapsing the triple (trace.other + panic backtrace + panic message) into **one** operator slug at INFO; keep full detail at DEBUG.
- Operator copy: ensure `vectorstore.runtime.panic` and `vectorstore.trace.other` formatters still read `progress_detail` on the surviving INFO line.
- Tests in `vectorstoreline/normalize_test.go`: repeated identical panic input → one INFO line + N DEBUG lines (or one combined INFO).

**Acceptance**

- Replaying 50 identical Qdrant panic lines yields **≤2** INFO lines in the supervisor log (initial + optional window refresh), not 150.
- A **different** error message or collection still emits a new INFO line immediately.
- Settings vectorstore card and event log show the optimization failure once with readable detail.

**Status:** `done`

---

## Phase 3 — Idle indexer heartbeat suppression

**Goal.** `indexer.queue.snapshot` proves the drain loop is alive without logging the same counters every 30 seconds while idle.

**Deliverables**

- In `chimera/chimera-indexer/internal/indexer/ops_events.go` / `workers.go`:
  - Track last emitted snapshot fingerprint (queue depths, `ingest_completed`, `jobs_dequeued`, skip counters, phase).
  - Emit at **INFO** when fingerprint changes **or** on phase transitions (`run_workers_start`, `after_initial_scan`, `worker_drain_tick` with non-zero queue).
  - When fingerprint unchanged and queue idle (`queue_depth == 0`, `ingest_inflight == 0`): emit at **DEBUG** or skip until a longer idle heartbeat (default 5 min, configurable `queue_snapshot_idle_info_interval_ms`).
  - Always emit at INFO immediately after leaving idle (queue depth or inflight becomes non-zero).
- Document new YAML knob in `config/indexer.example.yaml` and supervised template.
- UI: no change required — `latestIndexerQueueSnapshotMetaFromEntries` already reads the newest snapshot; `indexerServiceCardDirtyMsg` already ignores snapshot for card dirty.

**Acceptance**

- 30 minutes in `watch_idle` with empty queue: **≤2** INFO `indexer.queue.snapshot` lines (startup + optional long idle heartbeat), down from ~60.
- Active indexing (queue non-zero or counters changing): INFO snapshots continue at least every 30s while work is in flight.
- Indexer card queue cap / workers KV still populate from the log buffer tail.

**Status:** `done`

---

## Phase 4 — Unchanged storage observation collapse

**Goal.** Periodic storage observation stops re-logging identical point counts and `watch_idle` state every two minutes.

**Deliverables**

- In `chimera/chimera-indexer/internal/indexer/observation.go`:
  - Per scope: remember last `(available, qdrant_points, vector_dim, detail)` for `indexer.storage.stats`.
  - Per run: remember last `(state, queue_depth, ingest_inflight, qdrant_points_reported)` for `indexer.state`.
  - Emit at **INFO** on first observation, on any field change, or when `available:false` / degraded detail appears.
  - When unchanged: demote repeating lines to **DEBUG** (or suppress until state changes).
  - Keep **WARN** for fetch failures; keep **WARN on every poll** for unavailable storage (missing collection) so operators are not surprised by silent degradation.
- Align gateway-side `GET /v1/indexer/storage/stats` access log demotion with Phase 1 (indexer poll is already a gateway HTTP line).
- Tests: two consecutive identical polls → one INFO pair then DEBUG only; point count delta → new INFO.

**Acceptance**

- Idle watch mode with stable collections: **≤1** INFO `indexer.state` and **≤3** INFO `indexer.storage.stats` (one per scope) per poll **cycle** when values change; **zero** additional INFO lines when values are unchanged across cycles.
- Missing collection (404): **WARN on every poll** (repeats intentionally; no demotion to DEBUG).
- Indexer card state subtitle and storage-derived metrics match latest INFO (or WARN) line in buffer.

**Status:** `done`

---

## Phase 5 — Startup banner and routine chatter

**Goal.** Process startup emits a short operator story at INFO; ASCII art and housekeeping lines move to DEBUG.

**Deliverables**

### A. Broker normalizer (`brokerline/normalize.go`)

- Collapse `broker.startup.banner` ASCII / box-drawing lines: first banner line at INFO (empty or minimal `progress_detail`); subsequent art lines at DEBUG.
- Demote to DEBUG when slug is `broker.startup.banner` and `progress_detail` is purely decorative (box chars, ANSI) — keep `broker.version`, `broker.bootstrap.complete`, `broker.plugin.status`, and `broker.catalog.sync` **with** `catalog_model_count` at INFO.
- Demote routine one-shots to DEBUG: `broker.store.*`, `broker.auth.token_refresh`, `broker.mcp.startup`, `broker.governance.startup`, `broker.jobs.async_ready`, duplicate `broker.maintenance.log_retention`, catalog sync lines without model count.
- Keep ERROR/WARN: `broker.config.validation_failed`, `broker.config.schema_warn` (dedupe banner + WARN duplicate to one WARN).

### B. Vectorstore normalizer (`vectorstoreline/normalize.go`)

- Same banner collapse for `vectorstore.startup.banner` (Qdrant ASCII).
- Demote to DEBUG: duplicate `vectorstore.actix.bind`, TLS disabled hints, telemetry enabled, inference disabled, intermediate shard recover **progress** (0%); keep `vectorstore.ready`, `vectorstore.version`, shard **recovered** (100%), WARN/ERROR.

### C. Indexer scope heartbeat

- Demote `indexer.scope.status` with `change_reason: heartbeat` to DEBUG (edge-triggered lines with non-heartbeat `change_reason` stay INFO — aligns with [`indexer-health-and-quiet-logs.md`](indexer-health-and-quiet-logs.md) Phase 4 intent).

### D. Tests and operator copy

- Update `brokerline/normalize_test.go`, `vectorstoreline/normalize_test.go`, embed UI tests that assert banner visibility (`settings_components_test.go`) to expect one INFO banner marker per startup.
- Confirm `chimeraBrokerSliceSinceLastBanner` still finds a marker after startup.

**Acceptance**

- Cold start of full stack: **≤10** INFO lines per wrapped service for startup (down from ~50 combined), while `vectorstore.ready` and broker listen/ready lines remain INFO.
- Settings UI broker and vectorstore cards still show "starting" then "ready" subtitles from retained slugs.
- `log_level: debug` in gateway YAML still shows full banners in supervisor output when DEBUG is enabled.

**Status:** `done`

---

## Expected impact

| Scenario | Lines / ~40 min (approx.) |
|----------|---------------------------:|
| Reference capture (with Qdrant storm) | 1,040 |
| After Phase 2 only (same incident) | ~610 |
| Healthy idle session (Phases 1–5) | ~280 |
| Active indexing session (Phases 1–5) | ~350 (upsert summaries and job lines retained) |

---

## Open questions (resolved)

1. **Idle queue snapshot interval** — 5 minutes is acceptable; workspace state is preserved when leaving or reloading settings (UI reads latest snapshot from buffer/API, not per-line INFO count).
2. **Repeating storage 404** — Missing collections stay **WARN every poll** (repeat intentionally).
3. **Gateway log buffer vs supervisor file** — Settings is the only consumer at default INFO; demoted DEBUG lines need not stay in the ring buffer. Full detail remains available when `log_level: debug`.
4. **Scope status heartbeat** — `scope_status_poll_ms: -1` in supervised templates is correct; heartbeat `indexer.scope.status` lines demote to DEBUG; edge-triggered scope status stays INFO.

---

## References

- Reference capture: `data/locus-desktop-supervisor.log` (2026-05-25, ~39 min INFO)
- Gateway access levels: `chimera/chimera-gateway/internal/server/server.go` (`httpAccessLogLevel`, `loggingMiddleware`)
- Gateway UI row filter: `chimera/chimera-gateway/internal/server/adminui/embed/embedui/settings/derive/gatewayCardModel.js` (`gatewayPanelHideRow`)
- Indexer snapshots: `chimera/chimera-indexer/internal/indexer/ops_events.go`, `workers.go` (`workerDrainHeartbeatEvery`)
- Indexer observation: `chimera/chimera-indexer/internal/indexer/observation.go` (`defaultStorageStatsPoll`)
- Vectorstore dedupe/normalize: `chimera/chimera-vectorstore/internal/vectorstoreline/normalize.go`, `http_summary.go` (`postProcessNormalizedLine`)
- Broker normalize: `chimera/chimera-broker/internal/brokerline/normalize.go`
- Supervisor level filter: `chimera/chimera-supervisor/internal/supervise/log.go`
- UI dirty routing: `chimera/chimera-gateway/internal/server/adminui/embed/embedui/settings/app/summarizedDirtyRouting.js`
- Prior quiet-logs work: [`indexer-health-and-quiet-logs.md`](indexer-health-and-quiet-logs.md)
