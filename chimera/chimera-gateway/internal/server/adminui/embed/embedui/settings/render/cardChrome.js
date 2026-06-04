/**
 * Shared collapsible-card chrome (metrics wells, status pills, recent-window helpers).
 * Used on the settings summarized feed, component gallery, and setup wizard surfaces.
 */
globalThis.ChimeraSettings = globalThis.ChimeraSettings || {};
globalThis.ChimeraSettings.Render = globalThis.ChimeraSettings.Render || {};
globalThis.ChimeraSettings.Render.Cards = globalThis.ChimeraSettings.Render.Cards || {};

globalThis.ChimeraSettings.Render.Cards.mountCardChrome = function (ctx) {
  var escapeHtml = ctx.escapeHtml;

  function formatInt(n) {
    if (typeof ctx.formatInt === "function") return ctx.formatInt(n);
    if (n == null || isNaN(n)) return "—";
    return String(Math.round(n));
  }

  function humanDurationMs(ms) {
    if (
      globalThis.ChimeraSettings &&
      typeof ChimeraSettings.humanDurationMs === "function"
    ) {
      return ChimeraSettings.humanDurationMs(ms);
    }
    return ms != null ? String(ms) : "—";
  }

  function sliceRecent(arr, n) {
    if (!arr || !arr.length) return [];
    var take = Math.min(n, arr.length);
    return arr.slice(-take);
  }

  function sgOpInsetWellOkFailHtml(okN, failN, prefix, opts) {
    opts = opts || {};
    var lead = "";
    if (opts.leadIcon) {
      lead =
        '<span class="material-symbols-outlined material-symbols-outlined--sm" aria-hidden="true">' +
        escapeHtml(String(opts.leadIcon)) +
        "</span> ";
    } else if (prefix) {
      lead = escapeHtml(String(prefix)) + " ";
    }
    var titleAttr =
      opts.title != null && String(opts.title).trim() !== ""
        ? ' title="' + escapeHtml(String(opts.title)) + '"'
        : "";
    var out = '<span class="sg-op-inset-well"' + titleAttr + ">" + lead;
    if (opts.okIcon !== false) {
      out +=
        '<span class="material-symbols-outlined material-symbols-outlined--sm" aria-hidden="true">check_circle</span> ';
    }
    out += escapeHtml(formatInt(okN)) + " " + escapeHtml(formatInt(failN));
    if (opts.errorIcon !== false) {
      out += ' <span class="material-symbols-outlined material-symbols-outlined--sm" aria-hidden="true">error</span>';
    }
    return out + "</span>";
  }

  /** Append trailing summary chips/pills into one .sum-metrics cluster (user-card parity). */
  function summaryMetricsHtml(innerHtml, extraHtml) {
    innerHtml = innerHtml != null ? String(innerHtml) : "";
    extraHtml = extraHtml != null ? String(extraHtml) : "";
    if (!innerHtml && !extraHtml) return "";
    if (innerHtml.indexOf('class="sum-metrics"') >= 0) {
      if (!extraHtml) return innerHtml;
      return innerHtml.replace(/<\/span>\s*$/, extraHtml + "</span>");
    }
    return '<span class="sum-metrics">' + innerHtml + extraHtml + "</span>";
  }

  function serviceSummaryStatusPillHtml(st) {
    st = st || {};
    var label = st.st != null ? String(st.st) : "";
    var okStates = { active: 1, complete: 1, idle: 1, waiting: 1 };
    var variant = okStates[label] ? "ok" : "";
    var pulse = st.cls && String(st.cls).indexOf("sum-pulse") >= 0;
    if (typeof ctx.sgOpHealthPillHtml === "function") {
      return ctx.sgOpHealthPillHtml(label, variant, { pulse: pulse });
    }
    return '<span class="sum-status ' + (st.cls || "") + '">' + escapeHtml(label) + "</span>";
  }

  ctx.humanDurationMs = humanDurationMs;
  ctx.sliceRecent = sliceRecent;
  ctx.sgOpInsetWellOkFailHtml = sgOpInsetWellOkFailHtml;
  ctx.summaryMetricsHtml = summaryMetricsHtml;
  ctx.serviceSummaryStatusPillHtml = serviceSummaryStatusPillHtml;
};
