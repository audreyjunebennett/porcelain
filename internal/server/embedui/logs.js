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
  var CONV_RECENT_N = 5;
  /** Last N events used for summary-strip status pills ("error" vs active/complete). Matches Last-events preview depth. */
  var RECENT_CARD_STATUS_N = 3;
  var entryCache = [];
  /** Maps gateway tenant_id → token label from tokens.yaml (via GET /api/ui/tokens). */
  var tokenLabelByTenant = {};
  var storyRebuildTimer = null;
  var maxSeq = 0;
  var stickPx = 160;
  var es = null;
  var pollTimer = null;
  /** SQLite gateway metrics snapshot for summarized “Gateway usage” card (/api/ui/metrics). */
  var metricsCache = null;
  var metricsPollTimer = null;
  var METRICS_POLL_MS = 12000;
  /** Latest indexer partition map from the last summarized panel render (hydrate ↔ supervised YAML roots). */
  var lastIndexerSummarizeByRun = null;
  var lastIndexerSummarizePartitionRegistry = null;
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
    if (msg === "http response" || (flat.method && flat.path != null && flat.statusCode !== undefined && flat.statusCode !== null))
      return "http.access";
    if (msg === "chat.request") return "chat.request";
    if (msg.indexOf("chat.bifrost") === 0 || msg.indexOf("upstream chat") >= 0) return "chat.bifrost";
    if (msg.indexOf("virtual model fallback attempt") >= 0 || msg.indexOf("virtual model routing resolved") >= 0)
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

  function buildDetailsColumn(parsed, entryTs, rawText, badgeOpt) {
    var evLike = { parsed: parsed, text: rawText != null && rawText !== undefined ? rawText : "", ts: entryTs };
    var top = logSummaryHtml(evLike, badgeOpt !== undefined ? badgeOpt : null);
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

  function applyFilters() {}

  function applyViewLayout() {
    var psu = document.getElementById("panel-summarized");
    if (!psu) return;
    try {
      document.body.classList.toggle("logs-summarized", true);
      document.body.classList.toggle("logs-raw", false);
      document.body.classList.toggle("logs-raw-logs", false);
    } catch (x) {}
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
      bifrostN = 0,
      qdrantEvt = 0,
      ingestN = 0;
    var ragMs = 0;
    for (var i = 0; i < events.length; i++) {
      var sh = events[i].parsed.shape || "";
      var f = getFlat(events[i].parsed);
      if (sh === "rag" || (sh.indexOf("rag.") === 0 && sh !== "rag")) {
        ragN++;
        var lm = Number(f.latencyMs != null ? f.latencyMs : f.latency_ms != null ? f.latency_ms : f.elapsedMs);
        if (!isNaN(lm)) ragMs += lm;
      } else if (sh.indexOf("chat.bifrost") === 0 || (String(f.msg || "").toLowerCase().indexOf("bifrost") >= 0 && sh.indexOf("chat") >= 0)) {
        bifrostN++;
      } else if (sh === "service.qdrant" || f.service === "qdrant") {
        qdrantEvt++;
      } else if (sh === "ingest") {
        ingestN++;
      }
    }
    var parts = [];
    if (ragN) parts.push("RAG · " + ragN + (ragMs ? " · ~" + Math.round(ragMs) + " ms" : ""));
    if (bifrostN) parts.push("BiFrost · " + bifrostN);
    if (qdrantEvt) parts.push("Qdrant · " + qdrantEvt);
    if (ingestN) parts.push("ingest · " + ingestN);
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

  function primaryLogMessage(parsed, rawText) {
    var rf = getFlat(parsed);
    var sh = parsed.shape || "";
    if (sh === "http.access" && rf.statusCode !== undefined && rf.statusCode !== null) {
      var line =
        (rf.method || "?") +
        " " +
        (rf.path || "") +
        " → " +
        rf.statusCode +
        (rf.responseTimeMs != null ? " · " + rf.responseTimeMs + " ms" : "");
      return line.length > MAX_PRIMARY_MSG_CHARS ? line.slice(0, MAX_PRIMARY_MSG_CHARS - 1) + "…" : line;
    }
    if (sh === "chat.bifrost" || (rf.upstreamModel && (rf.statusCode != null || rf.status != null))) {
      var scB = rf.statusCode != null ? rf.statusCode : rf.status;
      var parts = [];
      if (scB !== undefined && scB !== null && scB !== "") parts.push(String(scB));
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
      if (rf.statusCode != null) bitsR.push("HTTP " + rf.statusCode);
      var lineR = bitsR.filter(Boolean).join(" · ") || "routing";
      return lineR.length > MAX_PRIMARY_MSG_CHARS ? lineR.slice(0, MAX_PRIMARY_MSG_CHARS - 1) + "…" : lineR;
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
      slugIx.indexOf("gateway.indexer") === 0
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
    if (badgeOpt && badgeOpt.lab && !hideIxBadge) {
      badgeHtml =
        '<span class="sum-svc-badge ' +
        badgeOpt.cls +
        '">' +
        escapeHtml(badgeOpt.lab) +
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
      "</span>" +
      '<span class="log-line-sum__msg">' +
      msg +
      "</span></div>"
    );
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

  function refreshSummarizedPanel() {
    var psu = document.getElementById("panel-summarized");
    if (viewMode !== "summarized" || !psu) return;
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
    } catch (e) {}

    var storyScroll = {};
    try {
      var sps = psu.querySelectorAll(".story-panel");
      for (var sj = 0; sj < sps.length; sj++) {
        var cStory = sps[sj].closest("details[id]");
        if (cStory && cStory.id) storyScroll[cStory.id] = sps[sj].scrollTop;
      }
    } catch (e1) {}

    var fullLogScroll = {};
    try {
      var fls = psu.querySelectorAll(".sum-full-log");
      for (var fk = 0; fk < fls.length; fk++) {
        if (fls[fk] && fls[fk].id) fullLogScroll[fls[fk].id] = fls[fk].scrollTop;
      }
    } catch (e2) {}

    var gatewayMetricsTableScroll = [];
    try {
      var gwCap = document.getElementById("gw-usage-metrics");
      if (gwCap) {
        var wrapsC = gwCap.querySelectorAll(".sum-metrics-table-wrap");
        for (var wc = 0; wc < wrapsC.length; wc++) {
          gatewayMetricsTableScroll.push({ left: wrapsC[wc].scrollLeft, top: wrapsC[wc].scrollTop });
        }
      }
    } catch (e3) {}

    psu.innerHTML = renderSummarizedUnified();

    hydrateIndexerServiceSummaryFromApi();

    for (var ri = 0; ri < openDetailIds.length; ri++) {
      var d = document.getElementById(openDetailIds[ri]);
      if (d && d.tagName === "DETAILS") d.open = true;
    }

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
    }
    window.requestAnimationFrame(finalizeSummarizedScrollAfterLayout);
  }

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
      } catch (x) {}
      metricsPollTimer = null;
    }
    if (viewMode !== "summarized") return;
    fetchGatewayMetrics();
    metricsPollTimer = setInterval(fetchGatewayMetrics, METRICS_POLL_MS);
  }

  /** Explainer strip at top of the Gateway usage metrics card (/api/ui/metrics). */
  function buildGatewayUsageIntroHtml() {
    return (
      '<div class="gw-usage-intro" id="gw-usage-intro">' +
      '<p class="gw-usage-intro-lead">' +
      "Shows which models ran, estimated token totals for the current UTC minute and calendar day, and the latest upstream calls." +
      "</p>" +
      '<p class="gw-usage-intro-follow">' +
      "Together this is both a sanity check on load and input to limit handling: the gateway weighs these totals against your provider request and token ceilings so usage can soften before quotas hard-stop traffic. Treat the counts as estimates—each vendor measures and bills tokens in its own way." +
      "</p>" +
      '<ul class="gw-usage-intro-bullets">' +
      "<li>" +
      '<a class="sum-ext-link" href="https://console.groq.com/docs/rate-limits" rel="noopener noreferrer">Groq rate limits</a>' +
      " and " +
      '<a class="sum-ext-link" href="https://ai.google.dev/gemini-api/docs/pricing" rel="noopener noreferrer">Gemini pricing &amp; free tier</a>' +
      " vendor pages that are scraped for the free-tier ceilings." +
      "</li>" +
      "<li>" +
      "The gateway applies rollups against the limit tables in " +
      '<a href="#" class="sum-proj-path" data-rel="config/provider-model-limits.yaml"><code>config/provider-model-limits.yaml</code></a>' +
      "." +
      "</li>" +
      "</ul>" +
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
      .catch(function () {});
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

  function conversationCardStatus(g, t1) {
    if (recentConvEventsHaveError(g.events)) return { st: "error", cls: "sum-st-error" };
    var now = Date.now();
    if (t1 && now - t1.getTime() < 45000) return { st: "active", cls: "sum-st-active sum-pulse" };
    return { st: "complete", cls: "sum-st-complete" };
  }

  function countWarnErrorInEntries(arr) {
    var n = 0;
    for (var i = 0; i < arr.length; i++) {
      var lv = arr[i].parsed.levelCanon || "";
      if (lv === "ERROR" || lv === "WARN") n++;
      var sc = Number(getFlat(arr[i].parsed).statusCode);
      if (!isNaN(sc) && sc >= 400) n++;
    }
    return n;
  }

  function sliceRecent(arr, n) {
    if (!arr || !arr.length) return [];
    var take = Math.min(n, arr.length);
    return arr.slice(-take);
  }

  /** Card pill "error": ERROR level or HTTP status ≥400 (not WARN — avoids noisy strips). */
  function entryHasErrorStatus(ent) {
    var p = ent.parsed;
    if (!p) return false;
    if (p.levelCanon === "ERROR") return true;
    var sc = Number(getFlat(p).statusCode);
    if (!isNaN(sc) && sc >= 400) return true;
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
      var lab = inferServiceBadge(evList[i]).lab;
      if (lab === "web") counts.web++;
      else if (lab === "qdrant") counts.qdrant++;
      else if (lab === "upstream") counts.upstream++;
      else if (lab === "indexer") counts.indexer++;
      else counts.gateway++;
    }
    var total = counts.web + counts.qdrant + counts.upstream + counts.indexer + counts.gateway || 1;
    var cols = { web: "#42a5f5", qdrant: "#66bb6a", upstream: "#9575cd", indexer: "#ffa726", gateway: "#78909c" };
    var keys = ["web", "qdrant", "upstream", "indexer", "gateway"];
    var segments = [];
    for (var k = 0; k < keys.length; k++) {
      var key = keys[k];
      var pct = (counts[key] / total) * 100;
      if (pct < 0.05) continue;
      segments.push({ pct: pct, bg: cols[key] });
    }
    return timelineSegmentsHtml(segments);
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
          } catch (x) {}
        }
        if (es.length > 140) es = es.slice(0, 138) + "…";
        return "Upstream fetch failed" + (es ? ": " + es : "");
      }
    }
    var reqF = bifrostLastRelayRequestFlat(arr);
    if (reqF) return summarizeBifrostRelayRequest(reqF);
    for (var rj = arr.length - 1; rj >= 0; rj--) {
      var fr = getFlat(arr[rj].parsed);
      if (String(fr.msg || "").trim() === "upstream chat response") {
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
    return "No upstream chat relay in buffer";
  }

  /** Aggregate metrics for the bifrost service card from gateway upstream relay / response logs. */
  function bifrostCardMetrics(arr) {
    if (globalThis.ClaudiaLogs && globalThis.ClaudiaLogs.Derive && globalThis.ClaudiaLogs.Derive.bifrostCardMetrics) {
      return globalThis.ClaudiaLogs.Derive.bifrostCardMetrics(arr, function (p) { return getFlat(p); });
    }
    return { reqN: 0, resN: 0, errN: 0, streamOn: 0, streamOff: 0, outgoingSum: 0, usageSum: 0, bytesSum: 0, sc2xx: 0, scErr: 0, topModel: "—", rlN: 0 };
  }

  function bifrostShortModelLabel(model) {
    if (!model || model === "—") return "—";
    var parts = String(model).split("/");
    var tail = parts[parts.length - 1] || model;
    if (tail.length > 36) return tail.slice(0, 34) + "…";
    return tail;
  }

  function badgeForServicePanel(name, ev) {
    if (name === "bifrost")
      return { cls: "sum-svc-upstream sum-svc-badge-filled sum-svc-upstream-filled", lab: "upstream" };
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
    var httpN = 0,
      sumMs = 0,
      ingestOk = 0,
      chatN = 0,
      ragN = 0;
    var err = countWarnErrorInEntries(arr);
    for (var k = 0; k < arr.length; k++) {
      var p = arr[k].parsed;
      var f = getFlat(p);
      if (p.shape === "http.access") {
        httpN++;
        var rt = Number(f.responseTimeMs);
        if (!isNaN(rt)) sumMs += rt;
      }
      var msg = String(f.msg != null ? f.msg : f.message != null ? f.message : "").toLowerCase();
      if (msg === "ingest.complete") ingestOk++;
      if (msg.indexOf("chat.") === 0 || msg === "chat.request") chatN++;
      if (msg.indexOf("rag.") === 0) ragN++;
    }
    return (
      '<div class="sum-mini-row">' +
      '<div class="sum-mini-card">HTTP · Σ ms<strong>' +
      escapeHtml(formatInt(httpN) + " · " + (httpN ? String(Math.round(sumMs)) : "—")) +
      '</strong></div>' +
      '<div class="sum-mini-card">ingest.complete · RAG · chat slugs<strong>' +
      escapeHtml(formatInt(ingestOk) + " · " + formatInt(ragN) + " · " + formatInt(chatN)) +
      '</strong></div>' +
      '<div class="sum-mini-card">Warn+error lines<strong>' +
      escapeHtml(String(err)) +
      '</strong><span class="sum-mini-sub">' +
      escapeHtml(formatInt(arr.length) + " lines") +
      "</span></div></div>"
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
    var rg = rollupGatewayRagPipeline();
    var qh = qdrantHttpPathRollup(arr);
    var httpN = 0,
      sumMs = 0;
    for (var k = 0; k < arr.length; k++) {
      if (arr[k].parsed.shape === "http.access") {
        httpN++;
        var rt = Number(getFlat(arr[k].parsed).responseTimeMs);
        if (!isNaN(rt)) sumMs += rt;
      }
    }
    var err = countWarnErrorInEntries(arr);
    var ragTriple =
      formatInt(rg.ragQuery) + " · " + formatInt(rg.ragEmbed) + " · " + formatInt(rg.ragHitLines);
    var embedMsStr = rg.embedMsSum > 0 ? String(Math.round(rg.embedMsSum)) + " ms" : "—";
    var restTriple =
      formatInt(qh.searchN) + " · " + formatInt(qh.upsertN) + " · " + formatInt(qh.scrollN);
    var httpSigma =
      formatInt(httpN) + " · " + (httpN ? String(Math.round(sumMs)) : "—");
    var vecRestSub =
      formatInt(arr.length) +
      " lines · " +
      err +
      " warn/err · HTTP " +
      httpSigma +
      " req·Σms";
    return (
      '<div class="sum-mini-row">' +
      '<div class="sum-mini-card">RAG retrieval (gateway)<strong>' +
      escapeHtml(ragTriple) +
      '</strong><span class="sum-mini-sub">searches · query embeds · hit rows logged</span></div>' +
      '<div class="sum-mini-card">Σ query embed time<strong>' +
      escapeHtml(embedMsStr) +
      '</strong><span class="sum-mini-sub">rag.embed elapsed_ms (embedding API)</span></div>' +
      '<div class="sum-mini-card">Vector REST (Qdrant process)<strong>' +
      escapeHtml(restTriple) +
      '</strong><span class="sum-mini-sub">search · upsert · scroll · ' +
      escapeHtml(vecRestSub) +
      "</span></div></div>"
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
    return null;
  }

  function buildIndexerRecentEvaluatedFilesHtml(evsScope, bucketId, maxItems) {
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
    var bar = timelineBarHtml(evs);
    var spanMs = convWindowMs(g);
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
    var full = '<div class="sum-full-log"><ul>';
    for (var u = evs.length - 1; u >= 0; u--) {
      var ev2 = evs[u];
      full += '<li class="sum-ev-item">' + buildDetailsColumn(ev2.parsed, ev2.ts, ev2.text) + "</li>";
    }
    full += "</ul></div>";
    return (
      '<div class="sum-body">' +
      '<div class="sum-section-label">Request timeline</div>' +
      bar +
      '<div class="sum-section-label">Summary</div>' +
      mini +
      (serviceStripHtml(evs) ? '<div class="sum-section-label">Services</div>' + serviceStripHtml(evs) : "") +
      (contextGrowthStripHtml(evs) ? '<div class="sum-section-label">Context</div>' + contextGrowthStripHtml(evs) : "") +
      '<div class="sum-section-label">Full event log</div>' +
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
    var met = scrapeConversationMetrics(g.events);
    var dur = humanDurationMs(convWindowMs(g));
    var st = conversationCardStatus(g, t1);
    var tokStr = met.tok != null ? formatInt(met.tok) + " tok" : "— tok";
    var vecStr = met.vec != null ? formatInt(met.vec) + " vec" : "0 vec";
    var cardKey =
      Array.isArray(g.cids) && g.cids.length > 1
        ? g.pid + "\0" + g.cids.slice().sort().join("\0")
        : g.pid + "\0" + g.cid;
    var cardId = strHash(cardKey);
    var ini = avatarInitials(tokenLabelByTenant[g.pid] || g.pid);
    var av = avatarHueClass(cardKey);
    var metrics =
      '<span class="sum-metrics">' +
      '<span class="sum-metric">' +
      escapeHtml(dur) +
      "</span>" +
      '<span class="sum-metric">' +
      escapeHtml(tokStr) +
      "</span>" +
      '<span class="sum-metric">' +
      escapeHtml(vecStr) +
      "</span></span>";
    return (
      '<details class="sum-card" id="' +
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
      "</span>" +
      metrics +
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

  /** Match supervised YAML root row to a partitioned indexer bucket id (same scope as Indexers cards). */
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
      if (!lab || lab === "—") continue;
      if (bid) {
        parts.push(
          '<a class="sum-ext-link indexer-svc-ws-link" href="' +
            escapeHtml(indexerWorkspaceCardHref(bid)) +
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
   * Distinct scopes from partitioned indexer runs with links to matching Indexers cards (fallback before API).
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
            var labR = formatIndexerSupervisedRootLabel(rowR);
            if (bidR && br && Object.prototype.hasOwnProperty.call(br, bidR)) {
              var runM = br[bidR];
              var pmM = null;
              if (
                preg &&
                globalThis.ClaudiaLogs &&
                ClaudiaLogs.Derive &&
                typeof ClaudiaLogs.Derive.indexerPartitionMetaForRun === "function"
              ) {
                pmM = ClaudiaLogs.Derive.indexerPartitionMetaForRun(
                  preg,
                  bidR,
                  runM.events,
                  getFlat
                );
              }
              var metaM = collectIndexerRunMeta(bidR, runM.events, pmM);
              metaM = mergePersistedIndexerWatchRoots(metaM, runM.events, bidR);
              labR = indexerCardTitleSortLabel(metaM);
            }
            rows.push({ label: labR, bucketId: bidR });
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
    if (name !== "indexer") {
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
        '<dl class="indexer-run-kv indexer-run-kv--service-aggregate">' +
        "<dt>Managed workspaces</dt><dd id=\"svc-indexer-summary-workspaces\">" +
        aggregateIndexerManagedWorkspacesHtml(svcCtx.byRun, svcCtx.partitionRegistry) +
        '</dd><dt>Indexer config file</dt><dd id="svc-indexer-summary-config-path"><span class="muted">Loading…</span></dd>' +
        "</dl>";
    }
    var wms = serviceWindowMs(arr);
    var mini;
    if (isBifrost) {
      var bx = bifrostCardMetrics(arr);
      var flowLine = formatInt(bx.reqN) + " · " + formatInt(bx.resN) + " · " + formatInt(bx.errN);
      var tokLine = "—";
      if (bx.outgoingSum > 0 || bx.usageSum > 0) {
        tokLine =
          (bx.outgoingSum > 0 ? formatInt(Math.round(bx.outgoingSum)) + " out" : "— out") +
          " → " +
          (bx.usageSum > 0 ? formatInt(Math.round(bx.usageSum)) + " usage" : "— usage");
      }
      var streamLine = "—";
      if (bx.streamOn > 0 || bx.streamOff > 0) {
        streamLine =
          (bx.streamOn > 0 ? formatInt(bx.streamOn) + " stream" : "") +
          (bx.streamOn > 0 && bx.streamOff > 0 ? " · " : "") +
          (bx.streamOff > 0 ? formatInt(bx.streamOff) + " JSON" : "");
      }
      var httpBits = [];
      if (bx.sc2xx > 0) httpBits.push(formatInt(bx.sc2xx) + "×2xx");
      if (bx.scErr > 0) httpBits.push(formatInt(bx.scErr) + "×err");
      if (bx.rlN > 0) httpBits.push(formatInt(bx.rlN) + "×rate-limit");
      var modelHttp =
        escapeHtml(bifrostShortModelLabel(bx.topModel)) +
        (httpBits.length ? " · " + escapeHtml(httpBits.join(" · ")) : "");
      var streamSub =
        streamLine !== "—"
          ? streamLine
          : wms > 0
            ? humanDurationMs(wms) + " in buffer"
            : "—";
      var bytesSub =
        bx.bytesSum > 0 ? formatInt(Math.round(bx.bytesSum)) + " B response bodies" : "";
      mini =
        '<div class="sum-mini-row">' +
        '<div class="sum-mini-card">Relay (req · res · err)<strong>' +
        escapeHtml(flowLine) +
        '</strong></div><div class="sum-mini-card">Tokens (out → usage)<strong>' +
        escapeHtml(tokLine) +
        "</strong>" +
        (bytesSub ? '<span class="sum-mini-sub">' + escapeHtml(bytesSub) + "</span>" : "") +
        '</div><div class="sum-mini-card">Model · stream · HTTP<strong>' +
        modelHttp +
        '</strong><span class="sum-mini-sub">' +
        escapeHtml(streamSub) +
        "</span></div></div>";
    } else if (name === "indexer") {
      mini = indexerStructuredRollupMiniHtml(arr);
    } else if (name === "gateway") {
      mini = gatewayServicePanelMiniHtml(arr);
    } else if (name === "qdrant") {
      mini = qdrantServicePanelMiniHtml(arr);
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
    var fullLogClass = isBifrost ? "sum-full-log sum-full-log--bifrost" : "sum-full-log";
    var full = '<div class="' + fullLogClass + '"><ul>';
    for (var u = arr.length - 1; u >= 0; u--) {
      var ent2 = arr[u];
      var ev2 = { parsed: ent2.parsed, text: ent2.text, ts: ent2.ts, source: ent2.source };
      var bd2 = badgeForServicePanel(name, ev2);
      if (isBifrost) {
        full +=
          '<li class="sum-ev-item sum-ev-item--bifrost-detail">' +
          buildDetailsColumn(ent2.parsed, ent2.ts, ent2.text, bd2) +
          "</li>";
      } else {
        full +=
          '<li class="sum-ev-item">' +
          logSummaryHtml(ev2, bd2, name === "indexer" ? { suppressIndexerBadge: true } : undefined) +
          "</li>";
      }
    }
    full += "</ul></div>";
    return (
      '<div class="sum-body">' +
      timelineBlock +
      '<div class="sum-section-label">Summary</div>' +
      indexerSummaryKv +
      aggregateIndexerProgressBlock +
      mini +
      '<div class="sum-section-label">Full event log</div>' +
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
    var lastMsg = isBifrost ? bifrostCollapsedCardSubtitle(arr) : "";
    if (!isBifrost) {
      var last = arr.length ? arr[arr.length - 1] : null;
      if (last) lastMsg = primaryLogMessage(last.parsed, last.text);
    }
    var st = recentServiceCardHasError(name, arr)
      ? { st: "error", cls: "sum-st-error" }
      : { st: "active", cls: "sum-st-active" };
    var sid = "svc-" + strHash(name);
    var ini = serviceAvatarInitials(name);
    var av = serviceAvatarClass(name);
    /** Single outer .sum-title only — avoid nesting .sum-title (was hiding pills / breaking layout). */
    var titleClass = "sum-title";
    var titleBlock = escapeHtml(name === "indexer" ? "indexer" : isBifrost ? "bifrost" : name);
    var wms = serviceWindowMs(arr);
    var metrics;
    if (isBifrost) {
      var bxC = bifrostCardMetrics(arr);
      var pill1 =
        formatInt(bxC.reqN) +
        "↑ " +
        formatInt(bxC.resN) +
        "↓" +
        (bxC.errN > 0 ? " ·" + formatInt(bxC.errN) + "✗" : "");
      var pill2 =
        bxC.outgoingSum > 0 || bxC.usageSum > 0
          ? formatInt(Math.round(bxC.outgoingSum)) + "→" + formatInt(Math.round(bxC.usageSum))
          : formatInt(arr.length) + " lines";
      var pill3 = bifrostShortModelLabel(bxC.topModel);
      if (pill3.length > 24) pill3 = pill3.slice(0, 22) + "…";
      metrics =
        '<span class="sum-metrics">' +
        '<span class="sum-metric">' +
        escapeHtml(pill1) +
        '</span><span class="sum-metric">' +
        escapeHtml(pill2) +
        '</span><span class="sum-metric">' +
        escapeHtml(pill3) +
        "</span></span>";
    } else if (name === "qdrant") {
      var rgQ = rollupGatewayRagPipeline();
      var qhQ = qdrantHttpPathRollup(arr);
      metrics =
        '<span class="sum-metrics">' +
        '<span class="sum-metric">' +
        escapeHtml(formatInt(rgQ.ragQuery) + " retrieve") +
        '</span><span class="sum-metric">' +
        escapeHtml(formatInt(qhQ.searchN) + " search") +
        '</span><span class="sum-metric">' +
        escapeHtml(formatInt(arr.length) + " lines") +
        "</span></span>";
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

  function renderExpandedIndexer(run, evs, meta, partitionRegistry) {
    var pathsBlock =
      meta.watchRootPaths && meta.watchRootPaths.length
        ? "<pre class=\"indexer-paths-pre\">" +
          escapeHtml(meta.watchRootPaths.join("\n")) +
          "</pre>"
        : '<span class="muted">—</span>';
    var wsDd =
      meta.scopeWorkspaceTotal != null && !isNaN(Number(meta.scopeWorkspaceTotal))
        ? "<dt>Workspace files</dt><dd>" +
          escapeHtml(formatInt(Math.round(Number(meta.scopeWorkspaceTotal)))) +
          "</dd>"
        : '<dt>Workspace files</dt><dd><span class="muted">—</span></dd>';
    var sumU = meta.userLabel && meta.userLabel !== "—" ? String(meta.userLabel).trim() : "—";
    var sumP = meta.projectId && meta.projectId !== "—" ? String(meta.projectId).trim() : "—";
    var sumF = meta.flavorId && meta.flavorId !== "—" ? String(meta.flavorId).trim() : "";
    var summaryOneLine = sumF !== "" ? sumU + " · " + sumP + " · " + sumF : sumU + " · " + sumP;
    var summaryRows =
      '<div class="indexer-run-summary-line">' +
      escapeHtml(summaryOneLine) +
      "</div>" +
      '<dl class="indexer-run-kv">' +
      "<dt>Watched paths</dt><dd>" +
      pathsBlock +
      "</dd>" +
      wsDd +
      "</dl>";
    var evsFull = filterEventsForIndexerScopeFullLog(evs, run.id, partitionRegistry || {});
    var recentFiles = buildIndexerRecentEvaluatedFilesHtml(evsFull, run.id, 18);
    var fullId = "ix-full-" + strHash(run.id);
    var full = '<div class="sum-full-log" id="' + escapeHtml(fullId) + '"><ul>';
    if (!evsFull.length) {
      full +=
        '<li class="sum-ev-item muted">No scope-specific lines in the loaded window (shared lines appear under Services → Indexer).</li>';
    } else {
      for (var u = evsFull.length - 1; u >= 0; u--) {
        var ent2 = evsFull[u];
        var ev2 = { parsed: ent2.parsed, text: ent2.text, ts: ent2.ts, source: ent2.source };
        var bd2 = badgeForIndexerRunLine(ent2);
        full +=
          '<li class="sum-ev-item">' +
          logSummaryHtml(ev2, bd2, { suppressIndexerBadge: true }) +
          "</li>";
      }
    }
    full += "</ul></div>";
    return (
      '<div class="sum-body">' +
      '<div class="sum-section-label">Summary</div>' +
      summaryRows +
      '<div class="sum-section-label">Recently evaluated files</div>' +
      recentFiles +
      '<div class="sum-section-label">Full event log</div>' +
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
      } catch (_e2) {}
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

  /** Same headline as indexer cards; used for stable alphabetical ordering in the Indexers section. */
  function indexerCardTitleSortLabel(o) {
    if (!o) return "—";
    var userLine =
      o.userLabel && o.userLabel !== "—" ? String(o.userLabel).trim() : "—";
    var prLine =
      o.projectId && o.projectId !== "—" ? String(o.projectId).trim() : "—";
    var flavLine =
      o.flavorId && o.flavorId !== "—" ? String(o.flavorId).trim() : "";
    return flavLine !== ""
      ? userLine + " — " + prLine + " — " + flavLine
      : userLine + " — " + prLine;
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
    var sumLine = escapeHtml(
      flavLine !== "" ? userLine + " · " + prLine + " · " + flavLine : userLine + " · " + prLine
    );
    var iid = "ix-stale-" + strHash(bucketId);
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
      '<div class="indexer-run-summary-line">' +
      sumLine +
      "</div>" +
      '<dl class="indexer-run-kv">' +
      "<dt>Watched paths</dt><dd>" +
      pathsBlock +
      '</dd><dt>Workspace files</dt><dd><span class="muted">—</span></dd>' +
      "</dl>" +
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
    var userLine =
      meta.userLabel && meta.userLabel !== "—" ? String(meta.userLabel).trim() : "—";
    var prLine =
      meta.projectId && meta.projectId !== "—" ? String(meta.projectId).trim() : "—";
    var flavLine =
      meta.flavorId && meta.flavorId !== "—" ? String(meta.flavorId).trim() : "";
    var titleText =
      flavLine !== "" ? userLine + " — " + prLine + " — " + flavLine : userLine + " — " + prLine;
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
    return (
      '<details class="sum-card" id="' +
      escapeHtml(iid) +
      '"><summary>' +
      avatarIndexer +
      '<span class="sum-main"><span class="sum-title">' +
      titleInner +
      '</span>' +
      sub +
      progressStack +
      "</span>" +
      statusSpan +
      '<span class="sum-chev"></span></summary>' +
      renderExpandedIndexer(run, evs, meta, partitionRegistry) +
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

  /** Roll up separate gateway conversation_ids for the same principal when bursts are close in time. */
  function clusterConversationGroupsByTime(groups, gapMs) {
    var arr = [];
    for (var key in groups) {
      if (!Object.prototype.hasOwnProperty.call(groups, key)) continue;
      var gx = groups[key];
      var tmin = convFirstTs(gx);
      var tmax = convLastTs(gx);
      if (!tmax) continue;
      if (!tmin) tmin = tmax;
      arr.push({ pid: gx.pid, cid: gx.cid, events: gx.events.slice(), tmin: tmin, tmax: tmax });
    }
    if (arr.length <= 1) {
      for (var i0 = 0; i0 < arr.length; i0++) {
        arr[i0].cids = [arr[i0].cid];
      }
      return arr;
    }
    arr.sort(function (a, b) {
      return a.tmin - b.tmin;
    });
    var out = [];
    var cur = null;
    for (var j = 0; j < arr.length; j++) {
      var g = arr[j];
      if (!cur) {
        cur = { pid: g.pid, cid: g.cid, cids: [g.cid], events: g.events.slice(), tmin: g.tmin, tmax: g.tmax };
        continue;
      }
      if (g.pid === cur.pid && g.tmin - cur.tmax <= gapMs) {
        cur.cids.push(g.cid);
        cur.events = cur.events.concat(g.events);
        cur.tmin = Math.min(cur.tmin, g.tmin);
        cur.tmax = Math.max(cur.tmax, g.tmax);
        if (g.tmax >= cur.tmax) cur.cid = g.cid;
      } else {
        out.push(cur);
        cur = { pid: g.pid, cid: g.cid, cids: [g.cid], events: g.events.slice(), tmin: g.tmin, tmax: g.tmax };
      }
    }
    if (cur) out.push(cur);
    for (var k = 0; k < out.length; k++) {
      out[k].events.sort(function (a, b) {
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
    out.sort(function (a, b) {
      return b.tmax - a.tmax;
    });
    return out;
  }

  function formatMergedConversationSubtitle(mergedCount) {
    if (!mergedCount || mergedCount <= 1) return "";
    return (
      ' <span class="muted" style="font-size:0.85em" title="Several gateway conversation ids occurred close together in time; events are rolled up here.">(' +
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

  /**
   * Per-scope indexer cards: Full event log lists only lines attributable to that scope.
   * Lines attributed to every scope (multi-bucket fan-out) stay in Services → Indexer only.
   */
  function indexerScopeFullLogInclude(ent, bucketId, partitionRegistry) {
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

    return false;
  }

  function filterEventsForIndexerScopeFullLog(evs, bucketId, partitionRegistry) {
    var out = [];
    if (!Array.isArray(evs)) return out;
    for (var i = 0; i < evs.length; i++) {
      if (indexerScopeFullLogInclude(evs[i], bucketId, partitionRegistry)) out.push(evs[i]);
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
      msg === "chat.bifrost.error" ||
      msg.indexOf("bifrost.error") >= 0
    ) {
      return true;
    }
    var sh = ent.parsed.shape || "";
    if (sh === "chat.bifrost" || sh.indexOf("chat.bifrost.") === 0) return true;
    return false;
  }

  /** Stable /ui/logs Indexers bucket: backend indexer_key or tenant + project + flavor fallback. */
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
    var groups = {};
    for (var i = 0; i < entryCache.length; i++) {
      var ent = entryCache[i];
      var p = ent.parsed;
      var f = getFlat(p);
      var cid = f.conversation_id ? String(f.conversation_id) : "";
      if (!cid) continue;
      var pid = f.principal_id ? String(f.principal_id) : f.tenant ? String(f.tenant) : "";
      if (!pid) pid = "(unknown principal)";
      var key = pid + "\0" + cid;
      if (!groups[key]) groups[key] = { pid: pid, cid: cid, events: [] };
      var g = groups[key];
      g.events.push({ parsed: p, text: ent.text || "", ts: ent.ts, seq: ent.seq });
    }
    var buckets = { gateway: [], qdrant: [], bifrost: [], indexer: [] };
    for (var bi = 0; bi < entryCache.length; bi++) {
      var entB = entryCache[bi];
      var pB = entB.parsed;
      var fB = getFlat(pB);
      var svc = fB.service ? String(fB.service) : "";
      if (entryIsGatewayUpstreamRelay(entB)) svc = "bifrost";
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
    var convClusterGapMs = 42 * 60 * 1000;
    var mergedConv = clusterConversationGroupsByTime(groups, convClusterGapMs);
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
    var rks = Object.keys(byRun);
    for (var rj = 0; rj < rks.length; rj++) {
      var run = byRun[rks[rj]];
      seenIndexerBuckets[run.id] = true;
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
        idxTimeline.push({
          sortKey: indexerCardTitleSortLabel(sn) + "\u0001" + String(sbi),
          html: buildIndexerStaleSnapshotCard(sbi, sn)
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
    if (idxTimeline.length) {
      body +=
        '<div class="sum-feed-section"><div class="sum-section-label sum-feed-section-title">Indexers</div>';
      for (var zi = 0; zi < idxTimeline.length; zi++) body += idxTimeline[zi].html;
      body += "</div>";
    }
    if (svcHtml) {
      body +=
        '<div class="sum-feed-section"><div class="sum-section-label sum-feed-section-title">Services</div>' +
        svcHtml +
        "</div>";
    }
    var hasThreads = convTimeline.length > 0 || idxTimeline.length > 0 || svcHtml.length > 0;
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
    appendTableRow: function () {},
    applyFilters: function () {},
    ensureAppOption: function () {},
    ensureLevelOption: function () {},
    entryMatchesFilters: function () { return true; },
    fetchTokenLabels: fetchTokenLabels,
    startingRef: { value: false },
    esRef: { value: es },
    pollTimerRef: { value: pollTimer }
  };

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
                if (ret && typeof ret.then === "function") ret.catch(function () {});
              } catch (x) {}
            } else {
              try {
                window.open(href, "_blank", "noopener,noreferrer");
              } catch (x2) {}
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
              if (ret2 && typeof ret2.then === "function") ret2.catch(function () {});
            } catch (x3) {}
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