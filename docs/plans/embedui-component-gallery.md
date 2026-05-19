# Plan: Embed UI component gallery repair and upkeep

| Field | Value |
|-------|-------|
| **Doc kind** | `feature-plan` |
| **Owners / areas** | Gateway embed UI (`chimera-gateway/internal/server/adminui/embedui`), `docs/component-gallery/` |
| **Status** | `done` |
| **Targets** | Component gallery usable again; stable contract for CSS and layout review without a Go build |
| **Last updated** | 2026-05-18 |
| **Supersedes / superseded by** | Extends shipped work in [`embedui-theme-styleguide.md`](embedui-theme-styleguide.md); unblocks [`embedui-component-system.md`](embedui-component-system.md) |

## At a glance

The static component gallery under `docs/component-gallery/` lets you tune operator UI look-and-feel in a browser without rebuilding the gateway. After the gateway embed tree moved to `chimera/chimera-gateway/internal/server/adminui/embedui/`, gallery HTML pointed at obsolete `../internal/server/embedui/` paths so stylesheets failed to load. Phases 1–4 repair paths, document usage, sync vocabulary with post-refactor embed UI, and add a CI path check plus change contract.

| Phase | Outcome | Status |
|-------|---------|--------|
| [Phase 1 — Fix production CSS paths](#phase-1--fix-production-css-paths) | Gallery pages load `theme-tokens.css` and `logs.css` from the canonical embed tree | `done` |
| [Phase 2 — Gallery README and open instructions](#phase-2--gallery-readme-and-open-instructions) | Contributors know how to open, reload, and what each HTML file is for | `done` |
| [Phase 3 — Vocabulary and class-name sync](#phase-3--vocabulary-and-class-name-sync) | Gallery demos match post-refactor broker/vectorstore class names and badges | `done` |
| [Phase 4 — Change contract and optional CI guard](#phase-4--change-contract-and-optional-ci-guard) | Edits to embed CSS or shared primitives require a gallery check; script fails on broken paths | `done` |

---

## Background

Phase 2 of [`embedui-theme-styleguide.md`](embedui-theme-styleguide.md) shipped the gallery as `temp/theme/gallery.html` loading `internal/server/embedui/logs.css`. The gallery now lives at **`docs/component-gallery/`** (side project for visual iteration). [`chimera-gateway-refactor.md`](chimera-gateway-refactor.md) moved the canonical embed tree to **`chimera/chimera-gateway/internal/server/adminui/embedui/`**.

**Production CSS paths** (from any file in `docs/component-gallery/`):

| Asset | Relative href |
|-------|----------------|
| Design tokens | `../../chimera/chimera-gateway/internal/server/adminui/embedui/theme-tokens.css` |
| Production UI sheet | `../../chimera/chimera-gateway/internal/server/adminui/embedui/logs.css` |
| Gallery-only layout | `gallery-shell.css` (same directory) |

**CI:** `make component-gallery-check` (also under `make chimera-gateway-test`) runs [`scripts/check-component-gallery-paths.sh`](../../scripts/check-component-gallery-paths.sh). Windows: `pwsh -File scripts/check-component-gallery-paths.ps1`.

**Related docs:** [`embedui-component-system.md`](embedui-component-system.md), [`embedui-event-log-panel.md`](embedui-event-log-panel.md), [`unified-logs-operator-shell.md`](unified-logs-operator-shell.md).

---

## Phase 1 — Fix production CSS paths

**Goal.** Opening `docs/component-gallery/gallery.html` in a browser shows styled components (not unstyled HTML).

**Deliverables**

- Update `<link rel="stylesheet">` hrefs in:
  - `docs/component-gallery/gallery.html`
  - `docs/component-gallery/gallery-unified-operator.html`
  - `docs/component-gallery/sample.html`
- Fix inline comments that still reference `../../internal/server/embedui/` or `temp/theme/`.
- Fix plan/doc links inside gallery HTML that use wrong depth (e.g. `../docs/plans/…` → `../plans/…` from `docs/component-gallery/`).
- Spot-check `gallery-shell.css` — it should remain gallery-local; no path to production embed required.

**Acceptance**

- `file://` open of `gallery.html` and `gallery-unified-operator.html` loads tokens + logs CSS (network tab shows 200 or local file success, not 404).
- Theme toggle (Default / Porcelain) still switches `html[data-theme]`.
- No gallery file references `internal/server/embedui/` without the `chimera/chimera-gateway/…/adminui/` prefix.

**Status:** `done`

---

## Phase 2 — Gallery README and open instructions

**Goal.** A new contributor can find and use the gallery in one step.

**Deliverables**

- `docs/component-gallery/README.md` with:
  - Purpose (static styleguide; not served by gateway by default).
  - **Open:** double-click HTML or `python -m http.server` from repo root with URL to gallery (optional but documented).
  - File map: `gallery.html` (primitives), `gallery-unified-operator.html` (operator cards draft), `sample.html` (tokens only), `gallery-shell.css`, demo JS files.
  - Production CSS paths table (copy of Background section above).
  - Link to [`embedui/logs/README.md`](../../chimera/chimera-gateway/internal/server/adminui/embedui/logs/README.md) for live app module map.
- One-line pointer from [`embedui-theme-styleguide.md`](embedui-theme-styleguide.md) **References** to this plan and `docs/component-gallery/`.

**Acceptance**

- README paths match Phase 1 hrefs.
- [`embedui-theme-styleguide.md`](embedui-theme-styleguide.md) notes gallery location moved from `temp/theme/` to `docs/component-gallery/`.

**Status:** `done`

---

## Phase 3 — Vocabulary and class-name sync

**Goal.** Gallery examples reflect operator vocabulary and CSS from the post–gateway-refactor embed UI.

**Deliverables**

- Audit gallery HTML for legacy class names and labels called out in [`chimera-gateway-refactor.md`](chimera-gateway-refactor.md):
  - Service badges: `sum-svc-broker`, `sum-svc-vectorstore` (not `sum-svc-upstream`, `sum-svc-qdrant` unless marked legacy).
  - Copy: **chimera-broker** / **chimera-vectorstore** where operator-facing.
- Align demo event-log section with current `sum-evlog-*` contract ([`embedui-event-log-panel.md`](embedui-event-log-panel.md)).
- Unified operator gallery: update provider/routing demo copy to match [`unified-logs-operator-shell.md`](unified-logs-operator-shell.md) shipped cards where drift exists.
- When [`embedui-component-system.md`](embedui-component-system.md) splits CSS into `ui.css` + section files, gallery `<link>` list updated in the same PR as the split.

**Acceptance**

- Manual side-by-side: a `sum-card` + `sum-evlog` region in gallery matches `/ui/logs` structure for the same class list.

**Status:** `done`

---

## Phase 4 — Change contract and optional CI guard

**Goal.** Gallery stays trustworthy as the visual acceptance gate for embed UI changes.

**Deliverables**

- **Change contract** (document in gallery README and [`embedui-component-system.md`](embedui-component-system.md)):
  - If you change `theme-tokens.css`, `logs.css`, or a primitive class used in production → update gallery section(s) in the same PR or explain N/A in PR description.
  - New component → add a gallery section before wiring production (gallery-first).
- Optional script `scripts/check-component-gallery-paths.ps1` (and Makefile target): parse `docs/component-gallery/*.html` link hrefs; fail if any `href` contains `internal/server/embedui` without `adminui/embedui`, or if target file missing on disk.
- Optional: link gallery repair target into `make chimera-gateway-test` only after Phase 1 is green (avoid blocking CI on a broken baseline).

**Acceptance**

- Script exits 0 on main after Phase 1.
- README lists the contract; at least one recent embed CSS PR follows it (pilot).

**Status:** `done`

**Implemented:** [`docs/component-gallery/README.md`](../component-gallery/README.md#change-contract); gallery change contract in [`embedui-component-system.md`](embedui-component-system.md); [`scripts/check-component-gallery-paths.sh`](../../scripts/check-component-gallery-paths.sh) + [`scripts/check-component-gallery-paths.ps1`](../../scripts/check-component-gallery-paths.ps1); `make component-gallery-check` wired into `make chimera-gateway-test`.

---

## Open questions

1. **Serve from gateway:** Should `/ui/gallery` (embedded read-only) be added for WebView/desktop, or keep gallery docs-only + `file://` / static server?
2. **Operator copy section:** Add a gallery section for slug → friendly line previews when [`operator-message-registry.md`](operator-message-registry.md) Phase 2 lands?

---

## References

- Gallery: [`docs/component-gallery/`](../component-gallery/)
- Production embed: [`chimera/chimera-gateway/internal/server/adminui/embedui/`](../../chimera/chimera-gateway/internal/server/adminui/embedui/)
- Prior styleguide plan: [`embedui-theme-styleguide.md`](embedui-theme-styleguide.md)
- Next: [`embedui-component-system.md`](embedui-component-system.md)
