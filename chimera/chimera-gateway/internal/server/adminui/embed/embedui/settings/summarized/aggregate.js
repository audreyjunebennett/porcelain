/**
 * entryCache → summarized aggregate (groups, buckets, byRun). No HTML.
 */
globalThis.ChimeraSettings = globalThis.ChimeraSettings || {};
globalThis.ChimeraSettings.Summarized = globalThis.ChimeraSettings.Summarized || {};

(function () {
  function indexerGroupIdForFlat(fR) {
    if (
      globalThis.ChimeraSettings &&
      ChimeraSettings.Derive &&
      typeof ChimeraSettings.Derive.indexerGroupKeyFromFlat === "function"
    ) {
      var gx = ChimeraSettings.Derive.indexerGroupKeyFromFlat(fR);
      if (gx != null && String(gx).trim() !== "") return String(gx).trim();
    }
    var itk =
      fR.indexer_target_key != null && String(fR.indexer_target_key).trim() !== ""
        ? String(fR.indexer_target_key).trim()
        : "";
    var ik =
      fR.indexer_key != null && String(fR.indexer_key).trim() !== "" ? String(fR.indexer_key).trim() : "";
    var rid = fR.index_run_id != null && fR.index_run_id !== "" ? String(fR.index_run_id) : "";
    return itk || ik || rid || "";
  }

  /**
   * @param {Array} entryCache
   * @param {{
   *   getFlat: function,
   *   entryInstant: function,
   *   normalizeServiceBucketKey: function,
   *   collectIndexerRunMeta: function,
   *   indexerRootScopeByRootId: object,
   *   operatorWsFullLogCtx: object,
   *   lastIndexerOperatorWorkspacesNested: Array
   * }} deps
   */
  function buildAggregateState(entryCache, deps) {
    deps = deps || {};
    var getFlat = deps.getFlat;
    var entryInstant = deps.entryInstant;
    var normalizeServiceBucketKey = deps.normalizeServiceBucketKey;
    var collectIndexerRunMeta = deps.collectIndexerRunMeta;
    var D = globalThis.ChimeraSettings && ChimeraSettings.Derive ? ChimeraSettings.Derive : {};
    entryCache = Array.isArray(entryCache) ? entryCache : [];

    var scopeState = {
      indexerRootScopeByRootId: deps.indexerRootScopeByRootId || {},
      operatorWsFullLogCtx: deps.operatorWsFullLogCtx || {},
      lastIndexerOperatorWorkspacesNested: deps.lastIndexerOperatorWorkspacesNested || []
    };
    if (typeof D.rebuildIndexerRootScopeMaps === "function") {
      D.rebuildIndexerRootScopeMaps(entryCache, getFlat, scopeState.indexerRootScopeByRootId);
    }

    var groups = {};
    var reqToConv = {};
    var indexRunToConv = {};
    var gix;

    for (gix = 0; gix < entryCache.length; gix++) {
      if (typeof D.tryRegisterRequestConversationCorrelationPrimary === "function") {
        D.tryRegisterRequestConversationCorrelationPrimary(reqToConv, getFlat(entryCache[gix].parsed));
      }
    }
    for (gix = 0; gix < entryCache.length; gix++) {
      if (typeof D.tryRegisterRequestConversationCorrelationRagFallback === "function") {
        D.tryRegisterRequestConversationCorrelationRagFallback(reqToConv, getFlat(entryCache[gix].parsed));
      }
    }
    for (gix = 0; gix < entryCache.length; gix++) {
      var entIr = entryCache[gix];
      var fIr = getFlat(entIr.parsed);
      var msgIr = String(fIr.msg != null ? fIr.msg : fIr.message != null ? fIr.message : "").trim();
      if (msgIr !== "ingest.complete" && msgIr !== "ingest.failed" && msgIr !== "ingest.chunked.error") continue;
      var irKey = fIr.index_run_id != null ? String(fIr.index_run_id).trim() : "";
      var cidIr = fIr.conversation_id != null ? String(fIr.conversation_id).trim() : "";
      var pidIr =
        fIr.principal_id != null ? String(fIr.principal_id).trim() : fIr.tenant != null ? String(fIr.tenant).trim() : "";
      if (irKey && cidIr && pidIr && !indexRunToConv[irKey]) indexRunToConv[irKey] = { pid: pidIr, cid: cidIr };
    }
    for (gix = 0; gix < entryCache.length; gix++) {
      var ent = entryCache[gix];
      var p = ent.parsed;
      var f = getFlat(p);
      var cid = f.conversation_id != null ? String(f.conversation_id).trim() : "";
      var pid = f.principal_id != null ? String(f.principal_id).trim() : f.tenant != null ? String(f.tenant).trim() : "";
      if (cid) {
        if (typeof D.pushConversationGroupedEvent === "function") {
          D.pushConversationGroupedEvent(groups, pid, cid, ent, p, "direct");
        }
        continue;
      }
      var ridJoin = f.request_id != null ? String(f.request_id).trim() : "";
      if (
        ridJoin &&
        reqToConv[ridJoin] &&
        typeof D.conversationRequestIdTier2EligibleLocal === "function" &&
        D.conversationRequestIdTier2EligibleLocal(f) &&
        typeof D.pushConversationGroupedEvent === "function"
      ) {
        D.pushConversationGroupedEvent(groups, reqToConv[ridJoin].pid, reqToConv[ridJoin].cid, ent, p, "request_id");
        continue;
      }
      var irJoin = f.index_run_id != null ? String(f.index_run_id).trim() : "";
      if (
        irJoin &&
        indexRunToConv[irJoin] &&
        typeof D.conversationIndexRunTier3EligibleLocal === "function" &&
        D.conversationIndexRunTier3EligibleLocal(f) &&
        typeof D.pushConversationGroupedEvent === "function"
      ) {
        D.pushConversationGroupedEvent(
          groups,
          indexRunToConv[irJoin].pid,
          indexRunToConv[irJoin].cid,
          ent,
          p,
          "ingest"
        );
      }
    }
    var gkSort;
    for (gkSort in groups) {
      if (!Object.prototype.hasOwnProperty.call(groups, gkSort)) continue;
      if (typeof D.sortEventsChronologically === "function") {
        D.sortEventsChronologically(groups[gkSort].events, entryInstant);
      }
    }
    if (typeof D.joinVectorstoreLineConversationTier === "function") {
      for (gix = 0; gix < entryCache.length; gix++) {
        var entQ = entryCache[gix];
        if (typeof D.entryIsVectorstoreSubprocessForConvJoin !== "function" || !D.entryIsVectorstoreSubprocessForConvJoin(entQ, getFlat)) {
          continue;
        }
        var fQ = getFlat(entQ.parsed);
        var collQ = fQ.collection != null ? String(fQ.collection).trim() : "";
        if (!collQ) continue;
        var tQ = entryInstant({ ts: entQ.ts });
        if (!tQ) continue;
        var tMs = tQ.getTime();
        var gkQ;
        for (gkQ in groups) {
          if (!Object.prototype.hasOwnProperty.call(groups, gkQ)) continue;
          var grp = groups[gkQ];
          var qMatch = null;
          if (typeof D.joinVectorstoreLineConversationMatch === "function") {
            qMatch = D.joinVectorstoreLineConversationMatch(grp.events, getFlat, fQ, tMs);
          }
          var tierQ =
            qMatch && qMatch.tier
              ? qMatch.tier
              : D.joinVectorstoreLineConversationTier(grp.events, getFlat, fQ, tMs);
          if (tierQ && typeof D.pushConversationGroupedEvent === "function") {
            D.pushConversationGroupedEvent(groups, grp.pid, grp.cid, entQ, entQ.parsed, tierQ, qMatch);
          }
        }
      }
      for (gkSort in groups) {
        if (!Object.prototype.hasOwnProperty.call(groups, gkSort)) continue;
        if (typeof D.sortEventsChronologically === "function") {
          D.sortEventsChronologically(groups[gkSort].events, entryInstant);
        }
      }
    }

    var buckets = {
      "chimera-gateway": [],
      "chimera-vectorstore": [],
      "chimera-broker": [],
      "chimera-indexer": []
    };
    for (var bi = 0; bi < entryCache.length; bi++) {
      var entB = entryCache[bi];
      var pB = entB.parsed;
      var fB = getFlat(pB);
      var svcKey = "";
      if (typeof D.entryRoutesToChimeraBrokerBucket === "function" && D.entryRoutesToChimeraBrokerBucket(entB, getFlat)) {
        svcKey = "chimera-broker";
      } else if (typeof D.entryIsVectorstoreLine === "function" && D.entryIsVectorstoreLine(entB, getFlat)) {
        svcKey = "chimera-vectorstore";
      } else if (typeof D.entryIsIndexerLine === "function" && D.entryIsIndexerLine(entB, getFlat)) {
        svcKey = "chimera-indexer";
      } else {
        svcKey = normalizeServiceBucketKey(fB.service, entB.source);
        if (!svcKey) svcKey = "chimera-gateway";
      }
      if (!buckets[svcKey]) buckets[svcKey] = [];
      buckets[svcKey].push(entB);
    }

    var byRun = {};
    var partitionRegistry = {};
    var ibuilt = null;
    if (typeof D.indexerBucketsFromCache === "function") {
      ibuilt = D.indexerBucketsFromCache(entryCache, getFlat);
      if (ibuilt && ibuilt.targetStateByRunId) partitionRegistry = ibuilt.targetStateByRunId;
      if (ibuilt && ibuilt.buckets) byRun = ibuilt.buckets;
    }
    if (!ibuilt) {
      byRun = {};
      partitionRegistry = {};
      for (var ri = 0; ri < entryCache.length; ri++) {
        var entRL = entryCache[ri];
        var fRL = getFlat(entRL.parsed);
        if (typeof D.indexerFlatMsgForPresent === "function") {
          var msgRL = D.indexerFlatMsgForPresent(fRL);
          if (msgRL === "indexer.state") continue;
          if (msgRL === "indexer.storage.stats" || msgRL.indexOf("indexer.storage.stats") === 0) continue;
        }
        var groupIdL = indexerGroupIdForFlat(fRL);
        if (!groupIdL) continue;
        if (!byRun[groupIdL]) byRun[groupIdL] = { id: groupIdL, events: [] };
        byRun[groupIdL].events.push(entRL);
      }
    } else {
      for (var normK in byRun) {
        if (!Object.prototype.hasOwnProperty.call(byRun, normK)) continue;
        var arrN = byRun[normK];
        byRun[normK] = { id: normK, events: arrN };
      }
    }

    var qFan = buckets["chimera-vectorstore"];
    if (qFan && qFan.length && byRun && Object.keys(byRun).length && typeof D.indexerExpectedVectorstoreCollectionForBucket === "function") {
      var collByRun = {};
      var buck;
      for (buck in byRun) {
        if (!Object.prototype.hasOwnProperty.call(byRun, buck)) continue;
        var runB = byRun[buck];
        collByRun[buck] = D.indexerExpectedVectorstoreCollectionForBucket(
          runB.id,
          runB.events,
          partitionRegistry,
          entryCache,
          getFlat,
          scopeState
        );
      }
      var qx, qb;
      for (qx = 0; qx < qFan.length; qx++) {
        var qEnt = qFan[qx];
        var qFl = getFlat(qEnt.parsed);
        var qCol = qFl.collection != null ? String(qFl.collection).trim() : "";
        if (!qCol) continue;
        for (qb in byRun) {
          if (!Object.prototype.hasOwnProperty.call(byRun, qb)) continue;
          if (collByRun[qb] === qCol) byRun[qb].events.push(qEnt);
        }
      }
      for (var qs in byRun) {
        if (!Object.prototype.hasOwnProperty.call(byRun, qs)) continue;
        if (typeof D.sortEventsChronologically === "function") {
          D.sortEventsChronologically(byRun[qs].events, entryInstant);
        }
      }
    }

    var gFan = buckets["chimera-gateway"];
    if (gFan && gFan.length && byRun && Object.keys(byRun).length && typeof collectIndexerRunMeta === "function") {
      var scopeByRun = {};
      var rkx;
      var runIds = Object.keys(byRun);
      for (rkx = 0; rkx < runIds.length; rkx++) {
        var runX = byRun[runIds[rkx]];
        if (!runX || !runX.events) continue;
        var pmX = null;
        if (typeof D.indexerPartitionMetaForRun === "function") {
          pmX = D.indexerPartitionMetaForRun(partitionRegistry, runX.id, runX.events, getFlat);
        }
        var metaX = collectIndexerRunMeta(runX.id, runX.events, pmX);
        if (!metaX) continue;
        scopeByRun[runX.id] = {
          tenant: metaX.tenantId != null ? String(metaX.tenantId).trim() : "",
          project: metaX.projectId != null ? String(metaX.projectId).trim() : "",
          flavor: metaX.flavorId != null ? String(metaX.flavorId).trim() : ""
        };
      }
      var gx, gb;
      for (gx = 0; gx < gFan.length; gx++) {
        var gEnt = gFan[gx];
        var gFl = getFlat(gEnt.parsed);
        var gMsg = String(gFl.msg != null ? gFl.msg : gFl.message != null ? gFl.message : "").trim();
        if (gMsg !== "rag.retrieve.source") continue;
        var gt = String(gFl.tenant_id != null ? gFl.tenant_id : gFl.principal_id != null ? gFl.principal_id : "").trim();
        var gp = String(gFl.project_id != null ? gFl.project_id : gFl.project != null ? gFl.project : "").trim();
        var gf = String(gFl.flavor_id != null ? gFl.flavor_id : "").trim();
        if (!gt || !gp) continue;
        for (gb in byRun) {
          if (!Object.prototype.hasOwnProperty.call(byRun, gb)) continue;
          var sc = scopeByRun[gb];
          if (!sc || !sc.project) continue;
          if (
            (sc.tenant && sc.tenant !== gt) ||
            sc.project !== gp ||
            (typeof D.normalizeIndexerScopeFlavor === "function"
              ? D.normalizeIndexerScopeFlavor(sc.flavor)
              : sc.flavor) !==
              (typeof D.normalizeIndexerScopeFlavor === "function" ? D.normalizeIndexerScopeFlavor(gf) : gf)
          )
            continue;
          byRun[gb].events.push(gEnt);
        }
      }
      for (var gsort in byRun) {
        if (!Object.prototype.hasOwnProperty.call(byRun, gsort)) continue;
        if (typeof D.sortEventsChronologically === "function") {
          D.sortEventsChronologically(byRun[gsort].events, entryInstant);
        }
      }
    }

    var mergedConv =
      typeof D.sortConversationGroupsByRecency === "function"
        ? D.sortConversationGroupsByRecency(groups, entryInstant)
        : [];

    return {
      groups: groups,
      reqToConv: reqToConv,
      indexRunToConv: indexRunToConv,
      buckets: buckets,
      byRun: byRun,
      partitionRegistry: partitionRegistry,
      mergedConv: mergedConv,
      scopeState: scopeState
    };
  }

  globalThis.ChimeraSettings.Summarized.buildAggregateState = buildAggregateState;
})();
