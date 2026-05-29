# Feature: Operator settings UI (cards and event log)

| Field | Value |
|-------|-------|
| **Doc kind** | `feature-record` |
| **Areas** | Gateway embed UI, operator logs, servicelogs, admin cards |
| **Status** | `current` |
| **Introduced** | Gateway v0.2 unified operator shell; v0.3 settings route rename |
| **Originated from** | [`plans/unified-logs-operator-shell.md`](../plans/unified-logs-operator-shell.md), [`plans/embedui-operator-settings-routes.md`](../plans/embedui-operator-settings-routes.md), [`plans/log-presentation-layer.md`](../plans/log-presentation-layer.md), [`plans/log-conversations.md`](../plans/log-conversations.md) |
| **Related features** | [Operator virtual models](operator-virtual-models.md), [Operator provider model availability](operator-provider-model-availability.md), [Indexer workspaces](indexer-workspaces.md), [Indexer health and operator logs](indexer-health-and-operator-logs.md), [Operator log message registry](operator-log-message-registry.md), [Operator left navigation ribbon](operator-left-navigation-ribbon.md) |
| **Depends on** | UI session auth, servicelogs ring buffer, `/api/ui/state`, `/api/ui/logs` |
| **Last updated** | See git history |

## At a glance

Configuration and observability live on **`/ui/settings`**: collapsible **summarized cards** (gateway overview, tokens/users, providers, virtual models, routing, indexer workspaces) above a live **event log** with Summary/Detailed toggle. The app shell at `/ui` opens settings in the iframe with `?embed=1`; legacy routes (`/ui/logs`, `/ui/desktop`, `/ui/panel`, deep-link `?focus=` params) are removed. Logs poll and SSE from the in-process ring buffer; the UI shapes raw JSON into operator headlines, conversation timelines, and per-service cards without changing wire format.

## Operator-visible behavior

- **Routes** — `/ui` (shell + ribbon), `/ui/chat`, `/ui/settings`, `/ui/settings/gallery`, `/ui/pwa`; login at `/ui/login`.
- **Settings embed mode** — `?embed=1` hides standalone chrome; shell posts `chimera-settings-activate` on load.
- **Summarized view (default)** — Cards for gateway version/health, usage metrics, API tokens, dynamic provider cards (Groq, Gemini, Ollama, …), virtual model cards, legacy global routing cards (where still wired), and indexer workspace cards fed from SQLite + structured indexer logs.
- **Event log** — Filter by app source and level; **Summary** shows registry-driven one-liners; **Detailed** shows parsed field grid. Conversation-scoped rows group routing, RAG, upstream relay, tools, and merge/dedup lifecycle (`conversation.*` slugs).
- **Provider cards** — Keys, model counts, availability summary, scoped log streams; **Configure** enters edit mode for per-model availability (see provider availability feature).
- **Virtual model cards** — CRUD, enable/disable, fallback/routing/tool-router editors, generate-from-catalog, scoped routing logs.
- **Indexer section** — Workspace CRUD, supervised YAML tuning, summarized progress cards (see indexer feature docs).
- **Component gallery** — `/ui/settings/gallery` for design-01 primitives (development aid).

## System behavior and contracts

**Invariants**

- HTML pages under `/ui/*`; JSON/SSE under `/api/ui/*` (unchanged API prefix).
- Log buffer entries remain `source + text + ts + seq`; presentation is client-side only.
- No prompt/response bodies in operator logs; redaction rules unchanged.
- Correlation dimensions: `request_id`, `conversation_id`, `principal_id`, `index_run_id`, `virtual_model_id`, service source tags.
- Summarized card rebuild preserves open/scroll state where possible; patch updates skip cards in active edit mode.

**Decisions**

| Topic | Decision |
|-------|----------|
| Primary surface | Single settings page replaces separate Main/Admin/Logs tabs |
| File naming | `settings.html`, `settings_app.js`, `/ui/assets/settings/**` (formerly logs.*) |
| Default post-login | `/ui` (not `/ui/logs`) |
| Log transport | SSE stream with tail replay + poll backfill |
| Conversation narrative | `conversation.*` slugs with per-turn indexing and tool round-trips (log-conversations plan) |
| View modes | `summarized` vs `detailed` in client state |
| Deep links | Former `?focus=`, `?card=`, `?conversation=` query params **ignored** |

**Persistence**

- Operator SQLite for tokens, virtual models, workspaces, provider availability (gateway-owned).
- Log ring buffer in-process (`servicelogs`); optional server-side event store for cross-restart history when configured.
- Metrics in separate `metrics.sqlite` surfaced via `/api/ui/metrics`.

## Interfaces

| Surface | Detail |
|---------|--------|
| `GET /ui/settings` | Settings page |
| `GET /ui/settings/gallery` | Component gallery |
| `GET /api/ui/state` | Gateway overview, providers, virtual models list for cards |
| `GET /api/ui/logs` | Poll log entries (`since_seq`, filters) |
| `GET /api/ui/logs/stream` | SSE (`replay=tail`, default tail 200) |
| `GET /api/ui/metrics` | Usage counts for card tables |
| `GET /api/ui/tokens` | Token/user cards |
| Virtual models, providers, indexer, routing | See related feature docs |
| Embed IPC | `{ type: "chimera-settings-activate" }` from shell |

## Code map

| Concern | Location |
|---------|------|
| Routes | `internal/server/adminui/embed/routes.go` |
| Settings shell | `embed/embedui/settings.html`, `settings_app.js`, `settings_entry.js` |
| Summarized feed | `embed/embedui/settings/app/summarizedFeed.js`, `summarizedDirtyRouting.js` |
| Card renderers | `embed/embedui/settings/render/cards/` — `adminVirtualModels.js`, `adminRouting.js`, provider cards, gateway overview |
| Derive / classify | `embed/embedui/settings/derive/` |
| Operator copy | `embed/embedui/settings/render/operatorMessage*.js`, generated `operator_copy.js` |
| Log API | `internal/server/adminui/api/logs/` |
| State API | `internal/server/adminui/api/state/build.go` |
| Ring buffer | `chimera/internal/servicelogs/store.go` |
| Tests | `embedui_test/settings_*_test.go`, `operator_message_test.go` |

## Verification

```bash
go test ./chimera/chimera-gateway/internal/server/adminui/embed/embedui_test -run 'Settings|Summarized|OperatorMessage'
go test ./chimera/internal/servicelogs/...
```

Manual: login → `/ui` → open **Settings** from ribbon; confirm cards hydrate from `/api/ui/state`; toggle Summary/Detailed on log panel; edit a provider model availability and confirm catalog change on chat model list.

## Out of scope and known gaps

- Full CSS sectioning of `settings.css` (logs-ui-maintainability Workstream C partial).
- Remote log shipping (Splunk, etc.) — ring buffer only.
- Legacy global routing YAML cards coexist with per-VM cards during migration; prefer virtual model cards for new config.
- `/ui/pwa` placeholder — not primary operator workflow.

## References

- Plans: [`unified-logs-operator-shell.md`](../plans/unified-logs-operator-shell.md), [`embedui-operator-settings-routes.md`](../plans/embedui-operator-settings-routes.md), [`log-presentation-layer.md`](../plans/log-presentation-layer.md), [`log-conversations.md`](../plans/log-conversations.md)
- Copy registry: [Operator log message registry](operator-log-message-registry.md)
- Configuration: [`configuration.md`](../configuration.md)
