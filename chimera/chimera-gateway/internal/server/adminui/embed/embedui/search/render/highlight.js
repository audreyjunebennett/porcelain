/**
 * Query term highlighting for search excerpts.
 */
(function () {
  "use strict";

  var esc =
    globalThis.ChimeraUI && ChimeraUI.escapeHtml
      ? ChimeraUI.escapeHtml
      : function (s) {
          return String(s || "");
        };

  function escapeRegExp(s) {
    return String(s).replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
  }

  function tokenizeQuery(query) {
    var q = String(query || "").trim();
    if (!q) return [];
    var parts = q.split(/\s+/).filter(function (p) {
      return p.length > 0;
    });
    var seen = {};
    var out = [];
    for (var i = 0; i < parts.length; i++) {
      var lower = parts[i].toLowerCase();
      if (seen[lower]) continue;
      seen[lower] = true;
      out.push(parts[i]);
    }
    return out.sort(function (a, b) {
      return b.length - a.length;
    });
  }

  function highlightPlain(text, query) {
    text = text == null ? "" : String(text);
    var tokens = tokenizeQuery(query);
    if (!tokens.length) return esc(text);
    var pattern = tokens.map(escapeRegExp).join("|");
    var re = new RegExp("(" + pattern + ")", "gi");
    var out = "";
    var last = 0;
    var m;
    while ((m = re.exec(text)) !== null) {
      out += esc(text.slice(last, m.index));
      out += '<mark class="search-hl">' + esc(m[1]) + "</mark>";
      last = m.index + m[0].length;
    }
    out += esc(text.slice(last));
    return out;
  }

  globalThis.ChimeraSearch = globalThis.ChimeraSearch || {};
  globalThis.ChimeraSearch.Render = globalThis.ChimeraSearch.Render || {};
  globalThis.ChimeraSearch.Render.Highlight = {
    highlightPlain: highlightPlain,
    tokenizeQuery: tokenizeQuery
  };
})();
