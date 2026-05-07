# Claudia Gateway — Version 0.3 plan

This document is the **working plan for v0.3**. It pulls forward the **peer-to-peer backend** scope from the master product plan ([`claudia-gateway.plan.md`](claudia-gateway.plan.md)) and adds **first-run / second-run onboarding** for the desktop application so new operators can configure and validate the gateway without hunting through docs. **v0.3** also targets the **project rename from Claudia to Porcelain** (see §6).

**Relationship to other docs**

- Authoritative **architecture and numbered requirements** remain in [`claudia-gateway.plan.md`](claudia-gateway.plan.md) unless this plan explicitly revises them.
- **Indexer** milestones labeled “v0.3” in [`indexer.plan.md`](indexer.plan.md) (e.g. scoped overrides, headers) are **indexer product versions**, not necessarily the same shipping train as **gateway desktop v0.3**; cross-link when both touch the same API.

---

## 1. Peer-to-peer model backends (from original v0.3 roadmap)

This section summarizes what [`claudia-gateway.plan.md`](claudia-gateway.plan.md) already assigns to **v0.3** so implementation and docs stay aligned.

### 1.1 Release-roadmap slice

From the master **Release roadmap** table:

- **Peer-to-peer model backends**: call **another operator’s LiteLLM** over a **host-routable** URL and **published** port (not Compose-internal DNS from another machine).
- **LiteLLM virtual keys** (or equivalent proxy credentials) for **cross-host** authentication.
- **Gateway / LiteLLM configuration** and **operator documentation** for peer paths: requirements **#24–27**, **#30** (peer as `api_base`), and **#9** (cross-host publishing vs intra-stack DNS)—see **Peer operators** in the master doc.
- **Per-key / usage observability** (**#46**): track which key/backend was used and exposure to RPM/TPM-style limits where upstream headers exist.

### 1.2 Product rules (condensed)

- **Peer = their LiteLLM, not their Gateway** (**#25**): configure OpenAI-compatible `api_base` to the **peer’s LiteLLM** endpoint (e.g. Tailscale/LAN IP + **published** LiteLLM port + `/v1`). Use a **LiteLLM virtual key** issued by the peer ([LiteLLM virtual keys](https://docs.litellm.ai/docs/proxy/virtual_keys)). Do **not** chain **Gateway → peer Gateway** as the default integration (**#27**).
- **Independent stacks** (**#24**): each operator has their own gateway, tokens, and policy; no assumption that one gateway “owns” another’s RAG.
- **Document ports per host** (**#26**): distinguish **Claudia Gateway** (IDE-facing) vs **LiteLLM** (peer `api_base` target); firewall/VPN expectations; TLS/mTLS deferred to **v0.7** unless operators add their own terminator.
- **Cloud vs local policy** (**#30**): **Peer LiteLLM** appears as a **remote-runner** `api_base` entry in routing policy.
- **Graceful degradation** (**#47**): same fail-over / fail-fast behavior when a peer LiteLLM is in the chain; **no** gateway queue until **v0.8**.
- **Containers / networking** (implementation decisions): from **v0.3**, compose/docs consider **LAN peer access** to LiteLLM where enabled; **normative** TLS for peer URLs remains **v0.7** (**#54**).

### 1.3 Deliverables checklist (peer scope)

- [ ] Configuration surfaces (and/or files) to add **peer LiteLLM** backends with **virtual key** auth and host-reachable base URLs.
- [ ] Operator docs: cross-host topology, **published** ports, virtual keys, anti-patterns (Compose hostname of peer stack, gateway-on-gateway).
- [ ] **Observability (#46)**: per-key / per-backend usage signals where APIs expose limits or identifiers.

---

## 2. First launch — API token handoff

**Goal:** On the **first** run, the user obtains a **gateway API token**, optionally persists it, then **restarts** the app and supplies the token (UI or environment) so the second-run wizard can run authenticated.

### 2.1 First screen (first run only)

1. The application displays an **API key** (gateway-issued token) that the user can **copy**.
2. Below the key:
   - Optional action: **Save key** — when pressed, **upsert** into a **dotenv** file (project/agreed path): if `CLAUDIA_GATEWAY_TOKEN` is **not** already defined, set it to this key; if already defined, do **not** overwrite without an explicit future “replace” flow (this plan: **only set when absent**).
3. User guidance: copy and/or save, then **close** the application.
4. On next launch, the user either:
   - Pastes the key into the app when prompted, or  
   - Relies on **`CLAUDIA_GATEWAY_TOKEN`** being read from the environment / dotenv load order as implemented.

### 2.2 Acceptance notes

- Token display must be compatible with whatever the gateway already uses for **tenant auth** (same token used for `Authorization: Bearer` elsewhere).
- **Save** behavior must be safe on repeated launches (idempotent upsert, no silent clobber of user-set values).

---

## 3. Second launch — multi-step setup wizard

**Goal:** After the token is available on second launch, walk through **configuration and testing** in **seven steps**, with **Skip setup** returning the user to the **normal multi-tab** UI.

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
- **“Add a Folder”** control: placed **upper-right** (per spec).
- **Embedding model** panel: **combobox** of **valid embedding models** derived from configured providers + local server (models suitable for `/embeddings` and gateway/Qdrant expectations).
  - **Default selection:** `ollama/nomic-embed-text:latest` or the project’s agreed default that matches **Qdrant**, **chunking**, and **indexer** settings from config.
- **No valid embedding models:**
  - Show a clear **message** that no embedding-capable models are available.
  - **Disable** “Add a Folder”.
  - If the user attempts folder add (or focus the disabled control), **animate** the embedding panel to indicate it cannot be configured yet, show **warning** + instructions to go **back** to earlier steps and add a **local embedding-capable** model (e.g. **step 3** local server or **step 2** provider keys, as appropriate).
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
    2. Run search **across all workspaces** (same semantics as production search).
    3. **Zero results:** show that explicitly; add **notes/warnings** based on indexer state (idle, error, no chunks, etc.).
    4. **Multiple results:**  
       - First block: **summary** — total hits across workspaces; **number of distinct workspaces** with a match.  
       - Second block: **details** — file paths and **short excerpts**.
  - Below: **indexer run log** view — **same content and live updates** as the dedicated **log** page in the app so users see progress and errors.
- **Back** → step 5. **Next** → step 7.

---

### Step 7 — Integration (VS Code Continue)

- Show the **Continue** integration panel (overview + actions consistent with the current UI).
- Rename **“indexed folders”** to **“setup projects”** in this context.
- Combo box lists **setup projects** (indexed-folder entries).
- Add a distinct entry that generates a **VS Code Continue** snippet for the **global** location Continue expects (global `.continue/config.yaml` — **no** project or flavor headers in that variant).
- Entries remain **copyable** and **creatable** like today; user selects an entry and uses **copy** and/or **create** when defined indexes exist as applicable.
- **Back** → step 6. **Finish** → **main multi-tab** application view.

---

## 4. Cross-cutting implementation notes

- **Router generator** and **fallback model list** must be **shared** between the wizard and the main settings UI so wizard changes do not use a one-off code path.
- **Second-run detection** should be robust (e.g. token present + first-time wizard flag in local state), so reinstalls and upgrades behave predictably—exact mechanics belong in implementation with UX review.
- Peer-to-peer **LiteLLM** configuration might surface in advanced settings in the same release or a follow-up; this plan does not require the **seven-step wizard** to cover peer URLs unless product wants it—default remains **operator docs + config files** from §1.

---

## 5. Suggested completion criteria for “v0.3 shipped”

- [ ] Peer LiteLLM + virtual keys + docs (#24–27, #30, #9, #46) meet §1 checklist.
- [ ] First-run token UX + optional `CLAUDIA_GATEWAY_TOKEN` dotenv upsert (§2).
- [ ] Second-run wizard steps 1–7 (§3) with skip/finish navigation and shared router regeneration behavior.
- [ ] **Porcelain rename** per §6 (branding + technical migration rules + docs/indexer parity).

When this plan is implemented, update [`claudia-gateway.plan.md`](claudia-gateway.plan.md) **Release roadmap** row for v0.3 if the shipped scope differs (e.g. split peer backends vs onboarding into separate releases).

---

## 6. Project rename (Claudia → Porcelain)

**Goal:** Ship **Porcelain** as the public product and repo identity, replacing **Claudia** in user-visible surfaces and in normative naming for new integrations, while minimizing breakage for existing operators.

### 6.1 Scope (typical buckets)

- **Operator-facing branding** — Application title, about screens, onboarding copy, and primary docs that refer to “Claudia” / “Claudia Gateway” should consistently use **Porcelain** (and agreed secondary wording, e.g. “Porcelain Gateway” vs a single-word product name—pick one spelling for UI and headings).
- **Technical identifiers** — Align on renames where they affect interoperability: **`make` targets**, install paths, container image names, default Compose service labels in published samples, CLI flags/help text, and **environment-variable** prefixes (e.g. migrate `CLAUDIA_*` → `PORCELAIN_*` or keep documented aliases for one release per decision in implementation).
- **HTTP / API ergonomics** — Custom headers (`X-Claudia-*` and similar), log fields, and token claims that encode the old name should get **Porcelain-prefixed** names with a **backward-compatible acceptance** period where clients (indexer, IDE plugins) might still send the old headers (**dual read**, single canonical write)—exact matrix belongs in implementation and indexer docs.
- **Repository and module naming** — If the GitHub org/repo or Go module path changes, treat it as part of this milestone (or explicitly defer with a checklist note); importer URLs and docs must track the canonical location.

### 6.2 Deliverables checklist

- [ ] Written **rename decision**: canonical product string, migration rules for env vars and headers, and whether old names remain accepted (and until when).
- [ ] UI, installer, and packaged artifacts use **Porcelain** branding throughout.
- [ ] Documentation set (starting with docs linked from onboarding and [`claudia-gateway.plan.md`](claudia-gateway.plan.md)) updated so new operators never need to hear “Claudia” unless reading historical release notes.
- [ ] Companion components (**indexer**, desktop app, Compose samples) updated in lockstep for any renamed env vars or headers called out above.

### 6.3 Completion criteria linkage

Treat §6 as satisfied when renaming is **consistent for v0.3 ship**—no dangling primary surfaces still saying “Claudia,” and no silent breaks for documented upgrade paths without a migration note.
