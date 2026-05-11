/**
 * Conversation card: join-tier helpers, RAG/Qdrant window matching, lifecycle model (pure).
 *
 * Exports:
 * - ClaudiaLogs.Derive.conversationRequestIdTier2Eligible(flat)
 * - ClaudiaLogs.Derive.conversationIndexRunTier3Eligible(flat)
 * - ClaudiaLogs.Derive.extractConversationQdrantJoinAnchors(events, getFlat)
 * - ClaudiaLogs.Derive.joinQdrantLineConversationMatch(events, getFlat, qFlat, qTimeMs)
 * - ClaudiaLogs.Derive.joinQdrantLineConversationTier(events, getFlat, qFlat, qTimeMs)
 * - ClaudiaLogs.Derive.conversationTurnGroupsForExpanded(events, getFlat)
 * - ClaudiaLogs.Derive.buildConversationCardModel(events, getFlat)
 */

var LEGACY_QDRANT_WINDOW_MS = 5000;
var DEFAULT_QDRANT_SPAN_WINDOW_MS = 10000;

function convNormMsg(flat) {
  if (!flat || typeof flat !== "object") return "";
  var m = flat.msg != null ? flat.msg : flat.message;
  return String(m || "").trim();
}

/** Tier 2: request_id join for chat-scoped gateway lines (extends BiFrost relay filter). */
function conversationRequestIdTier2Eligible(flat) {
  if (!flat || typeof flat !== "object") return false;
  if (
    globalThis.ClaudiaLogs &&
    ClaudiaLogs.Derive &&
    typeof ClaudiaLogs.Derive.conversationBifrostTimelineFlat === "function" &&
    ClaudiaLogs.Derive.conversationBifrostTimelineFlat(flat)
  ) {
    return true;
  }
  var msg = convNormMsg(flat);
  if (!msg) return false;
  if (msg.indexOf("conversation.") === 0) return true;
  if (msg.indexOf("rag.") === 0) return true;
  if (msg === "chat.request") return true;
  if (msg.indexOf("chat.bifrost.") === 0) return true;
  if (msg.indexOf("chat.routing.") === 0) return true;
  if (msg.indexOf("chat.provider_limits.") === 0) return true;
  if (msg.indexOf("chat.tool_router.") === 0) return true;
  var ml = msg.toLowerCase();
  if (ml === "gateway.http.access" || ml === "http response" || flat.path != null) {
    var p = String(flat.path || "").split("?")[0];
    if (p.indexOf("/v1/chat/completions") >= 0) return true;
  }
  return false;
}

/** Tier 3: index_run_id join for ingest + indexer process lines. */
function conversationIndexRunTier3Eligible(flat) {
  if (!flat || typeof flat !== "object") return false;
  var svc = String(flat.service || "").toLowerCase();
  if (svc === "indexer") return true;
  var msg = convNormMsg(flat);
  if (msg.indexOf("indexer.") === 0) return true;
  if (msg.indexOf("ingest.") === 0) return true;
  return false;
}

function eventTimeMs(ev) {
  if (!ev || ev.ts == null || ev.ts === "") return NaN;
  var d = ev.ts instanceof Date ? ev.ts : new Date(ev.ts);
  var t = d.getTime();
  return isNaN(t) ? NaN : t;
}

/**
 * Anchors for matching Qdrant subprocess HTTP lines to a conversation (tier 4 / 4b).
 * @returns {{ legacy: Array<{coll:string,t:number}>, spans: Array<{coll:string,t0:number,t1:number,span_id:string,turn_index:number|null}> }}
 */
function extractConversationQdrantJoinAnchors(events, getFlat) {
  events = Array.isArray(events) ? events : [];
  getFlat = typeof getFlat === "function" ? getFlat : function (p) { return (p && p.rawFlat) || {}; };
  var legacy = [];
  var spans = [];
  for (var i = 0; i < events.length; i++) {
    var ev = events[i];
    var f = getFlat(ev.parsed);
    var msg = convNormMsg(f);
    var coll = f.collection != null ? String(f.collection).trim() : "";
    if (!coll) continue;
    var t = eventTimeMs(ev);
    if (msg === "rag.query" || msg === "rag.embed") {
      if (isFinite(t)) legacy.push({ coll: coll, t: t });
    }
    if (msg === "conversation.rag.span") {
      if (!isFinite(t)) continue;
      var wm = f.window_ms != null ? Number(f.window_ms) : DEFAULT_QDRANT_SPAN_WINDOW_MS;
      if (isNaN(wm) || wm < 0) wm = DEFAULT_QDRANT_SPAN_WINDOW_MS;
      var spanID = f.span_id != null ? String(f.span_id).trim() : "";
      var turnIndex = f.turn_index != null && !isNaN(Number(f.turn_index)) ? Math.round(Number(f.turn_index)) : null;
      spans.push({ coll: coll, t0: t, t1: t + wm, span_id: spanID, turn_index: turnIndex });
    }
  }
  return { legacy: legacy, spans: spans };
}

/** @returns {{tier:"anchored_inferred"|"inferred",span_id?:string,turn_index?:number|null,span_start_ms?:number}|null} */
function joinQdrantLineConversationMatch(events, getFlat, qFlat, qTimeMs) {
  if (!qFlat || typeof qFlat !== "object" || !isFinite(qTimeMs)) return null;
  getFlat = typeof getFlat === "function" ? getFlat : function (p) { return (p && p.rawFlat) || {}; };
  var collQ = qFlat.collection != null ? String(qFlat.collection).trim() : "";
  if (!collQ) return null;
  var anchors = extractConversationQdrantJoinAnchors(events, getFlat);
  var j;
  var bestSpan = null;
  for (j = 0; j < anchors.spans.length; j++) {
    var sp = anchors.spans[j];
    if (sp.coll !== collQ) continue;
    if (qTimeMs >= sp.t0 && qTimeMs <= sp.t1 && (!bestSpan || sp.t0 >= bestSpan.t0)) {
      bestSpan = sp;
    }
  }
  if (bestSpan) {
    return {
      tier: "anchored_inferred",
      span_id: bestSpan.span_id,
      turn_index: bestSpan.turn_index,
      span_start_ms: bestSpan.t0
    };
  }
  for (j = 0; j < anchors.legacy.length; j++) {
    var leg = anchors.legacy[j];
    if (leg.coll !== collQ) continue;
    if (Math.abs(qTimeMs - leg.t) <= LEGACY_QDRANT_WINDOW_MS) return { tier: "inferred" };
  }
  return null;
}

/** @returns {"anchored_inferred"|"inferred"|null} */
function joinQdrantLineConversationTier(events, getFlat, qFlat, qTimeMs) {
  var match = joinQdrantLineConversationMatch(events, getFlat, qFlat, qTimeMs);
  return match && match.tier ? match.tier : null;
}

/**
 * Turn attribution for an event:
 * 1. explicit flat.turn_index when present and finite,
 * 2. ev.qdrantTurnIndex (set by tier-4b match metadata) when present,
 * 3. inherited from the most recent prior event with a known turn (events sorted by seq/ts).
 * Events that never see a turn (e.g. merge resolve_failed before cid resolved) get null.
 *
 * @returns {Array<{turnIndex:number|null,label:string,events:Array}>}
 *   Groups are ordered most-recent-turn-first to match the global reverse-chronological log view;
 *   within each group, events keep ascending seq/ts (callers reverse for display).
 */
function conversationTurnGroupsForExpanded(events, getFlat) {
  events = Array.isArray(events) ? events : [];
  getFlat = typeof getFlat === "function" ? getFlat : function (p) { return (p && p.rawFlat) || {}; };

  var sorted = sortEventsForModel(events);
  var attributed = [];
  var prevTurn = null;
  for (var i = 0; i < sorted.length; i++) {
    var ev = sorted[i];
    var f = getFlat(ev.parsed);
    var ti = null;
    if (f.turn_index != null && !isNaN(Number(f.turn_index))) {
      ti = Math.round(Number(f.turn_index));
    } else if (ev.qdrantTurnIndex != null && !isNaN(Number(ev.qdrantTurnIndex))) {
      ti = Math.round(Number(ev.qdrantTurnIndex));
    } else if (prevTurn != null) {
      ti = prevTurn;
    }
    if (ti != null) prevTurn = ti;
    attributed.push({ ev: ev, turnIndex: ti });
  }

  var groupMap = {};
  var order = [];
  for (var k = 0; k < attributed.length; k++) {
    var a = attributed[k];
    var key = a.turnIndex == null ? "_" : "t" + String(a.turnIndex);
    if (!groupMap[key]) {
      groupMap[key] = { turnIndex: a.turnIndex, events: [] };
      order.push(key);
    }
    groupMap[key].events.push(a.ev);
  }

  var groups = [];
  for (var oi = 0; oi < order.length; oi++) {
    var g = groupMap[order[oi]];
    var label = g.turnIndex == null ? "Unattributed" : "Turn " + String(g.turnIndex);
    groups.push({ turnIndex: g.turnIndex, label: label, events: g.events });
  }
  groups.sort(function (a, b) {
    if (a.turnIndex == null && b.turnIndex == null) return 0;
    if (a.turnIndex == null) return -1;
    if (b.turnIndex == null) return 1;
    return b.turnIndex - a.turnIndex;
  });
  return groups;
}

function sortEventsForModel(events) {
  return events.slice().sort(function (a, b) {
    var sa = a.seq != null ? Number(a.seq) : 0;
    var sb = b.seq != null ? Number(b.seq) : 0;
    if (sa !== sb) return sa - sb;
    var ta = eventTimeMs(a);
    var tb = eventTimeMs(b);
    if (!isFinite(ta) && !isFinite(tb)) return 0;
    if (!isFinite(ta)) return -1;
    if (!isFinite(tb)) return 1;
    return ta - tb;
  });
}

function lifecycleLabelForMsg(m) {
  switch (m) {
    case "conversation.received":
      return "received";
    case "conversation.merged":
      return "matched";
    case "conversation.dedup_hit":
      return "dedup";
    case "conversation.routing.resolved":
      return "routed";
    case "conversation.rag.attached":
      return "RAG attached";
    case "conversation.rag.skipped":
      return "RAG skipped";
    case "conversation.rag.span":
      return "RAG span";
    case "conversation.upstream.started":
      return "upstream…";
    case "conversation.upstream.completed":
      return "upstream ok";
    case "conversation.upstream.failed":
      return "upstream failed";
    case "conversation.fallback.attempted":
      return "fallback";
    case "conversation.fallback.model_not_found":
      return "model not found";
    case "conversation.fallback.exhausted":
      return "fallback exhausted";
    case "conversation.delivered":
      return "delivered";
    case "conversation.errored":
      return "error";
    case "conversation.request.witness":
      return "request witness";
    case "conversation.response.witness":
      return "response witness";
    case "conversation.payload.sample":
      return "payload sample";
    default:
      return m ? m.replace(/^conversation\./, "") : "—";
  }
}

/**
 * @returns {{
 *   stateLabel: string,
 *   stateKind: string,
 *   progress: Record<string,string>,
 *   kv: Record<string,string>,
 *   chips: { tools:number, fallback:number },
 *   ingestRunIds: string[],
 *   witness: { request:boolean, response:boolean }
 * }}
 */
function buildConversationCardModel(events, getFlat) {
  events = Array.isArray(events) ? events : [];
  getFlat = typeof getFlat === "function" ? getFlat : function (p) { return (p && p.rawFlat) || {}; };

  var sorted = sortEventsForModel(events);
  var progress = {
    received: "pending",
    routed: "pending",
    rag: "pending",
    upstream: "pending",
    delivered: "pending"
  };
  var kv = {
    turnIndex: "",
    clientModel: "",
    upstreamModel: "",
    stream: "",
    ragCollection: "",
    mergeHint: ""
  };
  var lastLifecycleMsg = "";
  var dedupShort = false;
  var witnessReq = false;
  var witnessResp = false;
  /** Last-wins for RAG step: done | skipped | pending */
  var ragLast = null;
  /** Last-wins upstream: done | failed | pending */
  var upstreamLast = null;

  var i;
  for (i = 0; i < sorted.length; i++) {
    var f = getFlat(sorted[i].parsed);
    var m = convNormMsg(f);
    if (m === "conversation.request.witness") {
      witnessReq = true;
      continue;
    }
    if (m === "conversation.response.witness") {
      witnessResp = true;
      continue;
    }
    if (m === "conversation.payload.sample") {
      continue;
    }
    if (m.indexOf("conversation.") !== 0) continue;
    lastLifecycleMsg = m;

    if (f.turn_index != null && !isNaN(Number(f.turn_index))) kv.turnIndex = String(Math.round(Number(f.turn_index)));

    if (m === "conversation.received") {
      progress.received = "done";
      if (f.clientModel != null && String(f.clientModel).trim() !== "") kv.clientModel = String(f.clientModel).trim();
      if (f.stream !== undefined && f.stream !== null) kv.stream = f.stream ? "stream" : "batch";
    }
    if (m === "conversation.merged") {
      progress.received = "done";
      kv.mergeHint = "merged";
    }
    if (m === "conversation.dedup_hit") {
      progress.received = "done";
      dedupShort = true;
      kv.mergeHint = "dedup";
    }
    if (m === "conversation.routing.resolved") {
      progress.routed = "done";
      if (f.upstreamModel != null && String(f.upstreamModel).trim() !== "") kv.upstreamModel = String(f.upstreamModel).trim();
    }
    if (m === "conversation.rag.span") {
      if (f.collection != null && String(f.collection).trim() !== "") kv.ragCollection = String(f.collection).trim();
      ragLast = "done";
    }
    if (m === "conversation.rag.skipped") {
      ragLast = "skipped";
    }
    if (m === "conversation.rag.attached") {
      if (f.collection != null && String(f.collection).trim() !== "") kv.ragCollection = String(f.collection).trim();
      ragLast = "done";
    }
    if (m === "conversation.upstream.started") {
      upstreamLast = "pending";
      if (f.upstreamModel != null && String(f.upstreamModel).trim() !== "") kv.upstreamModel = String(f.upstreamModel).trim();
    }
    if (m === "conversation.upstream.completed") {
      upstreamLast = "done";
      if (f.upstreamModel != null && String(f.upstreamModel).trim() !== "") kv.upstreamModel = String(f.upstreamModel).trim();
    }
    if (m === "conversation.upstream.failed") {
      upstreamLast = "failed";
      if (f.upstreamModel != null && String(f.upstreamModel).trim() !== "") kv.upstreamModel = String(f.upstreamModel).trim();
    }
    if (m === "conversation.delivered") progress.delivered = "done";
    if (m === "conversation.errored") progress.delivered = "failed";
  }

  if (dedupShort) {
    progress.routed = progress.routed === "done" ? "done" : "skipped";
    progress.rag = "skipped";
    progress.upstream = "skipped";
  } else {
    progress.rag = ragLast != null ? ragLast : "skipped";
    if (progress.rag === "pending") progress.rag = "done";
    progress.upstream = upstreamLast != null ? upstreamLast : "skipped";
  }

  var stateLabel = lastLifecycleMsg ? lifecycleLabelForMsg(lastLifecycleMsg) : "—";
  var stateKind = "complete";
  if (
    lastLifecycleMsg === "conversation.errored" ||
    lastLifecycleMsg === "conversation.upstream.failed" ||
    lastLifecycleMsg === "conversation.fallback.exhausted"
  ) {
    stateKind = "error";
  } else if (
    lastLifecycleMsg === "conversation.fallback.attempted" ||
    lastLifecycleMsg === "conversation.fallback.model_not_found"
  ) {
    stateKind = "warn";
  }

  var toolsN = 0;
  var fbN = 0;
  var ingestSeen = {};
  var ingestRunIds = [];
  for (i = 0; i < events.length; i++) {
    var ev = events[i];
    var ff = getFlat(ev.parsed);
    var mm = convNormMsg(ff);
    if (mm === "conversation.tool.call_completed" || mm === "conversation.tool.call_failed") {
      toolsN++;
    }
    if (mm.indexOf("chat.tool_router.") === 0) toolsN++;
    if (mm.indexOf("conversation.fallback.") === 0 || mm === "chat.routing.fallback" || mm === "chat.routing.model_not_found") fbN++;
    var tier = ev.convJoinTier != null ? String(ev.convJoinTier) : "";
    if (tier === "ingest") {
      var ir = ff.index_run_id != null ? String(ff.index_run_id).trim() : "";
      if (ir && !ingestSeen[ir]) {
        ingestSeen[ir] = true;
        ingestRunIds.push(ir);
      }
    }
  }

  return {
    stateLabel: stateLabel,
    stateKind: stateKind,
    progress: progress,
    kv: kv,
    chips: {
      tools: toolsN,
      fallback: fbN
    },
    ingestRunIds: ingestRunIds,
    witness: { request: witnessReq, response: witnessResp }
  };
}

globalThis.ClaudiaLogs = globalThis.ClaudiaLogs || {};
globalThis.ClaudiaLogs.Derive = globalThis.ClaudiaLogs.Derive || {};
globalThis.ClaudiaLogs.Derive.conversationRequestIdTier2Eligible = conversationRequestIdTier2Eligible;
globalThis.ClaudiaLogs.Derive.conversationIndexRunTier3Eligible = conversationIndexRunTier3Eligible;
globalThis.ClaudiaLogs.Derive.extractConversationQdrantJoinAnchors = extractConversationQdrantJoinAnchors;
globalThis.ClaudiaLogs.Derive.joinQdrantLineConversationMatch = joinQdrantLineConversationMatch;
globalThis.ClaudiaLogs.Derive.joinQdrantLineConversationTier = joinQdrantLineConversationTier;
globalThis.ClaudiaLogs.Derive.conversationTurnGroupsForExpanded = conversationTurnGroupsForExpanded;
globalThis.ClaudiaLogs.Derive.buildConversationCardModel = buildConversationCardModel;
