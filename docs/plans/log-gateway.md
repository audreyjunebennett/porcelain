# Plan: Operator-facing Gateway log classification

| Field | Value |
|-------|-------|
| **Doc kind** | `feature-plan` |
| **Owners / areas** | Gateway core (`internal/server`, `internal/chat`, `internal/rag`, `internal/routing`, `internal/conversationmerge`, `internal/tokens`, `internal/upstream`, `internal/config`), logs UI (`internal/server/embedui/logs`), parse/derive (`internal/server/embedui/logs/parse`, `derive`) |
| **Status** | `draft` |
| **Targets** | Gateway parent process (`cmd/claudia serve`) — structured `slog` only |
| **Last updated** | 2026-05-07 |

## At a glance

The gateway is the only **first-party** process in the stack. Its logs already drive the conversation, indexer, and bifrost cards through a handful of dotted slugs (`chat.bifrost.*`, `rag.*`, `ingest.*`, `indexer.*`). But many gateway lines still ship **without `msg`**, several are **logged at the wrong level** for an operator (Info noise, Warn that is benign, Debug that hides important state), and the gateway service card itself shows raw counters instead of operator-meaningful state. This plan captures:

1. A **stable `gateway.*` taxonomy** for non-domain-specific lines, plus tightened existing slugs (`chat.*`, `rag.*`, `ingest.*`, `routing.*`, `tokens.*`, `config.*`, `conversation.*`).
2. **Log-level reclassification** so the **default** stream (Info+) tells the operator story without being drowned by per-request debug.
3. **New structured objects** the UI needs but the gateway does not currently emit (e.g. `gateway.startup.listening`, `gateway.config.reloaded`, `gateway.health.upstream`, `tokens.reloaded`).
4. **Reformatted human messages** so the **headline** (passed as the first arg to `slog.Info`) actually summarizes the event for the operator, not the developer.
5. **Demotion / retirement** of low-value messages that crowd the buffer.

**Related docs:** [`supervisor.md`](../supervisor.md), [`log-presentation-layer.md`](log-presentation-layer.md), [`log-qdrant.md`](log-qdrant.md), [`log-bifrost.md`](log-bifrost.md), [`log-conversations.md`](log-conversations.md), [`docs/indexer.md`](../indexer.md).

| Phase | Outcome | Status |
|-------|---------|--------|
| [P1 — Inventory & spec](#p1--inventory--spec) | This doc + frozen `gateway.*` list and a per-line audit table | `todo` |
| [P2 — `msg` slugs everywhere](#p2--msg-slugs-everywhere) | Every gateway `slog` call carries `msg`; existing slugs renamed for consistency | `todo` |
| [P3 — Level reclassification](#p3--level-reclassification) | Default Info stream tells the operator story; dev noise demoted to Debug; benign Warns demoted to Info | `todo` |
| [P4 — New gateway objects](#p4--new-gateway-objects) | New structured events for startup, listening, config reload, token reload, upstream/qdrant/bifrost health | `todo` |
| [P5 — Card UI cleanup](#p5--card-ui-cleanup) | Gateway service card replaces generic counters with operator KV + counters | `todo` |
| [P6 — Demote / retire](#p6--demote--retire) | Low-value lines demoted or removed; buffer density improves | `todo` |

---

## Background

- The gateway uses a single `*slog.Logger` built in `cmd/claudia/serve.go` via `buildLoggerTo(...)`. Every line is JSON (or `key=value` fallback) and lands in `internal/servicelogs.New(...)` under source **`gateway`**.
- A few hot paths carry stable slugs:
  - `internal/chat/chat.go` — `chat.bifrost.request`, `chat.bifrost.error`, `chat.routing.fallback`, plus the un-slugged Info `upstream chat response` (rename tracked in [`log-bifrost.md`](log-bifrost.md) P4).
  - `internal/server/server.go` — `chat.request`, `rag.retrieve.error`, `rag.retrieve.ok`.
  - `internal/server/ingest.go` and `internal/server/ingest_session.go` — `ingest.complete`, `ingest.chunked.error`.
  - `internal/rag/service.go` — `rag.ingest.trace`, `rag.query`, `rag.embed`, `rag.hit`.
  - `internal/indexer/*` — `indexer.run.start`, `indexer.run.done`, `indexer.discovery.summary`, `indexer.queue.snapshot`, `indexer.recovery.poll`, `indexer.recovery.resumed`, `indexer.scope.status`, `indexer.scope.active_file` (these stay owned by the indexer doc).
- **Many** other gateway lines ship without `msg`: token reloads, config reloads, listen/banner messages, conversation merge debug, provider-limit decisions, upstream health probes, smoke chat, UI session errors, supervisor child startup messages, etc. The UI cannot route or rollup these without a slug.
- The current **gateway service card** (`gatewayServicePanelMiniHtml` in `internal/server/embedui/logs.js`) shows three generic mini-cards: **HTTP · Σ ms**, **ingest.complete · RAG · chat slugs**, **Warn+error lines**. None of those answer "is the gateway listening, is upstream reachable, are tokens loaded, what config is in effect?".

---

## Locked decisions (proposed — confirm during P1)

| Topic | Decision |
|-------|-----------|
| Slug prefix | **`gateway.*`** for parent-process / lifecycle lines that are **not** domain-specific. Domain prefixes stay: `chat.*`, `rag.*`, `ingest.*`, `routing.*`, `tokens.*`, `config.*`, `conversation.*`, `indexer.*` (owned), `qdrant.*` (owned by `log-qdrant.md`), `bifrost.*` (owned by `log-bifrost.md`). |
| Casing & separators | Lower-snake **dotted** slugs (`gateway.startup.listening`), aligning with `qdrant.*` / `indexer.*` precedent. |
| Headline rewrite | The **first arg** to `slog.Info/Warn/Error` is rewritten as a **short operator sentence** (e.g. `"gateway listening"` not `"claudia serve: gateway listening"`); structured fields carry the detail. Avoid logger-prefix duplication when the slug already conveys the kind. |
| Levels | **Info** = operator-relevant state changes & per-request milestones at low volume. **Warn** = degraded but auto-handled. **Error** = user-visible failure or data loss. **Debug** = developer-only / per-line traces. **Trace** (via `platform.LevelTrace`) reserved for ingest body excerpts and per-hit RAG. |
| Backwards compat | When renaming a slug used by the UI today, accept **both** old and new names in derive modules for **one release window**, then remove the alias. |
| New objects | New `gateway.*` objects start at **`level:"INFO"`** unless they are pure debug; emit them at **first observation** per process start (e.g. `gateway.startup.listening` is one-shot, not periodic). |

### Code references

- Slugs today (sample audit): [`internal/chat/chat.go`](../../internal/chat/chat.go), [`internal/server/server.go`](../../internal/server/server.go), [`internal/server/ingest.go`](../../internal/server/ingest.go), [`internal/server/ingest_session.go`](../../internal/server/ingest_session.go), [`internal/rag/service.go`](../../internal/rag/service.go), [`internal/indexer/*`](../../internal/indexer/), [`internal/upstream/upstream.go`](../../internal/upstream/upstream.go), [`internal/tokens/tokens.go`](../../internal/tokens/tokens.go), [`internal/config/config.go`](../../internal/config/config.go), [`internal/conversationmerge/service.go`](../../internal/conversationmerge/service.go), [`internal/server/runtime.go`](../../internal/server/runtime.go).
- UI: [`internal/server/embedui/logs.js`](../../internal/server/embedui/logs.js) (`gatewayServicePanelMiniHtml`, `entryIsGatewayUpstreamRelay`, summarized panel routing).

## Reference samples (local dev)

Capture during P1 to repo-root **`temp/`** (gitignored):

| Artifact | Path | Purpose |
|----------|------|---------|
| Cold-start gateway log | `temp/gateway-cold-start.log` | Listen, config load, tokens load, upstream probe, supervisor children attach |
| Mixed traffic | `temp/gateway-mixed.log` | A few `/v1/chat/completions` (success, fallback, 429), a `/v1/embeddings` ingest, a token reload, a config reload |
| Warn / error scenarios | `temp/gateway-degraded.log` | Missing upstream key, RAG init failure, 5xx from upstream, conversation merge embed failure |

---

## Canonical `msg` taxonomy

Stable dotted slugs across gateway-emitted lines. Every `slog.Info/Warn/Error/Debug` call should set `msg` (when it doesn't exist today, P2 adds it).

### Lifecycle / process (`gateway.*`)

| `msg` | Source today (or "**new**") | Level | Notes |
|-------|------------------------------|-------|-------|
| `gateway.startup.config_resolved` | `internal/config/config.go` `resolved gateway config paths` | **Info** *(was Debug)* | Headline: **"gateway config resolved"**. KV: `filePath`, `tokensPath`, `routingPolicyPath`. |
| `gateway.startup.bootstrap` | `cmd/claudia/serve.go` bootstrap mode notice | Info | Headline: **"gateway bootstrap mode"**. KV: `tokens_path`. |
| `gateway.startup.listening` | `cmd/claudia/serve.go` `claudia serve: gateway listening` | Info | Headline: **"gateway listening"**. KV: `addr`, `ui`, `upstream`, `bifrost_data`, `qdrant_supervised`, `indexer_supervised`, `config`. |
| `gateway.startup.disk_log` | `cmd/claudia/serve.go` `disk logging enabled` | Info | KV: `path`. |
| `gateway.shutdown.http` | `cmd/claudia/serve.go` `http shutdown` | Warn → **Info** | Headline: **"gateway http shutdown"**. KV: `err` (optional). |
| `gateway.shutdown.child_force_kill` | `cmd/claudia/serve.go` `did not exit after context cancel; forcing kill` | Warn | Keep level. KV: `name`, `pid`, `timeout`. |
| `gateway.config.reloaded` | `internal/server/runtime.go` `reloaded gateway.yaml` | Info | KV: `path`. |
| `gateway.config.reload_failed` | `internal/server/runtime.go` `failed to reload gateway.yaml` | **Error** *(was Error — keep)* | KV: `path`, `err`. |
| `gateway.config.missing` | `internal/server/runtime.go` `gateway config missing` | Error | KV: `path`, `err`. |
| `gateway.metrics.disabled_after_error` | `internal/gatewaymetrics/store.go` `gateway metrics disabled after write error` | Error | KV: `step`, `err`. |
| `gateway.metrics.init_failed` | `internal/server/runtime.go` `gateway metrics init failed` | **Warn** *(was Error — non-fatal, gateway continues)* | KV: `err`. |
| `gateway.metrics.migration_applied` | `internal/gatewaymetrics/migrate.go` `gateway metrics migration applied` | Info | KV: `version`, `file`. |

### Tokens / auth (`tokens.*`)

| `msg` | Source today | Level | Notes |
|-------|--------------|-------|-------|
| `tokens.reloaded` | `internal/tokens/tokens.go` `reloaded gateway API tokens` | Info | Headline: **"tokens reloaded"**. KV: `path`, `count`. |
| `tokens.file_missing` | `internal/tokens/tokens.go` `tokens file missing` | Error | KV: `path`, `err`. |
| `tokens.read_failed` | `internal/tokens/tokens.go` `read tokens yaml` | Error | KV: `path`, `err`. |
| `tokens.parse_failed` | `internal/tokens/tokens.go` `failed to parse tokens yaml` | Error | KV: `path`, `err`. |
| `tokens.append_failed` | `internal/server/ui_tokens.go` / `internal/server/ui_bootstrap.go` `append token` / `setup append token` | Error | KV: `err`. |
| `tokens.upstream_api_key.autogen` | `internal/config/upstream_api_key.go` `wrote auto-generated upstream.api_key to gateway.yaml` | Info | KV: `path`. |

### Routing / providers / chat (`routing.*`, `chat.*`)

| `msg` | Source today | Level | Notes |
|-------|--------------|-------|-------|
| `routing.policy.reloaded` | `internal/routing/routing.go` `reloaded routing policy` | Info | KV: `path`, `rules`. |
| `routing.policy.read_failed` | `internal/routing/routing.go` `read routing policy` | Error | KV: `path`, `err`. |
| `routing.policy.parse_failed` | `internal/routing/routing.go` `failed to parse routing policy yaml` | Error | KV: `path`, `err`. |
| `routing.policy.missing` | `internal/routing/routing.go` `routing policy file missing` | Error | KV: `path`, `err`. |
| `routing.fallback_chain.empty` | `internal/config/config.go` `routing.fallback_chain is empty or missing` | Warn | KV: none. |
| `routing.rule.matched` | `internal/routing/routing.go` `routing rule matched` | **Debug** *(keep)* | KV: `rule`, `initialModel`, `lastUserChars`. |
| `routing.rule.no_match` | `internal/routing/routing.go` `routing: no rule matched, using ambiguous_default_model` / `no policy default; using first fallback_chain entry` | Debug | Two distinct slugs preferred: `routing.rule.no_match.ambiguous_default` and `routing.rule.no_match.first_fallback`. |
| `chat.request` | `internal/server/server.go` `chat completion request` | Info | Keep slug; ensure `conversation_id`, `principal_id`, `request_id` present (already added by `routeLog.With(...)`). |
| `chat.bifrost.request` | `internal/chat/chat.go` `upstream chat relay` | Info | Keep slug; covered by [`log-bifrost.md`](log-bifrost.md). |
| `chat.bifrost.response` | `internal/chat/chat.go` `upstream chat response` (slug today is **only** human headline — needs `msg`) | Info | **P2/P4 add `msg`**; covered by [`log-bifrost.md`](log-bifrost.md). |
| `chat.bifrost.error` | `internal/chat/chat.go` `upstream chat fetch failed` | Info → **Warn** | Promote to Warn (failure operator can act on); covered by [`log-bifrost.md`](log-bifrost.md). |
| `chat.routing.fallback` | `internal/chat/chat.go` `retrying next fallback model` | Info | Keep slug. |
| `chat.routing.attempt` | `internal/chat/chat.go` `virtual model fallback attempt` (no slug today) | **Debug** *(was Info when chain>1)* | Demote unless final attempt; covered by [`log-bifrost.md`](log-bifrost.md). |
| `chat.routing.resolved` | `internal/chat/chat.go` `virtual model routing resolved` (no slug today) | Info | **P2 adds slug**; covered by [`log-bifrost.md`](log-bifrost.md). |
| `chat.provider_limits.blocked` | `internal/chat/chat.go` `chat blocked by provider limits` / `skipping upstream model (provider limits)` (no slug) | Info | **P2 adds slug**; covered by [`log-bifrost.md`](log-bifrost.md). |
| `chat.provider_limits.query_failed` | `internal/chat/chat.go` `provider limits admission query failed` | Warn | KV: `err`, `upstreamModel`. |
| `chat.provider_limits.config_invalid` | `internal/config/config.go` `provider-model-limits.yaml invalid` / `provider free tier yaml invalid` | Error | KV: `path`, `err`. |
| `chat.provider_limits.config_missing` | `internal/config/config.go` `provider free tier path not stat-able` / `routing.filter_free_tier_models is true but provider-free-tier.yaml missing` | Warn | KV: `path` (when known). |

### RAG / ingest (`rag.*`, `ingest.*`)

| `msg` | Source today | Level | Notes |
|-------|--------------|-------|-------|
| `rag.config.invalid` | `internal/config/config.go` `rag config invalid; disabling RAG` | Error | KV: `err`. |
| `rag.retrieve.error` | `internal/server/server.go` `rag retrieve failed; proceeding without context` | Warn | Keep. |
| `rag.retrieve.ok` | `internal/server/server.go` `rag context injected` | **Debug** *(keep)* | KV unchanged. |
| `rag.query` | `internal/rag/service.go` `rag search query` | Debug | KV unchanged. |
| `rag.embed` | `internal/rag/service.go` `rag embedding retrieved` | Debug | KV unchanged. |
| `rag.hit` | `internal/rag/service.go` `rag comparison` | Debug | KV unchanged. |
| `rag.ingest.trace` | `internal/rag/service.go` `rag ingest` | Trace | Keep. |
| `rag.ingest.delete_pre_failed` | `internal/rag/service.go` `delete-by-source pre-ingest failed` | Debug | Keep. |
| `ingest.complete` | `internal/server/ingest.go` / `ingest_session.go` | Info | Keep slug; ensure both call sites carry the same KV (`bytes`, `chunks`, `source`, `request_id`, `index_run_id`). |
| `ingest.failed` | `internal/server/ingest.go` `ingest failed` (no slug today) | Error | **P2 adds slug**. KV: `err`, `source`. |
| `ingest.chunked.error` | `internal/server/ingest_session.go` | Error | Keep. |
| `ingest.chunked.failed` | `internal/server/ingest_session.go` `chunked ingest failed` (no slug) | Error | **P2 adds slug**; deduplicate with `ingest.chunked.error`. |

### Conversation merge (`conversation.*`)

| `msg` | Source today | Level | Notes |
|-------|--------------|-------|-------|
| `conversation.merge.disabled` | `internal/conversationmerge/service.go` `conversation merge disabled: missing embedding URL or upstream API key` | Warn → **Info** *(operator decision, not a fault)* | KV: none. |
| `conversation.merge.embed_failed` | `internal/conversationmerge/service.go` `conversation merge: embed failed` | Warn | KV: `err`. |
| `conversation.merge.embed_dim_mismatch` | `internal/conversationmerge/service.go` `embedding dim mismatch` | Warn | KV: `got`, `want`. |
| `conversation.merge.list_candidates_failed` | `internal/conversationmerge/service.go` `list candidates failed` | Warn | KV: `err`. |
| `conversation.merge.dedup_read_failed` | `internal/conversationmerge/service.go` `dedup read failed` | Debug | KV: `err`. |
| `conversation.merge.upsert_failed` | `internal/conversationmerge/service.go` `upsert failed` | Warn | KV: `err`. |
| `conversation.merge.snapshot_upsert_failed` | `internal/conversationmerge/service.go` `resolve snapshot upsert failed` | Warn | KV: `err`, `conversation_id`. |
| `conversation.merge.dedup_cache_write_failed` | `internal/conversationmerge/service.go` `dedup cache write failed` | Debug | KV: `err`. |
| `conversation.merge.disabled_no_metrics` | `internal/config/config.go` `conversation_merge.enabled requires metrics.enabled; disabling` | Warn | KV: none. |

### Upstream / health (`upstream.*` — new namespace for gateway-side probes)

| `msg` | Source today | Level | Notes |
|-------|--------------|-------|-------|
| `upstream.models.fetch_failed` | `internal/upstream/upstream.go` `upstream models fetch failed` | Info → **Warn** | KV: `err`, `target`. |
| `upstream.models.non_ok` | `internal/upstream/upstream.go` `upstream models non-OK` | Info → **Warn** | KV: `status`, `target`. |
| `upstream.models.ok` | `internal/upstream/upstream.go` `upstream models` (Debug today) | Debug | Keep. |
| `upstream.health.probe_failed` | `internal/upstream/upstream.go` `upstream health probe failed` | Info → **Warn** | KV: `err`, `target`. |
| `upstream.smoke_chat.failed` | `internal/upstream/smoke_chat.go` `smoke chat completion failed` | Info → **Warn** | KV: `err`, `target`. |

### UI / sessions

| `msg` | Source today | Level | Notes |
|-------|--------------|-------|-------|
| `ui.session.error` | `internal/server/ui_handlers.go` `ui session issue` | **Debug** *(was Error — most are benign cookie reissue)* | Keep Error path only when persistence write actually fails. |
| `ui.tokens.append_failed` | `internal/server/ui_tokens.go` `append token` | Error | Keep. |
| `ui.bootstrap.append_failed` | `internal/server/ui_bootstrap.go` `setup append token` | Error | Keep. |

### HTTP access (`http.access`) — already shape-detected

`internal/server/server.go` `loggingMiddleware` emits `http response` lines that the UI shape-detects via `method`+`path`+`statusCode`. Keep behavior; **add** `msg: "http.access"` so filters and rollups can target the slug instead of inferring shape. Demote noisy probe paths (`/health`, `/status`, `/api/ui/logs`, SSE) to **Debug** when status is 2xx.

---

## UI contract: Gateway service card (summarized logs)

Applies to **Logs → Gateway** summary card (`gatewayServicePanelMiniHtml` + `buildServiceCard` + `renderExpandedService` for `name === "gateway"`).

### Collapsed card header

- **Replace** the generic last-message subtitle with a **gateway-aware** subtitle priority:
  1. recent `gateway.config.reload_failed` / `gateway.config.missing` / `tokens.parse_failed` (Error tint).
  2. recent `upstream.health.probe_failed` / `upstream.models.fetch_failed` (Warn tint).
  3. last `gateway.config.reloaded` or `tokens.reloaded` (operator-state change).
  4. last `gateway.startup.listening` (cold state).
  5. fallback to `primaryLogMessage(last.parsed, last.text)` (today's behavior).

### Expanded card — below the summary heading (KV row)

New **key-value** fields:

| Key | Source `msg` / logic |
|-----|---------------------|
| **listening** | `gateway.startup.listening` → `addr` (last value wins). |
| **upstream** | `gateway.startup.listening` → `upstream` (Bifrost URL). |
| **config** | `gateway.config.reloaded` / `gateway.startup.config_resolved` → `path` short label + a tiny indicator if a reload error has happened since last success. |
| **tokens** | `tokens.reloaded` → `count`; tint **error** if last token-related event was a parse/read failure. |
| **routing rules** | `routing.policy.reloaded` → `rules`. |
| **supervised children** | derived from `gateway.startup.listening` (`qdrant_supervised`, `indexer_supervised`) plus `bifrost_data` presence. |

### Expanded card — summary section (replace current mini-cards)

**Remove:** `HTTP · Σ ms`, `ingest.complete · RAG · chat slugs`, `Warn+error lines` (today's three).

**Add** four counter boxes:

| Box | Behavior |
|-----|----------|
| **HTTP success / fail** | From `http.access` rows: 2xx vs 4xx/5xx, with a separate `429` sub-count when present. |
| **Chat (req → resp)** | `chat.request` count → `chat.bifrost.response` count, with `chat.bifrost.error` shown as fail. |
| **RAG (queries · hits)** | `rag.query` count · `rag.hit` count (both already slugged); subtitle: latest `rag.retrieve.error` reason when present. |
| **Ingest (ok / fail)** | `ingest.complete` count vs `ingest.failed` + `ingest.chunked.failed`. |

### Full event log (expanded)

- **Suppress** the **gateway** source badge in this panel only (mirror `suppressQdrantBadge` / `suppressIndexerBadge`). Use the existing per-row shape badge to keep distinctness.
- Default filter inside the gateway panel **hides** `http.access` rows for `/api/ui/logs`, `/health`, `/status` (operators can re-enable via the existing level/app filters). Implementation: extend `entryIsGatewayUpstreamRelay` style helpers with a `gatewayPanelHideRow(ent)` predicate.

---

## Phased implementation

### P1 — Inventory & spec

**Goal.** Land this doc and an actionable per-line audit table.

**Deliverables**

- This file, plus a per-package audit (`temp/gateway-log-audit.md`) listing every `slog` call, current headline + slug + level, and the proposed change.
- `temp/gateway-cold-start.log`, `temp/gateway-mixed.log`, `temp/gateway-degraded.log` captured live (sanitize tokens / keys).
- Confirm taxonomy table covers ≥95% of lines in those fixtures.

**Acceptance.** Reviewer can map every line in the three fixtures to a slug from the taxonomy.

**Status:** `todo`

### P2 — `msg` slugs everywhere

**Goal.** Every gateway `slog` call carries `msg`; renames applied where the table changes a slug.

**Deliverables**

- Edits to `internal/chat/chat.go`, `internal/server/server.go`, `internal/server/ingest.go`, `internal/server/ingest_session.go`, `internal/server/runtime.go`, `internal/server/ui_handlers.go`, `internal/server/ui_tokens.go`, `internal/server/ui_bootstrap.go`, `internal/upstream/upstream.go`, `internal/upstream/smoke_chat.go`, `internal/tokens/tokens.go`, `internal/config/config.go`, `internal/conversationmerge/service.go`, `internal/routing/routing.go` — add `"msg", "<slug>"` to every `slog.Info/Warn/Error/Debug` (and `Trace` where used) per the taxonomy.
- Backwards-compat aliasing in [`internal/server/embedui/logs.js`](../../internal/server/embedui/logs.js) and any `derive/` helpers that match on the **headline** today (e.g. `bifrostMetrics.js` matching `"upstream chat response"`).
- `cmd/claudia/serve.go`: add `gateway.startup.*` and `gateway.shutdown.*` slugs to the listening / shutdown lines.

**Acceptance.** A grep for `slog\.(Info|Warn|Error|Debug|Log)\(` in `internal/` and `cmd/claudia/` returns zero hits without a sibling `"msg"` arg (allow-list dev-only `internal/platform` test helpers).

**Status:** `todo`

### P3 — Level reclassification

**Goal.** The default Info+ stream is the operator story; Debug is the developer story.

**Deliverables**

- Apply the **Level** column from the taxonomy table.
- Drop the `loggingMiddleware` Info for 2xx requests on `/api/ui/logs`, `/health`, `/status`, and SSE endpoints — emit at Debug instead. Keep Info for everything else and for any non-2xx.
- Promote `chat.bifrost.error` from Info → Warn (operator-actionable).
- Demote `routing.rule.*` debug lines (already Debug — verify).
- Demote `ui.session.error` from Error → Debug for benign cookie reissue paths; keep Error only when persistence fails.

**Acceptance.** With `LOG_LEVEL=info`, the cold-start fixture produces ≤30 lines (down from current baseline measured in P1) and every line maps to an operator-meaningful taxonomy entry.

**Status:** `todo`

### P4 — New gateway objects

**Goal.** Emit structured events the UI needs but the gateway does not produce today.

**Deliverables**

- `gateway.startup.listening` (replace today's `claudia serve: gateway listening` headline by adding the slug; KV stays the same).
- `gateway.startup.bootstrap` for the bootstrap-mode notice.
- `gateway.startup.config_resolved` (promote the existing Debug to Info with a slug).
- `gateway.config.reloaded` / `gateway.config.reload_failed` / `gateway.config.missing` (slug existing).
- `tokens.reloaded` and the `tokens.*_failed` family (slug existing).
- `routing.policy.reloaded` and the `routing.policy.*_failed` family (slug existing).
- `upstream.health.probe_failed`, `upstream.models.*` (slug existing + level adjust).
- `gateway.health.upstream` periodic event (**new** — every N seconds when `upstream.base_url` probe state changes; **not** every poll). Driven by a new helper in `internal/upstream/`.
- `gateway.health.qdrant` and `gateway.health.bifrost` (**new** — emitted by gateway when supervised child health flips). These power the new gateway card KV row "supervised children".

**Acceptance.** Card KV row for **listening / upstream / config / tokens / routing rules / supervised children** populates from cold-start fixture without any field staying `—` after the first 60s of normal traffic.

**Status:** `todo`

### P5 — Card UI cleanup

**Goal.** Gateway card matches the KV / counters / subtitle / full-log spec above.

**Deliverables**

- New `internal/server/embedui/logs/derive/gatewayCardModel.js` (parallel to `qdrantCardModel`) that exposes `subtitle`, `kv`, and `counters` from `entryCache` so logic stays goja-testable.
- Update `gatewayServicePanelMiniHtml` (or replace it) in `internal/server/embedui/logs.js`: render KV row + four counter boxes; remove the three legacy mini-cards.
- Add `suppressGatewayBadge` plumbing.
- Add `gatewayPanelHideRow(ent)` predicate to filter `/api/ui/logs` / `/health` / `/status` 2xx access lines from the panel by default; expose a "show probes" toggle in the panel header.
- Refresh `internal/server/logs_components_test.go`, `internal/server/ui_logs_test.go`, and add a derive test under `internal/server/embedui/logs/derive/` (goja).

**Acceptance.** Operator can read **listening / upstream / config / tokens / routing rules / supervised children** + the four counters without expanding any single log row; `/health` polling does not crowd the panel.

**Status:** `todo`

### P6 — Demote / retire

**Goal.** Reduce buffer noise; retire ineffective lines.

**Deliverables**

- Audit `temp/gateway-mixed.log` for top-N most frequent slugs and demote any that are pure dev signal at high volume (e.g. `routing.rule.matched` per-request → Debug; `loggingMiddleware` Info on probe paths → Debug; `ingest.complete` per-chunk if found → consolidate).
- Retire (delete) any `slog` call that the audit shows fires zero times across all three fixtures **and** has no operator value (candidates: leftover Debug breadcrumbs from earlier refactors).
- Document the demotions / retirements in [`log-presentation-layer.md`](log-presentation-layer.md) §10 changelog.

**Acceptance.** Median Info-line rate during a 5-minute mixed-traffic capture drops by ≥30% vs P1 baseline, with zero loss of operator-actionable signal (verified by checking the four counters and KV row are still accurate).

**Status:** `todo`

---

## Open questions

- **Per-request request_id:** `internal/server/requestid` middleware already adds `request_id` on inbound; confirm it propagates through every `slog.With(...)` chain (chat does, but ingest / RAG paths may drop it on subloggers).
- **HTTP access slug vs shape:** Adding `msg: "http.access"` is a new field — verify it does not break shape inference (`inferShape` already returns `http.access` from `method`+`path`+`statusCode`).
- **Headline wording:** Locale stays English (matches qdrant + indexer); confirm headline rewrites do not break any operator-facing tools that grep the human text.
- **Periodic health events:** `gateway.health.upstream` cadence — fixed interval vs only-on-change. Default proposal: **only-on-change** with a max once-per-30s rate cap, plus a one-shot Info on first observation.
- **Backwards compat window:** How long to keep alias names in derive modules (proposal: one minor release, e.g. v0.4 → v0.5).
- **Probe filter UX:** Should the "hide probes" default also apply to the **structured logs** raw view, or only to the gateway service card?

---

## Checklist before marking done

- [ ] Every gateway `slog` call carries **`msg`** (audit script in `scripts/` if useful).
- [ ] Levels reclassified per taxonomy; cold-start fixture shrinks materially at Info+.
- [ ] New `gateway.*` lifecycle events power the card KV row from cold start.
- [ ] Gateway card matches **KV**, **counters**, **subtitle**, **full log** spec above (no gateway pill in that panel; probe rows hidden by default).
- [ ] Fixture-backed tests for the derive module and the renamed slugs (`internal/server/embedui/logs/derive/` goja).
- [ ] Backwards-compat aliases listed in [`log-presentation-layer.md`](log-presentation-layer.md) §10 with the planned removal release.
