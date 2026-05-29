# Plan: Indexer health probes and quiet operator logs

| Field | Value |
|-------|-------|
| **Doc kind** | `feature-plan` |
| **Owners / areas** | Gateway (`indexerapi`, `/health`, catalog), `chimera-indexer`, operator logs (`servicelogs`, embed UI) |
| **Status** | `shipped` |
| **Targets** | Gateway + indexer minor after v0.3 operator-log work |
| **Last updated** | 2026-05-22 |
| **Supersedes / superseded by** | Extends [`indexer-workspaces-accurate-reporting.md`](indexer-workspaces-accurate-reporting.md) Phase 3 (log volume); complements [`indexer.md`](indexer.md) failure handling |
| **As-built** | [`docs/features/indexer-health-and-operator-logs.md`](../features/indexer-health-and-operator-logs.md) |

## At a glance

When embedding or vector storage is unavailable, the indexer should **pause for a real reason** instead of retrying ingest against a healthy Qdrant while Ollama is down. The gateway health surface exposed to the indexer will report **vector store status**, **configured embedding model**, **catalog presence**, and a **machine-readable reason** when ingest cannot succeed. Operator logs will stay informative at INFO through **batched skip summaries** and **edge-triggered scope status**, while per-file trace (`active_file`, per-file skips, pre-upload lines) moves to DEBUG by default.

| Phase | Outcome | Status |
|-------|---------|--------|
| [Phase 1 — Rich indexer health from the gateway](#phase-1--rich-indexer-health-from-the-gateway) | `/v1/indexer/storage/health` tells the indexer why ingest is blocked (vectorstore and/or embedding) | `done` |
| [Phase 2 — Indexer recovery and global embed gate](#phase-2--indexer-recovery-and-global-embed-gate) | Recovery waits for embed readiness; workers stop hammering ingest when health says embed is down | `done` |
| [Phase 3 — Quiet INFO logs and batched skip summaries](#phase-3--quiet-info-logs-and-batched-skip-summaries) | Per-file noise at DEBUG; INFO shows periodic skip batches only while work is in flight | `done` |
| [Phase 4 — Edge-triggered scope status and log retention](#phase-4--edge-triggered-scope-status-and-log-retention) | Scope cards update on meaningful changes; lifecycle lines survive indexer log trimming | `done` |

---

## Background

### Observed failure (Ollama down, Qdrant up)

During a supervised run with Ollama stopped, ingest returns **502** (embed failure) while `GET /v1/indexer/storage/health` returns **`ok: true`** because it only probes Qdrant (`StoreHealth`). Optional `GET /health` checks broker + vectorstore, not the configured embedding model. After per-file retries exhaust, `waitForRecovery` resumes immediately and workers continue draining the bulk queue—logging hundreds of `indexer.scope.active_file` + `indexer.job.skipped` lines for unchanged files while changed files fail again.

### Design principles

1. **Health answers “can ingest succeed?”** — not merely “is Qdrant reachable?”
2. **INFO = operator story** — milestones, errors, batched progress, edge-triggered scope snapshots.
3. **DEBUG = per-file trace** — active file, individual skips, pre-upload lines.
4. **Summaries are activity-gated** — emit batched skip lines when the indexer is doing work, not on a timer while idle.

**Related docs:** [`indexer.md`](../indexer.md), [`indexer-workspaces-accurate-reporting.md`](indexer-workspaces-accurate-reporting.md), [`indexer-scan-and-fanout-jobs.md`](indexer-scan-and-fanout-jobs.md) (per-scope status), [`configuration.md`](../configuration.md).

**Non-goals**

- Running embeddings inside the indexer process.
- Replacing the summarized feed UI architecture.
- Full provider-health UI parity on the indexer HTTP client (reuse gateway classification, not duplicate brokeradmin calls from the indexer).

---

## Phase 1 — Rich indexer health from the gateway

**Goal.** `GET /v1/indexer/storage/health` returns structured checks so the indexer (and operators) know **why ingest is blocked**, including embedding model availability in the live catalog.

**Deliverables**

### A. Response shape (extend `indexer.storage.health`)

Keep top-level `ok`, `status`, `tenant_id`, `backend`, `url`. Add a **`checks`** object (mirrors `/health` style) so partial failure is explicit:

```json
{
  "object": "indexer.storage.health",
  "ok": false,
  "status": "degraded",
  "tenant_id": "lynn",
  "checks": {
    "vectorstore": {
      "ok": true,
      "status": "ok",
      "detail": ""
    },
    "embedding": {
      "ok": false,
      "status": "unavailable",
      "model": "ollama/nomic-embed-text:latest",
      "model_in_catalog": false,
      "provider": "ollama",
      "provider_state": "down",
      "reason_code": "embed_provider_unreachable",
      "detail": "dial tcp 127.0.0.1:11434: connect refused"
    }
  }
}
```

**Top-level `ok`** is `true` only when **every check** required for ingest is OK (vectorstore + embedding when RAG enabled).

**Reason codes** (stable strings for indexer branching and UI copy):

| `reason_code` | When |
|---------------|------|
| `vectorstore_unreachable` | `StoreHealth` error |
| `rag_disabled` | RAG off or `rt.RAG() == nil` (existing behavior; keep structured) |
| `embed_model_not_in_catalog` | Configured model absent from fresh `CatalogSnapshot` |
| `embed_provider_down` | Provider classified `down` (reuse `ClassifyBrokerProviderResult` + live snapshot override) |
| `embed_provider_key_missing` | Provider `key_missing` where keys are required |
| `embed_catalog_stale` | Snapshot missing or older than freshness window — **degraded**, not OK |
| `embed_probe_failed` | Optional lightweight probe failed (future; see Open questions) |

### B. Gateway implementation

- **`HandleHealth`** (`chimera-gateway/internal/server/indexerapi/indexer.go`):
  - Run **vectorstore** check (existing `StoreHealth`); populate `checks.vectorstore` with `detail` on failure (already partially done via top-level `detail`; move/consolidate into `checks`).
  - Run **embedding** check:
    - Read configured model from `rt.RAG().EmbeddingModel()`.
    - Read **`rt.CatalogSnapshot()`** — use `HasModel(modelID)` when snapshot is fresh (`CatalogSnapshotFreshness`, same as provider health UI).
    - Resolve provider prefix (`ollama/...` → provider `ollama`); classify via existing **`providers.ClassifyBrokerProviderResult`** logic (extract shared helper if needed to avoid importing UI package from indexerapi).
    - Set `model_in_catalog`, `provider_state`, `reason_code`, `detail` from classification + snapshot.
  - HTTP status: **200** with `ok: false` for degraded (indexer polls without treating body as transport error); **503** only for auth/RAG-disabled structured errors (existing contract).
- **Tests** (`indexer_test.go`): vectorstore down; embed model missing from catalog; ollama provider down with Qdrant up; all OK.

### C. Indexer client parsing

- Extend **`HealthStatus`** / **`CheckHealth`** (`chimera-indexer/internal/indexer/client.go`) to parse `checks.vectorstore` and `checks.embedding`.
- Add helpers: `IngestReady()`, `EmbedOK()`, `VectorstoreOK()`, `ReasonCode()`, human `Detail()`.
- Log fields on **`indexer.recovery.poll`**: `storage_ok`, `embed_ok`, `embed_reason_code`, `embed_model`, `embed_model_in_catalog`, `vectorstore_detail`, `embed_detail` (keep `root_health_ok` when root health probe remains).

### D. Documentation

- Update [`docs/indexer.md`](../indexer.md) § Failure handling and structured log table for new health fields and reason codes.
- Document response in gateway operator docs / example JSON beside `HandleHealth`.

**Acceptance**

- With Ollama stopped and Qdrant up, `GET /v1/indexer/storage/health` returns `ok: false`, `checks.vectorstore.ok: true`, `checks.embedding.ok: false`, non-empty `reason_code` and `detail`.
- With embedding model not listed in chimera-broker `/v1/models`, health reports `embed_model_not_in_catalog` even if provider process is up.
- Indexer recovery poll logs include embed reason fields; no code change yet to pause behavior (Phase 2).

**Status:** `done`

---

## Phase 2 — Indexer recovery and global embed gate

**Goal.** After retries exhaust, the indexer **stays paused** until gateway health reports ingest-ready (vectorstore **and** embedding). While embed is down, workers **do not** keep dequeuing ingest jobs and spamming the gateway.

**Deliverables**

### A. Recovery gate uses Phase 1 health

- **`waitForRecovery`** (`workers.go`): `recovered := health.IngestReady()` (vectorstore + embed), not `storageOK` alone.
- **`indexer.recovery.resumed`** only when embed check passes; **`indexer.recovery.poll`** logs `embed_ok: false` with `embed_reason_code` until fixed.
- Optional: first poll **immediately** on entering recovery, then `recovery_poll_interval_ms` ticker (avoids waiting 30s before first check).

### B. Global ingest pause (embed / vectorstore degraded)

- When **`CheckHealth`** reports not ingest-ready, set a process-wide **`ingestGate`** (atomic or mutex + cond) so **all workers** block before dequeuing **ingest** work (fan-out / scan may continue or also pause—default: pause ingest only).
- First worker to observe embed failure after retries can **`openGate`**; gate closes on successful health poll.
- Log once per gate transition at INFO:
  - `indexer.ingest.gate.closed` — reason_code, detail, queue_depth
  - `indexer.ingest.gate.open` — queue_depth, embed_model
- Avoid per-file retry loops while gate is closed: **`processIngestWithRetries`** checks gate before attempt; if closed, return **`ErrPaused`** without burning `retry_max_attempts`.

### C. Per-file retry policy tweak

- After first **502/503 embed-classified** failure on a file, optionally **skip remaining per-file retries** and go straight to **`ErrPaused`** + gate (config **`retry_short_circuit_on_embed: true`**, default **true** when health exposes embed check).
- Keep bounded retries for ambiguous 5xx without embed reason.

### D. Operator copy

- Register slugs in `internal/operatorcopy/messages.yaml` for gate closed/open and embed-specific recovery poll prose.
- Summarized feed: show “Waiting for embedding — {reason}” on workspace card when latest recovery poll has `embed_ok: false`.

**Acceptance**

- Ollama down: within one recovery cycle, **all workers** quiesce ingest; logs show gate closed with reason; **no** repeated `indexer.job.upload` lines every few seconds.
- Start Ollama: within one health poll, gate opens, queue drain resumes, ingests succeed.
- Qdrant down: gate closed with `vectorstore_unreachable`; embed check may be skipped or marked unknown in JSON.

**Status:** `done`

---

## Phase 3 — Quiet INFO logs and batched skip summaries

**Goal.** Default supervised runs emit a **small INFO story**: run lifecycle, errors, ingests, batched skip totals, edge-triggered scope status (Phase 4)—not two lines per unchanged file.

**Deliverables**

### A. Demote per-file trace to DEBUG (Strategy 2 + 2.d)

| Message | New default level | Notes |
|---------|-------------------|--------|
| `indexer.scope.active_file` | **DEBUG** | Implement: change `scope_live_status.go` to `log.Debug`; keep rate-limit logic |
| `indexer.job.skipped` | **DEBUG** | Via default `job_skip_log: debug` in supervised example + desktop template |
| `indexer.job.upload` | **DEBUG** | Same; INFO remains for `indexer.job.ingested` / failures |
| `indexer.skip.*` | DEBUG | unchanged |

- **Do not emit `active_file`** before skip decision (Strategy 2.a): call `emitScopeActiveFileIfDue` only after unchanged checks pass, immediately before upload/ingest path.
- Document operator vs debug profiles in [`docs/indexer.md`](../indexer.md):
  - **Operator (default):** `log_level: info`, `job_skip_log: debug`
  - **Trace:** `log_level: debug`, `job_skip_log: info`

### B. Batched skip summary (Strategy 2.c)

New structured line: **`indexer.job.skipped.summary`**

**Fields (minimum):**

| Field | Description |
|-------|-------------|
| `window_ms` | Time since previous summary for this scope (or run) |
| `files_evaluated` | Jobs completed in window (skip + upload attempts + ingested) |
| `skip_unchanged_local_sync` | Delta or cumulative in window (prefer **delta** per window) |
| `skip_unchanged_corpus_client_hash` | Delta |
| `skip_unchanged_corpus_sync` | Delta |
| `skip_empty_or_whitespace` | Delta |
| `ingest_started` | Upload attempts started in window |
| `ingest_succeeded` | Ingest OK in window |
| `ingest_failed` | Failures in window |
| `queue_depth` | Optional snapshot |
| Scope fields | `indexer_target_key`, `tenant_id`, `project_id`, `ingest_project`, `flavor_id` |

**Emission rules (avoid idle clogging):**

1. **Activity gate:** Only emit if `files_evaluated > 0` in the window (something actually happened).
2. **Minimum interval:** Default **`skip_summary_min_interval_ms: 5000`** (5s); configurable in indexer YAML.
3. **No periodic idle summaries:** If no files evaluated since last summary, **do not** emit on a timer.
4. **Flush on phase change:** Emit a final summary when ingest gate closes, worker drain completes, or `indexer.run.done`.
5. **Per scope:** One summary line per active `(tenant, project, flavor)` scope with non-zero activity in the window (avoid N duplicate lines for single-scope runs).

Implementation sketch: accumulator struct per scope on the indexer, incremented from existing `opsSkip*` atomics / ingest counters; background ticker every 1s checks **due** summaries against min interval + activity.

### C. Default config updates

- [`config/indexer.example.yaml`](../../config/indexer.example.yaml): `job_skip_log: debug`, document `skip_summary_min_interval_ms`.
- Supervised / desktop bundled indexer config: same defaults.

### D. UI / operator copy

- Parse `indexer.job.skipped.summary` in `summarizedFeed.js` / `indexerPresent.js` for rollup mini-cards (“Skipped 842 unchanged · 3 ingested · last 5s”).
- Formatter in `operator_copy.js`.

**Acceptance**

- Steady-state rescan with Ollama up: INFO shows summary lines ~every 5s while queue drains, not thousands of skip lines.
- Idle watch mode (queue empty, no fs events): **no** repeating skip summaries.
- `log_level: debug` still shows per-file `active_file` and skips.

**Status:** `done`

---

## Phase 4 — Edge-triggered scope status and log retention

**Goal.** Workspace cards and operator logs show **meaningful progress changes** without a fixed 45s heartbeat for every field; critical lifecycle lines survive indexer log cap.

**Deliverables**

### A. Edge-triggered `indexer.scope.status` (Strategy 3.e)

Keep periodic heartbeat as **fallback** (`scope_status_poll_ms`, default 45s) but emit an **extra** INFO line promptly when any **tracked dimension** changes per scope.

**Suggested change events** (emit when value changes vs last emitted snapshot for that scope):

| Event key | Trigger | Example operator meaning |
|-----------|---------|---------------------------|
| `phase` | Declarative state transition | `initial_scanning` → `backlog` → `uploading` → `recovery` → `watch_idle` |
| `queue_ingest_pending` | Delta ≥ threshold or crosses bucket | 0→1, &gt;100, or every 10% of `workspace_files_total` |
| `queue_fanout_files_pending` | Same pattern | Fan-out backlog shifted |
| `workspace_files_total` | Discovery recounted files | New scan finished |
| `pending_bulk_tier1` | Bulk fan-out tier changed | Bulk tier backpressure |
| `current_rel` | Active ingest file changed | “Now indexing `src/foo.go`” (INFO one-liner optional; detail stays DEBUG) |
| `ingest_gate` | Open / closed | “Paused — embedding unavailable” |
| `embed_reason_code` | From last health poll | Align card subtitle with Phase 2 |
| `ingest_completed` | Milestone every N successes | Every 25 ingests in a run (optional `scope_status_ingest_milestone`) |
| `recovery_entered` / `recovery_exited` | Gate or `inRecovery` | Single line, not every poll tick |

**Implementation notes:**

- **`scope_live_status.go`:** Maintain `lastEmitted map[scopeKey]scopeStatusSnapshot`; compare on queue tick, gate transition, discovery, health poll.
- Add optional field **`change_reason`** (string enum from table) on edge-triggered lines only; heartbeat lines omit it or set `change_reason: heartbeat`.
- Reduce **`indexer.recovery.poll`** to **WARN** when still waiting; emit **one INFO** on state transition instead of identical polls every 30s (poll counter remains in DEBUG).

### B. Gateway log buffer pinning (Strategy 3 from accurate-reporting plan)

- **`servicelogs`:** Pin latest per run: `indexer.run.start`, `indexer.run.done`, latest `indexer.scope.status` per `indexer_target_key`, latest `indexer.ingest.gate.*`, latest `indexer.job.skipped.summary` per scope.
- Config: `operator_logs.indexer_pinned_lines_max` in gateway YAML.
- Pin before `trimSourceToMax` for `chimera-indexer` source.

### C. UI consumption

- Prefer latest edge-triggered `indexer.scope.status` for progress bar and subtitle (`indexerMetrics.js`).
- Show gate / embed reason in workspace card when present.

**Acceptance**

- Card subtitle updates when queue drops or embed gate closes within seconds, without waiting for 45s heartbeat.
- Scrolling heavy per-file DEBUG traffic does not evict `indexer.run.start` from `/api/ui/logs` tail.
- Recovery while Ollama down: one clear “paused for embedding” status line, not a wall of identical recovery polls at INFO.

**Status:** `done`

---

## Open questions answered

1. **Lightweight embed probe on health** — Is catalog + provider classification enough, or should `HandleHealth` optionally POST a 1-token embed (expensive; cache result ~30s)?
2. **Global gate** — fan-out list jobs are paused when embed is down
3. **Summary deltas vs cumulative** — Operator preference: run-to-date totals in summary line
4. **HTTP status on degraded health** — 503 when embed down

---

## References

- Gateway health today: `chimera/chimera-gateway/internal/server/indexerapi/indexer.go` (`HandleHealth`), `server.go` (`/health`)
- Catalog / embed model: `chimera/chimera-gateway/internal/server/catalog/availablemodels.go`, `providers/handlers.go`
- Indexer recovery: `chimera/chimera-indexer/internal/indexer/workers.go`, `client.go` (`CheckHealth`)
- Scope status: `chimera/chimera-indexer/internal/indexer/scope_live_status.go`
- Skip counters: `chimera/chimera-indexer/internal/indexer/ops_events.go`, `ingest.go` (`emitSkippedFile`)
- Log buffer: `chimera/internal/servicelogs/store.go`
- UI rollups: `chimera/chimera-gateway/internal/server/adminui/embed/embedui/settings/app/summarizedFeed.js`
- Operator messages: `internal/operatorcopy/messages.yaml`, `docs/indexer.md`
