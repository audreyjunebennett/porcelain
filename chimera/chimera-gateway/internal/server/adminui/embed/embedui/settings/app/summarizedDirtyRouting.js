/**
 * Pure dirty-card routing for live log lines (Phase 3).
 * Maps one cache entry to summarized card ids without DOM access.
 */
globalThis.ChimeraSettings = globalThis.ChimeraSettings || {};
globalThis.ChimeraSettings.Summarized = globalThis.ChimeraSettings.Summarized || {};

(function () {
  var SERVICE_BUCKET_ORDER = ["chimera-broker", "chimera-gateway", "chimera-indexer", "chimera-vectorstore"];

  function adminProviderIdsForDirtyRouting(deps) {
    if (deps && typeof deps.getAdminProviderIds === "function") {
      var visible = deps.getAdminProviderIds();
      if (visible && visible.length) return visible;
    }
    if (
      globalThis.ChimeraSettings &&
      ChimeraSettings.Providers &&
      ChimeraSettings.Providers.Catalog &&
      typeof ChimeraSettings.Providers.Catalog.providerCatalogEntries === "function"
    ) {
      var entries = ChimeraSettings.Providers.Catalog.providerCatalogEntries();
      var out = [];
      for (var ci = 0; ci < entries.length; ci++) {
        if (entries[ci] && entries[ci].id) out.push(entries[ci].id);
      }
      return out;
    }
    return [];
  }

  function flatMsg(f) {
    return String(f.msg != null ? f.msg : f.message != null ? f.message : "").trim();
  }

  function flatMsgLower(f) {
    return flatMsg(f).toLowerCase();
  }

  function flatTimelineKind(f) {
    return String(f.timeline_kind != null ? f.timeline_kind : "").trim().toLowerCase();
  }

  function flatMsgLower(f) {
    return flatMsg(f).toLowerCase();
  }

  function classify(ent, getFlat) {
    var D = globalThis.ChimeraSettings && ChimeraSettings.Derive ? ChimeraSettings.Derive : null;
    if (!D) return { relay: false, broker: false, vectorstore: false, indexer: false };
    return {
      relay: typeof D.entryIsGatewayUpstreamRelay === "function" ? D.entryIsGatewayUpstreamRelay(ent, getFlat) : false,
      broker: typeof D.entryRoutesToChimeraBrokerBucket === "function" ? D.entryRoutesToChimeraBrokerBucket(ent, getFlat) : false,
      vectorstore: typeof D.entryIsVectorstoreLine === "function" ? D.entryIsVectorstoreLine(ent, getFlat) : false,
      indexer: typeof D.entryIsIndexerLine === "function" ? D.entryIsIndexerLine(ent, getFlat) : false
    };
  }

  /** Gateway upstream relay lines bucket under chimera-broker (see derive/logLineClassification.js). */
  function entryIsGatewayUpstreamRelay(ent, getFlat) {
    return classify(ent, getFlat).relay;
  }

  function entryRoutesToChimeraBrokerBucket(ent, getFlat) {
    return classify(ent, getFlat).broker;
  }

  function entryIsVectorstoreLine(ent, getFlat) {
    return classify(ent, getFlat).vectorstore;
  }

  function entryIsIndexerLine(ent, getFlat) {
    return classify(ent, getFlat).indexer;
  }

  /** Gateway/broker/vectorstore lines that belong to an indexer run, not operator service cards. */
  function entryIsIndexerPipelineLine(f) {
    if (flatTimelineKind(f) !== "indexer") return false;
    var msg = flatMsg(f);
    if (msg === "ingest.complete" || msg === "ingest.failed" || msg.indexOf("ingest.") === 0) return true;
    if (msg === "gateway.http.access") {
      var path = String(f.path != null ? f.path : "").trim();
      return path.indexOf("/v1/ingest") === 0 || path.indexOf("/v1/indexer") === 0;
    }
    if (msg === "broker.http.access") {
      var target = String(f.http_target != null ? f.http_target : f.httpTarget != null ? f.httpTarget : "").trim();
      return target.indexOf("/v1/embeddings") >= 0;
    }
    if (
      msg.indexOf("vectorstore.http.") === 0 &&
      (msg.indexOf("points_") >= 0 ||
        msg.indexOf("collection_meta") >= 0 ||
        msg.indexOf("collection_create") >= 0 ||
        msg.indexOf("collection_index") >= 0)
    ) {
      return true;
    }
    if (msg === "vectorstore.collection.creating") return true;
    if (msg === "vectorstore.http.upsert.summary") return true;
    return false;
  }

  function scopeStatusChangeReason(f) {
    return f.change_reason != null ? String(f.change_reason).trim().toLowerCase() : "";
  }

  function indexerWorkspaceDirtyMsg(f, msg) {
    if (!msg) return false;
    if (msg === "indexer.run.start" || msg.indexOf("indexer.run.done") === 0) return true;
    if (msg === "indexer.job.skipped.summary" || msg === "indexer.job.ingested.summary") return true;
    if (msg.indexOf("indexer.ingest.gate.") === 0) return true;
    if (msg === "indexer.recovery.resumed") return true;
    if (msg.indexOf("indexer.job.failed") === 0 || msg === "indexer.work.failed") return true;
    if (msg.indexOf("indexer.discovery.") === 0 || msg === "indexer.scan.complete") return true;
    if (msg === "indexer.reconcile.summary") return true;
    if (msg.indexOf("indexer.run.progress") === 0) return true;
    if (msg === "indexer.sync_state.write_failed") return true;
    if (msg.indexOf("indexer.fanout.") === 0) return true;
    if (msg === "indexer.scope.status") {
      var cr = scopeStatusChangeReason(f);
      return cr !== "" && cr !== "heartbeat";
    }
    if (msg === "indexer.recovery.poll") {
      return f.embed_ok === false;
    }
    if (msg === "ingest.failed") return true;
    return false;
  }

  function indexerServiceCardDirtyMsg(f, msg) {
    if (!msg) return false;
    if (msg === "indexer.queue.snapshot") return false;
    if (msg === "indexer.scope.active_file") return false;
    if (msg === "indexer.job.skipped" || msg === "indexer.job.upload" || msg === "indexer.job.ingested") return false;
    if (msg.indexOf("indexer.skip.") === 0) return false;
    if (msg === "indexer.scope.status") {
      var cr = scopeStatusChangeReason(f);
      return cr !== "" && cr !== "heartbeat";
    }
    if (msg === "indexer.recovery.poll") {
      return f.embed_ok === false;
    }
    if (indexerWorkspaceDirtyMsg(f, msg)) return true;
    if (msg.indexOf("indexer.retry") === 0 || msg.indexOf("indexer.worker.paused") === 0) return true;
    if (msg === "indexer.state" || msg.indexOf("indexer.storage.") === 0) return true;
    return false;
  }

  function serviceBucketKeyForEntry(ent, deps) {
    if (entryRoutesToChimeraBrokerBucket(ent, deps.getFlat)) return "chimera-broker";
    if (entryIsVectorstoreLine(ent, deps.getFlat)) return "chimera-vectorstore";
    if (entryIsIndexerLine(ent, deps.getFlat)) return "chimera-indexer";
    var f = deps.getFlat(ent.parsed);
    var svcKey = deps.normalizeServiceBucketKey(f.service, ent.source);
    if (!svcKey) svcKey = "chimera-gateway";
    return svcKey;
  }

  function serviceCardIdForBucketKey(bucketKey, strHash) {
    return "svc-" + strHash(bucketKey);
  }

  function conversationCardIdForPrincipalAndCid(pid, cid, strHash) {
    if (!cid) return null;
    if (!pid) pid = "(unknown principal)";
    return strHash(pid + "\0" + cid);
  }

  function adminProviderIdsForEntry(ent, deps) {
    var f = deps.getFlat(ent.parsed);
    var msgEv = flatMsgLower(f);
    var out = [];
    var roster = adminProviderIdsForDirtyRouting(deps);
    for (var pi = 0; pi < roster.length; pi++) {
      var providerId = roster[pi];
      var providerHit =
        String(f.provider_id || f.provider || f.upstream_provider || "")
          .toLowerCase() === providerId ||
        String(f.upstreamModel || f.model || "")
          .toLowerCase()
          .indexOf(providerId + "/") === 0 ||
        msgEv.indexOf(providerId) >= 0;
      if (providerHit) out.push("admin-provider-" + providerId);
    }
    return out;
  }

  function indexerGroupIdForFlat(f, deps) {
    if (deps.indexerGroupIdForFlat) return deps.indexerGroupIdForFlat(f);
    if (
      globalThis.ChimeraSettings &&
      ChimeraSettings.Derive &&
      typeof ChimeraSettings.Derive.indexerGroupKeyFromFlat === "function"
    ) {
      var gx = ChimeraSettings.Derive.indexerGroupKeyFromFlat(f);
      if (gx != null && String(gx).trim() !== "") return String(gx).trim();
    }
    var itk =
      f.indexer_target_key != null && String(f.indexer_target_key).trim() !== ""
        ? String(f.indexer_target_key).trim()
        : "";
    var ik = f.indexer_key != null && String(f.indexer_key).trim() !== "" ? String(f.indexer_key).trim() : "";
    var rid = f.index_run_id != null && f.index_run_id !== "" ? String(f.index_run_id) : "";
    return itk || ik || rid || "";
  }

  /**
   * @param {{ parsed: object, source?: string }} entry
   * @param {{ reqToConv?: object, indexRunToConv?: object }} correlation
   * @param {{ getFlat: function, strHash: function, normalizeServiceBucketKey: function, indexerGroupIdForFlat?: function }} deps
   * @returns {{ cardIds: string[], indexerBucketIds: string[] }}
   */
  function dirtyTargetsForEntry(entry, correlation, deps) {
    correlation = correlation || {};
    deps = deps || {};
    var cardIds = [];
    var seen = Object.create(null);
    var indexerBucketIds = [];
    var seenIx = Object.create(null);

    function pushCard(id) {
      if (!id || seen[id]) return;
      seen[id] = true;
      cardIds.push(id);
    }

    function pushIndexerBucket(id) {
      if (!id || seenIx[id]) return;
      seenIx[id] = true;
      indexerBucketIds.push(id);
    }

    if (!entry || !entry.parsed) return { cardIds: cardIds, indexerBucketIds: indexerBucketIds };

    var f = deps.getFlat(entry.parsed);
    var msg = flatMsgLower(f);
    var pipelineLine = entryIsIndexerPipelineLine(f);
    var indexerLine = entryIsIndexerLine(entry, deps.getFlat);

    var cid = f.conversation_id != null ? String(f.conversation_id).trim() : "";
    var pid =
      f.principal_id != null
        ? String(f.principal_id).trim()
        : f.tenant != null
          ? String(f.tenant).trim()
          : "";

    if (cid) {
      pushCard(conversationCardIdForPrincipalAndCid(pid, cid, deps.strHash));
    } else {
      var ridJoin = f.request_id != null ? String(f.request_id).trim() : "";
      if (ridJoin && correlation.reqToConv && correlation.reqToConv[ridJoin]) {
        var rc = correlation.reqToConv[ridJoin];
        pushCard(conversationCardIdForPrincipalAndCid(rc.pid, rc.cid, deps.strHash));
      }
      var irJoin = f.index_run_id != null ? String(f.index_run_id).trim() : "";
      if (irJoin && correlation.indexRunToConv && correlation.indexRunToConv[irJoin]) {
        var ic = correlation.indexRunToConv[irJoin];
        pushCard(conversationCardIdForPrincipalAndCid(ic.pid, ic.cid, deps.strHash));
      }
    }

    if (!pipelineLine) {
      var svcKey = serviceBucketKeyForEntry(entry, deps);
      var dirtyServiceCard = true;
      if (indexerLine) {
        dirtyServiceCard = indexerServiceCardDirtyMsg(f, msg);
      } else if (flatTimelineKind(f) === "indexer") {
        dirtyServiceCard = false;
      }
      if (svcKey && dirtyServiceCard) {
        pushCard(serviceCardIdForBucketKey(svcKey, deps.strHash));
      }
    }

    var adminIds = adminProviderIdsForEntry(entry, deps);
    for (var ai = 0; ai < adminIds.length; ai++) pushCard(adminIds[ai]);

    var ixGroup = indexerGroupIdForFlat(f, deps);
    if (ixGroup) {
      if (pipelineLine || indexerLine) {
        if (pipelineLine && msg === "ingest.failed") {
          pushIndexerBucket(ixGroup);
        } else if (indexerLine && indexerWorkspaceDirtyMsg(f, msg)) {
          pushIndexerBucket(ixGroup);
        } else if (pipelineLine && msg === "ingest.complete") {
          // Successful pipeline lines are DEBUG in operator profile; if visible, skip card churn.
        }
      } else {
        pushIndexerBucket(ixGroup);
      }
    }

    return { cardIds: cardIds, indexerBucketIds: indexerBucketIds };
  }

  globalThis.ChimeraSettings.Summarized.dirtyTargetsForEntry = dirtyTargetsForEntry;
  globalThis.ChimeraSettings.Summarized.serviceBucketKeyForEntry = serviceBucketKeyForEntry;
  globalThis.ChimeraSettings.Summarized.serviceCardIdForBucketKey = serviceCardIdForBucketKey;
  globalThis.ChimeraSettings.Summarized.conversationCardIdForPrincipalAndCid = conversationCardIdForPrincipalAndCid;
  globalThis.ChimeraSettings.Summarized.adminProviderIdsForEntry = adminProviderIdsForEntry;
  globalThis.ChimeraSettings.Summarized.indexerWorkspaceDirtyMsg = indexerWorkspaceDirtyMsg;
  globalThis.ChimeraSettings.Summarized.indexerServiceCardDirtyMsg = indexerServiceCardDirtyMsg;
  globalThis.ChimeraSettings.Summarized.entryIsIndexerPipelineLine = entryIsIndexerPipelineLine;
  globalThis.ChimeraSettings.Summarized.SERVICE_BUCKET_ORDER = SERVICE_BUCKET_ORDER;
})();
