/**
 * Summarized feed indexer cards (Phase 4b).
 */
globalThis.ChimeraSettings = globalThis.ChimeraSettings || {};
globalThis.ChimeraSettings.Render = globalThis.ChimeraSettings.Render || {};
globalThis.ChimeraSettings.Render.Cards = globalThis.ChimeraSettings.Render.Cards || {};

globalThis.ChimeraSettings.Render.Cards.mountFeedLogIndexerRun = function (ctx) {
  var escapeHtml = ctx.escapeHtml;
  var getFlat = ctx.getFlat;
  var entryCache = ctx.entryCache;
  var strHash = ctx.strHash;
  var entryInstant = ctx.entryInstant;
  var formatInt = ctx.formatInt;
  var sumEvlogPanelHtml = ctx.sumEvlogPanelHtml;
  var sumEvlogBuildTbodyFromServiceEntries = ctx.sumEvlogBuildTbodyFromServiceEntries;
  var sumEvlogCountWarnFailFromEntries = ctx.sumEvlogCountWarnFailFromEntries;
  var scopedEvlogTitle = ctx.scopedEvlogTitle;
  var RECENT_CARD_STATUS_N = ctx.RECENT_CARD_STATUS_N;
  var sliceRecent = ctx.sliceRecent;
  var countErrorSignalsInEntries = ctx.countErrorSignalsInEntries;
  var collectIndexerRunMeta = ctx.collectIndexerRunMeta;
  var indexerBuildCardSubtitle = ctx.indexerBuildCardSubtitle;
  var indexerWorkspaceCollapsedMetricsHtml = ctx.indexerWorkspaceCollapsedMetricsHtml;
  var operatorCardChevronHtml = ctx.operatorCardChevronHtml;
  var serviceSummaryStatusPillHtml = ctx.serviceSummaryStatusPillHtml;
  var buildIndexerRecentEvaluatedFilesHtml = ctx.buildIndexerRecentEvaluatedFilesHtml;
  var indexerCardDomIdFromMeta = ctx.indexerCardDomIdFromMeta;
  var workspaceCardTitleFromIndexerMeta = ctx.workspaceCardTitleFromIndexerMeta;
  var resolveLogsOperatorUserLabel = ctx.resolveLogsOperatorUserLabel;
  var canonicalWorkspaceRowIdKey = ctx.canonicalWorkspaceRowIdKey;
  var applyOperatorWorkspacePathsToMeta = ctx.applyOperatorWorkspacePathsToMeta;
  var operatorWorkspacePaths = ctx.operatorWorkspacePaths;
  var pathsSetEqualForIndexerRoots = ctx.pathsSetEqualForIndexerRoots;
  var normalizeFlavorMatch = ctx.normalizeFlavorMatch;
  var filterEventsForIndexerScopeFullLog = ctx.filterEventsForIndexerScopeFullLog;

  function indexerScopeProgressTimelineBarHtml(pRem, qTot, doneSeen) {
    var timelineSegmentsHtml = ctx.timelineSegmentsHtml;
    if (typeof timelineSegmentsHtml !== "function") return "";
    var orange = "#ffa726";
    if (doneSeen) {
      return timelineSegmentsHtml([{ pct: 100, bg: orange }]);
    }
    if (
      pRem !== null &&
      !isNaN(Number(pRem)) &&
      Number(pRem) === 0 &&
      qTot !== null &&
      !isNaN(Number(qTot)) &&
      Number(qTot) >= 0
    ) {
      return timelineSegmentsHtml([{ pct: 100, bg: orange }]);
    }
    if (qTot != null && !isNaN(Number(qTot)) && Number(qTot) > 0 && pRem != null && !isNaN(Number(pRem))) {
      var q = Number(qTot);
      var r = Number(pRem);
      var done = q - r;
      var pctDone = (done / q) * 100;
      if (pctDone < 0) pctDone = 0;
      if (pctDone > 100) pctDone = 100;
      if (pctDone > 0 && pctDone < 0.05) pctDone = 0.05;
      return timelineSegmentsHtml([{ pct: pctDone, bg: orange }]);
    }
    return timelineSegmentsHtml([]);
  }

  function indexerExpandedSummaryKvInnerHtml(meta, kvOpts) {
    kvOpts = kvOpts || {};
    var sumU = meta.userLabel && meta.userLabel !== "—" ? String(meta.userLabel).trim() : "—";
    var sumP = meta.projectId && meta.projectId !== "—" ? String(meta.projectId).trim() : "—";
    var sumF = meta.flavorId && meta.flavorId !== "—" ? String(meta.flavorId).trim() : "";
    var flavStrong =
      sumF !== "" ? escapeHtml(sumF) : '<span class="muted">\u2014</span>';
    var wsRow = "";
    var wsLab = kvOpts.workspaceRowId != null ? String(kvOpts.workspaceRowId).trim() : "";
    if (wsLab) {
      wsRow = "<dt>Workspace ID</dt><dd>" + escapeHtml(wsLab) + "</dd>";
    }
    var fcTot =
      meta.scopeWorkspaceTotal != null && !isNaN(Number(meta.scopeWorkspaceTotal))
        ? Math.round(Number(meta.scopeWorkspaceTotal))
        : null;
    var fileRow = "";
    if (!kvOpts.omitFileCountIfZero || (fcTot != null && fcTot > 0)) {
      var fileCountStrong =
        fcTot != null
          ? escapeHtml(formatInt(fcTot))
          : '<span class="muted">\u2014</span>';
      fileRow = "<dt>File count</dt><dd>" + fileCountStrong + "</dd>";
    }
    return (
      "<dt>User name</dt><dd>" +
      escapeHtml(sumU) +
      "</dd>" +
      "<dt>Project ID</dt><dd>" +
      escapeHtml(sumP) +
      "</dd>" +
      "<dt>Flavor ID</dt><dd>" +
      flavStrong +
      "</dd>" +
      wsRow +
      fileRow
    );
  }

  function renderExpandedIndexer(run, evs, meta, partitionRegistry, expOpts) {
    expOpts = expOpts || {};
    var kvOpts = expOpts.kvOpts || {};
    var pathsBlock =
      expOpts.pathsBlockHtml != null
        ? String(expOpts.pathsBlockHtml)
        : meta.watchRootPaths && meta.watchRootPaths.length
          ? "<pre class=\"indexer-paths-pre\">" +
          escapeHtml(formatWatchPathsPreHtml(meta.watchRootPaths)) +
          "</pre>"
          : '<span class="muted">—</span>';
    var summaryRows =
      '<dl class="indexer-run-kv indexer-run-kv--gateway-summary">' +
      indexerExpandedSummaryKvInnerHtml(meta, kvOpts) +
      "<dt>Watched paths</dt><dd" +
      (expOpts.pathsUiPart ? ' data-ui-part="' + escapeHtml(String(expOpts.pathsUiPart)) + '"' : "") +
      ">" +
      pathsBlock +
      "</dd></dl>";
    var configureBtn = expOpts.configureBtnHtml != null ? String(expOpts.configureBtnHtml) : "";
    var afterSummary = expOpts.extraAfterSummaryHtml != null ? String(expOpts.extraAfterSummaryHtml) : "";
    var evsFull = filterEventsForIndexerScopeFullLog(evs, run.id, partitionRegistry || {});
    var recentOpts = expOpts.recentOpts || {};
    var recentFiles = buildIndexerRecentEvaluatedFilesHtml(evsFull, run.id, 10, recentOpts);
    var recentSection = recentFiles
      ? '<div class="sum-section-label">Recently evaluated files</div>' + recentFiles
      : "";
    var fullId = "ix-full-" + strHash(run.id);
    var ixScope = strHash("ixrun:" + run.id);
    var tbodyInner;
    var mc;
    if (!evsFull.length) {
      tbodyInner = "";
      mc = { warn: 0, fail: 0 };
    } else {
      tbodyInner = sumEvlogBuildTbodyFromServiceEntries("indexer", evsFull, {
        cardScope: ixScope,
        filterGatewayProbe: false,
        indexerRunLine: true,
        suppressIndexerBadge: true,
        suppressVectorstoreBadge: true
      });
      mc = sumEvlogCountWarnFailFromEntries(evsFull);
    }
    var ixLogTitle =
      typeof scopedEvlogTitle === "function"
        ? scopedEvlogTitle(indexerCardTitleSortLabel(meta))
        : "Scoped log";
    var full =
      '<div class="sum-full-log sum-full-log--evlog">' +
      sumEvlogPanelHtml({
        scrollTbodyId: fullId,
        warnN: mc.warn,
        failN: mc.fail,
        tbodyInnerHtml: tbodyInner,
        title: ixLogTitle,
        uiPart: expOpts.scopedEvlogUiPart || "indexer.scoped-evlog"
      }) +
      "</div>";
    return (
      '<div class="sum-body">' +
      configureBtn +
      '<div class="sum-section-label">Summary</div>' +
      summaryRows +
      afterSummary +
      recentSection +
      full +
      "</div>"
    );
  }

  function emptyIndexerWatchRootsStore() {
    return { byBucket: {}, byRunId: {}, snapshots: {} };
  }

  function normalizeIndexerWatchRootsStore(o) {
    if (!o || typeof o !== "object") return emptyIndexerWatchRootsStore();
    if (!o.byBucket || typeof o.byBucket !== "object") o.byBucket = {};
    if (!o.byRunId || typeof o.byRunId !== "object") o.byRunId = {};
    if (!o.snapshots || typeof o.snapshots !== "object") o.snapshots = {};
    return o;
  }

  function loadIndexerWatchRootsStore() {
    if (!ctx.indexerWatchRootsStore) {
      ctx.indexerWatchRootsStore = emptyIndexerWatchRootsStore();
    }
    return normalizeIndexerWatchRootsStore(ctx.indexerWatchRootsStore);
  }

  function saveIndexerWatchRootsStore(store) {
    ctx.indexerWatchRootsStore = normalizeIndexerWatchRootsStore(store);
  }

  function latestIndexRunIdFromEvs(evs) {
    if (!evs || !evs.length) return "";
    for (var i = evs.length - 1; i >= 0; i--) {
      var f = getFlat(evs[i].parsed);
      var rid = f.index_run_id != null ? String(f.index_run_id).trim() : "";
      if (rid) return rid;
    }
    return "";
  }

  function indexerScopeKeyFromMetaAndEvs(meta, evs) {
    var p =
      meta.projectId && meta.projectId !== "—" ? String(meta.projectId).trim() : "";
    var fv =
      meta.flavorId && meta.flavorId !== "—" ? String(meta.flavorId).trim() : "";
    if (!p && evs && evs.length) {
      for (var j = evs.length - 1; j >= 0; j--) {
        var g = getFlat(evs[j].parsed);
        var ip = String(g.ingest_project || "").trim();
        if (ip) {
          p = ip;
          if (!fv && g.flavor_id != null) fv = String(g.flavor_id).trim();
          break;
        }
      }
    }
    if (!p) return "";
    return p + "\0" + fv;
  }

  /**
   * Collapse duplicate live Workspaces rows that refer to the same indexer partition (restart /
   * polling produced multiple bucket ids). Prefer workspace id, then indexer_target_key; otherwise
   * keep one card per distinct bucket id.
   */
  function indexerRunTimelineDedupeKey(meta, bucketId) {
    var ws =
      meta.workspaceId && meta.workspaceId !== "\u2014" && String(meta.workspaceId).trim() !== ""
        ? String(meta.workspaceId).trim()
        : "";
    if (ws) return "ws:" + ws;
    var itk =
      meta.indexerKey && String(meta.indexerKey).trim() !== ""
        ? String(meta.indexerKey).trim()
        : "";
    if (itk) return "itk:" + itk;
    var bid = String(bucketId || "").trim();
    return bid ? "bid:" + bid : "none";
  }

  /** When multiple byRun buckets share indexerRunTimelineDedupeKey, keep the richest card. */
  function pickCanonicalIndexerRun(runs) {
    if (!runs || !runs.length) return null;
    if (runs.length === 1) return runs[0];
    var best = runs[0];
    var bi;
    for (bi = 1; bi < runs.length; bi++) {
      var r = runs[bi];
      var lenR = (r && r.events && r.events.length) || 0;
      var lenB = (best && best.events && best.events.length) || 0;
      if (lenR > lenB) best = r;
    }
    return best;
  }

  /** Stable key for deduping indexer cards when bucket id (run.id) churns between polls. */
  function indexerCardIdentityKey(meta) {
    var userLine =
      meta.userLabel && meta.userLabel !== "—" ? String(meta.userLabel).trim() : "—";
    var prLine =
      meta.projectId && meta.projectId !== "—" ? String(meta.projectId).trim() : "—";
    var flavLine =
      meta.flavorId && meta.flavorId !== "—" ? String(meta.flavorId).trim() : "";
    return userLine + "\0" + prLine + "\0" + flavLine;
  }

  function indexerCardIdentityKeyFromSnap(sn) {
    var userLine =
      sn.userLabel && sn.userLabel !== "—" ? String(sn.userLabel).trim() : "—";
    var prLine =
      sn.projectId && sn.projectId !== "—" ? String(sn.projectId).trim() : "—";
    var flavLine =
      sn.flavorId && sn.flavorId !== "—" ? String(sn.flavorId).trim() : "";
    return userLine + "\0" + prLine + "\0" + flavLine;
  }

  /** Same headline as indexer cards; used for stable alphabetical ordering in the Workspaces section. */
  function indexerCardTitleSortLabel(o) {
    if (!o) return "—";
    var userLine =
      o.userLabel && o.userLabel !== "—" ? String(o.userLabel).trim() : "—";
    var prLine =
      o.projectId && o.projectId !== "—" ? String(o.projectId).trim() : "—";
    var flavLine =
      o.flavorId && o.flavorId !== "—" ? String(o.flavorId).trim() : "";
    return flavLine !== ""
      ? userLine + ":" + prLine + ":" + flavLine
      : userLine + ":" + prLine;
  }

  function operatorTenantIdsForCollectionMap() {
    var seen = Object.create(null);
    var out = [];
    var byTenant = ctx.tokenLabelByTenant || {};
    var tk;
    for (tk in byTenant) {
      if (!Object.prototype.hasOwnProperty.call(byTenant, tk)) continue;
      var tid = String(tk).trim();
      if (tid && !seen[tid]) {
        seen[tid] = true;
        out.push(tid);
      }
    }
    var list = ctx.tokenListCache || [];
    var li;
    for (li = 0; li < list.length; li++) {
      var row = list[li] || {};
      var tid2 =
        row.tenant_id != null
          ? String(row.tenant_id).trim()
          : row.tenantId != null
            ? String(row.tenantId).trim()
            : row.tenant != null
              ? String(row.tenant).trim()
              : "";
      if (tid2 && !seen[tid2]) {
        seen[tid2] = true;
        out.push(tid2);
      }
    }
    return out;
  }

  function vectorstoreScopeLabelMapTokenFingerprint() {
    var keys = ctx.tokenLabelByTenant ? Object.keys(ctx.tokenLabelByTenant).sort() : [];
    return keys.join("\n") + "\n" + String((ctx.tokenListCache || []).length);
  }

  var vectorstoreScopeLabelMapCacheRun = null;
  var vectorstoreScopeLabelMapCachePreg = null;
  var vectorstoreScopeLabelMapCacheWsFp = null;
  var vectorstoreScopeLabelMapCacheTokenFp = null;
  var vectorstoreScopeLabelMapCache = null;

  function registerVectorstoreCollectionScopeLabel(map, collName, label) {
    var cn = String(collName != null ? collName : "").trim();
    var lab = String(label != null ? label : "").trim();
    if (!cn || !lab || lab === "—" || lab === "—:—" || lab.indexOf("—:") === 0) return;
    map[cn] = lab;
  }

  function buildVectorstoreCollectionScopeLabelMap() {
    var map = {};
    if (
      !globalThis.ChimeraSettings ||
      !ChimeraSettings.Derive ||
      typeof ChimeraSettings.Derive.vectorstoreCollectionNameFromIndexerMeta !== "function"
    ) {
      return map;
    }
    var byRun = ctx.lastIndexerSummarizeByRun;
    var preg = ctx.lastIndexerSummarizePartitionRegistry;
    if (byRun && typeof byRun === "object") {
      var keys = Object.keys(byRun);
      for (var i = 0; i < keys.length; i++) {
        var run = byRun[keys[i]];
        if (!run || !run.events || !run.events.length) continue;
        var pmeta = null;
        if (
          preg &&
          typeof ChimeraSettings.Derive.indexerPartitionMetaForRun === "function"
        ) {
          pmeta = ChimeraSettings.Derive.indexerPartitionMetaForRun(preg, run.id, run.events, getFlat);
        }
        var meta = collectIndexerRunMeta(run.id, run.events, pmeta);
        meta = mergePersistedIndexerWatchRoots(meta, run.events, run.id);
        var cn = ChimeraSettings.Derive.vectorstoreCollectionNameFromIndexerMeta(meta);
        registerVectorstoreCollectionScopeLabel(map, cn, indexerCardTitleSortLabel(meta));
      }
    }
    var nested = ctx.lastIndexerOperatorWorkspacesNested || [];
    var tenantIds = operatorTenantIdsForCollectionMap();
    var wi;
    for (wi = 0; wi < nested.length; wi++) {
      var ws = nested[wi];
      var wsLabel = operatorManagedWorkspaceTitleText(ws);
      var ti;
      for (ti = 0; ti < tenantIds.length; ti++) {
        var wsCn = ChimeraSettings.Derive.vectorstoreCollectionNameFromIndexerMeta({
          tenantId: tenantIds[ti],
          projectId: ws.project_id != null ? String(ws.project_id).trim() : "",
          flavorId: ws.flavor_id != null ? String(ws.flavor_id).trim() : ""
        });
        registerVectorstoreCollectionScopeLabel(map, wsCn, wsLabel);
      }
    }
    return map;
  }

  function vectorstoreCollectionScopeLabelForLogs(collRaw) {
    var wsFp = ctx.lastIndexerOperatorWorkspacesFingerprint || "";
    var tokenFp = vectorstoreScopeLabelMapTokenFingerprint();
    if (
      ctx.lastIndexerSummarizeByRun !== vectorstoreScopeLabelMapCacheRun ||
      ctx.lastIndexerSummarizePartitionRegistry !== vectorstoreScopeLabelMapCachePreg ||
      vectorstoreScopeLabelMapCacheWsFp !== wsFp ||
      vectorstoreScopeLabelMapCacheTokenFp !== tokenFp
    ) {
      vectorstoreScopeLabelMapCacheRun = ctx.lastIndexerSummarizeByRun;
      vectorstoreScopeLabelMapCachePreg = ctx.lastIndexerSummarizePartitionRegistry;
      vectorstoreScopeLabelMapCacheWsFp = wsFp;
      vectorstoreScopeLabelMapCacheTokenFp = tokenFp;
      vectorstoreScopeLabelMapCache = buildVectorstoreCollectionScopeLabelMap();
    }
    var c = String(collRaw != null ? collRaw : "").trim();
    if (!c) return c;
    var hit = vectorstoreScopeLabelMapCache && vectorstoreScopeLabelMapCache[c];
    if (hit != null && String(hit).trim() !== "" && hit !== c) return String(hit).trim();
    return c;
  }

  function ragCollectionLabelForUi(collRaw) {
    var r = collRaw != null ? String(collRaw).trim() : "";
    if (!r) return "";
    var lab = vectorstoreCollectionScopeLabelForLogs(r);
    return lab && lab !== r ? lab : r;
  }

  function persistIndexerWatchRoots(paths, indexRunId, scopeKey, bucketId) {
    if (!paths || !paths.length) return;
    var store = loadIndexerWatchRootsStore();
    var t = Date.now();
    var copy = paths.map(function (x) {
      return String(x);
    });
    if (bucketId) {
      store.byBucket[bucketId] = {
        paths: copy,
        t: t,
        indexRunId: indexRunId || "",
        scopeKey: scopeKey || ""
      };
    }
    if (indexRunId) {
      store.byRunId[indexRunId] = { paths: copy, scopeKey: scopeKey || "", bucketId: bucketId || "", t: t };
    }
    saveIndexerWatchRootsStore(store);
  }

  /** Remember cards so indexers that fell out of the log buffer still appear (stale row). */
  function rememberIndexerCardSnapshot(bucketId, meta) {
    if (!bucketId || !meta) return;
    var store = loadIndexerWatchRootsStore();
    var idKey = indexerCardIdentityKey(meta);
    for (var sk in store.snapshots) {
      if (!Object.prototype.hasOwnProperty.call(store.snapshots, sk)) continue;
      if (sk === bucketId) continue;
      if (indexerCardIdentityKeyFromSnap(store.snapshots[sk]) === idKey) delete store.snapshots[sk];
    }
    store.snapshots[bucketId] = {
      userLabel: meta.userLabel != null ? String(meta.userLabel) : "—",
      projectId: meta.projectId != null ? String(meta.projectId) : "—",
      flavorId: meta.flavorId != null ? String(meta.flavorId) : "—",
      paths: meta.watchRootPaths && meta.watchRootPaths.length ? meta.watchRootPaths.slice() : [],
      t: Date.now()
    };
    var keys = Object.keys(store.snapshots);
    if (keys.length > 40) {
      var arr = keys.map(function (k) {
        return { k: k, t: store.snapshots[k].t || 0 };
      });
      arr.sort(function (a, b) {
        return a.t - b.t;
      });
      for (var zi = 0; zi < arr.length - 32; zi++) delete store.snapshots[arr[zi].k];
    }
    saveIndexerWatchRootsStore(store);
  }

  function buildIndexerStaleSnapshotCard(bucketId, snap) {
    var userLine =
      snap.userLabel && snap.userLabel !== "—" ? String(snap.userLabel).trim() : "—";
    var prLine =
      snap.projectId && snap.projectId !== "—" ? String(snap.projectId).trim() : "—";
    var flavLine =
      snap.flavorId && snap.flavorId !== "—" ? String(snap.flavorId).trim() : "";
    var titleText = indexerCardTitleSortLabel({
      userLabel: userLine,
      projectId: prLine,
      flavorId: flavLine !== "" ? flavLine : "—"
    });
    var pathsBlock =
      snap.paths && snap.paths.length
        ? "<pre class=\"indexer-paths-pre\">" +
        escapeHtml(snap.paths.join("\n")) +
        "</pre>"
        : '<span class="muted">—</span>';
    var iid = "ix-stale-" + strHash(bucketId);
    var staleMeta = {
      userLabel: userLine,
      projectId: prLine,
      flavorId: flavLine !== "" ? flavLine : "—",
      scopeWorkspaceTotal: null
    };
    return (
      '<details class="sum-card sum-card--indexer-stale" id="' +
      escapeHtml(iid) +
      '">' +
      '<summary data-ui-part="indexer-stale.summary">' +
      '<span class="sum-avatar sum-av-c">IX</span>' +
      '<span class="sum-main"><span class="sum-title">' +
      '<span class="sum-title-indexer-head">' +
      escapeHtml(titleText) +
      "</span>" +
      '</span><span class="sum-sub sum-sub--clamp muted">' +
      escapeHtml("Waiting on status update from an indexer worker") +
      "</span></span>" +
      '<span class="sum-metrics">' +
      indexerWorkspaceCollapsedMetricsHtml(staleMeta, []) +
      "</span>" +
      operatorCardChevronHtml() +
      "</summary>" +
      '<div class="sum-body">' +
      '<div class="sum-section-label">Summary</div>' +
      '<dl class="indexer-run-kv">' +
      indexerExpandedSummaryKvInnerHtml(staleMeta, { omitFileCountIfZero: true }) +
      "<dt>Watched paths</dt><dd>" +
      pathsBlock +
      "</dd></dl>" +
      "</div></details>"
    );
  }

  /**
   * When indexer.run.start drops out of the ring buffer, restore watch roots from the session cache.
   * Lookup order: summarized card bucket id (unique per indexer partition), then index_run_id.
   * (Project+flavor alone is ambiguous when multiple indexers share gateway scope.)
   */
  function mergePersistedIndexerWatchRoots(meta, evs, bucketId) {
    var rid = latestIndexRunIdFromEvs(evs);
    var sk = indexerScopeKeyFromMetaAndEvs(meta, evs);

    if (meta.start && meta.watchRootPaths && meta.watchRootPaths.length) {
      persistIndexerWatchRoots(meta.watchRootPaths, rid, sk, bucketId);
      return meta;
    }

    var store = loadIndexerWatchRootsStore();
    var pick = null;
    if (
      bucketId &&
      store.byBucket[bucketId] &&
      store.byBucket[bucketId].paths &&
      store.byBucket[bucketId].paths.length
    ) {
      pick = store.byBucket[bucketId].paths;
    }
    if (!pick && rid && store.byRunId[rid] && store.byRunId[rid].paths && store.byRunId[rid].paths.length) {
      pick = store.byRunId[rid].paths;
    }
    if (pick && pick.length) {
      var curN = meta.watchRootPaths ? meta.watchRootPaths.length : 0;
      if (!curN || pick.length > curN) {
        meta.watchRootPaths = pick.slice();
        meta.filepath = pick.join("\n");
      }
    }
    return meta;
  }

  /** When logs lack indexer.run.start, use operator-store roots from /api/ui/indexer/config (same scope as managed workspaces). */
  function mergeOperatorStorePathsIntoIndexerMeta(meta) {
    if (!meta || typeof meta !== "object") return meta;
    if (meta.watchRootPaths && meta.watchRootPaths.length) return meta;
    var roots = ctx.lastIndexerOperatorRoots;
    if (!roots || !roots.length) return meta;
    var mp = meta.projectId && meta.projectId !== "—" ? String(meta.projectId).trim() : "";
    if (!mp) return meta;
    var mf = normalizeFlavorMatch(meta.flavorId);
    var mw =
      meta.workspaceId && meta.workspaceId !== "—" ? String(meta.workspaceId).trim() : "";
    var out = [];
    var ri;
    for (ri = 0; ri < roots.length; ri++) {
      var row = roots[ri] || {};
      var rp = row.project_id != null ? String(row.project_id).trim() : "";
      if (rp !== mp) continue;
      var rf = normalizeFlavorMatch(row.flavor_id);
      if (rf !== mf) continue;
      var rw = row.workspace_id != null ? String(row.workspace_id).trim() : "";
      if (mw !== "" && rw !== mw) continue;
      var pth = row.path != null ? String(row.path).trim() : "";
      if (pth && out.indexOf(pth) < 0) out.push(pth);
    }
    if (out.length) {
      meta.watchRootPaths = out;
      meta.filepath = out.join("\n");
      return meta;
    }
    var wsMatchFn = ctx.findOperatorWorkspaceMatchingIndexerMeta;
    var wsMatch = typeof wsMatchFn === "function" ? wsMatchFn(meta) : null;
    if (wsMatch) applyOperatorWorkspacePathsToMeta(meta, wsMatch);
    return meta;
  }

  function indexerMetaForBucketDedup(run, partitionRegistry) {
    var evs = run.events;
    var pmeta = null;
    if (
      partitionRegistry &&
      globalThis.ChimeraSettings &&
      ChimeraSettings.Derive &&
      typeof ChimeraSettings.Derive.indexerPartitionMetaForRun === "function"
    ) {
      pmeta = ChimeraSettings.Derive.indexerPartitionMetaForRun(
        partitionRegistry,
        run.id,
        evs,
        getFlat
      );
    }
    var meta = collectIndexerRunMeta(run.id, evs, pmeta);
    meta = mergePersistedIndexerWatchRoots(meta, evs, run.id);
    meta = mergeOperatorStorePathsIntoIndexerMeta(meta);
    return meta;
  }

  function buildIndexerCard(run, partitionRegistry) {
    var evs = run.events;
    var pmeta = null;
    if (
      partitionRegistry &&
      globalThis.ChimeraSettings &&
      ChimeraSettings.Derive &&
      typeof ChimeraSettings.Derive.indexerPartitionMetaForRun === "function"
    ) {
      pmeta = ChimeraSettings.Derive.indexerPartitionMetaForRun(partitionRegistry, run.id, evs, getFlat);
    }
    var meta = collectIndexerRunMeta(run.id, evs, pmeta);
    meta = mergePersistedIndexerWatchRoots(meta, evs, run.id);
    meta = mergeOperatorStorePathsIntoIndexerMeta(meta);
    if (meta.watchRootPaths && meta.watchRootPaths.length) {
      persistIndexerWatchRoots(
        meta.watchRootPaths,
        latestIndexRunIdFromEvs(evs),
        indexerScopeKeyFromMetaAndEvs(meta, evs),
        run.id
      );
    }
    var matchIx = ctx.findOperatorWorkspaceMatchingIndexerMeta;
    var opWsForIx = typeof matchIx === "function" ? matchIx(meta) : null;
    var wsNumFn = ctx.operatorWorkspaceNumericId;
    var wsNumIx =
      opWsForIx && typeof wsNumFn === "function" ? wsNumFn(opWsForIx) : 0;
    if (opWsForIx) {
      meta.userLabel = resolveLogsOperatorUserLabel();
      meta.projectId =
        opWsForIx.project_id != null ? String(opWsForIx.project_id).trim() : meta.projectId;
      meta.flavorId =
        opWsForIx.flavor_id != null && String(opWsForIx.flavor_id).trim() !== ""
          ? String(opWsForIx.flavor_id).trim()
          : "—";
      meta.workspaceId = canonicalWorkspaceRowIdKey(opWsForIx.id) || meta.workspaceId;
      applyOperatorWorkspacePathsToMeta(meta, opWsForIx);
    }
    var isIxEdit =
      wsNumIx > 0 &&
      ctx.workspaceManagedEditId != null &&
      ctx.workspaceManagedEditId === wsNumIx &&
      ctx.workspaceManagedStaging != null &&
      ctx.workspaceManagedStaging.wsNum === wsNumIx;
    var pathsBlockIx = null;
    if (isIxEdit) {
      pathsBlockIx = buildManagedWorkspacePathsEditHtml(wsNumIx, ctx.workspaceManagedStaging.paths);
    }
    var titleText = workspaceCardTitleFromIndexerMeta(meta);
    var configureBtnIx =
      wsNumIx > 0 ? buildManagedWorkspaceToolbarHtml(wsNumIx, isIxEdit, titleText) : "";
    var expOptsIx = {
      kvOpts: {
        omitFileCountIfZero: true,
        workspaceRowId: wsNumIx > 0 ? canonicalWorkspaceRowIdKey(opWsForIx.id) : undefined
      },
      recentOpts: wsNumIx > 0 ? { omitWhenEmpty: true } : undefined,
      pathsBlockHtml: pathsBlockIx,
      configureBtnHtml: configureBtnIx
    };
    var doneSeen = meta.doneSeen;
    var errRecent = countErrorSignalsInEntries(sliceRecent(evs, RECENT_CARD_STATUS_N));
    var declared = meta.lastDeclaredState ? String(meta.lastDeclaredState).trim() : "";

    var qIng =
      meta.scopeQueueIngestPending != null && !isNaN(Number(meta.scopeQueueIngestPending))
        ? Number(meta.scopeQueueIngestPending)
        : null;
    var qFan =
      meta.scopeQueueFanoutPending != null && !isNaN(Number(meta.scopeQueueFanoutPending))
        ? Number(meta.scopeQueueFanoutPending)
        : null;
    var pRem = null;
    if (qIng != null || qFan != null) {
      pRem = (qIng != null ? qIng : 0) + (qFan != null ? qFan : 0);
    }
    var qTot =
      meta.scopeWorkspaceTotal != null && !isNaN(Number(meta.scopeWorkspaceTotal))
        ? Math.round(Number(meta.scopeWorkspaceTotal))
        : null;

    var st =
      errRecent > 0
        ? { st: "error", cls: "sum-st-error" }
        : doneSeen
          ? { st: "complete", cls: "sum-st-complete" }
          : declared === "recovery"
            ? { st: "recovery", cls: "sum-st-monitor" }
            : pRem !== null && pRem === 0
              ? { st: "idle", cls: "sum-st-complete" }
              : declared === "watch_idle" || declared === "idle"
                ? { st: "waiting", cls: "sum-st-complete" }
                : { st: "indexing", cls: "sum-st-indexing" };
    var indexerCollapsedIdle = st.st === "idle";

    var prog = indexerBuildCardSubtitle(meta, evs);
    var sub = indexerCollapsedIdle
      ? ""
      : '<span class="sum-sub sum-sub--clamp">' + escapeHtml(prog) + "</span>";
    var titleInner =
      '<span class="sum-title-indexer-head">' +
      escapeHtml(titleText) +
      "</span>";
    var backlogLine = "";
    if (pRem !== null && !isNaN(Number(pRem)) && Number(pRem) === 0) {
      backlogLine = "";
    } else if (pRem != null && qTot != null) {
      backlogLine =
        formatInt(Math.round(pRem)) + " remaining of " + formatInt(qTot) + " total";
    } else if (pRem != null) {
      backlogLine = formatInt(Math.round(pRem)) + " remaining of — total";
    } else if (qTot != null) {
      backlogLine = "— remaining of " + formatInt(qTot) + " total";
    } else {
      backlogLine = "—";
    }
    var progressBarHtml = indexerScopeProgressTimelineBarHtml(pRem, qTot, doneSeen);
    var captionSpan =
      backlogLine !== ""
        ? '<span class="indexer-scope-caption">' + escapeHtml(backlogLine) + "</span>"
        : "";
    var indexerScopeProgressReady =
      doneSeen ||
      !!(meta.scopeStatusFlat || meta.scopeStatusEdgeFlat) ||
      (pRem !== null && qTot !== null);
    var progressStack =
      indexerCollapsedIdle || !indexerScopeProgressReady
        ? ""
        : '<div class="indexer-scope-progress" title="Scoped: ingest queue + fan-out file rows pending vs workspace files (from indexer.scope.status)">' +
          progressBarHtml +
          captionSpan +
          "</div>";
    var avatarIndexer = indexerCollapsedIdle
      ? '<span class="sum-avatar sum-av-c sum-av-indexer-idle" aria-hidden="true">\u2713</span>'
      : '<span class="sum-avatar sum-av-c">IX</span>';
    var statusSpan = indexerCollapsedIdle ? "" : serviceSummaryStatusPillHtml(st);
    var workspaceMetrics = indexerWorkspaceCollapsedMetricsHtml(meta, evs);
    var progressMetrics = "";
    if (workspaceMetrics || progressStack !== "" || statusSpan !== "") {
      progressMetrics =
        '<span class="sum-metrics' +
        (progressStack !== "" ? " sum-metrics--indexer-scope" : "") +
        '">' +
        progressStack +
        workspaceMetrics +
        statusSpan +
        "</span>";
    }
    var iid = indexerCardDomIdFromMeta(meta, run.id);
    rememberIndexerCardSnapshot(run.id, meta);
    var detailsCls = "sum-card";
    if (wsNumIx > 0) detailsCls += " sum-card--indexer-operator-workspace";
    if (isIxEdit) detailsCls += " sum-card--workspace-operator-editing";
    var dataManagedAttr =
      wsNumIx > 0 ? ' data-workspace-managed-id="' + escapeHtml(String(wsNumIx)) + '"' : "";
    return (
      '<details class="' +
      detailsCls +
      '" id="' +
      escapeHtml(iid) +
      '"' +
      dataManagedAttr +
      "><summary>" +
      avatarIndexer +
      '<span class="sum-main"><span class="sum-title">' +
      titleInner +
      '</span>' +
      sub +
      "</span>" +
      progressMetrics +
      operatorCardChevronHtml() +
      "</summary>" +
      renderExpandedIndexer(run, evs, meta, partitionRegistry, expOptsIx) +
      "</details>"
    );
  }
  ctx.indexerExpandedSummaryKvInnerHtml = indexerExpandedSummaryKvInnerHtml;
  ctx.renderExpandedIndexer = renderExpandedIndexer;
  ctx.emptyIndexerWatchRootsStore = emptyIndexerWatchRootsStore;
  ctx.normalizeIndexerWatchRootsStore = normalizeIndexerWatchRootsStore;
  ctx.loadIndexerWatchRootsStore = loadIndexerWatchRootsStore;
  ctx.saveIndexerWatchRootsStore = saveIndexerWatchRootsStore;
  ctx.latestIndexRunIdFromEvs = latestIndexRunIdFromEvs;
  ctx.indexerScopeKeyFromMetaAndEvs = indexerScopeKeyFromMetaAndEvs;
  ctx.indexerRunTimelineDedupeKey = indexerRunTimelineDedupeKey;
  ctx.pickCanonicalIndexerRun = pickCanonicalIndexerRun;
  ctx.indexerCardIdentityKey = indexerCardIdentityKey;
  ctx.indexerCardIdentityKeyFromSnap = indexerCardIdentityKeyFromSnap;
  ctx.indexerCardTitleSortLabel = indexerCardTitleSortLabel;
  ctx.persistIndexerWatchRoots = persistIndexerWatchRoots;
  ctx.rememberIndexerCardSnapshot = rememberIndexerCardSnapshot;
  ctx.buildIndexerStaleSnapshotCard = buildIndexerStaleSnapshotCard;
  ctx.mergePersistedIndexerWatchRoots = mergePersistedIndexerWatchRoots;
  ctx.mergeOperatorStorePathsIntoIndexerMeta = mergeOperatorStorePathsIntoIndexerMeta;
  ctx.indexerMetaForBucketDedup = indexerMetaForBucketDedup;
  ctx.indexerScopeProgressTimelineBarHtml = indexerScopeProgressTimelineBarHtml;
  ctx.buildIndexerCard = buildIndexerCard;
  ctx.ragCollectionLabelForUi = ragCollectionLabelForUi;
  ctx.vectorstoreCollectionScopeLabelForLogs = vectorstoreCollectionScopeLabelForLogs;
};
