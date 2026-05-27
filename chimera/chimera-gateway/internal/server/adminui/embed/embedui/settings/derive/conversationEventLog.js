/**
 * Conversation scoped event-log: hide noisy rows and attach per-turn metadata for formatters.
 */
(function () {
  globalThis.ChimeraSettings = globalThis.ChimeraSettings || {};
  globalThis.ChimeraSettings.Derive = globalThis.ChimeraSettings.Derive || {};

  function flatMsg(f) {
    if (!f || typeof f !== "object") return "";
    var raw = f.msg != null ? f.msg : f.message != null ? f.message : "";
    return String(raw).trim();
  }

  function brokerShortModel(model) {
    var m = model != null ? String(model).trim() : "";
    if (!m) return "";
    var parts = m.split("/");
    var tail = parts[parts.length - 1] || m;
    return tail.length > 48 ? tail.slice(0, 46) + "…" : tail;
  }

  function virtualModelIdFromUi() {
    var cache = globalThis.gatewayOverviewCache;
    if (cache && cache.virtual_model_id != null && String(cache.virtual_model_id).trim() !== "") {
      return String(cache.virtual_model_id).trim();
    }
    return "";
  }

  function sortEventsAsc(events) {
    return events.slice().sort(function (a, b) {
      var sa = a.seq != null ? Number(a.seq) : 0;
      var sb = b.seq != null ? Number(b.seq) : 0;
      if (sa !== sb) return sa - sb;
      var ta = Date.parse(a.ts || "");
      var tb = Date.parse(b.ts || "");
      if (!isFinite(ta) && !isFinite(tb)) return 0;
      if (!isFinite(ta)) return -1;
      if (!isFinite(tb)) return 1;
      return ta - tb;
    });
  }

  function turnHasMsg(sortedAsc, getFlat, slug) {
    for (var i = 0; i < sortedAsc.length; i++) {
      if (flatMsg(getFlat(sortedAsc[i].parsed)) === slug) return true;
    }
    return false;
  }

  function convEvlogDisplayRank(flat) {
    var msg = flatMsg(flat);
    if (msg === "conversation.delivered") return 100;
    if (msg === "conversation.errored") return 99;
    return 0;
  }

  function sortPreparedChronological(prepared, getFlat) {
    getFlat = typeof getFlat === "function" ? getFlat : function (p) { return (p && p.rawFlat) || {}; };
    return prepared.slice().sort(function (a, b) {
      var fa = getFlat(a.parsed);
      var fb = getFlat(b.parsed);
      var sa = a.seq != null ? Number(a.seq) : 0;
      var sb = b.seq != null ? Number(b.seq) : 0;
      if (sa !== sb) return sa - sb;
      var ta = Date.parse(a.ts || "");
      var tb = Date.parse(b.ts || "");
      if (!isFinite(ta) && !isFinite(tb)) {
        return convEvlogDisplayRank(fa) - convEvlogDisplayRank(fb);
      }
      if (!isFinite(ta)) return -1;
      if (!isFinite(tb)) return 1;
      if (ta !== tb) return ta - tb;
      return convEvlogDisplayRank(fa) - convEvlogDisplayRank(fb);
    });
  }

  function repositionRoutingBeforeSuccessfulSend(prepared, getFlat) {
    getFlat = typeof getFlat === "function" ? getFlat : function (p) { return (p && p.rawFlat) || {}; };
    var routingIdx = -1;
    for (var i = 0; i < prepared.length; i++) {
      if (flatMsg(getFlat(prepared[i].parsed)) === "conversation.routing.resolved") {
        routingIdx = i;
        break;
      }
    }
    if (routingIdx < 0) return prepared;

    var lastOkRequestIdx = -1;
    for (var j = prepared.length - 1; j >= 0; j--) {
      var fj = getFlat(prepared[j].parsed);
      var mj = flatMsg(fj);
      if (mj !== "chat.chimera-broker.response" && mj !== "upstream chat response") continue;
      var sc = Number(fj.statusCode != null ? fj.statusCode : fj.status_code);
      if (isNaN(sc) || sc < 200 || sc > 299) continue;
      for (var k = j - 1; k >= 0; k--) {
        if (flatMsg(getFlat(prepared[k].parsed)) === "chat.chimera-broker.request") {
          lastOkRequestIdx = k;
          break;
        }
      }
      if (lastOkRequestIdx >= 0) break;
    }
    if (lastOkRequestIdx < 0 || routingIdx <= lastOkRequestIdx) return prepared;

    var out = prepared.slice();
    var routingEv = out.splice(routingIdx, 1)[0];
    out.splice(lastOkRequestIdx, 0, routingEv);
    return out;
  }

  function routingSummaryFromTurnEvents(sortedAsc, getFlat) {
    var skipped = [];
    var upstream = "";
    var attempt = NaN;
    var chainLen = NaN;
    var clientModel = "";
    var outgoingTokens = NaN;
    var i;
    for (i = 0; i < sortedAsc.length; i++) {
      var f = getFlat(sortedAsc[i].parsed);
      var msg = flatMsg(f).toLowerCase();
      if (msg === "conversation.received" && f.clientModel != null) {
        clientModel = String(f.clientModel).trim();
      }
      if (msg === "chat.request" && !clientModel && f.clientModel != null) {
        clientModel = String(f.clientModel).trim();
      }
      if (msg === "chat.chimera-broker.request") {
        var ot = Number(f.outgoingTokens != null ? f.outgoingTokens : f.outgoing_tokens);
        if (!isNaN(ot) && ot > 0) outgoingTokens = ot;
      }
      if (msg === "conversation.routing.resolved") {
        if (f.upstreamModel != null) upstream = String(f.upstreamModel).trim();
        if (f.clientModel != null && !clientModel) clientModel = String(f.clientModel).trim();
        if (f.attempt != null) attempt = Number(f.attempt);
        if (f.chainLen != null) chainLen = Number(f.chainLen);
      }
      if (msg === "chat.provider_limits.blocked") {
        var sm = brokerShortModel(f.upstreamModel);
        var rsn = f.reason != null ? String(f.reason).trim().toLowerCase() : "quota";
        if (sm) skipped.push({ model: sm, reason: rsn });
      }
    }
    return {
      skipped: skipped,
      upstream: upstream,
      attempt: attempt,
      chainLen: chainLen,
      clientModel: clientModel,
      virtualModelId: virtualModelIdFromUi(),
      outgoingTokens: outgoingTokens
    };
  }

  function turnHasMerged(sortedAsc, getFlat) {
    for (var i = 0; i < sortedAsc.length; i++) {
      if (flatMsg(getFlat(sortedAsc[i].parsed)) === "conversation.merged") return true;
    }
    return false;
  }

  function turnIndexFromEvents(sortedAsc, getFlat) {
    for (var i = sortedAsc.length - 1; i >= 0; i--) {
      var f = getFlat(sortedAsc[i].parsed);
      if (f.turn_index != null && !isNaN(Number(f.turn_index))) return Math.round(Number(f.turn_index));
    }
    return null;
  }

  /** Slugs omitted from conversation-card event log (still in raw logs / service panels). */
  function convEvlogHideRow(flat, turnCtx) {
    turnCtx = turnCtx || {};
    var msg = flatMsg(flat);
    if (!msg) return false;
    var ml = msg.toLowerCase();
    if (msg === "conversation.merged") return true;
    if (msg === "chat.request") return true;
    if (msg === "conversation.request.witness") return true;
    if (msg === "conversation.response.witness") return true;
    if (msg === "conversation.broker.started") return true;
    if (msg === "conversation.broker.completed") return true;
    if (msg === "conversation.broker.failed") return true;
    if (msg === "chat.routing.resolved") return true;
    if (msg === "chat.routing.attempt") return true;
    if (msg === "chat.provider_limits.blocked") return true;
    if (msg === "chat.routing.fallback") return true;
    if (msg === "conversation.fallback.attempted") return true;
    if (msg === "chat.routing.model_not_found") return true;
    if (msg === "chat.routing.rate_limit") return true;
    if (msg === "conversation.rag.span" && turnCtx.hasRagAttached) return true;
    if (ml === "virtual model routing resolved" || ml === "virtual model fallback attempt") return true;
    if (msg === "gateway.http.access" || ml === "http response") {
      var pth = String(flat.path || "").split("?")[0];
      if (pth.indexOf("/v1/chat/completions") >= 0) return true;
    }
    return false;
  }

  /**
   * Filter turn events for conversation evlog and attach convEvlogMeta for formatters.
   * @param {Array} events ascending seq/ts within one turn
   * @param {function} getFlat
   * @returns {Array}
   */
  function convEvlogPrepareTurnEvents(events, getFlat) {
    events = Array.isArray(events) ? events : [];
    getFlat = typeof getFlat === "function" ? getFlat : function (p) { return (p && p.rawFlat) || {}; };
    var sorted = sortEventsAsc(events);
    var hasRagAttached = turnHasMsg(sorted, getFlat, "conversation.rag.attached");
    var ragCoords = null;
    if (ChimeraSettings.Derive.extractRagCoordsFromEvents) {
      ragCoords = ChimeraSettings.Derive.extractRagCoordsFromEvents(sorted, getFlat);
    }
    var turnCtx = { hasRagAttached: hasRagAttached };
    var routingSummary = routingSummaryFromTurnEvents(sorted, getFlat);
    var merged = turnHasMerged(sorted, getFlat);
    var turnIndex = turnIndexFromEvents(sorted, getFlat);
    var isNewConversation = turnIndex === 1 && !merged;
    var chainLen = routingSummary.chainLen;
    var clientModel = routingSummary.clientModel;
    var upstream = routingSummary.upstream;
    var virtualId = routingSummary.virtualModelId;
    var isPassthrough =
      (clientModel && virtualId && clientModel !== virtualId && clientModel === upstream) ||
      (chainLen <= 1 && clientModel && upstream && clientModel === upstream);
    var meta = {
      routingSummary: routingSummary,
      turnIndex: turnIndex,
      isNewConversation: isNewConversation,
      isPassthrough: isPassthrough,
      ragCoords: ragCoords
    };
    var out = [];
    for (var i = 0; i < sorted.length; i++) {
      var ev = sorted[i];
      var f = getFlat(ev.parsed);
      if (convEvlogHideRow(f, turnCtx)) continue;
      if (meta.isPassthrough && flatMsg(f) === "chat.chimera-broker.request") continue;
      var copy = {};
      for (var k in ev) {
        if (Object.prototype.hasOwnProperty.call(ev, k)) copy[k] = ev[k];
      }
      copy.convEvlogMeta = meta;
      out.push(copy);
    }
    var prepared = sortPreparedChronological(out, getFlat);
    return repositionRoutingBeforeSuccessfulSend(prepared, getFlat);
  }

  /**
   * Prepare all conversation card events: per-turn filter when turn groups exist.
   */
  function convEvlogPrepareConvEvents(evs, turnGroups, getFlat) {
    getFlat = typeof getFlat === "function" ? getFlat : function (p) { return (p && p.rawFlat) || {}; };
    if (turnGroups && turnGroups.length > 1) {
      var all = [];
      for (var tgi = 0; tgi < turnGroups.length; tgi++) {
        var prepared = convEvlogPrepareTurnEvents(turnGroups[tgi].events, getFlat);
        for (var pi = 0; pi < prepared.length; pi++) all.push(prepared[pi]);
      }
      return all;
    }
    return convEvlogPrepareTurnEvents(evs, getFlat);
  }

  ChimeraSettings.Derive.convEvlogHideRow = convEvlogHideRow;
  ChimeraSettings.Derive.convEvlogDisplayRank = convEvlogDisplayRank;
  ChimeraSettings.Derive.convEvlogPrepareTurnEvents = convEvlogPrepareTurnEvents;
  ChimeraSettings.Derive.convEvlogPrepareConvEvents = convEvlogPrepareConvEvents;
})();
