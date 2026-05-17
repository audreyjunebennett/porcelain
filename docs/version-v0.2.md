# Version 0.2 — RAG baseline


| Field                          | Value                                                                                                                            |
| ------------------------------ | -------------------------------------------------------------------------------------------------------------------------------- |
| **Doc kind**                   | `version-roadmap`                                                                                                                |
| **Owners / areas**             | Gateway RAG, indexer, vector storage, operator logs UI (webview / desktop), chat token estimates, usage metrics, provider limits |
| **Status**                     | `shipped`                                                                                                                        |
| **Targets**                    | Gateway v0.2 RAG baseline                                                                                                        |
| **Last updated**               | See git history                                                                                                                  |
| **Supersedes / superseded by** | Patch train **v0.2.0–v0.2.2** documented in § [Shipped releases](#shipped-releases-v020-through-v022)                            |


## At a glance

**v0.2** gives chat **memory of your code**: operators index workspace folders through `**chimera-indexer`** and gateway `**POST /v1/ingest**`; at chat time the virtual model **retrieves** from **Qdrant** and injects context into the prompt. **Tenant isolation** is enforced by the bearer token; **project** and **flavor** headers pick the corpus.

The **workspace indexer** pairs watch-mode ingest with a **queue-safe** scan and fan-out model so large trees and **multiple roots** stay fair and observable—see § **File indexer** (**Themes — indexer runtime and queue**).

**Operator observability** is a first-class concern: `**/ui/logs`** offers **conversation** vs **subsystem** lenses on the same stream, **correlation IDs** and stable `**msg`** tagging, **summarized** cards (including **Indexers**) with **SSE + poll** delivery, and **raw / structured** fallbacks—see § **Operator logs UI** (**Themes — logs and operator observability**). From **v0.2.2**, the **desktop** shell foregrounds this experience (`[gui-testing.md](gui-testing.md)`).

**Patch-level** configuration and routes (**v0.2.0** → **v0.2.2**) are spelled out in § **[Shipped releases](#shipped-releases-v020-through-v022)**.

The **chat path** records **tiktoken-compatible `cl100k_base`** estimates on the **proxied JSON body**, aggregates **request counts and estimated tokens** into **per-minute and per-day** windows in **SQLite**, and enforces **vendor-style caps** from `**provider-model-limits.yaml`** before calling upstream—see § **Token counting (chat path)** and § **Usage metrics and provider limits**.

**Also in v0.2:** **chunked ingest sessions** for oversized files; **custom headers** (including `**X-Chimera-Conversation-Id`**) documented for **Continue** in `**vscode-continue/`**; **chat robustness** (fallback, quotas, tool-router) carried forward from v0.1.x; **Make-driven catalog tools** that refresh **free-tier / limits snapshots** and `**provider-free-tier`** YAML—see § **Additional operator themes** below.


| Theme                                                                                  | Outcome                                                                                                                                       | Status |
| -------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------- | ------ |
| [Ingestion API](#ingestion-api)                                                        | `POST /v1/ingest` accepts whole files; the gateway chunks, embeds, and stores                                                                 | `done` |
| [Indexer REST](#indexer-rest-gateway-owned)                                            | Config, storage health and stats, paginated corpus inventory                                                                                  | `done` |
| [Chunking, embeddings, Qdrant](#chunking-embedding-and-qdrant)                         | One collection per (tenant, project, flavor); stable payload fields                                                                           | `done` |
| [Retrieval & prompt assembly](#retrieval-and-prompt-assembly)                          | Top-k with score floor; numbered context block before the user turn                                                                           | `done` |
| [Token counting (chat path)](#token-counting-chat-path)                                | `cl100k_base` estimate on outbound chat JSON; structured log (`outgoingTokens`); same estimate drives metrics + quota admission               | `done` |
| [Usage metrics and provider limits](#usage-metrics-and-provider-limits)                | SQLite minute/day rollups; RPM/RPD/TPM/TPD from YAML; **429** `gateway_provider_limits` when a call would exceed                              | `done` |
| [Health probe](#health-and-operations)                                                 | `/health` adds a Qdrant probe when RAG is enabled                                                                                             | `done` |
| [Workspace indexer (`chimera-indexer` v0.2)](#file-indexer-v02)                          | Watch roots, ignore rules, queue-safe scan/fan-out, ingest aligned with v0.2 APIs                                                             | `done` |
| [Operator logs UI](#operator-logs-ui-correlation--summarized-views)                    | Correlation + tagging; conversation / subsystem views; Indexers cards; SSE/poll; desktop shell — **Themes** subsections                       | `done` |
| [Indexer chunked ingestion](#themes-indexer-chunked-ingestion)                         | Session API when files exceed `**max_whole_file_bytes`**; indexer uses whole POST or chunked transport per config                             | `done` |
| [Conversation headers & Continue](#themes-conversation-headers-and-continue-templates) | `**X-Chimera-Conversation-Id**` (+ project/flavor); gateway accepts or generates; templates in `**vscode-continue/**`                         | `done` |
| [Chat robustness](#themes-chat-robustness)                                             | Virtual-model **429/5xx** fallback chain; **413** skip-to-next on virtual path; **tool-router** slimming; **gateway_provider_limits** **429** | `done` |
| [Catalog & limits tooling (Make)](#themes-catalog-and-limits-tooling-make)             | `**catalog-free`**, `**catalog-available**`, `**config-provider-free-tier**`; snapshots inform `**provider-model-limits.yaml**` maintenance   | `done` |
| [Qdrant log classification](#theme--qdrant-log-classification)                         | Normalize supervised Qdrant JSON into stable `qdrant.*` slugs and operator-friendly card summaries                                            | `done` |
| [Gateway log classification](#theme--gateway-log-classification)                       | Make gateway logs uniformly slugged (`msg`) and operator-meaningful for `/ui/logs`                                                            | `done` |
| [Conversation log classification](#theme--conversation-log-classification)             | Conversation cards show lifecycle state and fan-out from gateway/BiFrost/Qdrant via correlation                                               | `done` |
| [BiFrost log classification](#theme--bifrost-log-classification)                       | Normalize supervised BiFrost JSON into stable `bifrost.*` slugs and richer BiFrost service card                                               | `done` |


---

**Status:** The capabilities below are **shipped** in the **v0.2.0** baseline and subsequent patches (**v0.2.1** logging/UI/conversation merge, **v0.2.2** supervised indexer + shell). Per-patch operator detail lives in § **[Shipped releases: v0.2.0 through v0.2.2](#shipped-releases-v020-through-v022)** below.

This document pulls together **everything scoped to product v0.2** from `[porcelain.plan.md](porcelain.plan.md)` (authoritative product roadmap), `[overview.md](overview.md)`, `[network.md](network.md)`, `[configuration.md](configuration.md)`, and cross-links the **file indexer** work in a **separate** plan: `[plans/indexer.md](plans/indexer.md)`.

**Tone:** normative items below track **locked** product decisions in the gateway plan; where the **in-tree** stack differs from the original LiteLLM + TypeScript + Compose description, treat this document as the **capability target** and align the Go gateway + BiFrost implementation to the same **HTTP contracts** and **behavior**. Cross-reference topical requirements in `[porcelain.plan.md](porcelain.plan.md)` using *Section · item* notation (e.g. *Workspace indexing · 10*).

**Companion:** v0.1 working notes and checklist live in `[version-v0.1.md](version-v0.1.md)`.

---

## What v0.2 is

**v0.2** is the **RAG baseline**: gateway-mediated **ingestion**, **query-time retrieval**, **Qdrant** (or another backend behind the **vector-store adapter**), **tenant-scoped** access to ingested data, and **indexer-facing REST** so an external `chimera-indexer` (and operators) can drive indexing without embedding locally.

**Release roadmap summary** (from `[porcelain.plan.md](porcelain.plan.md)`):

- `POST /v1/ingest`
- **Indexer REST:** `GET /v1/indexer/config`, `GET /v1/indexer/storage/health`, `GET /v1/indexer/storage/stats`, `GET /v1/indexer/corpus/inventory` (live Qdrant readings + paginated source/hash inventory; no persisted metric history in-gateway)
- **Chunking defaults:** **512** UTF-8 code units, **128** overlap (configurable; surfaced via indexer config)
- **Qdrant adapter** + **query-time retrieval** + **prompt assembly**
- **Collection** naming rules; `X-Chimera-Project` / `X-Chimera-Flavor-Id` headers
- `GET /health` includes **Qdrant** probe when **RAG is enabled**
- **Chat token estimates** (`cl100k_base`) on the proxied body; **usage rollups** (minute/day) and `**provider-model-limits.yaml`** admission on the chat path (see § **Token counting (chat path)** and § **Usage metrics and provider limits**)

---

## Gateway and stack (v0.2)

### Authentication, tenant, and headers

- **Bearer token** (same as chat) defines **tenant**; **from v0.2** the token **authorizes RAG** so retrieval and ingested memory are **only** for that tenant’s data (gateway plan *Tenant authentication · 1*).
- `X-Chimera-Project: <slug>` on chat (when RAG applies) and on **ingestion**; falls back to token default (*Tenant authentication · 2*).
- Optional `X-Chimera-Flavor-Id: <key>` (or token default `flavor_id`) selects the **corpus** within tenant + project.
- Optional `**X-Chimera-Conversation-Id`** on chat — stable **conversation / thread** id for logs and `**/ui/logs`** (**Conversations** view); if omitted, the gateway **generates** one. Successful responses **echo** the id when the client should persist it. Operators wire these through IDE templates — see `**vscode-continue/`** (`[vscode-continue/README.md](../vscode-continue/README.md)`) and § **Themes: conversation headers and Continue templates** below.

### Virtual model and RAG

- `GET /v1/models`: same virtual `chimera-<semver>` id pattern as v0.1; **v0.2+** the virtual model **adds RAG when enabled** (explicit upstream model ids still **direct proxy**).

### Ingestion API

- `POST /v1/ingest` — **one document per request** (multipart `file` and/or JSON with `text`, `source`, etc.); finalize and document the **exact schema** in `docs/` and implementation.
- **Chunked ingest session** — For payloads larger than `**rag.ingest.max_whole_file_bytes`** (surfaced via `**GET /v1/indexer/config**` as `**max_whole_file_bytes**`), `**chimera-indexer**` uses the gateway `**/v1/ingest/session**` flow (start, chunk upload, complete) instead of a single whole-body POST. See `[indexer.md](indexer.md)` and § **Themes: indexer chunked ingestion** below.
- Accept **client-supplied `content_hash`** (algorithm and field name per contract) for **inventory / change detection**; gateway stores it as specified in `[plans/indexer.md](plans/indexer.md)` (indexer v0.2–v0.3 uses client hash as local truth until server-authoritative hash lands in later milestones).

### Indexer REST (gateway-owned)

- `GET /v1/indexer/config` — effective `chunk_size`, `chunk_overlap`, `embedding_model`, `ingest_method` + `ingest_path`, required/optional headers (`X-Chimera-Project`, `X-Chimera-Flavor-Id`), minimum Qdrant payload fields, collection naming summary, `gateway_version`, and related knobs from **running** config.
- `GET /v1/indexer/storage/health` — vector store reachability; **degraded**/ok; scoped to token **tenant**.
- `GET /v1/indexer/storage/stats` — **live** per-collection **point counts**, **vector dimension**, safe Qdrant metrics (document response fields).
- Optional additional `GET` under `/v1/indexer/…` as needed; document paths and keep stable within a **minor** release.

**Corpus inventory:** `[plans/indexer.md](plans/indexer.md)` `GET /v1/indexer/corpus/inventory` is implemented (paginated `source` + `content_sha256` + optional `client_content_hash`) for indexer startup reconciliation; see `[indexer.md](indexer.md)`.

### Chunking, embedding, and Qdrant

- **Chunking** happens **server-side** after ingest; defaults **512** / **128** overlap; configurable.
- **Embeddings** via the configured embed path (product plan: LiteLLM `/v1/embeddings`; in-tree: equivalent via BiFrost/embed configuration).
- **Qdrant defaults:** cosine (or dot if normalized — document with embed model); vector size **must** match embedding model dimension; default HNSW unless profiling says otherwise.
- **One collection** per `(tenant_id, project_id, flavor_id)`; **collection name encoding:** lowercase, spaces → hyphens, collapse repeats, strip illegal characters, deterministic short hash suffix on collision.
- **Qdrant payload (minimum):** `tenant_id`, `project_id`, `text`, `source`, optional `created_at`, optional `flavor_id`.

### Token counting (chat path)

**Status:** `**done`** — implemented in `**internal/tokencount**` (tiktoken-compatible `**cl100k_base**`) and wired through `**internal/chat**` for `**POST /v1/chat/completions**`.

- **What gets counted:** After the gateway builds the **outbound** JSON body (client fields plus resolved `**model`** and `**stream**`), the **entire marshalled string** is passed through `**tokencount.Count`** — the same estimate feeds **structured logs**, **SQLite usage metrics**, and **provider limit** admission (`[docs/tokencount-talk.md](tokencount-talk.md)` discusses calibration vs upstream tokenizers).
- **Logging:** Successful counts appear on the upstream relay line (e.g. structured field `**outgoingTokens`**, `**msg**` `chat.bifrost.request`). Count failures **do not** fail the request; they degrade to logging without the numeric field.
- **CLI:** Operators can run `**chimera tokencount`** for ad-hoc counts (`cl100k_base`, optional `**o200k_base**` display for comparison) — not required for gateway operation.
- **Embedding / RAG paths:** Pre-embed counting for ingest chunks and retrieval query strings remains aligned with the same `**tokencount`** package where those code paths need estimates; product caveats about **approximate** counts vs non–cl100k upstream models still apply.

### Usage metrics and provider limits

**Status:** `**done`** — persisted metrics and YAML caps work together on the chat path via `**internal/gatewaymetrics**`, `**internal/providerlimits**`, and `**config/provider-model-limits.yaml**` (see also `[version-v0.1.1.md](version-v0.1.1.md)` § gateway metrics for historical baseline).

- **Recording:** Each upstream `**/v1/chat/completions`** attempt records **model id**, **HTTP status**, and the **same estimated token count** used for logging (`internal/chat`). Aggregates are stored in **SQLite** under the configured metrics path (typically `**data/gateway/`** — see `[configuration.md](configuration.md)`).
- **Rollups:** Tables maintain **per-minute** (UTC minute bucket) and **per-calendar-day** windows so the gateway can answer “how many **calls** and **estimated tokens** for this model in the current minute / current usage day?” Day boundaries honor `**usage_day_timezone`** from the limits file (defaults and per-provider overrides — e.g. vendor-local midnight for Gemini-style reporting).
- `**provider-model-limits.yaml`:** `**schema_version: 1`** defines optional `**rpm**`, `**rpd**`, `**tpm**`, `**tpd**` per provider/model (`null` / omitted means **no cap** on that dimension). Defaults cascade **provider → model**; invalid values are rejected at load.
- **Admission:** Before issuing the upstream HTTP request, `**providerlimits.Guard`** compares current minute/day usage **plus this request’s estimate** against the resolved limits. If the call would exceed a configured cap, the gateway returns **HTTP 429** with error type `**gateway_provider_limits`** (body explains quota would be exceeded). Metrics lookup failures **fail open** (request allowed, warning logged) so a broken store does not brick chat.
- **Operator surfaces:** `**/ui/metrics`** and `**GET /api/ui/metrics**` expose rollups and recent events when metrics are enabled; logs UI may consume the same snapshot (`[plans/log-view-refactor.md](plans/log-view-refactor.md)`). Example limits live beside runtime config as `**config/provider-model-limits.example.yaml**`.

### Retrieval and prompt assembly

- **Query-time retrieval** for the virtual model when RAG is enabled.
- **Defaults:** **top_k = 8**; drop chunks below **~0.72** cosine similarity (configurable); optional `created_at` recency boost (default off unless config enables).
- **Prompt assembly:** inject retrieved chunks as a **single delimited section** before the user turn (e.g. markdown `### Retrieved context` with **numbered** chunks and a blank line before the rest of the conversation).

### Health and operations

- `GET /health`: when **RAG is enabled**, also probe **Qdrant** (e.g. `GET http://qdrant:6333/` in Compose); if RAG disabled, **omit** Qdrant. Failure → **503**, `**degraded`: true**, per-check detail (*Observability · 3*).
- **Structured logging (v0.1 baseline, v0.2 extension):** **DEBUG** (and appropriate levels) should cover **RAG** path activity — retrieve, ingest, collection id (**v0.2+**).

### Operator logs UI (correlation & summarized views)

**Status:** `**done`** — shipped across **v0.2.1** and **v0.2.2**; concrete bullets for those patches sit in § **[Shipped releases](#shipped-releases-v020-through-v022)** · [v0.2.1](#v021--logging-correlation-logs-ui-optional-conversation-merge) and [v0.2.2](#v022--desktop-shell-supervised-indexer-indexer--continue-operator-ui).

**Intent:** Treat logs as a **presentation layer**: structured lines stay **verbatim** in the gateway’s in-memory buffer (and any other capture paths), while `**/ui/logs`** **interprets, groups, threads, and summarizes** them so operators see **what happened** without scanning opaque JSON. Design rationale and vocabulary live in `**[plans/log-presentation-layer.md](plans/log-presentation-layer.md)`** (Phases **A–D** shipped; **Phase E** — optional server-side event store for cross-restart history — remains `**todo`**).

#### Themes — logs and operator observability

- **Two lenses on one stream** — **Conversations** follow **who** (principal / token) and **what happened in a chat thread**; **Subsystems** follow **gateway**, **BiFrost**, **Qdrant**, and **indexer** health and activity. Same underlying log lines, different narratives (`[plans/log-presentation-layer.md](plans/log-presentation-layer.md)` §3–4).
- **Explicit correlation and tagging** — Stable `**request_id`**, `**conversation_id**`, `**index_run_id**`, `**principal_id**`, `**service**`, and `**msg**` families so the UI can filter, thread, and roll up without guessing (`[plans/log-presentation-layer.md](plans/log-presentation-layer.md)` §5).
- **Summarized by default, lossless on demand** — Headlines and cards first; expand for full structured fields or switch to **structured grid / raw** modes so debugging never depends on summaries alone (`[plans/log-view-refactor.md](plans/log-view-refactor.md)`).
- **Live delivery and resilience** — `**/api/ui/logs`** plus **SSE** (`/api/ui/logs/stream`) with **poll fallback**, backfill, and filters so the page stays usable under transport hiccups (`[plans/log-view-refactor.md](plans/log-view-refactor.md)`).
- **Operator ergonomics and deep links** — URL and embedded-shell parameters (`**view`**, `**principal**`, `**conversation**`, `**seq**`, `**embed**`) support sharing and desktop/webview integration (`[plans/log-view-refactor.md](plans/log-view-refactor.md)`).
- **Presentation without a durable log DB (in v0.2)** — Interpretation happens in the client over the ring buffer; **cross-restart search / long retention** (presentation plan **Phase E**) is **not** part of the shipped v0.2 contract.

**Indexer ↔ logs** themes (cards, identity fields, stats polling) live under § **File indexer** · **Themes — indexer and log presentation** below. Desktop webview behavior — `**[gui-testing.md](gui-testing.md)`**.

**Related planning / maintenance docs** (implementation detail, not normative product contract): `[plans/log-view-refactor.md](plans/log-view-refactor.md)`, `[plans/logs-ui-maintainability.md](plans/logs-ui-maintainability.md)`.

**Optional:** `**conversation_merge`** in `gateway.yaml` (requires gateway metrics / SQLite) merges chat turns when no conversation id is sent — documented in `**[configuration.md](configuration.md)**` and § [v0.2.1](#v021--logging-correlation-logs-ui-optional-conversation-merge) below.

### Operator documentation (delta for v0.2)

The gateway plan requires `docs/` to cover overview, network, install, Docker cookbook, and configuration reference **for v0.1**; **v0.2** adds:

- Data flow **IDE → gateway → embed path → Qdrant** (and **indexer → gateway** for ingest).
- **Ingest** and **indexer** API paths, auth, and headers.
- **Operator Logs** (`/ui/logs`): summarized vs detailed views, correlation fields, themes under § **Operator logs UI** (**Themes — logs and operator observability**), and `**[plans/log-presentation-layer.md](plans/log-presentation-layer.md)`**; indexer card themes under § **File indexer**.
- **Continue** (or client) samples: `**X-Chimera-Project`**, `**X-Chimera-Flavor-Id**`, and `**X-Chimera-Conversation-Id**` — see `**vscode-continue/**` and § **Themes: conversation headers and Continue templates**; gateway plan continues convention for RAG headers.

`[network.md](network.md)` already notes **v0.2+**: Chimera → Qdrant for retrieval and indexer-backed workflows. `[configuration.md](configuration.md)` notes `tenant_id` in logs and **v0.2+** RAG scoping by tenant — keep these aligned as behavior lands.

---

## Additional operator themes

Normative API detail for RAG and indexer remains in **Gateway and stack** and `**[plans/indexer.md](plans/indexer.md)`**. This section groups **cross-cutting** behaviors that operators and IDE configs rely on together.

## Theme — Qdrant log classification

**Goal:** Qdrant subprocess output is operator-readable: every classified line has a stable `msg` slug (`qdrant.*`) and the logs UI can summarize Qdrant state and activity without raw `target` strings.

**Summary (from `[plans/log-qdrant.md](plans/log-qdrant.md)` — At a glance):**
Qdrant subprocess output is becoming **JSON lines** only; the operator log view currently shows raw Rust `target` strings and embedded access fragments. The plan defines a stable `**msg` taxonomy**, operator-facing derived copy, and a UI contract for the Qdrant card plus **collection → indexer card** fan-out.

**Scope**

- Normalize supervised Qdrant JSON lines to gateway-style rows with `service:"qdrant"` and stable `qdrant.*` `msg` slugs.
- Derive operator-facing subtitle/KV/counters for the Qdrant service card from the taxonomy.
- Map Qdrant collection activity into matching indexer cards using the shared collection naming rules.

**Acceptance**

- `/ui/logs` Qdrant card shows stable subtitles/KV/counters derived from `qdrant.*` slugs; expanded Qdrant panel omits the Qdrant badge as specified.
- Indexer cards include relevant Qdrant collection events when the collection resolves to that indexer’s `(tenant, project, flavor)`.

**Status:** `done`

---

## Theme — Gateway log classification

**Goal:** The gateway emits uniformly slugged, correctly-leveled structured logs so the default Info stream tells the operator story and the gateway service card can summarize real gateway state.

**Summary (from `[plans/log-gateway.md](plans/log-gateway.md)` — At a glance):**
Gateway parent-process logs use a stable `gateway.*` taxonomy (plus tightened existing domains), operator-appropriate levels, and structured lifecycle/health objects; the `/ui/logs` gateway card derives KV and counters from those slugs (`internal/server/embedui/logs/derive/gatewayCardModel.js`). See the plan for the full slug table and phase notes.

**Scope**

- Add `msg` slugs to all gateway `slog` calls across lifecycle/config/tokens/routing/chat/RAG/ingest/health/UI session paths.
- Reclassify log levels so Info is operator-relevant and high-volume noise moves to Debug/Trace.
- Extend the gateway service card to show operator KV + counters derived from the new slugs.

**Acceptance**

- Grepping `internal/` + `cmd/chimera/` shows no `slog.Info/Warn/Error/Debug` calls that omit `"msg", "<slug>"` (within the intended audit scope).
- With `LOG_LEVEL=info`, cold start produces a compact operator narrative (config resolved, tokens loaded, listening, upstream/Qdrant/BiFrost supervised status).
- `/ui/logs` Gateway card KV row populates (listening/upstream/config/tokens/routing/supervised children) without expanding individual rows.

**Status:** `done`

---

## Theme — Conversation log classification

**Goal:** Conversation cards show lifecycle state (received → routed → RAG → upstream → delivered) and can include the right supporting lines via correlation fan-out, not just “whatever already had `conversation_id`”.

**Summary (from `[plans/log-conversations.md](plans/log-conversations.md)` — At a glance):**
Conversation cards join gateway, relay, RAG, tool, and inferred subprocess lines via correlation tiers, a `conversation.*` lifecycle, per-turn indexing, and witness events (Phases 1–8 in the plan). Subprocess linkage remains conditional on echoed correlation where the upstream platform allows it.

**Scope**

- Ensure gateway lines touching a chat request carry correlation (`request_id`, `conversation_id`, `principal_id`) consistently.
- Emit `conversation.*` lifecycle slugs at named points in the request flow.
- Update `/ui/logs` conversation view to join related lines via request-id mapping and (optionally) collection+time heuristics for Qdrant.

**Acceptance**

- Conversation cards display a state pill, lifecycle progress, and accurate BiFrost/Qdrant/fallback counts derived from slugs.
- A conversation’s expanded view shows the relevant gateway-emitted lifecycle events even when subprocess lines are missing correlation.

**Status:** `done`

---

## Theme — BiFrost log classification

**Goal:** The supervised BiFrost subprocess log stream is classified into stable `bifrost.*` slugs and the BiFrost service card reflects both gateway relay events and subprocess health/config signals.

**Summary (from `[plans/log-bifrost.md](plans/log-bifrost.md)` — At a glance):**
Supervised BiFrost stdout/stderr is normalized in `internal/servicelogs/bifrostline` to stable `bifrost.*` slugs; the BiFrost service card combines subprocess rows with gateway relay lines (`chat.bifrost.*`, routing, provider limits) per the plan’s UI contract.

**Scope**

- Normalize supervised BiFrost stdout/stderr into gateway-style rows with stable `bifrost.*` `msg` slugs.
- Align gateway relay slugs (`chat.bifrost.*`, routing, provider-limit) so the BiFrost card draws from one consistent vocabulary.
- Update the BiFrost service card to show KV + counters for providers/health/rate limits, not just relay totals.

**Acceptance**

- `/ui/logs` BiFrost card shows version/port/auth/providers up/total and separates rate-limits vs other failures.
- BiFrost subprocess lines are queryable by `bifrost.*` slugs (with `bifrost.unparsed` only for truly unknown lines).

**Status:** `done`

---

### Themes: indexer chunked ingestion

- **Threshold:** Gateway `**rag.ingest.max_whole_file_bytes`** caps **single-request** whole-file ingest; effective ceiling is also exposed on `**GET /v1/indexer/config`** so `**chimera-indexer**` can choose **whole** vs **chunked** transport (`transport`: `**whole`**  `**chunked**` in structured logs — `[indexer.md](indexer.md)`).
- **Flow:** Chunked uploads use the `**/v1/ingest/session`** HTTP surface (session lifecycle + per-chunk writes + completion); correlates with `**index_run_id**` / ingest logging like simple ingest (`[plans/log-presentation-layer.md](plans/log-presentation-layer.md)` activity log).
- **Operator story:** Large workspace files still index without blowing HTTP body limits; tune `**max_whole_file_bytes`** in `**gateway.yaml**` (and optional indexer YAML override per `[indexer.md](indexer.md)`).

### Themes: conversation headers and Continue templates

- `**X-Chimera-Conversation-Id`:** Optional on `**POST /v1/chat/completions`**. When sent, the gateway uses it as the session key for **structured logs**, metrics joins, and `**/ui/logs`** conversation threading; when absent, it **generates** an id. Responses **echo** the header so clients can reuse it on follow-up turns.
- **RAG scope headers:** `**X-Chimera-Project`** and `**X-Chimera-Flavor-Id**` remain required for correct corpus selection when RAG applies (same as gateway plan).
- `**vscode-continue/`:** Example `**config.yml`** and `**[README.md](../vscode-continue/README.md)**` document how to attach **custom headers** for Continue (exact YAML keys vary by Continue version — `requestOptions.headers`, `defaultRequestOptions`, etc.). Operators should add **project**, **flavor**, and **conversation** headers there so IDE traffic matches gateway expectations and logs stay correlated.

### Themes: chat robustness

*v0.2 builds on the v0.1 / v0.1.1 gateway path; this is a **summary** — authoritative detail lives in `[version-v0.1.md](version-v0.1.md)`, `[version-v0.1.1.md](version-v0.1.1.md)`, and `[configuration.md](configuration.md)`.*

- **Virtual model (`chimera-<semver>`):** On upstream **429** or **5xx**, the gateway walks `**routing.fallback_chain`** from the appropriate index and retries the **same** client payload against the next candidate (`[internal/chat](../internal/chat/chat.go)`).
- **Payload too large (413):** On the **virtual-model** path, an upstream **413** records metrics and triggers **fallback** (same request, next model), skipping models that already returned **413** for that request; **direct** upstream model ids return **413** to the client (`[version-v0.1.1.md](version-v0.1.1.md)` § G5).
- **Tool payload size:** When enabled, `**transform.ApplyToolRouter`** may **trim** the `**tools`** array using `**routing.router_models**` / `**routing.tool_router**` before routing (`[version-v0.1.1.md](version-v0.1.1.md)`); per-request `**X-Chimera-Tool-Router: skip**` and `**X-Chimera-Tool-Confidence-Threshold**` remain supported.
- **Provider quotas:** Before upstream HTTP, `**providerlimits.Guard`** may return **429** `**gateway_provider_limits`** when `**provider-model-limits.yaml**` would be exceeded (§ **Usage metrics and provider limits**).

### Themes: catalog and limits tooling (Make)

Operator-maintained `**config/provider-model-limits.yaml`** follows `**schema_version: 1**` (`**rpm**`, `**rpd**`, `**tpm**`, `**tpd**`) — see § **Usage metrics and provider limits** and `[configuration.md](configuration.md)`. The repo ships **Make targets** and CLIs to **refresh vendor-derived snapshots** used when building or auditing that file and `**provider-free-tier`** routing input:


| Target                               | Purpose                                                                                                                                                                                                                                                                                                                                                                                                                                                                                    |
| ------------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `**make catalog-free**`              | Runs `**chimera/cmd/catalog-write-free**`: fetches **Groq** + **Gemini** public docs, extracts **BiFrost-style model ids** and **rate-limit metadata**, writes `**config/catalog-free-tier.snapshot.yaml`** (override with `**OUT=**`). Optional `**INTERSECT=**` filters to a catalog file.                                                                                                                                                                                                       |
| `**make catalog-available**`         | Runs `**chimera/cmd/catalog-write-available**`: `**GET**` BiFrost `**/v1/models**`, writes `**config/catalog-available.snapshot.yaml**` (**requires** BiFrost up; `**OUT=`** override).                                                                                                                                                                                                                                                                                                            |
| `**make config-provider-free-tier**` | Runs `**catalog-available**` then `**catalog-write-free**` with `**INTERSECT=**` the available snapshot; writes `**config/catalog-free-tier.snapshot.yaml**` and `**config/provider-free-tier.generated.yaml**` (override `**FREE_OUT=**`, `**PROVIDER_FT_OUT=**`). Produces `**provider-free-tier**` YAML (`**format_version**`, patterns such as `**ollama/***`, intersected Groq/Gemini ids) for **routing / free-tier filtering** — see `[docs/plans/makefile.md](plans/makefile.md)`. |


**Relating snapshots to `provider-model-limits.yaml`:** The **catalog-free** snapshot carries **per-model limit notes** from vendor pages; operators **merge or reconcile** those values into `**provider-model-limits.yaml`** (committed example: `**config/provider-model-limits.example.yaml**`). There is **no** single Make target that overwrites `**provider-model-limits.yaml`** automatically — by design, so operators review before replacing caps.

**Tests:** `**make test-catalog-free`** and `**make test-catalog-available**` exercise the catalog CLIs.

---

## File indexer (v0.2)

All **indexer** milestones, configuration schema, gateway client behavior, Makefile targets, and **checklists** live in:

`**[plans/indexer.md](plans/indexer.md)`**

**Summary for this release:** the first shippable `chimera-indexer` **aligns with gateway v0.2** — whole-file `POST /v1/ingest`, `GET /v1/indexer/config`, storage **health** (and related APIs), client `content_hash`, env-based token, watch roots + ignore rules, **no symlink follow** by default, debouncing/backpressure, and documented behavior for **oversized files** under whole-file-only ingest until **indexer v0.4** dual-mode exists.

#### Themes — indexer runtime and queue

- **Queue-safe initial indexing** — Initial work follows **scan → sharded fan-out list jobs → per-file ingest**, avoiding flooding a bounded queue with a single walk-and-enqueue-all pass (`[plans/indexer-scan-and-fanout-jobs.md](plans/indexer-scan-and-fanout-jobs.md)`).
- **Fairness across workspaces** — **Tiered priority** (watcher-driven work ahead of bulk backlog) plus **round-robin interleaving** of candidates across `**(project, flavor)`** scopes so multi-root configs do not starve later scopes (`[plans/indexer-scan-and-fanout-jobs.md](plans/indexer-scan-and-fanout-jobs.md)`).
- **Telemetry that matches the model** — Per-scope **discovery** lines (`**indexer.discovery.summary.scope`**), **scan complete**, and **queue snapshots** (including **per-tier depths**) align logs with the queue and fan-out design (`[plans/indexer-scan-and-fanout-jobs.md](plans/indexer-scan-and-fanout-jobs.md)`).
- **UI parity as incremental** — Summarized **card-per-scope** polish and **live “current file / totals”** status are **partial or planned** where `[plans/indexer-scan-and-fanout-jobs.md](plans/indexer-scan-and-fanout-jobs.md)` marks operator UI phases incomplete; v0.2 still ships the **runtime** behavior and structured signals those views consume.

#### Themes — indexer and log presentation

- **Stable indexer identity in telemetry** — After gateway config fetch, structured logs carry `**indexer_key`**, `**tenant_id**`, `**principal_id**`, `**user_label**` so the UI can group and title cards consistently (`[plans/log-view-indexer.md](plans/log-view-indexer.md)`).
- **State plus live vector-store snapshot** — `**indexer.state`** and periodic `**indexer.storage.stats**` (from gateway `**GET /v1/indexer/storage/stats**`, configurable `**storage_stats_poll_ms**`, `**-1**` disables periodic poll) give operators a current picture without per-file noise (`[plans/log-view-indexer.md](plans/log-view-indexer.md)`).
- **“Indexers” as first-class UI** — Summarized **Indexers** section: cards keyed by `**indexer_key`** (fallback `**index_run_id**`), titles reflecting **label · project · flavor**, expanded detail for **watched roots** and **ignore-rule impact** (`[plans/log-view-indexer.md](plans/log-view-indexer.md)`).
- **Readable signal, not just JSON** — Event-mix / queue / jobs rollups and **plain-language** lines interpret indexer traffic for humans; derivation is covered by tests (`[plans/log-view-indexer.md](plans/log-view-indexer.md)`, `[plans/log-view-refactor.md](plans/log-view-refactor.md)`).
- **Advanced topology honesty** — **Multi-ingest-target** YAML can split logical indexers; **P3** items (e.g. separate stats polls per flavor, optional human summary field) remain **deferred**; a **single default-header** stats scope per process is a **known limitation** until those plans land (`[plans/log-view-indexer.md](plans/log-view-indexer.md)` §Limitations / P3).

---

## Identifiers, keys, and picking the right Qdrant collection (v0.2 product plan)

This is the **current** system the gateway plan targets for **v0.2** — the anchor for how requests and index payloads map to storage.


| Concept                      | Where it comes from                                                                          | Role                                                                                                                                                                                                                                                         |
| ---------------------------- | -------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `tenant_id`                  | Gateway-issued **Bearer token** (server-side; not chosen per request by the client for chat) | Scopes **all** RAG data; retrieval and ingest apply only within this tenant.                                                                                                                                                                                 |
| `project_id`                 | `X-Chimera-Project` header on chat (when RAG applies) and on ingest, else **token default**  | Selects the **project** / corpus namespace within the tenant.                                                                                                                                                                                                |
| `flavor_id`                  | Optional `X-Chimera-Flavor-Id`, else **token default**                                       | Selects a **variant** corpus (e.g. branch, profile) within tenant + project.                                                                                                                                                                                 |
| **Qdrant collection**        | Derived **deterministically** by the gateway from `(tenant_id, project_id, flavor_id)`       | **One collection per triple**; naming follows encoding rules in `[porcelain.plan.md](porcelain.plan.md)` (lowercase, slug-safe, collision hash suffix). **No** reliance on payload filters for tenancy at v0.2 — isolation is by **collection**. |
| `**source` (indexed paths)** | Indexer / ingest client                                                                      | **Relative path** under configured roots in `[plans/indexer.md](plans/indexer.md)`; avoids leaking absolute host paths in bodies.                                                                                                                            |


**Operational note:** Operators still configure **how** the gateway reaches Qdrant (URL, API key, adapter). `[porcelain.plan.md](porcelain.plan.md)` defaults to an HTTP health probe (e.g. `6333` in Compose); a local **gRPC** client on `6334` remains compatible with the same **collection naming** and payload contract as long as the adapter uses one consistent Qdrant API mode.

---

## Shipped releases: v0.2.0 through v0.2.2

Operator-oriented summary of what landed in each **patch** on the v0.2 line (configuration knobs, HTTP surfaces, UI routes). Normative behavior across the line remains summarized above in **Gateway and stack** and companion plans.

The virtual model id stays `**Chimera-<gateway.semver>`** (set in `config/gateway.yaml`); example configs may show an older patch until you bump semver locally.

**Deeper references (beyond this section)**

- Indexer product plan: `[plans/indexer.md](plans/indexer.md)`, operator quick start: `[indexer.md](indexer.md)`
- Log UI and correlation: `[plans/log-presentation-layer.md](plans/log-presentation-layer.md)`


| Release                                                                         | Outcome                                                                     | Status |
| ------------------------------------------------------------------------------- | --------------------------------------------------------------------------- | ------ |
| [v0.2.0](#v020--rag-baseline-ingest-indexer-apis-chimera-indexer)                 | RAG baseline: ingest, retrieval, Qdrant, `chimera-indexer`                    | `done` |
| [v0.2.1](#v021--logging-correlation-logs-ui-optional-conversation-merge)        | Per-request correlation, richer logs UI, optional conversation merge        | `done` |
| [v0.2.2](#v022--desktop-shell-supervised-indexer-indexer--continue-operator-ui) | Desktop shell, supervised `chimera-indexer`, indexer and Continue admin pages | `done` |


### v0.2.0 — RAG baseline, ingest, indexer APIs, `chimera-indexer`

**Theme:** Gateway-mediated **retrieval-augmented generation** with **Qdrant**, **ingestion**, **indexer-facing REST**, and the `chimera-indexer` workspace indexer binary.

**Configuration (`config/gateway.yaml`)**

- `rag.enabled` and `rag.*`: Qdrant URL (optional API key), embedding path/model/dimension, chunk size/overlap, retrieval **top_k** and score threshold, ingest size limits (including `max_whole_file_bytes` vs chunked session ingest).

**HTTP API**

- `POST /v1/ingest` — document ingestion (multipart / JSON); server-side chunking, embedding, upsert into Qdrant.
- **Chunked ingest session** for files larger than `rag.ingest.max_whole_file_bytes` (see indexer docs).
- **Indexer REST** (Bearer-scoped): `GET /v1/indexer/config`, `GET /v1/indexer/storage/health`, `GET /v1/indexer/storage/stats`, `GET /v1/indexer/corpus/inventory` (paginated reconciliation).
- `GET /health` — when RAG is enabled, includes a **Qdrant** probe (degraded behavior per implementation).

**Chat**

- Virtual model `chimera-<semver>`: when RAG is enabled, **query-time retrieval** and **prompt assembly** (retrieved context injected ahead of the conversation).
- Tenant scoping via gateway tokens; `X-Chimera-Project` and `X-Chimera-Flavor-Id` select project/corpus (with token defaults).

**Indexer CLI**

- `chimera-indexer` walks configured roots, respects ignore rules, hashes files, and calls `POST /v1/ingest` (or chunked session for large files). Build: `make indexer-build`.

**Stack**

- Continues to use **BiFrost** as the OpenAI-compatible upstream for chat and embeddings (embedding model configured under `rag.embedding`).

### v0.2.1 — Logging correlation, logs UI, optional conversation merge

**Theme:** Request-scoped correlation across access logs, chat/RAG/ingest/indexer, and a richer **Logs** UI; optional **session merge** when the client does not send a conversation id.

**Middleware and access logs**

- `requestid` middleware: stable `request_id` on requests and responses.
- Access / structured logs include `service` and `request_id` where applicable.

**Chat and RAG logging**

- `conversation_id`: from header `X-Chimera-Conversation-Id` or generated by the gateway.
- `principal_id` on chat logs where applicable.
- Stable `msg` slugs on chat / RAG / ingest paths for filtering and dashboards (see log-presentation docs).

**Ingest and indexer**

- Ingest echoes `index_run_id`; indexer client sends the agreed header for correlation.
- Indexer process logs use `indexer.run.*` style messages and attach `index_run_id` to structured stderr when JSON logging is enabled.

**Logs UI (`/ui/logs`)**

- Views: **Detailed**, **Summary**, **Conversations**, **Subsystems**; preferred view stored in `localStorage`.
- `wrapResponse` logging fix so logged **statusCode** matches the handler outcome on early errors.

**Optional conversation merge (`conversation_merge` in `gateway.yaml`)**

- When enabled (requires **gateway metrics** / SQLite migrations): merges chat turns into an existing session using **embedding similarity** and recent-window rules when `X-Chimera-Conversation-Id` is absent. Schema and defaults are documented in `config/gateway.example.yaml` and `[configuration.md](configuration.md)`.

**Documentation added in-tree**

- `[plans/log-presentation-layer.md](plans/log-presentation-layer.md)`.

### v0.2.2 — Desktop shell, supervised indexer, indexer + Continue operator UI

**Theme:** First-run / **main** shell experience, **supervised `chimera-indexer`**, dedicated **Indexer** and **Continue** admin pages, consolidated **observability** (stats surfaced alongside logs), and desktop polish.

**Supervisor**

- When `indexer.supervised.enabled` is set (and RAG or `start_when_rag_disabled` conditions are met), `chimera serve` / desktop can start `chimera-indexer` as a child with `CHIMERA_GATEWAY_URL`, merged `--config`, and optional `--log-json`. See `[indexer.md](indexer.md)` § Supervised mode and `[supervisor.md](supervisor.md)`.

**Operator UI**

- **Main** shell page: clearer status/summary for the local stack.
- `/ui/indexer`: configure supervised indexer YAML (paths/roots), `GET/PUT /api/ui/indexer/config`, append roots (including desktop folder picker where supported).
- `/ui/continue` (or embedded Continue flow): **copy-ready** VS Code **Continue** snippet and guidance aligned with `vscode-continue/`, including hooks for RAG headers and indexer-related setup.
- **Logs** view: improved tracking and presentation (subsystem filters, correlation with v0.2.1 fields).
- Prior **Stats** metrics view consolidated into the logs/observability experience as shipped in this release (use `/ui/metrics` where still exposed for raw metrics JSON).

**Desktop**

- Native **window icon** support on Windows (and stubs on other OSes); embedded assets under `assets/`.

**Indexer runtime**

- Additional structured events (`internal/indexer/ops_events.go`), skip reasons, supervised config wiring, and related tests — see git history for `Version 0.2.2` if you need file-level detail.

### Suggested verification (v0.2.x)


| Release   | Quick check                                                                                                                                                                                                       |
| --------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **0.2.0** | `rag.enabled: true`, `GET /health` shows Qdrant when configured; `POST /v1/ingest` and `GET /v1/indexer/config` succeed with a valid token; virtual-model chat includes retrieved context when collections exist. |
| **0.2.1** | Logs show `request_id` and `conversation_id`; `/ui/logs` view modes persist; optional `conversation_merge` behaves per config when metrics DB is enabled.                                                         |
| **0.2.2** | With `indexer.supervised.enabled`, `chimera-indexer` appears in supervisor logs and `/ui/logs` (source **indexer**); `/ui/indexer` and Continue snippet pages load after login.                                     |


### See also (releases context)

- `[README.md](../README.md)` — install/build entrypoints
- `[network.md](network.md)` — ports and data paths
- `[version-v0.1.1.md](version-v0.1.1.md)` — tool router, metrics SQLite, quota limits (still applicable alongside 0.2.x)

---

## Explicitly not v0.2

Keep these on later roadmap entries (see `[porcelain.plan.md](porcelain.plan.md)` **Release roadmap**):

- **v0.3** — Chimera branding/onboarding, optional **internal embedding** exploration (see `[version-v0.3.md](version-v0.3.md)`), peer LiteLLM, virtual keys, cross-host publishing, per-key observability (*Resilience · 1*), etc.
- **v0.4** — ensembles, escalation, **dual-mode / streaming large-file ingest**, server-authoritative hash in ingest response (indexer plan **v0.4**).
- **v0.5+** — gateway MCP, conversation archive ingestion, etc.
- **v0.7** — TLS, hardening, `/health` lockdown on untrusted networks.
- **v0.8** — queues / priority scheduling (*Resilience · 2*).

**Exploration from v0.1** (e.g. embedded vector store to avoid a dedicated Qdrant process) remains **research**; v0.2 still assumes a **vector-store adapter** boundary so embedded and remote backends can swap under the same interface (`[version-v0.1.md](version-v0.1.md)` §4c and gateway plan).

---

## Implementation checklist (high level)

Use this to track cross-cutting v0.2 work; gate detailed indexer items in `[plans/indexer.md](plans/indexer.md)`.


| Area                       | Action                                                                                                                                                                                                                                                                                                                              |
| -------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **Config**                 | Gateway config to enable/disable RAG, embedding model id, Qdrant (or adapter) connection, chunking knobs, retrieval thresholds, feature flags as needed.                                                                                                                                                                            |
| **HTTP API**               | Implement `POST /v1/ingest`, `GET /v1/indexer/config`, `GET /v1/indexer/storage/health`, `GET /v1/indexer/storage/stats`; document schemas and limits (e.g. max body size for whole-file ingest).                                                                                                                                   |
| **Chat path**              | Virtual model: when RAG enabled, run retrieval + prompt assembly; honor `X-Chimera-Project` / `X-Chimera-Flavor-Id`.                                                                                                                                                                                                                |
| **Token counting**         | Shipped: `**internal/tokencount`** + chat-path wiring; `**outgoingTokens**` on relay logs (§ **Token counting (chat path)**).                                                                                                                                                                                                       |
| **Usage metrics & limits** | SQLite minute/day rollups; `**provider-model-limits.yaml`** + `**providerlimits.Guard**`; **429** `gateway_provider_limits` (§ **Usage metrics and provider limits**).                                                                                                                                                              |
| **Qdrant / adapter**       | Collections per triple; payload fields; collection naming; cosine/dot and dimension checks.                                                                                                                                                                                                                                         |
| **Health**                 | Extend `GET /health` with Qdrant probe when RAG enabled.                                                                                                                                                                                                                                                                            |
| **Logs UI**                | Themes under § **Operator logs UI** and § **File indexer**; correlation IDs; `/ui/logs` modes and APIs (`[plans/log-presentation-layer.md](plans/log-presentation-layer.md)`, `[plans/log-view-refactor.md](plans/log-view-refactor.md)`, `[plans/log-view-indexer.md](plans/log-view-indexer.md)`); desktop shell (`/ui/desktop`). |
| **Docs**                   | Update `docs/overview.md`, `docs/network.md`, `docs/configuration.md`, ingestion/indexer references; `**vscode-continue/`** headers (project, flavor, **conversation**); § **Additional operator themes**.                                                                                                                          |
| **Indexer**                | Follow `[plans/indexer.md](plans/indexer.md)` **Indexer v0.2** checklist and **Gateway coordination**; queue/fairness themes — `[plans/indexer-scan-and-fanout-jobs.md](plans/indexer-scan-and-fanout-jobs.md)`.                                                                                                                    |


---

## Quick reference — related plans


| Document                                                                         | Role                                                                                           |
| -------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------- |
| `[porcelain.plan.md](porcelain.plan.md)`                             | Authoritative product requirements and roadmap                                                 |
| `[plans/indexer.md](plans/indexer.md)`                                           | `chimera-indexer` milestones and gateway coordination                                            |
| `[version-v0.1.md](version-v0.1.md)`                                             | v0.1 delivery notes and exploration                                                            |
| `[overview.md](overview.md)`                                                     | Repo-oriented product summary                                                                  |
| `[network.md](network.md)`                                                       | Ports and v0.2+ Qdrant data path                                                               |
| `[configuration.md](configuration.md)`                                           | Config files and v0.2+ tenant scoping note                                                     |
| `[plans/log-presentation-layer.md](plans/log-presentation-layer.md)`             | Log presentation layer (correlation, view modes; Phase E deferred)                             |
| `[plans/log-view-refactor.md](plans/log-view-refactor.md)`                       | `/ui/logs` modularization, APIs (poll + SSE), view modes and deep-link params                  |
| `[plans/log-view-indexer.md](plans/log-view-indexer.md)`                         | Indexer cards and summarized indexer UX in `/ui/logs`                                          |
| `[plans/indexer-scan-and-fanout-jobs.md](plans/indexer-scan-and-fanout-jobs.md)` | Queue-safe scan/fan-out, fairness, indexer telemetry aligned with logs                         |
| `[plans/makefile.md](plans/makefile.md)`                                         | `**catalog-free`**, `**catalog-available**`, `**config-provider-free-tier**` targets           |
| `[version-v0.1.1.md](version-v0.1.1.md)`                                         | Gateway metrics SQLite, upstream events — baseline for § **Usage metrics and provider limits** |
| `[tokencount-talk.md](tokencount-talk.md)`                                       | Chat-path token estimate semantics vs TPM admission                                            |


Deferred notes on **running embeddings locally** (ONNX alignment, optional **vectordb-cli**-style paths, retrieval-depth ideas) live in `[version-v0.3.md](version-v0.3.md)` under **Internal embedding provider (exploration)** — relevant only if that exploration ships or informs indexer-side experiments; they are **not** part of the locked v0.2 HTTP ingest contract.

---

## Implementation snapshot (token metrics & limits)

The former agent checklist for token counting is **superseded** by the shipped layout below:


| Concern                               | Location                                                                                                                              |
| ------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------- |
| Tokenizer (`cl100k_base`)             | `[internal/tokencount](../internal/tokencount/)`                                                                                      |
| Chat relay, estimates, metrics record | `[internal/chat/chat.go](../internal/chat/chat.go)` (`prepareChatPayload`, `estTokensFromPayload`, `recordUpstreamMetrics`)           |
| SQLite recording / rollups            | `[internal/gatewaymetrics](../internal/gatewaymetrics/)`                                                                              |
| YAML limits + admission               | `[internal/providerlimits](../internal/providerlimits/)`, `[config/provider-model-limits.yaml](../config/provider-model-limits.yaml)` |


Regression coverage includes `**internal/providerlimits/*_test.go`**, `**internal/chat/chat_limits_test.go**`, and tokenizer tests under `**internal/tokencount**`.