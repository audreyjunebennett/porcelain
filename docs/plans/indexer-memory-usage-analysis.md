# Plan: Indexer memory usage analysis and reduction

| Field                          | Value                                                                 |
|--------------------------------|-----------------------------------------------------------------------|
| **Doc kind**                   | `feature-plan`                                                        |
| **Owners / areas**             | `chimera-indexer`, `chimera-supervisor`, locus-desktop, operator docs |
| **Status**                     | `shipped`                                                             |
| **Targets**                    | Supervised desktop / `chimera serve` stack on Windows                  |
| **Last updated**               | 2026-06-03 (Phase 1–2 complete; Phase 3 partial)                      |
| **Supersedes / superseded by** | Complements [`indexer-manifest-ingest.md`](indexer-manifest-ingest.md) (manifest ingest increases per-file RAM) |
| **As-built**                   | [`docs/features/indexer.md`](../features/indexer.md#memory-and-windows-resources) |

## At a glance

Operators running the supervised stack see **chimera-indexer** hold **~1.2 GB+** private working set per active workspace (scaling with each added workspace), while other Chimera Go children often sit near **~40 MB** for reference only—not a RAM target for the indexer. Footprint **does not shrink** after ingest finishes and the queue has been idle for 10+ minutes on a given workspace. This plan documents what we know, runs structured memory analysis, and ships targeted fixes so idle indexer RAM is **predictable** with an operator-acceptable floor of **≤300 MB per workspace** where feasible (not peer-service parity).

| Phase                                                                 | Outcome                                                                 | Status |
|-----------------------------------------------------------------------|-------------------------------------------------------------------------|--------|
| [Phase 1 — Baseline and reproduction](#phase-1--baseline-and-reproduction) | Repro checklist, Task Manager + log correlation, idle-vs-active thresholds documented | `done` |
| [Phase 2 — Attribution (heap and OS)](#phase-2--attribution-heap-and-os) | pprof/allocs + long-lived object inventory; root causes ranked with evidence | `done` |
| [Phase 3 — Mitigations and ship](#phase-3--mitigations-and-ship)       | Code/config changes + operator guidance; idle RAM measurably reduced on Windows | `done` |

---

## Background

### Reported symptoms (2026-06-03)

| Observation | Detail |
|-------------|--------|
| **Indexer RSS** | `chimera-indexer.exe` ~**1,237,424 K** active private working set (Task Manager) |
| **Peer services** | Other Chimera supervised children ~**40 MB** each under the same filter |
| **Idle persistence** | **10+ minutes** after last ingest with **no active uploads**; memory **unchanged** (not a short post-spike plateau) |
| **Queue state** | Supervisor logs show `queue_depth: 0`, `declarative_state: watch_idle`, `ingest_inflight: 0` after initial bulk pass |
| **Extra processes** | Multiple **`conhost.exe`** (~**800 K** each) visible when filtering Task Manager for “chimera”; they **remain running** alongside Chimera children |

### Reproduction context (porcelain repo)

From `data/locus-desktop-supervisor.log` on the same day:

- Single supervised workspace root: `C:\Users\lynnv\src\porcelain` (full monorepo).
- Initial scan: **866** file candidates; Qdrant collection ~**14.5k** points before/after pass.
- Config: **4 workers**, **queue_depth 1024**, manifest ingest (`chunk_schema: 2`).
- Corpus inventory: `GET /v1/indexer/corpus/inventory` returned **404** (inventory map not loaded; skip relies on local sync state only).
- Bulk queue peaked ~**697** ingest jobs, then drained to **0** in ~90s; **866** files completed (`ingest_completed: 866`).

### Why this matters

High idle RAM on a long-lived desktop indexer:

- Competes with IDE, browser, and Qdrant on operator machines.
- Looks like a **leak** even when it may be **retained Go heap + long-lived subsystems** (fsnotify, SQLite, HTTP pools).
- Feels like a leak next to **~40 MB** gateway/broker/vectorstore children (reference only; those binaries are not the RAM budget for indexing).

### Hypothesized memory drivers (pre-analysis)

These are **candidates** to confirm or reject in Phase 2—not conclusions.

| Area | Mechanism | Idle retained? |
|------|-----------|----------------|
| **Manifest ingest** | Per file: `ReadNormalizeFile` → `BuildManifest` → `json.Marshal` (full file + chunk texts + JSON). Up to **4** concurrent (`workers`). | Peak only; should be GC-eligible unless referenced |
| **Work queue** | Up to **1024** `WorkItem`s; fan-out jobs hold up to **4096** `TaggedCandidate`s per shard; bulk depth ~700 observed | Should drop to **0** at idle (verify nothing holds queue copies) |
| **Initial scan** | `all []TaggedCandidate` for every discovered file before fan-out + interleave copies | Should be scoped to scan; verify no global retention |
| **Go runtime** | Allocator does not return memory to OS after bulk spike | **Likely** contributor to flat RSS after idle |
| **fsnotify** | Watch on entire repo tree (Windows backend) | **Likely** long-lived |
| **Sync state SQLite** | `data/indexer/sync-state.sqlite`; `ListEntries()` for coherence on poll | Moderate; grows with file count |
| **Logging** | `indexer.discovery.summary.scope` embeds up to **200** `rel_paths` in one INFO JSON line | Spike during scan |
| **`remoteInv` map** | Full corpus inventory in RAM when API works | Not loaded in reported run (404) |
| **HTTP client** | Keep-alive pools to gateway | Small but long-lived |
| **Multi-workspace** | Per-workspace sync DB, fsnotify roots, scope maps, Qdrant collection handles | **Yes** — ~744 MB observed adding second workspace (porcelain + minecraft-character-editor) |

### `conhost.exe` (~800 K each)

On Windows, **Console Window Host** (`conhost.exe`) appears when a process is associated with a console or certain pipe/redirection setups. Chimera supervised children use `CREATE_NO_WINDOW` via `proc.ApplyNoConsoleWindow` (`chimera-supervisor`, gateway, broker, vectorstore, indexer). Indexer additionally uses `StdoutTee` / `StderrTee` only when `GetConsoleWindow() != 0` (`stdio_tee_windows.go`) to avoid blocking when built as `windowsgui`.

**Open hypothesis:** residual `conhost` instances are **not** the 1.2 GB problem (~800 K each) but may indicate **how** a child was launched or a **secondary** console attachment.

**Operator note (2026-06-03):** Filtering Task Manager for “chimera” under **locus-desktop** did **not** show `conhost.exe` before recent indexer work (SQLite sync state + manifest chunking). `conhost` instances appeared **after** those changes; correlation is **unconfirmed**. Parent-PID mapping remains useful if we chase console attachment, but not blocking memory work.

**Related docs:** [`docs/indexer.md`](../indexer.md), [`docs/features/indexer.md`](../features/indexer.md), [`docs/features/indexer-ingest-pipeline.md`](../features/indexer-ingest-pipeline.md), [`supervisor.md`](../supervisor.md).

---

## Phase 1 — Baseline and reproduction

**Goal.** Anyone on the team can reproduce the reported footprint on Windows and capture artifacts before changing code.

**Deliverables**

- **Repro worksheet** (in this plan or `docs/indexer.md` appendix) covering:
  - Workspace path scope (narrow vs full repo).
  - `config/indexer.yaml`: `workers`, `queue_depth`, `max_file_bytes`.
  - Fresh vs incremental run (sync state warm).
  - Wait **≥10 minutes** after `watch_idle` + `queue_depth: 0` before measuring RSS.
- **Task Manager capture**: `chimera-indexer.exe` private working set; all `chimera-*` children; **`conhost.exe`** count and parent process (Process Explorer or `Get-Process` / WMI).
- **Log correlation**: `indexer.queue.snapshot`, `indexer.scope.status`, `indexer.state` showing idle; `candidates_total`, peak `queue_depth`.
- **Environment note**: Windows build, desktop vs `chimera serve`, gateway/indexer versions (git SHA).
- Optional: **restart test** — stop supervised indexer only; confirm RSS drops; restart and compare cold vs warm sync state.

**Acceptance**

- Documented steps reproduce **≥500 MB** idle indexer RSS on a workspace with **500+** files **or** explain why porcelain 866-file case differs.
- Optional: `conhost` instances mapped to parent PIDs if still present (not required for memory acceptance).

**Status:** `done`

### Phase 1 capture (2026-06-03, PID 33636 session)

| Item | Value |
|------|--------|
| **Environment** | Windows, locus-desktop → `chimera-supervisor` (13748), indexer started 10:44:54 local |
| **Workspaces** | `porcelain` (867 files) + `minecraft-character-editor` (266 files) |
| **Config** | `workers: 4`, `queue_depth: 1024`, `sync_state_path: data/indexer/sync-state.json` |
| **Cold RSS** | ~**500 MB** private shortly after start (operator) |
| **Peak RSS** | ~**1,434 MB** private during second bulk pass |
| **Idle RSS (≥57 min)** | **1,434 MB** private unchanged at 11:47 local; `watch_idle` since 15:48:04Z; `queue_depth: 0` |
| **Handles (idle)** | **18,030** (stable) |
| **conhost** | Six instances under chimera children (~800 K each); indexer parent **33636** → conhost **19344**; dual gateway → **30008**, **12356** |
| **Trigger for bulk** | `indexer.reindex.requested` at 15:45:24Z (first workspace poll); `indexer.supervised.watch_shutdown_timeout` + `hot_reload` at 15:46:04Z → second full scan |

**Repro steps:** Restart locus-desktop → wait for UI “up to date” → observe indexer RSS at ~1 min and ~5 min after `watch_idle`; compare Task Manager private working set to `indexer.queue.snapshot` / `indexer.state` in `data/locus-desktop-supervisor.log`.

---

## Phase 2 — Attribution (heap and OS)

**Goal.** Identify what retains memory at idle with evidence (profiles, heap dumps, code pointers)—not guesses.

**Deliverables**

- **Go heap profile** (idle, post-ingest):
  - Run indexer with `GODEBUG=gctrace=1` during bulk, then capture `net/http/pprof` or `runtime/pprof` heap + allocs after 10 min idle (document how to enable pprof on indexer binary for dev builds).
  - Compare **inuse_space** top types: `[]byte`, `string`, queue/slice growth, JSON buffers, sqlite driver.
- **Goroutine dump** at idle: rule out worker/blocking leaks.
- **Long-lived field audit** on `Indexer` struct (`indexer.go`): `queue`, `matchers`, `remoteInv`, maps for scope status, `syncState`, fsnotify handles.
- **Per-subsystem estimates** for porcelain repro:
  - Queue empty: confirm `Len()==0` and no fan-out `Candidates` retained.
  - fsnotify: directory count under watch root vs ignore rules.
  - SQLite: `sync_entries` row count and driver cache behavior (`modernc.org/sqlite`).
  - Manifest path: max file size ingested; count of files that required full upload vs skip.
- **Per-workspace retention**: after adding a second workspace, profile what scales (~744 MB observed); separate from Go heap-not-returned-to-OS on a single workspace.
- **`conhost` analysis** (optional): document whether indexer or other children spawn hosts; operator reports appearance post–SQLite/chunking—correlation unconfirmed.
- **Comparison table**: indexer vs gateway idle heap (same machine, same session).

**Acceptance**

- Written **root-cause ranking** (P0/P1/P2) with “retained at idle” yes/no per item.
- At least one profile or equivalent measurement attached to PR/issue (screenshot or saved `pprof` proto).
- Decision: **leak** (unbounded growth) vs **retained working set** (stable high plateau)—**resolved:** no growth over **24 h** idle with no file changes; RSS **grows when adding workspaces** (see [Decisions](#decisions)), not unbounded idle leak.

**Status:** `done`

### Process comparison (idle, same session)

| Process | Private MB | Handles |
|---------|------------|---------|
| chimera-indexer | **1434** | **18030** |
| chimera-gateway (backend) | 62 | 237 |
| chimera-supervisor | 55 | 262 |
| chimera-vectorstore | 54 | 212 |
| chimera-broker | 52 | 204 |

### Directory watch estimate (filesystem walk, 2026-06-03)

| Root | Total dirs on disk | Rough ignored (`.git`, `node_modules`, `data`, …) | If watch ignores ingest rules |
|------|-------------------|---------------------------------------------------|-------------------------------|
| `porcelain` | 8713 | ~5379 | ~3334 (ingest discovers **867** files) |
| `minecraft-character-editor` | 2678 | ~976 | ~1702 (**266** files) |

**Before fix:** `addRecursiveWatch` registered **every** directory (no `Matcher`). That explains **~11k+** watch registrations and **~18k** handles vs **~1.1k** ingested files.

### Root-cause ranking

| Priority | Cause | Retained at idle? | Evidence |
|----------|--------|-------------------|----------|
| **P0** | Go heap not returned to OS after bulk manifest ingest (4 workers × full-file JSON) | **Yes** | RSS flat **1.43 GB** for 57+ min after `watch_idle`; queue empty |
| **P0** | **ReindexTracker** treated first workspace poll as generation bump → cleared sync → full re-ingest on every process start | Peak + retained heap | `ApplyWorkspacesReindex`: `!seen` fell through to `DeleteByRoot`; logs show `skip_unchanged_*: 0` and `indexer.reindex.requested` at 15:45:24Z |
| **P1** | fsnotify watches **unignored** directories (`.git`, `node_modules`, `data`, …) | **Yes** | `watch.go` had no matcher; ~8–11k dirs vs 867 candidates |
| **P1** | Supervised **hot_reload** after `watch_shutdown_timeout` (40s) doubled scan/ingest work | Peak | 15:46:04Z ERROR + second `indexer.run.start` |
| **P2** | Multi-workspace maps + SQLite (~528 KB DB) + HTTP pools | **Yes** | Scales per workspace; smaller than P0/P1 |
| **P2** | `ListEntries()` full-table slice on coherence poll | Transient | `coherence.go`; periodic, not 1.4 GB alone |
| **P3** | `conhost` per piped child | N/A (~MB) | Parent map; not RAM driver |
| **—** | `remoteInv` | No | Inventory 404 / not loaded in repro |

**Leak vs retain:** **Confirmed retained plateau** — no RSS growth 15:48Z–16:45Z with `queue_depth: 0`.

**pprof:** Not wired on production binary; use `go test`/dev build with `net/http/pprof` for heap confirmation (Phase 3 doc).

### Phase 3 started (code)

| Change | File | Intent |
|--------|------|--------|
| Baseline reindex generation on first poll (no sync clear) | `reindex.go` + `reindex_test.go` | Warm restart skips when corpus unchanged |
| fsnotify `addRecursiveWatch` respects `Matcher` ignores | `watch.go` + `watch_test.go` | Cut watch handles and kernel bookkeeping |

---

## Phase 3 — Mitigations and ship

**Goal.** Reduce idle and peak indexer RAM with measurable targets, without breaking manifest ingest or supervised watch.

**Deliverables**

- **P0 — Operator mitigations** (docs only, can ship before code):
  - Tune `workers: 2`, optional lower `queue_depth`.
  - Expand `ignore_extra` for `data/`, build artifacts if needed.
  - Expect RAM to scale with **each supervised workspace**; document multi-workspace footprint.
  - **Corpus inventory 404:** track under unfinished indexer implementation; fix there unless profiling shows it drives idle RAM (primarily skip/churn, not the 1.2 GB plateau).
- **P1 — Code mitigations** (pick based on Phase 2; examples):
  - Manifest: avoid holding normalized text + marshaled JSON simultaneously; stream or reuse buffer; cap concurrent manifest build memory.
  - Fan-out: drop `Candidates` slice after enqueue; avoid duplicate `append` copies where safe.
  - Scan: do not retain full `all` slice after fan-out scheduled; paginate discovery logging (counts at INFO, paths at DEBUG).
  - Coherence: paginate `ListEntries` instead of full-table slice every poll.
  - Optional idle: `debug.FreeOSMemory()` once when entering `watch_idle` (evaluate impact on Windows).
- **P2 — Config defaults**: consider lowering default `workers` in `config/indexer.example.yaml` for desktop profiles.
- **conhost**: if Phase 2 shows unintended console hosts, ensure supervised indexer path always uses `CREATE_NO_WINDOW` and no spurious `MultiWriter` to stdout without console.
- Update [`docs/features/indexer.md`](../features/indexer.md) **Out of scope** → add **Memory** subsection with measured targets and operator guidance.
- Update [`docs/indexer.md`](../indexer.md) troubleshooting: expected RAM order-of-magnitude, how to profile.

**Acceptance**

- On Windows repro machine (porcelain-like workspace, 866 files): after **10 min idle**, indexer private working set **≤300 MB** (stretch **≤150 MB**) **or** documented justified floor with profile proof.
- Peak during bulk ingest may still spike higher; document expected peak vs idle.
- No regression: `go test ./chimera/chimera-indexer/...`; supervised ingest + `watch_idle` smoke test.
- `conhost` count documented: either explained as benign or reduced for supervised desktop launch.

### Post-fix validation (2026-06-03, PID 8360)

After rebuild + locus-desktop restart; operator added workspace **`assistants`** (three workspaces total: porcelain, minecraft-character-editor, assistants).

| Metric | Before fix (idle) | After fix (idle) |
|--------|-------------------|------------------|
| Task Manager private WS | **~1,434 MB** | **~58 MB** (operator: 58,796 K) |
| Handles | **18,030** | **548** |
| Workspaces | 2 | 3 (new workspace ingested then `watch_idle`) |

Logs (`index_run_id` `df81f1cc-…`): `watch_idle`, `queue_depth: 0`, `ingest_completed: 269` for the new scope (bulk work largely skipped on warm roots). Meets Phase 3 acceptance (**≤300 MB** idle) with large margin.

**Status:** `done`

### Additional mitigations shipped (same day)

| Change | Intent |
|--------|--------|
| `IngestManifestBody` + single `json.Marshal` on whole-file path | Avoid duplicate manifest JSON allocation during ingest |
| `debug.FreeOSMemory()` on global transition to `watch_idle` | Nudge Go to return heap to OS after bulk (Windows) |
| `SyncState.ForEachEntry` | Coherence scan streams rows (API for future paginated push) |
| Docs | [features/indexer.md](../features/indexer.md#memory-and-windows-resources), [indexer.md](../indexer.md) troubleshooting |

### Follow-up fixes (post-validation)


| Item | Status |
|------|--------|
| **Per-scope corpus inventory** | Shipped — one paginated inventory fetch per `DistinctEffectiveStorageStatsScopes`; keys `CorpusInventoryKey(project, flavor, rel)` |
| **Supervised `watch_shutdown_timeout`** | Mitigated — parallel per-root watch registration; cancel returns without serial full-tree walk |

### Remaining backlog (optional)

| Item | Notes |
|------|--------|
| **Supervised `watch_shutdown_timeout`** | Re-test after reload with many workspaces; extend grace if still seen |
| **Peak RAM on first ingest** | Large new workspace may still spike; tune `workers: 2` if needed |
| **Fan-out / scan slice retention** | Low priority post-fix; profile if peak ingest RAM matters |
| **conhost** | Documented as piped-stdio artifact; optional future: avoid conhost per child |
| **pprof** | Dev-only heap capture procedure; not on release binary |

---

## Decisions

Resolved 2026-06-03 from operator feedback.

### 1. Target idle RSS

**≤300 MB per ~1k-file workspace + fsnotify is acceptable** as the plan’s success bar (Phase 3 acceptance). Other Chimera children at **~40 MB** are cited only as **reference** (same stack, Go binaries)—**not** a target architecture for the indexer.

### 2. Leak vs retain

| Scenario | Behavior |
|----------|----------|
| **Idle, no file changes (≥24 h)** | RSS does **not** keep growing—treat as **retained heap + OS working set**, not an idle leak. |
| **Add supervised workspace** | RSS **grows** with each new workspace on the same indexer process (pre-fix; post-fix idle **~58 MB** with three workspaces—re-measure peak during first ingest of a large new root). |

**Measured example — pre-fix** (same `chimera-indexer.exe` PID **29384**, Task Manager active private working set):

| State | Workspace(s) | Memory |
|-------|----------------|--------|
| Before | `~/src/porcelain` only | **1,237,424 K** (~1.2 GB) |
| After | porcelain + new `~/src/minecraft-character-editor` | **1,981,916 K** (~1.9 GB) |

**~744 MB** delta for one additional workspace—Phase 2 should attribute per-workspace retention (sync SQLite, watches, collections, maps) vs one-time bulk spike.

### 3. `conhost.exe`

- **Not** the primary memory issue (~800 K each).
- Operator did **not** see `conhost` when filtering Task Manager for “chimera” under locus-desktop **before** SQLite sync state + manifest chunking; instances appeared **after** those changes—**correlation uncertain**.
- Parent-PID mapping remains optional diagnostics; do not block memory mitigations on conhost fixes.

### 4. Corpus inventory 404

Fix in **indexer implementation** backlog (inventory API / routing still incomplete). Include in this plan only if profiling ties it to idle RAM; otherwise ship with general indexer work (skip/churn, not the GB plateau).

### 5. Workspace scope (product)

**Do not** discourage indexing repository root (e.g. full monorepo) in desktop defaults or operator copy—no product guidance to narrow scope; technical mitigations (`ignore_extra`, workers) remain fair game.

---

## References

- Code: `chimera/chimera-indexer/internal/indexer/{ingest.go,manifest.go,scan_fanout.go,queue.go,indexer.go,syncstate.go,coherence.go}`
- Code: `chimera/chimera-indexer/internal/platform/stdio_tee_windows.go`
- Code: `chimera/chimera-supervisor/internal/supervise/indexer.go`, `internal/proc/sysproc_windows.go`
- Config: [`config/indexer.yaml`](../../config/indexer.yaml), [`config/indexer.example.yaml`](../../config/indexer.example.yaml)
- Logs: [`data/locus-desktop-supervisor.log`](../../data/locus-desktop-supervisor.log) (2026-06-03 repro)
- Features: [`indexer.md`](../features/indexer.md), [`indexer-ingest-pipeline.md`](../features/indexer-ingest-pipeline.md)
- Plans: [`indexer-manifest-ingest.md`](indexer-manifest-ingest.md), [`indexer-scan-and-fanout-jobs.md`](indexer-scan-and-fanout-jobs.md)
