/**
 * Stable-ish short hash for DOM ids and grouping keys.
 * Mirrors legacy `strHash` in `embedui/logs.js`.
 *
 * @param {any} s
 * @returns {string}
 */
function strHash(s) {
  var h = 0;
  var t = String(s);
  for (var i = 0; i < t.length; i++) h = ((h << 5) - h) + t.charCodeAt(i) | 0;
  return "fc" + (h >>> 0).toString(16);
}

globalThis.ChimeraLogs = globalThis.ChimeraLogs || {};
globalThis.ChimeraLogs.strHash = strHash;

