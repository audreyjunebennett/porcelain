/**
 * Shared log-line → service bucket classifiers for aggregate + dirty routing.
 */
globalThis.ChimeraSettings = globalThis.ChimeraSettings || {};
globalThis.ChimeraSettings.Derive = globalThis.ChimeraSettings.Derive || {};

(function () {
  function flatMsg(f) {
    return String(f.msg != null ? f.msg : f.message != null ? f.message : "").trim();
  }

  function flatMsgLower(f) {
    return flatMsg(f).toLowerCase();
  }

  function entryIsGatewayUpstreamRelay(ent, getFlat) {
    getFlat = typeof getFlat === "function" ? getFlat : function (parsed) { return (parsed && parsed.rawFlat) || {}; };
    var f = getFlat(ent.parsed);
    var msg = flatMsg(f);
    if (
      msg === "chat.chimera-broker.request" ||
      msg === "upstream chat response" ||
      msg === "chat.chimera-broker.response" ||
      msg === "chat.chimera-broker.error" ||
      msg.indexOf("chimera-broker.error") >= 0
    ) {
      return true;
    }
    var sh = ent.parsed && ent.parsed.shape ? String(ent.parsed.shape) : "";
    if (sh === "chat.chimera-broker" || sh.indexOf("chat.chimera-broker.") === 0) return true;
    return false;
  }

  function entryRoutesToChimeraBrokerBucket(ent, getFlat) {
    getFlat = typeof getFlat === "function" ? getFlat : function (parsed) { return (parsed && parsed.rawFlat) || {}; };
    if (entryIsGatewayUpstreamRelay(ent, getFlat)) return true;
    var f = getFlat(ent.parsed);
    var msg = flatMsg(f);
    if (msg === "chat.chimera-broker.available_models") return true;
    if (msg === "chat.routing.fallback") return true;
    if (msg === "chat.routing.attempt") return true;
    if (msg === "chat.routing.resolved") return true;
    if (msg === "chat.provider_limits.blocked") return true;
    if (msg.indexOf("virtual model fallback attempt") >= 0) return true;
    if (msg.indexOf("virtual model routing resolved") >= 0) return true;
    return false;
  }

  function entryIsVectorstoreLine(ent, getFlat) {
    getFlat = typeof getFlat === "function" ? getFlat : function (parsed) { return (parsed && parsed.rawFlat) || {}; };
    var f = getFlat(ent.parsed);
    var svcL = String(f.service || "").toLowerCase();
    if (svcL === "vectorstore" || svcL === "chimera-vectorstore") return true;
    var srcL = ent && String(ent.source || "").toLowerCase();
    if (srcL === "vectorstore" || srcL === "chimera-vectorstore") return true;
    var msg = flatMsgLower(f);
    if (msg.indexOf("vectorstore.") === 0) return true;
    if (msg.indexOf("chimera-vectorstore.") === 0) return true;
    return false;
  }

  function entryIsIndexerLine(ent, getFlat) {
    getFlat = typeof getFlat === "function" ? getFlat : function (parsed) { return (parsed && parsed.rawFlat) || {}; };
    var f = getFlat(ent.parsed);
    var svcL = String(f.service || "").toLowerCase();
    if (svcL === "indexer" || svcL === "chimera-indexer") return true;
    var srcL = ent && String(ent.source || "").toLowerCase();
    if (srcL === "indexer" || srcL === "chimera-indexer") return true;
    var msg = flatMsgLower(f);
    if (msg.indexOf("indexer.") === 0) return true;
    if (msg.indexOf("gateway.indexer") === 0) return true;
    return false;
  }

  var D = globalThis.ChimeraSettings.Derive;
  D.entryIsGatewayUpstreamRelay = entryIsGatewayUpstreamRelay;
  D.entryRoutesToChimeraBrokerBucket = entryRoutesToChimeraBrokerBucket;
  D.entryIsVectorstoreLine = entryIsVectorstoreLine;
  D.entryIsIndexerLine = entryIsIndexerLine;
})();
