# Feature: Operator virtual models

| Field | Value |
|-------|-------|
| **Doc kind** | `feature-record` |
| **Areas** | Gateway runtime, operator SQLite, chat routing, settings UI |
| **Status** | `current` |
| **Introduced** | Gateway minor after unified operator cards baseline |
| **Originated from** | [`plans/virtual-models-operator.md`](../plans/virtual-models-operator.md) |
| **Related features** | [Operator settings UI](operator-settings-ui.md), [Operator provider model availability](operator-provider-model-availability.md), [Operator chat UI](operator-chat-ui.md), [Context window admission](context-window-admission.md), [Gateway chat routing pipeline](gateway-chat-routing-pipeline.md) |
| **Depends on** | Operator SQLite, broker catalog, routing policy engine, UI session auth |
| **Last updated** | See git history |

## At a glance

Operators create **virtual models** in operator SQLite—each with a client-facing `model_id` (name + version), description, enable flag, and visibility—and attach a **routing stack**: required ordered fallback chain, optional routing-policy rules, and optional tool-router block. The gateway loads enabled models into an in-memory registry, exposes them on `GET /v1/models`, and resolves `POST /v1/chat/completions` per model. Bootstrap imports the legacy single `Chimera-<semver>` stack from `gateway.yaml` on first run. Settings cards support full CRUD, generate-from-catalog, dry-run evaluate, and scoped routing-decision logs carrying `virtual_model_id`.

## Operator-visible behavior

- **Virtual model cards** on `/ui/settings` — list, create, edit metadata, enable/disable, delete.
- **Routing stack editors** — Fallback chain (required), routing policy YAML (toggleable), tool router models + confidence (toggleable). These configure the [chat routing pipeline](gateway-chat-routing-pipeline.md); cards do not execute routing themselves.
- **Generate from catalog** — Builds fallback or policy from **available** upstream models only (respects provider availability).
- **Evaluate / preview** — Dry-run policy against sample message text without sending chat.
- **Scoped logs** — Card expanded panel shows routing, fallback, and tool-router events for that model id.
- **Chat selector** — Enabled public virtual models appear in `/ui/chat` model dropdown alongside upstream ids.
- **Disabled / private** — Disabled models hidden from catalog and rejected on chat; private models visible only to creating principal (single-user desktop uses empty tenant today).

## System behavior and contracts

**Invariants**

- Virtual models persist in **operator SQLite** (`virtual_models` and attachment tables); not in `gateway.yaml` for new config.
- Client protocol unchanged: callers send one `model` string on chat/completions.
- Each VM compiles routing policy into `routing.InMemoryPolicy` at registry reload.
- Fallback walk skips unavailable models (provider availability), quota/context limits, and retriable upstream errors.
- Structured logs include `virtual_model_id` on routing resolution and fallback attempts.
- RAG remains **gateway-global** for v1 (not per-virtual-model scoped).

**Decisions**

| Topic | Decision |
|-------|----------|
| Model id format | `{Name}-{Version}` stored unique per tenant |
| Bootstrap | Import legacy `Chimera-<semver>` when SQLite empty |
| Fallback | Required non-empty chain; generate uses available catalog only |
| Routing rules | Shared policy YAML body per VM; first matching `when.min_message_chars` wins |
| Tool router | Optional; `router_models[]`, `confidence_threshold`, enable flag |
| Reload | Registry refresh after CRUD via `ReloadVirtualModels` |
| Legacy YAML | Global routing cards/API remain for migration; VM is source of truth for new work |

**Identity / auth / scoping**

- Rows scoped by `tenant_id` (empty string default desktop).
- `created_by_principal_id` tracks owner for `private` visibility.
- Chat resolves VM by exact `model_id` string match.

**Persistence**

- Migrations under `migrations/chimera-gateway/operator/` (virtual model tables).
- Attachments: fallback chain JSON, routing policy YAML blob, tool-router fields on VM row.

## Interfaces

| Surface | Detail |
|---------|--------|
| `GET /api/ui/virtual-models` | List summaries |
| `POST /api/ui/virtual-models` | Create |
| `GET /api/ui/virtual-models/{id}` | Detail + `fallback_unavailable` hints |
| `PUT /api/ui/virtual-models/{id}` | Update metadata |
| `DELETE /api/ui/virtual-models/{id}` | Delete |
| `PUT /api/ui/virtual-models/{id}/fallback` | Save fallback chain |
| `PUT /api/ui/virtual-models/{id}/routing-policy` | Save policy YAML + enable flag |
| `PUT /api/ui/virtual-models/{id}/tool-router` | Save tool router config |
| `POST /api/ui/virtual-models/{id}/routing/generate` | Generate stack from catalog |
| `POST /api/ui/virtual-models/{id}/routing/evaluate` | Dry-run policy |
| `GET /v1/models` | Includes enabled virtual models |
| `POST /v1/chat/completions` | Resolves `body.model` through VM registry |
| Log slugs | `chat.routing.resolved`, `conversation.routing.resolved`, `routing.rule.matched`, fallback attempt lines |

## Code map

| Concern | Location |
|---------|------|
| Operator store | `internal/operatorstore/` — virtual model CRUD, bootstrap |
| Runtime registry | `internal/virtualmodel/registry.go` |
| UI API | `internal/server/adminui/api/virtualmodels/` |
| Chat resolution | `internal/server/server.go`, `virtualmodel_chat.go`, `internal/chat/chat.go` |
| Settings cards | `embed/embedui/settings/render/cards/adminVirtualModels.js` |
| Routing engine | `internal/routing/`, `internal/routinggen/` |
| Generate helpers | `internal/server/runtime/fallback_availability_audit.go` |
| Tests | `internal/server/virtual_models_test.go`, `operatorstore/virtual_models_test.go`, `settings_cards_test.go` |

## Verification

```bash
go test ./chimera/chimera-gateway/internal/server/ -run VirtualModel
go test ./chimera/chimera-gateway/internal/operatorstore/ -run VirtualModel
go test ./chimera/chimera-gateway/internal/server/adminui/embed/embedui_test -run VirtualModel
```

Manual: create two VMs with different fallback chains; chat with each; confirm distinct upstream models and scoped log panels.

## Out of scope and known gaps

- Per-virtual-model RAG / workspace scope.
- Shared routing-rule definition catalog (reusable named rules across VMs) — VM stores policy YAML directly today.
- Rate-limit policy per VM.
- Deprecation/removal of legacy global `/api/ui/routing/*` YAML writers (still present).

## References

- Plan: [`plans/virtual-models-operator.md`](../plans/virtual-models-operator.md)
- Provider filtering: [Operator provider model availability](operator-provider-model-availability.md)
- Settings surface: [Operator settings UI](operator-settings-ui.md)
