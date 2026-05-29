# Feature: Context window admission (chat routing)

| Field | Value |
|-------|-------|
| **Doc kind** | `feature-record` |
| **Areas** | Gateway chat routing, provider limits, virtual model fallback |
| **Status** | `partial` |
| **Introduced** | Gateway patch after v0.2 routing baseline (2026-05) |
| **Originated from** | [`plans/context-window-admission.md`](../plans/context-window-admission.md) |
| **Related features** | [Operator virtual models](operator-virtual-models.md), [Operator log message registry](operator-log-message-registry.md), [Gateway chat routing pipeline](gateway-chat-routing-pipeline.md) |
| **Depends on** | `provider-model-limits.yaml`, token estimator, live catalog snapshot |
| **Last updated** | See git history |

## At a glance

Before calling upstream on each virtual-model fallback attempt, the gateway can **deny** models whose context window or body-size cap would be exceeded—similar to existing TPM/RPM guards but without metrics I/O. Limits come from `provider-model-limits.yaml` (`context_window`, `max_prompt_tokens`, `max_body_bytes`, safety factors); YAML values win over live catalog `context_length` overlay when set. Upstream `request_too_large` and `context_length_exceeded` responses trigger **fallback retry** to the next chain entry instead of terminating the client request. Operator logs emit `provider_limits` skips with `reason: context_window`.

## Operator-visible behavior

- **Transparent fallback** — Large prompts (e.g. IDE clients with big context + RAG) skip tight-window Groq models and continue to larger-context or local Ollama entries when configured.
- **Settings logs** — Context skip lines appear in conversation and gateway scoped streams (registry slug family `provider_limits` / context_window reason).
- **No separate UI** — Limits edited via `provider-model-limits.yaml` and `make catalog-write-limits` seeding; not exposed on settings cards today.

## System behavior and contracts

**Invariants**

- Admission runs on the **final proxied body** after RAG injection and token witness.
- Formula: `est_prompt + max_tokens_reserve ≤ effective_window` (with safety factor); optional byte cap on marshalled JSON.
- Context checks do **not** require metrics SQLite (unlike TPM).
- `shouldRetryVirtualModelFallback` treats HTTP 400 + `request_too_large` / `context_length_exceeded` as retriable.
- YAML `context_window` overrides catalog overlay; overlay fills when YAML omits field.

**Decisions**

| Topic | Decision |
|-------|----------|
| Schema version | `provider-model-limits.yaml` schema v2 |
| Catalog overlay | `providerlimits.OverlayCatalogContext` via gateway `CatalogSnapshot.ModelContext` |
| Seeding | `make catalog-write-limits` / `cmd/catalog-write-limits` from `catalog-available.snapshot.yaml` |
| Deny behavior | Skip model in fallback loop; log warning; try next candidate |
| TPM vs context | Independent checks; both can skip same model |

**Persistence**

- Limits file on disk (`paths.provider_model_limits`); catalog snapshot polled in memory.

## Interfaces

| Surface | Detail |
|---------|--------|
| Config | `config/provider-model-limits.yaml` — `context_window`, `max_prompt_tokens`, `max_body_bytes`, `context_safety_factor` |
| Runtime | `internal/providerlimits` — `ResolveWithCatalog`, `EffectiveFor`, admission in chat path |
| Catalog | `internal/server/catalog/availablemodels.go` — `ModelContext` map |
| Tooling | `chimera/cmd/catalog-write-limits`, Make target for seeding |
| Log slug | `gateway.provider_limits.context_window` (via registry) |

## Code map

| Concern | Location |
|---------|------|
| Limits resolve | `chimera/internal/providerlimits/resolve.go`, `overlay.go`, `guard.go` |
| Chat admission | `chimera/chimera-gateway/internal/chat/chat.go` |
| Fallback retry | `shouldRetryVirtualModelFallback`, `WithVirtualModelFallback` |
| Catalog context | `internal/server/catalog/availablemodels.go` |
| Tests | `chat_limits_test.go`, `providerlimits/overlay_test.go` |
| Docs | [`configuration.md`](../configuration.md), [`reference/tokencount-notes.md`](../reference/tokencount-notes.md) |

## Verification

```bash
go test ./chimera/chimera-gateway/internal/chat/ -run 'Context|request_too_large|Fallback'
go test ./chimera/internal/providerlimits/...
```

Manual: send oversized prompt through VM with mixed Groq + Ollama chain; confirm Groq context skips in logs and Ollama attempt succeeds.

## Out of scope and known gaps

- **Phase 4 (plan)** — Live catalog overlay marked `in progress`; overlay **is implemented** for unset YAML caps but ongoing catalog accuracy work remains (vendor `context_length` can overstate limits — use `max_prompt_tokens` override).
- Settings UI to edit context caps per model.
- Persisting context skip counts in metrics DB.
- Automatic `max_prompt_tokens` overrides from observed upstream 400s.

## References

- Plan: [`plans/context-window-admission.md`](../plans/context-window-admission.md)
- Example limits: [`config/provider-model-limits.example.yaml`](../../config/provider-model-limits.example.yaml)
- Virtual model fallback: [Operator virtual models](operator-virtual-models.md)
