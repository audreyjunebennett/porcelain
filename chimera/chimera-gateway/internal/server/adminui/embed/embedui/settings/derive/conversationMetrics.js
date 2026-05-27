/**
 * Pure metrics derivation for conversation cards.
 *
 * Exports:
 * - ChimeraSettings.Derive.scrapeConversationMetrics(events, getFlat)
 * - ChimeraSettings.Derive.conversationRagRetrievalSummary(events, getFlat)
 *
 * `events` is an array of { parsed: any, ... } where getFlat(parsed) returns a flat object.
 */

function flatMsg(f) {
  if (!f || typeof f !== "object") return "";
  var m = f.msg != null ? f.msg : f.message;
  return String(m || "").trim();
}

function scrapeConversationMetrics(events, getFlat) {
  events = Array.isArray(events) ? events : [];
  getFlat = typeof getFlat === "function" ? getFlat : function (p) { return (p && p.rawFlat) || {}; };

  var tok = null;
  var vec = null;
  var tokSum = 0;
  var tokSumCount = 0;

  for (var i = 0; i < events.length; i++) {
    var f = getFlat(events[i].parsed);
    var msg = flatMsg(f);
    if (msg === "conversation.rag.attached" && f.hits != null) {
      var attachedHits = Number(f.hits);
      if (!isNaN(attachedHits)) vec = attachedHits;
    }
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
    if (vec == null && f.hits != null && msg !== "conversation.rag.attached") vec = Number(f.hits);
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

function conversationRagRetrievalSummary(events, getFlat) {
  events = Array.isArray(events) ? events : [];
  getFlat = typeof getFlat === "function" ? getFlat : function (p) { return (p && p.rawFlat) || {}; };
  var metrics = scrapeConversationMetrics(events, getFlat);
  var Derive =
    globalThis.ChimeraSettings && ChimeraSettings.Derive ? ChimeraSettings.Derive : null;
  var coords =
    Derive && typeof Derive.extractRagCoordsFromEvents === "function"
      ? Derive.extractRagCoordsFromEvents(events, getFlat)
      : null;
  var ws = { label: "", known: false, proposed: "" };
  if (coords && coords.projectId && Derive && typeof Derive.resolveRagWorkspaceLabel === "function") {
    ws = Derive.resolveRagWorkspaceLabel(coords.tenantId, coords.projectId, coords.flavorId);
  }
  var workspaceTitle = ws.known ? ws.label : ws.proposed || "";
  return {
    hits: metrics.vec,
    hasWorkspace: !!(coords && coords.projectId),
    workspaceTitle: workspaceTitle,
    workspaceKnown: ws.known
  };
}

globalThis.ChimeraSettings = globalThis.ChimeraSettings || {};
globalThis.ChimeraSettings.Derive = globalThis.ChimeraSettings.Derive || {};
globalThis.ChimeraSettings.Derive.scrapeConversationMetrics = scrapeConversationMetrics;
globalThis.ChimeraSettings.Derive.conversationRagRetrievalSummary = conversationRagRetrievalSummary;

