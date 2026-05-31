# Plan: Operator embed UI mobile layout

| Field | Value |
|-------|-------|
| **Doc kind** | `feature-plan` |
| **Owners / areas** | Gateway embed UI (`/ui`, `/ui/settings`, `/ui/chat`) |
| **Status** | `active` |
| **Targets** | Gateway v0.4 operator UX |
| **Last updated** | See git history |
| **Supersedes / superseded by** | None |
| **As-built** | [`operator-embed-ui-mobile-layout.md`](../features/operator-embed-ui-mobile-layout.md) |

## At a glance

Phone-width viewports (from ~390px, e.g. iPhone 12) must render operator settings cards and scoped event logs without vertical letter-stacking, clipped toggles, or columns pushed off-screen. Phase 1 ships the settings summarized-card header grid, a two-column scoped event log with stacked timestamps and inline source/status meta, and virtual-model toggle stacking. Phase 2 extends the same mobile baseline to chat and the app shell ribbon. Phase 3 syncs the component gallery.

| Phase | Outcome | Status |
|-------|---------|--------|
| [Phase 1 — Settings cards and scoped event log](#phase-1--settings-cards-and-scoped-event-log) | Collapsed cards and scoped logs readable at 390px width | `done` |
| [Phase 2 — App shell and chat surfaces](#phase-2--app-shell-and-chat-surfaces) | Ribbon, chat transcript, and composer follow the same mobile baseline | `done` |
| [Phase 3 — Gallery and regression harness](#phase-3--gallery-and-regression-harness) | Gallery fixtures and docs match production markup; optional 390px visual checks | `done` |

---

## Background

The operator UI uses a fixed horizontal flex row for collapsed summarized cards (avatar, title, metrics, chevron) and a four-column scoped event log on gateway/indexer cards. At ~250px of usable card width (390px viewport minus ribbon and gutters), titles wrap one character per line and the message column collapses when source and status columns consume fixed width.

**Related docs:** [Operator settings UI](../features/operator-settings-ui.md), [Operator embed UI mobile layout](../features/operator-embed-ui-mobile-layout.md) (platform requirements), [Operator left navigation ribbon](../features/operator-left-navigation-ribbon.md).

---

## Phase 1 — Settings cards and scoped event log

**Goal.** Operators can read collapsed service/admin cards and scoped event logs on a 390px-wide phone without horizontal overflow or unreadable vertical text.

**Deliverables**

- `styles/settings-mobile.css` — collapsed summary grid (avatar corner, title, subtitle, metrics row); tighter feed gutters; VM toggle stack; identity KV single column.
- `styles/evlog.css` — two-column table (time + message); stacked datetime; message wrap with fixed meta row.
- `settings_app.js` — `formatLogDateTimeLocalHtml` (stacked, no year), `formatLogDateTimeLocalCompact` (footer/search).
- `settings/render/sumEvlog.js` — always inline source/status/tier in `.sum-evlog__msg-meta`; message body in `.sum-evlog__msg-text`; drop dedicated source/status columns.
- `settings/handlers/evlog.js` — copy/search/footer read inline meta selectors.
- `embedui_test/settings_components_test.go` — layout contract tests.
- Feature record: `docs/features/operator-embed-ui-mobile-layout.md`.

**Acceptance**

- Chrome DevTools iPhone 12 (390×844): Overview and Model usage metrics titles render horizontally; pills sit below title/subtitle aligned with title.
- Gateway scoped log: message text horizontal; source and status visible as chips in the meta row; no column off-screen.
- Virtual model card: Enabled and Visibility toggles fully visible with labels.
- `go test ./chimera/chimera-gateway/internal/server/adminui/embed/embedui_test -run SumEvlog` passes.

**Status:** `done`

---

## Phase 2 — App shell and chat surfaces

**Goal.** Chat and the left navigation ribbon use the same mobile baseline as settings (breakpoints, gutters, touch targets).

**Deliverables**

- `styles/embed-mobile.css` — shared `--embed-space-gutter` and card padding tokens at `max-width: 480px`; chat viewport/composer/message rules; ribbon narrow-mode CSS guardrails.
- Import `embed-mobile.css` from `settings.css` and `chat.css` (shell loads chat.css on `index.html`).
- `shell/navRibbon.js` — `matchMedia('(max-width: 480px)')` forces collapsed ribbon, disables history expand toggle, restores desktop preference when widening.
- `embedui_test/shell_nav_test.go` — asserts mobile assets and narrow-ribbon hooks.
- Breakpoint tokens documented in feature record.

**Acceptance**

- `/ui/chat` at 390px: transcript and composer usable without horizontal scroll.
- Ribbon collapsed width (~3rem) leaves ≥300px for iframe content at 390px viewport.
- Expanded ribbon cannot consume the frame on narrow viewports (toggle disabled; CSS fallback).

**Status:** `done`

---

## Phase 3 — Gallery and regression harness

**Goal.** Design gallery fixtures mirror production markup so mobile layout work is reviewable without a live gateway.

**Deliverables**

- Update `settings/gallery.html` scoped-log samples to two-column + inline meta markup.
- Update `gallery/gallery-event-log-demo.js` and `gallery-unified-operator-users.js` selectors/HTML.
- Optional: note in operator dev README for 390px Chrome device preset.

**Acceptance**

- `/ui/settings/gallery` event-log demos match production table structure.
- Gallery copy/search handlers use `.sum-evlog__msg-meta` / `.sum-evlog__msg-text`.

**Status:** `done`

---

## Open questions

1. Should desktop collapsed cards adopt the stacked header grid above ~480px for consistency, or keep horizontal row on tablet/desktop only?
2. Container queries on `.sum-evlog` vs viewport `@media` — defer until post–Phase 3?

---

## References

- Code: `chimera/chimera-gateway/internal/server/adminui/embed/embedui/styles/embed-mobile.css`, `styles/settings-mobile.css`, `settings/render/sumEvlog.js`, `styles/evlog.css`, `shell/navRibbon.js`
- Feature requirements: [`operator-embed-ui-mobile-layout.md`](../features/operator-embed-ui-mobile-layout.md)
- Related: [`operator-settings-ui.md`](../features/operator-settings-ui.md)
