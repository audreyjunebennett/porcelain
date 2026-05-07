# Version 0.4 - Ensembles, RAG workspace lifecycle, and in-app settings

| Field | Value |
|-------|-------|
| **Doc kind** | `version-roadmap` |
| **Owners / areas** | Gateway, BiFrost/upstream contracts, indexer/RAG, operator docs, desktop and web UI (settings, search, paste-back UX) |
| **Status** | `draft` |
| **Targets** | Gateway v0.4 |
| **Last updated** | See git history |
| **Supersedes / superseded by** | Builds on [`version-v0.3.md`](version-v0.3.md) |

## At a glance

**v0.4** delivers **ensemble** (“heavy thinking”): the gateway orchestrates **parallel drafts**, optional **critique/synthesize**, and clear **streaming error** behavior when a phase fails—without pushing queueing or security-milestone work forward. It also **productizes external human escalation**: configurable surfaces, privacy disclosure, confidence-based engagement, and **paste-back / session** handling so operators can safely involve a human outside the stack when policy demands it.

On the **RAG** side, indexers continue to send content through the gateway so **embeddings land in the vector store**; operators gain **lifecycle controls** to **remove or purge** corpus tied to a **specific workspace** (scoped indexer / tenant–project–flavor), without ad-hoc Qdrant surgery. The **desktop or web** shell gains **first-class settings**: change configuration **in the app** (not only by editing YAML on disk), plus **search** on the settings surface and/or **across the application** so options are discoverable as surface area grows.

| Focus | Outcome | Status |
|-------|---------|--------|
| [Two-phase ensemble](#two-phase-ensemble) | **N** parallel drafts then critique/synthesize → one answer; **default N = 3**; **N** capped by **available** backends from **upstream catalog introspection**; orchestration in **gateway**, **parallel completions** executed by upstream (*Responsibility split · 1–2*) | `todo` |
| [Triggers and streaming semantics](#triggers-and-streaming-semantics) | **Automatic** ensemble triggers plus manual **`//deep`** (trimmed) on **virtual `Claudia-<semver>`** only; gateway **may** strip `//deep` before upstream; **critique/synthesize** and **streaming** behavior on ensemble phase failure **fully specified** here (*Ensemble orchestration · 1–3*) | `todo` |
| [External human escalation](#external-human-escalation) | Configurable **name + URL** surfaces, mandatory **privacy disclosure**, engagement only when internal attempts are **exhausted** and **low confidence** (thresholds configurable), **single copy-paste** escalation prompt with **paste-back delimiter**, merge on delimiter, **no blocking wait** without explicit UX (*External human escalation · 1–6*; signals **3** aligned with ensemble milestone) | `todo` |
| [Indexer workspace lifecycle and purge](#indexer-workspace-lifecycle-and-purge) | Operator-visible way to **manage** indexers and **remove or purge** vectors for a **specific workspace** (aligned collection / tenant scope) while embeddings remain gateway-mediated into the RAG DB | `todo` |
| [Configuration in the desktop or web UI](#configuration-in-the-desktop-or-web-ui) | Change **runtime-relevant** settings through the **desktop or web** interface; file-based config remains source of truth or sync target as designed—**no** requirement to hand-edit YAML for supported knobs | `todo` |
| [Settings and application search](#settings-and-application-search) | **Search** on the settings/configuration experience and/or **global in-app search** so operators find pages and values quickly | `todo` |

---

## What this version is

**v0.4** is the **ensemble roadmap** milestone referenced in [`claudia-gateway.plan.md`](claudia-gateway.plan.md): the point at which **two-phase ensemble**, **triggers**, **streaming error** semantics, and **external human escalation** (including paste-back and session behavior) are **specified and shippable** as a coherent product slice—not partial stubs.

The same train adds **operator-grade RAG housekeeping**: indexers produce embeddings stored **via** the gateway into the **vector database**; operators can **target** a workspace (or indexer registration) for **purge / remove** so retired trees do not leave orphaned vectors. It also advances the **desktop (and any web)** shell toward **settings parity**: edit supported configuration **in UI**, with **search** to navigate dense settings and optionally the rest of the app.

For **`model: Claudia-<gateway_semver>`**, the gateway continues to own **routing policy** and the **fallback chain** (*Gateway turn orchestration*); **RAG** remains as in prior versions when enabled. **Per-turn dispatch** evaluates ensemble triggers anew each message (*Gateway runtime · 2*). **Fail-over / fail-fast** within the model chain and peers still apply; **no** gateway-side request queues (*Release roadmap · v0.8*).

**Companion docs:** [`claudia-gateway.plan.md`](claudia-gateway.plan.md) (requirements: *Ensemble orchestration*, *External human escalation*, *Responsibility split*, *Gateway runtime*, *Chat turn resilience and degradation*, *Workspace indexing and retrieval*, *Indexer live storage API*), [`configuration.md`](configuration.md), [`plans/indexer.md`](plans/indexer.md), [`plans/desktop-ui.md`](plans/desktop-ui.md), [`plans/_template.md`](plans/_template.md).

---

## Two-phase ensemble

**Goal:** Ship a **two-phase ensemble**: **N** parallel draft completions, then a **critique/synthesize** phase that produces **one** consolidated answer for the client, with **N** bounded by **real upstream capacity** inferred from the catalog.

**Scope**

- **Parallel drafts** — Upstream runs **parallel** chat completions for the draft phase (*Ensemble orchestration · 1*, *Responsibility split · 1*).
- **Default and cap** — **Default `N` = 3**; cap **N** by **available** backends; **availability** from **upstream catalog introspection** (*Ensemble orchestration · 1*).
- **Critique/synthesize** — Second phase consumes draft outputs and yields a **single** user-visible result (*Ensemble orchestration · 1*, *Responsibility split · 2*).
- **Gateway vs upstream** — **Orchestration** (when phases run, critique/synthesize, escalation merge) in **gateway**; upstream executes the **parallel** draft calls (*Ensemble orchestration · 3*, *Responsibility split · 1–2*).
- **Operator docs** — Document ensemble configuration, defaults, and how **explicit** upstream model ids (**direct proxy**) interact with ensemble (**virtual** path only for triggers—see [Triggers and streaming semantics](#triggers-and-streaming-semantics)).

**Acceptance**

- With ensemble enabled and sufficient healthy upstream models, a **`Claudia-<semver>`** turn can complete **draft → critique/synthesize** and return **one** answer; **N** does not exceed catalog-derived availability.
- Logs and metrics allow an operator to see **phase boundaries**, **draft count**, and **which backends** participated (within redaction rules in *Observability · 1*).

**Status:** `todo`

---

## Triggers and streaming semantics

**Goal:** Define **who** may enter an ensemble (**automatic** rules + **`//deep`**), confine it to the **virtual** orchestrated model, and nail **SSE/streaming** behavior—including **errors mid-ensemble**—so IDE clients behave predictably (*Compatibility · 1*, *Ensemble orchestration · 2*).

**Scope**

- **Triggers** — **Automatic** triggers plus manual **`//deep`** (trimmed from user text); **only** for **virtual `Claudia-<semver>`**; gateway **may** strip `//deep` before forwarding to upstream (*Ensemble orchestration · 2*).
- **`N` in triggers** — Manual and automatic paths respect the **N** rules from [Two-phase ensemble](#two-phase-ensemble) (*Ensemble orchestration · 2*).
- **Streaming contract** — Specify how **streaming** proceeds across **draft** and **critique/synthesize** phases; specify **streaming error** semantics when a **phase** fails (partial streams, terminal events, client-visible error shape)—this milestone is where that contract becomes **authoritative** (*Ensemble orchestration* intro paragraph in plan).
- **Interaction with fallback** — On failure or **429**, behavior remains consistent with **sequential fallback** for the orchestrated path where applicable (*Gateway turn orchestration · 2*, *Chat turn resilience · 2*); document any **ensemble-specific** nuances (e.g. which phase retries or fails the turn).
- **Docs** — Configuration reference for triggers, `//deep`, and streaming expectations for Continue-like clients.

**Acceptance**

- Documented matrix: **non-streaming** and **streaming** paths for **success**, **draft-phase failure**, and **synthesize-phase failure**.
- Operators can reproduce a minimal **manual `//deep`** turn and an **automatic** trigger using documented config and sample prompts.

**Status:** `todo`

---

## External human escalation

**Goal:** When internal policy cannot be satisfied with acceptable confidence, the gateway can **escalate to a human outside the operator stack** via **copy/paste** workflows—not a vendor API—with **clear privacy**, **paste-back** recognition, and **non-blocking** continuation (*External human escalation · 1–6*).

**Scope**

- **Configurable surfaces** — One or more **name** + **URL** entries in configuration (*External human escalation · 1*).
- **Privacy disclosure** — Escalation responses **must** disclose that **task or context** may leave the operator stack (*External human escalation · 2*).
- **When to engage** — Only after **exhausted** internal attempts **and** **low confidence**; **thresholds configurable**; **concrete signals** productized in lockstep with ensemble work (*External human escalation · 3*).
- **Escalation payload** — Summarize failure; point to configured URLs; **single copy-paste prompt**; instructions for **paste-back delimiter** (*External human escalation · 4*).
- **Paste-back** — A later user message containing the delimiter is treated as an **external answer**, **merged**, and the conversation **continues** (*External human escalation · 5*).
- **No delimiter** — Treat as **normal chat**; **do not** block waiting for paste-back unless optional UX explicitly adds a wait (*External human escalation · 6*).
- **Session/state** — Paste-back and escalation state are **polished** and aligned with the ensemble milestone (plan: full productization with ensemble for signals and paste-back **session/state**).

**Acceptance**

- End-to-end documented path: trigger escalation → copy → external step → paste delimiter → merged continuation.
- **Privacy** copy is always present on escalation surfaces; **no** silent exfil narrative.

**Status:** `todo`

---

## Indexer workspace lifecycle and purge

**Goal:** Give operators a **supported**, **authenticated** path to **manage** indexer-associated corpus in the **RAG vector store**—including **removing or purging** a **specific workspace**—without requiring direct database consoles for routine cleanup.

**Scope**

- **Embeddings path** — Preserve the contract that **indexers** (and ingest callers) go through the **gateway** for chunking/embeddings and **vector writes** (*Workspace indexing and retrieval · 1*); this section adds **management** and **delete** semantics, not a bypass for writes.
- **Identify “workspace”** — Define the operator-visible handle (e.g. **registered indexer** + roots, or **`tenant_id` / `project_id` / `flavor_id`** triple consistent with *Workspace indexing · 8–10* and *Tenant authentication · 1–2*) used to scope **purge** and **list** actions.
- **Operations** — At minimum: **purge** (delete vectors/payload for that scope) and clarity on **stop/disable** a specific indexer instance if multiple indexers run; optional **dry-run** or **preview counts** if live storage APIs support it (*Observability · 2*).
- **Surface** — Gateway **REST** (preferred for parity with ingest/indexer config) and/or **desktop** action that calls the same backend; document **auth** (same gateway token model as ingest).
- **Safety** — Confirmations, irreversibility callouts, and docs for **collection naming** (*Workspace indexing · 7*) so operators know what disappears.

**Acceptance**

- After purge for workspace **W**, **retrieval** for **W** returns **no** prior chunks; other tenants/projects/flavors **unchanged** (integration or documented manual check).
- Operator runbook documents the **exact** API or UI path and required **headers** / **identity** fields.

**Status:** `todo`

---

## Configuration in the desktop or web UI

**Goal:** Operators can **change configuration** for supported knobs **inside** the **desktop or web** application, instead of being forced to **edit YAML or env files** for those knobs.

**Scope**

- **Coverage** — Start from high-impact, low-ambiguity settings (e.g. listen addresses where safe, feature toggles, non-secret routing labels); **secrets** may remain **file- or OS-secret** based until the security milestone—document what is **in-UI** vs **files only** (*Security · 1*, *Operator documentation · 5*).
- **Persistence** — Writes apply to the **authoritative** config layer the gateway/desktop already uses (**mtime reload**, restart semantics, or explicit “apply” with validation—pick one product story and document it).
- **Validation** — Schema-aware errors **before** save; no silent partial writes.
- **Parity** — [`configuration.md`](configuration.md) and examples stay aligned: every UI-editable key is documented with its **file** equivalent for automation and gitops-friendly operators.

**Acceptance**

- For at least one **documented** settings category, a user completes the full flow **without opening** the underlying YAML file.
- Invalid values are **rejected** in UI with the same constraints the gateway would enforce at load time.

**Status:** `todo`

---

## Settings and application search

**Goal:** As settings and screens grow, operators can **find** options quickly via **search** on the **settings / configuration** experience and/or **across the whole** desktop or web application.

**Scope**

- **Settings search** — Filter settings **labels**, **descriptions**, and **section** titles (and optionally **current values** where not secret) to jump to the right control.
- **Global search (optional scope)** — If shipped in v0.4, define breadth: e.g. settings + **navigation** destinations + **log view** filters; **out** if deferred—then this section is **settings-only** and global search moves to a later version (call out in **Explicitly not** or **Status** above).
- **Keyboard / UX** — Sensible focus order and shortcut if the platform supports it (document in [`plans/desktop-ui.md`](plans/desktop-ui.md) when implemented).

**Acceptance**

- Typing a known setting name in the settings search **surfaces** that control within **one** interaction from results.
- If global search is in scope: a second documented query finds a **non-settings** screen (e.g. logs or indexer status) from one entry point.

**Status:** `todo`

---

## Explicitly not this version

- **Gateway MCP** or unified gateway tool surface (*Release roadmap · v0.5*, *MCP boundaries · 1*).
- **Conversation archive ingestion** automated pipeline (*Workspace indexing and retrieval · 14*, *Release roadmap · v0.5*).
- **Security & TLS** milestone: encryption posture, **`/health` hardening**, rate limiting, audit/redaction, threat model (*Release roadmap · v0.7*, *Security · 2–5*).
- **Queues** and priority scheduling for busy backends (*Release roadmap · v0.8*, *Chat turn resilience · 2*).
- Using **LLM-as-MCP-tool** for primary routing (*MCP boundaries · 3*) or **optional routing judge** as the default selection path (*Routing mechanics · 2*)—out of scope unless explicitly pulled into a later plan.
- **Full** secrets management entirely in UI without file/OS backing—unless explicitly brought into v0.4 under *Security · 1*; default assumption is **defer** sensitive material to files or later security work.

---

## Verification

| Area | Quick check |
|------|-------------|
| Two-phase ensemble | Configured **`Claudia-<semver>`** turn runs draft + synthesize; **N** matches catalog cap; structured logs show phases |
| Triggers / streaming | **`//deep`** on virtual model triggers ensemble; streaming client receives spec-compliant events on success and injected draft-phase failure |
| External human escalation | Forced low-confidence path produces escalation body with **privacy** line + delimiter docs; paste-back merges; no paste does not hang the session |
| Indexer / purge | Purge API or UI for workspace **W** clears vectors for **W** only; `GET` storage stats / inventory reflect drop (*Observability · 2*) |
| In-app configuration | Edit a documented setting in UI; gateway (or supervised stack) reflects it per documented reload/restart rules |
| Settings / app search | Settings search finds a documented control by partial name; if global search is in train, second scenario from **Acceptance** |
| Docs/config | [`configuration.md`](configuration.md) and examples list ensemble, escalation, purge, and UI-editable keys; cross-links from [`claudia-gateway.plan.md`](claudia-gateway.plan.md) release row when published |
| Tests | Unit/integration coverage for phase scheduling, delimiter parsing, streaming error branches, purge scoping, and settings validation per repo conventions |

---

## See also

- [`version-v0.3.md`](version-v0.3.md) - previous version (peer backends, onboarding narrative)
- [`releases-v0.4.x.md`](releases-v0.4.x.md) - patch release notes, once this train ships patches
- [`plans/_template.md`](plans/_template.md) - phase-level plan template for implementation breakdowns
