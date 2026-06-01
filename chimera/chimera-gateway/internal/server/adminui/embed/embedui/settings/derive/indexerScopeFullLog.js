/**
 * Indexer workspace scoped full-log filtering (opws synthetic buckets, vectorstore collection match).
 * Pure over entry arrays + scope maps; no DOM.
 */
globalThis.ChimeraSettings = globalThis.ChimeraSettings || {};
globalThis.ChimeraSettings.Derive = globalThis.ChimeraSettings.Derive || {};

(function () {
  function normalizeIndexerScopeFlavor(v) {
    var s = v != null ? String(v) : "";
    s = s.replace(/\s+/g, " ").trim();
    if (!s) return "";
    if (s === "—" || s === "\u2014" || s === "-" || s.toLowerCase() === "none") return "";
    return s;
  }

  function entryIsIndexerLine(ent, getFlat) {
    if (
      globalThis.ChimeraSettings &&
      ChimeraSettings.Derive &&
      typeof ChimeraSettings.Derive.entryIsIndexerLine === "function"
    ) {
      return ChimeraSettings.Derive.entryIsIndexerLine(ent, getFlat);
    }
    return false;
  }

  function rebuildIndexerRootScopeMaps(entryCache, getFlat, outMap) {
    outMap = outMap || {};
    for (var k in outMap) {
      if (Object.prototype.hasOwnProperty.call(outMap, k)) delete outMap[k];
    }
    if (
      !globalThis.ChimeraSettings ||
      !ChimeraSettings.Derive ||
      typeof ChimeraSettings.Derive.indexerParseRootScopes !== "function"
    ) {
      return outMap;
    }
    entryCache = Array.isArray(entryCache) ? entryCache : [];
    var gi;
    for (gi = 0; gi < entryCache.length; gi++) {
      var ent = entryCache[gi];
      if (!entryIsIndexerLine(ent, getFlat)) continue;
      var raw = getFlat(ent.parsed);
      var msg = String(raw.msg != null ? raw.msg : raw.message != null ? raw.message : "")
        .toLowerCase()
        .trim();
      if (msg !== "indexer.run.start" && msg !== "indexer run start") continue;
      var rows = ChimeraSettings.Derive.indexerParseRootScopes(raw.root_scopes);
      var ri;
      for (ri = 0; ri < rows.length; ri++) {
        var row = rows[ri];
        if (!row || typeof row !== "object") continue;
        var rslug = row.root_id != null ? String(row.root_id).trim() : "";
        if (!rslug) continue;
        outMap[rslug] = {
          workspace_id: row.workspace_id != null ? String(row.workspace_id).trim() : "",
          path: row.path != null ? String(row.path).trim() : "",
          ingest_project: row.ingest_project != null ? String(row.ingest_project).trim() : "",
          flavor_id: row.flavor_id != null ? String(row.flavor_id).trim() : ""
        };
      }
    }
    return outMap;
  }

  function rootUnderOneOfPrefixes(root, prefixes) {
    var r = String(root || "")
      .replace(/\\/g, "/")
      .replace(/\/+$/, "")
      .toLowerCase();
    if (!r) return false;
    var i;
    for (i = 0; i < prefixes.length; i++) {
      var p = String(prefixes[i] || "")
        .replace(/\\/g, "/")
        .replace(/\/+$/, "")
        .toLowerCase();
      if (!p) continue;
      if (r === p) return true;
      if (r.indexOf(p + "/") === 0) return true;
    }
    return false;
  }

  function inferTenantForOpwsBucket(bucketId, entryCache, getFlat, scopeState) {
    var segs = String(bucketId || "").split("\u001e");
    if (segs[0] !== "opws" || segs.length < 3) return "";
    var wantWid = String(segs[1] || "").trim();
    var wantProj = String(segs[2] || "").trim();
    var wantFlav = segs.length > 3 ? normalizeIndexerScopeFlavor(segs[3]) : "";
    var opWsCtx = scopeState.operatorWsFullLogCtx[bucketId];
    var roots = opWsCtx && opWsCtx.paths ? opWsCtx.paths : [];
    var rootScope = scopeState.indexerRootScopeByRootId || {};
    var ei;
    for (ei = entryCache.length - 1; ei >= 0; ei--) {
      var ent = entryCache[ei];
      if (!entryIsIndexerLine(ent, getFlat)) continue;
      var raw = getFlat(ent.parsed);
      var f =
        globalThis.ChimeraSettings &&
          ChimeraSettings.Derive &&
          typeof ChimeraSettings.Derive.indexerAugmentFlat === "function"
          ? ChimeraSettings.Derive.indexerAugmentFlat(ent, raw)
          : raw;
      var fp = String(f.project_id || f.ingest_project || "").trim();
      var ff = normalizeIndexerScopeFlavor(f.flavor_id);
      if (fp !== wantProj || ff !== wantFlav) continue;
      var sw = f.scope_workspace_id != null ? String(f.scope_workspace_id).trim() : "";
      if (wantWid && sw === wantWid) {
        return String(f.tenant_id || f.principal_id || f.tenant || "").trim();
      }
      var rk = f.root != null ? String(f.root).trim() : "";
      if (wantWid && rk && rootScope[rk]) {
        var rsi = rootScope[rk];
        if (String(rsi.workspace_id || "") === wantWid) {
          return String(f.tenant_id || f.principal_id || f.tenant || "").trim();
        }
        if (rsi.path && roots.length && rootUnderOneOfPrefixes(rsi.path, roots)) {
          return String(f.tenant_id || f.principal_id || f.tenant || "").trim();
        }
      }
    }
    return "";
  }

  function indexerOperatorWorkspaceScopeMatch(ent, bucketId, f, getFlat, scopeState) {
    if (!entryIsIndexerLine(ent, getFlat)) return false;
    var segs = String(bucketId || "").split("\u001e");
    if (segs[0] !== "opws" || segs.length < 3) return false;
    var wantWid = String(segs[1] || "").trim();
    if (!wantWid) return false;
    var wantProj = String(segs[2] || "").trim();
    var wantFlav = segs.length > 3 ? normalizeIndexerScopeFlavor(segs[3]) : "";
    var fp = String(f.project_id || f.ingest_project || "").trim();
    var ff = normalizeIndexerScopeFlavor(f.flavor_id);
    if (wantProj !== "") {
      if (fp !== wantProj || ff !== wantFlav) return false;
    } else if (fp !== "") {
      return false;
    }
    var sw = f.scope_workspace_id != null ? String(f.scope_workspace_id).trim() : "";
    if (wantWid && sw === wantWid) return true;
    var opWsCtx = scopeState.operatorWsFullLogCtx[bucketId];
    var roots = opWsCtx && opWsCtx.paths ? opWsCtx.paths : [];
    var rootScope = scopeState.indexerRootScopeByRootId || {};
    var rk = f.root != null ? String(f.root).trim() : "";
    if (wantWid && rk && rootScope[rk]) {
      var rsi = rootScope[rk];
      if (String(rsi.workspace_id || "") === wantWid) return true;
      if (rsi.path && roots.length && rootUnderOneOfPrefixes(rsi.path, roots)) return true;
    }
    return false;
  }

  function operatorWorkspaceSyntheticBucketId(ws, scopeState, deps) {
    var wid = deps.canonicalWorkspaceRowIdKey(ws.id);
    var pj = String(ws.project_id != null ? ws.project_id : "").trim();
    var fvKey = normalizeIndexerScopeFlavor(ws.flavor_id);
    var bucketId = "opws\u001e" + wid + "\u001e" + pj + "\u001e" + fvKey;
    scopeState.operatorWsFullLogCtx[bucketId] = {
      paths: deps.operatorWorkspacePaths(ws).slice()
    };
    return bucketId;
  }

  function indexerBucketScopeCoords(bucketId, evs, partitionRegistry, entryCache, getFlat, scopeState) {
    bucketId = bucketId != null ? String(bucketId).trim() : "";
    evs = Array.isArray(evs) ? evs : [];
    if (bucketId.indexOf("opws\u001e") === 0) {
      var opSegs = bucketId.split("\u001e");
      if (opSegs.length >= 3) {
        var opProj = String(opSegs[2] || "").trim();
        var opFlav = opSegs.length > 3 ? normalizeIndexerScopeFlavor(opSegs[3]) : "";
        var opTenant = inferTenantForOpwsBucket(bucketId, entryCache, getFlat, scopeState);
        if (opTenant && opProj && opProj !== "—") {
          return { tenant: opTenant, project: opProj, flavor: opFlav };
        }
      }
    }
    var syn =
      globalThis.ChimeraSettings &&
        ChimeraSettings.Derive &&
        typeof ChimeraSettings.Derive.parseIgSyntheticGid === "function"
        ? ChimeraSettings.Derive.parseIgSyntheticGid(bucketId)
        : null;
    var tenant = "";
    var proj = "";
    var flavor = "";
    if (syn) {
      tenant = syn.tenant || "";
      proj = syn.project || "";
      flavor = syn.flavor || "";
    } else {
      var i;
      for (i = 0; i < evs.length; i++) {
        var rawF = getFlat(evs[i].parsed);
        var fIx =
          globalThis.ChimeraSettings &&
            ChimeraSettings.Derive &&
            typeof ChimeraSettings.Derive.indexerAugmentFlat === "function"
            ? ChimeraSettings.Derive.indexerAugmentFlat(evs[i], rawF)
            : rawF;
        if (
          String(fIx.indexer_target_key || "").trim() === bucketId ||
          String(fIx.indexer_key || "").trim() === bucketId
        ) {
          tenant = String(fIx.tenant_id || fIx.principal_id || fIx.tenant || "").trim();
          proj = String(fIx.project_id || fIx.ingest_project || "").trim();
          flavor = String(fIx.flavor_id != null ? fIx.flavor_id : "").trim();
          break;
        }
      }
      if (!tenant || !proj) {
        for (i = 0; i < evs.length; i++) {
          var rawG = getFlat(evs[i].parsed);
          var fIy =
            globalThis.ChimeraSettings &&
              ChimeraSettings.Derive &&
              typeof ChimeraSettings.Derive.indexerAugmentFlat === "function"
              ? ChimeraSettings.Derive.indexerAugmentFlat(evs[i], rawG)
              : rawG;
          var rid = fIy.index_run_id != null ? String(fIy.index_run_id).trim() : "";
          if (rid && rid === bucketId) {
            tenant = String(fIy.tenant_id || fIy.principal_id || fIy.tenant || "").trim();
            proj = String(fIy.project_id || fIy.ingest_project || "").trim();
            flavor = String(fIy.flavor_id != null ? fIy.flavor_id : "").trim();
            break;
          }
        }
      }
    }
    if (!proj || proj === "—") return null;
    if (!tenant) return null;
    if (flavor === "—") flavor = "";
    return { tenant: tenant, project: proj, flavor: flavor };
  }

  function indexerExpectedVectorstoreCollectionForBucket(bucketId, evs, partitionRegistry, entryCache, getFlat, scopeState) {
    var c = indexerBucketScopeCoords(bucketId, evs, partitionRegistry, entryCache, getFlat, scopeState);
    if (!c) return "";
    if (
      globalThis.ChimeraSettings &&
      ChimeraSettings.Derive &&
      typeof ChimeraSettings.Derive.vectorstoreCollectionName === "function"
    ) {
      return ChimeraSettings.Derive.vectorstoreCollectionName(c.tenant, c.project, c.flavor);
    }
    return "";
  }

  function indexerSupervisedWorkspaceLifecycleSlug(msg) {
    return (
      msg === "indexer.supervised.workspaces_changed" ||
      msg === "indexer.supervised.workspaces_reload" ||
      msg === "indexer.supervised.workspaces_session_start" ||
      msg === "indexer.supervised.workspaces_apply_failed" ||
      msg === "gateway.operator.workspace.path_added" ||
      msg === "gateway.operator.workspace.path_deleted"
    );
  }

  function csvFieldIds(raw) {
    if (raw == null) return [];
    return String(raw)
      .split(",")
      .map(function (s) {
        return s.trim();
      })
      .filter(Boolean);
  }

  function indexerLifecycleEventMatchesBucket(f, bucketScopeCoords, scopeState, deps) {
    if (!f || typeof f !== "object") return false;
    var msgSlug = String(f.msg != null ? f.msg : f.message != null ? f.message : "").trim();
    if (!indexerSupervisedWorkspaceLifecycleSlug(msgSlug)) return false;
    if (!bucketScopeCoords || !bucketScopeCoords.project) return true;
    var logWsIds = csvFieldIds(f.workspace_ids);
    if (msgSlug === "gateway.operator.workspace.path_added" || msgSlug === "gateway.operator.workspace.path_deleted") {
      var wsId = f.workspace_id != null ? String(f.workspace_id).trim() : "";
      if (wsId) logWsIds = [wsId];
    }
    if (!logWsIds.length) return true;
    var nested = scopeState.lastIndexerOperatorWorkspacesNested || [];
    var wi;
    for (wi = 0; wi < nested.length; wi++) {
      var wsRow = nested[wi];
      var wsKey = deps.canonicalWorkspaceRowIdKey(wsRow.id);
      var wsNumFn = deps.operatorWorkspaceNumericId;
      var wsNum = typeof wsNumFn === "function" ? String(wsNumFn(wsRow)) : "";
      var hi;
      for (hi = 0; hi < logWsIds.length; hi++) {
        if (logWsIds[hi] !== wsKey && logWsIds[hi] !== wsNum) continue;
        if (String(wsRow.project_id || "").trim() === bucketScopeCoords.project) return true;
      }
    }
    return false;
  }

  function indexerScopeFullLogInclude(
    ent,
    bucketId,
    partitionRegistry,
    expectedVectorstoreCollection,
    bucketScopeCoords,
    getFlat,
    scopeState,
    deps
  ) {
    bucketId = bucketId != null ? String(bucketId).trim() : "";
    if (!bucketId) return true;

    var rawFlat = getFlat(ent.parsed);
    var f = rawFlat;
    if (
      globalThis.ChimeraSettings &&
      ChimeraSettings.Derive &&
      typeof ChimeraSettings.Derive.indexerAugmentFlat === "function"
    ) {
      f = ChimeraSettings.Derive.indexerAugmentFlat(ent, rawFlat);
    }

    if (
      globalThis.ChimeraSettings &&
      ChimeraSettings.Derive &&
      typeof ChimeraSettings.Derive.indexerFlatOmitFromWorkspaceScopedLog === "function" &&
      ChimeraSettings.Derive.indexerFlatOmitFromWorkspaceScopedLog(f)
    ) {
      return false;
    }

    if (indexerLifecycleEventMatchesBucket(f, bucketScopeCoords, scopeState, deps)) return true;

    var srcL = String(ent.source || "").toLowerCase();
    var svcL = String(f.service || "").toLowerCase();
    if (
      srcL === "chimera-vectorstore" ||
      svcL === "chimera-vectorstore"
    ) {
      var coll = f.collection != null ? String(f.collection).trim() : "";
      var exp = expectedVectorstoreCollection != null ? String(expectedVectorstoreCollection).trim() : "";
      if (!coll || !exp) return false;
      return coll === exp;
    }

    if (bucketId.indexOf("opws\u001e") === 0) {
      return indexerOperatorWorkspaceScopeMatch(ent, bucketId, f, getFlat, scopeState);
    }

    var rid = f.index_run_id != null ? String(f.index_run_id).trim() : "";
    var st = rid && partitionRegistry && partitionRegistry[rid] ? partitionRegistry[rid] : null;

    if (
      globalThis.ChimeraSettings &&
      ChimeraSettings.Derive &&
      typeof ChimeraSettings.Derive.indexerBucketGidsForLine === "function" &&
      st &&
      st.keys &&
      st.keys.length > 0
    ) {
      var gids = ChimeraSettings.Derive.indexerBucketGidsForLine(f, st);
      if (gids && gids.length === 1) {
        return String(gids[0]).trim() === bucketId;
      }
      if (gids && gids.length > 1) {
        return false;
      }
    }

    var itk = f.indexer_target_key != null ? String(f.indexer_target_key).trim() : "";
    if (itk && itk === bucketId) return true;

    var ikk = f.indexer_key != null ? String(f.indexer_key).trim() : "";
    if (ikk && ikk === bucketId) return true;

    if (
      globalThis.ChimeraSettings &&
      ChimeraSettings.Derive &&
      typeof ChimeraSettings.Derive.indexerGroupKeyFromFlat === "function"
    ) {
      var gk = ChimeraSettings.Derive.indexerGroupKeyFromFlat(f);
      if (gk != null && String(gk).trim() === bucketId) return true;
    }

    if (bucketId.indexOf("ig\u001e") === 0) {
      var parts = bucketId.split("\u001e");
      if (parts.length >= 4) {
        var wantP = parts[2] || "";
        var wantF = parts[3] || "";
        var fp = String(
          f.project_id != null ? f.project_id : f.ingest_project != null ? f.ingest_project : ""
        ).trim();
        var ff = String(f.flavor_id != null ? f.flavor_id : "").trim();
        if (fp === wantP && ff === wantF) return true;
      }
    }

    if (rid && rid === bucketId) return true;

    var coords = bucketScopeCoords;
    if (coords && coords.tenant && coords.project) {
      var ragMsg = String(f.msg != null ? f.msg : f.message != null ? f.message : "").trim();
      if (ragMsg.toLowerCase() === "rag.retrieve.source") {
        var rgt = String(f.tenant_id != null ? f.tenant_id : f.principal_id != null ? f.principal_id : "").trim();
        var rgp = String(f.project_id != null ? f.project_id : f.project != null ? f.project : "").trim();
        var rgf = String(f.flavor_id != null ? f.flavor_id : "").trim();
        if (
          rgt &&
          rgp &&
          rgt === coords.tenant &&
          rgp === coords.project &&
          normalizeIndexerScopeFlavor(rgf) === normalizeIndexerScopeFlavor(coords.flavor)
        )
          return true;
      }
    }

    return false;
  }

  function filterEventsForIndexerScopeFullLog(evs, bucketId, partitionRegistry, entryCache, getFlat, scopeState, deps) {
    var out = [];
    if (!Array.isArray(evs)) return out;
    var expColl = indexerExpectedVectorstoreCollectionForBucket(
      bucketId,
      evs,
      partitionRegistry,
      entryCache,
      getFlat,
      scopeState
    );
    var bucketCoords = indexerBucketScopeCoords(
      bucketId,
      evs,
      partitionRegistry,
      entryCache,
      getFlat,
      scopeState
    );
    for (var i = 0; i < evs.length; i++) {
      if (
        indexerScopeFullLogInclude(
          evs[i],
          bucketId,
          partitionRegistry,
          expColl,
          bucketCoords,
          getFlat,
          scopeState,
          deps
        )
      ) {
        out.push(evs[i]);
      }
    }
    return out;
  }

  var D = globalThis.ChimeraSettings.Derive;
  D.normalizeIndexerScopeFlavor = normalizeIndexerScopeFlavor;
  D.rebuildIndexerRootScopeMaps = rebuildIndexerRootScopeMaps;
  D.rootUnderOneOfPrefixes = rootUnderOneOfPrefixes;
  D.inferTenantForOpwsBucket = inferTenantForOpwsBucket;
  D.indexerOperatorWorkspaceScopeMatch = indexerOperatorWorkspaceScopeMatch;
  D.operatorWorkspaceSyntheticBucketId = operatorWorkspaceSyntheticBucketId;
  D.indexerBucketScopeCoords = indexerBucketScopeCoords;
  D.indexerExpectedVectorstoreCollectionForBucket = indexerExpectedVectorstoreCollectionForBucket;
  D.indexerScopeFullLogInclude = indexerScopeFullLogInclude;
  D.filterEventsForIndexerScopeFullLog = filterEventsForIndexerScopeFullLog;

  /**
   * ctx-facing scope bridge for feed + indexer cards (entryCache/getFlat from ctx).
   * Exports: ChimeraSettings.Derive.mountIndexerScopeBridge(ctx)
   */
  D.mountIndexerScopeBridge = function (ctx) {
    function indexerScopeDeps() {
      return {
        canonicalWorkspaceRowIdKey: ctx.canonicalWorkspaceRowIdKey,
        operatorWorkspaceNumericId: ctx.operatorWorkspaceNumericId,
        operatorWorkspacePaths: ctx.operatorWorkspacePaths
      };
    }

    function indexerScopeState() {
      return {
        indexerRootScopeByRootId: ctx.indexerRootScopeByRootId || {},
        operatorWsFullLogCtx: ctx.operatorWsFullLogCtx || {},
        lastIndexerOperatorWorkspacesNested: ctx.lastIndexerOperatorWorkspacesNested || []
      };
    }

    function indexerGroupIdForFlat(fR) {
      if (typeof D.indexerGroupKeyFromFlat === "function") {
        var gx = D.indexerGroupKeyFromFlat(fR);
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

    function filterEventsForIndexerScopeFullLogBound(evs, bucketId, partitionRegistry) {
      if (typeof D.filterEventsForIndexerScopeFullLog !== "function") return Array.isArray(evs) ? evs : [];
      return D.filterEventsForIndexerScopeFullLog(
        evs,
        bucketId,
        partitionRegistry,
        ctx.entryCache,
        ctx.getFlat,
        indexerScopeState(),
        indexerScopeDeps()
      );
    }

    function operatorWorkspaceSyntheticBucketIdBound(ws) {
      if (typeof D.operatorWorkspaceSyntheticBucketId !== "function") return "";
      return D.operatorWorkspaceSyntheticBucketId(ws, indexerScopeState(), indexerScopeDeps());
    }

    ctx.filterEventsForIndexerScopeFullLog = filterEventsForIndexerScopeFullLogBound;
    ctx.operatorWorkspaceSyntheticBucketId = operatorWorkspaceSyntheticBucketIdBound;
    ctx.indexerGroupIdForFlat = indexerGroupIdForFlat;

    return {
      indexerGroupIdForFlat: indexerGroupIdForFlat,
      filterEventsForIndexerScopeFullLog: filterEventsForIndexerScopeFullLogBound,
      operatorWorkspaceSyntheticBucketId: operatorWorkspaceSyntheticBucketIdBound
    };
  };
})();
