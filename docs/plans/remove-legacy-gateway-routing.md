# Plan: Remove legacy gateway routing YAML

| Field | Value |
|-------|-------|
| **Doc kind** | `refactor-plan` |
| **Owners / areas** | Gateway runtime, operator config, admin UI, operator docs, CLI tooling |
| **Status** | `shipped` |
| **Targets** | Gateway next minor after virtual models ship |
| **Last updated** | See git history |
| **Supersedes / superseded by** | Completes Phase 5 follow-up in [`virtual-models-operator.md`](virtual-models-operator.md); supersedes global routing sections in [`configuration.md`](../configuration.md) |
| **As-built** | [`operator-virtual-models.md`](../features/operator-virtual-models.md), [`gateway-chat-routing-pipeline.md`](../features/gateway-chat-routing-pipeline.md) |

## At a glance

Virtual models in operator SQLite are the routing source of truth today, but `gateway.yaml` still carries a legacy `routing:` block, a global `routing-policy.yaml` path, YAML bootstrap import, and unused `/api/ui/routing/*` admin endpoints. This plan removes those surfaces completely so operators configure fallback chains, routing rules, and tool routers only on virtual model cards—or chat directly with upstream provider model ids when no virtual model is defined.

| Phase | Outcome | Status |
|-------|---------|--------|
| [Phase 1 — Delete legacy admin API and gallery UI](#phase-1--delete-legacy-admin-api-and-gallery-ui) | No `/api/ui/routing/*` routes; gallery no longer references global YAML writers | `done` |
| [Phase 2 — Remove runtime YAML routing paths](#phase-2--remove-runtime-yaml-routing-paths) | Chat and catalog resolve VMs from SQLite only; direct upstream ids work without YAML | `done` |
| [Phase 3 — Remove config and file artifacts](#phase-3--remove-config-and-file-artifacts) | `gateway.yaml` has no `routing:` or `paths.routing_policy`; `Resolved` trimmed | `done` |
| [Phase 4 — Tooling, tests, and docs](#phase-4--tooling-tests-and-docs) | CLI audits, tests, and operator docs match VM-only routing | `done` |

---

## Background

[`virtual-models-operator.md`](virtual-models-operator.md) shipped per-virtual-model routing stacks in operator SQLite, VM CRUD APIs, and settings cards (`adminVirtualModels.js`). Live settings no longer use the old global routing cards (`adminRouting.js`, `adminFallback.js`, `adminRouterModels.js` were removed per [`embedui-settings-card-cleanup.md`](embedui-settings-card-cleanup.md)).

**What still lingers**

| Surface | Location | Runtime effect today |
|---------|----------|----------------------|
| `routing.fallback_chain`, `router_models`, `tool_router`, `filter_free_tier_models` | `config/gateway.yaml` | Parsed into `config.Resolved`; **not** used for chat after bootstrap import |
| `paths.routing_policy` → `routing-policy.yaml` | `gateway.yaml` | Loaded as global `routing.Policy`; only used by legacy YAML shim |
| Bootstrap import | `operatorstore/bootstrap.go` | On empty operator DB, inserts one `Chimera-<semver>` VM from YAML |
| Legacy chat shim | `virtualmodel_chat.go` `resolveVirtualModelChat` | Falls back to YAML when `body.model == Chimera-<semver>` and registry miss |
| Legacy admin API | `adminui/api/routing/` | Eight `POST /api/ui/routing/*` handlers that write YAML |
| State API fields | `GET /api/ui/state` → `gateway.fallback_chain`, etc. | Exposes stale global YAML to clients; live VM UI reads per-VM APIs |
| Catalog prepend shim | `prependVirtualModelsToCatalog` | Injects synthetic `Chimera-<semver>` when registry empty |
| `filter_free_tier_models` | `config.Resolved` | Loaded but **unused** at runtime; replaced by [operator model availability](../features/operator-provider-model-availability.md) |

**Target operator model**

- **Virtual model defined** → client sends VM `model_id`; gateway applies that VM's fallback, policy, and tool router.
- **No virtual model** → client sends an upstream id (`groq/...`, `gemini/...`, `ollama/...`); gateway proxies directly to chimera-broker with no fallback walk or tool-router slimming.
- **Fresh install** → operator DB starts with **zero** virtual models; no auto-import from `gateway.yaml`.

**Related docs:** [`operator-virtual-models.md`](../features/operator-virtual-models.md), [`gateway-chat-routing-pipeline.md`](../features/gateway-chat-routing-pipeline.md), [`operator-provider-model-availability.md`](../features/operator-provider-model-availability.md), [`configuration.md`](../configuration.md).

**Non-goals:** Changing virtual model CRUD APIs, per-VM generate/evaluate flows, RAG scoping, or provider availability semantics.

---

## Phase 1 — Delete legacy admin API and gallery UI

**Goal.** Remove HTTP and embed UI entry points that write or display global routing YAML; operators use virtual model cards exclusively.

**Deliverables**

- Delete package `chimera/chimera-gateway/internal/server/adminui/api/routing/` (`handlers.go`, `register.go`) and unregister from `adminui/register.go`.
- Remove types from `internal/operatorapi/routing.go` used only by legacy routes (or delete the file if fully unused).
- Delete gallery artifact `embed/embedui/gallery/gallery-unified-operator-routing.js` and its script tag in `settings/gallery.html`.
- Trim `GatewayState` in `internal/operatorapi/state.go` and `adminui/api/state/build.go`: drop `fallback_chain`, `router_models`, `tool_router_*`, `filter_free_tier_models`, `routing_policy_yaml`, `routing_policy_basename` from the JSON contract (keep `virtual_models[]`, `semver`, service overview).
- Remove dead reads in `adminVirtualModels.js` (e.g. unused `gw.filter_free_tier_models`).
- Delete tests: `ui_routing_generate_test.go` and any embed tests that call `/api/ui/routing/*`.

**Acceptance**

- `rg '/api/ui/routing/' chimera/` returns no route registrations or live UI callers (gallery/tests only before this phase).
- Settings feed still loads; virtual model cards save fallback/policy/tool-router via `/api/ui/virtual-models/{id}/…`.
- `go test ./chimera/chimera-gateway/internal/server/ -count=1` passes.

**Status:** `done`

---

## Phase 2 — Remove runtime YAML routing paths

**Goal.** Gateway chat and model catalog never read global routing from YAML; only virtual models trigger the routing pipeline.

**Deliverables**

- **`BootstrapVirtualModels`:** stop importing from `config.Resolved` routing fields; return immediately when operator DB is empty (no auto `Chimera-<semver>` row). Keep idempotent no-op when VMs already exist.
- **`resolveVirtualModelChat`:** remove legacy branch that matches `res.VirtualModelID` and reads `res.FallbackChain` / global tool-router settings.
- **`handleVirtualModelChat`:** remove `useLegacyPol` path and global `pol *routing.Policy` parameter; VMs always use compiled policy from `virtualmodel.Resolved`.
- **`runtime.go`:** remove `routing.NewPolicy(res.RoutingPolicyPath)` and policy reload on config sync; drop `Snapshot()` policy return if unused elsewhere.
- **`prependVirtualModelsToCatalog`:** remove synthetic `Chimera-<semver>` entry when registry is empty; catalog = enabled VMs + filtered upstream models only.
- **`handleV1Chat`:** remove global `res.ToolRouterConfidenceThreshold` defaulting (VM path only); confirm direct path remains `chat.ProxyChatCompletion` with client model id unchanged.
- **`fallback_availability_audit.go`:** audit VM fallback chains only; remove `gateway.fallback_chain` source and `gateway.fallback_chain` log slug references where obsolete.
- Update `virtual_models_test.go`, `registry_test.go`, `bootstrap` tests for no YAML import behavior.

**Acceptance**

- Fresh operator DB + gateway start → **zero** virtual model rows.
- `POST /v1/chat/completions` with upstream id (e.g. `groq/llama-3.1-8b-instant`) succeeds without any `routing:` in `gateway.yaml`.
- `POST /v1/chat/completions` with a VM id uses SQLite routing stack; YAML edits do not affect behavior.
- Request for unknown model id that is not a VM falls through to direct upstream (broker error if id invalid)—no YAML magic.
- Integration tests cover: direct upstream chat; VM chat with distinct fallback chains.

**Status:** `done`

---

## Phase 3 — Remove config and file artifacts

**Goal.** `gateway.yaml` no longer documents or loads global routing keys; repo examples match VM-only routing.

**Deliverables**

- Remove from `chimera/internal/config/config.go` and `Resolved`:
  - `FallbackChain`, `RouterModels`, `ToolRouterEnabled`, `ToolRouterConfidenceThreshold`
  - `FilterFreeTierModels`, `ProviderFreeTierSpec` loading tied to routing flag (keep `paths.provider_free_tier` for availability bootstrap only)
  - `RoutingPolicyPath`, `VirtualModelID` (keep `Semver` for version display)
  - `gatewayDoc.Routing` struct and empty-chain startup warning
  - `ShouldApplyFreeTierCatalogFilter()` (dead code)
- Delete or narrow `chimera/internal/config/gateway_fallback.go` and associated tests (`gateway_fallback_test.go`) to helpers still needed elsewhere, or remove entirely.
- Remove `routing:` block and `paths.routing_policy` from `config/gateway.example.yaml` and `config/gateway.yaml`.
- Remove or archive `config/routing-policy.yaml` and `config/routing-policy.example.yaml` if present; update any Makefile targets that reference them.
- Remove `paths.routing_policy` from gateway paths table in docs.
- Trim startup/status logging that references `VirtualModelID` as the sole virtual model (`server.go`, `status.go`, `ui_bootstrap.go`)—prefer first bootstrap VM from registry or omit when none.

**Acceptance**

- `config.LoadGatewayYAML` succeeds with no `routing:` key and no `routing_policy` path.
- `grep -R 'routing\.fallback_chain\|routing_policy' config/gateway.example.yaml` is empty.
- Gateway starts cleanly with empty operator DB and no routing YAML on disk.

**Status:** `done`

---

## Phase 4 — Tooling, tests, and docs

**Goal.** Operator docs, CLI tools, and feature records describe VM-only routing; no references imply YAML is authoritative.

**Deliverables**

- **`catalog-write-limits`:** stop calling `cataloglimits.LoadFallbackChain(gateway.yaml)`; accept explicit `--ensure` model list, catalog snapshot, or optional operator SQLite VM chains.
- Delete `cataloglimits.LoadFallbackChain` if unused after CLI update.
- Update [`configuration.md`](../configuration.md): remove global routing tables; document VM-only routing and direct upstream ids.
- Update [`operator-virtual-models.md`](../features/operator-virtual-models.md): remove "Legacy YAML" row; state bootstrap no longer imports YAML; note fresh installs start with zero VMs.
- Update [`gateway-chat-routing-pipeline.md`](../features/gateway-chat-routing-pipeline.md) if it references global YAML stages.
- Add **As-built** link in this plan's front matter when shipped; add row to [`docs/plans/README.md`](README.md) shipped table.
- Run verification commands from virtual models feature record plus:
  ```bash
  go test ./chimera/internal/config/... ./chimera/chimera-gateway/... -count=1
  go test ./chimera/chimera-gateway/internal/server/adminui/embed/embedui_test/ -run VirtualModel -count=1
  ```

**Acceptance**

- No doc tells operators to edit `routing.fallback_chain` in `gateway.yaml` for live routing.
- `make` / CLI tools do not require `routing.fallback_chain` in gateway config.
- Feature records are consistent with code; plan status → `shipped`.

**Status:** `done`

---

## Open questions

Resolved for this plan:

1. **Existing installs** — no migration; operators configure routing on virtual model cards only.
2. **Legacy `Chimera-<semver>` clients** — none in the wild; no compatibility shims required.
3. **`config/routing-policy.yaml`** — remove from repo and from `gateway.example.yaml` paths in Phase 3.

---

## References

- Code (remove or refactor):
  - `chimera/internal/config/config.go`, `gateway_fallback.go`
  - `chimera/chimera-gateway/internal/server/adminui/api/routing/`
  - `chimera/chimera-gateway/internal/server/virtualmodel_chat.go`
  - `chimera/chimera-gateway/internal/operatorstore/bootstrap.go`
  - `chimera/chimera-gateway/internal/server/runtime/runtime.go`
  - `chimera/chimera-gateway/internal/server/runtime/fallback_availability_audit.go`
  - `chimera/cmd/catalog-write-limits/main.go`
  - `chimera/internal/cataloglimits/seed.go` (`LoadFallbackChain`)
- Code (keep — VM routing):
  - `chimera/chimera-gateway/internal/virtualmodel/`
  - `chimera/chimera-gateway/internal/server/adminui/api/virtualmodels/`
  - `embed/embedui/settings/render/cards/adminVirtualModels.js`
- Docs:
  - [`configuration.md`](../configuration.md)
  - [`operator-virtual-models.md`](../features/operator-virtual-models.md)
  - [`gateway-chat-routing-pipeline.md`](../features/gateway-chat-routing-pipeline.md)
- Prior plan: [`virtual-models-operator.md`](virtual-models-operator.md) Phase 5 deliverable *"Remove dual-write to gateway.yaml from generate handlers after one release with bootstrap"*
