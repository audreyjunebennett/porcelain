/**
 * @param {{text: string, title?: string, variant?: string}} model
 * @returns {string}
 */
function Badge(model) {
  model = model || {};
  var txt = model.text != null ? String(model.text) : "";
  var title = model.title != null ? String(model.title) : "";
  var v = model.variant != null ? String(model.variant) : "neutral";
  var cls = "sum-svc-badge";
  // Keep mapping small; extend later as needed.
  if (v === "svc-chimera-vectorstore") cls += " sum-svc-chimera-vectorstore";
  else if (v === "svc-chimera-indexer") cls += " sum-svc-chimera-indexer";
  else if (v === "svc-chimera-broker") cls += " sum-svc-upstream";
  else if (v === "svc-chimera-gateway") cls += " sum-svc-chimera-gateway";
  else if (v === "error") cls += " sum-svc-upstream-filled";
  var esc = globalThis.ChimeraLogs && globalThis.ChimeraLogs.escapeHtml ? globalThis.ChimeraLogs.escapeHtml : function (s) { return String(s); };
  var escA = globalThis.ChimeraLogs && globalThis.ChimeraLogs.escapeAttr ? globalThis.ChimeraLogs.escapeAttr : function (s) { return String(s); };
  return (
    '<span class="' +
    escA(cls) +
    '"' +
    (title ? ' title="' + escA(title) + '"' : "") +
    ">" +
    esc(txt) +
    "</span>"
  );
}

globalThis.ChimeraLogs = globalThis.ChimeraLogs || {};
globalThis.ChimeraLogs.Badge = Badge;

