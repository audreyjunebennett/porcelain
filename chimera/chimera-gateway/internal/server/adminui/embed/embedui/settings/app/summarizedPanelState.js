/**
 * Summarized panel DOM preservation: scroll, open cards, evlog state, YAML editors.
 * Exports: ChimeraSettings.Summarized.mountPanelState(bridge)
 */
globalThis.ChimeraSettings = globalThis.ChimeraSettings || {};
globalThis.ChimeraSettings.Summarized = globalThis.ChimeraSettings.Summarized || {};

globalThis.ChimeraSettings.Summarized.mountPanelState = function (bridge) {
  var ctx = bridge.ctx;
  var stickPx = bridge.stickPx;

  var ADMIN_CARD_TABLE_SCROLL_SEL =
    ".sum-metrics-table-wrap, .sg-op-routing-table-scroll, .sg-op-fallback-table-scroll, .sg-op-router-table-scroll";
  var SUMMARIZED_CARD_SCROLL_SEL =
    ADMIN_CARD_TABLE_SCROLL_SEL + ", .sum-full-log--evlog .sum-evlog-table-wrap";

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
    } catch (_eOpen) {}

    var storyScroll = {};
    try {
      var sps = psu.querySelectorAll(".story-panel");
      for (var sj = 0; sj < sps.length; sj++) {
        var cStory = sps[sj].closest("details[id]");
        if (cStory && cStory.id) storyScroll[cStory.id] = sps[sj].scrollTop;
      }
    } catch (_eStory) {}

    var fullLogScroll = {};
    try {
      var fls = psu.querySelectorAll(".sum-full-log[id]");
      for (var fk = 0; fk < fls.length; fk++) {
        if (fls[fk] && fls[fk].id) fullLogScroll[fls[fk].id] = fls[fk].scrollTop;
      }
    } catch (_eFl) {}

    var panelUiSave = captureSummarizedPanelUiState(psu);

    destroySummarizedYamlEditors(psu);
    psu.innerHTML = bridge.renderSummarizedHtmlFromModel(nextModel);
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
      } catch (_eEv) {}
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
    } catch (_eDet) {}
    wireCollapsibleSummarizedPanel(psu);
    mountSummarizedYamlEditors(psu);

    restoreSummarizedPanelUiState(psu, panelUiSave, { scroll: false });

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

    if (nearPanelBottom) {
      psu.scrollTop = psu.scrollHeight;
    } else {
      var maxS = Math.max(0, psu.scrollHeight - psu.clientHeight);
      psu.scrollTop = Math.min(prevScrollTop, maxS);
    }
    restoreSummarizedNestedScrolls();

    window.requestAnimationFrame(function () {
      restoreSummarizedNestedScrolls();
      if (nearPanelBottom) {
        psu.scrollTop = psu.scrollHeight;
      } else if (prevScrollH > 0) {
        var dh = psu.scrollHeight - prevScrollH;
        psu.scrollTop = Math.max(0, prevScrollTop + dh);
      }
      restoreSummarizedPanelUiState(psu, panelUiSave, { scrollOnly: true });
    });
  }

  return {
    ADMIN_CARD_TABLE_SCROLL_SEL: ADMIN_CARD_TABLE_SCROLL_SEL,
    SUMMARIZED_CARD_SCROLL_SEL: SUMMARIZED_CARD_SCROLL_SEL,
    isSummarizedCardOpen: isSummarizedCardOpen,
    setSummarizedCardOpen: setSummarizedCardOpen,
    wireCollapsibleSummarizedPanel: wireCollapsibleSummarizedPanel,
    captureNestedScrollMap: captureNestedScrollMap,
    restoreNestedScrollMap: restoreNestedScrollMap,
    captureSummarizedPanelUiState: captureSummarizedPanelUiState,
    restoreSummarizedPanelUiState: restoreSummarizedPanelUiState,
    destroySummarizedYamlEditors: destroySummarizedYamlEditors,
    mountSummarizedYamlEditors: mountSummarizedYamlEditors,
    applySummarizedFullPanelRebuild: applySummarizedFullPanelRebuild
  };
};
