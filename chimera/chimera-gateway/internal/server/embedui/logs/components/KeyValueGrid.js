/**
 * Renders the 4-column key/value table (key|value|key|value).
 * Mirrors the behavior of the legacy buildDetailsCell in logs.js.
 *
 * @param {{k: string, v: string}[]} extras
 * @returns {string}
 */
function KeyValueGrid(extras) {
  var esc = globalThis.ChimeraLogs && globalThis.ChimeraLogs.escapeHtml ? globalThis.ChimeraLogs.escapeHtml : function (s) { return String(s); };
  extras = Array.isArray(extras) ? extras : [];
  if (!extras.length) return '<span class="muted">—</span>';

  var s =
    '<table class="props-table"><colgroup>' +
    '<col class="col-k" /><col class="col-v" /><col class="col-k" /><col class="col-v" />' +
    "</colgroup><tbody>";

  for (var i = 0; i < extras.length; i += 2) {
    var a = extras[i] || { k: "", v: "" };
    s += "<tr>";
    s += '<td class="prop-name">' + esc(a.k) + "</td>";
    if (i + 1 < extras.length) {
      var b = extras[i + 1] || { k: "", v: "" };
      s += '<td class="prop-val">' + esc(a.v) + "</td>";
      s += '<td class="prop-name">' + esc(b.k) + "</td>";
      s += '<td class="prop-val">' + esc(b.v) + "</td>";
    } else {
      s += '<td class="prop-val prop-val-wide" colspan="3">' + esc(a.v) + "</td>";
    }
    s += "</tr>";
  }
  s += "</tbody></table>";
  return s;
}

globalThis.ChimeraLogs = globalThis.ChimeraLogs || {};
globalThis.ChimeraLogs.KeyValueGrid = KeyValueGrid;

