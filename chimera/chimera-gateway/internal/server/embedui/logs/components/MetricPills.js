/**
 * @param {{text: string, title?: string}[]} pills
 * @returns {string}
 */
function MetricPillsRow(pills) {
  var esc = globalThis.ChimeraLogs && globalThis.ChimeraLogs.escapeHtml ? globalThis.ChimeraLogs.escapeHtml : function (s) { return String(s); };
  var escA = globalThis.ChimeraLogs && globalThis.ChimeraLogs.escapeAttr ? globalThis.ChimeraLogs.escapeAttr : function (s) { return String(s); };
  pills = Array.isArray(pills) ? pills : [];
  if (!pills.length) return "";
  var out = '<span class="sum-metrics">';
  for (var i = 0; i < pills.length; i++) {
    var p = pills[i] || {};
    var txt = p.text != null ? String(p.text) : "";
    var title = p.title != null ? String(p.title) : "";
    out +=
      '<span class="sum-metric"' +
      (title ? ' title="' + escA(title) + '"' : "") +
      ">" +
      esc(txt) +
      "</span>";
  }
  out += "</span>";
  return out;
}

globalThis.ChimeraLogs = globalThis.ChimeraLogs || {};
globalThis.ChimeraLogs.MetricPillsRow = MetricPillsRow;

