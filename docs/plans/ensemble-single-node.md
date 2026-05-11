# Ensemble Single-Node Implementation Plan

| Field | Value |
|-------|-------|
| **Doc kind** | `feature-plan` |
| **Status** | `active` |
| **Owners / areas** | `internal/server`, `internal/chat`, `internal/config`, logs UI |
| **Scope** | One-machine ensemble orchestration (no peer/fleet dependency) |

## Goal

Ship practical ensemble behavior for a single-user, single-machine Chimera stack:

1. Generate multiple draft answers for one turn.
2. Synthesize to one final response.
3. Keep hosted-first fallback as default, local models as backup.

This plan explicitly avoids peer/fleet cross-host routing.

## Why this fits this project

- Current workflow is one user on one machine.
- Hosted models are preferred for quality/consistency.
- Local vLLM/llama.cpp should be available as continuity backups.
- Ensemble gives "smarter" answers without requiring multi-PC infra.

## Phase 1: Config and feature flag plumbing

1. Parse `ensemble` block from `gateway.yaml` into `Resolved`.
2. Default `enabled=false` so behavior is unchanged unless opted in.
3. Validate:
   - `drafts` in `[2..max_drafts]`
   - `max_drafts` in `[2..8]`
   - `manual_trigger` non-empty when enabled

Acceptance:
- Gateway starts unchanged when `ensemble.enabled=false`.
- Invalid ensemble config returns clear startup error.

## Phase 2: Non-streaming ensemble MVP

1. Add virtual-model-only branch in `/v1/chat/completions`:
   - if ensemble disabled: current path
   - if enabled + trigger hit: ensemble path
2. Draft phase:
   - pick `N=min(drafts, availableModels, max_drafts)`
   - issue N upstream chat calls in parallel with per-draft model selection
3. Synthesis phase:
   - build internal synthesis prompt from draft outputs
   - run one final upstream call
4. Return one final completion payload to client.

Acceptance:
- Non-streaming requests return one answer with ensemble metadata in logs.
- Draft failures degrade gracefully (try remaining drafts; fail only when no viable draft path remains).

## Phase 3: Streaming semantics

1. Define SSE contract for ensemble turns:
   - no draft-token streaming to clients in MVP
   - stream only synthesis phase output
2. Define error events:
   - draft-phase failure before synthesis
   - synthesis failure after successful drafts
3. Keep behavior stable for non-ensemble requests.

Acceptance:
- Streaming clients receive consistent SSE events and terminal behavior.

## Phase 4: Triggering and confidence gates

1. Manual trigger: `//deep` (trimmed before upstream).
2. Optional auto-trigger based on simple heuristics:
   - user message length
   - optional tool-router confidence or retry pressure
3. Respect virtual model only.

Acceptance:
- `//deep` reliably triggers ensemble on virtual model.
- Direct upstream model ids bypass ensemble.

## Phase 5: Observability and operator UX

1. Add structured slugs:
   - `ensemble.start`
   - `ensemble.draft.request`
   - `ensemble.draft.result`
   - `ensemble.synthesis.request`
   - `ensemble.synthesis.result`
   - `ensemble.error`
2. Show ensemble participation in logs UI summary.
3. Add counters: turns with ensemble, avg draft count, synthesis failure rate.

Acceptance:
- Operators can answer: "Was this turn ensembled? how many drafts? which model won?"

## Out of scope for this plan

- Peer/fleet cross-host orchestration.
- Human escalation UX.
- Queueing/scheduling across many tenants.

## Suggested default profile

- `ensemble.enabled: false` initially.
- Enable for manual trigger first (`//deep`).
- Keep hosted-first routing chain; local models remain late fallback backups.
