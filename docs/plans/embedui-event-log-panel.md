# Plan: Event log panel layout and interaction (embed UI)

| Field | Value |
|-------|-------|
| **Doc kind** | `feature-plan` |
| **Owners / areas** | Gateway embed UI (`internal/server/embedui`) |
| **Status** | `implemented` |
| **Targets** | Gateway logs summarized view (next minor after current) |
| **Last updated** | See git history |
| **Supersedes / superseded by** | None |

## At a glance

Operators rely on per-card **full event logs** in the summarized logs feed to debug a single conversation or service slice. Today those lines reuse `log-line-sum` (time, level badge, optional service badge, message) inside a scrollable `.sum-full-log` list, with gateway-only visibility toggled by **Show probe HTTP rows** (`localStorage` key `chimera.logs.gateway.showProbes`, `window.__chimeraToggleGatewayProbes`). This plan moves toward a dedicated **event log component**: clearer columns, local search and filters, multi-select with stable identity across filter changes, copy-to-clipboard, summary counts above the list, and an oldest-entry time footer—without changing the meaning of underlying log data.

| Phase | Outcome | Status |
|-------|---------|--------|
| [Phase 1 — Gallery prototypes](#phase-1--gallery-prototypes) | Static gallery sections prove layout, states, and interactions before wiring real data | `done` |
| [Phase 2 — Summarized view implementation](#phase-2--summarized-view-implementation) | Summarized cards render the new component from live entries | `done` |

---

## Background

**Problem.** Full logs on expanded cards are dense: level badges appear for every line including DEBUG/INFO, HTTP success and failure share the same visual channel as prose, and there is no in-card search, filter, selection, or copy—operators scroll raw lists.

**Scope.** Primary target is the **summarized** embed UI: conversation expanded full log (`renderExpandedConv`) and service expanded full log (`renderExpandedService`, event table rows), including the gateway **probe row** toggle (row class / derive hide, not a log-level filter). The main raw table (`appendTableRow`, `#flt-app` / `#flt-level`) is out of scope unless we later unify patterns.

**Existing code touchpoints.** Row HTML: `logSummaryHtml` and conversation variants in `internal/server/embedui/logs.js`. Styles: `internal/server/embedui/logs.css` (e.g. `.sum-full-log`, `.log-line-sum`). Reference gallery: `assets/gallery.html` + `assets/gallery-shell.css`, already loading `theme-tokens.css` and `logs.css`. Gateway metrics strip above the log today lives in **mini cards** (`gatewayServicePanelMiniHtml` — HTTP ok/fail, chat errors, etc.); the new design calls for **compact indicators** aligned upper-right **above** the event table, echoing or summarizing those signals for the log chrome only.

**Related docs:** [`docs/version-v0.3.md`](../version-v0.3.md) (product context), gallery header comments in [`assets/gallery.html`](../../assets/gallery.html).

---

## Phase 1 — Gallery prototypes

**Goal.** Operators and reviewers can open the static gallery and validate typography, spacing, multi-line messages, status/level chips, selection persistence across mock filter changes, search highlighting behavior (if any), and copy button labels—without running the gateway.

**Deliverables**

- New gallery navigation entry and one or more sections under `assets/gallery.html` (and minimal `gallery-shell.css` if layout primitives are missing) documenting the **event log panel** building blocks:
  - **Row layout:** three logical columns — (1) **fixed-width** timestamp using the same display convention as today (`formatLogDateTimeLocal` / existing `log-line-sum__time` styling baseline); (2) **flexible** event message, wrapping across lines without breaking column alignment; (3) **fixed-size** status / severity indicator combining **HTTP status when present and not 2xx** with **WARN/ERROR** (and optionally TRACE/DEBUG if the design needs a neutral placeholder); **omit visible level text for DEBUG and INFO** in the message column (levels still filterable).
  - **Panel chrome:** title (e.g. “Full Event Logs”), search field, **log level** multi-filter, **status / severity** filter (include explicit handling of “non-2xx HTTP” vs “warn/error level”), multi-select rows (checkbox or row selection pattern), **Copy** control (static demo: `navigator.clipboard` optional; may use disabled button with tooltip in static HTML).
  - **Header metrics:** upper-right **warning** and **failure** (or warn+error) count indicators consistent with the vocabulary used on live gateway mini cards (`httpNot2xx`, relay errors, etc.)—static numbers are fine.
  - **Footer:** muted line showing **oldest visible entry** time (copy clarifies “oldest in current filtered set” vs “oldest in loaded window” in open questions below).
- **Multiple demo panels** with distinct datasets:
  - Panel A: at least one row exhibiting **each** warning-style / severity state used in the design.
  - Panel B: at least one row for **each distinct HTTP status class** (e.g. 2xx, 4xx, 5xx, 429) where the third column shows non-2xx clearly.
  - Panel C: **long wrapped messages** (multi-sentence, long URLs) to stress the middle column and row min-height.
- Lightweight **demo script** inline in the gallery page or a tiny `assets/gallery-event-log-demo.js` (only if needed): mock filter application, **selection set** keyed by stable demo ids so clearing search/filters does not drop selection; document the behavior in a short caption under each demo.

**Acceptance**

- Gallery loads from disk with relative paths unchanged; no Go build required to review CSS/HTML.
- At least three visually distinct event log demos (A/B/C above) are present and linked from the gallery nav.
- README-style caption under the section lists which CSS classes are **contract** for Phase 2 (`logs.css`).

**Status:** `done`

---

## Phase 2 — Summarized view implementation

**Goal.** Expanded conversation and service cards (including gateway with probe toggle) render the new event log component; behavior matches the gallery contract for columns, filters, selection persistence, copy, header counts, and footer time.

**Deliverables**

- **CSS** in `internal/server/embedui/logs.css`: grid or table layout for the three-column row; focus-visible and selected-row styles; toolbar and footer; compact header metrics. Prefer new BEM-like prefixes (e.g. `sum-evlog-*`) to avoid breaking `.log-line-sum` usages elsewhere until migrated.
- **Markup generation** in `logs.js` (or extracted helper module if size warrants): replace or wrap the current `<div class="sum-full-log"><ul><li class="sum-ev-item">…` pattern for in-scope cards with the new structure; preserve `refreshSummarizedPanel` scroll restoration by extending the existing `.sum-full-log` id / scroll capture pattern or equivalent for the new scroll root.
- **Data wiring:** map each `ent` / event to: timestamp, primary message (`primaryLogMessage`), level canon, structured HTTP status when available (`getFlat` / `shape === "http.access"` paths used today for pills). Third column renders combined state (text + color) per Phase 1.
- **Filters:** client-side only on the in-memory card slice; level filter hides rows but **does not clear selection** for entries still in the full card dataset (selection keyed by stable id: prefer `seq` + card id hash, or derived stable key; document choice in code comment).
- **Search:** substring (or token) match on message + optional timestamp string; debounced input.
- **Copy:** copies selected rows as plain text (one line per row or multi-line blocks with timestamps—match gallery decision); toast or status line feedback via existing status element patterns.
- **Gateway probes:** keep `GW_PROBES_LS` / `__chimeraToggleGatewayProbes` behavior; probe rows remain in the dataset and respect the toggle; optionally surface “probes hidden” in the new status filter copy if useful.
- **Header counts:** derive warn/error and non-2xx (or failure) counts from the same card `arr` used for the list so numbers align with visible semantics.

**Acceptance**

- Manual: expand a gateway, indexer, and conversation card; confirm columns, wrapping, missing level for DEBUG/INFO, third column for 404/500 and WARN/ERROR, search + filters + select + filter clear preserves selection, copy buffer contents, footer oldest time updates when filters change.
- Automated: if the repo has embed UI snapshot or DOM tests, add minimal coverage for the new root class and a filtered+selected row count; otherwise skip and note in PR.

**Status:** `done`

---

## Open questions

1. **Oldest time footer:** “Oldest entry” means oldest in the **currently filtered** list, oldest in the **full card dataset**, or oldest in the **scroll viewport**? Recommendation: match filtered list for consistency with search.
2. **Copy format:** single-line TSV, multi-line human block, or JSON lines for tooling?
3. **Service badge / tier chips:** conversation rows today show `sum-conv-tier` and service badges in `logSummaryHtml` options—should those move into column 2, column 3, or a narrow fourth strip?
4. **BiFrost / bifrost-only layout** (`sum-full-log--bifrost`): same component or exempt in v1?
5. **Indexer recent files** list uses a different row layout (`indexer-recent-files`); confirm it stays separate from this event log panel.

---

## References

- Code: `internal/server/embedui/logs.js` (`logSummaryHtml`, `renderExpandedService`, `renderExpandedConv`, `refreshSummarizedPanel`, `__chimeraToggleGatewayProbes`, summarized event log `sum-evlog-*` panel), `internal/server/embedui/logs.css`, `internal/server/embedui/logs/derive/gatewayCardModel.js`
- Gallery: `assets/gallery.html`, `assets/gallery-shell.css`
- Entry HTML: `internal/server/embedui/logs.html`
