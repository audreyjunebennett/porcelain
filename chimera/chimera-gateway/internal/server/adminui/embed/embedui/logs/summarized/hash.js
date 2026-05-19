/**
 * Stable card content hashing for summarized view model (Phase 4 / Phase 5 diff).
 */
globalThis.ChimeraLogs = globalThis.ChimeraLogs || {};
globalThis.ChimeraLogs.Summarized = globalThis.ChimeraLogs.Summarized || {};
globalThis.ChimeraLogs.Summarized.Hash = globalThis.ChimeraLogs.Summarized.Hash || {};

(function () {
  function stableSerialize(value) {
    return JSON.stringify(value, function (_key, val) {
      if (val && typeof val === "object" && !Array.isArray(val)) {
        var sorted = {};
        var keys = Object.keys(val).sort();
        for (var ki = 0; ki < keys.length; ki++) {
          sorted[keys[ki]] = val[keys[ki]];
        }
        return sorted;
      }
      return val;
    });
  }

  /** @param {object} summary collapsed-row fields */
  /** @param {object} body expanded-body fields */
  function cardContentHash(strHash, summary, body) {
    return strHash(stableSerialize({ summary: summary || {}, body: body || {} }));
  }

  function eventSeqSignature(events) {
    if (!events || !events.length) {
      return { count: 0, minSeq: null, maxSeq: null, lastSeq: null };
    }
    var minSeq = null;
    var maxSeq = null;
    var lastSeq = null;
    for (var i = 0; i < events.length; i++) {
      var sq = events[i].seq != null ? Number(events[i].seq) : null;
      if (sq == null || isNaN(sq)) continue;
      if (minSeq == null || sq < minSeq) minSeq = sq;
      if (maxSeq == null || sq > maxSeq) maxSeq = sq;
      lastSeq = sq;
    }
    return { count: events.length, minSeq: minSeq, maxSeq: maxSeq, lastSeq: lastSeq };
  }

  globalThis.ChimeraLogs.Summarized.Hash.stableSerialize = stableSerialize;
  globalThis.ChimeraLogs.Summarized.Hash.cardContentHash = cardContentHash;
  globalThis.ChimeraLogs.Summarized.Hash.eventSeqSignature = eventSeqSignature;
})();
