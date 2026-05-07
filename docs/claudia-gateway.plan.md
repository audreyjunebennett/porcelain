# Claudia Gateway

**Claudia Gateway** is the thin orchestrator clients talk to: a **single OpenAI-compatible** entrypoint (for example Continue and similar tools) so operators do not juggle manual model switching in the IDE for every turn. Clients authenticate with a **gateway-issued API token** that binds the **tenant** and, when retrieval is enabled, **scopes memory** so embeddings and retrieved context apply only to that tenant’s data.

## System components

The runnable system is **not** only the gateway binary: several **processes** and **external APIs** interact.

| Component | Role | Depends on (typical) | Consumed by | Interface (typical) |
|-----------|------|----------------------|-------------|---------------------|
| **Gateway** | Routing + **virtual `Claudia-<semver>`** + **fallback chain** + **structured logs**; **RAG**, vector store, **indexer REST** when enabled; **ensembles** + escalation when implemented | **BiFrost** (HTTP: chat; **embeddings** for RAG); **vector store** when RAG on; **gateway token YAML** | **Continue**, indexers, HTTP clients | `GET /v1/models`, `/v1/chat/completions`, `GET /health`; when RAG: `POST /v1/ingest`, `GET /v1/indexer/config`, storage endpoints, RAG headers |
| **BiFrost** | Multi-provider OpenAI-compatible proxy | Provider accounts & keys; optional **peer upstream** | Claudia Gateway | HTTP OpenAI-compatible `/v1` (chat + embeddings as configured) |
| **Cloud providers** (Groq, Gemini, …) | Cloud inference | Vendor accounts & API keys | BiFrost | HTTPS |
| **Local LLM servers** | On-box or LAN inference | GPU/CPU, model files | BiFrost | OpenAI-compatible or provider-specific |
| **Qdrant** | Vector persistence for gateway-mediated RAG | Persistent storage / ops backup policy | **Claudia Gateway**; direct writes allowed anytime | HTTP REST / gRPC (per backend) |
| **Indexer** | Workspace **watch/index**: configured roots, ignore rules, hashing; **`POST /v1/ingest`** or **chunked ingest session** when files exceed gateway limits; gateway **chunks, embeds, writes vectors**—no local embeddings | **Claudia Gateway** (`GET /v1/indexer/config`, ingest + optional storage/corpus paths); **`CLAUDIA_GATEWAY_TOKEN`**; YAML config layers; optional **`claudia serve`/desktop** supervision (`indexer.supervised.*`) | **Operators** (**standalone** **`claudia-index`** or supervised **child**); serves **gateway** RAG corpus **via** ingest | HTTP **client** → gateway: `GET /v1/indexer/config`, `POST /v1/ingest`, `/v1/ingest/session` (+ chunk/complete templates); optional `GET /v1/indexer/storage/health`, `GET /v1/indexer/storage/stats`, `GET /v1/indexer/corpus/inventory` |
| **Peer upstream** | Remote model capacity on another host | Host-published URL; **credential** from peer operator | Your gateway/BiFrost routing config | **HTTP(S)** to routable host + issued secret (*Security · 2*, TLS) |
| **MCP servers** | Tools, resources | Per server | **Continue** Agent mode—**not** the gateway until gateway MCP lands | MCP transports—**not** the primary model-routing path |

## Release roadmap

| Version | Scope | Release notes / plan |
|---------|--------|----------------------|
| **v0.1** | **Portable Go gateway** in front of **BiFrost** (OpenAI-compatible upstream): virtual `Claudia-<semver>` model on `GET /v1/models`, **routing + fallback chain**, **SSE streaming**, gateway tokens (**YAML**, **mtime** reload), `GET /health` probing the configured **upstream**, **structured logging**, operator **`docs/`** (overview, network/install, configuration reference), **`vscode-continue/`** samples. **Single-operator focus.** Optional **desktop** shell and **`claudia serve`** supervision of BiFrost/Qdrant for local ergonomics. **No** gateway-mediated RAG APIs yet. **MCP** only via the IDE until gateway-native MCP lands later. | [version-v0.1.md](version-v0.1.md) |
| **v0.1.1** | **Tool payload shaping** (tool router), **persistent gateway metrics**, **per-provider quotas** and related admin UX. | [version-v0.1.1.md](version-v0.1.1.md) |
| **v0.2** | **RAG**: `POST /v1/ingest`, indexer REST (`GET /v1/indexer/config`, storage **health** / **stats**, corpus inventory as implemented), chunking defaults (**512** / **128** overlap), **Qdrant** (or equivalent adapter), **query-time retrieval** and prompt assembly, collection rules, **project** / **flavor** headers, `GET /health` includes vector-store probe when RAG is enabled. | [version-v0.2.md](version-v0.2.md), [releases-v0.2.x.md](releases-v0.2.x.md) |
| **v0.3** | **Peer backends**: call **another operator’s OpenAI-compatible upstream** (BiFrost or compatible proxy) over a **host-routable** URL with credentials they issue; configuration and docs for **independent stacks**, **published ports**, anti-patterns (do not chain gateway→gateway as the default), and **per-key / usage observability** where upstream headers allow. | [version-v0.3.md](version-v0.3.md) |
| **v0.4** | **Ensemble** (“heavy thinking”): parallel drafts, triggers (`//deep`), critique/synthesize, and **streaming error** semantics; human escalation signals and paste-back/session handling (*External human escalation · 1–6* below; timing tied to ensemble roadmap). | — |
| **v0.5** | **Gateway MCP** (or unified tool surface): optional; scope TBD. **Conversation archive ingestion** (*Workspace indexing and retrieval · 14*): automated / folder-based pipeline calling `POST /v1/ingest` (**requires** indexer/RAG baseline above). | — |
| **v0.7** | **Security & TLS** (*Security and TLS · 2–5*): encryption in transit for gateway-facing and optional inter-service paths, trust-store / CA story, **`/health` hardening** when exposed, **rate limiting** and abuse controls, **audit logging** and **redaction**, documented **threat model** vs trusted-plain-HTTP defaults used earlier in trusted LAN setups. | — |
| **v0.8** | **Queues** and priority scheduling for degraded / busy backends (*Resilience and degradation · 2*)—**fail-over / fail-fast** only prior to this. | — |

## Workstream docs

Task breakdown and delivery notes—not a parallel source of truth. The **Spans** column maps each plan to requirement **sections · items**.

| Doc | Role | Spans |
|-----|------|-------|
| [plans/upstream-llm-bifrost.md](plans/upstream-llm-bifrost.md) | Upstream LLM via BiFrost (speed, portability, fewer moving parts) | *Portability · 1–4*; *Compatibility · 1–2*; *Gateway turn orchestration · 1–2*; *Client-facing naming · 1–4*; *Deployment · 1–6*; *Provider keys · 1–4* |
| [plans/indexer.md](plans/indexer.md) | Indexer product milestones and HTTP contracts | *Tenant authentication · 1–4*; *Workspace indexing · 1–14*; *Observability · 2* |
| [plans/indexer-scan-and-fanout-jobs.md](plans/indexer-scan-and-fanout-jobs.md) | Indexer scan and fan-out jobs | *Tenant authentication · 4*; *Workspace indexing · 1–2*; *Observability · 2* |
| [plans/desktop-ui.md](plans/desktop-ui.md) | Desktop shell and operator-facing UI | *Portability · 3–4*; *Deployment · 1–6*; *Operator docs · 1–7*; *Observability · 1*; *Compatibility · 3* |
| [plans/operator-cli.md](plans/operator-cli.md) | Operator CLI surface | *Operator docs · 4–5*; *Workspace indexing · 1–3* (ingest/indexer contracts operators invoke) |
| [plans/makefile.md](plans/makefile.md) | Makefile targets and build ergonomics | *Deployment · 5*; *Operator docs · 4–5* |
| [plans/log-presentation-layer.md](plans/log-presentation-layer.md) | Log presentation and correlation | *Resilience · 1*; *Operator docs · 6*; *Observability · 1* |
| [plans/log-view-refactor.md](plans/log-view-refactor.md) | Log view refactor | *Operator docs · 6*; *Observability · 1* |
| [plans/log-view-indexer.md](plans/log-view-indexer.md) | Log view and indexer integration | *Operator docs · 6*; *Observability · 1–2* |
| [plans/logs-ui-maintainability.md](plans/logs-ui-maintainability.md) | Logs UI maintainability | *Operator docs · 6*; *Observability · 1* |

## Requirements

### Portability and deployment footprint

Deliberate choices so operators can run Claudia **without** mandatory containers or heavy infrastructure.

1. **Go implementation** — The gateway is shipped as a **Go** binary (single artifact per platform).

2. **Upstream over HTTP** — The gateway talks to **BiFrost** (and optional embed endpoints) over **HTTP** using OpenAI-compatible request/response shapes—**no** required in-process coupling to the upstream proxy.

3. **Supervised stack** — **`claudia serve`** **may** supervise **BiFrost** and **Qdrant** as **child processes** so one operator command can bring up a working local stack.

4. **Optional containers** — **Docker / Compose** are **optional** (org previews, lab stacks)—not the **default** product contract for how operators run the gateway day to day.

---

### Compatibility and interoperability

Fit with **today’s IDE and OpenAI-shaped tooling** so Continue-like clients work without custom transports.

1. **SSE / streaming** — **SSE / streaming** for chat MUST match **OpenAI-compatible** behavior expected by Continue on **day one** (non-streaming-only gateways are not sufficient).

2. **`GET /v1/models` catalog** — Gateway **merges** upstream `GET /v1/models` and exposes a **single** OpenAI-compatible model list. **Explicit** upstream model ids **proxy** through unchanged so clients can address concrete providers directly when needed.

3. **Continue samples** — Directory `vscode-continue/`: `README.md` and example config showing `apiBase`, `apiKey`, model selection from `GET /v1/models`, and custom headers when RAG applies (`X-Claudia-Project`, `X-Claudia-Flavor-Id`).

---

### Gateway turn orchestration

How the gateway **owns each chat turn**: virtual orchestrated model id, routing chain, and coupling retrieval to that path.

1. **Virtual `Claudia-<semver>` model** — Gateway **prepends** a **virtual** entry on `GET /v1/models`: `id` = `Claudia-` + **gateway semantic version** (must match what clients send as `model`). That path receives **routing policy**, **fallback chain**, and **RAG** when enabled; it is the **product** orchestration surface—not merely a catalog alias.

2. **Sequential fallback chain** — For `model: Claudia-<gateway_semver>`, use an **ordered list** of upstream model ids from **gateway configuration** (operator-maintained). On failure or **429**, try the **next** entry (**fail-fast**—*Resilience · 2*).

---

### Security and TLS

Authentication storage, transport security, exposure policy, and abuse posture.

1. **Gateway API tokens** — Valid tokens and tenant bindings load from a **static YAML file** (path via config/env). The gateway **caches** the parsed document and **reloads** when the file’s **modification time** changes. (Authorization **semantics** for RAG and tenants: *Tenant authentication · 1*.)

2. **TLS and trust** — **TLS** termination for client→gateway (and optional upstream/vector paths), optional **mTLS**, **corporate/custom CA** trust—documented with a **threat model**.

3. **Health and attack surface** — **`/health`** may stay **unauthenticated** in trusted setups; functional behavior (*Observability · 3*). Security roadmap milestone adds hardening: bind internally, gate with auth/network policy, or reduce sensitive JSON when exposed.

4. **Abuse resistance and secrets hygiene** — **Rate limiting**, request-size limits, **audit** logging with **redaction**, documented **secrets** practices.

5. **Roadmap security posture** — Prior tiers assume **functional** tenancy on **plain HTTP** inside trusted LANs. Encryption in transit, trust stores, **`/health`** lockdown, and audit policies are fully specified with the security roadmap milestone (*Security · 2–4*). Operators may front the gateway with their own **TLS** terminator or VPN anytime.

---

### Observability, logging, and health

What operators and indexers rely on to **trust** that the system is alive, understandable, and observable.

1. **Structured logging** — Gateway uses **standard log levels** (`error`, `warn`, `info`, `debug`, …). **INFO**: **all** inbound and outbound **HTTP** connections (client route, upstream targets, **status codes**, duration); **key request/response parameters** (**redact** secrets and full bodies). **DEBUG**: routing branches; **RAG** (retrieve, ingest, collection id); config/token/policy reads; **configuration reload**; **upstream** relay (model id, stream vs non-stream, error summaries).

2. **Indexer live storage API** — Authenticated **REST** `GET` endpoints so the **file indexer** (and operators) read **live** vector-store state—**no** gateway-persisted history or time-series in baseline designs (responses **on-demand**). **Minimum**: `GET /v1/indexer/storage/health`; `GET /v1/indexer/storage/stats` — **live** per-collection **point counts**, **vector dimension**, safe backend metrics (document fields). Optional `GET` sub-resources under `/v1/indexer/…`; document paths in `docs/`; keep stable within a **minor** release. **Beyond** `GET /v1/indexer/config`, implement these surfaces so indexers can reconcile corpus state (*Tenant authentication · 4*).

3. **`GET /health`** — **No API token**. Probe configured **upstream**; when **RAG is enabled**, also probe **vector store**; when **RAG disabled**, **omit** vector probe. Failed included check → **503**, **`degraded`: true**, **per-check** detail; all pass → **200**. **Configurable** URLs; **expect 200**; **no retries**; **default ~5s** timeout. Ties to exposure rules (*Security · 3*).

---

### Chat turn resilience and degradation

1. **Per-key tracking** — Track which key/backend was used and exposure to RPM/TPM-style limits where headers exist (**Release roadmap** ties depth).

2. **Graceful degradation** — **Fail-over** within the **configured model chain** where applicable, otherwise **fail fast**—**no** gateway queue until the roadmap row for queues. Same rule when **peer** upstreams exist (*Gateway turn orchestration · 2*).

---

### Client-facing naming and API shape

1. **Product name** — Use **Claudia Gateway** as the canonical name for the entrypoint in docs, config, and operator runbooks (orchestrator / router are fine as synonyms where helpful).

2. **Single stable URL** — One base URL for clients (e.g. Continue); no manual per-request model switching in the UI.

3. **OpenAI-compatible chat surface** — Chat/completions (and related) shapes expected by common IDEs and agents; streaming details *Compatibility · 1*.

4. **Orchestrated vs explicit model choice** — Clients choose the **virtual** `Claudia-<semver>` id for gateway-orchestrated turns (*Gateway turn orchestration · 1*) or an **explicit** upstream id for direct proxy (*Compatibility · 2*).

---

### Responsibility split (upstream vs gateway)

1. **BiFrost / upstream responsibilities** — Provider keys, retries, streaming, OpenAI-shaped requests to configured backends, and **parallel completions** when the gateway orchestrates **ensembles**.

2. **Gateway responsibilities** — When `model: Claudia-<semver>`, apply **routing policy** and **configured fallback chain** (*Gateway turn orchestration*). When **RAG** is enabled, decide **if and what** to retrieve. For **ensembles**, decide **when** to run phases, **critique/synthesize**, escalation, paste-back merge.

3. **Delivery layering** — Ship a **working gateway + upstream** path before expanding orchestration depth; do **not** rebuild a full custom LLM proxy unless a hard requirement forces it. **`GET /health`** reflects configured probes (*Observability · 3*).

---

### Deployment and networking

1. **Documented operator path** — Document **`claudia`**, **`claudia serve`**, and **Makefile** flows (*Portability · 3–4*).

2. **RAG stack** — When **RAG** is **enabled**, vector store and **embedding** path MUST be **up** and **reachable**. When **disabled**, the gateway does **not** require the vector store for correctness; `GET /health` omits that probe (*Observability · 3*). Docs MUST list addresses and supervised children.

3. **Networks** — **Supervised** or **local** deployments use **loopback or LAN** addresses appropriate to the host. **Cross-operator** peer access uses **host-reachable** addresses and **published** ports (*Peer topology · 3*)—not internal supervision URLs meant only for another machine’s localhost.

4. **Published ports & health entrypoint** — Publish or bind ports for the **IDE** (gateway front door). **`GET /health`** behavior (*Observability · 3*); until security milestone, **trusted** reachability unless operators add TLS/policy (*Security · 3*).

5. **Single-document bootstrap** — Prefer **one** obvious getting-started path in README/Makefile so `make …` / `claudia serve` brings up the documented stack.

6. **Developer iteration** — Acceptable to run **gateway** alone against an already-running **BiFrost** during development; production-like checks follow documented full paths.

---

### Operator documentation and samples

`docs/` MUST ship the following **operator-facing** bundle (aligns with repo layout and Makefile flows):

1. **High-level overview** — Gateway purpose, **BiFrost** role, vector store role, roadmap milestones.

2. **Network architecture** — Logical topology: clients, gateway, upstream, vector store; localhost vs LAN vs peer URLs.

3. **Installation, setup, startup** — Prerequisites, config files, first **`GET /health`**, tokens, provider keys.

4. **Operations commands** — **`make`**, **`claudia`**, logs, rebuild, **Ops** helpers.

5. **Configuration reference** — Every runtime configuration source (tokens, routing, env mapping, reload semantics—including *Security · 1*).

6. **Structured logging** — Operators MUST be able to rely on leveled logs per *Observability · 1*.

7. **VS Code Continue samples** — Ship `vscode-continue/` per *Compatibility · 3*.

---

### Workspace indexing and retrieval

Indexing **files** into the vector store and **injecting** retrieved context into chat—contract for ingest, indexer config, chunking, collections, and prompt shaping.

1. **Gateway-owned ingest and retrieval** — Gateway is the **HTTP entrypoint** for **ingest** and **query-time retrieval**; **vector-store adapter** (Qdrant today); **embeddings** via configured embed HTTP surface (dimension must match config). Indexers call `GET /v1/indexer/config`, then `POST /v1/ingest`. **Direct vector-store writes** remain **allowed anytime** (operator ensures **tenant_id** / **project_id** / **flavor_id** consistency).

2. **`GET /v1/indexer/config`** — `Authorization: Bearer <gateway token>` (**same** as chat). JSON includes **effective** `chunk_size`, `chunk_overlap`, `embedding_model`, `ingest_method` + `ingest_path`, required/optional headers (`X-Claudia-Project`, `X-Claudia-Flavor-Id`), minimum payload fields, collection naming summary, `gateway_version`, and **running** indexer-relevant knobs.

3. **`POST /v1/ingest`** — **One document per request** (multipart `file` and/or JSON with `text`, `source`, etc.—document exact schema). `Authorization: Bearer <gateway token>` — **same** as `/v1/chat/completions`.

4. **Ingest chunking defaults** — Default **512** UTF-8 code units per chunk, **128** overlap—**configurable** (surfaced via `GET /v1/indexer/config`).

5. **RAG prompt assembly** — Inject retrieved chunks as a **single delimited section** before the model sees the user turn—e.g. markdown `### Retrieved context` with **numbered** chunks and a **blank line** before the rest of the conversation. **Gateway** orchestrates retrieval/injection.

6. **Vector index defaults** — **Cosine** (or **dot** if embeddings normalized—document with embed model). **Vector size** MUST match embedding dimension (operator-set). Backend **default HNSW** (or documented equivalent) unless profiling says otherwise.

7. **Collection name encoding** — **Lowercase**; **spaces → hyphens**; **collapse** repeats; strip illegal characters (**alphanumeric**, `-`, `_`); **deterministic hash suffix** if triples collide after normalization.

8. **`X-Claudia-Project`** — On chat/completions (when **RAG** applies) and on **ingestion**; falls back to token default (*Tenant authentication · 2*).

9. **`X-Claudia-Flavor-Id`** — Optional corpus selector within tenant+project—see *Workspace indexing · 10*.

10. **Vector collections** — **One** collection per `(tenant_id, project_id, flavor_id)`; names follow *Workspace indexing · 7*.

11. **Retrieval defaults** — Default **top_k = 8**; similarity floor **configurable**; optional `created_at` recency boost (**off** by default unless enabled).

12. **Vector payload** — Minimum fields: `tenant_id`, `project_id`, `text`, `source`, optional `created_at`, optional `flavor_id`; additional keys allowed as implementations evolve.

13. **RAG quality controls** — **Similarity floor**, optional **recency**, **`flavor_id` + project** boundaries; optional system vs irrelevant callbacks.

14. **Conversation archive ingestion** — Automated pipeline from a **configured folder** of exports: **one file per request**, correct **tenant** / **project** / `flavor_id`. Depends on ingest API (**Release roadmap**).

---

### Tenant authentication and project scope

Continue (and similar clients) do not know your **project** unless you pass it on the HTTP request. **One vector backend**, **one collection per `(tenant_id, project_id, flavor_id)`** (*Workspace indexing · 10*). Without RAG, project/flavor headers are **not** required for chat.

1. **Gateway API token — authentication and RAG tenancy** — Clients MUST authenticate with a gateway-issued **API token**. Token **storage and reload**: *Security · 1*. Tokens **authorize** retrieval, ingest, indexer endpoints, and archive pipelines by **tenant** when RAG applies; **default `project_id`** / `flavor_id` when headers omitted.

2. **Project scope on the wire** — Resolve `project_id` from `X-Claudia-Project` or token default; allowlists for unknown projects. `X-Claudia-Flavor-Id` selects corpus (*Workspace indexing · 8–10*).

3. **Per-workspace Continue config** — Use **Continue’s OpenAI-compatible** fields per [Continue’s config reference](https://docs.continue.dev/reference). **Workspace-local** `.continue/config.yaml`; copy from *Compatibility · 3*.

4. **Ingestion parity** — Indexers SHOULD call `GET /v1/indexer/config`, then `POST /v1/ingest`, and use *Observability · 2* for live corpus checks. **Direct vector-store writes** allowed when operators keep ids consistent with headers.

---

### Gateway runtime

1. **Long-lived service** — The gateway runs continuously while Claudia is in use; no process restart between ordinary user messages.

2. **Per-turn dispatch** — **Every** user message is evaluated anew for routing (**RAG** when enabled; ensemble triggers per *Ensemble orchestration · 1–3* when implemented).

---

### Provider keys and model fallback

1. **Local / multi-machine models** — Configure backends reachable from **your** BiFrost instance; gateway policy may prefer by health, capacity, or task type. **Peers**: *Peer topology · 1–3*.

2. **Groq** — Multiple API keys as **separate org/account** where rotation increases quota; document shared buckets within one account.

3. **Gemini** — Same multi-key / multi-account pattern where applicable.

4. **Key rotation** — Distribute across keys (e.g. round-robin); react to 429 / rate-limit signals.

---

### Peer topology (cross-host upstreams)

Each operator runs **their own** gateway stack on **that machine**. **IDEs** use **localhost** (or that host’s listen address) with **that person’s** token. Extra model capacity uses **routable** URLs—not another machine’s internal supervision DNS.

1. **Independent stacks, independent tenants** — Each **Claudia Gateway** is the sole **client-facing** entrypoint on **their** machine; separate tokens, vector partitions, policy.

2. **Peer upstream, not peer Gateway** — Use an OpenAI-compatible **`base_url`** to **their BiFrost** (or compatible proxy) reachable **from your** network (host/LAN/VPN—not their loopback or Compose-only DNS). Authenticate with credentials **they** issue (typically `Authorization: Bearer …`). **Do not** route **Gateway → peer Gateway**; that chains two orchestrators and is **not** the default integration.

3. **Operator clarity for cross-host** — Document **IDE-facing gateway** vs **peer `base_url`**, firewall/VPN expectations, and **TLS/mTLS** with the security roadmap (*Security · 2–5*).

---

### Model selection and routing policy

1. **Best model for the request** — Policy-driven selection (task class, latency vs quality, context length, cost heuristic).

2. **Uncertainty default** — When **ambiguous**, prefer a **safe, capable** model.

**Deterministic routing implementation (items 1–2)** — Realized in **gateway Go** without an extra LLM per turn (*Routing mechanics · 2* optional later). **Routing policy** YAML, **mtime** reload. **Rules** in **order**; each supplies **ordered upstream model ids**. Align with *Gateway turn orchestration · 2*. **`ambiguous_default_model`** when no rule matches.

3. **Cloud vs local policy** — Prefer **cloud** for generic volume; **local** (often **selective RAG**) for private continuity, heavy ensemble, quota exhaustion. **Peers** as **remote-runner** entries.

---

### Routing mechanics

1. **Rules and heuristics first** — For `Claudia-<semver>`, combine heuristics with **fallback chain**—not an LLM every turn. **Explicit** ids → **direct proxy**.

2. **Optional routing judge** — Later: small fast model may assist on ambiguous turns.

---

### MCP boundaries

1. **MCP is not the model router** — **Optional** tools/resources; configure in **Continue** until gateway-native MCP (**Release roadmap**).

2. **No mid-inference model switching via MCP** — Gateway chooses model(s) **before** generation.

3. **Avoid LLM-as-MCP-tool for primary routing** — Not the main backend-selection path.

4. **Deterministic routing preference** — Routing-critical behavior is **gateway-controlled**.

---

### Extension hooks

1. **Enrichment hooks** — Extension points for long-context detection and delegating tool loops to **MCP servers**.

---

### Ensemble orchestration

**Critique/synthesize** and **streaming** behavior when ensemble phases fail are fully specified only with the **ensemble roadmap** milestone (see **Release roadmap · v0.4**)—not before.

1. **Two-phase ensemble** — **N** parallel drafts, then **critique/synthesize** → one answer; **default `N` = 3**; cap by **available** backends; **availability** from **upstream catalog introspection**.

2. **Ensemble triggers** — **Automatic** + manual `//deep` (trimmed); only for **virtual `Claudia-<semver>`**; gateway **may** strip `//deep` upstream. `N` follows *Ensemble orchestration · 1*.

3. **Ensemble integration** — Orchestration lives in **gateway**; upstream executes parallel calls.

---

### External human escalation

When internal routing cannot satisfy policy, the gateway may use **human-in-the-loop** copy/paste to an external UI—not an API integration to that vendor.

Full productization aligns with the ensemble milestone for signals (*External human escalation · 3*), paste-back **session/state**, and polish. Items **1–6** are the **design contract**.

1. **Configurable external surfaces** — One or more **name** + **URL** entries in configuration.

2. **Privacy disclosure** — Escalation responses MUST disclose that **task or context** may leave the operator stack.

3. **When policy engages** — Only when **exhausted** internal attempts **and** **low confidence** (thresholds configurable); concrete signals with ensemble milestone.

4. **Escalation message contents** — Summarize failure; point to configured URLs; **single copy-paste prompt**; instructions for **paste-back delimiter**.

5. **Recognizing paste-back** — Later user message contains delimiter → treat as **external answer**, merge, continue.

6. **Continuing without paste-back** — No delimiter → assume **normal chat**; do **not** block waiting for paste unless UX adds it.
