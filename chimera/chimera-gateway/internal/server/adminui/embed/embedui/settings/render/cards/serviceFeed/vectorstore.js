/**
 * chimera-vectorstore service card: backend KV, upsert/search metrics.
 */
globalThis.ChimeraSettings = globalThis.ChimeraSettings || {};
globalThis.ChimeraSettings.Render = globalThis.ChimeraSettings.Render || {};
globalThis.ChimeraSettings.Render.Cards = globalThis.ChimeraSettings.Render.Cards || {};
globalThis.ChimeraSettings.Render.Cards.ServiceFeed = globalThis.ChimeraSettings.Render.Cards.ServiceFeed || {};

globalThis.ChimeraSettings.Render.Cards.ServiceFeed.mountVectorstore = function (deps) {
  var ctx = deps.ctx;
  var escapeHtml = deps.escapeHtml;
  var getFlat = deps.getFlat;
  var formatInt = deps.formatInt;
  var primaryLogMessage = deps.primaryLogMessage;

  function vectorstoreCardModel(arr) {
    if (
      globalThis.ChimeraSettings &&
      ChimeraSettings.Derive &&
      typeof ChimeraSettings.Derive.vectorstoreCardModel === "function"
    ) {
      return ChimeraSettings.Derive.vectorstoreCardModel(arr, getFlat, ctx.vectorstoreCollectionScopeLabelForLogs);
    }
    return null;
  }

  function vectorstoreServicePanelMiniHtml(arr) {
    var M = {
      version: "—",
      configuration: "—",
      mode: "—",
      tls: "—",
      tlsGrpc: "—",
      tlsInternal: "—",
      telemetry: "—",
      recovery: "—",
      restPort: null,
      grpcPort: null,
      collLoaded: 0,
      collTotal: 0,
      upsertOk: 0,
      upsertFail: 0,
      deleteOk: 0,
      deleteFail: 0,
      searchOk: 0,
      searchFail: 0
    };
    var derived = vectorstoreCardModel(arr);
    if (derived) M = derived;
    var ports = "—";
    if (M.restPort != null && M.grpcPort != null) ports = String(M.restPort) + " / " + String(M.grpcPort);
    else if (M.restPort != null) ports = String(M.restPort) + " / —";
    else if (M.grpcPort != null) ports = "— / " + String(M.grpcPort);
    var backendLab = "—";
    if (
      globalThis.ChimeraSettings &&
      ChimeraSettings.Derive &&
      typeof ChimeraSettings.Derive.wrapperBackendPanelLabel === "function"
    ) {
      backendLab = ChimeraSettings.Derive.wrapperBackendPanelLabel(M.backendName, M.backendMode);
    }
    var kv =
      '<dl class="indexer-run-kv indexer-run-kv--vectorstore-summary">' +
      "<dt>component</dt><dd>chimera-vectorstore</dd>" +
      "<dt>backend</dt><dd>" +
      escapeHtml(backendLab) +
      "</dd>" +
      "<dt>version</dt><dd>" +
      escapeHtml(M.version || "—") +
      '</dd><dt>configuration</dt><dd>' +
      escapeHtml(M.configuration || "—") +
      '</dd><dt>mode</dt><dd>' +
      escapeHtml(M.mode || "—") +
      '</dd><dt>TLS (REST)</dt><dd>' +
      escapeHtml(M.tls || "—") +
      '</dd><dt>TLS (gRPC)</dt><dd>' +
      escapeHtml(M.tlsGrpc || "—") +
      '</dd><dt>telemetry</dt><dd>' +
      escapeHtml(M.telemetry || "—") +
      '</dd><dt>recovery</dt><dd>' +
      escapeHtml(M.recovery || "—") +
      '</dd><dt>port (REST/gRPC)</dt><dd>' +
      escapeHtml(ports) +
      "</dd></dl>";
    return (
      kv +
      '<div class="sum-mini-row">' +
      '<div class="sum-mini-card">Collections<strong>' +
      escapeHtml(formatInt(M.collLoaded) + " / " + formatInt(M.collTotal)) +
      '</strong><span class="sum-mini-sub">loaded / total</span></div>' +
      '<div class="sum-mini-card">Upsert<strong>' +
      escapeHtml(formatInt(M.upsertOk) + " / " + formatInt(M.upsertFail)) +
      '</strong><span class="sum-mini-sub">success / fail (Not HTTP 200)</span></div>' +
      '<div class="sum-mini-card">Delete<strong>' +
      escapeHtml(formatInt(M.deleteOk) + " / " + formatInt(M.deleteFail)) +
      '</strong><span class="sum-mini-sub">success / fail</span></div>' +
      '<div class="sum-mini-card">Search<strong>' +
      escapeHtml(formatInt(M.searchOk) + " / " + formatInt(M.searchFail)) +
      '</strong><span class="sum-mini-sub">success / fail</span></div></div>'
    );
  }

  function buildVectorstoreCardIntroHtml() {
    return (
      '<div class="qd-card-intro" id="qd-card-intro">' +
      '<p class="qd-card-intro-lead">' +
      "chimera-vectorstore is the local vector store service the indexer fills and retrieval queries—this strip shows whether the wrapper is up and whether writes and searches are succeeding. Weak numbers here often mean thinner RAG before chat complains; counts reflect what the API reported, not a full on-disk audit." +
      "</p>" +
      "</div>"
    );
  }

  var impl = {
    id: "chimera-vectorstore",
    skipTimeline: true,
    deriveCollapsed: function (arr) {
      var qdrCardModel = vectorstoreCardModel(arr);
      var lastMsg = "";
      var last = arr.length ? arr[arr.length - 1] : null;
      if (last) lastMsg = primaryLogMessage(last.parsed, last.text);
      if (qdrCardModel && qdrCardModel.subtitle && qdrCardModel.subtitle !== "—") {
        lastMsg = qdrCardModel.subtitle;
      }
      return { subtitle: lastMsg, qdrCardModel: qdrCardModel };
    },
    collapsedMetricsHtml: function (arr, derived) {
      var vm = derived && derived.qdrCardModel ? derived.qdrCardModel : vectorstoreCardModel(arr);
      if (!vm) return "";
      return (
        '<span class="sum-metrics">' +
        ctx.sgOpInsetWellOkFailHtml(vm.upsertOk || 0, vm.upsertFail || 0, "", {
          leadIcon: "database_upload",
          title: "Upserts · success / fail (not HTTP 200)",
          okIcon: false
        }) +
        ctx.sgOpInsetWellOkFailHtml(vm.searchOk || 0, vm.searchFail || 0, "", {
          leadIcon: "database_search",
          title: "Searches · success / fail (not HTTP 200)",
          okIcon: false
        }) +
        "</span>"
      );
    },
    expandedMiniHtml: function (arr) {
      return buildVectorstoreCardIntroHtml() + vectorstoreServicePanelMiniHtml(arr);
    }
  };

  return {
    impl: impl,
    exports: {
      vectorstoreServicePanelMiniHtml: vectorstoreServicePanelMiniHtml,
      buildVectorstoreCardIntroHtml: buildVectorstoreCardIntroHtml
    }
  };
};
