# Plan: Log View Refactor (Claudia Gateway `/ui/logs`)

| Field | Value |
|-------|-------|
| **Doc kind** | `refactor-plan` |
| **Owners / areas** | Gateway embed UI, operator logs |
| **Status** | `shipped` |
| **Targets** | `/ui/logs` extraction and testability phases 0-7 |
| **Last updated** | See git history |
| **Supersedes / superseded by** | Followed by [`logs-ui-maintainability.plan.md`](logs-ui-maintainability.plan.md) |

## At a glance

Untangle the operator log page so future changes are safe. Split CSS, HTML, and JS into focused files, separate "what the metrics mean" from "how the page draws them," and ship the work in small, reviewable steps so nothing breaks along the way.

| Phase | Outcome | Status |
|-------|---------|--------|
| [Phase 0 — Safety net & invariants](#phase-0--safety-net--invariants-no-refactor-yet) | Tests and a manual checklist before any code moves | `done` |
| [Phase 1 — Extract CSS](#phase-1--extract-css-from-html) | `logs.css` served as its own asset | `done` |
| [Phase 2 — Extract JS](#phase-2--extract-js-from-html) | `logs.js` served as its own asset | `done` |
| [Phase 3 — Reusable components & models](#phase-3--create-reusable-ui-components--models-no-behavior-changes) | Pure components and JSDoc models, with tests | `done` |
| [Phase 4 — Modularize `logs.js`](#phase-4--modularize-logsjs-into-focused-modules) | Util / parse / filter / transport / render modules | `done` |
| [Phase 5 — Derivation vs rendering](#phase-5--separate-metrics-derivation-from-rendering-enables-correctness-work) | Pure `derive/*` view models with tests | `done` |
| [Phase 6 — Metrics correctness pass](#phase-6--metrics-correctness-pass-scoped-fixes-with-tests) | Conversation, BiFrost, indexer, Qdrant, and gateway cards verified | `done` |
| [Phase 7 — Cleanup & polish](#phase-7--cleanup--polish) | `StatusLine` component, dead code removed, escaping tightened | `done` |

---

**Scope**: `internal/server/embedui/logs.html` (currently monolithic: CSS + HTML + JS + metrics derivation + rendering + transport)

**Primary goals**
- **Maintainability**: split CSS/HTML/JS and modularize JS into focused files.
- **Correctness**: make metrics on the page reliable by separating *derivation* from *rendering*.
- **Consistency**: introduce reusable display components with clear models so formatting and semantics don’t drift.
- **Safety**: do this in small, reviewable steps with regression checks at each step.

**Non-goals (for this refactor)**
- Rewriting the UI in a framework (React/Vue/etc.)
- Introducing a full JS bundler toolchain unless absolutely necessary
- Changing API contracts for `/api/ui/logs`, `/api/ui/logs/stream`, `/api/ui/metrics`, `/api/ui/tokens` (unless explicitly planned)

---

## Current architecture (baseline)

### UI entry points
- **Page**: `/ui/logs` serves embedded HTML `internal/server/embedui/logs.html`
- **Data APIs**
  - `/api/ui/logs` (poll/backfill)
  - `/api/ui/logs/stream` (SSE)
  - `/api/ui/metrics` (gateway metrics snapshot)
  - `/api/ui/tokens` (token labels for tenant ids)

### Log view modes (URL + localStorage)
- View modes: `summarized`, `raw` (StructuredLogs), `raw_logs` (Raw Logs)
- LocalStorage keys:
  - `claudia_logs_view_mode`
  - `claudia_logs_flt_app`
  - `claudia_logs_flt_level`
- URL params (must remain compatible): `view`, `principal`, `conversation|conv`, `seq`, `embed`

### Known issue(s)
- JS looks up `document.getElementById("status")` but `logs.html` does not define an element with `id="status"`.

---

## Work strategy

### Rule: no “big bang” changes
Each phase should be a **standalone, shippable PR** with no behavior changes unless explicitly listed as a change objective.

### Rule: create seams first
Before changing metrics logic, extract the code so it’s **testable and localized**.

### Suggested PR structure
- PR title style: `ui/logs: <short change>`
- Keep diffs small; prefer many PRs over one massive refactor.

---

## Phase checklist (track progress)

### Phase 0 — Safety net + invariants (no refactor yet)
- [x] Write down invariants in this doc and keep updated as we learn more.
- [x] Add/extend Go tests to guard `/ui/logs` shell (presence of key IDs, assets once extracted).
- [x] Confirm existing API tests cover:
  - `/api/ui/logs` behaviors (`internal/server/ui_logs_test.go`)
  - `/api/ui/metrics` behaviors (`internal/server/ui_metrics_test.go`)
- [x] Add a short manual test checklist (below) that each PR must pass.

### Phase 1 — Extract CSS from HTML
- [x] Create `internal/server/embedui/logs.css` containing the existing `<style>` block unchanged.
- [x] Update `logs.html` to `<link rel="stylesheet" ...>` to the extracted CSS.
- [x] Update embed serving to include the CSS asset (see “Asset serving plan”).
- [x] Verify: layout parity in all view modes + embedded mode. (Automated: shell test + full `go test ./...`; manual spot-check recommended.)

### Phase 2 — Extract JS from HTML
- [x] Create `internal/server/embedui/logs.js` containing the existing IIFE unchanged.
- [x] Update `logs.html` to `<script src="..." defer></script>`.
- [x] Update embed serving to include the JS asset (see “Asset serving plan”).
- [x] Verify: SSE→poll fallback, backfill, filters, copy-to-clipboard, summarized feed, gateway metrics polling. (Automated: `/ui/logs` shell test + new `/ui/assets/logs.js` test + full `go test ./...`; manual spot-check recommended.)

### Phase 3 — Create reusable UI components + models (no behavior changes)
- [x] Create `internal/server/embedui/logs/` module folder (see “Target file layout”).
- [x] Add `models.js` with JSDoc typedefs for view models.
- [x] Build **pure** reusable display components (see “Component list”).
- [x] Add component tests (Go tests using `goja` JS VM). (Uses absolute paths derived from test file location; does not depend on working directory.)

### Phase 4 — Modularize `logs.js` into focused modules
- [x] Move utilities into `util/*` (escape, date formatting, hashing).
- [x] Move parsing into `parse/*` (text → `ParsedEntry`).
- [x] Move filters into `filters/*`.
- [x] Move transport/cache into `transport/*` (SSE/poll/backfill/dedupe).
- [x] Move rendering into `render/*`, using reusable components. (Initial extraction: raw textarea renderer + clipboard; summarized feed + structured table still pending follow-up splits.)
- [x] Keep a thin `index.js` bootstrap. (`/ui/assets/logs.js` is now a small bootstrap calling `ClaudiaLogs.Main()` from `/ui/assets/logs/main.js`.)

### Phase 5 — Separate “metrics derivation” from “rendering” (enables correctness work)
- [x] Create `derive/*` pure functions producing stable “view models” for cards/tables. (Started with conversation metrics.)
- [x] Add tests for derivation (fake `ParsedEntry[]` inputs → expected metrics outputs).
- [x] Ensure summarized feed uses derived view models + components, not ad-hoc scraping. (Conversation token/vector derivation now lives in `derive/`.)

### Phase 6 — Metrics correctness pass (scoped fixes, with tests)
- [x] Gateway usage card correctness (`/api/ui/metrics` mapping). (Covered by goja tests; UI delegates to `derive/`.)
- [x] Conversation metrics correctness (tokens, vectors, duration, status). (Conversation token/vector derivation now covered by goja tests; UI delegates to `derive/`.)
- [x] Bifrost card correctness (relay counts, stream vs JSON, HTTP rollups, rate-limit detection). (Covered by goja tests; UI delegates to `derive/`.)
- [x] Indexer run metrics correctness (vectors stored, ok/fail, progress/done heuristics). (Covered by goja tests; UI delegates to `derive/`.)
- [x] Qdrant/RAG rollups correctness (retrieve/search counts + path bucketing). (Covered by goja tests; UI delegates to `derive/`.)

### Phase 7 — Cleanup + polish
- [x] Fix `#status` mismatch by adding a `StatusLine` component + DOM node + tests.
- [x] Reduce HTML-string concatenation where it’s a readability or escaping risk (optional).
- [x] Remove dead legacy summarized code paths if confirmed unused (only after tests).

---

## Asset serving plan (needed for Phases 1–2)

`internal/server/ui_handlers.go` currently embeds HTML files only:

- `//go:embed embedui/login.html ... embedui/logs.html ...`

We will need to extend this to include static assets like:
- `embedui/logs.css`
- `embedui/logs.js`

And expose them under a stable path, e.g.:
- `/ui/assets/logs.css`
- `/ui/assets/logs.js`

**Requirements**
- Correct `Content-Type` headers (`text/css`, `application/javascript`)
- Auth model decision:
  - Prefer: assets require same auth as pages (safe default)
  - Alternative: allow assets unauthenticated if the page is auth-gated (usually OK, but decide explicitly)

---

## Target file layout (end state)

Under `internal/server/embedui/`:
- `logs.html` (thin shell)
- `logs.css` (styles)
- `logs.js` (bootstrap; or points to modular `logs/index.js`)
- `logs/`
  - `index.js` (bootstrap, wiring)
  - `models.js` (JSDoc typedefs)
  - `util/`
    - `escape.js`
    - `date.js`
    - `hash.js`
  - `parse/`
    - `parseLogText.js`
    - `flatten.js`
    - `inferShape.js`
  - `filters/`
    - `filters.js`
    - `prefs.js`
  - `transport/`
    - `sse.js`
    - `poll.js`
    - `backfill.js`
    - `cache.js`
  - `derive/`
    - `conversation.js`
    - `bifrost.js`
    - `indexer.js`
    - `qdrant.js`
    - `gatewayUsage.js`
  - `components/`
    - `Badge.js`
    - `MetricPills.js`
    - `TimelineBar.js`
    - `KeyValueGrid.js`
    - `Card.js`
    - `MetricsTable.js`
    - `StatusLine.js`
  - `render/`
    - `summarizedFeed.js`
    - `structuredTable.js`
    - `rawTextarea.js`

Note: this is a *directional* layout; implement incrementally.

---

## Reusable component list (initial cut)

All components should be **pure** (no network, no timers, no storage, no DOM reads). They should accept models and return:
- either an **HTML string** (fastest path, consistent with current code), or
- a **DOM node** (better safety/maintainability; can be introduced later)

### “Foundational”
- `escapeHtml(text)`: single source of truth for escaping.
- `Badge(model)`: small label with consistent variant → class mapping.
- `MetricPillsRow(models[])`: render pill strip consistently.

### “Layout”
- `Card(cardModel, bodyHtml)`: the `<details class="sum-card">` pattern.
- `TimelineBar(segments)`: consistent segment coloring rules across cards.
- `KeyValueGrid(extras[])`: replacement for the “Details” inner grid/table.

### “Metrics”
- `MetricsTable(tableModel)`: shared renderer for rollup tables + recent event tables.
- `StatusLine(model)`: page-level status (`Live (SSE)`, `SSE reconnecting…`, etc.)

---

## Models (for consistency)

Define JSDoc typedefs (or equivalent documentation) for:
- `ParsedEntry` (parsed log line + normalized `flat` fields)
- `BadgeModel` (text, variant, title)
- `MetricPillModel` (text, title, variant)
- `CardModel` (id, avatar, title/subtitle HTML, metrics array, status variant)
- `TableModel` / `RowModel` / `ColumnModel`
- `SectionModel` (for summarized feed sections)

**Rule**: derived metrics functions should output models; render functions should consume models.

---

## Testing plan

The repo is Go-first and currently has no Node toolchain. Two viable approaches:

### Option A (recommended): Go tests + JS VM
- Use a Go JS VM (e.g. `goja`) to evaluate component modules and call exported functions.
- Assertions focus on:
  - escaping correctness (XSS safety)
  - stable structure/class names
  - numeric formatting consistency
  - variant→class mapping for badges/status/pills

### Option B: Minimal Node test runner (optional)
- Add a small JS test harness and a `make` target.
- Only if CI/dev environment guarantees Node availability.

**Minimum bar**: at least the foundational components + derivation functions have unit tests.

---

## Manual test checklist (run for each PR)
- [ ] `/ui/logs` loads and shows content
- [ ] View switch: Summarized ↔ StructuredLogs ↔ Raw Logs
- [ ] Filter by Application and Level works
- [ ] Scroll up triggers backfill (older logs)
- [ ] Live updates via SSE; fallback to polling if SSE fails
- [ ] “Copy to clipboard” works in Raw Logs mode
- [ ] Summarized feed shows:
  - Gateway usage section (when metrics enabled)
  - Conversation cards (when conversation_id present)
  - Indexer run cards (when index_run_id present)
  - Service cards (bifrost/gateway/indexer/qdrant)

---

## Work distribution (good “agent-sized” chunks)
- **Agent A**: Phase 1 (CSS extraction) + serving assets
- **Agent B**: Phase 2 (JS extraction) + serving assets
- **Agent C**: Component library scaffolding + foundational components + tests
- **Agent D**: Parsing normalization (`ParsedEntry`) + tests
- **Agent E**: Metrics derivation modules + tests (conversation/bifrost/indexer/qdrant)

---

## Notes / references
- API tests: `internal/server/ui_logs_test.go`, `internal/server/ui_metrics_test.go`
- UI embed wiring: `internal/server/ui_handlers.go` (adminEmbedUI `//go:embed` + `serveEmbed`)

