/**
 * Pure derivations for Indexer run rollups.
 *
 * Exports:
 * - ChimeraSettings.Derive.flatIsIndexerService(flat)
 * - ChimeraSettings.Derive.collectIndexerRunMeta(runId, evs, opts?)
 *   opts.getFlat(parsed) -> flat
 *   opts.tokenLabelByTenant -> map
 *   opts.indexerFlatMsg(flat) -> msg string
 *   opts.flatLooksLikeIndexerRunStart(flat) -> bool
 *   opts.flatLooksLikeIndexerRunDone(flat) -> bool
 *   opts.flatLooksLikeIndexerRunProgress(flat) -> bool
 *   opts.flatLooksLikeIndexerJobIngested(flat) -> bool
 *   opts.partitionMeta — optional { workspace_id, ingest_project, flavor_id, paths[] } from root_scopes
 */

function flatIsIndexerService(fl) {
  var s = String((fl && fl.service) || "").toLowerCase();
  return s === "indexer" || s === "chimera-indexer";
}

function collectIndexerRunMeta(runId, evs, opts) {
  evs = Array.isArray(evs) ? evs : [];
  opts = opts || {};
  var getFlat = typeof opts.getFlat === "function" ? opts.getFlat : function (p) { return (p && p.rawFlat) || {}; };
  var tokenLabelByTenant = opts.tokenLabelByTenant || {};

  var indexerFlatMsg = typeof opts.indexerFlatMsg === "function"
    ? opts.indexerFlatMsg
    : function (fl) {
        var msg = fl.msg != null ? fl.msg : fl.message;
        return String(msg != null ? msg : "").toLowerCase().trim();
      };

  var flatLooksLikeIndexerRunStart = typeof opts.flatLooksLikeIndexerRunStart === "function"
    ? opts.flatLooksLikeIndexerRunStart
    : function (fl) {
        var m = indexerFlatMsg(fl);
        if (m === "indexer.run.start" || m === "indexer run start") return true;
        if (!flatIsIndexerService(fl)) return false;
        return fl.root_ids != null && (fl.roots != null || Array.isArray(fl.watch_root_paths));
      };

  var flatLooksLikeIndexerRunDone = typeof opts.flatLooksLikeIndexerRunDone === "function"
    ? opts.flatLooksLikeIndexerRunDone
    : function (fl) {
        var m = indexerFlatMsg(fl);
        if (m.indexOf("indexer.run.done") === 0) return true;
        if (m === "indexer run done" || m === "indexer run stopped") return true;
        return flatIsIndexerService(fl) && fl.ingest_completed != null && fl.mode != null && String(fl.mode).trim() !== "";
      };

  var flatLooksLikeIndexerRunProgress = typeof opts.flatLooksLikeIndexerRunProgress === "function"
    ? opts.flatLooksLikeIndexerRunProgress
    : function (fl) {
        var m = indexerFlatMsg(fl);
        if (m.indexOf("indexer.run.progress") === 0 || m === "indexer.run.progress") return true;
        if (m === "initial scan complete") return true;
        return fl.phase != null && String(fl.phase).trim() !== "" && fl.candidates_enqueued != null;
      };

  var flatLooksLikeIndexerJobIngested = typeof opts.flatLooksLikeIndexerJobIngested === "function"
    ? opts.flatLooksLikeIndexerJobIngested
    : function (fl) {
        var m = indexerFlatMsg(fl);
        if (!flatIsIndexerService(fl)) return false;
        if (m !== "indexer.job.ingested" && m !== "ingested") return false;
        return fl.chunks != null;
      };

  var start = null;
  for (var i = 0; i < evs.length; i++) {
    var fi = getFlat(evs[i].parsed);
    if (flatLooksLikeIndexerRunStart(fi)) { start = fi; break; }
  }

  var lastProg = null,
    doneFlat = null,
    doneSeen = false,
    tenantId = "",
    userLabelDirect = "",
    indexerKey = "",
    lastDeclaredState = "",
    stateQueueDepth = null,
    stateIngestInflight = null,
    qdrantPointsLive = (function () {
      var idxLatestState = -1;
      for (var u = evs.length - 1; u >= 0; u--) {
        var fs = getFlat(evs[u].parsed);
        if (indexerFlatMsg(fs) === "indexer.state") {
          idxLatestState = u;
          break;
        }
      }
      if (idxLatestState >= 0) {
        var sum = 0;
        var any = false;
        for (var k = idxLatestState - 1; k >= 0; k--) {
          var fK = getFlat(evs[k].parsed);
          if (indexerFlatMsg(fK) !== "indexer.storage.stats") break;
          if (fK.qdrant_points != null) {
            var qp = Number(fK.qdrant_points);
            if (!isNaN(qp)) {
              sum += qp;
              any = true;
            }
          }
        }
        if (any) return sum;
        return null;
      }
      var sum2 = 0;
      var any2 = false;
      for (var u2 = evs.length - 1; u2 >= 0; u2--) {
        var f2 = getFlat(evs[u2].parsed);
        if (indexerFlatMsg(f2) !== "indexer.storage.stats") break;
        if (f2.qdrant_points != null) {
          var qp2 = Number(f2.qdrant_points);
          if (!isNaN(qp2)) {
            sum2 += qp2;
            any2 = true;
          }
        }
      }
      return any2 ? sum2 : null;
    })(),
    filesExcludedByIgnores = null;
  for (var u = evs.length - 1; u >= 0; u--) {
    var fR = getFlat(evs[u].parsed);
    if (!tenantId && (fR.tenant_id || fR.tenant || fR.principal_id))
      tenantId = String(fR.tenant_id || fR.tenant || fR.principal_id || "").trim();
    if (!userLabelDirect && fR.user_label && String(fR.user_label).trim() !== "")
      userLabelDirect = String(fR.user_label).trim();
    var itkSnap = fR.indexer_target_key != null && String(fR.indexer_target_key).trim() !== "";
    if (!indexerKey && itkSnap) indexerKey = String(fR.indexer_target_key).trim();
    if (!indexerKey && fR.indexer_key && String(fR.indexer_key).trim() !== "")
      indexerKey = String(fR.indexer_key).trim();
    if (!lastProg && flatLooksLikeIndexerRunProgress(fR)) lastProg = fR;
    if (flatLooksLikeIndexerRunDone(fR)) {
      doneSeen = true;
      if (!doneFlat) doneFlat = fR;
    }
    var mR = indexerFlatMsg(fR);
    if (!lastDeclaredState && mR === "indexer.state" && fR.state) {
      lastDeclaredState = String(fR.state).trim();
      if (stateQueueDepth == null && fR.queue_depth != null) {
        var qd = Number(fR.queue_depth);
        if (!isNaN(qd)) stateQueueDepth = qd;
      }
      if (stateIngestInflight == null && fR.ingest_inflight != null) {
        var inf = Number(fR.ingest_inflight);
        if (!isNaN(inf)) stateIngestInflight = inf;
      }
    }
  }
  for (var di = 0; di < evs.length; di++) {
    var fd = getFlat(evs[di].parsed);
    var md = indexerFlatMsg(fd);
    if (
      (md === "indexer.discovery.summary" || md === "indexer.discovery.summary.scope") &&
      fd.files_excluded_by_ignore_rules != null
    ) {
      var ig = Number(fd.files_excluded_by_ignore_rules);
      if (!isNaN(ig)) {
        filesExcludedByIgnores = ig;
        break;
      }
    }
  }

  var vectorsSum = 0;
  for (var j = 0; j < evs.length; j++) {
    var fj = getFlat(evs[j].parsed);
    if (flatLooksLikeIndexerJobIngested(fj)) {
      var cj = Number(fj.chunks);
      if (!isNaN(cj)) vectorsSum += cj;
    }
  }

  var lpEmb = lastProg && (lastProg.chunks_embedded != null ? Number(lastProg.chunks_embedded) : lastProg.embedded_chunks != null ? Number(lastProg.embedded_chunks) : NaN);
  var vectorsStored = null;
  if (vectorsSum > 0) vectorsStored = vectorsSum;
  else if (!isNaN(lpEmb) && lpEmb > 0) vectorsStored = Math.round(lpEmb);

  var ok = 0, fail = 0;
  if (doneFlat) {
    var oc = Number(doneFlat.ingest_completed);
    var fc = Number(doneFlat.ingest_failed_dropped);
    ok = !isNaN(oc) ? oc : 0;
    fail = !isNaN(fc) ? fc : 0;
  } else {
    for (var k = 0; k < evs.length; k++) {
      var fk = getFlat(evs[k].parsed);
      var mk = indexerFlatMsg(fk);
      if (mk === "indexer.job.ingested" || mk === "ingested") ok++;
      else if (mk === "indexer.job.failed" || mk.indexOf("ingest failed (dropped)") === 0) fail++;
    }
  }

  var ws = start && start.scope_workspace_id ? String(start.scope_workspace_id).trim() : "";
  var sp = start && start.scope_project_id ? String(start.scope_project_id).trim() : "";
  var ip = start && start.ingest_project ? String(start.ingest_project).trim() : "";
  var flavor = start && start.flavor_id ? String(start.flavor_id).trim() : "";

  var defProj = "",
    defFlav = "";
  for (var bx = 0; bx < evs.length; bx++) {
    var fb = getFlat(evs[bx].parsed);
    if (!flatIsIndexerService(fb)) continue;
    var mb = indexerFlatMsg(fb);
    if (!ws && fb.scope_workspace_id) ws = String(fb.scope_workspace_id).trim();
    if (!sp && fb.scope_project_id) sp = String(fb.scope_project_id).trim();
    if (!ip && fb.project_id) ip = String(fb.project_id).trim();
    if (!ip && fb.ingest_project) ip = String(fb.ingest_project).trim();
    if (!flavor && fb.flavor_id) flavor = String(fb.flavor_id).trim();
    if (mb === "gateway.indexer.config") {
      if (!defProj && fb.defaults_project_id)
        defProj = String(fb.defaults_project_id).trim();
      if (!defProj && fb["defaults.project_id"])
        defProj = String(fb["defaults.project_id"]).trim();
      if (!defFlav && fb.defaults_flavor_id) defFlav = String(fb.defaults_flavor_id).trim();
      if (!defFlav && fb["defaults.flavor_id"]) defFlav = String(fb["defaults.flavor_id"]).trim();
    }
  }

  var pm = opts.partitionMeta;
  if (pm && typeof pm === "object") {
    var pws = pm.workspace_id != null ? String(pm.workspace_id).trim() : "";
    var pin = pm.ingest_project != null ? String(pm.ingest_project).trim() : "";
    var pfl = pm.flavor_id != null ? String(pm.flavor_id).trim() : "";
    if (pws !== "") ws = pws;
    if (pin !== "") {
      sp = pin;
      ip = pin;
    }
    if (pm.flavor_id !== undefined && pm.flavor_id !== null) flavor = pfl;
  }

  var projectId = sp || ip || defProj || "—";
  /** Bucket id for this card (indexer_target_key, indexer_key, or index_run_id). */
  var bucketGid = String(runId || "").trim();

  var rsRows = [];
  if (
    start &&
    start.root_scopes != null &&
    globalThis.ChimeraSettings &&
    ChimeraSettings.Derive &&
    typeof ChimeraSettings.Derive.indexerParseRootScopes === "function"
  ) {
    rsRows = ChimeraSettings.Derive.indexerParseRootScopes(start.root_scopes);
  }
  var distinctTargetKeys = {};
  for (var ri = 0; ri < rsRows.length; ri++) {
    var itk = String(rsRows[ri].indexer_target_key || "").trim();
    if (itk) distinctTargetKeys[itk] = true;
  }
  var nDistinctTargets = Object.keys(distinctTargetKeys).length;

  var startRunId =
    start && start.index_run_id != null ? String(start.index_run_id).trim() : "";

  function indexerFlatMatchesBucket(fl, bucketId) {
    if (!fl || typeof fl !== "object") return false;
    var bk = String(bucketId || "").trim();
    if (!bk) return false;
    var itkFl = String(fl.indexer_target_key || "").trim();
    if (itkFl && itkFl === bk) return true;
    var ridFl = fl.index_run_id != null ? String(fl.index_run_id).trim() : "";
    if (ridFl && ridFl === bk) return true;
    if (startRunId && bk === String(startRunId).trim()) return ridFl === String(startRunId).trim();
    if (bk.indexOf("ig\u001e") !== 0) return false;
    var parts = bk.split("\u001e");
    if (parts.length < 4) return false;
    var pj = String(fl.project_id != null ? fl.project_id : fl.ingest_project != null ? fl.ingest_project : "").trim();
    var fv = String(fl.flavor_id != null ? fl.flavor_id : "").trim();
    return pj === (parts[2] || "") && fv === (parts[3] || "");
  }

  var discoveryCandidatesForBucket = null,
    lastScopeStatusFlat = null,
    lastScopeActiveFlat = null;
  for (var sc = evs.length - 1; sc >= 0; sc--) {
    var fSc = getFlat(evs[sc].parsed);
    var mSc = indexerFlatMsg(fSc);
    if (
      discoveryCandidatesForBucket == null &&
      mSc === "indexer.discovery.summary.scope" &&
      indexerFlatMatchesBucket(fSc, bucketGid)
    ) {
      var cdi = Number(fSc.candidates_discovered);
      if (!isNaN(cdi)) discoveryCandidatesForBucket = cdi;
    }
  }
  for (var sc2 = evs.length - 1; sc2 >= 0; sc2--) {
    var fSb = getFlat(evs[sc2].parsed);
    var mSb = indexerFlatMsg(fSb);
    if (!lastScopeStatusFlat && mSb === "indexer.scope.status" && indexerFlatMatchesBucket(fSb, bucketGid))
      lastScopeStatusFlat = fSb;
    if (!lastScopeActiveFlat && mSb === "indexer.scope.active_file" && indexerFlatMatchesBucket(fSb, bucketGid))
      lastScopeActiveFlat = fSb;
    if (lastScopeStatusFlat && lastScopeActiveFlat) break;
  }

  var scopeWorkspaceTotal = null,
    scopeQueueIngestPending = null,
    scopeQueueFanoutPending = null,
    scopePendingBulkTier1 = null,
    scopeLatestRel = null,
    scopeLatestRoot = null;
  if (lastScopeStatusFlat) {
    scopeWorkspaceTotal = Number(lastScopeStatusFlat.workspace_files_total);
    if (isNaN(scopeWorkspaceTotal)) scopeWorkspaceTotal = null;
    scopeQueueIngestPending = Number(lastScopeStatusFlat.queue_ingest_pending);
    if (isNaN(scopeQueueIngestPending)) scopeQueueIngestPending = null;
    scopeQueueFanoutPending = Number(lastScopeStatusFlat.queue_fanout_files_pending);
    if (isNaN(scopeQueueFanoutPending)) scopeQueueFanoutPending = null;
    scopePendingBulkTier1 = Number(lastScopeStatusFlat.pending_bulk_tier1);
    if (isNaN(scopePendingBulkTier1)) scopePendingBulkTier1 = null;
  }
  if (scopeWorkspaceTotal == null && discoveryCandidatesForBucket != null) {
    scopeWorkspaceTotal = discoveryCandidatesForBucket;
  }
  if (lastScopeActiveFlat) {
    scopeLatestRel = lastScopeActiveFlat.rel != null ? String(lastScopeActiveFlat.rel).trim() : "";
    scopeLatestRoot = lastScopeActiveFlat.root != null ? String(lastScopeActiveFlat.root).trim() : "";
  }

  /** Match root_scopes row to card bucket: target UUID, synthetic ig\x1e… id, or index_run_id bucket. */
  function indexerRowMatchesBucket(row, bucketId) {
    if (!row) return false;
    var bk = String(bucketId || "").trim();
    var rk = String(row.indexer_target_key || "").trim();
    if (rk === bk) return true;
    if (startRunId && bk === startRunId) return true;
    if (bk.indexOf("ig\u001e") !== 0) return false;
    var parts = bk.split("\u001e");
    if (parts.length < 4) return false;
    var rp = String(row.ingest_project != null ? row.ingest_project : "").trim();
    var rf = String(row.flavor_id != null ? row.flavor_id : "").trim();
    return rp === (parts[2] || "") && rf === (parts[3] || "");
  }

  /** Paths whose root_scopes row matches this summarized card bucket (multi-target supervised). */
  var scopedToBucket = [];
  for (var rj = 0; rj < rsRows.length; rj++) {
    var row = rsRows[rj];
    if (!indexerRowMatchesBucket(row, bucketGid)) continue;
    var pp = row.path != null ? String(row.path).trim() : "";
    if (pp && scopedToBucket.indexOf(pp) < 0) scopedToBucket.push(pp);
  }

  /** Full watch_root_paths from start (one process — may list every root on disk). */
  var pathsFromStartWatch =
    start && Array.isArray(start.watch_root_paths) && start.watch_root_paths.length
      ? start.watch_root_paths.map(function (p) {
          return String(p);
        })
      : [];

  /** Legacy single-target: all paths from root_scopes when there is only one ingest target. */
  var pathsFromRootScopesAll = [];
  for (var rk = 0; rk < rsRows.length; rk++) {
    var rw = rsRows[rk];
    var rp = rw && rw.path != null ? String(rw.path).trim() : "";
    if (rp && pathsFromRootScopesAll.indexOf(rp) < 0) pathsFromRootScopesAll.push(rp);
  }

  var pathsFromPartition =
    pm && typeof pm === "object" && Array.isArray(pm.paths) && pm.paths.length
      ? pm.paths.map(function (x) {
          return String(x);
        })
      : [];

  var primaryPaths = [];
  if (scopedToBucket.length > 0) {
    primaryPaths = scopedToBucket;
  } else if (nDistinctTargets > 1) {
    /* Several indexer_target_key rows — never show full watch_root_paths on every card. */
    primaryPaths = pathsFromPartition.length > 0 ? pathsFromPartition.slice() : [];
    if (primaryPaths.length === 0 && rsRows.length > 0) {
      var prf = projectId && projectId !== "—" ? String(projectId).trim() : "";
      var fvf = flavor && flavor !== "—" ? String(flavor).trim() : "";
      if (prf !== "") {
        for (var rx = 0; rx < rsRows.length; rx++) {
          var rxw = rsRows[rx];
          var rip = String(rxw.ingest_project != null ? rxw.ingest_project : "").trim();
          var rfv = String(rxw.flavor_id != null ? rxw.flavor_id : "").trim();
          if (rip !== prf || rfv !== fvf) continue;
          var rpp = rxw.path != null ? String(rxw.path).trim() : "";
          if (rpp && primaryPaths.indexOf(rpp) < 0) primaryPaths.push(rpp);
        }
      }
    }
  } else {
    primaryPaths =
      pathsFromStartWatch.length > 0
        ? pathsFromStartWatch
        : pathsFromRootScopesAll.length > 0
          ? pathsFromRootScopesAll
          : pathsFromPartition;
  }
  var watchPathsLine =
    primaryPaths.length > 0 ? primaryPaths.join("\n") : "—";
  /** Do not fall back to rel (current ingest file) or root_ids — watched paths stay YAML/partition roots only. */
  var filepath = watchPathsLine;
  var userLab = userLabelDirect ? userLabelDirect : tenantId ? tokenLabelByTenant[tenantId] || tenantId : "—";

  var watchOut = primaryPaths.length > 0 ? primaryPaths.slice() : [];

  return {
    runId: runId,
    indexerKey: indexerKey,
    start: start,
    userLabel: userLab,
    tenantId: tenantId,
    workspaceId: ws || "—",
    projectId: projectId,
    flavorId: flavor || defFlav || "—",
    filepath: filepath,
    watchRootPaths: watchOut,
    doneSeen: doneSeen,
    doneFlat: doneFlat,
    lastProg: lastProg,
    vectorsStored: vectorsStored,
    okCount: ok,
    failCount: fail,
    lastDeclaredState: lastDeclaredState,
    stateQueueDepth: stateQueueDepth,
    stateIngestInflight: stateIngestInflight,
    qdrantPointsLive: qdrantPointsLive,
    filesExcludedByIgnores: filesExcludedByIgnores,
    scopeWorkspaceTotal: scopeWorkspaceTotal,
    scopeQueueIngestPending: scopeQueueIngestPending,
    scopeQueueFanoutPending: scopeQueueFanoutPending,
    scopePendingBulkTier1: scopePendingBulkTier1,
    scopeLatestRel: scopeLatestRel,
    scopeLatestRoot: scopeLatestRoot,
    scopeStatusFlat: lastScopeStatusFlat,
    scopeActiveFlat: lastScopeActiveFlat
  };
}

globalThis.ChimeraSettings = globalThis.ChimeraSettings || {};
globalThis.ChimeraSettings.Derive = globalThis.ChimeraSettings.Derive || {};
globalThis.ChimeraSettings.Derive.flatIsIndexerService = flatIsIndexerService;
globalThis.ChimeraSettings.Derive.collectIndexerRunMeta = collectIndexerRunMeta;

