globalThis.ChimeraSettings = globalThis.ChimeraSettings || {};
globalThis.ChimeraSettings.Main = function () {
  var params = new URLSearchParams(window.location.search);
  var embedded = params.get("embed") === "1" || window.self !== window.top;
  if (embedded) {
    document.documentElement.classList.add("logs-embedded");
    document.body.classList.add("logs-embedded");
  }
  var tbody = document.getElementById("log-body");
  var statusEl = document.getElementById("status");
  var statusLine =
    globalThis.ChimeraSettings && typeof globalThis.ChimeraSettings.StatusLine === "function"
      ? globalThis.ChimeraSettings.StatusLine(statusEl)
      : null;
  var fltApp = document.getElementById("flt-app");
  var fltLevel = document.getElementById("flt-level");
  var C = globalThis.ChimeraSettings && globalThis.ChimeraSettings.Contracts;
  /** Gateway expanded panel: show 2xx HTTP rows for /health, /status, logs poll/stream, etc. */
  var gatewayPanelShowProbes = false;
  /** Session-only cache for indexer watch roots and stale card snapshots. */
  var indexerWatchRootsStore = { byBucket: {}, byRunId: {}, snapshots: {} };
  var CONV_RECENT_N = 5;
  /** Last N events used for summary-strip status pills ("error" vs active/complete). Matches Last-events preview depth. */
  var RECENT_CARD_STATUS_N = 3;
  var entryCache = [];
  /** Maps gateway tenant_id → token label from api-keys.yaml (via GET /api/ui/tokens). */
  var tokenLabelByTenant = {};
  var maxSeq = 0;
  var stickPx = 160;
  var es = null;
  var pollTimer = null;
  var routingPolicyTouched = false;
  var routingPolicyDraft = null;
  var fallbackTouched = false;
  var routerModelsTouched = false;
  var routerModelsDraft = null;
  var routerThresholdTouched = false;
  var routerThresholdDraft = null;
  var routerEnabledTouched = false;
  var routerEnabledDraft = null;
  var adminUserDrafts = [];
  var nextAdminUserDraftId = 1;
  var virtualModelDrafts = [];
  var nextVirtualModelDraftId = 1;
  /** In-flight provider API key inputs keyed by provider id; survives summarized panel rebuild. */
  var adminProviderKeyDraft = {};
  /** Provider ids shown as summarized admin-provider-* cards (seeded from catalog configured_ids). */
  var adminVisibleProviderIds = [];
  var adminProviderCatalog = [];
  var adminProviderCatalogReady = false;
  var adminVisibleProviderIdsSeeded = false;
  /** In-flight Ollama base URL; null = use adminStateCache value on render. */
  var adminOllamaUrlDraft = null;
  var adminRoutingEditing = false;
  var adminFallbackEditing = false;
  var adminRouterEditing = false;
  /** Provider id whose model availability table is in edit mode, or null. */
  var adminProviderModelsEditingId = null;
  /** Draft availability maps keyed by provider id: { models: { modelId: bool }, saving?: bool }. */
  var adminProviderModelsDraft = {};
  /** Last GET /api/ui/providers/{id}/models response keyed by provider id. */
  var adminProviderModelsCache = {};
  /** When true, provider model usage list includes unavailable (disabled) models. */
  var adminProviderModelsShowUnavailable = {};
  /** Ephemeral token secrets from POST /api/ui/tokens, keyed by tenant_id for in-card masked display + copy. */
  var adminCreatedTokenByTenant = {};
  /** Flat roots from GET /api/ui/indexer/config — fills watched paths when indexer.run.start is outside the log buffer. */
  var lastIndexerOperatorRoots = [];
  var lastIndexerOperatorRootsJson = "";
  /** Nested workspaces from GET /api/ui/indexer/config (or POST save / derived from roots). Drives cards when logs have no bucket yet. */
  var lastIndexerOperatorWorkspacesNested = [];
  var lastIndexerOperatorWorkspacesFingerprint = "";
  var lastIndexerOperatorConfigPath = "";
  var indexerOperatorConfigHydratedOnce = false;
  var indexerOperatorConfigUnavailable = false;
  var indexerServiceSummaryFetchInFlight = false;
  var indexerServiceSummaryFetchWanted = false;
  /** Populated during Workspaces card render: watched paths per synthetic `opws\x1e…` bucket id for full-log filtering. */
  var operatorWsFullLogCtx = {};
  /** From latest indexer.run.start root_scopes in buffer: root_id slug → { workspace_id, path, … }. */
  var indexerRootScopeByRootId = {};
  /** Unsaved workspace rows created from Workspaces → Create (Phase 3). */
  var workspaceDrafts = [];
  var nextWorkspaceDraftId = 1;
  /** Phase 4: operator-managed workspace row in edit mode (numeric workspace id). */
  var workspaceManagedEditId = null;
  /** { wsNum: number, paths: { id: number|null, path: string }[] } — only valid while workspaceManagedEditId matches wsNum. */
  var workspaceManagedStaging = null;
  /** True while native folder picker is open for managed workspace path Add. */
  var workspaceManagedFolderPickerOpen = false;
  var started = false;
  /** Dedup live + historical loads (seq may overlap SSE vs initial tail fetch). */
  var seenSeq = {};
  /** Match server ring + UI budget; trim oldest when exceeded. */
  var CLIENT_CACHE_MAX = 5500;
  /** First poll chunk; indexer queue snapshots can crowd the tail — keep overlap with scrolling backfill (#panel-summarized / raw modes). */
  var INITIAL_TAIL_LIMIT = 720;
  var BACKFILL_CHUNK = 300;
  var RENDER_CHUNK = 160;
  var minLoadedSeq = 0;
  var bufferMinSeqFromServer = 0;
  var olderFetchBusy = false;
  var ssePending = [];
  var sseFlushScheduled = false;
  var levelOptionSet = { "": true, "(none)": true, DEBUG: true, INFO: true, WARN: true, ERROR: true };

  // Summarized-only logs UI (structured/raw modes removed).
  var viewMode = "summarized";
  var viewModeEl = null;
  var filtersBar = null;
  function inferShape(flat, source, rawText) {
    var oc = globalThis.ChimeraSettings && ChimeraSettings.OperatorCopy;
    if (oc && typeof oc.inferShapeForFlat === "function") {
      var regShape = oc.inferShapeForFlat(flat, source);
      if (regShape) return regShape;
    }
    if (!flat) {
      if (source === "chimera-vectorstore" || source === "chimera-broker" || source === "chimera-indexer") return "service." + source;
      return "generic";
    }
    var msg = String(flat.msg != null ? flat.msg : flat.message != null ? flat.message : "").toLowerCase();
    if (msg === "http response" || msg === "gateway.http.access" || (flat.method && flat.path != null && flat.statusCode !== undefined && flat.statusCode !== null))
      return "http.access";
    if (msg === "chat.request") return "chat.request";
    if (msg.indexOf("chat.chimera-broker") === 0 || msg.indexOf("upstream chat") >= 0) return "chat.chimera-broker";
    if (
      msg === "chat.routing.attempt" ||
      msg === "chat.routing.resolved" ||
      msg === "chat.routing.fallback" ||
      msg === "chat.provider_limits.blocked" ||
      msg.indexOf("virtual model fallback attempt") >= 0 ||
      msg.indexOf("virtual model routing resolved") >= 0
    )
      return "chat.routing";
    if (msg.indexOf("rag.") === 0) return "rag";
    if (msg === "ingest.complete" || (msg.indexOf("ingest") === 0 && msg !== "ingest complete")) return "ingest";
    if (msg.indexOf("indexer.run") === 0) return "indexer.run";
    if (
      flat.service &&
      String(flat.service) !== "gateway" &&
      String(flat.service) !== "chimera-gateway"
    )
      return "service." + flat.service;
    if (source === "chimera-vectorstore" || source === "chimera-broker" || source === "chimera-indexer") return "service." + source;
    return "generic";
  }

  function statusPillClass(code) {
    if (globalThis.ChimeraUI && globalThis.ChimeraUI.Pill && typeof globalThis.ChimeraUI.Pill.httpStatusClass === "function") {
      return globalThis.ChimeraUI.Pill.httpStatusClass(code);
    }
    var sc = Number(code);
    if (isNaN(sc)) return "pill-4xx";
    if (sc >= 500) return "pill-5xx";
    if (sc >= 400) return "pill-4xx";
    return "pill-2xx";
  }

  function buildHeadlineHtml(flat, shape) {
    if (!flat) return '<span class="muted">—</span>';
    if (shape === "http.access") {
      var sc = flat.statusCode;
      var auth = flat.authorization ? ' <span class="muted">' + escapeHtml(String(flat.authorization)) + "</span>" : "";
      return (
        '<div class="summary-headline"><span class="' +
        statusPillClass(sc) +
        '">' +
        escapeHtml(String(sc)) +
        "</span> <strong>" +
        escapeHtml(String(flat.method || "")) +
        '</strong> <code class="path-em">' +
        escapeHtml(String(flat.path || "")) +
        '</code> <span class="muted">' +
        (flat.responseTimeMs != null ? escapeHtml(String(flat.responseTimeMs)) + " ms" : "") +
        "</span>" +
        auth +
        "</div>"
      );
    }
    if (
      globalThis.ChimeraSettings &&
      ChimeraSettings.Derive &&
      typeof ChimeraSettings.Derive.chimeraBrokerOperatorLine === "function"
    ) {
      var boHead = ChimeraSettings.Derive.chimeraBrokerOperatorLine(flat);
      if (boHead && String(boHead).trim() !== "") {
        return '<div class="summary-headline">' + escapeHtml(String(boHead).trim()) + "</div>";
      }
    }
    var m = flat.msg != null ? flat.msg : flat.message != null ? flat.message : "";
    var one = [m && String(m), flat.upstreamModel && ("model " + flat.upstreamModel), flat.source && ("source " + flat.source)]
      .filter(Boolean)
      .join(" · ");
    if (!one) one = shape;
    return '<div class="summary-headline">' + escapeHtml(one) + "</div>";
  }

  var headlineKeys = {
    "http.access": { method: true, path: true, statusCode: true, responseTimeMs: true, authorization: true, msg: true, message: true }
  };

  function filterExtrasForSummary(shape, extras, flat) {
    var drop = headlineKeys[shape] || null;
    if (!drop) return extras;
    var out = [];
    for (var i = 0; i < extras.length; i++) {
      if (drop[extras[i].k]) continue;
      out.push(extras[i]);
    }
    return out;
  }

  function buildDetailsColumn(parsed, entryTs, rawText, badgeOpt, opts) {
    opts = opts || {};
    var evLike = { parsed: parsed, text: rawText != null && rawText !== undefined ? rawText : "", ts: entryTs };
    var top = logSummaryHtml(evLike, badgeOpt !== undefined ? badgeOpt : null, opts);
    var grid = buildDetailsCell(parsed.extras);
    return top + '<div class="log-fields-block">' + grid + "</div>";
  }

  function nearBottom() {
    var el = document.documentElement;
    var sh = el.scrollHeight;
    var y = window.scrollY + window.innerHeight;
    return sh - y <= stickPx;
  }

  function nearBottomTextarea(ta) {
    if (!ta) return true;
    return ta.scrollHeight - ta.scrollTop - ta.clientHeight <= stickPx;
  }

  var escapeHtml =
    globalThis.ChimeraSettings && globalThis.ChimeraSettings.escapeHtml
      ? globalThis.ChimeraSettings.escapeHtml
      : function (s) {
        if (s === null || s === undefined) return "";
        var d = document.createElement("div");
        d.textContent = String(s);
        return d.innerHTML;
      };

  // Expose selected helpers for parsing modules (Phase 4 extraction).
  globalThis.ChimeraSettings = globalThis.ChimeraSettings || {};
  globalThis.ChimeraSettings.buildDateTimeCells = buildDateTimeCells;
  globalThis.ChimeraSettings.inferShape = inferShape;

  function formatHumanDateTimeUTC(tsOrDate) {
    if (tsOrDate === null || tsOrDate === undefined || tsOrDate === "") return "";
    var d = tsOrDate instanceof Date ? tsOrDate : new Date(tsOrDate);
    if (isNaN(d.getTime())) return String(tsOrDate).replace("T", " ");
    try {
      return new Intl.DateTimeFormat("en-US", {
        timeZone: "UTC",
        weekday: "short",
        month: "short",
        day: "numeric",
        year: "numeric",
        hour: "numeric",
        minute: "2-digit",
        second: "2-digit",
        hour12: true
      }).format(d) + " UTC";
    } catch (err) {
      return d.toUTCString();
    }
  }

  function formatStackedDateTimeCell(d, timeZone) {
    var optsTime = {
      hour: "numeric",
      minute: "2-digit",
      second: "2-digit",
      hour12: true
    };
    var optsDate = {
      weekday: "short",
      month: "short",
      day: "numeric",
      year: "numeric"
    };
    if (timeZone) {
      optsTime.timeZone = timeZone;
      optsDate.timeZone = timeZone;
    }
    var timeStr = new Intl.DateTimeFormat("en-US", optsTime).format(d);
    var dateStr = new Intl.DateTimeFormat("en-US", optsDate).format(d);
    return (
      '<div class="dt-stack">' +
      '<span class="dt-line dt-time">' +
      escapeHtml(timeStr) +
      "</span>" +
      '<span class="dt-line dt-date">' +
      escapeHtml(dateStr) +
      "</span></div>"
    );
  }

  function buildDateTimeCells(instant, entryTS) {
    if (instant && !isNaN(instant.getTime())) {
      return {
        utc: formatStackedDateTimeCell(instant, "UTC"),
        local: formatStackedDateTimeCell(instant, undefined)
      };
    }
    var raw = "";
    if (entryTS !== null && entryTS !== undefined && entryTS !== "") {
      raw = formatHumanDateTimeUTC(entryTS);
    }
    if (!raw) {
      var dash = '<div class="dt-stack"><span class="dt-line muted">—</span></div>';
      return { utc: dash, local: dash };
    }
    return {
      utc:
        '<div class="dt-stack dt-fallback"><span class="dt-line dt-time">' +
        escapeHtml(raw) +
        "</span></div>",
      local: '<div class="dt-stack"><span class="dt-line muted">—</span></div>'
    };
  }

  var parseLogText =
    globalThis.ChimeraSettings && globalThis.ChimeraSettings.parseLogText
      ? globalThis.ChimeraSettings.parseLogText
      : function (source, text, entryTS) {
        // If parse module didn't load, keep a minimal fallback.
        return {
          app: source || "—",
          dtUtcHtml: "",
          dtLocalHtml: "",
          levelCanon: null,
          levelLabel: "—",
          extras: [{ k: "text", v: text }],
          rawFlat: null,
          shape: "generic"
        };
      };
  var filtersCtx = {
    fltAppEl: fltApp,
    fltLevelEl: fltLevel,
    tbodyEl: tbody,
    levelOptionSet: levelOptionSet,
    viewModeGetter: function () { return viewMode; },
    rebuildRawLogsTextarea: function (opts) { return rebuildRawLogsTextarea(opts); },
    nearBottomTextarea: nearBottomTextarea
  };

  function ensureAppOption(app) {
    if (globalThis.ChimeraSettings && globalThis.ChimeraSettings.Filters) {
      return globalThis.ChimeraSettings.Filters.ensureAppOption(filtersCtx, app);
    }
  }
  function ensureLevelOption(lvl) {
    if (globalThis.ChimeraSettings && globalThis.ChimeraSettings.Filters) {
      return globalThis.ChimeraSettings.Filters.ensureLevelOption(filtersCtx, lvl);
    }
  }
  function entryMatchesFilters(parsed) {
    if (globalThis.ChimeraSettings && globalThis.ChimeraSettings.Filters) {
      return globalThis.ChimeraSettings.Filters.entryMatches(filtersCtx, parsed);
    }
    return true;
  }

  function rebuildRawLogsTextarea(opts) {
    if (globalThis.ChimeraSettings && globalThis.ChimeraSettings.RawLogs) {
      return globalThis.ChimeraSettings.RawLogs.rebuild({ entryCache: entryCache, entryMatchesFilters: entryMatchesFilters }, opts);
    }
  }

  function appendRawLineToTextarea(ent, follow) {
    if (globalThis.ChimeraSettings && globalThis.ChimeraSettings.RawLogs) {
      return globalThis.ChimeraSettings.RawLogs.appendRawLine({}, ent, follow);
    }
  }

  function copyRawLogsToClipboard() {
    if (globalThis.ChimeraSettings && globalThis.ChimeraSettings.RawLogs) {
      return globalThis.ChimeraSettings.RawLogs.copyToClipboard({});
    }
  }

  function applyFilters() { }

  function applyViewLayout() {
    var psu = document.getElementById("panel-summarized");
    if (!psu) return;
    if (appCtx.syncChimeraBrokerProviderPolling) appCtx.syncChimeraBrokerProviderPolling();
    if (appCtx.syncUiStatePolling) appCtx.syncUiStatePolling();
    try {
      document.body.classList.toggle("logs-summarized", true);
      document.body.classList.toggle("logs-raw", false);
      document.body.classList.toggle("logs-raw-logs", false);
    } catch (x) { }
    viewMode = "summarized";
    if (appCtx.refreshSummarizedPanel) appCtx.refreshSummarizedPanel();
    if (appCtx.syncMetricsPolling) appCtx.syncMetricsPolling();
  }

  function getFlat(parsed) {
    return parsed.rawFlat || {};
  }

  /** Map legacy + normalized log service/source strings to summarized Services bucket keys. */
  function normalizeServiceBucketKey(svc, source) {
    var s = String(svc || "").trim().toLowerCase();
    var src = String(source || "").trim().toLowerCase();
    var alias = {
      gateway: "chimera-gateway",
      vectorstore: "chimera-vectorstore",
      broker: "chimera-broker",
      indexer: "chimera-indexer",
      "chimera-gateway": "chimera-gateway",
      "chimera-vectorstore": "chimera-vectorstore",
      "chimera-broker": "chimera-broker",
      "chimera-indexer": "chimera-indexer",
    };
    if (alias[s]) return alias[s];
    if (alias[src]) return alias[src];
    return "";
  }

  var strHash =
    globalThis.ChimeraSettings && globalThis.ChimeraSettings.strHash
      ? globalThis.ChimeraSettings.strHash
      : function (s) {
        var h = 0;
        var t = String(s);
        for (var i = 0; i < t.length; i++) h = ((h << 5) - h) + t.charCodeAt(i) | 0;
        return "fc" + (h >>> 0).toString(16);
      };

  var entryInstant =
    globalThis.ChimeraSettings && globalThis.ChimeraSettings.entryInstant
      ? globalThis.ChimeraSettings.entryInstant
      : function (entry) {
        if (!entry || entry.ts === null || entry.ts === undefined || entry.ts === "") return null;
        var d = entry.ts instanceof Date ? entry.ts : new Date(entry.ts);
        return isNaN(d.getTime()) ? null : d;
      };

  var humanDurationMs =
    globalThis.ChimeraSettings && globalThis.ChimeraSettings.humanDurationMs
      ? globalThis.ChimeraSettings.humanDurationMs
      : function (ms) {
        if (ms == null || isNaN(ms) || ms < 0) return "—";
        if (ms < 1000) return Math.round(ms) + " ms";
        if (ms < 60000) return (ms / 1000).toFixed(1) + " s";
        if (ms < 3600000) return Math.round(ms / 60000) + " min";
        return (ms / 3600000).toFixed(1) + " h";
      };

  function rollupHTTPInConversation(events) {
    var count = 0,
      sumMs = 0;
    var maxStatus = 0;
    for (var i = 0; i < events.length; i++) {
      if (events[i].parsed.shape !== "http.access") continue;
      var f = getFlat(events[i].parsed);
      if (f.statusCode === undefined || f.statusCode === null) continue;
      count++;
      var sc = Number(f.statusCode);
      if (!isNaN(sc) && sc > maxStatus) maxStatus = sc;
      var rt = Number(f.responseTimeMs);
      if (!isNaN(rt)) sumMs += rt;
    }
    if (count === 0) return null;
    return { count: count, sumMs: sumMs, worst: maxStatus };
  }

  function contextGrowthStripHtml(events) {
    var keys = [
      "turn_index",
      "turnIndex",
      "context_tokens_est",
      "context_chars_est",
      "rag_hits",
      "hits",
      "chunks",
      "tool_rounds",
      "response_tokens_est"
    ];
    for (var i = events.length - 1; i >= 0; i--) {
      var f = getFlat(events[i].parsed);
      var bits = [];
      for (var k = 0; k < keys.length; k++) {
        var key = keys[k];
        if (f[key] !== undefined && f[key] !== null && f[key] !== "") {
          bits.push(key.replace(/_/g, " ") + ": " + formatExtraValue(f[key]));
        }
      }
      if (bits.length)
        return '<div class="context-strip">' + escapeHtml(bits.join(" · ")) + "</div>";
    }
    return "";
  }

  var MAX_PRIMARY_MSG_CHARS = 900;
  /** Conversation expanded card: show Context strip (contextGrowthStripHtml). Off until UI is refined. */
  var SHOW_CONV_EXPANDED_CONTEXT_STRIP = false;

  function pad2(n) {
    return n < 10 ? "0" + n : String(n);
  }

  function formatLogDateTimeLocal(ts) {
    if (ts === null || ts === undefined || ts === "") return "—";
    var d = ts instanceof Date ? ts : new Date(ts);
    if (isNaN(d.getTime())) return String(ts).replace("T", " ").slice(0, 23);
    return (
      d.getFullYear() +
      "-" +
      pad2(d.getMonth() + 1) +
      "-" +
      pad2(d.getDate()) +
      " " +
      pad2(d.getHours()) +
      ":" +
      pad2(d.getMinutes()) +
      ":" +
      pad2(d.getSeconds())
    );
  }

  function toIsoDatetimeAttr(ts) {
    if (ts === null || ts === undefined || ts === "") return "";
    var d = ts instanceof Date ? ts : new Date(ts);
    if (isNaN(d.getTime())) return "";
    try {
      return d.toISOString();
    } catch (e) {
      return "";
    }
  }

  /** Plain-language one-liners for gateway lifecycle / RAG slugs (registry-driven; see logs/render/operatorMessage.js). */
  function operatorFriendlyGatewayMsg(flat, opts) {
    if (
      globalThis.ChimeraSettings &&
      ChimeraSettings.Render &&
      typeof ChimeraSettings.Render.operatorMessage === "function"
    ) {
      return ChimeraSettings.Render.operatorMessage(flat, opts) || "";
    }
    return "";
  }

  function primaryLogMessage(parsed, rawText, opts) {
    opts = opts || {};
    var forEventLog = opts.forEventLog === true;
    var rf = getFlat(parsed);
    var sh = parsed.shape || "";
    if (sh === "http.access" && rf.statusCode !== undefined && rf.statusCode !== null) {
      var line =
        (rf.method || "?") +
        " " +
        (rf.path || "") +
        (forEventLog ? "" : " → " + rf.statusCode) +
        (rf.responseTimeMs != null ? " · " + rf.responseTimeMs + " ms" : "");
      return line.length > MAX_PRIMARY_MSG_CHARS ? line.slice(0, MAX_PRIMARY_MSG_CHARS - 1) + "…" : line;
    }
    var opOpts = { forEventLog: forEventLog };
    if (opts.convEvlogMeta) opOpts.convEvlogMeta = opts.convEvlogMeta;
    if (typeof chimeraVectorstoreCollectionScopeLabelForLogs === "function") {
      opOpts.resolveColl = chimeraVectorstoreCollectionScopeLabelForLogs;
    }
    if (
      globalThis.ChimeraSettings &&
      ChimeraSettings.Render &&
      typeof ChimeraSettings.Render.operatorMessage === "function"
    ) {
      var registryOp = ChimeraSettings.Render.operatorMessage(rf, opOpts);
      if (registryOp && String(registryOp).trim() !== "") {
        var ro = String(registryOp).trim();
        return ro.length > MAX_PRIMARY_MSG_CHARS ? ro.slice(0, MAX_PRIMARY_MSG_CHARS - 1) + "…" : ro;
      }
    }
    var m = "";
    if (rf.msg != null && rf.msg !== "") m = String(rf.msg);
    else if (rf.message != null && rf.message !== "") m = String(rf.message);
    else if (rawText) m = String(rawText).trim();
    if (!m) m = "—";
    var slug = m.toLowerCase();
    var slugIx = indexerFlatMsg(rf);
    if (
      rf.service === "indexer" ||
      slug.indexOf("indexer.") === 0 ||
      slugIx.indexOf("indexer.") === 0 ||
      slugIx.indexOf("gateway.indexer") === 0 ||
      slugIx === "rag.retrieve.source"
    ) {
      var prose =
        globalThis.ChimeraSettings &&
          ChimeraSettings.Derive &&
          typeof ChimeraSettings.Derive.indexerProseSummary === "function"
          ? ChimeraSettings.Derive.indexerProseSummary(rf)
          : null;
      if (prose && String(prose).trim() !== "") m = String(prose).trim();
      else {
        var bitsIx = [m];
        if (rf.phase) bitsIx.push("phase " + rf.phase);
        if (rf.rel) bitsIx.push(String(rf.rel));
        if (rf.root != null && String(rf.root).trim() !== "") bitsIx.push("root " + rf.root);
        if (rf.chunks != null && rf.chunks !== "") bitsIx.push("chunks " + rf.chunks);
        if (rf.candidates_enqueued != null) bitsIx.push("candidates " + rf.candidates_enqueued);
        if (rf.candidates_discovered != null) bitsIx.push("discovered " + rf.candidates_discovered);
        if (slug.indexOf("indexer.run.done") === 0 && rf.mode) bitsIx.push("mode " + rf.mode);
        if (slug.indexOf("indexer.run.done") === 0 && rf.ingest_completed != null)
          bitsIx.push("ingested " + rf.ingest_completed);
        if (slug.indexOf("indexer.run.done") === 0 && rf.ingest_failed_dropped != null)
          bitsIx.push("failed " + rf.ingest_failed_dropped);
        if (rf.collection) bitsIx.push("collection " + rf.collection);
        if (rf.flavor_id) bitsIx.push("flavor " + rf.flavor_id);
        if (rf.ingest_project) bitsIx.push("project " + rf.ingest_project);
        if (rf.roots != null) bitsIx.push("roots " + rf.roots);
        if (rf.root_ids) bitsIx.push("root_ids " + rf.root_ids);
        if (rf.queue_depth != null) bitsIx.push("queue_depth " + rf.queue_depth);
        if (rf.worker != null) bitsIx.push("worker " + rf.worker);
        if (rf.attempt != null) bitsIx.push("attempt " + rf.attempt);
        if (rf.delay_ms != null) bitsIx.push("delay_ms " + rf.delay_ms);
        if (rf.err) bitsIx.push("err " + String(rf.err).replace(/\s+/g, " ").slice(0, 200));
        m = bitsIx.join(" · ");
      }
    }
    if (m.length > MAX_PRIMARY_MSG_CHARS) m = m.slice(0, MAX_PRIMARY_MSG_CHARS - 1) + "…";
    return m;
  }

  function levelBadgeClassForSummary(parsed) {
    var can = parsed.levelCanon;
    if (!can) return "log-line-sum__lvl--none";
    var safe = String(can).replace(/[^A-Z0-9_-]/gi, "");
    return safe ? "lvl-" + safe : "log-line-sum__lvl--none";
  }

  function logSummaryHtml(ev, badgeOpt, opts) {
    opts = opts || {};
    var parsed = ev.parsed;
    var dt = formatLogDateTimeLocal(ev.ts);
    var iso = toIsoDatetimeAttr(ev.ts);
    var lvlRaw = parsed.levelCanon || (parsed.levelLabel && parsed.levelLabel !== "—" ? parsed.levelLabel : "");
    var lvlStr = lvlRaw ? String(lvlRaw).trim() : "";
    var lvlClass = lvlStr ? levelBadgeClassForSummary(parsed) : "log-line-sum__lvl--none";
    var lvlHtml = lvlStr
      ? '<span class="log-line-sum__lvl ' + lvlClass + '">' + escapeHtml(lvlStr) + "</span>"
      : '<span class="log-line-sum__lvl log-line-sum__lvl--none">—</span>';
    var badgeHtml = "";
    var hideIndexerBadge =
      opts.suppressIndexerBadge && badgeOpt && (badgeOpt.key === "chimera-indexer" || badgeOpt.key === "indexer" || badgeOpt.lab === "indexer");
    var hideVectorstoreBadge =
      opts.suppressVectorstoreBadge &&
      badgeOpt &&
      (badgeOpt.key === "chimera-vectorstore" || badgeOpt.key === "vectorstore" || badgeOpt.lab === "vectorstore");
    var hideGatewayBadge =
      opts.suppressGatewayBadge &&
      badgeOpt &&
      (badgeOpt.key === "chimera-gateway" || badgeOpt.key === "gateway" || badgeOpt.lab === "gateway");
    if (badgeOpt && badgeOpt.lab && !hideIndexerBadge && !hideVectorstoreBadge && !hideGatewayBadge) {
      badgeHtml =
        '<span class="sum-svc-badge ' +
        badgeOpt.cls +
        '">' +
        escapeHtml(badgeOpt.lab) +
        "</span>";
    }
    var tierHtml = "";
    if (opts.convJoinTier && opts.convJoinTier !== "direct") {
      var tl = String(opts.convJoinTier);
      var safeTl = tl.replace(/[^a-z0-9_-]/gi, "");
      if (!safeTl) safeTl = "tier";
      var tierTitle = "Join tier";
      if (opts.vectorstoreSpanID) {
        tierTitle += " · span " + String(opts.vectorstoreSpanID);
      }
      if (opts.vectorstoreTurnIndex != null && opts.vectorstoreTurnIndex !== "") {
        tierTitle += " · turn " + String(opts.vectorstoreTurnIndex);
      }
      tierHtml =
        '<span class="sum-conv-tier sum-conv-tier--' +
        safeTl +
        '" title="' +
        escapeHtml(tierTitle) +
        '">' +
        escapeHtml(tl.replace(/_/g, " ")) +
        "</span>";
    }
    var msg = escapeHtml(primaryLogMessage(parsed, ev.text));
    return (
      '<div class="log-line-sum">' +
      '<span class="log-line-sum__meta">' +
      '<time class="log-line-sum__time"' +
      (iso ? ' datetime="' + escapeHtml(iso) + '"' : "") +
      ">" +
      escapeHtml(dt) +
      "</time>" +
      lvlHtml +
      badgeHtml +
      tierHtml +
      "</span>" +
      '<span class="log-line-sum__msg">' +
      msg +
      "</span></div>"
    );
  }

  function formatLogRelativeAgo(ms) {
    if (ms == null || !isFinite(Number(ms))) return "—";
    var now = Date.now();
    var sec = Math.round((now - Number(ms)) / 1000);
    if (sec < 0) return "in the future";
    if (sec < 10) return "just now";
    if (sec < 60) return "about " + sec + " seconds ago";
    if (sec < 3600) {
      var m = Math.floor(sec / 60);
      return m === 1 ? "about 1 minute ago" : "about " + m + " minutes ago";
    }
    if (sec < 86400) {
      var h = Math.floor(sec / 3600);
      return h === 1 ? "about 1 hour ago" : "about " + h + " hours ago";
    }
    var d = Math.floor(sec / 86400);
    return d === 1 ? "about 1 day ago" : "about " + d + " days ago";
  }


  function rebuildAllRows() {
    if (!tbody || !fltApp || !fltLevel) return;
    var rawTa = null;
    var rawWasAtBottom = false;
    if (viewMode === "raw_logs") {
      rawTa = document.getElementById("raw-logs-textarea");
      rawWasAtBottom = nearBottomTextarea(rawTa);
    }
    tbody.innerHTML = "";
    fltApp.innerHTML = '<option value="">All</option>';
    levelOptionSet = { "": true, "(none)": true, DEBUG: true, INFO: true, WARN: true, ERROR: true };
    for (var i = 0; i < entryCache.length; i++) {
      var ent = entryCache[i];
      var parsed = ent.parsed;
      ensureAppOption(parsed.app);
      if (parsed.levelCanon) ensureLevelOption(parsed.levelCanon);
      if (viewMode === "raw") {
        appendTableRow(parsed, false, ent.seq, ent.ts, ent.text);
      }
    }
    if (viewMode === "summarized") refreshSummarizedPanel();
    else if (viewMode === "raw_logs") rebuildRawLogsTextarea({ scrollBottom: rawWasAtBottom });
    else applyFilters();
    if (viewMode === "raw") {
      window.requestAnimationFrame(function () {
        window.scrollTo(0, document.documentElement.scrollHeight);
      });
    }
    if (viewMode === "raw_logs" && rawWasAtBottom) {
      window.requestAnimationFrame(function () {
        var ta = document.getElementById("raw-logs-textarea");
        if (ta) ta.scrollTop = ta.scrollHeight;
      });
    }
  }

  function appendTableRow(parsed, follow, seq, entryTs, rawText) {
    var tr = document.createElement("tr");
    tr.dataset.app = parsed.app;
    tr.dataset.level = parsed.levelCanon || "";
    if (seq !== undefined && seq !== null) tr.dataset.logSeq = String(seq);
    var lvlClass = "lvl-none";
    if (parsed.levelCanon) {
      var safe = String(parsed.levelCanon).replace(/[^A-Z0-9_-]/gi, "");
      if (safe) lvlClass = "lvl-" + safe;
    }
    tr.innerHTML =
      '<td class="col-app">' +
      escapeHtml(parsed.app) +
      "</td>" +
      '<td class="col-dt col-dt-utc">' +
      parsed.dtUtcHtml +
      "</td>" +
      '<td class="col-dt col-dt-local">' +
      parsed.dtLocalHtml +
      "</td>" +
      '<td class="col-lvl ' +
      lvlClass +
      '">' +
      escapeHtml(parsed.levelLabel) +
      "</td>" +
      '<td class="col-details">' +
      buildDetailsColumn(parsed, entryTs, rawText) +
      "</td>";
    if (globalThis.ChimeraSettings && globalThis.ChimeraSettings.Filters && globalThis.ChimeraSettings.Filters.matchesRow) {
      if (!globalThis.ChimeraSettings.Filters.matchesRow(filtersCtx, tr)) tr.style.display = "none";
    }
    tbody.appendChild(tr);
    while (tbody.children.length > CLIENT_CACHE_MAX) {
      var first = tbody.firstChild;
      var removedH = first.offsetHeight;
      tbody.removeChild(first);
      if (!follow && removedH) {
        window.scrollTo(0, Math.max(0, window.scrollY - removedH));
      }
    }
    if (follow) {
      window.scrollTo(0, document.documentElement.scrollHeight);
    }
  }

  function buildDetailsCell(extras) {
    if (globalThis.ChimeraUI && globalThis.ChimeraUI.KeyValueGrid) {
      return globalThis.ChimeraUI.KeyValueGrid(extras);
    }
    if (globalThis.ChimeraSettings && globalThis.ChimeraSettings.KeyValueGrid) {
      return globalThis.ChimeraSettings.KeyValueGrid(extras);
    }
    if (!extras.length) return '<span class="muted">—</span>';
    // Fallback keeps legacy behavior if component didn't load for some reason.
    var s =
      '<table class="props-table"><colgroup>' +
      '<col class="col-k" /><col class="col-v" /><col class="col-k" /><col class="col-v" />' +
      "</colgroup><tbody>";
    for (var i = 0; i < extras.length; i += 2) {
      s += "<tr>";
      s += '<td class="prop-name">' + escapeHtml(extras[i].k) + "</td>";
      if (i + 1 < extras.length) {
        s += '<td class="prop-val">' + escapeHtml(extras[i].v) + "</td>";
        s += '<td class="prop-name">' + escapeHtml(extras[i + 1].k) + "</td>";
        s += '<td class="prop-val">' + escapeHtml(extras[i + 1].v) + "</td>";
      } else {
        s += '<td class="prop-val prop-val-wide" colspan="3">' + escapeHtml(extras[i].v) + "</td>";
      }
      s += "</tr>";
    }
    s += "</tbody></table>";
    return s;
  }

  var appCtx = {
    contracts: C,
    tbody: tbody,
    statusEl: statusEl,
    entryCache: entryCache,
    getViewMode: function () { return viewMode; },
    setViewMode: function (next) { viewMode = next; },
    getFlat: getFlat,
    escapeHtml: escapeHtml,
    strHash: strHash,
    entryInstant: entryInstant,
    inferShape: inferShape,
    normalizeServiceBucketKey: normalizeServiceBucketKey,
    logSummaryHtml: logSummaryHtml,
    primaryLogMessage: primaryLogMessage,
    formatLogDateTimeLocal: formatLogDateTimeLocal,
    formatLogRelativeAgo: formatLogRelativeAgo,
    toIsoDatetimeAttr: toIsoDatetimeAttr,
    operatorFriendlyGatewayMsg: operatorFriendlyGatewayMsg,
    contextGrowthStripHtml: contextGrowthStripHtml,
    SHOW_CONV_EXPANDED_CONTEXT_STRIP: SHOW_CONV_EXPANDED_CONTEXT_STRIP,
    buildHeadlineHtml: buildHeadlineHtml,
    buildDetailsColumn: buildDetailsColumn,
    stickPx: stickPx,
    embedded: embedded,
    metricsCache: null,
    gatewayOverviewCache: null,
    adminStateCache: null,
    tokenListCache: [],
    tokenLabelByTenant: tokenLabelByTenant,
    chimeraBrokerProviderSnapshot: null,
    gatewayPanelShowProbes: gatewayPanelShowProbes,
    indexerWatchRootsStore: indexerWatchRootsStore,
    RECENT_CARD_STATUS_N: RECENT_CARD_STATUS_N,
    CONV_RECENT_N: CONV_RECENT_N,
    CHIMERA_BROKER_PROVIDER_STALE_MS: 90000,
    lastIndexerSummarizeByRun: null,
    lastIndexerSummarizePartitionRegistry: null,
    lastIndexerOperatorRoots: lastIndexerOperatorRoots,
    lastIndexerOperatorRootsJson: lastIndexerOperatorRootsJson,
    lastIndexerOperatorWorkspacesNested: lastIndexerOperatorWorkspacesNested,
    lastIndexerOperatorWorkspacesFingerprint: lastIndexerOperatorWorkspacesFingerprint,
    lastIndexerOperatorConfigPath: lastIndexerOperatorConfigPath,
    indexerOperatorConfigHydratedOnce: indexerOperatorConfigHydratedOnce,
    indexerOperatorConfigUnavailable: indexerOperatorConfigUnavailable,
    indexerServiceSummaryFetchInFlight: indexerServiceSummaryFetchInFlight,
    indexerServiceSummaryFetchWanted: indexerServiceSummaryFetchWanted,
    operatorWsFullLogCtx: operatorWsFullLogCtx,
    indexerRootScopeByRootId: indexerRootScopeByRootId,
    workspaceDrafts: workspaceDrafts,
    nextWorkspaceDraftId: nextWorkspaceDraftId,
    workspaceManagedEditId: workspaceManagedEditId,
    workspaceManagedStaging: workspaceManagedStaging,
    workspaceManagedFolderPickerOpen: workspaceManagedFolderPickerOpen,
    adminUserDrafts: adminUserDrafts,
    nextAdminUserDraftId: nextAdminUserDraftId,
    virtualModelDrafts: virtualModelDrafts,
    nextVirtualModelDraftId: nextVirtualModelDraftId,
    adminProviderKeyDraft: adminProviderKeyDraft,
    adminVisibleProviderIds: adminVisibleProviderIds,
    adminProviderCatalog: adminProviderCatalog,
    adminProviderCatalogReady: adminProviderCatalogReady,
    adminVisibleProviderIdsSeeded: adminVisibleProviderIdsSeeded,
    adminOllamaUrlDraft: adminOllamaUrlDraft,
    adminCreatedTokenByTenant: adminCreatedTokenByTenant,
    routingPolicyTouched: routingPolicyTouched,
    routingPolicyDraft: routingPolicyDraft,
    fallbackTouched: fallbackTouched,
    routerModelsTouched: routerModelsTouched,
    routerModelsDraft: routerModelsDraft,
    routerThresholdTouched: routerThresholdTouched,
    routerThresholdDraft: routerThresholdDraft,
    routerEnabledTouched: routerEnabledTouched,
    routerEnabledDraft: routerEnabledDraft,
    adminRoutingEditing: adminRoutingEditing,
    adminFallbackEditing: adminFallbackEditing,
    adminRouterEditing: adminRouterEditing,
    adminProviderModelsEditingId: adminProviderModelsEditingId,
    adminProviderModelsDraft: adminProviderModelsDraft,
    adminProviderModelsCache: adminProviderModelsCache,
    adminProviderModelsShowUnavailable: adminProviderModelsShowUnavailable,
    virtualModelDetails: {},
    virtualModelUi: {},
    storyRebuildTimer: null,
    sumEvlogUiDeferTimer: null,
    sumEvlogPointerSuppressedUntil: 0
  };

  if (globalThis.ChimeraSettings && globalThis.ChimeraSettings.Render && typeof globalThis.ChimeraSettings.Render.mountSumEvlog === "function") {
    globalThis.ChimeraSettings.Render.mountSumEvlog(appCtx);
  }
  if (globalThis.ChimeraSettings && globalThis.ChimeraSettings.App && typeof globalThis.ChimeraSettings.App.mountSummarizedFeed === "function") {
    globalThis.ChimeraSettings.App.mountSummarizedFeed(appCtx);
  }
  if (globalThis.ChimeraSettings && globalThis.ChimeraSettings.App && typeof globalThis.ChimeraSettings.App.mountWireHandlers === "function") {
    globalThis.ChimeraSettings.App.mountWireHandlers(appCtx);
  }

  var refreshSummarizedPanel = appCtx.refreshSummarizedPanel;
  var scheduleStoryRebuild = appCtx.scheduleStoryRebuild;
  var fetchTokenLabels = appCtx.fetchTokenLabels;

  var transportCtx = {
    /** When true, ingest into entryCache only; Raw Logs textarea is rebuilt once per batched chunk (initial load). */
    suppressRawLogsDom: false,
    /** Raw logs: coalesce DOM refresh to one rAF per frame (see streaming.js scheduleRawLogsDomFlush). */
    rawLogsRafPending: false,
    rawLogsFlushFollow: false,
    getViewMode: function () { return "summarized"; },
    setViewMode: function (_next) { viewMode = "summarized"; },
    getEmbedded: function () { return embedded; },
    getStarted: function () { return started; },
    setStarted: function (v) { started = !!v; },
    onViewModeChanged: function () { applyViewLayout(); },
    statusEl: statusEl,
    statusLine: statusLine,
    nearBottom: nearBottom,
    nearBottomTextarea: nearBottomTextarea,
    parseLogText: parseLogText,
    entryCache: entryCache,
    seenSeq: seenSeq,
    maxSeqRef: { value: maxSeq },
    minLoadedSeqRef: { value: minLoadedSeq },
    bufferMinSeqFromServerRef: { value: bufferMinSeqFromServer },
    olderFetchBusyRef: { value: olderFetchBusy },
    historyTailReadyRef: { value: false },
    CLIENT_CACHE_MAX: CLIENT_CACHE_MAX,
    INITIAL_TAIL_LIMIT: INITIAL_TAIL_LIMIT,
    BACKFILL_CHUNK: BACKFILL_CHUNK,
    RENDER_CHUNK: RENDER_CHUNK,
    scheduleStoryRebuild: function () { if (scheduleStoryRebuild) scheduleStoryRebuild(); },
    rebuildAllRows: rebuildAllRows,
    rebuildRawLogsTextarea: rebuildRawLogsTextarea,
    appendRawLineToTextarea: appendRawLineToTextarea,
    appendTableRow: function () { },
    applyFilters: function () { },
    ensureAppOption: function () { },
    ensureLevelOption: function () { },
    entryMatchesFilters: function () { return true; },
    fetchTokenLabels: fetchTokenLabels,
    startingRef: { value: false },
    esRef: { value: es },
    pollTimerRef: { value: pollTimer },
    markUiUnauthorized: function (msg) {
      if (appCtx.markUiUnauthorized) appCtx.markUiUnauthorized(msg);
    },
    getUiUnauthorized: function () {
      return !!appCtx.uiUnauthorized;
    },
    stopLogsTransport: null
  };

  appCtx.stopLogsTransport = function () {
    if (globalThis.ChimeraSettings && globalThis.ChimeraSettings.Transport && typeof globalThis.ChimeraSettings.Transport.stop === "function") {
      globalThis.ChimeraSettings.Transport.stop(transportCtx);
    }
  };
  transportCtx.stopLogsTransport = appCtx.stopLogsTransport;

  if (!appCtx.uiUnauthorized) fetchTokenLabels();
  applyViewLayout();
  if (globalThis.ChimeraSettings && globalThis.ChimeraSettings.Transport) {
    globalThis.ChimeraSettings.Transport.init(transportCtx);
  }
};
