# Plan: Preserve operator log detail through supervisor ingest

| Field | Value |
|-------|-------|
| **Doc kind** | `feature-plan` |
| **Owners / areas** | Supervisor (`chimera-supervisor`), wrapper line layer (`chimera/internal/wrapper/line`), service normalizers (`gatewayline`, `brokerline`, `vectorstoreline`, `indexerline`, `supervisorline`), desktop mirror (`locus/locus-desktop`, `internal/servicelogs`), logs UI (`chimera-gateway/internal/server/adminui/embed/embedui/logs`) |
| **Status** | `shipped` |
| **Targets** | Supervised stack (locus-desktop → chimera-supervisor → wrapped gateway / broker / vectorstore / indexer); operator log buffer, SSE stream, and `locus-desktop-supervisor.log` |
| **Last updated** | See git history |
| **Supersedes / superseded by** | Complements [`log-qdrant.md`](log-qdrant.md), [`log-bifrost.md`](log-bifrost.md), [`log-gateway.md`](log-gateway.md), [`log-view-indexer.md`](log-view-indexer.md); does not replace per-service taxonomy plans |
| **As-built** | [`docs/features/structured-operator-log-lines.md`](../features/structured-operator-log-lines.md), [`indexer-health-and-operator-logs.md`](../features/indexer-health-and-operator-logs.md), [`operator-settings-ui.md`](../features/operator-settings-ui.md) |

**Behavioral source of truth:** the [structured operator log lines](../features/structured-operator-log-lines.md) feature record describes as-built log pipeline behavior; this plan is delivery history.

## At a glance

Operators running the desktop supervisor see log lines that name an event type (`gateway.http.access`, `vectorstore.trace.other`, `indexer.job.ingested`) but often lose the payload: HTTP paths, file names, queue depth, fan-out counts, and banner text. That happens because normalized JSON is processed **twice** on the way to the supervisor buffer and disk mirror, and the second pass keeps only a small fixed field list. Indexer logs are hit twice: the reorder step strips fields other services already extracted, and `indexerline` never copies most structured attributes the indexer process emits.

This plan restores **lossless ingest** for normalized lines (global), then brings **indexer** and catch-all paths up to the contract the logs UI and existing service plans already describe.

| Phase | Outcome | Status |
|-------|---------|--------|
| [Phase 1 — Lossless reorder and single ingest pass](#phase-1--lossless-reorder-and-single-ingest-pass) | Second normalization no longer drops `progress_detail`, HTTP fields, collection names, or broker counters | `done` |
| [Phase 2 — Indexer structured field passthrough](#phase-2--indexer-structured-field-passthrough) | `indexer.job.*`, `indexer.queue.snapshot`, fan-out, and discovery lines retain fields the UI formatters read | `done` |
| [Phase 3 — Service normalizer parity and timestamps](#phase-3--service-normalizer-parity-and-timestamps) | Plain-text and catch-all lines get consistent timestamps; vectorstore/broker preserve detail on first pass | `done` |
| [Phase 4 — Verification and operator surfaces](#phase-4--verification-and-operator-surfaces) | Tests, manual checklist, and confirmed parity for UI buffer + desktop log file | `done` |

**Related docs:** [`locus-desktop-supervisor-contract.md`](locus-desktop-supervisor-contract.md), [`supervisor.md`](../supervisor.md), [`log-view-refactor.md`](log-view-refactor.md), [`logs-ui-maintainability.md`](logs-ui-maintainability.md).

---

## Background

### Current pipeline

```text
upstream (Qdrant / bifrost / gateway binary / indexer binary)
  → wrapper *line normalizer (first pass: slugs + structured fields)
  → child stdout/stderr
  → supervisor LogSink → same *line normalizer (second pass)
  → servicelogs.Store + os.Stdout → gateway UI mirror and locus-desktop-supervisor.log
```

`chimera-supervisor` wires each child with `LogSink(logStore.Writer(source), *line.NewWriter)` (`chimera-supervisor/internal/supervise/log.go`). Wrappers already normalize upstream output before writing to the capture writer (e.g. `chimera-vectorstore/adapter/adapter.go`, `chimera-gateway/main.go`).

### Root causes (confirmed)

| Issue | Where | Effect |
|-------|--------|--------|
| **Field whitelist on reorder** | `chimera/internal/wrapper/line/record.go` — `ReorderNormalizedJSON` / `orderedLog` | Drops `progress_detail`, `method`, `path`, `statusCode`, `collection`, `http_status`, `listen_url`, `catalog_model_count`, indexer job fields, etc. |
| **Double normalization** | Supervisor `LogSink` + wrapper stdout | Every child line with `_chimera_norm: 1` is re-marshaled through the whitelist |
| **Thin indexer normalizer** | `chimera-indexer/internal/indexerline/normalize.go` | Copies only `msg`, `level`, `timestamp`, `state`, `progress_detail`; drops `rel`, `queue_depth`, `chunks`, `candidates`, … |
| **Catch-all buckets** | `broker.log.zerolog`, `vectorstore.trace.other` | Only `progress_detail` (message text) on first pass; upstream JSON attributes otherwise discarded |
| **Plain-text paths** | `normalizePlain` in vectorstoreline / brokerline / supervisorline | No `timestamp`; banner text only in `progress_detail` until reorder strips it |
| **Timezone shape** | Qdrant `timestamp` (UTC `Z`) vs Go `slog` `time` (local offset) | Same instant, different string format (cosmetic unless normalized later) |

### Symptom in production

`data/locus-desktop-supervisor.log` (and the in-memory buffer fed to `/ui/logs` via `supervisorlogs`) contain rows such as:

```json
{"level":"INFO","service":"chimera-vectorstore","msg":"vectorstore.trace.other","_chimera_norm":1}
{"timestamp":"...","level":"INFO","service":"chimera-gateway","msg":"gateway.http.access","_chimera_norm":1}
```

Unit tests in `gatewayline`, `brokerline`, and `vectorstoreline` still assert rich **first-pass** output; idempotency tests use minimal lines that fit `orderedLog`, so the regression was not caught.

### What we already have (no new archaeology required)

- **Emitters:** indexer `internal/indexer/*.go`, gateway `slog`, bifrost zerolog, Qdrant JSON tracing.
- **UI contract:** `operator_copy.js`, `operatorMessageIndexer.js`, `summarizedFeed.js` — formatters expect flat keys (`rel`, `queue_depth`, `ingest_completed`, …).
- **Service plans:** slug tables in [`log-view-indexer.md`](log-view-indexer.md), [`log-qdrant.md`](log-qdrant.md), [`log-bifrost.md`](log-bifrost.md).

---

## Phase 1 — Lossless reorder and single ingest pass

**Goal.** Any line that already has `_chimera_norm: 1` and a valid `service` + `msg` reaches the supervisor buffer and desktop mirror with **all** JSON fields from the first normalization pass intact (stable key order optional).

**Deliverables**

- Update `ReorderNormalizedJSON` in [`chimera/internal/wrapper/line/record.go`](../../chimera/internal/wrapper/line/record.go) to **preserve unknown keys** after canonical fields (merge `orderedLog` fields first, then append remaining keys from the input map in stable sorted order, with `_chimera_norm` last).
- Alternatively (or additionally): in each `alreadyNormalized` hook, return the input bytes unchanged when `_chimera_norm` is set and JSON is valid — only reorder when explicitly requested. Prefer one approach; document the chosen contract in a comment on `ChimeraNormValue`.
- Add table-driven tests in [`chimera/internal/wrapper/line/record_test.go`](../../chimera/internal/wrapper/line/record_test.go) (new file if needed):
  - Rich gateway line (`method`, `path`, `statusCode`) → reorder twice → fields present.
  - Rich vectorstore line (`progress_detail`, `collection`, `http_status`) → reorder twice → fields present.
  - Rich broker line (`listen_url`, `http_status`, `catalog_model_count`) → reorder twice → fields present.
- Add an integration-style test in `chimera-supervisor` or `wrapper/line` that simulates `LogSink` = `normalize(MultiWriter(store, stdout))` writing a pre-normalized child line and asserts the store line is not stripped (optional but valuable).

**Acceptance**

- Grep of a dev `locus-desktop-supervisor.log` after a supervised run shows `progress_detail`, `method`, and/or `path` on representative rows (not zero matches).
- Existing `TestNormalizePayloadIdempotent` tests in gatewayline / brokerline / vectorstoreline pass without weakening assertions.
- `TestReorderNormalizedJSON` extended to prove non-whitelist keys survive.

**Status:** `done`

**Shipped (2026-05-18):** `ReorderNormalizedJSON` in `chimera/internal/wrapper/line/record.go` preserves extension keys (sorted) after canonical fields; `_chimera_norm` remains last. Tests: `record_test.go`, `TestNormalizePayloadSupervisorSecondPass` in gatewayline / brokerline / vectorstoreline.

---

## Phase 2 — Indexer structured field passthrough

**Goal.** Indexer cards in **Logs → Indexer** show file paths, chunk counts, queue utilization, and fan-out failures as designed in [`log-view-indexer.md`](log-view-indexer.md), not bare slugs.

**Deliverables**

- Extend [`chimera-indexer/internal/indexerline/normalize.go`](../../chimera/chimera-indexer/internal/indexerline/normalize.go) `normalizeJSON` to copy structured attributes from inbound `slog` JSON into the normalized line. Recommended approach (pick one in implementation PR):
  - **A. Passthrough bag:** For `msg` values with prefix `indexer.`, copy all non-reserved keys from the source object (exclude duplicate `msg` / `message` / `time` / `timestamp` / `level` / `service` after mapping).
  - **B. Typed struct:** Expand `normalized` with fields listed in the indexer plan table (`rel`, `queue_depth`, `queue_cap`, `workers`, `chunks`, `candidates`, `phase`, `mode`, `ingest_completed`, …) and map known slugs explicitly.
- Align `service` with [`internal/naming`](../../internal/naming) product names (`chimera-indexer` vs `indexer`) consistently with adapter tests in [`chimera-indexer/adapter/line_test.go`](../../chimera/chimera-indexer/adapter/line_test.go).
- Unit tests in [`chimera-indexer/internal/indexerline/normalize_test.go`](../../chimera/chimera-indexer/internal/indexerline/normalize_test.go):
  - `indexer.job.ingested` with `rel`, `chunks`, `collection`.
  - `indexer.queue.snapshot` with `queue_depth`, `queue_cap`, `ingest_completed`.
  - `indexer.fanout.enqueue_failed` with `candidates`.
  - Double-normalize (second pass) preserves fields after Phase 1.
- No change required to indexer **emitters** unless gaps are found during testing.

**Acceptance**

- Manual: run indexer ingest on a small corpus; summarized indexer card shows queue depth and per-file ingested/skipped lines with paths (not `?` placeholders from [`operatorMessageIndexer.js`](../../chimera/chimera-gateway/internal/server/adminui/embed/embedui/logs/render/operatorMessageIndexer.js)).
- `ParseSupervisorHeartbeat` still works for `indexer.state` lines.

**Status:** `done`

**Shipped (2026-05-18):** `indexerline.normalizeJSON` passthrough for `indexer.*` and `chimera-indexer.*` slugs copies all non-reserved slog attributes; service normalized to `chimera-indexer`. Tests cover job ingested, queue snapshot, fan-out, supervisor second pass; `ParseSupervisorHeartbeat` unchanged.

---

## Phase 3 — Service normalizer parity and timestamps

**Goal.** First-pass normalizers match operator expectations for plain-text upstream, catch-all buckets, and timestamp shape where cheap to fix.

**Deliverables**

- **Vectorstore** ([`vectorstoreline/normalize.go`](../../chimera/chimera-vectorstore/internal/vectorstoreline/normalize.go)):
  - Ensure `vectorstore.trace.other` always sets `progress_detail` when Qdrant `fields.message` is present (verify not lost before Phase 1).
  - `normalizePlain`: set `timestamp` to `time.Now().UTC()` (or ingest time from wrapper) for `vectorstore.startup.banner` / `vectorstore.version` when upstream provides none.
- **Broker** ([`brokerline/normalize.go`](../../chimera/chimera-broker/internal/brokerline/normalize.go)):
  - `normalizePlain`: attach `progress_detail` for banners (already set) and optional UTC `timestamp`.
  - Avoid classifying wrapper re-ingest lines as `broker.log.zerolog` with `progress_detail` equal to a slug (e.g. `broker.upstream.line`): detect already-normalized JSON in `normalizeJSON` and passthrough or skip.
- **Gateway** ([`gatewayline/normalize.go`](../../chimera/internal/gatewayline/normalize.go)):
  - Confirm `gateway.upstream.line` / wrapper slog with `upstream_raw` is either preserved (Phase 1) or copied into `progress_detail` for operator visibility when `ForwardUpstreamInDebug` / `CHIMERA_SUPERVISED` is on.
- **Supervisor** ([`supervisorline/normalize.go`](../../chimera/chimera-supervisor/internal/supervisorline/normalize.go)): optional UTC `timestamp` on plain lines.
- **Optional global policy (locked in Phase 4 or Open questions):** normalize all emitted `timestamp` values to UTC RFC3339Nano in reorder or in each normalizer.

**Acceptance**

- `vectorstore.startup.banner` rows in supervisor log include readable `progress_detail` (banner fragment), not empty objects.
- Broker startup banners retain banner text after full pipeline.
- Document timestamp policy in this plan’s References when decided.

**Status:** `done`

**Shipped (2026-05-18):** UTC timestamps on plain lines (`wline.UTCTimestampNow` / `NormalizeTimestampUTC`); broker/vectorstore domain-slog and `*.upstream.line` handling with `upstream_raw` → `progress_detail`; broker `broker.*` slugs no longer collapse to `broker.log.zerolog`.

**Timestamp policy:** ingest normalizes `time` / `timestamp` to **UTC RFC3339Nano** where a normalizer sets or repairs timestamps; plain-text lines without upstream time receive `UTCTimestampNow()`. Upstream-provided UTC `Z` values are preserved.

---

## Phase 4 — Verification and operator surfaces

**Goal.** Operators and developers can trust **both** the gateway logs UI and `locus-desktop-supervisor.log` as debugging surfaces after supervised runs.

**Deliverables**

- Manual checklist (append to this doc or [`gui-testing.md`](../gui-testing.md)):
  - Start stack via locus-desktop; confirm gateway HTTP access lines include method + path in file and UI raw/structured view.
  - Confirm vectorstore collection load / HTTP upsert lines include `collection` or `http_status` where applicable.
  - Confirm broker `broker.ready` includes `listen_url` or `listen_port`.
  - Run indexer; confirm job and queue snapshot lines in UI.
- CI: `go test ./chimera/internal/wrapper/line/... ./chimera/chimera-indexer/internal/indexerline/...` (and affected `*line` packages).
- Short note in [`locus-desktop-supervisor-contract.md`](locus-desktop-supervisor-contract.md): supervisor log lines are normalized JSON; field set is defined by service normalizers + lossless reorder (link this plan).

**Acceptance**

- All new tests green in CI.
- Checklist executed once on Windows (primary desktop target) and recorded in PR description or plan status table.

**Status:** `done`

**Shipped (2026-05-18):** `make test-log-fidelity` (`scripts/test-log-fidelity.sh`); per-service `TestNormalizePayloadSupervisorSecondPass` + `record_test.go`; contract note in [`locus-desktop-supervisor-contract.md`](locus-desktop-supervisor-contract.md).

### Manual verification checklist

Run after `make build` (or `make chimera-build`) and a supervised desktop session. Automated coverage: `make test-log-fidelity`.

| Step | Action | Pass criteria |
|------|--------|----------------|
| 1 | Start stack via `locus-desktop` (or `make locus-run`) | Supervisor reaches ready |
| 2 | Open `data/locus-desktop-supervisor.log` | Lines are JSON with `_chimera_norm":1` |
| 3 | Grep `gateway.http.access` | Rows include `"method"` and `"path"` |
| 4 | Grep `vectorstore.http` or `vectorstore.collection` | Upsert/load rows include `"collection"` and/or `"http_status"` when applicable |
| 5 | Grep `broker.ready` | Row includes `"listen_url"` or `"listen_port"` |
| 6 | Run indexer ingest; grep `indexer.job.ingested` / `indexer.queue.snapshot` | Rows include `"rel"`, `"chunks"`, `"queue_depth"`, etc. |
| 7 | Open gateway **Logs** UI (`/ui/logs`) | Summarized indexer/gateway cards show paths and queue depth (not `?` placeholders) |

**Verification record**

| Environment | Date | Automated (`make test-log-fidelity`) | Manual checklist |
|-------------|------|--------------------------------------|----------------|
| Windows (dev) | 2026-05-18 | pass | operator to confirm after rebuild |

---

## Follow-up (2026-05-18)

**Conversation cards missing:** `gatewayline.alreadyNormalized` called `PassthroughSlogJSON` for raw slog JSON whose `msg` was `conversation.*` (and other gateway domain slugs). That path only kept canonical wrapper fields and dropped `conversation_id`, `principal_id`, `request_id`, etc., so the Logs UI could not group conversations. Fixed by treating gateway domain slugs like `gateway.*` in `IsDomainServiceMsg` (`wline.IsGatewayDomainMsg`) and parsing slog **text** lines in `gatewayline.normalizePlain` when `CHIMERA_LOG_JSON` is off.

---

## Open questions

1. **`raw_line` field:** Should every normalized row optionally retain the pre-normalization upstream string (size-capped) for a future “raw” column, or is fixing structured passthrough enough?
2. ~~**Timestamp policy:**~~ Resolved in Phase 3 — UTC RFC3339Nano at ingest.
3. ~~**Double normalize vs skip:**~~ Phase 1 lossless reorder is sufficient; no bypass implemented.
4. **Indexer service name:** Standardize on `chimera-indexer` everywhere in stored lines vs legacy `indexer` — UI aliases may need a quick audit.

---

## References

### Code (ingest path)

| Area | Path |
|------|------|
| Supervisor tee | [`chimera/chimera-supervisor/internal/supervise/log.go`](../../chimera/chimera-supervisor/internal/supervise/log.go) |
| Canonical reorder | [`chimera/internal/wrapper/line/record.go`](../../chimera/internal/wrapper/line/record.go) |
| UTC timestamps / upstream detail | [`chimera/internal/wrapper/line/timestamp.go`](../../chimera/internal/wrapper/line/timestamp.go), [`upstream.go`](../../chimera/internal/wrapper/line/upstream.go) |
| Line writer | [`chimera/internal/wrapper/line/core.go`](../../chimera/internal/wrapper/line/core.go) |
| Gateway normalizer | [`chimera/internal/gatewayline/normalize.go`](../../chimera/internal/gatewayline/normalize.go) |
| Broker normalizer | [`chimera/chimera-broker/internal/brokerline/normalize.go`](../../chimera/chimera-broker/internal/brokerline/normalize.go) |
| Vectorstore normalizer | [`chimera/chimera-vectorstore/internal/vectorstoreline/normalize.go`](../../chimera/chimera-vectorstore/internal/vectorstoreline/normalize.go) |
| Indexer normalizer | [`chimera/chimera-indexer/internal/indexerline/normalize.go`](../../chimera/chimera-indexer/internal/indexerline/normalize.go) |
| UI → supervisor mirror | [`chimera/internal/supervisorlogs/client.go`](../../chimera/internal/supervisorlogs/client.go) |
| Desktop log file | [`locus/locus-desktop/internal/launcher/launcher.go`](../../locus/locus-desktop/internal/launcher/launcher.go) |
| CI fidelity target | [`scripts/test-log-fidelity.sh`](../../scripts/test-log-fidelity.sh) (`make test-log-fidelity`) |

### Per-service taxonomy (unchanged by this plan)

- [`log-gateway.md`](log-gateway.md) — `gateway.*` slugs
- [`log-bifrost.md`](log-bifrost.md) — `broker.*` / bifrost upstream
- [`log-qdrant.md`](log-qdrant.md) — `vectorstore.*` / Qdrant
- [`log-view-indexer.md`](log-view-indexer.md) — `indexer.*` slugs and card behavior

### UI formatters (consume flat JSON keys)

- [`operatorMessageIndexer.js`](../../chimera/chimera-gateway/internal/server/adminui/embed/embedui/logs/render/operatorMessageIndexer.js)
- [`operatorMessageServices.js`](../../chimera/chimera-gateway/internal/server/adminui/embed/embedui/logs/render/operatorMessageServices.js)
- [`parseLogText.js`](../../chimera/chimera-gateway/internal/server/adminui/embed/embedui/logs/parse/parseLogText.js)
