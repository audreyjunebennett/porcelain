/**
 * Summarized feed card render (Phase 3 extraction).
 * Registers builders on ctx during ChimeraSettings.Render.Cards.mount*.
 */
globalThis.ChimeraSettings = globalThis.ChimeraSettings || {};
globalThis.ChimeraSettings.Render = globalThis.ChimeraSettings.Render || {};
globalThis.ChimeraSettings.Render.Cards = globalThis.ChimeraSettings.Render.Cards || {};

globalThis.ChimeraSettings.Render.Cards.mountSharedFormat = function (ctx) {
  var escapeHtml = ctx.escapeHtml;

  function formatInt(n) {
    if (n == null || isNaN(n)) return "—";
    try {
      return new Intl.NumberFormat().format(Math.round(n));
    } catch (e) {
      return String(Math.round(n));
    }
  }

  function aggregateRollupRows(rows) {
    if (!rows || !rows.length) return { models: 0, tokens: 0, calls: 0 };
    var seen = {};
    var tokens = 0;
    var calls = 0;
    for (var i = 0; i < rows.length; i++) {
      var r = rows[i];
      var mid = r.model_id != null ? String(r.model_id) : "";
      if (mid) seen[mid] = true;
      tokens += Number(r.est_tokens) || 0;
      calls += Number(r.calls) || 0;
    }
    var nm = 0;
    for (var k in seen) {
      if (Object.prototype.hasOwnProperty.call(seen, k)) nm++;
    }
    return { models: nm, tokens: tokens, calls: calls };
  }

  function formatCompactTok(n) {
    if (n == null || isNaN(n)) return "—";
    var x = Number(n);
    if (x < 0) return "—";
    if (x >= 1000000) return (x / 1000000).toFixed(2).replace(/\.?0+$/, "") + "M";
    if (x >= 100000) return Math.round(x / 1000) + "k";
    if (x >= 10000) return (x / 1000).toFixed(1).replace(/\.0$/, "") + "k";
    if (x >= 1000) return (x / 1000).toFixed(2).replace(/0+$/, "").replace(/\.$/, "") + "k";
    return formatInt(x);
  }

  var GW_METRICS_COLGROUP =
    "<colgroup>" +
    '<col class="gw-metrics-col-provider">' +
    '<col class="gw-metrics-col-model">' +
    '<col class="gw-metrics-col-http">' +
    '<col class="gw-metrics-col-num">' +
    '<col class="gw-metrics-col-num">' +
    "</colgroup>";
  var GW_METRICS_EVENTS_COLGROUP =
    "<colgroup>" +
    '<col class="gw-metrics-col-time">' +
    '<col class="gw-metrics-col-provider">' +
    '<col class="gw-metrics-col-model">' +
    '<col class="gw-metrics-col-http">' +
    '<col class="gw-metrics-col-num">' +
    "</colgroup>";

  function metricsRollupTableHtml(rows) {
    if (!rows || !rows.length) {
      return (
        '<div class="sum-metrics-table-wrap sum-metrics-table-wrap--scroll-body">' +
        '<table class="sum-metrics-table sum-metrics-table--scroll-body"><tbody>' +
        '<tr><td class="muted">No models used</td></tr>' +
        "</tbody></table></div>"
      );
    }
    var h = [];
    h.push(
      '<div class="sum-metrics-table-wrap sum-metrics-table-wrap--scroll-body"><table class="sum-metrics-table sum-metrics-table--scroll-body">' +
      GW_METRICS_COLGROUP +
      '<thead><tr><th>Provider</th><th>Model</th><th>HTTP</th><th class="num">Calls</th><th class="num">tokens</th></tr></thead><tbody>'
    );
    for (var i = 0; i < rows.length; i++) {
      var r = rows[i];
      h.push(
        "<tr><td>" +
        escapeHtml(r.provider) +
        '</td><td class="gw-metrics-cell-model"><code class="sum-mono-id">' +
        escapeHtml(r.model_id) +
        '</code></td><td class="gw-metrics-cell-http">' +
        escapeHtml(r.status) +
        '</td><td class="num">' +
        escapeHtml(formatInt(r.calls)) +
        '</td><td class="num">' +
        escapeHtml(formatInt(r.est_tokens)) +
        "</td></tr>"
      );
    }
    h.push("</tbody></table></div>");
    return h.join("");
  }

  function pad2Utc(n) {
    return n < 10 ? "0" + n : String(n);
  }

  function formatUtcLikeLogTimestamp(tsOrDate) {
    if (tsOrDate === null || tsOrDate === undefined || tsOrDate === "") return "—";
    var d = tsOrDate instanceof Date ? tsOrDate : new Date(tsOrDate);
    if (isNaN(d.getTime())) return String(tsOrDate).replace("T", " ").slice(0, 23);
    return (
      d.getUTCFullYear() +
      "-" +
      pad2Utc(d.getUTCMonth() + 1) +
      "-" +
      pad2Utc(d.getUTCDate()) +
      " " +
      pad2Utc(d.getUTCHours()) +
      ":" +
      pad2Utc(d.getUTCMinutes()) +
      ":" +
      pad2Utc(d.getUTCSeconds())
    );
  }

  function formatUtcToMinute(tsOrDate) {
    if (tsOrDate === null || tsOrDate === undefined || tsOrDate === "") return "";
    var d = tsOrDate instanceof Date ? tsOrDate : new Date(tsOrDate);
    if (isNaN(d.getTime())) return String(tsOrDate).replace("T", " ").slice(0, 16);
    return (
      d.getUTCFullYear() +
      "-" +
      pad2Utc(d.getUTCMonth() + 1) +
      "-" +
      pad2Utc(d.getUTCDate()) +
      " " +
      pad2Utc(d.getUTCHours()) +
      ":" +
      pad2Utc(d.getUTCMinutes())
    );
  }

  function formatUtcToDay(tsOrDate) {
    if (tsOrDate === null || tsOrDate === undefined || tsOrDate === "") return "";
    var d = tsOrDate instanceof Date ? tsOrDate : new Date(tsOrDate);
    if (isNaN(d.getTime())) return String(tsOrDate).replace("T", " ").slice(0, 10);
    return d.getUTCFullYear() + "-" + pad2Utc(d.getUTCMonth() + 1) + "-" + pad2Utc(d.getUTCDate());
  }

  function metricsEventsTableHtml(rows) {
    if (!rows || !rows.length) {
      return '<p class="muted">No events recorded yet.</p>';
    }
    var h = [];
    h.push(
      '<div class="sum-metrics-table-wrap sum-metrics-table-wrap--scroll-body sum-metrics-events-scroll"><table class="sum-metrics-table sum-metrics-table--scroll-body">' +
      GW_METRICS_EVENTS_COLGROUP +
      '<thead><tr><th>Time (UTC)</th><th>Provider</th><th>Model</th><th>HTTP</th><th class="num">tokens</th></tr></thead><tbody>'
    );
    for (var i = 0; i < rows.length; i++) {
      var r = rows[i];
      h.push(
        "<tr><td>" +
        escapeHtml(formatUtcLikeLogTimestamp(r.occurred_at)) +
        "</td><td>" +
        escapeHtml(r.provider) +
        '</td><td class="gw-metrics-cell-model"><code class="sum-mono-id">' +
        escapeHtml(r.model_id) +
        '</code></td><td class="gw-metrics-cell-http">' +
        escapeHtml(r.status) +
        '</td><td class="num">' +
        escapeHtml(formatInt(r.est_tokens)) +
        "</td></tr>"
      );
    }
    h.push("</tbody></table></div>");
    return h.join("");
  }

  ctx.formatInt = formatInt;
  ctx.aggregateRollupRows = aggregateRollupRows;
  ctx.formatCompactTok = formatCompactTok;
  ctx.pad2Utc = pad2Utc;
  ctx.formatUtcLikeLogTimestamp = formatUtcLikeLogTimestamp;
  ctx.formatUtcToMinute = formatUtcToMinute;
  ctx.formatUtcToDay = formatUtcToDay;
  ctx.metricsRollupTableHtml = metricsRollupTableHtml;
  ctx.metricsEventsTableHtml = metricsEventsTableHtml;
};
