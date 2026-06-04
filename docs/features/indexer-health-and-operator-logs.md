# Feature: Indexer health, recovery, and operator logs

| Field | Value |
|-------|-------|
| **Doc kind** | `feature-record` |
| **Areas** | Gateway indexer health API, `chimera-indexer` recovery, `servicelogs`, settings embed UI |
| **Status** | `current` |
| **Introduced** | Gateway + indexer minor after v0.2 Phase 5 observability; health/quiet-logs plan 2026-05 |
| **Originated from** | [`plans/indexer.md`](../plans/indexer.md) Phase 5, [`plans/indexer-health-and-quiet-logs.md`](../plans/indexer-health-and-quiet-logs.md) |
| **Related features** | [Workspace file indexer](indexer.md), [Indexer ingest pipeline](indexer-ingest-pipeline.md), [Indexer workspaces](indexer-workspaces.md) |
| **Depends on** | Broker catalog snapshot, vector store health, supervised `--log-json` stderr tee |
| **Last updated** | See git history |

## At a glance

The gateway answers **“can ingest succeed?”** for the indexer—not merely “is Qdrant up?”—via structured checks on vector store reachability and configured embedding model presence in the live catalog. When either check fails, the indexer opens a process-wide **ingest gate** so workers stop dequeuing ingest jobs instead of hammering 502 responses. Recovery waits until `IngestReady()` before resuming. Supervised runs emit JSON **slog** on stderr with stable `msg` slugs; default log profiles keep INFO readable through batched skip summaries and edge-triggered `indexer.scope.status` lines while per-file trace stays at DEBUG. Critical lifecycle lines are pinned in the gateway log ring so heavy traffic cannot evict `indexer.run.start`.

## Operator-visible behavior

- **Settings log feed** — Filter `indexer` source; workspace cards show phase, queue depth, embed gate state, and rollup skip/ingest counts from parsed structured lines.
- **Paused for embedding** — When Ollama (or another embed provider) is down but Qdrant is up, cards and recovery copy reflect `embed_reason_code` rather than a misleading “healthy storage” story.
- **Operator vs trace profiles** (supervised default):
  - **Operator:** `log_level: info`, `job_skip_log: debug` — lifecycle, errors, ingests, ~5s skip summaries.
  - **Trace:** `log_level: debug`, `job_skip_log: info` — adds per-file active file, skip, and pre-upload lines.
- **Log volume** — Idle watch mode does not emit repeating skip summaries; summaries are activity-gated.

Full slug tables and YAML knobs: [`docs/indexer.md`](../indexer.md).

## System behavior and contracts

**Invariants**

- **`GET /v1/indexer/storage/health` top-level `ok`** is true only when **both** vector store and embedding checks pass (when RAG enabled).
- **HTTP 200 on degraded** — Indexer polls JSON without treating degraded body as transport failure; auth/RAG-disabled remain **503**.
- **Recovery uses `IngestReady()`** — not vector-store-only OK.
- **Ingest gate** — Closed on not-ready health; workers block before ingest attempts; transitions log once at INFO (`indexer.ingest.gate.closed` / `.open`).
- **Embed short-circuit** — After first embed-classified 502/503, `retry_short_circuit_on_embed` (default **true**) skips remaining per-file retries and closes gate.
- **Scope status** — `indexer.scope.status` emits on meaningful field changes plus ~45s heartbeat fallback (`change_reason: heartbeat`).
- **Log pinning** — Gateway retains latest run start/done, gate lines, scope status per target key, and skip summaries before trimming indexer source buffer.

**Decisions**

| Topic | Decision |
|-------|----------|
| Embedding `reason_code` | Stable strings: `vectorstore_unreachable`, `embed_model_not_in_catalog`, `embed_provider_down`, `embed_provider_key_missing`, `embed_catalog_stale`, … |
| Per-file trace level | `indexer.scope.active_file`, `indexer.job.skipped`, `indexer.job.upload` default DEBUG via `job_skip_log: debug` |
| Skip rollups | `indexer.job.skipped.summary` at INFO; min interval default 5s; disable with `skip_summary_min_interval_ms: -1` |
| Recovery poll logging | Repeated polls at WARN while waiting; edge-triggered INFO on state change |
| Supervised logging | `--log-json` default true; adds `index_run_id`, `service: indexer`, tenant/user scope fields |
| Legacy YAML | `verbose_job_logs` deprecated; maps to `job_skip_log` when unset |

**Identity / auth / scoping**

- Health and stats are Bearer-scoped like ingest; embedding check uses gateway runtime catalog snapshot and provider classification (same family as provider health UI).

## Interfaces

| Surface | Detail |
|---------|--------|
| `GET /v1/indexer/storage/health` | `checks.vectorstore`, `checks.embedding` with `reason_code`, `model_in_catalog`, `provider_state` |
| `GET /health` | Optional root readiness when `recovery_include_root_health: true` (default) |
| Indexer client helpers | `HealthStatus.IngestReady()`, `EmbedOK()`, `VectorstoreOK()`, `ReasonCode()` |
| Gateway config | `operator_logs.indexer_pinned_lines_max` (pin budget for indexer source) |
| Supervised gateway | `indexer.supervised.log_json` |
| UI formatters | `embed/embedui/settings/operator_copy.js`, `render/operatorMessageIndexer.js`, `summarizedFeed.js` |

## Code map

| Concern | Location |
|---------|----------|
| Gateway health assembly | `internal/server/indexerapi/health.go`, `indexer.go` (`HandleHealth`) |
| Indexer health client | `chimera-indexer/internal/indexer/client.go` |
| Ingest gate | `internal/indexer/ingest_gate.go` |
| Recovery loop | `internal/indexer/workers.go` (`waitForRecovery`, gate interaction) |
| Scope status edge + heartbeat | `internal/indexer/scope_live_status.go`, `scope_status_edge.go` |
| Skip summaries | `internal/indexer/ops_events.go`, ingest skip counters |
| Log pinning | `chimera/internal/servicelogs/pin_indexer.go`, `store.go` |
| Supervised stderr JSON | `chimera/chimera-indexer/main.go`, `internal/indexer/supervised_file.go` (defaults) |
| Operator message registry | `internal/naming/log_messages.go`, `internal/operatorcopy/messages.yaml` |
| UI tests | `embedui_test/settings_components_test.go`, `settings_summarized_dirty_test.go` |

## Verification

```bash
go test ./chimera/chimera-gateway/internal/server/indexerapi/ -run Health
go test ./chimera/chimera-indexer/internal/indexer/ -run 'Gate|ScopeStatus|IngestReady'
go test ./chimera/internal/servicelogs/...
```

Manual: stop embed provider with Qdrant running; confirm health `ok: false`, gate closed, no sustained `indexer.job.upload` spam; restart provider and confirm gate opens and queue drains.

## Out of scope and known gaps

- **Lightweight embed probe POST** on health endpoint — not implemented; catalog + provider classification only.
- **Remote log shipping** (Splunk, etc.) — in-process ring buffer only.
- **Replacing gateway ingest metrics** with indexer-reported truth — gateway/Qdrant remain authoritative for stored corpus.
- **Storage-stats empty collection** — friendly operator copy and INFO-level event log for expected 404; indexer auto-clears sync checkpoints once per scope when collection is missing.
- **Summarized UI card-per-scope for every rollup mode when N>1** — partial coverage; some story/rollup paths not fully verified.

## References

- Plans: [`plans/indexer-health-and-quiet-logs.md`](../plans/indexer-health-and-quiet-logs.md), [`plans/indexer.md`](../plans/indexer.md) Phase 5
- Operator slug reference: [`docs/indexer.md`](../indexer.md)
- Parent: [`indexer.md`](indexer.md)
