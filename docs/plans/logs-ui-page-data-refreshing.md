# Plan: Logs UI Page Data Refreshing

| Field | Value |
|-------|-------|
| **Doc kind** | `refactor-plan` |
| **Owners / areas** | Gateway embed UI, operator logs (`/ui/logs`) |
| **Status** | `draft` |
| **Targets** | Reliable card interaction, stable admin forms, low-flicker live feed |
| **Last updated** | 2026-05-19 |
| **Supersedes / superseded by** | Follows [`logs-ui-maintainability.md`](logs-ui-maintainability.md); complements [`unified-logs-operator-shell.md`](unified-logs-operator-shell.md) |

## At a glance

Operators on `/ui/logs` should open cards on the first click, edit provider API keys without losing focus, and see live log updates without the whole panel flashing. Today the summarized feed replaces `#panel-summarized` with a full `innerHTML` rebuild on almost every log line and on a 12s admin poll. This plan fixes the worst pain in small PRs, then moves to per-card patches, then to a testable view model with a diff/patch layer (Tier 3).

| Phase | Outcome | Status |
|-------|---------|--------|
| [Phase 1 — Interaction-safe rebuilds](#phase-1--interaction-safe-rebuilds) | Typing and card clicks are not fighting background refreshes | `done` |
| [Phase 2 — Poll-path card patching](#phase-2--poll-path-card-patching) | Metrics, gateway, admin, and provider cards update without wiping the panel | `done` |
| [Phase 3 — Live-log dirty cards](#phase-3--live-log-dirty-cards) | New log lines update only affected cards, coalesced per frame | `done` |
| [Phase 4 — Summarized view model](#phase-4--summarized-view-model) | Pure `buildSummarizedModel()` drives rendering; full rebuild is rare | `done` |
| [Phase 5 — Patch engine](#phase-5--patch-engine) | Stable card DOM + structural patch ops; Tier 3 complete | `done` |

---

## Background

The summarized logs UI (`embedui/logs/app/summarizedFeed.js`) renders operator cards as HTML strings and assigns them via `psu.innerHTML = renderSummarizedUnified()`. Scroll position, open `<details>` ids, and some nested scroll/evlog state are restored after each rebuild, but **focus**, **in-progress form values**, and **click timing** are not reliably preserved.

**Symptoms**

- Cards sometimes need two clicks to expand (race between native `<details>` toggle and debounced full rebuild from SSE).
- Provider API key fields lose focus and content when `/api/ui/state` polls every 12s or when new log lines arrive (~80ms debounced rebuild).
- Visual glitching from full-panel reflow plus scroll correction in `requestAnimationFrame`.

**Existing good patterns** (extend, do not replace blindly)

- `patchGatewayUsageMetricsCard()`, `patchGatewayOverviewCard()`, `patchChimeraBrokerProviderHealthStrip()` — single-element `replaceChild` with open/scroll preserved.
- `summarizedEvlogInteractionBlocksRebuild()` + `scheduleDeferredSummarizedRefresh()` — defers rebuild while editing evlog search/filters and some YAML fields.
- Raw log mode: `scheduleRawLogsDomFlush()` in `transport/streaming.js` — one DOM update per animation frame.

**Related docs:** [`embedui/logs/README.md`](../../chimera/chimera-gateway/internal/server/adminui/embed/embedui/logs/README.md), [`logs-ui-maintainability.md`](logs-ui-maintainability.md), [`embedui-event-log-panel.md`](embedui-event-log-panel.md).

**Non-goals**

- Rewriting the UI in a SPA framework.
- Changing `/api/ui/*` response shapes (client adapts only).
- Browser/Playwright CI in early phases (goja fixtures remain the default test loop).

---

## Phase 1 — Interaction-safe rebuilds

**Goal.** Operators can expand cards once and type in admin fields without background refresh stealing focus or clearing inputs.

**Deliverables**

- Broaden `summarizedEvlogInteractionBlocksRebuild()` (rename if helpful, e.g. `summarizedPanelInteractionBlocksRebuild`) to return true when `document.activeElement` is any `input`, `textarea`, or `select` inside `#panel-summarized`, not only evlog/YAML ids.
- After `pointerdown` on `details.sum-card > summary`, set a short suppression window (mirror `sumEvlogPointerSuppressedUntil`, ~400–500ms) so `refreshSummarizedPanel` defers while a card toggle is in flight.
- Persist in-flight **provider key** and **Ollama URL** drafts in `ctx` (same pattern as `routingPolicyDraft` / `adminUserDrafts`): wire `input` listeners in `wireHandlers.js`, read drafts when rendering `adminProvider.js` cards.
- Document the interaction contract in `embedui/logs/README.md` (“when rebuild is deferred / what is restored”).
- Goja or shell test: render provider card HTML with draft ctx populated; assert password field `value` attribute reflects draft (no browser required).

**Acceptance**

- With live SSE traffic, expanding a collapsed card succeeds on the first click in manual smoke test.
- Typing in `admin-groq-key` / `admin-gemini-key` / `admin-ollama-url` for 30+ seconds (spanning at least one admin poll interval) does not clear the field or drop focus.
- No regression to existing evlog deferral (search/filter/YAML).

**Status:** `done`

---

## Phase 2 — Poll-path card patching

**Goal.** Periodic API refreshes update only the cards whose data changed, matching the gateway metrics patch pattern.

**Deliverables**

- Replace `syncAdminStatePolling` full `refreshSummarizedPanel()` with targeted patches:
  - `patchAdminProviderCard(providerId)` for groq/gemini/ollama (metrics chips, key list, availability; preserve `open`, scroll, drafts).
  - `patchAdminUsersCard()` (token list; preserve drafts).
  - Optional: `patchAdminRoutingCard()`, `patchAdminFallbackCard()`, `patchAdminRouterModelsCard()` when only `adminStateCache` changed and editing flags are false.
- On patch miss (card id not in DOM), fall back to `scheduleStoryRebuild()` — not synchronous full rebuild from poll handler.
- Extract shared helper: `replaceCardById(id, htmlBuilder, { preserveOpen, preserveScrollSelectors })` used by gateway/admin patches.
- Extend `logs_components_test.go` with fixtures for patched card HTML fragments (stable ids, key chips).

**Acceptance**

- With summarized view open and no new log lines, admin poll (12s) does not replace `#panel-summarized` `innerHTML` (verify via temporary debug flag or DOM mutation observation in manual test).
- Provider key counts and gateway overview still match `/api/ui/state` after poll.
- Existing `patchGatewayUsageMetricsCard` / `patchGatewayOverviewCard` behavior unchanged.

**Status:** `done`

---

## Phase 3 — Live-log dirty cards

**Goal.** Incoming log lines update the feed incrementally and coalesced, instead of rebuilding the entire summarized panel on every SSE event.

**Deliverables**

- Introduce `ctx.summarizedDirtyCardIds` (Set or object map) and `scheduleSummarizedDirtyFlush()`:
  - Coalesce to one flush per `requestAnimationFrame` (same idea as `scheduleRawLogsDomFlush`).
  - Replace per-line `scheduleStoryRebuild()` from `appendLine` / `prependHistoricalEntries` with “mark dirty + schedule flush”.
- Implement dirty routing from a new log entry (pure function, goja-tested):
  - Conversation card id(s) from `conversation_id` / request-id / index-run correlation.
  - Service bucket cards (`chimera-gateway`, `chimera-broker`, etc.) from `service` / source.
  - Static admin cards only when scoped filter matches (reuse provider-scoped logic from `adminProvider.js`).
- `flushSummarizedDirtyCards()`:
  - For each dirty id, call `patchCard(id)` (Phase 2 helper) with a card-specific HTML builder.
  - If dirty set is large (threshold TBD, e.g. >30% of cards or >N cards), fall back to full `refreshSummarizedPanel()` once.
- Keep full rebuild for: initial load, view-mode switch, backfill that changes `minLoadedSeq` materially, explicit admin actions that restructure drafts (unchanged call sites).
- Tune debounce: remove or raise the 80ms `scheduleStoryRebuild` timer in favor of rAF batching.

**Acceptance**

- Under steady gateway logging, `#panel-summarized` `innerHTML` is not assigned on every line (observable in devtools or debug hook).
- Conversation and service cards still show new events within one frame of batch flush.
- Open cards and panel scroll remain stable when not interacting (Phase 1 guards still apply).

**Status:** `done`

---

## Phase 4 — Summarized view model

**Goal.** Separate **what to show** from **how to update the DOM**, with a pure, testable model as the single source of truth before Tier 3 patching.

**Deliverables**

- New module `embedui/logs/summarized/model.js` (or `app/summarizedModel.js`):
  - `buildSummarizedModel(deps, state) → { cards: CardModel[], meta }`
  - `CardModel`: `{ id, kind, hash, summary, body, children? }` — no HTML in model (or optional pre-rendered sections behind a flag for migration).
  - Move derivation inputs from `renderSummarizedUnified()` into model builders per kind (conversation, service, admin, gateway, workspace).
- `renderSummarizedUnified()` becomes: `model = buildSummarizedModel(...); return renderSummarizedHtml(model)` (thin).
- Per-card content hash: stable serialization of fields that affect collapsed row + expanded body (for diff in Phase 5).
- Goja fixtures: fixture `entryCache` + caches → assert card ids, order, hash changes when expected log line appended.
- README update: data flow diagram (cache → model → render/patch).

**Acceptance**

- `go test ./chimera/chimera-gateway/internal/server/adminui/embed/embedui_test/...` covers model build for at least: one conversation, one service bucket, one admin provider, gateway overview.
- Full `refreshSummarizedPanel()` uses model → HTML path; behavior matches pre-Phase-4 snapshots on fixed fixtures.

**Status:** `done`

---

## Phase 5 — Patch engine

**Goal.** Tier 3 complete: stable card roots, minimal DOM churn, full rebuild only on structural events.

**Deliverables**

- `embedui/logs/summarized/patch.js`:
  - `diffSummarizedModels(prev, next) → PatchOp[]` where ops include `replaceCard`, `updateSummary`, `updateBody`, `appendEvlogRows` (start with `replaceCard` only).
  - `applySummarizedPatches(container, ops, renderers)` — apply ops without `container.innerHTML = ...`.
- Card mount strategy:
  - Each card id maps to one root `details` node; patch updates subtrees (e.g. `.sum-metrics`, `[data-sum-evlog-tbody]`) when hash unchanged for rest of card.
  - Evlog tables: incremental row append where seq ordering allows (coordinate with `render/sumEvlog.js` hydrate path).
- Retain escape hatches: `forceSummarizedFullRebuild(reason)` for filter changes, cache trim, corruption recovery.
- Optional: `data-card-version` on roots for debugging.
- Performance note in plan/README: document threshold for full rebuild vs patch storm.

**Acceptance**

- Manual: live logs + admin poll + expand card + type API key — no full panel wipe for 5 minutes typical session.
- Automated: goja tests for `diffSummarizedModels` and patch op application on a minimal DOM stub (jsdom-like string container or HTML parser in goja).
- Phase 1–3 interaction and poll tests still pass.

**Status:** `done`

---

## Open questions

1. **Dirty flush threshold** — When should Phase 3 fall back to full rebuild (percent of cards dirty vs fixed N)?
2. **Conversation card churn** — Very chatty tenants may dirty the same card every line; is evlog-only append enough, or cap rerender rate per card?
3. **Admin editing modes** — While `adminRoutingEditing` / YAML dirty, should polls skip patches entirely for those cards?
4. **Desktop embed** — Does `logs-summarized` in desktop shell need the same defer rules when iframe focus differs from parent?

---

## References

- **Core refresh path:** `chimera/chimera-gateway/internal/server/adminui/embed/embedui/logs/app/summarizedFeed.js` (`refreshSummarizedPanel`, `scheduleStoryRebuild`, `patchGateway*`)
- **Live ingest:** `chimera/chimera-gateway/internal/server/adminui/embed/embedui/logs/transport/streaming.js` (`appendLine`, `scheduleRawLogsDomFlush`)
- **Handlers / drafts:** `chimera/chimera-gateway/internal/server/adminui/embed/embedui/logs/app/wireHandlers.js`
- **Card HTML:** `chimera/chimera-gateway/internal/server/adminui/embed/embedui/logs/render/cards/*`
- **Tests:** `chimera/chimera-gateway/internal/server/adminui/embed/embedui_test/logs_components_test.go`
- **Prior refactor plan:** [`logs-ui-maintainability.md`](logs-ui-maintainability.md) (module split — largely done; this plan owns update strategy)
