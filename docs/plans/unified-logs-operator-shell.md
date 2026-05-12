# Plan: Unified operator view in logs (shell + cards)

| Field | Value |
|-------|-------|
| **Doc kind** | `feature-plan` |
| **Owners / areas** | Gateway embed UI, desktop shell, operator logs |
| **Status** | `draft` |
| **Targets** | Gateway operator UI (post–log view refactor baseline) |
| **Last updated** | See git history |
| **Supersedes / superseded by** | None |

## At a glance

Operators should land in one place: the live event log, with the same facts and controls that today are split across the desktop **Main** iframe (gateway index at `GET /`), the **Admin** panel (`/ui/panel`), and a separate **Logs** tab. This plan consolidates configuration and status into collapsible **cards** above (or beside) the log stream, removes redundant shell tabs and the logs page chrome, and keeps deep links and embed mode working. **Static gallery examples ship before any production wiring in `/ui/logs`** so layout, density, and copy can be reviewed without touching the live app.

| Phase | Outcome | Status |
|-------|---------|--------|
| [Phase 1 — Gallery prototypes (static)](#phase-1--gallery-prototypes-static) | [`assets/gallery-unified-operator.html`](../../assets/gallery-unified-operator.html) (linked from gallery nav) shows static cards for overview, users, providers, routing, and scoped log snippets | `done` |
| [Phase 2 — Shell and layout unification](#phase-2--shell-and-layout-unification) | Desktop and browser default to a single primary surface; logs are the main view without a duplicate header | `done` |
| [Phase 3 — Operator overview cards](#phase-3--operator-overview-cards) | Gateway version, virtual model, and aggregate service health match what mattered on the old Main surface | `done` |
| [Phase 4 — Admin workflows as cards](#phase-4--admin-workflows-as-cards) | Tokens, providers, and routing controls from `/ui/panel` move into cards with metrics, actions, and scoped log streams | `done` |

---

## Background

Today the desktop shell loads three iframes: **Main** (`GET /`, HTML gateway index with version, virtual model, and dependency health), **Logs** (`/ui/logs?embed=1`), and **Admin** (`/ui/panel`, tokens, Groq/Gemini/Ollama, routing YAML, fallback chain, router tooling). Operators context-switch across tabs even though logs already host summarized metrics cards. Bringing configuration and status into `/ui/logs` reduces navigation and aligns “observe” with “configure.”

**Related docs:** [`desktop-ui.md`](desktop-ui.md), [`log-view-refactor.md`](log-view-refactor.md), [`logs-ui-maintainability.md`](logs-ui-maintainability.md), [`embedui-event-log-panel.md`](embedui-event-log-panel.md).

---

## Target card layout (authoritative detail)

Sections are ordered roughly as operators scan: **overview** (from Main), then **access** (tokens / users), then **providers**, then **routing**. Each collapsible card uses the existing logs visual language (collapse, chips, summarized metrics) unless a deliberate style pass says otherwise.

### Overview (Main parity)

- **Gateway** — Visible at a glance: **gateway version**, **virtual model** id/name, optional build or channel hint if already available from API.
- **Service health** — Single composite indicator: all critical services **up and healthy** vs degraded, with per-service breakdown (BiFrost/upstream, optional Qdrant when RAG on, indexer supervision when applicable) consistent with data already shown on `GET /` and/or `/api/ui/state`.

### Users and tokens (Admin parity)

- **Gateway tokens** — Present as a **workflow**: list existing tokens, create with label, one-time secret display, copy, delete; **existing and new users** appear as **user cards** in a dedicated **Users** section (labels, metadata, actions), reusing `/api/ui/tokens` semantics.
- **Provider section** — Section title plus short explanation that these rows drive upstream inference through BiFrost.

#### Provider card: Groq

- **Title:** provider name.
- **Collapsed:** chip — **keys count**; chip — **models available** (from catalog/state); indicator — **available** per BiFrost/gateway reporting (same notion as panel “reachable”).
- **Summary (expanded):** short description + **link** to Groq’s public docs/marketing page; **table or list** of models with **usage counts** from gateway metrics (`/api/ui/metrics` or derived); **manage keys** (add/remove, masked hints) wired to existing `POST /api/ui/provider/groq/keys` and delete.
- **Logs:** scoped stream for **health**, **metrics**, and **requests** that selected this provider, plus requests that enumerated **all** providers where applicable.

#### Provider card: Gemini

- Same interaction pattern as Groq (keys + models chips, availability, description + external link, per-model usage, key management, scoped logs).

#### Provider card: Ollama

- **Title:** provider name.
- **Collapsed:** chip — **models available**; indicator — **available** per BiFrost.
- **Summary:** description + external link; per-model **usage counts**; **server URL / base URL** editor (save/cancel) via existing Ollama endpoint; no API key chip (unless product later adds one).
- **Logs:** health, metrics, requests for Ollama; include “all providers” cases as for Groq/Gemini.

### Routing section

#### Card: Routing rules

- **Title:** routing rules (virtual model policy).
- **Collapsed:** chip — **active rules count** (or equivalent summary from policy).
- **Summary:** **table** of rules with **usage counts** (from log-derived or metrics-derived counts, aligned with how routing decisions are recorded today); **YAML** (or structured) **preview** of the policy; **edit / cancel / save** flow including **auto-generate from live catalog** (parity with **Preview** / **Save routing** on the panel); optional free-tier filter toggle if kept as a first-class control.
- **Logs:** full stream (or filter) for **routing decisions** and evaluation events.

#### Card: Routing fallback

- **Title:** fallback chain.
- **Summary:** **ordered list** with **provider** and **model** in separate columns; **provider column** uses the **same color tokens** as provider cards; **usage count per model** from metrics; **configure / modify / cancel / save** with **auto-generate from live catalog** (parity with panel fallback textarea + generate).
- **Logs:** events related to **fallback** and failover.

#### Card: Router model (tool router)

- **Title:** router model / tool-router (wording aligned with operator docs).
- **Collapsed:** chip — **number of router models** configured; chip or icon — **enabled vs disabled**.
- **Summary:** short description; **ordered list** of router models in **usage order**; controls to **enable/disable**, set **confidence threshold**, **transformer** toggles, and **save** (parity with panel router section); **configure router model list** with optional **auto-generate from live catalog** where the backend already supports filling the list; dry-run / evaluate if retained.
- **Logs:** events for **tool-router** calls and outcomes.

**Note:** The user-facing label “routing model” above means the **tool-router / router models** block from the admin panel, not the virtual model id.

---

## Phase 1 — Gallery prototypes (static)

**Goal.** Frozen **HTML/CSS examples** of every card family from **Target card layout** so implementers and reviewers agree on structure before `logs.js` grows new render paths.

**Deliverables**

- **Unified operator (draft)** page [`assets/gallery-unified-operator.html`](../../assets/gallery-unified-operator.html) (nav anchor `sg-unified-operator`; standalone page so later phases can iterate without the full component gallery) demonstrates, using existing `sum-card`, chips, tables, `log-line-sum`, and `indexer-run-kv` patterns:
  - **Overview:** gateway version + virtual model card; composite **all healthy** indicator + per-service breakdown.
  - **Users / tokens:** token-creation workflow row and a **grid of user cards** (labels, fake ids, Revoke).
  - **Providers:** section blurb; **Groq**, **Gemini**, and **Ollama** cards with collapsed-summary chips (keys/models/availability rules per spec); **one** expanded provider body showing description + external link, per-model usage table, key or URL editing, and a **scoped log** sample block.
  - **Routing:** **routing rules** card (active rule count chip, rule table + counts, YAML preview, Preview / Save / Generate-from-catalog buttons as static/disabled demos); **fallback** card (two-column provider/model table with provider **color chips** aligned to provider card hues); **tool-router** card (model count, enabled chip, threshold, textarea list, toggles).
- Add only **gallery-local** styling in [`assets/gallery-shell.css`](../../assets/gallery-shell.css) when production classes are insufficient (e.g. provider column pills in the fallback table, user-card grid gaps). Prefer `var(--embed-*)` from [`theme-tokens.css`](../../internal/server/embedui/theme-tokens.css).
- Mention in the gallery section prose that examples are **non-functional** and trace to this plan doc.

**Acceptance**

- Opening `gallery-unified-operator.html` in a browser shows the full operator draft **without** running the gateway; every bullet under **Target card layout** has a visible reference (possibly combined, e.g. one expanded provider covers the shared pattern for Groq/Gemini).
- No new routes or embed UI behavior are required for this phase.

**Status:** `done`

---

## Phase 2 — Shell and layout unification

**Goal.** One primary operator surface: authenticated users open **logs** (embedded or full page) without treating it as a secondary tab behind **Main**.

**Deliverables**

- Update [`internal/server/embedui/shell.html`](../../internal/server/embedui/shell.html) (and any desktop default URL in [`cmd/claudia`](../../cmd/claudia) if required) so the **default visible frame** is `/ui/logs` (or `/ui/desktop` loads logs directly without Main/Admin iframes).
- Remove or hide **Main** and **Admin** tab buttons once their content is migrated (or reduce to a single optional overflow menu if product needs a transition period).
- In [`internal/server/embedui/logs.html`](../../internal/server/embedui/logs.html) / [`logs.js`](../../internal/server/embedui/logs.js) (and modules under `embedui/logs/`), remove or fold the **logs header** when `embed=1` (and optionally in standalone) so the **log stream reads as the main view**; preserve critical controls (view mode, filters, reconnect) by moving them into a compact toolbar or card rail.
- Adjust post-login redirects and links ([`internal/server/ui_handlers.go`](../../internal/server/ui_handlers.go), [`embedui/login.html`](../../internal/server/embedui/login.html), [`metrics.html`](../../internal/server/embedui/metrics.html)) so they do not assume `/ui/panel` or `GET /` as the primary home.
- Decide fate of **`GET /` gateway index**: keep for unauthenticated/API discovery only, thin redirect when session present, or static link from a card—“?” in Open questions below.

**Acceptance**

- Fresh desktop launch after login shows **logs** as the dominant view without an extra redundant top header.
- Embed message flow (`postMessage` activation) still fires when the shell shows logs.
- No broken navigation from bookmarks to `/ui/logs` or `/ui/desktop`.

**Status:** `done`

---

## Phase 3 — Operator overview cards

**Goal.** Everything operators relied on from the **Main** tab is visible alongside logs without opening `GET /`.

**Deliverables**

- Add **Gateway** and **Service health** cards (or one combined card with two panels) sourcing the same underlying data as the gateway index template in [`internal/server/server.go`](../../internal/server/server.go) (`gatewayIndexTmpl` data assembly) and/or [`GET /api/ui/state`](../../internal/server/ui_handlers.go).
- Implement **polling or SSE-driven refresh** consistent with existing logs metrics refresh cadence so version/health do not drift silently.
- Document operator-visible fields in acceptance tests or UI tests (reuse patterns from [`internal/server/ui_logs_test.go`](../../internal/server/ui_logs_test.go)).

**Acceptance**

- Version string and virtual model match `GET /` for the same running process.
- Health summary matches degraded/ok semantics expected from BiFrost probe and optional Qdrant/indexer when enabled.

**Status:** `done`

---

## Phase 4 — Admin workflows as cards

**Goal.** Retire separate **Admin** iframe content by rehousing [`internal/server/embedui/panel.html`](../../internal/server/embedui/panel.html) behaviors into logs cards, including **scoped log panels** per concern.

**Deliverables**

- **Users / tokens:** Implement the workflow and **user cards** using existing `/api/ui/tokens` endpoints; migrate any client-side validation and error handling from panel scripts into shared logs modules.
- **Provider cards:** Groq, Gemini, Ollama per **Target card layout**; reuse `/api/ui/state`, `/api/ui/bifrost/providers`, provider key POST endpoints, Ollama base URL POST; wire **metrics** (`/api/ui/metrics`) for per-model counts.
- **Routing cards:** Rules, fallback, router model — migrate preview/generate/evaluate/filter/router_tooling POST flows from panel into cards with dirty-state, cancel, and save.
- **Scoped logs:** Extend log filtering UI (query params, client filter, or new narrow API — see Open questions) so expanding a card can show **only** relevant timeline rows (provider-scoped HTTP, routing decisions, fallback, router calls).
- **Deprecation:** Redirect `/ui/panel` → `/ui/logs` with fragment or query to open the right card; remove duplicate static assets when unused.

**Acceptance**

- All operations currently possible on `/ui/panel` (tokens, keys, Ollama URL, routing YAML, fallback list, router settings, dry-run evaluate) remain possible from `/ui/logs` without opening the admin iframe.
- Provider availability and key counts match pre-migration behavior for the same gateway state.

**Status:** `done`

---

## Agent handoff notes (card editing guide)

Use this as the quick map for follow-on card work in `/ui/logs`.

### Primary implementation files

- Main summarized-card renderer: [`internal/server/embedui/logs.js`](../../internal/server/embedui/logs.js).
- Card styling + shared strip/chip geometry: [`internal/server/embedui/logs.css`](../../internal/server/embedui/logs.css).
- Markup shell / script wiring: [`internal/server/embedui/logs.html`](../../internal/server/embedui/logs.html).
- Log derivation helpers used by card summaries/scoped streams: [`internal/server/embedui/logs/`](../../internal/server/embedui/logs/).

### Card structure and naming conventions

- Most operator cards are emitted by `build*CardHtml()` helpers in `logs.js`.
- Keep the existing shell shape: `<details class="sum-card">` with `<summary>` (collapsed row) and `.sum-body` (expanded article).
- Collapsed row conventions:
  - left: avatar + title/subtitle (`sum-avatar`, `sum-main`, `sum-title`, `sum-sub`);
  - middle: compact chips/indicators (`sum-metrics`, compact bars);
  - right: optional single status pill + chevron.
- Expanded body conventions:
  - `sum-section-label` headings;
  - `indexer-run-kv` for key/value facts;
  - timeline-style bars and mini cards for aggregate health/outcomes;
  - in-card event table via `buildEventLogPanelHtml(...)` for scoped logs.

### Service/status indicators

- Reuse existing segmented-strip primitives before creating new UI patterns:
  - `sum-bf-prov-health-*` classes for compact + expanded segment bars;
  - `sum-timeline-bar` / `sum-timeline-seg` for bucketed distributions;
  - `sum-strip-caption*` for optional textual state legends.
- If a card needs both collapsed and expanded health views, keep one helper that supports `opts.compact` so visuals and state mapping stay consistent.

### Data and refresh sources (do not fork)

- `gatewayOverviewCache` from `/api/ui/state` is the overview card source of truth.
- `bifrostProviderSnapshot` from `/api/ui/bifrost/providers` is the source of truth for provider health.
- `metricsCache` from `/api/ui/metrics` backs model counts and usage rollups.
- Keep polling behavior centralized (existing `sync*Polling` functions); avoid adding per-card timers.

### Safe-edit practices for multiple agents

- Preserve existing IDs/classes used by tests and event delegation (`data-admin-action`, summary card ids, scoped log hooks).
- Prefer adding/adjusting small renderer helpers over editing many call sites inline.
- Keep copy short and operational; avoid introducing new terminology for existing controls.
- When changing a card, verify both states:
  - collapsed summary row in normal list flow;
  - expanded body with labels, bars, and scoped log block.
- If a new pattern is needed, mirror it in gallery artifacts first when feasible (`assets/gallery-unified-operator.html`, `assets/gallery-shell.css`), then port to `logs.js`.

---

## Open questions

1. **`GET /` after authenticated desktop use:** Redirect to `/ui/logs`, keep the rich HTML dashboard for bookmarking, or show a minimal landing with a link back to logs?
2. **Scoped logs implementation:** Prefer client-side filter only (full stream already loaded), dedicated `GET /api/ui/logs` query parameters per scope, or a small number of SSE channels — tradeoffs for memory and security.
3. **“All providers” requests:** Exact log shape for attributing traffic to multiple providers; confirm timeline kinds/types in the log pipeline ([`embedui/logs/`](../../internal/server/embedui/logs/)) before UI promises.
4. **Accessibility and keyboard UX:** Collapse/expand parity for nested cards vs current panel forms.
5. **Transition:** One release with tabs hidden vs feature flag vs time-limited deprecation banner on `/ui/panel`.

---

## References

- Code: [`assets/gallery.html`](../../assets/gallery.html), [`assets/gallery-unified-operator.html`](../../assets/gallery-unified-operator.html), [`assets/gallery-shell.css`](../../assets/gallery-shell.css), [`internal/server/server.go`](../../internal/server/server.go) (`GET /`, `gatewayIndexTmpl`), [`internal/server/embedui/shell.html`](../../internal/server/embedui/shell.html), [`internal/server/embedui/panel.html`](../../internal/server/embedui/panel.html), [`internal/server/embedui/logs.html`](../../internal/server/embedui/logs.html), [`internal/server/embedui/logs.js`](../../internal/server/embedui/logs.js), [`internal/server/ui_handlers.go`](../../internal/server/ui_handlers.go)
- Docs: [`desktop-ui.md`](desktop-ui.md), [`configuration.md`](../configuration.md)
- Tickets / PRs: (add when filed)
