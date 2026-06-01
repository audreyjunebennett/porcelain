/**
 * Summarized per-card DOM patch, dirty flush, and poll-driven admin card updates.
 * Exports: ChimeraSettings.Summarized.mountCardPatch(bridge, panel)
 */
globalThis.ChimeraSettings = globalThis.ChimeraSettings || {};
globalThis.ChimeraSettings.Summarized = globalThis.ChimeraSettings.Summarized || {};

globalThis.ChimeraSettings.Summarized.mountCardPatch = function (bridge, panel) {
  var ctx = bridge.ctx;
  var getViewMode = bridge.getViewMode;
  var getFlat = bridge.getFlat;
  var strHash = bridge.strHash;
  var normalizeServiceBucketKey = bridge.normalizeServiceBucketKey;
  var summarizedAdminEditingActive = bridge.summarizedAdminEditingActive;
  var summarizedPanelInteractionBlocksRebuild = bridge.summarizedPanelInteractionBlocksRebuild;
  var summarizedPatchSkipCardIds = bridge.summarizedPatchSkipCardIds;
  var summarizedSkippedCardsHashDelta = bridge.summarizedSkippedCardsHashDelta;
  var adminVisibleProviderIds = bridge.adminVisibleProviderIds;
  var lookupAdminProviderSpec = bridge.lookupAdminProviderSpec;
  var SUMMARIZED_CARD_SCROLL_SEL = panel.SUMMARIZED_CARD_SCROLL_SEL;
  var ADMIN_CARD_TABLE_SCROLL_SEL = panel.ADMIN_CARD_TABLE_SCROLL_SEL;

  var SUMMARIZED_DIRTY_FLUSH_MS = 750;
  var SUMMARIZED_DIRTY_FULL_REBUILD_DEBOUNCE_MS = 800;
  var SUMMARIZED_LIVE_SETTLE_MS = 2000;
  var SUMMARIZED_DIRTY_FULL_REBUILD_MIN = 10;
  var SUMMARIZED_DIRTY_FULL_REBUILD_RATIO = 0.3;

  ctx.summarizedDirtyCardIds = ctx.summarizedDirtyCardIds || Object.create(null);
  ctx.summarizedDirtyIndexerBucketIds = ctx.summarizedDirtyIndexerBucketIds || Object.create(null);
  ctx.summarizedReqToConv = ctx.summarizedReqToConv || Object.create(null);
  ctx.summarizedIndexRunToConv = ctx.summarizedIndexRunToConv || Object.create(null);

  function summarizedPatchAvailable() {
    return !!(
      globalThis.ChimeraSettings &&
      ChimeraSettings.Summarized &&
      ChimeraSettings.Summarized.Patch &&
      typeof ChimeraSettings.Summarized.Patch.diffSummarizedModels === "function" &&
      typeof ChimeraSettings.Summarized.Patch.applySummarizedPatches === "function"
    );
  }

  function replaceCardById(cardId, buildHtml, opts) {
    opts = opts || {};
    if (getViewMode() !== "summarized") return false;
    if (!document.getElementById("panel-summarized")) return false;
    var oldEl = document.getElementById(cardId);
    if (!oldEl) return false;
    var preserveOpen = opts.preserveOpen !== false;
    var keepOpen = preserveOpen && panel.isSummarizedCardOpen(oldEl);
    var scrollSel = opts.preserveScrollSelectors;
    var scrollMap = scrollSel ? panel.captureNestedScrollMap(oldEl, scrollSel) : null;
    var cardUiSave = null;
    try {
      if (oldEl.querySelector && oldEl.querySelector("[data-sum-evlog-root]")) {
        cardUiSave = panel.captureSummarizedPanelUiState(oldEl);
      }
    } catch (_eCardUi) {}
    panel.destroySummarizedYamlEditors(oldEl);
    var wrap = document.createElement("div");
    wrap.innerHTML = (typeof buildHtml === "function" ? buildHtml() : String(buildHtml || "")).trim();
    var newEl = wrap.firstElementChild;
    if (!newEl || newEl.id !== cardId) return false;
    oldEl.parentNode.replaceChild(newEl, oldEl);
    if (preserveOpen) panel.setSummarizedCardOpen(newEl, keepOpen);
    if (opts.cardVersionAttr !== false && opts.cardHash && newEl.setAttribute) {
      newEl.setAttribute("data-card-hash", String(opts.cardHash));
    }
    if (scrollSel && scrollMap) {
      panel.restoreNestedScrollMap(newEl, scrollSel, scrollMap);
    }
    if (cardUiSave) {
      if (typeof globalThis.sumEvlogHydrateAllIn === "function") {
        try {
          globalThis.sumEvlogHydrateAllIn(newEl);
        } catch (_eEvCard) {}
      }
      panel.restoreSummarizedPanelUiState(newEl, cardUiSave, { scroll: false });
      window.requestAnimationFrame(function () {
        panel.restoreSummarizedPanelUiState(newEl, cardUiSave, { scrollOnly: true });
      });
    }
    if (newEl.classList && newEl.classList.contains("sum-card--collapsible")) {
      panel.wireCollapsibleSummarizedPanel(newEl);
    }
    panel.mountSummarizedYamlEditors(newEl);
    return true;
  }

  function replaceCardByIdForPatch(cardId, html, opts) {
    return replaceCardById(
      cardId,
      function () {
        return html;
      },
      opts
    );
  }

  function applySummarizedPanelPatch(psu, ops) {
    if (!psu || !ops || !ops.length) return { ok: true, applied: 0 };
    var uiSave = panel.captureSummarizedPanelUiState(psu);
    var result = ChimeraSettings.Summarized.Patch.applySummarizedPatches(
      psu,
      ops,
      bridge.summarizedHtmlRenderers(),
      {
        replaceCard: replaceCardByIdForPatch,
        preserveScrollSelectors: SUMMARIZED_CARD_SCROLL_SEL
      }
    );
    if (result.applied > 0) {
      if (typeof globalThis.sumEvlogHydrateAllIn === "function") {
        try {
          globalThis.sumEvlogHydrateAllIn(psu);
        } catch (_eEvPatch) {}
      }
      panel.restoreSummarizedPanelUiState(psu, uiSave, { scroll: false });
      panel.wireCollapsibleSummarizedPanel(psu);
      panel.mountSummarizedYamlEditors(psu);
      window.requestAnimationFrame(function () {
        panel.restoreSummarizedPanelUiState(psu, uiSave, { scrollOnly: true });
      });
    }
    return result;
  }

  function patchGatewayUsageMetricsCard() {
    if (
      !replaceCardById("gw-usage-metrics", bridge.buildGatewayUsageCardHtml, {
        preserveOpen: true,
        preserveScrollSelectors: ".sum-metrics-table-wrap"
      })
    ) {
      bridge.refreshSummarizedPanel();
    }
  }

  function patchGatewayOverviewCard() {
    if (!replaceCardById("gw-overview", bridge.buildGatewayOverviewCardHtml, { preserveOpen: true })) {
      bridge.refreshSummarizedPanel();
    }
  }

  function cancelCoalescedFullRebuild() {
    if (ctx.coalescedFullRebuildTimer) {
      clearTimeout(ctx.coalescedFullRebuildTimer);
      ctx.coalescedFullRebuildTimer = null;
    }
  }

  function scheduleCoalescedFullRebuild(reason) {
    if (summarizedAdminEditingActive()) {
      bridge.scheduleDeferredSummarizedRefresh();
      return;
    }
    if (ctx.coalescedFullRebuildTimer) return;
    ctx.coalescedFullRebuildTimer = setTimeout(function () {
      ctx.coalescedFullRebuildTimer = null;
      clearSummarizedDirtySets();
      ctx.suppressSummarizedDirty = true;
      try {
        bridge.forceSummarizedFullRebuild(reason || "dirty-storm-coalesced");
      } finally {
        ctx.suppressSummarizedDirty = false;
      }
    }, SUMMARIZED_DIRTY_FULL_REBUILD_DEBOUNCE_MS);
  }

  function beginSummarizedLiveSettle() {
    ctx.suppressSummarizedDirty = true;
    if (ctx.summarizedLiveSettleTimer) clearTimeout(ctx.summarizedLiveSettleTimer);
    cancelCoalescedFullRebuild();
    bridge.scheduleStoryRebuild();
    ctx.summarizedLiveSettleTimer = setTimeout(function () {
      ctx.summarizedLiveSettleTimer = null;
      ctx.suppressSummarizedDirty = false;
      scheduleSummarizedDirtyFlush();
    }, SUMMARIZED_LIVE_SETTLE_MS);
  }

  function summarizedDirtyRoutingDeps() {
    return {
      getFlat: getFlat,
      strHash: strHash,
      normalizeServiceBucketKey: normalizeServiceBucketKey,
      indexerGroupIdForFlat: bridge.indexerGroupIdForFlat,
      getAdminProviderIds: adminVisibleProviderIds
    };
  }

  function updateSummarizedCorrelationFromEntry(ent) {
    if (!ent || !ent.parsed) return;
    var f = getFlat(ent.parsed);
    if (typeof ctx.tryRegisterRequestConversationCorrelationPrimary === "function") {
      ctx.tryRegisterRequestConversationCorrelationPrimary(ctx.summarizedReqToConv, f);
    }
    if (typeof ctx.tryRegisterRequestConversationCorrelationRagFallback === "function") {
      ctx.tryRegisterRequestConversationCorrelationRagFallback(ctx.summarizedReqToConv, f);
    }
    var msgIr = String(f.msg != null ? f.msg : f.message != null ? f.message : "").trim();
    if (msgIr !== "ingest.complete" && msgIr !== "ingest.failed" && msgIr !== "ingest.chunked.error") return;
    var irKey = f.index_run_id != null ? String(f.index_run_id).trim() : "";
    var cidIr = f.conversation_id != null ? String(f.conversation_id).trim() : "";
    var pidIr =
      f.principal_id != null ? String(f.principal_id).trim() : f.tenant != null ? String(f.tenant).trim() : "";
    if (irKey && cidIr && pidIr && !ctx.summarizedIndexRunToConv[irKey]) {
      ctx.summarizedIndexRunToConv[irKey] = { pid: pidIr, cid: cidIr };
    }
  }

  function markSummarizedDirtyFromEntry(ent) {
    if (
      !ent ||
      !globalThis.ChimeraSettings ||
      !ChimeraSettings.Summarized ||
      typeof ChimeraSettings.Summarized.dirtyTargetsForEntry !== "function"
    ) {
      return;
    }
    var targets = ChimeraSettings.Summarized.dirtyTargetsForEntry(
      ent,
      { reqToConv: ctx.summarizedReqToConv, indexRunToConv: ctx.summarizedIndexRunToConv },
      summarizedDirtyRoutingDeps()
    );
    var ci;
    for (ci = 0; ci < targets.cardIds.length; ci++) {
      ctx.summarizedDirtyCardIds[targets.cardIds[ci]] = true;
    }
    for (ci = 0; ci < targets.indexerBucketIds.length; ci++) {
      ctx.summarizedDirtyIndexerBucketIds[targets.indexerBucketIds[ci]] = true;
    }
  }

  function summarizedDirtyCardCount() {
    var n = 0;
    var k;
    for (k in ctx.summarizedDirtyCardIds) {
      if (Object.prototype.hasOwnProperty.call(ctx.summarizedDirtyCardIds, k)) n++;
    }
    for (k in ctx.summarizedDirtyIndexerBucketIds) {
      if (Object.prototype.hasOwnProperty.call(ctx.summarizedDirtyIndexerBucketIds, k)) n++;
    }
    return n;
  }

  function clearSummarizedDirtySets() {
    ctx.summarizedDirtyCardIds = Object.create(null);
    ctx.summarizedDirtyIndexerBucketIds = Object.create(null);
  }

  function shouldSummarizedDirtyFullRebuild(dirtyCount) {
    var panelEl = document.getElementById("panel-summarized");
    if (!panelEl) return true;
    var total = panelEl.querySelectorAll("details.sum-card").length;
    if (!total) return true;
    if (dirtyCount >= SUMMARIZED_DIRTY_FULL_REBUILD_MIN) return true;
    if (dirtyCount / total >= SUMMARIZED_DIRTY_FULL_REBUILD_RATIO) return true;
    return false;
  }

  function conversationDomIdForGroup(g) {
    var cardKey =
      Array.isArray(g.cids) && g.cids.length > 1
        ? g.pid + "\0" + g.cids.slice().sort().join("\0")
        : g.pid + "\0" + g.cid;
    return strHash(cardKey);
  }

  function resolveIndexerDomIdsFromDirtyBuckets(bucketIds, agg) {
    var out = [];
    var seen = Object.create(null);
    if (!bucketIds.length || !agg || !agg.byRun) return out;
    var dedupeGroups = {};
    var rks = Object.keys(agg.byRun);
    var rj;
    for (rj = 0; rj < rks.length; rj++) {
      var runG = agg.byRun[rks[rj]];
      if (!runG) continue;
      var hit = false;
      var bi;
      for (bi = 0; bi < bucketIds.length; bi++) {
        if (bucketIds[bi] === runG.id) {
          hit = true;
          break;
        }
      }
      if (!hit) continue;
      var pmetaG = null;
      if (
        agg.partitionRegistry &&
        globalThis.ChimeraSettings &&
        ChimeraSettings.Derive &&
        typeof ChimeraSettings.Derive.indexerPartitionMetaForRun === "function"
      ) {
        pmetaG = ChimeraSettings.Derive.indexerPartitionMetaForRun(
          agg.partitionRegistry,
          runG.id,
          runG.events,
          getFlat
        );
      }
      var metaG = bridge.collectIndexerRunMeta(runG.id, runG.events, pmetaG);
      metaG = bridge.mergePersistedIndexerWatchRoots(metaG, runG.events, runG.id);
      var dk = bridge.indexerRunTimelineDedupeKey(metaG, runG.id);
      if (!dedupeGroups[dk]) dedupeGroups[dk] = [];
      dedupeGroups[dk].push(runG);
    }
    var dkIter;
    for (dkIter in dedupeGroups) {
      if (!Object.prototype.hasOwnProperty.call(dedupeGroups, dkIter)) continue;
      var grpRuns = dedupeGroups[dkIter];
      var run = bridge.pickCanonicalIndexerRun(grpRuns);
      if (!run) continue;
      var pmetaLive = null;
      if (
        agg.partitionRegistry &&
        globalThis.ChimeraSettings &&
        ChimeraSettings.Derive &&
        typeof ChimeraSettings.Derive.indexerPartitionMetaForRun === "function"
      ) {
        pmetaLive = ChimeraSettings.Derive.indexerPartitionMetaForRun(
          agg.partitionRegistry,
          run.id,
          run.events,
          getFlat
        );
      }
      var metaLive = bridge.collectIndexerRunMeta(run.id, run.events, pmetaLive);
      metaLive = bridge.mergePersistedIndexerWatchRoots(metaLive, run.events, run.id);
      var domId = bridge.indexerCardDomIdFromMeta(metaLive, run.id);
      if (!seen[domId]) {
        seen[domId] = true;
        out.push(domId);
      }
    }
    return out;
  }

  function buildHtmlForSummarizedCardId(cardId, agg) {
    if (!cardId) return null;
    var model = ctx.lastSummarizedModel;
    if (!model || !model.cards) {
      model = bridge.buildSummarizedModelForAgg(agg || bridge.buildSummarizedAggregateState());
    }
    if (
      model &&
      globalThis.ChimeraSettings.Summarized.Render &&
      typeof ChimeraSettings.Summarized.Render.findCardById === "function"
    ) {
      var card = ChimeraSettings.Summarized.Render.findCardById(model, cardId);
      if (card) return bridge.renderSummarizedCardFromModel(card);
    }
    if (!agg) return null;
    if (cardId.indexOf("admin-provider-") === 0) {
      var providerId = cardId.slice("admin-provider-".length);
      var specPatch = lookupAdminProviderSpec(providerId);
      if (!specPatch) return null;
      return bridge.buildAdminProviderCardHtml(
        specPatch.id,
        specPatch.title,
        specPatch.avatar,
        specPatch.subtitle
      );
    }
    var svcOrder =
      globalThis.ChimeraSettings &&
      ChimeraSettings.Summarized &&
      ChimeraSettings.Summarized.SERVICE_BUCKET_ORDER
        ? ChimeraSettings.Summarized.SERVICE_BUCKET_ORDER
        : ["chimera-broker", "chimera-gateway", "chimera-indexer", "chimera-vectorstore"];
    var si;
    for (si = 0; si < svcOrder.length; si++) {
      var nm = svcOrder[si];
      if (cardId !== "svc-" + strHash(nm)) continue;
      var arr = agg.buckets[nm];
      if (!arr || !arr.length) return null;
      return bridge.buildServiceCard(nm, arr, { byRun: agg.byRun, partitionRegistry: agg.partitionRegistry });
    }
    var ci;
    for (ci = 0; ci < agg.mergedConv.length; ci++) {
      var g = agg.mergedConv[ci];
      if (conversationDomIdForGroup(g) === cardId) return bridge.buildConvCard(g);
    }
    var rks = Object.keys(agg.byRun || {});
    var rj;
    for (rj = 0; rj < rks.length; rj++) {
      var runG = agg.byRun[rks[rj]];
      if (!runG) continue;
      var pmetaG = null;
      if (
        agg.partitionRegistry &&
        globalThis.ChimeraSettings &&
        ChimeraSettings.Derive &&
        typeof ChimeraSettings.Derive.indexerPartitionMetaForRun === "function"
      ) {
        pmetaG = ChimeraSettings.Derive.indexerPartitionMetaForRun(
          agg.partitionRegistry,
          runG.id,
          runG.events,
          getFlat
        );
      }
      var metaG = bridge.collectIndexerRunMeta(runG.id, runG.events, pmetaG);
      metaG = bridge.mergePersistedIndexerWatchRoots(metaG, runG.events, runG.id);
      if (bridge.indexerCardDomIdFromMeta(metaG, runG.id) === cardId) {
        return bridge.buildIndexerCard(runG, agg.partitionRegistry);
      }
    }
    return null;
  }

  function patchSummarizedCard(cardId, agg, nextModel) {
    var prevModel = ctx.lastSummarizedModel;
    if (!nextModel) nextModel = bridge.buildSummarizedModelForAgg(agg || bridge.buildSummarizedAggregateState());
    if (prevModel && summarizedPatchAvailable()) {
      var onlyCardIds = Object.create(null);
      onlyCardIds[cardId] = true;
      var ops = ChimeraSettings.Summarized.Patch.diffSummarizedModels(prevModel, nextModel, {
        onlyCardIds: onlyCardIds,
        skipCardIds: summarizedPatchSkipCardIds()
      });
      if (
        !ChimeraSettings.Summarized.Patch.shouldUseFullRebuildFromOps(ops) &&
        ChimeraSettings.Summarized.Patch.countReplaceCardOps(ops) > 0
      ) {
        var psu = document.getElementById("panel-summarized");
        var patchResult = applySummarizedPanelPatch(psu, ops);
        if (patchResult.ok) {
          ctx.lastSummarizedModel = nextModel;
          if (agg) ctx.lastSummarizedAggregate = agg;
          return true;
        }
      }
    }
    var html = buildHtmlForSummarizedCardId(cardId, agg);
    if (!html) return false;
    return replaceCardById(
      cardId,
      function () {
        return html;
      },
      {
        preserveOpen: true,
        preserveScrollSelectors: SUMMARIZED_CARD_SCROLL_SEL
      }
    );
  }

  function flushSummarizedDirtyCards() {
    if (getViewMode() !== "summarized") {
      clearSummarizedDirtySets();
      return;
    }
    if (summarizedPanelInteractionBlocksRebuild()) {
      bridge.scheduleDeferredSummarizedRefresh();
      return;
    }
    if (summarizedAdminEditingActive()) {
      return;
    }
    var dirtyCount = summarizedDirtyCardCount();
    if (!dirtyCount) return;
    var agg = bridge.buildSummarizedAggregateState();
    var nextModel = bridge.buildSummarizedModelForAgg(agg);
    var cardIds = [];
    var k;
    for (k in ctx.summarizedDirtyCardIds) {
      if (Object.prototype.hasOwnProperty.call(ctx.summarizedDirtyCardIds, k)) cardIds.push(k);
    }
    var ixBuckets = [];
    for (k in ctx.summarizedDirtyIndexerBucketIds) {
      if (Object.prototype.hasOwnProperty.call(ctx.summarizedDirtyIndexerBucketIds, k)) ixBuckets.push(k);
    }
    var ixDom = resolveIndexerDomIdsFromDirtyBuckets(ixBuckets, agg);
    for (var xi = 0; xi < ixDom.length; xi++) {
      if (cardIds.indexOf(ixDom[xi]) < 0) cardIds.push(ixDom[xi]);
    }
    if (shouldSummarizedDirtyFullRebuild(dirtyCount)) {
      scheduleCoalescedFullRebuild("dirty-storm");
      return;
    }
    clearSummarizedDirtySets();

    var prevModel = ctx.lastSummarizedModel;
    if (prevModel && cardIds.length && summarizedPatchAvailable()) {
      var onlyCardIds = Object.create(null);
      for (var ci = 0; ci < cardIds.length; ci++) onlyCardIds[cardIds[ci]] = true;
      var dirtyOps = ChimeraSettings.Summarized.Patch.diffSummarizedModels(prevModel, nextModel, {
        onlyCardIds: onlyCardIds,
        skipCardIds: summarizedPatchSkipCardIds()
      });
      if (
        !ChimeraSettings.Summarized.Patch.shouldUseFullRebuildFromOps(dirtyOps) &&
        ChimeraSettings.Summarized.Patch.countReplaceCardOps(dirtyOps) > 0
      ) {
        var psuDirty = document.getElementById("panel-summarized");
        var dirtyPatch = applySummarizedPanelPatch(psuDirty, dirtyOps);
        if (dirtyPatch.ok) {
          ctx.lastSummarizedModel = nextModel;
          ctx.lastSummarizedAggregate = agg;
          return;
        }
      } else if (!ChimeraSettings.Summarized.Patch.shouldUseFullRebuildFromOps(dirtyOps)) {
        ctx.lastSummarizedModel = nextModel;
        ctx.lastSummarizedAggregate = agg;
        return;
      }
    }

    var needRebuild = false;
    var pi;
    for (pi = 0; pi < cardIds.length; pi++) {
      if (!patchSummarizedCard(cardIds[pi], agg, nextModel)) needRebuild = true;
    }
    if (needRebuild) {
      bridge.scheduleStoryRebuild();
    } else {
      ctx.lastSummarizedModel = nextModel;
      ctx.lastSummarizedAggregate = agg;
    }
  }

  function scheduleSummarizedDirtyFlush() {
    if (ctx.suppressSummarizedDirty || ctx.storyRebuildTimer || ctx.coalescedFullRebuildTimer) return;
    if (ctx.summarizedDirtyFlushTimer) return;
    ctx.summarizedDirtyFlushTimer = setTimeout(function () {
      ctx.summarizedDirtyFlushTimer = null;
      if (ctx.suppressSummarizedDirty || ctx.storyRebuildTimer || ctx.coalescedFullRebuildTimer) return;
      flushSummarizedDirtyCards();
    }, SUMMARIZED_DIRTY_FLUSH_MS);
  }

  function patchAdminUsersCard() {
    return replaceCardById("admin-users", bridge.buildAdminUsersCardHtml, { preserveOpen: false });
  }

  function patchAdminProviderCard(providerId) {
    var spec = lookupAdminProviderSpec(providerId);
    if (!spec) return false;
    return replaceCardById(
      "admin-provider-" + providerId,
      function () {
        return bridge.buildAdminProviderCardHtml(spec.id, spec.title, spec.avatar, spec.subtitle);
      },
      { preserveOpen: true, preserveScrollSelectors: ADMIN_CARD_TABLE_SCROLL_SEL }
    );
  }

  function syncVmSectionOpenFromDom(cardEl, ui) {
    if (!cardEl || !ui) return;
    if (!ui.sectionOpen) ui.sectionOpen = { identity: true, fallback: true };
    var list = cardEl.querySelectorAll("details.sum-vm-section[data-vm-section]");
    for (var i = 0; i < list.length; i++) {
      var key = list[i].getAttribute("data-vm-section");
      if (key) ui.sectionOpen[key] = !!list[i].open;
    }
  }

  function lookupVmSummary(vmId) {
    var gw = ctx.adminStateCache && ctx.adminStateCache.gateway;
    var vms = gw && gw.virtual_models && Array.isArray(gw.virtual_models) ? gw.virtual_models : [];
    for (var i = 0; i < vms.length; i++) {
      if (vms[i] && Number(vms[i].id) === Number(vmId)) return vms[i];
    }
    return null;
  }

  function virtualModelCardEl(vmId) {
    return document.getElementById("virtual-model-" + String(vmId));
  }

  function virtualModelPanelIsOpen(vmId) {
    var el = virtualModelCardEl(vmId);
    return !!(el && el.open);
  }

  function patchVirtualModelCard(vmId, opts) {
    opts = opts || {};
    if (opts.onlyIfOpen && !virtualModelPanelIsOpen(vmId)) return false;
    var summary = lookupVmSummary(vmId);
    if (!summary || typeof bridge.buildVirtualModelCardHtml !== "function") return false;
    var cardId = "virtual-model-" + String(vmId);
    var oldEl = document.getElementById(cardId);
    var uiKey = String(vmId);
    if (oldEl && ctx.virtualModelUi && ctx.virtualModelUi[uiKey]) {
      syncVmSectionOpenFromDom(oldEl, ctx.virtualModelUi[uiKey]);
    }
    var keepOpen = oldEl ? !!oldEl.open : false;
    var ok = replaceCardById(
      cardId,
      function () {
        return bridge.buildVirtualModelCardHtml(summary);
      },
      { preserveOpen: keepOpen, preserveScrollSelectors: ADMIN_CARD_TABLE_SCROLL_SEL }
    );
    if (ok && ctx.virtualModelUi && ctx.virtualModelUi[String(vmId)]) {
      ctx.virtualModelUi[String(vmId)].hydrated = true;
    }
    return ok;
  }

  function patchAdminCardsFromPoll() {
    if (getViewMode() !== "summarized") return;
    if (summarizedPanelInteractionBlocksRebuild()) return;
    var psu = document.getElementById("panel-summarized");
    if (!psu) return;

    var prevModel = ctx.lastSummarizedModel;
    var agg = bridge.buildSummarizedAggregateState();
    var nextModel = bridge.buildSummarizedModelForAgg(agg);
    if (prevModel && summarizedPatchAvailable()) {
      var onlyCardIds = Object.create(null);
      onlyCardIds["admin-users"] = true;
      var visiblePoll = adminVisibleProviderIds();
      for (var ai = 0; ai < visiblePoll.length; ai++) {
        onlyCardIds["admin-provider-" + visiblePoll[ai]] = true;
      }
      var pollOps = ChimeraSettings.Summarized.Patch.diffSummarizedModels(prevModel, nextModel, {
        onlyCardIds: onlyCardIds,
        skipCardIds: summarizedPatchSkipCardIds()
      });
      if (
        !ChimeraSettings.Summarized.Patch.shouldUseFullRebuildFromOps(pollOps) &&
        ChimeraSettings.Summarized.Patch.countReplaceCardOps(pollOps) > 0
      ) {
        var pollPatch = applySummarizedPanelPatch(psu, pollOps);
        if (pollPatch.ok) {
          ctx.lastSummarizedModel = nextModel;
          ctx.lastSummarizedAggregate = agg;
          return;
        }
      } else if (!ChimeraSettings.Summarized.Patch.shouldUseFullRebuildFromOps(pollOps)) {
        ctx.lastSummarizedModel = nextModel;
        ctx.lastSummarizedAggregate = agg;
        return;
      }
    }

    var needRebuild = false;
    if (!patchAdminUsersCard()) needRebuild = true;
    var visibleAdmin = adminVisibleProviderIds();
    for (var aj = 0; aj < visibleAdmin.length; aj++) {
      if (!patchAdminProviderCard(visibleAdmin[aj])) needRebuild = true;
    }
    if (needRebuild) bridge.scheduleStoryRebuild();
    else {
      ctx.lastSummarizedModel = nextModel;
      ctx.lastSummarizedAggregate = agg;
    }
  }

  function removeVirtualModelFromSummarizedFeed(vmId) {
    var key = String(vmId);
    if (ctx.virtualModelDetails) delete ctx.virtualModelDetails[key];
    if (ctx.virtualModelUi) delete ctx.virtualModelUi[key];
    var gw = ctx.adminStateCache && ctx.adminStateCache.gateway;
    if (gw && gw.virtual_models && Array.isArray(gw.virtual_models)) {
      var kept = [];
      for (var i = 0; i < gw.virtual_models.length; i++) {
        if (gw.virtual_models[i] && Number(gw.virtual_models[i].id) !== Number(vmId)) {
          kept.push(gw.virtual_models[i]);
        }
      }
      gw.virtual_models = kept;
    }
    var cardEl = document.getElementById("virtual-model-" + key);
    if (cardEl && cardEl.parentNode) cardEl.parentNode.removeChild(cardEl);
    bridge.syncSummarizedModelCache();
  }

  ctx.conversationDomIdForGroup = conversationDomIdForGroup;

  return {
    summarizedPatchAvailable: summarizedPatchAvailable,
    replaceCardById: replaceCardById,
    applySummarizedPanelPatch: applySummarizedPanelPatch,
    patchGatewayUsageMetricsCard: patchGatewayUsageMetricsCard,
    patchGatewayOverviewCard: patchGatewayOverviewCard,
    beginSummarizedLiveSettle: beginSummarizedLiveSettle,
    markSummarizedDirtyFromEntry: markSummarizedDirtyFromEntry,
    updateSummarizedCorrelationFromEntry: updateSummarizedCorrelationFromEntry,
    clearSummarizedDirtySets: clearSummarizedDirtySets,
    shouldSummarizedDirtyFullRebuild: shouldSummarizedDirtyFullRebuild,
    flushSummarizedDirtyCards: flushSummarizedDirtyCards,
    scheduleSummarizedDirtyFlush: scheduleSummarizedDirtyFlush,
    patchAdminUsersCard: patchAdminUsersCard,
    patchAdminProviderCard: patchAdminProviderCard,
    patchAdminCardsFromPoll: patchAdminCardsFromPoll,
    patchVirtualModelCard: patchVirtualModelCard,
    removeVirtualModelFromSummarizedFeed: removeVirtualModelFromSummarizedFeed
  };
};
