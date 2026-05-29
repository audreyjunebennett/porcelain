# Porcelain · Chimera — product requirements

**chimera-gateway** is the thin orchestrator clients talk to: a **single OpenAI-compatible** entrypoint so operators do not juggle manual model switching in the IDE for every turn. Clients authenticate with a **gateway-issued API token** that binds the **tenant** and, when retrieval is enabled, **scopes memory** so embeddings and retrieved context apply only to that tenant’s data.

This document holds **vision and normative product requirements**. It does **not** track implementation status.

| Need | Read |
|------|------|
| **Install, run, configure** | [docs/README.md](README.md) operator runbooks |
| **As-built behavior** (routes, invariants, code map) | [features/README.md](features/README.md) |
| **Delivery history and phase notes** | [plans/README.md](plans/README.md) |
| **Shipped release trains** | [version-v0.1.md](version-v0.1.md) … [version-v0.4.md](version-v0.4.md) |
| **North-star architecture** (future routing depth) | [design.md](design.md) |
| **BiFrost upstream reference** | [reference/bifrost-upstream.md](reference/bifrost-upstream.md) |

---

## System vision

The runnable stack is a set of **Chimera wrapper processes** supervised by **`chimera-supervisor`**: gateway (client API), broker (LLM upstream), optional vector store, optional workspace indexer. The gateway owns **orchestration** — virtual models, routing policy, fallback, RAG retrieval, structured logs — while the broker owns **provider keys and upstream relay**.

See [supervisor.md](supervisor.md), [network.md](network.md), and platform contracts in [features/](features/README.md) for the current layout.

---

## Release roadmap

Future scope only; shipped behavior lives in version docs and feature records.

| Version | Focus | Doc |
|---------|--------|-----|
| **v0.1** | Portable Go gateway, BiFrost upstream, streaming, tokens, health, structured logs | [version-v0.1.md](version-v0.1.md) |
| **v0.1.1** | Tool router, metrics, provider quotas | [version-v0.1.1.md](version-v0.1.1.md) |
| **v0.2** | RAG ingest/retrieval, indexer REST, Qdrant, workspace indexer | [version-v0.2.md](version-v0.2.md) |
| **v0.3** | Branding, onboarding, operator virtual models, SQLite operator store, desktop shell | [version-v0.3.md](version-v0.3.md) |
| **v0.4** | Ensemble (“heavy thinking”), triggers, escalation, paste-back | [version-v0.4.md](version-v0.4.md) |
| **v0.5** | Gateway MCP (optional); conversation archive ingestion | — |
| **v0.7** | TLS, trust stores, `/health` hardening, rate limits, audit/redaction | — |
| **v0.8** | Queues and priority scheduling under load | — |

Engineering task breakdown: [plans/README.md](plans/README.md).

---

## Requirements

### Portability and deployment footprint

1. **Go implementation** — Ship the gateway and Chimera wrappers as **Go** binaries (single artifact per platform).
2. **Upstream over HTTP** — Gateway talks to the broker and embed endpoints over **HTTP** with OpenAI-compatible shapes — no required in-process coupling to upstream.
3. **Supervised stack** — **`chimera-supervisor`** may start broker, vector store, gateway, and indexer as managed children so one operator command brings up a working local stack.
4. **Optional containers** — Docker / Compose are **optional**, not the default day-to-day contract.

---

### Compatibility and interoperability

1. **SSE / streaming** — Chat streaming MUST match **OpenAI-compatible** behavior expected by Continue on day one.
2. **`GET /v1/models` catalog** — Gateway **merges** upstream models with **operator-defined virtual models**; explicit upstream ids **proxy** unchanged when clients address concrete providers directly.
3. **Continue samples** — Directory `vscode-continue/`: `apiBase`, `apiKey`, model selection, RAG headers (`X-Chimera-Project`, `X-Chimera-Flavor-Id`, conversation id when used).

---

### Gateway turn orchestration

1. **Virtual models** — Operators define one or more **virtual model ids** with per-model routing stacks (fallback, policy rules, tool router, RAG). Clients send a virtual model id for orchestrated turns; the gateway applies that stack. As-built: [operator-virtual-models](features/operator-virtual-models.md), [gateway-chat-routing-pipeline](features/gateway-chat-routing-pipeline.md).
2. **Sequential fallback chain** — On upstream failure, **429**, or admission block, walk the configured **ordered** upstream model list (**fail-fast** until queue milestone — *Resilience · 2*).

---

### Security and TLS

1. **Gateway API tokens** — Valid tokens and tenant bindings load from **YAML** (path via config/env); gateway **reloads** on file **mtime** change.
2. **TLS and trust** — Document TLS termination, optional mTLS, corporate CA trust, and a **threat model** for exposed deployments.
3. **Health and attack surface** — `/health` may stay unauthenticated in trusted setups; security milestone adds hardening when exposed.
4. **Abuse resistance** — Rate limiting, request-size limits, audit logging with **redaction**, documented secrets hygiene.
5. **Roadmap security posture** — Early tiers assume functional tenancy on plain HTTP inside trusted LANs; encryption and audit policies ship with the security milestone.

---

### Observability, logging, and health

1. **Structured logging** — Standard levels; INFO for HTTP connections and key parameters (**redact** secrets); DEBUG for routing, RAG, config reload, upstream relay summaries. As-built: [structured-operator-log-lines](features/structured-operator-log-lines.md).
2. **Indexer live storage API** — Authenticated REST `GET` endpoints for **live** vector-store state (health, stats, optional inventory) — no gateway-persisted time-series in baseline designs.
3. **`GET /health`** — No API token; probe upstream; when RAG enabled, probe vector store; **503** + per-check detail on failure; configurable URLs, no retries, ~5s default timeout.

---

### Chat turn resilience and degradation

1. **Per-key tracking** — Track which key/backend was used and exposure to RPM/TPM-style limits where headers exist.
2. **Graceful degradation** — **Fail-over** within the configured model chain; **fail fast** otherwise — no gateway queue until the queue milestone.

---

### Client-facing naming and API shape

1. **Product naming** — Layered names (Porcelain suite, Chimera binaries, Locus desktop) per [product-naming-contract](features/product-naming-contract.md).
2. **Single stable URL** — One base URL for clients; no manual per-request model switching in the UI.
3. **OpenAI-compatible chat surface** — Chat/completions shapes expected by common IDEs and agents.
4. **Orchestrated vs explicit model choice** — Virtual model id for orchestrated turns; explicit upstream id for direct proxy.

---

### Responsibility split (upstream vs gateway)

1. **Broker / upstream** — Provider keys, retries, streaming, OpenAI-shaped requests to backends; **parallel completions** when the gateway orchestrates ensembles.
2. **Gateway** — Apply routing policy and fallback for virtual models; RAG retrieve/inject when enabled; ensemble phase orchestration and escalation when implemented.
3. **Delivery layering** — Ship a working gateway + upstream path before expanding orchestration depth; do not rebuild a full custom LLM proxy unless forced.

---

### Deployment and networking

1. **Documented operator path** — Document **`chimera-supervisor`**, **`make`**, and wrapper binaries in operator runbooks.
2. **RAG stack** — When RAG is enabled, vector store and embedding path MUST be up; when disabled, gateway omits vector probe from health.
3. **Networks** — Supervised/local deployments use loopback or LAN; cross-operator peer access uses host-reachable addresses and published ports.
4. **Published ports** — Bind client-facing gateway port; document health entrypoint and trust assumptions.
5. **Single-document bootstrap** — One obvious getting-started path in README/Makefile.
6. **Developer iteration** — Gateway alone against an already-running broker is acceptable during development.

---

### Operator documentation and samples

`docs/` MUST ship an operator-facing bundle:

1. High-level overview and doc index ([README.md](README.md)).
2. Network architecture ([network.md](network.md)).
3. Installation, setup, startup ([installation.md](installation.md)).
4. Operations commands (`make`, logs, rebuild).
5. Configuration reference ([configuration.md](configuration.md)).
6. Structured logging expectations.
7. VS Code Continue samples (`vscode-continue/`).

As-built contracts and UI behavior: [features/README.md](features/README.md).

---

### Workspace indexing and retrieval

1. **Gateway-owned ingest and retrieval** — Gateway is the HTTP entrypoint for ingest and query-time retrieval; embeddings via configured embed surface; indexers call `GET /v1/indexer/config`, then `POST /v1/ingest`. Direct vector-store writes remain allowed when operators keep tenant/project/flavor ids consistent.
2. **`GET /v1/indexer/config`** — Bearer token; effective chunking, embedding model, ingest paths, headers, collection naming, gateway version.
3. **`POST /v1/ingest`** — One document per request; same auth as chat.
4. **Ingest chunking defaults** — Default 512 UTF-8 code units, 128 overlap — configurable and surfaced via indexer config.
5. **RAG prompt assembly** — Delimited retrieved section before the user turn (e.g. `### Retrieved context` with numbered chunks).
6. **Vector index defaults** — Cosine (or dot if normalized); vector size matches embedding dimension; sensible HNSW defaults.
7. **Collection name encoding** — Lowercase, slug-safe, deterministic hash suffix on collision.
8. **`X-Chimera-Project`** — On chat and ingest when RAG applies; token default when omitted.
9. **`X-Chimera-Flavor-Id`** — Optional corpus selector within tenant+project.
10. **Vector collections** — One collection per `(tenant_id, project_id, flavor_id)`.
11. **Retrieval defaults** — Default top_k = 8; configurable similarity floor; optional recency boost off by default.
12. **Vector payload** — Minimum: `tenant_id`, `project_id`, `text`, `source`; optional `created_at`, `flavor_id`.
13. **RAG quality controls** — Similarity floor, optional recency, flavor/project boundaries.
14. **Conversation archive ingestion** — Automated folder pipeline calling ingest API (future milestone).

As-built: [gateway-rag-ingest-and-retrieval](features/gateway-rag-ingest-and-retrieval.md), [indexer feature cluster](features/indexer.md).

---

### Tenant authentication and project scope

1. **Gateway API token** — Clients authenticate with gateway-issued token; tokens authorize retrieval, ingest, and indexer endpoints by **tenant** when RAG applies.
2. **Project scope on the wire** — Resolve `project_id` from `X-Chimera-Project` or token default; `X-Chimera-Flavor-Id` selects corpus.
3. **Per-workspace Continue config** — Workspace-local `.continue/config.yaml` from samples.
4. **Ingestion parity** — Indexers use indexer config + ingest + live storage APIs; direct vector writes allowed with consistent ids.

---

### Gateway runtime

1. **Long-lived service** — Gateway runs continuously while Chimera is in use.
2. **Per-turn dispatch** — Every user message is evaluated anew for routing (RAG when enabled; ensemble triggers when implemented).

---

### Provider keys and model fallback

1. **Local / multi-machine models** — Backends reachable from the broker; policy may prefer by health, capacity, or task type; peers as remote runners.
2. **Groq / Gemini** — Multiple API keys as separate accounts where rotation increases quota.
3. **Key rotation** — Distribute across keys; react to 429 / rate-limit signals.

---

### Peer topology (cross-host upstreams)

1. **Independent stacks** — Each gateway is the sole client-facing entrypoint on its machine; separate tokens and vector partitions.
2. **Peer upstream, not peer gateway** — OpenAI-compatible `base_url` to **their broker** on a routable network; do **not** chain gateway → peer gateway by default.
3. **Operator clarity** — Document IDE-facing gateway vs peer `base_url`, firewall/VPN, TLS with security milestone.

---

### Model selection and routing policy

1. **Best model for the request** — Policy-driven selection (task class, latency vs quality, context length, cost heuristic).
2. **Uncertainty default** — When ambiguous, prefer a safe, capable model.
3. **Deterministic routing** — Realized in gateway Go without an extra LLM per turn; routing policy YAML with mtime reload; ordered rules; `ambiguous_default_model` fallback.
4. **Cloud vs local policy** — Prefer cloud for generic volume; local (often selective RAG) for private continuity, heavy ensemble, quota exhaustion.

---

### Routing mechanics

1. **Rules and heuristics first** — For virtual models, combine heuristics with fallback chain — not an LLM every turn. Explicit ids → direct proxy.
2. **Optional routing judge** — Later: small fast model may assist on ambiguous turns.

---

### MCP boundaries

1. **MCP is not the model router** — Optional tools/resources in Continue until gateway-native MCP (roadmap).
2. **No mid-inference model switching via MCP** — Gateway chooses model(s) before generation.
3. **Avoid LLM-as-MCP-tool for primary routing**.
4. **Deterministic routing preference** — Routing-critical behavior is gateway-controlled.

---

### Extension hooks

1. **Enrichment hooks** — Extension points for long-context detection and delegating tool loops to MCP servers.

---

### Ensemble orchestration (future — v0.4)

1. **Two-phase ensemble** — N parallel drafts, then critique/synthesize → one answer; default N = 3; cap by available backends.
2. **Ensemble triggers** — Automatic + manual `//deep` (trimmed); virtual-model-only; gateway may strip `//deep` upstream.
3. **Ensemble integration** — Orchestration in gateway; upstream executes parallel calls.

Detail: [version-v0.4.md](version-v0.4.md).

---

### External human escalation (future — v0.4)

When internal routing cannot satisfy policy, the gateway may use **human-in-the-loop** copy/paste to an external UI — not an API integration to that vendor.

1. **Configurable external surfaces** — Name + URL entries in configuration.
2. **Privacy disclosure** — Escalation responses disclose that task or context may leave the operator stack.
3. **When policy engages** — Exhausted internal attempts and low confidence (configurable thresholds).
4. **Escalation message contents** — Summarize failure; URLs; single copy-paste prompt; paste-back delimiter instructions.
5. **Recognizing paste-back** — Delimiter in later user message → treat as external answer, merge, continue.
6. **Continuing without paste-back** — No delimiter → normal chat; do not block waiting for paste unless UX adds it.
