# Plan: Chimera gateway clarity and naming refactor

| Field | Value |
|-------|-------|
| **Doc kind** | `refactor-plan` |
| **Owners / areas** | `chimera/chimera-gateway` (Go runtime, operator embed UI) |
| **Status** | `done` |
| **Targets** | Gateway maintainability train (post v0.3 naming, post top-level layout cleanup) |
| **Last updated** | 2026-05-18 |
| **Supersedes / superseded by** | Extends [`logs-ui-maintainability.md`](logs-ui-maintainability.md); aligns with [`rename-vectorstore-broker-questions-answered.md`](rename-vectorstore-broker-questions-answered.md) |

## At a glance

Make **chimera-gateway** easier to change by using one vocabulary everywhere operators see it (**chimera-broker**, **chimera-vectorstore**, Chimera headers/env from [`internal/naming/contracts.go`](../../internal/naming/contracts.go)), and by finishing the logs UI split so a ~9k-line script is no longer the default edit surface. There is no legacy user base—rename hard, delete dead trees, and drop BiFrost/Qdrant/upstream names except in debug or upstream-implementation packages.

| Phase | Outcome | Status |
|-------|---------|--------|
| [Phase 1 — Inventory and naming contract](#phase-1--inventory-and-naming-contract) | Written rename matrix; shared constants for Go + UI | `done` |
| [Phase 2 — Go package and facade cleanup](#phase-2--go-package-and-facade-cleanup) | `brokeradmin`, slimmer `server` package, no duplicate UI paths | `done` |
| [Phase 3 — Broker vocabulary (Go)](#phase-3--broker-vocabulary-go) | Operator logs, APIs, and config language say **broker**, not bifrost/upstream | `done` |
| [Phase 4 — Vectorstore vocabulary (Go)](#phase-4--vectorstore-vocabulary-go) | RAG and timeline use **vectorstore**; Qdrant is an implementation detail | `done` |
| [Phase 5 — Logs UI structure and naming](#phase-5--logs-ui-structure-and-naming) | Modular logs app, honest asset names, broker/vectorstore in JS | `done` |
| [Phase 6 — Validation and doc sync](#phase-6--validation-and-doc-sync) | Tests green; operator docs match code | `done` |

---

## Background

**chimera-gateway** is the Porcelain API surface (chat, ingest, RAG, operator UI). v0.3 naming ([`v0-3-naming-migration.md`](v0-3-naming-migration.md)) shipped product/binary contracts in [`internal/naming/contracts.go`](../../internal/naming/contracts.go), but the gateway tree still carries pre-cutover vocabulary and layout debt:

| Area | Current pain | Direction |
|------|----------------|-----------|
| **Packages** | ~~`internal/bifrostadmin`~~ → [`internal/brokeradmin`](../../chimera/chimera-gateway/internal/brokeradmin); `server/export.go` slimmed (no provider-health re-exports) | `brokeradmin`; callers import `adminui` or `brokeradmin` directly |
| **HTTP / logs** | `timeline_kind` values `upstream`, `qdrant`; paths like `/v1/qdrant` in comments | `broker`, `vectorstore`; align with [`servicelogs` sources](../../chimera/internal/servicelogs/sources.go) |
| **Constants** | Headers use `naming` in `scope/`; most UI + slog slugs are string literals | Extend `contracts.go` (or thin `naming/ui.go`) + use in Go; mirror in JS with tests |
| **Operator UI** | Canonical embed tree: `internal/server/adminui/embedui/`; **`logs.js` ~9.1k lines**; confusing URLs (`logs_bootstrap.js` → `/ui/assets/logs.js`, `logs.js` → `main.js`) | Continue [`logs-ui-maintainability.md`](logs-ui-maintainability.md); rename derive files (`conversationBifrost.js`, `qdrantCollection.js`, …) |
| **Wrapper `main.go`** | Flags/env still say `upstream-override`, `debug-enable-upstream-logs` | Map to broker/backend vocabulary where operator-facing; keep `GATEWAY__*` env keys until a dedicated env plan |

Operator vocabulary is fixed in [`rename-vectorstore-broker-questions-answered.md`](rename-vectorstore-broker-questions-answered.md): UI, docs, logs, supervisor → **chimera-broker** / **chimera-vectorstore**; upstream product names only in architecture/debug.

**Related docs:** [`log-view-refactor.md`](log-view-refactor.md) (shipped extraction), [`logs-ui-maintainability.md`](logs-ui-maintainability.md) (active JS/CSS work), [`vectorstore-broker-wrapper-hard-cut.md`](vectorstore-broker-wrapper-hard-cut.md), [`configuration.md`](../configuration.md).

---

## Phase 1 — Inventory and naming contract

**Goal.** One authoritative map of symbols to rename and a small set of shared constants so later PRs do not debate strings ad hoc.

**Deliverables**

- **Rename matrix** — [below](#phase-1-rename-matrix).
- **[`internal/naming/gateway_logs.go`](../../internal/naming/gateway_logs.go)** — log sources, `timeline_kind` slugs, msg prefixes (product/env/headers remain in [`contracts.go`](../../internal/naming/contracts.go)).
- **Grep inventory** — [`scripts/chimera-gateway-legacy-vocab-inventory.sh`](../../scripts/chimera-gateway-legacy-vocab-inventory.sh); snapshot [below](#phase-1-legacy-vocabulary-inventory-2026-05-18).
- **Pattern consumer** — [`timeline_kind.go`](../../chimera/chimera-gateway/internal/server/timeline_kind.go) returns `naming.TimelineKind*` for HTTP path classification.

**Acceptance**

- Matrix reviewed; every Phase 3–5 bullet ties to a matrix row.
- New constants compile; `timeline_kind.go` uses `naming.TimelineKindBroker` (and siblings); `servicelogs` ↔ `naming` source strings tested in [`sources_naming_test.go`](../../chimera/internal/servicelogs/sources_naming_test.go).

**Status:** `done`

### Phase 1 — rename matrix

| Surface | Current | Target | Owner | PR batch |
|---------|---------|--------|-------|----------|
| Go package dir | `internal/bifrostadmin` | `internal/brokeradmin` | gateway | Phase 2 |
| Go type/func | `ClassifyBifrostProviderResult`, `FetchBifrostProviderHealth` | `ClassifyBrokerProviderResult`, `FetchBrokerProviderHealth` | `server` / `adminui` | Phase 2 |
| Go file | `ui_bifrost_providers.go` | `ui_broker_providers.go` | `adminui` | Phase 2 |
| Go embed derive | `conversationBifrost.js` | `conversationBroker.js` | `adminui/embedui` | Phase 5 |
| Go embed derive | `qdrantCollection.js`, `qdrantRagMetrics.js` | `vectorstoreCollection.js`, `vectorstoreRagMetrics.js` | `adminui/embedui` | Phase 5 |
| JSON `timeline_kind` | `upstream` | `naming.TimelineKindBroker` (`broker`) | `server`, `conversationwitness`, `conversation_tool_log` | Phase 3 |
| JSON `timeline_kind` | `qdrant` | `naming.TimelineKindVectorstore` (`vectorstore`) | `server` (RAG paths) | Phase 4 |
| JSON `timeline_kind` | `chimera-vectorstore` (RAG only) | `vectorstore` (unify with slug) | `internal/rag` | Phase 4 |
| HTTP path check | `/v1/qdrant` prefix | vectorstore routes only; drop Qdrant-shaped operator URLs | `timeline_kind.go` | Phase 4 |
| Go field/comment | `upstream` (broker base URL) | `broker` | `internal/chat`, `runtime` | Phase 3 |
| Go func | `NewRuntimeWithUpstreamOverride` | `NewRuntimeWithBrokerOverride` | `server/runtime` | Phase 3 |
| Wrapper flag/env | `upstream-override`, `GATEWAY__BROKER_OVERRIDE` | broker vocabulary (wire TBD — [open Q1](#open-questions)) | `main.go`, supervisor | Phase 3 / env plan |
| Env name | `CHIMERA_BROKER_API_KEY` | `CHIMERA_BROKER_API_KEY` (optional hard-cut) | `naming`, docs | Open Q1 |
| Status JSON | upstream component block | `broker`; nested `broker.upstream` for debug | `status.go` | Phase 3 |
| Admin API JSON | `bifrost` fields | `broker` | `adminui/ui_routing_generate.go` | Phase 3 |
| slog / catalog | `chimera-broker.*` + legacy upstream slugs | `chimera-broker.*` only | `catalog`, `chat` | Phase 3 |
| Log msg families | `bifrost.*`, mixed upstream chat slugs | `broker.*`, `chat.chimera-broker.*` | gateway, broker wrapper | Phase 3 |
| Log msg families | `qdrant.*` | `vectorstore.*` | gateway, vectorstore driver | Phase 4 |
| CSS class | `sum-svc-*-bifrost`, `*-qdrant` | `*-broker`, `*-vectorstore` | `logs.css`, `logs.js` | Phase 5 |
| JS `TIMELINE_BAR_KINDS` | keys `chimera-broker`, …; accepts legacy `upstream`/`qdrant` via fallback | import `contracts.js`; map `timeline_kind` slugs → bar keys | `embedui/logs.js` | Phase 5 |
| JS lifecycle step | `upstream` step key | `broker` | `logs.js`, `conversationCardModel.js` | Phase 5 |
| JS constants | string literals for service/timeline | `logs/contracts.js` from `naming` (hand or `go generate` — [open Q2](#open-questions)) | `embedui/logs` | Phase 5 |
| Embed asset names | `logs_bootstrap.js` → served as `logs.js` | `logs_entry.js`, `logs_app.js` | `adminui`, `ui_handlers.go` | Phase 5 |
| `server/export.go` | re-exports bifrost/admin symbols | direct imports; shrink facade | `server` | Phase 2 |
| Debug path | `/debug/upstream/logs` | `/debug/broker/logs` (lockstep with broker wrapper) | wrappers | Open Q3 |
| Metrics JSON | `upstream_*`, `qdrant_*` | `broker_*`, `vectorstore_*` | `gatewaymetrics`, migrations | Open Q4 |
| Repo package | `chimera/internal/upstream` | `brokerclient` (repo-wide) | chimera | Open Q5 |
| Product constant | `ProductBifrostHTTPBinName` | keep for install artifact basename only | `naming` | — (allowed) |
| Driver package | `vectorstore/qdrant` | stay; package comment = implementation detail | `vectorstore` | Phase 4 |

### Phase 1 — legacy vocabulary inventory (2026-05-18)

Run: `scripts/chimera-gateway-legacy-vocab-inventory.sh` (requires `rg` on PATH).

| Term | ~Matches | ~Files | Notes |
|------|----------|--------|-------|
| `bifrost` | 45 | 21 | Mostly `bifrostadmin`, UI providers, comments |
| `upstream` | 280 | 38 | Heavy in `server.go`, `chat.go`, `logs.js` |
| `qdrant` | 137 | 17 | `qdrantCollection.js` (70), `vectorstore/qdrant` driver |
| `BiFrost` | 38 | 19 | Comments and operator-facing copy |

Allowed after later phases: `ProductBifrostHTTPBinName`, `vectorstore/qdrant` driver, `upstream.name` in debug status ([operator Q&A](rename-vectorstore-broker-questions-answered.md)).

### Phase 1 — naming contract map

| Constant family | Go location | Consumer guidance |
|-----------------|-------------|-------------------|
| Product / env / headers | [`contracts.go`](../../internal/naming/contracts.go) | Binaries, supervisor, config paths |
| Log `service`, `timeline_kind`, msg prefixes, UI prefs | [`gateway_logs.go`](../../internal/naming/gateway_logs.go) | Gateway logs, UI (mirror in JS Phase 5) |
| Ring-buffer sources (canonical) | [`servicelogs/sources.go`](../../chimera/internal/servicelogs/sources.go) | Wrappers, log tail; must match `naming.LogSource*` |

---

## Phase 2 — Go package and facade cleanup

**Goal.** The `internal/server` tree reflects real boundaries: HTTP API, admin UI, runtime state—without legacy facades or misleading package names.

**Deliverables**

- **Rename `internal/bifrostadmin` → `internal/brokeradmin`** (single PR: directory `git mv`, package clause, imports). Update comments to “Chimera Broker management HTTP client”; reserve “BiFrost” for comments pointing at the wrapped upstream binary ([`ProductBifrostHTTPBinName`](../../internal/naming/contracts.go)).
- **Collapse `server/export.go` re-exports**: migrate tests and `cmd`/e2e imports to `adminui`, `catalog`, `ingest`, `scope`, `runtime` directly; delete or shrink `export.go` to only symbols that must stay on `package server` for external modules.
- **Consolidate admin UI Go files** under `internal/server/adminui/` only:
  - Move remaining `internal/server/ui_*.go` test helpers next to `adminui` (e.g. `ui_bifrost_providers_test.go` → `adminui/…_test.go`).
  - Rename `ui_bifrost_providers.go` → `ui_broker_providers.go`; `ClassifyBifrostProviderResult` → `ClassifyBrokerProviderResult`, etc.
- **Wrapper `main.go`**: keep `package main` thin; consider `internal/gatewaywrapper` for `gatewayAdapter`, config parse, and backend spawn (optional if it stays small after naming pass).
- **Confirm no second embed tree**: delete any leftover `internal/server/embedui/` paths on disk; fix comments still pointing at `internal/server/embedui/`.

**Acceptance**

- `go test ./chimera/chimera-gateway/...` passes.
- `rg bifrostadmin` and `rg ClassifyBifrost` under gateway return zero (except changelog/docs).
- `server.go` remains the HTTP router; admin registration is clearly `adminui.Register`.

**Status:** `done`

**Shipped (2026-05-18)**

- `git mv` `internal/bifrostadmin` → [`internal/brokeradmin`](../../chimera/chimera-gateway/internal/brokeradmin); package comment = Chimera Broker management HTTP client.
- [`ui_bifrost_providers.go`](../../chimera/chimera-gateway/internal/server/adminui/ui_broker_providers.go) → `ui_broker_providers.go`; `ClassifyBrokerProviderResult`, `FetchBrokerProviderHealth`.
- Unit tests in [`adminui/ui_broker_providers_test.go`](../../chimera/chimera-gateway/internal/server/adminui/ui_broker_providers_test.go); HTTP tests in [`server/ui_broker_providers_http_test.go`](../../chimera/chimera-gateway/internal/server/ui_broker_providers_http_test.go).
- [`export.go`](../../chimera/chimera-gateway/internal/server/export.go): removed provider-health re-exports; kept `Runtime`, catalog poller, ingest headers, `UIOptions`.
- No `internal/server/embedui/` tree on disk (canonical embed: `adminui/embedui/`).

---

## Phase 3 — Broker vocabulary (Go)

**Goal.** Runtime code and structured logs describe LLM routing through **chimera-broker**, not “upstream” or “BiFrost”, except when reporting wrapped upstream debug metadata.

**Deliverables**

- **`internal/chat`**: rename fields/comments/functions that mean “broker base URL” (today often `upstream`); config resolution uses broker endpoint from gateway YAML / env ([`EnvUpstreamAPIKeyTarget`](../../internal/naming/contracts.go) may stay as the env *name* until a separate env hard-cut—document in matrix).
- **`internal/server/catalog`**: “available models” snapshot is a **broker catalog**; slog lines like `chimera-broker.available_models` (already partially migrated) become the only slug family.
- **`conversationwitness`, `conversation_tool_log`**: `timeline_kind=upstream` → `timeline_kind=broker` (constant from Phase 1); update correlation tests and fixtures.
- **`timeline_kind.go`**: `/v1/chat/completions`, `/v1/models` → `broker`; update `TIMELINE_BAR_KINDS` contract in JS in Phase 5.
- **`internal/server/runtime`**: `NewRuntimeWithUpstreamOverride` → `NewRuntimeWithBrokerOverride` (or `WithBrokerBaseURL`); same behavior, new names.
- **Status / health payloads** (`status.go`, `/status` JSON): expose `broker` component block; nested upstream only under `broker.upstream` for debug (per operator Q&A).
- **Admin API handlers** (`adminui/ui_routing_generate.go`, save handlers): request/response field names visible to the UI use `broker`, not `bifrost`.

**Acceptance**

- `rg -i '\bbifrost\b' chimera/chimera-gateway --glob '*.go'` → only `brokeradmin` comments referencing the wrapped binary basename, or zero.
- `rg 'timeline_kind.*upstream' chimera/chimera-gateway` → zero.
- E2E and `logs_components_test` fixtures updated; no dual-read aliases.

**Status:** `done`

**Shipped (2026-05-18)**

- `timeline_kind=broker` on chat, witness, and tool-relay logs (`naming.TimelineKindBroker`).
- Lifecycle slugs `conversation.broker.{started,completed,failed}`; catalog/status/admin JSON use broker vocabulary.
- `NewRuntimeWithBrokerOverride`, `LogBrokerAvailableModelsForLogsUI`; `/status` exposes `broker` block with `broker.upstream.implementation` debug field.
- Minimal logs UI sync: lifecycle step `broker`, `timeline_kind` slug mapping, `gateway.startup.listening` `broker` KV.

---

## Phase 4 — Vectorstore vocabulary (Go)

**Goal.** RAG and ingest speak **vectorstore**; Qdrant appears only inside the storage driver and debug fields.

**Deliverables**

- **`internal/vectorstore`**: comments and types drop “Qdrant collection” as the primary metaphor—use “collection” / “coords” in the interface; move [`vectorstore/qdrant`](../../chimera/chimera-gateway/internal/vectorstore/qdrant/) to `internal/vectorstore/driver/qdrant` or `internal/vectorstore/qdrant` with package comment “Qdrant driver; not operator vocabulary”.
- **`timeline_kind.go`**: replace `qdrant` kind with `vectorstore`; remove or repurpose `/v1/qdrant` path prefix checks—gateway should not advertise Qdrant-shaped URLs to operators.
- **RAG service / ingest logs**: slugs and `timeline_kind` for scroll/upsert/search → `vectorstore`.
- **Indexer API** (`indexerapi`, ingest): health copy and error messages say chimera-vectorstore supervisor target, not Qdrant.
- **Metrics / gatewaymetrics** (if any `qdrant_*` fields): rename to `vectorstore_*` in stored JSON only if no external consumers—otherwise one hard-cut PR with test fixture updates.

**Acceptance**

- `rg -i '\bqdrant\b' chimera/chimera-gateway --glob '*.go'` limited to `vectorstore/qdrant` driver and explicit `upstream.name: qdrant` debug structs.
- Vectorstore interface tests pass without importing qdrant from `server` or `chat`.

**Status:** `done`

**Shipped (2026-05-18)**

- `timeline_kind=vectorstore` on RAG lifecycle and chat RAG paths (`naming.TimelineKindVectorstore`); unified RAG service logs (was `chimera-vectorstore`).
- `/health` check key `vectorstore`; `/status` supervisor fields `vectorstore_supervised` / `vectorstore_http`.
- Indexer storage health `backend: chimera-vectorstore`; removed `/v1/qdrant` from `timeline_kind` path classification.
- `vectorstore` package comments and Qdrant driver package doc; minimal logs UI sync for `vectorstore_supervised`.

---

## Phase 5 — Logs UI structure and naming

**Goal.** Operators see chimera service names in the log shell; developers edit small modules with tests instead of a single 9k-line file.

**Baseline (current)**

| Asset | Disk | Served URL |
|-------|------|------------|
| Bootstrap | `adminui/embedui/logs_bootstrap.js` | `/ui/assets/logs.js` |
| App shell | `adminui/embedui/logs.js` | `/ui/assets/logs/main.js` |
| Modules | `adminui/embedui/logs/**` | `/ui/assets/logs/...` |

Pure derive modules and goja tests exist ([`logs_components_test.go`](../../chimera/chimera-gateway/internal/server/logs_components_test.go)); **`logs.js` still owns** view modes, summarized panel rebuild, SSE wiring, filters, and most CSS class names.

**Deliverables**

- **Workstream A — Naming (from [`logs-ui-maintainability.md`](logs-ui-maintainability.md))**
  - Add `adminui/embedui/logs/README.md` (URL map, APIs, view modes).
  - Rename files for honesty: `logs.js` → `logs_app.js` (or `app/main.js`); `logs_bootstrap.js` → `logs_entry.js`; update `//go:embed` and routes in `ui_handlers.go`.
  - Rename derive modules: `conversationBifrost.js` → `conversationBroker.js`, `qdrantCollection.js` → `vectorstoreCollection.js`, `qdrantRagMetrics.js` → `vectorstoreRagMetrics.js`; delete duplicate `bifrostMetrics.js` if superseded by `chimeraBrokerMetrics.js`.
  - Introduce `adminui/embedui/logs/contracts.js` (or generate from Go) mirroring Phase 1 constants: `SOURCE_CHIMERA_BROKER`, `TIMELINE_KIND_BROKER`, service badge labels.
  - Replace string compares in `logs.js` / derive (`"chimera-gateway"`, `"upstream"`, `"qdrant"`) with `contracts.js` imports; load `contracts.js` before derive in `logs.html`.
- **Workstream B — Split the monolith (phased)**
  - Extract folders under `logs/`: `app/viewMode.js`, `app/summarizedPanel.js`, `app/structuredTable.js`, `app/rawLogs.js`, `app/metricsStrip.js`—each with injectable `deps` (`fetch`, `EventSource`, `document`, `storage`).
  - Shrink `logs_app.js` to boot + dependency injection + route between view modes.
  - Extend goja tests per extracted module (pattern from existing derive tests).
- **Workstream C — CSS / HTML**
  - Section `logs.css` with banners matching DOM regions; rename classes `sum-svc-*-bifrost` → `sum-svc-*-broker`, `*-qdrant` → `*-vectorstore` (single PR with JS class string updates).
  - Stable `data-testid` on chrome regions ([`logs-ui-maintainability.md`](logs-ui-maintainability.md) Workstream B).

**Acceptance**

- `logs_app.js` (or successor) under ~2k lines; no new logic added only to the monolith.
- `go test` logs component tests pass; manual checklist from log-view plan still passes on `/ui/logs`.
- Browser-visible service strip uses **chimera-broker** / **chimera-vectorstore** labels; timeline bar kinds match Go `timeline_kind` constants.

**Status:** `done`

**Shipped (2026-05-18)**

- Honest embed names: `logs_app.js` (main), `logs_entry.js` (bootstrap); derive renames `conversationBroker.js`, `vectorstoreCollection.js`, `vectorstoreRagMetrics.js`.
- `logs/contracts.js` mirrors `internal/naming/gateway_logs.go`; CSS classes `sum-svc-broker` / `sum-svc-vectorstore`.
- Split monolith: `app/summarizedFeed.js`, `app/wireHandlers.js`, `render/sumEvlog.js`; `logs_app.js` ~1.3k lines (boot + transport).
- `embedui/logs/README.md` (URL map, APIs, module owners); `data-testid` on logs chrome regions.

---

## Phase 6 — Validation and doc sync

**Goal.** Proof that the refactor is complete and operators are not surprised by renamed JSON fields or log lines.

**Deliverables**

- **CI**: `go test ./chimera/chimera-gateway/...` and supervisor tests that import gateway packages.
- **Audit scripts** (Makefile or `debug/host` one-liner): fail if forbidden tokens reappear in `chimera-gateway` (e.g. `bifrostadmin`, `timeline_kind=upstream`, `derive/qdrant` paths).
- **Doc pass**: [`configuration.md`](../configuration.md), [`network.md`](../network.md), gateway sections of README—broker/vectorstore vocabulary; link this plan.
- **Mark [`logs-ui-maintainability.md`](logs-ui-maintainability.md)** phases done or superseded by this plan where overlapping.

**Acceptance**

- Audit clean on `main`.
- Operator setup flow (`/ui/setup`, `/ui/logs`) smoke-tested locally.
- No compatibility shims for old JSON field names unless explicitly listed in Open questions.

**Status:** `done`

**Shipped (2026-05-18)**

- `chimera-gateway-test` runs audit before unit + e2e tests; `chimera-test` includes gateway + supervisor suites.
- Operator docs: [configuration.md](../configuration.md), [network.md](../network.md) use chimera-broker / chimera-vectorstore vocabulary; [logs-ui-maintainability.md](logs-ui-maintainability.md) marked superseded.

---

## Open questions (resolved 2026-05-18)

| # | Decision |
|---|----------|
| 1 | **Hard-cut env:** `CHIMERA_BROKER_API_KEY`, `GATEWAY__BROKER_OVERRIDE`; wrapper flags `-broker-override`, `-debug-enable-broker-logs` / `-debug-enable-vectorstore-logs`. |
| 2 | **`contracts.js`:** hand-maintained for now. |
| 3 | **Debug paths:** `/debug/broker/logs` (gateway, broker, indexer wrappers); `/debug/vectorstore/logs` (vectorstore wrapper). |
| 4 | **Metrics SQLite:** `000002_broker_metrics_rename.sql` renames `upstream_*` tables/index to `broker_*`; Go uses `RecordBrokerResponse` + `broker_*` tables. |
| 5 | **Package:** `chimera/internal/brokerclient` (was `upstream`). |

---

## References

- Code: [`chimera/chimera-gateway/`](../../chimera/chimera-gateway/), [`internal/naming/contracts.go`](../../internal/naming/contracts.go), [`chimera/internal/servicelogs/sources.go`](../../chimera/internal/servicelogs/sources.go), [`chimera/internal/gatewayline/`](../../chimera/internal/gatewayline/)
- UI: [`chimera/chimera-gateway/internal/server/adminui/embedui/`](../../chimera/chimera-gateway/internal/server/adminui/embedui/)
- Prior plans: [`logs-ui-maintainability.md`](logs-ui-maintainability.md), [`log-view-refactor.md`](log-view-refactor.md), [`v0-3-naming-migration.md`](v0-3-naming-migration.md)
