# Plan: Operator-facing Conversation log classification

| Field | Value |
|-------|-------|
| **Doc kind** | `feature-plan` |
| **Owners / areas** | Gateway core (`internal/server`, `internal/chat`, `internal/rag`, `internal/conversationmerge`), supervised subprocesses (`internal/servicelogs/qdrantline`, planned `internal/servicelogs/bifrostline`), logs UI (`internal/server/embedui/logs`), parse/derive (`internal/server/embedui/logs/parse`, `derive/conversationMetrics.js`) |
| **Status** | `draft` |
| **Targets** | Conversation card + per-conversation timeline in the operator log view |
| **Last updated** | 2026-05-07 |

## At a glance

A conversation card today is **incomplete**: it groups only lines that already carry `conversation_id`, which means BiFrost subprocess output, Qdrant subprocess output, and many gateway lifecycle lines never appear in the conversation timeline even when they belong to a specific user turn. Token / vector counters are best-effort scrapes from the last few lines, not a model of the conversation. This plan captures:

1. A **fan-out routing model**: lines from **bifrost**, **qdrant**, and supervised **indexer** subprocesses are joined to the right conversation card via shared `conversation_id` / `request_id` / `index_run_id` (plus collection-name fallback) — same projection idea as the qdrant→indexer fan-out in [`log-qdrant.md`](log-qdrant.md), now applied to conversations.
2. A **conversation event taxonomy** (`conversation.*`) that names the lifecycle states the operator wants to see — received, routed, RAG-attached, upstream-started, upstream-completed, fallback-attempted, delivered, merged, deduped — so the conversation card can show **state**, not just chronology.
3. **UI contract** for the conversation card: pills (BiFrost · Qdrant · indexer · fallback · error counts), KV summary fields (model · tenant/principal · merge state · context size), and a **progress bar** driven by lifecycle events (received → routed → upstream → delivered).

**Related docs:** [`log-presentation-layer.md`](log-presentation-layer.md) (especially §3 conversation view and §5 unified tagging), [`log-qdrant.md`](log-qdrant.md), [`log-bifrost.md`](log-bifrost.md), [`log-gateway.md`](log-gateway.md).

| Phase | Outcome | Status |
|-------|---------|--------|
| [P1 — Spec](#p1--spec) | This doc + frozen `conversation.*` list and routing rules | `todo` |
| [P2 — Correlation propagation](#p2--correlation-propagation) | Every gateway line that touches a request carries `conversation_id` + `request_id`; ingest carries `index_run_id`; subprocesses inherit via headers when feasible | `todo` |
| [P3 — Lifecycle events](#p3--lifecycle-events) | Gateway emits the new `conversation.*` lifecycle slugs at well-defined points | `todo` |
| [P4 — UI fan-out & conversation card](#p4--ui-fan-out--conversation-card) | Conversation card pulls fan-out lines from bifrost / qdrant / indexer buckets; pills and progress derived from the new slugs | `todo` |
| [P5 — Subprocess linkage hardening](#p5--subprocess-linkage-hardening) | Bifrost / Qdrant subprocess lines carry conversation tags when the upstream call originated inside a conversation (best-effort header propagation) | `todo` |

---

## Background

- The conversation card in the logs UI is built by `renderSummarizedUnified()` in [`internal/server/embedui/logs.js`](../../internal/server/embedui/logs.js):
  - Group key is `principal_id + "\0" + conversation_id`.
  - **Only** entries with both `principal_id` (or `tenant`) **and** `conversation_id` are eligible (`if (!cid) continue;`).
  - Token / vector counts come from `derive/conversationMetrics.js` — heuristic scrape over `usageTotalTokens`, `usagePromptTokens`, `usageCompletionTokens`, `rag_hits`, `hits`, `chunks`, `response_tokens_est`, `tokens`, `outgoingTokens`.
  - Card state is binary: `error` if any recent event has Warn/Error level, otherwise `active`.
  - Status pills currently shown are **only** Duration / Tokens / Vectors — there is no state indicator for **routing**, **fallback**, **merge**, or **delivery**.
- Today, conversation correlation reaches:
  - **Chat handler** (`internal/server/server.go`): adds `request_id`, `conversation_id`, `service:"gateway"`, `principal_id` via `routeLog.With(...)` → covers `chat.request`, `rag.retrieve.error`, `rag.retrieve.ok`.
  - **Upstream relay** (`internal/chat/chat.go`): inherits the With-context — `chat.bifrost.request`, `chat.bifrost.error`, `chat.routing.fallback`, plus the Info `upstream chat response` and `virtual model fallback attempt`.
  - **RAG pipeline** (`internal/rag/service.go`): explicitly carries `request_id`, `conversation_id`, optional `index_run_id` via `appendGatewayCorrelation`.
  - **Conversation merge** (`internal/conversationmerge/service.go`): emits a few Warn/Debug lines on merge failure, but **without** `conversation_id` on the failure path before the id is resolved.
- Today, conversation correlation does **not** reach:
  - **Bifrost subprocess** lines (raw bifrost-http output — no conversation hook because it terminates the upstream call).
  - **Qdrant subprocess** lines (HTTP access JSON has the collection in the path, but no conversation header). The qdrant→indexer fan-out in `log-qdrant.md` is **collection-keyed**, not conversation-keyed.
  - **Supervised indexer** lines (per-run, not per-conversation; correctly out of scope for conversation join).
  - Some **gateway** lines that fire **outside** the chat handler scope (e.g. periodic upstream health probes, conversation merge resolve failures before the id is known).

---

## Locked decisions (proposed — confirm during P1)

| Topic | Decision |
|-------|-----------|
| Slug prefix | **`conversation.*`** for lifecycle events the gateway emits at named points in the request flow. |
| Group key | **`principal_id + "\0" + conversation_id`** (unchanged); falls back to `tenant + "\0" + conversation_id` when `principal_id` is missing (already implemented). |
| Eligibility | A line joins a conversation card when **either** `conversation_id` is present **or** `request_id` is present **and** matches a request seen on that conversation **or** `index_run_id` is present **and** matches an indexer run already tied to a conversation by `ingest.complete`. |
| Subprocess fan-out | Bifrost subprocess lines join a conversation **only** when they carry an outgoing correlation field (`conversation_id` or `request_id`). Otherwise they stay bifrost-only. Qdrant lines join via **collection name** matching the conversation's RAG coords (project+flavor+tenant) and timestamp window — same approach as `log-qdrant.md` collection→indexer fan-out, but scoped to ±N seconds around the chat request. |
| Counter window | Conversation cards are bounded by the existing 42-minute cluster window (`convClusterGapMs` in `logs.js`). Gateway restart clears the buffer (already true). |
| Progress | Conversation progress = **received → routed → (rag-attached?) → upstream-started → upstream-completed → delivered**. Each transition is a distinct slug; the UI maps them to a 5-step progress bar. |

### Code references

- Routing (today): [`internal/server/embedui/logs.js`](../../internal/server/embedui/logs.js) (`renderSummarizedUnified`, `clusterConversationGroupsByTime`, `buildConvCard`, `renderExpandedConv`).
- Metrics scrape: [`internal/server/embedui/logs/derive/conversationMetrics.js`](../../internal/server/embedui/logs/derive/conversationMetrics.js).
- Gateway emission sites to extend: [`internal/server/server.go`](../../internal/server/server.go) (`handleChatCompletions`), [`internal/chat/chat.go`](../../internal/chat/chat.go), [`internal/conversationmerge/service.go`](../../internal/conversationmerge/service.go), [`internal/rag/service.go`](../../internal/rag/service.go), [`internal/server/ingest.go`](../../internal/server/ingest.go), [`internal/server/ingest_session.go`](../../internal/server/ingest_session.go).
- Headers in flight: `headerConversationID` (`X-Claudia-Conversation-Id`), `headerRequestFingerprint`, `headerRollingFingerprint`, `headerProject`, `headerFlavor` — defined in `internal/server/server.go`.

## Reference samples (local dev)

Capture during P1 to repo-root **`temp/`** (gitignored):

| Artifact | Path | Purpose |
|----------|------|---------|
| Single-conversation chat | `temp/conversation-single-turn.log` | One `/v1/chat/completions` end-to-end (received → delivered) including RAG inject and one Qdrant query |
| Multi-turn chat with merge | `temp/conversation-merge.log` | Two consecutive turns where the second triggers `conversationmerge.Resolve` to find the first as a candidate |
| Fallback chain | `temp/conversation-fallback.log` | Initial 429 / 5xx triggers `chat.routing.fallback`; conversation eventually delivered by second model |
| Conversation with ingest | `temp/conversation-with-ingest.log` | A `/v1/embeddings` ingest near the same time so `index_run_id`-keyed events can be linked |

---

## Canonical `msg` taxonomy

Stable dotted slugs for conversation lifecycle events. Every line is emitted by the gateway and **must** carry `request_id`, `conversation_id`, `principal_id` (after they are known), and `service:"gateway"`.

### Lifecycle (`conversation.*`)

| `msg` | Emit point | Level | Headline | Required KV |
|-------|------------|-------|----------|-------------|
| `conversation.received` | `internal/server/server.go` `handleChatCompletions` after `cid` is resolved (whether from header, merge, or freshly minted) | Info | "conversation received" | `conversation_id`, `request_id`, `principal_id`, `clientModel`, `stream`, `tenant`, `project`, `flavor`, `cid_source` (`header` / `merge` / `generated`) |
| `conversation.merged` | `internal/conversationmerge/service.go` `Resolve` when an existing candidate matches | Info | "conversation matched" | `conversation_id`, `request_id`, `principal_id`, `match_score`, `candidate_count`, `merge_reason` (`semantic` / `sticky`) |
| `conversation.dedup_hit` | `internal/conversationmerge/service.go` `Resolve` when dedup cache returns the previous body | Info | "conversation dedup hit" | `conversation_id`, `request_id`, `principal_id`, `dedup_bytes` |
| `conversation.routing.resolved` | `internal/chat/chat.go` `WithVirtualModelFallback` after the model that will be tried is decided | Info | "conversation routed" | `conversation_id`, `request_id`, `upstreamModel`, `attempt`, `chainLen`, `stream` |
| `conversation.rag.attached` | `internal/server/server.go` after `rag.InjectSystemMessage` | Info | "conversation RAG attached" | `conversation_id`, `request_id`, `tenant`, `project`, `flavor`, `hits`, `collection` |
| `conversation.rag.skipped` | `internal/server/server.go` when RAG is enabled but `q == ""` or hits == 0 | Debug | "conversation RAG skipped" | `conversation_id`, `request_id`, `reason` (`empty_query` / `no_hits` / `disabled`) |
| `conversation.upstream.started` | `internal/chat/chat.go` `proxyChatCompletionPayload` just before HTTP request | Info | "conversation upstream started" | `conversation_id`, `request_id`, `upstreamModel`, `stream`, `outgoingTokens` |
| `conversation.upstream.completed` | `internal/chat/chat.go` `logUpstreamChatResponse` on 2xx | Info | "conversation upstream completed" | `conversation_id`, `request_id`, `upstreamModel`, `statusCode`, `usagePromptTokens`, `usageCompletionTokens`, `usageTotalTokens`, `responseBytes` |
| `conversation.upstream.failed` | `internal/chat/chat.go` `logUpstreamChatResponse` on 4xx/5xx, **and** `chat.bifrost.error` path | Warn | "conversation upstream failed" | `conversation_id`, `request_id`, `upstreamModel`, `statusCode`, `err` |
| `conversation.fallback.attempted` | `internal/chat/chat.go` `WithVirtualModelFallback` retry log (currently `chat.routing.fallback`) | Info | "conversation fallback attempted" | `conversation_id`, `request_id`, `upstreamModel`, `prev_status`, `attempt`, `chainLen` |
| `conversation.fallback.exhausted` | `internal/chat/chat.go` exhaustion path (currently writes `gateway_exhausted` JSON error) | Warn | "conversation fallback exhausted" | `conversation_id`, `request_id`, `chainLen`, `excluded_413_count` |
| `conversation.delivered` | `internal/server/server.go` after the response writer finishes (success path) | Info | "conversation delivered" | `conversation_id`, `request_id`, `statusCode`, `stream`, `bytes`, `total_ms` |
| `conversation.errored` | `internal/server/server.go` final write on error response | Warn | "conversation errored" | `conversation_id`, `request_id`, `statusCode`, `errorType` |
| `conversation.merge.failed` | `internal/conversationmerge/service.go` any failure path before resolve completes | Warn | "conversation merge failed" | `request_id` (the line is emitted **before** `conversation_id` is set; UI ties via `request_id`), `step` (`embed` / `dim_mismatch` / `list_candidates` / `dedup_read` / `upsert` / `snapshot_upsert`), `err` |

### Tag propagation requirements

Every existing slug that already participates in a chat (`chat.bifrost.*`, `rag.*`, `chat.routing.*`, `ingest.*`) **must** carry `conversation_id` + `request_id` + `principal_id` in addition to its existing fields. This is largely true today via `routeLog.With(...)` in `handleChatCompletions`, but P2 plugs the remaining holes (notably the RAG `Retrieve` debug lines that already carry `request_id` / `conversation_id`, and the chat handler's `conversation merge resolve failed` Debug that does not carry the request id).

---

## Routing rules (UI fan-out into conversation cards)

The conversation grouping in `renderSummarizedUnified()` is extended as follows.

### Tier 1 — direct match

A line joins conversation `(principal_id, conversation_id)` if its flat fields contain **both** keys (today's behavior, unchanged). This is the primary path and covers all gateway-emitted lines once P2 lands.

### Tier 2 — request_id join

A line joins conversation `(principal_id, conversation_id)` if its flat fields contain `request_id` **and** the cache already contains a Tier-1 line with the same `request_id` mapped to that conversation. Implementation: maintain an in-memory `requestIdToConv` map updated as lines arrive; when a later line shows up with `request_id` only, look it up and tag the line accordingly.

### Tier 3 — index_run_id join (ingest only)

When `ingest.complete` lines carry both `request_id` (chat-originated ingest) and `index_run_id`, subsequent indexer / qdrant lines bearing the same `index_run_id` may join the conversation **as ingest activity**. This is opt-in per UI (otherwise the indexer card stays the canonical home).

### Tier 4 — collection + time window (qdrant fallback)

When a conversation has at least one `rag.query` / `rag.embed` line carrying `collection` (already true today), Qdrant subprocess HTTP lines (`qdrant.http.collection_meta`, `qdrant.http.points_upsert_ok`, `qdrant.http.points_delete`, `qdrant.http.vector_search`) may be **annotated** with the conversation id when:

- the qdrant line's `collection` matches the conversation's `collection`, **and**
- the qdrant line's timestamp falls within ±**5 s** of the conversation's `rag.query` line.

The annotated qdrant line still appears in the qdrant card (existing behavior) and **also** gets a tier-4 entry under the conversation's RAG sub-section. UI marks it visually as "inferred" so operators know it was joined heuristically.

### Tier 5 — bifrost subprocess (best-effort, P5 only)

Today bifrost subprocess lines carry no conversation hint. P5 explores propagating an outgoing `X-Claudia-Conversation-Id` header on the upstream HTTP call so bifrost-emitted JSON includes it; if BiFrost echoes it (today: unverified) the line gets Tier-1 status. Otherwise, the line stays bifrost-only and the conversation card uses **gateway-emitted** `chat.bifrost.*` slugs as the canonical "BiFrost activity in this conversation" view.

---

## UI contract: Conversation card (summarized logs)

Applies to the **Conversations** section of the summarized log view (`buildConvCard`, `renderExpandedConv`).

### Collapsed card header

Today: `<duration>` · `<tok> tok` · `<vec> vec` + binary `error / active` status.

**Add** four pill chips (left of the Duration metric, before the existing token / vector chips):

| Pill | Behavior |
|------|----------|
| **State** | Computed from the latest `conversation.*` lifecycle slug seen for this conversation. Values: **`received`** · **`routing`** · **`rag`** · **`upstream`** · **`delivered`** · **`failed`**. Color matches qdrant card semantics: blue/active for in-flight, green for delivered, red for failed. |
| **BiFrost** | `chat.bifrost.request` count → `chat.bifrost.response` count, with `chat.bifrost.error` shown as fail. Tooltip: "n requests · m responses · k errors". |
| **Qdrant** | Count of `rag.query` + tier-4-joined qdrant HTTP rows. Tooltip lists the collection name. |
| **Fallback** | Count of `conversation.fallback.attempted`. Hidden when 0. Tooltip lists the model attempted at each step. |

### Expanded card — below the summary heading (KV row)

Replace today's three mini-cards (Token count / Duration / Vectors retrieved) with a **KV row + counter row**:

KV row:

| Key | Source `msg` / logic |
|-----|---------------------|
| **principal** | `principal_id` (already used in card title) — keep token label fallback via `tokenLabelByTenant`. |
| **client model** | `conversation.received` `clientModel`. |
| **upstream model** | latest `conversation.routing.resolved` or `conversation.upstream.started` `upstreamModel`. |
| **stream** | `conversation.received` `stream` (true/false → `SSE` / `JSON`). |
| **RAG collection** | `conversation.rag.attached` `collection` (or first `rag.query` `collection`). |
| **merge** | `conversation.merged` → `matched (score)` · `conversation.dedup_hit` → `dedup` · else `new`. |
| **state** | mirrors the State pill (also available in expand for screen readers). |

Counter row:

| Box | Behavior |
|-----|----------|
| **Tokens (out → usage)** | Sum `outgoingTokens` (from `chat.bifrost.request` / `conversation.upstream.started`) → sum `usageTotalTokens`. Shown identically to the bifrost card so numbers reconcile. |
| **Vectors retrieved** | Sum `hits` from `conversation.rag.attached` (preferred) or `rag_hits` / `hits` / `chunks` (today's heuristic, kept as fallback). |
| **Duration** | `conversation.delivered.total_ms` when present, else `convWindowMs` (today's behavior). |
| **Fallbacks · errors** | `conversation.fallback.attempted` count · `conversation.errored` + `conversation.upstream.failed` count. |

### Progress bar

Below the KV / counter rows, render a **5-step progress bar** using the lifecycle slugs:

`received → routed → (rag) → upstream → delivered`

- Each step lights when its slug appears for this conversation; **rag** is dimmed (skipped) when `conversation.rag.skipped` fires before `conversation.upstream.started`.
- The bar tints **error** when `conversation.upstream.failed` or `conversation.errored` is present after `conversation.upstream.started`.
- Implementation: extend `derive/conversationMetrics.js` (or split into `derive/conversationCardModel.js`) to expose `{ state, steps[], progressIndex, errorAt }` for the renderer.

### Full event log (expanded)

- Keep the existing `<details>` with per-row `buildDetailsColumn` (lossless detail).
- **Add** badges per row indicating which **service** the line came from when not the gateway: **bifrost** / **qdrant** / **indexer**. Today the row badge is generic — use `inferServiceBadge` already in `logs.js`.
- **Add** a "tier" indicator (subtle muted text) when the line is tier-4 (qdrant heuristic join) so the operator sees that it is inferred.

---

## Phased implementation

### P1 — Spec

**Goal.** Land this doc, capture fixtures, freeze the `conversation.*` slug list.

**Deliverables**

- This file checked in.
- Four fixtures captured to `temp/conversation-*.log`.
- Confirm the routing tiers correctly classify every line in the fixtures (by hand or via a small Python helper).

**Acceptance.** Reviewer agrees every line in each fixture maps to a tier (1/2/3/4/5) and every gateway-emitted line maps to either an existing slug or a new `conversation.*` slug.

**Status:** `todo`

### P2 — Correlation propagation

**Goal.** Every gateway line that touches a chat request carries the full correlation triple (`request_id` + `conversation_id` + `principal_id`).

**Deliverables**

- Audit script (`scripts/audit-correlation.sh` or a Go test) that grep-checks every `slog` call in `internal/chat`, `internal/rag`, `internal/server`, `internal/conversationmerge` and flags any that omit the triple when the call site is inside a chat-handler scope.
- Edits to plug holes:
  - `internal/server/server.go` `handleChatCompletions`: pass `request_id` to `conversationmerge.Resolve` and have it emit failures with `request_id` (so tier-2 join can find them).
  - `internal/conversationmerge/service.go`: include `request_id` (when supplied) on every Warn/Debug emitted from `Resolve`. Re-emit a `conversation.merge.failed` line with `request_id` after a soft fallback to a fresh id.
  - `internal/chat/chat.go`: ensure `prepareChatPayload` debug, `chat blocked by provider limits`, `skipping upstream model (provider limits)`, and `provider limits admission query failed` carry the With-context.
  - `internal/server/ingest.go` / `ingest_session.go`: ensure `request_id`, `index_run_id`, and (when chat-originated) `conversation_id` are present on `ingest.complete` and `ingest.failed`.

**Acceptance.** The audit script reports zero violations; running the four fixtures shows every relevant line tier-1 or tier-2 joinable to the right conversation.

**Status:** `todo`

### P3 — Lifecycle events

**Goal.** Gateway emits the full `conversation.*` lifecycle.

**Deliverables**

- `internal/server/server.go`:
  - Emit `conversation.received` after `cid` is resolved (with `cid_source`).
  - Emit `conversation.rag.attached` after `rag.InjectSystemMessage` (with `hits`, `collection`).
  - Emit `conversation.rag.skipped` when RAG runs but produces 0 hits or empty query.
  - Emit `conversation.delivered` from a small response wrapper (or via existing `wrapResponse` in `internal/server/server.go`) with `total_ms`, `bytes`, `statusCode`, `stream`.
  - Emit `conversation.errored` from the JSON error writer paths.
- `internal/chat/chat.go`:
  - Emit `conversation.routing.resolved` (replacing/augmenting the existing `virtual model routing resolved`).
  - Emit `conversation.upstream.started` immediately before HTTP request.
  - Emit `conversation.upstream.completed` from `logUpstreamChatResponse` on 2xx (in addition to the existing `chat.bifrost.response`).
  - Emit `conversation.upstream.failed` on 4xx/5xx and on the `chat.bifrost.error` path.
  - Emit `conversation.fallback.attempted` (replacing/augmenting `chat.routing.fallback`).
  - Emit `conversation.fallback.exhausted` from `WithVirtualModelFallback` exhaustion path.
- `internal/conversationmerge/service.go`:
  - Emit `conversation.merged` on a successful candidate match.
  - Emit `conversation.dedup_hit` on a dedup cache hit.
  - Emit `conversation.merge.failed` (with `step`, `err`, `request_id`) on the soft-fallback paths that already log Warn/Debug today.

**Acceptance.** The four fixtures from P1 contain at least one of each lifecycle slug (across the four scenarios combined); the State pill in the conversation card transitions through the expected path.

**Status:** `todo`

### P4 — UI fan-out & conversation card

**Goal.** Conversation card pulls fan-out lines and renders pills / KV / counters / progress as specified.

**Deliverables**

- Extend `renderSummarizedUnified()` in [`internal/server/embedui/logs.js`](../../internal/server/embedui/logs.js) with the tier-2 (`request_id`) and tier-4 (qdrant collection + time window) joins. Tier-3 (`index_run_id`) is a follow-up.
- Replace `derive/conversationMetrics.js` (or add a new `derive/conversationCardModel.js`) that returns `{ state, kv, counters, steps[], progressIndex, errorAt }` from the cluster of events.
- Update `buildConvCard` and `renderExpandedConv` to render the new pills, KV row, counter row, and progress bar.
- Add CSS for the State pill colors and the progress bar (mirror the qdrant card styling for consistency).
- Refresh `internal/server/logs_components_test.go` and `internal/server/ui_logs_test.go`; add a derive test under `internal/server/embedui/logs/derive/` (goja) that asserts the lifecycle → state mapping.

**Acceptance.** Loading a fixture into the UI shows: pills correctly counting BiFrost / Qdrant / Fallback events, KV row populated for principal / client model / upstream model / stream / RAG collection / merge / state, counter row matching the bifrost card numbers within the same window, and the progress bar lighting through `received → routed → rag → upstream → delivered`.

**Status:** `todo`

### P5 — Subprocess linkage hardening

**Goal.** Best-effort tier-1 join for bifrost subprocess lines when the upstream call is conversation-scoped.

**Deliverables**

- Investigate whether BiFrost echoes a custom request header in its log output (read `bifrost-http` source). Document findings in [`bifrost-discovery.md`](../bifrost-discovery.md).
- If echo is supported: `internal/chat/chat.go` adds `X-Claudia-Conversation-Id` (and `X-Claudia-Request-Id`) to the upstream `http.NewRequestWithContext` call; verify `bifrostline.NormalizePayload` (from [`log-bifrost.md`](log-bifrost.md) P2) propagates the field into the normalized JSON.
- If echo is **not** supported: document the gap, accept that `chat.bifrost.*` (gateway-emitted) remains the canonical conversation view of BiFrost activity, and propose an upstream BiFrost feature request.

**Acceptance.** Either bifrost subprocess lines join the conversation card via tier-1 with the new headers, or a clearly written gap note in `bifrost-discovery.md` plus an upstream tracking issue.

**Status:** `todo`

---

## Open questions

- **Cross-link UX:** Should clicking a tier-4 (qdrant) row inside a conversation card jump to the qdrant card (and vice versa)? Default proposal: yes, via the existing `?seq=` focus mechanism.
- **Time window for tier-4:** ±5 s is a starting point; revisit after fixture analysis (some upstream calls may dilate >5 s under load).
- **Merge state visibility:** The `merge` KV value should distinguish "matched a previous conversation" from "deduped (returned cached body, did not call upstream)" — covered by `conversation.merged` vs `conversation.dedup_hit`, but verify the operator-facing copy reads cleanly.
- **Failed conversation id:** When `conversation.merge.failed` fires, the line carries `request_id` but the conversation id may not be known yet. Tier-2 join handles this once the next line carries `conversation_id` + `request_id`. Confirm there is always such a follow-up in practice.
- **Locale:** English-only operator strings (matches qdrant + indexer + bifrost + gateway).

---

## Checklist before marking done

- [ ] Every gateway line touching a chat request carries `request_id` + `conversation_id` + `principal_id` (audit script clean).
- [ ] All `conversation.*` lifecycle slugs emitted at the points specified in the taxonomy.
- [ ] Conversation card shows the **State** pill, BiFrost / Qdrant / Fallback chips, KV row, counter row, and 5-step progress bar.
- [ ] Tier-2 (`request_id`) and tier-4 (collection + time window) joins implemented; tier-5 (bifrost subprocess) implemented or documented as a known gap.
- [ ] Fixture-backed derive test under `internal/server/embedui/logs/derive/` covers lifecycle → state mapping.
- [ ] [`log-presentation-layer.md`](log-presentation-layer.md) §10 changelog updated.
