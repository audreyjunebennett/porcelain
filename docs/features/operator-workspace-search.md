# Feature: Operator workspace search

| Field | Value |
|-------|-------|
| **Doc kind** | `feature-record` |
| **Areas** | Gateway RAG retrieval, gateway embed UI, app shell navigation |
| **Status** | `partial` |
| **Introduced** | Gateway operator shell search train (Phases 1–3 of workspace search plan) |
| **Originated from** | [`plans/operator-workspace-search.md`](../plans/operator-workspace-search.md) |
| **Related features** | [Operator left navigation ribbon](operator-left-navigation-ribbon.md), [Operator chat UI](operator-chat-ui.md), [Gateway RAG ingest and retrieval](gateway-rag-ingest-and-retrieval.md), [Indexer workspaces](indexer-workspaces.md) |
| **Depends on** | [Operator UI session auth](operator-ui-session-auth.md), RAG enabled in gateway config, indexer workspaces API |
| **Last updated** | See git history |

## At a glance

Operators search indexed workspace content from **`/ui/search`** without sending a chat completion. They pick a single workspace, enter a query, optionally adjust the relevance score floor, and receive ranked hits with source path, whole-number percentage score, and highlighted excerpt. Search is reachable from the app-shell ribbon (`manage_search`) and loads in the main iframe beside the persistent left nav. A dedicated **`POST /api/ui/rag/search`** endpoint returns the same vector hits chat retrieval uses, plus optional indexer health hints when the collection is empty or embedding/vector store checks fail.

## Operator-visible behavior

### Layout

- **`GET /ui/search`** — standalone search page (`search.html`); **`GET /ui/search?embed=1`** when opened from the app shell iframe.
- **Controls pinned top** — chat-aligned composer panel: query textarea, workspace selector, score-threshold control, search button.
- **Results scroll below** — summary row, optional indexer hint banner, expandable hit cards (chat snippet styling).

### Workspace selector

- Populated from **`GET /api/ui/indexer/workspaces`**; labels show `project_id / flavor_id`.
- Search is blocked with an inline error until a workspace is selected.
- Workspace list refreshes on window focus and every 30 seconds.

### Query and submit

- **Enter** in the query field submits (via shared chat input composer helper).
- Submit briefly highlights the query textarea.
- Empty query clears results without calling the API.
- While a search is in flight, query, workspace, threshold controls, and submit are disabled.

### Score threshold

- Displayed as a **whole-number percentage** (default **72%**), using the same `readiness_score` icon as hit scores.
- **Text field** — direct entry (`72`, `72%`, etc.); invalid input reverts to the last valid value on blur/change.
- **Horizontal slider** — drag to set 0–100%; stays synced with the text field.
- **Arrow keys** — when the threshold field is focused, **↑ / ↓** nudge by one percentage point.
- API requests send `score_threshold` as a **0–1 fraction**; the response echoes the value actually used.

### Results

- **Summary** — hit count plus meta (`threshold NN%`, `top K` when present).
- **Hit rows** — reuse chat embed cards: source path, score as `NN%` with `readiness_score` icon, syntax-highlighted excerpt (Markdown/plain via shared snippet helpers; query terms highlighted in plain text).
- **Default expansion** — all hits render with `<details open>`; operators can collapse individual rows.
- **Zero hits** — explicit empty copy when the query was non-empty; **`indexer_hint`** from the API maps to operator-facing banners (empty collection, no index, embed/vectorstore issues).
- **Idle** — prompt to select workspace and enter a query before first search.

### Shell navigation

- Ribbon **Search** opens `/ui/search?embed=1`; clicking again restores the prior iframe route (same toggle pattern as Settings). See [Operator left navigation ribbon](operator-left-navigation-ribbon.md).

### Persistence

- Search UI state (selected workspace, threshold, last query/results) is **in-memory only** for the page session; reload clears it. No SQLite or `localStorage` for search fields.

## System behavior and contracts

**Invariants**

- Search retrieval calls **`RAG().Retrieve()`** with authenticated tenant coords — same path chat uses before system-message formatting.
- Scope is a **single** `(project_id, flavor_id)` workspace per request; no multi-workspace union.
- Empty or whitespace query → **200** with empty `hits[]` (no embed/search round-trip).
- `project_id` is required on the API; missing → **400**.
- RAG disabled → **503** `{ "error": "RAG is not enabled" }`.
- Retrieve errors → **502** with error message.
- Hit excerpts come from **`rag.SummarizeHits`** (same length cap as chat snippets).
- UI modules stay separated: `gatewayClient.js`, `state.js`, `render/*`, `app.js` orchestrator under `embed/embedui/search/`.

**Decisions**

| Topic | Decision |
|-------|----------|
| Auth | Valid UI session cookie **or** gateway Bearer token on `POST /api/ui/rag/search` |
| Score floor | Operator-adjustable; UI shows percent, API uses 0–1 float; response echoes applied threshold |
| Threshold UX | Percent text + slider + arrow-key nudge; not decimal 0–1 in the field |
| Hit presentation | Chat embed cards and highlight helpers; all hits expanded by default |
| Scope selector | Single workspace until v0.4 multi-scope / flavor-union rules |
| Indexer hints | Enriched server-side from collection stats + storage health checks; mapped to fixed copy in `results.js` |
| Wizard reuse | Deferred — setup wizard step 6 embed mode not shipped (see gaps) |

**Identity / auth / scoping**

- Tenant comes from session or Bearer token; `project_id` / `flavor_id` resolve through gateway RAG defaults when empty flavor/project.
- Collection name derived from tenant + resolved scope (same as ingest/retrieve elsewhere).

## Interfaces

| Surface | Detail |
|---------|--------|
| Page | `GET /ui/search` — authenticated HTML shell |
| Static assets | `GET /ui/assets/search/*`, `GET /ui/assets/styles/search.css` |
| Search API | `POST /api/ui/rag/search` — JSON body `{ query, project_id, flavor_id, score_threshold?, top_k? }` |
| Search response | `{ hits[], collection, top_k, score_threshold, indexer_hint? }`; each hit `{ source, score, text_excerpt, point_id }` |
| Workspaces | `GET /api/ui/indexer/workspaces` — populates selector |
| Request limits | Body max 1 MiB |
| CLI | Same Bearer token as other gateway APIs |

## Code map

| Concern | Location |
|---------|----------|
| Search page HTML | `chimera/chimera-gateway/internal/server/adminui/embed/embedui/search.html` |
| UI orchestrator | `…/embed/embedui/search/app.js` |
| Gateway client | `…/embed/embedui/search/gatewayClient.js` |
| State | `…/embed/embedui/search/state.js` |
| Results / hints | `…/embed/embedui/search/render/results.js` |
| Query highlight | `…/embed/embedui/search/render/highlight.js` |
| Styles | `…/embed/embedui/styles/search.css` |
| Embed routes | `…/embed/routes.go` (`/ui/search`, `/ui/assets/search/`) |
| API handler | `…/internal/server/adminui/api/rag/handlers.go`, `register.go` |
| Request/response types | `internal/operatorapi/rag.go` |
| Retrieval core | `chimera/chimera-gateway/internal/rag/service.go` (`Retrieve`) |
| Shell nav | `…/embed/embedui/shell/navRibbon.js` |
| UI tests | `…/embed/embedui_test/search_page_test.go` |
| API tests | `…/internal/server/adminui/api/rag/handlers_test.go` |

## Verification

- `go test ./chimera/chimera-gateway/internal/server/adminui/api/rag/ -run RAGSearch`
- `go test ./chimera/chimera-gateway/internal/server/adminui/embed/embedui_test -run TestSearch`
- Manual: from `/ui`, open **Search** in the ribbon → select workspace → query indexed content → confirm path, `NN%` score, expanded excerpt; adjust threshold slider and re-run; verify zero-hit hint when collection is empty.

## Out of scope and known gaps

- **Setup wizard step 6** — locked-workspace embed mode, shared wizard shell, and indexer-in-progress copy ([plan Phase 4](../plans/operator-workspace-search.md#phase-4--wizard-and-empty-states)) — not shipped.
- Multi-workspace union search (v0.4 flavor union rules).
- Semantic settings search (v0.4).
- Chat RAG snippet gutter changes.
- Persisting last workspace/threshold across reloads.
- `top_k` control in the UI (API accepts override; UI sends gateway default only).

## References

- Delivery plan: [`plans/operator-workspace-search.md`](../plans/operator-workspace-search.md)
- RAG behavior: [`gateway-rag-ingest-and-retrieval.md`](gateway-rag-ingest-and-retrieval.md)
- Shell entry: [`operator-left-navigation-ribbon.md`](operator-left-navigation-ribbon.md)
- Setup wizard context: [`version-v0.3.md`](../version-v0.3.md) (step 6 acceptance pending Phase 4)
