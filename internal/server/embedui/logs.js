globalThis.ClaudiaLogs = globalThis.ClaudiaLogs || {};
globalThis.ClaudiaLogs.Main = function () {
  var params = new URLSearchParams(window.location.search);
  var embedded = params.get("embed") === "1" || window.self !== window.top;
  if (embedded) {
    document.documentElement.classList.add("logs-embedded");
    document.body.classList.add("logs-embedded");
  }
  var focusPrincipal = (params.get("principal") || "").trim();
  var focusConv = (params.get("conversation") || params.get("conv") || "").trim();
  var focusSeq = (params.get("seq") || "").trim();
  var tbody = document.getElementById("log-body");
  var statusEl = document.getElementById("status");
  var statusLine =
    globalThis.ClaudiaLogs && typeof globalThis.ClaudiaLogs.StatusLine === "function"
      ? globalThis.ClaudiaLogs.StatusLine(statusEl)
      : null;
  var fltApp = document.getElementById("flt-app");
  var fltLevel = document.getElementById("flt-level");
  var VIEW_LS = "claudia_logs_view_mode";
  var FLT_APP_LS = "claudia_logs_flt_app";
  var FLT_LEVEL_LS = "claudia_logs_flt_level";
  /**
   * Persist watch roots per summarized indexer card (partition bucket id) + index_run_id.
   * v1 used project+flavor only — multiple indexers sharing scope showed each other's roots.
   */
  var INDEXER_WATCH_ROOTS_LS = "claudia.indexer.watchRoots.v2";
  /** Gateway expanded panel: show 2xx HTTP rows for /health, /status, logs poll/stream, etc. */
  var GW_PROBES_LS = "claudia.logs.gateway.showProbes";
  var gatewayPanelShowProbes = false;
  try {
    gatewayPanelShowProbes = localStorage.getItem(GW_PROBES_LS) === "1";
  } catch (eGwLs) {}
  var CONV_RECENT_N = 5;
  /** Last N events used for summary-strip status pills ("error" vs active/complete). Matches Last-events preview depth. */
  var RECENT_CARD_STATUS_N = 3;
  var entryCache = [];
  /** Maps gateway tenant_id → token label from tokens.yaml (via GET /api/ui/tokens). */
  var tokenLabelByTenant = {};
  var storyRebuildTimer = null;
  /** Full summarized `innerHTML` refresh is deferred while evlog UI is in use (see `summarizedEvlogInteractionBlocksRebuild`). */
  var sumEvlogUiDeferTimer = null;
  /** After pointer interaction in a card event log, suppress full panel rebuild briefly so row select / click completes. */
  var sumEvlogPointerSuppressedUntil = 0;
  var maxSeq = 0;
  var stickPx = 160;
  var es = null;
  var pollTimer = null;
  /** SQLite gateway metrics snapshot for summarized “Gateway usage” card (/api/ui/metrics). */
  var metricsCache = null;
  var metricsPollTimer = null;
  var METRICS_POLL_MS = 12000;
  /** Live BiFrost provider snapshot (/api/ui/bifrost/providers) — drives the Provider health strip on the BiFrost card. */
  var bifrostProviderSnapshot = null;
  var bifrostProviderPollTimer = null;
  var BIFROST_PROVIDER_POLL_MS = 30000;
  /** Snapshots older than this are treated as stale and the strip falls back to log-derived state. */
  var BIFROST_PROVIDER_STALE_MS = 90000;
  /** Latest indexer partition map from the last summarized panel render (hydrate ↔ supervised YAML roots). */
  var lastIndexerSummarizeByRun = null;
  var lastIndexerSummarizePartitionRegistry = null;
  /** Flat roots from GET /api/ui/indexer/config — fills watched paths when indexer.run.start is outside the log buffer. */
  var lastIndexerOperatorRoots = [];
  var lastIndexerOperatorRootsJson = "";
  /** Nested workspaces from GET /api/ui/indexer/config (or POST save / derived from roots). Drives cards when logs have no bucket yet. */
  var lastIndexerOperatorWorkspacesNested = [];
  /** Populated during Workspaces card render: watched paths per synthetic `opws\x1e…` bucket id for full-log filtering. */
  var operatorWsFullLogCtx = {};
  /** From latest indexer.run.start root_scopes in buffer: root_id slug → { workspace_id, path, … }. */
  var indexerRootScopeByRootId = {};
  var indexerOperatorRootsRefreshQueued = false;
  /** Unsaved workspace rows created from Workspaces → Create (Phase 3). */
  var workspaceDrafts = [];
  var nextWorkspaceDraftId = 1;
  /** Phase 4: operator-managed workspace row in edit mode (numeric workspace id). */
  var workspaceManagedEditId = null;
  /** { wsNum: number, paths: { id: number|null, path: string }[] } — only valid while workspaceManagedEditId matches wsNum. */
  var workspaceManagedStaging = null;
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
    if (!flat) {
      if (source === "qdrant" || source === "bifrost" || source === "indexer") return "service." + source;
      return "generic";
    }
    var msg = String(flat.msg != null ? flat.msg : flat.message != null ? flat.message : "").toLowerCase();
    if (msg === "http response" || msg === "gateway.http.access" || (flat.method && flat.path != null && flat.statusCode !== undefined && flat.statusCode !== null))
      return "http.access";
    if (msg === "chat.request") return "chat.request";
    if (msg.indexOf("chat.bifrost") === 0 || msg.indexOf("upstream chat") >= 0) return "chat.bifrost";
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
    if (flat.service && String(flat.service) !== "gateway") return "service." + flat.service;
    if (source === "qdrant" || source === "bifrost" || source === "indexer") return "service." + source;
    return "generic";
  }

  function statusPillClass(code) {
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
      globalThis.ClaudiaLogs &&
      ClaudiaLogs.Derive &&
      typeof ClaudiaLogs.Derive.bifrostOperatorLine === "function"
    ) {
      var boHead = ClaudiaLogs.Derive.bifrostOperatorLine(flat);
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
    globalThis.ClaudiaLogs && globalThis.ClaudiaLogs.escapeHtml
      ? globalThis.ClaudiaLogs.escapeHtml
      : function (s) {
        if (s === null || s === undefined) return "";
        var d = document.createElement("div");
        d.textContent = String(s);
        return d.innerHTML;
      };

  // Expose selected helpers for parsing modules (Phase 4 extraction).
  globalThis.ClaudiaLogs = globalThis.ClaudiaLogs || {};
  globalThis.ClaudiaLogs.buildDateTimeCells = buildDateTimeCells;
  globalThis.ClaudiaLogs.inferShape = inferShape;

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
    globalThis.ClaudiaLogs && globalThis.ClaudiaLogs.parseLogText
      ? globalThis.ClaudiaLogs.parseLogText
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
    FLT_APP_LS: FLT_APP_LS,
    FLT_LEVEL_LS: FLT_LEVEL_LS,
    viewModeGetter: function () { return viewMode; },
    rebuildRawLogsTextarea: function (opts) { return rebuildRawLogsTextarea(opts); },
    nearBottomTextarea: nearBottomTextarea
  };

  function ensureAppOption(app) {
    if (globalThis.ClaudiaLogs && globalThis.ClaudiaLogs.Filters) {
      return globalThis.ClaudiaLogs.Filters.ensureAppOption(filtersCtx, app);
    }
  }
  function ensureLevelOption(lvl) {
    if (globalThis.ClaudiaLogs && globalThis.ClaudiaLogs.Filters) {
      return globalThis.ClaudiaLogs.Filters.ensureLevelOption(filtersCtx, lvl);
    }
  }
  function entryMatchesFilters(parsed) {
    if (globalThis.ClaudiaLogs && globalThis.ClaudiaLogs.Filters) {
      return globalThis.ClaudiaLogs.Filters.entryMatches(filtersCtx, parsed);
    }
    return true;
  }

  function rebuildRawLogsTextarea(opts) {
    if (globalThis.ClaudiaLogs && globalThis.ClaudiaLogs.RawLogs) {
      return globalThis.ClaudiaLogs.RawLogs.rebuild({ entryCache: entryCache, entryMatchesFilters: entryMatchesFilters }, opts);
    }
  }

  function appendRawLineToTextarea(ent, follow) {
    if (globalThis.ClaudiaLogs && globalThis.ClaudiaLogs.RawLogs) {
      return globalThis.ClaudiaLogs.RawLogs.appendRawLine({}, ent, follow);
    }
  }

  function copyRawLogsToClipboard() {
    if (globalThis.ClaudiaLogs && globalThis.ClaudiaLogs.RawLogs) {
      return globalThis.ClaudiaLogs.RawLogs.copyToClipboard({});
    }
  }

  function applyFilters() { }

  function applyViewLayout() {
    var psu = document.getElementById("panel-summarized");
    if (!psu) return;
    syncBifrostProviderPolling();
    try {
      document.body.classList.toggle("logs-summarized", true);
      document.body.classList.toggle("logs-raw", false);
      document.body.classList.toggle("logs-raw-logs", false);
    } catch (x) { }
    viewMode = "summarized";
    refreshSummarizedPanel();
    syncMetricsPolling();
  }

  function getFlat(parsed) {
    return parsed.rawFlat || {};
  }

  var strHash =
    globalThis.ClaudiaLogs && globalThis.ClaudiaLogs.strHash
      ? globalThis.ClaudiaLogs.strHash
      : function (s) {
        var h = 0;
        var t = String(s);
        for (var i = 0; i < t.length; i++) h = ((h << 5) - h) + t.charCodeAt(i) | 0;
        return "fc" + (h >>> 0).toString(16);
      };

  var entryInstant =
    globalThis.ClaudiaLogs && globalThis.ClaudiaLogs.entryInstant
      ? globalThis.ClaudiaLogs.entryInstant
      : function (entry) {
        if (!entry || entry.ts === null || entry.ts === undefined || entry.ts === "") return null;
        var d = entry.ts instanceof Date ? entry.ts : new Date(entry.ts);
        return isNaN(d.getTime()) ? null : d;
      };

  var humanDurationMs =
    globalThis.ClaudiaLogs && globalThis.ClaudiaLogs.humanDurationMs
      ? globalThis.ClaudiaLogs.humanDurationMs
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

  function serviceStripHtml(events) {
    var ragN = 0,
      qdrantEvt = 0,
      ingestN = 0;
    var ragMs = 0;
    var bifrostN = 0;
    if (
      globalThis.ClaudiaLogs &&
      ClaudiaLogs.Derive &&
      typeof ClaudiaLogs.Derive.conversationBifrostRelayCount === "function"
    ) {
      bifrostN = ClaudiaLogs.Derive.conversationBifrostRelayCount(events, function (p) { return getFlat(p); });
    }
    for (var i = 0; i < events.length; i++) {
      var sh = events[i].parsed.shape || "";
      var f = getFlat(events[i].parsed);
      if (sh === "rag" || (sh.indexOf("rag.") === 0 && sh !== "rag")) {
        ragN++;
        var lm = Number(f.latencyMs != null ? f.latencyMs : f.latency_ms != null ? f.latency_ms : f.elapsedMs);
        if (!isNaN(lm)) ragMs += lm;
      } else if (sh === "service.qdrant" || f.service === "qdrant") {
        qdrantEvt++;
      } else if (sh === "ingest") {
        ingestN++;
      }
    }
    var parts = [];
    if (ragN) parts.push("RAG · " + ragN + (ragMs ? " · ~" + Math.round(ragMs) + " ms" : ""));
    if (qdrantEvt) parts.push("Qdrant · " + qdrantEvt);
    if (ingestN) parts.push("ingest · " + ingestN);
    if (bifrostN) parts.push("BiFrost · " + bifrostN);
    if (!parts.length) return "";
    return (
      '<div class="service-chips">' +
      parts
        .map(function (p) {
          return '<span class="chip">' + escapeHtml(p) + "</span>";
        })
        .join("") +
      "</div>"
    );
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

  /** Short hint for gateway lifecycle errorType (conversation.errored). */
  function gatewayLifecycleErrorHint(errorType) {
    var e = String(errorType || "").trim().toLowerCase();
    if (e === "invalid_request") return "Check the request body (model, messages, parameters).";
    if (e === "invalid_api_key") return "Verify the API key or gateway credentials.";
    if (e === "gateway_provider_limits") return "Provider or gateway quota blocked this request.";
    if (e === "gateway_config") return "Gateway routing or configuration could not satisfy this request.";
    if (e === "gateway_upstream") return "The upstream LLM or network returned an error.";
    return "";
  }

  /** Compress raw RAG/embed errors for the summarized column (avoid huge JSON blobs). */
  function summarizeRagRetrieveErr(rawErr) {
    var er = String(rawErr || "").replace(/\s+/g, " ").trim();
    if (!er) return "";
    var low = er.toLowerCase();
    if (low.indexOf("context length") >= 0 || low.indexOf("exceeds the context") >= 0)
      return "Embedding input too long for the model context window.";
    var msgMatch = er.match(/"message"\s*:\s*"((?:[^"\\]|\\.)*)"/);
    if (msgMatch && msgMatch[1]) {
      var inner = msgMatch[1].replace(/\\"/g, '"').replace(/\\n/g, " ").trim();
      if (inner.length > 220) inner = inner.slice(0, 219) + "…";
      return inner;
    }
    er = er.replace(/^embed query:\s*embed:\s*/i, "").trim();
    var stMatch = er.match(/\bstatus\s+(\d{3})\b/i);
    if (stMatch) {
      var code = stMatch[1];
      var idx = er.toLowerCase().indexOf("status " + code);
      var tail = idx >= 0 ? er.slice(idx + ("status " + code).length).replace(/^:\s*/, "").trim() : "";
      if (tail.charAt(0) === "{") {
        var nested = summarizeRagRetrieveErr(tail);
        if (nested) return "Embedding API rejected the query (HTTP " + code + "): " + nested;
      }
      if (tail.length > 120) tail = tail.slice(0, 119) + "…";
      return tail ? "Embedding API HTTP " + code + ": " + tail : "Embedding API returned HTTP " + code + ".";
    }
    if (er.length > 140) er = er.slice(0, 139) + "…";
    return er;
  }

  /** Plain-language one-liners for gateway lifecycle / RAG slugs (summarized logs primary column). */
  function operatorFriendlyGatewayMsg(flat) {
    if (!flat || typeof flat !== "object") return "";
    var raw = flat.msg != null ? flat.msg : flat.message != null ? flat.message : "";
    var slug = String(raw).trim();
    if (!slug) return "";
    switch (slug) {
      case "conversation.merge.embed_failed":
        return "Embedding failed so this turn cannot be associated with any current conversation; storing as a new conversation";
      case "rag.query": {
        var rq =
          "Vector search for this request: querying the index for chunks relevant to the user message.";
        var collRaw = flat.collection != null ? String(flat.collection).trim() : "";
        if (collRaw) {
          var collLab =
            typeof ragCollectionLabelForUi === "function" ? ragCollectionLabelForUi(collRaw) : collRaw;
          rq += " Reading collection " + collLab + ".";
        }
        return rq;
      }
      case "conversation.received":
        return "Inbound chat message recorded for this conversation.";
      case "chat.request":
        return "Chat completion request accepted and prepared for upstream routing.";
      case "conversation.rag.span": {
        var spanBase =
          "RAG retrieval span recorded for this conversation turn (links retrieval to the chat scope).";
        var collSpan = flat.collection != null ? String(flat.collection).trim() : "";
        if (collSpan && typeof ragCollectionLabelForUi === "function") {
          var spanLab = ragCollectionLabelForUi(collSpan);
          if (spanLab) spanBase += " Reading collection " + spanLab + ".";
        }
        return spanBase;
      }
      case "upstream.models.ok": {
        var n = flat.count != null ? Number(flat.count) : NaN;
        var base = "Upstream model catalog responded successfully.";
        if (!isNaN(n) && n >= 0) return base + " Models listed: " + Math.round(n) + ".";
        return base;
      }
      case "conversation.request.witness":
        return (
          "Request witness logged: structured snapshot of the chat payload after normalization " +
          "(message counts, roles, prompt size, tools) for correlation and auditing."
        );
      case "rag.retrieve.error": {
        var baseErr = "RAG retrieval failed; continuing without injected chunks.";
        var rawEr = flat.err != null ? String(flat.err) : "";
        var sum = summarizeRagRetrieveErr(rawEr);
        return sum ? baseErr + " Cause: " + sum : baseErr;
      }
      case "conversation.errored": {
        var baseConvErr =
          "This conversation turn ended with an error (no successful completion delivered).";
        var scErr = flat.statusCode != null ? Number(flat.statusCode) : NaN;
        var etErr = flat.errorType != null ? String(flat.errorType).trim() : "";
        var bitsErr = [];
        if (!isNaN(scErr)) bitsErr.push("HTTP " + Math.round(scErr));
        var hintErr = gatewayLifecycleErrorHint(etErr);
        if (hintErr) bitsErr.push(hintErr);
        return bitsErr.length ? baseConvErr + " · " + bitsErr.join(" · ") : baseConvErr;
      }
      case "conversation.delivered": {
        var baseD = "Completion delivered to the client (this turn finished successfully).";
        var sc = flat.statusCode != null ? Number(flat.statusCode) : NaN;
        var ms = flat.total_ms != null ? Number(flat.total_ms) : flat.totalMs != null ? Number(flat.totalMs) : NaN;
        var bitsD = [];
        if (!isNaN(sc)) bitsD.push("HTTP " + Math.round(sc));
        if (!isNaN(ms) && ms >= 0) bitsD.push(Math.round(ms) + " ms");
        return bitsD.length ? baseD + " · " + bitsD.join(" · ") : baseD;
      }
      case "conversation.routing.resolved":
      case "conversation.routing.resolve": {
        var partsR = ["Routing resolved: upstream model chosen for this completion."];
        var modR = flat.upstreamModel != null ? String(flat.upstreamModel).trim() : "";
        if (modR) partsR.push("Model " + modR);
        var att = flat.attempt != null ? Number(flat.attempt) : NaN;
        var chain = flat.chainLen != null ? Number(flat.chainLen) : NaN;
        if (!isNaN(att) && !isNaN(chain) && chain > 1)
          partsR.push("attempt " + Math.round(att) + "/" + Math.round(chain));
        return partsR.join(" · ");
      }
      case "conversation.upstream.started": {
        var baseUp = "Upstream provider request started (POST to chat/completions).";
        var modUp = flat.upstreamModel != null ? String(flat.upstreamModel).trim() : "";
        return modUp ? baseUp + " Model: " + modUp + "." : baseUp;
      }
      case "ingest.complete": {
        var bitsIc = ["Ingest finished — document indexed."];
        var ch = flat.chunks != null ? Number(flat.chunks) : NaN;
        if (!isNaN(ch) && ch >= 0) bitsIc.push(Math.round(ch) + " chunk" + (ch === 1 ? "" : "s"));
        var srcIc = flat.source != null ? String(flat.source).trim() : "";
        if (srcIc) bitsIc.push("source: " + (srcIc.length > 80 ? srcIc.slice(0, 79) + "…" : srcIc));
        var tenIc = flat.tenant != null ? String(flat.tenant).trim() : "";
        if (tenIc) bitsIc.push("tenant " + tenIc);
        return bitsIc.join(" · ");
      }
      case "gateway.auth.reloaded": {
        var baseAuth = "Client credentials reloaded from disk.";
        var nAuth = flat.count != null ? Number(flat.count) : NaN;
        if (!isNaN(nAuth) && nAuth >= 0) return baseAuth + " Active keys: " + Math.round(nAuth) + ".";
        return baseAuth;
      }
      case "gateway.health.upstream": {
        var okH = flat.ok === true || flat.ok === "true" || flat.ok === 1;
        var baseH = okH ? "Upstream health OK" : "Upstream health failed";
        var bitsH = [];
        var stH = flat.status != null ? Number(flat.status) : NaN;
        if (!isNaN(stH)) bitsH.push("probe HTTP " + Math.round(stH));
        var detH = flat.detail != null ? String(flat.detail).replace(/\s+/g, " ").trim() : "";
        if (detH.length > 100) detH = detH.slice(0, 99) + "…";
        if (!okH && detH) bitsH.push(detH);
        var tgtH = flat.target != null ? String(flat.target).trim() : "";
        if (tgtH) {
          var hostH = "";
          try {
            hostH = new URL(tgtH).host || "";
          } catch (eH) {
            hostH = "";
          }
          if (!hostH && tgtH) hostH = tgtH.length > 72 ? tgtH.slice(0, 71) + "…" : tgtH;
          if (hostH) bitsH.push(hostH);
        }
        return bitsH.length ? baseH + " · " + bitsH.join(" · ") : baseH;
      }
      case "gateway.startup.listening": {
        var bitsL = ["Gateway listening for HTTP requests."];
        var addrL = flat.addr != null ? String(flat.addr).trim() : "";
        if (addrL) bitsL.push("bind " + addrL);
        var upL = flat.upstream != null ? String(flat.upstream).trim() : "";
        if (upL) {
          var upShort = upL;
          try {
            upShort = new URL(upL).host || upL;
          } catch (eL) {
            upShort = upL;
          }
          if (upShort.length > 56) upShort = upShort.slice(0, 55) + "…";
          bitsL.push("upstream " + upShort);
        }
        return bitsL.join(" · ");
      }
      case "gateway.supervisor.indexer.starting": {
        var bitsIxS = ["Supervised indexer process starting."];
        if (flat.bin != null && String(flat.bin).trim() !== "") {
          var bn = String(flat.bin).replace(/\\/g, "/");
          var leaf = bn.split("/").pop();
          bitsIxS.push(leaf || bn);
        }
        var cfgIxS = flat.config != null ? String(flat.config).trim() : "";
        if (cfgIxS) bitsIxS.push("config " + (cfgIxS.length > 48 ? cfgIxS.slice(0, 47) + "…" : cfgIxS));
        return bitsIxS.join(" · ");
      }
      case "gateway.supervisor.bifrost.ready": {
        var baseBr = "BiFrost passed health check — ready.";
        var urlBr = flat.url != null ? String(flat.url).trim() : "";
        if (!urlBr) return baseBr;
        try {
          var uBr = new URL(urlBr);
          var tailBr = (uBr.host + (uBr.pathname === "/" ? "" : uBr.pathname)).slice(0, 96);
          return baseBr + " · " + tailBr;
        } catch (eBr) {
          return baseBr + " · " + (urlBr.length > 96 ? urlBr.slice(0, 95) + "…" : urlBr);
        }
      }
      case "gateway.supervisor.bifrost.starting": {
        var bitsBs = ["BiFrost subprocess starting."];
        if (flat.bin != null && String(flat.bin).trim() !== "") {
          var bbs = String(flat.bin).replace(/\\/g, "/").split("/").pop();
          if (bbs) bitsBs.push(bbs);
        }
        var appD = flat.app_dir != null ? String(flat.app_dir).trim() : flat.dir != null ? String(flat.dir).trim() : "";
        if (appD) bitsBs.push("data " + (appD.length > 40 ? appD.slice(0, 39) + "…" : appD));
        if (flat.host != null && String(flat.host).trim() !== "") bitsBs.push("host " + String(flat.host).trim());
        if (flat.port != null && String(flat.port).trim() !== "") bitsBs.push("port " + String(flat.port).trim());
        return bitsBs.join(" · ");
      }
      case "gateway.supervisor.qdrant.ready": {
        var baseQr = "Qdrant passed health check — ready.";
        var urlQr = flat.url != null ? String(flat.url).trim() : "";
        if (!urlQr) return baseQr;
        try {
          var uQr = new URL(urlQr);
          var tailQr = (uQr.host + (uQr.pathname === "/" ? "" : uQr.pathname)).slice(0, 96);
          return baseQr + " · " + tailQr;
        } catch (eQr) {
          return baseQr + " · " + (urlQr.length > 96 ? urlQr.slice(0, 95) + "…" : urlQr);
        }
      }
      case "gateway.supervisor.qdrant.starting": {
        var bitsQs = ["Qdrant subprocess starting."];
        if (flat.bin != null && String(flat.bin).trim() !== "") {
          var bqs = String(flat.bin).replace(/\\/g, "/").split("/").pop();
          if (bqs) bitsQs.push(bqs);
        }
        var stor = flat.storage != null ? String(flat.storage).trim() : "";
        if (stor) bitsQs.push("storage " + (stor.length > 40 ? stor.slice(0, 39) + "…" : stor));
        if (flat.http_port != null) bitsQs.push("http " + String(flat.http_port));
        if (flat.grpc_port != null) bitsQs.push("grpc " + String(flat.grpc_port));
        if (flat.host != null && String(flat.host).trim() !== "") bitsQs.push("host " + String(flat.host).trim());
        return bitsQs.join(" · ");
      }
      case "gateway.startup.seed":
        return "Gateway startup seed (early init before full serve).";
      case "gateway.startup.disk_log": {
        var phaseD = flat.phase != null ? String(flat.phase).trim() : "";
        var pathD = flat.path != null ? String(flat.path).trim() : "";
        var dirD = flat.dir != null ? String(flat.dir).trim() : "";
        var errD = flat.err != null ? String(flat.err).replace(/\s+/g, " ").trim() : "";
        if (errD.length > 120) errD = errD.slice(0, 119) + "…";
        if (phaseD === "mkdir" || phaseD === "open") {
          var locD = pathD || dirD || "";
          if (locD.length > 72) locD = locD.slice(0, 71) + "…";
          return (
            "Disk log setup failed (" +
            phaseD +
            ")" +
            (locD ? " · " + locD : "") +
            (errD ? " · " + errD : "")
          );
        }
        if (pathD) return "Disk logging enabled · " + (pathD.length > 100 ? pathD.slice(0, 99) + "…" : pathD);
        return "Disk logging enabled.";
      }
      case "gateway.startup.config_resolved": {
        var bitsCfg = ["Gateway configuration paths resolved."];
        var fpCfg = flat.filePath != null ? String(flat.filePath).trim() : "";
        if (fpCfg) bitsCfg.push("gateway " + (fpCfg.length > 56 ? fpCfg.slice(0, 55) + "…" : fpCfg));
        var akCfg =
          flat.api_keys_path != null
            ? String(flat.api_keys_path).trim()
            : flat.tokens_path != null
              ? String(flat.tokens_path).trim()
              : "";
        if (akCfg) bitsCfg.push("keys " + (akCfg.length > 48 ? akCfg.slice(0, 47) + "…" : akCfg));
        var rpCfg = flat.routingPolicyPath != null ? String(flat.routingPolicyPath).trim() : "";
        if (rpCfg) bitsCfg.push("routing " + (rpCfg.length > 48 ? rpCfg.slice(0, 47) + "…" : rpCfg));
        return bitsCfg.join(" · ");
      }
      default:
        return "";
    }
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
    var gatewayOp = operatorFriendlyGatewayMsg(rf);
    if (gatewayOp) {
      return gatewayOp.length > MAX_PRIMARY_MSG_CHARS ? gatewayOp.slice(0, MAX_PRIMARY_MSG_CHARS - 1) + "…" : gatewayOp;
    }
    if (
      globalThis.ClaudiaLogs &&
      ClaudiaLogs.Derive &&
      typeof ClaudiaLogs.Derive.bifrostOperatorLine === "function"
    ) {
      var bifrostLine = ClaudiaLogs.Derive.bifrostOperatorLine(rf, forEventLog ? { forEventLog: true } : undefined);
      if (bifrostLine && String(bifrostLine).trim() !== "") {
        var bl = String(bifrostLine).trim();
        return bl.length > MAX_PRIMARY_MSG_CHARS ? bl.slice(0, MAX_PRIMARY_MSG_CHARS - 1) + "…" : bl;
      }
    }
    if (sh === "chat.bifrost" || (rf.upstreamModel && (rf.statusCode != null || rf.status != null))) {
      var scB = rf.statusCode != null ? rf.statusCode : rf.status;
      var parts = [];
      if (!forEventLog && scB !== undefined && scB !== null && scB !== "") parts.push(String(scB));
      if (rf.upstreamModel) parts.push(String(rf.upstreamModel));
      var pathHint = rf.path || "";
      if (!pathHint && rf.target) {
        try {
          pathHint = new URL(String(rf.target)).pathname || "";
        } catch (e) {
          pathHint = "";
        }
      }
      if (pathHint) parts.push(String(pathHint));
      var baseMsg = rf.msg != null ? String(rf.msg) : rf.message != null ? String(rf.message) : "";
      if (!baseMsg && rawText) baseMsg = String(rawText).trim();
      var lineB = parts.length ? parts.join(" · ") : baseMsg || "—";
      return lineB.length > MAX_PRIMARY_MSG_CHARS ? lineB.slice(0, MAX_PRIMARY_MSG_CHARS - 1) + "…" : lineB;
    }
    if (sh === "chat.routing" || (rf.attempt != null && rf.chainLen != null && rf.upstreamModel)) {
      var bitsR = [];
      if (rf.msg != null || rf.message != null) bitsR.push(String(rf.msg != null ? rf.msg : rf.message));
      if (rf.upstreamModel) bitsR.push("model " + rf.upstreamModel);
      if (rf.attempt != null) bitsR.push("attempt " + rf.attempt);
      if (!forEventLog && rf.statusCode != null) bitsR.push("HTTP " + rf.statusCode);
      var lineR = bitsR.filter(Boolean).join(" · ") || "routing";
      return lineR.length > MAX_PRIMARY_MSG_CHARS ? lineR.slice(0, MAX_PRIMARY_MSG_CHARS - 1) + "…" : lineR;
    }
    if (
      String(rf.service || "").toLowerCase() === "qdrant" &&
      rf.msg != null &&
      String(rf.msg).indexOf("qdrant.") === 0 &&
      globalThis.ClaudiaLogs &&
      ClaudiaLogs.Derive &&
      typeof ClaudiaLogs.Derive.qdrantOperatorLine === "function"
    ) {
      var qOpLine = ClaudiaLogs.Derive.qdrantOperatorLine(
        rf,
        qdrantCollectionScopeLabelForLogs,
        forEventLog ? { forEventLog: true } : undefined
      );
      if (qOpLine && String(qOpLine).trim() !== "") {
        var mqQ = String(qOpLine).trim();
        return mqQ.length > MAX_PRIMARY_MSG_CHARS ? mqQ.slice(0, MAX_PRIMARY_MSG_CHARS - 1) + "…" : mqQ;
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
        globalThis.ClaudiaLogs &&
          ClaudiaLogs.Derive &&
          typeof ClaudiaLogs.Derive.indexerProseSummary === "function"
          ? ClaudiaLogs.Derive.indexerProseSummary(rf)
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
    var hideIxBadge = opts.suppressIndexerBadge && badgeOpt && badgeOpt.lab === "indexer";
    var hideQBadge = opts.suppressQdrantBadge && badgeOpt && badgeOpt.lab === "qdrant";
    var hideGwBadge = opts.suppressGatewayBadge && badgeOpt && badgeOpt.lab === "gateway";
    if (badgeOpt && badgeOpt.lab && !hideIxBadge && !hideQBadge && !hideGwBadge) {
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
      if (opts.qdrantSpanID) {
        tierTitle += " · span " + String(opts.qdrantSpanID);
      }
      if (opts.qdrantTurnIndex != null && opts.qdrantTurnIndex !== "") {
        tierTitle += " · turn " + String(opts.qdrantTurnIndex);
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

  function sumEvlogHttpStatusNumber(v) {
    if (v == null || v === "") return null;
    var n = Number(v);
    if (isNaN(n) || n < 100 || n > 599) return null;
    return n;
  }

  /**
   * HTTP status for summarized full-event-log row styling and Status column.
   * Includes gateway http.access plus bifrost/qdrant REST rows that carry http_status.
   */
  function sumEvlogHttpCode(parsed, flatOpt) {
    if (!parsed) return null;
    var flat = flatOpt != null ? flatOpt : getFlat(parsed);
    if (parsed.shape === "http.access" && flat) {
      var sc0 = flat.statusCode;
      if (sc0 != null && sc0 !== "") {
        var n0 = Number(sc0);
        if (!isNaN(n0)) return n0;
      }
      return null;
    }
    if (!flat) return null;
    var msgRaw = flat.msg != null ? flat.msg : flat.message != null ? flat.message : "";
    var msgL = String(msgRaw).toLowerCase();
    if (
      msgL === "bifrost.http.access" ||
      msgL === "bifrost.rate_limit" ||
      msgL === "qdrant.http.collection_meta" ||
      msgL === "qdrant.http.points_upsert_ok" ||
      msgL === "qdrant.http.points_upsert_rejected" ||
      msgL === "qdrant.http.points_delete" ||
      msgL === "qdrant.http.vector_search"
    ) {
      var hs = sumEvlogHttpStatusNumber(flat.http_status != null ? flat.http_status : flat.httpStatus);
      if (hs != null) return hs;
    }
    if (
      msgL === "chat.bifrost.response" ||
      msgL === "upstream chat response" ||
      msgL === "chat.routing.fallback" ||
      msgL === "chat.routing.resolved" ||
      msgL === "virtual model routing resolved"
    ) {
      var c = sumEvlogHttpStatusNumber(
        flat.statusCode != null ? flat.statusCode : flat.status_code != null ? flat.status_code : flat.status
      );
      if (c != null) return c;
    }
    return null;
  }

  /** Stable row id for selection across client filters: prefer log line seq, else row index + ts under card scope. */
  function sumEvlogStableRowId(cardScope, entLike, rowIndex) {
    var scope = String(cardScope || "s");
    if (entLike.seq != null && entLike.seq !== "") return scope + ":n:" + String(entLike.seq);
    return scope + ":i:" + String(rowIndex) + ":t:" + String(entLike.ts != null ? entLike.ts : "");
  }

  function sumEvlogLevelKey(levelStr) {
    var s = levelStr == null ? "" : String(levelStr).trim();
    return s === "" ? "_NONE" : s.toUpperCase();
  }

  function sumEvlogIsWarnish(levelCanon, http) {
    var lk = sumEvlogLevelKey(levelCanon);
    if (lk === "WARN") return true;
    if (http === 429) return true;
    return false;
  }

  function sumEvlogIsFailish(levelCanon, http) {
    var lk = sumEvlogLevelKey(levelCanon);
    if (lk === "ERROR") return true;
    if (http == null) return false;
    if (http >= 200 && http <= 299) return false;
    return true;
  }

  function sumEvlogCountWarnFailFromEntries(entries) {
    var warn = 0;
    var fail = 0;
    for (var i = 0; i < entries.length; i++) {
      var p = entries[i].parsed;
      var http = sumEvlogHttpCode(p, getFlat(p));
      var lk = p.levelCanon || (p.levelLabel && p.levelLabel !== "—" ? p.levelLabel : "");
      if (sumEvlogIsWarnish(lk, http)) warn++;
      if (sumEvlogIsFailish(lk, http)) fail++;
    }
    return { warn: warn, fail: fail };
  }

  function sumEvlogStatusInnerHtml(parsed) {
    var flat = getFlat(parsed);
    var http = sumEvlogHttpCode(parsed, flat);
    var lk = sumEvlogLevelKey(
      parsed.levelCanon || (parsed.levelLabel && parsed.levelLabel !== "—" ? parsed.levelLabel : "")
    );
    var parts = [];
    if (lk === "TRACE") {
      parts.push('<span class="sum-evlog-status__pill sum-evlog-status__lvl--TRACE">TRACE</span>');
    } else if (lk === "WARN") {
      parts.push('<span class="sum-evlog-status__pill sum-evlog-status__lvl--WARN">WARN</span>');
    } else if (lk === "ERROR") {
      parts.push('<span class="sum-evlog-status__pill sum-evlog-status__lvl--ERROR">ERROR</span>');
    }
    if (http != null) {
      if (http === 304) {
        parts.push('<span class="chip">' + escapeHtml(String(http)) + "</span>");
      } else {
        var pcl = statusPillClass(http);
        parts.push('<span class="' + pcl + '">' + escapeHtml(String(http)) + "</span>");
      }
    }
    if (!parts.length) {
      return '<span class="sum-evlog-status__empty" aria-hidden="true"></span>';
    }
    return parts.join("");
  }

  function sumEvlogMsgCellInnerHtml(ev, badgeOpt, opts) {
    opts = opts || {};
    var parsed = ev.parsed;
    var badgeHtml = "";
    var hideIxBadge = opts.suppressIndexerBadge && badgeOpt && badgeOpt.lab === "indexer";
    var hideQBadge = opts.suppressQdrantBadge && badgeOpt && badgeOpt.lab === "qdrant";
    var hideGwBadge = opts.suppressGatewayBadge && badgeOpt && badgeOpt.lab === "gateway";
    if (badgeOpt && badgeOpt.lab && !hideIxBadge && !hideQBadge && !hideGwBadge) {
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
      if (opts.qdrantSpanID) {
        tierTitle += " · span " + String(opts.qdrantSpanID);
      }
      if (opts.qdrantTurnIndex != null && opts.qdrantTurnIndex !== "") {
        tierTitle += " · turn " + String(opts.qdrantTurnIndex);
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
    var msg = escapeHtml(primaryLogMessage(parsed, ev.text, { forEventLog: true }));
    return badgeHtml + tierHtml + msg;
  }

  function sumEvlogRowTrHtml(entLike, cardScope, rowIndex, badgeOpt, summaryOpts) {
    summaryOpts = summaryOpts || {};
    var parsed = entLike.parsed;
    var flat = getFlat(parsed);
    var http = sumEvlogHttpCode(parsed, flat);
    var lvlRaw = parsed.levelCanon || (parsed.levelLabel && parsed.levelLabel !== "—" ? parsed.levelLabel : "");
    var lvlStr = lvlRaw ? String(lvlRaw).trim() : "";
    var lvlAttr = escapeHtml(lvlStr.toUpperCase());
    var httpAttr = http == null ? "" : ' data-evlog-http="' + escapeHtml(String(http)) + '"';
    var rowId = escapeHtml(sumEvlogStableRowId(cardScope, entLike, rowIndex));
    var iso = toIsoDatetimeAttr(entLike.ts);
    var dt = formatLogDateTimeLocal(entLike.ts);
    var rel = formatLogRelativeAgo(entLike.ts);
    var msgInner = sumEvlogMsgCellInnerHtml(entLike, badgeOpt, summaryOpts);
    var statusInner = sumEvlogStatusInnerHtml(parsed);
    return (
      '<tr class="sum-evlog__row" data-evlog-id="' +
      rowId +
      '" data-evlog-level="' +
      lvlAttr +
      '"' +
      httpAttr +
      ">" +
      '<td class="sum-evlog__cell--time"><time' +
      (iso ? ' datetime="' + escapeHtml(iso) + '"' : "") +
      ' title="' +
      escapeHtml(rel) +
      '">' +
      escapeHtml(dt) +
      "</time></td>" +
      '<td class="sum-evlog__cell--msg">' +
      msgInner +
      "</td>" +
      '<td class="sum-evlog__cell--status"><div class="sum-evlog-status">' +
      statusInner +
      "</div></td></tr>"
    );
  }

  function sumEvlogToolbarStaticHtml() {
    return (
      '<div class="sum-evlog__toolbar">' +
      '<input class="sum-evlog__search" type="search" placeholder="Search…" aria-label="Search log entries" autocomplete="off" />' +
      '<label class="sum-evlog__lvl-label" style="margin-left:auto">' +
      '<span class="sum-evlog__level-filters-label" style="margin-right:0.35rem">Status</span>' +
      '<select class="sum-evlog__filter-select" data-evlog-filter-status aria-label="Filter by severity">' +
      '<option value="all">All</option>' +
      '<option value="warnings">⚠ Warnings</option>' +
      '<option value="errors">✖ Errors</option>' +
      "</select></label>" +
      '<button type="button" class="sum-evlog__copy-btn" title="Copy as TSV — selected rows, or all visible if none selected" aria-label="Copy as TSV: selected rows, or all visible if none selected">' +
      '<svg class="sum-evlog__copy-icon" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><rect width="14" height="14" x="8" y="8" rx="2" ry="2"/><path d="M4 16c-1.1 0-2-.9-2-2V4c0-1.1.9-2 2-2h10c1.1 0 2 .9 2 2"/></svg>' +
      '<span class="sr-only"></span></button></div>'
    );
  }

  function sumEvlogPanelHtml(o) {
    o = o || {};
    var scrollTbodyId = o.scrollTbodyId || "sum-evlog-tb";
    var warnN = o.warnN != null ? o.warnN : 0;
    var failN = o.failN != null ? o.failN : 0;
    var tbodyInner = o.tbodyInnerHtml || "";
    var title = o.title != null ? o.title : "Full event log";
    var titleRightHtml = o.titleRightHtml || "";
    var titleBlock = titleRightHtml
      ? '<div class="sum-conv-full-log-head sum-evlog__title-row">' +
          '<div class="sum-section-label">' + escapeHtml(title) + "</div>" +
          '<div class="sum-conv-services-after-log-hdr">' + titleRightHtml + "</div>" +
        "</div>"
      : '<div class="sum-section-label">' + escapeHtml(title) + "</div>";
    return (
      '<div class="sum-evlog sum-evlog--in-card" data-sum-evlog-root>' +
      titleBlock +
      sumEvlogToolbarStaticHtml() +
      '<div class="sum-metrics-table-wrap sum-evlog__table-scroll">' +
      '<table class="sum-metrics-table sum-evlog__table">' +
      '<colgroup><col class="sum-evlog__col-time" /><col class="sum-evlog__col-msg" /><col class="sum-evlog__col-status" /></colgroup>' +
      '<thead><tr><th class="sum-evlog__cell--time" scope="col">Time</th><th scope="col">Message</th><th class="sum-evlog__th-status" scope="col">' +
      '<div class="sum-evlog__th-status-head" role="group" aria-label="Status: warning and error counts in this slice">' +
      '<span class="sum-evlog__th-status-label">Status</span>' +
      '<span class="sum-evlog-status__pill sum-evlog-status__lvl--WARN sum-evlog-metric-num" data-sum-evlog-metric-warn title="Lines with WARN or HTTP 429 in this card">' +
      escapeHtml(String(warnN)) +
      "</span>" +
      '<span class="sum-evlog-status__pill sum-evlog-status__lvl--WARN sum-evlog__metric-icon" aria-hidden="true">⚠</span>' +
      '<span class="sum-evlog-status__pill sum-evlog-status__lvl--ERROR sum-evlog-metric-num" data-sum-evlog-metric-fail title="Lines with ERROR or HTTP non-2xx in this card">' +
      escapeHtml(String(failN)) +
      "</span>" +
      '<span class="sum-evlog-status__pill sum-evlog-status__lvl--ERROR sum-evlog__metric-icon" aria-hidden="true">✖</span>' +
      "</div></th></tr></thead>" +
      '<tbody id="' +
      escapeHtml(scrollTbodyId) +
      '" data-sum-evlog-tbody>' +
      tbodyInner +
      "</tbody></table></div>" +
      '<div class="sum-evlog__footer-row">' +
      '<div class="sum-evlog__footer-left">' +
      '<p class="sum-evlog__footer" data-sum-evlog-oldest></p></div>' +
      '<p class="sum-evlog__toast sum-gallery-evlog__toast-align" data-sum-evlog-toast></p></div>' +
      "</div>"
    );
  }

  function sumEvlogBuildTbodyFromConvEvents(evs, turnGroups, cardScope) {
    var parts = [];
    var rowIdx = 0;
    function pushEvent(evT) {
      var evLine = {
        parsed: evT.parsed,
        text: evT.text != null && evT.text !== undefined ? evT.text : "",
        ts: evT.ts,
        source: evT.source,
        seq: evT.seq
      };
      var bd = inferServiceBadge(evLine);
      parts.push(
        sumEvlogRowTrHtml(evLine, cardScope, rowIdx, bd, {
          convJoinTier: evT.convJoinTier,
          qdrantSpanID: evT.qdrantSpanID,
          qdrantTurnIndex: evT.qdrantTurnIndex
        })
      );
      rowIdx++;
    }
    if (turnGroups && turnGroups.length > 1) {
      for (var tgi = 0; tgi < turnGroups.length; tgi++) {
        var tg = turnGroups[tgi];
        parts.push(
          '<tr class="sum-evlog__section"><td colspan="3" class="sum-evlog__section-cell">' +
          escapeHtml(tg.label) +
          "</td></tr>"
        );
        for (var ti2 = tg.events.length - 1; ti2 >= 0; ti2--) {
          pushEvent(tg.events[ti2]);
        }
      }
    } else {
      for (var u = evs.length - 1; u >= 0; u--) {
        pushEvent(evs[u]);
      }
    }
    return parts.join("");
  }

  function sumEvlogBuildTbodyFromServiceEntries(name, arr, opts) {
    opts = opts || {};
    var cardScope = opts.cardScope || strHash("svc:" + name);
    var parts = [];
    var rowIdx = 0;
    for (var u = arr.length - 1; u >= 0; u--) {
      var ent2 = arr[u];
      if (
        opts.filterGatewayProbe &&
        !gatewayPanelShowProbes &&
        globalThis.ClaudiaLogs &&
        ClaudiaLogs.Derive &&
        typeof ClaudiaLogs.Derive.gatewayPanelHideRow === "function" &&
        ClaudiaLogs.Derive.gatewayPanelHideRow(ent2, function (p) {
          return getFlat(p);
        })
      ) {
        continue;
      }
      var ev2 = {
        parsed: ent2.parsed,
        text: ent2.text,
        ts: ent2.ts,
        source: ent2.source,
        seq: ent2.seq
      };
      var summaryOpts =
        name === "indexer"
          ? { suppressIndexerBadge: true }
          : name === "qdrant"
            ? { suppressQdrantBadge: true }
            : name === "gateway"
              ? { suppressGatewayBadge: true }
              : {};
      var bd2 = opts.indexerRunLine ? badgeForIndexerRunLine(ent2) : badgeForServicePanel(name, ev2);
      parts.push(sumEvlogRowTrHtml(ev2, cardScope, rowIdx, bd2, summaryOpts));
      rowIdx++;
    }
    return parts.join("");
  }

  function sumEvlogVisibleEntriesForService(name, arr, filterProbe) {
    var vis = [];
    for (var u = arr.length - 1; u >= 0; u--) {
      var ent2 = arr[u];
      if (
        filterProbe &&
        !gatewayPanelShowProbes &&
        globalThis.ClaudiaLogs &&
        ClaudiaLogs.Derive &&
        typeof ClaudiaLogs.Derive.gatewayPanelHideRow === "function" &&
        ClaudiaLogs.Derive.gatewayPanelHideRow(ent2, function (p) {
          return getFlat(p);
        })
      ) {
        continue;
      }
      vis.push(ent2);
    }
    return vis;
  }

  function eventOneLiner(ev) {
    return primaryLogMessage(ev.parsed, ev.text);
  }

  function buildLogsHref(query) {
    try {
      var p = new URLSearchParams(window.location.search);
      var q = query || {};
      for (var k in q) {
        if (q[k] === null || q[k] === undefined) p.delete(k);
        else p.set(k, String(q[k]));
      }
      return window.location.pathname + (p.toString() ? "?" + p.toString() : "");
    } catch (e) {
      return "#";
    }
  }

  function scheduleFocusTargets() {
    window.setTimeout(function () {
      if (focusSeq) {
        var fs = String(focusSeq).replace(/\\/g, "\\\\").replace(/"/g, '\\"');
        var tr = tbody ? tbody.querySelector("tr[data-log-seq=\"" + fs + '"]') : null;
        if (tr) {
          tr.scrollIntoView({ block: "center", behavior: "smooth" });
          tr.style.outline = "2px solid #0b57d0";
        }
        return;
      }
      if (focusConv) {
        var id = strHash((focusPrincipal || "") + "\0" + focusConv);
        var el = document.getElementById(id);
        if (el) el.scrollIntoView({ block: "center", behavior: "smooth" });
      }
    }, 120);
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
    syncFiltersFromStorage();
    if (viewMode === "summarized") refreshSummarizedPanel();
    else if (viewMode === "raw_logs") rebuildRawLogsTextarea({ scrollBottom: rawWasAtBottom });
    else applyFilters();
    scheduleFocusTargets();
    if (viewMode === "raw" && !focusSeq && !focusConv) {
      window.requestAnimationFrame(function () {
        window.scrollTo(0, document.documentElement.scrollHeight);
      });
    }
    if (viewMode === "raw_logs" && rawWasAtBottom && !focusSeq && !focusConv) {
      window.requestAnimationFrame(function () {
        var ta = document.getElementById("raw-logs-textarea");
        if (ta) ta.scrollTop = ta.scrollHeight;
      });
    }
  }

  function summarizedEvlogInteractionBlocksRebuild() {
    if (Date.now() < sumEvlogPointerSuppressedUntil) return true;
    var a = document.activeElement;
    if (!a || !a.closest) return false;
    if (!a.closest("#panel-summarized")) return false;
    if (a.classList && a.classList.contains("sum-evlog__search")) return true;
    if (a.matches && a.matches("[data-evlog-filter-status]")) return true;
    return false;
  }

  function scheduleDeferredSummarizedRefresh() {
    if (sumEvlogUiDeferTimer) clearTimeout(sumEvlogUiDeferTimer);
    sumEvlogUiDeferTimer = setTimeout(function deferredSumEvlogRefresh() {
      sumEvlogUiDeferTimer = null;
      if (summarizedEvlogInteractionBlocksRebuild()) {
        sumEvlogUiDeferTimer = setTimeout(deferredSumEvlogRefresh, 300);
        return;
      }
      refreshSummarizedPanel();
    }, 300);
  }

  function refreshSummarizedPanel() {
    var psu = document.getElementById("panel-summarized");
    if (viewMode !== "summarized" || !psu) return;
    if (summarizedEvlogInteractionBlocksRebuild()) {
      scheduleDeferredSummarizedRefresh();
      return;
    }
    var prevScrollTop = psu.scrollTop;
    var prevScrollH = psu.scrollHeight;
    var nearPanelBottom =
      psu.scrollHeight - psu.scrollTop - psu.clientHeight <= stickPx;

    var openDetailIds = [];
    try {
      var dOpen = psu.querySelectorAll("details[open][id]");
      for (var di = 0; di < dOpen.length; di++) {
        if (dOpen[di].id) openDetailIds.push(dOpen[di].id);
      }
    } catch (e) { }

    var storyScroll = {};
    try {
      var sps = psu.querySelectorAll(".story-panel");
      for (var sj = 0; sj < sps.length; sj++) {
        var cStory = sps[sj].closest("details[id]");
        if (cStory && cStory.id) storyScroll[cStory.id] = sps[sj].scrollTop;
      }
    } catch (e1) { }

    /** Scroll for .sum-full-log[id] only; evlog tbody scroll lives in sumEvlog panel state (search/filter changes row height). */
    var fullLogScroll = {};
    try {
      var fls = psu.querySelectorAll(".sum-full-log[id]");
      for (var fk = 0; fk < fls.length; fk++) {
        if (fls[fk] && fls[fk].id) fullLogScroll[fls[fk].id] = fls[fk].scrollTop;
      }
    } catch (e2) { }

    var sumEvlogPanelSave = {};
    try {
      if (typeof globalThis.sumEvlogCapturePanelState === "function") {
        sumEvlogPanelSave = globalThis.sumEvlogCapturePanelState(psu) || {};
      }
    } catch (eEvCap) {
      sumEvlogPanelSave = {};
    }

    var gatewayMetricsTableScroll = [];
    try {
      var gwCap = document.getElementById("gw-usage-metrics");
      if (gwCap) {
        var wrapsC = gwCap.querySelectorAll(".sum-metrics-table-wrap");
        for (var wc = 0; wc < wrapsC.length; wc++) {
          gatewayMetricsTableScroll.push({ left: wrapsC[wc].scrollLeft, top: wrapsC[wc].scrollTop });
        }
      }
    } catch (e3) { }

    psu.innerHTML = renderSummarizedUnified();

    hydrateIndexerServiceSummaryFromApi();

    if (typeof globalThis.sumEvlogHydrateAllIn === "function") {
      try {
        globalThis.sumEvlogHydrateAllIn(psu);
      } catch (eEv) {}
    }

    for (var ri = 0; ri < openDetailIds.length; ri++) {
      var d = document.getElementById(openDetailIds[ri]);
      if (d && d.tagName === "DETAILS") d.open = true;
    }

    try {
      if (typeof globalThis.sumEvlogApplyPanelState === "function") {
        globalThis.sumEvlogApplyPanelState(psu, sumEvlogPanelSave, { scroll: false });
      }
    } catch (eEvApply) {}

    /** Best-effort outer scroll before paint: avoids scrollTop 0 flash while content height is still settling. */
    function applySummarizedOuterScrollSync() {
      if (nearPanelBottom) {
        psu.scrollTop = psu.scrollHeight;
      } else {
        var maxS = Math.max(0, psu.scrollHeight - psu.clientHeight);
        psu.scrollTop = Math.min(prevScrollTop, maxS);
      }
    }

    function restoreSummarizedNestedScrolls() {
      var gwR = document.getElementById("gw-usage-metrics");
      if (gwR && gatewayMetricsTableScroll.length) {
        var wrapsR = gwR.querySelectorAll(".sum-metrics-table-wrap");
        for (var mi = 0; mi < wrapsR.length && mi < gatewayMetricsTableScroll.length; mi++) {
          wrapsR[mi].scrollLeft = gatewayMetricsTableScroll[mi].left;
          wrapsR[mi].scrollTop = gatewayMetricsTableScroll[mi].top;
        }
      }
      for (var cid in storyScroll) {
        var cx = document.getElementById(cid);
        if (!cx) continue;
        var sp = cx.querySelector(".story-panel");
        if (sp) sp.scrollTop = storyScroll[cid];
      }
      for (var cid2 in fullLogScroll) {
        var fl = document.getElementById(cid2);
        if (!fl) continue;
        fl.scrollTop = fullLogScroll[cid2];
      }
    }

    applySummarizedOuterScrollSync();
    restoreSummarizedNestedScrolls();

    function finalizeSummarizedScrollAfterLayout() {
      restoreSummarizedNestedScrolls();
      if (nearPanelBottom) {
        psu.scrollTop = psu.scrollHeight;
      } else if (prevScrollH > 0) {
        var dh = psu.scrollHeight - prevScrollH;
        psu.scrollTop = Math.max(0, prevScrollTop + dh);
      }
      try {
        if (typeof globalThis.sumEvlogApplyPanelState === "function") {
          globalThis.sumEvlogApplyPanelState(psu, sumEvlogPanelSave, { scrollOnly: true });
        }
      } catch (eEvScroll) {}
    }
    window.requestAnimationFrame(finalizeSummarizedScrollAfterLayout);
  }

  window.__claudiaToggleGatewayProbes = function (on) {
    gatewayPanelShowProbes = !!on;
    try {
      localStorage.setItem(GW_PROBES_LS, gatewayPanelShowProbes ? "1" : "0");
    } catch (eTg) {}
    refreshSummarizedPanel();
  };

  /** Replace only the gateway metrics card so periodic /api/ui/metrics polls do not rebuild the whole feed. */
  function patchGatewayUsageMetricsCard() {
    if (viewMode !== "summarized") return;
    var psu = document.getElementById("panel-summarized");
    if (!psu) return;
    var oldEl = document.getElementById("gw-usage-metrics");
    if (!oldEl) {
      refreshSummarizedPanel();
      return;
    }
    var keepMainOpen = !!oldEl.open;
    var wrapsOld = oldEl.querySelectorAll(".sum-metrics-table-wrap");
    var tableScroll = [];
    for (var wi = 0; wi < wrapsOld.length; wi++) {
      tableScroll.push({ left: wrapsOld[wi].scrollLeft, top: wrapsOld[wi].scrollTop });
    }

    var wrap = document.createElement("div");
    wrap.innerHTML = buildGatewayUsageCardHtml().trim();
    var newEl = wrap.firstElementChild;
    if (!newEl || newEl.id !== "gw-usage-metrics") return;

    oldEl.parentNode.replaceChild(newEl, oldEl);

    newEl.open = keepMainOpen;
    var wrapsNew = newEl.querySelectorAll(".sum-metrics-table-wrap");
    for (var wj = 0; wj < wrapsNew.length && wj < tableScroll.length; wj++) {
      wrapsNew[wj].scrollLeft = tableScroll[wj].left;
      wrapsNew[wj].scrollTop = tableScroll[wj].top;
    }
  }

  function scheduleStoryRebuild() {
    if (storyRebuildTimer) clearTimeout(storyRebuildTimer);
    storyRebuildTimer = setTimeout(function () {
      storyRebuildTimer = null;
      refreshSummarizedPanel();
      scheduleFocusTargets();
    }, 80);
  }

  function formatInt(n) {
    if (n == null || isNaN(n)) return "—";
    try {
      return new Intl.NumberFormat().format(Math.round(n));
    } catch (e) {
      return String(Math.round(n));
    }
  }

  function aggregateRollupRows(rows) {
    if (!rows || !rows.length) return { models: 0, tokens: 0, calls: 0 };
    var seen = {};
    var tokens = 0;
    var calls = 0;
    for (var i = 0; i < rows.length; i++) {
      var r = rows[i];
      var mid = r.model_id != null ? String(r.model_id) : "";
      if (mid) seen[mid] = true;
      tokens += Number(r.est_tokens) || 0;
      calls += Number(r.calls) || 0;
    }
    var nm = 0;
    for (var k in seen) {
      if (Object.prototype.hasOwnProperty.call(seen, k)) nm++;
    }
    return { models: nm, tokens: tokens, calls: calls };
  }

  function formatCompactTok(n) {
    if (n == null || isNaN(n)) return "—";
    var x = Number(n);
    if (x < 0) return "—";
    if (x >= 1000000) return (x / 1000000).toFixed(2).replace(/\.?0+$/, "") + "M";
    if (x >= 100000) return Math.round(x / 1000) + "k";
    if (x >= 10000) return (x / 1000).toFixed(1).replace(/\.0$/, "") + "k";
    if (x >= 1000) return (x / 1000).toFixed(2).replace(/0+$/, "").replace(/\.$/, "") + "k";
    return formatInt(x);
  }

  function metricsRollupTableHtml(rows) {
    if (!rows || !rows.length) {
      return (
        '<div class="sum-metrics-table-wrap">' +
        '<table class="sum-metrics-table"><tbody>' +
        '<tr><td class="muted">No models used</td></tr>' +
        "</tbody></table></div>"
      );
    }
    var h = [];
    h.push(
      '<div class="sum-metrics-table-wrap"><table class="sum-metrics-table"><thead><tr><th>Provider</th><th>Model</th><th>HTTP</th><th class="num">Calls</th><th class="num">Est. tokens</th></tr></thead><tbody>'
    );
    for (var i = 0; i < rows.length; i++) {
      var r = rows[i];
      h.push(
        "<tr><td>" +
        escapeHtml(r.provider) +
        '</td><td><code class="sum-mono-id">' +
        escapeHtml(r.model_id) +
        '</code></td><td>' +
        escapeHtml(r.status) +
        '</td><td class="num">' +
        escapeHtml(r.calls) +
        '</td><td class="num">' +
        escapeHtml(r.est_tokens) +
        "</td></tr>"
      );
    }
    h.push("</tbody></table></div>");
    return h.join("");
  }

  function pad2Utc(n) {
    return n < 10 ? "0" + n : String(n);
  }

  function formatUtcLikeLogTimestamp(tsOrDate) {
    if (tsOrDate === null || tsOrDate === undefined || tsOrDate === "") return "—";
    var d = tsOrDate instanceof Date ? tsOrDate : new Date(tsOrDate);
    if (isNaN(d.getTime())) return String(tsOrDate).replace("T", " ").slice(0, 23);
    return (
      d.getUTCFullYear() +
      "-" +
      pad2Utc(d.getUTCMonth() + 1) +
      "-" +
      pad2Utc(d.getUTCDate()) +
      " " +
      pad2Utc(d.getUTCHours()) +
      ":" +
      pad2Utc(d.getUTCMinutes()) +
      ":" +
      pad2Utc(d.getUTCSeconds())
    );
  }

  function formatUtcToMinute(tsOrDate) {
    if (tsOrDate === null || tsOrDate === undefined || tsOrDate === "") return "";
    var d = tsOrDate instanceof Date ? tsOrDate : new Date(tsOrDate);
    if (isNaN(d.getTime())) return String(tsOrDate).replace("T", " ").slice(0, 16);
    return (
      d.getUTCFullYear() +
      "-" +
      pad2Utc(d.getUTCMonth() + 1) +
      "-" +
      pad2Utc(d.getUTCDate()) +
      " " +
      pad2Utc(d.getUTCHours()) +
      ":" +
      pad2Utc(d.getUTCMinutes())
    );
  }

  function formatUtcToDay(tsOrDate) {
    if (tsOrDate === null || tsOrDate === undefined || tsOrDate === "") return "";
    var d = tsOrDate instanceof Date ? tsOrDate : new Date(tsOrDate);
    if (isNaN(d.getTime())) return String(tsOrDate).replace("T", " ").slice(0, 10);
    return d.getUTCFullYear() + "-" + pad2Utc(d.getUTCMonth() + 1) + "-" + pad2Utc(d.getUTCDate());
  }

  function metricsEventsTableHtml(rows) {
    if (!rows || !rows.length) {
      return '<p class="muted">No events recorded yet.</p>';
    }
    var h = [];
    h.push(
      '<div class="sum-metrics-table-wrap sum-metrics-events-scroll"><table class="sum-metrics-table"><thead><tr><th>Time (UTC)</th><th>Provider</th><th>Model</th><th>HTTP</th><th class="num">Est. tokens</th></tr></thead><tbody>'
    );
    for (var i = 0; i < rows.length; i++) {
      var r = rows[i];
      h.push(
        "<tr><td>" +
        escapeHtml(formatUtcLikeLogTimestamp(r.occurred_at)) +
        "</td><td>" +
        escapeHtml(r.provider) +
        '</td><td><code class="sum-mono-id">' +
        escapeHtml(r.model_id) +
        '</code></td><td>' +
        escapeHtml(r.status) +
        '</td><td class="num">' +
        escapeHtml(r.est_tokens) +
        "</td></tr>"
      );
    }
    h.push("</tbody></table></div>");
    return h.join("");
  }

  function fetchGatewayMetrics() {
    fetch("/api/ui/metrics?limit=150", { credentials: "same-origin" })
      .then(function (res) {
        if (res.status === 401) return null;
        if (!res.ok) throw new Error("HTTP " + res.status);
        return res.json();
      })
      .then(function (data) {
        if (!data) return;
        metricsCache = data;
        if (viewMode === "summarized") patchGatewayUsageMetricsCard();
      })
      .catch(function (e) {
        metricsCache = {
          metrics_store_open: false,
          message: e && e.message ? String(e.message) : String(e)
        };
        if (viewMode === "summarized") patchGatewayUsageMetricsCard();
      });
  }

  function syncMetricsPolling() {
    if (metricsPollTimer) {
      try {
        clearInterval(metricsPollTimer);
      } catch (x) { }
      metricsPollTimer = null;
    }
    if (viewMode !== "summarized") return;
    fetchGatewayMetrics();
    metricsPollTimer = setInterval(fetchGatewayMetrics, METRICS_POLL_MS);
  }

  /**
   * Fetch the live BiFrost provider snapshot. The gateway-side handler at
   * /api/ui/bifrost/providers walks the configured provider roster and returns one
   * { id, state, key_count, ... } row per entry. Strip render prefers this snapshot
   * over log-derived state; logs lag/drop providers BiFrost doesn't explicitly slog.
   */
  function fetchBifrostProviderSnapshot() {
    fetch("/api/ui/bifrost/providers", { credentials: "same-origin" })
      .then(function (res) {
        if (res.status === 401) return null;
        if (!res.ok) throw new Error("HTTP " + res.status);
        return res.json();
      })
      .then(function (data) {
        if (!data) return;
        bifrostProviderSnapshot = { fetchedClientMs: Date.now(), data: data };
        if (viewMode === "summarized") patchBifrostProviderHealthStrip();
      })
      .catch(function () {
        // Keep any prior snapshot — staleness check in the renderer handles fallback.
      });
  }

  function syncBifrostProviderPolling() {
    if (bifrostProviderPollTimer) {
      try {
        clearInterval(bifrostProviderPollTimer);
      } catch (x) { }
      bifrostProviderPollTimer = null;
    }
    if (viewMode !== "summarized") return;
    fetchBifrostProviderSnapshot();
    bifrostProviderPollTimer = setInterval(fetchBifrostProviderSnapshot, BIFROST_PROVIDER_POLL_MS);
  }

  /** Replace bifrost provider health UI after a snapshot poll (expanded strip + collapsed summary indicators). */
  function patchBifrostProviderHealthStrip() {
    if (viewMode !== "summarized") return;
    var arr = collectBifrostBufferForStrip();
    var oldEl = document.getElementById("bifrost-provider-health-strip");
    if (oldEl) {
      var wrap = document.createElement("div");
      wrap.innerHTML = bifrostProviderHealthStripHtml(arr).trim();
      var newEl = wrap.firstElementChild;
      if (newEl && newEl.id === "bifrost-provider-health-strip") {
        oldEl.parentNode.replaceChild(newEl, oldEl);
      }
    }
    var compactOld = document.getElementById("bifrost-provider-health-compact");
    if (compactOld) {
      var w2 = document.createElement("div");
      w2.innerHTML = bifrostProviderHealthStripHtml(arr, { compact: true }).trim();
      var n2 = w2.firstElementChild;
      if (n2 && n2.id === "bifrost-provider-health-compact") {
        compactOld.parentNode.replaceChild(n2, compactOld);
      }
    }
  }

  /** Mirror the bifrost-bucket selection in refreshSummarizedPanel so the patched strip's
   *  log-derived fallback sees the same arr the original renderExpandedService("bifrost") saw. */
  function collectBifrostBufferForStrip() {
    var out = [];
    for (var i = 0; i < entryCache.length; i++) {
      var e = entryCache[i];
      if (!e || !e.parsed) continue;
      var f = getFlat(e.parsed);
      var svc = f.service ? String(f.service) : "";
      var isBifrost = svc === "bifrost" || (e.source === "bifrost") || entryRoutesToBifrostBucket(e);
      if (isBifrost) out.push(e);
    }
    return out;
  }

  /** Explainer strip at top of the Gateway usage metrics card (/api/ui/metrics). */
  function buildGatewayUsageIntroHtml() {
    return (
      '<div class="gw-usage-intro" id="gw-usage-intro">' +
      '<p class="gw-usage-intro-lead">' +
      "Which models ran, estimated tokens this UTC minute and calendar day, and the latest upstream calls." +
      "</p>" +
      '<p class="gw-usage-intro-follow">' +
      "Use this to spot load before quotas hard-stop traffic. The gateway compares these rollups to the ceilings in your limits file — counts are directional estimates; each vendor bills differently." +
      "</p>" +
      '<ul class="gw-usage-intro-bullets">' +
      "<li>" +
      '<a class="sum-ext-link" href="https://console.groq.com/docs/rate-limits" rel="noopener noreferrer">Groq rate limits</a>' +
      " and " +
      '<a class="sum-ext-link" href="https://ai.google.dev/gemini-api/docs/pricing" rel="noopener noreferrer">Gemini pricing &amp; free tier</a>' +
      " pages inform scraped free-tier hints." +
      "</li>" +
      "<li>" +
      "Ceiling tables live in " +
      '<a href="#" class="sum-proj-path" data-rel="config/provider-model-limits.yaml"><code>config/provider-model-limits.yaml</code></a>' +
      "; the gateway applies rollups against them when metrics are enabled." +
      "</li>" +
      "</ul>" +
      "</div>"
    );
  }

  /** Explainer strip at top of the Gateway service card (Services → Gateway). */
  function buildGatewayCardIntroHtml() {
    return (
      '<div class="gw-svc-card-intro" id="gw-svc-card-intro">' +
      '<p class="gw-svc-card-intro-lead">' +
      "An at-a-glance snapshot of this gateway instance—how it listens, where it connects, which supervised helpers started, and light traffic counts from gateway lines in the current view. For richer token rollups and upstream trails, open Gateway usage (Stats); figures here only cover lines loaded in this window." +
      "</p>" +
      "</div>"
    );
  }

  /** Explainer strip at top of the BiFrost relay card (mirrors buildGatewayUsageIntroHtml). */
  function buildBifrostCardIntroHtml() {
    return (
      '<div class="bf-card-intro" id="bf-card-intro">' +
      '<p class="bf-card-intro-lead">' +
      "A fast health and traffic summary for the relay path into models. Odd patterns here usually mean throttling, misconfiguration, or an upstream hiccup—not necessarily that chat is already broken." +
      "</p>" +
      "</div>"
    );
  }

  /** Explainer strip at top of the Qdrant service card (mirrors the gateway / BiFrost intros). */
  function buildQdrantCardIntroHtml() {
    return (
      '<div class="qd-card-intro" id="qd-card-intro">' +
      '<p class="qd-card-intro-lead">' +
      "Qdrant is the local vector store the indexer fills and retrieval queries—this strip shows whether that subprocess is up and whether writes and searches are succeeding. Weak numbers here often mean thinner RAG before chat complains; counts reflect what the API reported, not a full on-disk audit." +
      "</p>" +
      "</div>"
    );
  }

  /** Explainer strip at top of the Indexer service card (mirrors the gateway / BiFrost / Qdrant intros). */
  function buildIndexerCardIntroHtml() {
    return (
      '<div class="ix-card-intro" id="ix-card-intro">' +
      '<p class="ix-card-intro-lead">' +
      "A quick read on how the supervised indexer is keeping watched trees in sync—backlog, throughput, and per-file outcomes at a glance. When those numbers drift the wrong way, retrieval can go stale; pauses and skips shown here are intentional signals, not silent drops." +
      "</p>" +
      "</div>"
    );
  }

  function findWorkspaceDraft(id) {
    for (var i = 0; i < workspaceDrafts.length; i++) {
      if (workspaceDrafts[i].id === id) return workspaceDrafts[i];
    }
    return null;
  }

  function removeWorkspaceDraft(id) {
    var next = [];
    for (var i = 0; i < workspaceDrafts.length; i++) {
      if (workspaceDrafts[i].id !== id) next.push(workspaceDrafts[i]);
    }
    workspaceDrafts = next;
  }

  function notifyWorkspaceDraftMsg(msg, isErr) {
    if (!statusEl) return;
    statusEl.textContent = msg || "";
    statusEl.className = msg ? (isErr ? "workspace-draft-status workspace-draft-status--err" : "workspace-draft-status") : "";
    if (msg && !isErr) {
      try {
        window.clearTimeout(notifyWorkspaceDraftMsg._t);
      } catch (_e) {}
      notifyWorkspaceDraftMsg._t = window.setTimeout(function () {
        if (statusEl && statusEl.textContent === msg) {
          statusEl.textContent = "";
          statusEl.className = "";
        }
      }, 4800);
    }
  }

  function dirBasenameForWorkspace(p) {
    if (!p) return "";
    var s = String(p).replace(/[/\\]+$/, "");
    var i = Math.max(s.lastIndexOf("/"), s.lastIndexOf("\\"));
    return i >= 0 ? s.slice(i + 1) : s;
  }

  function nativeFolderPickerFn() {
    try {
      var topw = window.top;
      if (topw && typeof topw.claudiaPickFolder === "function") return topw.claudiaPickFolder;
    } catch (e) {}
    return typeof window.claudiaPickFolder === "function" ? window.claudiaPickFolder : null;
  }

  function syncWorkspaceDraftHeader(cardEl, d) {
    if (!cardEl || !d) return;
    var u = cardEl.querySelector(".ws-draft-lbl-user");
    var p = cardEl.querySelector(".ws-draft-lbl-proj");
    var f = cardEl.querySelector(".ws-draft-lbl-flav");
    var ulab = resolveLogsOperatorUserLabel();
    if (u) u.textContent = ulab !== "—" ? ulab : "";
    if (p) p.textContent = String(d.projectId != null ? d.projectId : "").trim();
    if (f) f.textContent = String(d.flavorId != null ? d.flavorId : "").trim();
  }

  function buildWorkspaceDraftCardHtml(d) {
    var uid = "ws-draft-" + d.id;
    var ulab = resolveLogsOperatorUserLabel();
    var projShown = String(d.projectId != null ? d.projectId : "").trim();
    var flavShown = String(d.flavorId != null ? d.flavorId : "").trim();
    var titleBits =
      '<span class="ws-draft-head-inline">' +
      '<span class="ws-draft-lbl ws-draft-lbl-user">' +
      (ulab !== "—" ? escapeHtml(ulab) : "") +
      "</span>" +
      '<span class="ws-draft-sep muted">·</span>' +
      '<span class="ws-draft-lbl ws-draft-lbl-proj">' +
      escapeHtml(projShown) +
      "</span>" +
      '<span class="ws-draft-sep muted">·</span>' +
      '<span class="ws-draft-lbl ws-draft-lbl-flav">' +
      escapeHtml(flavShown) +
      "</span>" +
      "</span>";
    var paths = d.paths && d.paths.length ? d.paths : [];
    var rmDisabledAttr = paths.length ? "" : " disabled";
    var selOpts = "";
    for (var pi = 0; pi < paths.length; pi++) {
      selOpts +=
        '<option value="' +
        pi +
        '">' +
        escapeHtml(paths[pi]) +
        "</option>";
    }
    var prVal = escapeHtml(String(d.projectId != null ? d.projectId : ""));
    var fvVal = escapeHtml(String(d.flavorId != null ? d.flavorId : ""));
    return (
      '<article class="sum-card sum-card--workspace-draft" id="' +
      escapeHtml(uid) +
      '" data-workspace-draft="' +
      String(d.id) +
      '">' +
      '<header class="sum-card__workspace-draft-hdr">' +
      '<span class="sum-avatar sum-av-c" title="New workspace">+</span>' +
      '<span class="sum-main sum-main--workspace-draft">' +
      '<span class="sum-title">' +
      titleBits +
      "</span>" +
      "</span>" +
      '<span class="ws-draft-actions">' +
      '<button type="button" class="ws-draft-btn ws-draft-btn-cancel">Cancel</button>' +
      '<button type="button" class="ws-draft-btn ws-draft-btn-save">Save</button>' +
      "</span>" +
      "</header>" +
      '<div class="sum-body">' +
      '<div class="ws-draft-fields">' +
      '<div class="ws-draft-field">' +
      '<label class="ws-draft-field-label" for="' +
      escapeHtml(uid) +
      '-pr">Project id</label>' +
      '<input id="' +
      escapeHtml(uid) +
      '-pr" class="ws-draft-input" type="text" autocomplete="off" data-ws-field="project" value="' +
      prVal +
      '" />' +
      "</div>" +
      '<div class="ws-draft-field">' +
      '<label class="ws-draft-field-label" for="' +
      escapeHtml(uid) +
      '-fv">Flavor id</label>' +
      '<input id="' +
      escapeHtml(uid) +
      '-fv" class="ws-draft-input" type="text" autocomplete="off" data-ws-field="flavor" value="' +
      fvVal +
      '" />' +
      "</div>" +
      "</div>" +
      '<div class="sum-section-label">Watched paths</div>' +
      '<div class="ws-draft-paths-row">' +
      '<select class="ws-draft-paths-select" size="6" aria-label="Watched paths" data-ws-draft-paths="' +
      String(d.id) +
      '">' +
      selOpts +
      "</select>" +
      '<div class="ws-draft-path-btns">' +
      '<button type="button" class="ws-draft-btn ws-draft-btn-add">Add</button>' +
      '<button type="button" class="ws-draft-btn ws-draft-btn-remove"' +
      rmDisabledAttr +
      ">Remove</button>" +
      "</div>" +
      "</div>" +
      '<p class="muted ws-draft-hint">Folder picker requires the Claudia desktop shell (or an environment that exposes <code>claudiaPickFolder</code>).</p>' +
      "</div>" +
      "</article>"
    );
  }

  function saveWorkspaceDraftById(draftId) {
    var d = findWorkspaceDraft(draftId);
    if (!d) return;
    var pj = String(d.projectId != null ? d.projectId : "").trim();
    var fv = String(d.flavorId != null ? d.flavorId : "").trim();
    if (!pj) {
      notifyWorkspaceDraftMsg("Project id is required.", true);
      return;
    }
    if (!d.paths || !d.paths.length) {
      notifyWorkspaceDraftMsg("Add at least one watched path.", true);
      return;
    }
    fetch("/api/ui/indexer/workspaces", {
      method: "POST",
      credentials: "same-origin",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        project_id: pj,
        flavor_id: fv,
        paths: d.paths.slice()
      })
    })
      .then(function (res) {
        return res.json().then(function (j) {
          if (!res.ok) throw new Error((j && j.error) || res.statusText || "save failed");
          return j;
        });
      })
      .then(function (j) {
        removeWorkspaceDraft(draftId);
        notifyWorkspaceDraftMsg("Workspace saved.", false);
        if (j && Array.isArray(j.roots)) {
          lastIndexerOperatorRoots = j.roots;
          try {
            lastIndexerOperatorRootsJson = JSON.stringify(j.roots);
          } catch (_eSaveRoots) {
            lastIndexerOperatorRootsJson = "";
          }
        }
        if (j && j.workspace && typeof j.workspace === "object") {
          mergeWorkspaceIntoOperatorNested(j.workspace);
        } else if (j && Array.isArray(j.roots)) {
          lastIndexerOperatorWorkspacesNested = dedupeOperatorWorkspacesNested(
            deriveNestedWorkspacesFromFlatRoots(j.roots)
          );
        }
        hydrateIndexerServiceSummaryFromApi();
        scheduleStoryRebuild();
      })
      .catch(function (err) {
        notifyWorkspaceDraftMsg(err && err.message ? err.message : String(err), true);
      });
  }

  function appendWorkspaceDraftPath(d, absPath) {
    var p = String(absPath || "").trim();
    if (!p) return;
    if (!d.paths) d.paths = [];
    var qi;
    for (qi = 0; qi < d.paths.length; qi++) {
      if (d.paths[qi] === p) return;
    }
    d.paths.push(p);
    var pjBlank = !String(d.projectId != null ? d.projectId : "").trim();
    if (pjBlank) {
      var base = dirBasenameForWorkspace(p);
      d.projectId = base;
      d.flavorId = "";
    }
  }

  function pickFolderForWorkspaceDraft(startDir) {
    var fn = nativeFolderPickerFn();
    if (!fn) {
      notifyWorkspaceDraftMsg("Folder picker requires the Claudia desktop app (claudiaPickFolder).", true);
      return Promise.resolve("");
    }
    var sd = startDir != null && startDir !== undefined ? String(startDir) : "";
    return Promise.resolve(fn(sd)).then(function (path) {
      return (path && String(path).trim()) || "";
    });
  }

  /** Intro copy for the summarized-feed Workspaces section (replaces per-card blurbs). */
  function buildWorkspacesSectionIntroHtml() {
    return (
      '<div class="sum-workspaces-intro">' +
      '<p class="sum-workspaces-intro-lead">' +
      "Find the right snippets and docs while you work, without you pasting whole files into the chat. Point it at the places you care about, and it stays quietly up to date." +
      "</p>" +
      "</div>"
    );
  }

  function buildGatewayUsageCardHtml() {
    var data = metricsCache;
    var m =
      globalThis.ClaudiaLogs &&
        globalThis.ClaudiaLogs.Derive &&
        globalThis.ClaudiaLogs.Derive.gatewayUsageCardModel
        ? globalThis.ClaudiaLogs.Derive.gatewayUsageCardModel(
          data,
          function (rows) { return aggregateRollupRows(rows); },
          function (id) { return bifrostShortModelLabel(id); }
        )
        : null;

    var loading = m ? !!m.loading : !data;
    var storeOpen = m ? !!m.storeOpen : !!(data && data.metrics_store_open);
    var lastModel = m ? m.lastModelId || "—" : "—";
    var minAgg = m ? m.minAgg || { models: 0, tokens: 0 } : { models: 0, tokens: 0 };
    var dayAgg = m ? m.dayAgg || { models: 0, tokens: 0 } : { models: 0, tokens: 0 };
    var lblMin = m ? m.lblMin || "" : "";
    var lblDay = m ? m.lblDay || "" : "";
    var lblMinFmt = lblMin ? formatUtcToMinute(lblMin) : "";
    var lblDayFmt = lblDay ? formatUtcToDay(lblDay) : "";

    var sub = loading
      ? '<span class="sum-sub sum-sub--clamp muted">Loading gateway metrics…</span>'
      : '<span class="sum-sub sum-sub--clamp">Last model <code class="sum-mono-id">' +
      escapeHtml(bifrostShortModelLabel(lastModel)) +
      "</code></span>";

    var minTail = loading ? "…" : formatInt(minAgg.models) + " models · " + formatCompactTok(minAgg.tokens) + " tokens";
    var dayTail = loading ? "…" : formatInt(dayAgg.models) + " models · " + formatCompactTok(dayAgg.tokens) + " tokens";
    var minPillHtml = "<strong>minute</strong> · " + escapeHtml(minTail);
    var dayPillHtml = "<strong>day</strong> · " + escapeHtml(dayTail);

    var metrics =
      '<span class="sum-metrics">' +
      '<span class="sum-metric" title="Distinct upstream models · summed est. tokens (UTC minute rollup)">' +
      minPillHtml +
      '</span><span class="sum-metric" title="Distinct upstream models · summed est. tokens (UTC calendar day rollup)">' +
      dayPillHtml +
      "</span></span>";

    var introHtml = buildGatewayUsageIntroHtml();

    var expandedInner = "";
    if (loading) {
      expandedInner = introHtml + '<p class="muted">Fetching /api/ui/metrics…</p>';
    } else if (!storeOpen) {
      expandedInner =
        introHtml +
        '<p class="muted">' +
        escapeHtml((m && m.message) || (data && data.message) || "Metrics store is not available.") +
        "</p>";
    } else {
      expandedInner = introHtml;
      expandedInner +=
        '<div class="sum-section-label">CURRENT MINUTE · ' +
        escapeHtml(lblMinFmt || lblMin) +
        "</div>" +
        metricsRollupTableHtml(data.minute_rollups || []) +
        '<div class="sum-section-label">CURRENT DAY · ' +
        escapeHtml(lblDayFmt || lblDay) +
        "</div>" +
        metricsRollupTableHtml(data.day_rollups || []) +
        '<div class="sum-section-label">Recent upstream calls</div>' +
        metricsEventsTableHtml(data.recent_events || []);
    }

    return (
      '<details class="sum-card" id="gw-usage-metrics">' +
      "<summary>" +
      '<span class="sum-avatar sum-av-svc-gateway">GW</span>' +
      '<span class="sum-main"><span class="sum-title">Model usage metrics</span>' +
      sub +
      "</span>" +
      metrics +
      '<span class="sum-chev"></span></summary>' +
      '<div class="sum-body">' +
      expandedInner +
      "</div></details>"
    );
  }

  function buildGatewayUsageFeedSection() {
    return (
      '<div class="sum-feed-section">' +
      '<div class="sum-section-label sum-feed-section-title">Gateway usage</div>' +
      buildGatewayUsageCardHtml() +
      "</div>"
    );
  }

  function fetchTokenLabels() {
    fetch("/api/ui/tokens", { credentials: "same-origin" })
      .then(function (r) {
        if (!r.ok) return null;
        return r.json();
      })
      .then(function (data) {
        if (!data || !Array.isArray(data.tokens)) return;
        tokenLabelByTenant = {};
        for (var i = 0; i < data.tokens.length; i++) {
          var row = data.tokens[i];
          var tid =
            row.tenant_id != null && String(row.tenant_id).trim() !== ""
              ? String(row.tenant_id).trim()
              : "";
          if (!tid) continue;
          var lb =
            row.label != null && String(row.label).trim() !== ""
              ? String(row.label).trim()
              : "";
          tokenLabelByTenant[tid] = lb || tid;
        }
        if (viewMode === "summarized") scheduleStoryRebuild();
      })
      .catch(function () { });
  }

  /** Conversation card title: "label (tenant_id) - uuid" using token label when known. */
  function formatConversationCardTitle(tenantId, convId) {
    var tid = String(tenantId || "").trim();
    if (!tid) tid = "(unknown principal)";
    var lab = tokenLabelByTenant[tid];
    var head;
    if (lab && lab !== tid)
      head = escapeHtml(lab) + " (" + escapeHtml(tid) + ")";
    else
      head = escapeHtml(tid);
    var c = String(convId || "");
    var cshow = c.length > 48 ? c.slice(0, 48) + "…" : c;
    return (
      head +
      ' <span style="opacity:.55">-</span> <code class="sum-mono-id" style="font-size:0.85em">' +
      escapeHtml(cshow) +
      "</code>"
    );
  }

  function avatarInitials(label) {
    var s = String(label || "?").trim();
    if (!s) return "??";
    var parts = s.split(/[^a-zA-Z0-9]+/).filter(Boolean);
    if (parts.length >= 2)
      return (String(parts[0][0] || "") + String(parts[1][0] || ""))
        .toUpperCase()
        .slice(0, 2);
    var t = s.replace(/[^a-zA-Z0-9]/g, "").toUpperCase();
    return t.slice(0, 2) || "??";
  }

  function avatarHueClass(seed) {
    var h = strHash(String(seed || ""));
    var n = parseInt(String(h).replace(/[^\d]/g, "0"), 10) || 0;
    var classes = ["sum-av-a", "sum-av-b", "sum-av-c", "sum-av-d", "sum-av-e", "sum-av-f"];
    return classes[n % classes.length];
  }

  function inferServiceBadge(ev) {
    var src = (ev.source || (ev.parsed && ev.parsed.app) || "").toLowerCase();
    var f = getFlat(ev.parsed);
    var sh = (ev.parsed && ev.parsed.shape) || "";
    if (src === "qdrant" || sh === "service.qdrant" || f.service === "qdrant")
      return { cls: "sum-svc-qdrant", lab: "qdrant" };
    if (src === "indexer" || sh.indexOf("indexer") === 0 || f.service === "indexer")
      return { cls: "sum-svc-indexer", lab: "indexer" };
    if (src === "bifrost" || sh.indexOf("bifrost") >= 0 || sh.indexOf("chat.bifrost") === 0)
      return { cls: "sum-svc-upstream", lab: "upstream" };
    if (sh === "http.access" || (f.method && f.path)) return { cls: "sum-svc-web", lab: "web" };
    if (sh === "chat.routing") return { cls: "sum-svc-gateway", lab: "routing" };
    return { cls: "sum-svc-gateway", lab: "gateway" };
  }

  /**
   * Timeline segment key for gateway request bar (TIMELINE_BAR_KINDS).
   * Prefers structured `timeline_kind` from gateway/RAG/ingest logs (server-emitted); falls back to inferServiceBadge.
   */
  function timelineKindLab(ev) {
    var f = getFlat(ev.parsed);
    var tk = f.timeline_kind != null ? String(f.timeline_kind).trim().toLowerCase() : "";
    if (tk === "web" || tk === "qdrant" || tk === "upstream" || tk === "indexer" || tk === "gateway") {
      return tk;
    }
    var lab = inferServiceBadge(ev).lab;
    if (lab === "web") return "web";
    if (lab === "qdrant") return "qdrant";
    if (lab === "upstream") return "upstream";
    if (lab === "indexer") return "indexer";
    return "gateway";
  }

  function formatTimeHm(ev) {
    var ins = entryInstant({ ts: ev.ts });
    if (!ins) return "—";
    try {
      return new Intl.DateTimeFormat(undefined, {
        hour: "2-digit",
        minute: "2-digit",
        second: "2-digit",
        hour12: false
      }).format(ins);
    } catch (e) {
      return ins.toTimeString().slice(0, 8);
    }
  }

  function scrapeConversationMetrics(events) {
    if (globalThis.ClaudiaLogs && globalThis.ClaudiaLogs.Derive && globalThis.ClaudiaLogs.Derive.scrapeConversationMetrics) {
      return globalThis.ClaudiaLogs.Derive.scrapeConversationMetrics(events, getFlat);
    }
    return { tok: null, vec: null };
  }

  function conversationCardModelForGroup(events) {
    if (globalThis.ClaudiaLogs && ClaudiaLogs.Derive && typeof ClaudiaLogs.Derive.buildConversationCardModel === "function") {
      return ClaudiaLogs.Derive.buildConversationCardModel(events, getFlat);
    }
    return {
      stateLabel: "—",
      stateKind: "complete",
      progress: {
        received: "pending",
        routed: "pending",
        rag: "pending",
        upstream: "pending",
        delivered: "pending"
      },
      kv: { turnIndex: "", clientModel: "", upstreamModel: "", stream: "", ragCollection: "", mergeHint: "" },
      chips: { tools: 0, fallback: 0 },
      ingestRunIds: [],
      witness: { request: false, response: false }
    };
  }

  function conversationLifecycleStepDefs() {
    return [
      { k: "received", lab: "Accepted" },
      { k: "rag", lab: "Context" },
      { k: "routed", lab: "Routed" },
      { k: "upstream", lab: "Upstream" },
      { k: "delivered", lab: "Delivered" }
    ];
  }

  function conversationLifecycleStateClass(raw) {
    var st = String(raw || "pending").replace(/[^a-z]/gi, "");
    return st || "pending";
  }

  /**
   * Segmented lifecycle bar (5 equal segments, small gaps). opts.compact: summary row, no labels (hidden when card open via CSS).
   */
  function conversationLifecycleBarHtml(progress, opts) {
    opts = opts || {};
    var compact = !!opts.compact;
    progress = progress || {};
    var steps = conversationLifecycleStepDefs();
    var trackParts = [];
    var labelParts = [];
    var ariaBits = [];
    var si;
    for (si = 0; si < steps.length; si++) {
      var step = steps[si];
      var rawSt = progress[step.k];
      var st = conversationLifecycleStateClass(rawSt);
      var shown = String(rawSt != null && rawSt !== "" ? rawSt : "pending");
      var title = step.lab + ": " + shown;
      ariaBits.push(step.lab + " " + shown);
      trackParts.push(
        '<span class="sum-conv-lifecycle-seg sum-conv-lifecycle-seg--' +
          st +
          '" title="' +
          escapeHtml(title) +
          '"></span>'
      );
      if (!compact) {
        labelParts.push(
          '<span class="sum-conv-lifecycle-bar-label" title="' +
            escapeHtml(title) +
            '">' +
            escapeHtml(step.lab) +
            "</span>"
        );
      }
    }
    var track =
      '<div class="sum-conv-lifecycle-bar-track">' + trackParts.join("") + "</div>";
    var labels = compact ? "" : '<div class="sum-conv-lifecycle-bar-labels">' + labelParts.join("") + "</div>";
    var wrapCls = "sum-conv-lifecycle-bar" + (compact ? " sum-conv-lifecycle-bar--compact" : "");
    var aria = ' role="group" aria-label="' +
      escapeHtml(compact ? "Lifecycle: " + ariaBits.join(", ") : "Request lifecycle") +
      '"';
    return '<div class="' + wrapCls + '"' + aria + ">" + track + labels + "</div>";
  }

  function conversationCardChipsSummaryHtml(model) {
    model = model || {};
    var ch = model.chips || {};
    var parts = [];
    if ((ch.tools || 0) > 0) parts.push("Tools · " + ch.tools);
    if ((ch.fallback || 0) > 0) parts.push("Fallback · " + ch.fallback);
    if (!parts.length) return "";
    var h = '<div class="sum-conv-chip-row sum-conv-chip-row--summary">';
    for (var pi = 0; pi < parts.length; pi++) {
      h += '<span class="sum-conv-chip">' + escapeHtml(parts[pi]) + "</span>";
    }
    h += "</div>";
    return h;
  }

  function conversationCardKvHtml(model) {
    model = model || {};
    var kv = model.kv || {};
    function row(k, v) {
      if (!v || String(v).trim() === "" || String(v) === "—") return "";
      return (
        '<div class="sum-conv-kv-row"><dt>' +
        escapeHtml(k) +
        "</dt><dd>" +
        escapeHtml(String(v)) +
        "</dd></div>"
      );
    }
    var body =
      row("Turn", kv.turnIndex) +
      row("Client model", kv.clientModel) +
      row("Upstream model", kv.upstreamModel) +
      row("Stream", kv.stream) +
      row(
        "RAG collection",
        kv.ragCollection && typeof ragCollectionLabelForUi === "function"
          ? ragCollectionLabelForUi(kv.ragCollection)
          : kv.ragCollection
      ) +
      row("Merge", kv.mergeHint);
    if (!body) return "";
    return (
      '<div class="sum-conv-kv"><div class="sum-section-label">Conversation</div><dl class="sum-conv-kv-grid">' +
      body +
      "</dl></div>"
    );
  }

  function convEventDedupeKey(ent) {
    if (ent.seq != null && ent.seq !== "") return "s:" + String(ent.seq);
    return "t:" + String(ent.ts) + ":" + String(ent.text || "").slice(0, 80);
  }

  function pushConversationGroupedEvent(groups, pidUse, cidUse, ent, p, tier, meta) {
    if (!cidUse) return;
    if (!pidUse) pidUse = "(unknown principal)";
    var keyC = pidUse + "\0" + cidUse;
    if (!groups[keyC]) groups[keyC] = { pid: pidUse, cid: cidUse, events: [] };
    var g = groups[keyC];
    var dk = convEventDedupeKey(ent);
    for (var ei = 0; ei < g.events.length; ei++) {
      if (convEventDedupeKey(g.events[ei]) === dk) return;
    }
    var outEv = {
      parsed: p,
      text: ent.text || "",
      ts: ent.ts,
      seq: ent.seq,
      convJoinTier: tier
    };
    if (meta && typeof meta === "object") {
      if (meta.span_id) outEv.qdrantSpanID = String(meta.span_id);
      if (meta.turn_index != null) outEv.qdrantTurnIndex = meta.turn_index;
      if (meta.span_start_ms != null) outEv.qdrantSpanStartMs = meta.span_start_ms;
    }
    g.events.push(outEv);
  }

  function entryIsQdrantSubprocessForConvJoin(ent) {
    var f = getFlat(ent.parsed);
    if (String(f.service || "").toLowerCase() !== "qdrant") return false;
    var msg = String(f.msg != null ? f.msg : "").toLowerCase();
    return msg.indexOf("qdrant.") === 0;
  }

  function conversationRequestIdTier2EligibleLocal(f) {
    if (globalThis.ClaudiaLogs && ClaudiaLogs.Derive && typeof ClaudiaLogs.Derive.conversationRequestIdTier2Eligible === "function") {
      return ClaudiaLogs.Derive.conversationRequestIdTier2Eligible(f);
    }
    return (
      globalThis.ClaudiaLogs &&
      ClaudiaLogs.Derive &&
      ClaudiaLogs.Derive.conversationBifrostTimelineFlat &&
      ClaudiaLogs.Derive.conversationBifrostTimelineFlat(f)
    );
  }

  function conversationIndexRunTier3EligibleLocal(f) {
    if (globalThis.ClaudiaLogs && ClaudiaLogs.Derive && typeof ClaudiaLogs.Derive.conversationIndexRunTier3Eligible === "function") {
      return ClaudiaLogs.Derive.conversationIndexRunTier3Eligible(f);
    }
    return false;
  }

  /**
   * Register request_id → (principal, conversation_id) only from authoritative gateway rows.
   * First mapping wins. Primary pass prefers lifecycle / chat.request / chat access so later rows
   * (or RAG lines that may appear early) cannot re-point a request at another conversation card.
   */
  function tryRegisterRequestConversationCorrelationPrimary(reqToConv, f) {
    if (!f || typeof f !== "object") return;
    var rid = f.request_id != null ? String(f.request_id).trim() : "";
    var cid = f.conversation_id != null ? String(f.conversation_id).trim() : "";
    var pid = f.principal_id != null ? String(f.principal_id).trim() : f.tenant != null ? String(f.tenant).trim() : "";
    if (!rid || !cid || !pid || reqToConv[rid]) return;
    var msg = String(f.msg != null ? f.msg : f.message != null ? f.message : "").trim();
    if (msg === "conversation.received" || msg === "chat.request") {
      reqToConv[rid] = { pid: pid, cid: cid };
      return;
    }
    var ml = msg.toLowerCase();
    if (ml === "gateway.http.access" || ml === "http response") {
      var pth = String(f.path || "").split("?")[0];
      if (pth.indexOf("/v1/chat/completions") >= 0) {
        reqToConv[rid] = { pid: pid, cid: cid };
      }
    }
  }

  function tryRegisterRequestConversationCorrelationRagFallback(reqToConv, f) {
    if (!f || typeof f !== "object") return;
    var rid = f.request_id != null ? String(f.request_id).trim() : "";
    var cid = f.conversation_id != null ? String(f.conversation_id).trim() : "";
    var pid = f.principal_id != null ? String(f.principal_id).trim() : f.tenant != null ? String(f.tenant).trim() : "";
    if (!rid || !cid || !pid || reqToConv[rid]) return;
    var msg = String(f.msg != null ? f.msg : f.message != null ? f.message : "").trim();
    if (msg === "rag.query" || msg === "rag.embed") {
      reqToConv[rid] = { pid: pid, cid: cid };
    }
  }

  function conversationCardStatus(g, t1, cardModel) {
    cardModel = cardModel || conversationCardModelForGroup(g.events);
    if (cardModel.stateKind === "error") {
      return { st: cardModel.stateLabel || "error", cls: "sum-st-error" };
    }
    if (cardModel.stateKind === "warn") {
      return { st: cardModel.stateLabel || "warn", cls: "sum-st-indexing" };
    }
    if (recentConvEventsHaveError(g.events)) return { st: "error", cls: "sum-st-error" };
    var now = Date.now();
    if (t1 && now - t1.getTime() < 45000) return { st: "active", cls: "sum-st-active sum-pulse" };
    var stLab = cardModel.stateLabel && cardModel.stateLabel !== "—" ? cardModel.stateLabel : "complete";
    return { st: stLab, cls: "sum-st-complete" };
  }

  function countWarnErrorInEntries(arr) {
    var n = 0;
    for (var i = 0; i < arr.length; i++) {
      var lv = arr[i].parsed.levelCanon || "";
      if (lv === "ERROR" || lv === "WARN") n++;
      var gfw = getFlat(arr[i].parsed);
      var sc = Number(gfw.statusCode);
      if (!isNaN(sc) && sc >= 400) n++;
      var msgW = String(gfw.msg || "").toLowerCase();
      if (msgW.indexOf("qdrant.http.") === 0 && gfw.http_status != null) {
        var hs = Number(gfw.http_status);
        if (!isNaN(hs) && hs !== 200) n++;
      }
    }
    return n;
  }

  function sliceRecent(arr, n) {
    if (!arr || !arr.length) return [];
    var take = Math.min(n, arr.length);
    return arr.slice(-take);
  }

  /** Latest supervised bootstrap line (indexer waiting for watch roots); drives service card idle pill + subtitle. */
  function indexerLatestSupervisedWaitFlat(entries) {
    var slice = sliceRecent(entries, RECENT_CARD_STATUS_N);
    for (var i = slice.length - 1; i >= 0; i--) {
      var f = getFlat(slice[i].parsed);
      if (String(f.service || "").toLowerCase() !== "indexer") continue;
      var typ = f.type != null ? String(f.type).trim() : "";
      if (typ === "indexer.supervised.wait_roots") return f;
      var m = indexerFlatMsg(f);
      if (m === "indexer.supervised.wait_roots") return f;
    }
    return null;
  }

  /** Card pill "error": ERROR level or HTTP status ≥400 (not WARN — avoids noisy strips). */
  function entryHasErrorStatus(ent) {
    var p = ent.parsed;
    if (!p) return false;
    if (p.levelCanon === "ERROR") return true;
    var fp = getFlat(p);
    var sc = Number(fp.statusCode);
    if (!isNaN(sc) && sc >= 400) return true;
    var msgQ = String(fp.msg || "").toLowerCase();
    if (msgQ.indexOf("qdrant.http.") === 0 && fp.http_status != null) {
      var hq = Number(fp.http_status);
      if (!isNaN(hq) && hq !== 200) return true;
    }
    return false;
  }

  function bifrostEntryHasRateLimit(ent) {
    if (globalThis.ClaudiaLogs && globalThis.ClaudiaLogs.Derive && globalThis.ClaudiaLogs.Derive.bifrostEntryHasRateLimit) {
      return globalThis.ClaudiaLogs.Derive.bifrostEntryHasRateLimit(ent, function (p) { return getFlat(p); });
    }
    var comb = (ent.text || "").toLowerCase() + String(getFlat(ent.parsed).msg || "").toLowerCase();
    return comb.indexOf("429") >= 0 || comb.indexOf("rate limit") >= 0 || comb.indexOf("rate_limit") >= 0;
  }

  function countErrorSignalsInEntries(arr) {
    var n = 0;
    for (var i = 0; i < arr.length; i++) {
      if (entryHasErrorStatus(arr[i])) n++;
    }
    return n;
  }

  function recentServiceCardHasError(name, arr) {
    var slice = sliceRecent(arr, RECENT_CARD_STATUS_N);
    for (var i = 0; i < slice.length; i++) {
      if (entryHasErrorStatus(slice[i])) return true;
      if (name === "bifrost" && bifrostEntryHasRateLimit(slice[i])) return true;
    }
    return false;
  }

  function recentConvEventsHaveError(events) {
    var slice = sliceRecent(events, RECENT_CARD_STATUS_N);
    for (var i = 0; i < slice.length; i++) {
      var p = slice[i].parsed;
      if (p.levelCanon === "ERROR") return true;
      var sc = Number(getFlat(p).statusCode);
      if (!isNaN(sc) && sc >= 400) return true;
    }
    return false;
  }

  /**
   * Bar segment colors / legend labels (keep in sync with inferServiceBadge; gateway HTTP lines also carry
   * flat.timeline_kind from the gateway — see internal/server/timeline_kind.go).
   */
  var TIMELINE_BAR_KINDS = [
    { key: "web", bg: "#42a5f5", label: "Web", title: "Inbound HTTP and API access lines" },
    { key: "qdrant", bg: "#66bb6a", label: "Qdrant", title: "Qdrant subprocess lines" },
    { key: "upstream", bg: "#9575cd", label: "Upstream", title: "BiFrost relay and upstream chat traffic" },
    { key: "indexer", bg: "#ffa726", label: "Indexer", title: "Indexer subprocess lines" },
    { key: "gateway", bg: "#78909c", label: "Gateway", title: "Gateway routing, startup, config, and other internal logs" }
  ];

  /** Shared with timelineBarHtml and indexer scope cards (same `.sum-timeline-bar` DOM). */
  function timelineSegmentsHtml(segments) {
    var html = '<div class="sum-timeline-bar">';
    for (var i = 0; i < segments.length; i++) {
      var pct = segments[i].pct;
      var bg = segments[i].bg;
      if (pct < 0.05) continue;
      html +=
        '<span class="sum-timeline-seg" style="width:' +
        Number(pct).toFixed(1) +
        "%;background:" +
        bg +
        '"></span>';
    }
    return html + "</div>";
  }

  function timelineBarHtml(evList) {
    var counts = { web: 0, qdrant: 0, upstream: 0, indexer: 0, gateway: 0 };
    for (var i = 0; i < evList.length; i++) {
      var lab = timelineKindLab(evList[i]);
      if (lab === "web") counts.web++;
      else if (lab === "qdrant") counts.qdrant++;
      else if (lab === "upstream") counts.upstream++;
      else if (lab === "indexer") counts.indexer++;
      else counts.gateway++;
    }
    var total = counts.web + counts.qdrant + counts.upstream + counts.indexer + counts.gateway || 1;
    var segments = [];
    for (var k = 0; k < TIMELINE_BAR_KINDS.length; k++) {
      var kind = TIMELINE_BAR_KINDS[k];
      var pct = (counts[kind.key] / total) * 100;
      if (pct < 0.05) continue;
      segments.push({ pct: pct, bg: kind.bg });
    }
    return timelineSegmentsHtml(segments);
  }

  /** Swatches for timelineBarHtml segment colors (gateway service panel). */
  function timelineLegendHtml() {
    var parts = [];
    for (var i = 0; i < TIMELINE_BAR_KINDS.length; i++) {
      var row = TIMELINE_BAR_KINDS[i];
      parts.push(
        '<span class="sum-timeline-legend-item" title="' +
          escapeHtml(row.title) +
          '">' +
          '<span class="sum-timeline-legend-swatch" style="background:' +
          row.bg +
          '"></span>' +
          '<span class="sum-timeline-legend-label">' +
          escapeHtml(row.label) +
          "</span></span>"
      );
    }
    return '<div class="sum-timeline-legend">' + parts.join("") + "</div>";
  }

  /**
   * Scope completion bar: indexer orange (#ffa726, same as timelineBarHtml) for all fills (no idle-green strip).
   * Uses the same `.sum-timeline-bar` structure as timelineBarHtml.
   */
  function indexerScopeProgressTimelineBarHtml(pRem, qTot, doneSeen) {
    var orange = "#ffa726";
    if (doneSeen) {
      return timelineSegmentsHtml([{ pct: 100, bg: orange }]);
    }
    if (pRem !== null && !isNaN(Number(pRem)) && Number(pRem) === 0 && qTot !== null && !isNaN(Number(qTot)) && Number(qTot) >= 0) {
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

  function serviceWindowMs(arr) {
    var t0 = null;
    var t1 = null;
    for (var i = 0; i < arr.length; i++) {
      var ins = entryInstant(arr[i]);
      if (ins) {
        if (!t0 || ins.getTime() < t0.getTime()) t0 = ins;
        if (!t1 || ins.getTime() > t1.getTime()) t1 = ins;
      }
    }
    return t0 && t1 ? t1.getTime() - t0.getTime() : 0;
  }

  function bifrostLastRelayRequestFlat(arr) {
    for (var i = arr.length - 1; i >= 0; i--) {
      var f = getFlat(arr[i].parsed);
      var msg = String(f.msg != null ? f.msg : f.message != null ? f.message : "").trim();
      if (msg === "chat.bifrost.request") return f;
    }
    return null;
  }

  /** One-line summary for collapsed bifrost card from a chat.bifrost.request row (no body excerpt / raw JSON). */
  function summarizeBifrostRelayRequest(f) {
    if (!f) return "";
    var model = bifrostShortModelLabel(f.upstreamModel != null ? String(f.upstreamModel).trim() : "—");
    var stream =
      f.stream === true || f.stream === "true"
        ? "SSE"
        : f.stream === false || f.stream === "false"
          ? "JSON"
          : "";
    var path = f.path != null && String(f.path).trim() !== "" ? String(f.path).trim() : "/v1/chat/completions";
    var ot = Number(f.outgoingTokens);
    var tokStr = !isNaN(ot) && ot > 0 ? formatInt(Math.round(ot)) + " tok out" : "";
    var bits = ["Last relay", "POST", path];
    if (model && model !== "—") bits.push(model);
    if (stream) bits.push(stream);
    if (tokStr) bits.push(tokStr);
    var line = bits.join(" · ");
    if (line.length > 220) line = line.slice(0, 217) + "…";
    return line;
  }

  /** Collapsed-card subtitle: prefer recent alerts, else last upstream relay request summary, else last response hint. */
  function bifrostCollapsedCardSubtitle(arr) {
    if (!arr.length) return "";
    var tailN = Math.min(12, arr.length);
    var t0 = Math.max(0, arr.length - tailN);
    var ti;
    for (ti = arr.length - 1; ti >= t0; ti--) {
      var f429 = getFlat(arr[ti].parsed);
      var m429 = String(f429.msg != null ? f429.msg : "").trim();
      if (m429 === "bifrost.rate_limit") {
        return "429 rate-limit (BiFrost HTTP)";
      }
    }
    for (ti = arr.length - 1; ti >= t0; ti--) {
      var raw = String(arr[ti].text || "");
      var m0 = String(getFlat(arr[ti].parsed).msg || "");
      var s = (raw + " " + m0).toLowerCase();
      if (s.indexOf("429") >= 0 || (s.indexOf("rate") >= 0 && s.indexOf("limit") >= 0)) {
        var retry = "5s";
        var rm = s.match(/retry[^\d]*(\d+)\s*s/);
        if (rm) retry = rm[1] + "s";
        var ra = s.match(/retry-after[:\s]+(\d+)/);
        if (ra) retry = ra[1] + "s";
        return "429 rate-limit — retry in " + retry;
      }
    }
    for (ti = arr.length - 1; ti >= t0; ti--) {
      var fe = getFlat(arr[ti].parsed);
      var merr = String(fe.msg != null ? fe.msg : "").trim();
      if (merr === "chat.bifrost.error" || merr.indexOf("bifrost.error") >= 0) {
        var es = String(fe.err != null ? fe.err : "").replace(/\s+/g, " ").trim();
        if (!es && arr[ti].text) {
          try {
            var jo = tryParseJSONObject(arr[ti].text);
            if (jo && jo.err != null && jo.err !== "") {
              es = typeof jo.err === "object" ? JSON.stringify(jo.err).slice(0, 120) : String(jo.err);
            }
          } catch (x) { }
        }
        if (es.length > 140) es = es.slice(0, 138) + "…";
        return "Upstream fetch failed" + (es ? ": " + es : "");
      }
    }
    for (ti = arr.length - 1; ti >= t0; ti--) {
      var fh = getFlat(arr[ti].parsed);
      var mh = String(fh.msg || "").trim();
      if (mh === "bifrost.provider.health.fail") {
        var pdn = fh.provider_id != null ? String(fh.provider_id).trim() : "";
        return "Provider health down" + (pdn ? ": " + pdn : "");
      }
      if (mh === "bifrost.provider.key_missing") {
        var pk = fh.provider_id != null ? String(fh.provider_id).trim() : "";
        return "Missing key" + (pk ? " for " + pk : "");
      }
    }
    var reqF = bifrostLastRelayRequestFlat(arr);
    if (reqF) return summarizeBifrostRelayRequest(reqF);
    for (var rj = arr.length - 1; rj >= 0; rj--) {
      var fr = getFlat(arr[rj].parsed);
      var mr = String(fr.msg || "").trim();
      if (mr === "upstream chat response" || mr === "chat.bifrost.response") {
        var sc = fr.statusCode != null && fr.statusCode !== "" ? String(fr.statusCode) : "—";
        var ut = Number(fr.usageTotalTokens);
        var up = Number(fr.usagePromptTokens);
        var uc = Number(fr.usageCompletionTokens);
        var uTot = !isNaN(ut) && ut > 0 ? ut : (!isNaN(up) || !isNaN(uc) ? (isNaN(up) ? 0 : up) + (isNaN(uc) ? 0 : uc) : 0);
        var uS = uTot > 0 ? formatInt(Math.round(uTot)) + " tok usage" : "";
        var mod = bifrostShortModelLabel(fr.upstreamModel != null ? String(fr.upstreamModel).trim() : "—");
        var bits = ["Last response", "HTTP " + sc];
        if (mod && mod !== "—") bits.push(mod);
        if (uS) bits.push(uS);
        return bits.join(" · ");
      }
    }
    return "Idle — no chat calls relayed yet";
  }

  /** Aggregate metrics for the bifrost service card from gateway upstream relay / response logs. */
  function bifrostCardMetrics(arr) {
    if (globalThis.ClaudiaLogs && globalThis.ClaudiaLogs.Derive && globalThis.ClaudiaLogs.Derive.bifrostCardMetrics) {
      return globalThis.ClaudiaLogs.Derive.bifrostCardMetrics(arr, function (p) { return getFlat(p); });
    }
    return { reqN: 0, resN: 0, errN: 0, streamOn: 0, streamOff: 0, outgoingSum: 0, usageSum: 0, bytesSum: 0, sc2xx: 0, scErr: 0, topModel: "—", rlN: 0, relayOk: 0, relayFail: 0, rateLimitSlugN: 0, relay429N: 0, rateLimitBoxN: 0, fallbackN: 0, providersTotal: 0, providersUp: 0, providersAnyDown: false };
  }

  function bifrostProviderHealthResolve(arr) {
    var stateColor = { up: "#66bb6a", down: "#ef5350", key_missing: "#ffa726", unknown: "#bdbdbd" };
    var stateLabel = { up: "up", down: "down", key_missing: "key missing", unknown: "unknown" };
    var list = null;
    var liveErr = "";
    if (bifrostProviderSnapshot && bifrostProviderSnapshot.data && Array.isArray(bifrostProviderSnapshot.data.providers)) {
      var snapshotAgeMs = Date.now() - Number(bifrostProviderSnapshot.fetchedClientMs || 0);
      if (snapshotAgeMs <= BIFROST_PROVIDER_STALE_MS) {
        list = bifrostProviderSnapshot.data.providers.slice();
        liveErr = String(bifrostProviderSnapshot.data.error || "").trim();
      }
    }
    if (!list && globalThis.ClaudiaLogs && ClaudiaLogs.Derive && typeof ClaudiaLogs.Derive.bifrostProviderHealthList === "function") {
      list = ClaudiaLogs.Derive.bifrostProviderHealthList(arr, function (p) { return getFlat(p); });
    }
    return {
      list: list,
      liveErr: liveErr,
      emptyMsg: liveErr ? "BiFrost unreachable" : "No providers loaded yet",
      stateColor: stateColor,
      stateLabel: stateLabel
    };
  }

  function bifrostProviderHealthSegTitle(entry, lab) {
    var titleBits = [String((entry || {}).id || "—") + " · " + lab];
    var keyHint = entry.key_hint != null ? String(entry.key_hint) : "";
    var keyCount = entry.key_count != null && !isNaN(Number(entry.key_count)) ? Number(entry.key_count) : null;
    if (keyCount != null) titleBits.push(keyCount + (keyCount === 1 ? " key" : " keys"));
    if (keyHint) titleBits.push(keyHint);
    if (entry.ollama_base_url) titleBits.push("base " + entry.ollama_base_url);
    if (entry.error) titleBits.push("err: " + entry.error);
    return titleBits.join(" · ");
  }

  /**
   * Provider-health strip: one segment per configured provider, colored by latest probe.
   * Visually aligned with conversation lifecycle bars (gapped segments; outer corners rounded).
   *
   * Source preference (high → low):
   *   1. Live snapshot from /api/ui/bifrost/providers (refreshed every 30s) — authoritative
   *      because BiFrost (this build) doesn't slog per-provider lifecycle events, so the log
   *      buffer alone can't enumerate groq / gemini / ollama.
   *   2. Log-derived list via `ClaudiaLogs.Derive.bifrostProviderHealthList` — fallback when
   *      the live snapshot is missing or stale (>90s) so an offline view still has something.
   *   3. Empty caption ("No providers loaded yet" / "BiFrost unreachable") when neither source
   *      yields entries.
   *
   * opts.compact: collapsed BiFrost service card — up to three gapped indicators, no labels.
   */
  function bifrostProviderHealthStripHtml(arr, opts) {
    opts = opts || {};
    var compact = !!opts.compact;
    var R = bifrostProviderHealthResolve(arr);
    var list = R.list;
    var stateColor = R.stateColor;
    var stateLabel = R.stateLabel;

    if (compact) {
      var trackTitle = R.emptyMsg;
      var segs = [];
      if (list && list.length) {
        var cap = list.length > 3 ? 3 : list.length;
        trackTitle =
          list.length > 3
            ? "Provider probe status (first " + cap + " of " + list.length + ")"
            : "Provider probe status";
        for (var ci = 0; ci < cap; ci++) {
          var entC = list[ci] || {};
          var stC = stateColor[entC.state] ? entC.state : "unknown";
          var labC = stateLabel[stC];
          segs.push(
            '<span class="sum-bf-prov-health-seg" title="' +
              escapeHtml(bifrostProviderHealthSegTitle(entC, labC)) +
              '" style="background:' +
              stateColor[stC] +
              '"></span>'
          );
        }
      } else {
        for (var zi = 0; zi < 3; zi++) {
          segs.push(
            '<span class="sum-bf-prov-health-seg" title="' +
              escapeHtml(R.emptyMsg) +
              '" style="background:' +
              stateColor.unknown +
              '"></span>'
          );
        }
      }
      return (
        '<div id="bifrost-provider-health-compact" class="sum-bf-prov-health-root sum-bf-prov-health-root--compact" role="img" aria-label="' +
        escapeHtml(trackTitle) +
        '">' +
        '<div class="sum-bf-prov-health-track sum-bf-prov-health-track--compact" title="' +
        escapeHtml(trackTitle) +
        '">' +
        segs.join("") +
        "</div></div>"
      );
    }

    var rootOpen = '<div id="bifrost-provider-health-strip" class="sum-bf-prov-health-root">';
    if (!list || !list.length) {
      return (
        rootOpen +
        '<div class="sum-bf-prov-health-track sum-bf-prov-health-track--empty" title="' +
        escapeHtml(R.emptyMsg) +
        '">' +
        '<span class="sum-bf-prov-health-seg sum-bf-prov-health-seg--empty" title="' +
        escapeHtml(R.emptyMsg) +
        '" style="background:' +
        stateColor.unknown +
        '"></span></div>' +
        '<div class="sum-strip-caption sum-strip-caption--muted">' +
        escapeHtml(R.emptyMsg) +
        "</div></div>"
      );
    }
    var trackParts = [];
    var labelParts = [];
    for (var i = 0; i < list.length; i++) {
      var entry = list[i] || {};
      var st = stateColor[entry.state] ? entry.state : "unknown";
      var bg = stateColor[st];
      var lab = stateLabel[st];
      trackParts.push(
        '<span class="sum-bf-prov-health-seg" title="' +
          escapeHtml(bifrostProviderHealthSegTitle(entry, lab)) +
          '" style="background:' +
          bg +
          '"></span>'
      );
      labelParts.push(
        '<span class="sum-bf-prov-health-label" title="' +
          escapeHtml(bifrostProviderHealthSegTitle(entry, lab)) +
          '">' +
          escapeHtml(String(entry.id || "—")) +
          "</span>"
      );
    }
    return (
      rootOpen +
      '<div class="sum-bf-prov-health-track" title="One segment per configured provider, colored by latest health probe">' +
      trackParts.join("") +
      '</div><div class="sum-bf-prov-health-labels">' +
      labelParts.join("") +
      "</div></div>"
    );
  }

  /**
   * Relay-outcome strip: buckets every chat relay row in the buffer by HTTP outcome.
   * Replaces the legacy generic "Request timeline" mix bar on the BiFrost panel
   * (which was always 100% purple because every BiFrost row maps to "upstream").
   * Backed by `ClaudiaLogs.Derive.bifrostRelayOutcomeBuckets`.
   */
  function bifrostRelayOutcomeStripHtml(arr) {
    var b = null;
    if (
      globalThis.ClaudiaLogs &&
      ClaudiaLogs.Derive &&
      typeof ClaudiaLogs.Derive.bifrostRelayOutcomeBuckets === "function"
    ) {
      b = ClaudiaLogs.Derive.bifrostRelayOutcomeBuckets(arr, function (p) { return getFlat(p); });
    }
    if (!b || !b.total) {
      return (
        '<div class="sum-timeline-bar sum-timeline-bar--relay-outcome"></div>' +
        '<div class="sum-strip-caption sum-strip-caption--muted">No chat relay activity yet</div>'
      );
    }
    var palette = [
      { key: "ok", label: "2xx", color: "#66bb6a" },
      { key: "redirect", label: "3xx", color: "#42a5f5" },
      { key: "rateLimit", label: "429", color: "#fb8c00" },
      { key: "clientErr", label: "4xx", color: "#ffa726" },
      { key: "serverErr", label: "5xx", color: "#ef5350" },
      { key: "errorNoResp", label: "fetch err", color: "#c62828" },
      { key: "inFlight", label: "in flight", color: "#9575cd" }
    ];
    var html = '<div class="sum-timeline-bar sum-timeline-bar--relay-outcome" title="Chat relay outcomes since last BiFrost ready (HTTP buckets + fetch errors + in-flight)">';
    var captionParts = [];
    for (var i = 0; i < palette.length; i++) {
      var p = palette[i];
      var n = Number(b[p.key] || 0);
      if (n <= 0) continue;
      var pct = (n / b.total) * 100;
      if (pct < 0.05) pct = 0.05;
      html +=
        '<span class="sum-timeline-seg" title="' +
        escapeHtml(p.label + " · " + n) +
        '" style="width:' +
        pct.toFixed(2) +
        "%;background:" +
        p.color +
        '"></span>';
      captionParts.push(
        formatInt(n) +
        ' <span class="sum-strip-caption-state sum-strip-caption-state--' +
        p.key +
        '">' +
        escapeHtml(p.label) +
        "</span>"
      );
    }
    html += "</div>";
    html += '<div class="sum-strip-caption">' + captionParts.join(" · ") + "</div>";
    return html;
  }

  function bifrostShortModelLabel(model) {
    if (!model || model === "—") return "—";
    var parts = String(model).split("/");
    var tail = parts[parts.length - 1] || model;
    if (tail.length > 36) return tail.slice(0, 34) + "…";
    return tail;
  }

  function badgeForServicePanel(name, ev) {
    if (name === "bifrost") {
      var w = { parsed: ev.parsed, text: ev.text, ts: ev.ts, source: ev.source };
      if (entryIsGatewayUpstreamRelay(w)) {
        return { cls: "sum-svc-upstream sum-svc-badge-filled sum-svc-upstream-filled", lab: "upstream" };
      }
      return null;
    }
    return inferServiceBadge(ev);
  }

  /** How long file-level indexer activity stays “fresh” for UI subtitle hints. */
  var INDEXER_IDLE_RECENCY_MS = 120000;

  /** Human label for indexer.state code — canonical mapping lives in derive/indexerPresent.js (goja-tested). */
  function indexerHumanDeclaredState(code) {
    if (
      globalThis.ClaudiaLogs &&
      ClaudiaLogs.Derive &&
      typeof ClaudiaLogs.Derive.indexerDeclaredStateLabel === "function"
    ) {
      return ClaudiaLogs.Derive.indexerDeclaredStateLabel(code);
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

  var INDEXER_HIST_COLS = {
    lifecycle: "#5c6bc0",
    discovery: "#7e57c2",
    jobs: "#fb8c00",
    queue: "#29b6f6",
    statestats: "#26a69a",
    config: "#78909c",
    recovery: "#ef5350",
    indexer_misc: "#9e9e9e",
    other: "#bdbdbd"
  };

  function indexerEventMixHistogramHtml(evs) {
    var counts = {
      lifecycle: 0,
      discovery: 0,
      jobs: 0,
      queue: 0,
      statestats: 0,
      config: 0,
      recovery: 0,
      indexer_misc: 0,
      other: 0
    };
    var bucketFn =
      globalThis.ClaudiaLogs &&
        ClaudiaLogs.Derive &&
        typeof ClaudiaLogs.Derive.indexerSlugHistogramBucket === "function"
        ? function (msg) {
          return ClaudiaLogs.Derive.indexerSlugHistogramBucket(msg);
        }
        : function () {
          return "other";
        };
    for (var i = 0; i < evs.length; i++) {
      var m = indexerFlatMsg(getFlat(evs[i].parsed));
      var b = bucketFn(m);
      if (counts[b] === undefined) counts.other++;
      else counts[b]++;
    }
    var total = evs.length || 1;
    var order = [
      "jobs",
      "queue",
      "statestats",
      "discovery",
      "lifecycle",
      "recovery",
      "config",
      "indexer_misc",
      "other"
    ];
    var html = '<div class="sum-timeline-bar indexer-event-mix-bar" title="Share of loaded log lines by indexer message category">';
    for (var o = 0; o < order.length; o++) {
      var key = order[o];
      var c = counts[key] || 0;
      var pct = (c / total) * 100;
      if (pct < 0.05) continue;
      html +=
        '<span class="sum-timeline-seg" style="width:' +
        pct.toFixed(1) +
        "%;background:" +
        (INDEXER_HIST_COLS[key] || INDEXER_HIST_COLS.other) +
        '"></span>';
    }
    return html + "</div>";
  }

  function indexerHistogramLegendHtml() {
    var order = [
      ["jobs", "file jobs"],
      ["queue", "queue snapshots"],
      ["statestats", "state / Qdrant stats"],
      ["discovery", "discovery / inventory"],
      ["lifecycle", "run start · done"],
      ["recovery", "retry / recovery"],
      ["config", "gateway config"],
      ["indexer_misc", "other indexer"],
      ["other", "other lines"]
    ];
    var parts = [];
    for (var o = 0; o < order.length; o++) {
      var k = order[o][0];
      var lab = order[o][1];
      var col = INDEXER_HIST_COLS[k] || INDEXER_HIST_COLS.other;
      parts.push(
        '<span class="indexer-mix-legend-item"><span class="indexer-mix-swatch" style="background:' +
        col +
        '"></span>' +
        escapeHtml(lab) +
        "</span>"
      );
    }
    return '<div class="indexer-mix-legend">' + parts.join("") + "</div>";
  }

  function badgeForIndexerRunLine(ent) {
    var src = (ent.source || "").toLowerCase();
    var f = getFlat(ent.parsed);
    var msg = String(f.msg || "").toLowerCase();
    if (src === "qdrant" || msg.indexOf("qdrant") >= 0)
      return { cls: "sum-svc-qdrant sum-svc-badge-filled sum-svc-qdrant-filled", lab: "qdrant" };
    return { cls: "sum-svc-indexer sum-svc-badge-filled sum-svc-indexer-filled", lab: "indexer" };
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
      return "Indexer uploading batch — " + cur + " of " + tot + " chunks embedded";
    if (candStr && candStr !== "—")
      return "Indexer uploading batch — latest counters: " + candStr + " candidates / chunks";
    return doneSeen ? "Indexer run completed" : "Indexer uploading batch — in progress";
  }

  /** Primary log `msg` / `message` (slog may put the human title in one and the slug in the other, or duplicate keys). */
  function indexerFlatMsg(fl) {
    if (
      globalThis.ClaudiaLogs &&
      ClaudiaLogs.Derive &&
      typeof ClaudiaLogs.Derive.indexerFlatMsgForPresent === "function"
    )
      return ClaudiaLogs.Derive.indexerFlatMsgForPresent(fl);
    return String(fl.msg != null ? fl.msg : fl.message != null ? fl.message : "")
      .toLowerCase()
      .trim();
  }

  /** True when flat is an indexer.state snapshot (slug or human title / structured fields). */
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
  function latestIndexerStateQueueInflightFromEntries(arr) {
    var qd = null,
      inf = null;
    if (!Array.isArray(arr)) return { queueDepth: qd, ingestInflight: inf };
    for (var i = arr.length - 1; i >= 0; i--) {
      var f = getFlat(arr[i].parsed);
      if (!isIndexerStateFlat(f)) continue;
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

  /** Latest queue_cap and workers from the newest indexer.queue.snapshot in the log window. */
  function latestIndexerQueueSnapshotMetaFromEntries(arr) {
    var cap = null;
    var workers = null;
    if (!Array.isArray(arr)) return { queueCap: cap, workers: workers };
    for (var i = arr.length - 1; i >= 0; i--) {
      var f = getFlat(arr[i].parsed);
      var m = indexerFlatMsg(f);
      if (m !== "indexer.queue.snapshot" && m.indexOf("indexer.queue.snapshot") !== 0) continue;
      if (f.queue_cap != null && f.queue_cap !== "") {
        var c = Number(f.queue_cap);
        if (!isNaN(c)) cap = c;
      }
      if (f.workers != null && f.workers !== "") {
        var w = Number(f.workers);
        if (!isNaN(w)) workers = w;
      }
      break;
    }
    return { queueCap: cap, workers: workers };
  }

  /** Per-file / operator slugs for expanded indexer Summary (service + run cards). */
  function indexerStructuredRollupCounts(entries) {
    var upload = 0,
      ingested = 0,
      skipped = 0,
      failed = 0,
      retry = 0,
      paused = 0,
      snapshots = 0;
    var relSet = {};
    var workers = null;
    var queueDepth = null;
    for (var i = 0; i < entries.length; i++) {
      var f = getFlat(entries[i].parsed);
      var m = indexerFlatMsg(f);
      if (m === "indexer.job.upload") {
        upload++;
        if (f.rel) relSet[String(f.rel)] = 1;
      } else if (m === "indexer.job.ingested" || m === "ingested") {
        ingested++;
        if (f.rel) relSet[String(f.rel)] = 1;
      } else if (m === "indexer.job.skipped") {
        skipped++;
        if (f.rel) relSet[String(f.rel)] = 1;
      } else if (m.indexOf("indexer.job.failed") === 0) failed++;
      else if (m.indexOf("indexer.retry") === 0) retry++;
      else if (m.indexOf("indexer.worker.paused") === 0) paused++;
      else if (m.indexOf("indexer.queue.snapshot") === 0) snapshots++;
    }
    for (var j = entries.length - 1; j >= 0; j--) {
      var fj = getFlat(entries[j].parsed);
      var mj = indexerFlatMsg(fj);
      if (mj.indexOf("indexer.queue.snapshot") === 0) {
        if (fj.workers != null && fj.workers !== "") workers = Number(fj.workers);
        if (fj.queue_depth != null && fj.queue_depth !== "") queueDepth = Number(fj.queue_depth);
        break;
      }
    }
    return {
      upload: upload,
      ingested: ingested,
      skipped: skipped,
      failed: failed,
      retry: retry,
      paused: paused,
      snapshots: snapshots,
      uniqRel: Object.keys(relSet).length,
      workers: workers,
      queueDepth: queueDepth
    };
  }

  function indexerStructuredRollupMiniHtml(entries) {
    var r = indexerStructuredRollupCounts(entries);
    var sk = formatInt(r.skipped);
    var ig = formatInt(r.ingested);
    var up = formatInt(r.upload);
    var rt = formatInt(r.retry);
    var fa = formatInt(r.failed);
    var pu = formatInt(r.paused);
    return (
      '<div class="sum-mini-row">' +
      '<div class="sum-mini-card">Skipped (before upload)<strong>' +
      escapeHtml(sk) +
      '</strong></div>' +
      '<div class="sum-mini-card">Successfully ingested<strong>' +
      escapeHtml(ig) +
      '</strong></div>' +
      '<div class="sum-mini-card">Started upload<strong>' +
      escapeHtml(up) +
      "</strong></div></div>" +
      '<div class="sum-mini-row">' +
      '<div class="sum-mini-card">Retries<strong>' +
      escapeHtml(rt) +
      '</strong></div>' +
      '<div class="sum-mini-card">Failed<strong>' +
      escapeHtml(fa) +
      '</strong></div>' +
      '<div class="sum-mini-card">Worker pauses<strong>' +
      escapeHtml(pu) +
      "</strong></div></div>"
    );
  }

  /** Caption under aggregate indexer progress bar (sums scope rows across partitioned indexer cards). */
  function indexerAggregateBacklogCaption(sumRem, sumTot) {
    if (sumRem !== null && !isNaN(Number(sumRem)) && sumTot !== null && !isNaN(Number(sumTot)))
      return (
        formatInt(Math.round(Number(sumRem))) +
        " remaining of " +
        formatInt(Math.round(Number(sumTot))) +
        " total"
      );
    if (sumRem !== null && !isNaN(Number(sumRem)))
      return formatInt(Math.round(Number(sumRem))) + " remaining of — total";
    if (sumTot !== null && !isNaN(Number(sumTot)))
      return "— remaining of " + formatInt(Math.round(Number(sumTot))) + " total";
    return "";
  }

  /** Explains aggregate scope bar while buffers warm up vs when metrics are live. */
  function indexerAggregateProgressDetailText(agg) {
    if (!agg || !agg.anyRun) {
      return "No indexer scopes in the loaded log window — scroll for older lines or wait for indexer traffic.";
    }
    var hasRem = agg.sumRem !== null && !isNaN(Number(agg.sumRem));
    var hasTot = agg.sumTot !== null && !isNaN(Number(agg.sumTot));
    if (!hasRem && !hasTot && !agg.allDone) {
      return "Waiting for indexer.scope.status heartbeats with queue and workspace file totals…";
    }
    if (agg.allDone) {
      return "Tracked runs in view have finished; bar reflects combined scope status when present.";
    }
    if (hasRem && Number(agg.sumRem) === 0 && hasTot) {
      return "No pending ingest or fan-out rows for scopes in view — totals match workspace file counts.";
    }
    return "Enqueued + fan-out backlog vs total workspace files.";
  }

  /**
   * Sum pending ingest + fan-out rows and workspace file totals across indexer partition buckets
   * (same metadata as each per-scope indexer card).
   */
  function rollupIndexerAggregateScopeProgress(byRun, partitionRegistry) {
    var sumRem = 0;
    var sumTot = 0;
    var anyRem = false;
    var anyTot = false;
    var anyRun = false;
    var allDone = true;
    if (!byRun || typeof byRun !== "object") {
      return { sumRem: null, sumTot: null, allDone: false, anyRun: false };
    }
    var keys = Object.keys(byRun);
    for (var i = 0; i < keys.length; i++) {
      var run = byRun[keys[i]];
      if (!run || !run.events || !run.events.length) continue;
      anyRun = true;
      var pmeta = null;
      if (
        partitionRegistry &&
        globalThis.ClaudiaLogs &&
        ClaudiaLogs.Derive &&
        typeof ClaudiaLogs.Derive.indexerPartitionMetaForRun === "function"
      ) {
        pmeta = ClaudiaLogs.Derive.indexerPartitionMetaForRun(
          partitionRegistry,
          run.id,
          run.events,
          getFlat
        );
      }
      var meta = collectIndexerRunMeta(run.id, run.events, pmeta);
      meta = mergePersistedIndexerWatchRoots(meta, run.events, run.id);
      if (!meta.doneSeen) allDone = false;
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
      if (pRem !== null) {
        sumRem += pRem;
        anyRem = true;
      }
      if (qTot !== null) {
        sumTot += qTot;
        anyTot = true;
      }
    }
    return {
      sumRem: anyRem ? sumRem : null,
      sumTot: anyTot ? sumTot : null,
      allDone: anyRun && allDone,
      anyRun: anyRun
    };
  }

  function gatewayServicePanelMiniHtml(arr) {
    var M = {
      kv: {
        listening: "—",
        upstream: "—",
        config: "—",
        apiKeys: "—",
        apiKeysTint: "none",
        routingRules: "—",
        supervised: "—"
      },
      counters: {
        http2xx: 0,
        httpNot2xx: 0,
        http429: 0,
        chatReq: 0,
        chatResp: 0,
        chatErr: 0,
        ragQuery: 0,
        ragHit: 0,
        ragRetrieveErr: "",
        ingestOk: 0,
        ingestFail: 0
      }
    };
    if (globalThis.ClaudiaLogs && ClaudiaLogs.Derive && typeof ClaudiaLogs.Derive.gatewayCardModel === "function") {
      M = ClaudiaLogs.Derive.gatewayCardModel(arr, getFlat);
    }
    var kv = M.kv || {};
    var c = M.counters || {};
    var apiKeysOpen =
      kv.apiKeysTint === "error"
        ? '<dd class="gateway-kv-dd gateway-kv-dd--error">'
        : "<dd>";
    var ragSub = c.ragRetrieveErr
      ? c.ragRetrieveErr
      : "search lines vs per-hit vectors";
    var httpSub =
      c.http429 > 0
        ? formatInt(c.http2xx) +
          " ok · " +
          formatInt(c.httpNot2xx) +
          " fail · " +
          formatInt(c.http429) +
          "×429"
        : formatInt(c.http2xx) + " ok · " + formatInt(c.httpNot2xx) + " fail";
    return (
      '<dl class="indexer-run-kv indexer-run-kv--gateway-summary">' +
      "<dt>listening</dt><dd>" +
      escapeHtml(kv.listening || "—") +
      '</dd><dt>upstream</dt><dd>' +
      escapeHtml(kv.upstream || "—") +
      '</dd><dt>config</dt><dd>' +
      escapeHtml(kv.config || "—") +
      "</dd><dt>API keys</dt>" +
      apiKeysOpen +
      escapeHtml(kv.apiKeys || "—") +
      '</dd><dt>routing rules</dt><dd>' +
      escapeHtml(kv.routingRules || "—") +
      '</dd><dt>supervised</dt><dd>' +
      escapeHtml(kv.supervised || "—") +
      "</dd></dl>" +
      '<div class="gw-panel-timeline">' +
      '<div class="sum-section-label">Request timeline</div>' +
      '<p class="sum-timeline-caption muted">Segment width is the share of lines in this view for each kind.</p>' +
      timelineBarHtml(arr) +
      timelineLegendHtml() +
      "</div>" +
      '<div class="sum-mini-row">' +
      '<div class="sum-mini-card">HTTP (ok / fail)<strong>' +
      escapeHtml(formatInt(c.http2xx) + " / " + formatInt(c.httpNot2xx)) +
      '</strong><span class="sum-mini-sub">' +
      escapeHtml(httpSub) +
      '</span></div>' +
      '<div class="sum-mini-card">Chat (req → resp)<strong>' +
      escapeHtml(formatInt(c.chatReq) + " → " + formatInt(c.chatResp)) +
      '</strong><span class="sum-mini-sub">' +
      escapeHtml(formatInt(c.chatErr) + " relay errors") +
      '</span></div>' +
      '<div class="sum-mini-card">RAG (queries · hits)<strong>' +
      escapeHtml(formatInt(c.ragQuery) + " · " + formatInt(c.ragHit)) +
      '</strong><span class="sum-mini-sub">' +
      escapeHtml(ragSub) +
      '</span></div>' +
      '<div class="sum-mini-card">Ingest (ok / fail)<strong>' +
      escapeHtml(formatInt(c.ingestOk) + " / " + formatInt(c.ingestFail)) +
      '</strong><span class="sum-mini-sub">complete vs failed + chunked errors</span></div></div>'
    );
  }

  /** Gateway DEBUG traces for RAG (counts entire buffer — same window as service cards). */
  function rollupGatewayRagPipeline() {
    if (globalThis.ClaudiaLogs && globalThis.ClaudiaLogs.Derive && globalThis.ClaudiaLogs.Derive.rollupGatewayRagPipeline) {
      return globalThis.ClaudiaLogs.Derive.rollupGatewayRagPipeline(entryCache, function (p) { return getFlat(p); });
    }
    return { ragQuery: 0, ragEmbed: 0, ragHitLines: 0, embedMsSum: 0 };
  }

  /** Classify Qdrant REST traffic from structured http.access rows (subprocess buffer). */
  function qdrantHttpPathRollup(arr) {
    if (globalThis.ClaudiaLogs && globalThis.ClaudiaLogs.Derive && globalThis.ClaudiaLogs.Derive.qdrantHttpPathRollup) {
      return globalThis.ClaudiaLogs.Derive.qdrantHttpPathRollup(arr, function (p) { return getFlat(p); });
    }
    return { searchN: 0, upsertN: 0, scrollN: 0 };
  }

  function qdrantServicePanelMiniHtml(arr) {
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
    if (globalThis.ClaudiaLogs && ClaudiaLogs.Derive && typeof ClaudiaLogs.Derive.qdrantCardModel === "function") {
      M = ClaudiaLogs.Derive.qdrantCardModel(arr, getFlat, qdrantCollectionScopeLabelForLogs);
    }
    var ports = "—";
    if (M.restPort != null && M.grpcPort != null) ports = String(M.restPort) + " / " + String(M.grpcPort);
    else if (M.restPort != null) ports = String(M.restPort) + " / —";
    else if (M.grpcPort != null) ports = "— / " + String(M.grpcPort);
    var kv =
      '<dl class="indexer-run-kv indexer-run-kv--qdrant-summary">' +
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

  function bifrostServicePanelKvHtml(arr) {
    var M = {
      version: "—",
      configuration: "—",
      port: "—",
      auth: "—",
      mcp: "—",
      governance: "—",
      lastModel: "—"
    };
    if (globalThis.ClaudiaLogs && ClaudiaLogs.Derive && typeof ClaudiaLogs.Derive.bifrostCardModel === "function") {
      var d = ClaudiaLogs.Derive.bifrostCardModel(arr, function (p) { return getFlat(p); });
      if (d.version) M.version = d.version;
      if (d.configuration) M.configuration = d.configuration;
      if (d.port) M.port = d.port;
      if (d.auth) M.auth = d.auth;
      if (d.mcp) M.mcp = d.mcp;
      if (d.governance) M.governance = d.governance;
      if (d.lastModel) M.lastModel = bifrostShortModelLabel(d.lastModel);
    }
    return (
      '<dl class="indexer-run-kv indexer-run-kv--bifrost-summary">' +
      "<dt>version</dt><dd>" +
      escapeHtml(M.version) +
      '</dd><dt>configuration</dt><dd>' +
      escapeHtml(M.configuration) +
      '</dd><dt>port</dt><dd>' +
      escapeHtml(M.port) +
      '</dd><dt>auth</dt><dd>' +
      escapeHtml(M.auth) +
      '</dd><dt>MCP</dt><dd>' +
      escapeHtml(M.mcp) +
      '</dd><dt>governance</dt><dd>' +
      escapeHtml(M.governance) +
      '</dd><dt>last model</dt><dd>' +
      escapeHtml(M.lastModel) +
      "</dd></dl>"
    );
  }

  function flatLooksLikeIndexerRunStart(fl) {
    var m = indexerFlatMsg(fl);
    if (m === "indexer.run.start" || m === "indexer run start") return true;
    if (String(fl.service || "").toLowerCase() !== "indexer") return false;
    return fl.root_ids != null && (fl.roots != null || Array.isArray(fl.watch_root_paths));
  }

  function flatLooksLikeIndexerRunDone(fl) {
    var m = indexerFlatMsg(fl);
    if (m.indexOf("indexer.run.done") === 0) return true;
    if (m === "indexer run done" || m === "indexer run stopped") return true;
    return (
      String(fl.service || "").toLowerCase() === "indexer" &&
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
    if (String(fl.service || "").toLowerCase() !== "indexer") return false;
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
      var err = f && (f.err != null ? f.err : f.error != null ? f.error : "");
      var es = err != null ? String(err).replace(/\s+/g, " ").trim() : "";
      if (es.length > 80) es = es.slice(0, 78) + "…";
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
    var want = maxItems != null && !isNaN(Number(maxItems)) ? Math.max(3, Math.min(60, Math.round(Number(maxItems)))) : 18;
    for (var i = evs.length - 1; i >= 0; i--) {
      var f = getFlat(evs[i].parsed);
      var st = indexerRecentEvalStatusForFlat(f);
      if (!st) continue;
      if (seen[st.rel]) continue;
      seen[st.rel] = true;
      var t = formatLogDateTimeLocal(evs[i].ts);
      rows.push({
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
      '<div class="sum-full-log indexer-recent-files" id="' + escapeHtml(sid) + '"><ul>';
    if (!rows.length) {
      html += '<li class="sum-ev-item muted">No file-level activity in the loaded window yet. Scroll up to load older lines.</li>';
    } else {
      for (var r = 0; r < rows.length; r++) {
        var it = rows[r];
        var lvlClass = "lvl-INFO";
        if (it.st === "failed") lvlClass = "lvl-ERROR";
        else if (it.st === "retrying" || it.st === "skipped") lvlClass = "lvl-WARN";
        else if (it.st === "evaluating" || it.st === "uploading") lvlClass = "lvl-DEBUG";
        else if (it.st === "retrieved") lvlClass = "lvl-INFO";
        html +=
          '<li class="sum-ev-item indexer-recent-row">' +
          '<span class="muted indexer-recent-time">' +
          escapeHtml(it.t) +
          "</span>" +
          '<span class="log-line-sum__lvl indexer-recent-op ' +
          escapeHtml(lvlClass) +
          '">' +
          escapeHtml(it.st) +
          "</span>" +
          '<code class="sum-mono-id indexer-recent-path">' +
          escapeHtml(it.rel) +
          "</code>" +
          (it.detail
            ? '<span class="muted indexer-recent-detail" title="' +
            escapeHtml(it.detail) +
            '">' +
            escapeHtml(it.detail) +
            "</span>"
            : '<span class="muted indexer-recent-detail"></span>') +
          "</li>";
      }
    }
    html += "</ul></div>";
    return html;
  }

  /** Rolls up indexer.run.start / progress / done / job lines for summarized cards. */
  function collectIndexerRunMeta(runId, evs, partitionMeta) {
    if (globalThis.ClaudiaLogs && globalThis.ClaudiaLogs.Derive && globalThis.ClaudiaLogs.Derive.collectIndexerRunMeta) {
      return globalThis.ClaudiaLogs.Derive.collectIndexerRunMeta(runId, evs, {
        getFlat: function (p) { return getFlat(p); },
        tokenLabelByTenant: tokenLabelByTenant,
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

    var userLab = tenantId ? tokenLabelByTenant[tenantId] || tenantId : "—";

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

  function convWindowMs(g) {
    var t0 = null;
    var t1 = null;
    for (var ti = 0; ti < g.events.length; ti++) {
      var ins = entryInstant({ ts: g.events[ti].ts });
      if (ins) {
        if (!t0 || ins.getTime() < t0.getTime()) t0 = ins;
        if (!t1 || ins.getTime() > t1.getTime()) t1 = ins;
      }
    }
    return t0 && t1 ? t1.getTime() - t0.getTime() : 0;
  }

  function renderExpandedConv(g) {
    var evs = g.events;
    var cardModel = conversationCardModelForGroup(evs);
    var ingestCount = 0;
    var ig;
    for (ig = 0; ig < evs.length; ig++) {
      if (evs[ig].convJoinTier === "ingest") ingestCount++;
    }
    var bar = timelineBarHtml(evs);
    var spanMs = convWindowMs({ events: evs });
    var met = scrapeConversationMetrics(evs);
    var tokLine = met.tok != null ? formatInt(met.tok) + " tok" : "—";
    var mini =
      '<div class="sum-mini-row">' +
      '<div class="sum-mini-card">Token count<strong>' +
      escapeHtml(tokLine) +
      '</strong></div><div class="sum-mini-card">Duration<strong>' +
      escapeHtml(humanDurationMs(spanMs)) +
      '</strong></div><div class="sum-mini-card">Vectors retrieved<strong>' +
      (met.vec != null ? escapeHtml(String(met.vec)) : "—") +
      "</strong></div></div>";
    var life = conversationLifecycleBarHtml(cardModel.progress, {});
    var chips = conversationCardChipsSummaryHtml(cardModel);
    var kvBlock = conversationCardKvHtml(cardModel);
    var ingestBlock = "";
    if (ingestCount > 0 && cardModel.ingestRunIds && cardModel.ingestRunIds.length) {
      ingestBlock =
        '<details class="sum-conv-ingest"><summary>' +
        escapeHtml(
          "Ingest · " +
            ingestCount +
            " line" +
            (ingestCount === 1 ? "" : "s") +
            " · runs " +
            cardModel.ingestRunIds.join(", ")
        ) +
        '</summary><p class="muted sum-conv-ingest-hint">Lines tagged <strong>ingest</strong> in the full log share <code>index_run_id</code> with this conversation.</p></details>';
    }
    var turnGroups = null;
    if (
      globalThis.ClaudiaLogs &&
      ClaudiaLogs.Derive &&
      typeof ClaudiaLogs.Derive.conversationTurnGroupsForExpanded === "function"
    ) {
      turnGroups = ClaudiaLogs.Derive.conversationTurnGroupsForExpanded(evs, getFlat);
    }
    var cardKey =
      Array.isArray(g.cids) && g.cids.length > 1
        ? g.pid + "\0" + g.cids.slice().sort().join("\0")
        : g.pid + "\0" + g.cid;
    var convScope = strHash(cardKey);
    var scrollTbodyId = "conv-log-" + convScope;
    var tbodyInner = sumEvlogBuildTbodyFromConvEvents(evs, turnGroups, convScope);
    var mc = sumEvlogCountWarnFailFromEntries(evs);
    var servicesStrip = serviceStripHtml(evs);
    var full =
      '<div class="sum-full-log sum-full-log--evlog">' +
      sumEvlogPanelHtml({
        scrollTbodyId: scrollTbodyId,
        warnN: mc.warn,
        failN: mc.fail,
        tbodyInnerHtml: tbodyInner,
        title: "Full event log",
        titleRightHtml: servicesStrip || ""
      }) +
      "</div>";
    var contextStrip = SHOW_CONV_EXPANDED_CONTEXT_STRIP ? contextGrowthStripHtml(evs) : "";
    return (
      '<div class="sum-body">' +
      '<div class="sum-section-label">Lifecycle</div>' +
      life +
      chips +
      kvBlock +
      mini +
      (contextStrip ? '<div class="sum-section-label">Context</div>' + contextStrip : "") +
      ingestBlock +
      full +
      "</div>"
    );
  }

  function buildConvCard(g) {
    var t1 = null;
    for (var ti = 0; ti < g.events.length; ti++) {
      var ins = entryInstant({ ts: g.events[ti].ts });
      if (ins && (!t1 || ins.getTime() > t1.getTime())) t1 = ins;
    }
    var cid = String(g.cid);
    var mergedN = Array.isArray(g.cids) ? g.cids.length : 1;
    var title = formatConversationCardTitle(g.pid, cid) + formatMergedConversationSubtitle(mergedN);
    var lastEv = g.events[g.events.length - 1];
    var sub =
      '<span class="sum-sub sum-sub--clamp">' +
      escapeHtml(primaryLogMessage(lastEv.parsed, lastEv.text)) +
      "</span>";
    var cardModel = conversationCardModelForGroup(g.events);
    var dur = humanDurationMs(convWindowMs(g));
    var st = conversationCardStatus(g, t1, cardModel);
    var cardKey =
      Array.isArray(g.cids) && g.cids.length > 1
        ? g.pid + "\0" + g.cids.slice().sort().join("\0")
        : g.pid + "\0" + g.cid;
    var cardId = strHash(cardKey);
    var ini = avatarInitials(tokenLabelByTenant[g.pid] || g.pid);
    var av = avatarHueClass(cardKey);
    var sumChips = conversationCardChipsSummaryHtml(cardModel);
    var metrics =
      '<span class="sum-metrics">' +
      '<span class="sum-metric">' +
      escapeHtml(dur) +
      "</span></span>";
    var lifeCompact = conversationLifecycleBarHtml(cardModel.progress, { compact: true });
    return (
      '<details class="sum-card sum-card--conversation" id="' +
      escapeHtml(cardId) +
      '"><summary>' +
      '<span class="sum-avatar ' +
      av +
      '">' +
      escapeHtml(ini) +
      "</span>" +
      '<span class="sum-main"><span class="sum-title">' +
      title +
      "</span>" +
      sub +
      (sumChips ? sumChips : "") +
      "</span>" +
      metrics +
      lifeCompact +
      '<span class="sum-status ' +
      st.cls +
      '">' +
      escapeHtml(st.st) +
      "</span>" +
      '<span class="sum-chev"></span></summary>' +
      renderExpandedConv(g) +
      "</details>"
    );
  }

  /** Display label for a supervised YAML root (no directory path). */
  function formatIndexerSupervisedRootLabel(row) {
    if (!row || typeof row !== "object") return "—";
    var proj = row.project_id != null ? String(row.project_id).trim() : "";
    var ws = row.workspace_id != null ? String(row.workspace_id).trim() : "";
    var flav = row.flavor_id != null ? String(row.flavor_id).trim() : "";
    var bits = [];
    if (proj) bits.push(proj);
    if (flav) bits.push(flav);
    if (ws) bits.push(ws);
    return bits.join(" · ") || "—";
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
    var arr = lastIndexerOperatorWorkspacesNested.slice();
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
    lastIndexerOperatorWorkspacesNested = dedupeOperatorWorkspacesNested(arr);
  }

  function syncIndexerOperatorPayloadFromConfigJson(d) {
    if (!d || typeof d !== "object") return;
    var roots = Array.isArray(d.roots) ? d.roots : [];
    lastIndexerOperatorRoots = roots;
    try {
      lastIndexerOperatorRootsJson = JSON.stringify(roots);
    } catch (_eSyn) {
      lastIndexerOperatorRootsJson = "";
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
      lastIndexerOperatorWorkspacesNested = dedupeOperatorWorkspacesNested(
        uniqWs.length ? uniqWs : deriveNestedWorkspacesFromFlatRoots(roots)
      );
    } else {
      lastIndexerOperatorWorkspacesNested = dedupeOperatorWorkspacesNested(
        deriveNestedWorkspacesFromFlatRoots(roots)
      );
    }
  }

  function indexerOperatorWorkspaceCardHrefByRowId(wsRowId) {
    return "#ix-opws-" + strHash(String(wsRowId || ""));
  }

  function resolveLogsOperatorUserLabel() {
    var z = tokenLabelByTenant[""];
    if (z != null && String(z).trim() !== "") return String(z).trim();
    var ks = Object.keys(tokenLabelByTenant);
    for (var i = 0; i < ks.length; i++) {
      var v = tokenLabelByTenant[ks[i]];
      if (v != null && String(v).trim() !== "") return String(v).trim();
    }
    return "—";
  }

  /** Same title line as IX / stale / managed WS cards (em dash segments). */
  function workspaceCardTitleFromIndexerMeta(meta) {
    var userLine =
      meta.userLabel && meta.userLabel !== "—" ? String(meta.userLabel).trim() : "—";
    var prLine =
      meta.projectId && meta.projectId !== "—" ? String(meta.projectId).trim() : "—";
    var flavLine =
      meta.flavorId && meta.flavorId !== "—" ? String(meta.flavorId).trim() : "";
    return flavLine !== ""
      ? userLine + " — " + prLine + " — " + flavLine
      : userLine + " — " + prLine;
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
        globalThis.ClaudiaLogs &&
        ClaudiaLogs.Derive &&
        typeof ClaudiaLogs.Derive.indexerPartitionMetaForRun === "function"
      ) {
        pmeta = ClaudiaLogs.Derive.indexerPartitionMetaForRun(
          partitionRegistry,
          run.id,
          run.events,
          getFlat
        );
      }
      var meta = collectIndexerRunMeta(run.id, run.events, pmeta);
      meta = mergePersistedIndexerWatchRoots(meta, run.events, run.id);
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

  /**
   * Distinct scopes from partitioned indexer runs with links to matching Workspaces cards (fallback before API).
   */
  function aggregateIndexerManagedWorkspacesHtml(byRun, partitionRegistry) {
    var seen = {};
    var rows = [];
    if (!byRun || typeof byRun !== "object") {
      return '<span class="muted">—</span>';
    }
    var keys = Object.keys(byRun);
    for (var i = 0; i < keys.length; i++) {
      var run = byRun[keys[i]];
      if (!run || !run.events || !run.events.length) continue;
      var pmeta = null;
      if (
        partitionRegistry &&
        globalThis.ClaudiaLogs &&
        ClaudiaLogs.Derive &&
        typeof ClaudiaLogs.Derive.indexerPartitionMetaForRun === "function"
      ) {
        pmeta = ClaudiaLogs.Derive.indexerPartitionMetaForRun(
          partitionRegistry,
          run.id,
          run.events,
          getFlat
        );
      }
      var meta = collectIndexerRunMeta(run.id, run.events, pmeta);
      meta = mergePersistedIndexerWatchRoots(meta, run.events, run.id);
      var lab = indexerCardTitleSortLabel(meta);
      if (!lab || lab === "—") continue;
      if (seen[lab]) continue;
      seen[lab] = true;
      rows.push({ label: lab, bucketId: run.id });
    }
    sortIndexerManagedWorkspaceRows(rows);
    return indexerManagedWorkspacesCommaLinksHtml(rows);
  }

  /**
   * Supersedes workspace list with supervised YAML roots when available; sets config path from gateway.
   */
  function hydrateIndexerServiceSummaryFromApi() {
    var wsEl = document.getElementById("svc-indexer-summary-workspaces");
    var cfgEl = document.getElementById("svc-indexer-summary-config-path");
    if (!cfgEl && !wsEl) return;
    fetch("/api/ui/indexer/config", { credentials: "same-origin" })
      .then(function (res) {
        if (!res.ok) throw new Error("bad status");
        return res.json();
      })
      .then(function (d) {
        var prevRootsJ = lastIndexerOperatorRootsJson;
        syncIndexerOperatorPayloadFromConfigJson(d);
        var nextRootsJ = lastIndexerOperatorRootsJson;
        var nextRoots = lastIndexerOperatorRoots;
        var prevHadRoots = prevRootsJ !== "" && prevRootsJ !== "[]";
        if (
          nextRootsJ !== prevRootsJ &&
          viewMode === "summarized" &&
          (nextRoots.length > 0 || prevHadRoots) &&
          !indexerOperatorRootsRefreshQueued
        ) {
          indexerOperatorRootsRefreshQueued = true;
          window.requestAnimationFrame(function () {
            indexerOperatorRootsRefreshQueued = false;
            refreshSummarizedPanel();
          });
        }
        if (cfgEl) {
          var pth = d.path != null ? String(d.path).trim() : "";
          cfgEl.innerHTML = pth
            ? "<code>" + escapeHtml(pth) + "</code>"
            : '<span class="muted">—</span>';
        }
        if (wsEl && Array.isArray(d.roots) && d.roots.length > 0) {
          var br = lastIndexerSummarizeByRun;
          var preg = lastIndexerSummarizePartitionRegistry;
          var rows = [];
          for (var ri = 0; ri < d.roots.length; ri++) {
            var rowR = d.roots[ri] || {};
            var bidR = findIndexerBucketIdForSupervisedRoot(rowR, br, preg);
            /** Operator-store shape: project · flavor · workspace row id (id last). Do not swap to log-card title (user · project · flavor) — that prepends user and drops the row id. */
            var labR = formatIndexerSupervisedRootLabel(rowR);
            var hrefR = "";
            if (!bidR) {
              var rowWs =
                rowR.workspace_row_id != null && String(rowR.workspace_row_id).trim() !== ""
                  ? String(rowR.workspace_row_id).trim()
                  : rowR.workspace_id != null && String(rowR.workspace_id).trim() !== ""
                    ? String(rowR.workspace_id).trim()
                    : "";
              if (rowWs) hrefR = indexerOperatorWorkspaceCardHrefByRowId(rowWs);
            }
            rows.push({ label: labR, bucketId: bidR, href: hrefR });
          }
          sortIndexerManagedWorkspaceRows(rows);
          wsEl.innerHTML = indexerManagedWorkspacesCommaLinksHtml(rows);
        }
      })
      .catch(function () {
        if (cfgEl) {
          cfgEl.innerHTML =
            '<span class="muted">Not available (supervised indexer config path not set)</span>';
        }
      });
  }

  function renderExpandedService(name, arr, svcCtx) {
    svcCtx = svcCtx || {};
    var isBifrost = name === "bifrost";
    var evConv = [];
    for (var j = 0; j < arr.length; j++) {
      evConv.push({ parsed: arr[j].parsed, text: arr[j].text, ts: arr[j].ts, source: arr[j].source });
    }
    var timelineBlock = "";
    if (name !== "indexer" && name !== "qdrant" && name !== "bifrost" && name !== "gateway") {
      timelineBlock = '<div class="sum-section-label">Request timeline</div>' + timelineBarHtml(evConv);
    }
    var aggregateIndexerProgressBlock = "";
    if (name === "indexer") {
      var aggIx = rollupIndexerAggregateScopeProgress(svcCtx.byRun, svcCtx.partitionRegistry);
      var aggCap = indexerAggregateBacklogCaption(aggIx.sumRem, aggIx.sumTot);
      var aggDetail = indexerAggregateProgressDetailText(aggIx);
      var aggCaptionRow =
        '<div class="indexer-aggregate-caption-row">' +
        (aggCap !== ""
          ? '<span class="indexer-scope-caption">' + escapeHtml(aggCap) + "</span>"
          : "") +
        '<span class="indexer-aggregate-progress-detail muted">' +
        escapeHtml(aggDetail) +
        "</span></div>";
      aggregateIndexerProgressBlock =
        '<div class="indexer-aggregate-progress-wrap">' +
        '<div class="sum-section-label sum-section-label--indexer-progress">' +
        escapeHtml("Progress (all indexers)") +
        "</div>" +
        '<div class="indexer-scope-progress indexer-scope-progress--aggregate" title="Sum of pending ingest + fan-out rows vs workspace files across all indexer scopes (from indexer.scope.status)">' +
        indexerScopeProgressTimelineBarHtml(aggIx.sumRem, aggIx.sumTot, aggIx.allDone) +
        aggCaptionRow +
        "</div></div>";
    }
    var indexerSummaryKv = "";
    if (name === "indexer") {
      indexerSummaryKv =
        buildIndexerCardIntroHtml() +
        '<dl class="indexer-run-kv indexer-run-kv--service-aggregate">' +
        "<dt>Managed workspaces</dt><dd id=\"svc-indexer-summary-workspaces\">" +
        aggregateIndexerManagedWorkspacesHtml(svcCtx.byRun, svcCtx.partitionRegistry) +
        '</dd><dt>Indexer config file</dt><dd id="svc-indexer-summary-config-path"><span class="muted">Loading…</span></dd>' +
        "</dl>";
    }
    var mini;
    if (isBifrost) {
      var bx = bifrostCardMetrics(arr);
      var kvB = bifrostServicePanelKvHtml(arr);
      var tokLineB = "— → —";
      if (bx.outgoingSum > 0 || bx.usageSum > 0) {
        tokLineB =
          (bx.outgoingSum > 0 ? formatInt(Math.round(bx.outgoingSum)) : "—") +
          " → " +
          (bx.usageSum > 0 ? formatInt(Math.round(bx.usageSum)) : "—");
      }
      var rlBox = bx.rateLimitBoxN != null ? bx.rateLimitBoxN : 0;
      var fbBox = bx.fallbackN != null ? bx.fallbackN : 0;
      var availModelsStr =
        bx.catalogModelCount != null && bx.catalogModelCount > 0 ? formatInt(bx.catalogModelCount) : "—";
      var providerHealthStrip = bifrostProviderHealthStripHtml(arr);
      var relayOutcomeStrip = bifrostRelayOutcomeStripHtml(arr);
      mini =
        buildBifrostCardIntroHtml() +
        '<div class="sum-section-label">Provider health</div>' +
        providerHealthStrip +
        '<div class="sum-mini-row sum-mini-row--bifrost-deck">' +
        '<div class="sum-mini-card">Available models<strong>' +
        escapeHtml(availModelsStr) +
        '</strong><span class="sum-mini-sub">Count from latest BiFrost catalog sync log (when BiFrost reports a numeric total)</span></div>' +
        "</div>" +
        kvB +
        '<div class="sum-section-label">Relay outcomes</div>' +
        relayOutcomeStrip +
        '<div class="sum-mini-row sum-mini-row--bifrost-deck2">' +
        '<div class="sum-mini-card">Relay (ok / fail)<strong>' +
        escapeHtml(formatInt(bx.relayOk) + " / " + formatInt(bx.relayFail)) +
        '</strong><span class="sum-mini-sub">Successful upstream responses vs errors (gateway relay)</span></div>' +
        '<div class="sum-mini-card">Tokens (out → usage)<strong>' +
        escapeHtml(tokLineB) +
        "</strong>" +
        '<span class="sum-mini-sub">Prompt tokens sent vs completion usage from upstream JSON</span></div>' +
        '<div class="sum-mini-card">Rate limits<strong>' +
        escapeHtml(formatInt(rlBox)) +
        '</strong><span class="sum-mini-sub">Throttling / HTTP 429 (BiFrost HTTP + chat relay)</span></div>' +
        '<div class="sum-mini-card">Routing fallback<strong>' +
        escapeHtml(formatInt(fbBox)) +
        '</strong><span class="sum-mini-sub">Virtual model fallback attempts (gateway)</span></div>' +
        '</div>';
    } else if (name === "indexer") {
      mini = indexerStructuredRollupMiniHtml(arr);
    } else if (name === "gateway") {
      mini = buildGatewayCardIntroHtml() + gatewayServicePanelMiniHtml(arr);
    } else if (name === "qdrant") {
      mini = buildQdrantCardIntroHtml() + qdrantServicePanelMiniHtml(arr);
    } else {
      var httpN2 = 0,
        sumMs2 = 0,
        err2 = countWarnErrorInEntries(arr);
      for (var k2 = 0; k2 < arr.length; k2++) {
        if (arr[k2].parsed.shape === "http.access") {
          httpN2++;
          var rt2 = Number(getFlat(arr[k2].parsed).responseTimeMs);
          if (!isNaN(rt2)) sumMs2 += rt2;
        }
      }
      mini =
        '<div class="sum-mini-row">' +
        '<div class="sum-mini-card">Lines<strong>' +
        escapeHtml(String(arr.length)) +
        '</strong></div><div class="sum-mini-card">HTTP · Σ ms<strong>' +
        escapeHtml(formatInt(httpN2) + " · " + (httpN2 ? String(Math.round(sumMs2)) : "—")) +
        '</strong></div><div class="sum-mini-card">Warn+error lines<strong>' +
        escapeHtml(String(err2)) +
        "</strong></div></div>";
    }
    var fullLogClass = isBifrost
      ? "sum-full-log sum-full-log--bifrost sum-full-log--evlog"
      : "sum-full-log sum-full-log--evlog";
    var scrollTbodyId = "svc-log-" + strHash(name);
    var cardScope = strHash("svc:" + name);
    var visEnt = sumEvlogVisibleEntriesForService(name, arr, name === "gateway");
    var mc = sumEvlogCountWarnFailFromEntries(visEnt);
    var tbodyInner = sumEvlogBuildTbodyFromServiceEntries(name, arr, {
      cardScope: cardScope,
      filterGatewayProbe: name === "gateway"
    });
    var full =
      '<div class="' + fullLogClass + '">' +
      sumEvlogPanelHtml({
        scrollTbodyId: scrollTbodyId,
        warnN: mc.warn,
        failN: mc.fail,
        tbodyInnerHtml: tbodyInner,
        title: "Full event log"
      }) +
      "</div>";
    return (
      '<div class="sum-body">' +
      timelineBlock +
      '<div class="sum-section-label">Summary</div>' +
      indexerSummaryKv +
      aggregateIndexerProgressBlock +
      mini +
      full +
      "</div>"
    );
  }

  function serviceAvatarClass(name) {
    switch (name) {
      case "gateway":
        return "sum-av-svc-gateway";
      case "bifrost":
        return "sum-av-svc-bifrost";
      case "qdrant":
        return "sum-av-svc-qdrant";
      case "indexer":
        return "sum-av-svc-indexer";
      default:
        return avatarHueClass(name);
    }
  }

  function serviceAvatarInitials(name) {
    switch (name) {
      case "bifrost":
        return "BF";
      case "gateway":
        return "GW";
      case "qdrant":
        return "QD";
      case "indexer":
        return "IX";
      default:
        return avatarInitials(name);
    }
  }

  function buildServiceCard(name, arr, svcCtx) {
    svcCtx = svcCtx || {};
    var httpN = 0;
    var sumMs = 0;
    for (var k = 0; k < arr.length; k++) {
      var p = arr[k].parsed;
      if (p.shape === "http.access") {
        httpN++;
        var rt = Number(getFlat(p).responseTimeMs);
        if (!isNaN(rt)) sumMs += rt;
      }
    }
    var isBifrost = name === "bifrost";
    var gwCardModel = null;
    if (
      name === "gateway" &&
      globalThis.ClaudiaLogs &&
      ClaudiaLogs.Derive &&
      typeof ClaudiaLogs.Derive.gatewayCardModel === "function"
    ) {
      gwCardModel = ClaudiaLogs.Derive.gatewayCardModel(arr, getFlat);
    }
    var qdrCardModel = null;
    if (
      name === "qdrant" &&
      globalThis.ClaudiaLogs &&
      ClaudiaLogs.Derive &&
      typeof ClaudiaLogs.Derive.qdrantCardModel === "function"
    ) {
      qdrCardModel = ClaudiaLogs.Derive.qdrantCardModel(arr, getFlat, qdrantCollectionScopeLabelForLogs);
    }
    var lastMsg = isBifrost ? bifrostCollapsedCardSubtitle(arr) : "";
    if (!isBifrost) {
      var last = arr.length ? arr[arr.length - 1] : null;
      if (last) lastMsg = primaryLogMessage(last.parsed, last.text);
      if (name === "qdrant" && qdrCardModel && qdrCardModel.subtitle && qdrCardModel.subtitle !== "—") {
        lastMsg = qdrCardModel.subtitle;
      }
      if (name === "gateway" && gwCardModel && gwCardModel.subtitle && gwCardModel.subtitle !== "—") {
        lastMsg = gwCardModel.subtitle;
      }
    }
    var ixWaitFlat = name === "indexer" ? indexerLatestSupervisedWaitFlat(arr) : null;
    if (ixWaitFlat) {
      var ixWaitProse =
        globalThis.ClaudiaLogs &&
          ClaudiaLogs.Derive &&
          typeof ClaudiaLogs.Derive.indexerProseSummary === "function"
          ? ClaudiaLogs.Derive.indexerProseSummary(ixWaitFlat)
          : null;
      if (ixWaitProse && String(ixWaitProse).trim() !== "") lastMsg = String(ixWaitProse).trim();
    }
    var st;
    if (recentServiceCardHasError(name, arr)) {
      st = { st: "error", cls: "sum-st-error" };
    } else if (name === "gateway" && gwCardModel) {
      if (gwCardModel.cardStatus === "error") st = { st: "error", cls: "sum-st-error" };
      else if (gwCardModel.cardStatus === "warn") st = { st: "degraded", cls: "sum-st-monitor" };
      else st = { st: "active", cls: "sum-st-active" };
    } else if (ixWaitFlat) {
      st = { st: "idle", cls: "sum-st-monitor" };
    } else {
      st = { st: "active", cls: "sum-st-active" };
    }
    var sid = "svc-" + strHash(name);
    var ini = serviceAvatarInitials(name);
    var av = serviceAvatarClass(name);
    /** Single outer .sum-title only — avoid nesting .sum-title (was hiding pills / breaking layout). */
    var titleClass = "sum-title";
    var titleBlock = escapeHtml(name === "indexer" ? "indexer" : isBifrost ? "bifrost" : name);
    var bifrostCompactHealth = isBifrost ? bifrostProviderHealthStripHtml(arr, { compact: true }) : "";
    var wms = serviceWindowMs(arr);
    var metrics;
    if (isBifrost) {
      var bxC = bifrostCardMetrics(arr);
      var pill1 = formatInt(bxC.relayOk) + " ok · " + formatInt(bxC.relayFail) + " fail";
      var rlb = bxC.rateLimitBoxN != null ? bxC.rateLimitBoxN : 0;
      var fbb = bxC.fallbackN != null ? bxC.fallbackN : 0;
      var pill3 = formatInt(rlb) + " rate-limit · " + formatInt(fbb) + " fallback";
      if (pill3.length > 36) pill3 = pill3.slice(0, 34) + "…";
      metrics =
        '<span class="sum-metrics">' +
        '<span class="sum-metric">' +
        escapeHtml(pill1) +
        '</span><span class="sum-metric">' +
        escapeHtml(pill3) +
        "</span></span>";
    } else if (name === "qdrant") {
      if (qdrCardModel) {
        var qm = qdrCardModel;
        var qColsPill = formatInt(qm.collLoaded || 0) + " / " + formatInt(qm.collTotal || 0);
        var qUpPill = formatInt(qm.upsertOk || 0) + " ok · " + formatInt(qm.upsertFail || 0) + " fail";
        var qSrPill = formatInt(qm.searchOk || 0) + " ok · " + formatInt(qm.searchFail || 0) + " fail";
        metrics =
          '<span class="sum-metrics">' +
          '<span class="sum-metric" title="Collections loaded / total (lines since last qdrant.version)">' +
          'Collections ' +
          escapeHtml(qColsPill) +
          '</span><span class="sum-metric" title="Points upsert: HTTP 200 vs rejected / non-200">' +
          'Upserts ' +
          escapeHtml(qUpPill) +
          '</span><span class="sum-metric" title="Vector search: HTTP 200 vs fail">' +
          'Searches ' +
          escapeHtml(qSrPill) +
          "</span></span>";
      } else {
        metrics = "";
      }
    } else if (name === "gateway") {
      if (gwCardModel) {
        var gc = gwCardModel.counters || {};
        var httpPill =
          formatInt(gc.http2xx || 0) + " ok · " + formatInt(gc.httpNot2xx || 0) + " fail";
        if (gc.http429 > 0) {
          httpPill += " · " + formatInt(gc.http429) + " rate-limited";
        }
        var chatPill =
          "chat " +
          formatInt(gc.chatReq || 0) +
          " reqs → " +
          formatInt(gc.chatResp || 0) +
          " resps · " +
          formatInt(gc.chatErr || 0) +
          " errs";
        metrics =
          '<span class="sum-metrics">' +
          '<span class="sum-metric" title="HTTP 2xx vs non-2xx (gateway.http.access in this buffer window)">' +
          escapeHtml(httpPill) +
          '</span><span class="sum-metric" title="chat.request vs chat.bifrost.response vs chat.bifrost.error">' +
          escapeHtml(chatPill) +
          "</span></span>";
      } else {
        metrics = "";
      }
    } else if (name === "indexer") {
      var qiIx = latestIndexerStateQueueInflightFromEntries(arr);
      var snapIx = latestIndexerQueueSnapshotMetaFromEntries(arr);
      var ixPills = "";
      if (qiIx.queueDepth != null && !isNaN(Number(qiIx.queueDepth))) {
        var qCur = formatInt(Math.round(Number(qiIx.queueDepth)));
        var qMax =
          snapIx.queueCap != null && !isNaN(Number(snapIx.queueCap))
            ? formatInt(Math.round(Number(snapIx.queueCap)))
            : "—";
        ixPills +=
          '<span class="sum-metric" title="Queue depth vs max (queue_depth / queue_cap)">' +
          escapeHtml("Queue " + qCur + " / " + qMax) +
          "</span>";
      }
      if (qiIx.ingestInflight != null && !isNaN(Number(qiIx.ingestInflight))) {
        var iCur = formatInt(Math.round(Number(qiIx.ingestInflight)));
        var iWorkers =
          snapIx.workers != null && !isNaN(Number(snapIx.workers))
            ? formatInt(Math.round(Number(snapIx.workers)))
            : "—";
        ixPills +=
          '<span class="sum-metric" title="In-flight ingests vs worker pool (ingest_inflight / workers)">' +
          escapeHtml(iCur + " inflight / " + iWorkers + " workers") +
          "</span>";
      }
      metrics = '<span class="sum-metrics">' + ixPills + "</span>";
    } else {
      metrics =
        '<span class="sum-metrics">' +
        '<span class="sum-metric">' +
        escapeHtml(String(arr.length)) +
        ' lines</span>' +
        (httpN ? '<span class="sum-metric">' + escapeHtml(String(httpN)) + " http</span>" : "") +
        (sumMs ? '<span class="sum-metric">' + escapeHtml(humanDurationMs(sumMs)) + " Σ</span>" : "") +
        "</span>";
    }
    return (
      '<details class="sum-card" id="' +
      escapeHtml(sid) +
      '"><summary>' +
      '<span class="sum-avatar ' +
      av +
      '">' +
      escapeHtml(ini) +
      "</span>" +
      '<span class="sum-main"><span class="' +
      titleClass +
      '">' +
      titleBlock +
      '</span><span class="sum-sub sum-sub--clamp">' +
      escapeHtml(lastMsg) +
      "</span></span>" +
      metrics +
      bifrostCompactHealth +
      '<span class="sum-status ' +
      st.cls +
      '">' +
      escapeHtml(st.st) +
      "</span>" +
      '<span class="sum-chev"></span></summary>' +
      renderExpandedService(name, arr, svcCtx) +
      "</details>"
    );
  }

  /** dt/dd fragment for expanded indexer Summary: user, project, flavor, optional workspace row id, file count. Joined into one indexer-run-kv with Watched paths. */
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
          escapeHtml(meta.watchRootPaths.join("\n")) +
          "</pre>"
          : '<span class="muted">—</span>';
    var summaryRows =
      '<dl class="indexer-run-kv">' +
      indexerExpandedSummaryKvInnerHtml(meta, kvOpts) +
      "<dt>Watched paths</dt><dd>" +
      pathsBlock +
      "</dd></dl>";
    var afterSummary = expOpts.extraAfterSummaryHtml != null ? String(expOpts.extraAfterSummaryHtml) : "";
    var evsFull = filterEventsForIndexerScopeFullLog(evs, run.id, partitionRegistry || {});
    var recentOpts = expOpts.recentOpts || {};
    var recentFiles = buildIndexerRecentEvaluatedFilesHtml(evsFull, run.id, 18, recentOpts);
    var recentSection = recentFiles
      ? '<div class="sum-section-label">Recently evaluated files</div>' + recentFiles
      : "";
    var fullId = "ix-full-" + strHash(run.id);
    var ixScope = strHash("ixrun:" + run.id);
    var tbodyInner;
    var mc;
    if (!evsFull.length) {
      tbodyInner =
        '<tr class="sum-evlog__row"><td colspan="3" class="muted">No scope-specific lines in the loaded window (shared lines appear under Services → Indexer).</td></tr>';
      mc = { warn: 0, fail: 0 };
    } else {
      tbodyInner = sumEvlogBuildTbodyFromServiceEntries("indexer", evsFull, {
        cardScope: ixScope,
        filterGatewayProbe: false,
        indexerRunLine: true
      });
      mc = sumEvlogCountWarnFailFromEntries(evsFull);
    }
    var full =
      '<div class="sum-full-log sum-full-log--evlog">' +
      sumEvlogPanelHtml({
        scrollTbodyId: fullId,
        warnN: mc.warn,
        failN: mc.fail,
        tbodyInnerHtml: tbodyInner,
        title: "Full event log"
      }) +
      "</div>";
    return (
      '<div class="sum-body">' +
      '<div class="sum-section-label">Summary</div>' +
      summaryRows +
      afterSummary +
      recentSection +
      full +
      "</div>"
    );
  }

  function loadIndexerWatchRootsStore() {
    try {
      var s = localStorage.getItem(INDEXER_WATCH_ROOTS_LS);
      if (!s) return { byBucket: {}, byRunId: {}, snapshots: {} };
      var o = JSON.parse(s);
      if (!o || typeof o !== "object") return { byBucket: {}, byRunId: {}, snapshots: {} };
      if (!o.byBucket || typeof o.byBucket !== "object") o.byBucket = {};
      if (!o.byRunId || typeof o.byRunId !== "object") o.byRunId = {};
      if (!o.snapshots || typeof o.snapshots !== "object") o.snapshots = {};
      return o;
    } catch (_e) {
      return { byBucket: {}, byRunId: {}, snapshots: {} };
    }
  }

  function saveIndexerWatchRootsStore(store) {
    try {
      localStorage.setItem(INDEXER_WATCH_ROOTS_LS, JSON.stringify(store));
    } catch (_e) {
      try {
        var st = loadIndexerWatchRootsStore();
        var keys = Object.keys(st.byBucket);
        keys.sort();
        for (var ki = 0; ki < Math.min(24, keys.length); ki++) delete st.byBucket[keys[ki]];
        localStorage.setItem(INDEXER_WATCH_ROOTS_LS, JSON.stringify(st));
      } catch (_e2) { }
    }
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

  var qdrantScopeLabelMapCacheRun = null;
  var qdrantScopeLabelMapCachePreg = null;
  var qdrantScopeLabelMapCache = null;

  function buildQdrantCollectionScopeLabelMap() {
    var map = {};
    var byRun = lastIndexerSummarizeByRun;
    if (!byRun || typeof byRun !== "object") return map;
    var preg = lastIndexerSummarizePartitionRegistry;
    var keys = Object.keys(byRun);
    for (var i = 0; i < keys.length; i++) {
      var run = byRun[keys[i]];
      if (!run || !run.events || !run.events.length) continue;
      var pmeta = null;
      if (
        preg &&
        globalThis.ClaudiaLogs &&
        ClaudiaLogs.Derive &&
        typeof ClaudiaLogs.Derive.indexerPartitionMetaForRun === "function"
      ) {
        pmeta = ClaudiaLogs.Derive.indexerPartitionMetaForRun(preg, run.id, run.events, getFlat);
      }
      var meta = collectIndexerRunMeta(run.id, run.events, pmeta);
      meta = mergePersistedIndexerWatchRoots(meta, run.events, run.id);
      if (
        !globalThis.ClaudiaLogs ||
        !ClaudiaLogs.Derive ||
        typeof ClaudiaLogs.Derive.qdrantCollectionNameFromIndexerMeta !== "function"
      )
        continue;
      var cn = ClaudiaLogs.Derive.qdrantCollectionNameFromIndexerMeta(meta);
      if (!cn) continue;
      map[cn] = indexerCardTitleSortLabel(meta);
    }
    return map;
  }

  /** Maps physical Qdrant collection dir → USER:PROJECT[:FLAVOR] (same colon format as indexer workspace labels). */
  function qdrantCollectionScopeLabelForLogs(collRaw) {
    if (
      lastIndexerSummarizeByRun !== qdrantScopeLabelMapCacheRun ||
      lastIndexerSummarizePartitionRegistry !== qdrantScopeLabelMapCachePreg
    ) {
      qdrantScopeLabelMapCacheRun = lastIndexerSummarizeByRun;
      qdrantScopeLabelMapCachePreg = lastIndexerSummarizePartitionRegistry;
      qdrantScopeLabelMapCache = buildQdrantCollectionScopeLabelMap();
    }
    var c = String(collRaw != null ? collRaw : "").trim();
    if (!c) return c;
    var hit = qdrantScopeLabelMapCache && qdrantScopeLabelMapCache[c];
    return hit != null && String(hit).trim() !== "" ? String(hit).trim() : c;
  }

  /** Physical collection id → user:project[:flavor] when indexer summary map is available (same as Qdrant lines). */
  function ragCollectionLabelForUi(collRaw) {
    var r = collRaw != null ? String(collRaw).trim() : "";
    if (!r) return "";
    if (
      globalThis.ClaudiaLogs &&
      ClaudiaLogs.Derive &&
      typeof ClaudiaLogs.Derive.qdrantCollectionDisplay === "function"
    ) {
      var lab = ClaudiaLogs.Derive.qdrantCollectionDisplay(r, qdrantCollectionScopeLabelForLogs);
      if (lab != null && String(lab).trim() !== "") return String(lab).trim();
    }
    return r;
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
    var titleText =
      flavLine !== "" ? userLine + " — " + prLine + " — " + flavLine : userLine + " — " + prLine;
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
      '<summary>' +
      '<span class="sum-avatar sum-av-c">IX</span>' +
      '<span class="sum-main"><span class="sum-title">' +
      '<span class="sum-title-indexer-head">' +
      escapeHtml(titleText) +
      "</span>" +
      '</span><span class="sum-sub sum-sub--clamp muted">' +
      escapeHtml("Waiting on status update from an indexer worker") +
      "</span></span>" +
      '<span class="sum-chev"></span></summary>' +
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
   * When indexer.run.start drops out of the ring buffer, restore watch roots from localStorage.
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
    var roots = lastIndexerOperatorRoots;
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
    }
    return meta;
  }

  function indexerMetaForBucketDedup(run, partitionRegistry) {
    var evs = run.events;
    var pmeta = null;
    if (
      partitionRegistry &&
      globalThis.ClaudiaLogs &&
      ClaudiaLogs.Derive &&
      typeof ClaudiaLogs.Derive.indexerPartitionMetaForRun === "function"
    ) {
      pmeta = ClaudiaLogs.Derive.indexerPartitionMetaForRun(
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
      var metaId = indexerMetaForBucketDedup(runP1, partitionRegistry);
      var mw0 =
        metaId.workspaceId && metaId.workspaceId !== "—" ? String(metaId.workspaceId).trim() : "";
      if (mw0 && canonicalWorkspaceRowIdKey(mw0) === wid) return true;
    }
    // Pass 2: project + flavor, then workspace id or path set (legacy empty workspace in logs).
    for (hi = 0; hi < keys.length; hi++) {
      var run = byRun[keys[hi]];
      if (!run || !run.events || !run.events.length) continue;
      var meta = indexerMetaForBucketDedup(run, partitionRegistry);
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
    var wsn = lastIndexerOperatorWorkspacesNested || [];
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
    if (!meta || !lastIndexerOperatorWorkspacesNested || !lastIndexerOperatorWorkspacesNested.length)
      return null;
    var mw =
      meta.workspaceId && meta.workspaceId !== "—" ? String(meta.workspaceId).trim() : "";
    if (mw) {
      var wkey = canonicalWorkspaceRowIdKey(mw);
      if (wkey) {
        var hi;
        for (hi = 0; hi < lastIndexerOperatorWorkspacesNested.length; hi++) {
          var w = lastIndexerOperatorWorkspacesNested[hi];
          if (canonicalWorkspaceRowIdKey(w.id) === wkey) return w;
        }
      }
    }
    var mp = meta.projectId && meta.projectId !== "—" ? String(meta.projectId).trim() : "";
    if (!mp) return null;
    var mf = normalizeFlavorMatch(meta.flavorId);
    var mpaths = meta.watchRootPaths && meta.watchRootPaths.length ? meta.watchRootPaths : [];
    if (!mpaths.length) return null;
    for (hi = 0; hi < lastIndexerOperatorWorkspacesNested.length; hi++) {
      var wx = lastIndexerOperatorWorkspacesNested[hi];
      var xp = wx.project_id != null ? String(wx.project_id).trim() : "";
      if (xp !== mp) continue;
      if (normalizeFlavorMatch(wx.flavor_id) !== mf) continue;
      if (pathsSetEqualForIndexerRoots(operatorWorkspacePaths(wx), mpaths)) return wx;
    }
    return null;
  }

  function normalizeManagedPathRowsForEdit(ws) {
    var out = [];
    if (!ws || !Array.isArray(ws.paths)) return out;
    var pi;
    for (pi = 0; pi < ws.paths.length; pi++) {
      var row = ws.paths[pi] || {};
      var pid = row.id != null ? Number(row.id) : NaN;
      var pth = row.path != null ? String(row.path).trim() : "";
      if (!pth) continue;
      if (!isNaN(pid) && pid > 0) out.push({ id: pid, path: pth });
    }
    return out;
  }

  function cloneManagedPathRows(arr) {
    var out = [];
    if (!Array.isArray(arr)) return out;
    var i;
    for (i = 0; i < arr.length; i++) {
      out.push({
        id: arr[i].id != null && !isNaN(Number(arr[i].id)) ? Math.trunc(Number(arr[i].id)) : null,
        path: String(arr[i].path != null ? arr[i].path : "")
      });
    }
    return out;
  }

  function buildManagedWorkspacePathsEditHtml(wsNum, pathRows) {
    var rows = pathRows && pathRows.length ? pathRows : [];
    var rmDisabled = rows.length ? "" : " disabled";
    var selOpts = "";
    var pi;
    for (pi = 0; pi < rows.length; pi++) {
      selOpts +=
        '<option value="' +
        pi +
        '">' +
        escapeHtml(rows[pi].path) +
        "</option>";
    }
    return (
      '<div class="ws-managed-paths-edit" data-ws-managed-paths="' +
      String(wsNum) +
      '">' +
      '<select class="ws-draft-paths-select ws-managed-paths-select" size="6" aria-label="Watched paths">' +
      selOpts +
      "</select>" +
      '<div class="ws-draft-path-btns">' +
      '<button type="button" class="ws-draft-btn ws-managed-btn-add">Add</button>' +
      '<button type="button" class="ws-draft-btn ws-managed-btn-remove"' +
      rmDisabled +
      ">Remove</button>" +
      "</div></div>"
    );
  }

  function buildManagedWorkspaceToolbarHtml(wsNum, isEdit) {
    if (isEdit) {
      return (
        '<div class="ws-managed-toolbar">' +
        '<span class="ws-managed-editing-hint muted">Editing</span>' +
        '<span class="ws-managed-actions">' +
        '<button type="button" class="ws-draft-btn ws-managed-btn-cancel">Cancel</button>' +
        '<button type="button" class="ws-draft-btn ws-managed-btn-save">Save</button>' +
        '<button type="button" class="ws-managed-btn-delete">Delete workspace</button>' +
        "</span></div>"
      );
    }
    return (
      '<div class="ws-managed-toolbar">' +
      '<button type="button" class="ws-draft-btn ws-managed-btn-configure" data-ws-managed-id="' +
      String(wsNum) +
      '">Configure</button>' +
      "</div>"
    );
  }

  function beginWorkspaceManagedEdit(wsNum) {
    var ws = findOperatorWorkspaceByNumericId(wsNum);
    if (!ws) {
      notifyWorkspaceDraftMsg("Workspace not found.", true);
      return;
    }
    var snap = normalizeManagedPathRowsForEdit(ws);
    workspaceManagedEditId = wsNum;
    workspaceManagedStaging = {
      wsNum: wsNum,
      initialSnapshot: cloneManagedPathRows(snap),
      paths: cloneManagedPathRows(snap)
    };
    scheduleStoryRebuild();
  }

  function cancelWorkspaceManagedEdit() {
    workspaceManagedEditId = null;
    workspaceManagedStaging = null;
    scheduleStoryRebuild();
  }

  function refreshOperatorIndexerWorkspaceStateFromConfig() {
    return fetch("/api/ui/indexer/config", { credentials: "same-origin" }).then(function (res) {
      return res.json().then(function (d) {
        if (!res.ok) throw new Error((d && d.error) || res.statusText || "config fetch failed");
        syncIndexerOperatorPayloadFromConfigJson(d);
        hydrateIndexerServiceSummaryFromApi();
        scheduleStoryRebuild();
      });
    });
  }

  function saveManagedWorkspacePaths(wsNum) {
    var st = workspaceManagedStaging;
    if (!st || st.wsNum !== wsNum || !Array.isArray(st.paths)) {
      notifyWorkspaceDraftMsg("Nothing to save.", true);
      return;
    }
    if (!st.paths.length) {
      notifyWorkspaceDraftMsg("Add at least one watched path.", true);
      return;
    }
    var initial = st.initialSnapshot || [];
    var cur = st.paths;
    var curPersistedIds = {};
    var ci;
    for (ci = 0; ci < cur.length; ci++) {
      if (cur[ci].id != null && !isNaN(Number(cur[ci].id))) curPersistedIds[Math.trunc(Number(cur[ci].id))] = true;
    }
    var toDelete = [];
    var ii;
    for (ii = 0; ii < initial.length; ii++) {
      var iid = initial[ii].id != null ? Math.trunc(Number(initial[ii].id)) : NaN;
      if (!isNaN(iid) && iid > 0 && !curPersistedIds[iid]) toDelete.push(iid);
    }
    var toAdd = [];
    for (ci = 0; ci < cur.length; ci++) {
      var pth = String(cur[ci].path != null ? cur[ci].path : "").trim();
      if (!pth) continue;
      if (cur[ci].id == null || isNaN(Number(cur[ci].id))) toAdd.push(pth);
    }

    var chain = Promise.resolve();
    var di;
    for (di = 0; di < toDelete.length; di++) {
      (function (pathId) {
        chain = chain.then(function () {
          return fetch("/api/ui/indexer/workspace-paths/" + pathId, {
            method: "DELETE",
            credentials: "same-origin"
          }).then(function (res) {
            return res.json().then(function (j) {
              if (!res.ok) throw new Error((j && j.error) || res.statusText || "delete path failed");
            });
          });
        });
      })(toDelete[di]);
    }
    var ai;
    for (ai = 0; ai < toAdd.length; ai++) {
      (function (absPath) {
        chain = chain.then(function () {
          return fetch("/api/ui/indexer/workspaces/" + wsNum + "/paths", {
            method: "POST",
            credentials: "same-origin",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ path: absPath })
          }).then(function (res) {
            return res.json().then(function (j) {
              if (!res.ok) throw new Error((j && j.error) || res.statusText || "add path failed");
              return j;
            });
          });
        });
      })(toAdd[ai]);
    }
    chain
      .then(function () {
        workspaceManagedEditId = null;
        workspaceManagedStaging = null;
        notifyWorkspaceDraftMsg("Workspace updated.", false);
        return refreshOperatorIndexerWorkspaceStateFromConfig();
      })
      .catch(function (err) {
        notifyWorkspaceDraftMsg(err && err.message ? err.message : String(err), true);
      });
  }

  function deleteManagedWorkspace(wsNum) {
    if (
      !window.confirm(
        "Delete this workspace and all watched paths from configuration? The indexer will stop indexing these paths."
      )
    ) {
      return;
    }
    fetch("/api/ui/indexer/workspaces/" + wsNum, { method: "DELETE", credentials: "same-origin" })
      .then(function (res) {
        return res.json().then(function (j) {
          if (!res.ok) throw new Error((j && j.error) || res.statusText || "delete failed");
        });
      })
      .then(function () {
        workspaceManagedEditId = null;
        workspaceManagedStaging = null;
        notifyWorkspaceDraftMsg("Workspace removed.", false);
        return refreshOperatorIndexerWorkspaceStateFromConfig();
      })
      .catch(function (err) {
        notifyWorkspaceDraftMsg(err && err.message ? err.message : String(err), true);
      });
  }

  function buildIndexerOperatorWorkspaceCard(ws, partitionRegistry) {
    var bucketId = operatorWorkspaceSyntheticBucketId(ws);
    var scopedEvs = filterEventsForIndexerScopeFullLog(entryCache, bucketId, partitionRegistry);
    var syntheticRun = { id: bucketId, events: entryCache };
    var meta = collectIndexerRunMeta(bucketId, scopedEvs, null);
    meta = mergePersistedIndexerWatchRoots(meta, scopedEvs, bucketId);
    meta = mergeOperatorStorePathsIntoIndexerMeta(meta);
    var opPaths = operatorWorkspacePaths(ws);
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
      workspaceManagedEditId != null &&
      workspaceManagedEditId === wsNum &&
      workspaceManagedStaging != null &&
      workspaceManagedStaging.wsNum === wsNum;
    var pathsBlockHtml = null;
    if (isEdit) {
      pathsBlockHtml = buildManagedWorkspacePathsEditHtml(wsNum, workspaceManagedStaging.paths);
    }
    var toolbar = buildManagedWorkspaceToolbarHtml(wsNum, isEdit);
    var wsRowKey = canonicalWorkspaceRowIdKey(ws.id);
    var expanded = renderExpandedIndexer(syntheticRun, entryCache, meta, partitionRegistry, {
      kvOpts: { omitFileCountIfZero: true, workspaceRowId: wsRowKey },
      recentOpts: { omitWhenEmpty: true },
      pathsBlockHtml: pathsBlockHtml,
      extraAfterSummaryHtml: toolbar
    });
    var cardCls =
      "sum-card sum-card--workspace-operator" + (isEdit ? " sum-card--workspace-operator-editing" : "");
    return (
      '<details class="' +
      cardCls +
      '" id="' +
      escapeHtml(iid) +
      '" open data-workspace-managed-id="' +
      escapeHtml(String(wsNum)) +
      '">' +
      '<summary>' +
      '<span class="sum-avatar sum-av-c" title="Managed workspace">WS</span>' +
      '<span class="sum-main"><span class="sum-title">' +
      '<span class="sum-title-indexer-head">' +
      escapeHtml(titleText) +
      "</span>" +
      '</span><span class="sum-sub sum-sub--clamp muted">' +
      escapeHtml(subProse) +
      "</span></span>" +
      '<span class="sum-chev"></span></summary>' +
      expanded +
      "</details>"
    );
  }

  function buildIndexerCard(run, partitionRegistry) {
    var evs = run.events;
    var pmeta = null;
    if (
      partitionRegistry &&
      globalThis.ClaudiaLogs &&
      ClaudiaLogs.Derive &&
      typeof ClaudiaLogs.Derive.indexerPartitionMetaForRun === "function"
    ) {
      pmeta = ClaudiaLogs.Derive.indexerPartitionMetaForRun(partitionRegistry, run.id, evs, getFlat);
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
    var opWsForIx = findOperatorWorkspaceMatchingIndexerMeta(meta);
    var wsNumIx = opWsForIx ? operatorWorkspaceNumericId(opWsForIx) : 0;
    if (opWsForIx) {
      meta.userLabel = resolveLogsOperatorUserLabel();
      meta.projectId =
        opWsForIx.project_id != null ? String(opWsForIx.project_id).trim() : meta.projectId;
      meta.flavorId =
        opWsForIx.flavor_id != null && String(opWsForIx.flavor_id).trim() !== ""
          ? String(opWsForIx.flavor_id).trim()
          : "—";
      meta.workspaceId = canonicalWorkspaceRowIdKey(opWsForIx.id) || meta.workspaceId;
    }
    var isIxEdit =
      wsNumIx > 0 &&
      workspaceManagedEditId != null &&
      workspaceManagedEditId === wsNumIx &&
      workspaceManagedStaging != null &&
      workspaceManagedStaging.wsNum === wsNumIx;
    var pathsBlockIx = null;
    if (isIxEdit) {
      pathsBlockIx = buildManagedWorkspacePathsEditHtml(wsNumIx, workspaceManagedStaging.paths);
    }
    var toolbarIx = wsNumIx > 0 ? buildManagedWorkspaceToolbarHtml(wsNumIx, isIxEdit) : "";
    var lastProg = meta.lastProg;
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
    var titleText = workspaceCardTitleFromIndexerMeta(meta);
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
    var progressStack = indexerCollapsedIdle
      ? ""
      : '<div class="indexer-scope-progress" title="Scoped: ingest queue + fan-out file rows pending vs workspace files (from indexer.scope.status)">' +
      progressBarHtml +
      captionSpan +
      "</div>";
    var avatarIndexer = indexerCollapsedIdle
      ? '<span class="sum-avatar sum-av-c sum-av-indexer-idle" aria-hidden="true">\u2713</span>'
      : '<span class="sum-avatar sum-av-c">IX</span>';
    var statusSpan = indexerCollapsedIdle
      ? ""
      : '<span class="sum-status ' + st.cls + '">' + escapeHtml(st.st) + "</span>";
    var iid = "ix-" + strHash(run.id);
    rememberIndexerCardSnapshot(run.id, meta);
    var detailsCls = "sum-card";
    if (wsNumIx > 0) detailsCls += " sum-card--indexer-operator-workspace";
    if (isIxEdit) detailsCls += " sum-card--workspace-operator-editing";
    var dataManagedAttr =
      wsNumIx > 0 ? ' data-workspace-managed-id="' + escapeHtml(String(wsNumIx)) + '"' : "";
    var expOptsIx = {
      kvOpts: {
        omitFileCountIfZero: true,
        workspaceRowId: wsNumIx > 0 ? canonicalWorkspaceRowIdKey(opWsForIx.id) : undefined
      },
      recentOpts: { omitWhenEmpty: true },
      pathsBlockHtml: pathsBlockIx,
      extraAfterSummaryHtml: toolbarIx
    };
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
      progressStack +
      "</span>" +
      statusSpan +
      '<span class="sum-chev"></span></summary>' +
      renderExpandedIndexer(run, evs, meta, partitionRegistry, expOptsIx) +
      "</details>"
    );
  }

  function convLastTs(g) {
    var mx = 0;
    for (var u = 0; u < g.events.length; u++) {
      var ti = entryInstant({ ts: g.events[u].ts });
      if (ti) mx = Math.max(mx, ti.getTime());
    }
    return mx;
  }

  function convFirstTs(g) {
    var mn = null;
    for (var u = 0; u < g.events.length; u++) {
      var ti = entryInstant({ ts: g.events[u].ts });
      if (ti) {
        if (mn == null || ti.getTime() < mn.getTime()) mn = ti;
      }
    }
    return mn ? mn.getTime() : 0;
  }

  /**
   * One conversation card per gateway group key (principal + conversation_id).
   * Do not merge different conversation_ids for the same principal by time gap — that hid separate chats in the log UI.
   */
  function sortConversationGroupsByRecency(groups) {
    var arr = [];
    for (var key in groups) {
      if (!Object.prototype.hasOwnProperty.call(groups, key)) continue;
      var gx = groups[key];
      var tmin = convFirstTs(gx);
      var tmax = convLastTs(gx);
      if (!tmax) continue;
      if (!tmin) tmin = tmax;
      arr.push({
        pid: gx.pid,
        cid: gx.cid,
        cids: [gx.cid],
        events: gx.events.slice(),
        tmin: tmin,
        tmax: tmax
      });
    }
    arr.sort(function (a, b) {
      return b.tmax - a.tmax;
    });
    for (var k = 0; k < arr.length; k++) {
      arr[k].events.sort(function (a, b) {
        var sa = a.seq != null ? Number(a.seq) : 0;
        var sb = b.seq != null ? Number(b.seq) : 0;
        if (sa !== sb) return sa - sb;
        var ta = entryInstant({ ts: a.ts });
        var tb = entryInstant({ ts: b.ts });
        if (!ta && !tb) return 0;
        if (!ta) return -1;
        if (!tb) return 1;
        return ta.getTime() - tb.getTime();
      });
    }
    return arr;
  }

  function formatMergedConversationSubtitle(mergedCount) {
    if (!mergedCount || mergedCount <= 1) return "";
    return (
      ' <span class="muted" style="font-size:0.85em" title="Multiple conversation ids in one card (unusual).">(' +
      mergedCount +
      " ids)</span>"
    );
  }

  /** Structured indexer stderr lines sometimes omit service=indexer; still bucket under Services → Indexer. */
  function entryIsIndexerLine(ent) {
    var f = getFlat(ent.parsed);
    if (String(f.service || "").toLowerCase() === "indexer") return true;
    if (ent && String(ent.source || "").toLowerCase() === "indexer") return true;
    var rawMsg = f.msg != null ? f.msg : f.message != null ? f.message : "";
    var msg = String(rawMsg).toLowerCase().trim();
    if (msg.indexOf("indexer.") === 0) return true;
    if (msg.indexOf("gateway.indexer") === 0) return true;
    return false;
  }

  /** Normalize indexer/Gateway scope flavor placeholders so "" matches UI "—". */
  function normalizeIndexerScopeFlavor(v) {
    var s = v != null ? String(v) : "";
    s = s.replace(/\s+/g, " ").trim();
    if (!s) return "";
    if (s === "—" || s === "\u2014" || s === "-" || s.toLowerCase() === "none") return "";
    return s;
  }

  function rebuildIndexerRootScopeMaps() {
    indexerRootScopeByRootId = {};
    if (
      !globalThis.ClaudiaLogs ||
      !ClaudiaLogs.Derive ||
      typeof ClaudiaLogs.Derive.indexerParseRootScopes !== "function"
    ) {
      return;
    }
    var gi;
    for (gi = 0; gi < entryCache.length; gi++) {
      var ent = entryCache[gi];
      if (!entryIsIndexerLine(ent)) continue;
      var raw = getFlat(ent.parsed);
      var msg = String(raw.msg != null ? raw.msg : raw.message != null ? raw.message : "")
        .toLowerCase()
        .trim();
      if (msg !== "indexer.run.start" && msg !== "indexer run start") continue;
      var rows = ClaudiaLogs.Derive.indexerParseRootScopes(raw.root_scopes);
      var ri;
      for (ri = 0; ri < rows.length; ri++) {
        var row = rows[ri];
        if (!row || typeof row !== "object") continue;
        var rslug = row.root_id != null ? String(row.root_id).trim() : "";
        if (!rslug) continue;
        indexerRootScopeByRootId[rslug] = {
          workspace_id: row.workspace_id != null ? String(row.workspace_id).trim() : "",
          path: row.path != null ? String(row.path).trim() : "",
          ingest_project: row.ingest_project != null ? String(row.ingest_project).trim() : "",
          flavor_id: row.flavor_id != null ? String(row.flavor_id).trim() : ""
        };
      }
    }
  }

  /** Normalize watched root prefix for matching indexer `root` paths (Windows-safe). */
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

  /** Infer gateway tenant id for Qdrant collection naming on synthetic operator workspace buckets. */
  function inferTenantForOpwsBucket(bucketId) {
    var segs = String(bucketId || "").split("\u001e");
    if (segs[0] !== "opws" || segs.length < 3) return "";
    var wantWid = String(segs[1] || "").trim();
    var wantProj = String(segs[2] || "").trim();
    var wantFlav = segs.length > 3 ? normalizeIndexerScopeFlavor(segs[3]) : "";
    var ctx = operatorWsFullLogCtx[bucketId];
    var roots = ctx && ctx.paths ? ctx.paths : [];
    var ei;
    for (ei = entryCache.length - 1; ei >= 0; ei--) {
      var ent = entryCache[ei];
      if (!entryIsIndexerLine(ent)) continue;
      var raw = getFlat(ent.parsed);
      var f =
        globalThis.ClaudiaLogs &&
          ClaudiaLogs.Derive &&
          typeof ClaudiaLogs.Derive.indexerAugmentFlat === "function"
          ? ClaudiaLogs.Derive.indexerAugmentFlat(ent, raw)
          : raw;
      var fp = String(f.project_id || f.ingest_project || "").trim();
      var ff = normalizeIndexerScopeFlavor(f.flavor_id);
      if (fp !== wantProj || ff !== wantFlav) continue;
      var sw = f.scope_workspace_id != null ? String(f.scope_workspace_id).trim() : "";
      if (wantWid && sw === wantWid) {
        return String(f.tenant_id || f.principal_id || f.tenant || "").trim();
      }
      var rk = f.root != null ? String(f.root).trim() : "";
      if (wantWid && rk && indexerRootScopeByRootId[rk]) {
        var rsi = indexerRootScopeByRootId[rk];
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

  function indexerOperatorWorkspaceScopeMatch(ent, bucketId, f) {
    if (!entryIsIndexerLine(ent)) return false;
    var segs = String(bucketId || "").split("\u001e");
    if (segs[0] !== "opws" || segs.length < 3) return false;
    var wantWid = String(segs[1] || "").trim();
    var wantProj = String(segs[2] || "").trim();
    var wantFlav = segs.length > 3 ? normalizeIndexerScopeFlavor(segs[3]) : "";
    var fp = String(f.project_id || f.ingest_project || "").trim();
    var ff = normalizeIndexerScopeFlavor(f.flavor_id);
    if (fp !== wantProj || ff !== wantFlav) return false;
    var sw = f.scope_workspace_id != null ? String(f.scope_workspace_id).trim() : "";
    if (wantWid && sw === wantWid) return true;
    var ctx = operatorWsFullLogCtx[bucketId];
    var roots = ctx && ctx.paths ? ctx.paths : [];
    var rk = f.root != null ? String(f.root).trim() : "";
    if (wantWid && rk && indexerRootScopeByRootId[rk]) {
      var rsi = indexerRootScopeByRootId[rk];
      if (String(rsi.workspace_id || "") === wantWid) return true;
      if (rsi.path && roots.length && rootUnderOneOfPrefixes(rsi.path, roots)) return true;
    }
    return false;
  }

  /** Synthetic bucket id so operator-managed workspaces reuse scoped full-log filtering over entryCache. */
  function operatorWorkspaceSyntheticBucketId(ws) {
    var wid = canonicalWorkspaceRowIdKey(ws.id);
    var pj = String(ws.project_id != null ? ws.project_id : "").trim();
    var fvKey = normalizeIndexerScopeFlavor(ws.flavor_id);
    var bucketId = "opws\u001e" + wid + "\u001e" + pj + "\u001e" + fvKey;
    operatorWsFullLogCtx[bucketId] = { paths: operatorWorkspacePaths(ws).slice() };
    return bucketId;
  }

  /** Tenant/project/flavor for an indexer bucket (same derivation as Qdrant collection naming). */
  function indexerBucketScopeCoords(bucketId, evs, partitionRegistry) {
    bucketId = bucketId != null ? String(bucketId).trim() : "";
    evs = Array.isArray(evs) ? evs : [];
    if (bucketId.indexOf("opws\u001e") === 0) {
      var opSegs = bucketId.split("\u001e");
      if (opSegs.length >= 3) {
        var opProj = String(opSegs[2] || "").trim();
        var opFlav = opSegs.length > 3 ? normalizeIndexerScopeFlavor(opSegs[3]) : "";
        var opTenant = inferTenantForOpwsBucket(bucketId);
        if (opTenant && opProj && opProj !== "—") {
          return { tenant: opTenant, project: opProj, flavor: opFlav };
        }
      }
    }
    var syn =
      globalThis.ClaudiaLogs &&
        ClaudiaLogs.Derive &&
        typeof ClaudiaLogs.Derive.parseIgSyntheticGid === "function"
        ? ClaudiaLogs.Derive.parseIgSyntheticGid(bucketId)
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
          globalThis.ClaudiaLogs &&
            ClaudiaLogs.Derive &&
            typeof ClaudiaLogs.Derive.indexerAugmentFlat === "function"
            ? ClaudiaLogs.Derive.indexerAugmentFlat(evs[i], rawF)
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
            globalThis.ClaudiaLogs &&
              ClaudiaLogs.Derive &&
              typeof ClaudiaLogs.Derive.indexerAugmentFlat === "function"
              ? ClaudiaLogs.Derive.indexerAugmentFlat(evs[i], rawG)
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

  function indexerExpectedQdrantCollectionForBucket(bucketId, evs, partitionRegistry) {
    var c = indexerBucketScopeCoords(bucketId, evs, partitionRegistry);
    if (!c) return "";
    if (
      globalThis.ClaudiaLogs &&
      ClaudiaLogs.Derive &&
      typeof ClaudiaLogs.Derive.qdrantCollectionName === "function"
    ) {
      return ClaudiaLogs.Derive.qdrantCollectionName(c.tenant, c.project, c.flavor);
    }
    return "";
  }

  /**
   * Per-scope indexer cards: Full event log lists only lines attributable to that scope.
   * Lines attributed to every scope (multi-bucket fan-out) stay in Services → Indexer only.
   * expectedQdrantCollection — from indexerExpectedQdrantCollectionForBucket, or "" to skip Qdrant routing.
   */
  function indexerScopeFullLogInclude(ent, bucketId, partitionRegistry, expectedQdrantCollection, bucketScopeCoords) {
    bucketId = bucketId != null ? String(bucketId).trim() : "";
    if (!bucketId) return true;

    var rawFlat = getFlat(ent.parsed);
    var f = rawFlat;
    if (
      globalThis.ClaudiaLogs &&
      ClaudiaLogs.Derive &&
      typeof ClaudiaLogs.Derive.indexerAugmentFlat === "function"
    ) {
      f = ClaudiaLogs.Derive.indexerAugmentFlat(ent, rawFlat);
    }

    var srcL = String(ent.source || "").toLowerCase();
    var svcL = String(f.service || "").toLowerCase();
    if (srcL === "qdrant" || svcL === "qdrant") {
      var coll = f.collection != null ? String(f.collection).trim() : "";
      var exp = expectedQdrantCollection != null ? String(expectedQdrantCollection).trim() : "";
      if (!coll || !exp) return false;
      return coll === exp;
    }

    if (bucketId.indexOf("opws\u001e") === 0) {
      return indexerOperatorWorkspaceScopeMatch(ent, bucketId, f);
    }

    var rid = f.index_run_id != null ? String(f.index_run_id).trim() : "";
    var st = rid && partitionRegistry && partitionRegistry[rid] ? partitionRegistry[rid] : null;

    if (
      globalThis.ClaudiaLogs &&
      ClaudiaLogs.Derive &&
      typeof ClaudiaLogs.Derive.indexerBucketGidsForLine === "function" &&
      st &&
      st.keys &&
      st.keys.length > 0
    ) {
      var gids = ClaudiaLogs.Derive.indexerBucketGidsForLine(f, st);
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
      globalThis.ClaudiaLogs &&
      ClaudiaLogs.Derive &&
      typeof ClaudiaLogs.Derive.indexerGroupKeyFromFlat === "function"
    ) {
      var gk = ClaudiaLogs.Derive.indexerGroupKeyFromFlat(f);
      if (gk != null && String(gk).trim() === bucketId) return true;
    }

    if (bucketId.indexOf("ig\u001e") === 0) {
      var parts = bucketId.split("\u001e");
      if (parts.length >= 4) {
        var wantP = parts[2] || "";
        var wantF = parts[3] || "";
        var fp = String(f.project_id != null ? f.project_id : f.ingest_project != null ? f.ingest_project : "").trim();
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

  function filterEventsForIndexerScopeFullLog(evs, bucketId, partitionRegistry) {
    var out = [];
    if (!Array.isArray(evs)) return out;
    var expColl = indexerExpectedQdrantCollectionForBucket(bucketId, evs, partitionRegistry);
    var bucketCoords = indexerBucketScopeCoords(bucketId, evs, partitionRegistry);
    for (var i = 0; i < evs.length; i++) {
      if (indexerScopeFullLogInclude(evs[i], bucketId, partitionRegistry, expColl, bucketCoords)) out.push(evs[i]);
    }
    return out;
  }

  /** Gateway logs upstream relay with service=gateway; bucket those lines under bifrost so the card updates with chat traffic. */
  function entryIsGatewayUpstreamRelay(ent) {
    var f = getFlat(ent.parsed);
    var msg = String(f.msg != null ? f.msg : f.message != null ? f.message : "").trim();
    if (
      msg === "chat.bifrost.request" ||
      msg === "upstream chat response" ||
      msg === "chat.bifrost.response" ||
      msg === "chat.bifrost.error" ||
      msg.indexOf("bifrost.error") >= 0
    ) {
      return true;
    }
    var sh = ent.parsed.shape || "";
    if (sh === "chat.bifrost" || sh.indexOf("chat.bifrost.") === 0) return true;
    return false;
  }

  /** Routing / virtual-model lines that belong on the BiFrost service card with relay traffic. */
  function entryRoutesToBifrostBucket(ent) {
    if (entryIsGatewayUpstreamRelay(ent)) return true;
    var f = getFlat(ent.parsed);
    var msg = String(f.msg != null ? f.msg : f.message != null ? f.message : "").trim();
    if (msg === "chat.bifrost.available_models") return true;
    if (msg === "chat.routing.fallback") return true;
    if (msg === "chat.routing.attempt") return true;
    if (msg === "chat.routing.resolved") return true;
    if (msg === "chat.provider_limits.blocked") return true;
    if (msg.indexOf("virtual model fallback attempt") >= 0) return true;
    if (msg.indexOf("virtual model routing resolved") >= 0) return true;
    return false;
  }

  /** Stable /ui/logs Workspaces bucket: backend indexer_key or tenant + project + flavor fallback. */
  function indexerGroupIdForFlat(fR) {
    if (
      globalThis.ClaudiaLogs &&
      ClaudiaLogs.Derive &&
      typeof ClaudiaLogs.Derive.indexerGroupKeyFromFlat === "function"
    ) {
      var gx = ClaudiaLogs.Derive.indexerGroupKeyFromFlat(fR);
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

  function renderSummarizedUnified() {
    operatorWsFullLogCtx = {};
    rebuildIndexerRootScopeMaps();
    var groups = {};
    var reqToConv = {};
    var indexRunToConv = {};
    var gix;
    for (gix = 0; gix < entryCache.length; gix++) {
      tryRegisterRequestConversationCorrelationPrimary(reqToConv, getFlat(entryCache[gix].parsed));
    }
    for (gix = 0; gix < entryCache.length; gix++) {
      tryRegisterRequestConversationCorrelationRagFallback(reqToConv, getFlat(entryCache[gix].parsed));
    }
    for (gix = 0; gix < entryCache.length; gix++) {
      var entIr = entryCache[gix];
      var fIr = getFlat(entIr.parsed);
      var msgIr = String(fIr.msg != null ? fIr.msg : fIr.message != null ? fIr.message : "").trim();
      if (msgIr !== "ingest.complete" && msgIr !== "ingest.failed" && msgIr !== "ingest.chunked.error") continue;
      var irKey = fIr.index_run_id != null ? String(fIr.index_run_id).trim() : "";
      var cidIr = fIr.conversation_id != null ? String(fIr.conversation_id).trim() : "";
      var pidIr =
        fIr.principal_id != null ? String(fIr.principal_id).trim() : fIr.tenant != null ? String(fIr.tenant).trim() : "";
      if (irKey && cidIr && pidIr && !indexRunToConv[irKey]) indexRunToConv[irKey] = { pid: pidIr, cid: cidIr };
    }
    for (gix = 0; gix < entryCache.length; gix++) {
      var ent = entryCache[gix];
      var p = ent.parsed;
      var f = getFlat(p);
      var cid = f.conversation_id != null ? String(f.conversation_id).trim() : "";
      var pid = f.principal_id != null ? String(f.principal_id).trim() : f.tenant != null ? String(f.tenant).trim() : "";
      if (cid) {
        pushConversationGroupedEvent(groups, pid, cid, ent, p, "direct");
        continue;
      }
      var ridJoin = f.request_id != null ? String(f.request_id).trim() : "";
      if (ridJoin && reqToConv[ridJoin] && conversationRequestIdTier2EligibleLocal(f)) {
        pushConversationGroupedEvent(groups, reqToConv[ridJoin].pid, reqToConv[ridJoin].cid, ent, p, "request_id");
        continue;
      }
      var irJoin = f.index_run_id != null ? String(f.index_run_id).trim() : "";
      if (irJoin && indexRunToConv[irJoin] && conversationIndexRunTier3EligibleLocal(f)) {
        pushConversationGroupedEvent(
          groups,
          indexRunToConv[irJoin].pid,
          indexRunToConv[irJoin].cid,
          ent,
          p,
          "ingest"
        );
      }
    }
    var gkSort;
    for (gkSort in groups) {
      if (!Object.prototype.hasOwnProperty.call(groups, gkSort)) continue;
      groups[gkSort].events.sort(function (a, b) {
        var sa = a.seq != null ? Number(a.seq) : 0;
        var sb = b.seq != null ? Number(b.seq) : 0;
        if (sa !== sb) return sa - sb;
        var ta = entryInstant({ ts: a.ts });
        var tb = entryInstant({ ts: b.ts });
        if (!ta && !tb) return 0;
        if (!ta) return -1;
        if (!tb) return 1;
        return ta.getTime() - tb.getTime();
      });
    }
    if (
      globalThis.ClaudiaLogs &&
      ClaudiaLogs.Derive &&
      typeof ClaudiaLogs.Derive.joinQdrantLineConversationTier === "function"
    ) {
      for (gix = 0; gix < entryCache.length; gix++) {
        var entQ = entryCache[gix];
        if (!entryIsQdrantSubprocessForConvJoin(entQ)) continue;
        var fQ = getFlat(entQ.parsed);
        var collQ = fQ.collection != null ? String(fQ.collection).trim() : "";
        if (!collQ) continue;
        var tQ = entryInstant({ ts: entQ.ts });
        if (!tQ) continue;
        var tMs = tQ.getTime();
        var gkQ;
        for (gkQ in groups) {
          if (!Object.prototype.hasOwnProperty.call(groups, gkQ)) continue;
          var grp = groups[gkQ];
          var qMatch = null;
          if (typeof ClaudiaLogs.Derive.joinQdrantLineConversationMatch === "function") {
            qMatch = ClaudiaLogs.Derive.joinQdrantLineConversationMatch(grp.events, getFlat, fQ, tMs);
          }
          var tierQ = qMatch && qMatch.tier ? qMatch.tier : ClaudiaLogs.Derive.joinQdrantLineConversationTier(grp.events, getFlat, fQ, tMs);
          if (tierQ) {
            pushConversationGroupedEvent(groups, grp.pid, grp.cid, entQ, entQ.parsed, tierQ, qMatch);
          }
        }
      }
      for (gkSort in groups) {
        if (!Object.prototype.hasOwnProperty.call(groups, gkSort)) continue;
        groups[gkSort].events.sort(function (a, b) {
          var sa = a.seq != null ? Number(a.seq) : 0;
          var sb = b.seq != null ? Number(b.seq) : 0;
          if (sa !== sb) return sa - sb;
          var ta2 = entryInstant({ ts: a.ts });
          var tb2 = entryInstant({ ts: b.ts });
          if (!ta2 && !tb2) return 0;
          if (!ta2) return -1;
          if (!tb2) return 1;
          return ta2.getTime() - tb2.getTime();
        });
      }
    }
    var buckets = { gateway: [], qdrant: [], bifrost: [], indexer: [] };
    for (var bi = 0; bi < entryCache.length; bi++) {
      var entB = entryCache[bi];
      var pB = entB.parsed;
      var fB = getFlat(pB);
      var svc = fB.service ? String(fB.service) : "";
      if (entryRoutesToBifrostBucket(entB)) svc = "bifrost";
      else if (entryIsIndexerLine(entB)) svc = "indexer";
      else if (!buckets[svc]) {
        svc = entB.source || "gateway";
        if (!buckets[svc]) svc = "gateway";
      }
      buckets[svc].push(entB);
    }
    var byRun = {};
    var partitionRegistry = {};
    var ibuilt = null;
    if (
      globalThis.ClaudiaLogs &&
      ClaudiaLogs.Derive &&
      typeof ClaudiaLogs.Derive.indexerBucketsFromCache === "function"
    ) {
      ibuilt = ClaudiaLogs.Derive.indexerBucketsFromCache(entryCache, getFlat);
      if (ibuilt && ibuilt.targetStateByRunId) partitionRegistry = ibuilt.targetStateByRunId;
      if (ibuilt && ibuilt.buckets) byRun = ibuilt.buckets;
    }
    if (!ibuilt || !Object.keys(byRun).length) {
      byRun = {};
      partitionRegistry = {};
      for (var ri = 0; ri < entryCache.length; ri++) {
        var entRL = entryCache[ri];
        var fRL = getFlat(entRL.parsed);
        var groupIdL = indexerGroupIdForFlat(fRL);
        if (!groupIdL) continue;
        if (!byRun[groupIdL]) byRun[groupIdL] = { id: groupIdL, events: [] };
        byRun[groupIdL].events.push(entRL);
      }
    } else {
      for (var normK in byRun) {
        if (!Object.prototype.hasOwnProperty.call(byRun, normK)) continue;
        var arrN = byRun[normK];
        byRun[normK] = { id: normK, events: arrN };
      }
    }
    lastIndexerSummarizeByRun = byRun;
    lastIndexerSummarizePartitionRegistry = partitionRegistry;
    var qFan = buckets.qdrant;
    if (qFan && qFan.length && byRun && Object.keys(byRun).length) {
      var collByRun = {};
      var buck;
      for (buck in byRun) {
        if (!Object.prototype.hasOwnProperty.call(byRun, buck)) continue;
        var runB = byRun[buck];
        collByRun[buck] = indexerExpectedQdrantCollectionForBucket(runB.id, runB.events, partitionRegistry);
      }
      var qx, qb;
      for (qx = 0; qx < qFan.length; qx++) {
        var qEnt = qFan[qx];
        var qFl = getFlat(qEnt.parsed);
        var qCol = qFl.collection != null ? String(qFl.collection).trim() : "";
        if (!qCol) continue;
        for (qb in byRun) {
          if (!Object.prototype.hasOwnProperty.call(byRun, qb)) continue;
          if (collByRun[qb] === qCol) byRun[qb].events.push(qEnt);
        }
      }
      for (var qs in byRun) {
        if (!Object.prototype.hasOwnProperty.call(byRun, qs)) continue;
        byRun[qs].events.sort(function (a, b) {
          var sa = a.seq != null ? Number(a.seq) : 0;
          var sb = b.seq != null ? Number(b.seq) : 0;
          if (sa !== sb) return sa - sb;
          var ta = entryInstant({ ts: a.ts });
          var tb = entryInstant({ ts: b.ts });
          if (!ta && !tb) return 0;
          if (!ta) return -1;
          if (!tb) return 1;
          return ta.getTime() - tb.getTime();
        });
      }
    }

    // Attach gateway RAG retrieval lines to the right indexer bucket so the
    // Workspaces card "Recently evaluated files" can show retrieved sources.
    var gFan = buckets.gateway;
    if (gFan && gFan.length && byRun && Object.keys(byRun).length) {
      // Build bucket → {tenant, project, flavor} map from existing indexer events.
      var scopeByRun = {};
      var rkx;
      var runIds = Object.keys(byRun);
      for (rkx = 0; rkx < runIds.length; rkx++) {
        var runX = byRun[runIds[rkx]];
        if (!runX || !runX.events) continue;
        var pmX = null;
        if (
          partitionRegistry &&
          globalThis.ClaudiaLogs &&
          ClaudiaLogs.Derive &&
          typeof ClaudiaLogs.Derive.indexerPartitionMetaForRun === "function"
        ) {
          pmX = ClaudiaLogs.Derive.indexerPartitionMetaForRun(partitionRegistry, runX.id, runX.events, getFlat);
        }
        var metaX = collectIndexerRunMeta(runX.id, runX.events, pmX);
        if (!metaX) continue;
        scopeByRun[runX.id] = {
          tenant: metaX.tenantId != null ? String(metaX.tenantId).trim() : "",
          project: metaX.projectId != null ? String(metaX.projectId).trim() : "",
          flavor: metaX.flavorId != null ? String(metaX.flavorId).trim() : ""
        };
      }
      var gx, gb;
      for (gx = 0; gx < gFan.length; gx++) {
        var gEnt = gFan[gx];
        var gFl = getFlat(gEnt.parsed);
        var gMsg = String(gFl.msg != null ? gFl.msg : gFl.message != null ? gFl.message : "").trim();
        if (gMsg !== "rag.retrieve.source") continue;
        var gt = String(gFl.tenant_id != null ? gFl.tenant_id : gFl.principal_id != null ? gFl.principal_id : "").trim();
        var gp = String(gFl.project_id != null ? gFl.project_id : gFl.project != null ? gFl.project : "").trim();
        var gf = String(gFl.flavor_id != null ? gFl.flavor_id : "").trim();
        if (!gt || !gp) continue;
        for (gb in byRun) {
          if (!Object.prototype.hasOwnProperty.call(byRun, gb)) continue;
          var sc = scopeByRun[gb];
          if (!sc || !sc.project) continue;
          if (
            (sc.tenant && sc.tenant !== gt) ||
            sc.project !== gp ||
            normalizeIndexerScopeFlavor(sc.flavor) !== normalizeIndexerScopeFlavor(gf)
          )
            continue;
          byRun[gb].events.push(gEnt);
        }
      }

      // Keep chronological order after attaching.
      for (var gsort in byRun) {
        if (!Object.prototype.hasOwnProperty.call(byRun, gsort)) continue;
        byRun[gsort].events.sort(function (a, b) {
          var sa = a.seq != null ? Number(a.seq) : 0;
          var sb = b.seq != null ? Number(b.seq) : 0;
          if (sa !== sb) return sa - sb;
          var ta = entryInstant({ ts: a.ts });
          var tb = entryInstant({ ts: b.ts });
          if (!ta && !tb) return 0;
          if (!ta) return -1;
          if (!tb) return 1;
          return ta.getTime() - tb.getTime();
        });
      }
    }
    var mergedConv = sortConversationGroupsByRecency(groups);
    var convTimeline = [];
    for (var ci = 0; ci < mergedConv.length; ci++) {
      var gx = mergedConv[ci];
      convTimeline.push({ sort: convLastTs(gx), html: buildConvCard(gx) });
    }
    convTimeline.sort(function (a, b) {
      return b.sort - a.sort;
    });
    var svcHtml = "";
    var order = ["bifrost", "gateway", "indexer", "qdrant"];
    for (var oi = 0; oi < order.length; oi++) {
      var nm = order[oi];
      var arr = buckets[nm];
      if (!arr || !arr.length) continue;
      svcHtml += buildServiceCard(nm, arr, { byRun: byRun, partitionRegistry: partitionRegistry });
    }
    var idxTimeline = [];
    var seenIndexerBuckets = {};
    var liveIndexerIdentities = {};
    var dedupeGroups = {};
    var rks = Object.keys(byRun);
    for (var rj = 0; rj < rks.length; rj++) {
      var runG = byRun[rks[rj]];
      if (!runG) continue;
      var pmetaG = null;
      if (
        partitionRegistry &&
        globalThis.ClaudiaLogs &&
        ClaudiaLogs.Derive &&
        typeof ClaudiaLogs.Derive.indexerPartitionMetaForRun === "function"
      ) {
        pmetaG = ClaudiaLogs.Derive.indexerPartitionMetaForRun(
          partitionRegistry,
          runG.id,
          runG.events,
          getFlat
        );
      }
      var metaG = collectIndexerRunMeta(runG.id, runG.events, pmetaG);
      metaG = mergePersistedIndexerWatchRoots(metaG, runG.events, runG.id);
      var dk = indexerRunTimelineDedupeKey(metaG, runG.id);
      if (!dedupeGroups[dk]) dedupeGroups[dk] = [];
      dedupeGroups[dk].push(runG);
    }
    var dkIter;
    var headlinesWithIndexerOrStaleCard = Object.create(null);
    for (dkIter in dedupeGroups) {
      if (!Object.prototype.hasOwnProperty.call(dedupeGroups, dkIter)) continue;
      var grpRuns = dedupeGroups[dkIter];
      var run = pickCanonicalIndexerRun(grpRuns);
      if (!run) continue;
      var gi;
      for (gi = 0; gi < grpRuns.length; gi++) {
        seenIndexerBuckets[grpRuns[gi].id] = true;
      }
      var pmetaLive = null;
      if (
        partitionRegistry &&
        globalThis.ClaudiaLogs &&
        ClaudiaLogs.Derive &&
        typeof ClaudiaLogs.Derive.indexerPartitionMetaForRun === "function"
      ) {
        pmetaLive = ClaudiaLogs.Derive.indexerPartitionMetaForRun(
          partitionRegistry,
          run.id,
          run.events,
          getFlat
        );
      }
      var metaLive = collectIndexerRunMeta(run.id, run.events, pmetaLive);
      metaLive = mergePersistedIndexerWatchRoots(metaLive, run.events, run.id);
      liveIndexerIdentities[indexerCardIdentityKey(metaLive)] = true;
      var ixHead = workspaceCardTitleFromIndexerMeta(metaLive);
      if (ixHead) headlinesWithIndexerOrStaleCard[ixHead] = true;
      var sortLabel = indexerCardTitleSortLabel(metaLive) + "\u0001" + String(run.id || "");
      idxTimeline.push({ sortKey: sortLabel, html: buildIndexerCard(run, partitionRegistry) });
    }
    var snapStore = loadIndexerWatchRootsStore();
    if (snapStore.snapshots) {
      for (var sbi in snapStore.snapshots) {
        if (!Object.prototype.hasOwnProperty.call(snapStore.snapshots, sbi)) continue;
        if (seenIndexerBuckets[sbi]) continue;
        var sn = snapStore.snapshots[sbi];
        if (liveIndexerIdentities[indexerCardIdentityKeyFromSnap(sn)]) continue;
        var staleHead = workspaceCardTitleFromIndexerMeta({
          userLabel: sn.userLabel,
          projectId: sn.projectId,
          flavorId: sn.flavorId
        });
        if (staleHead) headlinesWithIndexerOrStaleCard[staleHead] = true;
        idxTimeline.push({
          sortKey: indexerCardTitleSortLabel(sn) + "\u0001" + String(sbi),
          html: buildIndexerStaleSnapshotCard(sbi, sn)
        });
      }
    }
    var wsn = dedupeOperatorWorkspacesNested(lastIndexerOperatorWorkspacesNested.slice());
    wsn.sort(function (a, b) {
      var ak = canonicalWorkspaceRowIdKey(a.id);
      var bk = canonicalWorkspaceRowIdKey(b.id);
      var an = parseInt(ak, 10);
      var bn = parseInt(bk, 10);
      if (/^\d+$/.test(ak) && /^\d+$/.test(bk) && !isNaN(an) && !isNaN(bn)) return an - bn;
      return String(ak).localeCompare(String(bk));
    });
    var seenManagedWsTitle = Object.create(null);
    var wdx;
    for (wdx = 0; wdx < workspaceDrafts.length; wdx++) {
      var draftHead = workspaceDraftComparableManagedTitle(workspaceDrafts[wdx]);
      if (draftHead) seenManagedWsTitle[draftHead] = true;
    }
    if (wsn && wsn.length) {
      for (var owi = 0; owi < wsn.length; owi++) {
        var ows = wsn[owi];
        if (!ows || ows.id == null) continue;
        var headTtl = operatorManagedWorkspaceTitleText(ows);
        if (seenManagedWsTitle[headTtl]) continue;
        if (headlinesWithIndexerOrStaleCard[headTtl]) continue;
        seenManagedWsTitle[headTtl] = true;
        if (operatorWorkspaceCoveredByIndexerRuns(ows, byRun, partitionRegistry)) continue;
        var sortOp =
          headTtl + "\u0001opws-" + canonicalWorkspaceRowIdKey(ows.id);
        idxTimeline.push({
          sortKey: sortOp,
          html: buildIndexerOperatorWorkspaceCard(ows, partitionRegistry)
        });
      }
    }
    idxTimeline.sort(function (a, b) {
      var ka = a.sortKey != null ? String(a.sortKey) : "";
      var kb = b.sortKey != null ? String(b.sortKey) : "";
      return ka.localeCompare(kb, undefined, { sensitivity: "base", numeric: true });
    });
    var body = buildGatewayUsageFeedSection();
    if (convTimeline.length) {
      body +=
        '<div class="sum-feed-section"><div class="sum-section-label sum-feed-section-title">Conversations</div>';
      for (var zc = 0; zc < convTimeline.length; zc++) body += convTimeline[zc].html;
      body += "</div>";
    }
    body +=
      '<div class="sum-feed-section sum-feed-section--workspaces">' +
      '<div class="sum-feed-section-head">' +
      '<span class="sum-feed-section-title sum-section-label">Workspaces</span>' +
      '<button type="button" class="sum-workspaces-create-btn" data-sum-workspaces-create="1">Create</button>' +
      "</div>" +
      buildWorkspacesSectionIntroHtml();
    var wdi;
    for (wdi = 0; wdi < workspaceDrafts.length; wdi++) {
      body += buildWorkspaceDraftCardHtml(workspaceDrafts[wdi]);
    }
    for (var zi = 0; zi < idxTimeline.length; zi++) body += idxTimeline[zi].html;
    body += "</div>";
    if (svcHtml) {
      body +=
        '<div class="sum-feed-section"><div class="sum-section-label sum-feed-section-title">Services</div>' +
        svcHtml +
        "</div>";
    }
    var hasThreads =
      convTimeline.length > 0 ||
      idxTimeline.length > 0 ||
      svcHtml.length > 0 ||
      workspaceDrafts.length > 0;
    if (!hasThreads) {
      body +=
        '<p class="muted">No conversation / service cards in the <em>loaded</em> window yet. Chat traffic needs <code>conversation_id</code> in structured logs; <strong>scroll to the top</strong> of this feed to load older lines (indexer snapshots often crowd the recent tail). Switch to <strong>StructuredLogs</strong> for the full stream.</p>';
    }
    return body;
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
    if (globalThis.ClaudiaLogs && globalThis.ClaudiaLogs.Filters && globalThis.ClaudiaLogs.Filters.matchesRow) {
      if (!globalThis.ClaudiaLogs.Filters.matchesRow(filtersCtx, tr)) tr.style.display = "none";
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
    if (globalThis.ClaudiaLogs && globalThis.ClaudiaLogs.KeyValueGrid) {
      return globalThis.ClaudiaLogs.KeyValueGrid(extras);
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
    CLIENT_CACHE_MAX: CLIENT_CACHE_MAX,
    INITIAL_TAIL_LIMIT: INITIAL_TAIL_LIMIT,
    BACKFILL_CHUNK: BACKFILL_CHUNK,
    RENDER_CHUNK: RENDER_CHUNK,
    scheduleStoryRebuild: scheduleStoryRebuild,
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
    pollTimerRef: { value: pollTimer }
  };

  (function wireSummarizedEvlogPanels() {
    if (globalThis.__claudiaLogsSumEvlogWired) return;
    globalThis.__claudiaLogsSumEvlogWired = true;
    var searchTimers = typeof WeakMap !== "undefined" ? new WeakMap() : null;
    /** Pixels from tbody bottom to treat as "stuck to tail" when summarized panel is rebuilt. */
    var SUM_EVLOG_TB_TAIL_SLACK = 6;

    globalThis.sumEvlogCapturePanelState = function (container) {
      var out = {};
      if (!container || !container.querySelectorAll) return out;
      var roots = container.querySelectorAll("[data-sum-evlog-root]");
      for (var i = 0; i < roots.length; i++) {
        var root = roots[i];
        var tb = root.querySelector("tbody[data-sum-evlog-tbody]");
        if (!tb || !tb.id) continue;
        var q = "";
        try {
          var inpCap = root.querySelector(".sum-evlog__search");
          q = inpCap && inpCap.value != null ? String(inpCap.value) : "";
        } catch (eInC) {}
        var mode = "all";
        try {
          var stCap = root.querySelector("[data-evlog-filter-status]");
          mode = stCap && stCap.value ? String(stCap.value) : "all";
        } catch (eStC) {}
        var selectedIds = [];
        try {
          var picked = tb.querySelectorAll("tr[data-evlog-id].sum-evlog__row--selected");
          for (var pi = 0; pi < picked.length; pi++) {
            var did = picked[pi].getAttribute("data-evlog-id");
            if (did) selectedIds.push(did);
          }
        } catch (ePk) {}
        var anchorId = null;
        try {
          if (root._sumEvlogAnchor != null && root._sumEvlogAnchor >= 0) {
            var rowsA = tb.querySelectorAll("tr[data-evlog-id]");
            var ar = rowsA[root._sumEvlogAnchor];
            if (ar) anchorId = ar.getAttribute("data-evlog-id");
          }
        } catch (eAnc) {}
        var st = tb.scrollTop;
        var sh = tb.scrollHeight;
        var ch = tb.clientHeight;
        var nearBottom = sh - st - ch <= SUM_EVLOG_TB_TAIL_SLACK;
        out[tb.id] = {
          q: q,
          mode: mode,
          selectedIds: selectedIds,
          anchorId: anchorId,
          scrollTop: st,
          nearBottom: nearBottom
        };
      }
      return out;
    };

    globalThis.sumEvlogApplyPanelState = function (container, saved, opts) {
      opts = opts || {};
      var scrollOnly = !!opts.scrollOnly;
      if (!container || !saved || typeof saved !== "object") return;
      var keys = Object.keys(saved);
      for (var ki = 0; ki < keys.length; ki++) {
        var tid = keys[ki];
        var pack = saved[tid];
        if (!pack || typeof pack !== "object") continue;
        var tb = document.getElementById(tid);
        /* Boolean attribute `data-sum-evlog-tbody` has no value; getAttribute returns "" which is falsy. */
        if (!tb || !tb.hasAttribute || !tb.hasAttribute("data-sum-evlog-tbody")) {
          continue;
        }
        if (!container.contains(tb)) {
          continue;
        }
        var root = tb.closest("[data-sum-evlog-root]");
        if (!root) continue;

        if (scrollOnly) {
          var maxSo = Math.max(0, tb.scrollHeight - tb.clientHeight);
          if (pack.nearBottom) {
            tb.scrollTop = maxSo;
          } else {
            var wantSo = Number(pack.scrollTop);
            if (isNaN(wantSo)) wantSo = 0;
            tb.scrollTop = Math.min(wantSo, maxSo);
          }
          continue;
        }

        var inpAp = root.querySelector(".sum-evlog__search");
        if (inpAp) inpAp.value = pack.q != null ? String(pack.q) : "";
        var stSelAp = root.querySelector("[data-evlog-filter-status]");
        if (stSelAp && pack.mode) stSelAp.value = String(pack.mode);
        sumEvlogRebuildRoot(root);

        var selIds = pack.selectedIds;
        if (selIds && selIds.length) {
          var allR = tb.querySelectorAll("tr[data-evlog-id]");
          for (var ui = 0; ui < allR.length; ui++) {
            allR[ui].classList.remove("sum-evlog__row--selected");
          }
          for (var sj = 0; sj < selIds.length; sj++) {
            var wantId = selIds[sj];
            for (var sk = 0; sk < allR.length; sk++) {
              if (allR[sk].getAttribute("data-evlog-id") === wantId) {
                allR[sk].classList.add("sum-evlog__row--selected");
                break;
              }
            }
          }
        } else {
          var prevSel = tb.querySelectorAll("tr[data-evlog-id].sum-evlog__row--selected");
          for (var u2 = 0; u2 < prevSel.length; u2++) prevSel[u2].classList.remove("sum-evlog__row--selected");
        }

        var aid = pack.anchorId;
        if (aid != null && String(aid) !== "") {
          var rows2 = tb.querySelectorAll("tr[data-evlog-id]");
          root._sumEvlogAnchor = null;
          for (var ax = 0; ax < rows2.length; ax++) {
            if (rows2[ax].getAttribute("data-evlog-id") === aid) {
              root._sumEvlogAnchor = ax;
              break;
            }
          }
        } else {
          root._sumEvlogAnchor = null;
        }

        sumEvlogSyncFooter(root);

        if (opts.scroll !== false) {
          var maxT2 = Math.max(0, tb.scrollHeight - tb.clientHeight);
          if (pack.nearBottom) {
            tb.scrollTop = maxT2;
          } else {
            var want2 = Number(pack.scrollTop);
            if (isNaN(want2)) want2 = 0;
            tb.scrollTop = Math.min(want2, maxT2);
          }
        }
      }
    };
    function parseHttpAttr(attr) {
      if (attr == null || String(attr).trim() === "") return null;
      var n = parseInt(String(attr).trim(), 10);
      return isNaN(n) ? null : n;
    }
    function rowTimespec(tr) {
      var tEl = tr.querySelector("time[datetime]");
      if (!tEl || !tEl.getAttribute("datetime")) return NaN;
      var ms = Date.parse(tEl.getAttribute("datetime"));
      return isNaN(ms) ? NaN : ms;
    }
    function rowSearchBlob(tr) {
      var blob = "";
      try {
        var t = tr.querySelector("time");
        if (t) blob += " " + t.textContent.trim();
        var iso = tr.querySelector("time[datetime]");
        if (iso && iso.getAttribute("datetime")) blob += " " + iso.getAttribute("datetime");
        var msg = tr.querySelector(".sum-evlog__cell--msg");
        if (msg) blob += " " + msg.textContent.trim();
        var stat = tr.querySelector(".sum-evlog__cell--status");
        if (stat) blob += " " + stat.textContent.trim();
        var lk = (tr.getAttribute("data-evlog-level") || "").trim().toLowerCase();
        blob += " " + lk;
      } catch (e0) {}
      return blob.toLowerCase().replace(/\s+/g, " ").trim();
    }
    function rowPassesStatus(tr, mode) {
      var http = parseHttpAttr(tr.getAttribute("data-evlog-http"));
      var lk = (tr.getAttribute("data-evlog-level") || "").trim();
      if (mode === "all") return true;
      if (mode === "warnings") return sumEvlogIsWarnish(lk, http);
      if (mode === "errors") return sumEvlogIsFailish(lk, http);
      return true;
    }
    function ensureSearchEmptyRow(tbody) {
      var existing = tbody.querySelector("[data-sum-evlog-search-empty]");
      if (existing) return existing;
      var tr = document.createElement("tr");
      tr.className = "sum-evlog__row sum-evlog__search-empty-row";
      tr.setAttribute("data-sum-evlog-search-empty", "");
      tr.setAttribute("hidden", "");
      tr.setAttribute("role", "status");
      var td = document.createElement("td");
      td.className = "sum-evlog__search-empty-cell";
      td.colSpan = 3;
      td.appendChild(document.createTextNode("No matching entries for your search. "));
      var btn = document.createElement("button");
      btn.type = "button";
      btn.className = "sum-evlog__clear-inline-search";
      btn.setAttribute("data-sum-evlog-clear-search", "");
      btn.appendChild(document.createTextNode("Clear search"));
      td.appendChild(btn);
      tr.appendChild(td);
      tbody.insertBefore(tr, tbody.firstChild);
      return tr;
    }
    function sumEvlogSyncFooter(root) {
      var foot = root.querySelector("[data-sum-evlog-oldest]");
      var tbody = root.querySelector("[data-sum-evlog-tbody]");
      if (!foot || !tbody) return;
      var picked = tbody.querySelector(
        "tr[data-evlog-id].sum-evlog__row--selected:not(.sum-evlog__row--hidden)"
      );
      if (picked) {
        var msSel = rowTimespec(picked);
        var absSel = formatLogDateTimeLocal(msSel);
        var relSel = formatLogRelativeAgo(msSel);
        var tEl = picked.querySelector("time[datetime]");
        var dtAttr = tEl && tEl.getAttribute("datetime") ? tEl.getAttribute("datetime") : "";
        if (!dtAttr && isFinite(msSel)) {
          try {
            dtAttr = new Date(msSel).toISOString();
          } catch (eIso) {
            dtAttr = "";
          }
        }
        var timeOpen = dtAttr
          ? '<time datetime="' + escapeHtml(dtAttr) + '" title="' + escapeHtml(relSel) + '">'
          : '<time title="' + escapeHtml(relSel) + '">';
        foot.innerHTML =
          "Selected entry: " +
          timeOpen +
          escapeHtml(absSel) +
          "</time> <span class=\"sum-evlog__footer-rel\">(" +
          escapeHtml(relSel) +
          ")</span>";
        return;
      }
      var oldestMs = root._sumEvlogOldestVisible;
      foot.innerHTML =
        "Oldest <strong>visible</strong> entry: <time title=\"" +
        escapeHtml(formatLogRelativeAgo(oldestMs)) +
        "\">" +
        escapeHtml(formatLogDateTimeLocal(isFinite(oldestMs) ? oldestMs : null)) +
        "</time>";
    }
    function sumEvlogRebuildRoot(root) {
      var tbody = root.querySelector("[data-sum-evlog-tbody]");
      if (!tbody) return;
      var select = root.querySelector("[data-evlog-filter-status]");
      var mode = select && select.value ? select.value : "all";
      var q = "";
      try {
        var inp = root.querySelector(".sum-evlog__search");
        q = inp && inp.value ? String(inp.value).trim().toLowerCase() : "";
      } catch (eIn) {}
      var rows = Array.prototype.slice.call(tbody.querySelectorAll("tr[data-evlog-id]"), 0);
      var oldest = Infinity;
      var visibleCount = 0;
      var i;
      for (i = 0; i < rows.length; i++) {
        var tr = rows[i];
        var show =
          rowPassesStatus(tr, mode) &&
          (q === "" || rowSearchBlob(tr).indexOf(q) !== -1);
        tr.classList.toggle("sum-evlog__row--hidden", !show);
        if (show) {
          visibleCount++;
          var ts = rowTimespec(tr);
          if (isFinite(ts) && ts < oldest) oldest = ts;
        }
      }
      var searchEmptyRow = tbody.querySelector("[data-sum-evlog-search-empty]");
      if (searchEmptyRow) {
        searchEmptyRow.hidden = !(q !== "" && visibleCount === 0);
      }
      root._sumEvlogOldestVisible = oldest === Infinity ? NaN : oldest;
      sumEvlogSyncFooter(root);
    }
    function sumEvlogCopyFromRoot(root) {
      var tbody = root.querySelector("[data-sum-evlog-tbody]");
      var toast = root.querySelector("[data-sum-evlog-toast]");
      if (!tbody) return;
      var lines = [];
      var picked = tbody.querySelectorAll("tr[data-evlog-id].sum-evlog__row--selected");
      var allVisible = false;
      if (picked.length === 0) {
        picked = tbody.querySelectorAll("tr[data-evlog-id]:not(.sum-evlog__row--hidden)");
        allVisible = true;
      }
      for (var i = 0; i < picked.length; i++) {
        var tr = picked[i];
        var t = tr.querySelector("time");
        var timeStr = t ? t.textContent.trim() : "";
        var msg = tr.querySelector(".sum-evlog__cell--msg");
        var msgStr = msg ? msg.textContent.trim().replace(/\s+/g, " ") : "";
        var stat = tr.querySelector(".sum-evlog__cell--status");
        var statStr = stat ? stat.textContent.trim().replace(/\s+/g, " ") : "";
        lines.push(timeStr + "\t" + msgStr + "\t" + statStr);
      }
      var text = lines.join("\n");
      function showToast(ok, msg) {
        if (!toast) return;
        toast.classList.toggle("sum-evlog__toast--error", !ok);
        toast.textContent = msg;
      }
      if (!text) {
        showToast(false, allVisible ? "No visible rows." : "No rows selected.");
        return;
      }
      if (navigator.clipboard && navigator.clipboard.writeText) {
        navigator.clipboard.writeText(text).then(
          function () {
            var suffix = allVisible ? " visible line(s)." : " line(s).";
            showToast(true, "Copied " + lines.length + suffix);
          },
          function () {
            showToast(false, "Clipboard blocked.");
          }
        );
      } else {
        showToast(false, "Clipboard API unavailable in this browser.");
      }
    }
    function sumEvlogHydrateRoot(root) {
      var tbody = root.querySelector("[data-sum-evlog-tbody]");
      if (!tbody) return;
      ensureSearchEmptyRow(tbody);
      sumEvlogRebuildRoot(root);
    }
    globalThis.sumEvlogHydrateAllIn = function (container) {
      if (!container || !container.querySelectorAll) return;
      var roots = container.querySelectorAll("[data-sum-evlog-root]");
      for (var r = 0; r < roots.length; r++) {
        sumEvlogHydrateRoot(roots[r]);
      }
    };
    function debounceSearch(root) {
      if (searchTimers && searchTimers.get) {
        var prev = searchTimers.get(root);
        if (prev) window.clearTimeout(prev);
        searchTimers.set(
          root,
          window.setTimeout(function () {
            searchTimers.delete(root);
            sumEvlogRebuildRoot(root);
          }, 120)
        );
      } else {
        window.setTimeout(function () {
          sumEvlogRebuildRoot(root);
        }, 120);
      }
    }
    document.body.addEventListener(
      "input",
      function (ev) {
        var el = ev.target;
        if (!el || !el.classList || !el.classList.contains("sum-evlog__search")) return;
        var root = el.closest("[data-sum-evlog-root]");
        if (!root || !root.closest("#panel-summarized")) return;
        debounceSearch(root);
      },
      false
    );
    document.body.addEventListener(
      "change",
      function (ev) {
        var el = ev.target;
        if (!el || !el.closest) return;
        var root = el.closest("[data-sum-evlog-root]");
        if (!root || !root.closest("#panel-summarized")) return;
        if (el.matches("[data-evlog-filter-status]")) {
          sumEvlogRebuildRoot(root);
        }
      },
      false
    );
    document.body.addEventListener(
      "click",
      function (ev) {
        var t = ev.target;
        if (!t || typeof t.closest !== "function") return;
        var root = t.closest("[data-sum-evlog-root]");
        if (!root || !root.closest("#panel-summarized")) return;
        if (t.closest(".sum-evlog__copy-btn")) {
          ev.preventDefault();
          sumEvlogCopyFromRoot(root);
          return;
        }
        var clr = t.closest("[data-sum-evlog-clear-search]");
        if (clr) {
          ev.preventDefault();
          var inp = root.querySelector(".sum-evlog__search");
          if (inp) inp.value = "";
          sumEvlogRebuildRoot(root);
          if (inp) inp.focus();
          return;
        }
        var tr = t.closest("tr[data-evlog-id]");
        if (!tr || !root.contains(tr)) return;
        var tbody = tr.parentNode;
        if (!tbody || !tbody.hasAttribute || !tbody.hasAttribute("data-sum-evlog-tbody")) return;
        var rows = Array.prototype.slice.call(tbody.querySelectorAll("tr[data-evlog-id]"), 0);
        function rowIndex(rowsArr, trg) {
          for (var ri = 0; ri < rowsArr.length; ri++) {
            if (rowsArr[ri] === trg) return ri;
          }
          return -1;
        }
        function clearSel() {
          var sel = tbody.querySelectorAll(".sum-evlog__row--selected");
          for (var si = 0; si < sel.length; si++) sel[si].classList.remove("sum-evlog__row--selected");
        }
        function setRange(lo, hi, on) {
          var j;
          for (j = lo; j <= hi && j < rows.length; j++) {
            if (on) rows[j].classList.add("sum-evlog__row--selected");
            else rows[j].classList.remove("sum-evlog__row--selected");
          }
        }
        var idx = rowIndex(rows, tr);
        if (idx < 0) return;
        if (ev.shiftKey && root._sumEvlogAnchor != null && root._sumEvlogAnchor >= 0) {
          clearSel();
          var a = root._sumEvlogAnchor;
          setRange(Math.min(a, idx), Math.max(a, idx), true);
          sumEvlogSyncFooter(root);
          return;
        }
        if (ev.ctrlKey || ev.metaKey) {
          tr.classList.toggle("sum-evlog__row--selected");
          root._sumEvlogAnchor = idx;
          sumEvlogSyncFooter(root);
          return;
        }
        clearSel();
        tr.classList.add("sum-evlog__row--selected");
        root._sumEvlogAnchor = idx;
        sumEvlogSyncFooter(root);
      },
      false
    );
    document.body.addEventListener(
      "pointerdown",
      function (ev) {
        var t = ev.target;
        if (!t || typeof t.closest !== "function") return;
        var root = t.closest("[data-sum-evlog-root]");
        if (!root || !root.closest("#panel-summarized")) return;
        sumEvlogPointerSuppressedUntil = Date.now() + 480;
      },
      true
    );
    document.body.addEventListener(
      "focusout",
      function (ev) {
        var t = ev.target;
        if (!t || !t.closest) return;
        if (!t.closest("#panel-summarized")) return;
        var isSearch = t.classList && t.classList.contains("sum-evlog__search");
        var isStat = t.matches && t.matches("[data-evlog-filter-status]");
        if (!isSearch && !isStat) return;
        if (sumEvlogUiDeferTimer) {
          clearTimeout(sumEvlogUiDeferTimer);
          sumEvlogUiDeferTimer = null;
        }
        if (viewMode === "summarized") scheduleStoryRebuild();
      },
      true
    );
  })();

  (function wireWorkspaceDraftUi() {
    if (globalThis.__claudiaLogsWorkspaceDraftUiWired) return;
    globalThis.__claudiaLogsWorkspaceDraftUiWired = true;
    document.body.addEventListener(
      "click",
      function (ev) {
        var t = ev.target;
        if (!t || typeof t.closest !== "function") return;
        if (t.closest("[data-sum-workspaces-create]")) {
          ev.preventDefault();
          ev.stopPropagation();
          workspaceDrafts.push({
            id: nextWorkspaceDraftId++,
            projectId: "",
            flavorId: "",
            paths: []
          });
          scheduleStoryRebuild();
          return;
        }
        var managedCard = t.closest("[data-workspace-managed-id]");
        if (managedCard) {
          var wsNumM = Number(managedCard.getAttribute("data-workspace-managed-id"));
          if (!wsNumM) return;
          if (t.closest(".ws-managed-btn-configure")) {
            ev.preventDefault();
            ev.stopPropagation();
            beginWorkspaceManagedEdit(wsNumM);
            return;
          }
          if (t.closest(".ws-managed-btn-cancel")) {
            ev.preventDefault();
            ev.stopPropagation();
            cancelWorkspaceManagedEdit();
            return;
          }
          if (t.closest(".ws-managed-btn-save")) {
            ev.preventDefault();
            ev.stopPropagation();
            saveManagedWorkspacePaths(wsNumM);
            return;
          }
          if (t.closest(".ws-managed-btn-delete")) {
            ev.preventDefault();
            ev.stopPropagation();
            deleteManagedWorkspace(wsNumM);
            return;
          }
          if (t.closest(".ws-managed-btn-add")) {
            ev.preventDefault();
            ev.stopPropagation();
            if (
              workspaceManagedEditId !== wsNumM ||
              !workspaceManagedStaging ||
              workspaceManagedStaging.wsNum !== wsNumM
            ) {
              return;
            }
            var stA = workspaceManagedStaging.paths;
            var startDirA = stA && stA.length ? stA[stA.length - 1].path : "";
            pickFolderForWorkspaceDraft(startDirA).then(function (picked) {
              if (!picked) return;
              workspaceManagedStaging.paths.push({ id: null, path: String(picked).trim() });
              scheduleStoryRebuild();
            });
            return;
          }
          if (t.closest(".ws-managed-btn-remove")) {
            ev.preventDefault();
            ev.stopPropagation();
            if (
              workspaceManagedEditId !== wsNumM ||
              !workspaceManagedStaging ||
              workspaceManagedStaging.wsNum !== wsNumM
            ) {
              return;
            }
            var selMR = managedCard.querySelector(".ws-managed-paths-select");
            if (!selMR || selMR.selectedIndex < 0 || !workspaceManagedStaging.paths.length) return;
            workspaceManagedStaging.paths.splice(selMR.selectedIndex, 1);
            scheduleStoryRebuild();
            return;
          }
        }
        var card = t.closest("[data-workspace-draft]");
        if (!card) return;
        var draftId = Number(card.getAttribute("data-workspace-draft"));
        if (!draftId) return;
        if (t.closest(".ws-draft-btn-cancel")) {
          ev.preventDefault();
          removeWorkspaceDraft(draftId);
          scheduleStoryRebuild();
          return;
        }
        if (t.closest(".ws-draft-btn-save")) {
          ev.preventDefault();
          saveWorkspaceDraftById(draftId);
          return;
        }
        if (t.closest(".ws-draft-btn-add")) {
          ev.preventDefault();
          var dAdd = findWorkspaceDraft(draftId);
          if (!dAdd) return;
          var startDir = "";
          if (dAdd.paths && dAdd.paths.length) startDir = dAdd.paths[dAdd.paths.length - 1];
          pickFolderForWorkspaceDraft(startDir).then(function (picked) {
            if (!picked) return;
            appendWorkspaceDraftPath(dAdd, picked);
            scheduleStoryRebuild();
          });
          return;
        }
        if (t.closest(".ws-draft-btn-remove")) {
          ev.preventDefault();
          var dRm = findWorkspaceDraft(draftId);
          if (!dRm || !dRm.paths || !dRm.paths.length) return;
          var selRm = card.querySelector(".ws-draft-paths-select");
          if (!selRm || selRm.selectedIndex < 0) return;
          dRm.paths.splice(selRm.selectedIndex, 1);
          scheduleStoryRebuild();
          return;
        }
      },
      false
    );
    document.body.addEventListener(
      "input",
      function (ev) {
        var el = ev.target;
        if (!el || !el.getAttribute) return;
        var field = el.getAttribute("data-ws-field");
        if (!field) return;
        var cardIn = el.closest("[data-workspace-draft]");
        if (!cardIn) return;
        var did = Number(cardIn.getAttribute("data-workspace-draft"));
        var dIn = findWorkspaceDraft(did);
        if (!dIn) return;
        var vv = el.value != null ? String(el.value) : "";
        if (field === "project") dIn.projectId = vv;
        else if (field === "flavor") dIn.flavorId = vv;
        syncWorkspaceDraftHeader(cardIn, dIn);
      },
      false
    );
    document.body.addEventListener(
      "change",
      function (ev) {
        var el = ev.target;
        if (!el || !el.classList) return;
        var cardManagedCh = el.closest("[data-workspace-managed-id]");
        if (cardManagedCh && el.classList.contains("ws-managed-paths-select")) {
          var rmBtM = cardManagedCh.querySelector(".ws-managed-btn-remove");
          if (rmBtM)
            rmBtM.disabled =
              el.selectedIndex < 0 || !el.options || !el.options.length;
          return;
        }
        if (!el.classList.contains("ws-draft-paths-select")) return;
        var cardCh = el.closest("[data-workspace-draft]");
        if (!cardCh) return;
        var rmBt = cardCh.querySelector(".ws-draft-btn-remove");
        if (rmBt)
          rmBt.disabled =
            el.selectedIndex < 0 || !el.options || !el.options.length;
      },
      false
    );
  })();

  (function wireLogsChromeLinks() {
    document.body.addEventListener(
      "click",
      function (ev) {
        var t = ev.target;
        if (!t || typeof t.closest !== "function") return;
        var ext = t.closest("a.sum-ext-link");
        if (ext) {
          var href = ext.getAttribute("href") || "";
          if (/^https?:\/\//i.test(href)) {
            ev.preventDefault();
            ev.stopPropagation();
            if (typeof globalThis.claudiaOpenExternalURL === "function") {
              try {
                var ret = globalThis.claudiaOpenExternalURL(href);
                if (ret && typeof ret.then === "function") ret.catch(function () { });
              } catch (x) { }
            } else {
              try {
                window.open(href, "_blank", "noopener,noreferrer");
              } catch (x2) { }
            }
          }
          return;
        }
        var proj = t.closest("a.sum-proj-path");
        if (proj) {
          ev.preventDefault();
          ev.stopPropagation();
          var rel = proj.getAttribute("data-rel") || "";
          if (!rel) rel = proj.textContent || "";
          rel = String(rel).replace(/\s+/g, " ").trim();
          if (!rel) return;
          if (typeof globalThis.claudiaRevealProjectPath === "function") {
            try {
              var ret2 = globalThis.claudiaRevealProjectPath(rel);
              if (ret2 && typeof ret2.then === "function") ret2.catch(function () { });
            } catch (x3) { }
          }
          return;
        }
      },
      true
    );
  })();

  fetchTokenLabels();
  applyViewLayout();
  if (globalThis.ClaudiaLogs && globalThis.ClaudiaLogs.Transport) {
    globalThis.ClaudiaLogs.Transport.init(transportCtx);
  }
};