/**
 * Summarized feed card render (Phase 4 extraction).
 */
globalThis.ChimeraSettings = globalThis.ChimeraSettings || {};
globalThis.ChimeraSettings.Render = globalThis.ChimeraSettings.Render || {};
globalThis.ChimeraSettings.Render.Cards = globalThis.ChimeraSettings.Render.Cards || {};

globalThis.ChimeraSettings.Render.Cards.mountFeedLogConv = function (ctx) {
  var escapeHtml = ctx.escapeHtml;
  var getFlat = ctx.getFlat;
  var entryCache = ctx.entryCache;
  var strHash = ctx.strHash;
  var entryInstant = ctx.entryInstant;
  var primaryLogMessage = ctx.primaryLogMessage;
  var formatInt = ctx.formatInt;
  var getViewMode = ctx.getViewMode;
  var sumEvlogPanelHtml = ctx.sumEvlogPanelHtml;
  var sumEvlogBuildTbodyFromConvEvents = ctx.sumEvlogBuildTbodyFromConvEvents;
  var sumEvlogBuildTbodyFromServiceEntries = ctx.sumEvlogBuildTbodyFromServiceEntries;
  var sumEvlogCountWarnFailFromEntries = ctx.sumEvlogCountWarnFailFromEntries;
  var scopedEvlogTitle = ctx.scopedEvlogTitle;
  var contextGrowthStripHtml = ctx.contextGrowthStripHtml;
  var SHOW_CONV_EXPANDED_CONTEXT_STRIP = !!ctx.SHOW_CONV_EXPANDED_CONTEXT_STRIP;
  var formatMergedConversationSubtitle = ctx.formatMergedConversationSubtitle;
  var serviceAvatarClass = ctx.serviceAvatarClass;
  var serviceAvatarInitials = ctx.serviceAvatarInitials;
  var RECENT_CARD_STATUS_N = ctx.RECENT_CARD_STATUS_N;
  function conversationScopedLogSubject(tenantId, convId) {
    var tid = String(tenantId || "").trim();
    if (!tid) tid = "(unknown principal)";
    var lab = ctx.tokenLabelByTenant[tid];
    var head = lab && lab !== tid ? lab + " (" + tid + ")" : tid;
    var c = String(convId || "");
    if (c.length > 48) c = c.slice(0, 48) + "\u2026";
    return head + " - " + c;
  }

  /** Conversation card title: "label (tenant_id) - uuid" using token label when known. */
  function formatConversationCardTitle(tenantId, convId) {
    var tid = String(tenantId || "").trim();
    if (!tid) tid = "(unknown principal)";
    var lab = ctx.tokenLabelByTenant[tid];
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

  /**
   * Timeline segment key for gateway request bar (TIMELINE_BAR_KINDS).
   * Prefers structured `timeline_kind` from gateway/RAG/ingest logs (server-emitted); falls back to inferServiceBadge.
   */
  function timelineKindLab(ev) {
    var f = getFlat(ev.parsed);
    var tk = f.timeline_kind != null ? String(f.timeline_kind).trim().toLowerCase() : "";
    if (tk === "broker") return "chimera-broker";
    if (tk === "vectorstore") return "chimera-vectorstore";
    if (tk === "indexer") return "chimera-indexer";
    if (tk === "gateway") return "chimera-gateway";
    if (tk === "web" || tk === "chimera-vectorstore" || tk === "chimera-broker" || tk === "chimera-indexer" || tk === "chimera-gateway") {
      return tk;
    }
    var lab =
      typeof ctx.inferServiceBadge === "function"
        ? ctx.inferServiceBadge(ev).lab
        : "chimera-gateway";
    if (lab === "web") return "web";
    if (lab === "chimera-vectorstore") return "chimera-vectorstore";
    if (lab === "chimera-broker") return "chimera-broker";
    if (lab === "chimera-indexer") return "chimera-indexer";
    return "chimera-gateway";
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
    if (globalThis.ChimeraSettings && globalThis.ChimeraSettings.Derive && globalThis.ChimeraSettings.Derive.scrapeConversationMetrics) {
      return globalThis.ChimeraSettings.Derive.scrapeConversationMetrics(events, getFlat);
    }
    return { tok: null, vec: null };
  }

  function conversationCardModelForGroup(events) {
    events = Array.isArray(events) ? events : [];
    var model;
    if (globalThis.ChimeraSettings && ChimeraSettings.Derive && typeof ChimeraSettings.Derive.buildConversationCardModel === "function") {
      model = ChimeraSettings.Derive.buildConversationCardModel(events, getFlat);
    } else {
      model = {
        stateLabel: "—",
        stateKind: "complete",
        progress: {
          received: "pending",
          routed: "pending",
          rag: "pending",
          broker: "pending",
          delivered: "pending"
        },
        kv: { stream: "", ragCollection: "" },
        turnCount: 0,
        chips: { tools: 0, fallback: 0 },
        ingestRunIds: [],
        witness: { request: false, response: false }
      };
    }
    return model;
  }

  function conversationRagRetrievalSummary(events) {
    if (
      globalThis.ChimeraSettings &&
      ChimeraSettings.Derive &&
      typeof ChimeraSettings.Derive.conversationRagRetrievalSummary === "function"
    ) {
      return ChimeraSettings.Derive.conversationRagRetrievalSummary(events, getFlat);
    }
    var met = scrapeConversationMetrics(events);
    return { hits: met.vec, hasWorkspace: false, workspaceTitle: "", workspaceKnown: false };
  }

  function materialIconHtml(name) {
    return (
      '<span class="material-symbols-outlined material-symbols-outlined--sm" aria-hidden="true">' +
      escapeHtml(String(name || "")) +
      "</span>"
    );
  }

  function conversationVectorsRetrievedMiniCardHtml(rag) {
    rag = rag || {};
    var cardCls = "sum-mini-card sum-mini-card--rag-retrieval";
    if (rag.hasWorkspace && !rag.workspaceKnown) cardCls += " sum-mini-card--error";
    var parts = ['<div class="' + cardCls + '">'];
    if (!rag.hasWorkspace && rag.hits == null) {
      parts.push(
        '<span class="sum-mini-sub sum-mini-rag-error">' +
          materialIconHtml("error") +
          " unknown workspace provided</span>"
      );
    }
    if (rag.hasWorkspace && rag.workspaceTitle) {
      parts.push(
        '<span class="sum-mini-rag-line">' +
          materialIconHtml("database") +
          '<span class="sum-mini-rag-line-text">' +
          escapeHtml(rag.workspaceTitle) + 
          "</span></span>"
      );
    }
    if (rag.hits != null) {
      parts.push(
        '<strong class="sum-mini-rag-hits">' +
        materialIconHtml("text_snippet") +  
        " " +
        escapeHtml(String(rag.hits)) +
        " attached</strong>"
      );
    } else if (rag.hasWorkspace) {
      parts.push('<strong class="sum-mini-rag-hits">—</strong>');
    }
    if (rag.hasWorkspace && !rag.workspaceKnown) {
      parts.push(
        '<span class="sum-mini-sub sum-mini-rag-error">' +
          materialIconHtml("error") +
          " missing or undefined</span>"
      );
    }
    parts.push("</div>");
    return parts.join("");
  }

  function conversationLifecycleStepDefs() {
    return [
      { k: "received", lab: "Accepted" },
      { k: "rag", lab: "Context" },
      { k: "routed", lab: "Routed" },
      { k: "broker", lab: "Broker" },
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
    if (!parts.length) return "";
    var h = '<div class="sum-conv-chip-row sum-conv-chip-row--summary">';
    for (var pi = 0; pi < parts.length; pi++) {
      h += '<span class="sum-conv-chip">' + escapeHtml(parts[pi]) + "</span>";
    }
    h += "</div>";
    return h;
  }

  var convAgg =
    globalThis.ChimeraSettings && ChimeraSettings.Derive ? ChimeraSettings.Derive : {};

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
      if (msgW.indexOf("chimera-vectorstore.http.") === 0 && gfw.http_status != null) {
        var hs = Number(gfw.http_status);
        if (!isNaN(hs) && hs !== 200) n++;
      }
    }
    return n;
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
    if (msgQ.indexOf("chimera-vectorstore.http.") === 0 && fp.http_status != null) {
      var hq = Number(fp.http_status);
      if (!isNaN(hq) && hq !== 200) return true;
    }
    return false;
  }

  function chimeraBrokerEntryHasRateLimit(ent) {
    if (globalThis.ChimeraSettings && globalThis.ChimeraSettings.Derive && globalThis.ChimeraSettings.Derive.chimeraBrokerEntryHasRateLimit) {
      return globalThis.ChimeraSettings.Derive.chimeraBrokerEntryHasRateLimit(ent, function (p) { return getFlat(p); });
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

  function recentConvEventsHaveError(events) {
    var slice =
      typeof ctx.sliceRecent === "function"
        ? ctx.sliceRecent(events, RECENT_CARD_STATUS_N)
        : [];
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
    { key: "chimera-vectorstore", bg: "#66bb6a", label: "vectorstore", title: "vectorstore wrapper and backend lines" },
    { key: "chimera-broker", bg: "#9575cd", label: "broker", title: "broker relay and upstream chat traffic" },
    { key: "chimera-indexer", bg: "#ffa726", label: "indexer", title: "indexer subprocess lines" },
    { key: "chimera-gateway", bg: "#78909c", label: "gateway", title: "gateway routing, startup, config, and other internal logs" }
  ];

  /** Shared with timelineBarHtml and indexer scope cards (same `.sum-timeline-bar` DOM). */
  function timelineSegmentsHtml(segments) {
    if (globalThis.ChimeraUI && globalThis.ChimeraUI.TimelineBar && typeof globalThis.ChimeraUI.TimelineBar.segments === "function") {
      return globalThis.ChimeraUI.TimelineBar.segments(segments);
    }
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
    var counts = { web: 0, vectorstore: 0, broker: 0, indexer: 0, gateway: 0 };
    for (var i = 0; i < evList.length; i++) {
      var lab = timelineKindLab(evList[i]);
      if (lab === "web") counts.web++;
      else if (lab === "chimera-vectorstore") counts.vectorstore++;
      else if (lab === "chimera-broker") counts.broker++;
      else if (lab === "chimera-indexer") counts.indexer++;
      else counts.gateway++;
    }
    var total = counts.web + counts.vectorstore + counts.broker + counts.indexer + counts.gateway || 1;
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

  function renderExpandedConv(g) {
    var evs = g.events;
    var cardModel = conversationCardModelForGroup(evs);
    var ingestCount = 0;
    var ig;
    for (ig = 0; ig < evs.length; ig++) {
      if (evs[ig].convJoinTier === "ingest") ingestCount++;
    }
    var bar = timelineBarHtml(evs);
    var met = scrapeConversationMetrics(evs);
    var tokLine = met.tok != null ? formatInt(met.tok) : "—";
    var turnsLine = (cardModel.turnCount || 0) > 0 ? String(cardModel.turnCount) : "—";
    var ragSummary = conversationRagRetrievalSummary(evs);
    var mini =
      '<div class="sum-mini-row">' +
      '<div class="sum-mini-card">Turns<strong>' +
      escapeHtml(turnsLine) +
      '</strong></div><div class="sum-mini-card">Token Count<strong>' +
      escapeHtml(tokLine) +
      "</strong></div>" +
      conversationVectorsRetrievedMiniCardHtml(ragSummary) +
      "</div>";
    var life = conversationLifecycleBarHtml(cardModel.progress, {});
    var chips = conversationCardChipsSummaryHtml(cardModel);
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
      globalThis.ChimeraSettings &&
      ChimeraSettings.Derive &&
      typeof ChimeraSettings.Derive.conversationTurnGroupsForExpanded === "function"
    ) {
      turnGroups = ChimeraSettings.Derive.conversationTurnGroupsForExpanded(evs, getFlat);
    }
    var cardKey =
      Array.isArray(g.cids) && g.cids.length > 1
        ? g.pid + "\0" + g.cids.slice().sort().join("\0")
        : g.pid + "\0" + g.cid;
    var convScope = strHash(cardKey);
    var scrollTbodyId = "conv-log-" + convScope;
    var tbodyInner = sumEvlogBuildTbodyFromConvEvents(evs, turnGroups, convScope, { showSourceColumn: true });
    var mc = sumEvlogCountWarnFailFromEntries(evs);
    var full =
      '<div class="sum-full-log sum-full-log--evlog">' +
      sumEvlogPanelHtml({
        scrollTbodyId: scrollTbodyId,
        showSourceColumn: true,
        warnN: mc.warn,
        failN: mc.fail,
        tbodyInnerHtml: tbodyInner,
        title:
          typeof scopedEvlogTitle === "function"
            ? scopedEvlogTitle(conversationScopedLogSubject(g.pid, g.cid))
            : "Scoped log"
      }) +
      "</div>";
    var contextStrip =
      SHOW_CONV_EXPANDED_CONTEXT_STRIP && typeof contextGrowthStripHtml === "function"
        ? contextGrowthStripHtml(evs)
        : "";
    return (
      '<div class="sum-body">' +
      '<div class="sum-section-label">Lifecycle</div>' +
      life +
      chips +
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
    var st = conversationCardStatus(g, t1, cardModel);
    var cardKey =
      Array.isArray(g.cids) && g.cids.length > 1
        ? g.pid + "\0" + g.cids.slice().sort().join("\0")
        : g.pid + "\0" + g.cid;
    var cardId = strHash(cardKey);
    var ini =
      typeof ctx.avatarInitials === "function"
        ? ctx.avatarInitials(ctx.tokenLabelByTenant[g.pid] || g.pid)
        : "??";
    var av =
      typeof ctx.avatarHueClass === "function" ? ctx.avatarHueClass(cardKey) : "sum-av-a";
    var sumChips = conversationCardChipsSummaryHtml(cardModel);
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
      lifeCompact +
      (typeof ctx.serviceSummaryStatusPillHtml === "function" ? ctx.serviceSummaryStatusPillHtml(st) : "") +
      (typeof ctx.operatorCardChevronHtml === "function" ? ctx.operatorCardChevronHtml() : "") +
      "</summary>" +
      renderExpandedConv(g) +
      "</details>"
    );
  }
  function summarizedConversationsSectionHead() {
    if (typeof ctx.operatorSectionHeadHtml !== "function") {
      return (
        '<div class="sum-feed-section-head">' +
        '<span class="material-symbols-outlined sum-feed-section-icon" aria-hidden="true">forum</span>' +
        '<span class="sum-feed-section-title sum-section-label">Conversations</span></div>'
      );
    }
    return ctx.operatorSectionHeadHtml("Conversations", "forum");
  }

  function summarizedEmptyFeedMessage() {
    return (
      '<p class="muted">No conversation / service cards in the <em>loaded</em> window yet. Chat traffic needs <code>conversation_id</code> in structured logs; <strong>scroll to the top</strong> of this feed to load older lines (indexer snapshots often crowd the recent tail).</p>'
    );
  }

  ctx.summarizedConversationsSectionHead = summarizedConversationsSectionHead;
  ctx.summarizedEmptyFeedMessage = summarizedEmptyFeedMessage;
  ctx.pushConversationGroupedEvent = convAgg.pushConversationGroupedEvent;
  ctx.entryIsVectorstoreSubprocessForConvJoin = function (ent) {
    return convAgg.entryIsVectorstoreSubprocessForConvJoin
      ? convAgg.entryIsVectorstoreSubprocessForConvJoin(ent, getFlat)
      : false;
  };
  ctx.conversationRequestIdTier2EligibleLocal = convAgg.conversationRequestIdTier2EligibleLocal;
  ctx.conversationIndexRunTier3EligibleLocal = convAgg.conversationIndexRunTier3EligibleLocal;
  ctx.tryRegisterRequestConversationCorrelationPrimary = convAgg.tryRegisterRequestConversationCorrelationPrimary;
  ctx.tryRegisterRequestConversationCorrelationRagFallback = convAgg.tryRegisterRequestConversationCorrelationRagFallback;
  ctx.conversationScopedLogSubject = conversationScopedLogSubject;
  ctx.formatConversationCardTitle = formatConversationCardTitle;
  ctx.scrapeConversationMetrics = scrapeConversationMetrics;
  ctx.conversationCardModelForGroup = conversationCardModelForGroup;
  ctx.conversationRagRetrievalSummary = conversationRagRetrievalSummary;
  ctx.conversationVectorsRetrievedMiniCardHtml = conversationVectorsRetrievedMiniCardHtml;
  ctx.conversationLifecycleBarHtml = conversationLifecycleBarHtml;
  ctx.conversationCardChipsSummaryHtml = conversationCardChipsSummaryHtml;
  ctx.conversationCardStatus = conversationCardStatus;
  ctx.timelineBarHtml = timelineBarHtml;
  ctx.timelineLegendHtml = timelineLegendHtml;
  ctx.renderExpandedConv = renderExpandedConv;
  ctx.buildConvCard = buildConvCard;
  ctx.timelineSegmentsHtml = timelineSegmentsHtml;
  ctx.entryHasErrorStatus = entryHasErrorStatus;
  ctx.chimeraBrokerEntryHasRateLimit = chimeraBrokerEntryHasRateLimit;
  ctx.countErrorSignalsInEntries = countErrorSignalsInEntries;
  ctx.countWarnErrorInEntries = countWarnErrorInEntries;
};
