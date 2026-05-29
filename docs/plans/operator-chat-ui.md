# Plan: Operator chat UI (gateway embed)

| Field | Value |
|-------|-------|
| **Doc kind** | `feature-plan` |
| **Owners / areas** | Gateway embed UI, gateway chat/RAG metadata |
| **Status** | `shipped` |
| **Targets** | Gateway operator shell (`/ui`), chat route (`/ui/chat`) |
| **Last updated** | See git history |
| **Supersedes / superseded by** | None |

## At a glance

Operators need a minimal, polished chat surface inside the Chimera gateway UI: streaming assistant replies, model and workspace selection, per-message copy, session-local conversation state, and visible metadata when a virtual model resolves to an upstream provider model or when retrieval injects workspace snippets. The chat page uses the design-01 theme, lives in the operator shell with page-specific top-bar actions, and exports the full conversation—including snippets—as Markdown via **Copy chat**.

| Phase | Outcome | Status |
|-------|---------|--------|
| [Phase 1 — Core chat surface](#phase-1--core-chat-surface) | Streaming chat with composer, selectors, session state, errors, and modular client architecture | `done` |
| [Phase 2 — Presentation and shell integration](#phase-2--presentation-and-shell-integration) | design-01 styling, Cursor-style composer, shell menu bar, markdown rendering, settings copy icon | `done` |
| [Phase 3 — Model and workspace snippet metadata](#phase-3--model-and-workspace-snippet-metadata) | Resolved model in message header; expandable workspace snippets with scores, syntax highlighting, and safe UTF-8 transport | `done` |
| [Phase 4 — Copy chat export](#phase-4--copy-chat-export) | **Copy chat** produces a full Markdown transcript including models and snippet bodies | `done` |

---

## Background

**Problem.** The operator UI had settings, logs, and workspace management but no first-class way to exercise gateway chat with the same models, workspaces, and retrieval behavior that production clients use. Operators could not see streaming responses, which upstream model answered a virtual-model request, or which indexed snippets influenced a turn.

**Scope.** Browser embed UI only (`/ui` shell + `/ui/chat` iframe). No file attachments, system prompts, tools, images, or multi-modal inputs in this delivery. Conversation history is held in memory for the session; persistence beyond **New chat** is out of scope.

**Architecture constraint.** Chat rendering, input/composer behavior, streaming handling, and gateway API access must remain separate modules so later features (attachments, tool calls, saved threads) can attach without rewriting the page.

**Related docs:** [`internal-embedding-provider-exploration.md`](internal-embedding-provider-exploration.md) (embedding/RAG context), [`desktop-ui.md`](desktop-ui.md) (Locus desktop shell).

---

## Phase 1 — Core chat surface

**Goal.** Operators can hold a streaming conversation against the gateway with model and workspace selection, smart scroll, keyboard shortcuts, per-message copy, and inline error recovery—without blocking the UI during generation.

**Deliverables**

- Chat page shell and assets under `chimera/chimera-gateway/internal/server/adminui/embed/embedui/chat/`:
  - `chat.html`, `styles/chat.css`
  - Modules: `state.js`, `gatewayClient.js`, `streamHandler.js`, `scroll.js`, `render/messages.js`, `render/input.js`, `render/markdown.js`, `app.js`
- Primary viewport: chronological user and assistant message blocks with clear role separation.
- Bottom composer: text area above controls; **Enter** sends, **Shift+Enter** newline; auto-resize up to a max height then internal scroll; **Escape** blurs input; **↑** recalls prior user messages when the field is empty (terminal-style history).
- Model selector: populated from gateway (`GET /api/ui/tokens` + `GET /v1/models`); changing model does not clear the thread.
- Workspace selector: populated from `GET /api/ui/indexer/workspaces`; selection sent as `X-Chimera-Project` / `X-Chimera-Flavor-Id` on chat requests; includes a **Default** option; switching does not reset the thread.
- Streaming: non-blocking token/chunk updates via gateway streaming protocol; smart scroll pins to bottom only when the operator is already at the bottom.
- Session state: in-memory message list, conversation id from gateway headers, **New chat** clears messages and conversation id while keeping model/workspace selection.
- Per-message **Copy** on user, assistant, and error blocks (message text only).
- Errors: inline error blocks in the transcript with **Retry**; failures do not require reload.
- Routes and embed wiring: default operator shell route `/ui/chat`; static asset routes for `/ui/assets/chat/`.

**Acceptance**

- Operator can send a message, see it appear immediately, and watch the assistant reply stream in.
- Model and workspace selections affect the next request; **New chat** resets the thread only.
- Scrolling up stops auto-follow; returning to the bottom resumes follow.
- A failed request shows an error row and **Retry** resends the same user text.
- Code layout separates render, input, stream, and gateway client concerns.

**Status:** `done`

---

## Phase 2 — Presentation and shell integration

**Goal.** The chat page matches operator settings visually (design-01) and follows a Cursor-like composer layout with shell-level actions on the chat menu bar.

**Deliverables**

- **design-01** theme: Hanken Grotesk, theme tokens, card-style message blocks with hover raise (glaze/`sum-card`-like treatment).
- Composer layout (Cursor-style):
  - Message input on top.
  - Bottom row: model selector bottom-left, workspace selector to its right, send control on the right.
- Shell top bar (`index.html`):
  - On **chat page only**: left menu with **New chat** and **Copy chat** (formerly “Copy all”).
  - On **settings page**: empty settings menu slot (reserved for future actions).
  - **Refresh** and **Settings/Close** on all menu bars.
  - Shell → iframe `postMessage` for **New chat**; **Copy chat** handled in shell (see Phase 4).
- Assistant responses rendered as **Markdown** (safe subset).
- Per-message copy button uses the same SVG icon as settings event-log copy (`sum-evlog__copy-btn`).

**Acceptance**

- Chat visually aligns with settings (colors, fonts, borders, card hover).
- Shell menu shows chat actions only on `/ui/chat`; settings bar is present but empty on settings.
- Assistant prose renders headings, lists, code fences, and links where allowed by the markdown subset.

**Status:** `done`

---

## Phase 3 — Model and workspace snippet metadata

**Goal.** When a virtual model fans out to an upstream provider model or RAG returns hits, operators see that metadata without overshadowing the main reply—expandable snippet rows with scores and readable code.

**Deliverables**

- Gateway response metadata (before streaming body):
  - `X-Chimera-Upstream-Model` — resolved upstream model id.
  - `X-Chimera-RAG-Hits` — JSON array of hit summaries; **base64-encoded** on the wire so UTF-8 snippet text (em dashes, etc.) is not corrupted in HTTP headers.
- Go: `internal/rag/response_meta.go`, wiring in virtual-model chat path; tests for unicode, newlines, base64 headers.
- **Resolved model display:** `(Model: provider/model)` next to the **Assistant** title (uses upstream model when known, otherwise selected model while streaming). Removed from footer after layout iteration.
- **Workspace Snippets** (count) expander in the assistant footer:
  - Left-justified toggle; snippet list on a full-width row below (not in a column beside the model label).
  - Each hit: file path left, **score** right-aligned; snippet body expandable per row.
  - Language **not** shown in the UI; still used internally for syntax highlighting (`render/snippet.js` infers from path + server hint).
  - Snippet text preserves newlines; lightweight syntax highlighting / markdown for `.md` snippets.
- Footer copy: label **Workspace Snippets (N)** (not “Retrieved context”).

**Acceptance**

- Virtual-model turns show upstream model beside **Assistant** when it differs from the selected virtual id (and selected/upstream id while streaming).
- When RAG hits exist, operator can expand **Workspace Snippets**, see paths and numeric scores, and read highlighted snippet bodies without mojibake in typical UTF-8 content.
- Expanded snippet cards use the full width of the message card content area.

**Status:** `done`

---

## Phase 4 — Copy chat export

**Goal.** **Copy chat** copies the entire session transcript as Markdown, including resolved models and all workspace snippet text—not just visible message bodies.

**Deliverables**

- Shell button label **Copy chat** (`index.html`); tooltip describes Markdown export.
- `state.js` — `transcriptText()` builds Markdown:
  - `## User` / `## Assistant` / `## Error` sections separated by `---`.
  - Assistant blocks include `**Model:** \`provider/model\``.
  - `### Workspace Snippets` subsection with `#### path (score: 0.xxx)` headings and fenced code blocks (language tag from path inference when available).
- Shell copies from the iframe synchronously on button click (`readChatTranscript()` → parent `navigator.clipboard` / `execCommand` fallback) so WebView2/user-gesture rules are satisfied—**not** via async `postMessage` into the iframe for clipboard writes.
- Chat app exposes `ChimeraChat.App.getTranscriptText()` for the shell reader.

**Acceptance**

- With a multi-turn thread that includes RAG hits, **Copy chat** paste contains user text, assistant markdown, model lines, and every snippet path/score/body in code fences.
- Copy succeeds from the shell top bar in the desktop WebView / browser without silent clipboard failure.

**Status:** `done`

---

## Open questions

None for this delivery. Future work (out of scope here) may include persisted conversation history, attachments, system prompts, tool UI, and setup-wizard chat step wiring.

---

## References

- UI: `chimera/chimera-gateway/internal/server/adminui/embed/embedui/chat.html`, `chat/`, `styles/chat.css`, `index.html`
- Gateway metadata: `chimera/chimera-gateway/internal/rag/response_meta.go`, `internal/naming/contracts.go` (header names)
- Embed routes: `chimera/chimera-gateway/internal/server/adminui/embed/routes.go`, `embed/assets.go`
- Tests: `chimera/chimera-gateway/internal/rag/response_meta_test.go`, gateway UI shell tests
