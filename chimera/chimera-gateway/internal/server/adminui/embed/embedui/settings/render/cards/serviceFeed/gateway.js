/**
 * chimera-gateway service card: listening/upstream KV and HTTP ok/fail metrics.
 */
globalThis.ChimeraSettings = globalThis.ChimeraSettings || {};
globalThis.ChimeraSettings.Render = globalThis.ChimeraSettings.Render || {};
globalThis.ChimeraSettings.Render.Cards = globalThis.ChimeraSettings.Render.Cards || {};
globalThis.ChimeraSettings.Render.Cards.ServiceFeed = globalThis.ChimeraSettings.Render.Cards.ServiceFeed || {};

globalThis.ChimeraSettings.Render.Cards.ServiceFeed.mountGateway = function (deps) {
  var ctx = deps.ctx;
  var escapeHtml = deps.escapeHtml;
  var getFlat = deps.getFlat;
  var formatInt = deps.formatInt;
  var primaryLogMessage = deps.primaryLogMessage;

  function gatewayCardModel(arr) {
    if (
      globalThis.ChimeraSettings &&
      ChimeraSettings.Derive &&
      typeof ChimeraSettings.Derive.gatewayCardModel === "function"
    ) {
      return ChimeraSettings.Derive.gatewayCardModel(arr, getFlat);
    }
    return null;
  }

  function gatewayHttpOkFailTooltip(counters) {
    var c = counters || {};
    var detail =
      (c.http429 || 0) > 0
        ? formatInt(c.http2xx) +
          " ok · " +
          formatInt(c.httpNot2xx) +
          " fail · " +
          formatInt(c.http429) +
          " rate-limited (429)"
        : formatInt(c.http2xx) + " ok · " + formatInt(c.httpNot2xx) + " fail";
    return (
      "HTTP responses in the current log view: 2xx successes (check) vs non-2xx failures (error). " + detail + "."
    );
  }

  function gatewayServicePanelMiniHtml(arr) {
    var M = {
      kv: {
        listening: "—",
        upstream: "—",
        config: "—",
        apiKeys: "—",
        apiKeysTint: "none",
        routingRules: "—",
        supervised: "—"
      }
    };
    var gw = gatewayCardModel(arr);
    if (gw) M = gw;
    var kv = M.kv || {};
    var apiKeysOpen =
      kv.apiKeysTint === "error"
        ? '<dd class="gateway-kv-dd gateway-kv-dd--error">'
        : "<dd>";
    return (
      '<dl class="indexer-run-kv indexer-run-kv--gateway-summary">' +
      "<dt>listening</dt><dd>" +
      escapeHtml(kv.listening || "—") +
      '</dd><dt>chimera-broker</dt><dd>' +
      escapeHtml(kv.broker || "—") +
      '</dd><dt>config</dt><dd>' +
      escapeHtml(kv.config || "—") +
      "</dd><dt>API keys</dt>" +
      apiKeysOpen +
      escapeHtml(kv.apiKeys || "—") +
      '</dd><dt>routing rules</dt><dd>' +
      escapeHtml(kv.routingRules || "—") +
      '</dd><dt>supervised</dt><dd>' +
      escapeHtml(kv.supervised || "—") +
      "</dd></dl>"
    );
  }

  function buildGatewayCardIntroHtml() {
    return (
      '<div class="gw-svc-card-intro" id="gw-svc-card-intro">' +
      '<p class="gw-svc-card-intro-lead">' +
      "An at-a-glance snapshot of this gateway instance—how it listens, where it connects, and which supervised helpers started. HTTP ok/fail counts in the card header reflect gateway lines in the current view; per-line level and HTTP status appear in the event log. For richer token rollups and upstream trails, open Gateway usage (Stats)." +
      "</p>" +
      "</div>"
    );
  }

  var impl = {
    id: "chimera-gateway",
    skipTimeline: true,
    deriveCollapsed: function (arr) {
      var gwCardModel = gatewayCardModel(arr);
      var lastMsg = "";
      var last = arr.length ? arr[arr.length - 1] : null;
      if (last) lastMsg = primaryLogMessage(last.parsed, last.text);
      if (gwCardModel && gwCardModel.subtitle && gwCardModel.subtitle !== "—") {
        lastMsg = gwCardModel.subtitle;
      }
      var status = null;
      if (gwCardModel) {
        if (gwCardModel.cardStatus === "error") status = { st: "error", cls: "sum-st-error" };
        else if (gwCardModel.cardStatus === "warn") status = { st: "degraded", cls: "sum-st-monitor" };
        else status = { st: "active", cls: "sum-st-active" };
      }
      return { subtitle: lastMsg, status: status, gwCardModel: gwCardModel };
    },
    collapsedMetricsHtml: function (arr, derived) {
      var gwCardModel = derived && derived.gwCardModel ? derived.gwCardModel : gatewayCardModel(arr);
      if (!gwCardModel) return "";
      var gc = gwCardModel.counters || {};
      return (
        '<span class="sum-metrics">' +
        ctx.sgOpInsetWellOkFailHtml(gc.http2xx || 0, gc.httpNot2xx || 0, "", {
          title: gatewayHttpOkFailTooltip(gc)
        }) +
        "</span>"
      );
    },
    expandedMiniHtml: function (arr) {
      return buildGatewayCardIntroHtml() + gatewayServicePanelMiniHtml(arr);
    },
    evlogOptions: function () {
      return { filterGatewayProbe: true, showSourceColumn: true, sumEvlogVisibleGateway: true };
    }
  };

  return {
    impl: impl,
    exports: {
      gatewayHttpOkFailTooltip: gatewayHttpOkFailTooltip,
      gatewayServicePanelMiniHtml: gatewayServicePanelMiniHtml,
      buildGatewayCardIntroHtml: buildGatewayCardIntroHtml
    }
  };
};
