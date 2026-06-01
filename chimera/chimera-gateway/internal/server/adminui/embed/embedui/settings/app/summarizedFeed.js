/**
 * Summarized panel rebuild, service/conversation cards, and unified feed render.
 *
 * Exports: ChimeraSettings.App.mountSummarizedFeed(ctx)
 */

globalThis.ChimeraSettings = globalThis.ChimeraSettings || {};
globalThis.ChimeraSettings.App = globalThis.ChimeraSettings.App || {};
globalThis.ChimeraSettings.App.mountSummarizedFeed = function (ctx) {
  var statusEl = ctx.statusEl;
  var entryCache = ctx.entryCache;
  var getViewMode = ctx.getViewMode;
  var getFlat = ctx.getFlat;
  var escapeHtml = ctx.escapeHtml;
  var strHash = ctx.strHash;
  var entryInstant = ctx.entryInstant;
  var normalizeServiceBucketKey = ctx.normalizeServiceBucketKey;
  var primaryLogMessage = ctx.primaryLogMessage;
  var stickPx = ctx.stickPx;
  var embedded = ctx.embedded;

  if (
    globalThis.ChimeraSettings.Api &&
    typeof ChimeraSettings.Api.mountAdminClient === "function"
  ) {
    ChimeraSettings.Api.mountAdminClient(ctx);
  }

  function providerCatalogApi() {
    return globalThis.ChimeraSettings &&
      ChimeraSettings.Providers &&
      ChimeraSettings.Providers.Catalog
      ? ChimeraSettings.Providers.Catalog
      : null;
  }

  function adminVisibleProviderIds() {
    return Array.isArray(ctx.adminVisibleProviderIds) ? ctx.adminVisibleProviderIds : [];
  }

  function adminProviderSpecsFromVisible() {
    var api = providerCatalogApi();
    if (!api) return [];
    return api.buildSpecsFromVisibleIds(adminVisibleProviderIds());
  }

  function lookupAdminProviderSpec(providerId) {
    var api = providerCatalogApi();
    return api ? api.lookupProviderSpec(providerId) : null;
  }

  var CHIMERA_BROKER_PROVIDER_STALE_MS = 90000;
  ctx.uiUnauthorized = false;

  if (
    globalThis.ChimeraSettings.Summarized &&
    typeof ChimeraSettings.Summarized.mountRebuildPolicy === "function"
  ) {
    ChimeraSettings.Summarized.mountRebuildPolicy(ctx);
  }
  var summarizedAdminEditingActive = ctx.summarizedAdminEditingActive;
  var summarizedPanelInteractionBlocksRebuild = ctx.summarizedPanelInteractionBlocksRebuild;
  var summarizedPatchSkipCardIds = ctx.summarizedPatchSkipCardIds;
  var summarizedSkippedCardsHashDelta = ctx.summarizedSkippedCardsHashDelta;

  var panelMod;
  var patchMod;
  var pollMod;
  var modelGlue;
  var buildSummarizedModelForAgg;
  var renderSummarizedCardFromModel;

  function syncSummarizedModelCache() {
    var snap = buildSummarizedFeedSnapshot();
    ctx.lastSummarizedModel = snap.model;
    ctx.lastSummarizedAggregate = snap.agg;
  }

  function refreshAdminCardAfterEditToggle(patchFn) {
    if (typeof patchFn === "function" && patchFn()) {
      syncSummarizedModelCache();
      return;
    }
    ctx.summarizedForceFullRebuild = true;
    refreshSummarizedPanel();
  }

  function scheduleDeferredSummarizedRefresh() {
    if (ctx.sumEvlogUiDeferTimer) clearTimeout(ctx.sumEvlogUiDeferTimer);
    ctx.sumEvlogUiDeferTimer = setTimeout(function deferredSumEvlogRefresh() {
      ctx.sumEvlogUiDeferTimer = null;
      if (summarizedPanelInteractionBlocksRebuild()) {
        ctx.sumEvlogUiDeferTimer = setTimeout(deferredSumEvlogRefresh, 300);
        return;
      }
      refreshSummarizedPanel();
    }, 300);
  }

  function refreshSummarizedPanel() {
    var psu = document.getElementById("panel-summarized");
    if (getViewMode() !== "summarized" || !psu) return;
    if (summarizedPanelInteractionBlocksRebuild()) {
      scheduleDeferredSummarizedRefresh();
      return;
    }
    patchMod.clearSummarizedDirtySets();

    var forceFull = !!ctx.summarizedForceFullRebuild;
    ctx.summarizedForceFullRebuild = false;

    var snap = buildSummarizedFeedSnapshot();
    var nextModel = snap.model;
    var agg = snap.agg;
    var prevModel = ctx.lastSummarizedModel;

    if (!forceFull && prevModel && prevModel.cards && patchMod.summarizedPatchAvailable()) {
      var Patch = ChimeraSettings.Summarized.Patch;
      var ops = Patch.diffSummarizedModels(prevModel, nextModel, {
        skipCardIds: summarizedPatchSkipCardIds()
      });
      if (!Patch.shouldUseFullRebuildFromOps(ops)) {
        var replaceCount = Patch.countReplaceCardOps(ops);
        if (replaceCount === 0) {
          if (summarizedSkippedCardsHashDelta(prevModel, nextModel)) {
            panelMod.applySummarizedFullPanelRebuild(psu, nextModel, agg);
            ctx.lastSummarizedModel = nextModel;
            ctx.lastSummarizedAggregate = agg;
            return;
          }
          ctx.lastSummarizedModel = nextModel;
          ctx.lastSummarizedAggregate = agg;
          return;
        }
        if (!patchMod.shouldSummarizedDirtyFullRebuild(replaceCount)) {
          var patchResult = patchMod.applySummarizedPanelPatch(psu, ops);
          if (patchResult.ok) {
            ctx.lastSummarizedModel = nextModel;
            ctx.lastSummarizedAggregate = agg;
            return;
          }
        }
      }
    }

    panelMod.applySummarizedFullPanelRebuild(psu, nextModel, agg);
  }

  function forceSummarizedFullRebuild(reason) {
    ctx.summarizedForceFullRebuild = reason || true;
    refreshSummarizedPanel();
  }

  window.__chimeraToggleGatewayProbes = function (on) {
    ctx.gatewayPanelShowProbes = !!on;
    refreshSummarizedPanel();
  };

  function cancelCoalescedFullRebuild() {
    if (ctx.coalescedFullRebuildTimer) {
      clearTimeout(ctx.coalescedFullRebuildTimer);
      ctx.coalescedFullRebuildTimer = null;
    }
  }

  function scheduleStoryRebuild() {
    if (summarizedAdminEditingActive()) {
      scheduleDeferredSummarizedRefresh();
      return;
    }
    cancelCoalescedFullRebuild();
    if (ctx.storyRebuildTimer) clearTimeout(ctx.storyRebuildTimer);
    ctx.storyRebuildTimer = setTimeout(function () {
      ctx.storyRebuildTimer = null;
      ctx.suppressSummarizedDirty = true;
      try {
        forceSummarizedFullRebuild("structural");
      } finally {
        if (!ctx.summarizedLiveSettleTimer) ctx.suppressSummarizedDirty = false;
      }
    }, 0);
  }

  var WORKSPACE_WEB_UNAVAILABLE_TITLE =
    "Not available through the web. Use the desktop app.";

  function workspaceDesktopFeaturesAvailable() {
    if (typeof ctx.nativeFolderPickerFn === "function") return !!ctx.nativeFolderPickerFn();
    try {
      var topw = window.top;
      if (topw && typeof topw.chimeraPickFolder === "function") return true;
    } catch (_eWsPick) {}
    return typeof window.chimeraPickFolder === "function";
  }

  function wrapDesktopOnlyLockedControl(btnHtml, locked, overlay) {
    if (!locked) return btnHtml;
    var cls = "ws-desktop-only-locked";
    if (overlay) cls += " ws-desktop-only-locked--overlay";
    return (
      '<span class="' +
      cls +
      '" title="' +
      escapeHtml(WORKSPACE_WEB_UNAVAILABLE_TITLE) +
      '">' +
      btnHtml +
      "</span>"
    );
  }

  function adminSetMessage(kind, msg) {
    if (
      globalThis.ChimeraShared &&
      ChimeraShared.OperatorFeedback &&
      typeof ChimeraShared.OperatorFeedback.setStatusMessage === "function"
    ) {
      ChimeraShared.OperatorFeedback.setStatusMessage(statusEl, kind, msg);
      return;
    }
    if (!statusEl) return;
    statusEl.textContent = msg || "";
    statusEl.className = msg ? (kind === "err" ? "status-line err" : "status-line") : "status-line";
  }

  function fetchAdminState() {
    return pollMod.fetchUiState();
  }

  function buildSummarizedAggregateState() {
    if (
      !globalThis.ChimeraSettings ||
      !ChimeraSettings.Summarized ||
      typeof ChimeraSettings.Summarized.buildAggregateState !== "function"
    ) {
      return {
        groups: {},
        reqToConv: {},
        indexRunToConv: {},
        buckets: {},
        byRun: {},
        partitionRegistry: {},
        mergedConv: []
      };
    }
    var result = ChimeraSettings.Summarized.buildAggregateState(entryCache, {
      getFlat: getFlat,
      entryInstant: entryInstant,
      normalizeServiceBucketKey: normalizeServiceBucketKey,
      collectIndexerRunMeta: ctx.collectIndexerRunMeta,
      indexerRootScopeByRootId: ctx.indexerRootScopeByRootId,
      operatorWsFullLogCtx: ctx.operatorWsFullLogCtx,
      lastIndexerOperatorWorkspacesNested: ctx.lastIndexerOperatorWorkspacesNested
    });
    ctx.lastIndexerSummarizeByRun = result.byRun;
    ctx.lastIndexerSummarizePartitionRegistry = result.partitionRegistry;
    ctx.summarizedReqToConv = result.reqToConv;
    ctx.summarizedIndexRunToConv = result.indexRunToConv;
    if (result.scopeState && result.scopeState.indexerRootScopeByRootId) {
      ctx.indexerRootScopeByRootId = result.scopeState.indexerRootScopeByRootId;
    }
    return {
      groups: result.groups,
      reqToConv: result.reqToConv,
      indexRunToConv: result.indexRunToConv,
      buckets: result.buckets,
      byRun: result.byRun,
      partitionRegistry: result.partitionRegistry,
      mergedConv: result.mergedConv
    };
  }

  function entryRoutesToChimeraBrokerBucket(ent) {
    var D = globalThis.ChimeraSettings && ChimeraSettings.Derive ? ChimeraSettings.Derive : null;
    if (D && typeof D.entryRoutesToChimeraBrokerBucket === "function") {
      return D.entryRoutesToChimeraBrokerBucket(ent, getFlat);
    }
    if (typeof ctx.entryRoutesToChimeraBrokerBucket === "function") {
      return ctx.entryRoutesToChimeraBrokerBucket(ent);
    }
    return false;
  }

  function buildSummarizedFeedSnapshot() {
    ctx.operatorWsFullLogCtx = {};
    var agg = buildSummarizedAggregateState();
    var model = buildSummarizedModelForAgg(agg);
    return { agg: agg, model: model };
  }

  function renderSummarizedHtmlFromModel(model) {
    if (
      model &&
      globalThis.ChimeraSettings.Summarized.Render &&
      typeof ChimeraSettings.Summarized.Render.renderSummarizedHtml === "function"
    ) {
      return ChimeraSettings.Summarized.Render.renderSummarizedHtml(
        model,
        typeof ctx.buildSummarizedHtmlRenderers === "function" ? ctx.buildSummarizedHtmlRenderers() : {}
      );
    }
    return "";
  }

  function renderSummarizedUnified() {
    var snap = buildSummarizedFeedSnapshot();
    ctx.lastSummarizedModel = snap.model;
    ctx.lastSummarizedAggregate = snap.agg;
    return renderSummarizedHtmlFromModel(snap.model);
  }

  ctx.workspaceDesktopFeaturesAvailable = workspaceDesktopFeaturesAvailable;
  ctx.CHIMERA_BROKER_PROVIDER_STALE_MS = CHIMERA_BROKER_PROVIDER_STALE_MS;
  ctx.wrapDesktopOnlyLockedControl = wrapDesktopOnlyLockedControl;

  if (
    globalThis.ChimeraSettings.Render &&
    globalThis.ChimeraSettings.Render.Cards &&
    typeof globalThis.ChimeraSettings.Render.Cards.mountAll === "function"
  ) {
    globalThis.ChimeraSettings.Render.Cards.mountAll(ctx);
  }
  if (
    globalThis.ChimeraSettings.Render &&
    globalThis.ChimeraSettings.Render.Cards &&
    typeof globalThis.ChimeraSettings.Render.Cards.mountSummarizedFeedCards === "function"
  ) {
    globalThis.ChimeraSettings.Render.Cards.mountSummarizedFeedCards(ctx);
  }

  if (
    globalThis.ChimeraSettings.Derive &&
    typeof ChimeraSettings.Derive.mountIndexerScopeBridge === "function"
  ) {
    ChimeraSettings.Derive.mountIndexerScopeBridge(ctx);
  }

  if (
    globalThis.ChimeraSettings.Summarized &&
    typeof ChimeraSettings.Summarized.mountModelGlue === "function"
  ) {
    modelGlue = ChimeraSettings.Summarized.mountModelGlue(ctx, {
      strHash: strHash,
      entryInstant: entryInstant,
      primaryLogMessage: primaryLogMessage,
      getFlat: getFlat,
      adminProviderSpecsFromVisible: adminProviderSpecsFromVisible
    });
    buildSummarizedModelForAgg = modelGlue.buildSummarizedModelForAgg;
    renderSummarizedCardFromModel = modelGlue.renderSummarizedCardFromModel;
  }

  var feedBridge = {
    ctx: ctx,
    getViewMode: getViewMode,
    stickPx: stickPx,
    statusEl: statusEl,
    embedded: embedded,
    getFlat: getFlat,
    entryCache: entryCache,
    strHash: strHash,
    normalizeServiceBucketKey: normalizeServiceBucketKey,
    summarizedAdminEditingActive: summarizedAdminEditingActive,
    summarizedPanelInteractionBlocksRebuild: summarizedPanelInteractionBlocksRebuild,
    summarizedPatchSkipCardIds: summarizedPatchSkipCardIds,
    summarizedSkippedCardsHashDelta: summarizedSkippedCardsHashDelta,
    adminVisibleProviderIds: adminVisibleProviderIds,
    lookupAdminProviderSpec: lookupAdminProviderSpec,
    CHIMERA_BROKER_PROVIDER_STALE_MS: CHIMERA_BROKER_PROVIDER_STALE_MS,
    providerCatalogApi: providerCatalogApi,
    adminSetMessage: adminSetMessage,
    fetchAdminTokens: function () {
      return ctx.fetchAdminTokens();
    },
    fetchProviderModels: function (pid) {
      return ctx.fetchProviderModels(pid);
    },
    prefetchProviderModelsAvailability: function () {
      return ctx.prefetchProviderModelsAvailability();
    },
    buildSummarizedFeedSnapshot: buildSummarizedFeedSnapshot,
    buildSummarizedAggregateState: buildSummarizedAggregateState,
    buildSummarizedModelForAgg: buildSummarizedModelForAgg,
    renderSummarizedHtmlFromModel: renderSummarizedHtmlFromModel,
    renderSummarizedCardFromModel: renderSummarizedCardFromModel,
    summarizedHtmlRenderers: function () {
      return typeof ctx.buildSummarizedHtmlRenderers === "function" ? ctx.buildSummarizedHtmlRenderers() : {};
    },
    syncSummarizedModelCache: syncSummarizedModelCache,
    buildGatewayUsageCardHtml: ctx.buildGatewayUsageCardHtml,
    buildGatewayOverviewCardHtml: ctx.buildGatewayOverviewCardHtml,
    buildAdminUsersCardHtml: ctx.buildAdminUsersCardHtml,
    buildAdminProviderCardHtml: ctx.buildAdminProviderCardHtml,
    buildVirtualModelCardHtml: ctx.buildVirtualModelCardHtml,
    buildConvCard: ctx.buildConvCard,
    buildServiceCard: ctx.buildServiceCard,
    buildIndexerCard: ctx.buildIndexerCard,
    collectIndexerRunMeta: ctx.collectIndexerRunMeta,
    mergePersistedIndexerWatchRoots: ctx.mergePersistedIndexerWatchRoots,
    indexerRunTimelineDedupeKey: ctx.indexerRunTimelineDedupeKey,
    pickCanonicalIndexerRun: ctx.pickCanonicalIndexerRun,
    indexerCardDomIdFromMeta: ctx.indexerCardDomIdFromMeta,
    indexerGroupIdForFlat: ctx.indexerGroupIdForFlat,
    entryRoutesToChimeraBrokerBucket: entryRoutesToChimeraBrokerBucket,
    chimeraBrokerProviderHealthStripHtml: ctx.chimeraBrokerProviderHealthStripHtml,
    chimeraBrokerAvailableModelCountLabel: ctx.chimeraBrokerAvailableModelCountLabel,
    scheduleStoryRebuild: scheduleStoryRebuild,
    forceSummarizedFullRebuild: forceSummarizedFullRebuild,
    refreshSummarizedPanel: refreshSummarizedPanel,
    scheduleDeferredSummarizedRefresh: scheduleDeferredSummarizedRefresh
  };

  panelMod = ChimeraSettings.Summarized.mountPanelState(feedBridge);
  patchMod = ChimeraSettings.Summarized.mountCardPatch(feedBridge, panelMod);

  if (globalThis.ChimeraSettings.Api && typeof ChimeraSettings.Api.mountTokensApi === "function") {
    ChimeraSettings.Api.mountTokensApi(ctx, {
      getViewMode: getViewMode,
      scheduleStoryRebuild: scheduleStoryRebuild
    });
  }
  if (globalThis.ChimeraSettings.Api && typeof ChimeraSettings.Api.mountVirtualModelsApi === "function") {
    ChimeraSettings.Api.mountVirtualModelsApi(ctx, {});
  }
  if (globalThis.ChimeraSettings.Api && typeof ChimeraSettings.Api.mountProviderModelsApi === "function") {
    ChimeraSettings.Api.mountProviderModelsApi(ctx, {
      getViewMode: getViewMode,
      adminVisibleProviderIds: adminVisibleProviderIds,
      scheduleStoryRebuild: scheduleStoryRebuild,
      patchAdminProviderCard: function (id) {
        return patchMod.patchAdminProviderCard(id);
      }
    });
  }

  feedBridge.fetchAdminTokens = ctx.fetchAdminTokens;
  feedBridge.fetchProviderModels = ctx.fetchProviderModels;
  feedBridge.prefetchProviderModelsAvailability = ctx.prefetchProviderModelsAvailability;

  pollMod = ChimeraSettings.Summarized.mountPolling(feedBridge, patchMod);

  ctx.refreshSummarizedPanel = refreshSummarizedPanel;
  ctx.forceSummarizedFullRebuild = forceSummarizedFullRebuild;
  ctx.scheduleDeferredSummarizedRefresh = scheduleDeferredSummarizedRefresh;
  ctx.summarizedPanelInteractionBlocksRebuild = summarizedPanelInteractionBlocksRebuild;
  ctx.summarizedEvlogInteractionBlocksRebuild = summarizedPanelInteractionBlocksRebuild;
  ctx.scheduleStoryRebuild = scheduleStoryRebuild;
  ctx.markSummarizedDirtyFromEntry = patchMod.markSummarizedDirtyFromEntry;
  ctx.clearSummarizedDirtySets = patchMod.clearSummarizedDirtySets;
  ctx.updateSummarizedCorrelationFromEntry = patchMod.updateSummarizedCorrelationFromEntry;
  ctx.scheduleSummarizedDirtyFlush = patchMod.scheduleSummarizedDirtyFlush;
  ctx.beginSummarizedLiveSettle = patchMod.beginSummarizedLiveSettle;
  ctx.flushSummarizedDirtyCards = patchMod.flushSummarizedDirtyCards;
  ctx.buildSummarizedAggregateState = buildSummarizedAggregateState;
  ctx.renderSummarizedUnified = renderSummarizedUnified;
  ctx.replaceCardById = patchMod.replaceCardById;
  ctx.patchGatewayUsageMetricsCard = patchMod.patchGatewayUsageMetricsCard;
  ctx.patchGatewayOverviewCard = patchMod.patchGatewayOverviewCard;
  ctx.patchAdminUsersCard = patchMod.patchAdminUsersCard;
  ctx.patchAdminProviderCard = patchMod.patchAdminProviderCard;
  ctx.syncSummarizedModelCache = syncSummarizedModelCache;
  ctx.removeVirtualModelFromSummarizedFeed = patchMod.removeVirtualModelFromSummarizedFeed;
  ctx.refreshAdminCardAfterEditToggle = refreshAdminCardAfterEditToggle;
  ctx.patchAdminCardsFromPoll = patchMod.patchAdminCardsFromPoll;
  ctx.fetchGatewayMetrics = pollMod.fetchGatewayMetrics;
  ctx.fetchGatewayOverview = pollMod.fetchGatewayOverview;
  ctx.fetchChimeraBrokerProviderSnapshot = pollMod.fetchChimeraBrokerProviderSnapshot;
  ctx.fetchAdminState = fetchAdminState;
  ctx.syncMetricsPolling = pollMod.syncMetricsPolling;
  ctx.syncUiStatePolling = pollMod.syncUiStatePolling;
  ctx.syncChimeraBrokerProviderPolling = pollMod.syncChimeraBrokerProviderPolling;
  ctx.patchVirtualModelCard = patchMod.patchVirtualModelCard;
  ctx.adminSetMessage = adminSetMessage;
  ctx.markUiUnauthorized = pollMod.markUiUnauthorized;
  ctx.stopSummarizedPolling = pollMod.stopSummarizedPolling;

  if (typeof ctx.chimeraBrokerShortModelLabel === "function") {
    globalThis.chimeraBrokerShortModelLabel = ctx.chimeraBrokerShortModelLabel;
  }
  if (typeof ctx.ragCollectionLabelForUi === "function") {
    globalThis.ragCollectionLabelForUi = ctx.ragCollectionLabelForUi;
  }
  if (typeof ctx.vectorstoreCollectionScopeLabelForLogs === "function") {
    globalThis.chimeraVectorstoreCollectionScopeLabelForLogs =
      ctx.vectorstoreCollectionScopeLabelForLogs;
  }

  if (
    globalThis.ChimeraSettings &&
    ChimeraSettings.Derive &&
    typeof ChimeraSettings.Derive.mountRagWorkspaceLabel === "function"
  ) {
    ChimeraSettings.Derive.mountRagWorkspaceLabel({
      getOperatorWorkspaces: function () {
        return ctx.lastIndexerOperatorWorkspacesNested || [];
      },
      tenantUserLabel: function (tenantId) {
        var tid = String(tenantId || "").trim();
        if (tid && ctx.tokenLabelByTenant[tid]) return String(ctx.tokenLabelByTenant[tid]).trim();
        if (typeof ctx.resolveLogsOperatorUserLabel === "function") {
          return ctx.resolveLogsOperatorUserLabel();
        }
        return "—";
      },
      operatorWorkspaceTitle: function (ws) {
        if (typeof ctx.operatorManagedWorkspaceTitleText === "function") {
          return ctx.operatorManagedWorkspaceTitleText(ws);
        }
        return "—";
      },
      workspaceTitleFromParts: function (userLabel, projectId, flavorId) {
        var flav =
          flavorId != null && String(flavorId).trim() !== "" ? String(flavorId).trim() : "—";
        if (typeof ctx.workspaceCardTitleFromIndexerMeta !== "function") return "—";
        return ctx.workspaceCardTitleFromIndexerMeta({
          userLabel: userLabel,
          projectId: projectId,
          flavorId: flav
        });
      },
      normalizeFlavor: function (v) {
        if (typeof ctx.normalizeFlavorMatch === "function") return ctx.normalizeFlavorMatch(v);
        return v != null ? String(v).trim() : "";
      }
    });
  }

  if (
    globalThis.ChimeraSettings.Handlers &&
    globalThis.ChimeraSettings.Handlers.WorkspaceManaged &&
    typeof ChimeraSettings.Handlers.WorkspaceManaged.mount === "function"
  ) {
    ChimeraSettings.Handlers.WorkspaceManaged.mount(ctx);
  }

  if (
    globalThis.ChimeraSettings.Handlers &&
    globalThis.ChimeraSettings.Handlers.ProviderPicker &&
    typeof ChimeraSettings.Handlers.ProviderPicker.mount === "function"
  ) {
    ChimeraSettings.Handlers.ProviderPicker.mount(ctx, {
      getViewMode: getViewMode,
      scheduleStoryRebuild: scheduleStoryRebuild
    });
  }
};
