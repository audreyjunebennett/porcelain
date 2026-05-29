# Plan: Operator conversation history (gateway)

| Field | Value |
|-------|-------|
| **Doc kind** | `feature-plan` |
| **Owners / areas** | Gateway operator SQLite, chat/RAG persistence, gateway embed UI, admin UI session |
| **Status** | `done` |
| **Targets** | Gateway operator shell (`/ui`), chat route (`/ui/chat`), operator SQLite |
| **Last updated** | See git history |
| **Supersedes / superseded by** | Builds on [`operator-chat-ui.md`](operator-chat-ui.md) |

## At a glance

Operators should be able to return to past gateway chats days later: each saved thread includes what they asked, what the model answered, which upstream model actually ran, and which workspace snippets were retrieved—not the internal routing and broker lifecycle noise from the settings log. History is stored durably in operator SQLite, keyed by a stable **principal id** (not the rotating API secret), and exposed through authenticated UI APIs. Operators can title, flag, edit titles, delete threads, and browse history in a panel that reuses the live chat renderer.

| Phase | Outcome | Status |
|-------|---------|--------|
| [Phase 1 — Schema, session principal, and store](#phase-1--schema-session-principal-and-store) | Tables, indexes, store APIs, and UI session bound to `principal_id` | `done` |
| [Phase 2 — Persist turns on chat completion](#phase-2--persist-turns-on-chat-completion) | Gateway writes user, assistant, and error turns plus RAG hits (including dedup-cache replies) | `done` |
| [Phase 3 — History API](#phase-3--history-api) | List, detail, update title, flag, and delete—scoped to session principal | `done` |
| [Phase 4 — Chat UI history panel](#phase-4--chat-ui-history-panel) | Browse, flag, rename, delete, open, and copy saved threads | `done` |

---

## Background

**Problem.** The shipped operator chat UI ([`operator-chat-ui.md`](operator-chat-ui.md)) keeps the full transcript only in browser memory. **New chat** clears it; reload loses it. Operators cannot revisit prior threads, compare retrieval context across sessions, or share a durable transcript outside the ephemeral settings event log.

**What to persist vs omit.** The settings **Summarized** / structured log stream captures internal pipeline detail: routing attempts, token estimates, latency breakdowns, broker events, witness samples, and similar lifecycle lines keyed by `conversation_id`. Those lines are valuable for debugging but redundant for user-facing history and can be large. This feature persists only:

- User, assistant, and **error** message text (final content for completed turns; error rows mirror inline chat error blocks).
- Resolved upstream model id per assistant turn (same semantics as `X-Chimera-Upstream-Model` / chat UI “Model: …” header).
- Selected client/virtual model id when distinct from upstream.
- Token usage from upstream completion JSON when present (`prompt_tokens`, `completion_tokens`, `total_tokens`).
- Workspace retrieval hits attached to the assistant turn—the same bounded summaries already sent to the UI via `X-Chimera-RAG-Hits`, plus stable references (`vector_point_id`, optional `content_sha256`).
- **Dedup-cache responses:** when conversation merge returns a cached completion without re-running upstream, still append a history turn (resolved model from cache metadata or last known upstream when available).

**Identity model (principal, not API secret).** Gateway chat auth uses API tokens (`api-keys.yaml`); each token has a `tenant_id` used as `principal_id` in structured logs. **Rotating or replacing the API key must not orphan history** as long as the operator’s principal id is unchanged. Therefore:

- `conversations.principal_id` is the durable owner key (today: token `tenant_id` at login/persist time; the column name stays `principal_id` so a future stable operator identity can diverge from `tenant_id` without a schema migration).
- **UI session** (`internal/server/adminui/session`) stores `principal_id` when `/api/ui/login` validates a gateway token (`Record.TenantID` → session principal). All `/api/ui/conversations/*` handlers read principal from the session, **not** from “first token in `GET /api/ui/tokens`”.
- Chat persistence in `handleV1Chat` uses the same `principal_id` from the bearer token’s `sess.TenantID` on each request; login and chat must use tokens for the same principal for rows to align.
- Re-login after key rotation: operator signs in with the new secret; if `tenant_id` in YAML is unchanged, history remains visible. Changing `tenant_id` on the credential is treated as a new principal (existing rows stay under the old id).

**Retention.** No TTL or automatic pruning. Conversations remain until the operator **deletes** them.

**Titles.** `conversations.title` is nullable. On create, set an auto-generated title from the first user message (single-line excerpt, ~60–80 characters, ellipsis when truncated—same spirit as Cursor/VS Code Continue list labels). **Later (out of scope for initial delivery):** optional LLM “title this thread” pass on the first prompt. Operators can **edit** the title in the UI; persisted via API.

**Flagging.** `conversations.flagged` supports bookmark/favorite semantics (filter flagged threads in the history list). Not the same as delete; flagged rows stay until deleted.

**Workspace scope.** A conversation records the workspace active when it **started**: `project_id` + `flavor_id` (and optionally `workspace_row_id`). Later turns do not rewrite that snapshot.

**Conversation id alignment.** Live chat uses `X-Chimera-Conversation-Id`. Persisted rows use that id as `conversations.conversation_id`. **New chat** clears the client-held id so the next send creates a new row; continuing a thread reuses the id and appends turns.

**Related docs:** [`operator-chat-ui.md`](operator-chat-ui.md), [`internal-embedding-provider-exploration.md`](internal-embedding-provider-exploration.md).

---

## Decisions

| Topic | Decision |
|-------|----------|
| Dedup cache hits | **Yes** — append a history turn when merge serves cached JSON. |
| Failed / errored turns | **Yes** — persist `role = 'error'` rows with user-visible error text (and optional `retry_user_text` in metadata JSON or column). |
| Retention | **Until user deletes** — no automatic expiry. |
| API scope | **Session `principal_id`** — not derived from whichever API key is currently listed first. |
| Title | **Nullable column**; auto-generate from first user input; user-editable; LLM-generated titles deferred. |
| Delete | **User-initiated delete** via API + UI (hard delete, cascade turns/retrievals). |
| Flag | **`flagged` boolean** on conversation; toggle in UI; optional `?flagged=1` list filter. |

---

## Phase 1 — Schema, session principal, and store

**Goal.** Operator SQLite and the UI session can durably own conversations by `principal_id`, with metadata for title and flag, plus efficient list/load/delete access.

**Deliverables**

- **Session store** (`internal/server/adminui/session`):
  - Session record holds `principal_id` (and expiry) instead of only opaque id → expiry.
  - `Issue(principalID string)` / `SetSessionCookie` passes `Record.TenantID` from validated login token.
  - `PrincipalID(sessionID string) string` (or handler helper `SessionPrincipal(r)`) for API handlers.
  - Re-login issues a new session id but same principal when `tenant_id` unchanged.
- New migration `migrations/chimera-gateway/operator/000003_conversation_history.sql`:
  - **`conversations`**
    - `conversation_id` TEXT PRIMARY KEY.
    - `principal_id` TEXT NOT NULL.
    - `title` TEXT NULL (user override; when NULL, UI may fall back to `preview_text`).
    - `preview_text` TEXT NOT NULL DEFAULT '' (first user message excerpt, ≤512 runes).
    - `flagged` INTEGER NOT NULL DEFAULT 0 CHECK (`flagged` IN (0, 1)).
    - `workspace_project_id` TEXT NOT NULL DEFAULT ''.
    - `workspace_flavor_id` TEXT NOT NULL DEFAULT ''.
    - `workspace_row_id` INTEGER NULL.
    - `created_at` TEXT NOT NULL (RFC3339 UTC).
    - `updated_at` TEXT NOT NULL.
  - **`conversation_turns`**
    - `turn_id` TEXT PRIMARY KEY.
    - `conversation_id` TEXT NOT NULL REFERENCES `conversations` (`conversation_id`) ON DELETE CASCADE.
    - `turn_index` INTEGER NOT NULL.
    - `role` TEXT NOT NULL CHECK (`role` IN ('user', 'assistant', 'error')).
    - `content` TEXT NOT NULL.
    - `selected_model` TEXT NOT NULL DEFAULT ''.
    - `resolved_model` TEXT NOT NULL DEFAULT ''.
    - `error_detail` TEXT NOT NULL DEFAULT '' (optional type/message for `error` rows).
    - `retry_user_text` TEXT NOT NULL DEFAULT '' (for replaying **Retry** in UI).
    - `prompt_tokens` INTEGER NULL.
    - `completion_tokens` INTEGER NULL.
    - `total_tokens` INTEGER NULL.
    - `created_at` TEXT NOT NULL.
  - **`conversation_retrievals`** (unchanged intent; assistant turns only).
    - `retrieval_id`, `turn_id`, `sort_order`, `file_path`, `score`, `snippet_text`, `language`, `vector_point_id`, `content_sha256`.
- Indexes:
  - `idx_conversations_principal_updated` ON `conversations` (`principal_id`, `updated_at` DESC).
  - `idx_conversations_principal_flagged` ON `conversations` (`principal_id`, `flagged`, `updated_at` DESC).
  - `idx_conversation_turns_conversation` ON `conversation_turns` (`conversation_id`, `turn_index`, `role`).
  - `idx_conversation_retrievals_turn` ON `conversation_retrievals` (`turn_id`, `sort_order`).
- `internal/operatorstore/conversations.go`:
  - `EnsureConversation` — on insert set `preview_text`, auto **`title`** from first user message (same generator as preview, shorter limit).
  - `AppendTurn`, `ReplaceTurnRetrievals`.
  - `ListConversations(ctx, principalID, ListFilter)` — `limit`, `offset`, optional `flaggedOnly`.
  - `GetConversationTranscript`, `UpdateConversationTitle`, `SetConversationFlagged`, `DeleteConversation` (cascade).
- Title generator helper: `conversationtitle.FromFirstUserMessage(text string) string` (pure, tested).
- Unit tests: migration, CRUD, cross-principal isolation, delete cascade, flag filter.

**Acceptance**

- Login with token A binds session principal; list returns only that principal’s rows.
- Delete removes conversation and all child turns/retrievals.
- Flag toggle persists and affects filtered list.

**Status:** `done`

---

## Phase 2 — Persist turns on chat completion

**Goal.** After each operator chat exchange (including dedup-cache and failed paths), the gateway appends durable turns without storing lifecycle logs.

**Deliverables**

- Persistence service invoked when `rt.OperatorStore()` is non-nil:
  - **Conversation create:** `EnsureConversation` on first `conversation_id` for principal; workspace from headers; title auto-set from first user message.
  - **Successful exchange:** user row + assistant row + retrievals (same as prior plan).
  - **Dedup-cache path:** when merge returns cached body, still append user + assistant turns; set `resolved_model` from cached JSON `model` field or merge metadata when present.
  - **Error path:** when chat returns inline error to UI (HTTP error JSON or gateway error block), append `user` row (if not already persisted for this attempt) and `error` row with visible message + `retry_user_text`.
  - **Hooks:** `chat.ProxyOpts` — once per completed client delivery (stream end, non-stream body, dedup short-circuit, or error response).
  - **Do not persist:** settings lifecycle logs, routing witnesses, broker timeline rows.
- Best-effort: persistence failure logs warning; never fails chat response.

**Acceptance**

- Dedup replay produces new `turn_index` rows in SQLite.
- Failed request in chat UI leaves `error` role row reloadable in history detail.
- No broker `conversation.received` fields appear in SQLite.

**Status:** `done`

---

## Phase 3 — History API

**Goal.** Session-authenticated operators manage their conversation list and transcripts by `principal_id`.

**Deliverables**

- `internal/server/adminui/api/conversations/` with `RequireAuthJSON`; resolve `principalID := h.SessionPrincipal(r)` (503 if empty).
- Endpoints:
  - `GET /api/ui/conversations?limit=&offset=&flagged=` — list with `conversation_id`, `title`, `preview_text`, `flagged`, workspace fields, timestamps (title falls back to preview in JSON when null).
  - `GET /api/ui/conversations/{conversation_id}` — full transcript.
  - `PATCH /api/ui/conversations/{conversation_id}` — body `{ "title": "…" }` (trim, max length); empty string rejected or clears to auto-display only (product: reject empty, keep nullable DB).
  - `POST /api/ui/conversations/{conversation_id}/flag` — body `{ "flagged": true|false }` (or PATCH with `{ "flagged": … }`).
  - `DELETE /api/ui/conversations/{conversation_id}` — 204 on success; 404 when wrong principal.
- Chat persistence and APIs use the same `principal_id` definition.
- JSON types in `internal/operatorapi` matching UI (`ragHits`, `error`, `retryUserText`).
- Tests: list/detail, patch title, flag toggle, delete, 404 cross-principal, 401 without session.

**Acceptance**

- Operator A cannot read, patch, flag, or delete operator B’s `conversation_id`.
- Delete is permanent; list no longer returns the id.
- Flag filter returns only flagged rows when `flagged=1`.

**Status:** `done`

---

## Phase 4 — Chat UI history panel

**Goal.** Operators browse, flag, rename, delete, open, and copy saved threads with the same rendering as live chat.

**Deliverables**

- **History sidebar** (`chat/historyPanel.js`, `chat/historyClient.js`, `styles/chat.css`):
  - List: title (or preview), relative time, flagged indicator (star/bookmark icon).
  - Filter control: **All** / **Flagged**.
  - Row actions: open, toggle flag, rename (inline edit or small dialog), delete (confirm modal).
- **Title edit:** saves via `PATCH`; optimistic UI update.
- **Delete:** calls `DELETE`, removes from list, clears viewport if that thread was open.
- **Open thread:** `loadTranscript` → existing `MsgRender` (user, assistant, **error** with **Retry** when `retry_user_text` set).
- **Copy:** per-message and **Copy chat** unchanged for loaded history.
- **New chat:** clears active id and selection; does not delete persisted rows.
- Optional shell **History** toggle on chat menu bar if layout needs it.
- Tests: handler coverage from Phase 3; embed test asserts history panel markup exists.

**Acceptance**

- Flagging survives reload; flagged filter works.
- Renamed title appears in list and survives reload.
- Delete removes thread from DB and UI; no lifecycle log content in panel.

**Status:** `done`

---

## Future work (not in initial delivery)

- **LLM-generated titles:** after first user turn, async call with a small “title this conversation” prompt (Cursor / Continue style); update `title` if user has not set a custom one.
- **Search across conversations:** `search_text` column or FTS on `preview_text` + turn content.
- **Tags:** separate `conversation_tags` table.
- **Stable principal registry:** if `tenant_id` per key is insufficient, add explicit `principal_id` on `api-keys.yaml` rows decoupled from secrets.

---

## References

- Shipped chat UI: [`operator-chat-ui.md`](operator-chat-ui.md)
- UI: `chimera/chimera-gateway/internal/server/adminui/embed/embedui/chat.html`, `chat/`, `styles/chat.css`, `index.html`
- Session: `chimera/chimera-gateway/internal/server/adminui/session/session.go`, `handler/handler.go` (`SetSessionCookie`)
- Tokens: `chimera/internal/tokens/tokens.go` (`Record.TenantID`)
- RAG metadata: `chimera/chimera-gateway/internal/rag/response_meta.go`
- Chat path: `chimera/chimera-gateway/internal/server/server.go`, `virtualmodel_chat.go`
- Operator DB: `migrations/chimera-gateway/operator/`, `internal/operatorstore/`
