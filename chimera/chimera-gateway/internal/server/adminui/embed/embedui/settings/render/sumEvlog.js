/**
 * Summarized event-log table rows and panels (pure render helpers).
 *
 * Exports: ChimeraSettings.Render.mountSumEvlog(ctx)
 */

globalThis.ChimeraSettings = globalThis.ChimeraSettings || {};
globalThis.ChimeraSettings.Render = globalThis.ChimeraSettings.Render || {};
globalThis.ChimeraSettings.Render.mountSumEvlog = function (ctx) {
  var getFlat = ctx.getFlat;
  var escapeHtml = ctx.escapeHtml;
  var logSummaryHtml = ctx.logSummaryHtml;
  var primaryLogMessage = ctx.primaryLogMessage;
  var formatLogDateTimeLocal = ctx.formatLogDateTimeLocal;
  var formatLogRelativeAgo = ctx.formatLogRelativeAgo;
  var toIsoDatetimeAttr = ctx.toIsoDatetimeAttr;
  if (typeof primaryLogMessage !== "function") {
    primaryLogMessage = function (parsed, text) {
      var t = text != null ? String(text) : "";
      return t.trim() ? t : "";
    };
  }
  if (typeof formatLogDateTimeLocal !== "function") {
    formatLogDateTimeLocal = function () {
      return "—";
    };
  }
  if (typeof formatLogRelativeAgo !== "function") {
    formatLogRelativeAgo = function () {
      return "";
    };
  }
  if (typeof toIsoDatetimeAttr !== "function") {
    toIsoDatetimeAttr = function () {
      return "";
    };
  }
  var strHash = ctx.strHash;

  function inferServiceBadge(ev) {
    if (typeof ctx.inferServiceBadge === "function") return ctx.inferServiceBadge(ev);
    return { cls: "", lab: "" };
  }

  function sumEvlogColCount(opts) {
    return opts && opts.showSourceColumn === true ? 4 : 3;
  }

  function sumEvlogShouldHideBadge(badgeOpt, opts) {
    opts = opts || {};
    if (!badgeOpt || !badgeOpt.lab) return true;
    var k = (badgeOpt.key != null ? String(badgeOpt.key) : String(badgeOpt.lab || "")).toLowerCase();
    if (opts.suppressIndexerBadge && (k === "chimera-indexer" || k === "indexer")) return true;
    if (opts.suppressVectorstoreBadge && (k === "chimera-vectorstore" || k === "vectorstore")) return true;
    if (opts.suppressGatewayBadge && (k === "chimera-gateway" || k === "gateway")) return true;
    return false;
  }

  function sumEvlogBadgeHtml(badgeOpt) {
    if (!badgeOpt || !badgeOpt.lab) return "";
    var UIb = globalThis.ChimeraUI;
    return UIb && UIb.StatusIndicator && typeof UIb.StatusIndicator.serviceBadge === "function"
      ? UIb.StatusIndicator.serviceBadge(badgeOpt)
      : '<span class="sum-svc-badge ' + badgeOpt.cls + '">' + escapeHtml(badgeOpt.lab) + "</span>";
  }

  function sumEvlogHttpStatusNumber(v) {
    if (v == null || v === "") return null;
    var n = Number(v);
    if (isNaN(n) || n < 100 || n > 599) return null;
    return n;
  }

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
      msgL === "chimera-broker.http.access" ||
      msgL === "chimera-broker.rate_limit" ||
      msgL === "chimra-vectorstore.http.collection_meta" ||
      msgL === "chimra-vectorstore.http.points_upsert_ok" ||
      msgL === "chimra-vectorstore.http.points_upsert_rejected" ||
      msgL === "chimra-vectorstore.http.points_delete" ||
      msgL === "chimra-vectorstore.http.vector_search"
    ) {
      var hs = sumEvlogHttpStatusNumber(flat.http_status != null ? flat.http_status : flat.httpStatus);
      if (hs != null) return hs;
    }
    if (
      msgL === "chat.chimera-broker.response" ||
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
    var UI = globalThis.ChimeraUI;
    if (UI && UI.StatusIndicator && typeof UI.StatusIndicator.evlogRow === "function") {
      return UI.StatusIndicator.evlogRow({ levelKey: lk, http: http });
    }
    var Pill = UI && UI.Pill;
    var parts = [];
    if (Pill && typeof Pill.renderEvlogLevel === "function") {
      var lvl = Pill.renderEvlogLevel(lk);
      if (lvl) parts.push(lvl);
    }
    if (http != null && Pill && typeof Pill.renderHttpStatus === "function") {
      parts.push(Pill.renderHttpStatus(http, { asChip: http === 304 }));
    }
    if (!parts.length) {
      return '<span class="sum-evlog-status__empty" aria-hidden="true"></span>';
    }
    return parts.join("");
  }

  function sumEvlogSourceCellInnerHtml(badgeOpt, opts) {
    opts = opts || {};
    if (!opts.showSourceColumn) return "";
    if (sumEvlogShouldHideBadge(badgeOpt, opts)) return "";
    return sumEvlogBadgeHtml(badgeOpt);
  }

  function sumEvlogMsgCellInnerHtml(ev, badgeOpt, opts) {
    opts = opts || {};
    var parsed = ev.parsed;
    var badgeHtml = "";
    if (!opts.showSourceColumn && !sumEvlogShouldHideBadge(badgeOpt, opts)) {
      badgeHtml = sumEvlogBadgeHtml(badgeOpt);
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
    var msgOpts = { forEventLog: true };
    if (opts.convEvlogMeta) msgOpts.convEvlogMeta = opts.convEvlogMeta;
    var msg = escapeHtml(primaryLogMessage(parsed, ev.text, msgOpts));
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
    var sourceInner = sumEvlogSourceCellInnerHtml(badgeOpt, summaryOpts);
    var msgInner = sumEvlogMsgCellInnerHtml(entLike, badgeOpt, summaryOpts);
    var statusInner = sumEvlogStatusInnerHtml(parsed);
    var sourceTd = summaryOpts.showSourceColumn
      ? '<td class="sum-evlog__cell--source">' + sourceInner + "</td>"
      : "";
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
      sourceTd +
      '<td class="sum-evlog__cell--status"><div class="sum-evlog-status">' +
      statusInner +
      "</div></td></tr>"
    );
  }

  function scopedEvlogTitle(subject) {
    var s = subject != null ? String(subject).trim() : "";
    return s ? "Scoped log — " + s : "Scoped log";
  }

  function sumEvlogDataEmptyRowHtml(colspanOpt) {
    var colspan = colspanOpt != null ? colspanOpt : 3;
    return (
      '<tr class="sum-evlog__row sum-evlog__search-empty-row" data-sum-evlog-data-empty role="status">' +
      '<td class="sum-evlog__search-empty-cell" colspan="' +
      escapeHtml(String(colspan)) +
      '">No events to display</td>' +
      "</tr>"
    );
  }

  function sumEvlogToolbarStaticHtml() {
    return (
      '<div class="sum-evlog__toolbar">' +
      '<input class="sum-evlog__search" type="search" placeholder="Search message or time…" aria-label="Search log entries" autocomplete="off" />' +
      '<label class="sum-evlog__lvl-label" style="margin-left:auto">' +
      '<span class="sum-evlog__level-filters-label" style="margin-right:0.35rem">Status</span>' +
      '<span class="sum-evlog__filter-select-shell">' +
      '<span class="material-symbols-outlined sum-evlog__filter-opt-icon" data-evlog-filter-icon aria-hidden="true">filter_list</span>' +
      '<select class="sum-evlog__filter-select" data-evlog-filter-status aria-label="Filter by severity">' +
      '<option value="all">All</option>' +
      '<option value="warnings">Warnings</option>' +
      '<option value="errors">Errors</option>' +
      "</select></span></label></div>"
    );
  }

  var SUM_EVLOG_COPY_BTN =
    '<button type="button" class="sum-evlog__copy-btn sum-evlog__copy-btn--footer" title="Copy as TSV — selected rows, or all visible if none selected" aria-label="Copy as TSV: selected rows, or all visible if none selected">' +
    '<svg class="sum-evlog__copy-icon" xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><rect width="14" height="14" x="8" y="8" rx="2" ry="2"/><path d="M4 16c-1.1 0-2-.9-2-2V4c0-1.1.9-2 2-2h10c1.1 0 2 .9 2 2"/></svg>' +
    '<span class="sr-only"></span></button>';

  function sumEvlogRejectLeakedSource(v) {
    if (v == null || v === false) return "";
    if (typeof v === "function") return "";
    var str = typeof v === "string" ? v : String(v);
    if (/function\s+escapeHtml\s*\(/i.test(str)) return "";
    return str;
  }

  function sumEvlogCoerceTitle(v) {
    if (v == null || typeof v === "function") return "Scoped log";
    return String(v);
  }

  function sumEvlogServiceChipsHtml(parts) {
    if (!Array.isArray(parts) || !parts.length) return "";
    var clean = [];
    for (var i = 0; i < parts.length; i++) {
      var part = parts[i];
      if (part == null || typeof part === "function") continue;
      var lab = sumEvlogRejectLeakedSource(String(part).trim());
      if (!lab) continue;
      clean.push(lab);
    }
    if (!clean.length) return "";
    if (globalThis.ChimeraUI && globalThis.ChimeraUI.Chip && typeof globalThis.ChimeraUI.Chip.renderRow === "function") {
      var rowHtml = globalThis.ChimeraUI.Chip.renderRow(clean);
      if (typeof rowHtml === "string" && rowHtml && !/function\s+escapeHtml\s*\(/i.test(rowHtml)) {
        return rowHtml;
      }
    }
    var inner = "";
    for (var j = 0; j < clean.length; j++) {
      inner += '<span class="chip">' + escapeHtml(clean[j]) + "</span>";
    }
    return '<div class="service-chips">' + inner + "</div>";
  }

  function sumEvlogTitleRightHtml(o) {
    if (Array.isArray(o.titleRightParts)) {
      return sumEvlogServiceChipsHtml(o.titleRightParts);
    }
    return sumEvlogRejectLeakedSource(o.titleRightHtml);
  }

  function sumEvlogPanelHtml(o) {
    o = o || {};
    var showSource = o.showSourceColumn === true;
    var cols = showSource ? 4 : 3;
    var scrollTbodyId = o.scrollTbodyId || "sum-evlog-tb";
    var warnN = o.warnN != null ? o.warnN : 0;
    var failN = o.failN != null ? o.failN : 0;
    var tbodyInner = o.tbodyInnerHtml || "";
    if (!String(tbodyInner).trim()) {
      tbodyInner = sumEvlogDataEmptyRowHtml(cols);
    }
    var title = sumEvlogCoerceTitle(o.title != null ? o.title : "Scoped log");
    var titleRightHtml = sumEvlogTitleRightHtml(o);
    var titleBlock = titleRightHtml
      ? '<div class="sum-conv-full-log-head sum-evlog__title-row">' +
          '<div class="sum-section-label">' + escapeHtml(title) + "</div>" +
          '<div class="sum-conv-services-after-log-hdr">' + titleRightHtml + "</div>" +
        "</div>"
      : '<div class="sum-section-label">' + escapeHtml(title) + "</div>";
    var footerMetricsHtml =
      globalThis.ChimeraUI &&
      globalThis.ChimeraUI.StatusIndicator &&
      typeof globalThis.ChimeraUI.StatusIndicator.evlogFooterMetrics === "function"
        ? globalThis.ChimeraUI.StatusIndicator.evlogFooterMetrics({ warn: warnN, fail: failN })
        : "";
    var colgroup = showSource
      ? '<colgroup><col class="sum-evlog__col-time" /><col class="sum-evlog__col-msg" /><col class="sum-evlog__col-source" /><col class="sum-evlog__col-status" /></colgroup>'
      : '<colgroup><col class="sum-evlog__col-time" /><col class="sum-evlog__col-msg" /><col class="sum-evlog__col-status" /></colgroup>';
    var sourceTh = showSource ? '<th class="sum-evlog__th-source" scope="col">Source</th>' : "";
    var rootAttrs =
      ' data-sum-evlog-root data-sum-evlog-cols="' + escapeHtml(String(cols)) + '"' + (showSource ? ' data-sum-evlog-source' : "");
    return (
      '<div class="sum-evlog sum-evlog--in-card"' +
      rootAttrs +
      ">" +
      titleBlock +
      sumEvlogToolbarStaticHtml() +
      '<div class="sum-metrics-table-wrap sum-evlog__table-scroll">' +
      '<table class="sum-metrics-table sum-evlog__table">' +
      colgroup +
      '<thead><tr><th class="sum-evlog__cell--time" scope="col">Time</th>' +
      '<th class="sum-evlog__th-msg" scope="col">Message</th>' +
      sourceTh +
      '<th class="sum-evlog__th-status" scope="col">Status</th></tr></thead>' +
      '<tbody id="' +
      escapeHtml(scrollTbodyId) +
      '" data-sum-evlog-tbody>' +
      tbodyInner +
      "</tbody></table></div>" +
      '<div class="sum-evlog__footer-row">' +
      '<div class="sum-evlog__resize-handle" data-sum-evlog-resize-handle role="separator" aria-orientation="horizontal" aria-label="Drag to resize table height" tabindex="-1"></div>' +
      '<div class="sum-evlog__footer-left">' +
      '<p class="sum-evlog__footer" data-sum-evlog-oldest></p></div>' +
      '<div class="sum-evlog__footer-right">' +
      '<p class="sum-evlog__toast sum-gallery-evlog__toast-align" data-sum-evlog-toast role="status" aria-live="polite"></p>' +
      (footerMetricsHtml
        ? '<div class="sum-evlog__footer-metrics" role="group" aria-label="Status counts">' + footerMetricsHtml + "</div>"
        : "") +
      SUM_EVLOG_COPY_BTN +
      "</div></div>" +
      "</div>"
    );
  }

  function sumEvlogBuildTbodyFromConvEvents(evs, turnGroups, cardScope, buildOpts) {
    buildOpts = buildOpts || {};
    var rowBase = {};
    if (buildOpts.showSourceColumn === true) rowBase.showSourceColumn = true;
    var sectionColspan = sumEvlogColCount(rowBase);
    var parts = [];
    var rowIdx = 0;
    function pushEvent(evT, summaryExtra) {
      summaryExtra = summaryExtra || {};
      var evLine = {
        parsed: evT.parsed,
        text: evT.text != null && evT.text !== undefined ? evT.text : "",
        ts: evT.ts,
        source: evT.source,
        seq: evT.seq
      };
      var bd = inferServiceBadge(evLine);
      var rowOpts = {
        convJoinTier: evT.convJoinTier,
        vectorstoreSpanID: evT.vectorstoreSpanID,
        vectorstoreTurnIndex: evT.vectorstoreTurnIndex
      };
      if (rowBase.showSourceColumn) rowOpts.showSourceColumn = true;
      if (evT.convEvlogMeta) rowOpts.convEvlogMeta = evT.convEvlogMeta;
      if (summaryExtra.convEvlogMeta) rowOpts.convEvlogMeta = summaryExtra.convEvlogMeta;
      parts.push(sumEvlogRowTrHtml(evLine, cardScope, rowIdx, bd, rowOpts));
      rowIdx++;
    }
    function prepareTurnEvents(turnEvents) {
      if (
        globalThis.ChimeraSettings &&
        ChimeraSettings.Derive &&
        typeof ChimeraSettings.Derive.convEvlogPrepareTurnEvents === "function"
      ) {
        return ChimeraSettings.Derive.convEvlogPrepareTurnEvents(turnEvents, getFlat);
      }
      return turnEvents;
    }
    if (turnGroups && turnGroups.length > 1) {
      for (var tgi = 0; tgi < turnGroups.length; tgi++) {
        var tg = turnGroups[tgi];
        var preparedTurn = prepareTurnEvents(tg.events);
        parts.push(
          '<tr class="sum-evlog__section"><td colspan="' +
            sectionColspan +
            '" class="sum-evlog__section-cell">' +
            escapeHtml(tg.label) +
            "</td></tr>"
        );
        for (var ti2 = preparedTurn.length - 1; ti2 >= 0; ti2--) {
          pushEvent(preparedTurn[ti2]);
        }
      }
    } else {
      var preparedAll = prepareTurnEvents(evs);
      for (var u = preparedAll.length - 1; u >= 0; u--) {
        pushEvent(preparedAll[u]);
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
        !ctx.gatewayPanelShowProbes &&
        globalThis.ChimeraSettings &&
        ChimeraSettings.Derive &&
        typeof ChimeraSettings.Derive.gatewayPanelHideRow === "function" &&
        ChimeraSettings.Derive.gatewayPanelHideRow(ent2, function (p) {
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
      var summaryOpts = {};
      if (opts.showSourceColumn === true) summaryOpts.showSourceColumn = true;
      if (name === "chimera-indexer" || opts.suppressIndexerBadge) summaryOpts.suppressIndexerBadge = true;
      if (name === "chimera-vectorstore" || opts.suppressVectorstoreBadge) summaryOpts.suppressVectorstoreBadge = true;
      if (name === "chimera-gateway" || opts.suppressGatewayBadge) summaryOpts.suppressGatewayBadge = true;
      var bd2 = opts.indexerRunLine
        ? typeof ctx.badgeForIndexerRunLine === "function"
          ? ctx.badgeForIndexerRunLine(ent2)
          : inferServiceBadge(ev2)
        : typeof ctx.badgeForServicePanel === "function"
          ? ctx.badgeForServicePanel(name, ev2)
          : inferServiceBadge(ev2);
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
        !ctx.gatewayPanelShowProbes &&
        globalThis.ChimeraSettings &&
        ChimeraSettings.Derive &&
        typeof ChimeraSettings.Derive.gatewayPanelHideRow === "function" &&
        ChimeraSettings.Derive.gatewayPanelHideRow(ent2, function (p) {
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

  ctx.scopedEvlogTitle = scopedEvlogTitle;
  ctx.sumEvlogDataEmptyRowHtml = sumEvlogDataEmptyRowHtml;
  ctx.sumEvlogRowTrHtml = sumEvlogRowTrHtml;
  ctx.sumEvlogPanelHtml = sumEvlogPanelHtml;
  ctx.sumEvlogBuildTbodyFromConvEvents = sumEvlogBuildTbodyFromConvEvents;
  ctx.sumEvlogBuildTbodyFromServiceEntries = sumEvlogBuildTbodyFromServiceEntries;
  ctx.sumEvlogVisibleEntriesForService = sumEvlogVisibleEntriesForService;
  ctx.sumEvlogToolbarStaticHtml = sumEvlogToolbarStaticHtml;
  ctx.sumEvlogCountWarnFailFromEntries = sumEvlogCountWarnFailFromEntries;
  ctx.sumEvlogStatusInnerHtml = sumEvlogStatusInnerHtml;
  ctx.sumEvlogMsgCellInnerHtml = sumEvlogMsgCellInnerHtml;
  ctx.sumEvlogSourceCellInnerHtml = sumEvlogSourceCellInnerHtml;
  ctx.sumEvlogHttpCode = sumEvlogHttpCode;
  ctx.sumEvlogIsWarnish = sumEvlogIsWarnish;
  ctx.sumEvlogIsFailish = sumEvlogIsFailish;
  ctx.sumEvlogColCount = sumEvlogColCount;
  ctx.sumEvlogServiceChipsHtml = sumEvlogServiceChipsHtml;
  globalThis.ChimeraSettings.Render.sumEvlogServiceChipsHtml = sumEvlogServiceChipsHtml;
};
