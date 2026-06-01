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
  var getViewMode = ctx.getViewMode;
  var primaryLogMessage = ctx.primaryLogMessage;
  var sumEvlogPanelHtml = ctx.sumEvlogPanelHtml;
  var sumEvlogBuildTbodyFromServiceEntries = ctx.sumEvlogBuildTbodyFromServiceEntries;
  var sumEvlogCountWarnFailFromEntries = ctx.sumEvlogCountWarnFailFromEntries;
  var scopedEvlogTitle = ctx.scopedEvlogTitle;
  var RECENT_CARD_STATUS_N = ctx.RECENT_CARD_STATUS_N;
  var sliceRecent = ctx.sliceRecent;
  var buildManagedWorkspacePathsEditHtml = ctx.buildManagedWorkspacePathsEditHtml;
  var buildManagedWorkspaceToolbarHtml = ctx.buildManagedWorkspaceToolbarHtml;
  var operatorCardChevronHtml = ctx.operatorCardChevronHtml;
  var serviceSummaryStatusPillHtml = ctx.serviceSummaryStatusPillHtml;

  var indexerEvlogWorkspaceLabelMapCacheKey = null;
  var indexerEvlogWorkspaceLabelMapCache = null;

  function indexerEvlogWorkspaceLabelMapFingerprint() {
    return [
      ctx.lastIndexerSummarizeByRun,
      ctx.lastIndexerSummarizePartitionRegistry,
      ctx.lastIndexerOperatorWorkspacesFingerprint || "",
      typeof ctx.resolveLogsOperatorUserLabel === "function" ? ctx.resolveLogsOperatorUserLabel() : "—"
    ].join("\u0000");
  }

  function indexerEvlogRegisterWorkspaceLabel(map, label, keys) {
    if (!map || !label || label === "—" || label === "—:—" || label.indexOf("—:") === 0) return;
    for (var i = 0; i < keys.length; i++) {
      var k = String(keys[i] != null ? keys[i] : "").trim();
      if (k) map.byKey[k] = label;
    }
  }

  function buildIndexerEvlogWorkspaceLabelMap() {
    var byKey = Object.create(null);
    var byWorkspaceId = Object.create(null);
    var byProjectFlavor = Object.create(null);
    var map = { byKey: byKey, byWorkspaceId: byWorkspaceId, byProjectFlavor: byProjectFlavor };
    var igFn =
      globalThis.ChimeraSettings &&
      ChimeraSettings.Derive &&
      typeof ChimeraSettings.Derive.indexerIgSyntheticGid === "function"
        ? ChimeraSettings.Derive.indexerIgSyntheticGid
        : null;

    var byRun = ctx.lastIndexerSummarizeByRun;
    var preg = ctx.lastIndexerSummarizePartitionRegistry;
    if (byRun && typeof byRun === "object") {
      var runKeys = Object.keys(byRun);
      for (var ri = 0; ri < runKeys.length; ri++) {
        var bucketId = runKeys[ri];
        var run = byRun[bucketId];
        if (!run || !run.events || !run.events.length) continue;
        var pmeta = null;
        if (
          preg &&
          globalThis.ChimeraSettings &&
          ChimeraSettings.Derive &&
          typeof ChimeraSettings.Derive.indexerPartitionMetaForRun === "function"
        ) {
          pmeta = ChimeraSettings.Derive.indexerPartitionMetaForRun(
            preg,
            run.id,
            run.events,
            getFlat
          );
        }
        var meta = collectIndexerRunMeta(run.id, run.events, pmeta);
        if (typeof ctx.mergePersistedIndexerWatchRoots === "function") {
          meta = ctx.mergePersistedIndexerWatchRoots(meta, run.events, run.id);
        }
        var label =
          indexerCardTitleSortLabel(meta);
        indexerEvlogRegisterWorkspaceLabel(map, label, [
          bucketId,
          meta.indexerKey,
          meta.runId,
          meta.workspaceId && meta.workspaceId !== "—" ? meta.workspaceId : ""
        ]);
        var tid = meta.tenantId != null ? String(meta.tenantId).trim() : "";
        var proj = meta.projectId && meta.projectId !== "—" ? String(meta.projectId).trim() : "";
        var flav = meta.flavorId && meta.flavorId !== "—" ? String(meta.flavorId).trim() : "";
        if (proj) {
          byProjectFlavor[
            proj +
              "\u0000" +
              (typeof ctx.normalizeFlavorMatch === "function"
                ? ctx.normalizeFlavorMatch(flav)
                : String(flav || "").trim())
          ] = label;
          if (igFn) indexerEvlogRegisterWorkspaceLabel(map, label, [igFn(tid, proj, flav)]);
        }
        if (meta.workspaceId && meta.workspaceId !== "—") {
          byWorkspaceId[String(meta.workspaceId).trim()] = label;
        }
      }
    }

    var nested = ctx.lastIndexerOperatorWorkspacesNested || [];
    for (var wi = 0; wi < nested.length; wi++) {
      var ws = nested[wi];
      var wsLabel =
        typeof ctx.operatorManagedWorkspaceTitleText === "function"
          ? ctx.operatorManagedWorkspaceTitleText(ws)
          : "—";
      var wsId =
        typeof ctx.canonicalWorkspaceRowIdKey === "function"
          ? ctx.canonicalWorkspaceRowIdKey(ws.id)
          : "";
      var wsNumFn = ctx.operatorWorkspaceNumericId;
      var wsNum = typeof wsNumFn === "function" ? String(wsNumFn(ws)) : "";
      indexerEvlogRegisterWorkspaceLabel(map, wsLabel, [wsId, wsNum]);
      if (wsId) byWorkspaceId[wsId] = wsLabel;
      var wp = String(ws.project_id || "").trim();
      var wf =
        typeof ctx.normalizeFlavorMatch === "function"
          ? ctx.normalizeFlavorMatch(ws.flavor_id)
          : String(ws.flavor_id || "").trim();
      if (wp) byProjectFlavor[wp + "\u0000" + wf] = wsLabel;
    }

    return map;
  }

  function getIndexerEvlogWorkspaceLabelMap() {
    var fp = indexerEvlogWorkspaceLabelMapFingerprint();
    if (fp !== indexerEvlogWorkspaceLabelMapCacheKey) {
      indexerEvlogWorkspaceLabelMapCacheKey = fp;
      indexerEvlogWorkspaceLabelMapCache = buildIndexerEvlogWorkspaceLabelMap();
    }
    return indexerEvlogWorkspaceLabelMapCache;
  }

  function indexerEvlogFlatForEntry(ent) {
    var raw = getFlat(ent.parsed);
    if (
      globalThis.ChimeraSettings &&
      ChimeraSettings.Derive &&
      typeof ChimeraSettings.Derive.indexerAugmentFlat === "function"
    ) {
      return ChimeraSettings.Derive.indexerAugmentFlat(ent, raw);
    }
    return raw;
  }

  function indexerEvlogLineIsProcessWide(f) {
    if (
      globalThis.ChimeraSettings &&
      ChimeraSettings.Derive &&
      typeof ChimeraSettings.Derive.indexerFlatIsServiceGlobalOnly === "function" &&
      ChimeraSettings.Derive.indexerFlatIsServiceGlobalOnly(f)
    ) {
      return true;
    }
    var msg = indexerFlatMsg(f);
    if (msg === "indexer.state") return true;
    if (msg === "gateway.indexer.config") return true;
    if (flatLooksLikeIndexerRunStart(f) || flatLooksLikeIndexerRunProgress(f) || flatLooksLikeIndexerRunDone(f))
      return true;
    if (msg.indexOf("indexer.supervised.") === 0) return true;
    return false;
  }

  function indexerEvlogUserLabelFromFlat(f) {
    if (f.user_label && String(f.user_label).trim() !== "") return String(f.user_label).trim();
    var tid = String(f.tenant_id || f.principal_id || f.tenant || "").trim();
    if (tid && ctx.tokenLabelByTenant[tid]) return String(ctx.tokenLabelByTenant[tid]).trim();
    return typeof ctx.resolveLogsOperatorUserLabel === "function" ? ctx.resolveLogsOperatorUserLabel() : "—";
  }

  function indexerEvlogWorkspaceSourceLabel(ent) {
    var f = indexerEvlogFlatForEntry(ent);
    if (!f || typeof f !== "object") return "";
    if (indexerEvlogLineIsProcessWide(f)) return "";

    var map = getIndexerEvlogWorkspaceLabelMap();
    var itk = String(f.indexer_target_key || "").trim();
    if (itk && map.byKey[itk]) return map.byKey[itk];

    var ik = String(f.indexer_key || "").trim();
    if (ik && map.byKey[ik]) return map.byKey[ik];

    var rid = String(f.index_run_id || "").trim();
    if (rid && map.byKey[rid]) return map.byKey[rid];

    var preg = ctx.lastIndexerSummarizePartitionRegistry;
    if (
      rid &&
      preg &&
      preg[rid] &&
      globalThis.ChimeraSettings &&
      ChimeraSettings.Derive &&
      typeof ChimeraSettings.Derive.indexerBucketGidsForLine === "function"
    ) {
      var gids = ChimeraSettings.Derive.indexerBucketGidsForLine(f, preg[rid]);
      if (gids && gids.length === 1) {
        var gidLab = map.byKey[String(gids[0]).trim()];
        if (gidLab) return gidLab;
      }
    }

    var sws = String(f.scope_workspace_id || "").trim();
    if (sws && map.byWorkspaceId[sws]) return map.byWorkspaceId[sws];

    var proj = String(
      f.scope_project_id || f.project_id || f.ingest_project || ""
    ).trim();
    var flav =
      typeof ctx.normalizeFlavorMatch === "function"
        ? ctx.normalizeFlavorMatch(f.flavor_id)
        : String(f.flavor_id || "").trim();
    if (proj) {
      var pfLab = map.byProjectFlavor[proj + "\u0000" + flav];
      if (pfLab) return pfLab;
      var title = indexerCardTitleSortLabel({
        userLabel: indexerEvlogUserLabelFromFlat(f),
        projectId: proj,
        flavorId: flav || "—"
      });
      if (title && title !== "—" && title.indexOf("—:") !== 0) return title;
    }

    return "";
  }

  /** How long file-level indexer activity stays “fresh” for UI subtitle hints. */
  var INDEXER_IDLE_RECENCY_MS = 120000;

  /** Human label for indexer.state code — canonical mapping lives in derive/indexerPresent.js (goja-tested). */
  function indexerHumanDeclaredState(code) {
    if (
      globalThis.ChimeraSettings &&
      ChimeraSettings.Derive &&
      typeof ChimeraSettings.Derive.indexerDeclaredStateLabel === "function"
    ) {
      return ChimeraSettings.Derive.indexerDeclaredStateLabel(code);
    }
    return code ? String(code) : "";
  }

  function indexerLastFileEventTime(evs) {
    for (var i = evs.length - 1; i >= 0; i--) {
      var f = getFlat(evs[i].parsed);
      var m = indexerFlatMsg(f);
      if (m === "indexer.scope.active_file") {
        var insA = entryInstant(evs[i]);
        if (insA) return insA.getTime();
      }
      if (
        m === "indexer.job.upload" ||
        m === "indexer.job.ingested" ||
        m === "indexer.job.skipped" ||
        m.indexOf("indexer.retry") === 0 ||
        m.indexOf("indexer.job.failed") === 0
      ) {
        var ins = entryInstant(evs[i]);
        if (ins) return ins.getTime();
      }
    }
    return 0;
  }

  function indexerRelFromLatestFileLine(evs) {
    for (var i = evs.length - 1; i >= 0; i--) {
      var f = getFlat(evs[i].parsed);
      var mEarly = indexerFlatMsg(f);
      if (mEarly === "indexer.scope.active_file" && f.rel) return String(f.rel);
      if (!f.rel) continue;
      var m = indexerFlatMsg(f);
      if (
        m === "indexer.job.upload" ||
        m === "indexer.job.ingested" ||
        m === "indexer.job.skipped" ||
        m.indexOf("indexer.retry") === 0 ||
        m.indexOf("indexer.job.failed") === 0
      ) {
        return String(f.rel);
      }
    }
    return "";
  }

  function indexerBuildCardSubtitle(meta, evs) {
    if (meta && meta.lastRecoveryPollFlat && meta.lastRecoveryPollFlat.embed_ok === false) {
      var reason =
        meta.lastRecoveryPollFlat.embed_reason_code ||
        meta.lastRecoveryPollFlat.embed_detail ||
        "indexing unavailable";
      return "Waiting for indexing — " + String(reason).replace(/_/g, " ");
    }
    if (meta && meta.scopeStatusEdgeFlat) {
      var renderGate = globalThis.ChimeraSettings && ChimeraSettings.Render;
      if (renderGate && typeof renderGate.operatorMessage === "function") {
        var gateLine = renderGate.operatorMessage(meta.scopeStatusEdgeFlat, { slug: "indexer.scope.status" });
        if (gateLine && String(gateLine).trim() !== "") return String(gateLine).trim();
      }
    }
    if (meta && meta.lastIngestSummaryFlat) {
      var renderIngest = globalThis.ChimeraSettings && ChimeraSettings.Render;
      if (renderIngest && typeof renderIngest.operatorMessage === "function") {
        var ingestLine = renderIngest.operatorMessage(meta.lastIngestSummaryFlat, {
          slug: "indexer.job.ingested.summary"
        });
        if (ingestLine && String(ingestLine).trim() !== "") return String(ingestLine).trim();
      }
    }
    if (meta && meta.lastSkipSummaryFlat) {
      var render = globalThis.ChimeraSettings && ChimeraSettings.Render;
      if (render && typeof render.operatorMessage === "function") {
        var sumLine = render.operatorMessage(meta.lastSkipSummaryFlat, {
          slug: "indexer.job.skipped.summary"
        });
        if (sumLine && String(sumLine).trim() !== "") return String(sumLine).trim();
      }
    }
    var stateLine = indexerHumanDeclaredState(meta.lastDeclaredState);
    var ft = indexerLastFileEventTime(evs);
    if (!stateLine) {
      var cand =
        meta.lastProg && meta.lastProg.candidates_enqueued != null
          ? String(meta.lastProg.candidates_enqueued)
          : "—";
      stateLine = indexerRunProgressSubtitle(meta.lastProg, meta.doneSeen, cand);
    }

    var rp = meta && meta.scopeLatestRel ? String(meta.scopeLatestRel).trim() : "";
    if (!rp) rp = indexerRelFromLatestFileLine(evs);
    if (rp) {
      var recent = ft && Date.now() - ft <= INDEXER_IDLE_RECENCY_MS;
      var pathShow = recent ? rp : "last file: " + rp;
      return stateLine ? stateLine + " — " + pathShow : pathShow;
    }
    return stateLine || "—";
  }

  function indexerWorkspaceFileCountFromMeta(meta) {
    if (
      meta &&
      meta.scopeWorkspaceTotal != null &&
      !isNaN(Number(meta.scopeWorkspaceTotal))
    ) {
      return Math.round(Number(meta.scopeWorkspaceTotal));
    }
    return null;
  }

  /** Latest qdrant_points for this workspace from scoped logs, then rollup meta. */
  function indexerWorkspaceEmbeddedChunksFromMeta(meta, evs) {
    if (Array.isArray(evs) && evs.length) {
      for (var i = evs.length - 1; i >= 0; i--) {
        var f = getFlat(evs[i].parsed);
        var m = indexerFlatMsg(f);
        if (m !== "indexer.storage.stats" && m.indexOf("indexer.storage.stats") !== 0) continue;
        if (f.qdrant_points != null && f.qdrant_points !== "") {
          var qp = Number(f.qdrant_points);
          if (!isNaN(qp)) return Math.round(qp);
        }
      }
    }
    if (meta && meta.qdrantPointsLive != null && !isNaN(Number(meta.qdrantPointsLive))) {
      return Math.round(Number(meta.qdrantPointsLive));
    }
    if (meta && meta.vectorsStored != null && !isNaN(Number(meta.vectorsStored))) {
      return Math.round(Number(meta.vectorsStored));
    }
    return null;
  }

  function indexerWorkspaceMetricWellHtml(count, icon, title) {
    var lab = count != null && !isNaN(Number(count)) ? formatInt(Math.round(Number(count))) : "—";
    var titleAttr =
      title != null && String(title).trim() !== ""
        ? ' title="' + escapeHtml(String(title)) + '"'
        : "";
    return (
      '<span class="sg-op-inset-well"' +
      titleAttr +
      ">" +
      escapeHtml(lab) +
      ' <span class="material-symbols-outlined material-symbols-outlined--sm" aria-hidden="true">' +
      escapeHtml(icon) +
      "</span></span>"
    );
  }

  function indexerWorkspaceCollapsedMetricsHtml(meta, evs) {
    var files = indexerWorkspaceFileCountFromMeta(meta);
    var chunks = indexerWorkspaceEmbeddedChunksFromMeta(meta, evs);
    return (
      indexerWorkspaceMetricWellHtml(
        files,
        "note_stack",
        "Workspace files tracked by the indexer"
      ) +
      indexerWorkspaceMetricWellHtml(
        chunks,
        "text_snippet",
        "Embedded text chunks stored for search retrieval"
      )
    );
  }

  function badgeForIndexerRunLine(ent) {
    var src = (ent.source || "").toLowerCase();
    var f = getFlat(ent.parsed);
    var msg = String(f.msg || "").toLowerCase();
    if (src === "chimera-vectorstore" || src === "chimera-vectorstore" || msg.indexOf("chimera-vectorstore") >= 0)
      return { cls: "sum-svc-chimera-vectorstore sum-svc-badge-filled sum-svc-chimera-vectorstore-filled", lab: "chimera-vectorstore" };
    return { cls: "sum-svc-chimera-indexer sum-svc-badge-filled sum-svc-chimera-indexer-filled", lab: "chimera-indexer" };
  }

  function indexerRunProgressSubtitle(lastProg, doneSeen, candStr) {
    var lp = lastProg || {};
    var cur =
      lp.chunks_embedded != null
        ? Number(lp.chunks_embedded)
        : lp.chunks_done != null
          ? Number(lp.chunks_done)
          : lp.embedded_chunks != null
            ? Number(lp.embedded_chunks)
            : null;
    var tot =
      lp.chunks_total != null
        ? Number(lp.chunks_total)
        : lp.total_chunks != null
          ? Number(lp.total_chunks)
          : null;
    if (cur != null && tot != null && !isNaN(cur) && !isNaN(tot))
      return "Indexer uploading batch — " + cur + " of " + tot + " chunks indexed";
    if (candStr && candStr !== "—")
      return "Indexer uploading batch — latest counters: " + candStr + " candidates / chunks";
    return doneSeen ? "Indexer run completed" : "Indexer uploading batch — in progress";
  }

  /** Primary log `msg` / `message` (slog may put the human title in one and the slug in the other, or duplicate keys). */
  function indexerFlatMsg(fl) {
    if (
      globalThis.ChimeraSettings &&
      ChimeraSettings.Derive &&
      typeof ChimeraSettings.Derive.indexerFlatMsgForPresent === "function"
    )
      return ChimeraSettings.Derive.indexerFlatMsgForPresent(fl);
    return String(fl.msg != null ? fl.msg : fl.message != null ? fl.message : "")
      .toLowerCase()
      .trim();
  }

  function isIndexerStateFlat(f) {
    if (!f || typeof f !== "object") return false;
    if (indexerFlatMsg(f) === "indexer.state") return true;
    var raw = String(f.msg != null ? f.msg : f.message != null ? f.message : "")
      .toLowerCase()
      .trim();
    if (
      (raw === "indexer state" || raw === "indexer.state") &&
      (f.queue_depth != null || f.ingest_inflight != null || f.state != null || typeof f.watch_mode === "boolean")
    )
      return true;
    return false;
  }

  /** Latest process-wide queue depth / ingest inflight from the newest indexer.state in the log window. */
  function flatLooksLikeIndexerRunStart(fl) {
    var m = indexerFlatMsg(fl);
    if (m === "indexer.run.start" || m === "chimera-indexer run start") return true;
    if (String(fl.service || "").toLowerCase() !== "indexer") return false;
    return fl.root_ids != null && (fl.roots != null || Array.isArray(fl.watch_root_paths));
  }

  function flatLooksLikeIndexerRunDone(fl) {
    var m = indexerFlatMsg(fl);
    if (m.indexOf("indexer.run.done") === 0) return true;
    if (m === "indexer run done" || m === "indexer run stopped") return true;
    return (
      String(fl.service || "").toLowerCase() === "chimera-indexer" &&
      fl.ingest_completed != null &&
      fl.mode != null &&
      String(fl.mode).trim() !== ""
    );
  }

  function flatLooksLikeIndexerRunProgress(fl) {
    var m = indexerFlatMsg(fl);
    if (m.indexOf("indexer.run.progress") === 0 || m === "indexer.run.progress") return true;
    if (m === "initial scan complete") return true;
    return fl.phase != null && String(fl.phase).trim() !== "" && fl.candidates_enqueued != null;
  }

  function flatLooksLikeIndexerJobIngested(fl) {
    var m = indexerFlatMsg(fl);
    if (String(fl.service || "").toLowerCase() !== "chimera-indexer") return false;
    if (m !== "indexer.job.ingested" && m !== "ingested") return false;
    return fl.chunks != null;
  }

  function indexerRecentEvalStatusForFlat(f) {
    var m = indexerFlatMsg(f);
    var rel = f && f.rel != null ? String(f.rel).trim() : "";
    if (!rel) return null;

    if (m === "indexer.scope.active_file") {
      return { rel: rel, st: "evaluating", cls: "sum-st-indexing", detail: "" };
    }
    if (m === "indexer.job.upload") {
      return { rel: rel, st: "uploading", cls: "sum-st-indexing", detail: "" };
    }
    if (m === "indexer.job.ingested" || m === "ingested") {
      var chunks = f && f.chunks != null && !isNaN(Number(f.chunks)) ? Math.round(Number(f.chunks)) : null;
      return {
        rel: rel,
        st: "ingested",
        cls: "sum-st-complete",
        detail: chunks != null ? formatInt(chunks) + " chunks" : ""
      };
    }
    if (m === "indexer.job.skipped") {
      var why = f && f.reason != null ? String(f.reason).replace(/\s+/g, " ").trim() : "";
      if (why.length > 80) why = why.slice(0, 78) + "…";
      return { rel: rel, st: "skipped", cls: "sum-st-complete", detail: why };
    }
    if (m.indexOf("indexer.job.failed") === 0) {
      var errFlat = f && typeof f === "object" ? f : {};
      var detailFn =
        globalThis.ChimeraSettings &&
        ChimeraSettings.Derive &&
        typeof ChimeraSettings.Derive.shortIngestFailureDetail === "function"
          ? ChimeraSettings.Derive.shortIngestFailureDetail
          : globalThis.ChimeraSettings &&
              ChimeraSettings.Render &&
              typeof ChimeraSettings.Render.shortIngestFailureDetail === "function"
            ? ChimeraSettings.Render.shortIngestFailureDetail
            : null;
      var es = detailFn ? detailFn(errFlat) : "";
      if (!es) {
        var err = f && (f.err != null ? f.err : f.error != null ? f.error : "");
        es = err != null ? String(err).replace(/\s+/g, " ").trim() : "";
        if (es.length > 80) es = es.slice(0, 78) + "…";
      }
      return { rel: rel, st: "failed", cls: "sum-st-error", detail: es };
    }
    if (m.indexOf("indexer.retry") === 0) {
      return { rel: rel, st: "retrying", cls: "sum-st-monitor", detail: "" };
    }
    if (m === "rag.retrieve.source") {
      var srcHits =
        f && f.source_hits != null && !isNaN(Number(f.source_hits))
          ? Math.round(Number(f.source_hits))
          : null;
      return {
        rel: rel,
        st: "retrieved",
        cls: "sum-st-retrieved",
        detail: srcHits != null ? formatInt(srcHits) + " hits" : ""
      };
    }
    return null;
  }

  function buildIndexerRecentEvaluatedFilesHtml(evsScope, bucketId, maxItems, recentOpts) {
    recentOpts = recentOpts || {};
    var evs = Array.isArray(evsScope) ? evsScope : [];
    var seen = {};
    var rows = [];
    var want = maxItems != null && !isNaN(Number(maxItems)) ? Math.max(3, Math.min(60, Math.round(Number(maxItems)))) : 10;
    for (var i = evs.length - 1; i >= 0; i--) {
      var f = getFlat(evs[i].parsed);
      var st = indexerRecentEvalStatusForFlat(f);
      if (!st) continue;
      if (seen[st.rel]) continue;
      seen[st.rel] = true;
      if (st.st === "failed" && (f.bytes == null || f.bytes === undefined)) {
        for (var j = i - 1; j >= 0; j--) {
          var fj = getFlat(evs[j].parsed);
          if (String(fj.rel || "").trim() !== st.rel) continue;
          if (indexerFlatMsg(fj) !== "indexer.job.upload" || fj.bytes == null) continue;
          var detailFnBytes =
            globalThis.ChimeraSettings &&
            ChimeraSettings.Derive &&
            typeof ChimeraSettings.Derive.shortIngestFailureDetail === "function"
              ? ChimeraSettings.Derive.shortIngestFailureDetail
              : globalThis.ChimeraSettings &&
                  ChimeraSettings.Render &&
                  typeof ChimeraSettings.Render.shortIngestFailureDetail === "function"
                ? ChimeraSettings.Render.shortIngestFailureDetail
                : null;
          if (detailFnBytes) {
            st = Object.assign({}, st, {
              detail: detailFnBytes(Object.assign({}, f, { bytes: fj.bytes }))
            });
          }
          break;
        }
      }
      var t = formatLogDateTimeLocal(evs[i].ts);
      rows.push({
        ts: evs[i].ts,
        t: t || "—",
        rel: st.rel,
        st: st.st,
        cls: st.cls,
        detail: st.detail || ""
      });
      if (rows.length >= want) break;
    }

    if (recentOpts.omitWhenEmpty && !rows.length) {
      return "";
    }

    var sid = "ix-recent-" + strHash(String(bucketId || ""));
    var html =
      '<div class="sum-metrics-table-wrap indexer-recent-files sg-op-indexer-recent-scroll" id="' +
      escapeHtml(sid) +
      '">' +
      '<table class="sum-metrics-table sum-metrics-table--indexer-recent">' +
      "<colgroup>" +
      '<col class="indexer-recent-col-time">' +
      '<col class="indexer-recent-col-path">' +
      '<col class="indexer-recent-col-detail">' +
      '<col class="indexer-recent-col-status">' +
      "</colgroup>" +
      "<thead><tr><th class=\"indexer-recent-cell-time\">Time</th><th class=\"indexer-recent-cell-path\">Path</th><th class=\"indexer-recent-cell-detail\">Detail</th><th class=\"indexer-recent-cell-status\">Status</th></tr></thead><tbody>";
    if (!rows.length) {
      html +=
        '<tr><td colspan="4" class="muted">No file-level activity in the loaded window yet. Scroll up to load older lines.</td></tr>';
    } else {
      for (var r = 0; r < rows.length; r++) {
        var it = rows[r];
        var lvlClass = "lvl-INFO";
        if (it.st === "failed") lvlClass = "lvl-ERROR";
        else if (it.st === "retrying" || it.st === "skipped") lvlClass = "lvl-WARN";
        else if (it.st === "evaluating" || it.st === "uploading") lvlClass = "lvl-DEBUG";
        else if (it.st === "retrieved") lvlClass = "lvl-INFO";
        var iso = typeof toIsoDatetimeAttr === "function" ? toIsoDatetimeAttr(it.ts) : "";
        var relAgo = typeof formatLogRelativeAgo === "function" ? formatLogRelativeAgo(it.ts) : "";
        html +=
          "<tr>" +
          '<td class="indexer-recent-cell-time sum-evlog__cell--time">' +
          "<time" +
          (iso ? ' datetime="' + escapeHtml(iso) + '"' : "") +
          (relAgo ? ' title="' + escapeHtml(relAgo) + '"' : "") +
          ">" +
          escapeHtml(it.t) +
          "</time></td>" +
          '<td class="indexer-recent-cell-path"><code class="sum-mono-id">' +
          escapeHtml(it.rel) +
          "</code></td>" +
          '<td class="indexer-recent-cell-detail muted">' +
          (it.detail ? escapeHtml(it.detail) : "") +
          "</td>" +
          '<td class="indexer-recent-cell-status"><span class="log-line-sum__lvl ' +
          escapeHtml(lvlClass) +
          '">' +
          escapeHtml(it.st) +
          "</span></td></tr>";
      }
    }
    html += "</tbody></table></div>";
    return html;
  }

  /** Rolls up indexer.run.start / progress / done / job lines for summarized cards. */
  function collectIndexerRunMeta(runId, evs, partitionMeta) {
    if (globalThis.ChimeraSettings && globalThis.ChimeraSettings.Derive && globalThis.ChimeraSettings.Derive.collectIndexerRunMeta) {
      return globalThis.ChimeraSettings.Derive.collectIndexerRunMeta(runId, evs, {
        getFlat: function (p) { return getFlat(p); },
        tokenLabelByTenant: ctx.tokenLabelByTenant,
        indexerFlatMsg: function (fl) { return indexerFlatMsg(fl); },
        flatLooksLikeIndexerRunStart: function (fl) { return flatLooksLikeIndexerRunStart(fl); },
        flatLooksLikeIndexerRunDone: function (fl) { return flatLooksLikeIndexerRunDone(fl); },
        flatLooksLikeIndexerRunProgress: function (fl) { return flatLooksLikeIndexerRunProgress(fl); },
        flatLooksLikeIndexerJobIngested: function (fl) { return flatLooksLikeIndexerJobIngested(fl); },
        partitionMeta: partitionMeta || undefined
      });
    }

    var start = null;
    for (var i = 0; i < evs.length; i++) {
      var fi = getFlat(evs[i].parsed);
      if (flatLooksLikeIndexerRunStart(fi)) {
        start = fi;
        break;
      }
    }
    var lastProg = null;
    var doneFlat = null;
    var doneSeen = false;
    var tenantId = "";
    for (var u = evs.length - 1; u >= 0; u--) {
      var fR = getFlat(evs[u].parsed);
      if (!tenantId && (fR.tenant_id || fR.tenant || fR.principal_id))
        tenantId = String(fR.tenant_id || fR.tenant || fR.principal_id || "").trim();
      if (!lastProg && flatLooksLikeIndexerRunProgress(fR)) lastProg = fR;
      if (flatLooksLikeIndexerRunDone(fR)) {
        doneSeen = true;
        if (!doneFlat) doneFlat = fR;
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
    var lpEmb =
      lastProg &&
      (lastProg.chunks_embedded != null
        ? Number(lastProg.chunks_embedded)
        : lastProg.embedded_chunks != null
          ? Number(lastProg.embedded_chunks)
          : NaN);
    var vectorsStored = null;
    if (vectorsSum > 0) vectorsStored = vectorsSum;
    else if (!isNaN(lpEmb) && lpEmb > 0) vectorsStored = Math.round(lpEmb);

    var ok = 0;
    var fail = 0;
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

    for (var bx = 0; bx < evs.length; bx++) {
      var fb = getFlat(evs[bx].parsed);
      if (String(fb.service || "").toLowerCase() !== "indexer") continue;
      if (!ws && fb.scope_workspace_id) ws = String(fb.scope_workspace_id).trim();
      if (!sp && fb.scope_project_id) sp = String(fb.scope_project_id).trim();
      if (!ip && fb.ingest_project) ip = String(fb.ingest_project).trim();
      if (!flavor && fb.flavor_id) flavor = String(fb.flavor_id).trim();
    }

    var projectId = sp || ip || "—";
    var watchRootPathsFb = [];
    if (start && Array.isArray(start.watch_root_paths) && start.watch_root_paths.length) {
      watchRootPathsFb = start.watch_root_paths.map(function (p) {
        return String(p);
      });
    }
    var filepath = watchRootPathsFb.length ? watchRootPathsFb.join("\n") : "—";

    var userLab = tenantId ? ctx.tokenLabelByTenant[tenantId] || tenantId : "—";

    return {
      runId: runId,
      start: start,
      userLabel: userLab,
      tenantId: tenantId,
      workspaceId: ws || "—",
      projectId: projectId,
      flavorId: flavor || "—",
      filepath: filepath,
      watchRootPaths: watchRootPathsFb,
      doneSeen: doneSeen,
      doneFlat: doneFlat,
      lastProg: lastProg,
      vectorsStored: vectorsStored,
      okCount: ok,
      failCount: fail
    };
  }
  /** Same title line as IX / stale / managed WS cards (USER:PROJECT[:FLAVOR]). */
  function workspaceCardTitleFromIndexerMeta(meta) {
    return indexerCardTitleSortLabel(meta);
  }

  function indexerCardDomIdFromMeta(meta, bucketId) {
    var dedupeKey =
      typeof indexerRunTimelineDedupeKey === "function"
        ? indexerRunTimelineDedupeKey(meta, bucketId)
        : String(bucketId || "");
    return "ix-" + strHash(dedupeKey);
  }

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
          escapeHtml(
            typeof ctx.formatWatchPathsPreHtml === "function"
              ? ctx.formatWatchPathsPreHtml(meta.watchRootPaths)
              : meta.watchRootPaths.join("\n")
          ) +
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
    var evsFull =
      typeof ctx.filterEventsForIndexerScopeFullLog === "function"
        ? ctx.filterEventsForIndexerScopeFullLog(evs, run.id, partitionRegistry || {})
        : evs;
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
      var wsLabel =
        typeof ctx.operatorManagedWorkspaceTitleText === "function"
          ? ctx.operatorManagedWorkspaceTitleText(ws)
          : "—";
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
    var mf =
      typeof ctx.normalizeFlavorMatch === "function"
        ? ctx.normalizeFlavorMatch(meta.flavorId)
        : String(meta.flavorId || "").trim();
    var mw =
      meta.workspaceId && meta.workspaceId !== "—" ? String(meta.workspaceId).trim() : "";
    var out = [];
    var ri;
    for (ri = 0; ri < roots.length; ri++) {
      var row = roots[ri] || {};
      var rp = row.project_id != null ? String(row.project_id).trim() : "";
      if (rp !== mp) continue;
      var rf =
        typeof ctx.normalizeFlavorMatch === "function"
          ? ctx.normalizeFlavorMatch(row.flavor_id)
          : String(row.flavor_id || "").trim();
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
    if (wsMatch && typeof ctx.applyOperatorWorkspacePathsToMeta === "function")
      ctx.applyOperatorWorkspacePathsToMeta(meta, wsMatch);
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
      meta.userLabel =
        typeof ctx.resolveLogsOperatorUserLabel === "function" ? ctx.resolveLogsOperatorUserLabel() : "—";
      meta.projectId =
        opWsForIx.project_id != null ? String(opWsForIx.project_id).trim() : meta.projectId;
      meta.flavorId =
        opWsForIx.flavor_id != null && String(opWsForIx.flavor_id).trim() !== ""
          ? String(opWsForIx.flavor_id).trim()
          : "—";
      meta.workspaceId =
        (typeof ctx.canonicalWorkspaceRowIdKey === "function"
          ? ctx.canonicalWorkspaceRowIdKey(opWsForIx.id)
          : "") || meta.workspaceId;
      if (typeof ctx.applyOperatorWorkspacePathsToMeta === "function")
        ctx.applyOperatorWorkspacePathsToMeta(meta, opWsForIx);
    }
    var isIxEdit =
      wsNumIx > 0 &&
      ctx.workspaceManagedEditId != null &&
      ctx.workspaceManagedEditId === wsNumIx &&
      ctx.workspaceManagedStaging != null &&
      ctx.workspaceManagedStaging.wsNum === wsNumIx;
    var pathsBlockIx = null;
    if (isIxEdit && typeof buildManagedWorkspacePathsEditHtml === "function") {
      pathsBlockIx = buildManagedWorkspacePathsEditHtml(wsNumIx, ctx.workspaceManagedStaging.paths);
    }
    var titleText = workspaceCardTitleFromIndexerMeta(meta);
    var configureBtnIx =
      wsNumIx > 0 && typeof buildManagedWorkspaceToolbarHtml === "function"
        ? buildManagedWorkspaceToolbarHtml(wsNumIx, isIxEdit, titleText)
        : "";
    var expOptsIx = {
      kvOpts: {
        omitFileCountIfZero: true,
        workspaceRowId:
          wsNumIx > 0 && typeof ctx.canonicalWorkspaceRowIdKey === "function"
            ? ctx.canonicalWorkspaceRowIdKey(opWsForIx.id)
            : undefined
      },
      recentOpts: wsNumIx > 0 ? { omitWhenEmpty: true } : undefined,
      pathsBlockHtml: pathsBlockIx,
      configureBtnHtml: configureBtnIx
    };
    var doneSeen = meta.doneSeen;
    var errRecent =
      typeof ctx.countErrorSignalsInEntries === "function"
        ? ctx.countErrorSignalsInEntries(sliceRecent(evs, RECENT_CARD_STATUS_N))
        : 0;
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
  ctx.buildIndexerEvlogWorkspaceLabelMap = buildIndexerEvlogWorkspaceLabelMap;
  ctx.getIndexerEvlogWorkspaceLabelMap = getIndexerEvlogWorkspaceLabelMap;
  ctx.indexerEvlogFlatForEntry = indexerEvlogFlatForEntry;
  ctx.indexerEvlogLineIsProcessWide = indexerEvlogLineIsProcessWide;
  ctx.indexerEvlogUserLabelFromFlat = indexerEvlogUserLabelFromFlat;
  ctx.indexerEvlogWorkspaceSourceLabel = indexerEvlogWorkspaceSourceLabel;
  ctx.indexerHumanDeclaredState = indexerHumanDeclaredState;
  ctx.indexerLastFileEventTime = indexerLastFileEventTime;
  ctx.indexerRelFromLatestFileLine = indexerRelFromLatestFileLine;
  ctx.indexerBuildCardSubtitle = indexerBuildCardSubtitle;
  ctx.indexerWorkspaceFileCountFromMeta = indexerWorkspaceFileCountFromMeta;
  ctx.indexerWorkspaceEmbeddedChunksFromMeta = indexerWorkspaceEmbeddedChunksFromMeta;
  ctx.indexerWorkspaceMetricWellHtml = indexerWorkspaceMetricWellHtml;
  ctx.indexerWorkspaceCollapsedMetricsHtml = indexerWorkspaceCollapsedMetricsHtml;
  ctx.badgeForIndexerRunLine = badgeForIndexerRunLine;
  ctx.indexerRunProgressSubtitle = indexerRunProgressSubtitle;
  ctx.indexerFlatMsg = indexerFlatMsg;
  ctx.isIndexerStateFlat = isIndexerStateFlat;
  ctx.flatLooksLikeIndexerRunStart = flatLooksLikeIndexerRunStart;
  ctx.flatLooksLikeIndexerRunDone = flatLooksLikeIndexerRunDone;
  ctx.flatLooksLikeIndexerRunProgress = flatLooksLikeIndexerRunProgress;
  ctx.flatLooksLikeIndexerJobIngested = flatLooksLikeIndexerJobIngested;
  ctx.indexerRecentEvalStatusForFlat = indexerRecentEvalStatusForFlat;
  ctx.buildIndexerRecentEvaluatedFilesHtml = buildIndexerRecentEvaluatedFilesHtml;
  ctx.collectIndexerRunMeta = collectIndexerRunMeta;
  ctx.indexerCardDomIdFromMeta = indexerCardDomIdFromMeta;
  ctx.workspaceCardTitleFromIndexerMeta = workspaceCardTitleFromIndexerMeta;
  ctx.vectorstoreCollectionScopeLabelForLogs = vectorstoreCollectionScopeLabelForLogs;
};
