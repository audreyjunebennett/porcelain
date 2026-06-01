/**
 * Summarized feed indexer cards (Phase 4b).
 */
globalThis.ChimeraSettings = globalThis.ChimeraSettings || {};
globalThis.ChimeraSettings.Render = globalThis.ChimeraSettings.Render || {};
globalThis.ChimeraSettings.Render.Cards = globalThis.ChimeraSettings.Render.Cards || {};

globalThis.ChimeraSettings.Render.Cards.mountFeedLogIndexerWorkspace = function (ctx) {
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
  var operatorWorkspacePaths = ctx.operatorWorkspacePaths;
  var pathsSetEqualForIndexerRoots = ctx.pathsSetEqualForIndexerRoots;
  var normalizeFlavorMatch = ctx.normalizeFlavorMatch;
  var filterEventsForIndexerScopeFullLog = ctx.filterEventsForIndexerScopeFullLog;
  function dirBasenameForWorkspace(p) {
    if (!p) return "";
    var s = String(p).replace(/[/\\]+$/, "");
    var i = Math.max(s.lastIndexOf("/"), s.lastIndexOf("\\"));
    return i >= 0 ? s.slice(i + 1) : s;
  }

  function formatWatchPathDisplayLine(p) {
    var full = String(p || "").trim();
    if (!full) return "";
    var base = dirBasenameForWorkspace(full);
    if (base && base !== full) return base + " — " + full;
    return full;
  }

  function formatWatchPathsPreHtml(paths) {
    if (!paths || !paths.length) return "";
    var lines = [];
    var pi;
    for (pi = 0; pi < paths.length; pi++) {
      var line = formatWatchPathDisplayLine(paths[pi]);
      if (line) lines.push(line);
    }
    return lines.join("\n");
  }

  function applyOperatorWorkspacePathsToMeta(meta, ws) {
    if (!meta || !ws) return meta;
    var opPaths = operatorWorkspacePaths(ws);
    if (!opPaths.length) return meta;
    var cur = meta.watchRootPaths && meta.watchRootPaths.length ? meta.watchRootPaths.slice() : [];
    var changed = false;
    var oi;
    for (oi = 0; oi < opPaths.length; oi++) {
      if (cur.indexOf(opPaths[oi]) < 0) {
        cur.push(opPaths[oi]);
        changed = true;
      }
    }
    if (changed || !meta.watchRootPaths || !meta.watchRootPaths.length) {
      meta.watchRootPaths = cur.length ? cur : opPaths.slice();
      meta.filepath = meta.watchRootPaths.join("\n");
    }
    return meta;
  }
  function operatorWorkspaceCoveredByIndexerRuns(ws, byRun, partitionRegistry) {
    if (!ws || ws.id == null || !byRun || typeof byRun !== "object") return false;
    var wid = canonicalWorkspaceRowIdKey(ws.id);
    if (!wid) return false;
    var opPaths = operatorWorkspacePaths(ws);
    var opProj = String(ws.project_id || "").trim();
    var opFlav = normalizeFlavorMatch(ws.flavor_id);
    var keys = Object.keys(byRun);
    var hi;
    // Pass 1: workspace id in indexer partition/meta matches operator row. Must run before
    // project/flavor filters — drift between supervised YAML and log-derived project fields caused
    // covered=false and duplicate IX + managed WS cards (runtime: ids 3–7 uncovered while 1–2 covered).
    for (hi = 0; hi < keys.length; hi++) {
      var runP1 = byRun[keys[hi]];
      if (!runP1 || !runP1.events || !runP1.events.length) continue;
      var metaIdFn = ctx.indexerMetaForBucketDedup;
      if (typeof metaIdFn !== "function") continue;
      var metaId = metaIdFn(runP1, partitionRegistry);
      var mw0 =
        metaId.workspaceId && metaId.workspaceId !== "—" ? String(metaId.workspaceId).trim() : "";
      if (mw0 && canonicalWorkspaceRowIdKey(mw0) === wid) return true;
    }
    // Pass 2: project + flavor, then workspace id or path set (legacy empty workspace in logs).
    for (hi = 0; hi < keys.length; hi++) {
      var run = byRun[keys[hi]];
      if (!run || !run.events || !run.events.length) continue;
      var metaFn = ctx.indexerMetaForBucketDedup;
      if (typeof metaFn !== "function") continue;
      var meta = metaFn(run, partitionRegistry);
      var mp = meta.projectId && meta.projectId !== "—" ? String(meta.projectId).trim() : "";
      if (mp !== opProj) continue;
      if (normalizeFlavorMatch(meta.flavorId) !== opFlav) continue;
      var mw = meta.workspaceId && meta.workspaceId !== "—" ? String(meta.workspaceId).trim() : "";
      if (mw && canonicalWorkspaceRowIdKey(mw) === wid) return true;
      if (!mw || mw === "—") {
        if (pathsSetEqualForIndexerRoots(meta.watchRootPaths || [], opPaths)) return true;
      }
    }
    return false;
  }

  function operatorWorkspaceNumericId(ws) {
    if (!ws || ws.id == null) return 0;
    var k = canonicalWorkspaceRowIdKey(ws.id);
    if (/^\d+$/.test(k)) {
      var n = parseInt(k, 10);
      return isNaN(n) ? 0 : n;
    }
    return 0;
  }

  function findOperatorWorkspaceByNumericId(wsNum) {
    if (!wsNum) return null;
    var wsn = ctx.lastIndexerOperatorWorkspacesNested || [];
    var hi;
    for (hi = 0; hi < wsn.length; hi++) {
      if (operatorWorkspaceNumericId(wsn[hi]) === wsNum) return wsn[hi];
    }
    return null;
  }

  /**
   * When a live indexer partition is backed by an operator-store workspace row, surface the same
   * Configure / path editing UI on the IX card (managed-only cards are omitted when "covered").
   */
  function findOperatorWorkspaceMatchingIndexerMeta(meta) {
    if (!meta || !ctx.lastIndexerOperatorWorkspacesNested || !ctx.lastIndexerOperatorWorkspacesNested.length)
      return null;
    var mw =
      meta.workspaceId && meta.workspaceId !== "—" ? String(meta.workspaceId).trim() : "";
    if (mw) {
      var wkey = canonicalWorkspaceRowIdKey(mw);
      if (wkey) {
        var hi;
        for (hi = 0; hi < ctx.lastIndexerOperatorWorkspacesNested.length; hi++) {
          var w = ctx.lastIndexerOperatorWorkspacesNested[hi];
          if (canonicalWorkspaceRowIdKey(w.id) === wkey) return w;
        }
      }
    }
    var mp = meta.projectId && meta.projectId !== "—" ? String(meta.projectId).trim() : "";
    if (!mp) return null;
    var mf = normalizeFlavorMatch(meta.flavorId);
    var mpaths = meta.watchRootPaths && meta.watchRootPaths.length ? meta.watchRootPaths : [];
    if (!mpaths.length) return null;
    for (hi = 0; hi < ctx.lastIndexerOperatorWorkspacesNested.length; hi++) {
      var wx = ctx.lastIndexerOperatorWorkspacesNested[hi];
      var xp = wx.project_id != null ? String(wx.project_id).trim() : "";
      if (xp !== mp) continue;
      if (normalizeFlavorMatch(wx.flavor_id) !== mf) continue;
      if (pathsSetEqualForIndexerRoots(operatorWorkspacePaths(wx), mpaths)) return wx;
    }
    return null;
  }
  function buildIndexerOperatorWorkspaceCard(ws, partitionRegistry) {
    var synthBucket = ctx.operatorWorkspaceSyntheticBucketId;
    var bucketId = typeof synthBucket === "function" ? synthBucket(ws) : "";
    var filterScope = ctx.filterEventsForIndexerScopeFullLog;
    var scopedEvs =
      typeof filterScope === "function"
        ? filterScope(entryCache, bucketId, partitionRegistry)
        : [];
    var syntheticRun = { id: bucketId, events: entryCache };
    var meta = collectIndexerRunMeta(bucketId, scopedEvs, null);
    var mergePersisted = ctx.mergePersistedIndexerWatchRoots;
    if (typeof mergePersisted === "function") meta = mergePersisted(meta, scopedEvs, bucketId);
    var mergeOp = ctx.mergeOperatorStorePathsIntoIndexerMeta;
    if (typeof mergeOp === "function") meta = mergeOp(meta);
    var opPaths = operatorWorkspacePaths(ws);
    applyOperatorWorkspacePathsToMeta(meta, ws);
    if ((!meta.watchRootPaths || !meta.watchRootPaths.length) && opPaths.length) {
      meta.watchRootPaths = opPaths.slice();
      meta.filepath = opPaths.join("\n");
    }
    var fvOp =
      ws.flavor_id != null && String(ws.flavor_id).trim() !== ""
        ? String(ws.flavor_id).trim()
        : "—";
    meta.userLabel = resolveLogsOperatorUserLabel();
    meta.projectId = ws.project_id != null ? String(ws.project_id).trim() : "—";
    meta.flavorId = fvOp;
    meta.workspaceId = canonicalWorkspaceRowIdKey(ws.id) || "—";
    var titleText = workspaceCardTitleFromIndexerMeta({
      userLabel: meta.userLabel,
      projectId: meta.projectId,
      flavorId: fvOp
    });
    var widStr = String(ws.id);
    var iid = "ix-opws-" + strHash(widStr);
    var subProse =
      scopedEvs.length > 0
        ? indexerBuildCardSubtitle(meta, scopedEvs)
        : "Saved · waiting for indexer logs (reload supervised config or restart the indexer if this persists)";
    var wsNum = operatorWorkspaceNumericId(ws);
    var isEdit =
      ctx.workspaceManagedEditId != null &&
      ctx.workspaceManagedEditId === wsNum &&
      ctx.workspaceManagedStaging != null &&
      ctx.workspaceManagedStaging.wsNum === wsNum;
    var pathsBlockHtml = null;
    if (isEdit) {
      pathsBlockHtml = buildManagedWorkspacePathsEditHtml(wsNum, ctx.workspaceManagedStaging.paths);
    }
    var configureBtn = buildManagedWorkspaceToolbarHtml(wsNum, isEdit, titleText);
    var renderExpanded = ctx.renderExpandedIndexer;
    var expanded =
      typeof renderExpanded === "function"
        ? renderExpanded(syntheticRun, entryCache, meta, partitionRegistry, {
      kvOpts: { omitFileCountIfZero: true },
      recentOpts: { omitWhenEmpty: true },
      pathsBlockHtml: pathsBlockHtml,
      configureBtnHtml: configureBtn,
      pathsUiPart: "indexer-operator-workspace.paths",
      scopedEvlogUiPart: "indexer-operator-workspace.scoped-evlog"
    })
        : "";
    var cardCls =
      "sum-card sum-card--collapsible sum-card--indexer-operator-workspace sum-card--workspace-operator" +
      (isEdit ? " sum-card--workspace-operator-editing" : "");
    return (
      '<article class="' +
      cardCls +
      '" open id="' +
      escapeHtml(iid) +
      '" data-workspace-managed-id="' +
      escapeHtml(String(wsNum)) +
      '">' +
      '<header class="sum-card__hdr" data-ui-part="indexer-operator-workspace.summary">' +
      '<span class="sum-avatar sum-av-c" title="Managed workspace">WS</span>' +
      '<span class="sum-main"><span class="sum-title">' +
      '<span class="sum-title-indexer-head">' +
      escapeHtml(titleText) +
      "</span>" +
      '</span><span class="sum-sub sum-sub--clamp muted">' +
      escapeHtml(subProse) +
      "</span></span>" +
      '<span class="sum-metrics">' +
      indexerWorkspaceCollapsedMetricsHtml(meta, scopedEvs) +
      "</span>" +
      operatorCardChevronHtml() +
      "</header>" +
      expanded +
      "</article>"
    );
  }
  ctx.dirBasenameForWorkspace = dirBasenameForWorkspace;
  ctx.formatWatchPathDisplayLine = formatWatchPathDisplayLine;
  ctx.formatWatchPathsPreHtml = formatWatchPathsPreHtml;
  ctx.applyOperatorWorkspacePathsToMeta = applyOperatorWorkspacePathsToMeta;
  ctx.operatorWorkspaceCoveredByIndexerRuns = operatorWorkspaceCoveredByIndexerRuns;
  ctx.operatorWorkspaceNumericId = operatorWorkspaceNumericId;
  ctx.findOperatorWorkspaceByNumericId = findOperatorWorkspaceByNumericId;
  ctx.findOperatorWorkspaceMatchingIndexerMeta = findOperatorWorkspaceMatchingIndexerMeta;
  ctx.buildIndexerOperatorWorkspaceCard = buildIndexerOperatorWorkspaceCard;
};
