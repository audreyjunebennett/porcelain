# Plan: Embed UI settings card cleanup

| Field | Value |
|-------|-------|
| **Doc kind** | `refactor-plan` |
| **Owners / areas** | Gateway embed UI (`/ui/settings`), summarized feed, card renderers |
| **Status** | `active` |
| **Targets** | Gateway operator UI maintainability; shared card modules in `embedui/shared/` for later setup wizard reuse |
| **Last updated** | See git history |
| **Supersedes / superseded by** | Builds on [`logs-ui-page-data-refreshing.md`](logs-ui-page-data-refreshing.md), [`embedui-component-system.md`](embedui-component-system.md); supersedes ad-hoc card patterns in `summarizedFeed.js` where still monolithic |
| **As-built** | None — update [`operator-settings-ui.md`](../features/operator-settings-ui.md) when shipped |

## At a glance

The settings page grew through phased extraction of card renderers from a monolithic feed. Operators depend on stable, focus-safe cards; maintainers need **consistent structure, styling, and handler patterns** across provider, workspace, virtual model, routing, and service cards. This plan refactors JavaScript modules for **uniform card contracts**, reduces duplication in `summarizedFeed.js`, and aligns UX primitives (edit mode, save/cancel, warnings, evlog panels). Shared card controllers land in **`embedui/shared/`**; the setup wizard ships **after** this work and imports from there.

| Phase | Outcome | Status |
|-------|---------|--------|
| [Phase 1 — Card contract and inventory](#phase-1--card-contract-and-inventory) | Documented card module interface; audit of all `render/cards/*` and feed registration | `done` |
| [Phase 2 — Shared admin primitives](#phase-2--shared-admin-primitives) | Consolidate save/cancel, edit mode, drafts, and icon buttons in `adminShared.js` / `ChimeraUI` | `done` |
| [Phase 3 — Per-card refactors](#phase-3--per-card-refactors) | Provider, workspace, virtual model, routing trio, service cards match contract | `done` |
| [Phase 4 — Feed orchestration trim](#phase-4--feed-orchestration-trim) | `summarizedFeed.js` delegates rendering; feed refresh paths stay interaction-safe | `done` |
| [Phase 5 — Tests and gallery parity](#phase-5--tests-and-gallery-parity) | embedui tests + gallery fixtures cover refactored cards; no visual regressions on mobile | `done` |
| [Phase 6 — Gallery part taxonomy and overlay](#phase-6--gallery-part-taxonomy-and-overlay) | Textual part registry + `data-ui-part` labels with CSS overlay on `/ui/settings/gallery` | `done` |

---

## Background

**Problem.** Settings cards share patterns (collapsible `details`, scoped event log, YAML overlay, Configure/Save/Cancel) but implement them inconsistently across `adminProvider.js`, `adminVirtualModels.js`, `workspaceDraft.js`, and `serviceCard.js`. Handler logic in `handlers/admin.js` and `handlers/virtualModelsAdmin.js` duplicates fetch/error/toast flows. The summarized feed still contains card-specific HTML builders and complicates wizard extraction.

**Goals.**

- One **card module contract** (mount, render, interaction guards) with reusable implementations in `embedui/shared/`.
- Shared **operator form** and **edit-mode** behavior per [`settings/README.md`](../../chimera/chimera-gateway/internal/server/adminui/embed/embedui/settings/README.md) interaction contract.
- Smaller `summarizedFeed.js` focused on orchestration, not feature HTML.
- Gallery (`/ui/settings/gallery`) stays the visual reference for each card type; after parity (Phase 5), a **stable part taxonomy** and on-page labels (Phase 6) so implementers and agents can name exact regions.

**Related docs:** [`operator-settings-ui.md`](../features/operator-settings-ui.md), [`embedui-component-system.md`](embedui-component-system.md), [`embedui-dynamic-provider-cards.md`](embedui-dynamic-provider-cards.md), [`logs-ui-page-data-refreshing.md`](logs-ui-page-data-refreshing.md), [`operator-embed-ui-mobile-layout.md`](operator-embed-ui-mobile-layout.md).

**Out of scope:** Backend API changes; new card types (wizard-only screens); redesign of design-01 tokens.

---

## Phase 1 — Card contract and inventory

**Goal.** Define what every settings card module must export and map current code against it.

**Deliverables**

- Short contract doc section in [`settings/README.md`](../../chimera/chimera-gateway/internal/server/adminui/embed/embedui/settings/README.md): `mount*(ctx)`, `build*Html(ctx)`, draft key conventions, and which logic lives in `embedui/shared/` vs settings-only glue.
- Inventory table: card file → APIs used → handler module → known duplication — [`settings/CARD_INVENTORY.md`](../../chimera/chimera-gateway/internal/server/adminui/embed/embedui/settings/CARD_INVENTORY.md).
- List monolithic functions still in `summarizedFeed.js` targeted for extraction (same doc, § Monolithic builders).
- Seed list of major card regions per family (feeds Phase 6 part slugs); no `data-ui-part` or overlay in this phase (same doc, § Part regions).

**Acceptance**

- Every card under `render/cards/` listed with owner and refactor priority.

**Status:** `done`

---

## Phase 2 — Shared admin primitives

**Goal.** One implementation for repeated admin UX patterns, colocated under `embedui/shared/` where the setup wizard will import them later.

**Deliverables**

- Extend `adminShared.js` / `ChimeraUI` (or move equivalents into `embedui/shared/`) with: pending save button state, draft restore on cancel, standardized error/success message helper, Configure/edit mode toggle helper, scoped evlog panel wrapper.
- Migrate at least **provider key add** and **Ollama URL save** to shared helpers (reference implementations).
- CSS: align button classes (`sg-op-btn`, yaml overlay) — touch `admin-forms.css` only where needed.

**Acceptance**

- No behavioral change on provider card manual smoke; reduced duplicated strings in `admin.js`.

**Status:** `done`

**As-built:** [`embedui/shared/README.md`](../../chimera/chimera-gateway/internal/server/adminui/embed/embedui/shared/README.md); provider key add + Ollama URL save via `ChimeraShared.ProviderCredentials`; status/pending via `ChimeraShared.OperatorFeedback`.

---

## Phase 3 — Per-card refactors

**Goal.** Each major card type follows the contract and shared primitives.

**Deliverables**

- **Provider cards** (`adminProvider.js`) — edit mode, availability table, health strip hooks unified.
- **Workspace draft + saved cards** (`workspaceDraft.js`, feed workspace builders) — shared path field and folder picker row.
- **Virtual models** (`adminVirtualModels.js`, `virtualModelsAdmin.js`) — generate/evaluate flows use shared POST helper and message display.
- **Virtual model routing sections** (`adminVirtualModels.js`) — shared YAML dirty state and defer-rebuild interaction with feed (global `adminRouting.js` / `adminFallback.js` / `adminRouterModels.js` removed; live site uses per-VM panels only).
- **Service cards** (`serviceCard.js`, gateway overview) — consistent health pill and metrics strip mounting.

Order: provider → workspace → virtual model → routing → service (one PR per card family where possible).

**Acceptance**

- Each family has a gallery fixture row still passing visual/embedui tests.
- Card-specific logic removed from `summarizedFeed.js` for refactored families.

**Status:** `done`

**As-built:** Extended `embedui/shared/` (`adminAction.js`, `editToolbar.js`, `workspacePaths.js`, `serviceHealth.js`; `yamlEditor.textareaWrapHtml`). Provider/VM toolbars use `EditToolbar`; VM YAML wraps + dirty input via `YamlEditor`; VM generate/save via `AdminAction.runJson`; managed workspace path chrome in `workspaceDraft.js` (removed from `summarizedFeed.js`); gateway overview health via `ServiceHealth`.

---

## Phase 4 — Feed orchestration trim

**Goal.** `summarizedFeed.js` orchestrates data and refresh scheduling; cards own HTML.

**Deliverables**

- Move remaining inline HTML builders into card modules, `render/` helpers, or `embedui/shared/` where wizard-eligible.
- Refactor feed refresh paths (`summarizedPanelInteractionBlocksRebuild`, poll-driven card updates) as needed — no legacy DOM id or markup compatibility required.
- Document rebuild vs in-place update decision tree in settings README.

**Acceptance**

- `summarizedFeed.js` line count materially reduced; no regressions in [`logs-ui-page-data-refreshing.md`](logs-ui-page-data-refreshing.md) acceptance scenarios (focus retention, first-click expand).

**Status:** `done`

**As-built:** `render/cards/feedLogConv.js`, `feedLogService.js` (`buildConvCard`, `buildServiceCard`, service/indexer HTML helpers); `summarized/rebuildPolicy.js`; feed mounts log cards before `mountAll`; rebuild vs patch tree in [`settings/README.md`](../../chimera/chimera-gateway/internal/server/adminui/embed/embedui/settings/README.md).

### Phase 4b — Indexer card extraction

**Goal.** Move indexer run, stale, and operator-workspace card builders out of `summarizedFeed.js`.

**Deliverables**

- `render/cards/indexerRun.js` — `buildIndexerCard`, `buildIndexerStaleSnapshotCard`, watch-root persistence, dedupe keys.
- `render/cards/indexerWorkspace.js` — `buildIndexerOperatorWorkspaceCard`, workspace path chrome, coverage helpers.
- Restore `indexerScopeProgressTimelineBarHtml` (lost in Phase 4 extraction).

**Status:** `done`

**As-built:** `indexerRun.js`, `indexerWorkspace.js`; mount order conv → service → indexerRun → indexerWorkspace; `summarizedFeed.js` ~3.2k lines; `extract-feed-phase4b.py`.

---

## Phase 5 — Tests and gallery parity

**Goal.** Tests and gallery match the refactored modules; wizard can consume `embedui/shared/` afterward.

**Deliverables**

- Update `embedui_test/settings_cards_test.go` and component tests for new module layout and any changed DOM ids/classes (breaking changes are fine).
- Gallery HTML reflects refactored markup for provider, workspace, VM, routing cards.
- Optional: JS unit tests under `settings/testing/` for pure formatters moved out of feed.

**Acceptance**

- `go test ./chimera/chimera-gateway/internal/server/adminui/embed/embedui_test/...` passes.
- Mobile layout spot-check on provider and VM cards per embed UI mobile feature.

**Status:** `done`

**As-built:** `loadCardTestCtx` mounts feed-log cards; new tests in `settings_cards_test.go` (gateway health strip, workspace draft, indexer stale, title label); `gallery/gallery-card-fixtures.js` + updated `settings/gallery.html` fixture mount points; `ui_settings_test` asserts gallery script paths.

**Prerequisite for Phase 6:** Gallery markup and nav match production cards (this phase). Do not add part overlays or registry entries against stale gallery HTML.

---

## Phase 6 — Gallery part taxonomy and overlay

**Goal.** After the gallery reflects refactored cards, give every major card region a **stable name** (for docs, agents, and future settings-page context export) and make those names **visible** in the gallery without reading source.

**Depends on:** [Phase 5](#phase-5--tests-and-gallery-parity) complete — taxonomy and overlays target current gallery fixtures and production-aligned class names/ids.

**Deliverables**

- **Textual taxonomy (registry)** — Add [`settings/card-parts-registry.md`](../../chimera/chimera-gateway/internal/server/adminui/embed/embedui/settings/card-parts-registry.md) (or an equivalent section in [`settings/README.md`](../../chimera/chimera-gateway/internal/server/adminui/embed/embedui/settings/README.md)) with:
  - Card-level ids: feed `id` + summarized `kind` (e.g. `admin-provider-groq`, `virtual-model-12`).
  - Part slugs: `{card-kind}.{region}` (e.g. `admin-provider.models-toolbar`, `virtual-model.routing-policy`).
  - Columns: slug, human label, card `kind`, DOM hint (`data-ui-part` and/or stable `#id`), owning builder file.
  - Notes for **live vs legacy**: per-VM routing sections are live; global `admin-routing-*` modules are gallery/legacy unless removed in Phase 3.
- **Production `data-ui-part`** — On refactored card HTML (start with provider, VM, gateway overview, workspace), set `data-ui-part` on major regions matching the registry. Gallery fixtures mirror the same attributes.
- **CSS overlay** — In [`gallery/gallery-shell.css`](../../chimera/chimera-gateway/internal/server/adminui/embed/embedui/gallery/gallery-shell.css): styles for `body.gallery-show-parts [data-ui-part]` (dashed outline + `::after { content: attr(data-ui-part); }` pill). Gallery-only; not required on `/ui/settings` production unless useful for debugging.
- **Gallery controls** — Toolbar toggle on [`settings/gallery.html`](../../chimera/chimera-gateway/internal/server/adminui/embed/embedui/settings/gallery.html): “Show part names” toggles `gallery-show-parts` on `body`. Optional query `?parts=1` (small inline script or `gallery-parts.js`) for shareable review links.
- **Registry captions (approach A)** — Under each gallery section, short `styleguide-sub` bullets listing part slugs for that demo (complements the overlay).

**Acceptance**

- Opening `/ui/settings/gallery` with part names enabled shows labels on every documented major region for provider, workspace, VM (including routing/fallback/tool-router sections), and gateway overview demos.
- Registry slug count matches `data-ui-part` values in gallery fixtures for those families (grep-check or documented exception list).
- Part slugs are cited in plan/issues without ambiguous prose (“Configure row” → `admin-provider.models-toolbar`).

**Future hook (document only, not implemented here)** — Optional per-card `describeCard(card, ctx)` for operator chat context should reuse the same slugs; see registry “Future” subsection, no collector in this phase.

**Status:** `done`

**As-built:** [`settings/card-parts-registry.md`](../../chimera/chimera-gateway/internal/server/adminui/embed/embedui/settings/card-parts-registry.md); `data-ui-part` on gateway overview, admin provider, virtual model, workspace draft, indexer operator workspace, indexer stale; `sumEvlog.js` / `scopedEvlog.js` `uiPart` option; `gallery/gallery-parts.js` + overlay CSS in `gallery-shell.css`; gallery toolbar toggle and section slug captions.

---

## Decisions

1. **Wizard reuse** — Extract shared card controllers to **`embedui/shared/`**. Settings page and wizard each mount shared modules; **no** `mode: "wizard"` (or similar) parameter on settings mount functions.
2. **DOM / legacy compatibility** — **None required.** Gateway embed UI has a single operator; card ids, markup, and refresh mechanics may change freely. Update tests and gallery in the same change — no breaking-change policy doc.
3. **Phase ordering** — **Setup wizard ships after this plan.** All **six** phases ship in order; Phase 6 runs only after Phase 5 gallery parity.
4. **Part taxonomy** — Slugs are stable contracts for docs and agents; changing a slug requires updating the registry, gallery `data-ui-part`, and any tests that assert part attributes.

---

## References

- Code: `embed/embedui/shared/` (planned Phase 2), `embed/embedui/settings/app/summarizedFeed.js`, `embed/embedui/settings/render/cards/`, `embed/embedui/settings/handlers/`, `embed/embedui/settings/gallery.html`, `embed/embedui/gallery/gallery-shell.css`
- Phase 1 docs: [`settings/README.md`](../../chimera/chimera-gateway/internal/server/adminui/embed/embedui/settings/README.md#card-module-contract), [`settings/CARD_INVENTORY.md`](../../chimera/chimera-gateway/internal/server/adminui/embed/embedui/settings/CARD_INVENTORY.md); Phase 6: `settings/card-parts-registry.md`
- Tests: `embed/embedui_test/settings_cards_test.go`, `settings_components_test.go`
- Feature: [`operator-settings-ui.md`](../features/operator-settings-ui.md)
