/**
 * @param {{ts?: any}|any} entry
 * @returns {Date|null}
 */
function entryInstant(entry) {
  if (!entry) return null;
  var ts = entry.ts !== undefined ? entry.ts : entry;
  if (ts === null || ts === undefined || ts === "") return null;
  var d = ts instanceof Date ? ts : new Date(ts);
  return isNaN(d.getTime()) ? null : d;
}

/**
 * @param {any} ms
 * @returns {string}
 */
function humanDurationMs(ms) {
  if (ms == null || isNaN(ms) || ms < 0) return "—";
  if (ms < 1000) return Math.round(ms) + " ms";
  if (ms < 60000) return (ms / 1000).toFixed(1) + " s";
  if (ms < 3600000) return Math.round(ms / 60000) + " min";
  return (ms / 3600000).toFixed(1) + " h";
}

globalThis.ChimeraLogs = globalThis.ChimeraLogs || {};
globalThis.ChimeraLogs.entryInstant = entryInstant;
globalThis.ChimeraLogs.humanDurationMs = humanDurationMs;

