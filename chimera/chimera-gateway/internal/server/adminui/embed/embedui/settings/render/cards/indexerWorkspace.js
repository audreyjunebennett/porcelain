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
  var filterEventsForIndexerScopeFullLog = ctx.filterEventsForIndexerScopeFullLog;
  var collectIndexerRunMeta = ctx.collectIndexerRunMeta;
  var indexerBuildCardSubtitle = ctx.indexerBuildCardSubtitle;
  var indexerWorkspaceCollapsedMetricsHtml = ctx.indexerWorkspaceCollapsedMetricsHtml;
  var operatorCardChevronHtml = ctx.operatorCardChevronHtml;
  var serviceSummaryStatusPillHtml = ctx.serviceSummaryStatusPillHtml;
  var buildIndexerRecentEvaluatedFilesHtml = ctx.buildIndexerRecentEvaluatedFilesHtml;
  var indexerCardDomIdFromMeta = ctx.indexerCardDomIdFromMeta;
  var workspaceCardTitleFromIndexerMeta = ctx.workspaceCardTitleFromIndexerMeta;
  var getViewMode = ctx.getViewMode;

  /** Operator-store workspace label for summary links (USER:PROJECT[:FLAVOR], no row id). */
  function formatIndexerSupervisedRootLabel(row) {
    if (!row || typeof row !== "object") return "—";
    var fv =
      row.flavor_id != null && String(row.flavor_id).trim() !== ""
        ? String(row.flavor_id).trim()
        : "—";
    return typeof ctx.indexerCardTitleSortLabel === "function" ? ctx.indexerCardTitleSortLabel({
          userLabel: resolveLogsOperatorUserLabel(),
          projectId: row.project_id != null ? String(row.project_id).trim() : "—",
          flavorId: fv
        })
      : "—";
  }

  function indexerWorkspaceCardHrefFromBucket(bucketId, byRun, partitionRegistry) {
    if (!bucketId) return "#";
    var run = byRun && byRun[bucketId];
    if (!run || !run.events || !run.events.length) {
      return "#ix-" + strHash(String(bucketId));
    }
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
        run.events,
        getFlat
      );
    }
    var meta = collectIndexerRunMeta(run.id, run.events, pmeta);
    if (typeof ctx.mergePersistedIndexerWatchRoots === "function") {
      meta = ctx.mergePersistedIndexerWatchRoots(meta, run.events, run.id);
    }
    return "#" + indexerCardDomIdFromMeta(meta, bucketId);
  }

  function indexerWorkspaceCardHref(bucketId) {
    return "#ix-" + strHash(String(bucketId || ""));
  }

  function normalizeFlavorMatch(v) {
    if (v == null || v === "—") return "";
    return String(v).trim();
  }

  /**
   * One canonical key per operator-store workspace row id (flat roots vs nested workspaces,
   * JSON number vs string, leading zeros). Prevents duplicate managed WS cards for the same row.
   */
  function canonicalWorkspaceRowIdKey(raw) {
    if (raw == null || raw === "") return "";
    if (typeof raw === "number" && isFinite(raw)) {
      var tr = Math.trunc(raw);
      if (tr === raw || Math.abs(raw - tr) < 1e-9) return String(tr);
      return String(raw);
    }
    var s = String(raw).trim();
    if (!s) return "";
    if (/^\d+$/.test(s)) return String(parseInt(s, 10));
    return s;
  }

  function deriveNestedWorkspacesFromFlatRoots(roots) {
    if (!roots || !roots.length) return [];
    var byId = {};
    var i;
    for (i = 0; i < roots.length; i++) {
      var r = roots[i] || {};
      var widRaw =
        r.workspace_row_id != null && String(r.workspace_row_id).trim() !== ""
          ? String(r.workspace_row_id).trim()
          : r.workspace_id != null && String(r.workspace_id).trim() !== ""
            ? String(r.workspace_id).trim()
            : "";
      var wid = canonicalWorkspaceRowIdKey(widRaw);
      if (!wid) continue;
      if (!byId[wid]) {
        var idDisp = /^\d+$/.test(wid) ? parseInt(wid, 10) : wid;
        byId[wid] = {
          id: idDisp,
          project_id: r.project_id != null ? String(r.project_id).trim() : "",
          flavor_id: r.flavor_id != null ? String(r.flavor_id).trim() : "",
          paths: []
        };
      }
      var pth = r.path != null ? String(r.path).trim() : "";
      if (!pth) continue;
      var pid =
        r.path_id != null && String(r.path_id).trim() !== "" ? String(r.path_id).trim() : "";
      byId[wid].paths.push(pid ? { id: pid, path: pth } : { path: pth });
    }
    var out = [];
    for (var k in byId) {
      if (Object.prototype.hasOwnProperty.call(byId, k)) out.push(byId[k]);
    }
    out.sort(function (a, b) {
      return Number(a.id) - Number(b.id);
    });
    return dedupeOperatorWorkspacesNested(out);
  }

  /** Stable list by workspace row id — API hydrate / merges must not accumulate duplicate ids. */
  function dedupeOperatorWorkspacesNested(arr) {
    if (!arr || !arr.length) return [];
    var seen = Object.create(null);
    var out = [];
    var i;
    for (i = 0; i < arr.length; i++) {
      var w = arr[i];
      if (!w || w.id == null) continue;
      var k = canonicalWorkspaceRowIdKey(w.id);
      if (!k || seen[k]) continue;
      seen[k] = true;
      out.push(w);
    }
    return out;
  }

  function mergeWorkspaceIntoOperatorNested(ws) {
    if (!ws || ws.id == null) return;
    var wid = canonicalWorkspaceRowIdKey(ws.id);
    if (!wid) return;
    var arr = ctx.lastIndexerOperatorWorkspacesNested.slice();
    var replaced = false;
    var ii;
    for (ii = 0; ii < arr.length; ii++) {
      if (canonicalWorkspaceRowIdKey(arr[ii].id) === wid) {
        arr[ii] = ws;
        replaced = true;
        break;
      }
    }
    if (!replaced) arr.push(ws);
    ctx.lastIndexerOperatorWorkspacesNested = dedupeOperatorWorkspacesNested(arr);
  }

  function syncIndexerOperatorPayloadFromConfigJson(d) {
    if (!d || typeof d !== "object") return;
    var roots = Array.isArray(d.roots) ? d.roots : [];
    ctx.lastIndexerOperatorRoots = roots;
    try {
      ctx.lastIndexerOperatorRootsJson = JSON.stringify(roots);
    } catch (_eSyn) {
      ctx.lastIndexerOperatorRootsJson = "";
    }
    if (Array.isArray(d.workspaces) && d.workspaces.length) {
      var seenWs = {};
      var uniqWs = [];
      var wi;
      for (wi = 0; wi < d.workspaces.length; wi++) {
        var ww = d.workspaces[wi];
        if (!ww || ww.id == null) continue;
        var wkey = canonicalWorkspaceRowIdKey(ww.id);
        if (!wkey) continue;
        if (seenWs[wkey]) {
          var u;
          for (u = 0; u < uniqWs.length; u++) {
            if (canonicalWorkspaceRowIdKey(uniqWs[u].id) === wkey) {
              mergeOperatorWorkspacePathsInto(uniqWs[u], ww);
              break;
            }
          }
          continue;
        }
        seenWs[wkey] = true;
        uniqWs.push(ww);
      }
      ctx.lastIndexerOperatorWorkspacesNested = dedupeOperatorWorkspacesNested(
        uniqWs.length ? uniqWs : deriveNestedWorkspacesFromFlatRoots(roots)
      );
    } else {
      ctx.lastIndexerOperatorWorkspacesNested = dedupeOperatorWorkspacesNested(
        deriveNestedWorkspacesFromFlatRoots(roots)
      );
    }
  }

  function indexerOperatorWorkspaceCardHrefByRowId(wsRowId) {
    return "#ix-opws-" + strHash(String(wsRowId || ""));
  }

  function resolveLogsOperatorUserLabel() {
    var z = ctx.tokenLabelByTenant[""];
    if (z != null && String(z).trim() !== "") return String(z).trim();
    var ks = Object.keys(ctx.tokenLabelByTenant);
    for (var i = 0; i < ks.length; i++) {
      var v = ctx.tokenLabelByTenant[ks[i]];
      if (v != null && String(v).trim() !== "") return String(v).trim();
    }
    return "—";
  }

  /** Same headline as buildIndexerOperatorWorkspaceCard — collapse duplicate DB rows. */
  function operatorManagedWorkspaceTitleText(ws) {
    var fv =
      ws.flavor_id != null && String(ws.flavor_id).trim() !== ""
        ? String(ws.flavor_id).trim()
        : "—";
    return workspaceCardTitleFromIndexerMeta({
      userLabel: resolveLogsOperatorUserLabel(),
      projectId: ws.project_id != null ? String(ws.project_id).trim() : "—",
      flavorId: fv
    });
  }

  function workspaceDraftComparableManagedTitle(d) {
    if (!d) return "";
    var prLine = String(d.projectId != null ? d.projectId : "").trim();
    if (!prLine) return "";
    var fv =
      d.flavorId != null && String(d.flavorId).trim() !== ""
        ? String(d.flavorId).trim()
        : "—";
    return workspaceCardTitleFromIndexerMeta({
      userLabel: resolveLogsOperatorUserLabel(),
      projectId: prLine,
      flavorId: fv
    });
  }

  function operatorWorkspacePaths(ws) {
    var out = [];
    if (!ws || !Array.isArray(ws.paths)) return out;
    var pi;
    for (pi = 0; pi < ws.paths.length; pi++) {
      var row = ws.paths[pi] || {};
      var pth = row.path != null ? String(row.path).trim() : "";
      if (pth && out.indexOf(pth) < 0) out.push(pth);
    }
    return out;
  }

  function normalizeIndexerWatchPathForCompare(p) {
    return String(p || "")
      .trim()
      .replace(/\\/g, "/")
      .replace(/\/+$/, "")
      .toLowerCase();
  }

  function pathsSetEqualForIndexerRoots(a, b) {
    var ae = !a || !a.length;
    var be = !b || !b.length;
    if (ae && be) return true;
    if (ae || be) return false;
    var arrA = a.map(normalizeIndexerWatchPathForCompare).filter(Boolean);
    var arrB = b.map(normalizeIndexerWatchPathForCompare).filter(Boolean);
    arrA.sort();
    arrB.sort();
    return arrA.join("\u0000") === arrB.join("\u0000");
  }

  /** Append watched paths from duplicate API workspace rows sharing the same canonical row id. */
  function mergeOperatorWorkspacePathsInto(target, source) {
    if (!target || !source || !Array.isArray(source.paths)) return;
    if (!target.paths) target.paths = [];
    var seenPath = Object.create(null);
    var pi;
    for (pi = 0; pi < target.paths.length; pi++) {
      var rowT = target.paths[pi] || {};
      var pt = rowT.path != null ? String(rowT.path).trim() : "";
      if (pt) seenPath[normalizeIndexerWatchPathForCompare(pt)] = true;
    }
    for (pi = 0; pi < source.paths.length; pi++) {
      var rowS = source.paths[pi] || {};
      var pth = rowS.path != null ? String(rowS.path).trim() : "";
      if (!pth) continue;
      var nk = normalizeIndexerWatchPathForCompare(pth);
      if (seenPath[nk]) continue;
      seenPath[nk] = true;
      var pid = rowS.id != null && String(rowS.id).trim() !== "" ? String(rowS.id).trim() : "";
      target.paths.push(pid ? { id: pid, path: pth } : { path: pth });
    }
  }

  /** Match supervised YAML root row to a partitioned indexer bucket id (same scope as Workspaces cards). */
  function findIndexerBucketIdForSupervisedRoot(row, byRun, partitionRegistry) {
    if (!row || !byRun || typeof byRun !== "object") return "";
    var rp = row.project_id != null ? String(row.project_id).trim() : "";
    var rf = row.flavor_id != null ? String(row.flavor_id).trim() : "";
    var keys = Object.keys(byRun);
    for (var i = 0; i < keys.length; i++) {
      var run = byRun[keys[i]];
      if (!run || !run.events || !run.events.length) continue;
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
          run.events,
          getFlat
        );
      }
      var meta = collectIndexerRunMeta(run.id, run.events, pmeta);
      if (typeof ctx.mergePersistedIndexerWatchRoots === "function") {
        meta = ctx.mergePersistedIndexerWatchRoots(meta, run.events, run.id);
      }
      var mp = meta.projectId && meta.projectId !== "—" ? String(meta.projectId).trim() : "";
      var mf = normalizeFlavorMatch(meta.flavorId);
      if (mp !== rp) continue;
      if (mf !== rf) continue;
      return run.id;
    }
    return "";
  }

  /** Alphabetical ordering for managed workspace rows (shared by log-derived list + API hydrate). */
  function sortIndexerManagedWorkspaceRows(rows) {
    if (!rows || !rows.length) return rows || [];
    rows.sort(function (a, b) {
      return String(a.label != null ? a.label : "").localeCompare(
        String(b.label != null ? b.label : ""),
        undefined,
        { sensitivity: "base", numeric: true }
      );
    });
    return rows;
  }

  function indexerManagedWorkspacesCommaLinksHtml(items) {
    if (!items || !items.length) return '<span class="muted">—</span>';
    var parts = [];
    for (var j = 0; j < items.length; j++) {
      var it = items[j];
      var lab = it.label != null ? String(it.label) : "";
      var bid = it.bucketId != null ? String(it.bucketId) : "";
      var hrefExplicit = it.href != null ? String(it.href).trim() : "";
      var href = hrefExplicit;
      if (!href && bid) href = indexerWorkspaceCardHref(bid);
      if (!lab || lab === "—") continue;
      if (href) {
        parts.push(
          '<a class="sum-ext-link indexer-svc-ws-link" href="' +
          escapeHtml(href) +
          '">' +
          escapeHtml(lab) +
          "</a>"
        );
      } else {
        parts.push('<span class="indexer-svc-ws-plain">' + escapeHtml(lab) + "</span>");
      }
    }
    return parts.length ? parts.join(", ") : '<span class="muted">—</span>';
  }

  function indexerOperatorWorkspacesFingerprint(nested) {
    if (!nested || !nested.length) return "";
    var ids = [];
    var i;
    for (i = 0; i < nested.length; i++) {
      var w = nested[i];
      if (!w || w.id == null) continue;
      var k = canonicalWorkspaceRowIdKey(w.id);
      if (k) ids.push(k);
    }
    ids.sort();
    return ids.join(",");
  }

  function findIndexerBucketForOperatorWorkspace(ws, byRun, partitionRegistry) {
    if (!ws || ws.id == null || !byRun || typeof byRun !== "object") return "";
    var wkey = canonicalWorkspaceRowIdKey(ws.id);
    var paths = operatorWorkspacePaths(ws);
    var rowR = {
      project_id: ws.project_id,
      flavor_id: ws.flavor_id,
      workspace_id: wkey,
      workspace_row_id: wkey
    };
    if (paths.length) rowR.path = paths[0];
    return findIndexerBucketIdForSupervisedRoot(rowR, byRun, partitionRegistry);
  }

  function hrefForOperatorWorkspaceSummary(ws, bucketId, byRun, partitionRegistry) {
    if (bucketId) return indexerWorkspaceCardHrefFromBucket(bucketId, byRun, partitionRegistry);
    var wkey = canonicalWorkspaceRowIdKey(ws.id);
    return wkey ? indexerOperatorWorkspaceCardHrefByRowId(wkey) : "";
  }

  /** One summary link per operator-store workspace (not per watched path). */
  function buildIndexerManagedWorkspaceSummaryRowsFromOperatorStore(workspaces, byRun, partitionRegistry) {
    var rows = [];
    if (!workspaces || !workspaces.length) return rows;
    var seen = {};
    var wi;
    for (wi = 0; wi < workspaces.length; wi++) {
      var ws = workspaces[wi];
      if (!ws || ws.id == null) continue;
      var wkey = canonicalWorkspaceRowIdKey(ws.id);
      if (!wkey || seen[wkey]) continue;
      seen[wkey] = true;
      var lab = operatorManagedWorkspaceTitleText(ws);
      if (!lab || lab === "—") continue;
      var bidR = findIndexerBucketForOperatorWorkspace(ws, byRun, partitionRegistry);
      rows.push({
        label: lab,
        bucketId: bidR,
        href: hrefForOperatorWorkspaceSummary(ws, bidR, byRun, partitionRegistry)
      });
    }
    sortIndexerManagedWorkspaceRows(rows);
    return rows;
  }

  /**
   * Distinct scopes from partitioned indexer runs with links to matching Workspaces cards (fallback before API).
   */
  function buildIndexerManagedWorkspaceSummaryRowsFromLogs(byRun, partitionRegistry) {
    var seen = {};
    var rows = [];
    if (!byRun || typeof byRun !== "object") return rows;
    var keys = Object.keys(byRun);
    for (var i = 0; i < keys.length; i++) {
      var run = byRun[keys[i]];
      if (!run || !run.events || !run.events.length) continue;
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
          run.events,
          getFlat
        );
      }
      var meta = collectIndexerRunMeta(run.id, run.events, pmeta);
      if (typeof ctx.mergePersistedIndexerWatchRoots === "function") {
        meta = ctx.mergePersistedIndexerWatchRoots(meta, run.events, run.id);
      }
      var lab =
        typeof ctx.indexerCardTitleSortLabel === "function"
          ? ctx.indexerCardTitleSortLabel(meta)
          : "—";
      if (!lab || lab === "—") continue;
      var dedupeKey =
        meta.workspaceId && meta.workspaceId !== "—"
          ? "ws:" + String(meta.workspaceId)
          : run.id || lab;
      if (seen[dedupeKey]) continue;
      seen[dedupeKey] = true;
      rows.push({
        label: lab,
        bucketId: run.id,
        href: "#" + indexerCardDomIdFromMeta(meta, run.id)
      });
    }
    sortIndexerManagedWorkspaceRows(rows);
    return rows;
  }

  function aggregateIndexerManagedWorkspacesHtml(byRun, partitionRegistry) {
    var rows = buildIndexerManagedWorkspaceSummaryRowsFromLogs(byRun, partitionRegistry);
    if (!rows.length) return '<span class="muted">—</span>';
    return indexerManagedWorkspacesCommaLinksHtml(rows);
  }

  function indexerServiceSummaryConfigPathHtml() {
    var pth =
      ctx.lastIndexerOperatorConfigPath != null ? String(ctx.lastIndexerOperatorConfigPath).trim() : "";
    if (pth) return "<code>" + escapeHtml(pth) + "</code>";
    if (ctx.indexerOperatorConfigUnavailable) {
      return '<span class="muted">Not available (supervised indexer config path not set)</span>';
    }
    return '<span class="muted">—</span>';
  }

  function indexerServiceSummaryWorkspacesHtml(svcCtx) {
    svcCtx = svcCtx || {};
    var byRun = svcCtx.byRun;
    var partitionRegistry = svcCtx.partitionRegistry;
    var nested = ctx.lastIndexerOperatorWorkspacesNested;
    if (nested && nested.length) {
      var rows = buildIndexerManagedWorkspaceSummaryRowsFromOperatorStore(
        dedupeOperatorWorkspacesNested(nested.slice()),
        byRun,
        partitionRegistry
      );
      if (rows.length) return indexerManagedWorkspacesCommaLinksHtml(rows);
    }
    return aggregateIndexerManagedWorkspacesHtml(byRun, partitionRegistry);
  }

  function syncIndexerServiceSummaryDom() {
    var wsEl = document.getElementById("svc-indexer-summary-workspaces");
    var cfgEl = document.getElementById("svc-indexer-summary-config-path");
    if (wsEl) {
      wsEl.innerHTML = indexerServiceSummaryWorkspacesHtml({
        byRun: ctx.lastIndexerSummarizeByRun,
        partitionRegistry: ctx.lastIndexerSummarizePartitionRegistry
      });
    }
    if (cfgEl) cfgEl.innerHTML = indexerServiceSummaryConfigPathHtml();
  }

  var indexerServiceSummaryFetchTimer = null;
  function scheduleIndexerServiceSummaryFetch(force) {
    if (ctx.indexerServiceSummaryFetchInFlight) {
      ctx.indexerServiceSummaryFetchWanted = true;
      return;
    }
    if (!force && ctx.indexerOperatorConfigHydratedOnce) return;
    if (indexerServiceSummaryFetchTimer) return;
    indexerServiceSummaryFetchTimer = window.setTimeout(function () {
      indexerServiceSummaryFetchTimer = null;
      hydrateIndexerServiceSummaryFromApi(!!force);
    }, force ? 0 : 200);
  }

  /**
   * Fetches operator indexer config; updates ctx and patches summary DOM only (no full panel rebuild).
   */
  function hydrateIndexerServiceSummaryFromApi(force) {
    if (ctx.indexerServiceSummaryFetchInFlight) {
      ctx.indexerServiceSummaryFetchWanted = true;
      return Promise.resolve();
    }
    ctx.indexerServiceSummaryFetchInFlight = true;
    return fetch("/api/ui/indexer/config", { credentials: "same-origin" })
      .then(function (res) {
        return res.json().then(function (d) {
          if (!res.ok) throw new Error((d && d.error) || res.statusText || "config fetch failed");
          return d;
        });
      })
      .then(function (d) {
        ctx.indexerOperatorConfigUnavailable = false;
        var prevFp = ctx.lastIndexerOperatorWorkspacesFingerprint || "";
        syncIndexerOperatorPayloadFromConfigJson(d);
        var nextFp = indexerOperatorWorkspacesFingerprint(ctx.lastIndexerOperatorWorkspacesNested);
        ctx.lastIndexerOperatorWorkspacesFingerprint = nextFp;
        ctx.lastIndexerOperatorConfigPath = d.path != null ? String(d.path).trim() : "";
        ctx.indexerOperatorConfigHydratedOnce = true;
        syncIndexerServiceSummaryDom();
        if (nextFp !== prevFp && getViewMode() === "summarized" && typeof ctx.scheduleStoryRebuild === "function") {
          ctx.scheduleStoryRebuild();
        }
      })
      .catch(function () {
        ctx.indexerOperatorConfigUnavailable = true;
        ctx.lastIndexerOperatorConfigPath = "";
        syncIndexerServiceSummaryDom();
      })
      .finally(function () {
        ctx.indexerServiceSummaryFetchInFlight = false;
        if (ctx.indexerServiceSummaryFetchWanted) {
          ctx.indexerServiceSummaryFetchWanted = false;
          return hydrateIndexerServiceSummaryFromApi(true);
        }
      });
  }

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
  ctx.operatorManagedWorkspaceTitleText = operatorManagedWorkspaceTitleText;
  ctx.workspaceDraftComparableManagedTitle = workspaceDraftComparableManagedTitle;
  ctx.dedupeOperatorWorkspacesNested = dedupeOperatorWorkspacesNested;
  ctx.canonicalWorkspaceRowIdKey = canonicalWorkspaceRowIdKey;
  ctx.normalizeFlavorMatch = normalizeFlavorMatch;
  ctx.resolveLogsOperatorUserLabel = resolveLogsOperatorUserLabel;
  ctx.operatorWorkspacePaths = operatorWorkspacePaths;
  ctx.pathsSetEqualForIndexerRoots = pathsSetEqualForIndexerRoots;
  ctx.mergeOperatorWorkspacePathsInto = mergeOperatorWorkspacePathsInto;
  ctx.normalizeIndexerWatchPathForCompare = normalizeIndexerWatchPathForCompare;
  ctx.buildIndexerManagedWorkspaceSummaryRowsFromOperatorStore = buildIndexerManagedWorkspaceSummaryRowsFromOperatorStore;
  ctx.aggregateIndexerManagedWorkspacesHtml = aggregateIndexerManagedWorkspacesHtml;
  ctx.indexerServiceSummaryConfigPathHtml = indexerServiceSummaryConfigPathHtml;
  ctx.indexerServiceSummaryWorkspacesHtml = indexerServiceSummaryWorkspacesHtml;
  ctx.syncIndexerServiceSummaryDom = syncIndexerServiceSummaryDom;
  ctx.scheduleIndexerServiceSummaryFetch = scheduleIndexerServiceSummaryFetch;
  ctx.hydrateIndexerServiceSummaryFromApi = hydrateIndexerServiceSummaryFromApi;
  ctx.deriveNestedWorkspacesFromFlatRoots = deriveNestedWorkspacesFromFlatRoots;
  ctx.mergeWorkspaceIntoOperatorNested = mergeWorkspaceIntoOperatorNested;
  ctx.syncIndexerOperatorPayloadFromConfigJson = syncIndexerOperatorPayloadFromConfigJson;
  ctx.buildIndexerOperatorWorkspaceCard = buildIndexerOperatorWorkspaceCard;
};
