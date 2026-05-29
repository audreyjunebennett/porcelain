# Feature: Operator provider model availability

| Field | Value |
|-------|-------|
| **Doc kind** | `feature-record` |
| **Areas** | Gateway catalog, operator SQLite, settings provider cards, virtual models |
| **Status** | `current` |
| **Introduced** | Gateway minor after virtual models baseline |
| **Originated from** | [`plans/provider-model-availability.md`](../plans/provider-model-availability.md) |
| **Related features** | [Operator settings UI](operator-settings-ui.md), [Operator virtual models](operator-virtual-models.md), [Operator chat UI](operator-chat-ui.md) |
| **Depends on** | Broker catalog snapshot, operator SQLite, UI session tenant |
| **Last updated** | See git history |

## At a glance

Each operator tenant can mark individual broker-reported upstream models **available** or **unavailable** in operator SQLite. The gateway filters `GET /v1/models`, virtual-model **Generate from catalog**, and runtime fallback chains to **available** models only; skipped unavailable entries emit scoped warnings. Provider cards on `/ui/settings` expose **Configure** edit mode with per-model toggles and a **Apply free tier** assist (Groq/Gemini) seeded from `config/provider-free-tier.yaml`. New broker models default to **available** when no row exists.

## Operator-visible behavior

- **Provider card chips** — Show available vs unavailable model counts from `/api/ui/state`.
- **Configure** — Enters edit mode on a provider card; toggles per model id; **Save** / **Cancel**.
- **Apply free tier** (Groq/Gemini) — Sets availability from YAML allowlist intersection; **Ollama** button is no-op (all local models treated available).
- **Chat model list** — Unavailable models disappear from `/v1/models` for the tenant.
- **Virtual model warnings** — VM detail API returns `fallback_unavailable` when saved chain references unavailable ids; runtime skips them with log warnings.

## System behavior and contracts

**Invariants**

- **Upstream source of truth** — chimera-broker still returns full provider catalogs; gateway filters at expose/route time.
- **Opt-out default** — No SQLite row ⇒ model is **available**.
- **Tenant scoped** — Rows keyed by `(tenant_id, provider_id, model_id)`; UI session `tenant_id` matches chat token tenant.
- **Bootstrap** — First migration seeds from `provider-free-tier.yaml` when table empty (matching ids available, others unavailable; Ollama all available).
- **Reload** — `ReplaceProviderModelAvailability` triggers `ReloadProviderModelAvailability` on runtime registry.

**Decisions**

| Topic | Decision |
|-------|----------|
| Legacy `routing.filter_free_tier_models` | Superseded for catalog filtering; YAML flag may still exist for old UI paths but availability is primary |
| Generate from catalog | Uses available models only |
| Runtime fallback | Skips unavailable with warning slug (e.g. catalog fallback unavailable) |
| Ollama free-tier assist | Hidden/disabled — all models available |
| Persistence | `provider_model_config` + `provider_model_availability` tables |

**Persistence**

- Migrations under `migrations/chimera-gateway/operator/`.
- Bootstrap: `BootstrapProviderModelAvailability` on catalog refresh when no rows.

## Interfaces

| Surface | Detail |
|---------|--------|
| `GET /api/ui/providers/{provider_id}/models` | Broker ids merged with stored availability |
| `PUT /api/ui/providers/{provider_id}/models` | Body `{ "models": { "model_id": true/false, … } }` |
| `POST /api/ui/providers/{provider_id}/models/apply-free-tier` | Groq/Gemini YAML assist |
| `GET /v1/models` | Tenant-filtered catalog |
| `GET /api/ui/state` | Provider cards include availability counts |
| Config seed | `paths.provider_free_tier` → `config/provider-free-tier.yaml` |

## Code map

| Concern | Location |
|---------|------|
| Store | `internal/operatorstore/provider_models.go`, `provider_models_bootstrap.go` |
| Runtime registry | `internal/providermodels/registry.go` |
| UI API | `internal/server/adminui/api/providers/provider_models.go` |
| Catalog filter | `internal/server/catalog/availablemodels.go`, `handleV1Models` |
| Settings UI | `embed/embedui/settings/handlers/admin.js`, provider card renderers |
| VM integration | `api/virtualmodels/handlers.go` (`fallback_unavailable`), chat fallback loop |
| Tests | `ui_virtual_model_generate_test.go`, provider handler tests |

## Verification

```bash
go test ./chimera/chimera-gateway/internal/server/ -run ProviderModel
go test ./chimera/chimera-gateway/internal/operatorstore/ -run ProviderModel
```

Manual: mark a fallback-chain model unavailable; confirm it disappears from chat models and VM card shows warning; chat still succeeds via next fallback entry.

## Out of scope and known gaps

- Changing broker discovery or `make catalog-free` generation.
- Provider rate limits (`provider-model-limits.yaml` TPM/RPM — separate from availability).
- Tags/groups/cost tiers in `metadata_json` (schema reserved).
- Removing legacy free-tier toggle from global routing cards entirely.

## References

- Plan: [`plans/provider-model-availability.md`](../plans/provider-model-availability.md)
- Virtual models: [Operator virtual models](operator-virtual-models.md)
- Free-tier YAML: [`config/provider-free-tier.yaml`](../../config/provider-free-tier.yaml)
