# Plan: Operator-facing Gateway log classification

| Field | Value |
|-------|-------|
| **Doc kind** | `feature-plan` |
| **Owners / areas** | Gateway core (`internal/server`, `internal/chat`, `internal/rag`, `internal/routing`, `internal/conversationmerge`, `internal/tokens`, `internal/upstream`, `internal/config`), logs UI (`internal/server/embedui/logs`), parse/derive (`internal/server/embedui/logs/parse`, `derive`) |
| **Status** | `done` |
| **Targets** | Gateway parent process (`cmd/chimera serve`, `cmd/chimera-gateway`) — structured `slog` only |
| **Last updated** | 2026-05-09 |

## At a glance

The gateway is the only **first-party** process in the stack. Its logs drive the conversation, indexer, and bifrost cards through dotted **`msg`** slugs (`chat.bifrost.*`, `rag.*`, `ingest.*`, `gateway.*`, `gateway.auth.*`, …). **P2–P6** shipped **`msg`** everywhere, level tuning, new **`gateway.*`** lifecycle objects, the **gateway card** derive model, and demotions where agreed. This plan captures:

1. A **stable `gateway.*` taxonomy** for non-domain-specific lines, plus tightened existing slugs (`chat.*`, `rag.*`, `ingest.*`, `routing.*`, `gateway.auth.*`, `config.*`, `conversation.*`). Client-credential events use **`gateway.auth.*`** so **`msg` never uses a `tokens.` prefix** for gateway-issued secrets (avoids collision with model / tokenizer / usage “tokens”; see [`version-v0.3.md`](../version-v0.3.md) credential naming).
2. **Log-level reclassification** so the **default** stream (Info+) tells the operator story without being drowned by per-request debug.
3. **New structured objects** the UI needs but the gateway does not currently emit (e.g. `gateway.startup.listening`, `gateway.config.reloaded`, `gateway.supervisor.bifrost.ready`, `gateway.auth.reloaded`, `gateway.rag.init_failed`, `gateway.http.access`).
4. **Reformatted human messages** so the **headline** (passed as the first arg to `slog.Info`) actually summarizes the event for the operator, not the developer.
5. **Demotion / retirement** of low-value messages that crowd the buffer.

**Related docs:** [`supervisor.md`](../supervisor.md), [`log-presentation-layer.md`](log-presentation-layer.md), [`log-qdrant.md`](log-qdrant.md), [`log-bifrost.md`](log-bifrost.md), [`log-conversations.md`](log-conversations.md), [`docs/indexer.md`](../indexer.md).

| Phase | Outcome | Status |
|-------|---------|--------|
| [P1 — Inventory & spec](#p1--inventory--spec) | This doc + frozen `gateway.*` list and [implementation map](#implementation-map-where-parent-process-msg-is-emitted) | `done` |
| [P2 — `msg` slugs everywhere](#p2--msg-slugs-everywhere) | Every gateway `slog` call carries `msg`; `gateway.http.access` + `inferShape` alias wired | `done` |
| [P3 — Level reclassification](#p3--level-reclassification) | Default Info stream tells the operator story; dev noise demoted to Debug; benign Warns demoted to Info | `done` |
| [P4 — New gateway objects](#p4--new-gateway-objects) | New structured events for startup, listening, config reload, client credential reload, upstream/qdrant/bifrost health | `done` |
| [P5 — Card UI cleanup](#p5--card-ui-cleanup) | Gateway service card replaces generic counters with operator KV + counters | `done` |
| [P6 — Demote / retire](#p6--demote--retire) | Low-value lines demoted or removed; buffer density improves | `done` |

---

## Background

- The gateway uses a single `*slog.Logger` built in `cmd/chimera/serve.go` via `buildLoggerTo(...)` (`slog` **text** handler: `key=value` lines today). Lines land in `internal/servicelogs.New(...)` under source **`gateway`** (JSON remains valid when present).
- A few hot paths carry stable slugs:
  - `internal/chat/chat.go` — `chat.bifrost.request`, `chat.bifrost.response`, `chat.bifrost.error`, `chat.routing.fallback`, `chat.routing.attempt`, `chat.routing.resolved`, `chat.provider_limits.blocked` ([`log-bifrost.md`](log-bifrost.md) P4 **done**; relay headlines may still use legacy human text until P2 headline pass).
  - `internal/server/server.go` — `chat.request`, `rag.retrieve.error`, `rag.retrieve.source` (per-source hit summary after a successful retrieve; replaces the older single-line `rag context injected` / `rag.retrieve.ok` idea).
  - `internal/server/availablemodels.go` — `chat.bifrost.available_models` on merged `/v1/models` poll logs for the routing catalog.
  - `internal/server/ingest.go` and `internal/server/ingest_session.go` — `ingest.complete`, `ingest.chunked.error`.
  - `internal/rag/service.go` — `rag.ingest.trace`, `rag.query`, `rag.embed`, `rag.hit`.
  - `internal/indexer/*` — `indexer.run.start`, `indexer.run.done`, `indexer.discovery.summary`, `indexer.queue.snapshot`, `indexer.recovery.poll`, `indexer.recovery.resumed`, `indexer.scope.status`, `indexer.scope.active_file` (these stay owned by the indexer doc).
- **Parent-process gateway** lines in `cmd/chimera` + gateway-core `internal/` packages carry stable **`msg`** slugs. **Indexer** subprocess JSON and **BiFrost** stdout lines use their own taxonomies ([`log-view-indexer.md`](log-view-indexer.md), [`log-bifrost.md`](log-bifrost.md)).
- The **gateway service card** uses **`gatewayCardModel`** (derive) for operator KV, counters, and subtitle; probe rows are hideable per P5..

---

## Locked decisions (proposed — confirm during P1)

| Topic | Decision |
|-------|-----------|
| Slug prefix | **`gateway.*`** for parent-process / lifecycle lines that are **not** domain-specific. Domain prefixes stay: `chat.*`, `rag.*`, `ingest.*`, `routing.*`, **`gateway.auth.*`** (gateway-issued **client** credentials file / append / upstream key autogen — **not** LLM usage tokens), `config.*`, `conversation.*`, `indexer.*` (owned), `qdrant.*` (owned by `log-qdrant.md`), `bifrost.*` (owned by [`log-bifrost.md`](log-bifrost.md), **status `done`**). Structured fields may still name tokenizer/usage concepts (`outgoingTokens`, `usageTotalTokens`, …); the dotted **`msg`** slug must not overload **`tokens.`** for auth. |
| Casing & separators | Lower-snake **dotted** slugs (`gateway.startup.listening`), aligning with `qdrant.*` / `indexer.*` precedent. |
| Headline rewrite | The **first arg** to `slog.Info/Warn/Error` is rewritten as a **short operator sentence** (e.g. `"gateway listening"` not `"chimera serve: gateway listening"`); structured fields carry the detail. Avoid logger-prefix duplication when the slug already conveys the kind. |
| Levels | **Info** = operator-relevant state changes & per-request milestones at low volume. **Warn** = degraded but auto-handled. **Error** = user-visible failure or data loss. **Debug** = developer-only / per-line traces. **Trace** (via `platform.LevelTrace`) reserved for ingest body excerpts and per-hit RAG. |
| Backwards compat | When renaming a slug used by the UI today, accept **both** old and new names in derive modules for **one release window**, then remove the alias. |
| New objects | New `gateway.*` objects start at **`level:"INFO"`** unless they are pure debug; emit them at **first observation** per process start (e.g. `gateway.startup.listening` is one-shot, not periodic). |

### Code references

- Slugs today ([implementation map](#implementation-map-where-parent-process-msg-is-emitted)): [`internal/chat/chat.go`](../../internal/chat/chat.go), [`internal/server/server.go`](../../internal/server/server.go), [`internal/server/ingest.go`](../../internal/server/ingest.go), [`internal/server/ingest_session.go`](../../internal/server/ingest_session.go), [`internal/rag/service.go`](../../internal/rag/service.go), [`internal/indexer/*`](../../internal/indexer/), [`internal/upstream/upstream.go`](../../internal/upstream/upstream.go), [`internal/tokens/tokens.go`](../../internal/tokens/tokens.go), [`internal/config/config.go`](../../internal/config/config.go), [`internal/conversationmerge/service.go`](../../internal/conversationmerge/service.go), [`internal/server/runtime.go`](../../internal/server/runtime.go).
- UI: [`internal/server/embedui/logs.js`](../../internal/server/embedui/logs.js) (`gatewayServicePanelMiniHtml`, `entryIsGatewayUpstreamRelay`, summarized panel routing).

## Scope

This document is the **single reference** for **gateway-parent** structured logs: lines emitted by `cmd/chimera` (serve and non-serve HTTP entry) and gateway-core `internal/` packages that land in the **gateway** service log via `internal/servicelogs` / the logs UI.

**Out of scope here** (separate taxonomies / docs):

- **`internal/indexer/*.go`** subprocess — `indexer.*` `msg` values; see [`docs/indexer.md`](../indexer.md) and indexer-specific plans.
- **BiFrost subprocess stdout** after `bifrostline` normalization — [`log-bifrost.md`](log-bifrost.md).
- **Qdrant** HTTP client lines from the gateway — [`log-qdrant.md`](log-qdrant.md) where applicable.

## Implementation map (where parent-process `msg` is emitted)

Use this map to find **source files** for a slug family. Line numbers are intentionally omitted so the doc does not require churn on every edit. P1 inventory was validated against sources **2026-05-09**.

### Entrypoints

| Path | Slug families |
|------|----------------|
| [`cmd/chimera/serve.go`](../../cmd/chimera/serve.go) | `gateway.startup.*` (seed, disk log, bootstrap, listening), `gateway.listen.failed`, `gateway.shutdown.http`, `gateway.http.server_error`, `gateway.supervisor.indexer.not_started`; `waitForChildExit`: `gateway.supervisor.child.exited`, `gateway.shutdown.child_force_kill`, `gateway.shutdown.child_stuck` |
| [`cmd/chimera/gateway.go`](../../cmd/chimera/gateway.go) | Non-`serve` HTTP entry only: same lifecycle slugs as serve — `gateway.startup.listening`, `gateway.shutdown.http`, `gateway.http.server_error` (and seed when used). |

### Config, credentials, routing

| Path | Slug families |
|------|----------------|
| [`internal/config/config.go`](../../internal/config/config.go) | `chat.provider_limits.config_invalid`, `chat.provider_limits.config_missing`, `routing.fallback_chain.empty`, `rag.config.invalid`, `conversation.merge.disabled_no_metrics`, `gateway.startup.config_resolved` |
| [`internal/config/upstream_api_key.go`](../../internal/config/upstream_api_key.go) | `gateway.auth.upstream_api_key.autogen` |
| [`internal/tokens/tokens.go`](../../internal/tokens/tokens.go) | `gateway.auth.*` (reload, missing, read/parse failures) |
| [`internal/routing/routing.go`](../../internal/routing/routing.go) | `routing.policy.*`, `routing.rule.*` |

### Server core

| Path | Slug families |
|------|----------------|
| [`internal/server/runtime.go`](../../internal/server/runtime.go) | `gateway.metrics.init_failed`, `gateway.rag.init_failed`, `gateway.config.*` |
| [`internal/server/server.go`](../../internal/server/server.go) | `conversation.merge.resolve_failed`, `chat.request`, `rag.retrieve.error`, `rag.retrieve.source`, `gateway.http.access` |
| [`internal/server/http_multi.go`](../../internal/server/http_multi.go) | `gateway.listen.skipped`; Debug `http serve exit` uses `gateway.http.server_error` when `Serve` returns unexpectedly |
| [`internal/server/ingest.go`](../../internal/server/ingest.go) | `ingest.failed`, `ingest.complete` |
| [`internal/server/ingest_session.go`](../../internal/server/ingest_session.go) | `ingest.chunked.error`, `ingest.complete` |
| [`internal/server/ui_handlers.go`](../../internal/server/ui_handlers.go) | `ui.session.error` |
| [`internal/server/ui_tokens.go`](../../internal/server/ui_tokens.go) | `gateway.auth.append_failed` (`surface=ui`) |
| [`internal/server/ui_bootstrap.go`](../../internal/server/ui_bootstrap.go) | `gateway.auth.append_failed` (`surface=bootstrap`) |
| [`internal/server/availablemodels.go`](../../internal/server/availablemodels.go) | `gateway.catalog.auditor_panic`, `chat.bifrost.available_models` |

### Domains

| Path | Slug families |
|------|----------------|
| [`internal/chat/chat.go`](../../internal/chat/chat.go) | `chat.bifrost.*`, `chat.routing.*`, `chat.provider_limits.*`, `chat.bifrost.outgoing_tokens_count_failed`, `chat.routing.virtual_model_skipped` |
| [`internal/rag/service.go`](../../internal/rag/service.go) | `rag.*`, `rag.ingest.trace` |
| [`internal/conversationmerge/service.go`](../../internal/conversationmerge/service.go) | `conversation.merge.*` |
| [`internal/upstream/upstream.go`](../../internal/upstream/upstream.go) | `upstream.*` |
| [`internal/upstream/smoke_chat.go`](../../internal/upstream/smoke_chat.go) | `upstream.smoke_chat.failed` |
| [`internal/transform/toolrouter.go`](../../internal/transform/toolrouter.go) | `chat.tool_router.*` |

### Supervisor and metrics

| Path | Slug families |
|------|----------------|
| [`internal/supervisor/qdrant.go`](../../internal/supervisor/qdrant.go) | `gateway.supervisor.qdrant.starting` |
| [`internal/supervisor/bifrost.go`](../../internal/supervisor/bifrost.go) | `gateway.supervisor.bifrost.starting`, `gateway.supervisor.bifrost.ready`, `gateway.supervisor.qdrant.ready` |
| [`internal/supervisor/indexer.go`](../../internal/supervisor/indexer.go) | `gateway.supervisor.indexer.starting`, `gateway.supervisor.indexer.raw_exec` |
| [`internal/gatewaymetrics/store.go`](../../internal/gatewaymetrics/store.go) | `gateway.metrics.disabled_after_error` |
| [`internal/gatewaymetrics/migrate.go`](../../internal/gatewaymetrics/migrate.go) | `gateway.metrics.migration_applied` |

### Edge notes (not separate slugs)

- **Disk log:** Warn lines for `disk log: mkdir` / `disk log: open` share the **`gateway.startup.disk_log`** family with **`disk logging enabled`** (Info).
- **Indexer supervised `getwd`:** Same **`gateway.supervisor.indexer.not_started`** as “not started”, with structured **`detail=getwd`** (and distinct headline text) so operators get one pill.
- **Child kill failure:** `{name} kill failed` reuses **`gateway.shutdown.child_force_kill`** with **`detail=kill_send_failed`** (still shutdown-related).
- **`http_multi`:** Optional Debug **`http serve exit`** maps to **`gateway.http.server_error`** when the server stops with a non-`ErrServerClosed` error.

## Canonical `msg` taxonomy

Stable dotted slugs across gateway-emitted lines. Every `slog.Info/Warn/Error/Debug` call should set `msg` (when it doesn't exist today, P2 adds it).

**Retired / folded:** slug names removed from this revision are listed under [Removed or merged slugs](#removed-or-merged-slugs-revision-2026-05-08) so implementors know what not to add anew.

### Lifecycle / process (`gateway.*`)

| `msg` | Source today (or "**new**") | Level | Notes |
|-------|------------------------------|-------|-------|
| `gateway.startup.seed` | **new** — replace raw `fmt.Fprintln(..., "chimera.start")` with structured line | Info | One-shot buffer seed for desktop; KV: none or `semver` if added later. |
| `gateway.startup.config_resolved` | `internal/config/config.go` `resolved gateway config paths` | **Info** *(was Debug)* | Headline: **"gateway config resolved"**. KV: `filePath`, **`api_keys_path`** (resolved path to client credential YAML; today’s code logs `tokensPath` until [`version-v0.3.md`](../version-v0.3.md) config rename), `routingPolicyPath`. |
| `gateway.startup.bootstrap` | `cmd/chimera/serve.go` bootstrap mode notice | Info | Headline: **"gateway bootstrap mode"**. KV: **`api_keys_path`** (spec; field may remain `tokens_path` in code until v0.3). |
| `gateway.startup.listening` | `cmd/chimera/serve.go` `chimera serve: gateway listening` | Info | Headline: **"gateway listening"**. KV: `addr`, `ui`, `upstream`, `bifrost_data`, `qdrant_supervised`, `indexer_supervised`, `config`. |
| `gateway.startup.disk_log` | `cmd/chimera/serve.go` `disk logging enabled` (+ Warn `disk log: mkdir` / `disk log: open` same family) | Info / Warn | KV: `path` where applicable. |
| `gateway.listen.failed` | `cmd/chimera/serve.go` / `internal/server/server.go` listen errors (`listen`, `chimera serve: listen`) | Error | KV: `addr` / `addrs`, `err`. Unifies bootstrap and non-bootstrap listen failures. |
| `gateway.http.server_error` | `cmd/chimera/serve.go` / `cmd/chimera/gateway.go` HTTP server exit; Debug **`http serve exit`** in `internal/server/http_multi.go` when `Serve` errors | Error / Debug | KV: `err`. |
| `gateway.shutdown.http` | `cmd/chimera/serve.go` `http shutdown` | Warn → **Info** | Headline: **"gateway http shutdown"**. KV: `err` (optional). |
| `gateway.shutdown.child_force_kill` | `cmd/chimera/serve.go` `did not exit after context cancel; forcing kill`; `{name} kill failed` reuses slug with `detail=kill_send_failed` | Warn | KV: `child` (`qdrant` \| `bifrost` \| `indexer`), `pid`, `timeout`, optional `detail`. |
| `gateway.shutdown.child_stuck` | `cmd/chimera/serve.go` `still has not exited after forced kill` | Warn | KV: `child`. |
| `gateway.config.reloaded` | `internal/server/runtime.go` `reloaded gateway.yaml` | Info | KV: `path`. |
| `gateway.config.reload_failed` | `internal/server/runtime.go` `failed to reload gateway.yaml` | Error | KV: `path`, `err`. |
| `gateway.config.missing` | `internal/server/runtime.go` `gateway config missing` | Error | KV: `path`, `err`. |
| `gateway.rag.init_failed` | `internal/server/runtime.go` `rag init failed` | **Warn** | KV: `err`. Distinct from `rag.config.invalid` (YAML); this is runtime attach failure. |
| `gateway.metrics.disabled_after_error` | `internal/gatewaymetrics/store.go` `gateway metrics disabled after write error` | Error | KV: `step`, `err`. |
| `gateway.metrics.init_failed` | `internal/server/runtime.go` `gateway metrics init failed` | **Warn** *(was Error — non-fatal, gateway continues)* | KV: `err`. |
| `gateway.metrics.migration_applied` | `internal/gatewaymetrics/migrate.go` `gateway metrics migration applied` | Info | KV: `version`, `file`. |
| `gateway.catalog.auditor_panic` | `internal/server/availablemodels.go` `catalog auditor panicked` | Error | KV: `panic`. Defensive; rare. |
| `gateway.listen.skipped` | `internal/server/http_multi.go` `listen skipped` | Warn | KV: `addr`, `err`. Multi-listener bootstrap only. |

### Supervisor (`gateway.supervisor.*`)

Structured lifecycle for supervised children (`cmd/chimera/serve.go`, `internal/supervisor/*`). Operators use these to distinguish **startup success** from **relay failures** (`chat.bifrost.*`).

| `msg` | Source today (or "**new**") | Level | Notes |
|-------|------------------------------|-------|-------|
| `gateway.supervisor.qdrant.starting` | `internal/supervisor/qdrant.go` via serve `starting qdrant subprocess` | Info | KV: `bin`, `storage`, `http_port`, `grpc_port`, `host` (omit `raw` from stable KV). |
| `gateway.supervisor.bifrost.starting` | `internal/supervisor/bifrost.go` via serve `starting bifrost subprocess` | Info | KV: `bin`, `app_dir`, `host`, `port`. |
| `gateway.supervisor.bifrost.ready` | `internal/supervisor/bifrost.go` `bifrost health OK` | Info | KV: `url`. Confirms readiness poll — pair with BiFrost card. |
| `gateway.supervisor.qdrant.ready` | `internal/supervisor/bifrost.go` `WaitHealthy` (qdrant `/readyz` path) `qdrant health OK` | Info | KV: `url`. Same helper as BiFrost; **child** disambiguates log line + `msg`. |
| `gateway.supervisor.indexer.starting` | `internal/supervisor/indexer.go` `indexer supervised` | Info | KV: `bin`, `config`, `workdir`, `log_json`. |
| `gateway.supervisor.indexer.raw_exec` | `internal/supervisor/indexer.go` `indexer supervised (raw exec)` | Debug | KV: `bin`, `args`. Test / special layout only. |
| `gateway.supervisor.indexer.not_started` | `cmd/chimera/serve.go` `indexer supervised not started` and **`indexer supervised: getwd`** | Warn | KV: `err`, `bin`; **`detail=getwd`** when `getwd` fails. Config present but process did not launch. |
| `gateway.supervisor.child.exited` | **new** — wrap `waitForChildExit` / `cmd.Wait` when `err != nil` after gateway shutdown | **Debug** | KV: `child`, `err`. Optional Info if exit is unexpected mid-run (future). |

### Gateway client credentials / auth (`gateway.auth.*`)

Stable **`msg`** prefix for **gateway-issued client access** (Bearer / Continue `apiKey` file, UI append, bootstrap setup). Does **not** cover LLM usage, tokenizer counts, or BiFrost subprocess auth (`bifrost.jwt.startup`, …). Aligns with credential naming in [`version-v0.3.md`](../version-v0.3.md).

| `msg` | Source today | Level | Notes |
|-------|--------------|-------|-------|
| `gateway.auth.reloaded` | `internal/tokens/tokens.go` `reloaded gateway API tokens` | Info | Headline: **"gateway client credentials reloaded"** (or **"API keys reloaded"**). KV: `path`, `count` (number of entries). Human copy in code may still say "tokens" until P2 headline pass. |
| `gateway.auth.file_missing` | `internal/tokens/tokens.go` `tokens file missing` | Error | KV: `path`, `err`. |
| `gateway.auth.read_failed` | `internal/tokens/tokens.go` `read tokens yaml` | Error | KV: `path`, `err`. |
| `gateway.auth.parse_failed` | `internal/tokens/tokens.go` `failed to parse tokens yaml` | Error | KV: `path`, `err`. |
| `gateway.auth.append_failed` | `internal/server/ui_tokens.go` `append token` / `internal/server/ui_bootstrap.go` `setup append token` | Error | KV: `err`, **`surface`** (`ui` \| `bootstrap`). **Single slug** for both code paths (replaces separate `ui.tokens.*` rows). |
| `gateway.auth.upstream_api_key.autogen` | `internal/config/upstream_api_key.go` `wrote auto-generated upstream.api_key to gateway.yaml` | Info | KV: `path`. Upstream-facing API key material in `gateway.yaml`, distinct from per-tenant client rows in the credential YAML file. |

### Routing / providers / chat (`routing.*`, `chat.*`)

| `msg` | Source today | Level | Notes |
|-------|--------------|-------|-------|
| `routing.policy.reloaded` | `internal/routing/routing.go` `reloaded routing policy` | Info | KV: `path`, `rules`. |
| `routing.policy.read_failed` | `internal/routing/routing.go` `read routing policy` | Error | KV: `path`, `err`. |
| `routing.policy.parse_failed` | `internal/routing/routing.go` `failed to parse routing policy yaml` | Error | KV: `path`, `err`. |
| `routing.policy.missing` | `internal/routing/routing.go` `routing policy file missing` | Error | KV: `path`, `err`. |
| `routing.fallback_chain.empty` | `internal/config/config.go` `routing.fallback_chain is empty or missing` | Warn | KV: none. |
| `routing.rule.matched` | `internal/routing/routing.go` `routing rule matched` | **Debug** *(keep)* | KV: `rule`, `initialModel`, `lastUserChars`. |
| `routing.rule.no_match.ambiguous_default` | `internal/routing/routing.go` `routing: no rule matched, using ambiguous_default_model` | Debug | KV: `initialModel`, `lastUserChars`. |
| `routing.rule.no_match.first_fallback` | `internal/routing/routing.go` `routing: no policy default; using first fallback_chain entry` | Debug | KV: `initialModel`, `lastUserChars`. |
| `chat.request` | `internal/server/server.go` `chat completion request` | Info | Ensure `conversation_id`, `principal_id`, `request_id` present (`routeLog.With(...)`). |
| `chat.bifrost.available_models` | `internal/server/availablemodels.go` `upstream models (merged list)` / `upstream models unavailable` | Info / Warn | Catalog poll for routing; KV: `catalog_model_count`, `providers`, `ok`, `err`, … — summarized headlines in [`log-bifrost.md`](log-bifrost.md) §Reference. |
| `chat.bifrost.request` | `internal/chat/chat.go` `upstream chat relay` | Info | Covered by [`log-bifrost.md`](log-bifrost.md). |
| `chat.bifrost.response` | `internal/chat/chat.go` `logUpstreamChatResponse` (**`msg`** on attrs) | Info | [`log-bifrost.md`](log-bifrost.md) P4 **done**. |
| `chat.bifrost.error` | `internal/chat/chat.go` `upstream chat fetch failed` | **Warn** *(P3; today **Info**)* | Operator-actionable; [`log-bifrost.md`](log-bifrost.md). Already carries `"msg","chat.bifrost.error"`. |
| `chat.routing.fallback` | `internal/chat/chat.go` `retrying next fallback model` | Info | Keep slug. |
| `chat.routing.attempt` | `internal/chat/chat.go` `routing attempt` + **`msg`** | **Info** when the fallback chain has more than one model, else **Debug** | [`log-bifrost.md`](log-bifrost.md) P4 **done**. |
| `chat.routing.resolved` | `internal/chat/chat.go` `routing resolved` + **`msg`** | Info | [`log-bifrost.md`](log-bifrost.md) P4 **done**. |
| `chat.provider_limits.blocked` | `internal/chat/chat.go` (same human line + **`msg`**) | Info | [`log-bifrost.md`](log-bifrost.md) P4 **done**. |
| `chat.provider_limits.query_failed` | `internal/chat/chat.go` `provider limits admission query failed` | Warn | KV: `err`, `upstreamModel`. |
| `chat.provider_limits.config_invalid` | `internal/config/config.go` `provider-model-limits.yaml invalid` / `provider free tier yaml invalid` | Error | KV: `path`, `err`. |
| `chat.provider_limits.config_missing` | `internal/config/config.go` `provider free tier path not stat-able` / `routing.filter_free_tier_models is true but provider-free-tier.yaml missing` | Warn | KV: `path` (when known). |
| `chat.tool_router.skipped` | `internal/transform/toolrouter.go` `tool router skipped or failed; passing all tools` | **Debug** *(keep)* | KV: `err`. Fail-open path. |
| `chat.tool_router.applied` | `internal/transform/toolrouter.go` `tool router slimmed tools` | **Info** *(today)* → **Debug** *(P3 target)* | KV: `routerModel`, `before`, `after`, `threshold`. |
| `chat.tool_router.model_attempt_failed` | `internal/transform/toolrouter.go` `tool router model attempt failed` | Debug | KV: `routerModel`, `err`. Per-router-model try in the inner loop. |
| `chat.bifrost.outgoing_tokens_count_failed` | `internal/chat/chat.go` `outgoing token count failed` | Debug | KV: `err`. Tokenizer estimate for request logging failed; relay still proceeds. |
| `chat.routing.virtual_model_skipped` | `internal/chat/chat.go` `virtual model skipping model (413 earlier this request)` | Debug | KV: `upstreamModel`, `index`. |

### RAG / ingest (`rag.*`, `ingest.*`)

| `msg` | Source today | Level | Notes |
|-------|--------------|-------|-------|
| `rag.config.invalid` | `internal/config/config.go` `rag config invalid; disabling RAG` | Error | KV: `err`. |
| `rag.retrieve.error` | `internal/server/server.go` `rag retrieve failed; proceeding without context` | Warn | Keep. |
| `rag.retrieve.source` | `internal/server/server.go` `rag retrieved hits from source` | **Info** *(today)* → **Debug** *(P3 target)* | One line per source with hits after successful retrieve; KV: `rel`, `source_hits`, `tenant_id`, `project_id`, `flavor_id`. **Replaces** the older planned `rag.retrieve.ok` / “rag context injected” single-line shape (not present in current code). |
| `rag.query` | `internal/rag/service.go` `rag search query` | **Info** *(today)* → **Debug** *(P3 target)* | KV unchanged. |
| `rag.embed` | `internal/rag/service.go` `rag embedding retrieved` | **Info** *(today)* → **Debug** *(P3 target)* | KV unchanged. |
| `rag.hit` | `internal/rag/service.go` `rag comparison` | Debug | KV unchanged. |
| `rag.ingest.trace` | `internal/rag/service.go` `rag ingest` | Trace | Keep. |
| `rag.ingest.delete_pre_failed` | `internal/rag/service.go` `delete-by-source pre-ingest failed` | Debug | Keep. |
| `ingest.complete` | `internal/server/ingest.go` / `ingest_session.go` | Info | Align KV: `tenant`, `source`, `chunks`, `request_id`, `index_run_id`, `service`, `principal_id`. |
| `ingest.failed` | `internal/server/ingest.go` `ingest failed` | Error | **P2 adds `msg`**. KV: `err`, `source`, `tenant`, `request_id`, `index_run_id`. |
| `ingest.chunked.error` | `internal/server/ingest_session.go` `chunked ingest failed` | Error | **Canonical** slug for chunked path; KV: same family as `ingest.failed`. |

### Conversation merge (`conversation.*`)

| `msg` | Source today | Level | Notes |
|-------|--------------|-------|-------|
| `conversation.merge.disabled` | `internal/conversationmerge/service.go` `conversation merge disabled: missing embedding URL or upstream API key` | **Info** *(was Warn)* | Operator configuration choice. |
| `conversation.merge.embed_failed` | `internal/conversationmerge/service.go` `conversation merge: embed failed` | Warn | KV: `err`. |
| `conversation.merge.embed_dim_mismatch` | `internal/conversationmerge/service.go` `embedding dim mismatch` | Warn | KV: `got`, `want`. |
| `conversation.merge.list_candidates_failed` | `internal/conversationmerge/service.go` `list candidates failed` | Warn | KV: `err`. |
| `conversation.merge.dedup_read_failed` | `internal/conversationmerge/service.go` `dedup read failed` | Debug | KV: `err`. |
| `conversation.merge.upsert_failed` | `internal/conversationmerge/service.go` `upsert failed` | Warn | KV: `err`. |
| `conversation.merge.snapshot_upsert_failed` | `internal/conversationmerge/service.go` `resolve snapshot upsert failed` | Warn | KV: `err`, `conversation_id`. |
| `conversation.merge.dedup_cache_write_failed` | `internal/conversationmerge/service.go` `dedup cache write failed` | Debug | KV: `err`. |
| `conversation.merge.resolve_failed` | `internal/server/server.go` `conversation merge resolve failed` | Debug | KV: `err`. Best-effort merge before chat; request continues with a fresh id. **Lifecycle note:** [`log-conversations.md`](log-conversations.md) uses `conversation.merge.failed` as a doc-only umbrella; filters should match **`conversation.merge.*`**, not a separate emitted slug. |
| `conversation.merge.disabled_no_metrics` | Removed: conversation merge now uses operator SQLite and no longer depends on `metrics.enabled`. | N/A | N/A |

### Upstream / health (`upstream.*`)

| `msg` | Source today | Level | Notes |
|-------|--------------|-------|-------|
| `upstream.models.fetch_failed` | `internal/upstream/upstream.go` `upstream models fetch failed` | **Warn** | KV: `err`, `target`. |
| `upstream.models.non_ok` | `internal/upstream/upstream.go` `upstream models non-OK` | **Warn** | KV: `status`, `target`. |
| `upstream.models.ok` | `internal/upstream/upstream.go` `upstream models` (Debug today) | Debug | KV: `count`, `target`. |
| `upstream.health.probe_failed` | `internal/upstream/upstream.go` `upstream health probe failed` | **Warn** | KV: `err`, `target`. |
| `upstream.smoke_chat.failed` | `internal/upstream/smoke_chat.go` `smoke chat completion failed` | **Debug** | Diagnostic / CI only; demote to reduce noise (optional **Warn** behind flag). KV: `err`, `target`. |

### UI / sessions (`ui.*`)

| `msg` | Source today | Level | Notes |
|-------|--------------|-------|-------|
| `ui.session.error` | `internal/server/ui_handlers.go` `ui session issue` | **Debug** *(default)* | Use **Error** only when persistence write fails (split code paths in P3). |

### HTTP access (`gateway.http.access`)

| `msg` | Source today | Level | Notes |
|-------|--------------|-------|-------|
| `gateway.http.access` | `internal/server/server.go` `loggingMiddleware` (headline `http response`) | Info / **Debug** on probe 2xx | **Canonical slug** for inbound gateway HTTP. KV: `method`, `path`, `statusCode`, `responseTimeMs`, `authorization` (redacted), `service`, `request_id`. **Alias for one release:** also accept legacy shape detection via `msg=http response` or absence of `msg` (UI `inferShape`). Demote 2xx on `/health`, `/status`, `/api/ui/logs`, SSE to Debug per [P3](#p3--level-reclassification). |

### Removed or merged slugs (revisions 2026-05-08, 2026-05-09)

| Removed | Replaced by | Reason |
|---------|-------------|--------|
| `routing.rule.no_match` (combined) | `routing.rule.no_match.ambiguous_default`, `routing.rule.no_match.first_fallback` | Two different fallbacks; one slug was ambiguous. |
| `ingest.chunked.failed` | `ingest.chunked.error` | Duplicate intent; single canonical slug. |
| `ui.tokens.append_failed`, `ui.bootstrap.append_failed` | `gateway.auth.append_failed` + `surface` | Same failure class; fewer pills for operators. |
| `http.access` (bare slug in §HTTP) | `gateway.http.access` | Aligns with `gateway.*` prefix decision; old slug remains a UI alias only. |
| `rag.retrieve.ok` / “rag context injected” (never implemented as `msg` here) | `rag.retrieve.source` | Success path logs one line per contributing **source** after retrieve (`server.go`). |
| Planned **`tokens.*`** `msg` prefix for gateway client credentials | **`gateway.auth.*`** | Avoids operator confusion with **LLM / tokenizer / usage** “tokens”; aligns [`version-v0.3.md`](../version-v0.3.md) credential naming. |

---

## UI contract: Gateway service card (summarized logs)

Applies to **Logs → Gateway** summary card (`gatewayServicePanelMiniHtml` + `buildServiceCard` + `renderExpandedService` for `name === "gateway"`).

### Collapsed card header

- **Replace** the generic last-message subtitle with a **gateway-aware** subtitle priority:
  1. recent `gateway.config.reload_failed` / `gateway.config.missing` / `gateway.auth.parse_failed` (Error tint).
  2. recent `upstream.health.probe_failed` / `upstream.models.fetch_failed` (Warn tint).
  3. last `gateway.config.reloaded` or `gateway.auth.reloaded` (operator-state change).
  4. last `gateway.startup.listening` (cold state).
  5. fallback to `primaryLogMessage(last.parsed, last.text)` (today's behavior).

### Expanded card — below the summary heading (KV row)

New **key-value** fields:

| Key | Source `msg` / logic |
|-----|---------------------|
| **listening** | `gateway.startup.listening` → `addr` (last value wins). |
| **upstream** | `gateway.startup.listening` → `upstream` (Bifrost URL). |
| **config** | `gateway.config.reloaded` / `gateway.startup.config_resolved` → `path` short label + a tiny indicator if a reload error has happened since last success. |
| **API keys** | `gateway.auth.reloaded` → `count`; tint **error** if last **`gateway.auth.*`** failure was parse/read/missing-file (not LLM usage). |
| **routing rules** | `routing.policy.reloaded` → `rules`. |
| **supervised children** | derived from `gateway.startup.listening` (`qdrant_supervised`, `indexer_supervised`) plus `bifrost_data` presence. |

### Expanded card — summary section (replace current mini-cards)

**Remove:** `HTTP · Σ ms`, `ingest.complete · RAG · chat slugs`, `Warn+error lines` (today's three).

**Add** four counter boxes:

| Box | Behavior |
|-----|----------|
| **HTTP success / fail** | From `gateway.http.access` rows (UI shape `http.access`): 2xx vs 4xx/5xx, with a separate `429` sub-count when present. |
| **Chat (req → resp)** | `chat.request` count → `chat.bifrost.response` count, with `chat.bifrost.error` shown as fail. |
| **RAG (queries · hits)** | `rag.query` count · `rag.hit` count (both already slugged); subtitle: latest `rag.retrieve.error` reason when present. |
| **Ingest (ok / fail)** | `ingest.complete` count vs `ingest.failed` + `ingest.chunked.error`. |

### Full event log (expanded)

- **Suppress** the **gateway** source badge in this panel only (mirror `suppressQdrantBadge` / `suppressIndexerBadge`). Use the existing per-row shape badge to keep distinctness.
- Default filter inside the gateway panel **hides** `gateway.http.access` rows for `/api/ui/logs`, `/health`, `/status` (operators can re-enable via the existing level/app filters). Implementation: extend `entryIsGatewayUpstreamRelay` style helpers with a `gatewayPanelHideRow(ent)` predicate.

---

## Phased implementation

### P1 — Inventory & spec

**Goal.** Land this doc, frozen taxonomy, and a durable **implementation map** (source file → slug families).

**Deliverables**

- This file (taxonomy frozen for parent-process gateway logs; **`gateway.auth.*`** for client credential **`msg`** slugs — no `tokens.` prefix on `msg`).
- **[Implementation map](#implementation-map-where-parent-process-msg-is-emitted)** — gateway-parent packages and entrypoints that emit structured `msg` values, aligned with this taxonomy. *(Supersedes the former line-level [`gateway-log-audit.md`](gateway-log-audit.md); merged 2026-05-09.)*

**Acceptance.** A reviewer can map **every row** in the implementation map to slug families in the taxonomy (or to “out of scope” in [Scope](#scope)).

**P1 validation (2026-05-09).** Cross-checked against [`log-bifrost.md`](log-bifrost.md) (**status `done`**): gateway relay `chat.bifrost.*` / routing / provider-limit slugs and headlines match implemented `internal/chat/chat.go` + `internal/server/availablemodels.go` as of this date; remaining gaps are **missing `msg`** on many lines (P2) and **level** drift (e.g. `rag.query` / `rag.embed` / `rag.retrieve.source` still **Info** in code; P3 demotes to Debug). RAG retrieve success path is **`rag.retrieve.source`**, not legacy `rag.retrieve.ok`.

**Status:** `done`

### P2 — `msg` slugs everywhere

**Goal.** Every gateway `slog` call carries `msg`; renames applied where the table changes a slug.

**Deliverables**

- Edits to `internal/chat/chat.go`, `internal/server/server.go` (including `loggingMiddleware` → `gateway.http.access`), `internal/server/http_multi.go`, `internal/server/ingest.go`, `internal/server/ingest_session.go`, `internal/server/runtime.go`, `internal/server/ui_handlers.go`, `internal/server/ui_tokens.go`, `internal/server/ui_bootstrap.go`, `internal/upstream/upstream.go`, `internal/upstream/smoke_chat.go`, `internal/tokens/tokens.go`, `internal/config/config.go`, `internal/conversationmerge/service.go`, `internal/routing/routing.go`, `internal/transform/toolrouter.go`, `internal/supervisor/*.go` (supervisor uses parent `slog` — add `gateway.supervisor.*` where those lines are emitted) — add `"msg", "<slug>"` to every `slog.Info/Warn/Error/Debug` (and `Trace` where used) per the taxonomy.
- Backwards-compat: [`internal/server/embedui/logs.js`](../../internal/server/embedui/logs.js) and `derive/bifrostMetrics.js` still match **legacy** `msg`/`message` strings from older gateway builds where P4 slugs were absent (`upstream chat response`, old virtual-model wording).
- `cmd/chimera/serve.go`: add `gateway.startup.*`, `gateway.shutdown.*`, `gateway.supervisor.*`, `gateway.startup.seed`, and structured supervisor/indexer failure lines per taxonomy.

**Acceptance.** Every gateway-parent `slog.Logger` `Info` / `Warn` / `Error` / `Debug` / `Log` call in the P2 file list carries a `"msg", "<slug>"` pair per the taxonomy and [implementation map](#implementation-map-where-parent-process-msg-is-emitted); [`internal/server/embedui/logs.js`](../../internal/server/embedui/logs.js) `inferShape` treats **`gateway.http.access`** like legacy **`http response`** for `http.access` UI shape.

**Status:** `done`

### P3 — Level reclassification

**Goal.** The default Info+ stream is the operator story; Debug is the developer story.

**Deliverables**

- Apply the **Level** column from the taxonomy table.
- Drop the `loggingMiddleware` Info for 2xx requests on `/api/ui/logs`, `/health`, `/status`, and SSE endpoints — emit at Debug instead. Keep Info for everything else and for any non-2xx.
- Promote `chat.bifrost.error` from Info → Warn (operator-actionable).
- Demote `routing.rule.*` debug lines (already Debug — verify).
- Demote `ui.session.error` from Error → Debug for benign cookie reissue paths; keep Error only when persistence fails.

**Acceptance.** With `LOG_LEVEL=info`, the cold-start fixture produces ≤30 lines (down from current baseline measured in P1) and every line maps to an operator-meaningful taxonomy entry.

**Status:** `done`

### P4 — New gateway objects

**Goal.** Emit structured events the UI needs but the gateway does not produce today.

**Deliverables**

- `gateway.startup.listening` (replace today's `chimera serve: gateway listening` headline by adding the slug; KV stays the same).
- `gateway.startup.bootstrap` for the bootstrap-mode notice.
- `gateway.startup.config_resolved` (promote the existing Debug to Info with a slug).
- `gateway.config.reloaded` / `gateway.config.reload_failed` / `gateway.config.missing` (slug existing).
- `gateway.auth.reloaded` and the `gateway.auth.*_failed` family (slug existing lines in `internal/tokens` / UI append paths).
- `routing.policy.reloaded` and the `routing.policy.*_failed` family (slug existing).
- `upstream.health.probe_failed`, `upstream.models.*` (slug existing + level adjust).
- `gateway.health.upstream` periodic event (**new** — every N seconds when `upstream.base_url` probe state changes; **not** every poll). Driven by a new helper in `internal/upstream/`.
- `gateway.health.qdrant` and `gateway.health.bifrost` (**new** — emitted by gateway when supervised child health flips). These power the new gateway card KV row "supervised children".

**Acceptance.** Card KV row for **listening / upstream / config / API keys / routing rules / supervised children** populates from cold-start fixture without any field staying `—` after the first 60s of normal traffic.

**Status:** `done`

### P5 — Card UI cleanup

**Goal.** Gateway card matches the KV / counters / subtitle / full-log spec above.

**Deliverables**

- New `internal/server/embedui/logs/derive/gatewayCardModel.js` (parallel to `qdrantCardModel`) that exposes `subtitle`, `kv`, and `counters` from `entryCache` so logic stays goja-testable.
- Update `gatewayServicePanelMiniHtml` (or replace it) in `internal/server/embedui/logs.js`: render KV row + four counter boxes; remove the three legacy mini-cards.
- Add `suppressGatewayBadge` plumbing.
- Add `gatewayPanelHideRow(ent)` predicate to filter `/api/ui/logs` / `/health` / `/status` 2xx access lines from the panel by default; expose a "show probes" toggle in the panel header.
- Refresh `internal/server/logs_components_test.go`, `internal/server/ui_logs_test.go`, and add a derive test under `internal/server/embedui/logs/derive/` (goja).

**Acceptance.** Operator can read **listening / upstream / config / API keys / routing rules / supervised children** + the four counters without expanding any single log row; `/health` polling does not crowd the panel.

**Status:** `done`

### P6 — Demote / retire

**Goal.** Reduce buffer noise; retire ineffective lines.

**Deliverables**

- Retire (delete) any `slog` call that the audit shows fires zero times across all three fixtures **and** has no operator value (candidates: leftover Debug breadcrumbs from earlier refactors).
- Document the demotions / retirements in [`log-presentation-layer.md`](log-presentation-layer.md) §10 changelog.

**Acceptance.** Median Info-line rate during a 5-minute mixed-traffic capture drops by ≥30% vs P1 baseline, with zero loss of operator-actionable signal (verified by checking the four counters and KV row are still accurate).

**Status:** `done`

**P6 notes (2026-05-09).** No separate checked-in “three fixtures” in-repo for per-slug hit counts; implemented **demotion** of repeated successful **`chat.bifrost.available_models`** polls (first success **Info**, later **Debug**) per [`log-presentation-layer.md`](log-presentation-layer.md) §10. RAG hot-path lines (`rag.query`, `rag.embed`, `rag.retrieve.source`, `rag.hit`) were already **Debug** in code; tool-router applied line already **Debug**.

---

## Open questions

- **Per-request request_id:** `internal/server/requestid` middleware already adds `request_id` on inbound; confirm it propagates through every `slog.With(...)` chain (chat does, but ingest / RAG paths may drop it on subloggers).
- **HTTP access slug vs shape:** `msg: "gateway.http.access"` is emitted; `inferShape` returns `http.access` for **`gateway.http.access`**, legacy **`http response`**, or `method`+`path`+`statusCode` alone (alias window).
- **Headline wording:** Locale stays English (matches qdrant + indexer); confirm headline rewrites do not break any operator-facing tools that grep the human text.
- **Periodic health events:** `gateway.health.upstream` cadence — fixed interval vs only-on-change. Default proposal: **only-on-change** with a max once-per-30s rate cap, plus a one-shot Info on first observation.
- **Backwards compat window:** How long to keep alias names in derive modules (proposal: one minor release, e.g. v0.4 → v0.5).
- **Probe filter UX:** Should the "hide probes" default also apply to the **structured logs** raw view, or only to the gateway service card?

---

## Checklist before marking done

- [x] Every gateway `slog` call carries **`msg`** (audit script in `scripts/` if useful).
- [x] Levels reclassified per taxonomy; cold-start fixture shrinks materially at Info+.
- [x] New `gateway.*` lifecycle events power the card KV row from cold start.
- [x] Gateway card matches **KV**, **counters**, **subtitle**, **full log** spec above (no gateway pill in that panel; probe rows hidden by default).
- [x] Fixture-backed tests for the derive module and the renamed slugs (`internal/server/embedui/logs/derive/` goja).
- [x] Backwards-compat aliases listed in [`log-presentation-layer.md`](log-presentation-layer.md) §10 with the planned removal release.
