# Version 0.4 - Ensembles, RAG scope, peer backends, and in-app settings

| Field                          | Value                                                                                                                 |
|--------------------------------|-----------------------------------------------------------------------------------------------------------------------|
| **Doc kind**                   | `version-roadmap`                                                                                                     |
| **Owners / areas**             | Gateway, BiFrost/upstream contracts, indexer/RAG, operator docs, desktop and web UI (settings, search, paste-back UX) |
| **Status**                     | `draft`                                                                                                               |
| **Targets**                    | Gateway v0.4                                                                                                          |
| **Last updated**               | See git history                                                                                                       |
| **Supersedes / superseded by** | Builds on [`version-v0.3.md`](version-v0.3.md)                                                                        |

## At a glance

**v0.4** delivers **ensemble** (“heavy thinking”): the gateway orchestrates **parallel drafts**, optional **critique/synthesize**, and clear **streaming error** behavior when a phase fails—without pushing queueing or security-milestone work forward. It also **productizes external human escalation**: configurable surfaces, privacy disclosure, confidence-based engagement, and **paste-back / session** handling so operators can safely involve a human outside the stack when policy demands it.

On the **RAG** side, indexers send **manifest ingest** (pre-chunked files with line metadata) through the gateway so **embeddings land in the vector store** with **line-accurate snippets** in chat; **workspace embedding scope** (**user + project + flavor**, base corpus + unions) ships as a coherent retrieval model. Operators gain **lifecycle controls** to **remove or purge** corpus tied to a **specific workspace** (scoped indexer / tenant–project–flavor), without ad-hoc Qdrant surgery. **Peer backends** let one operator route to another’s published OpenAI-compatible upstream (typically BiFrost) over a host-routable URL. The **desktop or web** shell gains **first-class settings**: change configuration **in the app** (not only by editing YAML on disk), plus **search** on the settings surface and/or **across the application** so options are discoverable as surface area grows.

| Focus                                                                                      | Outcome                                                                                                                                             | Status  |
|--------------------------------------------------------------------------------------------|-----------------------------------------------------------------------------------------------------------------------------------------------------|---------|
| [Two-phase ensemble](#two-phase-ensemble)                                                  | **N** parallel drafts → critique/synthesize → one answer; gateway orchestrates, upstream runs drafts                                                | `todo`  |
| [Triggers and streaming semantics](#triggers-and-streaming-semantics)                      | Auto + **`//deep`** on virtual **`chimera-<semver>`** only; streaming and phase-failure contract specified here                                     | `todo`  |
| [External human escalation](#external-human-escalation)                                    | Configurable surfaces, privacy disclosure, confidence-gated copy-paste escalation with paste-back merge                                             | `todo`  |
| [Workspace embedding scope (project + flavor)](#workspace-embedding-scope-project--flavor) | Ingest and retrieve by **(user, project, flavor?)**; base + flavored union; multi-workspace pool                                                    | `todo`  |
| [Indexer manifest ingest (line metadata)](#indexer-manifest-ingest-line-metadata)          | Manifest pre-chunk ingest; line-accurate spans in vector store and chat UI ([`plans/indexer-manifest-ingest.md`](plans/indexer-manifest-ingest.md)) | `todo`  |
| [Indexer workspace lifecycle and purge](#indexer-workspace-lifecycle-and-purge)            | Operator path to manage indexers and purge vectors for one workspace scope                                                                          | `todo`  |
| [Indexer Phase 7 — model-assisted strategy](#indexer-phase-7--model-assisted-strategy)     | Optional gateway-mediated indexing recommendations from tree + config summary ([`plans/indexer.md`](plans/indexer.md) Phase 7)                      | `todo`  |
| [Configuration in the desktop or web UI](#configuration-in-the-desktop-or-web-ui)          | Edit supported runtime settings in desktop or web UI, not YAML-only                                                                                 | `todo`  |
| [plans/env-precedence-contract.md](plans/env-precedence-contract.md)                       | Unified env/config precedence and runtime profiles across binaries                                                                                  | `draft` |
| [Settings and application search](#settings-and-application-search)                        | Search settings and/or app-wide so operators find controls quickly                                                                                  | `todo`  |
| [Peer backends](#peer-backends)                                                            | Route to a peer operator's published OpenAI-compatible upstream with their credentials                                                              | `todo`  |

***

## What this version is

**v0.4** is the **ensemble roadmap** milestone referenced in [`chimera.plan.md`](chimera.plan.md): the point at which **two-phase ensemble**, **triggers**, **streaming error** semantics, and **external human escalation** (including paste-back and session behavior) are **specified and shippable** as a coherent product slice—not partial stubs.

The same train adds **operator-grade RAG housekeeping**: indexers produce embeddings stored **via** the gateway into the **vector database**; **workspace embedding scope (project + flavor)** defines ingestion keys and **base + flavor union** retrieval; operators can **target** a workspace (or indexer registration) for **purge / remove** so retired trees do not leave orphaned vectors. **Peer backends** enable cross-host upstream routing without gateway-to-gateway chaining. **Indexer Phase 7** (model-assisted indexing strategy) is scoped here as an optional operator-facing improvement on top of the shipped indexer baseline ([`plans/indexer.md`](plans/indexer.md) Phases 2–6). It also advances the **desktop (and any web)** shell toward **settings parity**: edit supported configuration **in UI**, with **search** to navigate dense settings and optionally the rest of the app.

For **`model: Chimera-<gateway_semver>`**, the gateway continues to own **routing policy** and the **fallback chain** (*Gateway turn orchestration*); **RAG** remains as in prior versions when enabled. **Per-turn dispatch** evaluates ensemble triggers anew each message (*Gateway runtime · 2*). **Fail-over / fail-fast** within the model chain and peers still apply; **no** gateway-side request queues (*Release roadmap · v0.8*).

**Companion docs:** [`chimera.plan.md`](chimera.plan.md) (requirements: *Ensemble orchestration*, *External human escalation*, *Responsibility split*, *Gateway runtime*, *Chat turn resilience and degradation*, *Workspace indexing and retrieval*, *Indexer live storage API*), [`configuration.md`](configuration.md), [`plans/indexer.md`](plans/indexer.md), [`plans/env-precedence-contract.md`](plans/env-precedence-contract.md) (unified env/config precedence), [`plans/desktop-ui.md`](plans/desktop-ui.md), [`plans/operator-cli.md`](plans/operator-cli.md) (operator CLI for gateway/BiFrost), [`plans/_template.md`](plans/_template.md).

***

## Two-phase ensemble

**Goal:** Ship a **two-phase ensemble**: **N** parallel draft completions, then a **critique/synthesize** phase that produces **one** consolidated answer for the client, with **N** bounded by **real upstream capacity** inferred from the catalog.

**Scope**

* **Parallel drafts** — Upstream runs **parallel** chat completions for the draft phase (*Ensemble orchestration · 1*, *Responsibility split · 1*).
* **Default and cap** — **Default `N` = 3**; cap **N** by **available** backends; **availability** from **upstream catalog introspection** (*Ensemble orchestration · 1*).
* **Critique/synthesize** — Second phase consumes draft outputs and yields a **single** user-visible result (*Ensemble orchestration · 1*, *Responsibility split · 2*).
* **Gateway vs upstream** — **Orchestration** (when phases run, critique/synthesize, escalation merge) in **gateway**; upstream executes the **parallel** draft calls (*Ensemble orchestration · 3*, *Responsibility split · 1–2*).
* **Operator docs** — Document ensemble configuration, defaults, and how **explicit** upstream model ids (**direct proxy**) interact with ensemble (**virtual** path only for triggers—see [Triggers and streaming semantics](#triggers-and-streaming-semantics)).

**Acceptance**

* With ensemble enabled and sufficient healthy upstream models, a **`chimera-<semver>`** turn can complete **draft → critique/synthesize** and return **one** answer; **N** does not exceed catalog-derived availability.
* Logs and metrics allow an operator to see **phase boundaries**, **draft count**, and **which backends** participated (within redaction rules in *Observability · 1*).

**Status:** `todo`

***

## Triggers and streaming semantics

**Goal:** Define **who** may enter an ensemble (**automatic** rules + **`//deep`**), confine it to the **virtual** orchestrated model, and nail **SSE/streaming** behavior—including **errors mid-ensemble**—so IDE clients behave predictably (*Compatibility · 1*, *Ensemble orchestration · 2*).

**Scope**

* **Triggers** — **Automatic** triggers plus manual **`//deep`** (trimmed from user text); **only** for **virtual `chimera-<semver>`**; gateway **may** strip `//deep` before forwarding to upstream (*Ensemble orchestration · 2*).
* **`N` in triggers** — Manual and automatic paths respect the **N** rules from [Two-phase ensemble](#two-phase-ensemble) (*Ensemble orchestration · 2*).
* **Streaming contract** — Specify how **streaming** proceeds across **draft** and **critique/synthesize** phases; specify **streaming error** semantics when a **phase** fails (partial streams, terminal events, client-visible error shape)—this milestone is where that contract becomes **authoritative** (*Ensemble orchestration* intro paragraph in plan).
* **Interaction with fallback** — On failure or **429**, behavior remains consistent with **sequential fallback** for the orchestrated path where applicable (*Gateway turn orchestration · 2*, *Chat turn resilience · 2*); document any **ensemble-specific** nuances (e.g. which phase retries or fails the turn).
* **Docs** — Configuration reference for triggers, `//deep`, and streaming expectations for Continue-like clients.

**Acceptance**

* Documented matrix: **non-streaming** and **streaming** paths for **success**, **draft-phase failure**, and **synthesize-phase failure**.
* Operators can reproduce a minimal **manual `//deep`** turn and an **automatic** trigger using documented config and sample prompts.

**Status:** `todo`

***

## External human escalation

**Goal:** When internal policy cannot be satisfied with acceptable confidence, the gateway can **escalate to a human outside the operator stack** via **copy/paste** workflows—not a vendor API—with **clear privacy**, **paste-back** recognition, and **non-blocking** continuation (*External human escalation · 1–6*).

**Scope**

* **Configurable surfaces** — One or more **name** + **URL** entries in configuration (*External human escalation · 1*).
* **Privacy disclosure** — Escalation responses **must** disclose that **task or context** may leave the operator stack (*External human escalation · 2*).
* **When to engage** — Only after **exhausted** internal attempts **and** **low confidence**; **thresholds configurable**; **concrete signals** productized in lockstep with ensemble work (*External human escalation · 3*).
* **Escalation payload** — Summarize failure; point to configured URLs; **single copy-paste prompt**; instructions for **paste-back delimiter** (*External human escalation · 4*).
* **Paste-back** — A later user message containing the delimiter is treated as an **external answer**, **merged**, and the conversation **continues** (*External human escalation · 5*).
* **No delimiter** — Treat as **normal chat**; **do not** block waiting for paste-back unless optional UX explicitly adds a wait (*External human escalation · 6*).
* **Session/state** — Paste-back and escalation state are **polished** and aligned with the ensemble milestone (plan: full productization with ensemble for signals and paste-back **session/state**).

**Acceptance**

* End-to-end documented path: trigger escalation → copy → external step → paste delimiter → merged continuation.
* **Privacy** copy is always present on escalation surfaces; **no** silent exfil narrative.

**Status:** `todo`

***

## Indexer manifest ingest (line metadata)

**Goal:** Operators see **file path and line ranges** (e.g. `L42–58`) on workspace snippets in chat, with a **line-number gutter** and **mid-line** `…` markers; the model receives the same ranges in injected context. The indexer **pre-chunks** every file and sends a **manifest**; the gateway embeds and stores spans in **Qdrant** and a **segment index** in operator SQLite for **revision coherence** and **expansion tools**.

**Execution plan:** [`plans/indexer-manifest-ingest.md`](plans/indexer-manifest-ingest.md) — eight phases from shared `chimera/internal/chunk` through re-index and ship. **No legacy ingest path** after ship.

**Scope**

* **Manifest-only ingest** — Indexer uploads chunk texts + line/byte metadata; gateway trusts manifest, rejects invalid files with bounded logging.
* **Shared chunk library** — `chimera/internal/chunk`; knobs from `GET /v1/indexer/config` only.
* **Retrieval + UI** — Extend `X-Chimera-RAG-Hits`, system context, conversation history, `/ui/chat` gutter rendering.
* **Revision coherence** — Staleness vs live file hash; warn/strict modes (toggle placement TBD in plan open questions).
* **Tooling** — Context-around, adjacent chunks, line read APIs; indexer vs gateway file-serving TBD.
* **Re-index** — Full re-index all workspaces on ship; coordinate with [`plans/indexer-sync-state-sqlite-and-force-reindex.md`](plans/indexer-sync-state-sqlite-and-force-reindex.md).

**Acceptance**

* After re-index, chat RAG hits show **L{start}–{end}** with gutter lines on a known repo file.
* Saved conversation history includes line fields on retrieval rows.
* Documented tool APIs resolve segment neighbors and line windows.

**Status:** `todo`

***

## Indexer workspace lifecycle and purge

**Goal:** Give operators a **supported**, **authenticated** path to **manage** indexer-associated corpus in the **RAG vector store**—including **removing or purging** a **specific workspace**—without requiring direct database consoles for routine cleanup.

**Scope**

* **Embeddings path** — Preserve the contract that **indexers** (and ingest callers) go through the **gateway** for **embeddings** and **vector writes** (*Workspace indexing and retrieval · 1*); chunking moves to the indexer manifest ([Indexer manifest ingest](#indexer-manifest-ingest-line-metadata)). This section adds **management** and **delete** semantics, not a bypass for writes.
* **Identify “workspace”** — Define the operator-visible handle (e.g. **registered indexer** + roots, or **`tenant_id` / `project_id` / `flavor_id`** triple consistent with *Workspace indexing · 8–10* and *Tenant authentication · 1–2*) used to scope **purge** and **list** actions.
* **Operations** — At minimum: **purge** (delete vectors/payload for that scope) and clarity on **stop/disable** a specific indexer instance if multiple indexers run; optional **dry-run** or **preview counts** if live storage APIs support it (*Observability · 2*).
* **Surface** — Gateway **REST** (preferred for parity with ingest/indexer config) and/or **desktop** action that calls the same backend; document **auth** (same gateway token model as ingest).
* **Safety** — Confirmations, irreversibility callouts, and docs for **collection naming** (*Workspace indexing · 7*) so operators know what disappears.

**Acceptance**

* After purge for workspace **W**, **retrieval** for **W** returns **no** prior chunks; other tenants/projects/flavors **unchanged** (integration or documented manual check).
* Operator runbook documents the **exact** API or UI path and required **headers** / **identity** fields.

**Status:** `todo`

***

## Indexer Phase 7 — model-assisted strategy

**Goal:** Give operators an **optional**, **gateway-mediated** way to obtain a **recommended indexing strategy** (ignore patterns, priorities, exclusions) from a **model** or structured endpoint—without embedding inside `chimera-indexer` and without replacing human review of what gets indexed.

**Execution plan:** [`plans/indexer.md`](plans/indexer.md) — **Phase 7 — Model-assisted strategy** (Phases 2–6 are **done**; Phase 7 is the remaining indexer plan item).

**Scope**

* **Inputs (conceptual)** — A **directory tree summary**, **effective ignore sets** (`.chimeraignore`, `.gitignore`, built-ins), and **current indexer / workspace config** (roots, project/flavor scope)—**no** raw file bodies required for the recommendation call unless a later design explicitly adds a bounded sample.
* **Output** — Actionable recommendations: suggested **globs**, **priority** hints, or **exclusion** patterns operators can **apply** to YAML or workspace settings (exact schema **TBD**).
* **Call path** — Prefer a **gateway** HTTP surface (authenticated like ingest/indexer REST) so policy, logging, and model routing stay in Chimera; a companion CLI or `/ui/settings` action may invoke the same API.
* **Non-goals** — **Automatic** application of model output without operator confirm; **replacing** Phase 2–6 ingest, watch, or reconciliation; **local** embedding or vector writes from the indexer binary.
* **Dependencies** — Stable workspace/indexer identity (tenant, project, flavor) and operator surfaces from prior gateway trains; may reuse virtual-model or tool-router infrastructure where it reduces duplicate LLM wiring—document the chosen path when implemented.

**Acceptance**

* Documented API or UI flow: operator triggers strategy assist → gateway returns structured recommendation → operator can **preview** and **accept or discard** changes to indexer config.
* Recommendations never bypass **relative `source`** rules or tenant scoping; secrets and absolute host paths are **not** sent in the assist payload.
* [`plans/indexer.md`](plans/indexer.md) Phase 7 checklist item marked **done** when the normative contract and at least one operator path ship.

**Status:** `todo`

***

## Configuration in the desktop or web UI

**Goal:** Operators can **change configuration** for supported knobs **inside** the **desktop or web** application, instead of being forced to **edit YAML or env files** for those knobs.

**Scope**

* **Coverage** — Start from high-impact, low-ambiguity settings (e.g. listen addresses where safe, feature toggles, non-secret routing labels); **secrets** may remain **file- or OS-secret** based until the security milestone—document what is **in-UI** vs **files only** (*Security · 1*, *Operator documentation · 5*).
* **Persistence** — Writes apply to the **authoritative** config layer the gateway/desktop already uses (**mtime reload**, restart semantics, or explicit “apply” with validation—pick one product story and document it).
* **Validation** — Schema-aware errors **before** save; no silent partial writes.
* **Parity** — [`configuration.md`](configuration.md) and examples stay aligned: every UI-editable key is documented with its **file** equivalent for automation and gitops-friendly operators.

**Acceptance**

* For at least one **documented** settings category, a user completes the full flow **without opening** the underlying YAML file.
* Invalid values are **rejected** in UI with the same constraints the gateway would enforce at load time.

**Status:** `todo`

***

## Settings and application search

**Goal:** As settings and screens grow, operators can **find** options quickly via **search** on the **settings / configuration** experience and/or **across the whole** desktop or web application.

**Scope**

* **Settings search** — Filter settings **labels**, **descriptions**, and **section** titles (and optionally **current values** where not secret) to jump to the right control.
* **Global search (optional scope)** — If shipped in v0.4, define breadth: e.g. settings + **navigation** destinations + **log view** filters; **out** if deferred—then this section is **settings-only** and global search moves to a later version (call out in **Explicitly not** or **Status** above).
* **Keyboard / UX** — Sensible focus order and shortcut if the platform supports it (document in [`plans/desktop-ui.md`](plans/desktop-ui.md) when implemented).

**Acceptance**

* Typing a known setting name in the settings search **surfaces** that control within **one** interaction from results.
* If global search is in scope: a second documented query finds a **non-settings** screen (e.g. logs or indexer status) from one entry point.

**Status:** `todo`

***

## Workspace embedding scope (project + flavor)

**Goal:** When files are embedded, ingest them under a **unique key** derived from **user** (the authenticated operator / tenant identity used for isolation today) **+ project + flavor**. Operators can start with a **broad corpus** (e.g. all journal files) on a **project id** alone, then add **flavors** for private areas or specializations without re-copying that base corpus into every flavor.

**Scope**

### Ingestion identity

* Each indexed chunk (or equivalent vector payload) is associated with a **workspace scope** that includes:
  * **User** — the same identity that gates auth and multi-tenant separation (exact field name in config/API may follow existing gateway/indexer conventions).
  * **Project** — required **project id** for the workspace.
  * **Flavor** — optional **flavor id**; **absent** or **empty** means this workspace is the **base / global** embeddings bucket for that **user + project**.

### Base (project-only) vs flavored workspaces

* A workspace defined with **project id** and **no flavor id** is the **base** embeddings set for that project: its vectors participate in **every** retrieval for that **user + project**, regardless of which **flavor id** (if any) the client sends on a given request.
* A workspace defined with the **same project id** and a **non-empty flavor id** holds **additional** embeddings scoped to that flavor only (in addition to base, not instead of replacing base unless product explicitly adds a “replace base” mode—**v0.4 default:** additive union as below).

### Retrieval union (flavored request)

**Requirement**

Given a user has defined a workspace with a **project id** but **no flavor id**.\
And the contents of that workspace have been **indexed**.\
And the user defines a **new** workspace with the **same project id** and a **flavor id** set.\
And the contents of that workspace have been **indexed**.\
When the user performs a **chat** (or RAG) request scoped to that **project id** and the **flavor id**,\
Then retrieval must query embeddings from **both**:

* the workspace scoped to **project** with **no flavor** (base / global for that project); and
* the workspace scoped to **project + flavor**.

**Multi-workspace request pool**

* The client may specify **any number** of workspace selectors **(project + optional flavor)** on a request (exact header or body shape belongs in API design; align with the `X-Chimera-*` header contract in [`version-v0.3.md`](version-v0.3.md#product-naming)).
* **All valid** declared workspaces for that authenticated user are included in the **embedding search pool** for that request (union of hits across those scopes, with deduplication by chunk identity where the same file appears under multiple selectors only if product allows—**v0.4 default:** dedupe by stable chunk id / source key so overlapping paths do not double-count unless intentionally indexed twice).
* **Invalid** or unknown workspace references should fail **loudly** (clear error to the client) or be **ignored** with a warning in logs—pick one behavior in implementation and document it; do not silently drop **all** scopes.

### Operator story

* **Natural flow:** index “everything I want everywhere” under **project only**; add **flavored** folders or repos later for **sensitive** or **topic-specific** material; flavored chats automatically see **shared baseline** plus **flavor overlay**.
* **Docs:** Update [`plans/indexer.md`](plans/indexer.md) and [`configuration.md`](configuration.md) when fields for project/flavor per index and per-request workspace lists are fixed.

### Relationship to the v0.3 setup wizard

* [`version-v0.3.md`](version-v0.3.md) setup wizard step 5 (indexing setup) collects basic **project** and optional **flavor** per index; step 6 (test indexing) should run retrieval using the **same union rules** as production once this theme ships so operators validate behavior before leaving the wizard.

**Deliverables checklist**

* Indexer / gateway: persist and filter vectors by **(user, project, flavor?)** consistently on ingest and search.
* Chat (RAG) path: implement **base + flavor** union when a flavor is present; implement **multi-workspace** union when multiple selectors are provided.
* Operator docs: examples for “journal base + `private` flavor” and multi-selector requests.

**Acceptance**

* The **Given / When / Then** requirement above holds in automated or manual acceptance tests for at least one reference project.
* Multi-selector requests include every **valid** workspace in the search pool; documented behavior for invalid selectors.

**Status:** `todo`

***

## Peer backends

**Goal:** Let one operator route to another operator's published OpenAI-compatible upstream without chaining gateway-to-gateway.

**Scope**

This theme summarizes what [`chimera.plan.md`](chimera.plan.md) assigns to **v0.4** so implementation and docs stay aligned.

### Release-roadmap slice

From the master **Release roadmap** table:

* **Peer-to-peer model backends**: call **another operator’s BiFrost** (or compatible OpenAI proxy) over a **host-routable** URL and **published** port (not Compose-internal DNS from another machine).
* **Proxy-issued credentials** (e.g. virtual keys where the upstream supports them) for **cross-host** authentication.
* **Gateway / upstream configuration** and **operator documentation** for peer paths: *Peer topology · 1–3*, *Model selection and routing policy · 3* (peer as `base_url` / `api_base`), and *Deployment · 3* (cross-host publishing vs intra-stack DNS)—see [`chimera.plan.md`](chimera.plan.md).
* **Per-key / usage observability** (*Resilience · 1*): track which key/backend was used and exposure to RPM/TPM-style limits where upstream headers exist.

### Product rules

* **Peer = their upstream (BiFrost / compatible proxy), not their Gateway** (*Peer topology · 2*): configure OpenAI-compatible `api_base` / `base_url` to the **peer’s published upstream** (e.g. Tailscale/LAN IP + **published** proxy port + `/v1`). Use credentials **they** issue (virtual keys or equivalent when supported). Do **not** chain **Gateway → peer Gateway** as the default integration (same bullet).
* **Independent stacks** (*Peer topology · 1*): each operator has their own Chimera instance, client-auth secrets (`api-keys.yaml`), and policy; no assumption that one gateway “owns” another’s RAG.
* **Document ports per host** (*Peer topology · 3*): distinguish **Chimera** (IDE-facing OpenAI-compatible entry) vs **peer upstream** (`api_base` / `base_url` target); firewall/VPN expectations; TLS/mTLS deferred to **v0.7** unless operators add their own terminator.
* **Cloud vs local policy** (*Model selection and routing policy · 3*): **Peer upstream** appears as a **remote-runner** entry in routing policy.
* **Graceful degradation** (*Resilience · 2*): same fail-over / fail-fast behavior when a peer upstream is in the chain; **no** gateway queue until **v0.8**.
* **Containers / networking**: from **v0.4**, compose/docs consider **LAN peer access** to published upstreams where enabled; TLS posture for peer URLs ships with **v0.7** (*Security · 2–5*).

### Deliverables checklist

* Configuration surfaces (and/or files) to add **peer upstream** backends with proxy-issued credentials where applicable and host-reachable base URLs.
* Operator docs: cross-host topology, **published** ports, virtual keys, anti-patterns (Compose hostname of peer stack, gateway-on-gateway).
* **Observability (*Resilience · 1*)**: per-key / per-backend usage signals where APIs expose limits or identifiers.

**Acceptance**

* Peer upstream configuration can target a host-routable OpenAI-compatible proxy URL with credentials issued by the peer operator.
* Operator docs explain ports, network expectations, credential handoff, and the gateway-on-gateway anti-pattern.
* Per-key or per-backend usage signals are visible where upstream APIs expose enough data.

**Status:** `todo`

***

## Explicitly not this version

* Do not route Gateway -> peer Gateway as the default peer integration; peer routes target a host-routable upstream proxy.
* Do not make TLS/mTLS or untrusted-network hardening a v0.4 requirement; that remains a later hardening release.
* Do not add a gateway queue or priority scheduler; graceful degradation remains the v0.4 behavior.

***

## Verification

| Area                      | Quick check                                                                                                                                                                                                                  |
|---------------------------|------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| Two-phase ensemble        | Configured **`chimera-<semver>`** turn runs draft + synthesize; **N** matches catalog cap; structured logs show phases                                                                                                       |
| Triggers / streaming      | **`//deep`** on virtual model triggers ensemble; streaming client receives spec-compliant events on success and injected draft-phase failure                                                                                 |
| External human escalation | Forced low-confidence path produces escalation body with **privacy** line + delimiter docs; paste-back merges; no paste does not hang the session                                                                            |
| Workspace embedding scope | Ingestion keys `(user, project, flavor?)`; flavored chat unions base + flavor; multi-workspace requests pool all valid scopes; v0.3 wizard step 5–6 matches production                                                       |
| Indexer / purge           | Purge API or UI for workspace **W** clears vectors for **W** only; `GET` storage stats / inventory reflect drop (*Observability · 2*)                                                                                        |
| Indexer Phase 7           | Documented assist flow returns strategy JSON (or equivalent); operator can apply or reject without auto-mutating config                                                                                                      |
| In-app configuration      | Edit a documented setting in UI; gateway (or supervised stack) reflects it per documented reload/restart rules                                                                                                               |
| Settings / app search     | Settings search finds a documented control by partial name; if global search is in train, second scenario from **Acceptance**                                                                                                |
| Peer backends             | Peer upstream + credentials + docs meet the peer scope checklist                                                                                                                                                             |
| Docs/config               | [`configuration.md`](configuration.md) and examples list ensemble, escalation, workspace scope, purge, peer backends, and UI-editable keys; cross-links from [`chimera.plan.md`](chimera.plan.md) release row when published |
| Tests                     | Unit/integration coverage for phase scheduling, delimiter parsing, streaming error branches, purge scoping, and settings validation per repo conventions                                                                     |

***

## See also

* [`version-v0.3.md`](version-v0.3.md) - previous version (onboarding, virtual models, setup wizard)
* [`releases-v0.4.x.md`](releases-v0.4.x.md) - patch release notes, once this train ships patches
* [`plans/indexer.md`](plans/indexer.md) - `chimera-indexer` plan (Phase 7 model-assisted strategy scoped to this release)
* [`plans/operator-cli.md`](plans/operator-cli.md) - `chimera` operator CLI (config, health, models, chat smoke tests)
* [`plans/_template.md`](plans/_template.md) - phase-level plan template for implementation breakdowns
