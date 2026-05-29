# Feature: Operator left navigation ribbon

| Field | Value |
|-------|-------|
| **Doc kind** | `feature-record` |
| **Areas** | Gateway embed UI (app shell), chat iframe, conversation history panel |
| **Status** | `current` |
| **Introduced** | Gateway operator shell v0.2 train (embed UI refactor) |
| **Originated from** | Chat-only implementation; no delivery plan |
| **Related features** | [Operator conversation history](operator-conversation-history.md), [Operator chat UI](operator-chat-ui.md) |
| **Depends on** | UI session auth, `/ui` app shell iframe, conversation history API |
| **Last updated** | See git history |

## At a glance

The operator app at `/ui` shows a persistent left navigation ribbon beside the main content iframe. The ribbon has two states—**collapsed** (a narrow icon rail) and **expanded** (a panel with saved chat history). Navigation, history browsing, new chat, and settings all live in the ribbon; there is no separate top navigation bar. Chat and settings load in the iframe to the right while the ribbon stays visible. Expanded/collapsed state is remembered in browser `localStorage`.

## Operator-visible behavior

### Layout

- **`GET /ui`** renders the app shell (`index.html`): `[ ribbon | iframe ]`.
- Default iframe route is **`/ui/chat`**. Settings opens at **`/ui/settings?embed=1`** inside the same iframe.
- The former shell top bar (reload, copy chat, settings gear) is **removed**.

### Collapsed ribbon (~3 rem)

Always visible. Top to bottom:

1. **History toggle** — Material icon `side_navigation`. Tooltip: **Show chat history**. Expands the panel.
2. **New chat** — icon only (`edit_square`). Starts a new chat session in the iframe.
3. *(flex spacer)*
4. **Settings** — fixed at the **bottom-left** of the ribbon (`settings` icon). Opens settings in the iframe; when settings is already open, returns to the prior route.

### Expanded ribbon (~16 rem)

Animated width transition. Top to bottom:

1. **Brand row** — **Porcelain** title (left) and history toggle (upper-right). Tooltip on toggle: **Hide chat history**.
2. **New chat** — same vertical position as collapsed; icon plus **New chat** label to the right.
3. **All / Bookmarks** filter tabs (above the rule).
4. **Horizontal rule**.
5. **Chat history list** — scrolls independently; subtle bottom fade indicates more content above the footer.
6. **Settings** — fixed footer at **bottom-left**; icon plus **Settings** label when expanded.

### History list (expanded only)

- Rows show conversation title and relative last-updated time.
- **All** lists every saved thread; **Bookmarks** lists flagged threads only (UI label; API filter unchanged).
- Row action: **bookmark** toggle (`bookmark_star` icon). Rename and delete are **not** exposed in the ribbon UI.
- Clicking a row opens that conversation in the chat iframe (title and transcript appear in the main panel).
- Active row is highlighted; opening a row does **not** reset the list scroll position.

### Keyboard shortcuts

Shortcuts apply when focus is not in a text field, select, or content-editable region:

| Shortcut | Action |
|----------|--------|
| **Ctrl+B** | Toggle ribbon expanded / collapsed |
| **Ctrl+N** | New chat (navigates to chat if needed) |

### Typography and color

Ribbon text uses the same **design-01** tokens as `/ui/settings` (Hanken Grotesk via `--embed-font-ui`):

| Element | Settings reference |
|---------|-------------------|
| **Porcelain** | Section title (`.sg-op-section-title`) |
| Conversation titles | Card title (`.sum-title`) |
| All / Bookmarks, timestamps | Card subtitle (`.sum-sub`) |
| Nav controls | Primary / secondary embed text colors |

Font **sizes** in the ribbon are unchanged from the ribbon UX pass; only family, weight, letter-spacing, and foreground colors align with settings.

### New chat and settings

- **New chat** clears the in-memory chat session and assigns a new `conversation_id` on the next send. It does **not** delete persisted history rows (see [Operator conversation history](operator-conversation-history.md)).
- **Settings** remembers the iframe route before opening settings and restores it when settings is closed from the ribbon.

## System behavior and contracts

**Invariants**

- The ribbon is **always visible** on `/ui`; it is not part of the chat or settings iframe documents.
- The main iframe resizes horizontally when the ribbon expands; chat composer and viewport remain usable.
- History list data comes from the shell-mounted panel (`historyPanel.js` + `historyClient.js`), not from `chat.html`.
- Opening a saved conversation requires **`historyClient.js` in the chat iframe** so `app.js` can load transcripts via `GET /api/ui/conversations/{id}`.
- Shell → chat actions wait until the chat iframe route is `/ui/chat` and `ChimeraChat.App` is ready before `postMessage`.
- History list scroll position is preserved across row selection and list refresh (active row updates via CSS class, not full re-render).

**Decisions**

| Topic | Decision |
|-------|----------|
| Ribbon placement | App shell (`index.html`), not inside `chat.html`, so navigation persists across chat ↔ settings |
| Top bar | Removed; ribbon is sole chrome for navigation |
| History filters | Rendered in `#shell-ribbon-filters` above the rule, not inside the scrollable list |
| Settings footer | Only control in ribbon footer; pinned bottom-left with `margin-top: auto` |
| Bookmarks vs flag | UI says **Bookmarks**; API and store still use `flagged` / `?flagged=1` |
| Row actions | Bookmark toggle only in ribbon; rename/delete deferred from ribbon (title edit remains in chat title bar when a thread is open) |
| Expanded state persistence | `localStorage` key `chimera-ribbon-expanded` (`"1"` / `"0"`) |
| IPC | `postMessage` on same origin between shell and iframe (see Interfaces) |

**Persistence**

- Ribbon expanded/collapsed: browser `localStorage` only (per device/browser).
- Conversation rows: operator SQLite via conversation history feature (unchanged).

## Interfaces

| Surface | Detail |
|---------|--------|
| `GET /ui` | App shell with ribbon + iframe |
| `GET /ui/chat` | Chat page loaded in iframe |
| `GET /ui/settings?embed=1` | Settings page loaded in iframe |
| `GET /api/ui/conversations` | History list (shell panel); see conversation history feature |
| `GET /api/ui/conversations/{id}` | Transcript load (chat iframe via `historyClient.js`) |

**Shell → chat iframe** (`postMessage`, same origin):

| Message | Purpose |
|---------|---------|
| `{ type: "chimera-chat-action", action: "new" }` | New chat session |
| `{ type: "chimera-chat-action", action: "open", conversationId }` | Load saved thread |
| `{ type: "chimera-chat-action", action: "deleted", conversationId }` | Clear viewport if deleted thread was open (handler retained; delete UI not in ribbon) |

**Chat iframe → shell** (`postMessage`, same origin):

| Message | Purpose |
|---------|---------|
| `{ type: "chimera-chat-state", action: "refresh-history" }` | Reload history list after send/title save |
| `{ type: "chimera-chat-state", action: "set-active", conversationId }` | Highlight open thread in list |
| `{ type: "chimera-chat-state", action: "clear-active" }` | Clear list selection (new chat) |

**Settings iframe activation** (unchanged): shell posts `{ type: "chimera-settings-activate" }` on load when route is settings.

## Code map

| Concern | Location |
|---------|----------|
| App shell + ribbon DOM | `chimera/chimera-gateway/internal/server/adminui/embed/embedui/index.html` |
| Ribbon controller | `embed/embedui/shell/navRibbon.js` |
| Ribbon styles | `embed/embedui/styles/shell-ribbon.css` |
| History panel (embedded) | `embed/embedui/chat/historyPanel.js`, `historyClient.js` |
| History list styles (shared) | `embed/embedui/styles/chat.css` (ribbon overrides in `shell-ribbon.css`) |
| Chat iframe orchestrator | `embed/embedui/chat/app.js`, `chat.html` |
| Static assets route | `embed/routes.go` — `/ui/assets/shell/` |
| Tests | `embed/embedui_test/shell_nav_test.go`, `chat_history_test.go` |

## Verification

```bash
go test ./chimera/chimera-gateway/internal/server/adminui/embed/embedui_test -run 'Shell|ChatHTML'
```

Manual:

1. Open `/ui` — confirm no top bar; collapsed ribbon with toggle, new chat, and bottom settings.
2. Expand ribbon (**Ctrl+B** or toggle) — **Porcelain**, filters above rule, scrollable history, fixed settings footer.
3. Open a saved chat — title and messages appear in the main panel; list scroll unchanged.
4. **New chat** (**Ctrl+N**) — empty chat; list selection clears.
5. **Settings** — settings in iframe; ribbon stays; close returns to prior page.
6. Reload `/ui` — expanded state restored from `localStorage`.

## Out of scope and known gaps

- Rename/delete from the ribbon history list (bookmark-only row actions by design).
- **Copy chat** and **Reload** removed with the old top bar; copy remains per-message inside chat.
- Mobile overlay / drawer treatment for narrow viewports.
- `Cmd+B` / `Cmd+N` on macOS (shortcuts require **Ctrl** today).
- Ribbon does not appear on standalone `/ui/chat` or `/ui/settings` without the shell.

## References

- Related capability: [Operator conversation history](operator-conversation-history.md)
- Operator configuration: [`configuration.md`](../configuration.md)
- Design tokens: `embed/embedui/theme-tokens.css`, `styles/design-01.css`
