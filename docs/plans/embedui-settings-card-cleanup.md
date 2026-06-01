# Plan: Embed UI settings card cleanup

| Field | Value |
|-------|-------|
| **Doc kind** | `refactor-plan` |
| **Owners / areas** | Gateway embed UI (`/ui/settings`), summarized feed, card renderers |
| **Status** | `draft` |
| **Targets** | Gateway operator UI maintainability; prerequisite quality for setup wizard card reuse |
| **Last updated** | See git history |
| **Supersedes / superseded by** | Builds on [`logs-ui-page-data-refreshing.md`](logs-ui-page-data-refreshing.md), [`embedui-component-system.md`](embedui-component-system.md); supersedes ad-hoc card patterns in `summarizedFeed.js` where still monolithic |
| **As-built** | None — update [`operator-settings-ui.md`](../features/operator-settings-ui.md) when shipped |

## At a glance

The settings page grew through phased extraction of card renderers from a monolithic feed. Operators depend on stable, focus-safe cards; maintainers need **consistent structure, styling, and handler patterns** across provider, workspace, virtual model, routing, and service cards. This plan refactors JavaScript modules for **uniform card contracts**, reduces duplication in `summarizedFeed.js`, and aligns UX primitives (edit mode, save/cancel, warnings, evlog panels) before the setup wizard reuses those components.

| Phase | Outcome | Status |
|-------|---------|--------|
| [Phase 1 — Card contract and inventory](#phase-1--card-contract-and-inventory) | Documented card module interface; audit of all `render/cards/*` and feed registration | `todo` |
| [Phase 2 — Shared admin primitives](#phase-2--shared-admin-primitives) | Consolidate save/cancel, edit mode, drafts, and icon buttons in `adminShared.js` / `ChimeraUI` | `todo` |
| [Phase 3 — Per-card refactors](#phase-3--per-card-refactors) | Provider, workspace, virtual model, routing trio, service cards match contract | `todo` |
| [Phase 4 — Feed orchestration trim](#phase-4--feed-orchestration-trim) | `summarizedFeed.js` delegates rendering; patch/rebuild paths stay interaction-safe | `todo` |
| [Phase 5 — Tests and gallery parity](#phase-5--tests-and-gallery-parity) | embedui tests + gallery fixtures cover refactored cards; no visual regressions on mobile | `todo` |

---

## Background

**Problem.** Settings cards share patterns (collapsible `details`, scoped event log, YAML overlay, Configure/Save/Cancel) but implement them inconsistently across `adminProvider.js`, `adminVirtualModels.js`, `workspaceDraft.js`, `adminRouting.js`, `adminFallback.js`, `adminRouterModels.js`, and `serviceCard.js`. Handler logic in `handlers/admin.js` and `handlers/virtualModelsAdmin.js` duplicates fetch/error/toast flows. The summarized feed still contains card-specific HTML builders and complicates wizard extraction.

**Goals.**

- One **card module contract** (mount, render, patch, interaction guards).
- Shared **operator form** and **edit-mode** behavior per [`settings/README.md`](../../chimera/chimera-gateway/internal/server/adminui/embed/embedui/settings/README.md) interaction contract.
- Smaller `summarizedFeed.js` focused on orchestration, not feature HTML.
- Gallery (`/ui/settings/gallery`) stays the visual reference for each card type.

**Related docs:** [`operator-settings-ui.md`](../features/operator-settings-ui.md), [`embedui-component-system.md`](embedui-component-system.md), [`embedui-dynamic-provider-cards.md`](embedui-dynamic-provider-cards.md), [`logs-ui-page-data-refreshing.md`](logs-ui-page-data-refreshing.md), [`operator-embed-ui-mobile-layout.md`](operator-embed-ui-mobile-layout.md).

**Out of scope:** Backend API changes; new card types (wizard-only screens); redesign of design-01 tokens.

---

## Phase 1 — Card contract and inventory

**Goal.** Define what every settings card module must export and map current code against it.

**Deliverables**

- Short contract doc section in [`settings/README.md`](../../chimera/chimera-gateway/internal/server/adminui/embed/embedui/settings/README.md): `mount*(ctx)`, `build*Html(ctx)`, optional `patch*(ctx, el)`, draft key conventions, stable card `id` attributes for `replaceCardById`.
- Inventory table: card file → APIs used → handler module → known duplication.
- List monolithic functions still in `summarizedFeed.js` targeted for extraction.

**Acceptance**

- Every card under `render/cards/` listed with owner and refactor priority.

**Status:** `todo`

---

## Phase 2 — Shared admin primitives

**Goal.** One implementation for repeated admin UX patterns.

**Deliverables**

- Extend `adminShared.js` / `ChimeraUI` with: pending save button state, draft restore on cancel, standardized error/success message helper, Configure/edit mode toggle helper, scoped evlog panel wrapper.
- Migrate at least **provider key add** and **Ollama URL save** to shared helpers (reference implementations).
- CSS: align button classes (`sg-op-btn`, yaml overlay) — touch `admin-forms.css` only where needed.

**Acceptance**

- No behavioral change on provider card manual smoke; reduced duplicated strings in `admin.js`.

**Status:** `todo`

---

## Phase 3 — Per-card refactors

**Goal.** Each major card type follows the contract and shared primitives.

**Deliverables**

- **Provider cards** (`adminProvider.js`) — edit mode, availability table, health strip hooks unified.
- **Workspace draft + saved cards** (`workspaceDraft.js`, feed workspace builders) — shared path field and folder picker row.
- **Virtual models** (`adminVirtualModels.js`, `virtualModelsAdmin.js`) — generate/evaluate flows use shared POST helper and message display.
- **Routing trio** (`adminRouting.js`, `adminFallback.js`, `adminRouterModels.js`) — shared YAML dirty state and defer-rebuild interaction with feed.
- **Service cards** (`serviceCard.js`, gateway overview) — consistent health pill and metrics strip mounting.

Order: provider → workspace → virtual model → routing → service (one PR per card family where possible).

**Acceptance**

- Each family has a gallery fixture row still passing visual/embedui tests.
- Card-specific logic removed from `summarizedFeed.js` for refactored families.

**Status:** `todo`

---

## Phase 4 — Feed orchestration trim

**Goal.** `summarizedFeed.js` orchestrates data and patch scheduling; cards own HTML.

**Deliverables**

- Move remaining inline HTML builders into card modules or `render/` helpers.
- Verify `summarizedPanelInteractionBlocksRebuild`, `patchAdminCardsFromPoll`, and `replaceCardById` still work after moves.
- Document rebuild vs patch decision tree in settings README.

**Acceptance**

- `summarizedFeed.js` line count materially reduced; no regressions in [`logs-ui-page-data-refreshing.md`](logs-ui-page-data-refreshing.md) acceptance scenarios (focus retention, first-click expand).

**Status:** `todo`

---

## Phase 5 — Tests and gallery parity

**Goal.** Refactor is safe to extend for setup wizard reuse.

**Deliverables**

- Update `embedui_test/settings_cards_test.go` and component tests for moved DOM ids/classes.
- Gallery HTML reflects refactored markup for provider, workspace, VM, routing cards.
- Optional: JS unit tests under `settings/testing/` for pure formatters moved out of feed.

**Acceptance**

- `go test ./chimera/chimera-gateway/internal/server/adminui/embed/embedui_test/...` passes.
- Mobile layout spot-check on provider and VM cards per embed UI mobile feature.

**Status:** `todo`

---

## Open questions

1. **Wizard reuse** — Extract shared card controllers to `embedui/shared/` vs wizard passes `mode: "wizard"` into existing mount functions.
2. **Strict DOM id stability** — Required for patch path; document breaking-change policy for card ids.
3. **Phase ordering vs wizard deadline** — If wizard ships first, minimum slice is Phase 1–2 + provider/workspace only.

---

## References

- Code: `embed/embedui/settings/app/summarizedFeed.js`, `embed/embedui/settings/render/cards/`, `embed/embedui/settings/handlers/`, `embed/embedui/settings/gallery.html`
- Tests: `embed/embedui_test/settings_cards_test.go`, `settings_components_test.go`
- Feature: [`operator-settings-ui.md`](../features/operator-settings-ui.md)
