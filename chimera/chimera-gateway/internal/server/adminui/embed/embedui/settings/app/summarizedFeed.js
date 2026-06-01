/**
 * Summarized panel rebuild, service/conversation cards, and unified feed render.
 *
 * Exports: ChimeraSettings.App.mountSummarizedFeed(ctx)
 */

globalThis.ChimeraSettings = globalThis.ChimeraSettings || {};
globalThis.ChimeraSettings.App = globalThis.ChimeraSettings.App || {};
globalThis.ChimeraSettings.App.mountSummarizedFeed = function (ctx) {
  var statusEl = ctx.statusEl;
  var formatLogDateTimeLocal = ctx.formatLogDateTimeLocal;
  var formatLogRelativeAgo = ctx.formatLogRelativeAgo;
  var toIsoDatetimeAttr = ctx.toIsoDatetimeAttr;
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
  var RECENT_CARD_STATUS_N = ctx.RECENT_CARD_STATUS_N;
  var sumEvlogPanelHtml = ctx.sumEvlogPanelHtml;
  var sumEvlogBuildTbodyFromConvEvents = ctx.sumEvlogBuildTbodyFromConvEvents;
  var sumEvlogBuildTbodyFromServiceEntries = ctx.sumEvlogBuildTbodyFromServiceEntries;
  var sumEvlogVisibleEntriesForService = ctx.sumEvlogVisibleEntriesForService;
  var sumEvlogCountWarnFailFromEntries = ctx.sumEvlogCountWarnFailFromEntries;
  var scopedEvlogTitle = ctx.scopedEvlogTitle;
  var contextGrowthStripHtml = ctx.contextGrowthStripHtml;
  var SHOW_CONV_EXPANDED_CONTEXT_STRIP = !!ctx.SHOW_CONV_EXPANDED_CONTEXT_STRIP;
  var metricsPollTimer = null;
  var METRICS_POLL_MS = 12000;
  var uiStatePollTimer = null;
  var UI_STATE_POLL_MS = 60000;
  function providerCatalogApi() {
    return globalThis.ChimeraSettings &&
      ChimeraSettings.Providers &&
      ChimeraSettings.Providers.Catalog
      ? ChimeraSettings.Providers.Catalog
      : null;
  }

  /** Ordered provider ids for summarized admin-provider-* cards (see settings/providers/catalog.js). */
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

  function ensureAdminProviderCatalog() {
    var api = providerCatalogApi();
    if (!api) return Promise.resolve(null);
    return api.fetchProviderCatalog(ctx).then(function (data) {
      if (data && getViewMode() === "summarized") scheduleStoryRebuild();
      return data;
    }).catch(function () {
      return null;
    });
  }

  function closeAdminProviderPicker() {
    var picker = document.getElementById("sg-op-provider-picker");
    var trigger = document.getElementById("sg-op-provider-picker-trigger");
    if (picker) picker.hidden = true;
    if (trigger) trigger.setAttribute("aria-expanded", "false");
  }

  function setAdminProviderPickerOpen(open) {
    var picker = document.getElementById("sg-op-provider-picker");
    var trigger = document.getElementById("sg-op-provider-picker-trigger");
    if (!picker || !trigger || trigger.disabled || trigger.getAttribute("aria-disabled") === "true") return;
    picker.hidden = !open;
    trigger.setAttribute("aria-expanded", open ? "true" : "false");
  }
  var ADMIN_CARD_TABLE_SCROLL_SEL =
    ".sum-metrics-table-wrap, .sg-op-routing-table-scroll, .sg-op-fallback-table-scroll, .sg-op-router-table-scroll";
  /** Per-card patch + full rebuild: admin tables and evlog table wrappers (tbody scroll is in evlog state). */
  var SUMMARIZED_CARD_SCROLL_SEL =
    ADMIN_CARD_TABLE_SCROLL_SEL + ", .sum-full-log--evlog .sum-evlog-table-wrap";
  var chimeraBrokerProviderPollTimer = null;
  var CHIMERA_BROKER_PROVIDER_POLL_MS = 30000;
  var CHIMERA_BROKER_PROVIDER_STALE_MS = 90000;
  /** Min gap between live-log dirty flushes (SSE can otherwise rebuild DOM every frame). */
  var SUMMARIZED_DIRTY_FLUSH_MS = 750;
  /** Debounce full-panel rebuild when many cards are dirty at once. */
  var SUMMARIZED_DIRTY_FULL_REBUILD_DEBOUNCE_MS = 800;
  /** After initial tail ingest, hold per-line patches until the first full rebuild finishes. */
  var SUMMARIZED_LIVE_SETTLE_MS = 2000;
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

  function stopSummarizedPolling() {
    if (metricsPollTimer) {
      try {
        clearInterval(metricsPollTimer);
      } catch (_eM) {}
      metricsPollTimer = null;
    }
    if (uiStatePollTimer) {
      try {
        clearInterval(uiStatePollTimer);
      } catch (_eU) {}
      uiStatePollTimer = null;
    }
    if (chimeraBrokerProviderPollTimer) {
      try {
        clearInterval(chimeraBrokerProviderPollTimer);
      } catch (_eB) {}
      chimeraBrokerProviderPollTimer = null;
    }
  }

  function markUiUnauthorized(msg) {
    if (ctx.uiUnauthorized) return;
    ctx.uiUnauthorized = true;
    stopSummarizedPolling();
    if (typeof ctx.stopLogsTransport === "function") ctx.stopLogsTransport();
    var text = msg || (embedded ? "Unauthorized — sign in from the shell" : "Unauthorized — sign in");
    if (statusEl) {
      statusEl.textContent = text;
      statusEl.className = "status-line err";
    }
    if (!embedded) {
      try {
        var next = window.location.pathname + window.location.search;
        window.location.replace("/ui/login?next=" + encodeURIComponent(next));
      } catch (_eLogin) {}
    }
  }

  function syncSummarizedModelCache() {
    var snap = buildSummarizedFeedSnapshot();
    ctx.lastSummarizedModel = snap.model;
    ctx.lastSummarizedAggregate = snap.agg;
  }

  /** Enter/exit admin card edit mode: patch one card (bypasses skipCardIds) or full rebuild. */
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

  function summarizedPatchAvailable() {
    return !!(
      globalThis.ChimeraSettings &&
      ChimeraSettings.Summarized &&
      ChimeraSettings.Summarized.Patch &&
      typeof ChimeraSettings.Summarized.Patch.diffSummarizedModels === "function" &&
      typeof ChimeraSettings.Summarized.Patch.applySummarizedPatches === "function"
    );
  }

  function replaceCardByIdForPatch(cardId, html, opts) {
    return replaceCardById(cardId, function () {
      return html;
    }, opts);
  }

  function isSummarizedCardOpen(el) {
    if (!el) return false;
    if (el.tagName === "DETAILS") return !!el.open;
    return el.hasAttribute && el.hasAttribute("open");
  }

  function setSummarizedCardOpen(el, open) {
    if (!el) return;
    if (el.tagName === "DETAILS") {
      el.open = !!open;
      return;
    }
    if (el.classList && el.classList.contains("sum-card--collapsible")) {
      if (open) el.setAttribute("open", "");
      else el.removeAttribute("open");
      var hdr = el.querySelector(":scope > .sum-card__hdr");
      if (hdr) hdr.setAttribute("aria-expanded", open ? "true" : "false");
    }
  }

  function wireCollapsibleSummarizedPanel(root) {
    var psu = root || document.getElementById("panel-summarized");
    if (!psu) return;
    var CC = globalThis.ChimeraUI && globalThis.ChimeraUI.CollapsibleCard;
    if (CC && typeof CC.wireAll === "function") {
      try {
        CC.wireAll(psu);
      } catch (_eWire) {}
    }
    if (!ctx.summarizedCollapsibleObs && typeof MutationObserver !== "undefined") {
      ctx.summarizedCollapsibleObs = new MutationObserver(function () {
        wireCollapsibleSummarizedPanel(psu);
      });
      try {
        ctx.summarizedCollapsibleObs.observe(psu, { childList: true, subtree: true });
      } catch (_eObs) {}
    }
  }

  function scrollKindFromEl(el) {
    if (!el || !el.classList) return "metrics";
    if (el.classList.contains("sg-op-fallback-table-scroll")) return "fallback";
    if (el.classList.contains("sg-op-routing-table-scroll")) return "routing";
    if (el.classList.contains("sg-op-router-table-scroll")) return "router";
    if (el.classList.contains("sum-evlog__table-scroll")) return "evlog";
    return "metrics";
  }

  /** Stable key for nested scroll restore across card replace / full panel rebuild. */
  function nestedScrollCaptureKey(el, scrollSel) {
    if (!el) return "";
    if (el.id) {
      var cardId = "";
      try {
        var card = el.closest("details[id], article[id], .sum-feed-section[id]");
        cardId = card && card.id ? card.id : "panel";
      } catch (_eCard) {
        cardId = "panel";
      }
      return cardId + "#" + el.id;
    }
    var scope = null;
    try {
      scope = el.closest("details[id], article[id], .sum-feed-section[id]");
    } catch (_eScope) {}
    var scopeId = scope && scope.id ? scope.id : "panel";
    var kind = scrollKindFromEl(el);
    var peers = scope ? scope.querySelectorAll(scrollSel) : [];
    var idx = 0;
    for (var p = 0; p < peers.length; p++) {
      if (peers[p] === el) {
        idx = p;
        break;
      }
    }
    return scopeId + ":" + kind + ":" + idx;
  }

  function captureNestedScrollMap(scopeRoot, scrollSel) {
    var map = Object.create(null);
    if (!scopeRoot || !scrollSel || !scopeRoot.querySelectorAll) return map;
    try {
      var nodes = scopeRoot.querySelectorAll(scrollSel);
      for (var i = 0; i < nodes.length; i++) {
        var el = nodes[i];
        var key = nestedScrollCaptureKey(el, scrollSel);
        if (!key) continue;
        map[key] = { left: el.scrollLeft, top: el.scrollTop };
      }
    } catch (_eCap) {}
    return map;
  }

  function restoreNestedScrollMap(scopeRoot, scrollSel, map) {
    if (!scopeRoot || !scrollSel || !map) return;
    try {
      var nodes = scopeRoot.querySelectorAll(scrollSel);
      for (var i = 0; i < nodes.length; i++) {
        var el = nodes[i];
        var key = nestedScrollCaptureKey(el, scrollSel);
        var snap = map[key];
        if (!snap) continue;
        el.scrollLeft = snap.left;
        el.scrollTop = snap.top;
      }
    } catch (_eRest) {}
  }

  function captureSummarizedPanelUiState(psu) {
    var evlog = {};
    try {
      if (typeof globalThis.sumEvlogCapturePanelState === "function") {
        evlog = globalThis.sumEvlogCapturePanelState(psu) || {};
      }
    } catch (_eEvCap) {}
    return {
      evlog: evlog,
      nestedScroll: captureNestedScrollMap(psu, SUMMARIZED_CARD_SCROLL_SEL)
    };
  }

  /**
   * @param {{ scroll?: boolean, scrollOnly?: boolean }} [opts]
   * scroll:false — selection/search + nested table scroll, not evlog tbody scroll
   * scrollOnly — evlog tbody scroll + nested scroll after layout
   */
  function restoreSummarizedPanelUiState(psu, saved, opts) {
    opts = opts || {};
    if (!psu || !saved) return;
    var scrollOnly = !!opts.scrollOnly;
    if (!scrollOnly && saved.nestedScroll) {
      restoreNestedScrollMap(psu, SUMMARIZED_CARD_SCROLL_SEL, saved.nestedScroll);
    }
    if (!scrollOnly && typeof globalThis.sumEvlogApplyPanelState === "function") {
      try {
        globalThis.sumEvlogApplyPanelState(psu, saved.evlog || {}, { scroll: false });
      } catch (_eEvApply) {}
    }
    if (scrollOnly) {
      if (saved.nestedScroll) {
        restoreNestedScrollMap(psu, SUMMARIZED_CARD_SCROLL_SEL, saved.nestedScroll);
      }
      if (typeof globalThis.sumEvlogApplyPanelState === "function") {
        try {
          globalThis.sumEvlogApplyPanelState(psu, saved.evlog || {}, { scrollOnly: true });
        } catch (_eEvScroll) {}
      }
    }
  }

  function applySummarizedPanelPatch(psu, ops) {
    if (!psu || !ops || !ops.length) return { ok: true, applied: 0 };
    var uiSave = captureSummarizedPanelUiState(psu);
    var result = ChimeraSettings.Summarized.Patch.applySummarizedPatches(
      psu,
      ops,
      summarizedHtmlRenderers(),
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
      restoreSummarizedPanelUiState(psu, uiSave, { scroll: false });
      wireCollapsibleSummarizedPanel(psu);
      mountSummarizedYamlEditors(psu);
      window.requestAnimationFrame(function () {
        restoreSummarizedPanelUiState(psu, uiSave, { scrollOnly: true });
      });
    }
    return result;
  }

  function applySummarizedFullPanelRebuild(psu, nextModel, agg) {
    var prevScrollTop = psu.scrollTop;
    var prevScrollH = psu.scrollHeight;
    var nearPanelBottom =
      psu.scrollHeight - psu.scrollTop - psu.clientHeight <= stickPx;

    var openDetailIds = [];
    try {
      var dOpen = psu.querySelectorAll("details[open][id], article.sum-card--collapsible[open][id]");
      for (var di = 0; di < dOpen.length; di++) {
        if (dOpen[di].id) openDetailIds.push(dOpen[di].id);
      }
    } catch (e) { }

    var storyScroll = {};
    try {
      var sps = psu.querySelectorAll(".story-panel");
      for (var sj = 0; sj < sps.length; sj++) {
        var cStory = sps[sj].closest("details[id]");
        if (cStory && cStory.id) storyScroll[cStory.id] = sps[sj].scrollTop;
      }
    } catch (e1) { }

    /** Scroll for .sum-full-log[id] only; evlog tbody scroll lives in sumEvlog panel state (search/filter changes row height). */
    var fullLogScroll = {};
    try {
      var fls = psu.querySelectorAll(".sum-full-log[id]");
      for (var fk = 0; fk < fls.length; fk++) {
        if (fls[fk] && fls[fk].id) fullLogScroll[fls[fk].id] = fls[fk].scrollTop;
      }
    } catch (e2) { }

    var panelUiSave = captureSummarizedPanelUiState(psu);

    destroySummarizedYamlEditors(psu);
    psu.innerHTML = renderSummarizedHtmlFromModel(nextModel);
    ctx.lastSummarizedModel = nextModel;
    ctx.lastSummarizedAggregate = agg;

    if (typeof ctx.syncIndexerServiceSummaryDom === "function") {
      ctx.syncIndexerServiceSummaryDom();
    }
    if (typeof ctx.scheduleIndexerServiceSummaryFetch === "function") {
      ctx.scheduleIndexerServiceSummaryFetch(false);
    }

    if (typeof globalThis.sumEvlogHydrateAllIn === "function") {
      try {
        globalThis.sumEvlogHydrateAllIn(psu);
      } catch (eEv) {}
    }

    var openDetailSet = Object.create(null);
    for (var ri = 0; ri < openDetailIds.length; ri++) {
      if (openDetailIds[ri]) openDetailSet[openDetailIds[ri]] = true;
    }
    try {
      var allDet = psu.querySelectorAll("details[id]");
      for (var dj = 0; dj < allDet.length; dj++) {
        var det = allDet[dj];
        if (!det.id) continue;
        det.open = !!openDetailSet[det.id];
      }
      var allArt = psu.querySelectorAll("article.sum-card--collapsible[id]");
      for (var aj = 0; aj < allArt.length; aj++) {
        var art = allArt[aj];
        if (!art.id) continue;
        setSummarizedCardOpen(art, !!openDetailSet[art.id]);
      }
    } catch (eDet) {}
    wireCollapsibleSummarizedPanel(psu);
    mountSummarizedYamlEditors(psu);

    restoreSummarizedPanelUiState(psu, panelUiSave, { scroll: false });

    /** Best-effort outer scroll before paint: avoids scrollTop 0 flash while content height is still settling. */
    function applySummarizedOuterScrollSync() {
      if (nearPanelBottom) {
        psu.scrollTop = psu.scrollHeight;
      } else {
        var maxS = Math.max(0, psu.scrollHeight - psu.clientHeight);
        psu.scrollTop = Math.min(prevScrollTop, maxS);
      }
    }

    function restoreSummarizedNestedScrolls() {
      if (panelUiSave && panelUiSave.nestedScroll) {
        restoreNestedScrollMap(psu, SUMMARIZED_CARD_SCROLL_SEL, panelUiSave.nestedScroll);
      }
      for (var cid in storyScroll) {
        var cx = document.getElementById(cid);
        if (!cx) continue;
        var sp = cx.querySelector(".story-panel");
        if (sp) sp.scrollTop = storyScroll[cid];
      }
      for (var cid2 in fullLogScroll) {
        var fl = document.getElementById(cid2);
        if (!fl) continue;
        fl.scrollTop = fullLogScroll[cid2];
      }
    }

    applySummarizedOuterScrollSync();
    restoreSummarizedNestedScrolls();

    function finalizeSummarizedScrollAfterLayout() {
      restoreSummarizedNestedScrolls();
      if (nearPanelBottom) {
        psu.scrollTop = psu.scrollHeight;
      } else if (prevScrollH > 0) {
        var dh = psu.scrollHeight - prevScrollH;
        psu.scrollTop = Math.max(0, prevScrollTop + dh);
      }
      restoreSummarizedPanelUiState(psu, panelUiSave, { scrollOnly: true });
    }
    window.requestAnimationFrame(finalizeSummarizedScrollAfterLayout);
  }

  function refreshSummarizedPanel() {
    var psu = document.getElementById("panel-summarized");
    if (getViewMode() !== "summarized" || !psu) return;
    if (summarizedPanelInteractionBlocksRebuild()) {
      scheduleDeferredSummarizedRefresh();
      return;
    }
    clearSummarizedDirtySets();

    var forceFull = !!ctx.summarizedForceFullRebuild;
    ctx.summarizedForceFullRebuild = false;

    var snap = buildSummarizedFeedSnapshot();
    var nextModel = snap.model;
    var agg = snap.agg;
    var prevModel = ctx.lastSummarizedModel;

    if (!forceFull && prevModel && prevModel.cards && summarizedPatchAvailable()) {
      var Patch = ChimeraSettings.Summarized.Patch;
      var ops = Patch.diffSummarizedModels(prevModel, nextModel, {
        skipCardIds: summarizedPatchSkipCardIds()
      });
      if (!Patch.shouldUseFullRebuildFromOps(ops)) {
        var replaceCount = Patch.countReplaceCardOps(ops);
        if (replaceCount === 0) {
          if (summarizedSkippedCardsHashDelta(prevModel, nextModel)) {
            applySummarizedFullPanelRebuild(psu, nextModel, agg);
            ctx.lastSummarizedModel = nextModel;
            ctx.lastSummarizedAggregate = agg;
            return;
          }
          ctx.lastSummarizedModel = nextModel;
          ctx.lastSummarizedAggregate = agg;
          return;
        }
        if (!shouldSummarizedDirtyFullRebuild(replaceCount)) {
          var patchResult = applySummarizedPanelPatch(psu, ops);
          if (patchResult.ok) {
            ctx.lastSummarizedModel = nextModel;
            ctx.lastSummarizedAggregate = agg;
            return;
          }
        }
      }
    }

    applySummarizedFullPanelRebuild(psu, nextModel, agg);
  }

  function forceSummarizedFullRebuild(reason) {
    ctx.summarizedForceFullRebuild = reason || true;
    refreshSummarizedPanel();
  }

  window.__chimeraToggleGatewayProbes = function (on) {
    ctx.gatewayPanelShowProbes = !!on;
    refreshSummarizedPanel();
  };

  function yamlEditorApi() {
    return globalThis.ChimeraUI && globalThis.ChimeraUI.YamlEditorPanel
      ? globalThis.ChimeraUI.YamlEditorPanel
      : null;
  }

  function destroySummarizedYamlEditors(root) {
    var api = yamlEditorApi();
    if (api && typeof api.destroyIn === "function") api.destroyIn(root);
  }

  function mountSummarizedYamlEditors(root) {
    var api = yamlEditorApi();
    if (api && typeof api.mountAll === "function") {
      api.mountAll(root || document.getElementById("panel-summarized"));
    }
  }

  /**
   * Replace a single summarized card by id without assigning #panel-summarized innerHTML.
   * @returns {boolean} true when the card was found and replaced
   */
  function replaceCardById(cardId, buildHtml, opts) {
    opts = opts || {};
    if (getViewMode() !== "summarized") return false;
    if (!document.getElementById("panel-summarized")) return false;
    var oldEl = document.getElementById(cardId);
    if (!oldEl) return false;
    var preserveOpen = opts.preserveOpen !== false;
    var keepOpen = preserveOpen && isSummarizedCardOpen(oldEl);
    var scrollSel = opts.preserveScrollSelectors;
    var scrollMap = scrollSel ? captureNestedScrollMap(oldEl, scrollSel) : null;
    var cardUiSave = null;
    try {
      if (oldEl.querySelector && oldEl.querySelector("[data-sum-evlog-root]")) {
        cardUiSave = captureSummarizedPanelUiState(oldEl);
      }
    } catch (_eCardUi) {}
    destroySummarizedYamlEditors(oldEl);
    var wrap = document.createElement("div");
    wrap.innerHTML = (typeof buildHtml === "function" ? buildHtml() : String(buildHtml || "")).trim();
    var newEl = wrap.firstElementChild;
    if (!newEl || newEl.id !== cardId) return false;
    oldEl.parentNode.replaceChild(newEl, oldEl);
    if (preserveOpen) setSummarizedCardOpen(newEl, keepOpen);
    if (opts.cardVersionAttr !== false && opts.cardHash && newEl.setAttribute) {
      newEl.setAttribute("data-card-hash", String(opts.cardHash));
    }
    if (scrollSel && scrollMap) {
      restoreNestedScrollMap(newEl, scrollSel, scrollMap);
    }
    if (cardUiSave) {
      if (typeof globalThis.sumEvlogHydrateAllIn === "function") {
        try {
          globalThis.sumEvlogHydrateAllIn(newEl);
        } catch (_eEvCard) {}
      }
      restoreSummarizedPanelUiState(newEl, cardUiSave, { scroll: false });
      window.requestAnimationFrame(function () {
        restoreSummarizedPanelUiState(newEl, cardUiSave, { scrollOnly: true });
      });
    }
    if (newEl.classList && newEl.classList.contains("sum-card--collapsible")) {
      wireCollapsibleSummarizedPanel(newEl);
    }
    mountSummarizedYamlEditors(newEl);
    return true;
  }

  /** Replace only the gateway metrics card so periodic /api/ui/metrics polls do not rebuild the whole feed. */
  function patchGatewayUsageMetricsCard() {
    if (
      !replaceCardById("gw-usage-metrics", buildGatewayUsageCardHtml, {
        preserveOpen: true,
        preserveScrollSelectors: ".sum-metrics-table-wrap"
      })
    ) {
      refreshSummarizedPanel();
    }
  }

  /** Replace only the gateway overview card so /api/ui/state polls avoid full feed rebuilds. */
  function patchGatewayOverviewCard() {
    if (!replaceCardById("gw-overview", buildGatewayOverviewCardHtml, { preserveOpen: true })) {
      refreshSummarizedPanel();
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
      scheduleDeferredSummarizedRefresh();
      return;
    }
    if (ctx.coalescedFullRebuildTimer) return;
    ctx.coalescedFullRebuildTimer = setTimeout(function () {
      ctx.coalescedFullRebuildTimer = null;
      clearSummarizedDirtySets();
      ctx.suppressSummarizedDirty = true;
      try {
        forceSummarizedFullRebuild(reason || "dirty-storm-coalesced");
      } finally {
        ctx.suppressSummarizedDirty = false;
      }
    }, SUMMARIZED_DIRTY_FULL_REBUILD_DEBOUNCE_MS);
  }

  function beginSummarizedLiveSettle() {
    ctx.suppressSummarizedDirty = true;
    if (ctx.summarizedLiveSettleTimer) clearTimeout(ctx.summarizedLiveSettleTimer);
    cancelCoalescedFullRebuild();
    scheduleStoryRebuild();
    ctx.summarizedLiveSettleTimer = setTimeout(function () {
      ctx.summarizedLiveSettleTimer = null;
      ctx.suppressSummarizedDirty = false;
      scheduleSummarizedDirtyFlush();
    }, SUMMARIZED_LIVE_SETTLE_MS);
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

  /** Phase 3: coalesced per-card patches for live SSE lines (see summarizedDirtyRouting.js). */
  var SUMMARIZED_DIRTY_FULL_REBUILD_MIN = 10;
  var SUMMARIZED_DIRTY_FULL_REBUILD_RATIO = 0.3;
  ctx.summarizedDirtyCardIds = ctx.summarizedDirtyCardIds || Object.create(null);
  ctx.summarizedDirtyIndexerBucketIds = ctx.summarizedDirtyIndexerBucketIds || Object.create(null);
  ctx.summarizedReqToConv = ctx.summarizedReqToConv || Object.create(null);
  ctx.summarizedIndexRunToConv = ctx.summarizedIndexRunToConv || Object.create(null);

  function summarizedDirtyRoutingDeps() {
    return {
      getFlat: getFlat,
      strHash: strHash,
      normalizeServiceBucketKey: normalizeServiceBucketKey,
      indexerGroupIdForFlat: indexerGroupIdForFlat,
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
    var panel = document.getElementById("panel-summarized");
    if (!panel) return true;
    var total = panel.querySelectorAll("details.sum-card").length;
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
      var metaG = collectIndexerRunMeta(runG.id, runG.events, pmetaG);
      metaG = mergePersistedIndexerWatchRoots(metaG, runG.events, runG.id);
      var dk = indexerRunTimelineDedupeKey(metaG, runG.id);
      if (!dedupeGroups[dk]) dedupeGroups[dk] = [];
      dedupeGroups[dk].push(runG);
    }
    var dkIter;
    for (dkIter in dedupeGroups) {
      if (!Object.prototype.hasOwnProperty.call(dedupeGroups, dkIter)) continue;
      var grpRuns = dedupeGroups[dkIter];
      var run = pickCanonicalIndexerRun(grpRuns);
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
      var metaLive = collectIndexerRunMeta(run.id, run.events, pmetaLive);
      metaLive = mergePersistedIndexerWatchRoots(metaLive, run.events, run.id);
      var domId = indexerCardDomIdFromMeta(metaLive, run.id);
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
      model = buildSummarizedModelForAgg(agg || buildSummarizedAggregateState());
    }
    if (
      model &&
      globalThis.ChimeraSettings.Summarized.Render &&
      typeof ChimeraSettings.Summarized.Render.findCardById === "function"
    ) {
      var card = ChimeraSettings.Summarized.Render.findCardById(model, cardId);
      if (card) return renderSummarizedCardFromModel(card);
    }
    if (!agg) return null;
    if (cardId.indexOf("admin-provider-") === 0) {
      var providerId = cardId.slice("admin-provider-".length);
      var specPatch = lookupAdminProviderSpec(providerId);
      if (!specPatch) return null;
      return buildAdminProviderCardHtml(specPatch.id, specPatch.title, specPatch.avatar, specPatch.subtitle);
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
      return buildServiceCard(nm, arr, { byRun: agg.byRun, partitionRegistry: agg.partitionRegistry });
    }
    var ci;
    for (ci = 0; ci < agg.mergedConv.length; ci++) {
      var g = agg.mergedConv[ci];
      if (conversationDomIdForGroup(g) === cardId) return buildConvCard(g);
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
      var metaG = collectIndexerRunMeta(runG.id, runG.events, pmetaG);
      metaG = mergePersistedIndexerWatchRoots(metaG, runG.events, runG.id);
      if (indexerCardDomIdFromMeta(metaG, runG.id) === cardId) {
        return buildIndexerCard(runG, agg.partitionRegistry);
      }
    }
    return null;
  }

  function patchSummarizedCard(cardId, agg, nextModel) {
    var prevModel = ctx.lastSummarizedModel;
    if (!nextModel) nextModel = buildSummarizedModelForAgg(agg || buildSummarizedAggregateState());
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
      scheduleDeferredSummarizedRefresh();
      return;
    }
    if (summarizedAdminEditingActive()) {
      return;
    }
    var dirtyCount = summarizedDirtyCardCount();
    if (!dirtyCount) return;
    var agg = buildSummarizedAggregateState();
    var nextModel = buildSummarizedModelForAgg(agg);
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
      scheduleStoryRebuild();
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

  function fetchGatewayMetrics() {
    if (ctx.uiUnauthorized) return;
    fetch("/api/ui/metrics?limit=150", { credentials: "same-origin" })
      .then(function (res) {
        if (res.status === 401) {
          markUiUnauthorized();
          return null;
        }
        if (!res.ok) throw new Error("HTTP " + res.status);
        return res.json();
      })
      .then(function (data) {
        if (!data) return;
        ctx.metricsCache = data;
        if (getViewMode() === "summarized") patchGatewayUsageMetricsCard();
      })
      .catch(function (e) {
        ctx.metricsCache = {
          metrics_store_open: false,
          message: e && e.message ? String(e.message) : String(e)
        };
        if (getViewMode() === "summarized") patchGatewayUsageMetricsCard();
      });
  }

  function syncMetricsPolling() {
    if (metricsPollTimer) {
      try {
        clearInterval(metricsPollTimer);
      } catch (x) { }
      metricsPollTimer = null;
    }
    if (ctx.uiUnauthorized || getViewMode() !== "summarized") return;
    fetchGatewayMetrics();
    metricsPollTimer = setInterval(fetchGatewayMetrics, METRICS_POLL_MS);
  }

  function fetchUiState() {
    if (ctx.uiUnauthorized) return Promise.resolve(null);
    return fetch("/api/ui/state", { credentials: "same-origin" })
      .then(function (r) {
        if (r.status === 401) {
          markUiUnauthorized();
          return null;
        }
        if (!r.ok) throw new Error("HTTP " + r.status);
        return r.json();
      })
      .then(function (j) {
        if (!j) return null;
        ctx.adminStateCache = j;
        if (j.gateway) ctx.gatewayOverviewCache = j.gateway;
        return j;
      });
  }

  function fetchGatewayOverview() {
    if (ctx.uiUnauthorized) return;
    fetchUiState()
      .then(function (data) {
        if (!data || !data.gateway) return;
        if (getViewMode() === "summarized") patchGatewayOverviewCard();
      })
      .catch(function (e) {
        ctx.gatewayOverviewCache = {
          _error: e && e.message ? String(e.message) : String(e)
        };
        if (getViewMode() === "summarized") patchGatewayOverviewCard();
      });
  }

  function resyncVisibleProvidersFromCatalog() {
    if (adminVisibleProviderIds().length > 0) return Promise.resolve();
    var api = providerCatalogApi();
    if (!api) return Promise.resolve();
    return api.fetchProviderCatalog(ctx, { force: true }).then(function (data) {
      if (!data || getViewMode() !== "summarized") return;
      if (adminVisibleProviderIds().length > 0) scheduleStoryRebuild();
    }).catch(function () {
      return null;
    });
  }

  function runUiStatePoll(opts) {
    if (ctx.uiUnauthorized) return Promise.resolve();
    var showErr = opts && opts.showErr;
    return Promise.all([fetchUiState(), fetchAdminTokens()])
      .then(function () {
        if (ctx.uiUnauthorized || getViewMode() !== "summarized") return;
        return resyncVisibleProvidersFromCatalog();
      })
      .then(function () {
        if (ctx.uiUnauthorized || getViewMode() !== "summarized") return;
        return prefetchProviderModelsAvailability();
      })
      .then(function () {
        if (ctx.uiUnauthorized || getViewMode() !== "summarized") return;
        patchGatewayOverviewCard();
        patchAdminCardsFromPoll();
      })
      .catch(function (e) {
        ctx.gatewayOverviewCache = {
          _error: e && e.message ? String(e.message) : String(e)
        };
        if (getViewMode() === "summarized") patchGatewayOverviewCard();
        if (showErr && !ctx.uiUnauthorized) {
          adminSetMessage("err", e && e.message ? e.message : String(e));
        }
      });
  }

  function syncUiStatePolling() {
    if (uiStatePollTimer) {
      try {
        clearInterval(uiStatePollTimer);
      } catch (x) {}
      uiStatePollTimer = null;
    }
    if (ctx.uiUnauthorized || getViewMode() !== "summarized") return;
    runUiStatePoll({ showErr: true });
    uiStatePollTimer = setInterval(function () {
      runUiStatePoll({ showErr: false });
    }, UI_STATE_POLL_MS);
  }

  function fetchChimeraBrokerProviderSnapshot() {
    if (ctx.uiUnauthorized) return;
    fetch("/api/ui/chimera-broker/providers", { credentials: "same-origin" })
      .then(function (res) {
        if (res.status === 401) {
          markUiUnauthorized();
          return null;
        }
        if (!res.ok) throw new Error("HTTP " + res.status);
        return res.json();
      })
      .then(function (data) {
        if (!data) return;
        ctx.chimeraBrokerProviderSnapshot = { fetchedClientMs: Date.now(), data: data };
        if (getViewMode() === "summarized") patchChimeraBrokerProviderUiFromSnapshot();
      })
      .catch(function () {
        // Keep any prior snapshot — staleness check in the renderer handles fallback.
      });
  }

  function syncChimeraBrokerProviderPolling() {
    if (chimeraBrokerProviderPollTimer) {
      try {
        clearInterval(chimeraBrokerProviderPollTimer);
      } catch (x) { }
      chimeraBrokerProviderPollTimer = null;
    }
    if (ctx.uiUnauthorized || getViewMode() !== "summarized") return;
    fetchChimeraBrokerProviderSnapshot();
    chimeraBrokerProviderPollTimer = setInterval(fetchChimeraBrokerProviderSnapshot, CHIMERA_BROKER_PROVIDER_POLL_MS);
  }

  /** Replace chimera-broker provider health UI after a snapshot poll (expanded strip + collapsed summary indicators). */
  function patchChimeraBrokerProviderHealthStrip() {
    if (getViewMode() !== "summarized") return;
    var arr = collectChimeraBrokerBufferForStrip();
    var oldEl = document.getElementById("chimera-broker-provider-health-strip");
    if (oldEl) {
      var wrap = document.createElement("div");
      wrap.innerHTML = chimeraBrokerProviderHealthStripHtml(arr).trim();
      var newEl = wrap.firstElementChild;
      if (newEl && newEl.id === "chimera-broker-provider-health-strip") {
        oldEl.parentNode.replaceChild(newEl, oldEl);
      }
    }
    var compactOld = document.getElementById("chimera-broker-provider-health-compact");
    if (compactOld) {
      var w2 = document.createElement("div");
      w2.innerHTML = chimeraBrokerProviderHealthStripHtml(arr, { compact: true }).trim();
      var n2 = w2.firstElementChild;
      if (n2 && n2.id === "chimera-broker-provider-health-compact") {
        compactOld.parentNode.replaceChild(n2, compactOld);
      }
    }
  }

  /** After /api/ui/chimera-broker/providers returns, refresh broker strip + admin provider cards. */
  function patchChimeraBrokerProviderUiFromSnapshot() {
    if (getViewMode() !== "summarized") return;
    patchChimeraBrokerProviderHealthStrip();
    patchChimeraBrokerAvailableModelsCount();
    var needRebuild = false;
    var visibleForBroker = adminVisibleProviderIds();
    for (var pi = 0; pi < visibleForBroker.length; pi++) {
      if (!patchAdminProviderCard(visibleForBroker[pi])) needRebuild = true;
    }
    if (needRebuild) refreshSummarizedPanel();
  }

  function chimeraBrokerProviderSnapshotDataForUi() {
    if (!ctx.chimeraBrokerProviderSnapshot || !ctx.chimeraBrokerProviderSnapshot.data) return null;
    var snapshotAgeMs = Date.now() - Number(ctx.chimeraBrokerProviderSnapshot.fetchedClientMs || 0);
    if (snapshotAgeMs > CHIMERA_BROKER_PROVIDER_STALE_MS) return null;
    return ctx.chimeraBrokerProviderSnapshot.data;
  }

  function patchChimeraBrokerAvailableModelsCount() {
    if (getViewMode() !== "summarized") return;
    var el = document.getElementById("chimera-broker-available-models-count");
    if (!el) return;
    if (typeof ctx.chimeraBrokerAvailableModelCountLabel !== "function") return;
    el.textContent = ctx.chimeraBrokerAvailableModelCountLabel(collectChimeraBrokerBufferForStrip());
  }

  /** Mirror the chimera-broker-bucket selection in refreshSummarizedPanel so the patched strip's
   *  log-derived fallback sees the same arr the original renderExpandedService("chimera-broker") saw. */
  function collectChimeraBrokerBufferForStrip() {
    var out = [];
    for (var i = 0; i < entryCache.length; i++) {
      var e = entryCache[i];
      if (!e || !e.parsed) continue;
      var f = getFlat(e.parsed);
      var svc = f.service ? String(f.service) : "";
      var isChimeraBroker = svc === "chimera-broker" || (e.source === "chimera-broker") || entryRoutesToChimeraBrokerBucket(e);
      if (isChimeraBroker) out.push(e);
    }
    return out;
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

  function notifyWorkspaceDraftMsg(msg, isErr) {
    if (typeof ctx.notifyWorkspaceDraftMsg === "function") ctx.notifyWorkspaceDraftMsg(msg, isErr);
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


  function overviewStatePillClass(state) {
    var s = String(state || "").toLowerCase();
    if (s === "ok" || s === "up") return "sum-st-active";
    if (s === "degraded" || s === "down" || s === "unavailable") return "sum-st-error";
    return "sum-st-monitor";
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

  function adminJsonRequest(url, method, body) {
    return fetch(url, {
      method: method || "POST",
      credentials: "same-origin",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(body || {})
    }).then(function (r) {
      return r.json().catch(function () { return {}; }).then(function (j) {
        if (r.status === 401) throw new Error("Unauthorized");
        if (!r.ok) throw new Error((j && (j.error || (j.error && j.error.message))) || ("HTTP " + r.status));
        return j;
      });
    });
  }

  function adminPostJSON(url, body) {
    return adminJsonRequest(url, "POST", body);
  }

  function adminPutJSON(url, body) {
    return adminJsonRequest(url, "PUT", body);
  }

  function fetchProviderModels(providerId) {
    if (ctx.uiUnauthorized) return Promise.reject(new Error("Unauthorized"));
    var pid = String(providerId || "").trim().toLowerCase();
    if (!pid) return Promise.reject(new Error("provider required"));
    return fetch("/api/ui/providers/" + encodeURIComponent(pid) + "/models", { credentials: "same-origin" })
      .then(function (r) {
        return r.json().catch(function () { return {}; }).then(function (j) {
          if (r.status === 401) throw new Error("Unauthorized");
          if (!r.ok) throw new Error((j && j.error) || ("HTTP " + r.status));
          return j;
        });
      });
  }

  function lookupVmSummary(vmId) {
    var gw = ctx.adminStateCache && ctx.adminStateCache.gateway;
    var vms = gw && gw.virtual_models && Array.isArray(gw.virtual_models) ? gw.virtual_models : [];
    for (var i = 0; i < vms.length; i++) {
      if (vms[i] && Number(vms[i].id) === Number(vmId)) return vms[i];
    }
    return null;
  }

  /** Drop a saved virtual model from feed caches and DOM immediately after DELETE. */
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
    syncSummarizedModelCache();
  }

  function syncVmSummaryFromDetail(detail) {
    if (!detail || detail.id == null) return;
    var gw = ctx.adminStateCache && ctx.adminStateCache.gateway;
    if (!gw || !gw.virtual_models) return;
    var key = String(detail.id);
    for (var i = 0; i < gw.virtual_models.length; i++) {
      if (gw.virtual_models[i] && String(gw.virtual_models[i].id) === key) {
        var row = gw.virtual_models[i];
        row.enabled = !!detail.enabled;
        row.name = detail.name;
        row.version = detail.version;
        row.description = detail.description;
        row.visibility = detail.visibility;
        row.routing_policy_enabled = !!detail.routing_policy_enabled;
        row.tool_router_enabled = !!detail.tool_router_enabled;
        row.router_models = detail.router_models;
        row.fallback_depth = detail.fallback_chain && detail.fallback_chain.length ? detail.fallback_chain.length : 0;
        break;
      }
    }
  }

  function virtualModelCardEl(vmId) {
    return document.getElementById("virtual-model-" + String(vmId));
  }

  function virtualModelPanelIsOpen(vmId) {
    var el = virtualModelCardEl(vmId);
    return !!(el && el.open);
  }

  function fetchVirtualModelDetail(vmId, force) {
    if (ctx.uiUnauthorized) return Promise.resolve(null);
    var key = String(vmId);
    if (!ctx.virtualModelDetails) ctx.virtualModelDetails = {};
    if (!ctx.virtualModelUi) ctx.virtualModelUi = {};
    var ui = ctx.virtualModelUi[key];
    if (!ui) {
      ui = ctx.virtualModelUi[key] = { panelOpen: false, hydrated: false };
    }
    if (!force && ctx.virtualModelDetails[key]) {
      return Promise.resolve(ctx.virtualModelDetails[key]);
    }
    ui.detailLoading = true;
    return fetch("/api/ui/virtual-models/" + encodeURIComponent(key), { credentials: "same-origin" })
      .then(function (r) {
        if (r.status === 401) {
          markUiUnauthorized();
          return null;
        }
        if (!r.ok) throw new Error("HTTP " + r.status);
        return r.json();
      })
      .then(function (j) {
        ui.detailLoading = false;
        if (!j) return null;
        ctx.virtualModelDetails[key] = j;
        syncVmSummaryFromDetail(j);
        return j;
      })
      .catch(function (e) {
        ui.detailLoading = false;
        throw e;
      });
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

  function patchVirtualModelCard(vmId, opts) {
    opts = opts || {};
    if (opts.onlyIfOpen && !virtualModelPanelIsOpen(vmId)) return false;
    var summary = lookupVmSummary(vmId);
    if (!summary || typeof buildVirtualModelCardHtml !== "function") return false;
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
        return buildVirtualModelCardHtml(summary);
      },
      { preserveOpen: keepOpen, preserveScrollSelectors: ADMIN_CARD_TABLE_SCROLL_SEL }
    );
    if (ok && ctx.virtualModelUi && ctx.virtualModelUi[String(vmId)]) {
      ctx.virtualModelUi[String(vmId)].hydrated = true;
    }
    return ok;
  }

  function fetchAdminState() {
    return fetchUiState();
  }

  function fetchAdminTokens() {
    if (ctx.uiUnauthorized) return Promise.resolve();
    return fetch("/api/ui/tokens", { credentials: "same-origin" })
      .then(function (r) {
        if (r.status === 401) {
          markUiUnauthorized();
          return null;
        }
        if (!r.ok) throw new Error("HTTP " + r.status);
        return r.json();
      })
      .then(function (j) {
        if (!j) return;
        ctx.tokenListCache = Array.isArray(j.tokens) ? j.tokens : [];
        for (var i = 0; i < ctx.tokenListCache.length; i++) {
          var row = ctx.tokenListCache[i] || {};
          var tid = row.tenant_id != null ? String(row.tenant_id).trim() : "";
          var tok = row.token != null ? String(row.token).trim() : "";
          if (tid && tok) ctx.adminCreatedTokenByTenant[tid] = tok;
        }
      });
  }

  function patchAdminUsersCard() {
    return replaceCardById("admin-users", buildAdminUsersCardHtml, { preserveOpen: false });
  }

  function patchAdminProviderCard(providerId) {
    var spec = lookupAdminProviderSpec(providerId);
    if (!spec) return false;
    return replaceCardById(
      "admin-provider-" + providerId,
      function () {
        return buildAdminProviderCardHtml(spec.id, spec.title, spec.avatar, spec.subtitle);
      },
      { preserveOpen: true, preserveScrollSelectors: ADMIN_CARD_TABLE_SCROLL_SEL }
    );
  }

  var providerModelsPrefetchInFlight = Object.create(null);

  function providerIdsNeedingModelsPrefetch() {
    var st = ctx.adminStateCache || {};
    var providers = st.providers || {};
    var hasCreds = typeof ctx.providerHasCredentials === "function" ? ctx.providerHasCredentials : null;
    var visible = adminVisibleProviderIds();
    var out = [];
    for (var i = 0; i < visible.length; i++) {
      var pid = String(visible[i] || "").trim().toLowerCase();
      if (!pid) continue;
      if (ctx.adminProviderModelsEditingId === pid) continue;
      if (ctx.adminProviderModelsCache && ctx.adminProviderModelsCache[pid]) continue;
      var prow = providers[pid] || {};
      if (!prow.models_configured) continue;
      if (hasCreds && !hasCreds(pid, prow)) continue;
      out.push(pid);
    }
    return out;
  }

  /** Load saved per-model availability for providers with tenant configuration (read-only checkboxes). */
  function prefetchProviderModelsAvailability() {
    if (ctx.uiUnauthorized || typeof fetchProviderModels !== "function") return Promise.resolve(false);
    var ids = providerIdsNeedingModelsPrefetch();
    if (!ids.length) return Promise.resolve(false);
    var jobs = [];
    for (var i = 0; i < ids.length; i++) {
      (function (pid) {
        if (providerModelsPrefetchInFlight[pid]) {
          jobs.push(providerModelsPrefetchInFlight[pid]);
          return;
        }
        providerModelsPrefetchInFlight[pid] = fetchProviderModels(pid)
          .then(function (doc) {
            if (!ctx.adminProviderModelsCache) ctx.adminProviderModelsCache = {};
            ctx.adminProviderModelsCache[pid] = doc;
          })
          .catch(function () {
            /* keep read-only fallback until a later poll retries */
          })
          .finally(function () {
            delete providerModelsPrefetchInFlight[pid];
          });
        jobs.push(providerModelsPrefetchInFlight[pid]);
      })(ids[i]);
    }
    return Promise.all(jobs).then(function () {
      if (getViewMode() !== "summarized") return false;
      var anyPatched = false;
      var needRebuild = false;
      for (var j = 0; j < ids.length; j++) {
        var id = ids[j];
        if (!ctx.adminProviderModelsCache || !ctx.adminProviderModelsCache[id]) continue;
        if (patchAdminProviderCard(id)) anyPatched = true;
        else needRebuild = true;
      }
      if (needRebuild) scheduleStoryRebuild();
      return anyPatched;
    });
  }

  /** Targeted admin card updates after /api/ui/state + /api/ui/tokens poll (no full panel innerHTML). */
  function patchAdminCardsFromPoll() {
    if (getViewMode() !== "summarized") return;
    if (summarizedPanelInteractionBlocksRebuild()) return;
    var psu = document.getElementById("panel-summarized");
    if (!psu) return;

    var prevModel = ctx.lastSummarizedModel;
    var agg = buildSummarizedAggregateState();
    var nextModel = buildSummarizedModelForAgg(agg);
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
    if (needRebuild) scheduleStoryRebuild();
    else {
      ctx.lastSummarizedModel = nextModel;
      ctx.lastSummarizedAggregate = agg;
    }
  }










  function fetchTokenLabels() {
    if (ctx.uiUnauthorized) return;
    fetch("/api/ui/tokens", { credentials: "same-origin" })
      .then(function (r) {
        if (r.status === 401) {
          markUiUnauthorized();
          return null;
        }
        if (!r.ok) return null;
        return r.json();
      })
      .then(function (data) {
        if (!data || !Array.isArray(data.tokens)) return;
        ctx.tokenLabelByTenant = {};
        for (var i = 0; i < data.tokens.length; i++) {
          var row = data.tokens[i];
          var tid =
            row.tenant_id != null && String(row.tenant_id).trim() !== ""
              ? String(row.tenant_id).trim()
              : "";
          if (!tid) continue;
          var tok = row.token != null && String(row.token).trim() !== "" ? String(row.token).trim() : "";
          if (tok) ctx.adminCreatedTokenByTenant[tid] = tok;
          var lb =
            row.label != null && String(row.label).trim() !== ""
              ? String(row.label).trim()
              : "";
          ctx.tokenLabelByTenant[tid] = lb || tid;
        }
        if (getViewMode() === "summarized") scheduleStoryRebuild();
      })
      .catch(function () { });
  }

  /** Plain-text subject for scoped log panel titles (no HTML). */

  /** dt/dd fragment for expanded indexer Summary: user, project, flavor, optional workspace row id, file count. Joined into one indexer-run-kv with Watched paths. */

  function normalizeManagedPathRowsForEdit(ws) {
    var out = [];
    if (!ws || !Array.isArray(ws.paths)) return out;
    var pi;
    for (pi = 0; pi < ws.paths.length; pi++) {
      var row = ws.paths[pi] || {};
      var pid = row.id != null ? Number(row.id) : NaN;
      var pth = row.path != null ? String(row.path).trim() : "";
      if (!pth) continue;
      if (!isNaN(pid) && pid > 0) out.push({ id: pid, path: pth });
    }
    return out;
  }

  function cloneManagedPathRows(arr) {
    var out = [];
    if (!Array.isArray(arr)) return out;
    var i;
    for (i = 0; i < arr.length; i++) {
      out.push({
        id: arr[i].id != null && !isNaN(Number(arr[i].id)) ? Math.trunc(Number(arr[i].id)) : null,
        path: String(arr[i].path != null ? arr[i].path : "")
      });
    }
    return out;
  }

  function buildManagedWorkspacePathsEditHtml(wsNum, pathRows) {
    if (typeof ctx.buildManagedWorkspacePathsEditHtml === "function") {
      return ctx.buildManagedWorkspacePathsEditHtml(wsNum, pathRows);
    }
    return "";
  }

  function buildManagedWorkspaceToolbarHtml(wsNum, isEdit, titleText) {
    if (typeof ctx.buildManagedWorkspaceToolbarHtml === "function") {
      return ctx.buildManagedWorkspaceToolbarHtml(wsNum, isEdit, titleText);
    }
    return "";
  }

  function beginWorkspaceManagedEdit(wsNum) {
    var ws =
      typeof ctx.findOperatorWorkspaceByNumericId === "function"
        ? ctx.findOperatorWorkspaceByNumericId(wsNum)
        : null;
    if (!ws) {
      notifyWorkspaceDraftMsg("Workspace not found.", true);
      return;
    }
    var snap = normalizeManagedPathRowsForEdit(ws);
    ctx.workspaceManagedEditId = wsNum;
    ctx.workspaceManagedStaging = {
      wsNum: wsNum,
      initialSnapshot: cloneManagedPathRows(snap),
      paths: cloneManagedPathRows(snap)
    };
    ctx.summarizedForceFullRebuild = true;
    refreshSummarizedPanel();
  }

  function cancelWorkspaceManagedEdit() {
    ctx.workspaceManagedEditId = null;
    ctx.workspaceManagedStaging = null;
    ctx.workspaceManagedFolderPickerOpen = false;
    ctx.summarizedForceFullRebuild = true;
    refreshSummarizedPanel();
  }

  function refreshWorkspaceManagedPaths() {
    var st = ctx.workspaceManagedStaging;
    if (!st || !Array.isArray(st.initialSnapshot)) return;
    st.paths = cloneManagedPathRows(st.initialSnapshot);
    ctx.workspaceManagedFolderPickerOpen = false;
    ctx.summarizedForceFullRebuild = true;
    refreshSummarizedPanel();
  }

  function refreshOperatorIndexerWorkspaceStateFromConfig() {
    if (typeof ctx.hydrateIndexerServiceSummaryFromApi !== "function") {
      return Promise.resolve();
    }
    return ctx.hydrateIndexerServiceSummaryFromApi(true).then(function () {
      scheduleStoryRebuild();
    });
  }

  function saveManagedWorkspacePaths(wsNum) {
    var st = ctx.workspaceManagedStaging;
    if (!st || st.wsNum !== wsNum || !Array.isArray(st.paths)) {
      notifyWorkspaceDraftMsg("Nothing to save.", true);
      return;
    }
    if (!st.paths.length) {
      notifyWorkspaceDraftMsg("Add at least one watched path.", true);
      return;
    }
    var initial = st.initialSnapshot || [];
    var cur = st.paths;
    var curPersistedIds = {};
    var ci;
    for (ci = 0; ci < cur.length; ci++) {
      if (cur[ci].id != null && !isNaN(Number(cur[ci].id))) curPersistedIds[Math.trunc(Number(cur[ci].id))] = true;
    }
    var toDelete = [];
    var ii;
    for (ii = 0; ii < initial.length; ii++) {
      var iid = initial[ii].id != null ? Math.trunc(Number(initial[ii].id)) : NaN;
      if (!isNaN(iid) && iid > 0 && !curPersistedIds[iid]) toDelete.push(iid);
    }
    var toAdd = [];
    for (ci = 0; ci < cur.length; ci++) {
      var pth = String(cur[ci].path != null ? cur[ci].path : "").trim();
      if (!pth) continue;
      if (cur[ci].id == null || isNaN(Number(cur[ci].id))) toAdd.push(pth);
    }

    var chain = Promise.resolve();
    var di;
    for (di = 0; di < toDelete.length; di++) {
      (function (pathId) {
        chain = chain.then(function () {
          return fetch("/api/ui/indexer/workspace-paths/" + pathId, {
            method: "DELETE",
            credentials: "same-origin"
          }).then(function (res) {
            return res.json().then(function (j) {
              if (!res.ok) throw new Error((j && j.error) || res.statusText || "delete path failed");
            });
          });
        });
      })(toDelete[di]);
    }
    var ai;
    for (ai = 0; ai < toAdd.length; ai++) {
      (function (absPath) {
        chain = chain.then(function () {
          return fetch("/api/ui/indexer/workspaces/" + wsNum + "/paths", {
            method: "POST",
            credentials: "same-origin",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ path: absPath })
          }).then(function (res) {
            return res.json().then(function (j) {
              if (!res.ok) throw new Error((j && j.error) || res.statusText || "add path failed");
              return j;
            });
          });
        });
      })(toAdd[ai]);
    }
    chain
      .then(function () {
        ctx.workspaceManagedEditId = null;
        ctx.workspaceManagedStaging = null;
        notifyWorkspaceDraftMsg("Workspace updated.", false);
        return refreshOperatorIndexerWorkspaceStateFromConfig();
      })
      .catch(function (err) {
        notifyWorkspaceDraftMsg(err && err.message ? err.message : String(err), true);
      });
  }

  function deleteManagedWorkspace(wsNum) {
    if (
      !window.confirm(
        "Delete this workspace and all watched paths from configuration? The indexer will stop indexing these paths."
      )
    ) {
      return;
    }
    fetch("/api/ui/indexer/workspaces/" + wsNum, { method: "DELETE", credentials: "same-origin" })
      .then(function (res) {
        return res.json().then(function (j) {
          if (!res.ok) throw new Error((j && j.error) || res.statusText || "delete failed");
        });
      })
      .then(function () {
        ctx.workspaceManagedEditId = null;
        ctx.workspaceManagedStaging = null;
        notifyWorkspaceDraftMsg("Workspace removed.", false);
        return refreshOperatorIndexerWorkspaceStateFromConfig();
      })
      .catch(function (err) {
        notifyWorkspaceDraftMsg(err && err.message ? err.message : String(err), true);
      });
  }



  function convLastTs(g) {
    var mx = 0;
    for (var u = 0; u < g.events.length; u++) {
      var ti = entryInstant({ ts: g.events[u].ts });
      if (ti) mx = Math.max(mx, ti.getTime());
    }
    return mx;
  }

  function convFirstTs(g) {
    var mn = null;
    for (var u = 0; u < g.events.length; u++) {
      var ti = entryInstant({ ts: g.events[u].ts });
      if (ti) {
        if (mn == null || ti.getTime() < mn.getTime()) mn = ti;
      }
    }
    return mn ? mn.getTime() : 0;
  }

  /**
   * One conversation card per gateway group key (principal + conversation_id).
   * Do not merge different conversation_ids for the same principal by time gap — that hid separate chats in the log UI.
   */
  function sortConversationGroupsByRecency(groups) {
    var arr = [];
    for (var key in groups) {
      if (!Object.prototype.hasOwnProperty.call(groups, key)) continue;
      var gx = groups[key];
      var tmin = convFirstTs(gx);
      var tmax = convLastTs(gx);
      if (!tmax) continue;
      if (!tmin) tmin = tmax;
      arr.push({
        pid: gx.pid,
        cid: gx.cid,
        cids: [gx.cid],
        events: gx.events.slice(),
        tmin: tmin,
        tmax: tmax
      });
    }
    arr.sort(function (a, b) {
      return b.tmax - a.tmax;
    });
    for (var k = 0; k < arr.length; k++) {
      arr[k].events.sort(function (a, b) {
        var sa = a.seq != null ? Number(a.seq) : 0;
        var sb = b.seq != null ? Number(b.seq) : 0;
        if (sa !== sb) return sa - sb;
        var ta = entryInstant({ ts: a.ts });
        var tb = entryInstant({ ts: b.ts });
        if (!ta && !tb) return 0;
        if (!ta) return -1;
        if (!tb) return 1;
        return ta.getTime() - tb.getTime();
      });
    }
    return arr;
  }

  /** Structured vectorstore lines may use service=chimera-vectorstore or vectorstore.* msgs without service. */
  function entryIsVectorstoreLine(ent) {
    var f = getFlat(ent.parsed);
    var svcL = String(f.service || "").toLowerCase();
    if (svcL === "vectorstore" || svcL === "chimera-vectorstore") return true;
    var srcL = ent && String(ent.source || "").toLowerCase();
    if (srcL === "vectorstore" || srcL === "chimera-vectorstore") return true;
    var rawMsg = f.msg != null ? f.msg : f.message != null ? f.message : "";
    var msg = String(rawMsg).toLowerCase().trim();
    if (msg.indexOf("vectorstore.") === 0) return true;
    if (msg.indexOf("chimera-vectorstore.") === 0) return true;
    return false;
  }

  /** Structured indexer stderr lines sometimes omit service=indexer; still bucket under Services → Indexer. */
  function entryIsIndexerLine(ent) {
    var f = getFlat(ent.parsed);
    var svcL = String(f.service || "").toLowerCase();
    if (svcL === "indexer" || svcL === "chimera-indexer") return true;
    var srcL = ent && String(ent.source || "").toLowerCase();
    if (srcL === "indexer" || srcL === "chimera-indexer") return true;
    var rawMsg = f.msg != null ? f.msg : f.message != null ? f.message : "";
    var msg = String(rawMsg).toLowerCase().trim();
    if (msg.indexOf("indexer.") === 0) return true;
    if (msg.indexOf("gateway.indexer") === 0) return true;
    return false;
  }

  /** Normalize indexer/Gateway scope flavor placeholders so "" matches UI "—". */
  function normalizeIndexerScopeFlavor(v) {
    var s = v != null ? String(v) : "";
    s = s.replace(/\s+/g, " ").trim();
    if (!s) return "";
    if (s === "—" || s === "\u2014" || s === "-" || s.toLowerCase() === "none") return "";
    return s;
  }

  function rebuildIndexerRootScopeMaps() {
    ctx.indexerRootScopeByRootId = {};
    if (
      !globalThis.ChimeraSettings ||
      !ChimeraSettings.Derive ||
      typeof ChimeraSettings.Derive.indexerParseRootScopes !== "function"
    ) {
      return;
    }
    var gi;
    for (gi = 0; gi < entryCache.length; gi++) {
      var ent = entryCache[gi];
      if (!entryIsIndexerLine(ent)) continue;
      var raw = getFlat(ent.parsed);
      var msg = String(raw.msg != null ? raw.msg : raw.message != null ? raw.message : "")
        .toLowerCase()
        .trim();
      if (msg !== "indexer.run.start" && msg !== "indexer run start") continue;
      var rows = ChimeraSettings.Derive.indexerParseRootScopes(raw.root_scopes);
      var ri;
      for (ri = 0; ri < rows.length; ri++) {
        var row = rows[ri];
        if (!row || typeof row !== "object") continue;
        var rslug = row.root_id != null ? String(row.root_id).trim() : "";
        if (!rslug) continue;
        ctx.indexerRootScopeByRootId[rslug] = {
          workspace_id: row.workspace_id != null ? String(row.workspace_id).trim() : "",
          path: row.path != null ? String(row.path).trim() : "",
          ingest_project: row.ingest_project != null ? String(row.ingest_project).trim() : "",
          flavor_id: row.flavor_id != null ? String(row.flavor_id).trim() : ""
        };
      }
    }
  }

  /** Normalize watched root prefix for matching indexer `root` paths (Windows-safe). */
  function rootUnderOneOfPrefixes(root, prefixes) {
    var r = String(root || "")
      .replace(/\\/g, "/")
      .replace(/\/+$/, "")
      .toLowerCase();
    if (!r) return false;
    var i;
    for (i = 0; i < prefixes.length; i++) {
      var p = String(prefixes[i] || "")
        .replace(/\\/g, "/")
        .replace(/\/+$/, "")
        .toLowerCase();
      if (!p) continue;
      if (r === p) return true;
      if (r.indexOf(p + "/") === 0) return true;
    }
    return false;
  }

  function inferTenantForOpwsBucket(bucketId) {
    var segs = String(bucketId || "").split("\u001e");
    if (segs[0] !== "opws" || segs.length < 3) return "";
    var wantWid = String(segs[1] || "").trim();
    var wantProj = String(segs[2] || "").trim();
    var wantFlav = segs.length > 3 ? normalizeIndexerScopeFlavor(segs[3]) : "";
    var opWsCtx = ctx.operatorWsFullLogCtx[bucketId];
    var roots = opWsCtx && opWsCtx.paths ? opWsCtx.paths : [];
    var ei;
    for (ei = entryCache.length - 1; ei >= 0; ei--) {
      var ent = entryCache[ei];
      if (!entryIsIndexerLine(ent)) continue;
      var raw = getFlat(ent.parsed);
      var f =
        globalThis.ChimeraSettings &&
          ChimeraSettings.Derive &&
          typeof ChimeraSettings.Derive.indexerAugmentFlat === "function"
          ? ChimeraSettings.Derive.indexerAugmentFlat(ent, raw)
          : raw;
      var fp = String(f.project_id || f.ingest_project || "").trim();
      var ff = normalizeIndexerScopeFlavor(f.flavor_id);
      if (fp !== wantProj || ff !== wantFlav) continue;
      var sw = f.scope_workspace_id != null ? String(f.scope_workspace_id).trim() : "";
      if (wantWid && sw === wantWid) {
        return String(f.tenant_id || f.principal_id || f.tenant || "").trim();
      }
      var rk = f.root != null ? String(f.root).trim() : "";
      if (wantWid && rk && ctx.indexerRootScopeByRootId[rk]) {
        var rsi = ctx.indexerRootScopeByRootId[rk];
        if (String(rsi.workspace_id || "") === wantWid) {
          return String(f.tenant_id || f.principal_id || f.tenant || "").trim();
        }
        if (rsi.path && roots.length && rootUnderOneOfPrefixes(rsi.path, roots)) {
          return String(f.tenant_id || f.principal_id || f.tenant || "").trim();
        }
      }
    }
    return "";
  }

  function indexerOperatorWorkspaceScopeMatch(ent, bucketId, f) {
    if (!entryIsIndexerLine(ent)) return false;
    var segs = String(bucketId || "").split("\u001e");
    if (segs[0] !== "opws" || segs.length < 3) return false;
    var wantWid = String(segs[1] || "").trim();
    if (!wantWid) return false;
    var wantProj = String(segs[2] || "").trim();
    var wantFlav = segs.length > 3 ? normalizeIndexerScopeFlavor(segs[3]) : "";
    var fp = String(f.project_id || f.ingest_project || "").trim();
    var ff = normalizeIndexerScopeFlavor(f.flavor_id);
    if (wantProj !== "") {
      if (fp !== wantProj || ff !== wantFlav) return false;
    } else if (fp !== "") {
      return false;
    }
    var sw = f.scope_workspace_id != null ? String(f.scope_workspace_id).trim() : "";
    if (wantWid && sw === wantWid) return true;
    var opWsCtx = ctx.operatorWsFullLogCtx[bucketId];
    var roots = opWsCtx && opWsCtx.paths ? opWsCtx.paths : [];
    var rk = f.root != null ? String(f.root).trim() : "";
    if (wantWid && rk && ctx.indexerRootScopeByRootId[rk]) {
      var rsi = ctx.indexerRootScopeByRootId[rk];
      if (String(rsi.workspace_id || "") === wantWid) return true;
      if (rsi.path && roots.length && rootUnderOneOfPrefixes(rsi.path, roots)) return true;
    }
    return false;
  }

  /** Synthetic bucket id so operator-managed workspaces reuse scoped full-log filtering over entryCache. */
  function operatorWorkspaceSyntheticBucketId(ws) {
    var wid = canonicalWorkspaceRowIdKey(ws.id);
    var pj = String(ws.project_id != null ? ws.project_id : "").trim();
    var fvKey = normalizeIndexerScopeFlavor(ws.flavor_id);
    var bucketId = "opws\u001e" + wid + "\u001e" + pj + "\u001e" + fvKey;
    ctx.operatorWsFullLogCtx[bucketId] = { paths: operatorWorkspacePaths(ws).slice() };
    return bucketId;
  }

  function indexerBucketScopeCoords(bucketId, evs, partitionRegistry) {
    bucketId = bucketId != null ? String(bucketId).trim() : "";
    evs = Array.isArray(evs) ? evs : [];
    if (bucketId.indexOf("opws\u001e") === 0) {
      var opSegs = bucketId.split("\u001e");
      if (opSegs.length >= 3) {
        var opProj = String(opSegs[2] || "").trim();
        var opFlav = opSegs.length > 3 ? normalizeIndexerScopeFlavor(opSegs[3]) : "";
        var opTenant = inferTenantForOpwsBucket(bucketId);
        if (opTenant && opProj && opProj !== "—") {
          return { tenant: opTenant, project: opProj, flavor: opFlav };
        }
      }
    }
    var syn =
      globalThis.ChimeraSettings &&
        ChimeraSettings.Derive &&
        typeof ChimeraSettings.Derive.parseIgSyntheticGid === "function"
        ? ChimeraSettings.Derive.parseIgSyntheticGid(bucketId)
        : null;
    var tenant = "";
    var proj = "";
    var flavor = "";
    if (syn) {
      tenant = syn.tenant || "";
      proj = syn.project || "";
      flavor = syn.flavor || "";
    } else {
      var i;
      for (i = 0; i < evs.length; i++) {
        var rawF = getFlat(evs[i].parsed);
        var fIx =
          globalThis.ChimeraSettings &&
            ChimeraSettings.Derive &&
            typeof ChimeraSettings.Derive.indexerAugmentFlat === "function"
            ? ChimeraSettings.Derive.indexerAugmentFlat(evs[i], rawF)
            : rawF;
        if (
          String(fIx.indexer_target_key || "").trim() === bucketId ||
          String(fIx.indexer_key || "").trim() === bucketId
        ) {
          tenant = String(fIx.tenant_id || fIx.principal_id || fIx.tenant || "").trim();
          proj = String(fIx.project_id || fIx.ingest_project || "").trim();
          flavor = String(fIx.flavor_id != null ? fIx.flavor_id : "").trim();
          break;
        }
      }
      if (!tenant || !proj) {
        for (i = 0; i < evs.length; i++) {
          var rawG = getFlat(evs[i].parsed);
          var fIy =
            globalThis.ChimeraSettings &&
              ChimeraSettings.Derive &&
              typeof ChimeraSettings.Derive.indexerAugmentFlat === "function"
              ? ChimeraSettings.Derive.indexerAugmentFlat(evs[i], rawG)
              : rawG;
          var rid = fIy.index_run_id != null ? String(fIy.index_run_id).trim() : "";
          if (rid && rid === bucketId) {
            tenant = String(fIy.tenant_id || fIy.principal_id || fIy.tenant || "").trim();
            proj = String(fIy.project_id || fIy.ingest_project || "").trim();
            flavor = String(fIy.flavor_id != null ? fIy.flavor_id : "").trim();
            break;
          }
        }
      }
    }
    if (!proj || proj === "—") return null;
    if (!tenant) return null;
    if (flavor === "—") flavor = "";
    return { tenant: tenant, project: proj, flavor: flavor };
  }

  function indexerExpectedVectorstoreCollectionForBucket(bucketId, evs, partitionRegistry) {
    var c = indexerBucketScopeCoords(bucketId, evs, partitionRegistry);
    if (!c) return "";
    if (
      globalThis.ChimeraSettings &&
      ChimeraSettings.Derive &&
      typeof ChimeraSettings.Derive.vectorstoreCollectionName === "function"
    ) {
      return ChimeraSettings.Derive.vectorstoreCollectionName(c.tenant, c.project, c.flavor);
    }
    return "";
  }

  function indexerSupervisedWorkspaceLifecycleSlug(msg) {
    return (
      msg === "indexer.supervised.workspaces_changed" ||
      msg === "indexer.supervised.workspaces_reload" ||
      msg === "indexer.supervised.workspaces_session_start" ||
      msg === "indexer.supervised.workspaces_apply_failed" ||
      msg === "gateway.operator.workspace.path_added" ||
      msg === "gateway.operator.workspace.path_deleted"
    );
  }

  function csvFieldIds(raw) {
    if (raw == null) return [];
    return String(raw)
      .split(",")
      .map(function (s) {
        return s.trim();
      })
      .filter(Boolean);
  }

  function indexerLifecycleEventMatchesBucket(f, bucketScopeCoords) {
    if (!f || typeof f !== "object") return false;
    var msgSlug = String(f.msg != null ? f.msg : f.message != null ? f.message : "").trim();
    if (!indexerSupervisedWorkspaceLifecycleSlug(msgSlug)) return false;
    if (!bucketScopeCoords || !bucketScopeCoords.project) return true;
    var logWsIds = csvFieldIds(f.workspace_ids);
    if (msgSlug === "gateway.operator.workspace.path_added" || msgSlug === "gateway.operator.workspace.path_deleted") {
      var wsId = f.workspace_id != null ? String(f.workspace_id).trim() : "";
      if (wsId) logWsIds = [wsId];
    }
    if (!logWsIds.length) return true;
    var nested = ctx.lastIndexerOperatorWorkspacesNested || [];
    var wi;
    for (wi = 0; wi < nested.length; wi++) {
      var wsRow = nested[wi];
      var wsKey = canonicalWorkspaceRowIdKey(wsRow.id);
      var wsNumFn = ctx.operatorWorkspaceNumericId;
      var wsNum = typeof wsNumFn === "function" ? String(wsNumFn(wsRow)) : "";
      var hi;
      for (hi = 0; hi < logWsIds.length; hi++) {
        if (logWsIds[hi] !== wsKey && logWsIds[hi] !== wsNum) continue;
        if (String(wsRow.project_id || "").trim() === bucketScopeCoords.project) return true;
      }
    }
    return false;
  }

  function indexerScopeFullLogInclude(ent, bucketId, partitionRegistry, expectedVectorstoreCollection, bucketScopeCoords) {
    bucketId = bucketId != null ? String(bucketId).trim() : "";
    if (!bucketId) return true;

    var rawFlat = getFlat(ent.parsed);
    var f = rawFlat;
    if (
      globalThis.ChimeraSettings &&
      ChimeraSettings.Derive &&
      typeof ChimeraSettings.Derive.indexerAugmentFlat === "function"
    ) {
      f = ChimeraSettings.Derive.indexerAugmentFlat(ent, rawFlat);
    }

    if (
      globalThis.ChimeraSettings &&
      ChimeraSettings.Derive &&
      typeof ChimeraSettings.Derive.indexerFlatOmitFromWorkspaceScopedLog === "function" &&
      ChimeraSettings.Derive.indexerFlatOmitFromWorkspaceScopedLog(f)
    ) {
      return false;
    }

    if (indexerLifecycleEventMatchesBucket(f, bucketScopeCoords)) return true;

    var srcL = String(ent.source || "").toLowerCase();
    var svcL = String(f.service || "").toLowerCase();
    if (srcL === "chimera-vectorstore" || srcL === "chimera-vectorstore" || svcL === "chimera-vectorstore" || svcL === "chimera-vectorstore") {
      var coll = f.collection != null ? String(f.collection).trim() : "";
      var exp = expectedVectorstoreCollection != null ? String(expectedVectorstoreCollection).trim() : "";
      if (!coll || !exp) return false;
      return coll === exp;
    }

    if (bucketId.indexOf("opws\u001e") === 0) {
      return indexerOperatorWorkspaceScopeMatch(ent, bucketId, f);
    }

    var rid = f.index_run_id != null ? String(f.index_run_id).trim() : "";
    var st = rid && partitionRegistry && partitionRegistry[rid] ? partitionRegistry[rid] : null;

    if (
      globalThis.ChimeraSettings &&
      ChimeraSettings.Derive &&
      typeof ChimeraSettings.Derive.indexerBucketGidsForLine === "function" &&
      st &&
      st.keys &&
      st.keys.length > 0
    ) {
      var gids = ChimeraSettings.Derive.indexerBucketGidsForLine(f, st);
      if (gids && gids.length === 1) {
        return String(gids[0]).trim() === bucketId;
      }
      if (gids && gids.length > 1) {
        return false;
      }
    }

    var itk = f.indexer_target_key != null ? String(f.indexer_target_key).trim() : "";
    if (itk && itk === bucketId) return true;

    var ikk = f.indexer_key != null ? String(f.indexer_key).trim() : "";
    if (ikk && ikk === bucketId) return true;

    if (
      globalThis.ChimeraSettings &&
      ChimeraSettings.Derive &&
      typeof ChimeraSettings.Derive.indexerGroupKeyFromFlat === "function"
    ) {
      var gk = ChimeraSettings.Derive.indexerGroupKeyFromFlat(f);
      if (gk != null && String(gk).trim() === bucketId) return true;
    }

    if (bucketId.indexOf("ig\u001e") === 0) {
      var parts = bucketId.split("\u001e");
      if (parts.length >= 4) {
        var wantP = parts[2] || "";
        var wantF = parts[3] || "";
        var fp = String(f.project_id != null ? f.project_id : f.ingest_project != null ? f.ingest_project : "").trim();
        var ff = String(f.flavor_id != null ? f.flavor_id : "").trim();
        if (fp === wantP && ff === wantF) return true;
      }
    }

    if (rid && rid === bucketId) return true;

    var coords = bucketScopeCoords;
    if (coords && coords.tenant && coords.project) {
      var ragMsg = String(f.msg != null ? f.msg : f.message != null ? f.message : "").trim();
      if (ragMsg.toLowerCase() === "rag.retrieve.source") {
        var rgt = String(f.tenant_id != null ? f.tenant_id : f.principal_id != null ? f.principal_id : "").trim();
        var rgp = String(f.project_id != null ? f.project_id : f.project != null ? f.project : "").trim();
        var rgf = String(f.flavor_id != null ? f.flavor_id : "").trim();
        if (
          rgt &&
          rgp &&
          rgt === coords.tenant &&
          rgp === coords.project &&
          normalizeIndexerScopeFlavor(rgf) === normalizeIndexerScopeFlavor(coords.flavor)
        )
          return true;
      }
    }

    return false;
  }

  function filterEventsForIndexerScopeFullLog(evs, bucketId, partitionRegistry) {
    var out = [];
    if (!Array.isArray(evs)) return out;
    var expColl = indexerExpectedVectorstoreCollectionForBucket(bucketId, evs, partitionRegistry);
    var bucketCoords = indexerBucketScopeCoords(bucketId, evs, partitionRegistry);
    for (var i = 0; i < evs.length; i++) {
      if (indexerScopeFullLogInclude(evs[i], bucketId, partitionRegistry, expColl, bucketCoords)) out.push(evs[i]);
    }
    return out;
  }

  function entryIsGatewayUpstreamRelay(ent) {
    if (typeof ctx.entryIsGatewayUpstreamRelay === "function") {
      return ctx.entryIsGatewayUpstreamRelay(ent);
    }
    return false;
  }

  function entryRoutesToChimeraBrokerBucket(ent) {
    if (typeof ctx.entryRoutesToChimeraBrokerBucket === "function") {
      return ctx.entryRoutesToChimeraBrokerBucket(ent);
    }
    return false;
  }

  /** Stable /ui/settings Workspaces bucket: backend indexer_key or tenant + project + flavor fallback. */
  function indexerGroupIdForFlat(fR) {
    if (
      globalThis.ChimeraSettings &&
      ChimeraSettings.Derive &&
      typeof ChimeraSettings.Derive.indexerGroupKeyFromFlat === "function"
    ) {
      var gx = ChimeraSettings.Derive.indexerGroupKeyFromFlat(fR);
      if (gx != null && String(gx).trim() !== "") return String(gx).trim();
    }
    var itk =
      fR.indexer_target_key != null && String(fR.indexer_target_key).trim() !== ""
        ? String(fR.indexer_target_key).trim()
        : "";
    var ik =
      fR.indexer_key != null && String(fR.indexer_key).trim() !== "" ? String(fR.indexer_key).trim() : "";
    var rid = fR.index_run_id != null && fR.index_run_id !== "" ? String(fR.index_run_id) : "";
    return itk || ik || rid || "";
  }

  function buildSummarizedAggregateState() {
    rebuildIndexerRootScopeMaps();
    var groups = {};
    var reqToConv = {};
    var indexRunToConv = {};
    var gix;
    for (gix = 0; gix < entryCache.length; gix++) {
      if (typeof ctx.tryRegisterRequestConversationCorrelationPrimary === "function") {
        ctx.tryRegisterRequestConversationCorrelationPrimary(reqToConv, getFlat(entryCache[gix].parsed));
      }
    }
    for (gix = 0; gix < entryCache.length; gix++) {
      if (typeof ctx.tryRegisterRequestConversationCorrelationRagFallback === "function") {
        ctx.tryRegisterRequestConversationCorrelationRagFallback(reqToConv, getFlat(entryCache[gix].parsed));
      }
    }
    for (gix = 0; gix < entryCache.length; gix++) {
      var entIr = entryCache[gix];
      var fIr = getFlat(entIr.parsed);
      var msgIr = String(fIr.msg != null ? fIr.msg : fIr.message != null ? fIr.message : "").trim();
      if (msgIr !== "ingest.complete" && msgIr !== "ingest.failed" && msgIr !== "ingest.chunked.error") continue;
      var irKey = fIr.index_run_id != null ? String(fIr.index_run_id).trim() : "";
      var cidIr = fIr.conversation_id != null ? String(fIr.conversation_id).trim() : "";
      var pidIr =
        fIr.principal_id != null ? String(fIr.principal_id).trim() : fIr.tenant != null ? String(fIr.tenant).trim() : "";
      if (irKey && cidIr && pidIr && !indexRunToConv[irKey]) indexRunToConv[irKey] = { pid: pidIr, cid: cidIr };
    }
    for (gix = 0; gix < entryCache.length; gix++) {
      var ent = entryCache[gix];
      var p = ent.parsed;
      var f = getFlat(p);
      var cid = f.conversation_id != null ? String(f.conversation_id).trim() : "";
      var pid = f.principal_id != null ? String(f.principal_id).trim() : f.tenant != null ? String(f.tenant).trim() : "";
      if (cid) {
        if (typeof ctx.pushConversationGroupedEvent === "function") {
          ctx.pushConversationGroupedEvent(groups, pid, cid, ent, p, "direct");
        }
        continue;
      }
      var ridJoin = f.request_id != null ? String(f.request_id).trim() : "";
      if (
        ridJoin &&
        reqToConv[ridJoin] &&
        typeof ctx.conversationRequestIdTier2EligibleLocal === "function" &&
        ctx.conversationRequestIdTier2EligibleLocal(f) &&
        typeof ctx.pushConversationGroupedEvent === "function"
      ) {
        ctx.pushConversationGroupedEvent(
          groups,
          reqToConv[ridJoin].pid,
          reqToConv[ridJoin].cid,
          ent,
          p,
          "request_id"
        );
        continue;
      }
      var irJoin = f.index_run_id != null ? String(f.index_run_id).trim() : "";
      if (
        irJoin &&
        indexRunToConv[irJoin] &&
        typeof ctx.conversationIndexRunTier3EligibleLocal === "function" &&
        ctx.conversationIndexRunTier3EligibleLocal(f) &&
        typeof ctx.pushConversationGroupedEvent === "function"
      ) {
        ctx.pushConversationGroupedEvent(
          groups,
          indexRunToConv[irJoin].pid,
          indexRunToConv[irJoin].cid,
          ent,
          p,
          "ingest"
        );
      }
    }
    var gkSort;
    for (gkSort in groups) {
      if (!Object.prototype.hasOwnProperty.call(groups, gkSort)) continue;
      groups[gkSort].events.sort(function (a, b) {
        var sa = a.seq != null ? Number(a.seq) : 0;
        var sb = b.seq != null ? Number(b.seq) : 0;
        if (sa !== sb) return sa - sb;
        var ta = entryInstant({ ts: a.ts });
        var tb = entryInstant({ ts: b.ts });
        if (!ta && !tb) return 0;
        if (!ta) return -1;
        if (!tb) return 1;
        return ta.getTime() - tb.getTime();
      });
    }
    if (
      globalThis.ChimeraSettings &&
      ChimeraSettings.Derive &&
      typeof ChimeraSettings.Derive.joinVectorstoreLineConversationTier === "function"
    ) {
      for (gix = 0; gix < entryCache.length; gix++) {
        var entQ = entryCache[gix];
        if (
          typeof ctx.entryIsVectorstoreSubprocessForConvJoin !== "function" ||
          !ctx.entryIsVectorstoreSubprocessForConvJoin(entQ)
        ) {
          continue;
        }
        var fQ = getFlat(entQ.parsed);
        var collQ = fQ.collection != null ? String(fQ.collection).trim() : "";
        if (!collQ) continue;
        var tQ = entryInstant({ ts: entQ.ts });
        if (!tQ) continue;
        var tMs = tQ.getTime();
        var gkQ;
        for (gkQ in groups) {
          if (!Object.prototype.hasOwnProperty.call(groups, gkQ)) continue;
          var grp = groups[gkQ];
          var qMatch = null;
          if (typeof ChimeraSettings.Derive.joinVectorstoreLineConversationMatch === "function") {
            qMatch = ChimeraSettings.Derive.joinVectorstoreLineConversationMatch(grp.events, getFlat, fQ, tMs);
          }
          var tierQ = qMatch && qMatch.tier ? qMatch.tier : ChimeraSettings.Derive.joinVectorstoreLineConversationTier(grp.events, getFlat, fQ, tMs);
          if (tierQ && typeof ctx.pushConversationGroupedEvent === "function") {
            ctx.pushConversationGroupedEvent(groups, grp.pid, grp.cid, entQ, entQ.parsed, tierQ, qMatch);
          }
        }
      }
      for (gkSort in groups) {
        if (!Object.prototype.hasOwnProperty.call(groups, gkSort)) continue;
        groups[gkSort].events.sort(function (a, b) {
          var sa = a.seq != null ? Number(a.seq) : 0;
          var sb = b.seq != null ? Number(b.seq) : 0;
          if (sa !== sb) return sa - sb;
          var ta2 = entryInstant({ ts: a.ts });
          var tb2 = entryInstant({ ts: b.ts });
          if (!ta2 && !tb2) return 0;
          if (!ta2) return -1;
          if (!tb2) return 1;
          return ta2.getTime() - tb2.getTime();
        });
      }
    }
    var buckets = {
      "chimera-gateway": [],
      "chimera-vectorstore": [],
      "chimera-broker": [],
      "chimera-indexer": []
    };
    for (var bi = 0; bi < entryCache.length; bi++) {
      var entB = entryCache[bi];
      var pB = entB.parsed;
      var fB = getFlat(pB);
      var svcKey = "";
      if (entryRoutesToChimeraBrokerBucket(entB)) svcKey = "chimera-broker";
      else if (entryIsVectorstoreLine(entB)) svcKey = "chimera-vectorstore";
      else if (entryIsIndexerLine(entB)) svcKey = "chimera-indexer";
      else {
        svcKey = normalizeServiceBucketKey(fB.service, entB.source);
        if (!svcKey) svcKey = "chimera-gateway";
      }
      if (!buckets[svcKey]) buckets[svcKey] = [];
      buckets[svcKey].push(entB);
    }
    var byRun = {};
    var partitionRegistry = {};
    var ibuilt = null;
    if (
      globalThis.ChimeraSettings &&
      ChimeraSettings.Derive &&
      typeof ChimeraSettings.Derive.indexerBucketsFromCache === "function"
    ) {
      ibuilt = ChimeraSettings.Derive.indexerBucketsFromCache(entryCache, getFlat);
      if (ibuilt && ibuilt.targetStateByRunId) partitionRegistry = ibuilt.targetStateByRunId;
      if (ibuilt && ibuilt.buckets) byRun = ibuilt.buckets;
    }
    if (!ibuilt) {
      byRun = {};
      partitionRegistry = {};
      for (var ri = 0; ri < entryCache.length; ri++) {
        var entRL = entryCache[ri];
        var fRL = getFlat(entRL.parsed);
        if (
          globalThis.ChimeraSettings &&
          ChimeraSettings.Derive &&
          typeof ChimeraSettings.Derive.indexerFlatMsgForPresent === "function"
        ) {
          var msgRL = ChimeraSettings.Derive.indexerFlatMsgForPresent(fRL);
          if (msgRL === "indexer.state") continue;
          if (msgRL === "indexer.storage.stats" || msgRL.indexOf("indexer.storage.stats") === 0) continue;
        }
        var groupIdL = indexerGroupIdForFlat(fRL);
        if (!groupIdL) continue;
        if (!byRun[groupIdL]) byRun[groupIdL] = { id: groupIdL, events: [] };
        byRun[groupIdL].events.push(entRL);
      }
    } else {
      for (var normK in byRun) {
        if (!Object.prototype.hasOwnProperty.call(byRun, normK)) continue;
        var arrN = byRun[normK];
        byRun[normK] = { id: normK, events: arrN };
      }
    }
    ctx.lastIndexerSummarizeByRun = byRun;
    ctx.lastIndexerSummarizePartitionRegistry = partitionRegistry;
    var qFan = buckets["chimera-vectorstore"];
    if (qFan && qFan.length && byRun && Object.keys(byRun).length) {
      var collByRun = {};
      var buck;
      for (buck in byRun) {
        if (!Object.prototype.hasOwnProperty.call(byRun, buck)) continue;
        var runB = byRun[buck];
        collByRun[buck] = indexerExpectedVectorstoreCollectionForBucket(runB.id, runB.events, partitionRegistry);
      }
      var qx, qb;
      for (qx = 0; qx < qFan.length; qx++) {
        var qEnt = qFan[qx];
        var qFl = getFlat(qEnt.parsed);
        var qCol = qFl.collection != null ? String(qFl.collection).trim() : "";
        if (!qCol) continue;
        for (qb in byRun) {
          if (!Object.prototype.hasOwnProperty.call(byRun, qb)) continue;
          if (collByRun[qb] === qCol) byRun[qb].events.push(qEnt);
        }
      }
      for (var qs in byRun) {
        if (!Object.prototype.hasOwnProperty.call(byRun, qs)) continue;
        byRun[qs].events.sort(function (a, b) {
          var sa = a.seq != null ? Number(a.seq) : 0;
          var sb = b.seq != null ? Number(b.seq) : 0;
          if (sa !== sb) return sa - sb;
          var ta = entryInstant({ ts: a.ts });
          var tb = entryInstant({ ts: b.ts });
          if (!ta && !tb) return 0;
          if (!ta) return -1;
          if (!tb) return 1;
          return ta.getTime() - tb.getTime();
        });
      }
    }

    // Attach gateway RAG retrieval lines to the right indexer bucket so the
    // Workspaces card "Recently evaluated files" can show retrieved sources.
    var gFan = buckets["chimera-gateway"];
    if (gFan && gFan.length && byRun && Object.keys(byRun).length) {
      // Build bucket → {tenant, project, flavor} map from existing indexer events.
      var scopeByRun = {};
      var rkx;
      var runIds = Object.keys(byRun);
      for (rkx = 0; rkx < runIds.length; rkx++) {
        var runX = byRun[runIds[rkx]];
        if (!runX || !runX.events) continue;
        var pmX = null;
        if (
          partitionRegistry &&
          globalThis.ChimeraSettings &&
          ChimeraSettings.Derive &&
          typeof ChimeraSettings.Derive.indexerPartitionMetaForRun === "function"
        ) {
          pmX = ChimeraSettings.Derive.indexerPartitionMetaForRun(partitionRegistry, runX.id, runX.events, getFlat);
        }
        var metaX = collectIndexerRunMeta(runX.id, runX.events, pmX);
        if (!metaX) continue;
        scopeByRun[runX.id] = {
          tenant: metaX.tenantId != null ? String(metaX.tenantId).trim() : "",
          project: metaX.projectId != null ? String(metaX.projectId).trim() : "",
          flavor: metaX.flavorId != null ? String(metaX.flavorId).trim() : ""
        };
      }
      var gx, gb;
      for (gx = 0; gx < gFan.length; gx++) {
        var gEnt = gFan[gx];
        var gFl = getFlat(gEnt.parsed);
        var gMsg = String(gFl.msg != null ? gFl.msg : gFl.message != null ? gFl.message : "").trim();
        if (gMsg !== "rag.retrieve.source") continue;
        var gt = String(gFl.tenant_id != null ? gFl.tenant_id : gFl.principal_id != null ? gFl.principal_id : "").trim();
        var gp = String(gFl.project_id != null ? gFl.project_id : gFl.project != null ? gFl.project : "").trim();
        var gf = String(gFl.flavor_id != null ? gFl.flavor_id : "").trim();
        if (!gt || !gp) continue;
        for (gb in byRun) {
          if (!Object.prototype.hasOwnProperty.call(byRun, gb)) continue;
          var sc = scopeByRun[gb];
          if (!sc || !sc.project) continue;
          if (
            (sc.tenant && sc.tenant !== gt) ||
            sc.project !== gp ||
            normalizeIndexerScopeFlavor(sc.flavor) !== normalizeIndexerScopeFlavor(gf)
          )
            continue;
          byRun[gb].events.push(gEnt);
        }
      }

      // Keep chronological order after attaching.
      for (var gsort in byRun) {
        if (!Object.prototype.hasOwnProperty.call(byRun, gsort)) continue;
        byRun[gsort].events.sort(function (a, b) {
          var sa = a.seq != null ? Number(a.seq) : 0;
          var sb = b.seq != null ? Number(b.seq) : 0;
          if (sa !== sb) return sa - sb;
          var ta = entryInstant({ ts: a.ts });
          var tb = entryInstant({ ts: b.ts });
          if (!ta && !tb) return 0;
          if (!ta) return -1;
          if (!tb) return 1;
          return ta.getTime() - tb.getTime();
        });
      }
    }
    ctx.summarizedReqToConv = reqToConv;
    ctx.summarizedIndexRunToConv = indexRunToConv;
    var mergedConv = sortConversationGroupsByRecency(groups);
    return {
      groups: groups,
      reqToConv: reqToConv,
      indexRunToConv: indexRunToConv,
      buckets: buckets,
      byRun: byRun,
      partitionRegistry: partitionRegistry,
      mergedConv: mergedConv
    };
  }

  function summarizedModelState(agg) {
    return {
      agg: agg,
      gatewayOverviewCache: ctx.gatewayOverviewCache,
      metricsCache: ctx.metricsCache,
      adminStateCache: ctx.adminStateCache,
      tokenListCache: ctx.tokenListCache,
      workspaceDrafts: ctx.workspaceDrafts,
      adminProviderSpecs: adminProviderSpecsFromVisible(),
      virtualModelDrafts: ctx.virtualModelDrafts,
      adminProviderModelsEditingId: ctx.adminProviderModelsEditingId,
      workspaceManagedEditId: ctx.workspaceManagedEditId,
      lastIndexerOperatorWorkspacesNested: ctx.lastIndexerOperatorWorkspacesNested
    };
  }

  function summarizedModelDeps() {
    return {
      strHash: strHash,
      conversationDomIdForGroup: conversationDomIdForGroup,
      convLastTs: convLastTs,
      primaryLogMessage: primaryLogMessage,
      conversationCardModelForGroup: ctx.conversationCardModelForGroup,
      conversationCardStatus: ctx.conversationCardStatus,
      indexerPartitionMetaForRun: function (partitionRegistry, runId, events) {
        if (
          partitionRegistry &&
          globalThis.ChimeraSettings &&
          ChimeraSettings.Derive &&
          typeof ChimeraSettings.Derive.indexerPartitionMetaForRun === "function"
        ) {
          return ChimeraSettings.Derive.indexerPartitionMetaForRun(partitionRegistry, runId, events, getFlat);
        }
        return null;
      },
      collectIndexerRunMeta: ctx.collectIndexerRunMeta,
      mergePersistedIndexerWatchRoots: ctx.mergePersistedIndexerWatchRoots,
      indexerRunTimelineDedupeKey: ctx.indexerRunTimelineDedupeKey,
      pickCanonicalIndexerRun: ctx.pickCanonicalIndexerRun,
      workspaceCardTitleFromIndexerMeta: ctx.workspaceCardTitleFromIndexerMeta,
      indexerCardTitleSortLabel: ctx.indexerCardTitleSortLabel,
      indexerCardDomIdFromMeta: ctx.indexerCardDomIdFromMeta,
      indexerCardIdentityKey: ctx.indexerCardIdentityKey,
      indexerCardIdentityKeyFromSnap: ctx.indexerCardIdentityKeyFromSnap,
      loadIndexerWatchRootsStore: ctx.loadIndexerWatchRootsStore,
      dedupeOperatorWorkspacesNested: ctx.dedupeOperatorWorkspacesNested,
      canonicalWorkspaceRowIdKey: ctx.canonicalWorkspaceRowIdKey,
      workspaceDraftComparableManagedTitle: ctx.workspaceDraftComparableManagedTitle,
      operatorManagedWorkspaceTitleText: ctx.operatorManagedWorkspaceTitleText,
      operatorWorkspaceCoveredByIndexerRuns: ctx.operatorWorkspaceCoveredByIndexerRuns,
      operatorWorkspaceNumericId: ctx.operatorWorkspaceNumericId,
      indexerWorkspaceEditActiveForMeta: function (meta) {
        if (ctx.workspaceManagedEditId == null || !ctx.workspaceManagedStaging) return false;
        var opWs =
          typeof ctx.findOperatorWorkspaceMatchingIndexerMeta === "function"
            ? ctx.findOperatorWorkspaceMatchingIndexerMeta(meta)
            : null;
        if (!opWs) return false;
        return typeof ctx.operatorWorkspaceNumericId === "function"
          ? ctx.operatorWorkspaceNumericId(opWs) === ctx.workspaceManagedEditId
          : false;
      },
      indexerRunQualifiesForWorkspaceCard: function (run, partitionRegistry) {
        if (
          globalThis.ChimeraSettings &&
          ChimeraSettings.Derive &&
          typeof ChimeraSettings.Derive.indexerRunQualifiesForWorkspaceCard === "function"
        ) {
          return ChimeraSettings.Derive.indexerRunQualifiesForWorkspaceCard(
            run,
            partitionRegistry,
            getFlat,
            function (runId, evs, opts) {
              return typeof ctx.collectIndexerRunMeta === "function"
                ? ctx.collectIndexerRunMeta(runId, evs, opts && opts.partitionMeta)
                : null;
            },
            {
              tokenLabelByTenant: ctx.tokenLabelByTenant,
              indexerFlatMsg: function (fl) {
                return typeof ctx.indexerFlatMsg === "function" ? ctx.indexerFlatMsg(fl) : "";
              },
              flatLooksLikeIndexerRunStart: function (fl) {
                return typeof ctx.flatLooksLikeIndexerRunStart === "function"
                  ? ctx.flatLooksLikeIndexerRunStart(fl)
                  : false;
              },
              flatLooksLikeIndexerRunDone: function (fl) {
                return typeof ctx.flatLooksLikeIndexerRunDone === "function"
                  ? ctx.flatLooksLikeIndexerRunDone(fl)
                  : false;
              },
              flatLooksLikeIndexerRunProgress: function (fl) {
                return typeof ctx.flatLooksLikeIndexerRunProgress === "function"
                  ? ctx.flatLooksLikeIndexerRunProgress(fl)
                  : false;
              },
              flatLooksLikeIndexerJobIngested: function (fl) {
                return typeof ctx.flatLooksLikeIndexerJobIngested === "function"
                  ? ctx.flatLooksLikeIndexerJobIngested(fl)
                  : false;
              }
            }
          );
        }
        return true;
      },
      adminProvidersSectionBreakHtml: ctx.buildAdminProvidersSectionBreakHtml,
      virtualModelsSectionBreakHtml: function (count) {
        if (typeof ctx.buildVirtualModelsSectionBreakHtml === "function") {
          return ctx.buildVirtualModelsSectionBreakHtml(count);
        }
        if (typeof ctx.buildVirtualModelsSectionIntroHtml === "function") {
          return (
            '<div class="sum-section-label sum-feed-section-title">Virtual models</div>' +
            ctx.buildVirtualModelsSectionIntroHtml(count)
          );
        }
        return '<div class="sum-section-label sum-feed-section-title">Virtual models</div>';
      },
    };
  }

  function renderSummarizedCardFromModel(card) {
    if (!card || card.kind === "section-break") return null;
    var src = card.source;
    switch (card.kind) {
      case "gateway-overview":
        return buildGatewayOverviewCardHtml();
      case "gateway-usage":
        return buildGatewayUsageCardHtml();
      case "admin-users":
        return buildAdminUsersCardHtml();
      case "admin-provider":
        return buildAdminProviderCardHtml(src.spec.id, src.spec.title, src.spec.avatar, src.spec.subtitle);
      case "virtual-model":
        return typeof buildVirtualModelCardHtml === "function" ? buildVirtualModelCardHtml(src.vm) : null;
      case "virtual-model-draft":
        return typeof buildVirtualModelDraftCardHtml === "function" ? buildVirtualModelDraftCardHtml(src.draft) : null;
      case "conversation":
        return buildConvCard(src);
      case "service":
        return buildServiceCard(src.name, src.events, src.svcCtx);
      case "indexer":
        return buildIndexerCard(src.run, src.partitionRegistry);
      case "indexer-stale":
        return buildIndexerStaleSnapshotCard(src.bucketId, src.snap);
      case "workspace-draft":
        return buildWorkspaceDraftCardHtml(src);
      case "indexer-operator-workspace":
        return buildIndexerOperatorWorkspaceCard(src.workspace, src.partitionRegistry);
      default:
        return null;
    }
  }

  function buildSummarizedModelForAgg(agg) {
    if (
      !globalThis.ChimeraSettings ||
      !ChimeraSettings.Summarized ||
      !ChimeraSettings.Summarized.Model ||
      typeof ChimeraSettings.Summarized.Model.buildSummarizedModel !== "function"
    ) {
      return null;
    }
    return ChimeraSettings.Summarized.Model.buildSummarizedModel(
      summarizedModelDeps(),
      summarizedModelState(agg)
    );
  }

  function summarizedHtmlRenderers() {
    return {
      renderCard: renderSummarizedCardFromModel,
      conversationsSectionHead: function () {
        if (typeof ctx.operatorSectionHeadHtml !== "function") {
          return (
            '<div class="sum-feed-section-head">' +
            '<span class="material-symbols-outlined sum-feed-section-icon" aria-hidden="true">forum</span>' +
            '<span class="sum-feed-section-title sum-section-label">Conversations</span></div>'
          );
        }
        return ctx.operatorSectionHeadHtml("Conversations", "forum");
      },
      workspacesSectionHead: function () {
        if (typeof ctx.operatorSectionHeadHtml !== "function") {
          return (
            '<div class="sum-feed-section-head">' +
            '<span class="sum-feed-section-title sum-section-label">Workspaces</span>' +
            buildWorkspacesCreateBtnHtml("Create") +
            "</div>"
          );
        }
        var webOnly = !workspaceDesktopFeaturesAvailable();
        return ctx.operatorSectionHeadHtml("Workspaces", "database", {
          actionHtml:
            typeof ctx.operatorSectionAddBtn === "function"
              ? ctx.operatorSectionAddBtn(
                  { "data-sum-workspaces-create": "1" },
                  "Create workspace",
                  webOnly
                    ? {
                        disabled: true,
                        title: WORKSPACE_WEB_UNAVAILABLE_TITLE,
                        desktopLocked: true
                      }
                    : undefined
                )
              : buildWorkspacesCreateBtnHtml("Create workspace"),
        });
      },
      servicesSectionHead: function () {
        if (typeof ctx.operatorSectionHeadHtml !== "function") {
          return '<div class="sum-section-label sum-feed-section-title">Services</div>';
        }
        return ctx.operatorSectionHeadHtml("Core services", "dns", { iconPrimary: true });
      },
      workspacesSectionIntro: function () {
        if (typeof ctx.buildWorkspacesSectionIntroHtml === "function") {
          return ctx.buildWorkspacesSectionIntroHtml();
        }
        return "";
      },
      buildWorkspacesCreateBtnHtml: function (label) {
        if (typeof ctx.buildWorkspacesCreateBtnHtml === "function") {
          return ctx.buildWorkspacesCreateBtnHtml(label);
        }
        return "";
      },
      emptyFeedMessage: function () {
        return (
          '<p class="muted">No conversation / service cards in the <em>loaded</em> window yet. Chat traffic needs <code>conversation_id</code> in structured logs; <strong>scroll to the top</strong> of this feed to load older lines (indexer snapshots often crowd the recent tail). Switch to <strong>StructuredLogs</strong> for the full stream.</p>'
        );
      }
    };
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
      return ChimeraSettings.Summarized.Render.renderSummarizedHtml(model, summarizedHtmlRenderers());
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
  ctx.operatorWorkspaceSyntheticBucketId = operatorWorkspaceSyntheticBucketId;

  if (
    globalThis.ChimeraSettings.Render &&
    globalThis.ChimeraSettings.Render.Cards &&
    typeof globalThis.ChimeraSettings.Render.Cards.mountAll === "function"
  ) {
    globalThis.ChimeraSettings.Render.Cards.mountAll(ctx);
  }
  if (globalThis.ChimeraSettings.Render && globalThis.ChimeraSettings.Render.Cards) {
    var FeedCards = globalThis.ChimeraSettings.Render.Cards;
    if (typeof FeedCards.mountFeedLogConv === "function") FeedCards.mountFeedLogConv(ctx);
    if (typeof FeedCards.mountFeedLogService === "function") FeedCards.mountFeedLogService(ctx);
    if (typeof FeedCards.mountFeedLogIndexerRun === "function") FeedCards.mountFeedLogIndexerRun(ctx);
    if (typeof FeedCards.mountFeedLogIndexerWorkspace === "function") FeedCards.mountFeedLogIndexerWorkspace(ctx);
  }

  var buildConvCard = ctx.buildConvCard;
  var buildServiceCard = ctx.buildServiceCard;
  var chimeraBrokerProviderHealthStripHtml = ctx.chimeraBrokerProviderHealthStripHtml;
  var buildAdminProvidersSectionBreakHtml = ctx.buildAdminProvidersSectionBreakHtml;
  var buildWorkspacesCreateBtnHtml = ctx.buildWorkspacesCreateBtnHtml;
  var buildWorkspacesSectionIntroHtml = ctx.buildWorkspacesSectionIntroHtml;
  var collectIndexerRunMeta = ctx.collectIndexerRunMeta;
  var buildIndexerCard = ctx.buildIndexerCard;
  var buildIndexerStaleSnapshotCard = ctx.buildIndexerStaleSnapshotCard;
  var buildIndexerOperatorWorkspaceCard = ctx.buildIndexerOperatorWorkspaceCard;
  var mergePersistedIndexerWatchRoots = ctx.mergePersistedIndexerWatchRoots;
  var indexerRunTimelineDedupeKey = ctx.indexerRunTimelineDedupeKey;
  var pickCanonicalIndexerRun = ctx.pickCanonicalIndexerRun;
  var indexerCardDomIdFromMeta = ctx.indexerCardDomIdFromMeta;
  var operatorWorkspaceCoveredByIndexerRuns = ctx.operatorWorkspaceCoveredByIndexerRuns;
  var findOperatorWorkspaceMatchingIndexerMeta = ctx.findOperatorWorkspaceMatchingIndexerMeta;
  var findOperatorWorkspaceByNumericId = ctx.findOperatorWorkspaceByNumericId;
  var operatorWorkspaceNumericId = ctx.operatorWorkspaceNumericId;

  ctx.wrapDesktopOnlyLockedControl = wrapDesktopOnlyLockedControl;
  var formatInt = ctx.formatInt;
  var aggregateRollupRows = ctx.aggregateRollupRows;
  var formatCompactTok = ctx.formatCompactTok;
  var formatUtcLikeLogTimestamp = ctx.formatUtcLikeLogTimestamp;
  var formatUtcToMinute = ctx.formatUtcToMinute;
  var formatUtcToDay = ctx.formatUtcToDay;
  var metricsRollupTableHtml = ctx.metricsRollupTableHtml;
  var metricsEventsTableHtml = ctx.metricsEventsTableHtml;
  var buildGatewayOverviewCardHtml = ctx.buildGatewayOverviewCardHtml;
  var buildGatewayUsageCardHtml = ctx.buildGatewayUsageCardHtml;
  var buildGatewayOverviewFeedSection = ctx.buildGatewayOverviewFeedSection;
  var buildAdminUsersCardHtml = ctx.buildAdminUsersCardHtml;
  var buildAdminProviderCardHtml = ctx.buildAdminProviderCardHtml;
  var buildVirtualModelCardHtml = ctx.buildVirtualModelCardHtml;
  var buildVirtualModelDraftCardHtml = ctx.buildVirtualModelDraftCardHtml;
  var buildWorkspaceDraftCardHtml = ctx.buildWorkspaceDraftCardHtml;
  var fallbackChainToYAML = ctx.fallbackChainToYAML;
  var parseFallbackChainInput = ctx.parseFallbackChainInput;
  var serviceAvatarClass = ctx.serviceAvatarClass;
  var serviceAvatarInitials = ctx.serviceAvatarInitials;
  var formatMergedConversationSubtitle = ctx.formatMergedConversationSubtitle;
  ctx.refreshSummarizedPanel = refreshSummarizedPanel;
  ctx.forceSummarizedFullRebuild = forceSummarizedFullRebuild;
  ctx.scheduleDeferredSummarizedRefresh = scheduleDeferredSummarizedRefresh;
  ctx.summarizedPanelInteractionBlocksRebuild = summarizedPanelInteractionBlocksRebuild;
  ctx.summarizedEvlogInteractionBlocksRebuild = summarizedPanelInteractionBlocksRebuild;
  ctx.scheduleStoryRebuild = scheduleStoryRebuild;
  ctx.markSummarizedDirtyFromEntry = markSummarizedDirtyFromEntry;
  ctx.clearSummarizedDirtySets = clearSummarizedDirtySets;
  ctx.updateSummarizedCorrelationFromEntry = updateSummarizedCorrelationFromEntry;
  ctx.scheduleSummarizedDirtyFlush = scheduleSummarizedDirtyFlush;
  ctx.beginSummarizedLiveSettle = beginSummarizedLiveSettle;
  ctx.flushSummarizedDirtyCards = flushSummarizedDirtyCards;
  ctx.buildSummarizedAggregateState = buildSummarizedAggregateState;
  ctx.buildSummarizedModelForAgg = buildSummarizedModelForAgg;
  ctx.renderSummarizedCardFromModel = renderSummarizedCardFromModel;
  ctx.renderSummarizedUnified = renderSummarizedUnified;
  ctx.replaceCardById = replaceCardById;
  ctx.patchGatewayUsageMetricsCard = patchGatewayUsageMetricsCard;
  ctx.patchGatewayOverviewCard = patchGatewayOverviewCard;
  ctx.patchAdminUsersCard = patchAdminUsersCard;
  ctx.patchAdminProviderCard = patchAdminProviderCard;
  ctx.syncSummarizedModelCache = syncSummarizedModelCache;
  ctx.removeVirtualModelFromSummarizedFeed = removeVirtualModelFromSummarizedFeed;
  ctx.refreshAdminCardAfterEditToggle = refreshAdminCardAfterEditToggle;
  ctx.patchAdminCardsFromPoll = patchAdminCardsFromPoll;
  ctx.fetchTokenLabels = fetchTokenLabels;
  ctx.fetchGatewayMetrics = fetchGatewayMetrics;
  ctx.fetchGatewayOverview = fetchGatewayOverview;
  ctx.fetchChimeraBrokerProviderSnapshot = fetchChimeraBrokerProviderSnapshot;
  ctx.fetchAdminState = fetchAdminState;
  ctx.fetchAdminTokens = fetchAdminTokens;
  ctx.syncMetricsPolling = syncMetricsPolling;
  ctx.syncUiStatePolling = syncUiStatePolling;
  ctx.syncChimeraBrokerProviderPolling = syncChimeraBrokerProviderPolling;
  ctx.adminPostJSON = adminPostJSON;
  ctx.adminPutJSON = adminPutJSON;
  ctx.fetchProviderModels = fetchProviderModels;
  ctx.providerIdsNeedingModelsPrefetch = providerIdsNeedingModelsPrefetch;
  ctx.prefetchProviderModelsAvailability = prefetchProviderModelsAvailability;
  ctx.fetchVirtualModelDetail = fetchVirtualModelDetail;
  ctx.patchVirtualModelCard = patchVirtualModelCard;
  ctx.adminSetMessage = adminSetMessage;
  ctx.parseFallbackChainInput = parseFallbackChainInput;
  ctx.fallbackChainToYAML = fallbackChainToYAML;
  ctx.beginWorkspaceManagedEdit = beginWorkspaceManagedEdit;
  ctx.cancelWorkspaceManagedEdit = cancelWorkspaceManagedEdit;
  ctx.refreshWorkspaceManagedPaths = refreshWorkspaceManagedPaths;
  ctx.saveManagedWorkspacePaths = saveManagedWorkspacePaths;
  ctx.deleteManagedWorkspace = deleteManagedWorkspace;
  ctx.markUiUnauthorized = markUiUnauthorized;
  ctx.stopSummarizedPolling = stopSummarizedPolling;
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

  ctx.ensureAdminProviderCatalog = ensureAdminProviderCatalog;
  ctx.closeAdminProviderPicker = closeAdminProviderPicker;
  ctx.setAdminProviderPickerOpen = setAdminProviderPickerOpen;
  ensureAdminProviderCatalog();
};

