/**
 * Pure metrics derivation for conversation cards.
 *
 * Exports:
 * - ChimeraLogs.Derive.scrapeConversationMetrics(events, getFlat)
 *
 * `events` is an array of { parsed: any, ... } where getFlat(parsed) returns a flat object.
 */

function scrapeConversationMetrics(events, getFlat) {
  events = Array.isArray(events) ? events : [];
  getFlat = typeof getFlat === "function" ? getFlat : function (p) { return (p && p.rawFlat) || {}; };

  var tok = null;
  var vec = null;
  var tokSum = 0;
  var tokSumCount = 0;

  for (var i = 0; i < events.length; i++) {
    var f = getFlat(events[i].parsed);
    var ut =
      f.usageTotalTokens != null
        ? Number(f.usageTotalTokens)
        : f["usage.total_tokens"] != null
          ? Number(f["usage.total_tokens"])
          : NaN;
    if (!isNaN(ut) && ut > 0) {
      tokSum += ut;
      tokSumCount++;
    }
    if (vec == null && f.rag_hits != null) vec = Number(f.rag_hits);
    if (vec == null && f.hits != null) vec = Number(f.hits);
    if (vec == null && f.chunks != null) vec = Number(f.chunks);
  }

  if (tokSumCount > 0) tok = tokSum;

  if (tok == null) {
    for (var j = 0; j < events.length; j++) {
      var f2 = getFlat(events[j].parsed);
      var p2 = f2.usagePromptTokens != null ? Number(f2.usagePromptTokens) : NaN;
      var c2 = f2.usageCompletionTokens != null ? Number(f2.usageCompletionTokens) : NaN;
      if (!isNaN(p2) || !isNaN(c2)) {
        var sumPc = (isNaN(p2) ? 0 : p2) + (isNaN(c2) ? 0 : c2);
        if (sumPc > 0) tok = (tok || 0) + sumPc;
      }
    }
  }

  if (tok == null) {
    for (var k = 0; k < events.length; k++) {
      var f3 = getFlat(events[k].parsed);
      if (f3.response_tokens_est != null) {
        tok = Number(f3.response_tokens_est);
        break;
      }
      if (f3.tokens != null) {
        tok = Number(f3.tokens);
        break;
      }
      if (f3.outgoingTokens != null) {
        tok = Number(f3.outgoingTokens);
        break;
      }
    }
  }

  if (tok != null && isNaN(tok)) tok = null;
  if (vec != null && isNaN(vec)) vec = null;
  return { tok: tok, vec: vec };
}

globalThis.ChimeraLogs = globalThis.ChimeraLogs || {};
globalThis.ChimeraLogs.Derive = globalThis.ChimeraLogs.Derive || {};
globalThis.ChimeraLogs.Derive.scrapeConversationMetrics = scrapeConversationMetrics;

