# Plan: Per-provider model availability

| Field | Value |
|-------|-------|
| **Doc kind** | `feature-plan` |
| **Owners / areas** | Gateway runtime, operator SQLite, admin UI (`/ui/settings`), routing, virtual models |
| **Status** | `done` |
| **Targets** | Gateway operator settings, `GET /v1/models`, virtual-model routing |
| **Last updated** | See git history |
| **Supersedes / superseded by** | Replaces `routing.filter_free_tier_models` and YAML-only catalog filtering; builds on [`embedui-dynamic-provider-cards.md`](embedui-dynamic-provider-cards.md) and [`virtual-models-operator.md`](virtual-models-operator.md) |
| **As-built** | [`docs/features/operator-provider-model-availability.md`](../features/operator-provider-model-availability.md) |

**Behavioral source of truth:** the [feature record](../features/operator-provider-model-availability.md) describes as-built behavior; this plan is delivery history.

## At a glance

Operators need fine-grained, **tenant-scoped** control over which upstream models the gateway exposes. Each provider card on `/ui/settings` should let the operator mark individual broker-reported models as **available** or **unavailable**, persist those choices in operator SQLite (bootstrapped from `provider-free-tier.yaml`), and have the gateway immediately honor them in `GET /v1/models`, “Generate from catalog”, and virtual-model fallback chains—with runtime warnings when a saved chain references a model that is no longer available.

| Phase | Outcome | Status |
|-------|---------|--------|
| [Phase 1 — Data model and persistence](#phase-1--data-model-and-persistence) | Tenant-scoped availability in operator SQLite, bootstrapped from `provider-free-tier.yaml` | `done` |
| [Phase 2 — Gateway catalog filter and routing skips](#phase-2--gateway-catalog-filter-and-routing-skips) | `GET /v1/models` and chat routing respect availability; skipped models log warnings | `done` |
| [Phase 3 — Operator HTTP API](#phase-3--operator-http-api) | Session-authenticated read/write of per-provider model availability | `done` |
| [Phase 4 — Settings UI edit mode](#phase-4--settings-ui-edit-mode) | Provider cards support configure/save/cancel, per-model toggles, and free-tier assist | `done` |
| [Phase 5 — Virtual model integration](#phase-5--virtual-model-integration) | “Generate from catalog” and routing evaluation use only available models | `done` |

---

## Background

**Problem.** Today the gateway exposes every model chimera-broker returns from its merged catalog (optionally intersected with a global free-tier allowlist via `routing.filter_free_tier_models` in `gateway.yaml`). Operators cannot hide specific models per tenant without editing YAML or using a coarse global filter. Virtual-model fallback chains and routing rules can reference models the operator does not want exposed, and there is no per-provider UI to curate the catalog.

**Desired behavior.**

| Concern | Today | Target |
|---------|-------|--------|
| **Model source of truth (upstream)** | chimera-broker returns full provider model lists after keys/URL are configured | Unchanged — broker still returns the full list |
| **Gateway catalog (`GET /v1/models`)** | All broker models (+ virtual models), optionally filtered by `provider-free-tier.yaml` | Only models marked **available** for the request tenant; unavailable models omitted entirely |
| **Settings provider cards** | Usage table (requests/errors) from metrics + broker snapshot; key/URL editing | Same table, plus **Configure** → edit mode with per-model availability toggles |
| **Free-tier assist** | Global `routing.filter_free_tier_models` flag + YAML intersection at generate/catalog time | Per-provider button in edit mode (Groq/Gemini only): mark free-tier models available, non-free-tier unavailable, using `config/provider-free-tier.yaml`; **Ollama no-op** (all local models treated as free tier) |
| **Generate from catalog** | Uses live broker list (± global free-tier filter) | Uses **available** models only |
| **Virtual model fallback at runtime** | Walks saved chain; skips on quota/context/HTTP errors | Also **skips unavailable** models; emits scoped warning in virtual-model logs |
| **Persistence** | Free-tier list is YAML on disk; fallback chains in operator SQLite | Per-tenant, per-model availability in **operator SQLite** (survives restarts); **bootstrap** seeds from `config/provider-free-tier.yaml` |

**Related docs:** [`configuration.md`](../configuration.md) (`provider-free-tier.yaml`), [`embedui-dynamic-provider-cards.md`](embedui-dynamic-provider-cards.md), [`virtual-models-operator.md`](virtual-models-operator.md), [`operator-message-registry.md`](operator-message-registry.md).

**Out of scope (for this plan):** Changing how chimera-broker discovers models; rewriting `provider-free-tier.yaml` generation (`make catalog-free`); provider rate-limit enforcement (`provider-model-limits.yaml`); backward compatibility with `routing.filter_free_tier_models`; tagging/grouping/cost filters beyond reserving schema space.

---

## Decisions

| Topic | Decision |
|-------|----------|
| Default for new broker models | **Opt-out** — models newly reported by chimera-broker are **available** until the operator marks them unavailable (no stored row ⇒ available). |
| Legacy `routing.filter_free_tier_models` | **Remove** — do not keep as an additional filter or migration path; operator availability is the sole catalog filter. |
| Ollama free-tier assist | **No-op** — all Ollama models are assumed free tier; hide or disable the free-tier button on the Ollama card (no availability changes when pressed). |
| Tenant scope | **Per `tenant_id`** — availability rows are scoped like virtual models; UI session and `GET /v1/models` resolve availability for the authenticated tenant (API token `tenant_id` / UI session principal). |
| Bootstrap | **Seed from YAML** — on first migration, populate each tenant’s availability from `config/provider-free-tier.yaml` (paths from `gateway.yaml` → `paths.provider_free_tier`): matching broker model ids → available, non-matching → unavailable; Ollama models → all available. |

---

## Design overview

### Availability semantics

- **Available:** model appears in gateway `GET /v1/models` for the tenant, is eligible for “Generate from catalog”, and is tried during fallback/routing evaluation.
- **Unavailable:** model is hidden from the tenant’s gateway catalog and skipped at runtime (with a warning) when referenced by a virtual model’s saved fallback chain or routing policy.
- **Default for newly discovered models:** when broker reports a model id with no stored row for that tenant, treat it as **available** until the operator marks it otherwise.

### Data model (extensible)

Add operator SQLite tables under `migrations/chimera-gateway/operator/`:

```sql
-- provider_model_config: one row per tenant + provider
CREATE TABLE provider_model_config (
  tenant_id TEXT NOT NULL,
  provider_id TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  metadata_json TEXT NOT NULL DEFAULT '{}',   -- future: tags, groups, cost tier
  PRIMARY KEY (tenant_id, provider_id)
);

-- provider_model_availability: sparse overrides per tenant; absent row => default available
CREATE TABLE provider_model_availability (
  tenant_id TEXT NOT NULL,
  provider_id TEXT NOT NULL,
  model_id TEXT NOT NULL,                   -- full unified id, e.g. groq/llama-3.1-8b-instant
  available INTEGER NOT NULL,               -- 1 = available, 0 = unavailable
  metadata_json TEXT NOT NULL DEFAULT '{}', -- future per-model notes/tags
  updated_at TEXT NOT NULL,
  PRIMARY KEY (tenant_id, provider_id, model_id)
);

CREATE INDEX idx_provider_model_avail_tenant ON provider_model_availability (tenant_id);
```

Store APIs live in `chimera/chimera-gateway/internal/operatorstore/` (new methods: `GetProviderModelAvailability(ctx, tenantID, providerID)`, `SetProviderModelAvailability`, bulk replace per tenant+provider). Load into a per-tenant in-memory snapshot on the gateway runtime (keyed by `tenant_id`, same family as virtual-model registry reload) and refresh after successful UI saves.

**Bootstrap migration** (`operatorstore.BootstrapProviderModelAvailability` or inline in migration runner): for each existing `tenant_id` in operator SQLite (and a default tenant when none exist yet), read `provider-free-tier.yaml` from the resolved config path, match unified model ids per provider prefix, write availability rows. Ollama: skip YAML matching; do not write unavailable rows (implicit all-available). Re-run is idempotent (upsert by primary key).

**Free-tier assist** (edit-mode UI only, Groq/Gemini): reads the same YAML via `providerfreetier.Spec`. For the selected provider, intersect broker model ids with the spec: matches → available, non-matches → unavailable. Operator can override individual rows before save. **Ollama:** button hidden or disabled; handler returns success with unchanged draft and an operator-facing note that Ollama is always free tier.

### Gateway filter pipeline

Apply tenant-scoped operator availability **after** broker catalog fetch; **remove** `FilterOpenAIModelDataByFreeTier` from catalog and routing generate paths:

```
broker /v1/models  →  tenant availability filter  →  GET /v1/models response (per auth tenant)
                      ↘  catalog snapshot (available ids per tenant)
```

Touch points:

| Location | Change |
|----------|--------|
| `internal/server/server.go` `handleV1Models` | Filter `data` by availability snapshot |
| `internal/server/catalog/availablemodels.go` `BuildSnapshot` | Same filter so health/auditors see the operator-facing catalog |
| `internal/server/catalog/models_filter.go` | Add `FilterOpenAIModelDataByAvailability`; remove free-tier filter from catalog/routing (YAML retained for assist + bootstrap only) |
| `internal/config/config.go` / `gateway.yaml` | Remove or ignore `routing.filter_free_tier_models` |
| `internal/routinggen` + `adminui/api/routing/handlers.go` `computeRoutingDraft` | Pool = available models only |
| `adminui/api/virtualmodels/handlers.go` `handleGeneratePOST` | Same pool semantics |
| `internal/chat/chat.go` `WithVirtualModelFallback` + routing policy pickers | Skip unavailable ids before attempt; log warning with virtual model scope |

**Runtime skip logging:** register a new operator message slug (e.g. `routing.model.unavailable_skipped`) in `internal/operatorcopy/messages.yaml`, emitted at Warn with fields `virtual_model_id`, `upstream_model`, `provider_id`, `reason=operator_unavailable`. Surface in virtual-model scoped log panels on settings/logs UI.

### Operator HTTP API

Session-authenticated routes (pattern: existing `/api/ui/provider/{id}/keys`):

| Method | Path | Purpose |
|--------|------|---------|
| `GET` | `/api/ui/providers/{provider_id}/models` | Broker model ids + stored availability for session tenant + 24h metrics counts |
| `PUT` | `/api/ui/providers/{provider_id}/models` | Replace availability map for session tenant + provider; reload tenant snapshot |
| `POST` | `/api/ui/providers/{provider_id}/models/apply-free-tier` | Compute draft from `provider-free-tier.yaml` (Groq/Gemini); Ollama no-op; does not save until PUT |

Extend `GET /api/ui/state` `providers.{id}` with summary fields: `models_available_count`, `models_unavailable_count`, `models_configured` (boolean: operator has saved any override for this provider).

### Settings UI (`/ui/settings`)

Extend provider cards in `settings/render/cards/adminProvider.js` and handlers in `settings/handlers/admin.js`:

| Mode | UI |
|------|-----|
| **Read-only (default)** | Current layout: intro, usage table (all broker models + metrics), keys/URL, scoped log |
| **Edit (Configure pressed)** | Visual edit affordance on card (border/badge); usage table gains **Available** column with checkbox/toggle per row; toolbar: **Apply free-tier defaults** (Groq/Gemini only), **Save**, **Cancel**; free-tier button hidden outside edit mode and on Ollama |

Draft state mirrors existing virtual-model edit patterns (`adminVirtualModels.js` configure/save/cancel). Save calls PUT API; cancel discards draft. After save, re-fetch state and broker provider snapshot so model count chips reflect available-only counts.

Component gallery / goja tests: update fixtures under `embedui_test/`.

---

## Phase 1 — Data model and persistence

**Goal.** Operator SQLite can store per-provider, per-model availability with room for future metadata, and the gateway can load a read-only snapshot at startup.

**Deliverables**

- Migration `migrations/chimera-gateway/operator/000003_provider_model_availability.sql` (next sequence number) + bootstrap hook that seeds from `config/provider-free-tier.yaml`.
- `operatorstore` CRUD: get merged view for tenant+provider (broker ids + overrides), bulk upsert, clear tenant+provider overrides.
- `BootstrapProviderModelAvailability`: for each tenant, apply YAML free-tier matching (Ollama all-available); idempotent upsert.
- Runtime wiring in `internal/server/runtime/runtime.go`: load per-tenant snapshots after operator store open; expose `ProviderModelAvailability(tenantID)` accessor.
- Unit tests for store round-trip, bootstrap from fixture YAML, default-available for unknown model ids, and tenant isolation.

**Acceptance**

- After migration/bootstrap, Groq/Gemini availability matches `provider-free-tier.yaml`; Ollama models all available.
- Broker adds a new model id with no row for a tenant: treated as available.
- After writing `{tenant, groq, foo: unavailable}`, that tenant’s snapshot reports `foo` unavailable; other tenants unchanged.

**Status:** `done`

---

## Phase 2 — Gateway catalog filter and routing skips

**Goal.** The public model catalog and chat routing never use operator-marked unavailable models; skipped chain entries produce scoped warnings.

**Deliverables**

- `FilterOpenAIModelDataByAvailability` applied in `handleV1Models` (tenant from bearer token) and `catalog.BuildSnapshot` (per-tenant snapshots).
- Remove `FilterOpenAIModelDataByFreeTier` from `handleV1Models`, `BuildSnapshot`, and routing generate.
- Shared helper `AvailableModelIDs(tenantSnapshot, brokerIDs) []string` used by routing generate paths.
- Fallback walker (`WithVirtualModelFallback`) and routing policy model resolution skip unavailable ids **before** upstream attempt.
- Structured log slug + operator copy entry for skipped-unavailable events.
- Tests: `models_test.go`, `chat` fallback tests, catalog snapshot tests with mixed availability.

**Acceptance**

- `GET /v1/models` omits unavailable ids; virtual model ids still listed when enabled.
- Chat against a virtual model whose chain contains an unavailable model tries the next available entry and logs one warning per skipped id (deduped per request).
- Catalog snapshot `ModelIDs` matches filtered catalog (auditors see operator-facing list).

**Status:** `done`

---

## Phase 3 — Operator HTTP API

**Goal.** Authenticated operators can read and write provider model availability through the admin BFF; saves reload the runtime snapshot immediately.

**Deliverables**

- Handlers under `internal/server/adminui/api/providers/` (or extend existing package): GET/PUT models, POST apply-free-tier (draft only); all scoped to UI session `tenant_id` / principal.
- Request/response types in `internal/operatorapi/` (e.g. `ProviderModelsResponse`, `ProviderModelsUpdateRequest`).
- `GET /api/ui/state` enrichment for provider summary counts (session tenant).
- HTTP tests mirroring `ui_broker_providers_http_test.go` patterns, including tenant isolation.

**Acceptance**

- PUT persists to SQLite for session tenant; subsequent GET returns saved toggles; other tenants unaffected.
- POST apply-free-tier returns proposed map without DB write until PUT; Ollama returns unchanged draft with explanatory note.
- Invalid provider id → 404; empty body / malformed json → 400.

**Status:** `done`

**Goal.** Each provider card on `/ui/settings` supports configure/save/cancel edit mode with per-model toggles and a free-tier assist button visible only while editing.

**Deliverables**

- Provider card toolbar: **Configure** (enter edit), **Save**, **Cancel**; clear edit-mode styling.
- Model table: **Available** checkbox column (edit mode only); merge broker `model_ids` with draft availability from GET API.
- **Apply free-tier defaults** button (Groq/Gemini cards only) → POST apply-free-tier → update draft toggles in UI (no save until operator confirms).
- Handler wiring in `admin.js`; draft state in `settings_app.js` (per-provider map, parallel to key drafts).
- CSS in `settings.css` for edit-mode card chrome and toggle column.
- Update `adminProviderModelCount` / summary chips to count **available** models when configuration exists.
- Goja / component tests for edit flow and free-tier button visibility.

**Acceptance**

- Operator can mark models unavailable, save, reload page, and see toggles restored.
- Free-tier button marks Groq/Gemini models per YAML patterns; manual toggle after assist persists on save; Ollama card shows no free-tier button.
- Configure button not shown (or disabled) until provider has credentials (same gate as usage table).

**Status:** `done`

---

## Phase 5 — Virtual model integration

**Goal.** Virtual-model “Generate from catalog” and runtime routing stay consistent with operator availability choices without requiring operators to regenerate chains manually.

**Deliverables**

- Update `handleGeneratePOST` (`virtualmodels/handlers.go`) and global `computeRoutingDraft` to filter pool through tenant availability snapshot only.
- Virtual-model fallback UI (`adminVirtualModels.js`): indicate chain entries that are currently unavailable (badge/warning in read-only table); optional “prune unavailable” helper (stretch — not required if runtime skip + logs suffice).
- Register catalog auditor (optional): warn when saved fallback chain references unavailable models (extends `RegisterCatalogAuditor` hook in `catalog/availablemodels.go`).
- Docs update in `configuration.md`: document tenant-scoped operator availability; remove `routing.filter_free_tier_models` from operator workflow.

**Acceptance**

- Generate from catalog for a virtual model never inserts unavailable model ids.
- Existing saved chains continue to work but skip unavailable entries at runtime with scoped warnings.
- Changing availability and saving does not require editing virtual models for the gateway to behave correctly.

**Status:** `done`

---

## References

- Code: `chimera/chimera-gateway/internal/server/catalog/`, `internal/server/server.go` (`handleV1Models`), `internal/server/adminui/embed/embedui/settings/render/cards/adminProvider.js`, `internal/server/adminui/api/routing/handlers.go`, `internal/server/adminui/api/virtualmodels/handlers.go`, `internal/chat/chat.go`, `chimera/chimera-gateway/internal/operatorstore/`, `chimera/internal/providerfreetier/`, `config/provider-free-tier.yaml`
- Docs: [`configuration.md`](../configuration.md), [`version-v0.3.md`](../version-v0.3.md)
- Related plans: [`embedui-dynamic-provider-cards.md`](embedui-dynamic-provider-cards.md), [`virtual-models-operator.md`](virtual-models-operator.md), [`unified-logs-operator-shell.md`](unified-logs-operator-shell.md)
