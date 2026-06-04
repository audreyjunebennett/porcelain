# Feature: Indexer ingest pipeline (queue, scan, skip)

| Field | Value |
|-------|-------|
| **Doc kind** | `feature-record` |
| **Areas** | `chimera-indexer` queue, scan/fan-out, scope merge, sync state, gateway ingest |
| **Status** | `current` |
| **Introduced** | Gateway v0.2 baseline; scan/fan-out queue refactor in later minors |
| **Originated from** | [`plans/indexer.md`](../plans/indexer.md) Phases 2–4, [`plans/indexer-scan-and-fanout-jobs.md`](../plans/indexer-scan-and-fanout-jobs.md) |
| **Related features** | [Workspace file indexer](indexer.md), [Indexer health and operator logs](indexer-health-and-operator-logs.md) |
| **Depends on** | Gateway ingest + corpus inventory APIs, local ignore engine |
| **Last updated** | See git history |

## At a glance

After startup config fetch, the indexer loads remote corpus inventory, schedules a **ScanJob** (not a synchronous per-file enqueue), walks roots with ignore/binary rules, and fans candidates into bounded **FanoutListJob** chunks that enqueue tier-1 ingest work. Fair-share caps prevent one project+flavor from monopolizing the bulk queue when multiple scopes run. Filesystem edits enqueue higher-priority ingest jobs (Write tier 2, Create tier 3). Before upload, the pipeline skips unchanged files using corpus inventory, local SQLite sync state, or empty-content checks. Successful ingests persist client and server SHA-256 digests locally (incremental upsert per file).

## Operator-visible behavior

- **Initial indexing** — Large trees progress without silently dropping files at queue capacity; logs show per-scope discovery summaries (`indexer.discovery.summary.scope`), scan complete (`indexer.scan.complete`), and periodic queue snapshots.
- **Multi-workspace fairness** — With several scopes, candidate ordering is **round-robin interleaved** by `(project, flavor)` before fan-out so one root does not block others in logs or scheduling.
- **Live edits** — Saving a file triggers debounced re-ingest ahead of bulk backlog.
- **Skips** — At default supervised settings, unchanged files roll up into `indexer.job.skipped.summary` INFO lines (~5s windows), not thousands of per-file INFO lines.

## System behavior and contracts

**Invariants**

- **Work kinds** — `WorkScan`, `WorkFanoutList`, `WorkIngest` share one priority-aware queue; ingest dedup key is root id + relative path.
- **No `EnqueueInitialScan` flood** — `ScheduleInitialScan` enqueues exactly one tier-1 `ScanJob`.
- **Fan-out chunk size** — 4096 candidates per initial `FanoutListJob` payload; remainders chain as new list jobs.
- **Fair-share budget** — `per_scope_fanout_budget = floor(cap × p / max(N, 1))` where `p` defaults to 0.75 (`queue_fanout_high_water_mark_percent`) and `N` is distinct scope keys from walk skips ∪ candidates.
- **`initial_scan_complete`** — Set after scan finishes and fan-out jobs are **queued**, not when all ingests finish.
- **Scope at discovery time** — Each candidate carries project/flavor from the same `IngestHeaders` rules used at ingest.
- **Sync state** — Indexer-local SQLite (`sync-state.sqlite` beside configured `sync_state_path` when it ends in `.json`); incremental `Put` per file; legacy JSON migrates once to `.bak`.

**Decisions**

| Topic | Decision |
|-------|----------|
| Priority dequeue | Tier 3 (create) > tier 2 (write) > tier 1 (bulk scan/fan-out/scan ingests) |
| Dedup vs priority | Re-enqueue same path at higher tier **upgrades** pending item |
| Ingest modes | Whole-body under `max_whole_file_bytes`; session chunks above; each session step retries with same backoff as whole-file |
| Skip order | Corpus client hash → corpus + sync → local sync only → empty/whitespace |
| Ignore stack | Built-ins + `ignore_extra` + `.locusignore` + `.gitignore` + binary NUL sniff |
| Queue on crash | In-memory only; remainder fan-out payloads lost if process dies mid-backlog |
| Queue full during fan-out | Remainder re-enqueue may warn/fail; no durable retry loop |

**Persistence**

- Sync state: `sync-state.sqlite` (resolved from `sync_state_path`; default config still names `indexer.sync-state.json`) — rows keyed by `root_id + "\x00" + rel_path`; optional `chunk_count`, `chunk_schema`.

## Interfaces

| Surface | Detail |
|---------|--------|
| `GET /v1/indexer/corpus/inventory` | Paginated `source`, `content_sha256`, optional `client_content_hash` |
| `POST /v1/ingest` | JSON `ingest.manifest` only + scope headers |
| Session API | `POST /v1/ingest/session`, `POST …/complete` with full manifest JSON (transport chunks optional) |
| Headers | `X-Chimera-Project`, `X-Chimera-Flavor-Id` per resolved scope |
| YAML | `workers`, `queue_depth`, `debounce_ms`, `retry_*`, `sync_state_path`, `max_whole_file_bytes`, `queue_fanout_high_water_mark_percent`, `job_skip_log`, `skip_summary_min_interval_ms` |
| Key log slugs | `indexer.discovery.summary.scope`, `indexer.scan.complete`, `indexer.queue.snapshot`, `indexer.job.ingested`, `indexer.job.skipped.summary`, `indexer.fanout.*`, `indexer.coherence.stale` (DEBUG, rate-limited per source) |
| Coherence | `CollectStaleSources`, `PushStaleSources` → `PUT /v1/indexer/corpus/stale` on workspace poll (~30s) |

## Code map

| Concern | Location |
|---------|----------|
| Work model | `internal/indexer/work.go` |
| Priority queue | `internal/indexer/queue.go` |
| Scan + fan-out | `internal/indexer/scan_fanout.go` (`interleaveTaggedCandidatesByScope`, `runScanJob`, `runFanoutList`) |
| Ingest + skip | `internal/indexer/ingest.go` |
| Sync state SQLite | `internal/indexer/syncstate.go` |
| Manifest build + upload | `internal/indexer/manifest.go`, `ingest.go`, `client.go` |
| Coherence | `internal/indexer/coherence.go`, `coherence_push.go` |
| Scope merge | `internal/indexer/scope.go` |
| Walk + ignore | `internal/indexer/walk.go`, ignore matcher packages |
| Workers | `internal/indexer/workers.go`, `indexer.go` (`ScheduleInitialScan`, `processWorkItem`) |
| Debounced watch | `internal/indexer/debounce.go` |
| Tests | `scan_fanout_test.go`, `queue_test.go`, `indexer_test.go` |

## Verification

```bash
go test ./chimera/chimera-indexer/internal/indexer/ -run 'Scan|Fanout|Queue|Interleave|ScheduleInitial'
```

Manual: index a tree with 10k+ unchanged files; confirm queue depth stays bounded and INFO shows skip summaries rather than per-file spam.

## Out of scope and known gaps

- **Force re-index** — operator bumps `reindex_generation` via settings or API; indexer poll clears checkpoints (`indexer.reindex.requested`). **Auto-repair** clears sync rows once per scope when storage stats report a missing Qdrant collection (`indexer.reindex.auto_collection_missing`).
- **Tier-3 delete / fsnotify Remove** — not implemented.
- **Durable queue** — crash loses pending fan-out remainders.
- **Integration tests** for queue-full remainder chains under multi-scope contention — partial unit coverage only.
- **Session resume** after partial chunk failure without restarting from byte zero — limited; may restart session on outer retry.

## References

- Plans: [`plans/indexer.md`](../plans/indexer.md), [`plans/indexer-scan-and-fanout-jobs.md`](../plans/indexer-scan-and-fanout-jobs.md)
- Operator guide: [`docs/indexer.md`](../indexer.md) (ignore rules, corpus inventory, modes)
- Parent: [`indexer.md`](indexer.md)
