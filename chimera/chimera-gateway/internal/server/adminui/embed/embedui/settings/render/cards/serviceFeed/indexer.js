/**
 * chimera-indexer service card: queue depth, wait_roots subtitle, RAG embedding panel.
 */
globalThis.ChimeraSettings = globalThis.ChimeraSettings || {};
globalThis.ChimeraSettings.Render = globalThis.ChimeraSettings.Render || {};
globalThis.ChimeraSettings.Render.Cards = globalThis.ChimeraSettings.Render.Cards || {};
globalThis.ChimeraSettings.Render.Cards.ServiceFeed = globalThis.ChimeraSettings.Render.Cards.ServiceFeed || {};

globalThis.ChimeraSettings.Render.Cards.ServiceFeed.mountIndexer = function (deps) {
  var ctx = deps.ctx;
  var escapeHtml = deps.escapeHtml;
  var getFlat = deps.getFlat;
  var formatInt = deps.formatInt;
  var primaryLogMessage = deps.primaryLogMessage;
  var RECENT_CARD_STATUS_N = deps.RECENT_CARD_STATUS_N;

  function indexerLatestSupervisedWaitFlat(entries) {
    var slice =
      typeof ctx.sliceRecent === "function"
        ? ctx.sliceRecent(entries, RECENT_CARD_STATUS_N)
        : [];
    for (var i = slice.length - 1; i >= 0; i--) {
      var f = getFlat(slice[i].parsed);
      if (String(f.service || "").toLowerCase() !== "indexer") continue;
      var typ = f.type != null ? String(f.type).trim() : "";
      if (typ === "indexer.supervised.wait_roots") return f;
      var m =
        typeof ctx.indexerFlatMsg === "function"
          ? ctx.indexerFlatMsg(f)
          : "";
      if (m === "indexer.supervised.wait_roots") return f;
    }
    return null;
  }

  function latestIndexerStateQueueInflightFromEntries(arr) {
    var qd = null,
      inf = null;
    if (!Array.isArray(arr)) return { queueDepth: qd, ingestInflight: inf };
    for (var i = arr.length - 1; i >= 0; i--) {
      var f = getFlat(arr[i].parsed);
      if (typeof ctx.isIndexerStateFlat === "function" && !ctx.isIndexerStateFlat(f)) continue;
      if (f.queue_depth != null) {
        var n = Number(f.queue_depth);
        if (!isNaN(n)) qd = n;
      }
      if (f.ingest_inflight != null) {
        var n2 = Number(f.ingest_inflight);
        if (!isNaN(n2)) inf = n2;
      }
      break;
    }
    return { queueDepth: qd, ingestInflight: inf };
  }

  function indexerQueueDepthTooltip(qi) {
    qi = qi || {};
    var detail = formatInt(Math.round(Number(qi.queueDepth))) + " queued";
    if (qi.ingestInflight != null && !isNaN(Number(qi.ingestInflight))) {
      detail += " · " + formatInt(Math.round(Number(qi.ingestInflight))) + " in flight";
    }
    return (
      "Indexing queue depth as reported by the indexer service logs." +
      detail +
      "."
    );
  }

  function buildIndexerCardIntroHtml() {
    return (
      '<div class="ix-card-intro" id="ix-card-intro">' +
      '<p class="ix-card-intro-lead">' +
      "Keeps your workspaces indexed and searchable by monitoring watched paths, detecting file changes, and converting updated files into vectors stored in the catalog. Shows system‑wide indexing activity so you can track progress, drift, and any issues across all workspaces." +
      "</p>" +
      "</div>"
    );
  }

  function expandedPrefixHtml(svcCtx) {
    var ragEmbeddingPanel = "";
    if (typeof ctx.ragEmbeddingPanelHtml === "function") {
      ragEmbeddingPanel =
        '<div class="sum-section-label">RAG embedding</div>' + ctx.ragEmbeddingPanelHtml();
    }
    return (
      buildIndexerCardIntroHtml() +
      '<dl class="indexer-run-kv indexer-run-kv--service-aggregate">' +
      "<dt>Managed workspaces</dt><dd id=\"svc-indexer-summary-workspaces\">" +
      (typeof ctx.indexerServiceSummaryWorkspacesHtml === "function" ? ctx.indexerServiceSummaryWorkspacesHtml(svcCtx) : "") +
      '</dd><dt>Indexer config file</dt><dd id="svc-indexer-summary-config-path">' +
      (typeof ctx.indexerServiceSummaryConfigPathHtml === "function" ? ctx.indexerServiceSummaryConfigPathHtml() : "") +
      "</dd>" +
      "</dl>" +
      ragEmbeddingPanel
    );
  }

  var impl = {
    id: "chimera-indexer",
    skipTimeline: true,
    deriveCollapsed: function (arr) {
      var lastMsg = "";
      var last = arr.length ? arr[arr.length - 1] : null;
      if (last) lastMsg = primaryLogMessage(last.parsed, last.text);
      var ixWaitFlat = indexerLatestSupervisedWaitFlat(arr);
      if (ixWaitFlat) {
        var ixWaitProse =
          globalThis.ChimeraSettings &&
            ChimeraSettings.Derive &&
            typeof ChimeraSettings.Derive.indexerProseSummary === "function"
            ? ChimeraSettings.Derive.indexerProseSummary(ixWaitFlat)
            : null;
        if (ixWaitProse && String(ixWaitProse).trim() !== "") lastMsg = String(ixWaitProse).trim();
      }
      var status = null;
      if (ixWaitFlat) status = { st: "idle", cls: "sum-st-monitor" };
      return { subtitle: lastMsg, status: status, ixWaitFlat: ixWaitFlat };
    },
    collapsedMetricsHtml: function (arr) {
      var qiIx = latestIndexerStateQueueInflightFromEntries(arr);
      if (qiIx.queueDepth != null && !isNaN(Number(qiIx.queueDepth))) {
        var qCurIx = formatInt(Math.round(Number(qiIx.queueDepth)));
        var qTooltipIx = indexerQueueDepthTooltip(qiIx);
        return (
          '<span class="sum-metrics">' +
          '<span class="sg-op-inset-well" title="' +
          escapeHtml(qTooltipIx) +
          '">' +
          escapeHtml(qCurIx) +
          ' <span class="material-symbols-outlined material-symbols-outlined--sm" aria-hidden="true">stacks</span></span>' +
          "</span>"
        );
      }
      return "";
    },
    expandedPrefixHtml: expandedPrefixHtml,
    expandedMiniHtml: function () {
      return "";
    },
    badgeForPanel: function (ev) {
      var ixLabFn = ctx.indexerEvlogWorkspaceSourceLabel;
      var ixLab = typeof ixLabFn === "function" ? ixLabFn(ev) : "";
      if (!ixLab) return null;
      return { kind: "indexer-workspace", lab: ixLab, key: ixLab };
    },
    evlogOptions: function () {
      return { showSourceColumn: true, sourceColumnKind: "indexer-workspace" };
    }
  };

  return {
    impl: impl,
    exports: {
      indexerLatestSupervisedWaitFlat: indexerLatestSupervisedWaitFlat,
      latestIndexerStateQueueInflightFromEntries: latestIndexerStateQueueInflightFromEntries,
      indexerQueueDepthTooltip: indexerQueueDepthTooltip,
      buildIndexerCardIntroHtml: buildIndexerCardIntroHtml
    }
  };
};
