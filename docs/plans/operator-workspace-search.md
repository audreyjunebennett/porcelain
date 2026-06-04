# Plan: Operator workspace search

| Field                          | Value                                                                                              |
|--------------------------------|----------------------------------------------------------------------------------------------------|
| **Doc kind**                   | `feature-plan`                                                                                     |
| **Owners / areas**             | Gateway RAG, operator UI app shell, embed UI                                                       |
| **Status**                     | `active`                                                                                           |
| **Targets**                    | Gateway v0.3 setup wizard step 6, gateway v0.5 navigation                                          |
| **Last updated**               | See git history                                                                                    |
| **Supersedes / superseded by** | Complements [`operator-chat-ui.md`](operator-chat-ui.md); distinct from v0.4 settings search theme |
| **As-built**                   | [`operator-workspace-search.md`](../features/operator-workspace-search.md) (Phases 1–3)            |

## At a glance

Operators should **search indexed workspace content directly** — without sending a chat completion — and see ranked hits (path, score, excerpt). Today retrieval runs only on the chat path with results injected when scores pass the gateway floor. This plan adds a **search API**, a **`/ui/search` page**, and a **left-nav entry** in the app shell; the v0.3 setup wizard reuses the same search component for “test indexing” step 6.

| Phase                                                                  | Outcome                                                                               | Status |
|------------------------------------------------------------------------|---------------------------------------------------------------------------------------|--------|
| [Phase 1 — RAG search API](#phase-1--rag-search-api)                   | Session- or Bearer-authenticated query endpoint returns raw retrieval hits for a workspace scope | `shipped` |
| [Phase 2 — Search UI page](#phase-2--search-ui-page)                   | Workspace selector + query input + results list; design-01 styling                    | `shipped` |
| [Phase 3 — App shell navigation](#phase-3--app-shell-navigation)       | Ribbon item switches iframe to `/ui/search`; state persists like chat/settings        | `shipped` |
| [Phase 4 — Wizard and empty states](#phase-4--wizard-and-empty-states) | Setup wizard embeds search with workspace locked; zero-hit and indexer-idle copy      | `todo` |

---

## Background

**Problem.** [`RAG().Retrieve()`](../../chimera/chimera-gateway/internal/rag/service.go) already embeds the query and searches Qdrant with score threshold and top-k. Chat attaches hits only when formatting yields non-empty context ([`virtualmodel_chat.go`](../../chimera/chimera-gateway/internal/server/virtualmodel_chat.go)). Operators validating indexing during setup or debugging retrieval need **deterministic, hit-level feedback** without model latency or confidence masking.

**Related docs:** [`gateway-rag-ingest-and-retrieval.md`](../features/gateway-rag-ingest-and-retrieval.md), [`operator-chat-ui.md`](../features/operator-chat-ui.md), [`operator-left-navigation-ribbon.md`](../features/operator-left-navigation-ribbon.md), [`version-v0.3.md`](../version-v0.3.md) (setup wizard step 6), [`indexer-workspaces.md`](../features/indexer-workspaces.md).

**Out of scope:** Multi-workspace union search (v0.4 flavor union rules), semantic settings search (v0.4), chat RAG snippet gutter changes.

---

## Phase 1 — RAG search API

**Goal.** Expose retrieval hits to the operator UI without a chat round-trip.

**Deliverables**

- `POST /api/ui/rag/search` — **session cookie or Bearer token** (same gateway token as CLI); body `{ "query": "...", "project_id": "...", "flavor_id": "...", "score_threshold": 0.72?, "top_k": 8? }` (tenant from auth).
- Handler calls `rt.RAG().Retrieve()` with authenticated tenant coords; optional `score_threshold` / `top_k` override gateway defaults (threshold also returned for UI display).
- Returns JSON:
  - `hits[]`: `{ source, score, text_excerpt, point_id }` (excerpt length capped).
  - `collection`, `top_k`, `score_threshold` for transparency.
  - `indexer_hint` when storage stats / health suggest empty collection or embed misconfig (optional enrichment from existing health helpers).
- Rate limit or max body size consistent with other UI APIs.
- Tests: empty query → empty hits; seeded vectors → ordered hits; wrong scope → empty hits; unauthorized without session or Bearer.

**Acceptance**

- curl/UI client can search a workspace scope and receive the same hits chat would use before system-message formatting.

**Status:** `shipped`

---

## Phase 2 — Search UI page

**Goal.** Dedicated operator page for workspace-scoped search.

**Deliverables**

- `GET /ui/search` — `search.html` + modules under `embed/embedui/search/` (or `chat/` sibling): `app.js`, `gatewayClient.js`, minimal state.
- **Workspace selector** — single `(project_id, flavor_id)` pair from workspace list (`GET /api/ui/indexer/workspaces`); require selection before search (no multi-scope union until v0.4).
- **Score threshold control** — numeric input bound to `score_threshold` on search requests (gateway default pre-filled).
- **Query input** — Enter submits; highlight query on submit (wizard spec).
- **Results panel** — Summary row (hit count); detail rows with path, score, excerpt (reuse snippet highlight helpers from chat where possible).
- **Zero results** — Explicit empty state + indexer status hints (idle, error, no chunks) from API `indexer_hint` or secondary health poll.
- Styles in `styles/search.css` (chat-aligned embed cards; **controls pinned top**, results scroll below); mobile baseline aligned with [`operator-embed-ui-mobile-layout.md`](operator-embed-ui-mobile-layout.md) when practical.

**Acceptance**

- Manual: index a file → search page finds it by keyword; score and path visible.

**Status:** `shipped`

---

## Phase 3 — App shell navigation

**Goal.** Search is a first-class destination beside chat and settings.

**Deliverables**

- Ribbon icon + label (e.g. `manage_search` / “Search”) in [`index.html`](../../chimera/chimera-gateway/internal/server/adminui/embed/embedui/index.html) and [`navRibbon.js`](../../chimera/chimera-gateway/internal/server/adminui/embed/embedui/shell/navRibbon.js).
- Iframe route `/ui/search?embed=1` when opened from shell; remember prior route when switching back (same pattern as settings).
- Register static assets in [`embed/routes.go`](../../chimera/chimera-gateway/internal/server/adminui/embed/routes.go).
- Update [`operator-left-navigation-ribbon.md`](../features/operator-left-navigation-ribbon.md) when shipped.

**Acceptance**

- From `/ui`, operator opens search from ribbon; chat and settings navigation unchanged.

**Status:** `shipped`

---

## Phase 4 — Wizard and empty states

**Goal.** Setup wizard step 6 and production search share one component.

**Deliverables**

- Wizard embed mode props: `lockedWorkspaceId`, hide workspace picker or pre-select recent workspace from step 5.
- Shared search module imported by wizard shell (when wizard ships) and `/ui/search`.
- Copy for: no indexes in step 5 → step 6 skipped/greyed; indexer still running → “ indexing in progress ” note.

**Acceptance**

- Wizard step 6 acceptance from [`version-v0.3.md`](../version-v0.3.md) met using this search surface.

**Status:** `todo`

---

## Resolved decisions

| Topic | Decision |
|-------|----------|
| **Score floor** | Expose `score_threshold` in the search UI; API accepts optional override and echoes the value used. |
| **Scope selector** | Single `(project_id, flavor_id)` workspace until v0.4 multi-scope / flavor-union search. |
| **Auth** | Required for UI **and** CLI: valid session cookie **or** gateway Bearer token on `POST /api/ui/rag/search`. |

---

## References

- Code: `internal/rag/service.go` (`Retrieve`), `internal/server/adminui/api/rag/` (`POST /api/ui/rag/search`), `embed/embedui/search/` (`GET /ui/search`), `internal/rag/prompt.go`, `embed/embedui/chat/gatewayClient.js`, `embed/embedui/shell/navRibbon.js`
- Features: [`gateway-rag-ingest-and-retrieval.md`](../features/gateway-rag-ingest-and-retrieval.md)
