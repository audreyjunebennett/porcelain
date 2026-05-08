# Plan: Operator-facing BiFrost log classification

| Field | Value |
|-------|-------|
| **Doc kind** | `feature-plan` |
| **Owners / areas** | Gateway supervision (`internal/supervisor`), upstream relay (`internal/chat`), logs UI (`internal/server/embedui/logs`), parse/derive (`internal/server/embedui/logs/parse`, `derive`), desktop mirror (`internal/servicelogs`) |
| **Status** | `draft` |
| **Targets** | Gateway + supervised BiFrost (`bifrost-http -log-style json`) — single subprocess per gateway, mirror format unchanged |
| **Last updated** | 2026-05-07 |

## At a glance

The supervised **BiFrost** subprocess (`bifrost-http`) writes JSON lines today, but they reach the operator UI **unclassified**. The bifrost service card is driven almost entirely by **gateway-emitted** relay lines (`chat.bifrost.request`, `upstream chat response`, `chat.bifrost.error`) and ignores what the BiFrost process itself reports about providers, keys, MCP, governance, and rate limits. This plan captures:

1. A **stable `msg` taxonomy** on **every** classified line out of `bifrost-http` (parallel to `qdrant.*` and `indexer.*`).
2. **Operator-facing copy** (subtitle, KV summary fields, counters) derived from those slugs **plus** the existing gateway upstream relay slugs.
3. **UI contract** for the **BiFrost service card** — collapsed pills, expanded mini-cards, full event log — so an operator can answer "which providers are healthy, what model is in use, and why did the last call fail?" at a glance.

**Related docs:** [`supervisor.md`](../supervisor.md), [`bifrost-discovery.md`](../bifrost-discovery.md), [`log-presentation-layer.md`](log-presentation-layer.md), [`log-qdrant.md`](log-qdrant.md), [`log-conversations.md`](log-conversations.md).

| Phase | Outcome | Status |
|-------|---------|--------|
| [P1 — Spec](#p1--spec) | This doc + frozen `bifrost.*` list | `todo` |
| [P2 — Parse & `msg`](#p2--parse--msg) | `internal/servicelogs/bifrostline` normalizer wraps the bifrost stdout/stderr writers | `todo` |
| [P3 — Card UI cleanup](#p3--card-ui-cleanup) | KV fields, counters, replace pills/mini-cards, suppress badge in own panel | `todo` |
| [P4 — Gateway relay alignment](#p4--gateway-relay-alignment) | Reformat / dedupe gateway upstream relay slugs so card draws from one shared vocabulary | `todo` |
| [P5 — Conversation linkage](#p5--conversation-linkage) | Cross-link relay events to conversation cards (depends on [`log-conversations.md`](log-conversations.md)) | `todo` |

---

## Background

- BiFrost is supervised by `claudia serve` (`internal/supervisor/bifrost.go`) and started with `-log-style json` (`internal/supervisor/bifrost.go` `StartBifrost`). Stdout / stderr go straight to **`logStore.Writer("bifrost")`** in `cmd/claudia/serve.go` with **no normalization** (compare to qdrant which goes through `qdrantline.NewWriter`).
- The bifrost card in the logs UI today (`internal/server/embedui/logs.js` `bifrostCardMetrics`, `bifrostCollapsedCardSubtitle`, `renderExpandedService` for `name === "bifrost"`) builds its mini-cards from **gateway**-side lines emitted in `internal/chat/chat.go`:
  - `chat.bifrost.request` (request relay started — outgoing tokens, stream flag, model, request body excerpt).
  - `upstream chat response` (response received — status, usage tokens, response bytes/excerpt).
  - `chat.bifrost.error` (relay fetch failed).
  - `chat.routing.fallback`, `virtual model fallback attempt`, `virtual model routing resolved` (routing decisions).
- Anything BiFrost **itself** logs (provider key load, governance plugin, MCP startup, JWT auth, listening port, internal errors) flows to the bifrost bucket as raw JSON without a stable `msg`. The card has no way to surface it.
- **Implementation requirement:** after normalization, **every** bifrost-derived row exposed to the UI should carry a **`msg`** field (same pattern as `indexer.*` and `qdrant.*` slogs). Gateway-emitted relay lines already do; subprocess lines should match.

---

## Locked decisions (proposed — confirm during P1)

| Topic | Decision |
|-------|-----------|
| Slug prefix | **`bifrost.*`** for subprocess-origin events (parallel to `qdrant.*`); **`chat.bifrost.*`** stays on gateway-origin relay events. Both feed the same card. |
| Normalization location | **On ingest:** new `internal/servicelogs/bifrostline` package wraps the bifrost stdout/stderr writer in `cmd/claudia/serve.go`, mirroring `internal/servicelogs/qdrantline/`. |
| Counter window | Aggregates use lines **at or after the last `bifrost.startup.banner`** in the buffer (detect bifrost restart while gateway stays up). Gateway restart clears the ring buffer. |
| HTTP success | Real status codes shown; **2xx** counts as success for relay/response counters; **4xx/5xx** as fail; **429** breaks out separately as **rate-limited**. |
| Non-JSON lines | If a line is not JSON or schema is unknown, emit `bifrost.unparsed` with the raw text in `progress_detail` (mirrors `qdrant.unparsed`). |
| Timeline | Keep the **request timeline** + bar on the bifrost panel (unlike qdrant); chat traffic timing is the most useful per-card visual. |
| Badge in own panel | **Suppress** the **bifrost** source badge inside the bifrost expanded panel rows (parallel to `suppressQdrantBadge` / `suppressIndexerBadge`). Keep the **upstream** badge as today. |

### Code references

- Go (today): [`internal/supervisor/bifrost.go`](../../internal/supervisor/bifrost.go), [`internal/chat/chat.go`](../../internal/chat/chat.go) (`chat.bifrost.*` slogs), [`cmd/claudia/serve.go`](../../cmd/claudia/serve.go) (`logStore.Writer("bifrost")`).
- Go (new): `internal/servicelogs/bifrostline/` (`NormalizePayload`, `NewWriter`) — pattern from `internal/servicelogs/qdrantline/`.
- JS: [`internal/server/embedui/logs.js`](../../internal/server/embedui/logs.js) (search `isBifrost`, `bifrostCardMetrics`, `bifrostCollapsedCardSubtitle`).
- JS derive: [`internal/server/embedui/logs/derive/bifrostMetrics.js`](../../internal/server/embedui/logs/derive/bifrostMetrics.js) (extend with subprocess counters).

## Reference samples (local dev)

Capture during P1 to repo-root **`temp/`** (gitignored), mirroring qdrant fixtures:

| Artifact | Path | Purpose |
|----------|------|---------|
| Cold-start bifrost log | `temp/bifrost-startup.log` | Banner, version, providers loaded, listening port, MCP/governance startup |
| Mixed traffic | `temp/bifrost-mixed.log` | Request/response cycles, at least one 200, one 429, one 5xx |
| Operator-prefix prototype | `temp/prefix_bifrost_operator_lines.py` | Executable reference for classification (model after `prefix_qdrant_operator_lines.py`) |

```bash
python temp/prefix_bifrost_operator_lines.py temp/bifrost-startup.log temp/bifrost-startup.operator-prefixed.log
```

---

## Canonical `msg` taxonomy

Stable machine slug pattern: **`bifrost.<segment>.<segment>…`** (subprocess-origin) and **`chat.bifrost.<segment>`** (gateway-origin relay). Use **dots** between segments. Detection uses the JSON envelope BiFrost emits with `-log-style json` (top-level `level`, `time`, `msg`, plus subsystem fields).

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

**Add** four counter boxes:

| Box | Behavior |
|-----|----------|
| **Relay success / fail** | From `chat.bifrost.response` (HTTP 2xx vs 4xx/5xx) + `chat.bifrost.error` (always fail). |
| **Tokens (out → usage)** | Sum `outgoingTokens` (`chat.bifrost.request`) → sum `usageTotalTokens` (`chat.bifrost.response`). Show "— → —" until non-zero. |
| **Rate-limit / fallback** | Count `bifrost.rate_limit` and `chat.routing.fallback`. Side-by-side counts; subtitle text "n×429 · m×fallback". |
| **Providers (up / loaded)** | From `bifrost.provider.loaded` + `bifrost.provider.health.ok/fail`. Tint **error** when any provider is `down`. |

### Full event log (expanded)

- **Suppress** the **bifrost** source badge on each row in this panel only (mirror `suppressQdrantBadge`). Implementation: pass `{ suppressBifrostBadge: true }` from `renderExpandedService` for `name === "bifrost"`.
- **Keep** the **upstream** chip for `chat.bifrost.*` rows (already added by `badgeForServicePanel`).
- Each row uses the existing `buildDetailsColumn` so structured fields (model, status, tokens, retry-after) remain visible in expand.

---

## Tooling

| Tool | Location | Role |
|------|----------|------|
| Operator-prefix prototype (planned) | `temp/prefix_bifrost_operator_lines.py` | Executable reference for classification (model after `prefix_qdrant_operator_lines.py`) |

---

## Phased implementation

### P1 — Spec

**Goal.** This doc + a frozen `bifrost.*` list, validated against captured fixtures.

**Deliverables**

- This file checked in.
- `temp/bifrost-startup.log`, `temp/bifrost-mixed.log` captured from a live `claudia serve` cold start + a few chat round-trips (sanitize keys).
- Confirm taxonomy table covers ≥95% of lines in those fixtures; list outliers under **Open questions**.

**Acceptance.** Reviewer can map every line in the two fixtures to a `msg` slug or to `bifrost.unparsed`.

**Status:** `todo`

### P2 — Parse & `msg`

**Goal.** Every BiFrost-derived row exposed to the UI carries a `msg`.

**Deliverables**

- `internal/servicelogs/bifrostline/normalize.go` (+ `_test.go`): `NormalizePayload(raw string) []byte` returns gateway-style JSON with `service:"bifrost"`, `_claudia_norm:1`, and the `bifrost.*` slug. Mirrors `qdrantline.NormalizePayload`.
- `internal/servicelogs/bifrostline/writer.go`: `NewWriter(downstream io.Writer) io.Writer` line-buffers raw stdout and forwards normalized JSON.
- `cmd/claudia/serve.go`: wrap `logStore.Writer("bifrost")` with `bifrostline.NewWriter(...)`.
- Unit fixtures from P1 included in `internal/servicelogs/bifrostline/testdata/`.

**Acceptance.** A `claudia serve` cold start writes only normalized JSON into the bifrost bucket; `bifrost.unparsed` count is 0 against both fixtures.

**Status:** `todo`

### P3 — Card UI cleanup

**Goal.** BiFrost card matches the KV / counters / subtitle / full-log spec above.

**Deliverables**

- Extend `internal/server/embedui/logs/derive/bifrostMetrics.js` with provider/key/health counters from `bifrost.*` lines (kept in goja-tested derive module to match qdrant pattern).
- Update `internal/server/embedui/logs.js` `renderExpandedService` (`isBifrost` branch): replace mini-cards with KV row + four counter boxes.
- Add `suppressBifrostBadge` plumbing to `logSummaryHtml` / `buildDetailsColumn`.
- Refresh `internal/server/logs_components_test.go` and `internal/server/ui_logs_test.go` for new selectors.

**Acceptance.** Operator can read **version / port / providers up/total / auth** without expanding any single log row; counters reset on the next `bifrost.startup.banner`.

**Status:** `todo`

### P4 — Gateway relay alignment

**Goal.** Rename / demote relay slugs so the card draws from one shared vocabulary.

**Deliverables**

- `internal/chat/chat.go`: rename `"upstream chat response"` → `chat.bifrost.response`; demote `virtual model fallback attempt` (Info) to `chat.routing.attempt` at `Debug` when chain length is 1; rename `chat.routing.fallback` retry log to keep the existing slug (already correct); rename `chat blocked by provider limits` and `skipping upstream model (provider limits)` to `chat.provider_limits.blocked` (keep level Info).
- Update derive (`bifrostMetrics.js`) to recognize the new slug names alongside the old ones for one release window.
- Update [`log-presentation-layer.md`](log-presentation-layer.md) §10 changelog.

**Acceptance.** Existing UI metrics (relay req/res/err, tokens, status mix) remain identical numerically after the rename; new `chat.bifrost.response` slug is queryable in `/api/ui/logs`.

**Status:** `todo`

### P5 — Conversation linkage

**Goal.** When a `chat.bifrost.*` line carries `conversation_id`, it is **also** routed into the conversation card's BiFrost chip (same line, two projections — see [`log-presentation-layer.md`](log-presentation-layer.md) §3 / §4).

**Deliverables**

- Coordinated with [`log-conversations.md`](log-conversations.md): conversation card consumes `chat.bifrost.request`, `chat.bifrost.response`, `chat.bifrost.error`, `chat.routing.fallback`, `chat.routing.resolved`, `chat.provider_limits.blocked` to drive the **BiFrost** sub-chip and the conversation timeline.
- Subprocess `bifrost.*` lines stay **bifrost-only** unless they carry `conversation_id` (out of scope until BiFrost itself echoes a header — track in [`log-conversations.md`](log-conversations.md) Open questions).

**Acceptance.** Opening a conversation card shows a **BiFrost** chip whose count matches the relay events scoped to that `conversation_id`; bifrost service card numbers stay independent.

**Status:** `todo`

---

## Open questions

- **Volume:** Provider-health probes can be high frequency; consider a per-provider rollup window (e.g. last status only) for the **providers up/total** KV to avoid flapping.
- **JSON schema drift:** BiFrost upstream may rename fields between releases; pin a `bifrost_version` field at normalization time so older fixtures stay parseable.
- **Key visibility:** Confirm `bifrost.provider.key_loaded` never echoes the secret (only the env var name + provider id). Match `SECURITY.md`.
- **Locale:** English-only operator strings (matches qdrant + indexer).
- **Cross-link UX:** Whether expanded BiFrost rows should jump to the originating conversation card (depends on [`log-conversations.md`](log-conversations.md) P3).

---

## Checklist before marking done

- [ ] Every BiFrost-normalized line carries **`msg`** + structured fields needed for UI (via `bifrostline` on supervised ingest).
- [ ] BiFrost card matches **KV**, **counters**, **subtitle**, **full log** (no bifrost pill in that panel) spec above.
- [ ] Gateway relay slugs renamed / demoted per P4 with derive recognizing both old and new for one release window.
- [ ] Fixture-backed tests from `temp/bifrost-*.log` under `internal/servicelogs/bifrostline/testdata/`.
- [ ] Supervisor doc notes normalization boundary (parallel to qdrant note in [`supervisor.md`](../supervisor.md)).
