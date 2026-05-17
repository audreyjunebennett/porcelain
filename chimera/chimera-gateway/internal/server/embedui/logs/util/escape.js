/**
 * Minimal escaping for HTML text nodes.
 * @param {any} s
 * @returns {string}
 */
function escapeHtml(s) {
  if (s === null || s === undefined) return "";
  var str = String(s);
  return str
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/\"/g, "&quot;")
    .replace(/'/g, "&#39;");
}

/**
 * @param {any} s
 * @returns {string}
 */
function escapeAttr(s) {
  // Same as escapeHtml for now; explicit name helps audits.
  return escapeHtml(s);
}

// Export to global for browser + goja tests (no bundler).
globalThis.ChimeraLogs = globalThis.ChimeraLogs || {};
globalThis.ChimeraLogs.escapeHtml = escapeHtml;
globalThis.ChimeraLogs.escapeAttr = escapeAttr;

