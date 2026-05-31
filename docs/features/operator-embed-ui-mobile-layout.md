# Feature: Operator embed UI mobile layout

| Field | Value |
|-------|-------|
| **Doc kind** | `feature-record` |
| **Areas** | Gateway embed UI — `/ui`, `/ui/settings`, `/ui/chat`, app shell |
| **Status** | `active` |
| **Introduced** | Gateway v0.4 — settings (Phase 1), chat + ribbon (Phase 2), gallery sync (Phase 3) |
| **Originated from** | [`plans/operator-embed-ui-mobile-layout.md`](../plans/operator-embed-ui-mobile-layout.md) |
| **Related features** | [Operator settings UI](operator-settings-ui.md), [Operator chat UI](operator-chat-ui.md), [Operator left navigation ribbon](operator-left-navigation-ribbon.md) |
| **Depends on** | design-01 theme tokens, summarized card + sum-evlog render pipeline |
| **Last updated** | See git history |

## At a glance

Operator embed surfaces must remain readable on phone-width viewports (~390px CSS pixels, e.g. iPhone 12 in Chrome DevTools). **Settings** use a stacked collapsed-card grid, two-column scoped event logs with inline meta chips, and stacked timestamps. **Chat and the app shell** share `embed-mobile.css` gutter/padding tokens at `max-width: 480px`; the left ribbon auto-collapses on narrow viewports and disables history expand so the main iframe keeps ≥~300px width. **Component gallery** (`/ui/settings/gallery`) uses the same two-column scoped-log markup as production settings cards.

## Operator-visible behavior

### Settings (all phases)

- **Settings card summary (≤480px viewport, open or closed)** — Two columns: small avatar; title, subtitle, and metrics/pills in the content column (shared left edge with title). Pills flow in a left-aligned row and wrap only when they no longer fit. Chevron is overlaid on the right edge and does not consume a grid column.
- **Scoped event log (all widths)** — Table headers are **Time** and **Message** only. Stacked time + short date (no year). Message cell: fixed-height meta row (chips) then text row.
- **Virtual model toggles (≤480px)** — Enabled and Visibility stack with labels; identity KV single column.

### Chat + shell (Phase 2)

- **Shared gutters (≤480px)** — `--embed-space-gutter: 0.65rem` via `embed-mobile.css` (settings feed, chat viewport, composer).
- **Chat messages** — Reduced assistant/user side margins; composer footer keeps model, workspace, and send on one row (send anchored bottom-right); snippet blocks scroll/wrap instead of forcing horizontal page scroll.
- **Ribbon (≤480px)** — Stays collapsed (3rem icon rail); history expand toggle disabled with explanatory label; desktop expanded preference restored when viewport widens past 480px.

### Gallery (Phase 3)

- **`GET /ui/settings/gallery`** — Static scoped-log fixtures use `data-sum-evlog-cols="2"` and `.sum-evlog__msg-wrap` / `.sum-evlog__msg-meta` / `.sum-evlog__msg-text`; gallery demo JS copy/search reads inline meta like production `handlers/evlog.js`.

## System behavior and contracts

**Invariants**

- Primary narrow breakpoint: **`@media (max-width: 480px)`** — implemented in `embed-mobile.css` (shared) and `settings-mobile.css` (settings-only grid).
- Shared tokens at narrow width: `--embed-space-gutter`, `--embed-space-card-pad-x`, `--embed-space-card-pad-y`, `--embed-mobile-breakpoint-max`.
- Scoped event log: **`data-sum-evlog-cols="2"`**; `.sum-evlog__msg-wrap` > `.sum-evlog__msg-meta` + `.sum-evlog__msg-text`.
- Ribbon: `navRibbon.js` uses `matchMedia('(max-width: 480px)')`; class **`shell-ribbon--narrow`** on `#shell-ribbon`; expanded state cannot persist on narrow viewports.

**Decisions**

| Topic | Decision |
|-------|----------|
| Breakpoint | 480px max-width media query (covers 390px devices with margin) |
| Shared CSS | `embed-mobile.css` imported by `settings.css` and `chat.css` |
| Ribbon on phone | Force collapsed; disable toggle; do not overwrite stored expanded pref |
| Event log columns | Two columns everywhere; inline meta replaces source/status columns |
| Card summary (settings) | CSS grid; conversation cards excluded |
| Gallery fixtures | Share production markup via `gallery/gallery-evlog-markup.js` |

## Interfaces

| Surface | Detail |
|---------|--------|
| CSS shared | `styles/embed-mobile.css` |
| CSS settings | `styles/settings-mobile.css`, `styles/evlog.css` |
| CSS shell | `styles/shell-ribbon.css` + `embed-mobile.css` ribbon narrow rules |
| JS ribbon | `shell/navRibbon.js` — `NARROW_BREAKPOINT`, `applyViewportRibbonLayout` |
| JS formatters | `formatLogDateTimeLocalHtml`, `formatLogDateTimeLocalCompact` |
| Gallery markup | `gallery/gallery-evlog-markup.js`, `gallery/gallery-event-log-demo.js` |

## Code map

| Concern | Location |
|---------|----------|
| Shared mobile tokens + chat + ribbon CSS | `embed/embedui/styles/embed-mobile.css` |
| Settings card grid | `embed/embedui/styles/settings-mobile.css` |
| Event log table | `embed/embedui/styles/evlog.css` |
| Ribbon controller | `embed/embedui/shell/navRibbon.js` |
| Chat styles entry | `embed/embedui/styles/chat.css` |
| Settings styles entry | `embed/embedui/settings.css` |
| Scoped log render | `embed/embedui/settings/render/sumEvlog.js` |
| Gallery fixtures | `embed/embedui/settings/gallery.html`, `embed/embedui/gallery/gallery-evlog-markup.js` |
| Tests | `embedui_test/settings_components_test.go`, `embedui_test/shell_nav_test.go`, `ui_settings_test.go` (gallery route) |

## Verification

```bash
go test ./chimera/chimera-gateway/internal/server/adminui/embed/embedui_test -run 'SumEvlog|Shell|ChatHTML'
go test ./chimera/chimera-gateway/internal/server -run 'Gallery|UIOperator'
```

Manual: Chrome DevTools **iPhone 12 (390×844)** → `/ui`, `/ui/chat`, `/ui/settings`, `/ui/settings/gallery`.

## Out of scope and known gaps

- Container queries (card-width-based) — deferred.
- Tablet landscape (481–760px) uses desktop card summary row.

## References

- Delivery plan: [`plans/operator-embed-ui-mobile-layout.md`](../plans/operator-embed-ui-mobile-layout.md)
- Settings: [`operator-settings-ui.md`](operator-settings-ui.md)
- Chat: [`operator-chat-ui.md`](operator-chat-ui.md)
- Shell: [`operator-left-navigation-ribbon.md`](operator-left-navigation-ribbon.md)
