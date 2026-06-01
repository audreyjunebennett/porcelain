# Plan: Split feed log service card module

| Field                          | Value                                                                                                                                                                                            |
|--------------------------------|--------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| **Doc kind**                   | `refactor-plan`                                                                                                                                                                                  |
| **Owners / areas**             | Gateway embed UI (`/ui/settings`), summarized feed, `render/cards/*`                                                                                                                             |
| **Status**                     | `shipped`                                                                                                                                                                                        |
| **Targets**                    | Maintainable card modules; `feedLogService.js` owns service cards only                                                                                                                           |
| **Last updated**               | See git history                                                                                                                                                                                  |
| **Supersedes / superseded by** | Follows [`embedui-settings-card-cleanup.md`](embedui-settings-card-cleanup.md) Phases 4–4b (conv/service/indexer extraction shipped); does not require legacy DOM or export names                |
| **As-built**                   | [`operator-settings-ui.md`](../features/operator-settings-ui.md), [`settings/CARD_INVENTORY.md`](../../chimera/chimera-gateway/internal/server/adminui/embed/embedui/settings/CARD_INVENTORY.md) |

## At a glance

Phase 4 moved conversation, service, and indexer HTML out of `summarizedFeed.js`, but **`feedLogService.js` (~2.6k lines) still mixes four service feed cards with indexer run metadata, workspace operator store, evlog label maps, and dead extraction leftovers.** Operators should see the same cards on `/ui/settings`; maintainers need **one module per card family**, a single mount path, and `ctx` exports that match real callers. This plan **splits and deletes** misplaced code—**no backwards-compatibility** for old `ctx` names, script load order quirks, or duplicate mounts.

| Phase                                                                    | Outcome                                                                                           | Status |
|--------------------------------------------------------------------------|---------------------------------------------------------------------------------------------------|--------|
| [Phase 1 — Hygiene and wiring fixes](#phase-1--hygiene-and-wiring-fixes) | Remove dead code; fix missing `ctx` exports and bare cross-module calls                           | `done` |
| [Phase 2 — Shared feed card helpers](#phase-2--shared-feed-card-helpers) | One home for `sliceRecent`, metric wells, status pills used by conv/service/indexer cards         | `done` |
| [Phase 3 — Service-only module](#phase-3--service-only-module)           | `feedLogService.js` (or `serviceFeed.js`) contains only `buildServiceCard` and per-service panels | `done` |
| [Phase 4 — Indexer ownership](#phase-4--indexer-ownership)               | Run/meta/evlog helpers live in `indexerRun.js`; workspace store/API in `indexerWorkspace.js`      | `done` |
| [Phase 5 — Mount order and tests](#phase-5--mount-order-and-tests)       | Single mount per module; CI guards; inventory and feature record updated                          | `done` |

---

## Background

**Problem.** [`feedLogService.js`](../../chimera/chimera-gateway/internal/server/adminui/embed/embedui/settings/render/cards/feedLogService.js) is the largest card module after Phase 4b. It exports **~80 symbols on `ctx`**, but most are indexer- or workspace-specific while only **`buildServiceCard`**, **`renderExpandedService`**, and chimera-broker health helpers** belong to the service card family. [`indexerRun.js`](../../chimera/chimera-gateway/internal/server/adminui/embed/embedui/settings/render/cards/indexerRun.js) and [`indexerWorkspace.js`](../../chimera/chimera-gateway/internal/server/adminui/embed/embedui/settings/render/cards/indexerWorkspace.js) already consume those exports; the split completes the intent of Phase 4b without carrying forward extraction debt.

**Audit findings (pre-plan).**

| Category              | Examples                                                                                                                                                                                | Action                                                  |
|-----------------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|---------------------------------------------------------|
| Dead code             | `indexerEventMixHistogramHtml`, `indexerHistogramLegendHtml`, `latestIndexerQueueSnapshotMetaFromEntries`; unused conv imports at top of `feedLogService.js`                            | Delete                                                  |
| Orphan `ctx` wrappers | `rollupGatewayRagPipeline`, `vectorstoreHttpPathRollup` on `ctx` (implementations live in `derive/vectorstoreRagMetrics.js`)                                                            | Delete wrappers; callers use `ChimeraSettings.Derive.*` |
| Duplicate helpers     | `sliceRecent` in `feedLogConv.js` and `feedLogService.js`; `ctx.recentServiceCardHasError` assigned twice                                                                               | Consolidate in Phase 2                                  |
| Wiring bugs           | `operatorWorkspacePaths`, `pathsSetEqualForIndexerRoots` defined but not exported on `ctx`; bare `mergePersistedIndexerWatchRoots` / `indexerCardTitleSortLabel` in `feedLogService.js` | Fix in Phase 1; move with owner in Phase 4              |
| Double mount          | `mountFeedLogService` in `mount.js` and again in `summarizedFeed.js`                                                                                                                    | Single mount in Phase 5                                 |

**Goals.**

- **`feedLogService.js` ≤ ~600 lines** — service cards, broker health strips used outside service cards, service section head.
- **Indexer run card logic in `indexerRun.js`** — meta collection, flat classifiers, run progress, recent evaluated files, evlog badges.
- **Indexer workspace / operator store in `indexerWorkspace.js`** — path helpers, managed-workspace summary, API hydrate for chimera-indexer service summary.
- **Explicit mount order** documented in [`CARD_INVENTORY.md`](../../chimera/chimera-gateway/internal/server/adminui/embed/embedui/settings/CARD_INVENTORY.md) and enforced by tests.

**Non-goals.**

- Backend API or log schema changes.
- Visual redesign of service/indexer cards (class names and `data-ui-part` may change freely).
- **Backwards compatibility** — old `ctx` export names, gallery fixture ids, and duplicate mount patterns may be removed in the same PR as the move; update tests and gallery in that PR.

**Related docs:** [`operator-settings-ui.md`](../features/operator-settings-ui.md), [`embedui-settings-card-cleanup.md`](embedui-settings-card-cleanup.md), [`settings/README.md` § Card module contract](../../chimera/chimera-gateway/internal/server/adminui/embed/embedui/settings/README.md#card-module-contract), [`settings/CARD_INVENTORY.md`](../../chimera/chimera-gateway/internal/server/adminui/embed/embedui/settings/CARD_INVENTORY.md).

---

## Target module layout (end state)

| Module                                              | `mount*`                                              | Owns                                                                                                                                                                                                                                                                                                                                                   |
|-----------------------------------------------------|-------------------------------------------------------|--------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `sharedFormat.js` or new `render/feedCardShared.js` | `mountFeedCardShared` (or extend `mountSharedFormat`) | `sliceRecent`, `sgOpInsetWellOkFailHtml`, `summaryMetricsHtml`, `serviceSummaryStatusPillHtml`                                                                                                                                                                                                                                                         |
| `serviceCard.js`                                    | `mountServiceCard`                                    | Avatars, `serviceDisplayLabel`, `inferServiceBadge` (no duplicate wrappers in service feed module)                                                                                                                                                                                                                                                     |
| `feedLogService.js`                                 | `mountFeedLogService`                                 | `buildServiceCard`, `renderExpandedService`, gateway/broker/vectorstore/indexer **service** intros and mini panels, `chimeraBrokerProviderHealthStripHtml` and broker metric labels, `summarizedServicesSectionHead`, `entryIsGatewayUpstreamRelay`, `entryRoutesToChimeraBrokerBucket`                                                                |
| `indexerRun.js`                                     | `mountFeedLogIndexerRun`                              | `buildIndexerCard`, `buildIndexerStaleSnapshotCard`, `collectIndexerRunMeta`, flat classifiers, run subtitles/metrics, `buildIndexerRecentEvaluatedFilesHtml`, `indexerCardDomIdFromMeta`, `mergePersistedIndexerWatchRoots`, `indexerCardTitleSortLabel`, evlog workspace label map (or `render/indexerEvlogLabels.js` if run file exceeds ~1k lines) |
| `indexerWorkspace.js`                               | `mountFeedLogIndexerWorkspace`                        | `buildIndexerOperatorWorkspaceCard`, `operatorWorkspacePaths`, `pathsSetEqualForIndexerRoots`, dedupe/merge operator workspaces, `hydrateIndexerServiceSummaryFromApi`, `indexerServiceSummary*`, managed-workspace aggregate HTML for chimera-indexer service card                                                                                    |
| `feedLogConv.js`                                    | `mountFeedLogConv`                                    | Conversation cards only (drop local `sliceRecent` after Phase 2)                                                                                                                                                                                                                                                                                       |

**Mount order (single chain in `summarizedFeed.js` after `mountAll` admin/gateway cards):**

1. `mountFeedCardShared` (if new)
2. `mountServiceCard`
3. `mountFeedLogService`
4. `mountFeedLogConv`
5. `mountFeedLogIndexerRun`
6. `mountFeedLogIndexerWorkspace`

Remove `mountFeedLogService` from [`mount.js`](../../chimera/chimera-gateway/internal/server/adminui/embed/embedui/settings/render/cards/mount.js) so service feed mounts once, after `serviceCard` and before indexer modules that depend on shared helpers.

---

## Phase 1 — Hygiene and wiring fixes

**Goal.** Safe deletions and correct `ctx` wiring so later moves do not hide ReferenceErrors.

**Deliverables**

- Delete unused top-of-file captures in `feedLogService.js`: `sumEvlogBuildTbodyFromConvEvents`, `contextGrowthStripHtml`, `SHOW_CONV_EXPANDED_CONTEXT_STRIP`, `formatMergedConversationSubtitle`.
- Delete dead functions and exports: `indexerEventMixHistogramHtml`, `indexerHistogramLegendHtml`, `INDEXER_HIST_COLS`, `latestIndexerQueueSnapshotMetaFromEntries`.
- Delete `ctx.rollupGatewayRagPipeline` and `ctx.vectorstoreHttpPathRollup` wrappers; grep settings for bare/`ctx` uses and point to `ChimeraSettings.Derive` where needed.
- Remove duplicate `ctx.recentServiceCardHasError` assignment.
- Export on `ctx` from the owning closure (temporary: still `feedLogService.js` until Phase 4): `operatorWorkspacePaths`, `pathsSetEqualForIndexerRoots`, `mergeOperatorWorkspacePathsInto`, `normalizeIndexerWatchPathForCompare`.
- Replace all bare `mergePersistedIndexerWatchRoots(...)` and `indexerCardTitleSortLabel(...)` in `feedLogService.js` with `ctx.*` at call time (or move call sites with the function in Phase 4).

**Acceptance**

- `rg` shows no remaining references to deleted symbols except CHANGELOG/plan docs.
- `go test ./chimera/chimera-gateway/internal/server/adminui/embed/embedui_test/...` passes.
- Manual smoke: expand chimera-indexer service card (managed workspaces + config path), one indexer run card, one operator workspace card.

**Status:** `done`

**As-built:** Dead histogram/conv imports and orphan `ctx` rollup wrappers removed; `operatorWorkspacePaths` / `pathsSetEqualForIndexerRoots` exported; bare indexer cross-module calls use `ctx.*`; `feed_mount_exports_test.go` guards.

---

## Phase 2 — Shared feed card helpers

**Goal.** One implementation for helpers shared across conversation, service, and indexer feed cards.

**Deliverables**

- Add `mountFeedCardShared(ctx)` in [`render/feedCardShared.js`](../../chimera/chimera-gateway/internal/server/adminui/embed/embedui/settings/render/feedCardShared.js) **or** extend [`sharedFormat.js`](../../chimera/chimera-gateway/internal/server/adminui/embed/embedui/settings/render/cards/sharedFormat.js) with: `sliceRecent`, `sgOpInsetWellOkFailHtml`, `summaryMetricsHtml`, `serviceSummaryStatusPillHtml`, `humanDurationMs` delegate if not already on `ctx`.
- Remove duplicate `sliceRecent` from `feedLogConv.js` and `feedLogService.js`; consumers use `ctx.sliceRecent` only.
- Register script in `settings.html` and `gallery.html` before feed card modules.
- Mount `feedCardShared` first in the feed mount chain (see target layout).

**Acceptance**

- Exactly one `function sliceRecent` under `settings/render/`.
- Indexer and conversation cards still render status pills and recent-window error logic in goja card tests.

**Status:** `done`

**As-built:** [`settings/render/feedCardShared.js`](../../chimera/chimera-gateway/internal/server/adminui/embed/embedui/settings/render/feedCardShared.js) (`mountFeedCardShared`); mounted from `mount.js` + `summarizedFeed.js` before feed-log cards; duplicates removed from `feedLogConv.js` / `feedLogService.js`.

---

## Phase 3 — Service-only module

**Goal.** `feedLogService.js` contains only the four summarized **service** cards and broker health UI reused by gateway usage / admin.

**Deliverables**

- Keep in `feedLogService.js`: `buildServiceCard`, `renderExpandedService`, `recentServiceCardHasError`, `serviceWindowMs`, `summarizedServicesSectionHead`, chimera-broker collapsed/expanded metrics and provider health strips, `buildGatewayCardIntroHtml`, `buildBrokerCardIntroHtml`, `buildVectorstoreCardIntroHtml`, `buildIndexerCardIntroHtml` (indexer **service** aggregate intro only), `gatewayServicePanelMiniHtml`, `vectorstoreServicePanelMiniHtml`, `chimeraBrokerServicePanelKvHtml`, `badgeForServicePanel`, broker relay helpers, `entryIsGatewayUpstreamRelay`, `entryRoutesToChimeraBrokerBucket`.
- Remove thin wrappers that only delegate to `ctx.serviceDisplayLabel` / `ctx.inferServiceBadge` / `ctx.serviceAvatar*` — call `ctx` directly in service builders ( [`serviceCard.js`](../../chimera/chimera-gateway/internal/server/adminui/embed/embedui/settings/render/cards/serviceCard.js) already exports these).
- Optional rename: `serviceFeed.js` + `mountServiceFeed` if clearer; update `mount.js`, HTML script tags, and tests in one PR (no alias re-exports required).
- Trim `ctx` exports at end of mount to **only** symbols consumed outside the file (grep `ctx.<name>` across `settings/` and `embedui_test/`).

**Acceptance**

- `feedLogService.js` (or `serviceFeed.js`) line count ≤ ~650 (excluding comments).
- [`modelMount.js`](../../chimera/chimera-gateway/internal/server/adminui/embed/embedui/settings/summarized/modelMount.js) still builds `kind: "service"` cards via `ctx.buildServiceCard`.
- Gallery service fixtures still render (update ids/classes if changed).

**Status:** `done`

**As-built:** Indexer helpers moved to [`indexerFeedHelpers.js`](../../chimera/chimera-gateway/internal/server/adminui/embed/embedui/settings/render/cards/indexerFeedHelpers.js); `feedLogService.js` is service-only (~1k lines — broker panel HTML still inline; ~650 target optional via broker extract). Avatar/label wrappers removed; `ctx` exports trimmed; `mountFeedLogService` removed from `mountAll` except for gateway/admin broker deps (still mounted once there + again in feed chain).

---

## Phase 4 — Indexer ownership

**Goal.** Indexer run and workspace modules own their HTML and metadata; no indexer symbols exported from the service module. (Partially satisfied: definitions live in `indexerFeedHelpers.js`; Phase 4 moves them into `indexerRun.js` / `indexerWorkspace.js` and deletes the bridge module.)

**Deliverables**

- **Move to `indexerRun.js`:** `collectIndexerRunMeta`, `flatLooksLikeIndexerRun*`, `flatLooksLikeIndexerJobIngested`, `indexerFlatMsg`, `isIndexerStateFlat`, `latestIndexerStateQueueInflightFromEntries`, `indexerRecentEvalStatusForFlat`, `buildIndexerRecentEvaluatedFilesHtml`, `indexerBuildCardSubtitle`, workspace metric wells used on run cards, `badgeForIndexerRunLine`, `indexerRunProgressSubtitle`, `indexerCardDomIdFromMeta`, `workspaceCardTitleFromIndexerMeta`, `indexerLatestSupervisedWaitFlat`, evlog workspace label map (`buildIndexerEvlogWorkspaceLabelMap`, `getIndexerEvlogWorkspaceLabelMap`, …), `badgeForIndexerRunLine` consumers in `sumEvlog.js` unchanged via `ctx`.
- **Move to `indexerWorkspace.js`:** `dedupeOperatorWorkspacesNested`, `canonicalWorkspaceRowIdKey`, `normalizeFlavorMatch`, `resolveLogsOperatorUserLabel`, `operatorManagedWorkspaceTitleText`, `workspaceDraftComparableManagedTitle`, `buildIndexerManagedWorkspaceSummaryRowsFromOperatorStore`, log-derived managed rows, `aggregateIndexerManagedWorkspacesHtml`, `hydrateIndexerServiceSummaryFromApi`, `scheduleIndexerServiceSummaryFetch`, `syncIndexerServiceSummaryDom`, `indexerServiceSummaryConfigPathHtml`, `indexerServiceSummaryWorkspacesHtml`, path merge helpers, bucket href helpers used only for workspace summaries.
- **Service module** keeps chimera-indexer **service card** expanded block that *calls* `ctx.indexerServiceSummaryWorkspacesHtml` / `ctx.indexerServiceSummaryConfigPathHtml` — those functions live on `ctx` from workspace mount (mount workspace **after** service feed, or service calls `ctx` at render time only).
- Update [`summarized/model.js`](../../chimera/chimera-gateway/internal/server/adminui/embed/embedui/settings/summarized/model.js) / [`modelMount.js`](../../chimera/chimera-gateway/internal/server/adminui/embed/embedui/settings/summarized/modelMount.js) `summarizedModelDeps` to reference new `ctx` locations (no bare identifiers).
- Delete obsolete extraction scripts or refresh [`extract-feed-phase4.py`](../../chimera/chimera-gateway/internal/server/adminui/embed/embedui/scripts/extract-feed-phase4.py) symbol lists if still used.

**Acceptance**

- `feedLogService.js` has zero `collectIndexerRunMeta` / `flatLooksLikeIndexer` / `dedupeOperatorWorkspaces` definitions.
- `indexerRun.js` + `indexerWorkspace.js` pass existing indexer stale/run/workspace card tests.
- [`feed_mount_exports_test.go`](../../chimera/chimera-gateway/internal/server/adminui/embed/embedui_test/feed_mount_exports_test.go) `requiredCtxExportsAfterCardMount` lists symbols **after** full mount chain with correct owning module comments optional.

**Status:** `done`

**As-built:** Deleted `indexerFeedHelpers.js`; merged helpers into `indexerRun.js` (~1.6k lines) and `indexerWorkspace.js` (~850 lines) via `scripts/merge-indexer-feed-helpers-phase4.py`. Feed mount chain: `feedCardShared` → `indexerRun` → `indexerWorkspace` → `conv` → `service`. `collectIndexerRunMeta` / evlog map on run mount; workspace paths + `hydrateIndexerServiceSummaryFromApi` on workspace mount. Tests updated (`TestFeedLogIndexerPhase4_workspaceAndRunExports`).

---

## Phase 5 — Mount order and tests

**Goal.** One mount path, documented inventory, and CI that prevents regression to monolithic `feedLogService.js`.

**Deliverables**

- Remove duplicate `mountFeedLogService` from `mount.js`; document final order in [`CARD_INVENTORY.md`](../../chimera/chimera-gateway/internal/server/adminui/embed/embedui/settings/CARD_INVENTORY.md) and [`settings/README.md`](../../chimera/chimera-gateway/internal/server/adminui/embed/embedui/settings/README.md).
- Update `requiredCtxExportsAfterCardMount` and add tests:
  - `TestFeedLogService_noBareIndexerCalls` — no bare `mergePersistedIndexerWatchRoots` / `indexerCardTitleSortLabel` in service module.
  - `TestFeedLogService_ctxExportsSubset` — every `ctx.foo =` in service module has ≥1 `ctx.foo(` consumer outside the assigning file (or listed exception in test comment).
  - `TestMount_order_feedCards` — assert mount sequence in `summarizedFeed.js` matches documented order.
- Update [`operator-settings-ui.md`](../features/operator-settings-ui.md) code map: service vs indexer run vs workspace modules.
- Mark plan **shipped**; link As-built in front matter.

**Acceptance**

- `go test ./chimera/chimera-gateway/internal/server/adminui/embed/embedui_test/...` green.
- `/ui/settings` summarized feed: Services section (4 cards), indexer runs, operator workspaces — expand/collapse and scoped evlog without console errors.
- `CARD_INVENTORY.md` row for `feedLogService.js` reads “service cards + broker health only”.

**Status:** `done`

**As-built:** Renamed `feedLogService.js` → `serviceFeed.js` (`mountServiceFeed`), `feedCardShared.js` → `cardChrome.js` (`mountCardChrome`). Added `mountSummarizedFeedCards` in `mount.js`; removed duplicate service/cardChrome mounts from `mountAll`. Settings feed and gallery call `mountSummarizedFeedCards` only. CI: `TestMount_summarizedFeedCards_order`, `TestSummarizedFeed_mountSummarizedFeedCardsOnly`, `TestServiceFeed_requiredExternalCtxConsumers`.

---

## Decisions

1. **No backwards compatibility** — DOM ids, `ctx` export names, script filenames, and mount duplication may change without shims. Update gallery, goja tests, and `feed_mount_exports_test.go` in the same PR as each phase.
2. **Call-time `ctx` resolution** — Cross-module helpers use `ctx.fn(...)` inside functions that run after all mounts complete; no bare symbols from later-mounted modules.
3. **Derive vs `ctx` for metrics** — Prefer `ChimeraSettings.Derive.*` for pure log rollups; reserve `ctx` for HTML builders and feed-local state (`lastIndexerOperatorWorkspacesNested`, broker snapshot).
4. **Optional `indexerEvlogLabels.js`** — If `indexerRun.js` exceeds ~1k lines after Phase 4, split evlog label map into `render/indexerEvlogLabels.js` mounted immediately before `mountFeedLogIndexerRun`.

---

## Open questions (resolved)

1. **Filename:** `serviceFeed.js` + `mountServiceFeed` (Phase 5).
2. **Shared chrome:** `cardChrome.js` + `mountCardChrome` — reusable on settings feed, gallery, and setup wizard via `mountSummarizedFeedCards`; not tied to `sharedFormat.js`.

---

## References

- Code: [`embed/embedui/settings/render/cardChrome.js`](../../chimera/chimera-gateway/internal/server/adminui/embed/embedui/settings/render/cardChrome.js), [`serviceFeed.js`](../../chimera/chimera-gateway/internal/server/adminui/embed/embedui/settings/render/cards/serviceFeed.js), [`indexerRun.js`](../../chimera/chimera-gateway/internal/server/adminui/embed/embedui/settings/render/cards/indexerRun.js), [`indexerWorkspace.js`](../../chimera/chimera-gateway/internal/server/adminui/embed/embedui/settings/render/cards/indexerWorkspace.js), [`summarizedFeed.js`](../../chimera/chimera-gateway/internal/server/adminui/embed/embedui/settings/app/summarizedFeed.js)
- Tests: [`embedui_test/feed_mount_exports_test.go`](../../chimera/chimera-gateway/internal/server/adminui/embed/embedui_test/feed_mount_exports_test.go), [`embedui_test/feed_smoke_test.go`](../../chimera/chimera-gateway/internal/server/adminui/embed/embedui_test/feed_smoke_test.go), [`embedui_test/goja_test.go`](../../chimera/chimera-gateway/internal/server/adminui/embed/embedui_test/goja_test.go) (`loadCardTestCtx`)
- Prior plan: [`embedui-settings-card-cleanup.md`](embedui-settings-card-cleanup.md)
- Feature (update on ship): [`operator-settings-ui.md`](../features/operator-settings-ui.md)
