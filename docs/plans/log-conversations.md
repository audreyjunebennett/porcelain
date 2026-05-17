# Plan: Operator-facing conversation logs and full request lifecycle

| Field | Value |
|-------|-------|
| **Doc kind** | `feature-plan` |
| **Owners / areas** | Gateway core (`internal/server`, `internal/chat`, `internal/rag`, `internal/conversationmerge`, `internal/transform`, `internal/upstream`), supervised subprocess ingest (`internal/servicelogs/qdrantline`, `internal/servicelogs/bifrostline`), logs UI (`internal/server/embedui/logs`), parse/derive (`internal/server/embedui/logs/parse`, `derive/conversationMetrics.js`, `derive/conversationBifrost.js`, `derive/gatewayCardModel.js`, `derive/qdrantCollection.js`, additional derive modules as needed) |
| **Status** | `done` |
| **Targets** | Conversation card and per-conversation timeline in the operator log view; correlation and slugs sufficient to reconstruct each chat turn end-to-end (routing, RAG, tools, upstream, delivery) without stopping at heuristics alone |
| **Last updated** | 2026-05-09 |
| **Supersedes / superseded by** | None |

## At a glance

Operators need a single conversation timeline that tells the **whole story of each user turn**: how the request entered the gateway, how it was merged or deduped, how routing chose a model, whether RAG and vector search ran, what tools ran and whether they succeeded, what went upstream to the model provider, and how the response left the gateway. Phases **1–8** shipped correlation tiers, a **`conversation.*`** lifecycle, **per-turn indexing**, **tool round-trip slugs**, **Qdrant** joins where the gateway can anchor them, **subprocess linkage** when platforms echo correlation headers, and a **payload witness policy** without logging secrets. This document remains the normative spec for that behavior.

**Related docs:** [`log-presentation-layer.md`](log-presentation-layer.md) (conversation view and unified tagging). Classification / routing foundations: [`log-qdrant.md`](log-qdrant.md) (shipped), [`log-bifrost.md`](log-bifrost.md) (shipped), [`log-gateway.md`](log-gateway.md) (shipped — parent-process taxonomy, `msg` everywhere, gateway card derive). Those three define stable `msg` shapes, ingest normalization, service-card derive modules, and rules this plan extends — see [Alignment with shipped classification](#alignment-with-shipped-classification). [`log-view-indexer.md`](log-view-indexer.md).

| Phase | Outcome | Status |
|-------|---------|--------|
| [Phase 1 — Spec, fixtures, and frozen taxonomies](#phase-1--spec-fixtures-and-frozen-taxonomies) | Frozen slug lists, routing rules, payload policy, and fixture set agreed | `done` |
| [Phase 2 — Correlation propagation](#phase-2--correlation-propagation) | Every gateway line that touches a chat request carries `conversation_id`, `request_id`, and `principal_id` where known; ingest lines carry `index_run_id` and optional `conversation_id` from `X-Chimera-Conversation-Id` | `done` |
| [Phase 3 — Lifecycle events](#phase-3--lifecycle-events) | Gateway emits the full `conversation.*` lifecycle at named call sites | `done` |
| [Phase 4 — User interface fan-out and conversation card](#phase-4--user-interface-fan-out-and-conversation-card) | Conversation card uses tiers 1–4, tier 3 for ingest, pills, progress bar, and inferred-line labeling | `done` |
| [Phase 5 — Subprocess linkage hardening](#phase-5--subprocess-linkage-hardening) | Gateway sets upstream `X-Request-Id`, BiFrost subprocess joins only when the platform exposes it, and Qdrant rows join by gateway RAG spans | `done` |
| [Phase 6 — Turn identity and timeline grouping](#phase-6--turn-identity-and-timeline-grouping) | Each turn is explicitly numbered and the expanded timeline can group by turn | `done` |
| [Phase 7 — Tool execution logging](#phase-7--tool-execution-logging) | Each tool round-trip is visible with correlation, outcome, and timing | `done` |
| [Phase 8 — Request and response witness](#phase-8--request-and-response-witness) | Operator-safe summaries and gated payload samples complete the story without logging secrets | `done` |

---

## Background

The conversation card is built in [`internal/server/embedui/logs.js`](../../internal/server/embedui/logs.js) (`renderSummarizedUnified`, `sortConversationGroupsByRecency`, `buildConvCard`, `renderExpandedConv`). The group key is `principal_id` (or `tenant`) plus `conversation_id` — **each pair is its own card**; the UI must not roll up different `conversation_id` values for one principal into a single card. Related lines join the card via **correlation tiers** (relay, request-id mapping, ingest, RAG spans, collection/time heuristics for Qdrant, and optional subprocess linkage when echoed headers exist) — see Phase 4 and [`conversationMetrics.js`](../../internal/server/embedui/logs/derive/conversationMetrics.js).

Correlation reaches the chat handler, upstream relay, RAG pipeline, conversation merge, and derive-time fan-out described in Phases 2–6. BiFrost **relay** rows merge by **`conversation_id`** (tier 1) or **`request_id`** (tier 2) via [`conversationBifrost.js`](../../internal/server/embedui/logs/derive/conversationBifrost.js). **Subprocess** `bifrost.*` lines can surface on the conversation card when tier-5 linkage applies ([`log-bifrost.md`](log-bifrost.md)). Qdrant **`qdrant.*`** lines join by tier 4 / 4b using the same collection naming as [`qdrantCollection.js`](../../internal/server/embedui/logs/derive/qdrantCollection.js).

Parent-process gateway logs use the frozen dotted **`msg`** taxonomy ([`log-gateway.md`](log-gateway.md)): **`gateway.auth.*`**, **`gateway.http.access`**, **`conversation.merge.*`**, **`rag.retrieve.source`**, **`chat.tool_router.*`**, and the **lifecycle** slugs in [Phase 1](#lifecycle-conversation--gateway-emitted) (`conversation.received`, `conversation.rag.span`, …) as implemented in Phases 3–8.

Heavy taxonomy tables and routing tiers live under Phase 1 as the single spec source.

### Alignment with shipped classification

| Area | Shipped structures to reuse when routing or labeling conversation logs |
|------|-------------------------------------------------------------------------|
| **Gateway (parent)** | Every gateway `slog` line carries **`msg`**; HTTP rows use **`gateway.http.access`** (`inferShape` still aliases legacy `http response`). Service card summarize path uses **`gatewayCardModel`** in [`gatewayCardModel.js`](../../internal/server/embedui/logs/derive/gatewayCardModel.js). Upstream / catalog / health slugs live under **`upstream.*`**, **`chat.bifrost.available_models`**, **`gateway.supervisor.*`**, **`gateway.health.*`** (see [`log-gateway.md`](log-gateway.md)). Merge failures today emit **`conversation.merge.resolve_failed`** (and related **`conversation.merge.*`**) — align any new **`conversation.merge.failed`** naming with that doc’s compatibility rules. |
| **BiFrost (subprocess)** | Stdout is normalized in **`internal/servicelogs/bifrostline`** to stable **`bifrost.*`** slugs. Counter windows reset on **`bifrost.startup.banner`**. Summarized headlines use **`bifrostOperatorLine`** in [`bifrostMetrics.js`](../../internal/server/embedui/logs/derive/bifrostMetrics.js). Conversation **BiFrost · N** chip uses **`conversationBifrostRelayCount`** / **`conversationBifrostTimelineFlat`** ([`conversationBifrost.js`](../../internal/server/embedui/logs/derive/conversationBifrost.js)). Tier 5 conversation join for subprocess rows remains conditional on echoed **`X-Chimera-Conversation-Id`** / **`X-Chimera-Request-Id`** in normalized payloads ([`log-bifrost.md`](log-bifrost.md)). |
| **Qdrant (subprocess)** | Stdout is normalized in **`internal/servicelogs/qdrantline`** to **`qdrant.*`**. Collection names for UI routing match indexer / gateway rules (**`qdrantCollectionName`** in [`qdrantCollection.js`](../../internal/server/embedui/logs/derive/qdrantCollection.js), parity with `internal/vectorstore`). Counter windows reset on **`qdrant.version`**. Global Qdrant card metrics use **`qdrantCardModel`** (same module). Tier 4 joins from this plan should use the **same collection string** as that derive path so conversation RAG subsection stays consistent with Qdrant and indexer cards. |

---

## Phase 1 — Spec, fixtures, and frozen taxonomies

**Goal.** Lock slug names, routing tiers, turn and tool fields, and the payload witness policy so later phases implement against a fixed contract.

**Deliverables**

- This document as the authority for conversation logging (aligned with [`_template.md`](_template.md)).
- Frozen **`conversation.*`** lifecycle list (table below) plus extensions for **turn**, **tool**, and **witness** slugs (tables below).
- Frozen **routing tiers** (tiers 1–5) and **tier 4b** (gateway-anchored Qdrant window); default window constants documented with rationale.
- Reference captures under repo-root **`temp/sessions`** (gitignored; operators drop exports there): single-turn chat, multi-turn merge, fallback chain, chat plus ingest, **multi-turn with tools**, **streamed completion** (if applicable). Example layout: `temp/sessions/<timestamp>_<git-sha>/data/gateway/chimera-desktop.log` plus `comment.txt`.
- A short **fixture map** (spreadsheet or markdown table) listing each fixture and which slugs and tiers it must exercise.
- Cross-check with [`log-gateway.md`](log-gateway.md) and [`log-bifrost.md`](log-bifrost.md): no duplicate intent under conflicting names; renames go through those docs’ compatibility rules.

**Acceptance**

- Reviewer can map every line in every fixture to a tier and a slug (or `*.unparsed` for subprocess).
- Reviewer signs off on **payload witness defaults** (what is always Info vs Trace-only) and on **turn_index** reset semantics (process-local).
- **Phase 1 fixture map:** [below](#phase-1-fixture-map).

**Status:** `done`

### Phase 1 fixture map

Captures live under `temp/sessions/` (not committed). Map each export you add or refresh against the slugs and tiers it is meant to exercise.

| Fixture folder (example) | Intent | Slugs / behavior to cover |
|--------------------------|--------|---------------------------|
| `20260509-*_5cbf00e` (three time slices, same git sha) | Desktop gateway tail during dev | Tier **1** (`principal_id` + `conversation_id` on `chat.request`, `rag.*`, `chat.routing.*`); tier **2** BiFrost relay lines joined by `request_id`; streaming (`stream=true`); RAG path (`rag.query`, `rag.retrieve.error` / success variants when present). |
| *(add)* `single-turn` | Happy path, no merge | `chat.request` → routing → upstream → delivery; optional `rag.retrieve.source`. |
| *(add)* `multi-turn-merge` | Sticky / semantic merge | `conversation.merge.*` or future `conversation.merged`; same `conversation_id` across HTTP turns. |
| *(add)* `fallback-chain` | Virtual model fallback | `chat.routing.attempt`, `chat.routing.fallback`, `chat.bifrost.*` errors then success. |
| *(add)* `chat-plus-ingest` | Tier 3 | `ingest.complete` with `index_run_id` and chat `conversation_id` when applicable. |
| *(add)* `tools` | Phase 7 prep | `chat.tool_router.*` and future `conversation.tool.*`. |
| *(add)* `streamed` | Streaming aggregate | Witness / completion when Phase 8 lands. |

### Phase 1 — merge failure naming (cross-check)

- **Shipped:** `conversation.merge.resolve_failed` (and other `conversation.merge.*` diagnostics) per [`log-gateway.md`](log-gateway.md). **No second emitted slug** for the same intent.
- **Lifecycle table** row `conversation.merge.failed` is a **logical umbrella** for operator docs and future UI filters: group by prefix `conversation.merge.` — do not fork a conflicting `msg` value in the gateway until an explicit rename window is agreed in `log-gateway.md`.

### Phase 1 — payload witness defaults (sign-off)

- **Info:** counts, sizes, models, ids, cheap hashes only — per lifecycle / witness tables above.
- **Trace:** `conversation.payload.sample` only at trace (or dedicated flag); **256** chars max per `head` / `tail` after redaction unless config overrides.
- **Never:** full bodies at Info; never raw tool arguments at Info (size fields only).

### Phase 1 — `turn_index` and dedup (**locked**)

A dedup cache hit that does **not** call upstream is still a new inbound HTTP chat completion request: **`turn_index` increments** (same rules as a non-dedup turn for that `conversation_id` once Phase 6 implements the counter). Documented in fixtures when dedup fixtures exist.

### Product decisions (confirm in Phase 1 review)

| Topic | Decision |
|-------|----------|
| Slug prefix | **`conversation.*`** for lifecycle, turn, tool, and witness events the gateway emits at named points in the request flow. |
| Group key | **`principal_id + "\0" + conversation_id`** with fallback to **`tenant + "\0" + conversation_id`** when `principal_id` is absent. |
| Eligibility | A line joins a conversation card when it matches **tier 1–5** (see Routing rules). |
| Subprocess fan-out | BiFrost subprocess lines join only when normalized rows carry `request_id` from upstream `X-Request-Id` (**Phase 5**). Clients are not expected to send Chimera correlation headers, and gateway relay slugs remain canonical for the turn when BiFrost cannot expose request ids. |
| Qdrant join | **Tier 4** uses collection plus time; **tier 4b** uses a gateway-emitted anchor line (**Phase 5**) so the window is tied to `request_id`, not only to `rag.query` timestamps. |
| Log UI conversation cards | **One card per** `principal_id` + `conversation_id` (see [`sortConversationGroupsByRecency`](../../internal/server/embedui/logs.js)); never merge distinct `conversation_id` values by wall-clock gap. |
| Counter window | **Tier 4** (Qdrant inferred join): default **±5 s** from anchor (`rag.query` / `rag.embed` time) unless `conversation.rag.span` supplies **`window_ms`** (default **10000** ms for tier 4b). Gateway process restart clears in-memory UI buffers. |
| Progress bar | **received → routed → (rag) → upstream → delivered**; each step maps to a distinct `conversation.*` slug (see Phase 3). |
| Locale | English-only operator headlines and pill tooltips. |

### Lifecycle (`conversation.*`) — gateway-emitted

Every line in this table is emitted by the gateway and **must** carry `request_id`, `conversation_id`, `principal_id` (once known), `service:"gateway"`, and **`turn_index`** after Phase 6 lands (Phase 2–3 may introduce `turn_index` incrementally; Phase 6 makes it mandatory on chat-scoped lines).

| `msg` | Emit point | Level | Headline | Required KV |
|-------|------------|-------|----------|-------------|
| `conversation.received` | `internal/server/server.go` `handleChatCompletions` after conversation id is resolved | Info | "conversation received" | `conversation_id`, `request_id`, `principal_id`, `clientModel`, `stream`, `tenant`, `project`, `flavor`, `cid_source` (`header` / `merge` / `generated`), `turn_index` |
| `conversation.merged` | `internal/conversationmerge/service.go` `Resolve` when a candidate matches | Info | "conversation matched" | `conversation_id`, `request_id`, `principal_id`, `match_score`, `candidate_count`, `merge_reason` (`semantic` / `sticky`), `turn_index` |
| `conversation.dedup_hit` | `internal/conversationmerge/service.go` `Resolve` on dedup cache hit | Info | "conversation dedup hit" | `conversation_id`, `request_id`, `principal_id`, `dedup_bytes`, `turn_index` |
| `conversation.routing.resolved` | `internal/chat/chat.go` after the model to try is decided | Info | "conversation routed" | `conversation_id`, `request_id`, `upstreamModel`, `attempt`, `chainLen`, `stream`, `turn_index` |
| `conversation.rag.attached` | `internal/server/server.go` after `rag.InjectSystemMessage` | Info | "conversation RAG attached" | `conversation_id`, `request_id`, `tenant`, `project`, `flavor`, `hits`, `collection`, `turn_index` |
| `conversation.rag.skipped` | `internal/server/server.go` when RAG is enabled but query empty or hits zero | Debug | "conversation RAG skipped" | `conversation_id`, `request_id`, `reason` (`empty_query` / `no_hits` / `disabled`), `turn_index` |
| `conversation.rag.span` | `internal/rag/service.go` immediately before the first outbound vector or Qdrant-related work for this request, and once per retrieve path if multiple | Info | "conversation RAG span" | `conversation_id`, `request_id`, `collection`, `turn_index`, `span_id`, `window_ms` (default **10000**; used for tier 4b UI join) |
| `conversation.upstream.started` | `internal/chat/chat.go` just before upstream HTTP | Info | "conversation upstream started" | `conversation_id`, `request_id`, `upstreamModel`, `stream`, `outgoingTokens`, `turn_index` |
| `conversation.upstream.completed` | `internal/chat/chat.go` `logUpstreamChatResponse` on 2xx | Info | "conversation upstream completed" | `conversation_id`, `request_id`, `upstreamModel`, `statusCode`, `usagePromptTokens`, `usageCompletionTokens`, `usageTotalTokens`, `responseBytes`, `turn_index` |
| `conversation.upstream.failed` | `internal/chat/chat.go` on 4xx/5xx and `chat.bifrost.error` path | Warn | "conversation upstream failed" | `conversation_id`, `request_id`, `upstreamModel`, `statusCode`, `err`, `turn_index` |
| `conversation.fallback.attempted` | `internal/chat/chat.go` retry path | Info | "conversation fallback attempted" | `conversation_id`, `request_id`, `upstreamModel`, `prev_status`, `attempt`, `chainLen`, `turn_index` |
| `conversation.fallback.exhausted` | `internal/chat/chat.go` exhaustion path | Warn | "conversation fallback exhausted" | `conversation_id`, `request_id`, `chainLen`, `excluded_413_count`, `turn_index` |
| `conversation.delivered` | `internal/server/server.go` after response writer finishes (success) | Info | "conversation delivered" | `conversation_id`, `request_id`, `statusCode`, `stream`, `bytes`, `total_ms`, `turn_index` |
| `conversation.errored` | `internal/server/server.go` on final error response | Warn | "conversation errored" | `conversation_id`, `request_id`, `statusCode`, `errorType`, `turn_index` |
| `conversation.merge.failed` | *(Lifecycle / doc umbrella; **no new emitted `msg`**. Use shipped **`conversation.merge.resolve_failed`** and other **`conversation.merge.*`** — [Phase 1 merge naming](#phase-1--merge-failure-naming-cross-check).)* | Warn | "conversation merge failed" | `request_id`, `step`, `err` (and `conversation_id` when known); `turn_index` when known |

*Merge diagnostics today:* [`log-gateway.md`](log-gateway.md) lists **`conversation.merge.resolve_failed`**, **`conversation.merge.embed_failed`**, **`conversation.merge.dedup_read_failed`**, and other **`conversation.merge.*`** rows. **Phase 1:** no rename; filter by prefix **`conversation.merge.`**.

### Turn fields (Phase 6)

| Field | Type | Semantics |
|-------|------|-----------|
| `turn_index` | int | **1-based** count of user turns for this `conversation_id` within the **gateway process**. Incremented on each new `handleChatCompletions` request for that conversation, **including** dedup short-circuits that never call upstream (Phase 1 completion notes). |
| `span_id` | string | Short random id per `conversation.rag.span` for correlating optional debug lines. |

### Tool execution (`conversation.tool.*`) — gateway-emitted

Emit at the points where the gateway applies tool routing, invokes tools, or records tool results (exact call sites in Phase 7; may wrap `internal/transform` and chat completion streaming paths).

| `msg` | Emit point | Level | Headline | Required KV |
|-------|------------|-------|----------|-------------|
| `conversation.tool.router` | After tool router pass completes | Debug | "conversation tool router" | `conversation_id`, `request_id`, `turn_index`, `tools_before`, `tools_after`, `router_model` (if any), `err` (empty on success) |
| `conversation.tool.call_started` | Immediately before executing one tool call | Info | "conversation tool started" | `conversation_id`, `request_id`, `turn_index`, `tool_name`, `tool_call_id` (if protocol supplies), `arg_bytes` (size only; **no raw args** at Info) |
| `conversation.tool.call_completed` | On tool success | Info | "conversation tool completed" | Same ids, `tool_name`, `tool_call_id`, `latency_ms`, `result_bytes` (size only) |
| `conversation.tool.call_failed` | On tool error | Warn | "conversation tool failed" | Same ids, `tool_name`, `tool_call_id`, `latency_ms`, `err` (short, no secrets) |

**Tag propagation:** Existing `chat.tool_router.*` slugs in [`log-gateway.md`](log-gateway.md) gain the same correlation triple and `turn_index`; derive modules accept both during any rename window.

### Request and response witness (`conversation.*`) — gateway-emitted (Phase 8)

| `msg` | Emit point | Level | Headline | Required KV |
|-------|------------|-------|----------|-------------|
| `conversation.request.witness` | After body parsed and normalized for upstream (redaction applied) | Info | "conversation request witness" | `conversation_id`, `request_id`, `turn_index`, `message_count`, `role_counts` (structured or JSON map), `prompt_char_estimate`, `tool_decl_count`, `sha256_canonical` (optional; omit if expensive) |
| `conversation.response.witness` | After upstream success path has assembled assistant output length | Info | "conversation response witness" | `conversation_id`, `request_id`, `turn_index`, `completion_char_estimate`, `finish_reason`, `chunk_count` (non-streaming: 1) |
| `conversation.payload.sample` | Optional gated excerpt | **Trace** only | "conversation payload sample" | `conversation_id`, `request_id`, `turn_index`, `kind` (`request` / `response`), `head`, `tail`, `redacted` (bool) — **never** log API keys, bearer tokens, or `Authorization`; follow repo security docs |

**Policy defaults (Phase 1 locks the numbers)**

- **Info:** counts, sizes, models, ids, hashes only if cheap and stable.
- **Trace:** samples only when `LOG_LEVEL=trace` (or dedicated config flag); max **256** chars per `head`/`tail` after redaction unless config overrides.
- **Never:** full payloads at Info; never raw tool arguments at Info (use size fields).

### Tag propagation (all chat-related gateway slugs)

Every slug that participates in a chat (`chat.bifrost.*`, `rag.*`, `chat.routing.*`, `ingest.*`, **`chat.tool_router.*`**, **`gateway.http.access`** on `/v1/chat/completions`, and renamed relay slugs per [`log-bifrost.md`](log-bifrost.md)) **must** carry `conversation_id`, `request_id`, `principal_id` when in scope, and **`turn_index`** after Phase 6. Client-credential lines remain **`gateway.auth.*`** — correlation applies only when a chat-scoped logger wraps those paths.

### Routing rules (user interface fan-out into conversation cards)

#### Tier 1 — direct match

Line joins `(principal_id, conversation_id)` when both appear on the row.

#### Tier 2 — request identifier join

Line joins when `request_id` matches a tier-1 row already mapped to that conversation (`requestIdToConv` map in the UI buffer).

#### Tier 3 — index run join (ingest)

When `ingest.complete` carries `request_id`, `index_run_id`, and (when chat-originated) `conversation_id`, later indexer or Qdrant rows with the same `index_run_id` may appear under the conversation card **ingest** subsection (toggle or collapsed by default to avoid noise). This is sufficient for ingest correlation; do not add a separate `conversation.ingest.span` unless tier 3 proves insufficient.

#### Tier 4 — collection and time window (Qdrant heuristic)

When a conversation has `rag.query` or `rag.embed` with `collection`, Qdrant subprocess lines with matching **`collection`** and timestamp within **±N seconds** of the **anchor** (see tier 4b) may be annotated for the conversation **RAG** subsection. **`collection`** must be compared using the **same derived name** as [`qdrantCollection.js`](../../internal/server/embedui/logs/derive/qdrantCollection.js) / indexer routing ([`log-qdrant.md`](log-qdrant.md) Phase 3) so tier 4 agrees with Qdrant and indexer cards. UI marks **inferred**.

#### Tier 4b — gateway-anchored window (preferred over tier 4 alone)

When `conversation.rag.span` is present for a `request_id`, tier 4 uses **`span_id`** and `window_ms`: Qdrant lines matching **`collection`** (same derivation as tier 4) with `timestamp` within `[span_start, span_start + window_ms]` join as **anchored inferred** (distinct pill or sub-label from pure tier 4). If multiple spans match, attribute the Qdrant row to the **most recent span start** and preserve `span_id` / `turn_index` metadata for Phase 6 grouping. If `conversation.rag.span` is absent, fall back to tier 4 relative to `rag.query` / `rag.embed` time (legacy behavior).

#### Tier 5 — BiFrost subprocess

BiFrost subprocess lines join when normalized JSON carries a `request_id` that matches a gateway conversation request. The gateway sets upstream **`X-Request-Id`** to its `request_id`; custom `X-Chimera-*` headers are opportunistic only and must not be required because common clients do not send or preserve them. If BiFrost does not expose the upstream request id in subprocess logs, subprocess rows remain on the BiFrost service card only; **gateway relay** `chat.bifrost.*` / routing lines are already merged at tier 1 / tier 2 ([`conversationBifrost.js`](../../internal/server/embedui/logs/derive/conversationBifrost.js)) and stay canonical for the conversation **BiFrost · N** chip.

### User interface contract — conversation card (summary)

Collapsed: **State** pill from latest `conversation.*`; **BiFrost**, **Qdrant**, **Tools** (terminal tool rounds: `conversation.tool.call_completed` / `conversation.tool.call_failed`, plus `chat.tool_router.*`), **Fallback** chips; existing duration and token or vector summaries where still useful.

Expanded: key-value row includes **last `turn_index`**, **upstream model**, **client model**, **stream mode**, **RAG collection**, **merge state**; counter row reconciles with BiFrost card where possible; progress bar as in Phase 4; full log shows **service** badge, **tier** label (`direct`, `request_id`, `ingest`, `inferred`, `anchored_inferred`, `bifrost_echo`), and optional **turn** sub-headings when Phase 6 ships.

---

## Phase 2 — Correlation propagation

**Goal.** Every gateway line that touches a chat request carries the full correlation triple (`request_id`, `conversation_id`, `principal_id`) wherever those values are known; ingest lines carry `index_run_id` and chat linkage when applicable.

**Deliverables**

- Contract tests (merge, chat, RAG, ingest) plus `testdata/correlation` fixtures; exhaustive static `slog` audit for `internal/chat` deferred (gateway injects a contextualized logger for production chat).
- `internal/server/server.go` `handleChatCompletions`: pass `request_id` into `conversationmerge.Resolve`; merge failures emit `request_id` for tier 2.
- `internal/conversationmerge/service.go`: every `Resolve` path includes `request_id`; soft fallback emits merge failure lines with `request_id` (today **`conversation.merge.resolve_failed`** / related **`conversation.merge.*`** — extend rather than duplicate per [`log-gateway.md`](log-gateway.md)).
- `internal/chat/chat.go`: provider limits, payload prep debug, and admission failures carry the request logger context.
- `internal/server/ingest.go` and `ingest_session.go`: `ingest.complete` / `ingest.failed` include `request_id`, `index_run_id`, and `conversation_id` when chat-originated.

**Acceptance**

- Contract tests cover merge, chat proxy, RAG retrieve, and ingest paths; see Phase 2 implementation notes below.
- Text fixtures under `internal/server/testdata/correlation/` document expected log key shapes.

**Status:** `done`

**Implementation notes (Phase 2 shipped)**

- `conversationmerge.ResolveInput` includes `RequestID`; all `Resolve` and `RecordTurn` persistence warnings include `request_id`, `principal_id`, and `conversation_id` when known.
- `handleV1Chat` passes `request_id` into `Resolve`, logs merge resolve failures with `request_id` + `principal_id`, passes `request_id` into `RecordTurn`, and attaches `conversation_id` to the tool-router logger when the client sends `X-Chimera-Conversation-Id` before merge resolves.
- `internal/rag` `appendGatewayCorrelation` adds `principal_id` (from `Coords.TenantID`) on retrieve and ingest trace paths.
- `ingest.go` / `ingest_session.go` pass optional `X-Chimera-Conversation-Id` through to `rag.IngestRequest` and emit it on `ingest.complete`, `ingest.failed`, and `ingest.chunked.error` when set.
- Tests: `internal/conversationmerge/merge_correlation_test.go`, `internal/chat/correlation_contract_test.go`, `internal/rag/service_test.go` (`TestService_Retrieve_logContainsPrincipalId`), `internal/server/ingest_test.go` (`TestIngest_JSON_logsConversationIDWhenHeaderPresent`), `internal/server/correlation_phase2_doc_test.go`.
- **`internal/chat`:** production `/v1/chat/completions` uses a logger pre-wrapped by the gateway; contract test asserts relay logs inherit the triple. Static audit of every `slog` line in chat is not enforced (logger is injected per call site).

---

## Phase 3 — Lifecycle events

**Goal.** Gateway emits the full **`conversation.*`** lifecycle at the call sites named in Phase 1, including **`conversation.rag.span`** before vector work.

**Deliverables**

- Implementations in `internal/server/server.go`, `internal/chat/chat.go`, `internal/conversationmerge/service.go`, and `internal/rag/service.go` per Phase 1 table.
- Keep existing relay slugs where [`log-bifrost.md`](log-bifrost.md) defines renames; add `conversation.*` in addition during any transition window if needed.

**Acceptance**

- Combined fixtures contain at least one instance of each lifecycle slug (spread across fixtures if necessary).
- State pill transitions through **received → routed → upstream → delivered** on the happy path with RAG both on and off.

**Status:** `done`

**Implementation notes (Phase 3 shipped)**

- `internal/server/server.go` `handleV1Chat`: `flowStart` after JSON decode; `cid_source` (`header` / `merge` / `generated`); `conversation.received` after `conversation_id` is fixed; `Runtime.NextChatTurnIndex` on `routeLog` via `chatRouteLogger`; dedup short-circuit logs `conversation.received` + `conversation.delivered` (no upstream); `attachConversationDelivery` wires `conversation.delivered` / `conversation.errored` through `chat.ProxyOpts.OnChatDelivery`; virtual-model RAG emits `conversation.rag.skipped` (`disabled` / `empty_query` / `no_hits`), `conversation.rag.attached` after inject, passes `TurnIndex` + `LifecycleLog` into `rag.Retrieve`; missing initial model / missing client model emit `conversation.errored`.
- `internal/conversationmerge`: `ResolveInput.NextTurnIndex` bumps turn for `conversation.dedup_hit` / `conversation.merged`; `ResolveOutcome.TurnIndex` lets the handler reuse the same index for received/delivered.
- `internal/rag`: `conversation.rag.span` before embed/search with `window_ms=10000` and random `span_id`.
- `internal/chat`: `notifyChatDelivery` on marshal failure and provider-limits denial; virtual-model path emits `conversation.routing.resolved` even when `chainLen==1`.
- Fixtures: `internal/server/testdata/correlation/lifecycle-phase3.example.log`; tests `lifecycle_phase3_doc_test.go`, `correlation_phase2_doc_test.go` (file presence).

---

## Phase 4 — User interface fan-out and conversation card

**Goal.** The conversation card consumes all join tiers that apply without hand-waving tier 3 as a follow-up; pills, counters, and progress match the spec in Phase 1.

**Foundation already shipped** (see sibling plans): **tier 1 + tier 2** for BiFrost relay rows (**`conversationBifrostRelayCount`**, **`requestIdToConv`** merge in `renderSummarizedUnified`). Service cards use **`gatewayCardModel`**, **`qdrantCardModel`**, and **`bifrostMetrics`** patterns — conversation summarize should reuse the same derive/test style when adding **`conversationCardModel`** (or extending **`conversationMetrics.js`**) for state, progress, and tier labels.

**Deliverables**

- `renderSummarizedUnified()` in [`internal/server/embedui/logs.js`](../../internal/server/embedui/logs.js): tier 2 (complete for BiFrost relay; extend as needed), tier 3 (ingest subsection), tier 4, and tier 4b join logic.
- Derive module (`conversationCardModel` or extended `conversationMetrics.js`) exposing state, key-value row, counters, progress steps, and **tool** / **witness** summaries.
- `buildConvCard` / `renderExpandedConv`: new chips, tier labels, optional ingest collapse, **Tools** chip; **Qdrant** pill metrics may align with **`qdrantCardModel`** counts scoped to the conversation RAG join window.
- Styles mirroring Qdrant card semantics; tests in `internal/server/logs_components_test.go`, `internal/server/ui_logs_test.go`, and goja derive tests.

**Acceptance**

- Fixture load shows correct pill counts, anchored vs unanchored Qdrant labeling, ingest subsection when `index_run_id` is present, and progress bar behavior for RAG skipped versus attached.

**Status:** `done`

**Implementation notes (Phase 4 shipped)**

- Derive: [`internal/server/embedui/logs/derive/conversationCardModel.js`](../../internal/server/embedui/logs/derive/conversationCardModel.js) — `conversationRequestIdTier2Eligible`, `conversationIndexRunTier3Eligible`, `extractConversationQdrantJoinAnchors`, `joinQdrantLineConversationTier`, `buildConversationCardModel`.
- [`internal/server/embedui/logs.js`](../../internal/server/embedui/logs.js) `renderSummarizedUnified`: tier 1 direct; tier 2 `request_id` only after mapping from **`conversation.received`**, **`chat.request`**, or **`gateway.http.access`** on `/v1/chat/completions`, then **`rag.query` / `rag.embed`** only if still unmapped (first mapping wins); tier-2 lines limited to BiFrost relay set, `conversation.*`, `rag.*`, `chat.request` / `chat.bifrost.*` / `chat.routing.*` / `chat.provider_limits.*` / `chat.tool_router.*`, and chat completions access — not blanket `chat.*` or `ingest.*`; tier 3 `index_run_id` (first mapping wins from ingest terminal lines); tier 4/4b Qdrant subprocess as before.
- Conversation card: lifecycle step row, KV row, chip row (BiFrost, Qdrant inferred vs anchored, tools from `chat.tool_router.*` / `conversation.tool.*`, fallback), collapsed ingest summary when tier-3 lines present, join-tier badges on expanded rows (non-`direct`).
- Tests: `TestLogsDerive_conversationCardModel_joinAndProgress` in [`internal/server/logs_components_test.go`](../../internal/server/logs_components_test.go); `/ui/logs` shell includes `conversationCardModel.js`.

---

## Phase 5 — Subprocess linkage hardening

**Goal.** Subprocess lines join conversations when the platform exposes usable request ids; when BiFrost cannot expose upstream request ids, the operator still sees canonical gateway relay lines and a **deterministic** Qdrant window via **`conversation.rag.span`**.

**Deliverables**

- BiFrost: set upstream `X-Request-Id` from the gateway `request_id`; investigate whether BiFrost exposes that id on subprocess rows; document the result in [`bifrost-discovery.md`](../bifrost-discovery.md). Implement `bifrostline` `request_id` propagation only when the platform provides the value.
- Qdrant: ensure normalized lines expose `collection` and timestamps for tier 4/4b; if local Qdrant can accept a harmless query parameter for correlation in dev-only mode, document behind config (optional; do not require for acceptance). Attribute overlapping tier-4b matches to the most recent span.
- Gateway: always emit **`conversation.rag.span`** when RAG retrieval runs so tier 4b does not depend on subprocess cooperation alone. Future RAG interactions should emit the same span shape when they perform vector work.

**Acceptance**

- Either BiFrost subprocess rows appear with `request_id`, or `bifrost-discovery.md` records the gap and gateway relay remains canonical. Do not add `bifrost.relay.echo_missing` unless it carries operator-useful detail beyond the existing gateway relay lifecycle.
- Tier 4b join verified in UI or derive tests using text fixtures with RAG plus Qdrant HTTP lines inside `window_ms`, including overlapping spans where the most recent span wins.

**Status:** `done`

**Implementation notes (Phase 5 shipped)**

- Gateway upstream relay sets `X-Request-Id` from the gateway `request_id`; no client-provided Chimera header is required.
- `internal/rag`: `conversation.rag.span` default window is **10000** ms and emits through the RAG service logger when no conversation lifecycle logger is supplied.
- Derive/UI: tier 4b returns span metadata through `joinQdrantLineConversationMatch`; overlapping Qdrant rows attribute to the most recent matching span while preserving the existing tier label.
- Fixtures/tests: `internal/server/testdata/correlation/phase5-qdrant-tier4b.example.log` covers overlapping spans; focused Go tests cover upstream header propagation, fallback RAG span logging, and Qdrant attribution.
- BiFrost subprocess rows remain service-card-only unless BiFrost exposes `X-Request-Id`; gateway relay rows remain canonical and no synthetic `bifrost.relay.echo_missing` is emitted by default.

---

## Phase 6 — Turn identity and timeline grouping

**Goal.** Operators and support can separate **turn one** from **turn two** without inferring from wall clock alone.

**Deliverables**

- In-memory per-`conversation_id` counter in the gateway chat path; increment rules documented and tested (including dedup and streaming).
- Attach **`turn_index`** to every chat-scoped structured log line listed in Phase 1 and Phase 7; backfill derive and UI to group expanded rows by `turn_index` (sub-headings or dividers).
- Document: counter resets on gateway restart (acceptable); optional future persistence is out of scope unless Phase 1 chose otherwise.

**Acceptance**

- Multi-turn fixture shows `turn_index` 1 then 2 on `conversation.received` and downstream lines for the second HTTP request.
- Expanded conversation view groups or sorts by `turn_index` deterministically.

**Status:** `done`

**Implementation notes (Phase 6 shipped)**

- `Runtime.NextChatTurnIndex` (`internal/server/runtime.go`) is the in-process per-`conversation_id` counter; `chatRouteLogger` attaches `turn_index` to every chat-scoped logger so `conversation.*`, `chat.request`, `chat.bifrost.*`, `chat.routing.*`, `chat.provider_limits.*`, `rag.*`, and dedup short-circuits all inherit the value. Counter resets on gateway restart by design; no persistence.
- Dedup short-circuit and merge `Resolve` paths reuse the assigned turn (`ResolveOutcome.TurnIndex`) so `conversation.dedup_hit` / `conversation.merged` agree with the downstream `conversation.received` for that HTTP request.
- Derive: `ChimeraLogs.Derive.conversationTurnGroupsForExpanded(events, getFlat)` (`internal/server/embedui/logs/derive/conversationCardModel.js`) attributes each event to a turn using `flat.turn_index`, then `ev.qdrantTurnIndex` from the tier-4b match, then inherited from the most recent prior attribution. Groups are returned most-recent-turn-first; events inside a turn keep ascending seq/ts so the UI reverses for display while keeping the global newest-first feel.
- UI: `renderExpandedConv` renders one `Turn N` sub-heading per group and falls back to the flat ordering when only one turn (or fewer) is present.
- Fixtures: `internal/server/testdata/correlation/phase6-multi-turn.example.log`. Tests: `lifecycle_phase6_doc_test.go` (fixture content), `TestLogsDerive_conversationTurnGroupsForExpanded_*` in `internal/server/logs_components_test.go` (derive grouping, inheritance, unattributed events, tier-4b Qdrant attribution).

---

## Phase 7 — Tool execution logging

**Goal.** Each tool invocation for a conversation request has started, completed, or failed events suitable for the **Tools** chip and expanded timeline.

**Deliverables**

- Wire `conversation.tool.*` at gateway boundaries where tools are selected, invoked, and return (success or error).
- Ensure `tool_call_id` is populated when the upstream protocol provides it; omit when absent.
- Levels: router at Debug; per-call started or completed at Info; failed at Warn.

**Acceptance**

- Fixture with at least one successful and one failed tool call shows four slugs with correct correlation and chips.
- No raw secrets or full tool args at Info.

**Status:** `done`

**Implementation notes (Phase 7 shipped)**

- `internal/transform/toolrouter.go`: `ApplyToolRouter` returns `(body, ToolRouterSummary)` for operator metrics (`tools_before`, `tools_after`, `router_model`, `err`). Dedup short-circuit skips the router (no upstream tool-router HTTP on cache hits).
- `internal/server/server.go`: after `routeLog` is built, runs `ApplyToolRouter` with `routeLog` so `chat.tool_router.*` inherits `turn_index` and the correlation triple. Emits Debug `conversation.tool.router` when the router pass ran (`trSum.Ran`). Calls `LogConversationIncomingToolMessages` for each `messages[]` entry with `role=tool` (relay boundary): Info `conversation.tool.call_started` / `conversation.tool.call_completed`, or Warn `conversation.tool.call_failed` when content matches conservative error heuristics (`error:` prefix, JSON `error` / `is_error`, embedded `"is_error":true` in a string body). Sizes only (`arg_bytes`, `result_bytes`); optional `tool_call_id` / `tool_name` omitted when absent.
- `internal/server/embedui/logs/derive/conversationCardModel.js`: Tools chip counts `chat.tool_router.*` plus terminal `conversation.tool.call_completed` and `conversation.tool.call_failed` (excludes `call_started` and `conversation.tool.router` so the chip reflects completed rounds).
- Fixtures / tests: `internal/server/testdata/correlation/phase7-tools.example.log`, `lifecycle_phase7_doc_test.go`, `conversation_tool_log_test.go`, `TestLogsDerive_conversationCardModel_toolsChipCounts` in `logs_components_test.go`.

---

## Phase 8 — Request and response witness

**Goal.** Operators can confirm **what class of payload** moved through the gateway (sizes, counts, finish reason) and, when explicitly enabled, inspect **redacted** samples at Trace without implementing “full body at Info.”

**Deliverables**

- Emit `conversation.request.witness` and `conversation.response.witness` on every chat completion path (including early errors where response witness is omitted).
- Implement `conversation.payload.sample` behind trace or config; central redaction helper shared with any existing excerpt logic.
- Configuration keys documented in [`config/gateway.example.yaml`](../../config/gateway.example.yaml) (names only in this plan; wire in implementation PR).

**Acceptance**

- Security review confirms no token or key material in witness or sample fields for default configs.
- Fixture at Info shows stable size and count fields; trace fixture shows sample with `redacted: true` when applicable.

**Status:** `done`

**Implementation notes (Phase 8 shipped)**

- `internal/conversationwitness`: `LogRequestWitness`, `LogResponseWitness`, `RedactSecrets`, `LogPayloadSample` (slog level -8 “trace”); request stats from `messages` / `tools` in the proxied body map; response stats from non-stream JSON or SSE tail; `sha256_canonical` omitted (optional per Phase 1).
- `internal/server/server.go`: `emitConversationRequestWitness` after final request shaping — inside the virtual-model branch (post-RAG) and once on the direct path before proxy; merge **dedup** short-circuit logs request + response witness + optional sample from cached JSON. Payload samples use `gateway.yaml` `gateway.log_witness` (`payload_sample_max_chars`, `force_payload_sample_at_debug`) plus `config.Resolved.ShouldEmitPayloadSample` / `WitnessSampleMaxRunes`.
- `internal/chat/chat.go`: `ProxyOpts` carries witness flags; after successful `prepareChatPayload`, trace request `conversation.payload.sample` when enabled; `logUpstreamChatResponse` emits `conversation.response.witness` and response sample on **2xx** bodies (streaming uses SSE tail + chunk counts). Virtual-model inner opts inherit witness flags.
- `internal/server/embedui/logs/derive/conversationCardModel.js`: `buildConversationCardModel.witness` reflects presence of `conversation.request.witness` / `conversation.response.witness`; witness lines do not override the lifecycle state pill.
- Fixtures / tests: `testdata/correlation/phase8-witness.example.log`, `lifecycle_phase8_doc_test.go`, `internal/conversationwitness/witness_test.go`, `TestLogsDerive_buildConversationCardModel_witnessFlags` in `logs_components_test.go`.
- `config/gateway.example.yaml`: documents `gateway.log_witness` keys.

---

## Open questions

1. **Dedup and `turn_index`:** **Locked:** increment on every new HTTP chat completion for the conversation, including dedup short-circuit (see [Phase 1](#phase-1--turn_index-and-dedup-locked)).
2. **Cross-link:** Clicking an inferred Qdrant row jumps to Qdrant card via `?seq=` (default yes).
3. **Tier 4 window:** Starting **±5 s** from legacy anchor; **10 s** default for `conversation.rag.span`.
4. **Streaming:** Whether `conversation.response.witness` emits once or per finalization chunk; default **once** after stream completes with aggregate `chunk_count`.
5. **BiFrost echo:** **Locked:** no synthetic **`bifrost.relay.echo_missing`** by default. Gateway relay lines remain canonical unless a future synthetic line carries additional operator-useful detail.

---

## References

- Code: [`internal/server/embedui/logs.js`](../../internal/server/embedui/logs.js); derive [`conversationMetrics.js`](../../internal/server/embedui/logs/derive/conversationMetrics.js), [`conversationBifrost.js`](../../internal/server/embedui/logs/derive/conversationBifrost.js), [`gatewayCardModel.js`](../../internal/server/embedui/logs/derive/gatewayCardModel.js), [`qdrantCollection.js`](../../internal/server/embedui/logs/derive/qdrantCollection.js) (`qdrantCardModel`); ingest [`internal/servicelogs/bifrostline/`](../../internal/servicelogs/bifrostline/), [`internal/servicelogs/qdrantline/`](../../internal/servicelogs/qdrantline/); gateway [`internal/server/server.go`](../../internal/server/server.go), [`internal/chat/chat.go`](../../internal/chat/chat.go), [`internal/conversationmerge/service.go`](../../internal/conversationmerge/service.go), [`internal/rag/service.go`](../../internal/rag/service.go), [`internal/server/ingest.go`](../../internal/server/ingest.go), [`internal/server/ingest_session.go`](../../internal/server/ingest_session.go)
- Plans: [`log-presentation-layer.md`](log-presentation-layer.md), [`log-qdrant.md`](log-qdrant.md), [`log-bifrost.md`](log-bifrost.md), [`log-gateway.md`](log-gateway.md), [`log-view-indexer.md`](log-view-indexer.md)
- Tests: `internal/server/logs_components_test.go`, `internal/server/ui_logs_test.go`, derive tests under `internal/server/embedui/logs/derive/`

### Checklist before marking the overall feature done

- [x] Correlation audit clean; ingest triple complete when chat-originated.
- [x] All **`conversation.*`** lifecycle and **`conversation.rag.span`** events emitted per Phase 1.
- [x] Conversation card: State pill, BiFrost, Qdrant, Tools, Fallback; key-value and counter rows; progress bar; tier labels including **anchored_inferred**.
- [x] Tiers 2 (beyond BiFrost relay), 3, 4, 4b, and 5 implemented or documented with upstream tracking where impossible.
- [x] **`turn_index`** on all chat-scoped lines; UI grouping by turn.
- [x] **`conversation.tool.*`** wired for real tool paths; no secrets at Info.
- [x] **`conversation.request.witness`** / **`conversation.response.witness`** / gated **`conversation.payload.sample`** per Phase 8.
- [x] [`log-presentation-layer.md`](log-presentation-layer.md#10-changelog-implementation) changelog updated.
