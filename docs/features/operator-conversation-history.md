# Feature: Operator conversation history

| Field | Value |
|-------|-------|
| **Doc kind** | `feature-record` |
| **Areas** | Gateway operator SQLite, chat persistence, gateway embed UI, admin UI session |
| **Status** | `current` |
| **Introduced** | Gateway operator shell v0.2 train |
| **Originated from** | [`plans/operator-conversation-history.md`](../plans/operator-conversation-history.md) |
| **Related features** | [Operator chat UI](operator-chat-ui.md), [Operator left navigation ribbon](operator-left-navigation-ribbon.md), [Operator UI session auth](operator-ui-session-auth.md), [Operator SQLite store](operator-sqlite-store.md) |
| **Depends on** | [Operator UI session auth](operator-ui-session-auth.md), operator SQLite, live chat renderer |
| **Last updated** | See git history |

## At a glance

Operators can return to past gateway chats days later. Each saved thread includes user and assistant (or error) messages, the upstream model that ran, token usage when available, and workspace retrieval snippets—not internal routing or broker lifecycle lines from the settings log. History lives in operator SQLite, keyed by a stable **principal id** (not the rotating API secret). The chat UI exposes a history panel to browse, flag, rename, delete, open, and copy saved threads using the same renderer as live chat.

## Operator-visible behavior

- **History panel** on `/ui/chat`: list shows title (or preview excerpt), relative time, flagged indicator; filter **All** / **Flagged**; row actions for open, toggle flag, rename, delete (with confirm).
- **Open thread** loads the full transcript with the live chat renderer (user, assistant, error with **Retry** when `retry_user_text` is set).
- **Copy** per-message and **Copy chat** work on loaded history the same as live chat.
- **New chat** clears the client-held conversation id and panel selection; it does **not** delete persisted rows.
- **Title edit** saves via API; list shows the custom title after reload.
- **Delete** removes the thread permanently from the database and list; clears the viewport if that thread was open.

## System behavior and contracts

**Invariants**

- History is scoped to **session `principal_id`**, never to “whichever API key is listed first” in the tokens UI.
- Rotating an API secret must **not** orphan history when `tenant_id` on the credential is unchanged.
- Persisted content is **user-facing transcript data only**—not settings lifecycle logs, routing witnesses, broker timeline, or similar debug streams.
- **New chat** starts a new `conversation_id` on the next send; continuing a thread reuses the id and appends turns.
- Workspace scope is snapshotted when the conversation **starts** (`project_id`, `flavor_id`, optional `workspace_row_id`); later turns do not rewrite it.
- Persistence is **best-effort**: failures log a warning and never fail the chat HTTP response.
- Retention is **until operator delete**—no TTL or automatic pruning.

**Decisions**

| Topic | Decision |
|-------|----------|
| Dedup cache hits | Append a history turn when merge serves cached JSON; set `resolved_model` from cache metadata when present. |
| Failed / errored turns | Persist `role = 'error'` with user-visible text and optional `retry_user_text`. |
| Retention | Until user deletes (hard delete, cascade turns/retrievals). |
| API scope | Session `principal_id` from login token `tenant_id`. |
| Title | Nullable DB column; auto-generate from first user message (~60–80 chars); user-editable via PATCH; LLM titles deferred. |
| Flag | `flagged` boolean; toggle in UI; list filter `?flagged=1`. |
| Delete | User-initiated only; permanent. |

**Identity / auth / scoping**

- `conversations.principal_id` is the durable owner key (today: token `tenant_id` at login/persist time).
- UI session stores `principal_id` on `/api/ui/login` (`Record.TenantID` → session principal).
- Chat persistence in `handleV1Chat` uses the bearer token’s `sess.TenantID` on each request; login and chat must use tokens for the same principal for rows to align.
- Re-login after key rotation: new session id, same principal when `tenant_id` unchanged. Changing `tenant_id` on the credential is a new principal.

**Persistence**

- Tables: `conversations`, `conversation_turns`, `conversation_retrievals` (migration `000003_conversation_history.sql`).
- Per turn: user/assistant/error content, selected and resolved model ids, token counts, RAG hit snippets with `vector_point_id` / optional `content_sha256`.
- Live chat `X-Chimera-Conversation-Id` aligns with `conversations.conversation_id`.

## Interfaces

| Surface | Detail |
|---------|--------|
| `GET /api/ui/conversations` | List: `limit`, `offset`, optional `flagged=1`. Returns id, title, preview, flag, workspace fields, timestamps. |
| `GET /api/ui/conversations/{id}` | Full transcript for session principal. |
| `PATCH /api/ui/conversations/{id}` | Body `{ "title": "…" }` — trim, max length; empty rejected. |
| `POST /api/ui/conversations/{id}/flag` | Body `{ "flagged": true\|false }`. |
| `DELETE /api/ui/conversations/{id}` | 204; 404 when wrong principal. |
| Header | `X-Chimera-Conversation-Id` — client-held id; cleared on new chat. |
| Chat hook | Persistence runs once per completed client delivery (stream end, non-stream body, dedup short-circuit, or error response). |

All conversation routes require authenticated UI session JSON handlers (`RequireAuthJSON`).

## Code map

| Concern | Location |
|---------|----------|
| History panel UI | `chimera/chimera-gateway/internal/server/adminui/embed/embedui/chat/historyPanel.js`, `historyClient.js`, `styles/chat.css` |
| Chat shell | `embed/embedui/chat.html`, `chat/app.js`, `chat/state.js` |
| History API | `internal/server/adminui/api/conversations/` |
| Session / principal | `internal/server/adminui/session/session.go`, `handler/handler.go` |
| Store | `internal/operatorstore/conversations.go`, `store.go` |
| Title helper | `conversationtitle.FromFirstUserMessage` |
| Chat persistence hooks | `internal/server/server.go`, `virtualmodel_chat.go` |
| RAG metadata | `internal/rag/response_meta.go` |
| Migration | `migrations/chimera-gateway/operator/000003_conversation_history.sql` |
| Tests | `embed/embedui_test/chat_history_test.go`, operatorstore unit tests |

## Verification

```bash
go test ./chimera/chimera-gateway/internal/server/adminui/embed/embedui_test -run History
go test ./chimera/chimera-gateway/internal/operatorstore/... -run Conversation
```

Manual: sign in at `/ui`, send chat messages, open history panel, flag/rename/delete a thread, reload and confirm persistence; verify **New chat** does not remove saved rows.

## Out of scope and known gaps

- LLM-generated titles after first turn.
- Full-text search across conversations.
- Conversation tags.
- Explicit `principal_id` on `api-keys.yaml` rows decoupled from `tenant_id` (future stable principal registry).

## References

- Delivery plan: [`plans/operator-conversation-history.md`](../plans/operator-conversation-history.md)
- Shipped chat UI plan: [`plans/operator-chat-ui.md`](../plans/operator-chat-ui.md)
- Operator configuration: [`configuration.md`](../configuration.md)
