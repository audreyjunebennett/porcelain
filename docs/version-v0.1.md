# Version 0.1 — gateway baseline (working notes)

| Field | Value |
|-------|-------|
| **Doc kind** | `working-notes` |
| **Owners / areas** | Gateway, desktop, supervisor, operator UI |
| **Status** | `shipped` |
| **Targets** | Gateway v0.1 |
| **Last updated** | See git history |
| **Supersedes / superseded by** | Historical working notes; followed by [`version-v0.1.1.md`](version-v0.1.1.md) and [`version-v0.2.md`](version-v0.2.md) |

## At a glance

Stand up the chimera-gateway in Go in front of BiFrost: chat completions work, the operator UI works, and a single desktop binary opens a window where operators manage provider keys, see logs, and copy a Continue config. This is the working-notes scratchpad from that release — useful as history; the current code is the source of truth.

| Theme | Outcome | Status |
|-------|---------|--------|
| [Hide extra console; in-app logs](#1-desktop-hide-the-extra-console-show-logs-in-the-webview-implemented) | No console window; gateway logs surface in the webview | `done` |
| [Tabbed desktop shell](#2-desktop-webview-multiple-tabs-main-logs-admin-implemented) | Main / Logs / Admin tabs in one window | `done` |
| [Move off containers](#3-moving-away-from-containers-implemented) | Single Go binary with optional supervised BiFrost / Qdrant | `done` |
| [Portable first run](#4-portable-first-run-implemented) | Copyable token, no auto-seeded `tokens.yaml` | `done` |
| [Bootstrap before first token](#5-startup--login-first-run-and-missing-config-experience-implemented) | Loopback-only setup page; full stack starts after a token exists | `done` |
| [Multiple provider keys](#6-support-multiple-groq-gemini-api-keys-and-ollama) | Add and remove Groq / Gemini key rows in the panel | `done` |
| [Routing / fallback from catalog](#7-routing--fallback-v01-basic-ordering-from-available-models) | Deterministic chain generated from `/v1/models`; admin UI partial | `active` |
| [Exploration: smarter routers](#exploration) | Self-organizing router and per-turn small-model router | `deferred` |

---

This document is for **Audrey** (and a Cursor agent helping her) to **explore** what "done enough" for **v0.1** means in practice, how the repo behaves **today**, and which directions are **worth investigating** versus **already decided** in the product plan.

**Tone:** everything under *Explorations* is **optional research**, not a commitment. The authoritative roadmap and locked decisions remain in [`porcelain.plan.md`](porcelain.plan.md) and [`overview.md`](overview.md). Normative UI/desktop detail lives in [`plans/desktop-ui.md`](plans/desktop-ui.md).

---

## Current state (as implemented)

The gateway is a **small Go** service in front of **BiFrost** (OpenAI-compatible HTTP). It exposes the URLs:
- **/** 
- **GET /health**
- **GET /v1/models**, 
- **POST /v1/chat/completions**
- **GET /status** (when `**chimera serve**` supervises children)

**What works today**

- **Virtual model** `chimera-<semver>` (semver from `config/gateway.yaml`) appears first on `**GET /v1/models`**; concrete upstream ids pass through.
- **Token auth** from YAML (`config/tokens.yaml` by default), with **mtime reload**.
- **Routing policy** (`config/routing-policy.yaml`): for the virtual model only, **rule-based** selection of the **first** upstream model to try (`internal/routing`). Conditions today are thin (e.g. `min_message_chars` on the **last** user message); then optional `ambiguous_default_model`, else `**routing.fallback_chain[0]`** in `config/gateway.yaml`.
- **Fallback chain**: on **429 / 5xx** from the upstream, the gateway walks `**routing.fallback_chain`** starting at the index of the model that was attempted (`internal/chat`).
- **Streaming** (SSE) and non-streaming proxying to BiFrost.
- `**GET /health`**: probes the configured upstream (JSON field `**checks.upstream`). **Qdrant** is optional via `chimera serve`**; the **v0.1** gateway does not call Qdrant for chat.
- **Upstream API key**: if missing in config and env, `**EnsureGeneratedUpstreamAPIKey`** can **generate and persist** `upstream.api_key` in `gateway.yaml` (see `internal/config/upstream_api_key.go`).
- **Operator UI** (gateway-served): `**GET /ui/login`**, `**GET /ui/panel` (session after `POST /api/ui/login`), BiFrost provider rows via `/api/ui/***` (`internal/server/ui_handlers.go`, embedded HTML in `internal/server/embedui/`). `**GET /ui/models` mirrors the merged model list for tools.
- **Desktop shell** (optional build): `go build -tags desktop`** produces a binary whose **default no-subcommand** path runs **supervisor + gateway + webview** (`cmd/chimera/default_mode_desktop.go`, `webview_desktop.go`). `**chimera desktop`**, `**chimera serve`, `--headless`, and `chimera-gateway` behave as documented in `chimera help**`. The webview **opens the panel URL** derived from the listen address (`cmd/chimera/serve.go` → `panelURLFromListenAddr`); unauthenticated users are **redirected** to `**/ui/login`**.
- **Supervisor children** (BiFrost, Qdrant): subprocess **stdout/stderr** are wired to `**os.Stdout` / `os.Stderr`** (`internal/supervisor/bifrost.go`, `qdrant.go`), so all service logs go to the **same console** as the gateway. On **Windows**, a **desktop** build can still show a **console window** (and users report an extra command window alongside the webview); hiding that console and surfacing logs only in-app is **not** done yet (see below).

**Default local stack:** `**make up`** or `**go run ./cmd/chimera serve` with `./bin/bifrost-http` after `make chimera-install` (or `make install` for desktop OS deps too), plus provider env keys for `config/bifrost.config.json`. Desktop: `make desktop-build` / `make desktop-run**` per [`gui-testing.md`](gui-testing.md).

---

## v0.1 — Features to Implement

These are the **last mile** items for v0.1 UX and routing, articulated from the current product direction.

### 1. Desktop: hide the extra console; show logs in the webview (**implemented**)

**Today:** Supervisor processes inherit the parent’s stdio; the gateway logger also writes to **stdout**. On Windows especially, launching the **desktop** binary can open a **terminal window** beside the webview while logs and child output stream there.

**Goal:**

- **No visible console** for the desktop entry (platform-specific: e.g. Windows subsystem / build flags, or subprocess creation flags that avoid allocating a new console).
- **Operator-visible logs** inside the app: capture **gateway** `slog` output and **child** stdout/stderr (or selected streams), then expose them through a **gateway endpoint** (e.g. SSE or chunked HTTP) or in-memory ring buffer + `**GET /api/ui/logs`** authenticated like other admin routes, consumed by a **Logs** tab in the shell (see §2).

**Design notes:** Avoid logging secrets; consider log level and backpressure; headless `**chimera serve`** should keep writing to stderr for servers and automation.

### 2. Desktop webview: multiple tabs (main, logs, admin) (**implemented**)

**Today:** Single webview window navigates to **one URL** (currently the panel URL after listen).

**Goal:** A **tabbed** (or segmented) shell:


| Tab       | Purpose                                                                                                                                            |
| --------- | -------------------------------------------------------------------------------------------------------------------------------------------------- |
| **Main**  | Primary operator surface — welcome, status, or landing agreed with [`plans/desktop-ui.md`](plans/desktop-ui.md) (not only jumping straight to panel). |
| **Logs**  | Live tail of gateway + supervised services (feeds §1).                                                                                             |
| **Admin** | Existing admin console — equivalent to today’s `**/ui/panel`** (and login flow when needed).                                                       |


Implementation options include **native tab UI** around multiple webviews, **one webview** loading a thin local “chrome” page with **iframes** or **client-side tabs** hitting gateway routes, or a **small shell HTML** page served by the gateway. The shared requirement is **same-origin or documented cookie** behavior so the **admin session** remains valid across tabs.

### 3. Moving away from containers (**implemented**)

Making a fast, portable application is important for the v0.1 release as it dictates the framework we are building on top of going forward.

**Default deployment shape:** **Go** `**chimera`** / `**chimera serve**` with **BiFrost** — see [`plans/upstream-llm-bifrost.md`](plans/upstream-llm-bifrost.md) for the phased history.

**4c. Vector store without a dedicated Qdrant process**

- **Context:** **v0.2** RAG may use supervised **Qdrant** or another backend. A portable install may want vectors **off by default** until RAG is in use.
- **Idea:** spike **embedded** Qdrant (where license and artifact size allow), another embedded vector backend, or a small local store for early RAG—so operators are not required to run a separate Qdrant container for every setup.
- **Plan alignment:** v0.2 in [`porcelain.plan.md`](porcelain.plan.md) assumes a **swappable vector-store adapter**; if that boundary stays stable, embedded and remote Qdrant can remain interchangeable behind the same interface.

### 4. Portable “first run” (**implemented**)

**Idea:** ship or build a **portable application** that enables a user to enter:

- **Provider credentials** (or env-file generation) for the backends they use.
- **Local LLM server** URL (Ollama, vLLM, llama.cpp-compatible, etc.) and how that maps to BiFrost (or a future proxy).
- **Gateway token** minting or `tokens.yaml` generation.
- Optional: **probe** BiFrost and the gateway `**/health`** before declaring success.

**v0.1 bridge:** Concrete behavior for **gateway tokens and first boot** is specified in [§5](#5-startup--login-first-run-and-missing-config-experience) (bootstrap UI, no auto-seeded `tokens.yaml`, supervisor gating). The broader portable wizard (providers, probes, etc.) remains optional beyond that slice.

### 5. Startup / login: first-run and missing-config experience (**implemented**)

**Status:** **Product decisions below are locked.** The **implementation** described here is **in the tree** (bootstrap serve/gateway path, setup UI, token admin API, packaging/configure no longer seeding `tokens.yaml`). If this section drifts from code, update the doc.

**Previously (before bootstrap work):** The webview opens toward `**/ui/panel`**; without a session the user lands on `**/ui/login`, which asks for a **gateway token** and points them at `config/tokens.yaml`**. `config/tokens.yaml` is often **created automatically** from `config/tokens.example.yaml` (`make configure`, release packaging, `.goreleaser.yaml`). There is **no** dedicated setup path that **mints** a token in the UI on first launch. **BiFrost** and **Qdrant** start under `**chimera serve**` even when no gateway tokens exist.

---

#### Decisions (normative for v0.1)

1. `config/tokens.example.yaml`
   - **Keep** the file in the repo and **continue to ship** it in archives (documentation and manual YAML editing).
   - **Do not** automatically create `config/tokens.yaml` from the example on first boot, in `scripts/configure.sh`, or in **release / Goreleaser** steps. Operators get `tokens.yaml` only after they **create a token in the UI** (bootstrap) or **copy/edit the example themselves**.

2. **When bootstrap mode applies**  
   Enter **bootstrap** when the resolved `tokens.yaml` path is **missing**, **unreadable**, **unparseable**, or contains **zero valid token rows** (same rule as today’s parser: e.g. `token` and `tenant_id` must be non-empty). Do **not** treat “empty tokens” as normal mode.

3. **Listen address**
   - **Bootstrap:** bind only to **loopback**, **dual-stack**: `127.0.0.1` and `[::1]` (or equivalent so both IPv4 and IPv6 `localhost` work). Ignore `gateway.yaml` `listen_host` / port for the public gateway socket in this mode only, or override to loopback with a documented effective listen (agents: pick one implementation strategy and document it in code comments).
   - **Normal mode** (after at least one valid token exists): use `gateway.yaml` (and CLI overrides) as today, e.g. `0.0.0.0` when configured.

4. **Supervisor: BiFrost and Qdrant**
   - **Do not start** BiFrost or Qdrant while in **bootstrap** (no valid tokens). The gateway HTTP server still runs to serve the **limited** bootstrap surface.
   - **After** `tokens.yaml` contains at least one valid token, **restart** is required so the process can **start children** and apply **normal** listen binding (see below). Hot-rebinding or starting children mid-process is **not** required for v0.1.

5. **Data safety**
   - If the user **deletes** or **empties** `tokens.yaml`, **never** automatically delete or wipe **BiFrost** or **Qdrant** data directories. Recovery is **operator-driven** (restore backup, recreate tokens, etc.).

6. **HTTP surface in bootstrap**
   - Mount only a **minimal router**: static assets and routes needed for **token creation** (and health if required for the shell). **No** `**/v1/*`** proxy**, no full **admin panel** features that assume upstream or children.
   - **Unauthenticated** access is acceptable **only** on this **loopback-bound** bootstrap surface and **only** for those setup endpoints (same threat model as other localhost-first tools).

7. **UI flow**
   - **Bootstrap:** dedicated page (e.g. `/ui/setup`) — **create first token** (label/name → generate secret → **atomic write** `tokens.yaml` → show **full token once** with **copy** affordance + message to **save it** and **restart** Chimera). Do **not** land users on the existing full `/ui/panel` during bootstrap.
   - **Normal:** existing **login** + `/ui/panel`. Add **token management** (list metadata, **add**, **delete**; optional **rotate** later) in admin — **new** routes/handlers (e.g. under `/api/ui/...`) that **read/write** `tokens.yaml` with the same validation rules as the file format today. Authenticate these like other admin APIs (session after gateway token login).

8. **Desktop / webview entry**
   - When bootstrap applies, open `/ui/setup` (or equivalent), not `/ui/login` or `/ui/panel`.

---

#### Implementation checklist (for agents)

Use this to track work; tick in PRs or remove items as completed.

| Area | Action |
|------|--------|
| `cmd/chimera/serve.go` | Detect bootstrap vs normal from token store state; **skip** `StartQdrant` / BiFrost when bootstrapping; avoid **blocking** gateway startup on upstream health in bootstrap; **dual-stack loopback** listen in bootstrap. |
| `internal/server/` | Separate **bootstrap** mux vs **full** mux; redirect or entry URLs for desktop; **POST** (or equivalent) to **append** token to YAML safely; admin **token CRUD** behind session auth. |
| `internal/tokens/` | Helpers as needed: **count valid tokens**, **atomic save** / merge into YAML without dropping unrelated content. |
| `internal/server/embedui/` | `setup.html` (or similar) for first token; admin components for **token list / add / delete**. |
| **Packaging** | `scripts/release-package.sh`, `.goreleaser.yaml`: ship `tokens.example.yaml`; **do not** copy it to `tokens.yaml`. |
| `scripts/configure.sh` | **Do not** auto-create `config/tokens.yaml` from the example (or gate behind an explicit flag if you must preserve dev ergonomics — default off). |
| **Docs / README** | First-run story: **no** pre-created `tokens.yaml`; optional manual copy from example for advanced users. |
| **Tests** | Tests that assume BiFrost always up with empty/missing tokens need **bootstrap** or **fixture tokens** paths. |

---

#### Rationale (short)

- **Loopback + no children** during bootstrap limits exposure while `tokens.yaml` is absent.
- **Dedicated setup page** avoids half-working admin widgets that need **BiFrost** / **Qdrant** / `gateway.yaml` listen.
- **Restart** keeps a single clear transition to **normal** bind and **supervised** stack.

This aligns with **§ Portable “first run”** but narrows v0.1 to **bootstrap token UI → persist `tokens.yaml` → restart → normal login and panel**, not a full multi-step provider wizard.

### 6. Support multiple Groq, Gemini API Keys, and Ollama

**Groq / Gemini (implemented):** The operator panel (`**/ui/panel**`, same embed as desktop **Admin** tab) lists **all** API key rows returned by BiFrost for each provider, sorted for display: `chimera-<provider>-key-<n>` names (numeric order), then any other keys by name. Adding a key **appends** a new row with `weight: 1` and a generated name, **without** a `models` field (BiFrost treats missing `models` as all models for that key); all keys are normalized to **weight 1** on each append/remove. Removing a key **DELETE**s that row by exact `name` and **PUT**s the rest of the provider document (last write wins if two tabs race). Authenticated BFF routes: `POST /api/ui/provider/{groq|gemini}/keys` body `{"value":"…"}`, `POST /api/ui/provider/{groq|gemini}/keys/delete` body `{"name":"…"}`. `GET /api/ui/state` includes `providers.<id>.keys` as `{ name, key_hint, key_configured }[]` plus existing `key_hint` / `key_configured` summaries. Implementation: `internal/bifrostadmin/` (merge + summarize), `internal/server/ui_save.go`, `ui_handlers.go`, `embedui/panel.html`.

**Ollama:** Still **one** `network_config.base_url` per `ollama` provider in BiFrost; **multiple Ollama sites** remains future work unless BiFrost gains a first-class pattern for it.

### 7. Routing / fallback: v0.1 "basic" ordering from available models

**Longer-term exploration** (LLM coordinator emitting full routing policy) stays under [Exploration §1](#1-self-organizing-router) below.

**For v0.1**, the intent is a **basic, deterministic** pipeline: derive an ordered `routing.fallback_chain` and a matching `routing-policy.yaml` from what BiFrost actually exposes, with **validation before persist**, and an **operator-facing** path to **generate**, **inspect results**, and **optionally test** before trusting the new files.

#### Product goals (normative)

1. **Source of truth:** Upstream `GET /v1/models` (already proxied/merged on the gateway) and/or static config already known to BiFrost.
2. **Heuristic:** Build or **suggest** `routing.fallback_chain` ordered **remote / higher-performance first**, then **local** (e.g. Ollama) — using **metadata** when available (provider id, known model families, optional operator overrides) and a **small curated map** or rules file when metadata is thin.
3. **Persist with safety:** Updates to `config/routing-policy.yaml` and `routing.fallback_chain` in `config/gateway.yaml` must **validate** (parseable YAML, policy shape the gateway accepts, gateway reloadable) **before** writing; on failure, **do not** partially update files — return a clear error to the operator.
4. **Admin panel UX (desired):** A **Routing** area in the operator UI (after provider setup is usable) that:
   - Explains how the **virtual model** (`chimera-<semver>`), `routing-policy.yaml`, and `routing.fallback_chain` interact (initial model vs 429/5xx failover).
   - Offers a primary control to **regenerate routing from the live catalog** (same behavior as the API below), respecting `routing.filter_free_tier_models` and `config/provider-free-tier.yaml` when that flag is on.
   - Shows the **saved** `fallback_chain` and policy summary **after** a successful generation (no separate “preview” step required unless we add one later).
   - **Test computed router (not implemented yet):** Let the operator run a **dry evaluation** with sample messages (e.g. short vs long user turn) and see **which rule would match**, **initial upstream id**, and **ordered fallback slice** — **without** calling a paid completion, or optionally with a **minimal** test completion — before or after save.
5. **Other entry points:** The same generation logic may also be exposed from **first-run / setup** or a **CLI** later; v0.1 does not require all surfaces if the **admin API** exists.

#### Implemented in the tree today

- **Authenticated HTTP API:** `POST /api/ui/routing/generate` (session after `POST /api/ui/login`) fetches upstream `GET /v1/models`, optionally intersects with `provider-free-tier.yaml` when `routing.filter_free_tier_models` is **true**, orders models deterministically (`internal/routinggen`), writes `routing-policy.yaml` and patches `routing.fallback_chain` via `config.WriteGatewayFallbackChain`, and **rejects** invalid output **before** leaving inconsistent files. See `internal/server/ui_routing_generate.go`.
- **Free-tier catalog reference (offline / CI-friendly):** `make catalog-free` (`go run ./chimera/cmd/catalog-write-free`) fetches public Groq + Gemini docs and emits a **reference** YAML snapshot (optional `INTERSECT=` against a local models export); operators may **manually** merge into `provider-free-tier.yaml` — the gateway does **not** load that snapshot automatically. See `docs/configuration.md`, `internal/freecatalog/`, `chimera/cmd/catalog-write-free/`.
- **Catalog visibility:** When `routing.filter_free_tier_models` is on and the allowlist loads, merged `GET /v1/models` lists only allowlisted upstream ids (virtual model still first).

#### Not implemented yet (track against v0.1 admin experience)

- **Panel UI** for §7: `embedui/panel.html` (or a dedicated routing view) does **not** yet expose the routing explanation, **Regenerate** button, post-save **fallback_chain** / rule summary, or **test harness** — only the **API** exists; the desktop **Admin** tab is still a thin iframe to the same panel.
- **Test computed router** (dry-run / sample evaluation UI and/or API) — **not built**.
- **Setup / bootstrap wizard** step that runs generation automatically after keys — **not built** (generation is manual via API today).
- **Token-count-based** routing conditions (instead of or in addition to `min_message_chars` on the last user message) — **not built** (`**internal/routing**` still uses character length only).
- **LLM-assisted** policy generation (pick a coordinator model, run fixed prompts) — intentionally **out of scope** for v0.1; see [Exploration §1](#1-self-organizing-router).
- **Centralized service** to distribute allowlists / pricing-derived model lists — **future**; today lists are file-based or **make**-generated snapshots.

This section should stay aligned with `docs/configuration.md` and `README.md` as behavior evolves.

---

## Exploration

These are ideas discovered during the development of v0.1 but should be pushed off to further releases.

### 1. Self-Organizing Router

Instead of hand-authoring `routing-policy.yaml` and fallback chains from scratch, the gateway (or a setup job) would:

1. **Collect a model list** — e.g. call upstream `**GET /v1/models`** / BiFrost `**/api/models**` and/or merge configured static entries.
2. **Order models by “strength”** at **interpreting prompts and producing configuration** — this is intentionally vague: it could mean parameter size, bench scores, operator tiers, latency class, or a curated map file checked into the repo.
3. **Choose the top model as a “router coordinator”** — a single model that runs **once** (or on a schedule) to emit **machine-readable** routing config.
4. Feed that coordinator:
  - a **prompt** focused on routing rules (e.g. when the client specifies a concrete model in the request, honor it; when using the virtual model with **no** explicit tier, **default toward the most capable** option subject to cost/latency constraints);
  - the **full list** of available model ids and any metadata (context length, vision, local vs cloud);
  - a **short specification document** embedded in-repo describing the **router config schema** (could be an extension of today’s YAML or a generated artifact).

**Relationship to current code:** `RoutingPolicy` is **deterministic YAML** + simple predicates — there is **no** LLM-in-the-loop router today. This exploration would be **new behavior**, likely gated behind a setup mode or admin API.

**v0.1 subset:** [§7 in “Features to Implement”](#7-routing--fallback-v01-basic-ordering-from-available-models) — **deterministic** fallback ordering from the catalog (**remote / strong first**, **local second**) without an LLM coordinator; admin **UI** polish and **router test harness** in §7 may still be open.

**Risks:** coordinator hallucinates ids; nondeterministic setup; security if the coordinator can be prompted during normal traffic. Mitigations: validate emitted ids against `/v1/models`, dry-run, human review step, separate command.

---

### 2. Router with **small** Model

**Idea:** at **each** chat completion (when using `chimera-<semver>`), run a **cheap** model first to **classify** or **select** the upstream model (and optionally parameters), then call the chosen backend for the real completion.

**Contrast with §1:** §1 is closer to **bootstrap / config generation**; §2 is **runtime** routing every turn.

**Why explore it:** YAML rules do not see **semantics** (only things like message length today). A small model could use the **last user turn** (and maybe tool/schema hints) to pick “fast vs strong” or “local vs cloud.”

**Costs:** extra **latency** and **cost** per request; need **timeouts** and a **safe default** if the router fails (fall back to first chain entry or last good choice). Streaming UX needs a clear story (router call must finish before streaming the main model).

---

## Quick reference — key files

| Area                              | Path                                                         |
| --------------------------------- | ------------------------------------------------------------ |
| Gateway CLI                       | `cmd/chimera/`                                               |
| Desktop webview (tag `desktop`)   | `cmd/chimera/webview_desktop.go`, `default_mode_desktop.go`  |
| HTTP server, health, models, chat | `internal/server/`, `internal/chat/`, `internal/upstream/`   |
| Config load / reload              | `internal/config/`                                           |
| Routing policy                    | `internal/routing/`                                          |
| Supervisor (BiFrost / Qdrant)     | `internal/supervisor/`                                       |
| Gateway config                    | `config/gateway.yaml`                                        |
| Gateway tokens (example ships; live file not auto-created on first boot) | `config/tokens.example.yaml`, `config/tokens.yaml` (see §5) |
| BiFrost bootstrap                 | `config/bifrost.config.json`                                 |
| Routing rules                     | `config/routing-policy.yaml`                                 |
| Free-tier allowlist (optional)    | `config/provider-free-tier.yaml`                             |
| Catalog snapshot tool (reference) | `make catalog-free`, `chimera/cmd/catalog-write-free/`, `internal/freecatalog/` |
| Regenerate routing (API)          | `POST /api/ui/routing/generate` (`internal/server/ui_routing_generate.go`) |
| Operator UI (embed)               | `internal/server/embedui/`, `internal/server/ui_handlers.go` |
| Product / locked decisions        | [`porcelain.plan.md`](porcelain.plan.md)                               |
