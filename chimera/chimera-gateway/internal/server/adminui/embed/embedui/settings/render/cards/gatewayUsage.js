/**
 * Summarized feed card render (Phase 3 extraction).
 * Registers builders on ctx during ChimeraSettings.Render.Cards.mount*.
 */
globalThis.ChimeraSettings = globalThis.ChimeraSettings || {};
globalThis.ChimeraSettings.Render = globalThis.ChimeraSettings.Render || {};
globalThis.ChimeraSettings.Render.Cards = globalThis.ChimeraSettings.Render.Cards || {};

globalThis.ChimeraSettings.Render.Cards.mountGatewayUsage = function (ctx) {
  var escapeHtml = ctx.escapeHtml;
  var operatorCardChevronHtml =
    typeof ctx.operatorCardChevronHtml === "function"
      ? ctx.operatorCardChevronHtml
      : function () {
          return (
            '<span class="material-symbols-outlined sg-op-chev-icon" aria-hidden="true">chevron_right</span>' +
            '<span class="sum-chev" aria-hidden="true"></span>'
          );
        };
  var formatInt = ctx.formatInt;
  var formatCompactTok = ctx.formatCompactTok;
  var formatUtcToMinute = ctx.formatUtcToMinute;
  var formatUtcToDay = ctx.formatUtcToDay;
  var aggregateRollupRows = ctx.aggregateRollupRows;
  var metricsRollupTableHtml = ctx.metricsRollupTableHtml;
  var metricsEventsTableHtml = ctx.metricsEventsTableHtml;
  var sgOpHealthPillHtml = ctx.sgOpHealthPillHtml;

  function chimeraBrokerShortModelLabel(model) {
    if (typeof ctx.chimeraBrokerShortModelLabel === "function") {
      return ctx.chimeraBrokerShortModelLabel(model);
    }
    if (!model || model === "—") return "—";
    return String(model);
  }

  function gatewayUsageRollupStatHtml(value, icon, title) {
    return (
      '<span class="sg-op-usage-rollup-stat" title="' +
      escapeHtml(title) +
      '">' +
      escapeHtml(value) +
      ' <span class="material-symbols-outlined material-symbols-outlined--sm sg-op-health-pill__icon" aria-hidden="true">' +
      escapeHtml(icon) +
      "</span></span>"
    );
  }

  function gatewayUsageRollupMetricsHtml(periodLabel, agg, loading, rollupTitle) {
    if (typeof sgOpHealthPillHtml !== "function") {
      var tail = loading ? "…" : formatInt(agg.models) + " models · " + formatCompactTok(agg.tokens) + " tokens";
      return (
        '<span class="sum-metric" title="' +
        escapeHtml(rollupTitle) +
        '"><strong>' +
        escapeHtml(periodLabel) +
        "</strong> · " +
        escapeHtml(tail) +
        "</span>"
      );
    }
    var modelsVal = loading ? "…" : formatInt(agg.models);
    var tokVal = loading ? "…" : formatCompactTok(agg.tokens);
    return (
      '<span class="sg-op-health-pill sg-op-health-pill--metric sg-op-usage-rollup-tag" title="' +
      escapeHtml(rollupTitle) +
      '">' +
      '<span class="sg-op-usage-rollup-period">' +
      escapeHtml(periodLabel) +
      "</span>" +
      gatewayUsageRollupStatHtml(modelsVal, "network_intelligence", "Models") +
      gatewayUsageRollupStatHtml(tokVal, "water_drop", "Tokens") +
      "</span>"
    );
  }

  function buildGatewayUsageIntroHtml() {
    return (
      '<div class="sum-section-label">Summary</div>' +
      '<div class="gw-usage-intro" id="gw-usage-intro">' +
      '<p class="gw-usage-intro-lead">' +
      "Which models ran, estimated tokens this UTC minute and calendar day, and the latest upstream calls." +
      "</p>" +
      '<p class="gw-usage-intro-follow">' +
      "Use this to spot load before quotas hard-stop traffic. The gateway compares these rollups to the ceilings in your limits file — counts are directional estimates; each vendor bills differently." +
      "</p>" +
      '<ul class="gw-usage-intro-bullets">' +
      "<li>" +
      '<a class="sum-ext-link" href="https://console.groq.com/docs/rate-limits" rel="noopener noreferrer">Groq rate limits</a>' +
      " and " +
      '<a class="sum-ext-link" href="https://ai.google.dev/gemini-api/docs/pricing" rel="noopener noreferrer">Gemini pricing &amp; free tier</a>' +
      " pages inform scraped free-tier hints." +
      "</li>" +
      "<li>" +
      "Ceiling tables live in " +
      '<a href="#" class="sum-proj-path" data-rel="config/provider-model-limits.yaml"><code>config/provider-model-limits.yaml</code></a>' +
      "; the gateway applies rollups against them when metrics are enabled." +
      "</li>" +
      "</ul>" +
      "</div>"
    );
  }

  function buildGatewayUsageCardHtml() {
    var data = ctx.metricsCache;
    var m =
      globalThis.ChimeraSettings &&
        globalThis.ChimeraSettings.Derive &&
        globalThis.ChimeraSettings.Derive.gatewayUsageCardModel
        ? globalThis.ChimeraSettings.Derive.gatewayUsageCardModel(
          data,
          function (rows) { return aggregateRollupRows(rows); },
          function (id) { return chimeraBrokerShortModelLabel(id); }
        )
        : null;

    var loading = m ? !!m.loading : !data;
    var storeOpen = m ? !!m.storeOpen : !!(data && data.metrics_store_open);
    var lastModel = m ? m.lastModelId || "—" : "—";
    var minAgg = m ? m.minAgg || { models: 0, tokens: 0 } : { models: 0, tokens: 0 };
    var dayAgg = m ? m.dayAgg || { models: 0, tokens: 0 } : { models: 0, tokens: 0 };
    var lblMin = m ? m.lblMin || "" : "";
    var lblDay = m ? m.lblDay || "" : "";
    var lblMinFmt = lblMin ? formatUtcToMinute(lblMin) : "";
    var lblDayFmt = lblDay ? formatUtcToDay(lblDay) : "";

    var sub = loading
      ? '<span class="sum-sub sum-sub--clamp muted">Loading gateway metrics…</span>'
      : '<span class="sum-sub sum-sub--clamp">Last model <code class="sum-mono-id">' +
      escapeHtml(chimeraBrokerShortModelLabel(lastModel)) +
      "</code></span>";

    var metrics =
      '<span class="sum-metrics">' +
      gatewayUsageRollupMetricsHtml(
        "minute",
        minAgg,
        loading,
        "Distinct upstream models · summed est. tokens (UTC minute rollup)"
      ) +
      gatewayUsageRollupMetricsHtml(
        "day",
        dayAgg,
        loading,
        "Distinct upstream models · summed est. tokens (UTC calendar day rollup)"
      ) +
      "</span>";

    var introHtml = buildGatewayUsageIntroHtml();

    var expandedInner = "";
    if (loading) {
      expandedInner = introHtml + '<p class="muted">Fetching /api/ui/metrics…</p>';
    } else if (!storeOpen) {
      expandedInner =
        introHtml +
        '<p class="muted">' +
        escapeHtml((m && m.message) || (data && data.message) || "Metrics store is not available.") +
        "</p>";
    } else {
      expandedInner = introHtml;
      expandedInner +=
        '<div class="sum-section-label">CURRENT MINUTE · ' +
        escapeHtml(lblMinFmt || lblMin) +
        "</div>" +
        metricsRollupTableHtml(data.minute_rollups || []) +
        '<div class="sum-section-label">CURRENT DAY · ' +
        escapeHtml(lblDayFmt || lblDay) +
        "</div>" +
        metricsRollupTableHtml(data.day_rollups || []) +
        '<div class="sum-section-label">Recent upstream calls</div>' +
        metricsEventsTableHtml(data.recent_events || []);
    }

    return (
      '<details class="sum-card" id="gw-usage-metrics">' +
      "<summary>" +
      '<span class="sum-avatar sum-av-svc-chimera-gateway">GW</span>' +
      '<span class="sum-main"><span class="sum-title">Model usage metrics</span>' +
      sub +
      "</span>" +
      metrics +
      operatorCardChevronHtml() +
      "</summary>" +
      '<div class="sum-body">' +
      expandedInner +
      "</div></details>"
    );
  }

  ctx.buildGatewayUsageIntroHtml = buildGatewayUsageIntroHtml;
  ctx.buildGatewayUsageCardHtml = buildGatewayUsageCardHtml;
};
