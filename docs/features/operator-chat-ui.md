# Feature: Operator chat UI

| Field | Value |
|-------|-------|
| **Doc kind** | `feature-record` |
| **Areas** | Gateway embed UI, chat/RAG metadata, virtual models |
| **Status** | `current` |
| **Introduced** | Gateway operator shell v0.2 train |
| **Originated from** | [`plans/operator-chat-ui.md`](../plans/operator-chat-ui.md) |
| **Related features** | [Operator left navigation ribbon](operator-left-navigation-ribbon.md), [Operator conversation history](operator-conversation-history.md), [Operator virtual models](operator-virtual-models.md), [Indexer workspaces](indexer-workspaces.md) |
| **Depends on** | [Operator UI session auth](operator-ui-session-auth.md), `GET /v1/models`, `POST /v1/chat/completions`, [Gateway RAG](gateway-rag-ingest-and-retrieval.md), indexer workspaces API |
| **Last updated** | See git history |

## At a glance

Operators exercise gateway chat from `/ui/chat` (iframe inside the app shell at `/ui`). The page streams assistant replies, lets them pick a virtual or upstream model and an optional indexer workspace for RAG scope, and shows per-turn metadata: resolved upstream model, expandable workspace retrieval snippets with syntax highlighting, and inline errors with **Retry**. Messages render with design-01 styling; assistant text uses a safe Markdown subset. Per-message **Copy** is available on all roles; full-thread Markdown export exists in code but is not exposed in the ribbon shell (see gaps).

## Operator-visible behavior

- **Layout** — Viewport of chronological message cards; Cursor-style composer (textarea above controls; model and workspace selectors on the bottom row; send on the right).
- **Model selector** — Populated from `GET /v1/models` (includes enabled virtual models). Changing model does not clear the thread.
- **Workspace selector** — Populated from `GET /api/ui/indexer/workspaces`; **Default** sends no scope headers; otherwise `X-Chimera-Project` / `X-Chimera-Flavor-Id` on chat requests. Switching workspace does not reset the thread.
- **Streaming** — Token/chunk updates while generating; smart scroll follows only when the viewport is already at the bottom.
- **Keyboard** — **Enter** sends, **Shift+Enter** newline, **Escape** blurs input, **↑** recalls prior user messages when the composer is empty.
- **Errors** — Failed turns render as error blocks with **Retry** (resends stored user text).
- **RAG snippets** — Expandable blocks under assistant messages; show source path, relevance score, and highlighted code or Markdown from `X-Chimera-RAG-Hits` (base64 JSON header).
- **Resolved model** — Message header shows upstream model id when the gateway resolved a virtual model to a concrete provider model.
- **Title bar** — Editable conversation title when a saved thread is open (persists via conversation history API).
- **New chat** — Clears in-memory messages and assigns a new `conversation_id`; does not delete SQLite history rows.
- **Shell integration** — Ribbon posts `chimera-chat-action` messages for **new** and **open**; chat posts `chimera-chat-state` to refresh or highlight history.

## System behavior and contracts

**Invariants**

- Chat modules stay separated: `gatewayClient.js`, `streamHandler.js`, `render/*`, `state.js`, `app.js` orchestrator.
- `X-Chimera-Conversation-Id` is client-held and sent on every chat request; aligns with conversation history persistence when turns complete.
- Workspace scope applies per request from the current selector; it does not retroactively change prior turns.
- RAG hit text in headers is base64-encoded JSON for UTF-8 safety; client decodes before render.
- No attachments, system prompts, tools UI, or multi-modal inputs in shipped chat embed.

**Decisions**

| Topic | Decision |
|-------|----------|
| Auth for chat API | First token from `GET /api/ui/tokens` as Bearer on `/v1/*` |
| Workspace key | `(project_id, flavor_id)` tuple; duplicate keys last-write-wins in selector map |
| Markdown | Safe subset in `render/markdown.js`; streaming uses partial render + tag balancing |
| Snippet language | Inferred from file extension when header omits hint |
| History vs live | Same renderer for loaded history and live stream |
| Copy scope | Per-message copies message text; error blocks copy error text |

**Identity / auth / scoping**

- Chat uses session-scoped UI token list; persistence uses session `principal_id` (see conversation history feature).

## Interfaces

| Surface | Detail |
|---------|--------|
| `GET /ui/chat` | Chat page (iframe) |
| `GET /api/ui/tokens` | Bearer token for `/v1/models` and chat |
| `GET /v1/models` | Model catalog (virtual + available upstream) |
| `GET /api/ui/indexer/workspaces` | Workspace list for selector |
| `POST /v1/chat/completions` | Chat (stream and non-stream) |
| Header | `X-Chimera-Conversation-Id` — thread id |
| Headers | `X-Chimera-Project`, `X-Chimera-Flavor-Id` — optional RAG scope |
| Response header | `X-Chimera-RAG-Hits` — base64 JSON snippet metadata |
| Shell IPC | `postMessage` `{ type: "chimera-chat-action", action: "new"|"open"|"copy-all"|"deleted", … }` |

## Code map

| Concern | Location |
|---------|------|
| Page shell | `embed/embedui/chat.html`, `styles/chat.css` |
| Orchestrator | `embed/embedui/chat/app.js` |
| State + transcript export | `embed/embedui/chat/state.js` |
| Gateway client | `embed/embedui/chat/gatewayClient.js` |
| Streaming | `embed/embedui/chat/streamHandler.js`, `scroll.js` |
| Rendering | `embed/embedui/chat/render/messages.js`, `input.js`, `markdown.js`, `snippet.js`, `titleBar.js` |
| History load | `embed/embedui/chat/historyClient.js` |
| Routes | `embed/routes.go` — `/ui/chat`, `/ui/assets/chat/` |
| Chat + persistence hooks | `internal/server/server.go`, `virtualmodel_chat.go` |
| Tests | `embedui_test/chat_history_test.go`, related HTML tests |

## Verification

```bash
go test ./chimera/chimera-gateway/internal/server/adminui/embed/embedui_test -run Chat
```

Manual: open `/ui` → chat iframe; send a message with a workspace selected; confirm streaming, snippet expand, resolved model label; trigger an error and **Retry**; **New chat** clears viewport but history panel retains rows.

## Out of scope and known gaps

- **Copy chat** shell action — `copy-all` handler exists in `app.js` but ribbon removed top-bar **Copy chat**; per-message copy only in UI today.
- File attachments, tools, images, system prompt editor.
- Line-number gutter on snippets — planned in [`plans/indexer-manifest-ingest.md`](../plans/indexer-manifest-ingest.md) (not shipped).
- Standalone `/ui/chat` without shell lacks ribbon navigation (by design).

## References

- Delivery plan: [`plans/operator-chat-ui.md`](../plans/operator-chat-ui.md)
- Shell: [Operator left navigation ribbon](operator-left-navigation-ribbon.md)
- Persistence: [Operator conversation history](operator-conversation-history.md)
- Configuration: [`configuration.md`](../configuration.md)
