# Plan: Operator workspace search

| Field                          | Value                                                                                              |
|--------------------------------|----------------------------------------------------------------------------------------------------|
| **Doc kind**                   | `feature-plan`                                                                                     |
| **Owners / areas**             | Gateway RAG, operator UI app shell, embed UI                                                       |
| **Status**                     | `draft`                                                                                            |
| **Targets**                    | Gateway v0.3 setup wizard step 6, gateway v0.5 navigation                                          |
| **Last updated**               | See git history                                                                                    |
| **Supersedes / superseded by** | Complements [`operator-chat-ui.md`](operator-chat-ui.md); distinct from v0.4 settings search theme |
| **As-built**                   | None — link to [`docs/features/`](../features/README.md) when shipped                              |

## At a glance

Operators should **search indexed workspace content directly** — without sending a chat completion — and see ranked hits (path, score, excerpt). Today retrieval runs only on the chat path with results injected when scores pass the gateway floor. This plan adds a **search API**, a **`/ui/search` page**, and a **left-nav entry** in the app shell; the v0.3 setup wizard reuses the same search component for “test indexing” step 6.

| Phase                                                                  | Outcome                                                                               | Status |
|------------------------------------------------------------------------|---------------------------------------------------------------------------------------|--------|
| [Phase 1 — RAG search API](#phase-1--rag-search-api)                   | Session-authenticated query endpoint returns raw retrieval hits for a workspace scope | `todo` |
| [Phase 2 — Search UI page](#phase-2--search-ui-page)                   | Workspace selector + query input + results list; design-01 styling                    | `todo` |
| [Phase 3 — App shell navigation](#phase-3--app-shell-navigation)       | Ribbon item switches iframe to `/ui/search`; state persists like chat/settings        | `todo` |
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

- `POST /api/ui/rag/search` — session auth; body `{ "query": "...", "project_id": "...", "flavor_id": "..." }` (tenant from session).
- Handler calls `rt.RAG().Retrieve()` with session tenant coords; returns JSON:
  - `hits[]`: `{ source, score, text_excerpt, point_id }` (excerpt length capped).
  - `collection`, `top_k`, `score_threshold` for transparency.
  - `indexer_hint` when storage stats / health suggest empty collection or embed misconfig (optional enrichment from existing health helpers).
- Rate limit or max body size consistent with other UI APIs.
- Tests: empty query → empty hits; seeded vectors → ordered hits; wrong scope → empty or 404 per documented behavior.

**Acceptance**

- curl/UI client can search a workspace scope and receive the same hits chat would use before system-message formatting.

**Status:** `todo`

---

## Phase 2 — Search UI page

**Goal.** Dedicated operator page for workspace-scoped search.

**Deliverables**

- `GET /ui/search` — `search.html` + modules under `embed/embedui/search/` (or `chat/` sibling): `app.js`, `gatewayClient.js`, minimal state.
- **Workspace selector** — reuse workspace list fetch from chat (`GET /api/ui/indexer/workspaces`); require selection before search.
- **Query input** — Enter submits; highlight query on submit (wizard spec).
- **Results panel** — Summary row (hit count); detail rows with path, score, excerpt (reuse snippet highlight helpers from chat where possible).
- **Zero results** — Explicit empty state + indexer status hints (idle, error, no chunks) from API `indexer_hint` or secondary health poll.
- Styles in `styles/search.css`; mobile baseline aligned with [`operator-embed-ui-mobile-layout.md`](operator-embed-ui-mobile-layout.md) when practical.

**Acceptance**

- Manual: index a file → search page finds it by keyword; score and path visible.

**Status:** `todo`

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

**Status:** `todo`

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

## Open questions

1. **Score floor** — Expose threshold in UI vs fixed gateway default only.
2. **Flavor union** — Single `(project, flavor)` selector until v0.4 multi-scope search ships.
3. **Auth for search API** — UI session only vs also Bearer for CLI smoke tests.

---

## References

- Code: `internal/rag/service.go` (`Retrieve`), `internal/rag/prompt.go`, `embed/embedui/chat/gatewayClient.js`, `embed/embedui/shell/navRibbon.js`
- Features: [`gateway-rag-ingest-and-retrieval.md`](../features/gateway-rag-ingest-and-retrieval.md)
