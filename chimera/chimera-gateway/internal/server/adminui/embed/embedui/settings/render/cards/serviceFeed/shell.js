/**
 * Generic service card shell: registry dispatch, collapsed summary, expanded evlog body.
 */
globalThis.ChimeraSettings = globalThis.ChimeraSettings || {};
globalThis.ChimeraSettings.Render = globalThis.ChimeraSettings.Render || {};
globalThis.ChimeraSettings.Render.Cards = globalThis.ChimeraSettings.Render.Cards || {};
globalThis.ChimeraSettings.Render.Cards.ServiceFeed = globalThis.ChimeraSettings.Render.Cards.ServiceFeed || {};

globalThis.ChimeraSettings.Render.Cards.ServiceFeed.mountShell = function (deps, registry) {
  var ctx = deps.ctx;
  var escapeHtml = deps.escapeHtml;
  var getFlat = deps.getFlat;
  var strHash = deps.strHash;
  var entryInstant = deps.entryInstant;
  var sumEvlogPanelHtml = deps.sumEvlogPanelHtml;
  var sumEvlogBuildTbodyFromServiceEntries = deps.sumEvlogBuildTbodyFromServiceEntries;
  var sumEvlogCountWarnFailFromEntries = deps.sumEvlogCountWarnFailFromEntries;
  var scopedEvlogTitle = deps.scopedEvlogTitle;
  var RECENT_CARD_STATUS_N = deps.RECENT_CARD_STATUS_N;
  var operatorCardChevronHtml = deps.operatorCardChevronHtml;

  function serviceWindowMs(arr) {
    var t0 = null;
    var t1 = null;
    for (var i = 0; i < arr.length; i++) {
      var ins = entryInstant(arr[i]);
      if (ins) {
        if (!t0 || ins.getTime() < t0.getTime()) t0 = ins;
        if (!t1 || ins.getTime() > t1.getTime()) t1 = ins;
      }
    }
    return t0 && t1 ? t1.getTime() - t0.getTime() : 0;
  }

  function entryIsGatewayUpstreamRelay(ent) {
    var D = globalThis.ChimeraSettings && ChimeraSettings.Derive ? ChimeraSettings.Derive : null;
    if (D && typeof D.entryIsGatewayUpstreamRelay === "function") {
      return D.entryIsGatewayUpstreamRelay(ent, getFlat);
    }
    return false;
  }

  function entryRoutesToChimeraBrokerBucket(ent) {
    var D = globalThis.ChimeraSettings && ChimeraSettings.Derive ? ChimeraSettings.Derive : null;
    if (D && typeof D.entryRoutesToChimeraBrokerBucket === "function") {
      return D.entryRoutesToChimeraBrokerBucket(ent, getFlat);
    }
    return false;
  }

  function recentServiceCardHasError(name, arr) {
    var impl = registry.get(name);
    if (impl && typeof impl.recentHasError === "function") {
      return impl.recentHasError(arr);
    }
    var hasErr = ctx.entryHasErrorStatus;
    var slice =
      typeof ctx.sliceRecent === "function" ? ctx.sliceRecent(arr, RECENT_CARD_STATUS_N) : [];
    for (var i = 0; i < slice.length; i++) {
      if (typeof hasErr === "function" && hasErr(slice[i])) return true;
    }
    return false;
  }

  function badgeForServicePanel(name, ev) {
    var impl = registry.get(name);
    if (impl && typeof impl.badgeForPanel === "function") {
      return impl.badgeForPanel(ev);
    }
    return ctx.inferServiceBadge(ev);
  }

  function resolveCardStatus(name, arr, derived) {
    derived = derived || {};
    if (recentServiceCardHasError(name, arr)) {
      return { st: "error", cls: "sum-st-error" };
    }
    if (derived.status) return derived.status;
    if (derived.ixWaitFlat) return { st: "idle", cls: "sum-st-monitor" };
    return { st: "active", cls: "sum-st-active" };
  }

  function evlogOptionsFor(name, arr) {
    var impl = registry.get(name);
    var opts =
      impl && typeof impl.evlogOptions === "function"
        ? impl.evlogOptions()
        : {};
    return {
      filterGatewayProbe: !!opts.filterGatewayProbe,
      showSourceColumn: !!opts.showSourceColumn,
      sourceColumnKind: opts.sourceColumnKind || "",
      sumEvlogVisibleGateway: !!opts.sumEvlogVisibleGateway
    };
  }

  function renderExpandedService(name, arr, svcCtx) {
    svcCtx = svcCtx || {};
    var impl = registry.get(name);
    var timelineBlock = "";
    if (!(impl && impl.skipTimeline) && typeof impl.expandedTimelineHtml === "function") {
      timelineBlock = impl.expandedTimelineHtml(arr);
    }
    var prefixHtml =
      impl && typeof impl.expandedPrefixHtml === "function" ? impl.expandedPrefixHtml(svcCtx) : "";
    var mini =
      impl && typeof impl.expandedMiniHtml === "function" ? impl.expandedMiniHtml(arr, svcCtx) : "";
    var evOpts = evlogOptionsFor(name, arr);
    var fullLogExtra = impl && impl.fullLogClassExtra ? impl.fullLogClassExtra : "";
    var fullLogClass = "sum-full-log sum-full-log--evlog" + fullLogExtra;
    var scrollTbodyId = "svc-log-" + strHash(name);
    var cardScope = strHash("svc:" + name);
    var visEnt =
      typeof ctx.sumEvlogVisibleEntriesForService === "function"
        ? ctx.sumEvlogVisibleEntriesForService(name, arr, evOpts.sumEvlogVisibleGateway)
        : arr;
    var mc = sumEvlogCountWarnFailFromEntries(visEnt);
    var evlogBuildOpts = {
      cardScope: cardScope,
      filterGatewayProbe: evOpts.filterGatewayProbe
    };
    if (evOpts.showSourceColumn) evlogBuildOpts.showSourceColumn = true;
    var tbodyInner = sumEvlogBuildTbodyFromServiceEntries(name, arr, evlogBuildOpts);
    var evlogPanelOpts = {
      scrollTbodyId: scrollTbodyId,
      showSourceColumn: evOpts.showSourceColumn,
      warnN: mc.warn,
      failN: mc.fail,
      tbodyInnerHtml: tbodyInner,
      title: typeof scopedEvlogTitle === "function" ? scopedEvlogTitle(ctx.serviceDisplayLabel(name)) : "Scoped log"
    };
    if (evOpts.sourceColumnKind) evlogPanelOpts.sourceColumnKind = evOpts.sourceColumnKind;
    var full =
      '<div class="' + fullLogClass + '">' +
      sumEvlogPanelHtml(evlogPanelOpts) +
      "</div>";
    return (
      '<div class="sum-body">' +
      timelineBlock +
      '<div class="sum-section-label">Summary</div>' +
      prefixHtml +
      mini +
      full +
      "</div>"
    );
  }

  function buildServiceCard(name, arr, svcCtx) {
    svcCtx = svcCtx || {};
    var impl = registry.get(name);
    var derived =
      impl && typeof impl.deriveCollapsed === "function"
        ? impl.deriveCollapsed(arr, svcCtx)
        : {};
    var lastMsg = derived.subtitle != null ? String(derived.subtitle) : "";
    var st = resolveCardStatus(name, arr, derived);
    var sid = "svc-" + strHash(name);
    var ini = ctx.serviceAvatarInitials(name);
    var av = ctx.serviceAvatarClass(name);
    var titleClass = "sum-title";
    var displayServiceName = ctx.serviceDisplayLabel(name);
    var titleBlock = escapeHtml(displayServiceName);
    var metrics =
      impl && typeof impl.collapsedMetricsHtml === "function"
        ? impl.collapsedMetricsHtml(arr, derived)
        : "";
    var statusHtml =
      (impl && impl.skipStatusPill) || typeof ctx.serviceSummaryStatusPillHtml !== "function"
        ? ""
        : ctx.serviceSummaryStatusPillHtml(st);
    metrics =
      typeof ctx.summaryMetricsHtml === "function"
        ? ctx.summaryMetricsHtml(metrics, statusHtml)
        : metrics;
    return (
      '<details class="sum-card" id="' +
      escapeHtml(sid) +
      '"><summary>' +
      '<span class="sum-avatar ' +
      av +
      '">' +
      escapeHtml(ini) +
      "</span>" +
      '<span class="sum-main"><span class="' +
      titleClass +
      '">' +
      titleBlock +
      '</span><span class="sum-sub sum-sub--clamp">' +
      escapeHtml(lastMsg) +
      "</span></span>" +
      metrics +
      (typeof operatorCardChevronHtml === "function" ? operatorCardChevronHtml() : "") +
      "</summary>" +
      renderExpandedService(name, arr, svcCtx) +
      "</details>"
    );
  }

  function summarizedServicesSectionHead() {
    if (typeof ctx.operatorSectionHeadHtml !== "function") {
      return '<div class="sum-section-label sum-feed-section-title">Services</div>';
    }
    return ctx.operatorSectionHeadHtml("Core services", "dns", { iconPrimary: true });
  }

  return {
    buildServiceCard: buildServiceCard,
    renderExpandedService: renderExpandedService,
    recentServiceCardHasError: recentServiceCardHasError,
    serviceWindowMs: serviceWindowMs,
    badgeForServicePanel: badgeForServicePanel,
    entryIsGatewayUpstreamRelay: entryIsGatewayUpstreamRelay,
    entryRoutesToChimeraBrokerBucket: entryRoutesToChimeraBrokerBucket,
    summarizedServicesSectionHead: summarizedServicesSectionHead
  };
};
