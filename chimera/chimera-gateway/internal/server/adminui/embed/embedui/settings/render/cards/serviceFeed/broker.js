/**
 * chimera-broker service card: provider health, relay outcomes, collapsed metrics.
 */
globalThis.ChimeraSettings = globalThis.ChimeraSettings || {};
globalThis.ChimeraSettings.Render = globalThis.ChimeraSettings.Render || {};
globalThis.ChimeraSettings.Render.Cards = globalThis.ChimeraSettings.Render.Cards || {};
globalThis.ChimeraSettings.Render.Cards.ServiceFeed = globalThis.ChimeraSettings.Render.Cards.ServiceFeed || {};

globalThis.ChimeraSettings.Render.Cards.ServiceFeed.mountBroker = function (deps) {
  var ctx = deps.ctx;
  var escapeHtml = deps.escapeHtml;
  var getFlat = deps.getFlat;
  var formatInt = deps.formatInt;
  var RECENT_CARD_STATUS_N = deps.RECENT_CARD_STATUS_N;
  var CHIMERA_BROKER_PROVIDER_STALE_MS = deps.CHIMERA_BROKER_PROVIDER_STALE_MS;

  function entryIsGatewayUpstreamRelay(ent) {
    var D = globalThis.ChimeraSettings && ChimeraSettings.Derive ? ChimeraSettings.Derive : null;
    if (D && typeof D.entryIsGatewayUpstreamRelay === "function") {
      return D.entryIsGatewayUpstreamRelay(ent, getFlat);
    }
    return false;
  }

  function chimeraBrokerProviderHealthResolve(arr) {
    var stateLabel = {
      up: "reachable",
      down: "offline",
      key_missing: "key missing",
      unknown: "configured",
      not_configured: "not configured"
    };
    var list = null;
    var liveErr = "";
    if (ctx.chimeraBrokerProviderSnapshot && ctx.chimeraBrokerProviderSnapshot.data && Array.isArray(ctx.chimeraBrokerProviderSnapshot.data.providers)) {
      var snapshotAgeMs = Date.now() - Number(ctx.chimeraBrokerProviderSnapshot.fetchedClientMs || 0);
      if (snapshotAgeMs <= CHIMERA_BROKER_PROVIDER_STALE_MS) {
        list = ctx.chimeraBrokerProviderSnapshot.data.providers.slice();
        liveErr = String(ctx.chimeraBrokerProviderSnapshot.data.error || "").trim();
      }
    }
    if (!list && globalThis.ChimeraSettings && ChimeraSettings.Derive && typeof ChimeraSettings.Derive.chimeraBrokerProviderHealthList === "function") {
      list = ChimeraSettings.Derive.chimeraBrokerProviderHealthList(arr, function (p) { return getFlat(p); });
    }
    if (list && list.length) {
      list = list.filter(function (ent) {
        return String((ent || {}).state || "").toLowerCase() !== "not_configured";
      });
    }
    return {
      list: list,
      liveErr: liveErr,
      emptyMsg: liveErr ? "chimera-broker unreachable" : "No providers loaded yet",
      stateLabel: stateLabel
    };
  }

  function chimeraBrokerProviderHealthSegTitle(entry, lab) {
    var titleBits = [String((entry || {}).id || "—") + " · " + lab];
    var keyHint = entry.key_hint != null ? String(entry.key_hint) : "";
    var keyCount = entry.key_count != null && !isNaN(Number(entry.key_count)) ? Number(entry.key_count) : null;
    if (keyCount != null) titleBits.push(keyCount + (keyCount === 1 ? " key" : " keys"));
    if (keyHint) titleBits.push(keyHint);
    if (entry.ollama_base_url) titleBits.push("base " + entry.ollama_base_url);
    if (entry.error) titleBits.push("err: " + entry.error);
    return titleBits.join(" · ");
  }

  function chimeraBrokerProviderHealthStripHtml(arr, opts) {
    opts = opts || {};
    var compact = !!opts.compact;
    var R = chimeraBrokerProviderHealthResolve(arr);
    var list = R.list;
    var stateLabel = R.stateLabel;
    var healthSeg =
      globalThis.ChimeraUI && typeof globalThis.ChimeraUI.healthSegSpan === "function"
        ? globalThis.ChimeraUI.healthSegSpan
        : function (title, tone) {
            var t = tone === "up" || tone === "down" || tone === "key_missing" ? tone : "unknown";
            return (
              '<span class="sum-bf-prov-health-seg sum-bf-prov-health-seg--' +
              t +
              '" title="' +
              escapeHtml(title) +
              '"></span>'
            );
          };

    if (compact) {
      var trackTitle = R.emptyMsg;
      var segs = [];
      if (list && list.length) {
        var cap = list.length > 3 ? 3 : list.length;
        trackTitle =
          list.length > 3
            ? "Provider probe status (first " + cap + " of " + list.length + ")"
            : "Provider probe status";
        for (var ci = 0; ci < cap; ci++) {
          var entC = list[ci] || {};
          var stC = entC.state && stateLabel[entC.state] != null ? entC.state : "unknown";
          var labC = stateLabel[stC];
          segs.push(healthSeg(chimeraBrokerProviderHealthSegTitle(entC, labC), stC));
        }
      } else {
        for (var zi = 0; zi < 3; zi++) {
          segs.push(healthSeg(R.emptyMsg, "unknown"));
        }
      }
      return (
        '<span id="chimera-broker-provider-health-compact" class="sum-bf-prov-health-root sum-bf-prov-health-root--compact" role="img" aria-label="' +
        escapeHtml(trackTitle) +
        '">' +
        '<span class="sum-bf-prov-health-track sum-bf-prov-health-track--compact" title="' +
        escapeHtml(trackTitle) +
        '">' +
        segs.join("") +
        "</span></span>"
      );
    }

    var rootOpen = '<div id="chimera-broker-provider-health-strip" class="sum-bf-prov-health-root">';
    if (!list || !list.length) {
      return (
        rootOpen +
        '<div class="sum-bf-prov-health-track sum-bf-prov-health-track--empty" title="' +
        escapeHtml(R.emptyMsg) +
        '">' +
        healthSeg(R.emptyMsg, "unknown", "sum-bf-prov-health-seg--empty") +
        "</div>" +
        '<div class="sum-strip-caption sum-strip-caption--muted">' +
        escapeHtml(R.emptyMsg) +
        "</div></div>"
      );
    }
    var trackParts = [];
    var labelParts = [];
    for (var i = 0; i < list.length; i++) {
      var entry = list[i] || {};
      var st = entry.state && stateLabel[entry.state] != null ? entry.state : "unknown";
      var lab = stateLabel[st];
      trackParts.push(healthSeg(chimeraBrokerProviderHealthSegTitle(entry, lab), st));
      labelParts.push(
        '<span class="sum-bf-prov-health-label sum-bf-prov-health-label--' +
          escapeHtml(st) +
          '" title="' +
          escapeHtml(chimeraBrokerProviderHealthSegTitle(entry, lab)) +
          '">' +
          escapeHtml(String(entry.id || "—")) +
          " · " +
          escapeHtml(lab) +
          "</span>"
      );
    }
    return (
      rootOpen +
      '<div class="sum-bf-prov-health-track" title="One segment per configured provider, colored by latest health probe">' +
      trackParts.join("") +
      '</div><div class="sum-bf-prov-health-labels">' +
      labelParts.join("") +
      "</div></div>"
    );
  }

  function chimeraBrokerRelayOutcomeStripHtml(arr) {
    var b = null;
    if (
      globalThis.ChimeraSettings &&
      ChimeraSettings.Derive &&
      typeof ChimeraSettings.Derive.chimeraBrokerRelayOutcomeBuckets === "function"
    ) {
      b = ChimeraSettings.Derive.chimeraBrokerRelayOutcomeBuckets(arr, function (p) { return getFlat(p); });
    }
    if (!b || !b.total) {
      return (
        '<div class="sum-timeline-bar sum-timeline-bar--relay-outcome"></div>' +
        '<div class="sum-strip-caption sum-strip-caption--muted">No chat relay activity yet</div>'
      );
    }
    var palette = [
      { key: "ok", label: "2xx", color: "#66bb6a" },
      { key: "redirect", label: "3xx", color: "#42a5f5" },
      { key: "rateLimit", label: "429", color: "#fb8c00" },
      { key: "clientErr", label: "4xx", color: "#ffa726" },
      { key: "serverErr", label: "5xx", color: "#ef5350" },
      { key: "errorNoResp", label: "fetch err", color: "#c62828" },
      { key: "inFlight", label: "in flight", color: "#9575cd" }
    ];
    var html = '<div class="sum-timeline-bar sum-timeline-bar--relay-outcome" title="Chat relay outcomes since last chimera-broker ready (HTTP buckets + fetch errors + in-flight)">';
    var captionParts = [];
    for (var pi = 0; pi < palette.length; pi++) {
      var p = palette[pi];
      var n = Number(b[p.key] || 0);
      if (n <= 0) continue;
      var pct = (n / b.total) * 100;
      if (pct < 0.05) pct = 0.05;
      html +=
        '<span class="sum-timeline-seg" title="' +
        escapeHtml(p.label + " · " + n) +
        '" style="width:' +
        pct.toFixed(2) +
        "%;background:" +
        p.color +
        '"></span>';
      captionParts.push(
        formatInt(n) +
        ' <span class="sum-strip-caption-state sum-strip-caption-state--' +
        p.key +
        '">' +
        escapeHtml(p.label) +
        "</span>"
      );
    }
    html += "</div>";
    html += '<div class="sum-strip-caption">' + captionParts.join(" · ") + "</div>";
    return html;
  }

  function chimeraBrokerShortModelLabel(model) {
    if (!model || model === "—") return "—";
    var parts = String(model).split("/");
    var tail = parts[parts.length - 1] || model;
    if (tail.length > 36) return tail.slice(0, 34) + "…";
    return tail;
  }

  function chimeraBrokerProviderSnapshotDataForUi() {
    if (!ctx.chimeraBrokerProviderSnapshot || !ctx.chimeraBrokerProviderSnapshot.data) return null;
    var staleMs =
      typeof ctx.CHIMERA_BROKER_PROVIDER_STALE_MS === "number"
        ? ctx.CHIMERA_BROKER_PROVIDER_STALE_MS
        : 120000;
    var snapshotAgeMs = Date.now() - Number(ctx.chimeraBrokerProviderSnapshot.fetchedClientMs || 0);
    if (snapshotAgeMs > staleMs) return null;
    return ctx.chimeraBrokerProviderSnapshot.data;
  }

  function chimeraBrokerCardMetrics(arr) {
    if (
      globalThis.ChimeraSettings &&
      ChimeraSettings.Derive &&
      typeof ChimeraSettings.Derive.chimeraBrokerCardMetrics === "function"
    ) {
      return ChimeraSettings.Derive.chimeraBrokerCardMetrics(arr, function (p) {
        return getFlat(p);
      });
    }
    return {
      catalogModelCount: 0,
      relayOk: 0,
      relayFail: 0,
      outgoingSum: 0,
      usageSum: 0
    };
  }

  function chimeraBrokerAvailableModelCountResolve(arr) {
    var snap = chimeraBrokerProviderSnapshotDataForUi();
    if (
      globalThis.ChimeraSettings &&
      ChimeraSettings.Derive &&
      typeof ChimeraSettings.Derive.chimeraBrokerAvailableModelCount === "function"
    ) {
      return ChimeraSettings.Derive.chimeraBrokerAvailableModelCount(arr, function (p) {
        return getFlat(p);
      }, snap);
    }
    var bx = chimeraBrokerCardMetrics(arr);
    return bx.catalogModelCount != null && bx.catalogModelCount > 0 ? bx.catalogModelCount : 0;
  }

  function chimeraBrokerAvailableModelCountLabel(arr) {
    var n = chimeraBrokerAvailableModelCountResolve(arr);
    return n > 0 ? formatInt(n) : "—";
  }

  function chimeraBrokerCollapsedCardSubtitle(arr) {
    if (
      globalThis.ChimeraSettings &&
      ChimeraSettings.Derive &&
      typeof ChimeraSettings.Derive.chimeraBrokerCollapsedHealthSubtitle === "function"
    ) {
      return ChimeraSettings.Derive.chimeraBrokerCollapsedHealthSubtitle(arr, function (p) {
        return getFlat(p);
      });
    }
    return "";
  }

  function chimeraBrokerRelayOkFailTooltip(metrics) {
    var m = metrics || {};
    var detail =
      (m.relay429N || 0) > 0
        ? formatInt(m.relayOk) +
          " ok · " +
          formatInt(m.relayFail) +
          " fail · " +
          formatInt(m.relay429N) +
          " rate-limited (429)"
        : formatInt(m.relayOk) + " ok · " + formatInt(m.relayFail) + " fail";
    return (
      "Chat relay outcomes since last chimera-broker ready: successful upstream responses (check) vs errors (error). " +
      detail +
      "."
    );
  }

  function chimeraBrokerServicePanelKvHtml(arr) {
    var M = {
      version: "—",
      configuration: "—",
      port: "—",
      auth: "—",
      mcp: "—",
      governance: "—",
      backendName: "",
      backendMode: ""
    };
    if (globalThis.ChimeraSettings && ChimeraSettings.Derive && typeof ChimeraSettings.Derive.chimeraBrokerCardModel === "function") {
      var d = ChimeraSettings.Derive.chimeraBrokerCardModel(arr, function (p) { return getFlat(p); });
      if (d.version) M.version = d.version;
      if (d.configuration) M.configuration = d.configuration;
      if (d.port) M.port = d.port;
      if (d.auth) M.auth = d.auth;
      if (d.mcp) M.mcp = d.mcp;
      if (d.governance) M.governance = d.governance;
      if (d.backendName) M.backendName = d.backendName;
      if (d.backendMode) M.backendMode = d.backendMode;
    }
    var brokerBackendLab = "—";
    if (
      globalThis.ChimeraSettings &&
      ChimeraSettings.Derive &&
      typeof ChimeraSettings.Derive.wrapperBackendPanelLabel === "function"
    ) {
      brokerBackendLab = ChimeraSettings.Derive.wrapperBackendPanelLabel(M.backendName, M.backendMode);
    }
    return (
      '<dl class="indexer-run-kv indexer-run-kv--chimera-broker-summary">' +
      "<dt>component</dt><dd>chimera-broker</dd>" +
      "<dt>backend</dt><dd>" +
      escapeHtml(brokerBackendLab) +
      "</dd>" +
      "<dt>version</dt><dd>" +
      escapeHtml(M.version) +
      '</dd><dt>configuration</dt><dd>' +
      escapeHtml(M.configuration) +
      '</dd><dt>port</dt><dd>' +
      escapeHtml(M.port) +
      '</dd><dt>auth</dt><dd>' +
      escapeHtml(M.auth) +
      '</dd><dt>MCP</dt><dd>' +
      escapeHtml(M.mcp) +
      '</dd><dt>governance</dt><dd>' +
      escapeHtml(M.governance) +
      "</dd></dl>"
    );
  }

  function buildBrokerCardIntroHtml() {
    return (
      '<div class="bf-card-intro" id="bf-card-intro">' +
      '<p class="bf-card-intro-lead">' +
      "A fast health and traffic summary for the chimera-broker relay path into models. Odd patterns here usually mean throttling, misconfiguration, or an upstream hiccup—not necessarily that chat is already broken." +
      "</p>" +
      "</div>"
    );
  }

  function expandedMiniHtml(arr) {
    var bx = chimeraBrokerCardMetrics(arr);
    var kvB = chimeraBrokerServicePanelKvHtml(arr);
    var availModelsStr = chimeraBrokerAvailableModelCountLabel(arr);
    var providerHealthStrip = chimeraBrokerProviderHealthStripHtml(arr);
    var relayOutcomeStrip = chimeraBrokerRelayOutcomeStripHtml(arr);
    return (
      buildBrokerCardIntroHtml() +
      '<div class="sum-section-label">Provider health</div>' +
      providerHealthStrip +
      kvB +
      '<div class="sum-mini-row sum-mini-row--chimera-broker-deck">' +
      '<div class="sum-mini-card">Available models<strong id="chimera-broker-available-models-count">' +
      escapeHtml(availModelsStr) +
      '</strong><span class="sum-mini-sub">Live catalog from chimera-broker /v1/models (refreshed with provider health polls); falls back to log sync lines when stale</span></div>' +
      "</div>" +
      '<div class="sum-section-label">Relay outcomes</div>' +
      relayOutcomeStrip +
      '<div class="sum-mini-row sum-mini-row--chimera-broker-deck2">' +
      '<div class="sum-mini-card">Relay (ok / fail)<strong>' +
      escapeHtml(formatInt(bx.relayOk) + " / " + formatInt(bx.relayFail)) +
      '</strong><span class="sum-mini-sub">Successful upstream responses vs errors (gateway relay)</span></div>' +
      "</div>"
    );
  }

  var impl = {
    id: "chimera-broker",
    skipTimeline: true,
    skipStatusPill: true,
    fullLogClassExtra: " sum-full-log--chimera-broker",
    deriveCollapsed: function (arr) {
      return { subtitle: chimeraBrokerCollapsedCardSubtitle(arr) };
    },
    collapsedMetricsHtml: function (arr) {
      var bxC = chimeraBrokerCardMetrics(arr);
      return (
        '<span class="sum-metrics">' +
        ctx.sgOpInsetWellOkFailHtml(bxC.relayOk, bxC.relayFail, "", {
          title: chimeraBrokerRelayOkFailTooltip(bxC)
        }) +
        chimeraBrokerProviderHealthStripHtml(arr, { compact: true }) +
        "</span>"
      );
    },
    expandedMiniHtml: expandedMiniHtml,
    recentHasError: function (arr) {
      var hasErr = ctx.entryHasErrorStatus;
      var hasRl = ctx.chimeraBrokerEntryHasRateLimit;
      var slice =
        typeof ctx.sliceRecent === "function" ? ctx.sliceRecent(arr, RECENT_CARD_STATUS_N) : [];
      for (var i = 0; i < slice.length; i++) {
        if (typeof hasErr === "function" && hasErr(slice[i])) return true;
        if (typeof hasRl === "function" && hasRl(slice[i])) return true;
      }
      return false;
    },
    badgeForPanel: function (ev) {
      var w = { parsed: ev.parsed, text: ev.text, ts: ev.ts, source: ev.source };
      if (entryIsGatewayUpstreamRelay(w)) {
        return {
          cls: "sum-svc-broker sum-svc-badge-filled sum-svc-broker-filled",
          key: "chimera-broker",
          lab: typeof ctx.serviceDisplayLabel === "function" ? ctx.serviceDisplayLabel("chimera-broker") : "chimera-broker"
        };
      }
      return null;
    }
  };

  return {
    impl: impl,
    exports: {
      chimeraBrokerProviderHealthStripHtml: chimeraBrokerProviderHealthStripHtml,
      chimeraBrokerRelayOutcomeStripHtml: chimeraBrokerRelayOutcomeStripHtml,
      chimeraBrokerShortModelLabel: chimeraBrokerShortModelLabel,
      chimeraBrokerAvailableModelCountLabel: chimeraBrokerAvailableModelCountLabel,
      chimeraBrokerAvailableModelCountResolve: chimeraBrokerAvailableModelCountResolve,
      chimeraBrokerCollapsedCardSubtitle: chimeraBrokerCollapsedCardSubtitle,
      chimeraBrokerRelayOkFailTooltip: chimeraBrokerRelayOkFailTooltip,
      chimeraBrokerServicePanelKvHtml: chimeraBrokerServicePanelKvHtml,
      buildBrokerCardIntroHtml: buildBrokerCardIntroHtml
    }
  };
};
