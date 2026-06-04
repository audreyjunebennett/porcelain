# Version 0.5 - Operator desired-state gateway and model-assisted configuration

| Field                          | Value                                                                                          |
|--------------------------------|------------------------------------------------------------------------------------------------|
| **Doc kind**                   | `version-roadmap`                                                                              |
| **Owners / areas**             | Gateway, operator UI, admin API, observability, routing/indexer configuration                    |
| **Status**                     | `draft`                                                                                        |
| **Targets**                    | Gateway v0.5                                                                                   |
| **Last updated**               | See git history                                                                                |
| **Supersedes / superseded by** | Builds on [`version-v0.4.md`](version-v0.4.md)                                                 |

## At a glance

**v0.5** introduces a gateway that can **return itself to the configuration the operator wants** — not only by editing fields manually, but by understanding **what each operator surface is for** and applying changes through documented APIs. Every major page (or card) exposes or generates a **textual self-description**: purpose, current state summary, relevant URLs, and an **AGENTS.md-style** contract for which `/api/ui/*` and `/v1/*` endpoints read or write that state. From any such screen, the operator can open a **model-assisted help panel**, describe the problem or desired outcome in natural language, and have the gateway propose or apply configuration changes through those APIs.

The same train ships **actionable operator alerts** built from recurring upstream/model failures (complementing v0.3’s lightweight catalog validation), and optional **richer provider/model validation** when the operator explicitly requests it — without making live probes a first-run gate.

| Focus                                                                                              | Outcome                                                                                                                                 | Status  |
|----------------------------------------------------------------------------------------------------|-----------------------------------------------------------------------------------------------------------------------------------------|---------|
| [Operator desired-state gateway](#operator-desired-state-gateway)                                  | Gateway reconciles runtime toward operator-declared intent; safe apply/rollback patterns for supervised stack                            | `todo`  |
| [Page self-description and operator API guides](#page-self-description-and-operator-api-guides)    | Each operator surface publishes machine- and human-readable “how to configure me” text with URLs and API shapes                         | `todo`  |
| [Model-assisted configuration](#model-assisted-configuration)                                      | In-context prompt → model reads page guide + live state → proposes or applies fixes via authenticated APIs                              | `todo`  |
| [Provider and model error alerts](#provider-and-model-error-alerts)                                | Aggregate structured log failures into actionable alerts (keys, availability, fallback regen)                                           | `todo`  |
| [Optional provider/model probe](#optional-providermodel-probe)                                     | Explicit operator-triggered validation beyond catalog health; not required for setup wizard                                             | `todo`  |
| [plans/indexer-embedding-model-and-workspace-purge.md](plans/indexer-embedding-model-and-workspace-purge.md) | Operator-selectable embedding model on indexer card; workspace delete drops vector collection                                           | `todo`  |
| [plans/operator-workspace-search.md](plans/operator-workspace-search.md)                           | Direct workspace search in app shell (not chat-only RAG)                                                                                | `todo`  |
| [plans/embedui-settings-card-cleanup.md](plans/embedui-settings-card-cleanup.md)                 | Settings feed and card component consistency refactor                                                                                   | `todo`  |

***

## What this version is

**v0.5** is the **operator autonomy** milestone: Chimera should help operators **recover from misconfiguration** and **reach a known-good state** without reading every YAML file or spelunking logs. The gateway already skips failing models and logs routing decisions; v0.5 **closes the loop** by turning repeated failures into **alerts with next steps**, and by letting a model **read what a page means** and **call the same APIs** the settings UI uses.

This builds on v0.4’s in-app configuration, workspace lifecycle, and search themes. v0.5 does **not** replace human review for destructive actions (purge, key rotation, embedding model changes) — confirmations and audit logs remain required.

**Companion docs:** [`chimera.plan.md`](chimera.plan.md), [`configuration.md`](configuration.md), [`version-v0.3.md`](version-v0.3.md) (setup wizard + deferred validation), [`version-v0.4.md`](version-v0.4.md), [`plans/_template.md`](plans/_template.md).

Authoritative **architecture and numbered requirements** remain in [`chimera.plan.md`](chimera.plan.md) unless this plan explicitly revises them.

***

## Operator desired-state gateway

**Goal:** The gateway **knows how to return itself** (and supervised children where applicable) to the **operator’s desired configuration** — a declarative or UI-persisted intent — using the same mutation paths as `/ui/settings`, not ad-hoc side channels.

**Scope**

### Desired state vs live state

- **Desired state** — What the operator configured: provider keys, model availability, virtual model stacks, workspace rows, embedding model id, routing toggles, etc. (sources: operator SQLite, `gateway.yaml`, broker management API, `api-keys.yaml` as applicable).
- **Live state** — What the running stack actually reflects after reload/sync, catalog polls, and child process health.
- **Drift** — Documented differences (e.g. fallback chain references unavailable model, embedding model absent from catalog, indexer watching paths that fail materialize).

### Reconciliation behaviors

- **Detect** — Periodic or on-demand comparison of desired vs live (extend catalog auditors, provider health strip, VM `fallback_unavailable` hints).
- **Report** — Operator-visible summary per domain (providers, RAG/indexer, virtual models) with links to the screen that owns the fix.
- **Remediate (guided)** — Suggest or execute **safe** fixes: regenerate VM routing from catalog, re-sync broker after key save, reload gateway config after YAML patch — always through existing authenticated APIs.
- **Remediate (model-assisted)** — Optional path where a model proposes a remediation plan from page self-description + drift report; operator confirms before apply (see [Model-assisted configuration](#model-assisted-configuration)).
- **Non-goals for v0.5** — Fully autonomous self-healing without confirmation; cross-host peer reconciliation; automatic corpus re-embed without explicit operator ack.

**Acceptance**

- Documented flow: operator introduces intentional drift (e.g. disable all Groq models) → gateway surfaces drift → guided fix restores chat for a virtual model without manual YAML editing.
- Reconciliation actions are logged with stable slugs and `principal_id` when applied via UI or model-assist.

**Status:** `todo`

***

## Page self-description and operator API guides

**Goal:** Any operator page (or settings card) that mutates gateway behavior can produce a **textual description of itself** suitable for humans and for models — analogous to an **AGENTS.md** scoped to that surface.

**Scope**

### Content of a page guide

Each guide should include, at minimum:

- **Purpose** — What the operator is configuring and what breaks if it is wrong.
- **Current state snapshot** — Non-secret summary (counts, ids, enabled flags, health labels) fetched from existing `/api/ui/state` or scoped GET endpoints.
- **URLs** — Deep links to the operator page (`/ui/settings#…`, future `/ui/search`, wizard steps).
- **API contract** — For each read/write the UI uses: method, path, request body shape, success/error semantics, and **idempotency** notes.
- **Safety** — Destructive or expensive actions (purge, embedding model change, delete workspace) called out with required confirmations.

### Delivery mechanisms (pick one primary, others optional)

- **Static template + live interpolation** — Card module exports a `describePage(ctx)` string; gateway endpoint aggregates guides on demand.
- **`GET /api/ui/pages/{page_id}/guide`** — Session-authenticated JSON `{ markdown, apis[], state_snapshot }`.
- **Embedded help drawer** — Same markdown rendered in UI; copy button for external agents.

### Coverage priority

1. Provider cards (keys, availability, health).
2. Virtual model cards (fallback, routing, generate).
3. Indexer / workspace cards (paths, embedding model, purge).
4. Users / tokens, chimera-broker service card.
5. Chat and workspace search surfaces.

**Acceptance**

- At least **three** settings card types publish a guide that lists correct `/api/ui/*` endpoints verified against handlers.
- A guide for the provider card includes chimera-broker health interpretation consistent with [`operator-provider-model-availability.md`](features/operator-provider-model-availability.md).

**Status:** `todo`

***

## Model-assisted configuration

**Goal:** From any screen with a page guide, the operator can **prompt a model** to help diagnose or fix configuration — using the guide as tool context and authenticated APIs as the only mutation path.

**Scope**

### Operator UX

- **Help entry point** — Button or command palette on settings (and later chat/search): “Ask Chimera to fix this…”
- **Context bundle** — Page guide markdown + current state snapshot + recent scoped event log lines (redacted).
- **Prompt** — Operator describes issue or desired end state in natural language.
- **Model response** — Explanation, proposed API sequence, or diff-style summary — **never** raw secrets in the model transcript stored for history unless operator opts in.
- **Apply** — Operator confirms each mutation (or batch with explicit “Apply all”); gateway executes via existing handlers; results refresh the card.

### Gateway responsibilities

- **Virtual model routing** — Assist calls use a dedicated VM or scoped upstream model with low temperature; tool use limited to documented operator APIs (no arbitrary shell).
- **Auth** — Same UI session as the operator; model cannot escalate privilege.
- **Audit** — Log `operator.model_assist.proposed` and `operator.model_assist.applied` with page id and API slugs.

### Relationship to desired-state reconciliation

- Model-assist is the **interactive** face of desired-state recovery; reconciliation jobs are the **background** face. Both must call the same APIs.

**Acceptance**

- Documented demo: broken provider key → operator opens provider card assist → model identifies `broker.provider.health.fail` pattern → proposes key re-entry or availability change → operator confirms → health returns `up`.
- Model-assist cannot write files outside documented API surfaces in v0.5.

**Status:** `todo`

***

## Provider and model error alerts

**Goal:** When providers are in active use and **multiple model-level errors** appear in structured logs, surface **actionable alerts** instead of requiring operators to read the full event log.

**Scope**

### Signal sources

- Routing fallback skip lines (unavailable, quota, context, upstream HTTP errors).
- `broker.provider.health.fail`, `broker.provider.model_discovery.fail`.
- Virtual model `fallback_unavailable` hints from VM detail API.
- RAG/indexer errors tied to embedding model or collection health.

### Alert object (conceptual)

- **Severity**, **title**, **summary**, **affected provider/model ids**, **suggested actions** (links to settings card + API guide anchors), **first_seen / last_seen**, **count**.
- Dismiss/snooze per operator session or persisted per tenant.

### UI surfaces

- Settings header or ribbon badge.
- Scoped alert strip on the relevant provider or VM card.
- Optional feed in model-assist context.

**Acceptance**

- Simulated repeated upstream 401 on one model produces a single aggregated alert with “Update API key” and link to provider card.
- Alerts clear or downgrade when health/catalog recovers.

**Status:** `todo`

***

## Optional provider/model probe

**Goal:** Offer **explicit**, **operator-triggered** validation stricter than catalog polling — deferred from v0.3 setup wizard ([`version-v0.3.md`](version-v0.3.md#deferred-provider-probes-and-operator-alerts-v05)).

**Scope**

- **Not automatic on key save** — Wizard and default settings path stay: broker health + model count + availability.
- **Probe action** — “Test models” on provider card or assist-driven: optional minimal chat or `/v1/embeddings` call per model (capped batch, timeout budget).
- **Outcome** — Update availability toggles or show per-model status table; never silently disable without operator confirm.
- **Integration** — Probe results feed [Provider and model error alerts](#provider-and-model-error-alerts) when failures persist.

**Acceptance**

- Operator can run probe on Groq card; unavailable models marked in UI; chat fallback skips them with existing runtime behavior unchanged.

**Status:** `todo`

***

## Explicitly not this version

- Autonomous configuration changes without operator confirmation.
- Gateway-to-gateway or peer-stack reconciliation (see v0.4 peer backends).
- Replacing `/ui/settings` with chat-only configuration.
- Automatic full corpus re-embed on embedding model change without explicit operator acknowledgment (see embedding plan warning UX).

***

## Verification

| Area                         | Quick check                                                                                                      |
|------------------------------|------------------------------------------------------------------------------------------------------------------|
| Desired-state reconciliation | Introduce drift → UI reports it → guided or model-assisted fix restores documented behavior                      |
| Page guides                  | Three card types expose accurate API markdown; deep links open correct settings section                          |
| Model-assisted configuration | End-to-end assist flow with confirm-before-apply; audit logs present                                             |
| Provider alerts              | Repeated model errors aggregate to one alert with actionable link                                                |
| Optional probe               | Manual probe updates per-model status; not required for wizard continue                                          |
| Indexer embedding + purge    | [`plans/indexer-embedding-model-and-workspace-purge.md`](plans/indexer-embedding-model-and-workspace-purge.md) acceptance met |
| Workspace search             | [`plans/operator-workspace-search.md`](plans/operator-workspace-search.md) acceptance met                        |
| Settings cleanup             | [`plans/embedui-settings-card-cleanup.md`](plans/embedui-settings-card-cleanup.md) phases shipped               |

***

## See also

- [`version-v0.4.md`](version-v0.4.md) — previous version (ensemble, RAG scope, purge theme, in-app settings)
- [`version-v0.3.md`](version-v0.3.md) — setup wizard; light-touch provider validation
- [`chimera.plan.md`](chimera.plan.md) — product roadmap and requirements
- [`configuration.md`](configuration.md) — configuration reference
- [`plans/_template.md`](plans/_template.md) — phase-level plan template
