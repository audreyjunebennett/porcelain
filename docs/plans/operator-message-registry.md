# Plan: Operator log message registry

| Field | Value |
|-------|-------|
| **Doc kind** | `refactor-plan` |
| **Owners / areas** | Gateway logs UI, log emission (`chimera-gateway`, wrappers, indexer, broker, vectorstore) |
| **Status** | `done` |
| **Targets** | One editable catalog for structured-log slugs and operator-friendly lines |
| **Last updated** | 2026-05-18 |
| **Supersedes / superseded by** | Extends [`log-presentation-layer.md`](log-presentation-layer.md); revisits open Q2 in [`chimera-gateway-refactor.md`](chimera-gateway-refactor.md) (codegen for JS constants) |

## At a glance

Operators read friendly sentences in the logs UI, but those strings are scattered across large JavaScript switches and duplicated slug matching in Go and JS. Introduce a single **operator message registry**: canonical `msg` slugs, legacy aliases, and editable summary templates—so changing how a line reads happens in one place and stays tied to the structured log type that produced it.

| Phase | Outcome | Status |
|-------|---------|--------|
| [Phase 1 — Inventory and schema](#phase-1--inventory-and-schema) | Complete slug list and YAML schema for copy + aliases | `done` |
| [Phase 2 — Gateway and conversation copy](#phase-2--gateway-and-conversation-copy) | Registry replaces `operatorFriendlyGatewayMsg` switch | `done` |
| [Phase 3 — Broker and vectorstore copy](#phase-3--broker-and-vectorstore-copy) | Registry replaces broker/vectorstore operator line switches; slug drift fixed | `done` |
| [Phase 4 — Indexer copy and aliases](#phase-4--indexer-copy-and-aliases) | Registry replaces `indexerProseSummary` + most of `indexerFlatMsg` | `done` |
| [Phase 5 — Go slug constants](#phase-5--go-slug-constants) | Emission sites use generated constants, not string literals | `done` |
| [Phase 6 — Shape and metrics metadata](#phase-6--shape-and-metrics-metadata) | Optional `shape` / counter tags drive `inferShape` and card metrics | `done` |

---

## Background

Three layers exist today with no shared catalog:

| Layer | Location | Job |
|-------|----------|-----|
| **Emit** | Go `slog` calls (`server.go`, `chat.go`, `rag/service.go`, indexer, …) | Write `"msg", "<slug>"` |
| **Classify** | `*line/normalize.go`, `indexerFlatMsg()` in JS | Map raw output → canonical slug |
| **Translate** | `primaryLogMessage()` → `operatorFriendlyGatewayMsg`, `chimeraBrokerOperatorLine`, `vectorstoreOperatorLine`, `indexerProseSummary` | Human-readable column |

[`internal/naming/gateway_logs.go`](../../internal/naming/gateway_logs.go) defines **`LogMsgPrefix*`** and timeline kinds only—not per-message slugs or operator copy.

**Known drift:** Go normalizers emit `broker.*` and `vectorstore.*` while JS still matches `chimera-broker.*` and `qdrant.*` in places—lines fall through to raw slugs.

**Related docs:** [`log-gateway.md`](log-gateway.md), [`log-bifrost.md`](log-bifrost.md) (historical; broker vocabulary), [`log-qdrant.md`](log-qdrant.md) (historical; vectorstore vocabulary), [`log-presentation-layer.md`](log-presentation-layer.md).

---

## Phase 1 — Inventory and schema

**Goal.** Agree on the registry file format and enumerate every slug the UI translates today.

**Deliverables**

- **`internal/operatorcopy/messages.yaml`** (or `docs/operatorcopy/messages.yaml`—pick one canonical path in PR) with schema:
  - `slug` (canonical key)
  - `summary` — static operator string, or omit when using `formatter`
  - `formatter` — id referencing shared JS/Go formatter (`http_inbound`, `rag_collection`, `truncate_err`, …)
  - `append` — list of `{ field, fmt, omit_in: event_log }` for dynamic tails
  - `aliases` — legacy slugs and human slog titles
  - `shape` — optional presentation shape (`http.access`, `chat.routing`, …)
  - `timeline_kind` — optional echo of [`gateway_logs.go`](../../internal/naming/gateway_logs.go)
- Script **`scripts/operatorcopy-inventory.sh`** (or `.ps1`): grep Go `"msg",` and JS `case "` / `msg ===` into a report for diffing against YAML.
- Design note in plan PR: registry is **copy + identity**, not storage or metrics logic.

**Acceptance**

- Inventory report checked in or generated in CI artifact; ≥90% of slugs handled in Phases 2–4 are listed.
- Schema validated by `go generate` in [`internal/operatorcopy`](../../internal/operatorcopy) (`cmd/validate`).

**Status:** `done`

**Implemented (2026-05-18)**

- Canonical registry: [`internal/operatorcopy/messages.yaml`](../../internal/operatorcopy/messages.yaml) (embedded; 140+ messages, English, `gallery_preview` on every slug).
- Schema + validation: [`internal/operatorcopy/schema.go`](../../internal/operatorcopy/schema.go), `go generate` via [`cmd/validate`](../../internal/operatorcopy/cmd/validate).
- Bootstrap catalog: [`bootstrap_registry.go`](../../internal/operatorcopy/bootstrap_registry.go) → `go run ./internal/operatorcopy/cmd/bootstrap`.
- Inventory: [`scripts/operatorcopy-inventory.ps1`](../../scripts/operatorcopy-inventory.ps1) / [`.sh`](../../scripts/operatorcopy-inventory.sh) → `go run ./internal/operatorcopy/cmd/inventory` (report: [`inventory-report.txt`](../../internal/operatorcopy/inventory-report.txt)).

---

## Phase 2 — Gateway and conversation copy

**Goal.** Gateway lifecycle, RAG, ingest, and supervisor strings editable in YAML; one JS renderer.

**Deliverables**

- `go generate` → `adminui/embedui/logs/operator_copy.js` (lookup object + alias map).
- `adminui/embedui/logs/render/operatorMessage.js`:
  - `resolveCanonicalSlug(flat)` — aliases + minimal legacy rules
  - `operatorMessage(flat, opts)` — replaces direct `operatorFriendlyGatewayMsg` body
- Migrate entries from `operatorFriendlyGatewayMsg` in [`logs_app.js`](../../chimera/chimera-gateway/internal/server/adminui/embedui/logs_app.js) (~40+ slugs).
- Goja tests: fixture per slug → expected summary string.
- Optional gallery subsection **Operator copy** ([`embedui-component-gallery.md`](embedui-component-gallery.md) open Q2).

**Acceptance**

- `operatorFriendlyGatewayMsg` removed or thin wrapper calling registry.
- Existing `logs_components_test.go` cases for gateway/conversation lines pass.
- Editing YAML + regenerate changes UI text without touching switches.

**Status:** `done`

**Implemented (2026-05-18)**

- `go generate` → [`logs/operator_copy.js`](../../chimera/chimera-gateway/internal/server/adminui/embed/embedui/logs/operator_copy.js) (`ChimeraLogs.OperatorCopy` lookup + aliases).
- [`logs/render/operatorMessage.js`](../../chimera/chimera-gateway/internal/server/adminui/embed/embedui/logs/render/operatorMessage.js): `resolveCanonicalSlug`, `operatorMessage`, formatters for gateway-phase slugs.
- [`logs_app.js`](../../chimera/chimera-gateway/internal/server/adminui/embed/embedui/logs_app.js): `operatorFriendlyGatewayMsg` delegates to registry renderer.
- Goja: [`operator_message_test.go`](../../chimera/chimera-gateway/internal/server/adminui/embed/embedui_test/operator_message_test.go).
- Make: `operator-copy-generate`, `contracts-check` (wired into `chimera-gateway-test`).

---

## Phase 3 — Broker and vectorstore copy

**Goal.** Broker relay and vectorstore backend lines use registry; canonical slugs align with normalizers.

**Deliverables**

- YAML entries for all slugs in [`chimeraBrokerMetrics.js`](../../chimera/chimera-gateway/internal/server/adminui/embedui/logs/derive/chimeraBrokerMetrics.js) `chimeraBrokerOperatorLine`.
- YAML entries for vectorstore slugs (migrate from [`vectorstoreCollection.js`](../../chimera/chimera-gateway/internal/server/adminui/embedui/logs/derive/vectorstoreCollection.js) `qdrantOperatorLine`).
- **Alias table:** `qdrant.*` → `vectorstore.*`, `chimera-broker.*` → `broker.*` for one release window; document removal date.
- `primaryLogMessage()` dispatch simplified to: HTTP special-case → `operatorMessage()` → field fallback.
- Update [`brokerline`](../../chimera/chimera-broker/internal/brokerline/normalize.go) / [`vectorstoreline`](../../chimera/chimera-vectorstore/internal/vectorstoreline/normalize.go) comments to point at registry (no prose in normalizers).

**Acceptance**

- Fixture lines with `msg=broker.ready` and `msg=vectorstore.version` render friendly text in UI tests.
- No remaining `case "qdrant.` in operator line switch (aliases only in registry).

**Status:** `done`

**Implemented (2026-05-18)**

- Full registry in [`operator_copy.js`](../../chimera/chimera-gateway/internal/server/adminui/embed/embedui/logs/operator_copy.js) (broker + vectorstore + gateway).
- Broker/vectorstore formatters: [`operatorMessageServices.js`](../../chimera/chimera-gateway/internal/server/adminui/embed/embedui/logs/render/operatorMessageServices.js).
- [`chimeraBrokerMetrics.js`](../../chimera/chimera-gateway/internal/server/adminui/embed/embedui/logs/derive/chimeraBrokerMetrics.js) / [`vectorstoreCollection.js`](../../chimera/chimera-gateway/internal/server/adminui/embed/embedui/logs/derive/vectorstoreCollection.js): operator lines delegate to registry (no `case "qdrant.` switches).
- [`logs_app.js`](../../chimera/chimera-gateway/internal/server/adminui/embed/embedui/logs_app.js) `primaryLogMessage`: HTTP shape → `operatorMessage()` → indexer fallback.
- Normalizer comments: [`brokerline`](../../chimera/chimera-broker/internal/brokerline/normalize.go), [`vectorstoreline`](../../chimera/chimera-vectorstore/internal/vectorstoreline/normalize.go).

---

## Phase 4 — Indexer copy and aliases

**Goal.** Indexer operator prose and slog-title disambiguation live in the registry.

**Deliverables**

- Migrate [`indexerPresent.js`](../../chimera/chimera-gateway/internal/server/adminui/embedui/logs/derive/indexerPresent.js) `indexerProseSummary` cases to YAML.
- Move **`indexerFlatMsg`** alias rules into registry `aliases` + `match_fields` (document duplicate `"msg"` key behavior from slog JSON).
- Keep **`indexerDeclaredStateLabel`** as small enum map in YAML or separate `states.yaml`.
- Retain **`shortIngestFailureDetail`** as named formatter referenced from registry.

**Acceptance**

- `indexerProseSummary` removed or ≤20 lines delegating to registry.
- `TestLogsDerive_indexerPartition_humanStartMsgSplitsBuckets` and related goja tests pass.

**Status:** `done`

**Implemented (2026-05-18)**

- `match_fields` / `match_prefix` on registry messages; `resolveFlat(flat)` in generated [`operator_copy.js`](../../chimera/chimera-gateway/internal/server/adminui/embed/embedui/logs/operator_copy.js) (slog duplicate `msg` keys).
- [`indexer_states`](../../internal/operatorcopy/messages.yaml) → `indexerStateLabels`; indexer formatters in [`operatorMessageIndexer.js`](../../chimera/chimera-gateway/internal/server/adminui/embed/embedui/logs/render/operatorMessageIndexer.js).
- [`indexerPresent.js`](../../chimera/chimera-gateway/internal/server/adminui/embed/embedui/logs/derive/indexerPresent.js) delegates `indexerFlatMsg` / `indexerProseSummary` to registry.

---

## Phase 5 — Go slug constants

**Goal.** Emitters and tests import slugs; drift caught at compile time.

**Deliverables**

- `go generate` → **`internal/naming/log_messages.go`** (const per canonical slug).
- Mechanical PRs: replace `"msg", "conversation.delivered"` with `naming.MsgConversationDelivered` in gateway, chat, rag, ingest (batch by package).
- CI check: new `"msg", "` string literals outside generated helpers fail audit (extend [`chimera-gateway-vocab-audit`](../../scripts/chimera-gateway-vocab-audit.ps1) or sibling script).
- Extend [`gateway_logs.go`](../../internal/naming/gateway_logs.go) doc comment to point at registry.

**Acceptance**

- `rg '"msg", "conversation\.'` under gateway core trends to zero (except codegen/tests).
- Registry completeness test: every `Msg*` const has YAML entry with summary or formatter.

**Status:** `done`

**Implemented (2026-05-18)**

- `go generate` → [`internal/naming/log_messages.go`](../../internal/naming/log_messages.go) (`Msg*` per canonical slug; 163 messages).
- Gateway `conversation.*` emitters use `naming.Msg*` (server, chat, merge, witness, tools, rag span).
- CI: `contracts-check` runs log_messages staleness + [`scripts/operatorcopy-msg-audit.sh`](../../scripts/operatorcopy-msg-audit.sh) (no raw `conversation.*` msg literals).

---

## Phase 6 — Shape and metrics metadata

**Goal.** Optional registry fields reduce duplicate slug matching for layout and counters.

**Deliverables**

- YAML optional fields: `shape`, `metrics_counter` (e.g. `chatResp`, `ragQuery`).
- Generate **`inferShape` hints** or replace prefix rules with registry-driven lookup where safe.
- Card derive modules (`gatewayCardModel.js`, etc.) import slug constants or generated JS enum instead of string literals.
- Document relationship to [`log-presentation-layer.md`](log-presentation-layer.md) shape taxonomy.

**Acceptance**

- At least gateway card counters sourced from registry tags for ingest/RAG/chat slugs.
- No behavior change in summarized metrics totals (fixture comparison).

**Status:** `done`

**Implemented (2026-05-18)**

- Optional `shape` and `metrics_counter` on registry messages (validated counter keys for gateway card).
- Generated [`operator_copy.js`](../../chimera/chimera-gateway/internal/server/adminui/embed/embedui/logs/operator_copy.js): `Slug`, `inferShapeForFlat`, `metricsCounterForFlat`.
- [`logs_app.js`](../../chimera/chimera-gateway/internal/server/adminui/embed/embedui/logs_app.js) `inferShape` consults registry before legacy prefix rules.
- [`gatewayCardModel.js`](../../chimera/chimera-gateway/internal/server/adminui/embed/embedui/logs/derive/gatewayCardModel.js): ingest/RAG/chat counters from `metrics_counter`; HTTP counts still from `http.access` shape.
- Cross-ref: [`log-presentation-layer.md`](log-presentation-layer.md) shape taxonomy.

---

## Resolved decisions

1. **Registry path:** **`internal/operatorcopy/messages.yaml`** (Go-owned, embedded for `go generate`).
2. **i18n:** English-only (`locale: en`); see [`log-conversations.md`](log-conversations.md).
3. **Gallery previews:** **Required for every slug** (`gallery_preview` field; validated in Phase 1).

---

## References

- Dispatch hub: [`logs_app.js`](../../chimera/chimera-gateway/internal/server/adminui/embedui/logs_app.js) (`primaryLogMessage`, `operatorFriendlyGatewayMsg`)
- Derive: [`logs/derive/chimeraBrokerMetrics.js`](../../chimera/chimera-gateway/internal/server/adminui/embedui/logs/derive/chimeraBrokerMetrics.js), [`vectorstoreCollection.js`](../../chimera/chimera-gateway/internal/server/adminui/embedui/logs/derive/vectorstoreCollection.js), [`indexerPresent.js`](../../chimera/chimera-gateway/internal/server/adminui/embedui/logs/derive/indexerPresent.js)
- Naming: [`internal/naming/gateway_logs.go`](../../internal/naming/gateway_logs.go)
- Normalizers: [`brokerline`](../../chimera/chimera-broker/internal/brokerline/normalize.go), [`vectorstoreline`](../../chimera/chimera-vectorstore/internal/vectorstoreline/normalize.go)
- UI components (parallel): [`embedui-component-system.md`](embedui-component-system.md)
