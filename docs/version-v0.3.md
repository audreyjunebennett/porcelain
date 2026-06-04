# Version 0.3 - Chimera branding and onboarding


| Field                          | Value                                          |
|--------------------------------|------------------------------------------------|
| **Doc kind**                   | `version-roadmap`                              |
| **Owners / areas**             | Gateway desktop, onboarding, branding          |
| **Status**                     | `active`                                       |
| **Targets**                    | Gateway/desktop v0.3                           |
| **Last updated**               | See git history                                |
| **Supersedes / superseded by** | Builds on `[version-v0.2.md](version-v0.2.md)` |


## At a glance

Make the gateway easier to set up and clearer about what it is. This plan’s **sections follow a single narrative**: **rename** (Porcelain · Chimera · Locus), **credential file naming** (`api-keys` / `secret`), **internal embedding** exploration, **operator-managed virtual models** (per-model routing from `/ui/settings`), then **first-run token handoff** and the **setup wizard**.


| Theme                                                                                              | Outcome                                                                                                                                | Status        |
|----------------------------------------------------------------------------------------------------|----------------------------------------------------------------------------------------------------------------------------------------|---------------|
| [Product naming](#product-naming)                                                                  | Layered names in docs, UI, and startup logs with naming contracts ([`plans/v0-3-naming-migration.md`](plans/v0-3-naming-migration.md)) | `done`        |
| [Credential file naming](#credential-file-naming)                                                  | `api-keys.yaml` / `api_keys` / `secret`; reserve "token" for tokenizer counts                                                          | `done`        |
| [Operator UI filesystem dev mode](#operator-ui-filesystem-dev-mode)                                | Serve `embedui/` from disk via `CHIMERA_ADMINUI_ROOT` for live UI edits without rebuilding the gateway                                 | `done`        |
| [plans/chimera-gateway-package-boundaries.md](plans/chimera-gateway-package-boundaries.md)         | Admin UI / operator API package split; shared DTOs                                                                                     | `done`        |
| [plans/chimera-gateway-refactor.md](plans/chimera-gateway-refactor.md)                             | Gateway naming clarity; logs UI modularization train                                                                                   | `done`        |
| [plans/adminui-filesystem-dev-mode.md](plans/adminui-filesystem-dev-mode.md)                       | Optional `CHIMERA_ADMINUI_ROOT` disk serving for embed UI dev                                                                          | `done`        |
| [plans/embedui-component-gallery.md](plans/embedui-component-gallery.md)                           | Static component gallery paths and upkeep                                                                                              | `done`        |
| [plans/embedui-component-system.md](plans/embedui-component-system.md)                             | Reusable embed UI primitives and module split                                                                                          | `done`        |
| [plans/embedui-event-log-panel.md](plans/embedui-event-log-panel.md)                               | Per-card event log layout and interaction                                                                                              | `done`        |
| [plans/embedui-logs-workspaces-merge.md](plans/embedui-logs-workspaces-merge.md)                   | Unify logs and workspace indexers in embed UI                                                                                          | `done`        |
| [plans/embedui-theme-styleguide.md](plans/embedui-theme-styleguide.md)                             | Theme tokens and static styleguide                                                                                                     | `done`        |
| [plans/locus-desktop-supervisor-contract.md](plans/locus-desktop-supervisor-contract.md)           | Desktop ↔ supervisor process and readiness contract                                                                                    | `done`        |
| [plans/vectorstore-broker-wrapper-hard-cut.md](plans/vectorstore-broker-wrapper-hard-cut.md)       | **chimera-vectorstore** / **chimera-broker** wrapper binaries and supervisor cutover                                                   | `done`        |
| [plans/v0-3-naming-migration.md](plans/v0-3-naming-migration.md)                                   | Product naming hard-cut execution                                                                                                      | `done`        |
| [plans/logs-ui-page-data-refreshing.md](plans/logs-ui-page-data-refreshing.md)                     | Summarized logs feed: interaction-safe rebuilds, card patches, view model                                                              | `done`        |
| [plans/embedui-operator-settings-routes.md](plans/embedui-operator-settings-routes.md)             | App shell at `/ui`, settings page rename, single gallery; drop legacy routes and deep links                                            | `done`        |
| [plans/context-window-admission.md](plans/context-window-admission.md)                             | Context window admission on chat path. Capture provider model context limits to enabled proper model router fallbacks                  | `done`        |
| [plans/supervisor-info-log-trim.md](plans/supervisor-info-log-trim.md)                             | Refine the logs even further to reduce service start up and vectorstore erroring. Lowering heartbeat logs to DEBUG                     | `done`        |
| [plans/embedui-dynamic-provider-cards.md](plans/embedui-dynamic-provider-cards.md)                 | Settings provider cards: catalog, Add provider picker, hide usage/log until configured                                                 | `done`        |
| [Internal embedding provider (exploration)](#internal-embedding-provider-exploration)              | Optional in-repo or first-install embedding runtime to reduce reliance on Ollama for `/embeddings`                                     | `exploration` |
| [Logs UI page data refreshing](#logs-ui-page-data-refreshing)                                      | Interaction-safe summarized feed; per-card patches and view model (phased)                                                             | `done`        |
| [Operator-managed virtual models](#operator-managed-virtual-models)                                | Create virtual models from `/ui/settings`; per-model fallback, routing rules, and tool-router; operator SQLite + scoped routing logs   | `done`        |
| [First-run token handoff](#first-run-token-handoff)                                                | Show, copy, and optionally save the gateway token; restart-friendly                                                                    | `done`        |
| [Setup wizard](#setup-wizard)                                                                      | Guided keys -> local server -> test chat -> indexing                                                                                   | `todo`        |
| [Deferred: provider probes and operator alerts](#deferred-provider-probes-and-operator-alerts-v05) | Per-model live probes and actionable error alerts deferred to v0.5; v0.3 uses catalog + broker health only                             | `deferred`    |


---

## What this version is

This document is the **working plan for v0.3** for this repository (**Chimera**: intelligent routing and memory layer; see [Product naming](#product-naming)). Body **sections are ordered** for delivery narrative: [Product naming](#product-naming) and [Credential file naming](#credential-file-naming) first; then [Internal embedding provider (exploration)](#internal-embedding-provider-exploration) and [Operator-managed virtual models](#operator-managed-virtual-models); then [First-run token handoff](#first-run-token-handoff) and [Setup wizard](#setup-wizard). **v0.3** targets **layered product naming** (**Porcelain**, **Chimera**, **Locus**), **api-keys** language, optional **in-repo / first-install** embedding weights **within license**, and **multi-model routing** (virtual models with per-model policy). **Workspace embedding scope (project + flavor)** and **peer backends** are scoped in [`version-v0.4.md`](version-v0.4.md). Naming and README wording in line with branch `origin/feat/chimera-branding` should be folded into this release unless superseded by a written decision.

**Companion docs:** `[chimera.plan.md](chimera.plan.md)`, `[configuration.md](configuration.md)`, `[plans/indexer.md](plans/indexer.md)`, `[plans/v0-3-naming-migration.md](plans/v0-3-naming-migration.md)` (product naming execution), `[plans/virtual-models-operator.md](plans/virtual-models-operator.md)` (virtual models execution), plus implementation plans in [Related plans](#related-plans) (gateway/embed UI refactor, supervisor contract, wrapper hard cut).

Authoritative **architecture and numbered requirements** remain in `[chimera.plan.md](chimera.plan.md)` unless this plan explicitly revises them. **Indexer** **Phase 3** in `[plans/indexer.md](plans/indexer.md)` (e.g. scoped overrides, headers) is **not** the same shipping train as **gateway desktop v0.3**; cross-link when both touch the same API.

---

## Product naming

**Execution plan:** [`plans/v0-3-naming-migration.md`](plans/v0-3-naming-migration.md) — consolidated discovery-through-closeout train for hard-cut naming (env, headers, binaries, paths, make namespace, layout, operator docs).

**Goal:** Align operator-visible language and implementation logging with the **layered architecture** introduced on `origin/feat/chimera-branding`, while retiring ambiguous “chimera-gateway” wording where it meant “this binary / service.”

**Scope**

### Architecture narrative

These names are **roles**, not four separate shipping binaries unless noted:


| Layer         | Role                                                                                                                                                                                                                                                        |
|---------------|-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
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
- Documentation set (README, onboarding, `[chimera.plan.md](chimera.plan.md)` release row when updated) reflects the architecture narrative; historical “chimera-gateway” OK in release notes only when labeled historical.
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
|-----------------------------------------------------------------------|---------------------------------------------------------------------------------------|
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

## Operator UI filesystem dev mode

**Execution plan:** [`plans/adminui-filesystem-dev-mode.md`](plans/adminui-filesystem-dev-mode.md)

**Goal:** Let developers edit operator UI assets under `adminui/embed/embedui/` and see changes after a browser refresh, without rebuilding `chimera-gateway` for every JavaScript or CSS change. Production and packaged desktop builds keep compile-time `//go:embed` when the env var is unset.

**Scope**

- **`CHIMERA_ADMINUI_ROOT`** — Absolute or relative path to the gateway embed package directory (the folder that contains `embedui/`, typically `chimera/chimera-gateway/internal/server/adminui/embed`). Inherited by supervisor → gateway child; set in the shell, `.env` (`env.example`), or via `make locus-desktop-dev-ui` / `make chimera-supervisor-dev-ui`.
- **Loopback only** — Disk mode is refused when the gateway HTTP listen address is not loopback.
- **Same URLs** — `/ui/settings`, `/ui/assets/settings/**`, etc. unchanged; only the asset backend switches from embedded bytes to disk.
- **Still requires rebuild** when changing Go handlers, `internal/naming` → `contracts.js` (`make operator-contracts-generate`), or `operator_copy.js` generation.

**Acceptance**

- With `CHIMERA_ADMINUI_ROOT` set, editing a file under `embedui/settings/` and refreshing `/ui/settings` shows the change without `make chimera-gateway-build`.
- With the env var unset, behavior matches pre-change embedded serving.

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

**Map to Chimera identity:** Older sketches derived collections from **user + project**. The target model is **tenant + project + optional flavor** and **base + flavor union** at retrieval time ([Workspace embedding scope (project + flavor)](version-v0.4.md#workspace-embedding-scope-project--flavor) in [`version-v0.4.md`](version-v0.4.md)). Any **manager + vectordb-cli** or pure-local indexer design must reconcile **collection naming** and **path** conventions with that model (and with **relative `source`** in HTTP ingest — see `[plans/indexer.md](plans/indexer.md)`) if both stacks coexist.

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
  - Derive stable names from the **same logical keys** as production retrieval (for v0.4: **tenant + project + flavor** semantics, not an ad-hoc alternate scheme unless documented).
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

- **Document order:** This section appears **after** [Product naming](#product-naming) and [Credential file naming](#credential-file-naming) and **immediately before** [Operator-managed virtual models](#operator-managed-virtual-models); it precedes [First-run token handoff](#first-run-token-handoff) and [Setup wizard](#setup-wizard) so wizard copy and combobox sources can include an **internal** embedding entry once the contract is clear—see [Setup wizard](#setup-wizard) step 5 below.
- If exploration is still open when wizard ships, the wizard keeps today’s behavior (Ollama / provider-derived lists) and this section’s **config sketch** becomes the **forward-looking** contract.

**Deliverables checklist**

- Spike or design note: feasible Go/native path, memory and disk budget, and mapping to existing embedding dimensions and indexer expectations.
- Legal checklist per candidate model: **bundle** vs **download-on-first-use** vs **operator-supplied path only**.
- Config sketch: `enabled`, internal **provider** key, **model** id, optional **weights path** / **cache directory**, listen address for the local `/embeddings` shim.

**Acceptance**

- Written recommendation: **ship in v0.3**, **pilot behind a flag**, or **defer**—with explicit notes for [Setup wizard](#setup-wizard) step 5 (combobox includes internal provider + model when implemented).

**Status:** `exploration`

---

## Logs UI page data refreshing

**Execution plan:** [`plans/logs-ui-page-data-refreshing.md`](plans/logs-ui-page-data-refreshing.md) — phased fix for `/ui/settings` summarized feed flicker, double-click card expansion, and admin form focus loss during SSE and admin poll rebuilds.

**Goal:** Operators on `/ui/settings` open cards on the first click, edit provider API keys without losing focus, and see live log updates without the whole panel flashing. Today `refreshSummarizedPanel()` assigns `innerHTML` on almost every log line and on the 12s admin poll; later phases add per-card patches and a testable view model.

### Phase 1 — Interaction-safe rebuilds (shipped)

- `summarizedPanelInteractionBlocksRebuild()` defers rebuild when focus is in any `#panel-summarized` `input` / `textarea` / `select`, plus existing evlog and admin YAML fields.
- Short pointer suppression after `details.sum-card > summary` click so card toggle races SSE debounced rebuild.
- `adminProviderKeyDraft` / `adminOllamaUrlDraft` on `ctx`, wired from `wireHandlers.js` and rendered in `adminProvider.js`.
- Interaction contract documented in [`chimera/chimera-gateway/internal/server/adminui/embed/embedui/settings/README.md`](../chimera/chimera-gateway/internal/server/adminui/embed/embedui/settings/README.md).

**Acceptance (Phase 1)**

- First click expands a collapsed card under live SSE traffic (manual smoke).
- Typing in provider key inputs (`admin-{providerId}-key`, e.g. `admin-groq-key`) or `admin-ollama-url` for 30+ seconds (spanning an admin poll) does not clear the field or drop focus.
- Existing evlog search/filter/YAML deferral unchanged.

### Phase 2 — Poll-path card patching (shipped)

- `replaceCardById()` shared helper (open state + table scroll preservation).
- `syncAdminStatePolling` calls `patchAdminCardsFromPoll()` instead of `refreshSummarizedPanel()`.
- Patches users, visible provider cards (`admin-provider-{id}` from session/catalog), routing, fallback, and router cards; skips routing trio while Configure/YAML edit mode is active.
- Missing card id → `scheduleStoryRebuild()` (debounced), not synchronous full rebuild.

**Status:** `done` (see execution plan).

---

## Operator-managed virtual models

**Execution plan:** [`plans/virtual-models-operator.md`](plans/virtual-models-operator.md) — phased delivery for virtual models in operator SQLite, gateway runtime resolution, `/api/ui` CRUD, and `/ui/settings` cards.

**Goal:** Replace the single hard-coded virtual model (`Chimera-<semver>` from `gateway.semver`) with **first-class virtual models** operators create from `/ui/settings`, the same way they manage **users** and **indexer workspaces**. Each virtual model has its own client-facing id, metadata, enable toggle, and visibility (**public** = any user; **private** = creator only). Routing that is global today — **fallback chain**, **routing-policy rules**, and **tool-router** settings — becomes **per virtual model**.

**Scope**

### Virtual model object

Each row in **operator SQLite** (same store family as workspaces; see [`plans/indexer-workspaces-sqlite-gateway-api.md`](plans/indexer-workspaces-sqlite-gateway-api.md)):

| Field           | Default  | Notes                                                       |
|-----------------|----------|-------------------------------------------------------------|
| **name**        | —        | Human label                                                 |
| **version**     | —        | With name forms the OpenAI `model` id sent by clients       |
| **description** | empty    | UI + optional `GET /v1/models` metadata                     |
| **enabled**     | `true`   | Disabled models hidden from catalog and rejected on chat    |
| **visibility**  | `public` | `private` limits catalog and chat to the creating principal |

**Bootstrap:** on first open of an empty operator DB, import the legacy stack — one public enabled model with id `Chimera-<semver>`, current `routing.fallback_chain`, `routing-policy.yaml`, and global tool-router settings from `gateway.yaml`.

### Per-model routing stack

| Block              | Required | Toggleable | Parity with today                                                           |
|--------------------|----------|------------|-----------------------------------------------------------------------------|
| **Fallback chain** | yes      | no         | Ordered upstream model ids; 429/5xx (and virtual-path 413) failover         |
| **Routing rules**  | no       | yes        | `ambiguous_default_model` + ordered `rules[]` (`when.min_message_chars`, …) |
| **Tool router**    | no       | yes        | `router_models[]`, `enabled`, `confidence_threshold`                        |
| **Future routers** | no       | yes        | Schema placeholder only in v0.3 unless a router ships                       |

### Routing rule catalog

Reusable **routing rule definitions** in the database (not duplicated logic per VM):

| Field                     | Purpose                                                                                                                  |
|---------------------------|--------------------------------------------------------------------------------------------------------------------------|
| **name**                  | Operator label (e.g. `long-user-turn`)                                                                                   |
| **routing.slug**          | Stable key for logs and metrics (aligns with [`plans/operator-message-registry.md`](plans/operator-message-registry.md)) |
| **default configuration** | Default `when` + `models` fragment; VM attachment may override                                                           |

v1 may store a monolithic **policy YAML** per virtual model for fastest parity with [`config/routing-policy.yaml`](../config/routing-policy.yaml); normalized bindings to the catalog can follow in a later phase (see open questions in the execution plan).

### Runtime and API

- **`POST /v1/chat/completions`:** resolve `body.model` against the virtual model registry; enforce enabled + visibility; run **that model’s** policy, fallback, and tool-router (RAG stays gateway-global in v0.3 unless decided otherwise).
- **`GET /v1/models`:** list all enabled **public** models plus **private** models for the authenticated principal.
- **`/api/ui/virtual-models/*`:** session-authenticated CRUD; per-model generate / evaluate (port of today’s `/api/ui/routing/*`); deprecate global YAML writes once UI lands.
- **Logs:** emit **`virtual_model_id`** on routing and conversation lines; cards show **24h usage** and **scoped evlog** panels (rule match, fallback, tool-router) filtered to that model.

### Settings UI (`/ui/settings`)

- New **Virtual models** section: **Add virtual model** draft card (workspace pattern); one **collapsible card per model** with nested **Fallback**, **Routing rules**, and **Tool router** sub-panels — reuse existing admin card renderers ([`adminFallback.js`](../chimera/chimera-gateway/internal/server/adminui/embed/embedui/settings/render/cards/adminFallback.js), [`adminRouting.js`](../chimera/chimera-gateway/internal/server/adminui/embed/embedui/settings/render/cards/adminRouting.js), [`adminRouterModels.js`](../chimera/chimera-gateway/internal/server/adminui/embed/embedui/settings/render/cards/adminRouterModels.js)) wired to per-model API endpoints.
- Retire or collapse the current **global** Routing / Fallback / Router model cards after bootstrap migration.

**Deliverables checklist**

- Operator SQLite migrations + repository; bootstrap import from legacy YAML.
- Gateway runtime: multi-model chat + models list; `virtual_model_id` in structured logs.
- `/api/ui/virtual-models` CRUD and scoped routing generate/evaluate.
- `/ui/settings` virtual model cards with create/edit/enable/disable and scoped routing logs.
- [`configuration.md`](configuration.md) update: `gateway.semver` no longer implies a single routable model id once DB is populated; global `routing.*` keys deprecated (import-only).

**Acceptance**

- Operator creates a second virtual model with a distinct fallback chain; a client using that `model` id routes differently from the bootstrap `Chimera-<semver>` model.
- Disabled and other users’ **private** models are absent from catalog and rejected on chat.
- Per-model card scoped log panel shows routing decisions for that model only.
- Existing single-VM installs auto-import without manual YAML edit.

**Status:** `done`

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

**Status:** `done`

---

## Setup wizard

**Goal:** After the token is available on second launch, walk through **configuration and testing** in **six steps**, with **Skip setup** returning the user to the **normal multi-tab** UI.

**Scope**

**Global navigation**

- **Step 1 (welcome):** Bottom-left **Skip** → main tab view. Bottom-right **Continue** → step 2.
- **Steps 2–5:** Bottom-left **Back** (step 2 back goes to welcome). Bottom-right **Continue** / **Next** advances.
- **Step 6:** Bottom-left **Back**. Bottom-right **Finish** → main multi-tab view.

---

### Step 1 — Welcome / overview

- High-level overview of what will be configured.
- Show **how many steps** the process has (six).
- **Skip** (bottom-left) → current main tab view.
- **Continue** (bottom-right) → step 2.

---

### Step 2 — Provider keys (Groq, Gemini, …)

- Collect **provider API keys** (at minimum the fields used today for Groq and Gemini).
- **Validation UX (v0.3 — light touch):**
  - When a key is **added** or **removed**, poll **chimera-broker provider health** and the live **`/v1/models`** catalog for that provider.
  - Display a **count of models discovered** for that provider configuration.
  - Optionally apply **free-tier availability** assist (Groq/Gemini) or rely on bootstrap seeding from `provider-free-tier.yaml`.
  - Whenever the **model count** or availability set changes, regenerate the **virtual model** routing stack (`POST /api/ui/virtual-models/{id}/routing/generate`) — not legacy global `gateway.yaml` routing alone.
  - **Do not** block setup on per-model live probes (chat/embed ping). Runtime already **skips and logs** unavailable or failing upstream models at use time. Richer validation, operator **alerts**, and **self-healing configuration** are scoped in [`version-v0.5.md`](version-v0.5.md).
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
- **Project and flavor (optional):** each index may be configured with a **project id** and an optional **flavor id**. Full **base + flavor union** retrieval semantics are scoped in [`version-v0.4.md`](version-v0.4.md); the v0.3 wizard collects basic indexer entries.
- **“Add a Folder”** control: placed **upper-right** (per spec).
- **Embedding model** panel: **combobox** of models from the live broker catalog (see [`plans/indexer-embedding-model-and-workspace-purge.md`](plans/indexer-embedding-model-and-workspace-purge.md)).
  - **Default selection:** `ollama/nomic-embed-text:latest` or the project’s agreed default that matches **Qdrant**, **chunking**, and **indexer** settings from config.
  - **On change:** show re-embed warning — switching models requires re-indexing workspaces.
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
  - **Query panel:** text box; on **Enter** or **Query** button (reuse [`plans/operator-workspace-search.md`](plans/operator-workspace-search.md)):
    1. **Highlight** the query text (visual feedback).
    2. Run search **across configured indexes** (same semantics as production search for the scopes defined in step 5).
    3. **Zero results:** show that explicitly; add **notes/warnings** based on indexer state (idle, error, no chunks, etc.).
    4. **Multiple results:**
      - First block: **summary** — total hits across workspaces; **number of distinct workspaces** with a match.  
      - Second block: **details** — file paths and **short excerpts**.
  - Below: **indexer run log** view — **same content and live updates** as the dedicated **log** page in the app so users see progress and errors.
- **Back** → step 5. **Finish** → **main multi-tab** application view.

---

### Cross-cutting implementation notes

- **Router generator** and **fallback model list** must be **shared** between the wizard and the main settings UI so wizard changes do not use a one-off code path.
- **Second-run detection** should be robust (e.g. token present + first-time wizard flag in local state), so reinstalls and upgrades behave predictably—exact mechanics belong in implementation with UX review.

**Acceptance**

- The six-step wizard can be skipped, navigated with Back/Continue/Next/Finish, and returns to the normal multi-tab UI.
- Provider and local-server model count changes trigger the shared router generator and update fallback model lists.
- Chat and indexing checks use the same logs and operator surfaces as the main application.

**Status:** `todo`

---

## Deferred: provider probes and operator alerts (v0.5)

**Goal:** Capture intent for a **stricter provider/model validator** and **actionable operator notifications** without blocking the v0.3 setup wizard.

**v0.3 decision:** Setup and settings rely on **broker health + catalog model counts + operator availability toggles**. When an upstream model fails at chat or embed time, the gateway **skips and logs** (fallback chain, routing warnings). That is sufficient for first-run onboarding.

**Deferred to v0.5** ([`version-v0.5.md`](version-v0.5.md)):

- **Per-model probes** — Optional live validation (minimal chat or embedding call) when an operator explicitly requests it, not as a wizard gate.
- **Error-driven alerts** — When a provider is in use and multiple model errors accumulate in structured logs, surface **actionable alerts** in the operator UI (re-enter key, mark model unavailable, regenerate fallback, etc.).
- **Desired-state gateway** — A gateway that can **return itself** (and supervised children) to the **operator’s intended configuration**, guided by page-level self-description and model-assisted remediation — see v0.5 **Operator desired-state and model-assisted configuration**.

**Status:** `deferred`

---

## Verification


| Area                             | Quick check                                                                                                                                                              |
|----------------------------------|--------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| Product naming                   | README, onboarding, UI copy, and startup logs reflect Porcelain / Chimera / Locus decisions; [`plans/v0-3-naming-migration.md`](plans/v0-3-naming-migration.md) closed.  |
| Credential naming                | `api-keys.yaml`, `api_keys`, `secret`, and `paths.api_keys` are implemented or migration behavior is documented.                                                         |
| Internal embedding (exploration) | Spike or design note, per-model legal/distribution checklist, and ship / pilot / defer decision; config sketch matches indexer-style opt-in start.                       |
| Operator-managed virtual models  | Bootstrap import; multi-model chat + catalog; per-model fallback/policy/tool-router from SQLite; `/ui/settings` CRUD cards; scoped routing logs with `virtual_model_id`. |
| First-run token handoff          | First launch shows a copyable gateway API key and optional safe dotenv save.                                                                                             |
| Setup wizard                     | Six steps navigate correctly, support skip/finish, and use shared router regeneration; embedding combobox reflects internal provider when implemented.                   |
| Deferred provider probes         | Documented in v0.3 as deferred; v0.5 roadmap covers alerts and optional probes ([`version-v0.5.md`](version-v0.5.md)).                                                   |


When this plan is implemented, update `[chimera.plan.md](chimera.plan.md)` **Release roadmap** row for v0.3 if the shipped scope differs (e.g. split onboarding vs RAG scope into separate releases).

---

## Related plans

| Document                                   | Role                                                     | Status |
|--------------------------------------------|----------------------------------------------------------|--------|
| `[plans/indexer.md](plans/indexer.md)`     | Indexer milestones that may cross-link with this release | —      |
| `[plans/_template.md](plans/_template.md)` | Phase-level plan template                                | —      |

---

## See also

- `[version-v0.2.md](version-v0.2.md)` - previous version
- `[version-v0.4.md](version-v0.4.md)` - next version (workspace embedding scope, peer backends)
- `[version-v0.5.md](version-v0.5.md)` - operator desired-state gateway, model-assisted configuration, provider alerts
- `[chimera.plan.md](chimera.plan.md)` - product roadmap and requirements
- `[configuration.md](configuration.md)` - configuration reference

