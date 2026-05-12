# Plan: Operator-facing BiFrost log classification

| Field | Value |
|-------|-------|
| **Doc kind** | `feature-plan` |
| **Owners / areas** | Gateway supervision (`internal/supervisor`), upstream relay (`internal/chat`), logs UI (`internal/server/embedui/logs`), parse/derive (`internal/server/embedui/logs/parse`, `derive`), desktop mirror (`internal/servicelogs`) |
| **Status** | `done` |
| **Targets** | Gateway + supervised BiFrost (`bifrost-http -log-style json`) — single subprocess per gateway, mirror format unchanged |
| **Last updated** | 2026-05-09 |

## At a glance

The supervised **BiFrost** subprocess (`bifrost-http`) writes JSON lines (`-log-style json` by default when supervised). **`internal/servicelogs/bifrostline`** normalizes each complete stdout/stderr line into stable **`bifrost.*`** **`msg`** values (plus structured fields) before lines enter `servicelogs` and the operator UI. The **BiFrost** service card combines those subprocess rows with **gateway-emitted** relay lines (`chat.bifrost.request`, `chat.bifrost.response`, `chat.bifrost.error`, routing, provider limits). This plan specified and records:

1. A **stable `msg` taxonomy** on **every** classified line out of `bifrost-http` (parallel to `qdrant.*` and `indexer.*`).
2. **Operator-facing copy** (subtitle, KV summary fields, counters) derived from those slugs **plus** the existing gateway upstream relay slugs.
3. **UI contract** for the **BiFrost service card** — collapsed pills, expanded mini-cards, full event log — so an operator can answer "which providers are healthy, what model is in use, and why did the last call fail?" at a glance.
4. **[P6](#p6--summarized-headline-prose)** One-line **summarized log headlines** so rows in **Logs → BiFrost** (and relay-adjacent gateway lines) show prose derived from structured fields instead of bare dotted slugs.

**Related docs:** [`supervisor.md`](../supervisor.md), [`bifrost-discovery.md`](../bifrost-discovery.md), [`log-presentation-layer.md`](log-presentation-layer.md), [`log-qdrant.md`](log-qdrant.md), [`log-conversations.md`](log-conversations.md).

| Phase | Outcome | Status |
|-------|---------|--------|
| [P1 — Spec](#p1--spec) | This doc + frozen `bifrost.*` list | `done` |
| [P2 — Parse & `msg`](#p2--parse--msg) | `internal/servicelogs/bifrostline` normalizer wraps the bifrost stdout/stderr writers | `done` |
| [P3 — Card UI cleanup](#p3--card-ui-cleanup) | KV fields, counters, replace pills/mini-cards, suppress badge in own panel | `done` |
| [P4 — Gateway relay alignment](#p4--gateway-relay-alignment) | Reformat / dedupe gateway upstream relay slugs so card draws from one shared vocabulary | `done` |
| [P5 — Conversation linkage](#p5--conversation-linkage) | Cross-link relay events to conversation cards (depends on [`log-conversations.md`](log-conversations.md)) | `done` |
| [P6 — Summarized headline prose](#p6--summarized-headline-prose) | Operator-readable one-line headlines for every `bifrost.*` and relay vocabulary row | `done` |

---

## Background

- BiFrost is supervised by `claudia serve` (`internal/supervisor/bifrost.go`) and started with `-log-style json` (`internal/supervisor/bifrost.go` `StartBifrost`). Stdout / stderr go through **`bifrostline.NewWriter`** into **`logStore.Writer("bifrost")`** in `cmd/claudia/serve.gop` (same pattern as qdrant + `qdrantline`).
- The bifrost card in the logs UI today (`internal/server/embedui/logs.js` `bifrostCardMetrics`, `bifrostCollapsedCardSubtitle`, `renderExpandedService` for `name === "bifrost"`) builds its mini-cards from **gateway**-side lines emitted in `internal/chat/chat.go`:
  - `chat.bifrost.request` (request relay started — outgoing tokens, stream flag, model, request body excerpt).
  - `chat.bifrost.response` (response received — status, usage tokens, response bytes/excerpt).
  - `chat.bifrost.error` (relay fetch failed).
  - `chat.routing.fallback`, `chat.routing.attempt`, `chat.routing.resolved` (routing decisions); **`chat.provider_limits.blocked`** for quota skips.
- Anything BiFrost **itself** logs (provider key load, governance plugin, MCP startup, JWT auth, listening port, internal errors) flows to the bifrost bucket as raw JSON without a stable `msg`. The card has no way to surface it.
- **Implementation requirement:** after normalization, **every** bifrost-derived row exposed to the UI should carry a **`msg`** field (same pattern as `indexer.*` and `qdrant.*` slogs). Gateway-emitted relay lines already do; subprocess lines should match.

---

## Locked decisions (2026-05-08)

| Topic | Decision |
|-------|-----------|
| Slug prefix | **`bifrost.*`** for subprocess-origin events (parallel to `qdrant.*`); **`chat.bifrost.*`** stays on gateway-origin relay events. Both feed the same card. |
| Normalization location | **On ingest:** new `internal/servicelogs/bifrostline` package wraps the bifrost stdout/stderr writer in `cmd/claudia/serve.go`, mirroring `internal/servicelogs/qdrantline/`. |
| Counter window | Aggregates use lines **at or after the last `bifrost.startup.banner`** in the buffer (detect bifrost restart while gateway stays up). Gateway restart clears the ring buffer. |
| Summarized headlines | **`ClaudiaLogs.Derive.bifrostOperatorLine(flat)`** in [`bifrostMetrics.js`](../../internal/server/embedui/logs/derive/bifrostMetrics.js), parallel to **`qdrantOperatorLine`**; invoked from **`primaryLogMessage`** and **`buildHeadlineHtml`** in [`logs.js`](../../internal/server/embedui/logs.js). Stable **`msg`** in stored JSON unchanged. |
| HTTP success | Real status codes shown; **2xx** counts as success for relay/response counters; **4xx/5xx** as fail; **429** breaks out separately as **rate-limited**. |
| Non-JSON lines | If a line is not JSON or schema is unknown, emit `bifrost.unparsed` with the raw text in `progress_detail` (mirrors `qdrant.unparsed`). |
| Timeline | **Replace** the generic service-mix "Request timeline" on the bifrost panel (it was always 100% purple — every BiFrost row maps to `upstream`) with two purpose-built strips driven from the slug taxonomy: a **Provider health** strip (segment per loaded provider, colored by latest `bifrost.provider.health.*` / `bifrost.provider.key_missing`) **before** the Available models card, and a **Relay outcomes** strip (HTTP 2xx / 3xx / 429 / 4xx / 5xx / fetch-err / in-flight buckets from `chat.bifrost.{request,response,error}`) **after** it. `bifrost.rate_limit` (subprocess inbound HTTP, often `/v1/embeddings`) is excluded from the relay strip; the existing "Rate limits" mini-card still aggregates both. |
| Badge in own panel | **Suppress** the **bifrost** source badge inside the bifrost expanded panel rows (parallel to `suppressQdrantBadge` / `suppressIndexerBadge`). Keep the **upstream** badge as today. |
| Conversation correlation | Gateway relay rows are canonical for conversation cards. Phase 5 of [`log-conversations.md`](log-conversations.md) sets upstream `X-Request-Id` from the gateway `request_id`; subprocess `bifrost.*` rows join conversations only if BiFrost exposes that id in normalized logs. Custom `X-Claudia-*` headers are opportunistic only and not required. |

### Code references

- Go (today): [`internal/supervisor/bifrost.go`](../../internal/supervisor/bifrost.go), [`internal/chat/chat.go`](../../internal/chat/chat.go) (`chat.bifrost.*` slogs), [`cmd/claudia/serve.go`](../../cmd/claudia/serve.go) (`logStore.Writer("bifrost")`).
- Go (new): `internal/servicelogs/bifrostline/` (`NormalizePayload`, `NewWriter`) — pattern from `internal/servicelogs/qdrantline/`.
- JS: [`internal/server/embedui/logs.js`](../../internal/server/embedui/logs.js) (search `isBifrost`, `bifrostCardMetrics`, `bifrostCollapsedCardSubtitle`).
- JS derive: [`internal/server/embedui/logs/derive/bifrostMetrics.js`](../../internal/server/embedui/logs/derive/bifrostMetrics.js): metrics plus **`bifrostOperatorLine`** (summarized headlines); [`internal/server/embedui/logs/derive/conversationBifrost.js`](../../internal/server/embedui/logs/derive/conversationBifrost.js) for **conversation** BiFrost chip counts (P5).

## Reference samples (local dev)

**P1.** Capture under repo-root **`temp/`** (gitignored). Paths below were produced from a live desktop mirror (`timestamp<TAB>source<TAB>payload`); payloads are raw bifrost stdout lines with host paths replaced by `<REPO>`.

| Artifact | Path | Purpose |
|----------|------|--------|
| Cold-start bifrost log | `temp/bifrost-startup.log` | Banner, bootstrap, stores, catalog, plugins, ready line, first HTTP access |
| Mixed traffic | `temp/bifrost-mixed.log` | Embeddings + `/v1/chat/completions` lines including **200** and **429** |

---

## Canonical `msg` taxonomy

Stable machine slug pattern: **`bifrost.<segment>.<segment>…`** (subprocess-origin) and **`chat.bifrost.<segment>`** (gateway-origin relay). Use **dots** between segments. Detection uses the JSON envelope BiFrost emits with `-log-style json` (top-level `level`, `time`, human text in **`message`** or **`msg`**, plus subsystem fields). After normalization, every row exposed to the UI uses a single **`msg`** field for the slug.

### Subprocess-origin (`bifrost.*`) — emitted by `bifrost-http`

| `msg` | Typical detection | Notes |
|-------|-------------------|--------|
| `bifrost.startup.banner` | First non-JSON or banner line | Subtitle: **"Starting up …"**; resets counter window. |
| `bifrost.version` | Plain or JSON line carrying version field | Populate KV **version**. |
| `bifrost.listen.http` | "HTTP listening" / "Server started" with port | Populate KV **port**. |
| `bifrost.config.loaded` | "config loaded" / "config.json applied" | Populate KV **configuration** = **`supervised`**. |
| `bifrost.provider.loaded` | "provider registered" / "loaded provider" | Increment **providers loaded** counter; capture provider id (e.g. `groq`, `gemini`). |
| `bifrost.provider.key_loaded` | "key loaded for provider X" | Increment **keys loaded** for provider id; **never** log key value. |
| `bifrost.provider.key_missing` | "no API key for provider X" / env var missing | Subtitle: **"Missing key for {provider}"**; counter **provider-config errors**. |
| `bifrost.provider.health.ok` | Provider health probe success | Per-provider **health = up**; updates KV **providers up/total**. |
| `bifrost.provider.health.fail` | Provider health probe failure | Per-provider **health = down**; counter **provider-health errors**. |
| `bifrost.mcp.startup` | MCP integration init | Populate KV **MCP** = **`enabled`** / **`disabled`**. |
| `bifrost.governance.startup` | Governance plugin init | Populate KV **governance** = **`enabled`** / **`disabled`**. |
| `bifrost.jwt.startup` | JWT/auth plugin init | Populate KV **auth** = **`jwt`** / **`api-key`** / **`disabled`**. |
| `bifrost.upstream.request` | Outbound provider HTTP request started (subprocess view) | Optional secondary counter; primary is gateway `chat.bifrost.request`. |
| `bifrost.upstream.response` | Outbound provider HTTP response received | Optional secondary counter; status code in `http_status`. |
| `bifrost.upstream.error` | Provider request errored at network layer | Counter **upstream errors**; subtitle: **"Provider {id} error: {short}"**. |
| `bifrost.rate_limit` | Provider returned 429 / "rate limit" surfaced | Counter **rate-limited**; subtitle: **"429 rate-limit — retry in {n}s"** when retry-after parses. |
| `bifrost.governance.rejected` | Governance plugin denied request | Counter **governance rejections**; subtitle: **"Rejected by governance: {reason}"**. |
| `bifrost.shutdown` | Graceful shutdown notice | Subtitle: **"Shutting down …"**; do not reset counters. |
| `bifrost.unparsed` | Schema unknown or non-JSON tail line | Carry raw in `progress_detail`; do not advance counters. |

The rows below are **not** emitted today as stable slugs; they map recurring **bifrost-http** / **core** log texts and structured lines that operators need for health and incident triage. Normalization should assign these `msg` values (and structured fields) so the logs UI can filter, badge, and subtitle on them.

| `msg` | Typical detection (upstream source) | Operator translation |
|-------|--------------------------------------|----------------------|
| `bifrost.bootstrap.complete` | `Time spent in Bifrost server bootstrap %d ms` (`main.go`) | Subtitle / KV: **"Bootstrap completed in {n} ms"**; flag unusually slow starts. |
| `bifrost.ready` | `successfully started bifrost, serving UI on http://…` (`server.go`) | Subtitle: **"Ready — serving UI on {host}:{port}"**; confirms HTTP stack + UI, not just bind. |
| `bifrost.client.ready` | `bifrost client initialized` (`server.go`) | Pill / KV: **"Core client initialized"**; engine ready before catalog/plugins finish. |
| `bifrost.http.access` | Zerolog `"request completed"` + `http.method` / `http.target` / `http.status_code` / `http.request_duration_ms` (`middlewares.go` `CorsMiddleware`) | **Inbound** HTTP to BiFrost (distinct from gateway `chat.bifrost.*` and from outbound `bifrost.upstream.*`). Pills by status bucket (2xx / 4xx / 5xx); subtitle on 5xx: **"HTTP {path} → {code} ({ms} ms)"**. |
| `bifrost.plugin.status` | `plugin status: {name} - {status}` (`server.go` bootstrap) | Per-plugin pill; subtitle on failure: **"Plugin {name}: {status}"**. Supplements coarse `bifrost.mcp.startup` / `bifrost.governance.startup`. |
| `bifrost.plugin.custom_load` | `loading custom plugin from path …` (`server/plugins.go`) | Subtitle: **"Loading custom plugin from {path}"**; security / audit visibility. |
| `bifrost.plugin.hook.error` | `error in HTTPTransportPostHook for plugin …` (`middlewares.go`) | Counter **plugin hook errors**; subtitle: **"Plugin hook failed: {plugin}"**. |
| `bifrost.store.config_ready` | `config store initialized` / `… (default SQLite)` (`lib/config.go`) | KV **config store**: **`sqlite`** / **`memory`** (infer from message). |
| `bifrost.store.request_logs_ready` | `logs store initialized` (request-log DB) (`lib/config.go`) | KV **request logs store**: **`ready`** — LLM usage logging available. |
| `bifrost.vectorstore.connect` | `connecting to vectorstore` (`lib/config.go`) | Subtitle when stuck: **"Connecting to vector store …"**; semantic cache / embeddings path. |
| `bifrost.catalog.sync` | `listing all models and adding to model catalog` / `models added to catalog` / `model-parameters-sync` / model-catalog fallback warnings (`server.go`) | KV **model catalog**: **`synced`** vs **`fallback (static)`**; subtitle on warn: **"Model catalog degraded — using static datasheet"**. |
| `bifrost.provider.model_discovery.fail` | `Model discovery failed for provider …` (`handlers/providers.go`) | Counter; subtitle: **"Model list sync failed for {provider}"** (routing may be stale). |
| `bifrost.pricing.sync.warn` | pricing URL checks / `failed to refresh pricing` / `failed to seed pricing` (`handlers/config.go`, `server.go`) | Subtitle: **"Pricing sync issue — {short}"**; cost display may be wrong. |
| `bifrost.overload.queue_drop` | `request dropped: queue is full …` (`core/bifrost.go`) | Counter **dropped requests**; subtitle: **"Overload — request dropped (queue full)"**; distinct from 429. |
| `bifrost.config.hot_reload` | `client config was updated in config.json, syncing` (`lib/config.go`) | Pill **config reload**; subtitle: **"config.json changed — hot reload"**. |
| `bifrost.config.proxy.reloaded` | `proxy configuration reloaded: enabled=…` (`server.go`) | KV / pill **outbound proxy**; subtitle when relevant. |
| `bifrost.config.header_filter.reloaded` | `header filter configuration reloaded: …` (`server.go`) | Pill **header filter updated** (allow/deny counts). |
| `bifrost.jobs.async_ready` | `async job executor initialized` (`server.go`) | KV **async jobs**: **`enabled`**. |
| `bifrost.maintenance.log_retention` | `log retention cleaner initialized with %d days retention` (`server.go`) | KV **log retention**: **`{n} days`**. |
| `bifrost.mcp.persistence.disabled` | `config store is disabled - MCP manager will not be initialized` (`lib/config.go`) | Subtitle: **"MCP disabled — no config store"**; explains missing MCP without guessing. |
| `bifrost.observability.prometheus.missing` | `prometheus plugin not found…` (`server.go`) | Pill **metrics endpoint off**; subtitle for ops expecting `/metrics`. |
| `bifrost.dev.pprof_enabled` | `dev mode enabled, registering pprof endpoints` (`server.go`) | Pill **dev / pprof** (security-relevant in prod-like setups). |
| `bifrost.transport.websocket.warn` | `websocket upgrade failed`, upstream WS fallback (`handlers/wsresponses.go`) | Subtitle: **"WebSocket transport issue — {short}"** if using Responses/WebSocket path. |
| `bifrost.shutdown.signal` | `received signal %v, initiating graceful shutdown` (`server.go`) | Subtitle: **"Shutdown: signal {sig}"**; pairs with `bifrost.shutdown`. |
| `bifrost.observability.trace_inject.warn` | `observability plugin … failed to inject trace` (`middlewares.go`) | Subtitle: **"Trace injection failed — {plugin}"**. |

### Gateway-origin (`chat.bifrost.*` and friends) — emitted by `internal/chat`

| `msg` | Source today | Card behavior after P4 |
|-------|--------------|------------------------|
| `chat.bifrost.request` | `internal/chat/chat.go` `proxyChatCompletionPayload` | **Relay req** counter; subtitle: **"Relay → {short_model} ({stream})"**; populate KV **last model**. |
| `chat.bifrost.response` *(rename of `upstream chat response`)* | `internal/chat/chat.go` `logUpstreamChatResponse` | **Relay res** counter; populate **usage tokens**, **response bytes**; subtitle: **"Response {status} · {tok} usage"**. |
| `chat.bifrost.error` | `internal/chat/chat.go` upstream fetch failed | **Relay err** counter; subtitle: **"Upstream fetch failed: {short_err}"**. |
| `chat.routing.fallback` | `internal/chat/chat.go` retry next model | **Fallback attempts** counter; subtitle: **"Fallback {n}/{N} → {short_model}"**. |
| `chat.routing.resolved` *(rename of `virtual model routing resolved`)* | `internal/chat/chat.go` | **Routing resolved** counter; subtitle: **"Routed → {short_model} ({attempt}/{N})"**. |
| `chat.routing.attempt` *(demote `virtual model fallback attempt` to `Debug`)* | `internal/chat/chat.go` | Debug-only by default; included in expanded log. |
| `chat.provider_limits.blocked` *(rename of `chat blocked by provider limits` / `skipping upstream model (provider limits)`)* | `internal/chat/chat.go` | **Provider-limit blocks** counter; subtitle: **"Blocked by provider quota: {reason}"**. |

**HTTP status labeling:** use **2xx** as success, **4xx/5xx** as fail, with a separate **429** label for rate-limit visibility (matches existing `bifrostEntryHasRateLimit`).

---

## UI contract: BiFrost service card (summarized logs)

Applies to **Logs → BiFrost** summary card (`renderExpandedService` / `buildServiceCard` in `internal/server/embedui/logs.js`).

### Collapsed card header

- **Keep** the operator subtitle line (already populated by `bifrostCollapsedCardSubtitle`); extend its source priority order to:
  1. recent `bifrost.rate_limit` or `chat.bifrost.error` (kept as today).
  2. recent `bifrost.provider.health.fail` / `bifrost.provider.key_missing` (new — show subprocess-side health issues even when no relay traffic).
  3. last `chat.bifrost.request` summary (kept).
  4. last `chat.bifrost.response` summary (kept).
- **Remove** the legacy "BiFrost · N" rollup pill in the conversation feed strip (today added by `inferShape` rollup in the conversations panel) — replace with the conversation-card chip defined in [`log-conversations.md`](log-conversations.md).

### Expanded card — below the summary heading (KV row)

New / extended **key-value** fields (always show keys; values fill as events arrive):

| Key | Source `msg` / logic |
|-----|---------------------|
| **version** | `bifrost.version` |
| **configuration** | `bifrost.config.loaded` → **`supervised`** (matches qdrant card tone) |
| **port** | `bifrost.listen.http` |
| **auth** | `bifrost.jwt.startup` → **`jwt`** / **`api-key`** / **`disabled`** |
| **MCP** | `bifrost.mcp.startup` → **`enabled`** / **`disabled`** |
| **governance** | `bifrost.governance.startup` → **`enabled`** / **`disabled`** |
| **providers up/total** | `bifrost.provider.loaded` → total; `bifrost.provider.health.ok/fail` → up |
| **last model** | latest `chat.bifrost.request` `upstreamModel` (short label via `bifrostShortModelLabel`) |

### Expanded card — summary section (replace current mini-cards)

**Remove** today's three mini-cards (`Relay (req · res · err)`, `Tokens (out → usage)`, `Model · stream · HTTP`).

**Layout order** under the Summary KV table:

1. `Provider health` strip (`sum-timeline-bar--provider-health`) — one segment per configured provider, colored by latest probe. Source preference: **(a)** live snapshot from **`GET /api/ui/bifrost/providers`** (gateway BFF: `internal/server/ui_bifrost_providers.go`, fetched every 30s, classifier `up | down | key_missing | unknown`), then **(b)** log-derived state via `ClaudiaLogs.Derive.bifrostProviderHealthList` (covers any future `bifrost.provider.health.*` / `bifrost.provider.key_missing` slugs the subprocess emits), then **(c)** empty caption ("BiFrost unreachable" or "No providers loaded yet"). Caption lists each provider id with its current state. <br/>**Live `/v1/models` override:** the BFF reads the runtime catalog snapshot maintained by `internal/server/availablemodels.go` (periodic poller in `cmd/claudia/serve.go`). When a provider is configured (`up`) but absent from the freshly polled `/v1/models` response, BiFrost is signalling that it can't reach the upstream — classifier downgrades the entry to `down` with `error="no models available in live catalog"`. Period configurable via gateway.yaml `health.available_models_poll_ms` (default 30 000; ≤0 disables periodic polling).
2. `Available models` mini-card (`sum-mini-row--bifrost-deck`) — catalog count from latest sync.
3. `Relay outcomes` strip (`sum-timeline-bar--relay-outcome`) — HTTP-bucket strip over chat relay rows. Backed by `ClaudiaLogs.Derive.bifrostRelayOutcomeBuckets`. Buckets: 2xx / 3xx / 429 / 4xx / 5xx / fetch err / in-flight.
4. Counter row (`sum-mini-row--bifrost-deck2`) — four counter boxes:

| Box | Behavior |
|-----|----------|
| **Relay success / fail** | From `chat.bifrost.response` (HTTP 2xx vs 4xx/5xx) + `chat.bifrost.error` (always fail). |
| **Tokens (out → usage)** | Sum `outgoingTokens` (`chat.bifrost.request`) → sum `usageTotalTokens` (`chat.bifrost.response`). Show "— → —" until non-zero. |
| **Rate-limit / fallback** | Count `bifrost.rate_limit` and `chat.routing.fallback`. Side-by-side counts; subtitle text "n×429 · m×fallback". |
| **Providers (up / loaded)** | From `bifrost.provider.loaded` + `bifrost.provider.health.ok/fail`. Tint **error** when any provider is `down`. |

### Full event log (expanded)

- **Suppress** the **bifrost** source badge on each row in this panel only (same effect as `suppressQdrantBadge` on the qdrant panel). Implementation: bifrost full-log rows call `logSummaryHtml(ev, null, {})` so no service pill is rendered for subprocess lines.
- **Keep** the **upstream** chip for `chat.bifrost.*` rows (already added by `badgeForServicePanel`).
- Each row uses the existing `buildDetailsColumn` so structured fields (model, status, tokens, retry-after) remain visible in expand.

---

## Phased implementation

### P1 — Spec

**Goal.** This doc + a frozen `bifrost.*` list, validated against captured fixtures.

**Deliverables**

- This file checked in.
- `temp/bifrost-startup.log`, `temp/bifrost-mixed.log` captured from a live `claudia serve` cold start + chat round-trips (sanitized paths / org ids).
- Taxonomy reviewed against fixtures; outliers listed under **Open questions**.

**Acceptance.** Reviewer can map every line in the two fixtures to a `msg` slug or to `bifrost.unparsed`.

**P1 validation (2026-05-08).** Frozen taxonomy confirmed by owners. **`temp/bifrost-startup.log`** (85 lines): ASCII banner + zerolog JSON (`message` key); maps to `bifrost.startup.banner`, `bifrost.bootstrap.complete`, `bifrost.ready`, `bifrost.plugin.status`, `bifrost.store.*`, `bifrost.catalog.sync`, `bifrost.http.access`, `bifrost.governance.startup`, `bifrost.jobs.async_ready`, `bifrost.maintenance.log_retention`, `bifrost.provider.loaded` (via `added provider:`), `bifrost.config.validation_failed` / `bifrost.transport.serve_error` for the long validation and TCP-abort samples, etc. **`temp/bifrost-mixed.log`** (20 lines): `bifrost.http.access` for embeddings and chat completions; **`bifrost.rate_limit`** where `http.status_code` is **429**. P2 normalizer + `testdata/` copies classify **100%** of both fixtures without **`bifrost.unparsed`**.

**Status:** `done`

### P2 — Parse & `msg`

**Goal.** Every BiFrost-derived row exposed to the UI carries a `msg`.

**Deliverables**

- `internal/servicelogs/bifrostline/normalize.go` (+ `_test.go`): `NormalizePayload(raw string) []byte` returns gateway-style JSON with `service:"bifrost"`, `_claudia_norm:1`, and the `bifrost.*` slug. Mirrors `qdrantline.NormalizePayload`.
- `internal/servicelogs/bifrostline/writer.go`: `NewWriter(downstream io.Writer) io.Writer` line-buffers raw stdout and forwards normalized JSON.
- `cmd/claudia/serve.go`: wrap `logStore.Writer("bifrost")` with `bifrostline.NewWriter(...)`.
- Unit fixtures from P1 included in `internal/servicelogs/bifrostline/testdata/`.

**Acceptance.** A `claudia serve` cold start writes only normalized JSON into the bifrost bucket; `bifrost.unparsed` count is 0 against both fixtures.

**Status:** `done`

### P3 — Card UI cleanup

**Goal.** BiFrost card matches the KV / counters / subtitle / full-log spec above.

**Deliverables**

- Extend `internal/server/embedui/logs/derive/bifrostMetrics.js` with provider/key/health counters from `bifrost.*` lines (kept in goja-tested derive module to match qdrant pattern).
- Update `internal/server/embedui/logs.js` `renderExpandedService` (`isBifrost` branch): replace mini-cards with KV row + four counter boxes.
- Bifrost full event log: render each row with `logSummaryHtml(ev, null, {})` so the bifrost source pill does not appear in that panel (`logs.js` bifrost branch of the full-log loop).
- Refresh `internal/server/logs_components_test.go` and `internal/server/ui_logs_test.go` for new selectors.

**Acceptance.** Operator can read **version / port / providers up/total / auth** without expanding any single log row; counters reset on the next `bifrost.startup.banner`.

**Status:** `done`

### P4 — Gateway relay alignment

**Goal.** Rename / demote relay slugs so the card draws from one shared vocabulary.

**Deliverables**

- `internal/chat/chat.go`: rename `"upstream chat response"` → `chat.bifrost.response`; demote `virtual model fallback attempt` (Info) to `chat.routing.attempt` at `Debug` when chain length is 1; rename `chat.routing.fallback` retry log to keep the existing slug (already correct); rename `chat blocked by provider limits` and `skipping upstream model (provider limits)` to `chat.provider_limits.blocked` (keep level Info).
- Update derive (`bifrostMetrics.js`) to recognize the new slug names alongside the old ones for one release window.
- Update [`log-presentation-layer.md`](log-presentation-layer.md) §10 changelog.

**Acceptance.** Existing UI metrics (relay req/res/err, tokens, status mix) remain identical numerically after the rename; new `chat.bifrost.response` slug is queryable in `/api/ui/logs`.

**Status:** `done`

### P5 — Conversation linkage

**Goal.** When a `chat.bifrost.*` line carries `conversation_id`, it is **also** routed into the conversation card's BiFrost chip (same line, two projections — see [`log-presentation-layer.md`](log-presentation-layer.md) §3 / §4).

**Deliverables**

- Coordinated with [`log-conversations.md`](log-conversations.md): conversation card consumes `chat.bifrost.request`, `chat.bifrost.response`, `chat.bifrost.error`, `chat.routing.fallback`, `chat.routing.attempt`, `chat.routing.resolved`, `chat.provider_limits.blocked` (plus legacy string `msg`s) for the Services strip **BiFrost · N** chip — via [`conversationBifrost.js`](../../internal/server/embedui/logs/derive/conversationBifrost.js) `conversationBifrostRelayCount`.
- Conversation grouping attaches relay lines **tier 1** when `conversation_id` is present (already true for `routeLog.With` in [`server.go`](../../internal/server/server.go)) and **tier 2** when `conversation_id` is missing but **`request_id`** matches an earlier anchored row (`principal_id`, `conversation_id`, `request_id`) and `conversationBifrostTimelineFlat` matches — see `renderSummarizedUnified` in [`logs.js`](../../internal/server/embedui/logs.js).
- Subprocess `bifrost.*` lines stay **bifrost-only** unless they carry `conversation_id` (out of scope until BiFrost echoes correlation headers — still tracked in [`log-conversations.md`](log-conversations.md)).

**Acceptance.** Opening a conversation card shows **BiFrost · N** in the Services strip where **N** equals relay / routing scoped lines in that conversation (including **`request_id` join** rows); bifrost subsystem card aggregates stay unchanged.

**Status:** `done`

### P6 — Summarized headline prose

**Goal.** Operators scanning **summarized** log rows see **human headlines** built from **`msg`** plus structured fields (never replacing the slug in stored logs or filters).

**Deliverables**

- **`bifrostOperatorLine(flat)`** in [`internal/server/embedui/logs/derive/bifrostMetrics.js`](../../internal/server/embedui/logs/derive/bifrostMetrics.js): maps subprocess **`bifrost.*`** and gateway relay (**`chat.bifrost.*`**, **`chat.routing.*`**, **`chat.provider_limits.blocked`**) plus **legacy** vocabulary (e.g. **`upstream chat response`**, older virtual-model wording) for one release window.
- **`logs.js`**: call **`bifrostOperatorLine`** from **`primaryLogMessage`** (row summary text) and **`buildHeadlineHtml`** (expanded detail headline).
- Goja coverage in **`internal/server/logs_components_test.go`** (`TestLogsDerive_bifrostOperatorLine`).

**Acceptance.**

- Summarized BiFrost rows show prose such as **Inbound …**, **Model catalog updated …**, **Relay request …**, not bare **`bifrost.http.access`** / **`chat.bifrost.available_models`** alone (unless **`bifrostOperatorLine`** intentionally falls through — unknown **`bifrost.*`** uses **BiFrost ·** plus the raw slug as fallback).

**Status:** `done`

---

## Reference — Summarized log headlines

Canonical headline patterns implemented by **`bifrostOperatorLine`**. **`{fields}`** are filled when present on the flattened JSON (`http_method`, `http_target`, `listen_url`, `catalog_model_count`, etc.). Inbound HTTP uses **`http_target`** pathname.

### Subprocess (`service:bifrost`)

| `msg` | Operator headline pattern |
|-------|---------------------------|
| `bifrost.http.access` | **Inbound ·** `{method}` `{path}` **· →** `{status}` **·** `{rounded ms}` **ms** |
| `bifrost.rate_limit` | **Rate limited ·** … (same field layout as access) |
| `bifrost.catalog.sync` | **Model catalog updated ·** `{n}` **models** — or **Model catalog / pricing ·** `{short detail}` — or **Model catalog sync** |
| `bifrost.startup.banner` | **BiFrost starting** |
| `bifrost.version` | **BiFrost version** `{bifrost_version}` |
| `bifrost.bootstrap.complete` | **Startup finished · bootstrap** `{ms}` **ms** |
| `bifrost.client.ready` | **Core client ready** |
| `bifrost.jobs.async_ready` | **Background jobs enabled** |
| `bifrost.governance.startup` | **Governance enabled** |
| `bifrost.mcp.startup` | **MCP catalog initializing** |
| `bifrost.mcp.persistence.disabled` | **MCP disabled · no config store** |
| `bifrost.jwt.startup` | **Auth ·** JWT / API key / disabled (from `progress_detail`) |
| `bifrost.auth.token_refresh` | **Auth · token refresh worker started** |
| `bifrost.config.loaded` | **Configuration loaded · supervised** |
| `bifrost.config.validation_failed` | **Configuration invalid ·** `{detail}` |
| `bifrost.config.schema_warn` | **Configuration warning ·** `{detail}` |
| `bifrost.store.config_ready` | **Config store ready** [· **sqlite** / **memory**] |
| `bifrost.store.request_logs_ready` | **Usage / request log store ready** |
| `bifrost.listen.http` | **HTTP listening ·** `{detail}` |
| `bifrost.ready` | **Ready · UI at** `{listen_url}` — or **Ready · port** `{port}` — or **Ready** |
| `bifrost.plugin.status` | **Plugin** `{name}` **·** `{status}` |
| `bifrost.provider.loaded` | **Provider registered ·** `{provider_id}` |
| `bifrost.provider.health.ok` / `fail` | **Provider healthy** / **Provider health failed ·** `{provider_id}` |
| `bifrost.provider.key_loaded` / `key_missing` | **Provider API key loaded** / **Missing API key ·** `{provider_id}` |
| `bifrost.maintenance.log_retention` | **Request log retention ·** `{n}` **days** — or **Log retention ·** `{detail}` |
| `bifrost.transport.serve_error` | **Connection error ·** `{detail}` |
| `bifrost.log.zerolog` | `{trimmed progress_detail}` |
| `bifrost.unparsed` | **Unrecognized BiFrost log ·** `{detail}` |
| `bifrost.governance.rejected` | **Rejected by governance ·** `{detail}` |
| `bifrost.upstream.request` / `response` / `error` | **Upstream request/response/error ·** `{detail}` |
| `bifrost.shutdown` / `bifrost.shutdown.signal` | **Shutting down ·** `{detail}` |
| *(other `bifrost.*`)* | **BiFrost ·** `{msg}` |

### Gateway relay and routing

| `msg` (or legacy text) | Operator headline pattern |
|------------------------|---------------------------|
| `chat.bifrost.available_models` | **Model list for routing ·** `{n}` **models** — or **Model list for routing refreshed** |
| `chat.bifrost.request` | **Relay request ·** `{short model}` **·** streaming on/off **·** `{out}` **tok out** |
| `chat.bifrost.response` (legacy: `upstream chat response`) | **Relay response · HTTP** `{code}` **·** `{usage}` **usage tok ·** `{bytes}` **B** |
| `chat.bifrost.error` | **Relay failed ·** `{err}` |
| `chat.routing.fallback` | **Fallback retry ·** `{short model}` **· HTTP** `{status}` *(optional: no retry)* |
| `chat.routing.attempt` (legacy: virtual model fallback attempt text) | **Routing attempt ·** `{model}` **· attempt** `{i}/{N}` |
| `chat.routing.resolved` (legacy: virtual model routing resolved text) | **Routing resolved ·** `{model}` **· attempt** `{i}/{N}` **· HTTP** `{code}` *(optional)* |
| `chat.provider_limits.blocked` (legacy: blocked / skipping … text) | **Blocked by provider limits ·** `{reason}` |

English-only; unknown subprocess slugs still expose the dotted slug for debugging via **BiFrost ·** prefix.

---

## Open questions

- **P1 fixtures — no 5xx sample:** `temp/bifrost-mixed.log` has **2xx** and **429** inbound HTTP access lines; no **`http.status_code` 5xx** line appeared in this capture. The **`bifrost.http.access`** slug still covers 5xx when they occur.
- **Volume:** Provider-health probes can be high frequency; consider a per-provider rollup window (e.g. last status only) for the **providers up/total** KV to avoid flapping.
- **JSON schema drift:** BiFrost upstream may rename fields between releases; pin a `bifrost_version` field at normalization time so older fixtures stay parseable.
- **Key visibility:** Confirm `bifrost.provider.key_loaded` never echoes the secret (only the env var name + provider id). Match `SECURITY.md`.
- **Locale:** English-only operator strings (matches qdrant + indexer).
- **Cross-link UX:** Whether expanded BiFrost rows should jump to the originating conversation card (depends on [`log-conversations.md`](log-conversations.md) P3).

---

## Checklist (completed)

- [x] Every BiFrost-normalized line carries **`msg`** + structured fields needed for UI (via `bifrostline` on supervised ingest).
- [x] BiFrost card matches **KV**, **counters**, **subtitle**, **full log** (no bifrost source badge in that panel — rows render without a service pill) spec above.
- [x] Summarized log rows use **`bifrostOperatorLine`** for headlines (P6).
- [x] Gateway relay slugs renamed / demoted per P4 with derive recognizing both old and new for one release window.
- [x] Conversation cards show **BiFrost · N** and merge relay lines by **`conversation_id`** / **`request_id`** (P5).
- [x] Fixture-backed tests: sanitized copies of the P1 **`temp/bifrost-*.log`** captures live in **`internal/servicelogs/bifrostline/testdata/`** (`TestNormalizePayload_fixturesNoUnparsed` asserts no `bifrost.unparsed` and `bifrost.*` + `service:bifrost` on every line).
- [x] Supervisor doc notes normalization boundary (parallel to qdrant — [`supervisor.md`](../supervisor.md) runtime table **Child** row for BiFrost).
