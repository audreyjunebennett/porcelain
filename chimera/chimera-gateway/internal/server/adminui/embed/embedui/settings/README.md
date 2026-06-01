# Operator settings modules (`embedui/settings/`)

JavaScript for **`GET /ui/settings`** (`settings.html`). Namespace: **`globalThis.ChimeraSettings`**.

## Load order

See `settings.html`: `contracts.js` → `ChimeraUI` (`ui/`) → util/parse → derive → components → handlers → `settings_app.js` (as `/ui/assets/settings/main.js`) → `settings_entry.js` (as `/ui/assets/settings.js`).

## Module map

| Directory | Responsibility |
|-----------|----------------|
| `app/` | Feed orchestration (`summarizedFeed.js`), handler wiring, dirty routing |
| `derive/` | Pure metrics/card models from parsed log lines |
| `handlers/` | DOM event handlers (admin saves, chrome, evlog panel) |
| `render/` | HTML builders; `render/cards/` registers card types on `ChimeraSettings.Render.Cards` |
| `summarized/` | Diff/patch model for incremental card updates |
| `transport/` | Log poll/SSE (`/api/ui/logs`, `/api/ui/logs/stream`) |
| `parse/` | Log line text → flat field map |
| `components/` | Settings-local widgets (also uses `ui/components/`) |
| `util/` | hash, time, escape re-exports |
| `testing/` | Test harness loader (not used in production HTML) |
| `contracts.js` | Generated constants (products, timeline kinds) |
| `operator_copy.js` | Generated operator message registry |

## Related routes

| Route | File |
|-------|------|
| `GET /ui/settings/gallery` | `../settings/gallery.html` (assets in `../gallery/`); part slugs in [`card-parts-registry.md`](card-parts-registry.md) |

**APIs (unchanged):** `GET /api/ui/logs`, `GET /api/ui/logs/stream`, `GET /api/ui/state`, `GET /api/ui/metrics`, indexer/routing save endpoints, etc.

## Card module contract

Card renderers live under `render/cards/`. Each module follows the same registration pattern (see [`CARD_INVENTORY.md`](CARD_INVENTORY.md) for the full audit).

### Registration

1. Export `ChimeraSettings.Render.Cards.mount{Name} = function (ctx) { … }`.
2. Register in [`render/cards/mount.js`](render/cards/mount.js); `mountAll(ctx)` runs at the end of `summarizedFeed.js` initialization.
3. Assign HTML builders on **`ctx`** (e.g. `ctx.buildAdminProviderCardHtml = buildAdminProviderCardHtml`) so the feed and tests can call them.

### Mount order and `ctx` (Phase 4 feed-log)

1. **`mountAll`** (`mount.js`) runs first: `sharedFormat` → admin/gateway cards (`serviceCard` avatars only; no log-feed mounts).
2. **`mountSummarizedFeedCards`** (`mount.js`, called from `summarizedFeed.js` or setup wizard): `cardChrome` → `indexerRun` → `indexerWorkspace` → `feedLogConv` → `serviceFeed`.
3. **`summarizedFeed.js`** orchestration calls **`ctx.*` only** — never bare symbols moved into card modules.
4. **Do not** write `var fn = ctx.fn` at the top of a `mount*` closure when `function fn` is defined in the same closure; the `var` assignment overwrites the hoisted function with `undefined` (common extraction bug). Export the local function on `ctx` once at the end of mount instead.
5. Cross-module helpers: resolve from **`ctx` at call time** (or a thin local wrapper that delegates to `ctx`), not a stale capture at mount when the exporter mounts later.

CI: `embedui_test/card_mount_shadow_test.go`, `feed_mount_exports_test.go`, `feed_smoke_test.go`.

### Required exports (per card family)

| Export | Purpose |
|--------|---------|
| `mount*(ctx)` | Wire helpers from `ctx`, define builders, assign `ctx.build*Html` |
| `build*Html(...)` | Return HTML string; use production class names (`sum-card`, `sg-op-*`) |
| Stable root **`id`** | Feed `replaceCardById` and patch paths match this id (see inventory); ids may change freely per plan — update tests/gallery in the same PR |

Optional (Phase 6+): `describe*(card, ctx)` for operator chat context — same part slugs as `card-parts-registry.md`; not required until that phase.

### Summarized feed integration

Cards on the live rail must appear in [`summarized/model.js`](summarized/model.js) with a `kind` and `id`, and in `renderSummarizedCardFromModel` (`app/summarizedFeed.js`). Section labels use `kind: "section-break"` rows.

Gateway routing, fallback, and tool-router UI live **inside** virtual model cards (`adminVirtualModels.js`); handlers in `handlers/virtualModelsAdmin.js`.

### Draft and edit mode

- Draft arrays on `ctx` (`adminUserDrafts`, `virtualModelDrafts`, `workspaceDrafts`, …).
- Per-card edit flags (`adminProviderModelsEditingId`, `virtualModelUi[vmId].routingEditing`, …).
- Actions via `data-admin-action` on buttons; handlers in `handlers/admin.js`, `handlers/virtualModelsAdmin.js`, `handlers/providerModelsAdmin.js`.
- Full rebuild is skipped while interaction guards are active (`summarizedPanelInteractionBlocksRebuild`, edit-mode checks in the feed).

Convention tables: [`CARD_INVENTORY.md` § Draft and edit-state](CARD_INVENTORY.md#draft-and-edit-state-conventions-ctx).

### Shared vs settings-only code

| Location | Owns |
|----------|------|
| `ui/components/` (`ChimeraUI`) | Presentation primitives: pills, KV grid, `CollapsibleCard`, `YamlEditorPanel`, … |
| `render/cards/adminShared.js` | Operator-specific HTML helpers, YAML parse, scoped evlog builders (candidate for `embedui/shared/` in Phase 2) |
| `embedui/shared/` (`ChimeraShared`) | Wizard + settings: save/cancel, configure toolbar, YAML dirty, scoped evlog, credentials, `AdminAction.runJson`, `EditToolbar`, `WorkspacePaths`, `ServiceHealth` |
| `settings/handlers/` | DOM events, `adminPostJSON` / `fetch`, toast messages |
| `settings/app/summarizedFeed.js` | Model build, poll/SSE, `replaceCardById`, patch scheduling — not card HTML long-term |

### Rebuild vs patch

- **Patch:** `replaceCardById(cardId, buildHtml, { preserveOpen, cardHash })` when `summarized/model.js` hash changes but focus/edit guards allow.
- **Full rebuild:** `renderSummarizedUnified()` when structure or filters demand it.
- **Guards:** [`summarized/rebuildPolicy.js`](summarized/rebuildPolicy.js) (`mountRebuildPolicy`) — focus in `#panel-summarized`, admin drafts/editing, open VM/provider cards in `summarizedPatchSkipCardIds`. Live dirty routing: [`summarizedDirtyRouting.js`](summarized/summarizedDirtyRouting.js).

**Decision tree**

1. **Structural change** (new card id/kind, section order, filter mode) → `scheduleStoryRebuild` / `forceSummarizedFullRebuild` (full `renderSummarizedUnified`).
2. **Single-card data change** with stable id → `replaceCardById` or `Summarized.Patch.applySummarizedPatches` when hash differs and card not in skip set.
3. **Skip set** (`summarizedPatchSkipCardIds`) — card open or VM/provider/workspace edit; poll/SSE may still patch via `refreshAdminCardAfterEditToggle` or rebuild when `summarizedSkippedCardsHashDelta` detects hash drift on skipped ids.
4. **Defer** — `summarizedPanelInteractionBlocksRebuild` or `summarizedAdminEditingActive` → `scheduleDeferredSummarizedRefresh` (300ms retry) instead of clobbering focus.
5. **Dirty storm** — many cards dirty → coalesced full rebuild (`SUMMARIZED_DIRTY_FULL_REBUILD_*` in feed); live settle defers patches briefly after tail ingest.

## Maintainer docs

- [`CARD_INVENTORY.md`](CARD_INVENTORY.md) — card file → API → handler map and part slug seeds.
