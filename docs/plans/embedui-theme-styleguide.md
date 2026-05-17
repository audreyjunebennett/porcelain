# Plan: Embed UI theme and static styleguide

| Field | Value |
|-------|-------|
| **Doc kind** | `feature-plan` |
| **Owners / areas** | Gateway embed UI (`internal/server/embedui`) |
| **Status** | `draft` |
| **Targets** | Summarized logs UI (cards, tables, chips, progress, lists) |
| **Last updated** | See git history |
| **Supersedes / superseded by** | None |

## At a glance

Operators and contributors iterate faster on a consistent product color scheme when they can open a static HTML page, edit CSS, and reload without rebuilding the gateway or running `make`. This plan adds a self-contained token layer plus a component gallery that mirrors the real class names used by `logs.js` / `logs.css`, then optionally refactors shipped CSS to consume the same variables.

| Phase | Outcome | Status |
|-------|---------|--------|
| [Phase 1 — Token layer and static styleguide scaffold](#phase-1--token-layer-and-static-styleguide-scaffold) | Semantic CSS variables and a minimal browser-openable page under `temp/theme/` (or agreed path) with no dependency on the running app | `done` |
| [Phase 2 — Component gallery and card matrix](#phase-2--component-gallery-and-card-matrix) | Exhaustive small primitives (pills, levels, service badges, statuses) plus representative cards, tables, progress bars, and list surfaces matching production class names | `done` |
| [Phase 3 — Align product CSS with tokens](#phase-3--align-product-css-with-tokens) | `logs.css` consumes shared tokens via `@import` of `theme-tokens.css` so theme tweaks propagate to the live embed UI | `done` |

---

## Background

Summarized logs embed UI is built from string templates in `internal/server/embedui/logs.js` and styles in `internal/server/embedui/logs.css`. Today, many colors are hard-coded hex values. A static styleguide lets designers and developers tune surfaces, borders, and state colors in isolation. Anything under `temp/` must remain **unlinked** from shipping code: the Phase 1 page loads canonical tokens via a **relative path** into `internal/server/embedui/theme-tokens.css` (no `temp/` imports from `cmd/` or embed). Tokens are also embedded and served at `/ui/assets/theme-tokens.css` when the logs UI is enabled.

**Related docs:** [`embedui-logs-workspaces-merge.md`](embedui-logs-workspaces-merge.md) (adjacent embed UI work), [`porcelain.plan.md`](../porcelain.plan.md).

---

## Phase 1 — Token layer and static styleguide scaffold

**Goal.** Establish semantic design tokens and a single entry HTML file that loads them via relative paths so `file://` or any static preview works without a dev server.

**Deliverables**

- A `:root` (and optional `html[data-theme="…"]`) token sheet: surfaces, borders, text primary/secondary/muted, semantic success/warn/error, and service-tint slots aligned with how `logs.css` already groups colors.
- One `temp/theme/` HTML entry point (extend or supersede `temp/theme/sample.html` as decided during implementation) that imports token CSS and documents token names in-page or in a short comment block.
- Explicit repo rule restated in this plan: no `cmd/`, `internal/` embed, or Makefile references to `temp/` paths.

**Acceptance**

- Opening the HTML file in a browser shows swatches driven only by CSS variables; changing a variable and reloading updates the page without a Go build.

**Status:** `done`

**Implemented:** `internal/server/embedui/theme-tokens.css` (`:root` + `html[data-theme="porcelain"]`); `temp/theme/sample.html` links that file and shows swatches, a token index, and a Default / Porcelain toggle. `theme-tokens.css` is included in `//go:embed` and registered as `GET /ui/assets/theme-tokens.css` next to `logs.css`.

---

## Phase 2 — Component gallery and card matrix

**Goal.** Provide a component-library-style catalog of the embed UI primitives operators see most often, using the **same class names** as production so the gallery doubles as a contract for styling.

**Deliverables**

- **Typography and chrome:** `sum-title`, `sum-sub`, `sum-section-label`, `muted`, `sum-mono-id`, toolbar/filter patterns (`#filters select`, `sum-full-log-toolbar-*`) as needed.
- **Pills, chips, badges:** `pill-2xx` / `pill-4xx` / `pill-5xx`, `.chip` in `.service-chips`, `sum-conv-chip`, `sum-conv-tier--*` variants, `sum-svc-badge` with outline and filled variants (`sum-svc-web`, `sum-svc-qdrant`, `sum-svc-indexer`, `sum-svc-gateway`, `sum-svc-upstream`).
- **Log line:** `log-line-sum` row with `log-line-sum__lvl` and `lvl-DEBUG` / `lvl-INFO` / `lvl-WARN` / `lvl-ERROR` / `lvl-TRACE` / `log-line-sum__lvl--none`, plus `sum-svc-badge` on the line as in production.
- **Progress:** `sum-timeline-bar` + `sum-timeline-seg` + legend/caption; `sum-conv-lifecycle-bar` (full and `sum-conv-lifecycle-bar--compact`) with `sum-conv-lifecycle-seg--*` states; indexer-style `indexer-scope-progress` / captions where styles exist.
- **Tables:** `sum-metrics-table` with `td.num`, `code.sum-mono-id`.
- **Lists:** `sum-full-log` with `li.sum-ev-item`; indexer recent-file row pattern (`indexer-recent-*`) if still present in CSS.
- **Cards:** `<details class="sum-card">` with variants (`sum-card--conversation`, `sum-card--indexer-stale` as applicable); matrix of closed vs open, multiple `sum-avatar` hue classes (`sum-av-a` … `sum-av-f`, `sum-av-svc-*`), `sum-status` + `sum-st-*`; at least one expanded card with lorem body and numeric metrics; collapsed cards showing distinct summary markings (stripe/hover/chevron behavior).
- Optional split into `tokens.css` + `gallery-components.css` if a single file exceeds comfortable edit size; still no server required.

**Acceptance**

- A reviewer can compare a production summarized view side-by-side with the gallery and recognize matching structures; chip/badge inventory is exhaustive without requiring every chip-on-card combination.

**Status:** `done`

**Implemented:** `temp/theme/gallery.html` loads `theme-tokens.css`, `logs.css`, and `gallery-shell.css`; sections cover typography, `#filters` / `sum-full-log-toolbar`, pills/chips/tiers/service badges, `sum-status`, `log-line-sum` levels, timeline + lifecycle + indexer scope bars, `sum-metrics-table`, `sum-full-log` / indexer recent rows, `sum-conv-kv` + `indexer-run-kv`, and a card matrix (avatar hues, conversation collapsed/open, service + `sum-card--indexer-stale`). `temp/theme/sample.html` links to the gallery.

---

## Phase 3 — Align product CSS with tokens

**Goal.** Reduce drift: shipped `logs.css` consumes the same semantic variables the gallery uses, so theme tweaks propagate to the real embed UI.

**Deliverables**

- Incremental replacement of repeated hex values in `logs.css` with `var(--…)` referencing the token set (start with high-churn areas: cards, pills, log levels, service badges, status pills).
- If tokens need to ship beside `logs.css`, the canonical file is **`internal/server/embedui/theme-tokens.css`** (already embedded and served at `/ui/assets/theme-tokens.css` when logs UI is enabled). The static gallery under `temp/theme/` loads the same file via a **relative path** for `file://` iteration.

**Acceptance**

- Changing a token in the canonical shipped sheet changes both the app (after `chimera-build` per project rules) and, when the gallery imports or mirrors that sheet, the static gallery.

**Status:** `done`

**Implemented:** `logs.css` begins with `@import url("theme-tokens.css");` (resolved relative to `/ui/assets/`). High-churn colors (body, tables, filters, pills, log levels, service badges, status pills, cards, chips, KV, lifecycle, strip captions, avatars, chevron hovers, open-card body tints, metrics tables, etc.) use `var(--embed-*)`. `theme-tokens.css` gained a Phase 3 variable block for values not covered by the original semantic set. `rgba(...)` / `rgb(...)` in gradients and card-intro strips are unchanged for this pass.

---

## Resolved decisions

1. **Key–value UI** — Means definition-list and KV rows (`sum-conv-kv*`, `indexer-run-kv*`, inline label/value with colons), not a separate “vault” feature. Phase 2 gallery will include those surfaces.
2. **Canonical tokens** — `internal/server/embedui/theme-tokens.css` from the start (no long-lived duplicate definitions in `temp/`).
3. **`make`** — Add a Makefile target only if multiple sources must be assembled into a single artifact; otherwise multi-file + relative links only.

---

## References

- Code: `internal/server/embedui/logs.js`, `internal/server/embedui/logs.css`, `internal/server/embedui/theme-tokens.css`, `temp/theme/sample.html`, `temp/theme/gallery.html`, `temp/theme/gallery-shell.css`
- Plans: [`embedui-logs-workspaces-merge.md`](embedui-logs-workspaces-merge.md)
- Config (naming only): `config/tokens.yaml` — unrelated runtime tokens unless explicitly unified later
