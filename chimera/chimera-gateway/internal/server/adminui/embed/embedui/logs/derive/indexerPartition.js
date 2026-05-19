/**
 * Multi-root indexer partitioning for /ui/logs (one card per ingest scope / indexer_target_key).
 *
 * ChimeraLogs.Derive.indexerParseRootScopes(jsonString | array)
 * ChimeraLogs.Derive.indexerExtractRunTargetsByRunId(entryCache, getFlat)
 * ChimeraLogs.Derive.indexerBucketGidsForLine(flat, st)
 * ChimeraLogs.Derive.indexerBucketsFromCache(entryCache, getFlat)
 */

function indexerFlatMsgLocal(fl) {
  if (
    globalThis.ChimeraLogs &&
    ChimeraLogs.Derive &&
    typeof ChimeraLogs.Derive.indexerFlatMsgForPresent === "function"
  ) {
    return ChimeraLogs.Derive.indexerFlatMsgForPresent(fl);
  }
  var msg = fl.msg != null ? fl.msg : fl.message;
  return String(msg != null ? msg : "").toLowerCase().trim();
}

function indexerInferIsIndexerLine(ent, flat) {
  var f = flat || {};
  var svc = String(f.service || "").toLowerCase();
  if (svc === "indexer" || svc === "chimera-indexer") return true;
  if (ent && String(ent.source || "").toLowerCase() === "indexer") return true;
  if (ent && ent.parsed && String(ent.parsed.app || "").toLowerCase() === "indexer") return true;
  return false;
}

/** Ensure structured fields see service=indexer when the log source is indexer (rawFlat may omit it). */
function indexerAugmentFlat(ent, flat) {
  var f = flat;
  if (!f || typeof f !== "object") f = {};
  if (String(f.service || "").toLowerCase() === "indexer" || String(f.service || "").toLowerCase() === "chimera-indexer")
    return f;
  if (!indexerInferIsIndexerLine(ent, f)) return f;
  var out = {};
  for (var k in f) {
    if (Object.prototype.hasOwnProperty.call(f, k)) out[k] = f[k];
  }
  out.service = "chimera-indexer";
  return out;
}

function indexerParseRootScopes(v) {
  if (v == null) return [];
  if (Array.isArray(v)) return v;
  var s = String(v).trim();
  if (!s) return [];
  try {
    var a = JSON.parse(s);
    return Array.isArray(a) ? a : [];
  } catch (e) {
    return [];
  }
}

/** Parse UI bucket id `ig\x1etenant\x1eproject\x1eflavor` (see indexerGroupKeyFromFlat). */
function parseIgSyntheticGid(gid) {
  if (gid == null || String(gid).indexOf("ig\u001e") !== 0) return null;
  var parts = String(gid).split("\u001e");
  if (parts.length < 4) return null;
  return { tenant: parts[1] || "", project: parts[2] || "", flavor: parts[3] || "" };
}

/** Union paths + labels from all partition targets (when bucket id is index_run_id). */
function indexerMergeAllKeyMetaPaths(st) {
  if (!st || !st.keyMeta) return null;
  var paths = [];
  var ip = "";
  var fv = "";
  var ws = "";
  for (var kk in st.keyMeta) {
    if (!Object.prototype.hasOwnProperty.call(st.keyMeta, kk)) continue;
    var km = st.keyMeta[kk];
    if (!km) continue;
    if (!ip && km.ingest_project) ip = String(km.ingest_project).trim();
    if (!fv && km.flavor_id != null && String(km.flavor_id).trim() !== "")
      fv = String(km.flavor_id).trim();
    if (!ws && km.workspace_id) ws = String(km.workspace_id).trim();
    if (km.paths) {
      for (var p = 0; p < km.paths.length; p++) {
        var x = km.paths[p];
        if (x && paths.indexOf(x) < 0) paths.push(x);
      }
    }
  }
  if (!paths.length && !ip && !fv && !ws) return null;
  return { ingest_project: ip, flavor_id: fv, workspace_id: ws, paths: paths };
}

function indexerBuildRunTargetState(startFlat) {
  var rows = indexerParseRootScopes(startFlat.root_scopes);
  var rootToKey = {};
  var keysFirst = [];
  var keyMeta = {};
  for (var r = 0; r < rows.length; r++) {
    var row = rows[r];
    if (!row || typeof row !== "object") continue;
    var k = String(row.indexer_target_key || "").trim();
    if (!k) continue;
    var rootId = String(row.root_id || "").trim();
    if (rootId) rootToKey[rootId] = k;
    if (keysFirst.indexOf(k) < 0) keysFirst.push(k);
    if (!keyMeta[k]) {
      keyMeta[k] = {
        ingest_project: String(row.ingest_project != null ? row.ingest_project : "").trim(),
        flavor_id: String(row.flavor_id != null ? row.flavor_id : "").trim(),
        workspace_id: String(row.workspace_id != null ? row.workspace_id : "").trim(),
        paths: []
      };
    }
    var p = String(row.path != null ? row.path : "").trim();
    if (p && keyMeta[k].paths.indexOf(p) < 0) keyMeta[k].paths.push(p);
  }
  return { keys: keysFirst, rootToKey: rootToKey, keyMeta: keyMeta };
}

function flatLooksIndexerRunStart(f, ent) {
  if (!f || typeof f !== "object") return false;
  var m = indexerFlatMsgLocal(f);
  if (m === "indexer.run.start") return true;
  if (m === "indexer run start") return true;
  if (f.msg === "indexer.run.start" || f.message === "indexer.run.start") return true;
  if (!indexerInferIsIndexerLine(ent, f)) return false;
  var rs = f.root_scopes != null && String(f.root_scopes).trim() !== "";
  return (
    (f.root_ids != null &&
      f.root_ids !== "" &&
      (Array.isArray(f.watch_root_paths) || typeof f.watch_root_paths === "string")) ||
    rs ||
    indexerParseRootScopes(f.root_scopes).length > 0
  );
}

function indexerExtractRunTargetsByRunId(entryCache, getFlat) {
  var out = {};
  if (!Array.isArray(entryCache) || typeof getFlat !== "function") return out;
  for (var i = 0; i < entryCache.length; i++) {
    var ent = entryCache[i];
    var f = indexerAugmentFlat(ent, getFlat(ent.parsed));
    if (!indexerInferIsIndexerLine(ent, f)) continue;
    var rid = f.index_run_id != null && String(f.index_run_id).trim() !== "" ? String(f.index_run_id).trim() : "";
    if (!rid) continue;
    if (!flatLooksIndexerRunStart(f, ent)) continue;
    out[rid] = indexerBuildRunTargetState(f);
  }
  return out;
}

/**
 * Ring buffer may drop indexer.run.start while job lines remain. Build a minimal per-run
 * target map from distinct (tenant, ingest_project, flavor) on job rows so partitioning
 * and titles still work.
 */
function indexerSyntheticRunTargetsFromJobs(entryCache, getFlat) {
  var jobKeysByRid = {};
  if (!Array.isArray(entryCache) || typeof getFlat !== "function") return jobKeysByRid;
  for (var i = 0; i < entryCache.length; i++) {
    var ent = entryCache[i];
    var f = indexerAugmentFlat(ent, getFlat(ent.parsed));
    if (!indexerInferIsIndexerLine(ent, f)) continue;
    var rid = f.index_run_id != null && String(f.index_run_id).trim() !== "" ? String(f.index_run_id).trim() : "";
    if (!rid) continue;
    var m = indexerFlatMsgLocal(f);
    if (
      m.indexOf("indexer.job.") !== 0 &&
      m !== "ingested" &&
      m !== "indexer.scope.status" &&
      m !== "indexer.scope.active_file"
    )
      continue;
    var ip = String(f.project_id != null ? f.project_id : f.ingest_project != null ? f.ingest_project : "").trim();
    if (!ip) continue;
    var tid = String(f.tenant_id || f.principal_id || f.tenant || "").trim();
    var fv = String(f.flavor_id != null ? f.flavor_id : "").trim();
    var sk = "ig\u001e" + tid + "\u001e" + ip + "\u001e" + fv;
    if (!jobKeysByRid[rid]) jobKeysByRid[rid] = { keys: [], rootToKey: {}, keyMeta: {} };
    var jb = jobKeysByRid[rid];
    if (jb.keys.indexOf(sk) < 0) jb.keys.push(sk);
    jb.keyMeta[sk] = {
      ingest_project: ip,
      flavor_id: fv,
      workspace_id: "",
      paths: []
    };
    var rk = f.root != null ? String(f.root).trim() : "";
    if (rk) jb.rootToKey[rk] = sk;
  }
  return jobKeysByRid;
}

function indexerMergeSyntheticTargets(targetState, entryCache, getFlat) {
  var synth = indexerSyntheticRunTargetsFromJobs(entryCache, getFlat);
  for (var rid in synth) {
    if (!Object.prototype.hasOwnProperty.call(synth, rid)) continue;
    var jb = synth[rid];
    if (!jb || !jb.keys || jb.keys.length <= 1) continue;
    var existing = targetState[rid];
    if (!existing || !existing.keys || existing.keys.length === 0) {
      targetState[rid] = jb;
    }
  }
  return targetState;
}

function indexerBucketGidsForLine(flat, st) {
  if (!st || !st.keys || st.keys.length === 0) return [];
  var itkLine = flat && flat.indexer_target_key != null ? String(flat.indexer_target_key).trim() : "";
  if (itkLine && st.keys.indexOf(itkLine) >= 0) return [itkLine];
  if (st.keys.length === 1) return [st.keys[0]];
  var rk = flat.root != null ? String(flat.root).trim() : "";
  if (rk && st.rootToKey[rk]) return [st.rootToKey[rk]];
  var ip = String(flat.project_id != null ? flat.project_id : flat.ingest_project != null ? flat.ingest_project : "").trim();
  var fav = String(flat.flavor_id != null ? flat.flavor_id : "").trim();
  if (ip !== "") {
    var i;
    for (i = 0; i < st.keys.length; i++) {
      var k = st.keys[i];
      var meta = st.keyMeta[k];
      if (!meta) continue;
      var mp = String(meta.ingest_project || "").trim();
      var mf = String(meta.flavor_id != null ? meta.flavor_id : "").trim();
      if (mp === ip && mf === fav) return [k];
    }
    for (i = 0; i < st.keys.length; i++) {
      var k2 = st.keys[i];
      var meta2 = st.keyMeta[k2];
      if (meta2 && String(meta2.ingest_project || "").trim() === ip) return [k2];
    }
  }
  return st.keys.slice();
}

function indexerGroupKeyFromFlatImport(fl) {
  if (globalThis.ChimeraLogs && ChimeraLogs.Derive && typeof ChimeraLogs.Derive.indexerGroupKeyFromFlat === "function") {
    return String(ChimeraLogs.Derive.indexerGroupKeyFromFlat(fl) || "").trim();
  }
  var ik =
    fl.indexer_key != null && String(fl.indexer_key).trim() !== "" ? String(fl.indexer_key).trim() : "";
  var rid =
    fl.index_run_id != null && String(fl.index_run_id).trim() !== "" ? String(fl.index_run_id).trim() : "";
  return ik || rid || "";
}

function indexerBucketsFromCache(entryCache, getFlat) {
  var targetState = indexerExtractRunTargetsByRunId(entryCache, getFlat);
  indexerMergeSyntheticTargets(targetState, entryCache, getFlat);
  var buckets = {};
  function push(gid, ent) {
    if (!gid) return;
    if (!buckets[gid]) buckets[gid] = [];
    buckets[gid].push(ent);
  }
  for (var i = 0; i < entryCache.length; i++) {
    var ent = entryCache[i];
    var fRaw = getFlat(ent.parsed);
    var f = indexerAugmentFlat(ent, fRaw);
    if (!indexerInferIsIndexerLine(ent, f)) continue;
    var rid = f.index_run_id != null && String(f.index_run_id).trim() !== "" ? String(f.index_run_id).trim() : "";
    var st = rid ? targetState[rid] : null;
    var gids = indexerBucketGidsForLine(f, st);
    var g;
    if (gids.length) {
      for (g = 0; g < gids.length; g++) push(gids[g], ent);
      continue;
    }
    var fb = indexerGroupKeyFromFlatImport(f);
    if (fb) push(fb, ent);
  }
  return { buckets: buckets, targetStateByRunId: targetState };
}

function indexerPartitionMetaForRun(partitionMetaRegistry, gid, sampleEvs, getFlat) {
  if (!sampleEvs || !sampleEvs.length || typeof getFlat !== "function") return null;

  var ridsSeen = {};
  var i;
  for (i = 0; i < sampleEvs.length; i++) {
    var rf = indexerAugmentFlat(sampleEvs[i], getFlat(sampleEvs[i].parsed));
    var rr = rf.index_run_id != null ? String(rf.index_run_id).trim() : "";
    if (rr) ridsSeen[rr] = true;
  }
  for (var rid in ridsSeen) {
    if (!Object.prototype.hasOwnProperty.call(ridsSeen, rid)) continue;
    var st = partitionMetaRegistry && partitionMetaRegistry[rid];
    if (st && st.keyMeta && st.keyMeta[gid]) return st.keyMeta[gid];
    if (st && st.keyMeta && String(gid).indexOf("ig\u001e") === 0) {
      var syn = parseIgSyntheticGid(gid);
      if (syn) {
        for (var kmKey in st.keyMeta) {
          if (!Object.prototype.hasOwnProperty.call(st.keyMeta, kmKey)) continue;
          var km = st.keyMeta[kmKey];
          if (!km) continue;
          var kmp = String(km.ingest_project || "").trim();
          var kmf = String(km.flavor_id != null ? km.flavor_id : "").trim();
          if (kmp === syn.project && kmf === syn.flavor) return km;
        }
      }
    }
    if (st && st.keyMeta && String(gid) === String(rid)) {
      var merged = indexerMergeAllKeyMetaPaths(st);
      if (merged && merged.paths && merged.paths.length) return merged;
    }
  }

  var aggPaths = [];
  var aggProj = "";
  var aggFlav = "";
  var aggWs = "";
  for (i = 0; i < sampleEvs.length; i++) {
    var ff = indexerAugmentFlat(sampleEvs[i], getFlat(sampleEvs[i].parsed));
    if (!flatLooksIndexerRunStart(ff, sampleEvs[i])) continue;
    var rows = indexerParseRootScopes(ff.root_scopes);
    var ridStart = ff.index_run_id != null ? String(ff.index_run_id).trim() : "";
    for (var k = 0; k < rows.length; k++) {
      var row = rows[k];
      if (!row) continue;
      var rowKey = String(row.indexer_target_key || "").trim();
      var synG = parseIgSyntheticGid(gid);
      var rowSynMatch =
        synG &&
        String(row.ingest_project != null ? row.ingest_project : "").trim() === synG.project &&
        String(row.flavor_id != null ? row.flavor_id : "").trim() === synG.flavor;
      var rowJoinRun = ridStart !== "" && String(gid) === ridStart;
      if (rowKey !== String(gid).trim() && !rowSynMatch && !rowJoinRun) continue;
      var pathOne = String(row.path != null ? row.path : "").trim();
      if (pathOne && aggPaths.indexOf(pathOne) < 0) aggPaths.push(pathOne);
      if (!aggProj && row.ingest_project != null && String(row.ingest_project).trim() !== "")
        aggProj = String(row.ingest_project).trim();
      if (!aggFlav && row.flavor_id != null && String(row.flavor_id).trim() !== "")
        aggFlav = String(row.flavor_id).trim();
      if (!aggWs && row.workspace_id != null && String(row.workspace_id).trim() !== "")
        aggWs = String(row.workspace_id).trim();
    }
  }
  if (aggPaths.length || aggProj || aggFlav || aggWs) {
    return {
      ingest_project: aggProj,
      flavor_id: aggFlav,
      workspace_id: aggWs,
      paths: aggPaths
    };
  }

  for (i = 0; i < sampleEvs.length; i++) {
    var fj = indexerAugmentFlat(sampleEvs[i], getFlat(sampleEvs[i].parsed));
    var m = indexerFlatMsgLocal(fj);
    if (
      m.indexOf("indexer.job.") !== 0 &&
      m !== "ingested" &&
      m !== "indexer.scope.status" &&
      m !== "indexer.scope.active_file"
    )
      continue;
    var ip = String(fj.project_id != null ? fj.project_id : fj.ingest_project != null ? fj.ingest_project : "").trim();
    if (!ip) continue;
    var tid = String(fj.tenant_id || fj.principal_id || fj.tenant || "").trim();
    var fv = String(fj.flavor_id != null ? fj.flavor_id : "").trim();
    var sk = "ig\u001e" + tid + "\u001e" + ip + "\u001e" + fv;
    if (sk === gid) {
      return { ingest_project: ip, flavor_id: fv, workspace_id: "", paths: [] };
    }
  }

  return null;
}

globalThis.ChimeraLogs = globalThis.ChimeraLogs || {};
globalThis.ChimeraLogs.Derive = globalThis.ChimeraLogs.Derive || {};
globalThis.ChimeraLogs.Derive.indexerParseRootScopes = indexerParseRootScopes;
globalThis.ChimeraLogs.Derive.indexerExtractRunTargetsByRunId = indexerExtractRunTargetsByRunId;
globalThis.ChimeraLogs.Derive.indexerBucketGidsForLine = indexerBucketGidsForLine;
globalThis.ChimeraLogs.Derive.indexerBucketsFromCache = indexerBucketsFromCache;
globalThis.ChimeraLogs.Derive.indexerPartitionMetaForRun = indexerPartitionMetaForRun;
globalThis.ChimeraLogs.Derive.indexerAugmentFlat = indexerAugmentFlat;
globalThis.ChimeraLogs.Derive.indexerInferIsIndexerLine = indexerInferIsIndexerLine;
globalThis.ChimeraLogs.Derive.parseIgSyntheticGid = parseIgSyntheticGid;
