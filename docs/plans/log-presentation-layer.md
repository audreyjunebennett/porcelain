# Plan: Log presentation layer (chimera-gateway)

| Field | Value |
|-------|-------|
| **Doc kind** | `feature-plan` |
| **Owners / areas** | Gateway, embed UI, operator logs |
| **Status** | `done` |
| **Targets** | Operator log presentation layer |
| **Last updated** | See git history |
| **Supersedes / superseded by** | None |
| **As-built** | [`docs/features/operator-log-message-registry.md`](../features/operator-log-message-registry.md), [`operator-settings-ui.md`](../features/operator-settings-ui.md) |

**Behavioral source of truth:** the [log message registry](../features/operator-log-message-registry.md) and [settings UI](../features/operator-settings-ui.md) feature records describe as-built behavior; this plan is delivery history.

## At a glance

Make the operator log view tell a story instead of dumping JSON. Each user's chat reads as a thread — request, routing, retrieval, answer — and each subsystem (gateway, BiFrost, Qdrant, indexer) shows a running status card. Raw lines stay one click away whenever you need them.

| Phase | Outcome | Status |
|-------|---------|--------|
| [Phase A — Shape & headline](#phase-a--shape-detection--headline-ui-only) | Summary vs Detailed toggle; HTTP and indexer headlines | `done` |
| [Phase B — Correlation IDs](#phase-b--correlation-ids-gateway--clients) | Stable `request_id`, `conversation_id`, `index_run_id` across logs | `done` |
| [Phase C — Indexer run narrative](#phase-c--indexer-run-narrative) | One card per indexer run with progress | `done` |
| [Phase D — Conversations & subsystem cards](#phase-d--conversation--bifrost-rollup--principal-panel) | Per-user threads and per-service health cards | `done` |
| [Phase E — Server-side event store](#phase-e--optional-server-side-event-store) | Optional persistence for cross-restart history and search | `done` |

---

This document describes a **log presentation layer**: how operator-facing logs stay **verbatim on the wire** (or in the ring buffer) while the **UI interprets, shapes, groups, and summarizes** them so troubleshooting is faster and less noisy. It also defines **how an implementation agent should work with you**—exploration spikes, checkpoints, and optional experiments—so the design evolves deliberately rather than as a one-shot UI tweak.

**Intent:** Readers should grasp **what** happened (nouns + verbs), **where** (path, project, tenant), and **outcome** (success vs error, latency) **at a glance**, with optional **threads** (indexer runs, chat completions, BiFrost round-trips) and **rollup** of high-volume lines. A **dedicated, user-centric view** (see [§3](#3-user-and-conversation-centric-view)) groups traffic by **gateway principal** (the authenticated API key / token) so each **conversation** reads as a **story**: routing and fallbacks, RAG (Qdrant), upstream (BiFrost), and the response path back to the client—**concise by default**, expandable for depth, with **error state** visible on the parent conversation. A parallel **subsystem view** (see [§4](#4-subsystem-health-and-service-narratives)) tells the **same underlying events** from the **service perspective** (gateway, **Qdrant**, **BiFrost**, **indexer**): health, uptime, last activity, **metrics**, and **errors**—also as a **story** with **one-line summaries** that **expand** to full detail. **Unified tagging** ([§5](#5-unified-event-tagging)) ties user, conversation, index run, and service dimensions to **every** class of message so both views filter and correlate consistently. A **detailed / developer** view must remain available so nothing is lost for deep debugging.

**Related surfaces today:**

- Log buffer and API: [`internal/servicelogs/store.go`](../internal/servicelogs/store.go), [`internal/server/ui_logs.go`](../internal/server/ui_logs.go)
- Embedded logs UI: [`internal/server/embedui/logs.html`](../internal/server/embedui/logs.html) (JSON / `key=value` parse, flat key/value grid)
- HTTP access logging: [`internal/server/server.go`](../internal/server/server.go) (`loggingMiddleware`, `http response` fields)
- Security posture for logs: [`SECURITY.md`](../SECURITY.md)

---

## 1. Problem statement

### 1.1 Current behavior

- Each log line is stored as **`source` + `text` + `ts` + `seq`**; the browser parses structured JSON (or simple `key=value`) into a **uniform** details table.
- Important dimensions (**HTTP path**, **status class**, **duration**) compete visually with every other field; there is no **shape** or **headline** row.
- There is no **correlation id** in the UI model, so related lines (same chat, same indexer run, same request) cannot be **collapsed**, **threaded**, or **rolled up** without heuristics.
- There is no **principal** (token identity) dimension in the UI, so operators cannot see **per-user conversation** narratives—only a flat chronological stream.

### 1.2 Target behavior

| Concern | Target |
|--------|--------|
| Scan | One row or card communicates **event kind**, **primary noun**, **outcome**, and **key dimensions** without opening details. |
| HTTP | **Path** and **status** are visually primary; success vs error is obvious (color + label + code). |
| Indexer | **Per-project / per-run** narrative: start → progress (counts or %) → done; raw lines available on expand. |
| Chat / upstream | **Conversation-scoped** card where follow-up requests and BiFrost-related lines can roll up when desired. |
| **By user (principal)** | **Dedicated panel**: each API key (see [§3](#3-user-and-conversation-centric-view)) lists **conversations**; each conversation shows a **header**, **timeline**, **service summaries** (Qdrant, BiFrost, fallbacks), **errors**, and **safe context growth** metrics—compact first, **folder-style** expand for older events. |
| **Subsystems** | **Dedicated panel** (see [§4](#4-subsystem-health-and-service-narratives)): **gateway**, **Qdrant**, **BiFrost**, **indexer** each as a **running story**—**start time**, **last message**, **uptime**, **metrics**, **captured errors**—with the same **summary line → expand** pattern as the user view. Content is **overlapping / redundant** with lines that appear under conversations (same events, operator-first framing). |
| **Tagging** | Structured logs carry **dimensions** for **principal**, **conversation**, **index** / `index_run_id`, **service** (`gateway` / `qdrant` / `bifrost` / `indexer`), and **system** context so both panels and **Detailed** grid stay aligned ([§5](#5-unified-event-tagging)). |
| Safety | No prompt/response bodies; redaction rules unchanged; correlation ids are metadata only. |

---

## 2. Design principles

1. **Presentation ≠ storage (initially).** Prefer interpreting existing structured logs in the UI before adding new databases or transports. Add storage or indexed events only when grouping, search, or retention demands it.
2. **Two modes, one stream:** **Summary** (opinionated layout, optional collapse) and **Detailed** (today’s grid or superset). Toggle must be obvious; state stays in-memory for the session.
3. **Shape over syntax:** Classify lines into a small **taxonomy** (`http.access`, `chat.*`, `indexer.*`, `bifrost.*`, `generic`, …) from fields and `msg` patterns, with a safe fallback. Canonical slugs may declare an optional `shape` in [`internal/operatorcopy/messages.yaml`](../../internal/operatorcopy/messages.yaml); the logs UI uses `ChimeraLogs.OperatorCopy.inferShapeForFlat` first, then legacy prefix rules in `logs_app.js`.
4. **Correlation is explicit:** Threading and rollup require stable **ids** attached in structured logs (`request_id`, `conversation_id`, `index_run_id`, …). Heuristic grouping is a stopgap, not the end state.
5. **Progressive disclosure:** Headline + badge + metrics first; full key/value (or raw JSON) behind “Details”.
6. **Principal-first narrative:** When viewing **Conversations**, the spine of the story is **what the user’s token did** through the gateway: one `conversation_id` (or equivalent) should tie together chat, RAG, upstream, and response-path logs. **Subprocess** lines (Qdrant, BiFrost) may join that story via **shared request/conversation id** logged on the gateway side at call time, or via summarized gateway-only events if raw child logs cannot be correlated.
7. **Two stories, one event stream:** **User/conversation** and **subsystem** panels are **redundant presentations** of the same underlying log lines (plus service-local metrics). Implementations should **not** duplicate storage; the UI **projects** the same tagged events into either narrative. Operators choose the lens; **Detailed** remains the lossless view.
8. **Tags on every narrative line:** Where feasible, each structured log line includes **facets** ([§5](#5-unified-event-tagging)) so filters like “this conversation” and “this indexer run” and “Qdrant” intersect cleanly.

---

## 3. User and conversation-centric view

This section specifies the **operator UI** experience you described: logs organized around **who** (as defined by the **gateway API key** / validated token), and **each conversation** as a **coherent narrative** across components—not only a flat table of lines.

### 3.1 Principal (user) identity

- **Definition:** The **authenticated principal** is whoever the gateway associates with the `Authorization: Bearer` token on `/v1/*` (same model as chat). For grouping and display:
  - Use a **stable internal id** (e.g. token index, configured **label** from [`tokens.yaml`](../config/tokens.yaml) if present, or a **short fingerprint** such as hash prefix).
  - **Never** show the full API key or raw token in the UI or in log fields intended for this view ([`SECURITY.md`](../SECURITY.md)).
- **Layout:** A **dedicated section** (tab, sidebar, or top-level panel) **“By user”** / **“Conversations”** lists principals; expanding a principal shows **recent conversations** (sessions) for that principal.

### 3.2 Conversation session model (the story spine)

Each **conversation** (or **session**) is a container for everything that supports **one user-visible chat arc**:

| Story beat (illustrative) | Examples of underlying events |
|---------------------------|-------------------------------|
| User request in | Inbound `/v1/chat/completions` (or related) with `conversation_id` / `request_id`. |
| Routing / fallback | Logs indicating **model routing**, **provider fallback**, retries, or degraded path (exact `msg` / shape TBD in implementation). |
| RAG / retrieval | Gateway-side **Qdrant** calls (or summarized “retrieval” events); count + latency rollup. |
| Upstream LLM | **BiFrost** (or other upstream) request/response **metadata**—not bodies. |
| User response out | Successful or failed **response** to client; status and duration. |

**Ordering:** Timeline is **chronological** within the conversation. **Rollup** groups consecutive low-level lines of the same shape (e.g. many HTTP polls) into one summarized row where appropriate.

### 3.3 Visual design (concise, expandable, folder-style history)

- **Conversation header / summary** — One line (or card title): e.g. model id, first line of intent **if already logged safely** as a short label, or “Chat completion” + `conversation_id` short id; **error badge** if anything in the session failed.
- **Session metadata** — **Started at**, **ended at** (or **Active**), **elapsed time** for the session window.
- **Auto-compact timeline** — By default show only the **last N** events (e.g. 3–5), configurable. Older events live under a control such as **“Earlier events (M)”** (disclosure / folder / accordion)—still loaded, not deleted.
- **Per-event rows** — Each row is **one story beat** (or rolled-up group); **click to expand** for full structured fields / link to **Detailed** row or raw JSON.
- **Service strip** — Compact summary chips or a single sub-row: e.g. **Qdrant: 2 calls · 95 ms**, **BiFrost: 1 round-trip · 1.2 s**, **Fallback: 1×**—derived from classified logs, not guessed.
- **Errors** — Any child event with **error** / **5xx** / **fatal** shape tints the **conversation** header (color) and shows an **error count** (e.g. badge **“2 errors”**). Expand any row to see which step failed first.

### 3.4 Context growth (safe visibility)

You want to see **how context evolves** as the conversation continues **without** turning logs into a transcript.

- **Allowed:** Metrics logged or derived on the gateway, such as **turn index**, **approximate context size** (chars/tokens **estimate**), **RAG chunk count attached**, **tool round count**—all **metadata**, aligned with product security rules.
- **Not allowed by default:** Full message text, system prompts, or raw completion bodies in this UI (unless a separate, explicitly gated **debug** mode and policy exist—out of scope for the default presentation layer).
- **UI:** Optional **sparkline**, stepped list, or table column “context Δ” showing **growth between turns** when the backend exposes comparable numbers on successive events.

### 3.5 Correlation requirements (backend)

The conversation view **depends** on explicit ids and consistent shapes:

- **`conversation_id`:** Propagated from the chat handler through **RAG**, **upstream**, and **response** logging (see [Phase B](#phase-b--correlation-ids-gateway--clients)).
- **`request_id`:** Middleware-scoped id for every inbound request; child operations should log **parent** `request_id` or `conversation_id`.
- **Qdrant / BiFrost:** Prefer **gateway-emitted** summary lines (“qdrant.query”, “upstream.completion”) that already carry `conversation_id`. Relying on raw **qdrant** / **bifrost** `servicelogs` **source** lines may require **id injection** in the gateway before the call, or accepting that some beats are **gateway-only** summaries.

### 3.6 Relation to other phases

- **Phase A–B** are prerequisites for a usable conversation panel (shape + ids).
- **Phase D** implements **rollup** and BiFrost alignment; the **principal panel** extends D with **grouping by token** and the **folder-style** timeline UX.
- **Phase E** becomes more likely if the browser must query **“all events for conversation X”** across large buffers—see trigger conditions there.

---

## 4. Subsystem health and service narratives

This section specifies a **second major panel** in the presentation layer: **health and events of the underlying subsystems**, using the **same interaction language** as [§3](#3-user-and-conversation-centric-view) (story, **one-line summary** per beat, **expand** for full detail, optional **folder** for older events)—but centered on **services**, not on a single user’s chat.

### 4.1 Services in scope

| Service | Role in the UI story |
|---------|----------------------|
| **Gateway** | The `chimera` process itself: listen address, config loads, auth outcomes, chat/RAG/orchestration **without** duplicating every HTTP row—**headline** events plus errors. |
| **Qdrant** | Vector store (supervised or external): readiness, collection health, query/update **summaries** when logged; errors and latency **rollup**. |
| **BiFrost** | Upstream LLM hub: startup, provider health, request/response **metadata** (not bodies), errors, fallbacks **as seen by the gateway or BiFrost logs**. |
| **Indexer** | `chimera-indexer` (standalone or supervised): run lifecycle, progress, backoff, ingest outcomes—aligned with [`indexer.plan.md`](indexer.plan.md) |

### 4.2 Service card model (what each row shows)

For **each** subsystem, the UI maintains a **living card** (or column) that answers:

- **Started at** — First observed activity or process start (from logs or supervisor attach time).
- **Last message** — Short text derived from the **most recent** log line for that `servicelogs` **source** / shape (e.g. last headline or `msg`).
- **Running for** — **Uptime** or **time since last activity** (implementation choice: both may be useful; show **elapsed since start** for long-lived children).
- **Metrics** — Counters and gauges **inferred or explicit** in structured logs: e.g. request counts, error rate window, queue depth (indexer), health probe status—**only what the service actually logs**; no invented numbers.
- **Errors** — Distinct **error** / **warn** / **failed probe** events **rolled up** with count; tint the card when **errors > 0**; expand to the **timeline** of failures.

### 4.3 Timeline inside each service (story of events)

- **Chronological event list** under each card: each item is a **single summarized line** (shape + outcome + key dimensions).
- **Expand** reveals the **full** structured fields / raw JSON, same as conversation rows.
- **Auto-compact** optional: show last **N** events; older under **“Earlier events”**—mirrors §3.3.

### 4.4 Redundancy with the user/conversation panel

- The **same underlying log lines** often appear in **both** places: e.g. a **Qdrant** health line is part of the **subsystem** story **and** may be **referenced** (or duplicated in summary form) under a **conversation** that triggered a query.
- **Requirement:** treat this as **one event stream, two projections**. The UI may **link** (“open in conversation”) when **tags** ([§5](#5-unified-event-tagging)) share `conversation_id` or `request_id`.
- **Subsystem-first** view helps when **no user** is involved (startup, indexer-only, BiFrost idle) or when debugging **infra** without picking a principal.

### 4.5 Data sources today

- **`servicelogs` `source`:** `gateway`, `bifrost`, `qdrant`, and (when implemented) `indexer`—see `cmd/chimera/serve.go` and [`servicelogs`](../internal/servicelogs/store.go).
- **Gateway `slog`:** may emit additional **shapes** that are not a separate child process but belong on the **gateway** card.

---

## 5. Unified event tagging

All **presentation modes** (Detailed grid, Summary, **Conversations**, **Subsystems**) rely on a **common tagging model** attached to structured logs (or inferred with gaps documented).

### 5.1 Tag dimensions (facets)

Implementations should prefer **explicit** fields on each relevant `slog` record (names illustrative—finalize in code + fixtures):

| Tag | Meaning |
|-----|---------|
| `principal_id` / `tenant` | Authenticated API key identity (never the raw secret)—who the event **belongs to** when applicable. |
| `conversation_id` | Chat session thread; empty when N/A (e.g. pure indexer or startup). |
| `request_id` | HTTP request correlation from middleware. |
| `index_run_id` | Indexer run / batch correlation; empty when N/A. |
| `service` | `gateway` \| `qdrant` \| `bifrost` \| `indexer` \| `system` (cross-cutting: supervisor, OS). |
| `shape` / **`msg` family** | Presentation classification (`http.access`, `chat.completion`, `rag.query`, `indexer.run.progress`, …). |

**Optional:** `project_id`, `flavor_id` where already used for RAG scope.

### 5.2 Rules

- **Not every line has every tag.** **Startup** lines may be `service=gateway` only; **ingest** lines may carry `index_run_id` + `principal_id`; **chat** lines should carry `conversation_id` + `principal_id` and propagate to **RAG** and **upstream** summaries.
- **Subprocess** stdout (Qdrant/BiFrost) may lack tags until the **supervisor or gateway** prefixes structured lines or the UI **infers** `service` from `source`.
- **Goal:** filters **“show me Qdrant errors for this conversation”** or **“indexer events for this principal”** are **well-defined** when tags are present.

### 5.3 Relation to phases

- **[Phase B](#phase-b--correlation-ids-gateway--clients)** introduces ids; tagging extends Phase B with `service` and stable `principal_id` representation.
- **Subsystem panel ([§4](#4-subsystem-health-and-service-narratives))** primarily filters/group by `service` + time; **conversation panel** by `principal_id` + `conversation_id`.

---

## 6. Architecture

```
┌─────────────────────────────────────────────────────────────┐
│  Writers (gateway slog, subprocess stdout, indexer, …)       │
│  → servicelogs.Entry { source, text, ts, seq }             │
└───────────────────────────┬─────────────────────────────────┘
                            │
                            ▼
┌─────────────────────────────────────────────────────────────┐
│  Transport (unchanged at first): GET /api/ui/logs, SSE       │
└───────────────────────────┬─────────────────────────────────┘
                            │
                            ▼
┌─────────────────────────────────────────────────────────────┐
│  Presentation layer (logs.html or extracted JS module)      │
│  • Parse → flatten (existing)                                 │
│  • Classify → shape + headline fields                         │
│  • Correlate → optional in-memory indexes by id + principal     │
│  • Render → Detailed OR Summary OR Conversations OR Subsystems   │
└─────────────────────────────────────────────────────────────┘
```

**Optional later:** a **server-side projection** (e.g. normalized events in SQLite) if the client cannot hold enough history or filtering becomes too heavy. This plan treats that as **Phase E** (escape hatch), not the default path.

---

## 7. Phased implementation

### Phase A — Shape detection + headline (UI-only)

**Deliverables**

- Toolbar: **View: Detailed | Summary** (persist choice).
- For each line, compute `shape` from parsed fields (e.g. `msg == "http response"` or presence of `method` + `path` + `statusCode` → `http.access`).
- **Summary row:** pill for status family (2xx / 4xx / 5xx), method, **emphasized path**, duration; shape badge; secondary line for redacted auth if present.
- **Details:** existing props table behind disclosure or below fold in Summary mode.

**Exit criteria:** An operator can answer “which route failed and with what code?” without scanning the full grid.

### Phase B — Correlation IDs (gateway + clients)

**Deliverables**

- `request_id` on every inbound gateway request (middleware); include on downstream slog calls where feasible.
- Chat completion path: `conversation_id` (client-supplied if allowed and validated, else generated) on related logs.
- Ingest / indexer: agreed header or field `index_run_id` on batches; gateway logs echo it.
- `service` (and related facets per [§5](#5-unified-event-tagging)) on gateway-emitted lines so **subsystem** and **conversation** projections share a **tag vocabulary**.

**Exit criteria:** UI can group lines by id without guessing from timestamps; tags enable **subsystem** vs **user** lenses on the **same** events.

### Phase C — Indexer run narrative

**Deliverables**

- Indexer emits structured lifecycle events: `indexer.run.start`, `indexer.run.progress`, `indexer.run.done` (names illustrative) with `project`, `index_run_id`, counts, phase, errors.
- UI: collapsible **run** card with progress derived from latest progress line.

**Exit criteria:** One project sync is understandable as a single card with an optional drill-down timeline.

### Phase D — Conversation + BiFrost rollup + principal panel

**Deliverables**

- Consistent `msg` prefixes or `shape` for BiFrost-related gateway logs, **routing/fallback** events, and **Qdrant/RAG** gateway-side logs; same `request_id` / `conversation_id` where applicable.
- Summary mode: optional rollup (“N HTTP events, worst status, total ms”) inside a conversation thread.
- **Principal-aware UI ([§3](#3-user-and-conversation-centric-view)):** panel listing **by API key / principal label**; nested **conversation cards** with header, start/end/elapsed, service summaries, **error count** and header tint, **auto-compact** last-N timeline + **expandable earlier events**, per-row expand for details.
- **Subsystem panel ([§4](#4-subsystem-health-and-service-narratives)):** **gateway**, **Qdrant**, **BiFrost**, **indexer** cards with **started / last message / uptime**, **metrics**, **error rollup**, and **expandable** event timelines (same summary-line pattern as conversations).
- **Unified tagging ([§5](#5-unified-event-tagging)):** structured logs include `service`, `principal_id`, `conversation_id`, `request_id`, `index_run_id` where applicable so user and subsystem views **cross-link** and filter consistently.
- **Context growth:** display **metadata only** (turn index, estimated sizes, RAG attachment counts) when the gateway logs or exposes them—no bodies by default.

**Exit criteria:** A chat session’s related traffic is navigable as one thread **under the correct principal**, with service participation **summarized** and **errors** visible at conversation scope; **subsystem health** is visible as a **parallel story** with the same expand/collapse behavior; raw lines remain reachable.

### Phase E — Optional server-side event store

**Trigger:** DOM/history limits, need for server-side filters (“errors only for tenant X”), **per-conversation replay** across restarts, or cross-restart analytics.

**Deliverables:** TBD; likely small schema (`seq`, `shape`, `thread_id`, `headline_json`, `raw_ref`) and API extensions—only after A–D prove the product direction.

---

## 8. Agent-assisted implementation protocol

This section is **normative for agents** implementing this plan: it defines **collaboration rhythm** with you so exploration is explicit and decisions are captured.

### 8.1 Roles

- **You:** product intent, acceptance (“this is clearer”), privacy boundaries, and priority between phases.
- **Agent:** implements slices, runs the gateway, inspects real log lines, proposes **concrete** field names and shapes, and documents trade-offs in this file or short inline comments where appropriate.

### 8.2 Mandatory exploration passes (before large UI rewrites)

For each phase **before** merging a large diff, the agent must:

1. **Log corpus sample** — Run representative workloads (health check, failed auth, successful `/v1/chat/completions`, indexer ingest if available) and capture **5–15 representative JSON lines** per shape (sanitize tokens). Use them as **fixtures** for client-side classification tests (even if tests are “golden object + expected shape” in a small JS or Go test harness).
2. **Field audit** — Grep / read handlers for **actual** slog keys today (`http response`, chat, ingest). The taxonomy in code must match **reality**, not only this doc.
3. **Contrast check** — In **Detailed** mode, verify **no field loss** vs current behavior (Summary may hide fields in the first paint but must retain access).

### 8.3 Checkpoint questions (agent → you)

The agent should pause and ask when:

- **Correlation** would require new headers on `/v1/*` (client impact).
- **Summary** hides a field that might be needed for support (e.g. `tenant` visibility).
- **Indexer estimates** are expensive or misleading (which estimate: files, chunks, bytes?).
- **Rollup** could obscure **ordering** or **first failure** (need your preference: “first error wins” vs “worst status wins”).
- **Principal panel** could imply **token fingerprinting** or labels—confirm what may be kept in memory-only session state vs omitted from the browser.
- **Context metrics**—which fields are safe to add to slog (token estimates, chunk counts) vs off-limits.
- **Subsystem card** “metrics” — which numbers are **observed** vs **misleading** if computed client-side from sparse logs.

Default: agent proposes a recommendation and a **fallback**, then you choose in one reply.

### 8.4 Deliverable shape per PR

Each PR should include:

- **User-visible:** what changed in Summary vs Detailed (screenshot or short description).
- **Fixture:** at least one new or updated golden sample (or table-driven test) for classification.
- **Docs:** one subsection under **§10 Changelog (implementation)** below (timestamp + PR intent), not a duplicate architecture essay.

### 8.5 Optional experiments (agent may propose; you approve)

Short spikes (time-boxed, may be thrown away):

- **E1:** Virtualized list vs capped 5000 rows — only if scroll performance fails.
- **E2:** Web Worker for parse/classify — only if main thread stutters on bursts.
- **E3:** Minimal NDJSON `log_schema` version field — only if JSON ambiguity blocks reliable shapes.

### 8.6 What the agent must not do without explicit approval

- Log **message bodies**, full tokens, or raw API keys.
- Remove **Detailed** mode or break **verbatim** inspection paths.
- Add a heavy **server event store** (Phase E) before Phases A–B justify it.
- Ship **full chat transcripts** in the conversation panel without explicit product approval and security review.

---

## 9. Open questions (for you + agent to resolve during implementation)

1. **Conversation identity:** Accept client-provided id (header?) vs gateway-only generated id for grouping.
2. **Indexer ↔ gateway:** Single `index_run_id` scheme and whether the gateway mints it or the indexer does.
3. **Persistence of UI prefs:** Summary/Detailed only, or also default filters (app, level)?
4. **Cross-origin / embedded shell:** Ensure any new query params (`?view=summary`) work in iframe + `postMessage` activation paths in [`logs.html`](../internal/server/embedui/logs.html).
5. **Principal label:** Fingerprint-only vs mapping to `tokens.yaml` comment/name when available.
6. **Context metrics contract:** Which fields are logged per turn (e.g. `context_tokens_est`, `rag_hits`) and whether they are suitable for a **delta** visualization.
7. **Subsystem metrics:** Which **gateway/supervised** lines become the **source of truth** for Qdrant/BiFrost “metrics” on the card vs leaving fields empty when not logged.
8. **Cross-link UX:** Whether “jump to conversation” / “jump to service timeline” is **in-scope** for v1 of the presentation layer or deferred.

---

## 10. Changelog (implementation)

_Append here as phases land (date, PR or commit, one paragraph)._

| Date | Note |
|------|------|
| — | Plan authored; no implementation yet. |
| 2026-04-21 | **version-0.2.1:** Phases A–D (initial): `requestid` middleware; `service` + `request_id` on access logs; chat `conversation_id` (header `X-Chimera-Conversation-Id` or generated) + `principal_id` on chat logs; `msg` tags on chat/RAG/ingest; ingest `index_run_id` echo + indexer client header; indexer `indexer.run.*` + `index_run_id` on process logs; logs UI **Detailed / Summary / Conversations / Subsystems**; `wrapResponse` initial status fixed so logged **statusCode** matches handler. See [`log-presentation-acceptance.md`](log-presentation-acceptance.md). **Phase E** not implemented. |
| 2026-04-21 | **Logs UI (presentation follow-up):** **Conversations:** principals as `<details>`, nested cards with start/last/spanned time, **HTTP rollup** (worst status, count, Σ ms), **service chips** (RAG / BiFrost / Qdrant / ingest), **context** strip when `turn_index` / token estimates / hits appear on lines, per-event `<details>` with full fields, **Earlier events** folder. **Subsystems:** first/last/window time, buffered **metrics** chips, expandable timelines, **Conversation** links when `conversation_id`+`principal_id` present. **Indexer runs** view: one card per `index_run_id`, latest `indexer.run.progress` phase/candidates, timelines. `?view=` sync, `?seq=` / `?principal=` + `?conversation=` focus, `data-log-seq` on table rows, `postMessage` may set `view`. **Phase E** (server event store) still out of scope. |
| 2026-05-03 | **Phase B (hardening):** Response `X-Request-ID` on all requests; `X-Chimera-Conversation-Id` echoed on chat responses; RAG **internal** DEBUG/TRACE lines carry `request_id`, `conversation_id`, `index_run_id` (ingest), `msg` + `service=gateway`; `ingest_session` (chunked) logs match simple ingest (`ingest.complete`, correlation on errors); `X-chimera-indexer-Run-Id` on session start and indexer config `optional_headers`. |
| 2026-05-03 | **Indexer structured operator events:** `chimera-indexer` emits `msg` slugs for discovery/reconcile/queue/retry/recovery/job/run-done (`internal/indexer/ops_events.go`, `indexer.go`, `cmd/chimera-indexer/main.go`); field table in `docs/indexer.md` § Structured operator logs. Counters roll up into `indexer.run.done`. |
| 2026-05-04 | **Summarized expand panels:** removed **Last events** previews from conversation + service `<details>` bodies; **Services** expanded Summary uses rollups from buffered lines — **indexer:** upload/ingest/skipped, fail/retry/pause, workers + latest queue snapshot + unique `rel`; **gateway:** HTTP Σ ms, `ingest.complete` / RAG / chat slug counts, warn+error; **qdrant:** HTTP Σ ms, line count, warn+error. **Indexer run** expand adds the same job rollup plus existing vector / gateway OK\|error mini row. |
| 2026-05-03 | **Gateway v0.2.2 (release):** supervised `chimera-indexer` optional child; operator `/ui/indexer` + `/ui/continue`; shell main summary and logs refinements; consolidated stats/observability in the desktop shell. Operator summary: [`version-v0.2.md`](../version-v0.2.md#shipped-releases-v020-through-v022) (§ Shipped releases). |
| 2026-05-08 | **`log-bifrost.md` P4 (gateway relay alignment):** `internal/chat/chat.go` attaches stable slugs **`msg=chat.bifrost.response`** on upstream completion logs, **`chat.routing.attempt`** / **`chat.routing.resolved`** for virtual-model routing (attempt at **Debug** when fallback chain length is 1), and **`chat.provider_limits.blocked`** on provider-limit blocks; derive + `logs.js` keep **legacy** string matching (`upstream chat response`, old virtual-model wording) for one release window. |
| 2026-05-09 | **`log-gateway.md` P2:** Every gateway-parent `slog` line in the P2 file list carries **`msg`** (including `gateway.auth.*` for client credentials, `gateway.startup.*` / shutdown / supervisor child lifecycle, `routing.*`, `upstream.*`, `conversation.*`, `ingest.failed`, RAG delete-pre, tool-router, HTTP access as **`gateway.http.access`**). `WaitHealthy` logs **`gateway.supervisor.qdrant.ready`** vs **`gateway.supervisor.bifrost.ready`** by **child** parameter. Raw **`chimera.start`** buffer seed replaced with **`gateway.startup.seed`**. `inferShape` in **`logs.js`** treats **`gateway.http.access`** like legacy **`http response`** for **`http.access`** shape. |
| 2026-05-09 | **`log-gateway.md` P6 (demotions):** Gateway **`chat.bifrost.available_models`** / “upstream models (merged list)” — **first successful** merged `/v1/models` poll in a process stays **`INFO`**; **each subsequent successful** poll is **`DEBUG`** (same `msg` and fields). **Warn** path for unavailable catalog unchanged. Cuts periodic catalog noise at default `LOG_LEVEL=info` without removing the one-shot cold-start signal; full poll trail remains at debug. No checked-in “three fixtures” showed zero-fire lines worth deleting outright; prefer demotion over deletion for this pass. |
| 2026-05-09 | **`log-conversations.md` Phase 8 (witness):** Gateway emits **`conversation.request.witness`** (message counts, `role_counts` JSON, `prompt_char_estimate`, `tool_decl_count`) and **`conversation.response.witness`** (`completion_char_estimate`, `finish_reason`, `chunk_count`; streaming uses SSE tail). **`conversation.payload.sample`** at slog trace (-8) with redacted `head`/`tail` when `log_level=trace` or `gateway.log_witness.force_payload_sample_at_debug` with debug; `payload_sample_max_chars` caps excerpt runes. Package **`internal/conversationwitness`**; `conversationCardModel` **`witness`** flags; fixture `phase8-witness.example.log`. |
| 2026-05-08 | **`log-bifrost.md` P5 (conversation linkage):** [`conversationBifrost.js`](../../internal/server/embedui/logs/derive/conversationBifrost.js) + **`serviceStripHtml`** adds **BiFrost · N** on conversation cards; `renderSummarizedUnified` merges gateway relay rows into **`(principal_id, conversation_id)`** groups by **`request_id`** when correlation attrs are incomplete on individual lines yet the row matches the BiFrost relay slug set (tier 2 from [`log-conversations.md`](log-conversations.md)). |

---

## 11. References

- [`internal/server/embedui/logs.html`](../internal/server/embedui/logs.html) — parse, `sortExtraKeys`, row cap, SSE/poll
- [`internal/servicelogs/store.go`](../internal/servicelogs/store.go) — entry model
- [`SECURITY.md`](../SECURITY.md) — logging and redaction
