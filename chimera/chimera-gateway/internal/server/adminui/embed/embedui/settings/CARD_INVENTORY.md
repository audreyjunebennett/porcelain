# Settings card inventory (Phase 1)

Audit for [`docs/plans/embedui-settings-card-cleanup.md`](../../../../../../../../docs/plans/embedui-settings-card-cleanup.md). Behavioral source of truth remains [`docs/features/operator-settings-ui.md`](../../../../../../../../docs/features/operator-settings-ui.md).

**Related:** [Card module contract](README.md#card-module-contract) · [Part registry](card-parts-registry.md) (`data-ui-part` slugs).

---

## Feed registration

Summarized cards are built in `summarized/model.js` (`buildSummarizedModel`) and rendered via `app/summarizedFeed.js` → `renderSummarizedCardFromModel` → `summarized/renderHtml.js` (`renderSummarizedHtml`).

| Summarized `kind` | Stable `id` pattern | `render/cards/` builder | On live `/ui/settings` feed |
|-------------------|---------------------|-------------------------|------------------------------|
| `gateway-overview` | `gw-overview` | `gatewayOverview.js` → `buildGatewayOverviewCardHtml` | Yes |
| `gateway-usage` | `gw-usage-metrics` | `gatewayUsage.js` → `buildGatewayUsageCardHtml` | Yes |
| `admin-users` | `admin-users` (section wrapper) | `adminUsers.js` → `buildAdminUsersCardHtml` | Yes |
| `admin-provider` | `admin-provider-{providerId}` | `adminProvider.js` → `buildAdminProviderCardHtml` | Yes |
| `virtual-model-draft` | `virtual-model-draft-{n}` | `adminVirtualModels.js` → `buildVirtualModelDraftCardHtml` | Yes |
| `virtual-model` | `virtual-model-{id}` | `adminVirtualModels.js` → `buildVirtualModelCardHtml` | Yes |
| `section-break` | `section-break-{sortKey}` | HTML from deps (`adminProvidersSectionBreakHtml`, VM section break/intro) | Yes (chrome only) |
| `conversation` | dynamic (`conversationDomIdForGroup`) | `feedLogConv.js` → `buildConvCard` (mounted from feed before `mountAll`) | Yes |
| `service` | `svc-{hash(serviceName)}` | `feedLogService.js` → `buildServiceCard` (+ `serviceCard.js` avatar helpers) | Yes |
| `indexer` | per-run id from feed | `indexerRun.js` → `buildIndexerCard` | Yes |
| `indexer-stale` | per-bucket | `indexerRun.js` → `buildIndexerStaleSnapshotCard` | Yes |
| `workspace-draft` | `workspace-draft-{id}` | `workspaceDraft.js` → `buildWorkspaceDraftCardHtml` | Yes (when drafts exist) |
| `indexer-operator-workspace` | `opws-{workspaceId}` | `indexerWorkspace.js` → `buildIndexerOperatorWorkspaceCard` | Yes |

Per-VM routing/fallback/tool-router UI is **inside** `buildVirtualModelCardHtml` (`buildRoutingSection`, `buildFallbackSection`, `buildToolRouterSection`); handlers in `handlers/virtualModelsAdmin.js`. Global gateway cards (`admin-routing-rules`, `admin-fallback-chain`, `admin-router-model`) were removed in favor of this layout.

---

## `render/cards/` module inventory

Mount order: `mountAll(ctx)` in `mount.js` (`serviceCard` → `feedLogService` → gateway/admin) then in `summarizedFeed.js`: `mountFeedLogConv` → `mountFeedLogIndexerRun` → `mountFeedLogIndexerWorkspace` (re-assigns feed `ctx` exports). Orchestrator uses `ctx.*` only. CI: `embedui_test/card_mount_shadow_test.go`, `feed_mount_exports_test.go`, `feed_smoke_test.go`. Rebuild guards: `summarized/rebuildPolicy.js`.

| File | `mount*` | Primary `ctx` exports | Handler(s) | APIs (representative) | Duplication / notes | Refactor priority |
|------|----------|----------------------|------------|----------------------|---------------------|-------------------|
| `sharedFormat.js` | `mountSharedFormat` | `formatInt`, `aggregateRollupRows`, `formatCompactTok`, time formatters | — | — | Shared formatters; not a card | — (support) |
| `adminShared.js` | `mountAdminShared` | YAML parse/format, provider health, tier spans; delegates configure/evlog/credentials HTML to `ChimeraShared` | `admin.js`, `providerModelsAdmin.js`, cards | `/api/ui/chimera-broker/providers` (read via ctx poll) | Remaining YAML/usage helpers | **P1** |
| `gatewayOverview.js` | `mountGatewayOverview` | `buildGatewayOverviewCardHtml`, `buildGatewayOverviewFeedSection`, `gatewayServiceHealthStripHtml` | Feed poll patches overview | `GET /api/ui/state` | `buildGatewayOverviewFeedSection` unused on live path (like workflows) | **P1** |
| `gatewayUsage.js` | `mountGatewayUsage` | `buildGatewayUsageCardHtml`, `buildGatewayUsageIntroHtml` | Feed `patchGatewayUsageMetricsCard` | `GET /api/ui/metrics` | Intro + card split | **P1** |
| `adminUsers.js` | `mountAdminUsers` | `buildAdminUsersCardHtml`, `buildAdminUserDraftCardHtml`, `adminBuildUserCardHtml` | `admin.js` (`user-add`, `user-draft-*`, token revoke) | `GET/POST /api/ui/tokens` | Draft pattern mirrors workspace draft | **P1** |
| `adminProvider.js` | `mountAdminProvider` | `buildAdminProviderCardHtml`, `providerHasCredentials` | `admin.js` (keys, Ollama URL), `providerModelsAdmin.js` (availability) | `POST /api/ui/provider/{id}/keys`, `.../ollama/base_url`, `PUT .../models` | Panel/toolbar pattern; model for VM sections | **P0** |
| `adminVirtualModels.js` | `mountAdminVirtualModels` | `buildVirtualModelCardHtml`, draft/section helpers | `virtualModelsAdmin.js` | `GET/PUT/POST/DELETE /api/ui/virtual-models/...` | Routing/fallback/tool-router sections inline (not shared card builders) | **P0** |
| `workspaceDraft.js` | `mountWorkspaceDraft` | `buildWorkspaceDraftCardHtml`, `buildManagedWorkspace*` toolbar/paths | `admin.js` + feed workspace save | `POST /api/ui/indexer/workspaces` | Shared `WorkspacePaths` + `EditToolbar` | **P1** |
| `feedLogConv.js` | `mountFeedLogConv` | `buildConvCard`, conv metrics/evlog, `avatarInitials`, `sliceRecent`, error helpers | `evlog.js`, feed | — | Phase 4 extraction from feed | — |
| `indexerRun.js` | `mountFeedLogIndexerRun` | `buildIndexerCard`, `buildIndexerStaleSnapshotCard`, watch-root store, dedupe keys | feed, `feedLogService.js` helpers | — | Phase 4b | — |
| `indexerWorkspace.js` | `mountFeedLogIndexerWorkspace` | `buildIndexerOperatorWorkspaceCard`, path apply, coverage | feed, `workspaceDraft.js` toolbars | — | Phase 4b | — |
| `feedLogService.js` | `mountFeedLogService` | `buildServiceCard`, indexer evlog/meta helpers, broker health strips | feed, `evlog.js` | — | Service + shared indexer helpers | — |
| `convCard.js` | `mountConvCard` | `formatMergedConversationSubtitle` only | `evlog.js`, feed | — | Subtitle helper; body in `feedLogConv.js` | — |
| `serviceCard.js` | `mountServiceCard` | `serviceAvatarClass`, `serviceAvatarInitials` | — | — | Avatar helpers only | — |
| `mount.js` | `mountAll` | orchestrates admin/gateway card mounts | — | — | Log feed mounts called from feed, not `mountAll` | — |

---

## Draft and edit-state conventions (`ctx`)

| State key | Card family | DOM / data attributes |
|-----------|-------------|------------------------|
| `ctx.adminUserDrafts[]` | Users | `data-admin-user-draft`, `data-admin-user-field`, `data-draft-id`, actions `user-draft-save` / `user-draft-cancel` |
| `ctx.tokenListCache` | Users (saved) | Section `#admin-users`; per-user `id="admin-user-{principal}"` |
| `ctx.adminProviderKeyDraft.{providerId}` | Provider keys | `id="admin-{provider}-key"` |
| `ctx.adminOllamaUrlDraft` | Ollama URL | Ollama-specific inputs in provider card |
| `ctx.adminProviderModelsEditingId` | Provider models | `sum-card--provider-models-editing`; actions `provider-models-*` |
| `ctx.virtualModelDrafts[]` | VM create | `id="virtual-model-draft-{id}"`, `data-vm-draft-field`, `vm-draft-save` |
| `ctx.virtualModelUi[vmId]` | VM saved | `identityEditing`, `fallbackEditing`, `routingEditing`, `routerEditing`, drafts `policyDraft`, `fallbackDraft`, …; actions `vm-*` |
| `ctx.workspaceDrafts[]` | Indexer workspace create | `data-workspace-draft`, draft article id `workspace-draft-{id}` |
| `ctx.workspaceManagedEditId` / `ctx.workspaceManagedStaging` | Managed workspace paths | Feed-built operator workspace card; actions in `admin.js` |

Interaction guards: `summarized/rebuildPolicy.js` (`summarizedPanelInteractionBlocksRebuild`, `summarizedPatchSkipCardIds`, `summarizedAdminEditingActive`); YAML dirty routing in `summarizedDirtyRouting.js`.

---

## Monolithic builders still in `summarizedFeed.js`

Target for later extraction (~3.2k lines in `app/summarizedFeed.js` after Phase 4b). Grouped by section.

### Shipped (Phase 4–4b)

| Function | Module |
|----------|--------|
| `buildConvCard` | `feedLogConv.js` |
| `buildServiceCard` | `feedLogService.js` |
| `buildIndexerCard`, `buildIndexerStaleSnapshotCard` | `indexerRun.js` |
| `buildIndexerOperatorWorkspaceCard` | `indexerWorkspace.js` |
| `buildWorkspacesCreateBtnHtml`, `buildWorkspacesSectionIntroHtml` | `workspaceDraft.js` |

### Feed orchestration (stay in feed)

| Function | ~role |
|----------|--------|
| `buildHtmlForSummarizedCardId` | Fallback router; delegates to `ctx.build*` |
| `filterEventsForIndexerScopeFullLog`, `operatorWorkspaceSyntheticBucketId` | Indexer scope filtering for model + opws cards |
| `buildSummarizedAggregateState`, poll/SSE, managed workspace save handlers | Orchestration only |

### Overview chrome (small)

| Function | Notes |
|----------|--------|
| `buildAdminProviderPickerHtml`, `buildAdminProvidersSectionBreakHtml` | Could move to `adminProvider.js` |

### Orchestration (stay in feed)

`buildSummarizedModelForAgg`, `renderSummarizedUnified`, `replaceCardById`, `patchAdminCardsFromPoll`, `buildSummarizedAggregateState`, poll/SSE wiring — Phase 4 documents rebuild vs patch only.

---

## Part regions — Phase 6 seed

Slug pattern: `{card-kind}.{region}`. Canonical list: [`card-parts-registry.md`](card-parts-registry.md). Production builders set `data-ui-part`; gallery overlay toggles labels via `body.gallery-show-parts`.

### `gateway-overview` (`gw-overview`)

| Seed slug | Region |
|-----------|--------|
| `gateway-overview.summary` | `<summary>` title, subtitle, compact health |
| `gateway-overview.health-strip` | Expanded service health strip |
| `gateway-overview.kv` | Version / virtual model / updated KV |

### `gateway-usage` (`gw-usage-metrics`)

| Seed slug | Region |
|-----------|--------|
| `gateway-usage.summary` | Card header metrics chips |
| `gateway-usage.intro` | `#gw-usage-intro` |
| `gateway-usage.tables` | Rollup / event tables |

### `admin-users` (`admin-users`)

| Seed slug | Region |
|-----------|--------|
| `admin-users.section-head` | Title + Add user |
| `admin-users.drafts` | `.sg-op-user-drafts-stack` |
| `admin-users.saved-card` | Per `admin-user-{principal}` article |
| `admin-users.scoped-evlog` | Per-user scoped log |

### `admin-provider` (`admin-provider-{id}`)

| Seed slug | Region |
|-----------|--------|
| `admin-provider.summary` | `<summary>` |
| `admin-provider.intro` | Provider blurb / link |
| `admin-provider.models-toolbar` | Configure / save / cancel icon row |
| `admin-provider.models-list` | `sg-op-provider-model-item` list |
| `admin-provider.keys` | Key list + add block |
| `admin-provider.scoped-evlog` | In-card `sum-evlog` |

### `virtual-model` (`virtual-model-{id}`)

| Seed slug | Region |
|-----------|--------|
| `virtual-model.summary` | Card `<summary>` |
| `virtual-model.client-usage` | Client usage block |
| `virtual-model.identity` | `data-vm-section="identity"` |
| `virtual-model.fallback` | `data-vm-section="fallback"` |
| `virtual-model.routing` | `data-vm-section="routing"` |
| `virtual-model.tool-router` | `data-vm-section="router"` |
| `virtual-model.scoped-evlog` | Scoped routing log |

### `virtual-model-draft`

| Seed slug | Region |
|-----------|--------|
| `virtual-model-draft.form` | Draft fields + save/cancel |

### `workspace-draft` / `indexer-operator-workspace`

| Seed slug | Region |
|-----------|--------|
| `workspace-draft.form` | Project/flavor/paths fields |
| `indexer-operator-workspace.toolbar` | Configure / path edit toolbar |
| `indexer-operator-workspace.paths` | Watch roots table |
| `indexer-operator-workspace.scoped-evlog` | Workspace-scoped log |

### `conversation` / `service` / `indexer` (feed-built)

| Kind | Seed slugs (high level) |
|------|-------------------------|
| `conversation` | `conversation.summary`, `conversation.timeline`, `conversation.metrics`, `conversation.scoped-evlog` |
| `service` | `service.summary`, `service.health-timeline`, `service.metrics`, `service.scoped-evlog` |
| `indexer` | `indexer.summary`, `indexer.progress`, `indexer.kv`, `indexer.scoped-evlog` |

### Virtual model sub-panels (live site)

| Kind | Seed slug | Note |
|------|-----------|------|
| `virtual-model` | `virtual-model.routing`, `virtual-model.fallback`, `virtual-model.tool-router` | Per-VM sections in `adminVirtualModels.js` |

---

## `embedui/shared/` (Phase 2–3)

| Module | Role |
|--------|------|
| `operatorFeedback.js` | `#status` messages; save-button pending (`aria-disabled`) |
| `configureEdit.js` | `configureBtnInline`; `restoreEditOnCancel` |
| `yamlEditor.js` | YAML wrap dirty/vscroll; `textareaWrapHtml`; `applyTextareaInputDirty` |
| `draftInput.js` | Provider key + Ollama URL draft fields on `ctx` |
| `providerCredentials.js` | `keyAddBlockHtml`, `runProviderKeyAdd`, `runOllamaUrlSave` |
| `scopedEvlog.js` | `panelFromEvents` (deps wired from `adminShared` mount) |
| `adminAction.js` | `runJson` for handler save/generate flows |
| `editToolbar.js` | Provider/VM/managed-workspace icon toolbars |
| `workspacePaths.js` | Draft + managed watched-paths editor row |
| `serviceHealth.js` | Gateway overview + service card health segments |

Loaded at `/ui/assets/shared/*.js` before card modules. Settings-only glue stays in `settings/handlers/` and `settings/app/`.
