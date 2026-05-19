/**
 * Pure dirty-card routing for live log lines (Phase 3).
 * Maps one cache entry to summarized card ids without DOM access.
 */
globalThis.ChimeraLogs = globalThis.ChimeraLogs || {};
globalThis.ChimeraLogs.Summarized = globalThis.ChimeraLogs.Summarized || {};

(function () {
  var ADMIN_PROVIDER_IDS = ["groq", "gemini", "ollama"];
  var SERVICE_BUCKET_ORDER = ["chimera-broker", "chimera-gateway", "chimera-indexer", "chimera-vectorstore"];

  function flatMsg(f) {
    return String(f.msg != null ? f.msg : f.message != null ? f.message : "").trim();
  }

  function flatMsgLower(f) {
    return flatMsg(f).toLowerCase();
  }

  /** Gateway upstream relay lines bucket under chimera-broker (mirrors summarizedFeed). */
  function entryIsGatewayUpstreamRelay(ent, getFlat) {
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
    for (var pi = 0; pi < ADMIN_PROVIDER_IDS.length; pi++) {
      var providerId = ADMIN_PROVIDER_IDS[pi];
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
      globalThis.ChimeraLogs &&
      ChimeraLogs.Derive &&
      typeof ChimeraLogs.Derive.indexerGroupKeyFromFlat === "function"
    ) {
      var gx = ChimeraLogs.Derive.indexerGroupKeyFromFlat(f);
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

    var svcKey = serviceBucketKeyForEntry(entry, deps);
    if (svcKey) pushCard(serviceCardIdForBucketKey(svcKey, deps.strHash));

    var adminIds = adminProviderIdsForEntry(entry, deps);
    for (var ai = 0; ai < adminIds.length; ai++) pushCard(adminIds[ai]);

    var ixGroup = indexerGroupIdForFlat(f, deps);
    if (ixGroup) pushIndexerBucket(ixGroup);

    return { cardIds: cardIds, indexerBucketIds: indexerBucketIds };
  }

  globalThis.ChimeraLogs.Summarized.dirtyTargetsForEntry = dirtyTargetsForEntry;
  globalThis.ChimeraLogs.Summarized.serviceBucketKeyForEntry = serviceBucketKeyForEntry;
  globalThis.ChimeraLogs.Summarized.serviceCardIdForBucketKey = serviceCardIdForBucketKey;
  globalThis.ChimeraLogs.Summarized.conversationCardIdForPrincipalAndCid = conversationCardIdForPrincipalAndCid;
  globalThis.ChimeraLogs.Summarized.adminProviderIdsForEntry = adminProviderIdsForEntry;
  globalThis.ChimeraLogs.Summarized.SERVICE_BUCKET_ORDER = SERVICE_BUCKET_ORDER;
})();
