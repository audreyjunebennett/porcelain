/**
 * Default service card for unknown / generic supervised services (lines, HTTP, warn+error).
 */
globalThis.ChimeraSettings = globalThis.ChimeraSettings || {};
globalThis.ChimeraSettings.Render = globalThis.ChimeraSettings.Render || {};
globalThis.ChimeraSettings.Render.Cards = globalThis.ChimeraSettings.Render.Cards || {};
globalThis.ChimeraSettings.Render.Cards.ServiceFeed = globalThis.ChimeraSettings.Render.Cards.ServiceFeed || {};

globalThis.ChimeraSettings.Render.Cards.ServiceFeed.mountDefault = function (deps) {
  var ctx = deps.ctx;
  var escapeHtml = deps.escapeHtml;
  var getFlat = deps.getFlat;
  var formatInt = deps.formatInt;
  var primaryLogMessage = deps.primaryLogMessage;

  function countHttpMetrics(arr) {
    var httpN = 0;
    var sumMs = 0;
    for (var k = 0; k < arr.length; k++) {
      var p = arr[k].parsed;
      if (p.shape === "http.access") {
        httpN++;
        var rt = Number(getFlat(p).responseTimeMs);
        if (!isNaN(rt)) sumMs += rt;
      }
    }
    return { httpN: httpN, sumMs: sumMs };
  }

  var impl = {
    deriveCollapsed: function (arr) {
      var lastMsg = "";
      var last = arr.length ? arr[arr.length - 1] : null;
      if (last) lastMsg = primaryLogMessage(last.parsed, last.text);
      return { subtitle: lastMsg };
    },
    collapsedMetricsHtml: function (arr) {
      var m = countHttpMetrics(arr);
      return (
        '<span class="sum-metrics">' +
        '<span class="sum-metric">' +
        escapeHtml(String(arr.length)) +
        ' lines</span>' +
        (m.httpN ? '<span class="sum-metric">' + escapeHtml(String(m.httpN)) + " http</span>" : "") +
        (m.sumMs && typeof ctx.humanDurationMs === "function"
          ? '<span class="sum-metric">' + escapeHtml(ctx.humanDurationMs(m.sumMs)) + " Σ</span>"
          : "") +
        "</span>"
      );
    },
    expandedTimelineHtml: function (arr) {
      var evConv = [];
      for (var j = 0; j < arr.length; j++) {
        evConv.push({ parsed: arr[j].parsed, text: arr[j].text, ts: arr[j].ts, source: arr[j].source });
      }
      var timelineFn = ctx.timelineBarHtml;
      return (
        '<div class="sum-section-label">Request timeline</div>' +
        (typeof timelineFn === "function" ? timelineFn(evConv) : "")
      );
    },
    expandedMiniHtml: function (arr) {
      var httpN2 = 0,
        sumMs2 = 0,
        err2 =
          typeof ctx.countWarnErrorInEntries === "function"
            ? ctx.countWarnErrorInEntries(arr)
            : 0;
      for (var k2 = 0; k2 < arr.length; k2++) {
        if (arr[k2].parsed.shape === "http.access") {
          httpN2++;
          var rt2 = Number(getFlat(arr[k2].parsed).responseTimeMs);
          if (!isNaN(rt2)) sumMs2 += rt2;
        }
      }
      return (
        '<div class="sum-mini-row">' +
        '<div class="sum-mini-card">Lines<strong>' +
        escapeHtml(String(arr.length)) +
        '</strong></div><div class="sum-mini-card">HTTP · Σ ms<strong>' +
        escapeHtml(formatInt(httpN2) + " · " + (httpN2 ? String(Math.round(sumMs2)) : "—")) +
        '</strong></div><div class="sum-mini-card">Warn+error lines<strong>' +
        escapeHtml(String(err2)) +
        "</strong></div></div>"
      );
    }
  };

  return { impl: impl };
};
