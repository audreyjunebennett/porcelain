# Plan: Chimera gateway package boundaries

| Field | Value |
|-------|-------|
| **Doc kind** | `refactor-plan` |
| **Owners / areas** | `chimera/chimera-gateway`, shared `chimera/internal`, `internal/naming` |
| **Status** | `done` |
| **Targets** | Single-responsibility packages; reusable operator API shapes |
| **Last updated** | 2026-05-18 |
| **Supersedes / superseded by** | Extends Phase 2 of [`chimera-gateway-refactor.md`](chimera-gateway-refactor.md); complements [`embedui-component-system.md`](embedui-component-system.md) and [`operator-message-registry.md`](operator-message-registry.md) |

## At a glance

The gateway refactor cleaned naming and split the logs JavaScript monolith, but Go still mixes HTTP routing, session auth, JSON handlers, and embed asset delivery in one `adminui` package—and operator JSON shapes are implicit in handler files. Split backend boundaries so UI assets, session, API handlers, and shared DTOs are importable and testable on their own, without breaking the single Go module layout.

| Phase | Outcome | Status |
|-------|---------|--------|
| [Phase 1 — Admin UI package split](#phase-1--admin-ui-package-split) | `embed`, `session`, and `api/*` subpackages replace monolithic `adminui` | `done` |
| [Phase 2 — Operator API DTOs](#phase-2--operator-api-dtos) | Shared request/response types for `/api/ui/*` | `done` |
| [Phase 3 — Generated contracts](#phase-3--generated-contracts) | `contracts.js` and slug constants generated from Go naming registry | `done` |
| [Phase 4 — Test colocation](#phase-4--test-colocation) | Logs UI goja tests live beside embed assets | `done` |
| [Phase 5 — Deprecate duplicate operator surfaces](#phase-5--deprecate-duplicate-operator-surfaces) | Single canonical path for admin workflows in `/ui/logs` | `done` |

---

## Background

Today:

- **`adminui/`** — `ui_handlers.go` (embed + mux + login), `ui_*.go` handlers, `uisession.go`, all embed assets under `embedui/`.
- **`adminui/embed/embedui_test/`** — goja tests for JavaScript under `embedui/logs/` and `embedui/ui/` (external package beside embed assets).
- **Shared chimera libs** — [`servicelogs`](../../chimera/internal/servicelogs/), [`brokerclient`](../../chimera/internal/brokerclient/), [`gatewayline`](../../chimera/internal/gatewayline/) already follow reasonable SRP.

[`chimera-gateway-refactor.md`](chimera-gateway-refactor.md) Phase 2 moved `brokeradmin` and slimmed `export.go`. This plan finishes **operator-facing boundaries** on the gateway side.

**Non-goals:** Splitting `github.com/lynn/porcelain` into multiple modules; changing public REST API outside `/api/ui/*` without a dedicated API plan.

**Related docs:** [`configuration.md`](../configuration.md), [`unified-logs-operator-shell.md`](unified-logs-operator-shell.md), [`env-precedence-contract.md`](env-precedence-contract.md) Phase 4 (shared startup components).

---

## Phase 1 — Admin UI package split

**Goal.** Clear import graph: register routes in one place; handlers grouped by domain.

**Deliverables**

- Restructure under `chimera-gateway/internal/server/adminui/`:
  - **`embed/`** — `//go:embed` (assets under `embed/embedui/`), `ReadEmbedFile`, MIME routes, logs module asset handler
  - **`session/`** — cookies, `UIOptions`, session store (`uisession.go`)
  - **`api/logs/`** — `ui_logs.go`
  - **`api/tokens/`** — `ui_tokens.go`
  - **`api/metrics/`** — `ui_metrics.go`
  - **`api/routing/`** — `ui_routing_generate.go`
  - **`api/indexer/`** — `ui_indexer.go`
  - **`api/providers/`** — `ui_broker_providers.go`
  - **`api/save/`** — provider key save, logout (`ui_save.go`)
  - **`register.go`** — `Register(mux, runtime, …)` wiring only
- Shared JSON helpers (`writeUIJSONError`, …) in `api/internal` or `adminui/apijson`.
- **`server.go`** continues to own non-UI gateway routes; calls `adminui.Register`.

**Acceptance**

- `go test ./chimera/chimera-gateway/...` passes.
- No new import cycles; `adminui` consumers import `adminui/api/...` or `adminui/register` only.

**Status:** `done`

---

## Phase 2 — Operator API DTOs

**Goal.** One place for JSON the logs UI and future clients consume.

**Deliverables**

- New package **`internal/operatorapi/`** (repo root):
  - `state.go`, `tokens.go`, `routing.go`, `metrics.go`, `indexer.go`, `providers.go`
  - Struct tags and field names match current `/api/ui/*` responses (no breaking rename in this phase).
- Handlers map domain types → DTOs; document breaking-change policy in package comment.
- Optional: OpenAPI fragment or JSON Schema for `/api/ui/state` (defer if heavy).

**Acceptance**

- `handleState` and routing handlers construct `operatorapi.*` types before `json.Encode`.
- UI contract documented in [`embedui/logs/README.md`](../../chimera/chimera-gateway/internal/server/adminui/embedui/logs/README.md) with link to `operatorapi`.

**Status:** `done`

---

## Phase 3 — Generated contracts

**Goal.** Go naming registry is authoritative for JS constants and log slugs.

**Deliverables**

- **`go generate`** pipeline (single entry `//go:generate` in `internal/naming/` or `operatorcopy/`):
  - `adminui/embedui/logs/contracts.js` from [`gateway_logs.go`](../../internal/naming/gateway_logs.go) + timeline/service constants
  - `internal/naming/log_messages.go` from [`operator-message-registry.md`](operator-message-registry.md) Phase 5 (when YAML exists)
- Replace hand-maintained duplicates in `contracts.js` with generated output; keep thin re-export if needed.
- Makefile target `operator-contracts-generate` invoked in CI or pre-test.

**Acceptance**

- Editing `gateway_logs.go` and running generate updates `contracts.js`; CI fails if stale.
- Resolves open Q2 from [`chimera-gateway-refactor.md`](chimera-gateway-refactor.md) for constants (copy remains YAML in operator-message plan).

**Status:** `done`

---

## Phase 4 — Test colocation

**Goal.** Frontend tests sit next to the assets they exercise.

**Deliverables**

- Move `logs_components_test.go`, `logs_cards_test.go`, and `ui_components_test.go` into `adminui/embed/embedui_test/` as external package `embedui_test`.
- Keep shared goja helpers (`evalJS`, `logsUIPath`, `uiEmbedPath`, `loadCardTestCtx`) in `goja_test.go`.
- Fixture paths resolve via `serverTestdataPath` to `internal/server/testdata/`.

**Acceptance**

- `go test ./chimera/chimera-gateway/internal/server/adminui/embed/embedui_test/...` runs all JS component/derive tests.
- No regression in test count vs prior `logs_components_test.go` + cards + UI tests (47 tests).

**Status:** `done`

---

## Phase 5 — Deprecate duplicate operator surfaces

**Goal.** One operator story: logs shell + cards, not parallel panel implementations.

**Decision (2026-05-18):** **Redirect** — retire embedded `panel.html` and `metrics.html`; HTTP routes redirect into `/ui/logs` with `focus` query params. Desktop [`shell.html`](../../chimera/chimera-gateway/internal/server/adminui/embed/embedui/shell.html) uses a single iframe; the settings control toggles `/ui/logs?embed=1` (no separate Admin tab).

**Primary operator URLs**

| Surface | URL |
|---------|-----|
| Browser (full page) | `/ui/logs` |
| Desktop shell | `/ui/desktop` → settings opens `/ui/logs?embed=1` |
| Bootstrap (first token) | `/ui/setup` |
| Deep links (legacy) | `/ui/panel` → `/ui/logs?focus=admin`; `/ui/metrics` → `/ui/logs?focus=metrics` |

**Parity checklist** (`/ui/panel` → logs admin cards)

| Former panel feature | Logs location |
|----------------------|---------------|
| Gateway tokens (create/list/delete) | `admin-users` card |
| Groq / Gemini keys | `admin-provider-*` cards |
| Ollama base URL | `admin-provider-ollama` card |
| Routing policy YAML + save | `admin-routing-rules` card |
| Fallback chain + generate | `admin-fallback-chain` card |
| Router models + threshold + enable | `admin-router-model` card |
| Free-tier catalog filter toggle | Admin routing/fallback/router cards |
| Preview / generate from catalog | `routing-generate` → `/api/ui/routing/preview` |
| Dry-run evaluate + optional smoke | `routing-evaluate` on routing rules card |
| Gateway usage rollups | `gw-usage-metrics` card (was standalone metrics page) |

**Deliverables**

- Redirects in [`embed/routes.go`](../../chimera/chimera-gateway/internal/server/adminui/embed/routes.go): `/ui/panel`, `/ui/metrics`.
- Removed `embedui/panel.html`, `embedui/metrics.html` from `//go:embed`.
- Logs chrome nav: Shell, Metrics (`?focus=metrics`), Admin (`?focus=admin`).
- Routing dry-run wired in `wireHandlers.js` (no panel-only save scripts remain).

**Acceptance**

- Operator setup flow uses `/ui/setup` then `/ui/logs` after restart; desktop uses `/ui/desktop`.
- No duplicate routing-save logic in retired panel scripts.

**Status:** `done`

---

## Decisions (resolved)

1. **`operatorapi` location:** repo-root [`internal/operatorapi/`](../../internal/operatorapi/) (shared with future Locus CLI). Phase 2 deliverable.
2. **Module split:** stay monorepo single module (`github.com/lynn/porcelain`) until later version.

---

## References

- Gateway refactor: [`chimera-gateway-refactor.md`](chimera-gateway-refactor.md)
- UI refactor: [`embedui-component-system.md`](embedui-component-system.md)
- Log copy: [`operator-message-registry.md`](operator-message-registry.md)
- Code: [`chimera/chimera-gateway/internal/server/adminui/`](../../chimera/chimera-gateway/internal/server/adminui/)
