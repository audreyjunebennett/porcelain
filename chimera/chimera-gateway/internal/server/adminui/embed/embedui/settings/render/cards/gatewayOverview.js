/**
 * Summarized feed card render (Phase 3 extraction).
 * Registers builders on ctx during ChimeraSettings.Render.Cards.mount*.
 */
globalThis.ChimeraSettings = globalThis.ChimeraSettings || {};
globalThis.ChimeraSettings.Render = globalThis.ChimeraSettings.Render || {};
globalThis.ChimeraSettings.Render.Cards = globalThis.ChimeraSettings.Render.Cards || {};

globalThis.ChimeraSettings.Render.Cards.mountGatewayOverview = function (ctx) {
  var escapeHtml = ctx.escapeHtml;
  var formatUtcLikeLogTimestamp = ctx.formatUtcLikeLogTimestamp;
  var operatorCardChevronHtml =
    typeof ctx.operatorCardChevronHtml === "function"
      ? ctx.operatorCardChevronHtml
      : function () {
          return (
            '<span class="material-symbols-outlined sg-op-chev-icon" aria-hidden="true">chevron_right</span>' +
            '<span class="sum-chev" aria-hidden="true"></span>'
          );
        };

  function serviceDisplayLabel(key) {
    if (
      globalThis.ChimeraSettings &&
      ChimeraSettings.Contracts &&
      typeof ChimeraSettings.Contracts.serviceDisplayLabel === "function"
    ) {
      return ChimeraSettings.Contracts.serviceDisplayLabel(key);
    }
    var k = String(key || "").trim().toLowerCase();
    if (!k) return "";
    if (k.indexOf("chimera-") === 0) return k.slice("chimera-".length);
    return k;
  }

  var SH = globalThis.ChimeraShared && globalThis.ChimeraShared.ServiceHealth;

  function gatewayServiceHealthTone(raw) {
    if (SH && typeof SH.normalizeHealthTone === "function") return SH.normalizeHealthTone(raw);
    return "unknown";
  }

  function gatewayServiceHealthEntries(ov) {
    var bf = ov && ov["chimera-broker"] ? ov["chimera-broker"] : {};
    var qd = ov && ov["chimera-vectorstore"] ? ov["chimera-vectorstore"] : {};
    var ix = ov && ov["chimera-indexer"] ? ov["chimera-indexer"] : {};
    return [
      { id: "chimera-gateway", raw: "up" },
      { id: "chimera-broker", raw: bf.state },
      { id: "chimera-vectorstore", raw: qd.state },
      { id: "chimera-indexer", raw: ix.worker }
    ];
  }

  /**
   * Gateway service-health strip: compact in collapsed summary row, full strip in expanded body.
   */
  function gatewayServiceHealthStripHtml(ov, opts) {
    opts = opts || {};
    var compact = !!opts.compact;
    var list = gatewayServiceHealthEntries(ov);
    var stateLabel = { up: "up", down: "down", unknown: "unknown" };
    var healthSeg =
      SH && typeof SH.healthSegSpan === "function"
        ? function (title, tone, extraClass) {
            return SH.healthSegSpan(escapeHtml, title, tone, extraClass);
          }
        : globalThis.ChimeraUI && typeof globalThis.ChimeraUI.healthSegSpan === "function"
          ? globalThis.ChimeraUI.healthSegSpan
          : function (title, tone) {
              return (
                '<span class="sum-bf-prov-health-seg sum-bf-prov-health-seg--' +
                (tone === "up" || tone === "down" ? tone : "unknown") +
                '" title="' +
                escapeHtml(title) +
                '"></span>'
              );
            };
    var segs = [];
    var labels = [];
    for (var i = 0; i < list.length; i++) {
      var ent = list[i] || {};
      var tone = gatewayServiceHealthTone(ent.raw);
      var lab = stateLabel[tone];
      var svcLab = serviceDisplayLabel(ent.id || "service");
      var title = svcLab + " · " + lab + (ent.raw != null && ent.raw !== "" ? " (" + String(ent.raw) + ")" : "");
      segs.push(healthSeg(title, tone));
      if (!compact) {
        labels.push(
          '<span class="sum-bf-prov-health-label" title="' +
            escapeHtml(title) +
            '">' +
            escapeHtml(svcLab || "—") +
            "</span>"
        );
      }
    }
    if (compact) {
      var compactTitle = "gateway, broker, vectorstore, indexer";
      var stripInner =
        '<span class="sum-bf-prov-health-root sum-bf-prov-health-root--compact" role="img" aria-label="service health">' +
        '<span class="sum-bf-prov-health-track sum-bf-prov-health-track--compact" title="' +
        escapeHtml(compactTitle) +
        '">' +
        segs.join("") +
        "</span></span>";
      if (SH && typeof SH.metricsWrapHtml === "function") return SH.metricsWrapHtml(stripInner);
      return '<span class="sum-metrics">' + stripInner + "</span>";
    }
    return (
      '<div class="sum-bf-prov-health-root" id="gateway-service-health-strip">' +
      '<div class="sum-bf-prov-health-track" title="Service health: gateway, broker, vectorstore, indexer">' +
      segs.join("") +
      '</div><div class="sum-bf-prov-health-labels">' +
      labels.join("") +
      "</div></div>"
    );
  }

  function buildGatewayOverviewCardHtml() {
    var data = ctx.gatewayOverviewCache;
    var loading = !data;
    var hasErr = !!(data && data._error);
    var ov = data && data.service_overview ? data.service_overview : null;
    var compactHealth = gatewayServiceHealthStripHtml(ov, { compact: true });
    var sub;
    if (loading) {
      sub = '<span class="sum-sub sum-sub--clamp muted">Loading overview…</span>';
    } else if (hasErr) {
      sub = '<span class="sum-sub sum-sub--clamp muted">Overview unavailable — using last known logs.</span>';
    } else {
      sub = '<span class="sum-sub sum-sub--clamp">Main-surface parity: service health.</span>';
    }
    var body = "";
    if (loading) {
      body = '<p class="muted">Fetching /api/ui/state…</p>';
    } else if (hasErr) {
      body = '<p class="muted">' + escapeHtml(String(data._error || "overview unavailable")) + "</p>";
    } else {
      var refAt = ov && ov.refreshed_at ? formatUtcLikeLogTimestamp(ov.refreshed_at) : "—";
      body =
        '<div class="sum-section-label">Service health</div>' +
        '<div data-ui-part="gateway-overview.health-strip">' +
        gatewayServiceHealthStripHtml(ov) +
        "</div>" +
        '<dl class="indexer-run-kv indexer-run-kv--gateway-summary" data-ui-part="gateway-overview.kv">' +
        "<dt>updated</dt><dd>" + escapeHtml(refAt) + "</dd>" +
        "</dl>";
    }
    return (
      '<details class="sum-card" id="gw-overview">' +
      '<summary data-ui-part="gateway-overview.summary">' +
      '<span class="sum-avatar sum-av-svc-chimera-gateway">GW</span>' +
      '<span class="sum-main"><span class="sum-title">Overview</span>' +
      sub +
      "</span>" +
      compactHealth +
      operatorCardChevronHtml() +
      "</summary>" +
      '<div class="sum-body">' + body + "</div></details>"
    );
  }

  function buildGatewayOverviewFeedSection() {
    var buildGatewayUsageCardHtml = ctx.buildGatewayUsageCardHtml;
    return (
      '<div class="sum-feed-section">' +
      buildGatewayOverviewCardHtml() +
      (typeof buildGatewayUsageCardHtml === "function" ? buildGatewayUsageCardHtml() : "") +
      "</div>"
    );
  }

  ctx.gatewayServiceHealthTone = gatewayServiceHealthTone;
  ctx.gatewayServiceHealthEntries = gatewayServiceHealthEntries;
  ctx.gatewayServiceHealthStripHtml = gatewayServiceHealthStripHtml;
  ctx.buildGatewayOverviewCardHtml = buildGatewayOverviewCardHtml;
  ctx.buildGatewayOverviewFeedSection = buildGatewayOverviewFeedSection;
};
