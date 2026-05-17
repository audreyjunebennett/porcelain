# Version 0.3 - Chimera branding, RAG scope, onboarding, peer backends


| Field                          | Value                                                |
| ------------------------------ | ---------------------------------------------------- |
| **Doc kind**                   | `version-roadmap`                                    |
| **Owners / areas**             | Gateway desktop, onboarding, peer backends, branding |
| **Status**                     | `active`                                             |
| **Targets**                    | Gateway/desktop v0.3                                 |
| **Last updated**               | See git history                                      |
| **Supersedes / superseded by** | Builds on `[version-v0.2.md](version-v0.2.md)`       |


## At a glance

Make the gateway easier to set up, friendlier to share between operators, and clearer about what it is. This plan’s **sections follow a single narrative**: **rename** (Porcelain · Chimera · Locus), **credential file naming** (`api-keys` / `secret`), **internal embedding** exploration, **workspace embedding scope** (**user + project + flavor**, base corpus + unions), then **first-run token handoff** and the **setup wizard** (including **VS Code Cline** integration instead of Continue-oriented samples), and finally **peer backends** for cross-host upstream routing. Multiple operators can call each other's models.


| Theme                                                                                      | Outcome                                                                                                                                  | Status        |
| ------------------------------------------------------------------------------------------ | ---------------------------------------------------------------------------------------------------------------------------------------- | ------------- |
| [Product naming](#product-naming)                                                          | Layered names in docs, UI, and startup logs with hard-cut naming contracts                                                                | `done`        |
| [Credential file naming](#credential-file-naming)                                          | `api-keys.yaml` / `api_keys` / `secret`; reserve "token" for tokenizer counts                                                            | `done`        |
| [Internal embedding provider (exploration)](#internal-embedding-provider-exploration)      | Optional in-repo or first-install embedding runtime to reduce reliance on Ollama for `/embeddings`                                       | `exploration` |
| [Workspace embedding scope (project + flavor)](#workspace-embedding-scope-project--flavor) | Ingestion keys `(user, project, flavor)`; project-only = base corpus; flavored queries union base + flavor; multi-workspace request pool | `todo`        |
| [First-run token handoff](#first-run-token-handoff)                                        | Show, copy, and optionally save the gateway token; restart-friendly                                                                      | `todo`        |
| [Setup wizard](#setup-wizard)                                                              | Guided keys -> local server -> test chat -> indexing -> integration                                                                      | `todo`        |
| [IDE integration (VS Code Cline)](#ide-integration-vs-code-cline)                          | Replace Continue-focused snippets and examples with **Cline**-oriented samples and wizard copy                                           | `todo`        |
| [Peer backends](#peer-backends)                                                            | Call another operator's OpenAI-compatible upstream (typically BiFrost) with credentials they issue over a host-routable URL              | `todo`        |


---

## What this version is

This document is the **working plan for v0.3** for this repository (**Chimera**: intelligent routing and memory layer; see [Product naming](#product-naming)). Body **sections are ordered** for delivery narrative: [Product naming](#product-naming) and [Credential file naming](#credential-file-naming) first; then [Internal embedding provider (exploration)](#internal-embedding-provider-exploration) and [Workspace embedding scope (project + flavor)](#workspace-embedding-scope-project--flavor); then [First-run token handoff](#first-run-token-handoff) and [Setup wizard](#setup-wizard); then [IDE integration (VS Code Cline)](#ide-integration-vs-code-cline) (operator samples and wizard step 7 target **Cline** instead of **Continue**); then [Peer backends](#peer-backends) from the master product plan (`[porcelain.plan.md](porcelain.plan.md)`). **v0.3** targets **layered product naming** (**Porcelain**, **Chimera**, **Locus**), **api-keys** language, optional **in-repo / first-install** embedding weights **within license**, and **RAG** rules for **project + flavor** unions and **multi-workspace** pools. Naming and README wording in line with branch `origin/feat/chimera-branding` should be folded into this release unless superseded by a written decision.

**Companion docs:** `[porcelain.plan.md](porcelain.plan.md)`, `[configuration.md](configuration.md)`, `[plans/indexer.md](plans/indexer.md)`.

Authoritative **architecture and numbered requirements** remain in `[porcelain.plan.md](porcelain.plan.md)` unless this plan explicitly revises them. **Indexer** milestones labeled “v0.3” in `[plans/indexer.md](plans/indexer.md)` (e.g. scoped overrides, headers) are **indexer product versions**, not necessarily the same shipping train as **gateway desktop v0.3**; cross-link when both touch the same API.

---

## Product naming

**Goal:** Align operator-visible language and implementation logging with the **layered architecture** introduced on `origin/feat/chimera-branding`, while retiring ambiguous “chimera-gateway” wording where it meant “this binary / service.”

**Scope**

### Architecture narrative

These names are **roles**, not four separate shipping binaries unless noted:


| Layer         | Role                                                                                                                                                                                                                                                        |
| ------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **Porcelain** | The **creative system** umbrella—the product story that contains workspace tooling, this gateway, and inference/RAG plumbing.                                                                                                                               |
| **Chimera**   | This repository’s **gateway**: the **intelligent routing and memory layer**—OpenAI-compatible façade in front of **BiFrost**, optional **Qdrant** RAG, indexer REST, routing policy, and admin UI. Bridges workspace-side traffic to inference + retrieval. |
| **Locus**     | **Workspace-side** context: docs describe **Locus clients** authenticating to Chimera (e.g. `Authorization: Bearer …`). Use where copy previously said “client” vs Chimera without naming the workspace tier.                                               |
| **BiFrost**   | Upstream **inference** proxy; Chimera stays in front of BiFrost as today. README-style copy may spell out “BiFrost (inference)” and “Qdrant (vector search)” so roles stay clear.                                                                           |


**Canonical positioning sentence (README-level):** Chimera is **part of Porcelain**; it is not a separate unrelated product. Full-system context may live outside this repo (e.g. **Rebirth** repository `PORCELAIN.md`—update this plan’s pointer if the canonical doc moves).

**Concrete deltas already modeled on `origin/feat/chimera-branding`:**

- **README** title and lede: **“Chimera: Intelligent Routing & Memory Layer”**; first paragraph states membership in **Porcelain** and assigns Chimera (not “the gateway” generically) as the component that owns BiFrost-facing behavior, RAG, and `chimera serve` supervision wording where updated.
- **Config table copy:** **Chimera** substitutes for “Chimera” where it describes **client auth** (`tokens.yaml`), `gateway.yaml` (“Chimera listen + upstream”), `.env` (Chimera↔BiFrost key line), and **desktop** install note (“admin UI for Chimera”).
- `**cmd/chimera/gateway.go`:** structured startup logs use `Chimera (go) listening` (and bootstrap variant) instead of `chimera (go) listening`.

### Scope buckets

- **Operator-facing branding** — Primary headings and overview docs should say **Chimera** for this service and **Porcelain** for the suite; use **Locus** where workspace clients are meant. Avoid presenting “chimera-gateway” as the product name on first-run surfaces unless migration docs require it.
- **Technical identifiers** — Use canonical runtime identifiers for this train: `CHIMERA_*` env names, `X-Chimera-*` headers, and current `chimera`/`chimera-indexer`/`chimera-supervisor`/`locus-desktop` build + package names.
- **HTTP / API ergonomics** — Use only `X-Chimera-*` header contracts for current behavior. Historical `X-Chimera-*` strings are documentation history only.
- **Repository naming** — GitHub org/repo or Go module path changes remain optional for v0.3; if deferred, checklist explicitly “no repo rename this train.”

### Deliverables checklist

- Written **naming decision**: when **Chimera** vs **Porcelain** appears (gateway-only vs suite), **Locus** copy guidelines, and canonical names for env/header/bin/path surfaces.
- UI, installer, about screens, and packaged artifacts match the layered story (**Chimera** gateway inside **Porcelain**).
- Documentation set (README, onboarding, `[porcelain.plan.md](porcelain.plan.md)` release row when updated) reflects the architecture narrative; historical “chimera-gateway” OK in release notes only when labeled historical.
- Companion components (**indexer**, desktop app, Compose samples) updated for any new headers/env aliases called out above.
- Make tasks are updated to use a variable name for the product name
- Product name is defined in a minimum number of locations

**Acceptance**

Treat this theme as satisfied when **first-touch** operator docs and UI consistently present **Chimera** + **Porcelain** + **Locus** as described above, startup logs match the Chimera wording where implemented, and legacy identifiers are treated as historical context only.

**Status:** `done`

**Final cutover note:** v0.3 is hard cut. Legacy naming aliases are retired and unsupported in current runtime behavior.

### Supervised-process shutdown

`origin/feat/chimera-branding` also adjusts `chimera serve` so supervised **Qdrant**, **BiFrost**, and **indexer** children don’t hang after context cancel: wait with a **timeout**, then **kill** if needed, with structured `slog` diagnostics. When merging or reimplementing v0.3 desktop supervision, preserve this behavior so window-close / shutdown reliably tears down children.

---

## Credential file naming

**Goal:** Stop overloading **token** for both **gateway client access** (Bearer / Continue `apiKey`) and **LLM usage** (tokenizer counts, `est_tokens`, context limits). Operators and docs should read **api key / secret** on the auth side and reserve **token** for model-token semantics.

**Scope**

### File names


| Current                                                               | v0.3 target                                                                           |
| --------------------------------------------------------------------- | ------------------------------------------------------------------------------------- |
| `config/tokens.example.yaml`                                          | `config/api-keys.example.yaml`                                                        |
| Operator copy / runtime file `tokens.yaml` (path from `gateway.yaml`) | `api-keys.yaml` (recommended default filename; operators may still use a custom path) |


Comments in the example file should tell operators to copy to `api-keys.yaml` and to reload on mtime, matching today’s behavior.

### YAML shape

- **Top-level key:** `api_keys` — list of gateway-issued **client access** credentials (not LLM tokens).
- **Per-entry credential field:** `secret` — the sensitive string the client sends (e.g. `Authorization: Bearer …`, Continue `apiKey`). **Do not** use the YAML key `token` for this value; that word stays aligned with upstream/model-token usage elsewhere in the stack.
- **Unchanged fields on each row:** `label`, `tenant_id` (same semantics as today).

Illustrative layout:

```yaml
api_keys:
  - label: personal
    secret: "replace-me-gateway-client-secret"
    tenant_id: personal
```

### Gateway config path key

In `gateway.yaml`, the path that points at this file should use `paths.api_keys` (replacing `paths.tokens`) so the operator-facing key matches the document (`api_keys`). Example: `api_keys: "./api-keys.yaml"` under `paths:`.

### Implementation notes

- **Code:** loader package names, struct fields, and log messages should prefer **api key** / **gateway client secret** language where they refer to this file; reserve **token** in logs and metrics for tokenizer / usage paths where applicable.
- **Migration policy:** v0.3 is hard cut. Use only `api_keys` / `secret` and `paths.api_keys` for current behavior.

**Acceptance**

- Example and runtime credential files use `api-keys.yaml`, `api_keys`, and `secret` where implemented.
- `gateway.yaml` uses `paths.api_keys` for current behavior.
- Docs and logs reserve "token" for tokenizer/model-token usage except in explicitly historical notes.

**Status:** `done`

---

## Internal embedding provider (exploration)

**Goal:** Explore loading and running an **embedding model inside the gateway stack** (or a tightly coupled child process) so operators can **reduce reliance on Ollama** (or another external OpenAI-compatible server) **only for embeddings**, while chat and other providers keep their current paths.

**Scope**

### Operator model (config + lifecycle)

- **Start when configured:** Mirror the **indexer** mental model—an **internal embedding** capability is **off by default** and **starts with supervision** (or an explicit enable + health gate) when `gateway.yaml` (or a dedicated stanza) says so, so idle installs do not pay RAM or disk for weights they do not use.
- **Configuration surface:** When enabled, the operator sets:
  - A reserved **internal provider name** (string used wherever embedding “provider” is selected today—wizard, indexer client, metrics labels).
  - The **embedding model id** (and, if needed, **revision** / **quantization** tag) the runtime should load.
- **API contract:** Prefer exposing **OpenAI-compatible `/embeddings`** on **localhost** (or a Unix socket) so existing **indexer → gateway → embed** call paths change minimally compared to pointing at Ollama.

### Technical directions to evaluate

- **Inference backend:** Options might include an embedded **native** runtime (e.g. **ONNX Runtime**, **GGUF** via a maintained **CGO** binding, or another **Go-callable** library) versus a **small dedicated sidecar** that is still “not Ollama” but easier to isolate for crashes and upgrades.
- **Fit with indexing:** Cold-start time, **batching** for scan/fan-out, **vector dimension** stability vs **Qdrant** collection metadata, and behavior when the model **version** changes mid-workspace.
- **Resource policy:** CPU-only vs GPU, maximum concurrent embed calls, and how this coexists with **BiFrost** / local LLM contention on the same machine.

### Distribution and legal

- **License-first:** Any **bundled weights** or **default download URL** must comply with the model’s **license and redistribution terms**; maintain **NOTICE** / **third-party** attribution in the repo or installer as required.
- **Practical packaging paths (pick per model):**
  - **Ship in-repo or in the installer** only when redistribution is explicitly allowed and artifact size is acceptable.
  - **Download on first install or first enable** (checksum-verified, org-mirror-friendly) when the license permits **runtime fetch** but not **vendoring**—document size, hash, and offline fallback for air-gapped operators.
- **Exploration output:** A short **spike or design note** listing candidate models, legal constraints, and a **recommendation** (ship in v0.3, feature-flag pilot, or defer).

### Research notes: local ONNX embedding, optional vectordb-cli path, retrieval depth

The material below was **carried from `[version-v0.2.md](version-v0.2.md)`** when that doc was trimmed to the **shipped** RAG baseline. It **only** informs this exploration (internal ONNX/sidecar embedding, indexer experiments, and retrieval quality ideas); it is **not** a parallel locked contract. Today’s ingest path remains gateway-mediated (`POST /v1/ingest`, indexer REST) unless an implementation explicitly adds an alternative populator.

**Map to v0.3 identity:** Older sketches derived collections from **user + project**. Chimera v0.3 targets **tenant + project + optional flavor** and **base + flavor union** at retrieval time ([Workspace embedding scope (project + flavor)](#workspace-embedding-scope-project--flavor)). Any **manager + vectordb-cli** or pure-local indexer design must reconcile **collection naming** and **path** conventions with that model (and with **relative `source`** in HTTP ingest — see `[plans/indexer.md](plans/indexer.md)`) if both stacks coexist.

#### 1. Connection information, ports, paths, and configuration

- **Qdrant ports** (firewall / localhost only):
  - **Primary:** **6334/TCP (gRPC)** — intended for indexing and querying in this design.
  - **Optional:** **6333/TCP (HTTP/REST)** — dashboard, manual checks, or health-style probes.
  - No external exposure; bind to **localhost** or same-machine private network. **TLS** only if traffic leaves the host.
- **Connection details:**
  - `QDRANT_URL` (example default: `http://localhost:6334` — **note:** URL scheme must match client library expectations for gRPC vs REST; align with Qdrant client docs) or equivalent in `config.toml`.
  - Optional `QDRANT_API_KEY` shared between indexer manager, gateway, and Qdrant when enabled.
  - **Manager** process injects these per indexing run; **gateway** reuses the same logical connection (singleton + pooling).
- **Key paths** (manager / gateway):
  - **Source directories:** **absolute** paths resolved from gateway **project config** (contrasts with **relative `source`** in HTTP ingest — if both worlds coexist, define an explicit mapping at integration time).
  - **ONNX embedding model + tokenizer:** fixed **read-only** paths to the `.onnx` file and tokenizer assets; **must match exactly** between indexer (**vectordb-cli** or equivalent) and gateway at query time.
  - **vectordb-cli config:** prefer **environment variables + CLI flags** over `~/.config/vectordb-cli/config.toml` to reduce file-locking and stale state.
- **Collection naming** (deterministic; **shared** manager + router code):
  - Derive stable names from the **same logical keys** as production retrieval (for v0.3: **tenant + project + flavor** semantics, not an ad-hoc alternate scheme unless documented).
  - Sanitize for Qdrant (no slashes, respect length limits).
  - **Per-scope collections** — isolation without relying on payload filters alone where that matches the deployed adapter (same *shape* as one collection per `(tenant, project, flavor)` in the gateway plan).

#### 2. Indexing flow (manager process)

- **Manager** (separate **Go** process) periodically or via webhook **pulls project config** from the gateway (tenant/workspace keys + file paths).
- **Per workspace:** derive **collection name**, run **vectordb-cli** repo management + sync/index with retries and exponential backoff.
- **Full re-index vs delta** depending on **Git repo** vs plain directory.
- Indexing = **short-lived CLI invocations** (not a daemon); data lands in Qdrant and is **immediately** queryable.
- **Watch-outs:** **fsnotify** or gateway push for change detection; schedule work on **separate CPU cores** so indexing does not starve the gateway.

#### 3. Query-time flow (router / gateway layer)

Target **request-scoped** pipeline (**under ~600 ms** end-to-end where practical):

1. Extract **tenant + project (+ flavor)** identifiers from the incoming request (and apply **union** rules when flavors are present — see workspace scope above).
2. Compute the **exact Qdrant collection name(s)** (same derivation as the manager).
3. **Enrich** the raw query text (see §4).
4. **Embed** enriched text with the **identical ONNX model** as the indexer.
5. **Vector search** on the relevant collection(s).
6. **Validate and rerank** top‑k (score thresholds, intra-file checks, micro-judging).
7. **Optional** iterative refinement (**≤ 2** rounds): follow-up queries → re-search → merge.
8. Attach validated top‑k chunks (metadata: `file_path`, `language`, `chunk_type`) to the final LLM prompt.
9. **Graceful fallback:** if the collection is missing or Qdrant is unreachable, return **empty context** rather than failing the chat request.

#### 4. Embedding the query + enrichment strategies

- **Core embedding:** always the **same ONNX model and tokenizer** as at index time. Input = **enriched** query text; output vector goes straight to Qdrant search. **Dimension and normalization** must match.
- **Enrichment** (before embedding), examples:
  - **Simple rewrite:** small LLM reframes the query as a precise dev-style search (symbols, file patterns, edge cases).
  - **Multi-query:** **3–5** variants; embed each and fuse (**RRF** or vector averaging).
  - **HyDE:** LLM drafts a short hypothetical snippet that would answer the query; embed the hypothetical.
  - **Context injection:** prefix with **project hints** from gateway config (language, framework, etc.).
- **Normalization:** final enriched text should follow the **same whitespace / newline rules** as the indexer to stay in the same embedding space.
- **Alignment test:** index a known snippet → enrich a matching query → expect **self-retrieval score > ~0.85** (tune per model).

#### 5. Model size and type recommendations (CPU-friendly, local)

Aim for **~4–6 GB RAM** total, **quantized** execution, **sub‑300 ms** per hot path on a typical dev machine (targets, not guarantees).

- **Embedding (index + query):** e.g. **BGE-M3**, **bge-base-en-v1.5** (dense + sparse hybrid where supported); alternatives **Nomic Embed Text v1.5**, **E5-base-v2**, **Jina Code Embeddings v2** (code-heavy). Require **ONNX/GGUF**, **8-bit** quantization where used; **fixed dimension**.
- **Small LLM** (enrichment, HyDE, follow-ups, micro-judge): e.g. **Phi-4-mini-instruct** (~3.8B); alternatives **Llama 3.2** 1B/3B, **Gemma 3** 1B/4B, **Qwen3** small, **SmolLM2** 1.7B. Run **4-bit/8-bit GGUF** via **llama.cpp** / **Ollama** or ONNX bindings.
- **Dedicated reranker:** classic **cross-encoder** (e.g. **ms-marco-MiniLM** L-6 / L-12) on top‑20–50; or **bge-reranker-base**, **mxbai-rerank-xsmall**.

#### 6. Caching, better matching, validation, and iteration

- **Caching:**
  - **Embedding cache:** key ≈ hash(enriched query + tenant + project + flavor scope + model hash) → vector; in-memory or **BoltDB**; **5–15 min TTL** or invalidate on re-index for that collection.
  - **Full result cache:** top‑k + scores; invalidate on **any indexer run** for that collection.
- **Better matching:** hybrid **dense + sparse/BM25** at collection creation where supported; **rerank** post-retrieval; **metadata** filters (`file_path`, `language`, `chunk_type`); optional **pseudo-relevance** feedback (average top‑k vectors or text → new search).
- **Validation** before prompt attachment: hard **cosine** floor (e.g. **> 0.75**); **intra-file** neighborhood embedding check; **self-similarity** across top‑k; **LLM micro-judge** (batched, confidence **> 0.7**); code signals (AST/symbols) where available.
- **Iterative loop:** router-controlled; **max 2** rounds; **relevance-delta** stop + **overall timeout**; enable only for **complex** queries.

#### 7. Implementation watch-outs and best practices

- **Embedding alignment** is non-negotiable — **golden** test projects.
- **Collection naming** must be **identical** and **collision-free** in manager and router.
- Keep router decisions **request-scoped** and **unit-testable** (enrichment + validation).
- **Latency budget:** enrichment + validation + optional iteration **~300–600 ms** total when features are on.
- **Resource isolation:** indexer/manager vs gateway **CPU affinity**; **fallback** paths always available.
- **Test loop:** small golden codebase → full manager cycle → end-to-end gateway request → assert relevant chunks.
- **Operations:** monitor Qdrant **disk**; **payload indexes** on frequently filtered fields.

### Relationship to the setup wizard

- **Document order:** This section appears **after** [Product naming](#product-naming) and [Credential file naming](#credential-file-naming) and **immediately before** [Workspace embedding scope (project + flavor)](#workspace-embedding-scope-project--flavor); together they precede [First-run token handoff](#first-run-token-handoff) and [Setup wizard](#setup-wizard) so wizard copy and combobox sources can include an **internal** embedding entry once the contract is clear—see [Setup wizard](#setup-wizard) step 5 below.
- If exploration is still open when wizard ships, the wizard keeps today’s behavior (Ollama / provider-derived lists) and this section’s **config sketch** becomes the **forward-looking** contract.

**Deliverables checklist**

- Spike or design note: feasible Go/native path, memory and disk budget, and mapping to existing embedding dimensions and indexer expectations.
- Legal checklist per candidate model: **bundle** vs **download-on-first-use** vs **operator-supplied path only**.
- Config sketch: `enabled`, internal **provider** key, **model** id, optional **weights path** / **cache directory**, listen address for the local `/embeddings` shim.

**Acceptance**

- Written recommendation: **ship in v0.3**, **pilot behind a flag**, or **defer**—with explicit notes for [Setup wizard](#setup-wizard) step 5 (combobox includes internal provider + model when implemented).

**Status:** `exploration`

---

## Workspace embedding scope (project + flavor)

**Goal:** When files are embedded, ingest them under a **unique key** derived from **user** (the authenticated operator / tenant identity used for isolation today) **+ project + flavor**. Operators can start with a **broad corpus** (e.g. all journal files) on a **project id** alone, then add **flavors** for private areas or specializations without re-copying that base corpus into every flavor.

**Scope**

### Ingestion identity

- Each indexed chunk (or equivalent vector payload) is associated with a **workspace scope** that includes:
  - **User** — the same identity that gates auth and multi-tenant separation (exact field name in config/API may follow existing gateway/indexer conventions).
  - **Project** — required **project id** for the workspace.
  - **Flavor** — optional **flavor id**; **absent** or **empty** means this workspace is the **base / global** embeddings bucket for that **user + project**.

### Base (project-only) vs flavored workspaces

- A workspace defined with **project id** and **no flavor id** is the **base** embeddings set for that project: its vectors participate in **every** retrieval for that **user + project**, regardless of which **flavor id** (if any) the client sends on a given request.
- A workspace defined with the **same project id** and a **non-empty flavor id** holds **additional** embeddings scoped to that flavor only (in addition to base, not instead of replacing base unless product explicitly adds a “replace base” mode—**v0.3 default:** additive union as below).

### Retrieval union (flavored request)

**Requirement**

Given a user has defined a workspace with a **project id** but **no flavor id**.  
And the contents of that workspace have been **indexed**.  
And the user defines a **new** workspace with the **same project id** and a **flavor id** set.  
And the contents of that workspace have been **indexed**.  
When the user performs a **chat** (or RAG) request scoped to that **project id** and the **flavor id**,  
Then retrieval must query embeddings from **both**:

- the workspace scoped to **project** with **no flavor** (base / global for that project); and  
- the workspace scoped to **project + flavor**.

**Multi-workspace request pool**

- The client may specify **any number** of workspace selectors **(project + optional flavor)** on a request (exact header or body shape belongs in API design; align with the `X-Chimera-*` header contract in [Product naming](#product-naming)).
- **All valid** declared workspaces for that authenticated user are included in the **embedding search pool** for that request (union of hits across those scopes, with deduplication by chunk identity where the same file appears under multiple selectors only if product allows—**v0.3 default:** dedupe by stable chunk id / source key so overlapping paths do not double-count unless intentionally indexed twice).
- **Invalid** or unknown workspace references should fail **loudly** (clear error to the client) or be **ignored** with a warning in logs—pick one behavior in implementation and document it; do not silently drop **all** scopes.

### Operator story

- **Natural flow:** index “everything I want everywhere” under **project only**; add **flavored** folders or repos later for **sensitive** or **topic-specific** material; flavored chats automatically see **shared baseline** plus **flavor overlay**.
- **Docs:** Update `[plans/indexer.md](plans/indexer.md)` and `[configuration.md](configuration.md)` when fields for project/flavor per index and per-request workspace lists are fixed.

### Relationship to the setup wizard

- [Setup wizard](#setup-wizard) step 5 (indexing setup) should allow defining **project** and optional **flavor** per index in line with this model; step 6 (test indexing) should run retrieval using the **same union rules** as production so operators validate behavior before leaving the wizard.

**Deliverables checklist**

- Indexer / gateway: persist and filter vectors by **(user, project, flavor?)** consistently on ingest and search.
- Chat (RAG) path: implement **base + flavor** union when a flavor is present; implement **multi-workspace** union when multiple selectors are provided.
- Operator docs: examples for “journal base + `private` flavor” and multi-selector requests.

**Acceptance**

- The **Given / When / Then** requirement above holds in automated or manual acceptance tests for at least one reference project.
- Multi-selector requests include every **valid** workspace in the search pool; documented behavior for invalid selectors.

**Status:** `todo`

---

## First-run token handoff

**Goal:** On the **first** run, the user obtains a **gateway API token**, optionally persists it, then **restarts** the app and supplies the token (UI or environment) so the second-run wizard can run authenticated.

**Scope**

### First screen

1. The application displays an **API key** (gateway-issued token) that the user can **copy**.
2. Below the key:
  - Optional action: **Save key** — when pressed, **upsert** into a **dotenv** file (project/agreed path): if `CHIMERA_GATEWAY_TOKEN` is **not** already defined, set it to this key; if already defined, do **not** overwrite without an explicit future “replace” flow (this plan: **only set when absent**).
3. User guidance: copy and/or save, then **close** the application.
4. On next launch, the user either:
  - Pastes the key into the app when prompted, or  
  - Relies on `CHIMERA_GATEWAY_TOKEN` being read from the environment / dotenv load order as implemented.

**Acceptance**

- Token display must be compatible with whatever the gateway already uses for **tenant auth** (same token used for `Authorization: Bearer` elsewhere).
- **Save** behavior must be safe on repeated launches (idempotent upsert, no silent clobber of user-set values).

**Status:** `todo`

---

## Setup wizard

**Goal:** After the token is available on second launch, walk through **configuration and testing** in **seven steps**, with **Skip setup** returning the user to the **normal multi-tab** UI.

**Scope**

**Global navigation**

- **Step 1 (welcome):** Bottom-left **Skip** → main tab view. Bottom-right **Continue** → step 2.
- **Steps 2–6:** Bottom-left **Back** (step 2 back goes to welcome). Bottom-right **Continue** / **Next** advances.
- **Step 7:** Bottom-left **Back**. Bottom-right **Finish** → main multi-tab view.

---

### Step 1 — Welcome / overview

- High-level overview of what will be configured.
- Show **how many steps** the process has (seven).
- **Skip** (bottom-left) → current main tab view.
- **Continue** (bottom-right) → step 2.

---

### Step 2 — Provider keys (Groq, Gemini, …)

- Collect **provider API keys** (at minimum the fields used today for Groq and Gemini).
- **Validation UX:**
  - When a key is **added** or **removed**, the system **immediately** validates against the upstream/provider and retrieves **model list**.
  - Display a **count of models discovered** for that provider configuration.
  - Whenever the **model count** changes, run **router generator** logic: regenerate **router file** and update the **fallback model list** to match the new union of models.
- **Back** → welcome. **Continue** → step 3.

---

### Step 3 — Local OpenAI-compatible server (Ollama / LM Studio / custom)

- Show **model count** from step 2; this count **updates live** as configuration changes on this page too.
- **Autodetect** a local LLM server using **common ports** for **Ollama** and **LM Studio**. If found, **pre-fill** host/port/base path fields.
- If **none** detected, leave fields empty; user **must** supply custom connection values before proceeding (or block **Continue** until valid).
- Once a URL/base is **set or detected**, query the server for **models** and show **total model count**.
- On **any model count change**, run **router generator** → update **router file** and **fallback model list** (same contract as step 2).
- **Back** → step 2. **Next** → step 4.

---

### Step 4 — Test chat with a model

- **Purpose:** Verify end-to-end **chat** through the gateway (or equivalent orchestrated path) using the models and routing available after steps 2–3.
- **Prompt area:** A **ready-to-go** default prompt is shown with its text **selected / highlighted** so the user can **start typing** to immediately replace it with their own message.
- **Send:** **Enter** or a **Send** control submits the prompt.
- **Conversation panel:** The assistant **reply streams or appears live** in the same view, **after** the user’s message, as a **conversation chain** (user and assistant turns in order).
- **Logs (below the conversation):** A **summarized conversation log** for this exchange—**openable and viewable the same way** as on the main **logs** page (same structure, expand/collapse, and detail as production logs for this session).
- **Back** → step 3. **Next** → step 5.

---

### Step 5 — Indexing setup

- Brief explanation of **why indexing matters** and that users should choose folders they want searchable.
- **Project and flavor:** each index is configured with a **project id** and an optional **flavor id**, matching [Workspace embedding scope (project + flavor)](#workspace-embedding-scope-project--flavor): **project-only** indexes are **base** context for that project; indexes with a **flavor** add a scoped overlay. Copy should explain the “start broad, add flavors later” flow (e.g. journals vs private notes).
- **“Add a Folder”** control: placed **upper-right** (per spec).
- **Embedding model** panel: **combobox** of **valid embedding models** derived from configured providers + local server (models suitable for `/embeddings` and gateway/Qdrant expectations).
  - **Default selection:** `ollama/nomic-embed-text:latest` or the project’s agreed default that matches **Qdrant**, **chunking**, and **indexer** settings from config.
  - When [Internal embedding provider (exploration)](#internal-embedding-provider-exploration) ships, include entries for the **internal provider name** + configured **embedding model** alongside Ollama/provider-derived options.
- **No valid embedding models:**
  - Show a clear **message** that no embedding-capable models are available.
  - **Disable** “Add a Folder”.
  - If the user attempts folder add (or focus the disabled control), **animate** the embedding panel to indicate it cannot be configured yet, show **warning** + instructions to go **back** to earlier steps and add a **local embedding-capable** model (e.g. **step 3** local server or **step 2** provider keys, as appropriate), or enable/configure the **internal embedding** path when available.
- **When valid models exist:** user can **create, modify, and delete** indexes (folders / indexer entries per existing product behavior).
- **Behavior:** index changes trigger **index creation** / updates as they do in the main app; embedding model changes re-point embedding configuration.
- **Back** → step 4. **Next** → step 6.

---

### Step 6 — Test indexing (conditional)

- **If the user defined no indexes in step 5:** this step is **disabled** or skipped (implementation choice: auto-skip vs greyed step with explanation—product should not pretend indexing can be tested).
- **When indexes exist:**
  - Explain how **embeddings** are used in practice.
  - **Query panel:** text box; on **Enter** or **Query** button:
    1. **Highlight** the query text (visual feedback).
    2. Run search **across all workspaces** (same semantics as production search), including **base + flavor** union and any **multi-workspace** rules from [Workspace embedding scope (project + flavor)](#workspace-embedding-scope-project--flavor).
    3. **Zero results:** show that explicitly; add **notes/warnings** based on indexer state (idle, error, no chunks, etc.).
    4. **Multiple results:**
      - First block: **summary** — total hits across workspaces; **number of distinct workspaces** with a match.  
      - Second block: **details** — file paths and **short excerpts**.
  - Below: **indexer run log** view — **same content and live updates** as the dedicated **log** page in the app so users see progress and errors.
- **Back** → step 5. **Next** → step 7.

---

### Step 7 — Integration (VS Code Cline)

- Show the **Cline** integration panel (overview + actions aligned with [IDE integration (VS Code Cline)](#ide-integration-vs-code-cline); **Continue**-specific UI and copy are **replaced** for this release’s default path).
- Rename **“indexed folders”** to **“setup projects”** in this context.
- Combo box lists **setup projects** (indexed-folder entries).
- Snippets and **create** actions generate **VS Code Cline**–appropriate configuration (paths and keys per Cline’s documented OpenAI-compatible / custom-base setup—exact filenames belong in implementation and `[configuration.md](configuration.md)`); include a **global** or **user-default** variant where Cline supports it (analogous to the old global `.continue/config.yaml` flow, but **not** Continue-specific).
- Entries remain **copyable** and **creatable** like today; user selects an entry and uses **copy** and/or **create** when defined indexes exist as applicable.
- **Back** → step 6. **Finish** → **main multi-tab** application view.

---

### Cross-cutting implementation notes

- **Router generator** and **fallback model list** must be **shared** between the wizard and the main settings UI so wizard changes do not use a one-off code path.
- **RAG / indexer** behavior for **project + flavor** and **multi-workspace** requests must match production (same code path or thin wrapper), not a wizard-only subset—see [Workspace embedding scope (project + flavor)](#workspace-embedding-scope-project--flavor).
- **Second-run detection** should be robust (e.g. token present + first-time wizard flag in local state), so reinstalls and upgrades behave predictably—exact mechanics belong in implementation with UX review.
- Peer-to-peer **upstream** configuration might surface in advanced settings in the same release or a follow-up; this plan does not require the **seven-step wizard** to cover peer URLs unless product wants it—default remains **operator docs + config files** from [Peer backends](#peer-backends).

**Acceptance**

- The seven-step wizard can be skipped, navigated with Back/Continue/Next/Finish, and returns to the normal multi-tab UI.
- Provider and local-server model count changes trigger the shared router generator and update fallback model lists.
- Chat, indexing, and **Cline** integration checks use the same logs and operator surfaces as the main application.

**Status:** `todo`

---

## IDE integration (VS Code Cline)

**Goal:** Replace **VS Code Continue**–oriented integration **samples, snippets, and in-repo examples** with **VS Code Cline**–equivalent guidance so operators wire Chimera through the extension the product standardizes on for v0.3.

**Scope**

- **Wizard and desktop UI:** [Setup wizard](#setup-wizard) step 7 and any **main-app** “integration” or “IDE setup” surfaces default to **Cline** labels, snippet text, and file targets (deprecate or remove **Continue** as the primary advertised path unless a short **migration** footnote is explicitly desired).
- **Repository docs and packaged samples:** Update paths such as packaged `**vscode-continue/`** (or successor folder name), README sections, and any **embed UI** / static HTML that still says **Continue** where the intent is “IDE OpenAI-compatible client”—retarget to **Cline** with accurate install links and config shapes.
- **Parity:** Preserve today’s operator affordances—**copy**, **create**, per–**setup project** variants, and **project / flavor** header behavior where Cline can consume them—mapping from the old Continue contract rather than dropping features silently.

**Deliverables checklist**

- Wizard step 7 + main settings: **Cline** snippets and copy; no dangling **Continue**-only instructions on first-touch paths.
- In-repo examples and README(s): **Cline** samples; archive or relocate **Continue**-only content if kept for history (label **legacy**).
- `[configuration.md](configuration.md)` (and Continue/Cline pointer in root **README** if present): document **Cline** base URL, API key / gateway secret, and model routing expectations against Chimera.

**Acceptance**

- A new operator following **only** v0.3 docs and the setup wizard can connect **VS Code Cline** to Chimera without hunting obsolete **Continue** examples.
- Spot-check: search the repo for prominent **Continue** integration strings on operator-facing paths; remaining hits are intentional (migration notes) or out of scope.

**Status:** `todo`

---

## Peer backends

**Goal:** Let one operator route to another operator's published OpenAI-compatible upstream without chaining gateway-to-gateway.

**Scope**

This theme summarizes what `[porcelain.plan.md](porcelain.plan.md)` already assigns to **v0.3** so implementation and docs stay aligned.

### Release-roadmap slice

From the master **Release roadmap** table:

- **Peer-to-peer model backends**: call **another operator’s BiFrost** (or compatible OpenAI proxy) over a **host-routable** URL and **published** port (not Compose-internal DNS from another machine).
- **Proxy-issued credentials** (e.g. virtual keys where the upstream supports them) for **cross-host** authentication.
- **Gateway / upstream configuration** and **operator documentation** for peer paths: *Peer topology · 1–3*, *Model selection and routing policy · 3* (peer as `base_url` / `api_base`), and *Deployment · 3* (cross-host publishing vs intra-stack DNS)—see `[porcelain.plan.md](porcelain.plan.md)`.
- **Per-key / usage observability** (*Resilience · 1*): track which key/backend was used and exposure to RPM/TPM-style limits where upstream headers exist.

### Product rules

- **Peer = their upstream (BiFrost / compatible proxy), not their Gateway** (*Peer topology · 2*): configure OpenAI-compatible `api_base` / `base_url` to the **peer’s published upstream** (e.g. Tailscale/LAN IP + **published** proxy port + `/v1`). Use credentials **they** issue (virtual keys or equivalent when supported). Do **not** chain **Gateway → peer Gateway** as the default integration (same bullet).
- **Independent stacks** (*Peer topology · 1*): each operator has their own Chimera instance, client-auth secrets (`api-keys.yaml`), and policy; no assumption that one gateway “owns” another’s RAG.
- **Document ports per host** (*Peer topology · 3*): distinguish **Chimera** (IDE-facing OpenAI-compatible entry) vs **peer upstream** (`api_base` / `base_url` target); firewall/VPN expectations; TLS/mTLS deferred to **v0.7** unless operators add their own terminator.
- **Cloud vs local policy** (*Model selection and routing policy · 3*): **Peer upstream** appears as a **remote-runner** entry in routing policy.
- **Graceful degradation** (*Resilience · 2*): same fail-over / fail-fast behavior when a peer upstream is in the chain; **no** gateway queue until **v0.8**.
- **Containers / networking**: from **v0.3**, compose/docs consider **LAN peer access** to published upstreams where enabled; TLS posture for peer URLs ships with **v0.7** (*Security · 2–5*).

### Deliverables checklist

- Configuration surfaces (and/or files) to add **peer upstream** backends with proxy-issued credentials where applicable and host-reachable base URLs.
- Operator docs: cross-host topology, **published** ports, virtual keys, anti-patterns (Compose hostname of peer stack, gateway-on-gateway).
- **Observability (*Resilience · 1*)**: per-key / per-backend usage signals where APIs expose limits or identifiers.

**Acceptance**

- Peer upstream configuration can target a host-routable OpenAI-compatible proxy URL with credentials issued by the peer operator.
- Operator docs explain ports, network expectations, credential handoff, and the gateway-on-gateway anti-pattern.
- Per-key or per-backend usage signals are visible where upstream APIs expose enough data.

**Status:** `todo`

---

## Explicitly not this version

- Do not route Gateway -> peer Gateway as the default peer integration; peer routes target a host-routable upstream proxy.
- Do not make TLS/mTLS or untrusted-network hardening a v0.3 requirement; that remains a later hardening release.
- Do not add a gateway queue or priority scheduler; graceful degradation remains the v0.3 behavior.
- Do not reintroduce legacy `chimera`, `CHIMERA_*`, or `X-Chimera-*` identifiers in current operator-facing behavior.

---

## Verification


| Area                             | Quick check                                                                                                                                                        |
| -------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Product naming                   | README, onboarding, UI copy, and startup logs reflect Porcelain / Chimera / Locus decisions.                                                                       |
| Credential naming                | `api-keys.yaml`, `api_keys`, `secret`, and `paths.api_keys` are implemented or migration behavior is documented.                                                   |
| Internal embedding (exploration) | Spike or design note, per-model legal/distribution checklist, and ship / pilot / defer decision; config sketch matches indexer-style opt-in start.                 |
| Workspace embedding scope        | Ingestion keys `(user, project, flavor?)`; flavored chat unions base + flavor; multi-workspace requests pool all valid scopes; wizard step 5–6 matches production. |
| First-run token handoff          | First launch shows a copyable gateway API key and optional safe dotenv save.                                                                                       |
| Setup wizard                     | Seven steps navigate correctly, support skip/finish, and use shared router regeneration; embedding combobox reflects internal provider when implemented.           |
| IDE integration (Cline)          | Step 7 and in-repo samples target Cline; Continue-only operator paths removed or marked legacy.                                                                    |
| Peer backends                    | Peer upstream + credentials + docs meet the peer scope checklist.                                                                                                  |


When this plan is implemented, update `[porcelain.plan.md](porcelain.plan.md)` **Release roadmap** row for v0.3 if the shipped scope differs (e.g. split peer backends vs onboarding into separate releases).

---

## See also

- `[version-v0.2.md](version-v0.2.md)` - previous version
- `[porcelain.plan.md](porcelain.plan.md)` - product roadmap and requirements
- `[configuration.md](configuration.md)` - configuration reference
- `[plans/indexer.md](plans/indexer.md)` - indexer milestones that may cross-link with this release
- `[plans/_template.md](plans/_template.md)` - phase-level plan template

