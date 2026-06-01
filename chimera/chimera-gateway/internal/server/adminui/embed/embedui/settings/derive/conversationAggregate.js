/**
 * Conversation grouping for summarized feed aggregation (entryCache → groups).
 * Pure functions; no DOM. Used by summarized/aggregate.js and re-exported on ctx from feedLogConv.
 */
globalThis.ChimeraSettings = globalThis.ChimeraSettings || {};
globalThis.ChimeraSettings.Derive = globalThis.ChimeraSettings.Derive || {};

(function () {
  function convEventDedupeKey(ent) {
    if (ent.seq != null && ent.seq !== "") return "s:" + String(ent.seq);
    return "t:" + String(ent.ts) + ":" + String(ent.text || "").slice(0, 80);
  }

  function pushConversationGroupedEvent(groups, pidUse, cidUse, ent, p, tier, meta) {
    if (!cidUse) return;
    if (!pidUse) pidUse = "(unknown principal)";
    var keyC = pidUse + "\0" + cidUse;
    if (!groups[keyC]) groups[keyC] = { pid: pidUse, cid: cidUse, events: [] };
    var g = groups[keyC];
    var dk = convEventDedupeKey(ent);
    for (var ei = 0; ei < g.events.length; ei++) {
      if (convEventDedupeKey(g.events[ei]) === dk) return;
    }
    var outEv = {
      parsed: p,
      text: ent.text || "",
      ts: ent.ts,
      seq: ent.seq,
      convJoinTier: tier
    };
    if (meta && typeof meta === "object") {
      if (meta.span_id) outEv.vectorstoreSpanID = String(meta.span_id);
      if (meta.turn_index != null) outEv.vectorstoreTurnIndex = meta.turn_index;
      if (meta.span_start_ms != null) outEv.vectorstoreSpanStartMs = meta.span_start_ms;
    }
    g.events.push(outEv);
  }

  function entryIsVectorstoreSubprocessForConvJoin(ent, getFlat) {
    getFlat = typeof getFlat === "function" ? getFlat : function (parsed) { return (parsed && parsed.rawFlat) || {}; };
    var f = getFlat(ent.parsed);
    if (String(f.service || "").toLowerCase() !== "chimera-vectorstore") return false;
    var msg = String(f.msg != null ? f.msg : "").toLowerCase();
    return msg.indexOf("chimera-vectorstore.") === 0;
  }

  function conversationRequestIdTier2EligibleLocal(f) {
    if (
      globalThis.ChimeraSettings &&
      ChimeraSettings.Derive &&
      typeof ChimeraSettings.Derive.conversationRequestIdTier2Eligible === "function"
    ) {
      return ChimeraSettings.Derive.conversationRequestIdTier2Eligible(f);
    }
    return (
      globalThis.ChimeraSettings &&
      ChimeraSettings.Derive &&
      ChimeraSettings.Derive.conversationChimeraBrokerTimelineFlat &&
      ChimeraSettings.Derive.conversationChimeraBrokerTimelineFlat(f)
    );
  }

  function conversationIndexRunTier3EligibleLocal(f) {
    if (
      globalThis.ChimeraSettings &&
      ChimeraSettings.Derive &&
      typeof ChimeraSettings.Derive.conversationIndexRunTier3Eligible === "function"
    ) {
      return ChimeraSettings.Derive.conversationIndexRunTier3Eligible(f);
    }
    return false;
  }

  function tryRegisterRequestConversationCorrelationPrimary(reqToConv, f) {
    if (!f || typeof f !== "object") return;
    var rid = f.request_id != null ? String(f.request_id).trim() : "";
    var cid = f.conversation_id != null ? String(f.conversation_id).trim() : "";
    var pid = f.principal_id != null ? String(f.principal_id).trim() : f.tenant != null ? String(f.tenant).trim() : "";
    if (!rid || !cid || !pid || reqToConv[rid]) return;
    var msg = String(f.msg != null ? f.msg : f.message != null ? f.message : "").trim();
    if (msg === "conversation.received" || msg === "chat.request") {
      reqToConv[rid] = { pid: pid, cid: cid };
      return;
    }
    var ml = msg.toLowerCase();
    if (ml === "gateway.http.access" || ml === "http response") {
      var pth = String(f.path || "").split("?")[0];
      if (pth.indexOf("/v1/chat/completions") >= 0) {
        reqToConv[rid] = { pid: pid, cid: cid };
      }
    }
  }

  function tryRegisterRequestConversationCorrelationRagFallback(reqToConv, f) {
    if (!f || typeof f !== "object") return;
    var rid = f.request_id != null ? String(f.request_id).trim() : "";
    var cid = f.conversation_id != null ? String(f.conversation_id).trim() : "";
    var pid = f.principal_id != null ? String(f.principal_id).trim() : f.tenant != null ? String(f.tenant).trim() : "";
    if (!rid || !cid || !pid || reqToConv[rid]) return;
    var msg = String(f.msg != null ? f.msg : f.message != null ? f.message : "").trim();
    if (msg === "rag.query" || msg === "rag.embed") {
      reqToConv[rid] = { pid: pid, cid: cid };
    }
  }

  function convLastTs(g, entryInstant) {
    var mx = 0;
    for (var u = 0; u < g.events.length; u++) {
      var ti = entryInstant({ ts: g.events[u].ts });
      if (ti) mx = Math.max(mx, ti.getTime());
    }
    return mx;
  }

  function convFirstTs(g, entryInstant) {
    var mn = null;
    for (var u = 0; u < g.events.length; u++) {
      var ti = entryInstant({ ts: g.events[u].ts });
      if (ti) {
        if (mn == null || ti.getTime() < mn.getTime()) mn = ti;
      }
    }
    return mn ? mn.getTime() : 0;
  }

  function sortEventsChronologically(events, entryInstant) {
    events.sort(function (a, b) {
      var sa = a.seq != null ? Number(a.seq) : 0;
      var sb = b.seq != null ? Number(b.seq) : 0;
      if (sa !== sb) return sa - sb;
      var ta = entryInstant({ ts: a.ts });
      var tb = entryInstant({ ts: b.ts });
      if (!ta && !tb) return 0;
      if (!ta) return -1;
      if (!tb) return 1;
      return ta.getTime() - tb.getTime();
    });
  }

  /**
   * One conversation card per gateway group key (principal + conversation_id).
   */
  function sortConversationGroupsByRecency(groups, entryInstant) {
    var arr = [];
    for (var key in groups) {
      if (!Object.prototype.hasOwnProperty.call(groups, key)) continue;
      var gx = groups[key];
      var tmin = convFirstTs(gx, entryInstant);
      var tmax = convLastTs(gx, entryInstant);
      if (!tmax) continue;
      if (!tmin) tmin = tmax;
      arr.push({
        pid: gx.pid,
        cid: gx.cid,
        cids: [gx.cid],
        events: gx.events.slice(),
        tmin: tmin,
        tmax: tmax
      });
    }
    arr.sort(function (a, b) {
      return b.tmax - a.tmax;
    });
    for (var k = 0; k < arr.length; k++) {
      sortEventsChronologically(arr[k].events, entryInstant);
    }
    return arr;
  }

  var D = globalThis.ChimeraSettings.Derive;
  D.pushConversationGroupedEvent = pushConversationGroupedEvent;
  D.entryIsVectorstoreSubprocessForConvJoin = entryIsVectorstoreSubprocessForConvJoin;
  D.conversationRequestIdTier2EligibleLocal = conversationRequestIdTier2EligibleLocal;
  D.conversationIndexRunTier3EligibleLocal = conversationIndexRunTier3EligibleLocal;
  D.tryRegisterRequestConversationCorrelationPrimary = tryRegisterRequestConversationCorrelationPrimary;
  D.tryRegisterRequestConversationCorrelationRagFallback = tryRegisterRequestConversationCorrelationRagFallback;
  D.convLastTs = convLastTs;
  D.convFirstTs = convFirstTs;
  D.sortConversationGroupsByRecency = sortConversationGroupsByRecency;
  D.sortEventsChronologically = sortEventsChronologically;
})();
