# Settings card part registry

Stable part slugs for operator settings cards: docs, agents, gallery overlays, and (future) `describeCard` context export.

**Slug pattern:** `{card-kind}.{region}`

**DOM:** Production builders set `data-ui-part="{slug}"` on major regions. Gallery fixtures use the same modules, so attributes match `/ui/settings` without duplication.

**Legacy:** Global `admin-routing-*` cards are gallery-only static HTML (see `settings/gallery.html` § Legacy routing demos). Per-VM routing/fallback/tool-router sections in `adminVirtualModels.js` are live.

## Card-level ids

| Card kind | Feed / DOM `id` pattern | Builder |
|-----------|------------------------|---------|
| Gateway overview | `gw-overview` | `gatewayOverview.js` |
| Gateway usage | `gw-usage-metrics` | `gatewayUsage.js` |
| Admin provider | `admin-provider-{providerId}` | `adminProvider.js` |
| Virtual model | `virtual-model-{rowId}` | `adminVirtualModels.js` |
| Virtual model draft | `virtual-model-draft-{draftId}` | `adminVirtualModels.js` |
| Workspace draft | `ws-draft-{draftId}` | `workspaceDraft.js` |
| Indexer operator workspace | `ix-opws-{hash}` | `indexerWorkspace.js` |
| Indexer stale snapshot | `ix-stale-{hash}` | `indexerRun.js` |
| Indexer run | `ix-{hash}` | `indexerRun.js` |
| Conversation | `conv-{hash}` | `feedLogConv.js` |
| Service | service name slug | `serviceFeed.js` |

Summarized feed `kind` values align with these families (`admin-provider`, `virtual-model`, `workspace-draft`, `indexer`, …) — see [`summarized/model.js`](summarized/model.js).

## Parts — gateway overview

| Slug | Label | DOM hint | Builder |
|------|-------|----------|---------|
| `gateway-overview.summary` | Card header (title, subtitle, compact health) | `<summary data-ui-part>` | `gatewayOverview.js` |
| `gateway-overview.health-strip` | Expanded service health strip | `div[data-ui-part]` wrapping strip | `gatewayOverview.js` |
| `gateway-overview.kv` | Version / virtual model / updated | `dl[data-ui-part].indexer-run-kv--gateway-summary` | `gatewayOverview.js` |

## Parts — admin provider

| Slug | Label | DOM hint | Builder |
|------|-------|----------|---------|
| `admin-provider.summary` | Card `<summary>` | `#admin-provider-{id} > summary` | `adminProvider.js` |
| `admin-provider.intro` | Provider blurb + docs link | `p.sg-op-provider-intro` | `adminShared.js` |
| `admin-provider.models-toolbar` | Configure / save / cancel icons | `.sg-op-provider-panel__actions` | `adminProvider.js` |
| `admin-provider.models-list` | Model usage rows | `.sg-op-provider-panel__body` (usage panel) | `adminProvider.js` |
| `admin-provider.keys` | API keys or Ollama URL panel | `section.sg-op-provider-panel--keys\|endpoint` | `adminProvider.js` |
| `admin-provider.scoped-evlog` | In-card scoped log | `.sum-evlog[data-ui-part]` | `adminShared.js` → `scopedEvlog.js` |

## Parts — virtual model

| Slug | Label | DOM hint | Builder |
|------|-------|----------|---------|
| `virtual-model.summary` | Card `<summary>` | `#virtual-model-{id} > summary` | `adminVirtualModels.js` |
| `virtual-model.client-usage` | Public / enabled toggles bar | `div[data-ui-part].sum-vm-section--bar` | `adminVirtualModels.js` |
| `virtual-model.identity` | Identity section | `details[data-ui-part][data-vm-section=identity]` | `adminVirtualModels.js` |
| `virtual-model.fallback` | Fallback chain section | `details[data-ui-part][data-vm-section=fallback]` | `adminVirtualModels.js` |
| `virtual-model.routing` | Routing policy section | `details[data-ui-part][data-vm-section=routing]` | `adminVirtualModels.js` |
| `virtual-model.tool-router` | Tool router section | `details[data-ui-part][data-vm-section=router]` | `adminVirtualModels.js` |
| `virtual-model.scoped-evlog` | Scoped routing log | `.sum-evlog[data-ui-part]` | `adminVirtualModels.js` |

## Parts — virtual model draft

| Slug | Label | DOM hint | Builder |
|------|-------|----------|---------|
| `virtual-model-draft.form` | Draft fields + actions | `.sum-body[data-ui-part]` | `adminVirtualModels.js` |

## Parts — workspace draft

| Slug | Label | DOM hint | Builder |
|------|-------|----------|---------|
| `workspace-draft.form` | Project / flavor / paths | `.sum-body[data-ui-part]` | `workspaceDraft.js` |

## Parts — indexer operator workspace

| Slug | Label | DOM hint | Builder |
|------|-------|----------|---------|
| `indexer-operator-workspace.summary` | Card header | `header.sum-card__hdr` | `indexerWorkspace.js` |
| `indexer-operator-workspace.toolbar` | Configure / path edit toolbar | `div[data-ui-part].ws-managed-edit-controls` | `workspaceDraft.js` |
| `indexer-operator-workspace.paths` | Watched paths block | `dd[data-ui-part]` (Watched paths) | `indexerRun.js` (`renderExpandedIndexer`) |
| `indexer-operator-workspace.scoped-evlog` | Workspace-scoped log | `.sum-evlog[data-ui-part]` | `indexerRun.js` |

## Parts — indexer stale snapshot

| Slug | Label | DOM hint | Builder |
|------|-------|----------|---------|
| `indexer-stale.summary` | Card `<summary>` | `details.sum-card--indexer-stale > summary` | `indexerRun.js` |

## Parts — feed cards (seed; not all gallery demos)

| Slug | Label | Builder |
|------|-------|---------|
| `conversation.summary` | Conversation card header | `feedLogConv.js` |
| `conversation.timeline` | Lifecycle timeline | `feedLogConv.js` |
| `conversation.scoped-evlog` | Scoped log | `feedLogConv.js` |
| `service.summary` | Service card header | `serviceFeed.js` |
| `service.scoped-evlog` | Service scoped log | `serviceFeed.js` |
| `indexer.summary` | Indexer run card header | `indexerRun.js` |
| `indexer.scoped-evlog` | Indexer run scoped log | `indexerRun.js` |

## Future

Optional `describeCard(cardEl, ctx)` for operator chat context should return the same slugs and visible text per region. No collector in the gateway until a consumer exists.

## Maintenance

When adding a region: update this file, set `data-ui-part` in the builder, add a gallery caption bullet if the card appears on `/ui/settings/gallery`, and grep-check slug parity:

```bash
rg 'data-ui-part="' settings/render/cards shared scopedEvlog.js
```
