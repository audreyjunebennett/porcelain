# Plan: Embed UI component system and module split

| Field | Value |
|-------|-------|
| **Doc kind** | `refactor-plan` |
| **Owners / areas** | Gateway embed UI (`adminui/embedui`), operator logs |
| **Status** | `done` |
| **Targets** | Reusable UI primitives, smaller app modules, gallery-driven CSS |
| **Last updated** | 2026-05-18 |
| **Supersedes / superseded by** | Builds on [`logs-ui-maintainability.md`](logs-ui-maintainability.md), [`chimera-gateway-refactor.md`](chimera-gateway-refactor.md) Phase 5; gallery repair [`embedui-component-gallery.md`](embedui-component-gallery.md) Phases 1–4 `done` |

## At a glance

Operators see cards, chips, tables, and forms across `/ui/logs`, setup, metrics, and the legacy panel. Today most of that is built from large HTML string blocks in a few JavaScript files and one ~2.4k-line stylesheet. Formalize a small in-house component layer (pure render functions + shared CSS), split the remaining monoliths, and use the component gallery as the review surface—without adopting a full SPA framework.

| Phase | Outcome | Status |
|-------|---------|--------|
| [Phase 1 — Shared CSS primitives](#phase-1--shared-css-primitives) | `ui.css` + section imports; orphan pages can share buttons, tables, callouts | `done` |
| [Phase 2 — UI component modules](#phase-2--ui-component-modules) | `embedui/ui/components/*` for Badge, Pill, Chip, Card, Button, tables, timeline | `done` |
| [Phase 3 — Card render extraction](#phase-3--card-render-extraction) | `summarizedFeed.js` orchestrates only; card HTML lives under `render/cards/` | `todo` |
| [Phase 4 — Handler and app shell split](#phase-4--handler-and-app-shell-split) | `wireHandlers.js` and `logs_app.js` shrink to mount + transport | `done` |
| [Phase 5 — Unify standalone operator pages](#phase-5--unify-standalone-operator-pages) | `setup.html`, `metrics.html`, `panel.html` use shared tokens + `ui.css` | `done` |
| [Phase 6 — Optional bundle step](#phase-6--optional-bundle-step) | CI/Makefile can emit one embed bundle; dev still loads modules individually | `deferred` |

---

## Background

[`chimera-gateway-refactor.md`](chimera-gateway-refactor.md) split `logs_app.js` and introduced `logs/components/` (`Badge`, `MetricPillsRow`, `KeyValueGrid`). **`summarizedFeed.js` remains ~6k+ lines** with inline `build*CardHtml` functions. **`logs.css`** is flat and mostly logs-specific but also defines primitives used only in gallery demos. Standalone pages (`setup.html`, `metrics.html`, `panel.html`) duplicate button/table/error styles inline.

**Non-goals:** React/Vue/Svelte rewrite; mandatory Node/npm for the default developer loop; changing `/api/ui/*` JSON contracts.

**Framework stance:** Stay vanilla JS with optional **`htm`** tagged templates or a tiny `html`` escape helper. Revisit **Pico CSS / Shoelace** only for form-heavy pages in Phase 5 if shared `ui.css` is insufficient.

**Related docs:** [`embedui-component-gallery.md`](embedui-component-gallery.md), [`unified-logs-operator-shell.md`](unified-logs-operator-shell.md), [`embedui-event-log-panel.md`](embedui-event-log-panel.md), [`logs-ui-maintainability.md`](logs-ui-maintainability.md).

### Gallery change contract

The static gallery under [`docs/component-gallery/`](../component-gallery/) is the visual acceptance gate for embed CSS and shared class names. When you touch production styling or primitives:

- Update the matching gallery section in the same PR, or explain **N/A** in the PR description.
- Add a gallery section before (or with) new production components (gallery-first).
- Run `make component-gallery-check` (also runs under `make chimera-gateway-test`).

Details: [`docs/component-gallery/README.md`](../component-gallery/README.md#change-contract).

---

## Phase 1 — Shared CSS primitives

**Goal.** One place for buttons, inputs, callouts, errors, and base tables—shared by logs and standalone pages.

**Deliverables**

- Introduce `adminui/embedui/ui.css` (or `styles/primitives.css`) for `.btn`, `.callout`, `.err`, form fields, base `table` patterns—not logs-layout-specific rules.
- Split `logs.css` into imported sections (single PR or incremental):
  - `styles/tokens.css` → re-export or `@import` existing `theme-tokens.css`
  - `styles/card.css`, `styles/evlog.css`, `styles/timeline.css`, `styles/admin-forms.css` (`.sg-op-yaml-*`)
- `logs.css` becomes composition `@import` list; gateway still serves one URL for compatibility (`/ui/assets/logs.css`).
- Update [`embedui-component-gallery.md`](embedui-component-gallery.md) gallery `<link>` if split files are loaded directly for debugging.
- Section banners in CSS matching gallery nav: Typography, Chrome, Pills, Status, Progress, Tables, KV, Cards.

**Acceptance**

- `/ui/logs` visually unchanged (smoke: summarized panel, one expanded card, event log table).
- Gallery loads same effective styles after path repair.
- New primitive class added in `ui.css` appears in gallery **and** at least one production surface.

**Status:** `done`

---

## Phase 2 — UI component modules

**Goal.** Production HTML for primitives is built through small tested functions, not copy-pasted strings.

**Deliverables**

- Directory `adminui/embedui/ui/`:
  - `util/escape.js` (or re-export from `logs/util/`)
  - `util/html.js` optional tagged-template helper
  - `components/Badge.js`, `Chip.js`, `Pill.js`, `Button.js`, `Card.js`, `KeyValueGrid.js`, `MetricPillsRow.js`, `DataTable.js`, `TimelineBar.js`, `StatusIndicator.js`, `Callout.js`, `YamlEditorPanel.js`
- Namespace: `globalThis.ChimeraUI` (presentation) vs `globalThis.ChimeraLogs` (logs app orchestration)—migrate existing `logs/components/*` into `ui/components/` with re-exports during transition.
- Extract status pill HTML from `logs/render/sumEvlog.js` into `Pill.js` / `StatusIndicator.js`.
- Goja tests in `logs_components_test.go` (or `adminui/embedui/ui_*_test.go`) per component; pattern from existing `KeyValueGrid` tests.
- Gallery section per new component (gallery-first for new primitives).

**Acceptance**

- No new duplicate string templates for badges/pills outside `ui/components/`.
- `go test` component tests pass for each exported component.
- [`models.js`](../../chimera/chimera-gateway/internal/server/adminui/embedui/logs/models.js) loaded in `logs.html` with JSDoc typedefs for component models (optional but recommended in this phase).

**Status:** `done`

---

## Phase 3 — Card render extraction

**Goal.** `summarizedFeed.js` preserves scroll/focus/defer behavior only; card bodies live in focused modules.

**Deliverables**

- `adminui/embedui/logs/render/cards/`:
  - `gatewayOverview.js`, `gatewayUsage.js`
  - `adminUsers.js`, `adminProvider.js`, `adminRouting.js`, `adminFallback.js`, `adminRouterModels.js`
  - `workspaceDraft.js`
  - (existing service cards stay in derive + thin render wrappers where appropriate)
- Each module: `(model, deps) → htmlString` using `ChimeraUI.*` components.
- Move `build*CardHtml` functions out of `summarizedFeed.js`; target **under ~800 lines** for that file.
- Goja fixtures: one JSON model → expected HTML substring per card type.
- `data-testid` preserved on card roots ([`logs-ui-maintainability.md`](logs-ui-maintainability.md)).

**Acceptance**

- No new logic added only to `summarizedFeed.js` (orchestration + state only).
- Card render tests cover gateway overview, one admin card, one conversation/service card path.
- Manual checklist from [`log-view-refactor.md`](log-view-refactor.md) still passes.

**Status:** `done`

---

## Phase 4 — Handler and app shell split

**Goal.** Event wiring and boot code are navigable by concern.

**Deliverables**

- Split `logs/app/wireHandlers.js`:
  - `logs/handlers/evlog.js` — row selection, copy, filters
  - `logs/handlers/admin.js` — routing save, provider keys, tokens, workspace drafts
  - `logs/handlers/chrome.js` — external links, project path reveal
- `app/wireHandlers.js` — thin `mountWireHandlers(ctx)` delegating to `ChimeraLogs.Handlers.*`
- `logs_app.js` retains: ctx construction, module mount order, view mode, SSE mount (further shell slimming deferred)
- `logs.html` script order documented in [`embedui/logs/README.md`](../../chimera/chimera-gateway/internal/server/adminui/embedui/logs/README.md).

**Acceptance**

- `wireHandlers.js` deleted or reduced to a thin `mountWireHandlers(ctx)` re-export.
- No behavior change in admin form save paths (same API calls).

**Status:** `done`

---

## Phase 5 — Unify standalone operator pages

**Goal.** Setup, metrics, and panel pages share the same visual language as logs.

**Deliverables**

- `setup.html`, `metrics.html`, `panel.html`: load `theme-tokens.css` + `ui.css`; remove duplicated inline `<style>` blocks where replaced.
- Decision doc in PR: **`panel.html` deprecation** vs maintenance—[`unified-logs-operator-shell.md`](unified-logs-operator-shell.md) shipped in-logs admin cards; keep `/ui/panel` as redirect or legacy iframe until parity verified.
- Optional: Shoelace/Pico for form controls if `ui.css` forms are still too bare—document choice in PR.

**Acceptance**

- Setup and metrics pages visually align with logs chrome (buttons, tables, errors).
- If panel is deprecated: link from desktop shell points to `/ui/logs?focus=admin` (or agreed replacement).

**Status:** `done`

**Panel / metrics deprecation (2026-05):** Standalone `panel.html` and `metrics.html` were removed. `/ui/panel` → `/ui/logs?focus=admin`; `/ui/metrics` → `/ui/logs?focus=metrics` (see [`embed/routes.go`](../../chimera/chimera-gateway/internal/server/adminui/embed/routes.go)). Desktop shell settings opens `/ui/logs?embed=1`. No Shoelace/Pico — `ui.css` primitives are sufficient for setup and login.

---

## Phase 6 — Optional bundle step

**Goal.** Production embed can ship one JS file without forcing npm on every developer.

**Deliverables**

- Makefile target `embedui-bundle` (esbuild or simple concat) producing `adminui/embedui/dist/bundle.js` for optional embed.
- CI builds bundle; default dev workflow unchanged (multi-file load).
- Document in [`embedui/logs/README.md`](../../chimera/chimera-gateway/internal/server/adminui/embedui/logs/README.md).

**Acceptance**

- Gateway can serve either module list or bundle behind a build tag or env (implementation choice documented).
- Bundle size and load order equivalent to unbundled behavior.

**Status:** `deferred`

---

## Open questions

1. **Namespace merge:** Collapse `ChimeraUI` into `ChimeraLogs` after migration, or keep separate permanently?
2. **Shadow DOM:** Rule out web components unless Phase 5 form experiment requires them.

---

## References

- Code: [`chimera/chimera-gateway/internal/server/adminui/embedui/`](../../chimera/chimera-gateway/internal/server/adminui/embedui/)
- Gallery: [`docs/component-gallery/`](../../docs/component-gallery/), plan [`embedui-component-gallery.md`](embedui-component-gallery.md)
- Log copy (parallel track): [`operator-message-registry.md`](operator-message-registry.md)
- Go boundaries: [`chimera-gateway-package-boundaries.md`](chimera-gateway-package-boundaries.md)
