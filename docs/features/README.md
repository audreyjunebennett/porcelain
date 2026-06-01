# Feature records — as-built system behavior

**Plans** ([`docs/plans/`](../plans/)) describe what to build and track delivery phases. **Feature records** in this directory describe **what shipped**: behavior, invariants, interfaces, and code entry points. Use them when starting a chat that touches an existing capability, or after implementation to capture decisions from plan + follow-up refinement.

**New record:** copy [`_template.md`](_template.md) into this folder, rename, fill in, delete the authoring notes, and add a row to the appropriate index below.

## Platform contracts

Durable contracts for new binaries, wrappers, and cross-cutting integration — not operator-product UI.

| Feature | Summary | Areas | Status |
|---------|---------|-------|--------|
| [Chimera wrapper binary contract](chimera-wrapper-binary-contract.md) | Shared `/healthz`, `/readyz`, lifecycle, exit codes, metrics for `chimera-*` wrappers | `internal/wrapper`, broker, vectorstore, gateway, indexer | `current` |
| [Structured operator log lines](structured-operator-log-lines.md) | JSON line schema, `*line` normalizers, lossless supervisor reorder | `wrapper/line`, servicelogs, settings UI | `current` |
| [Product naming contract](product-naming-contract.md) | Binary basenames, `CHIMERA__*` / `BROKER__*` env, `X-Chimera-*` headers | `internal/naming`, scripts | `current` |
| [Locus desktop ↔ supervisor](locus-desktop-supervisor.md) | Connect-first launch, readiness handshake, shutdown ownership | `locus-desktop`, `chimera-supervisor` | `partial` |
| [Operator UI session auth](operator-ui-session-auth.md) | UI session cookie, `principal_id`, `/api/ui/*` gate | Admin UI session store | `current` |
| [Operator SQLite store](operator-sqlite-store.md) | `operator.sqlite`, migrations, shared persistence for UI features | `internal/operatorstore` | `current` |
| [Operator UI filesystem dev mode](operator-ui-filesystem-dev-mode.md) | `CHIMERA_ADMINUI_ROOT` serves embed UI from disk on loopback | Gateway embed assets | `current` |
| [Gateway chat routing pipeline](gateway-chat-routing-pipeline.md) | Tool router, RAG inject, policy pick, fallback loop; extensibility target for routers | Gateway chat path | `current` |

## Operator features

| Feature | Summary | Areas | Status |
|---------|---------|-------|--------|
| [Gateway RAG ingest and retrieval](gateway-rag-ingest-and-retrieval.md) | Server-side chunk/embed/upsert; chat retrieval + snippet headers | Gateway RAG, vector store, ingest APIs | `current` |
| [Operator bootstrap and API tokens](operator-bootstrap-and-api-tokens.md) | `/ui/setup`, `api-keys.yaml`, token CRUD API | Bootstrap mode, settings users card | `current` |
| [Workspace file indexer](indexer.md) | Watches roots, sends files to gateway for embed; supervised child | `chimera-indexer`, gateway RAG, supervisor | `current` |
| [Indexer workspaces](indexer-workspaces.md) | Operator SQLite workspace CRUD; API-driven watch roots; DB-first settings cards | Operator SQLite, settings UI, indexer poll | `current` |
| [Indexer ingest pipeline](indexer-ingest-pipeline.md) | Scan/fan-out queue, fair-share bulk, priority watch ingest, JSON sync-state skips | `chimera-indexer` queue, gateway ingest | `current` |
| [Indexer health and operator logs](indexer-health-and-operator-logs.md) | Embed-aware health, ingest gate, structured JSON logs, quiet INFO + pinning | Gateway health API, servicelogs, settings UI | `current` |
| [Operator left navigation ribbon](operator-left-navigation-ribbon.md) | Persistent collapsed/expanded ribbon on `/ui`; history, new chat, settings | Gateway embed UI (app shell) | `current` |
| [Operator workspace search](operator-workspace-search.md) | Workspace-scoped vector search page and API; threshold control, hit excerpts | Gateway embed UI, RAG retrieval | `partial` |
| [Operator chat UI](operator-chat-ui.md) | Streaming chat, model/workspace selectors, RAG snippets, Markdown render | Gateway embed UI, chat/RAG | `current` |
| [Operator conversation history](operator-conversation-history.md) | Durable chat threads in operator SQLite; history panel with title, flag, delete | Gateway embed UI, operator SQLite, session | `current` |
| [Operator settings UI](operator-settings-ui.md) | `/ui/settings` cards + event log; unified admin/observability surface | Gateway embed UI, servicelogs | `current` |
| [Operator embed UI mobile layout](operator-embed-ui-mobile-layout.md) | Phone-width settings cards, scoped event log, VM toggles; site-wide mobile baseline; gallery fixtures | Gateway embed UI | `active` |
| [Operator virtual models](operator-virtual-models.md) | Per-VM routing stacks in SQLite; catalog + chat resolution | Gateway runtime, operator SQLite, settings UI | `current` |
| [Operator provider model availability](operator-provider-model-availability.md) | Tenant-scoped upstream model enable/disable; catalog filter | Operator SQLite, settings provider cards | `current` |
| [Operator log message registry](operator-log-message-registry.md) | Canonical log slugs + operator copy in YAML; generated JS/Go constants | `internal/operatorcopy`, settings UI | `current` |
| [Context window admission](context-window-admission.md) | Pre-upstream context/body limits; retriable `request_too_large` fallback | Chat routing, `providerlimits` | `partial` |
