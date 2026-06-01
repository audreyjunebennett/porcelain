/**
 * Service summarized cards (gateway, broker, vectorstore, indexer) and broker health strips.
 */
globalThis.ChimeraSettings = globalThis.ChimeraSettings || {};
globalThis.ChimeraSettings.Render = globalThis.ChimeraSettings.Render || {};
globalThis.ChimeraSettings.Render.Cards = globalThis.ChimeraSettings.Render.Cards || {};

globalThis.ChimeraSettings.Render.Cards.mountServiceFeed = function (ctx) {
  var escapeHtml = ctx.escapeHtml;
  var getFlat = ctx.getFlat;
  var entryCache = ctx.entryCache;
  var strHash = ctx.strHash;
  var entryInstant = ctx.entryInstant;
  var primaryLogMessage = ctx.primaryLogMessage;
  var formatInt = ctx.formatInt;
  var getViewMode = ctx.getViewMode;
  var sumEvlogPanelHtml = ctx.sumEvlogPanelHtml;
  var sumEvlogBuildTbodyFromServiceEntries = ctx.sumEvlogBuildTbodyFromServiceEntries;
  var sumEvlogCountWarnFailFromEntries = ctx.sumEvlogCountWarnFailFromEntries;
  var scopedEvlogTitle = ctx.scopedEvlogTitle;
  var RECENT_CARD_STATUS_N = ctx.RECENT_CARD_STATUS_N;
  var CHIMERA_BROKER_PROVIDER_STALE_MS = ctx.CHIMERA_BROKER_PROVIDER_STALE_MS || 90000;
  var operatorCardChevronHtml = ctx.operatorCardChevronHtml;

  function chimeraBrokerProviderHealthResolve(arr) {
    var stateLabel = {
      up: "reachable",
      down: "offline",
      key_missing: "key missing",
      unknown: "configured",
      not_configured: "not configured"
    };
    var list = null;
    var liveErr = "";
    if (ctx.chimeraBrokerProviderSnapshot && ctx.chimeraBrokerProviderSnapshot.data && Array.isArray(ctx.chimeraBrokerProviderSnapshot.data.providers)) {
      var snapshotAgeMs = Date.now() - Number(ctx.chimeraBrokerProviderSnapshot.fetchedClientMs || 0);
      if (snapshotAgeMs <= CHIMERA_BROKER_PROVIDER_STALE_MS) {
        list = ctx.chimeraBrokerProviderSnapshot.data.providers.slice();
        liveErr = String(ctx.chimeraBrokerProviderSnapshot.data.error || "").trim();
      }
    }
    if (!list && globalThis.ChimeraSettings && ChimeraSettings.Derive && typeof ChimeraSettings.Derive.chimeraBrokerProviderHealthList === "function") {
      list = ChimeraSettings.Derive.chimeraBrokerProviderHealthList(arr, function (p) { return getFlat(p); });
    }
    if (list && list.length) {
      list = list.filter(function (ent) {
        return String((ent || {}).state || "").toLowerCase() !== "not_configured";
      });
    }
    return {
      list: list,
      liveErr: liveErr,
      emptyMsg: liveErr ? "chimera-broker unreachable" : "No providers loaded yet",
      stateLabel: stateLabel
    };
  }

  function chimeraBrokerProviderHealthSegTitle(entry, lab) {
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
   *   1. Live snapshot from /api/ui/chimera-broker/providers (refreshed every 30s) — authoritative
   *      because Chimera Broker (this build) doesn't slog per-provider lifecycle events, so the log
   *      buffer alone can't enumerate groq / gemini / ollama.
   *   2. Log-derived list via `ChimeraSettings.Derive.chimeraBrokerProviderHealthList` — fallback when
   *      the live snapshot is missing or stale (>90s) so an offline view still has something.
   *   3. Empty caption ("No providers loaded yet" / "chimera-broker unreachable") when neither source
   *      yields entries.
   *
   * opts.compact: collapsed Chimera Broker service card — up to three gapped indicators, no labels.
   */
  function chimeraBrokerProviderHealthStripHtml(arr, opts) {
    opts = opts || {};
    var compact = !!opts.compact;
    var R = chimeraBrokerProviderHealthResolve(arr);
    var list = R.list;
    var stateLabel = R.stateLabel;
    var healthSeg =
      globalThis.ChimeraUI && typeof globalThis.ChimeraUI.healthSegSpan === "function"
        ? globalThis.ChimeraUI.healthSegSpan
        : function (title, tone) {
            var t = tone === "up" || tone === "down" || tone === "key_missing" ? tone : "unknown";
            return (
              '<span class="sum-bf-prov-health-seg sum-bf-prov-health-seg--' +
              t +
              '" title="' +
              escapeHtml(title) +
              '"></span>'
            );
          };

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
          var stC = entC.state && stateLabel[entC.state] != null ? entC.state : "unknown";
          var labC = stateLabel[stC];
          segs.push(healthSeg(chimeraBrokerProviderHealthSegTitle(entC, labC), stC));
        }
      } else {
        for (var zi = 0; zi < 3; zi++) {
          segs.push(healthSeg(R.emptyMsg, "unknown"));
        }
      }
      return (
        '<span id="chimera-broker-provider-health-compact" class="sum-bf-prov-health-root sum-bf-prov-health-root--compact" role="img" aria-label="' +
        escapeHtml(trackTitle) +
        '">' +
        '<span class="sum-bf-prov-health-track sum-bf-prov-health-track--compact" title="' +
        escapeHtml(trackTitle) +
        '">' +
        segs.join("") +
        "</span></span>"
      );
    }

    var rootOpen = '<div id="chimera-broker-provider-health-strip" class="sum-bf-prov-health-root">';
    if (!list || !list.length) {
      return (
        rootOpen +
        '<div class="sum-bf-prov-health-track sum-bf-prov-health-track--empty" title="' +
        escapeHtml(R.emptyMsg) +
        '">' +
        healthSeg(R.emptyMsg, "unknown", "sum-bf-prov-health-seg--empty") +
        "</div>" +
        '<div class="sum-strip-caption sum-strip-caption--muted">' +
        escapeHtml(R.emptyMsg) +
        "</div></div>"
      );
    }
    var trackParts = [];
    var labelParts = [];
    for (var i = 0; i < list.length; i++) {
      var entry = list[i] || {};
      var st = entry.state && stateLabel[entry.state] != null ? entry.state : "unknown";
      var lab = stateLabel[st];
      trackParts.push(healthSeg(chimeraBrokerProviderHealthSegTitle(entry, lab), st));
      labelParts.push(
        '<span class="sum-bf-prov-health-label sum-bf-prov-health-label--' +
          escapeHtml(st) +
          '" title="' +
          escapeHtml(chimeraBrokerProviderHealthSegTitle(entry, lab)) +
          '">' +
          escapeHtml(String(entry.id || "—")) +
          " · " +
          escapeHtml(lab) +
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
   * Replaces the legacy generic "Request timeline" mix bar on the Chimera Broker panel
   * (which was always 100% purple because every Chimera Broker row maps to "upstream").
   * Backed by `ChimeraSettings.Derive.chimeraBrokerRelayOutcomeBuckets`.
   */
  function chimeraBrokerRelayOutcomeStripHtml(arr) {
    var b = null;
    if (
      globalThis.ChimeraSettings &&
      ChimeraSettings.Derive &&
      typeof ChimeraSettings.Derive.chimeraBrokerRelayOutcomeBuckets === "function"
    ) {
      b = ChimeraSettings.Derive.chimeraBrokerRelayOutcomeBuckets(arr, function (p) { return getFlat(p); });
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
    var html = '<div class="sum-timeline-bar sum-timeline-bar--relay-outcome" title="Chat relay outcomes since last chimera-broker ready (HTTP buckets + fetch errors + in-flight)">';
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

  function chimeraBrokerShortModelLabel(model) {
    if (!model || model === "—") return "—";
    var parts = String(model).split("/");
    var tail = parts[parts.length - 1] || model;
    if (tail.length > 36) return tail.slice(0, 34) + "…";
    return tail;
  }

  function chimeraBrokerProviderSnapshotDataForUi() {
    if (!ctx.chimeraBrokerProviderSnapshot || !ctx.chimeraBrokerProviderSnapshot.data) return null;
    var staleMs =
      typeof ctx.CHIMERA_BROKER_PROVIDER_STALE_MS === "number"
        ? ctx.CHIMERA_BROKER_PROVIDER_STALE_MS
        : 120000;
    var snapshotAgeMs = Date.now() - Number(ctx.chimeraBrokerProviderSnapshot.fetchedClientMs || 0);
    if (snapshotAgeMs > staleMs) return null;
    return ctx.chimeraBrokerProviderSnapshot.data;
  }

  function chimeraBrokerCardMetrics(arr) {
    if (
      globalThis.ChimeraSettings &&
      ChimeraSettings.Derive &&
      typeof ChimeraSettings.Derive.chimeraBrokerCardMetrics === "function"
    ) {
      return ChimeraSettings.Derive.chimeraBrokerCardMetrics(arr, function (p) {
        return getFlat(p);
      });
    }
    return {
      catalogModelCount: 0,
      relayOk: 0,
      relayFail: 0,
      outgoingSum: 0,
      usageSum: 0
    };
  }

  function chimeraBrokerAvailableModelCountResolve(arr) {
    var snap = chimeraBrokerProviderSnapshotDataForUi();
    if (
      globalThis.ChimeraSettings &&
      ChimeraSettings.Derive &&
      typeof ChimeraSettings.Derive.chimeraBrokerAvailableModelCount === "function"
    ) {
      return ChimeraSettings.Derive.chimeraBrokerAvailableModelCount(arr, function (p) {
        return getFlat(p);
      }, snap);
    }
    var bx = chimeraBrokerCardMetrics(arr);
    return bx.catalogModelCount != null && bx.catalogModelCount > 0 ? bx.catalogModelCount : 0;
  }

  function chimeraBrokerAvailableModelCountLabel(arr) {
    var n = chimeraBrokerAvailableModelCountResolve(arr);
    return n > 0 ? formatInt(n) : "—";
  }

  function chimeraBrokerCollapsedCardSubtitle(arr) {
    if (
      globalThis.ChimeraSettings &&
      ChimeraSettings.Derive &&
      typeof ChimeraSettings.Derive.chimeraBrokerCollapsedHealthSubtitle === "function"
    ) {
      return ChimeraSettings.Derive.chimeraBrokerCollapsedHealthSubtitle(arr, function (p) {
        return getFlat(p);
      });
    }
    return "";
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

  /** Gateway logs upstream relay with service=gateway; bucket under chimera-broker (see derive/logLineClassification.js). */
  function entryIsGatewayUpstreamRelay(ent) {
    var D = globalThis.ChimeraSettings && ChimeraSettings.Derive ? ChimeraSettings.Derive : null;
    if (D && typeof D.entryIsGatewayUpstreamRelay === "function") {
      return D.entryIsGatewayUpstreamRelay(ent, getFlat);
    }
    return false;
  }

  function entryRoutesToChimeraBrokerBucket(ent) {
    var D = globalThis.ChimeraSettings && ChimeraSettings.Derive ? ChimeraSettings.Derive : null;
    if (D && typeof D.entryRoutesToChimeraBrokerBucket === "function") {
      return D.entryRoutesToChimeraBrokerBucket(ent, getFlat);
    }
    return false;
  }

  function badgeForServicePanel(name, ev) {
    if (name === "chimera-broker") {
      var w = { parsed: ev.parsed, text: ev.text, ts: ev.ts, source: ev.source };
      if (entryIsGatewayUpstreamRelay(w)) {
        return {
          cls: "sum-svc-broker sum-svc-badge-filled sum-svc-broker-filled",
          key: "chimera-broker",
          lab: (typeof ctx.serviceDisplayLabel === "function" ? ctx.serviceDisplayLabel("chimera-broker") : "chimera-broker")
        };
      }
      return null;
    }
    if (name === "chimera-indexer") {
      var ixLabFn = ctx.indexerEvlogWorkspaceSourceLabel;
      var ixLab = typeof ixLabFn === "function" ? ixLabFn(ev) : "";
      if (!ixLab) return null;
      return { kind: "indexer-workspace", lab: ixLab, key: ixLab };
    }
    return ctx.inferServiceBadge(ev);
  }
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

  function recentServiceCardHasError(name, arr) {
    var hasErr = ctx.entryHasErrorStatus;
    var hasRl = ctx.chimeraBrokerEntryHasRateLimit;
    var slice =
      typeof ctx.sliceRecent === "function" ? ctx.sliceRecent(arr, RECENT_CARD_STATUS_N) : [];
    for (var i = 0; i < slice.length; i++) {
      if (typeof hasErr === "function" && hasErr(slice[i])) return true;
      if (name === "chimera-broker" && typeof hasRl === "function" && hasRl(slice[i])) return true;
    }
    return false;
  }

  /** True when flat is an indexer.state snapshot (slug or human title / structured fields). */
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

  function gatewayHttpOkFailTooltip(counters) {
    var c = counters || {};
    var detail =
      (c.http429 || 0) > 0
        ? formatInt(c.http2xx) +
          " ok · " +
          formatInt(c.httpNot2xx) +
          " fail · " +
          formatInt(c.http429) +
          " rate-limited (429)"
        : formatInt(c.http2xx) + " ok · " + formatInt(c.httpNot2xx) + " fail";
    return (
      "HTTP responses in the current log view: 2xx successes (check) vs non-2xx failures (error). " + detail + "."
    );
  }

  function chimeraBrokerRelayOkFailTooltip(metrics) {
    var m = metrics || {};
    var detail =
      (m.relay429N || 0) > 0
        ? formatInt(m.relayOk) +
          " ok · " +
          formatInt(m.relayFail) +
          " fail · " +
          formatInt(m.relay429N) +
          " rate-limited (429)"
        : formatInt(m.relayOk) + " ok · " + formatInt(m.relayFail) + " fail";
    return (
      "Chat relay outcomes since last chimera-broker ready: successful upstream responses (check) vs errors (error). " +
      detail +
      "."
    );
  }

  function indexerQueueDepthTooltip(qi) {
    qi = qi || {};
    var detail = formatInt(Math.round(Number(qi.queueDepth))) + " queued";
    if (qi.ingestInflight != null && !isNaN(Number(qi.ingestInflight))) {
      detail += " · " + formatInt(Math.round(Number(qi.ingestInflight))) + " in flight";
    }
    return (
      "Ingest queue depth from the latest indexer.state line in this log view (stacks icon). " +
      detail +
      "."
    );
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
      }
    };
    if (globalThis.ChimeraSettings && ChimeraSettings.Derive && typeof ChimeraSettings.Derive.gatewayCardModel === "function") {
      M = ChimeraSettings.Derive.gatewayCardModel(arr, getFlat);
    }
    var kv = M.kv || {};
    var apiKeysOpen =
      kv.apiKeysTint === "error"
        ? '<dd class="gateway-kv-dd gateway-kv-dd--error">'
        : "<dd>";
    return (
      '<dl class="indexer-run-kv indexer-run-kv--gateway-summary">' +
      "<dt>listening</dt><dd>" +
      escapeHtml(kv.listening || "—") +
      '</dd><dt>chimera-broker</dt><dd>' +
      escapeHtml(kv.broker || "—") +
      '</dd><dt>config</dt><dd>' +
      escapeHtml(kv.config || "—") +
      "</dd><dt>API keys</dt>" +
      apiKeysOpen +
      escapeHtml(kv.apiKeys || "—") +
      '</dd><dt>routing rules</dt><dd>' +
      escapeHtml(kv.routingRules || "—") +
      '</dd><dt>supervised</dt><dd>' +
      escapeHtml(kv.supervised || "—") +
      "</dd></dl>"
    );
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
    if (globalThis.ChimeraSettings && ChimeraSettings.Derive && typeof ChimeraSettings.Derive.vectorstoreCardModel === "function") {
      M = ChimeraSettings.Derive.vectorstoreCardModel(arr, getFlat, ctx.vectorstoreCollectionScopeLabelForLogs);
    }
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

  function chimeraBrokerServicePanelKvHtml(arr) {
    var M = {
      version: "—",
      configuration: "—",
      port: "—",
      auth: "—",
      mcp: "—",
      governance: "—",
      backendName: "",
      backendMode: ""
    };
    if (globalThis.ChimeraSettings && ChimeraSettings.Derive && typeof ChimeraSettings.Derive.chimeraBrokerCardModel === "function") {
      var d = ChimeraSettings.Derive.chimeraBrokerCardModel(arr, function (p) { return getFlat(p); });
      if (d.version) M.version = d.version;
      if (d.configuration) M.configuration = d.configuration;
      if (d.port) M.port = d.port;
      if (d.auth) M.auth = d.auth;
      if (d.mcp) M.mcp = d.mcp;
      if (d.governance) M.governance = d.governance;
      if (d.backendName) M.backendName = d.backendName;
      if (d.backendMode) M.backendMode = d.backendMode;
    }
    var brokerBackendLab = "—";
    if (
      globalThis.ChimeraSettings &&
      ChimeraSettings.Derive &&
      typeof ChimeraSettings.Derive.wrapperBackendPanelLabel === "function"
    ) {
      brokerBackendLab = ChimeraSettings.Derive.wrapperBackendPanelLabel(M.backendName, M.backendMode);
    }
    return (
      '<dl class="indexer-run-kv indexer-run-kv--chimera-broker-summary">' +
      "<dt>component</dt><dd>chimera-broker</dd>" +
      "<dt>backend</dt><dd>" +
      escapeHtml(brokerBackendLab) +
      "</dd>" +
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
      "</dd></dl>"
    );
  }

  /** Explainer strip at top of the Gateway service card (Services → Gateway). */
  function buildGatewayCardIntroHtml() {
    return (
      '<div class="gw-svc-card-intro" id="gw-svc-card-intro">' +
      '<p class="gw-svc-card-intro-lead">' +
      "An at-a-glance snapshot of this gateway instance—how it listens, where it connects, and which supervised helpers started. HTTP ok/fail counts in the card header reflect gateway lines in the current view; per-line level and HTTP status appear in the event log. For richer token rollups and upstream trails, open Gateway usage (Stats)." +
      "</p>" +
      "</div>"
    );
  }

  /** Explainer strip at top of the chimera-broker relay card (mirrors buildGatewayUsageIntroHtml). */
  function buildBrokerCardIntroHtml() {
    return (
      '<div class="bf-card-intro" id="bf-card-intro">' +
      '<p class="bf-card-intro-lead">' +
      "A fast health and traffic summary for the chimera-broker relay path into models. Odd patterns here usually mean throttling, misconfiguration, or an upstream hiccup—not necessarily that chat is already broken." +
      "</p>" +
      "</div>"
    );
  }

  /** Explainer strip at top of the chimera-vectorstore service card (mirrors the gateway / broker intros). */
  function buildVectorstoreCardIntroHtml() {
    return (
      '<div class="qd-card-intro" id="qd-card-intro">' +
      '<p class="qd-card-intro-lead">' +
      "chimera-vectorstore is the local vector store service the indexer fills and retrieval queries—this strip shows whether the wrapper is up and whether writes and searches are succeeding. Weak numbers here often mean thinner RAG before chat complains; counts reflect what the API reported, not a full on-disk audit." +
      "</p>" +
      "</div>"
    );
  }

  /** Explainer strip at top of the Indexer service card (mirrors gateway / broker / vectorstore intros). */
  function buildIndexerCardIntroHtml() {
    return (
      '<div class="ix-card-intro" id="ix-card-intro">' +
      '<p class="ix-card-intro-lead">' +
      "A quick read on how the supervised indexer is keeping watched trees in sync—backlog, throughput, and per-file outcomes at a glance. When those numbers drift the wrong way, retrieval can go stale; pauses and skips shown here are intentional signals, not silent drops." +
      "</p>" +
      "</div>"
    );
  }

  function renderExpandedService(name, arr, svcCtx) {
    svcCtx = svcCtx || {};
    var isChimeraBroker = name === "chimera-broker";
    var evConv = [];
    for (var j = 0; j < arr.length; j++) {
      evConv.push({ parsed: arr[j].parsed, text: arr[j].text, ts: arr[j].ts, source: arr[j].source });
    }
    var timelineBlock = "";
    if (
      name !== "chimera-indexer" &&
      name !== "chimera-vectorstore" &&
      name !== "chimera-broker" &&
      name !== "chimera-gateway"
    ) {
      var timelineFn = ctx.timelineBarHtml;
      timelineBlock =
        '<div class="sum-section-label">Request timeline</div>' +
        (typeof timelineFn === "function" ? timelineFn(evConv) : "");
    }
    var indexerSummaryKv = "";
    if (name === "chimera-indexer") {
      indexerSummaryKv =
        buildIndexerCardIntroHtml() +
        '<dl class="indexer-run-kv indexer-run-kv--service-aggregate">' +
        "<dt>Managed workspaces</dt><dd id=\"svc-indexer-summary-workspaces\">" +
        (typeof ctx.indexerServiceSummaryWorkspacesHtml === "function" ? ctx.indexerServiceSummaryWorkspacesHtml(svcCtx) : "") +
        '</dd><dt>Indexer config file</dt><dd id="svc-indexer-summary-config-path">' +
        (typeof ctx.indexerServiceSummaryConfigPathHtml === "function" ? ctx.indexerServiceSummaryConfigPathHtml() : "") +
        "</dd>" +
        "</dl>";
    }
    var mini;
    if (isChimeraBroker) {
      var bx = chimeraBrokerCardMetrics(arr);
      var kvB = chimeraBrokerServicePanelKvHtml(arr);
      var tokLineB = "— → —";
      if (bx.outgoingSum > 0 || bx.usageSum > 0) {
        tokLineB =
          (bx.outgoingSum > 0 ? formatInt(Math.round(bx.outgoingSum)) : "—") +
          " → " +
          (bx.usageSum > 0 ? formatInt(Math.round(bx.usageSum)) : "—");
      }
      var availModelsStr = chimeraBrokerAvailableModelCountLabel(arr);
      var providerHealthStrip = chimeraBrokerProviderHealthStripHtml(arr);
      mini =
        buildBrokerCardIntroHtml() +
        '<div class="sum-section-label">Provider health</div>' +
        providerHealthStrip +
        '<div class="sum-section-label">Relay outcomes</div>' +
        chimeraBrokerRelayOutcomeStripHtml(arr) +
        '<div class="sum-mini-row sum-mini-row--chimera-broker-deck">' +
        '<div class="sum-mini-card">Available models<strong id="chimera-broker-available-models-count">' +
        escapeHtml(availModelsStr) +
        '</strong><span class="sum-mini-sub">Live catalog from chimera-broker /v1/models (refreshed with provider health polls); falls back to log sync lines when stale</span></div>' +
        "</div>" +
        kvB +
        '<div class="sum-mini-row sum-mini-row--chimera-broker-deck2">' +
        '<div class="sum-mini-card">Relay (ok / fail)<strong>' +
        escapeHtml(formatInt(bx.relayOk) + " / " + formatInt(bx.relayFail)) +
        '</strong><span class="sum-mini-sub">Successful upstream responses vs errors (gateway relay)</span></div>' +
        '<div class="sum-mini-card">Tokens (out → usage)<strong>' +
        escapeHtml(tokLineB) +
        "</strong>" +
        '<span class="sum-mini-sub">Prompt tokens sent vs completion usage from upstream JSON</span></div>' +
        '</div>';
    } else if (name === "chimera-indexer") {
      mini = "";
    } else if (name === "chimera-gateway") {
      mini = buildGatewayCardIntroHtml() + gatewayServicePanelMiniHtml(arr);
    } else if (name === "chimera-vectorstore") {
      mini = buildVectorstoreCardIntroHtml() + vectorstoreServicePanelMiniHtml(arr);
    } else {
      var httpN2 = 0,
        sumMs2 = 0,
        err2 =
          typeof ctx.countWarnErrorInEntries === "function"
            ? ctx.countWarnErrorInEntries(arr)
            : 0;
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
    var fullLogClass = isChimeraBroker
      ? "sum-full-log sum-full-log--chimera-broker sum-full-log--evlog"
      : "sum-full-log sum-full-log--evlog";
    var scrollTbodyId = "svc-log-" + strHash(name);
    var cardScope = strHash("svc:" + name);
    var visEnt =
      typeof ctx.sumEvlogVisibleEntriesForService === "function"
        ? ctx.sumEvlogVisibleEntriesForService(name, arr, name === "chimera-gateway")
        : arr;
    var mc = sumEvlogCountWarnFailFromEntries(visEnt);
    var showSourceColumn = name === "chimera-gateway" || name === "chimera-indexer";
    var evlogBuildOpts = {
      cardScope: cardScope,
      filterGatewayProbe: name === "chimera-gateway"
    };
    if (showSourceColumn) evlogBuildOpts.showSourceColumn = true;
    var tbodyInner = sumEvlogBuildTbodyFromServiceEntries(name, arr, evlogBuildOpts);
    var evlogPanelOpts = {
      scrollTbodyId: scrollTbodyId,
      showSourceColumn: showSourceColumn,
      warnN: mc.warn,
      failN: mc.fail,
      tbodyInnerHtml: tbodyInner,
      title: typeof scopedEvlogTitle === "function" ? scopedEvlogTitle(ctx.serviceDisplayLabel(name)) : "Scoped log"
    };
    if (name === "chimera-indexer") evlogPanelOpts.sourceColumnKind = "indexer-workspace";
    var full =
      '<div class="' + fullLogClass + '">' +
      sumEvlogPanelHtml(evlogPanelOpts) +
      "</div>";
    return (
      '<div class="sum-body">' +
      timelineBlock +
      '<div class="sum-section-label">Summary</div>' +
      indexerSummaryKv +
      mini +
      full +
      "</div>"
    );
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
    var isChimeraBroker = name === "chimera-broker";
    var gwCardModel = null;
    if (
      name === "chimera-gateway" &&
      globalThis.ChimeraSettings &&
      ChimeraSettings.Derive &&
      typeof ChimeraSettings.Derive.gatewayCardModel === "function"
    ) {
      gwCardModel = ChimeraSettings.Derive.gatewayCardModel(arr, getFlat);
    }
    var qdrCardModel = null;
    if (
      name === "chimera-vectorstore" &&
      globalThis.ChimeraSettings &&
      ChimeraSettings.Derive &&
      typeof ChimeraSettings.Derive.vectorstoreCardModel === "function"
    ) {
      qdrCardModel = ChimeraSettings.Derive.vectorstoreCardModel(arr, getFlat, ctx.vectorstoreCollectionScopeLabelForLogs);
    }
    var lastMsg = isChimeraBroker ? chimeraBrokerCollapsedCardSubtitle(arr) : "";
    if (!isChimeraBroker) {
      var last = arr.length ? arr[arr.length - 1] : null;
      if (last) lastMsg = primaryLogMessage(last.parsed, last.text);
      if (name === "chimera-vectorstore" && qdrCardModel && qdrCardModel.subtitle && qdrCardModel.subtitle !== "—") {
        lastMsg = qdrCardModel.subtitle;
      }
      if (name === "chimera-gateway" && gwCardModel && gwCardModel.subtitle && gwCardModel.subtitle !== "—") {
        lastMsg = gwCardModel.subtitle;
      }
    }
    var ixWaitFlat = name === "chimera-indexer" ? indexerLatestSupervisedWaitFlat(arr) : null;
    if (ixWaitFlat) {
      var ixWaitProse =
        globalThis.ChimeraSettings &&
          ChimeraSettings.Derive &&
          typeof ChimeraSettings.Derive.indexerProseSummary === "function"
          ? ChimeraSettings.Derive.indexerProseSummary(ixWaitFlat)
          : null;
      if (ixWaitProse && String(ixWaitProse).trim() !== "") lastMsg = String(ixWaitProse).trim();
    }
    var st;
    if (recentServiceCardHasError(name, arr)) {
      st = { st: "error", cls: "sum-st-error" };
    } else if (name === "chimera-gateway" && gwCardModel) {
      if (gwCardModel.cardStatus === "error") st = { st: "error", cls: "sum-st-error" };
      else if (gwCardModel.cardStatus === "warn") st = { st: "degraded", cls: "sum-st-monitor" };
      else st = { st: "active", cls: "sum-st-active" };
    } else if (ixWaitFlat) {
      st = { st: "idle", cls: "sum-st-monitor" };
    } else {
      st = { st: "active", cls: "sum-st-active" };
    }
    var sid = "svc-" + strHash(name);
    var ini = ctx.serviceAvatarInitials(name);
    var av = ctx.serviceAvatarClass(name);
    /** Single outer .sum-title only — avoid nesting .sum-title (was hiding pills / breaking layout). */
    var titleClass = "sum-title";
    var displayServiceName = ctx.serviceDisplayLabel(name);
    var titleBlock = escapeHtml(displayServiceName);
    var wms = serviceWindowMs(arr);
    var metrics;
    if (isChimeraBroker) {
      var bxC = chimeraBrokerCardMetrics(arr);
      metrics =
        '<span class="sum-metrics">' +
        ctx.sgOpInsetWellOkFailHtml(bxC.relayOk, bxC.relayFail, "", {
          title: chimeraBrokerRelayOkFailTooltip(bxC)
        }) +
        chimeraBrokerProviderHealthStripHtml(arr, { compact: true }) +
        "</span>";
    } else if (name === "chimera-vectorstore") {
      if (qdrCardModel) {
        var vm = qdrCardModel;
        metrics =
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
          "</span>";
      } else {
        metrics = "";
      }
    } else if (name === "chimera-gateway") {
      if (gwCardModel) {
        var gc = gwCardModel.counters || {};
        metrics =
          '<span class="sum-metrics">' +
          ctx.sgOpInsetWellOkFailHtml(gc.http2xx || 0, gc.httpNot2xx || 0, "", {
            title: gatewayHttpOkFailTooltip(gc)
          }) +
          "</span>";
      } else {
        metrics = "";
      }
    } else if (name === "chimera-indexer") {
      var qiIx = latestIndexerStateQueueInflightFromEntries(arr);
      if (qiIx.queueDepth != null && !isNaN(Number(qiIx.queueDepth))) {
        var qCurIx = formatInt(Math.round(Number(qiIx.queueDepth)));
        var qTooltipIx = indexerQueueDepthTooltip(qiIx);
        metrics =
          '<span class="sum-metrics">' +
          '<span class="sg-op-inset-well" title="' +
          escapeHtml(qTooltipIx) +
          '">' +
          escapeHtml(qCurIx) +
          ' <span class="material-symbols-outlined material-symbols-outlined--sm" aria-hidden="true">stacks</span></span></span>';
      } else {
        metrics = "";
      }
    } else {
      metrics =
        '<span class="sum-metrics">' +
        '<span class="sum-metric">' +
        escapeHtml(String(arr.length)) +
        ' lines</span>' +
        (httpN ? '<span class="sum-metric">' + escapeHtml(String(httpN)) + " http</span>" : "") +
        (sumMs && typeof ctx.humanDurationMs === "function"
          ? '<span class="sum-metric">' + escapeHtml(ctx.humanDurationMs(sumMs)) + " Σ</span>"
          : "") +
        "</span>";
    }
    var statusHtml =
      isChimeraBroker || typeof ctx.serviceSummaryStatusPillHtml !== "function"
        ? ""
        : ctx.serviceSummaryStatusPillHtml(st);
    metrics =
      typeof ctx.summaryMetricsHtml === "function"
        ? ctx.summaryMetricsHtml(metrics, statusHtml)
        : metrics;
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
      (typeof operatorCardChevronHtml === "function" ? operatorCardChevronHtml() : "") +
      "</summary>" +
      renderExpandedService(name, arr, svcCtx) +
      "</details>"
    );
  }

  function summarizedServicesSectionHead() {
    if (typeof ctx.operatorSectionHeadHtml !== "function") {
      return '<div class="sum-section-label sum-feed-section-title">Services</div>';
    }
    return ctx.operatorSectionHeadHtml("Core services", "dns", { iconPrimary: true });
  }

  ctx.recentServiceCardHasError = recentServiceCardHasError;
  ctx.serviceWindowMs = serviceWindowMs;
  ctx.chimeraBrokerProviderHealthStripHtml = chimeraBrokerProviderHealthStripHtml;
  ctx.chimeraBrokerRelayOutcomeStripHtml = chimeraBrokerRelayOutcomeStripHtml;
  ctx.chimeraBrokerShortModelLabel = chimeraBrokerShortModelLabel;
  ctx.chimeraBrokerAvailableModelCountLabel = chimeraBrokerAvailableModelCountLabel;
  ctx.chimeraBrokerAvailableModelCountResolve = chimeraBrokerAvailableModelCountResolve;
  ctx.chimeraBrokerCollapsedCardSubtitle = chimeraBrokerCollapsedCardSubtitle;
  ctx.badgeForServicePanel = badgeForServicePanel;
  ctx.gatewayHttpOkFailTooltip = gatewayHttpOkFailTooltip;
  ctx.chimeraBrokerRelayOkFailTooltip = chimeraBrokerRelayOkFailTooltip;
  ctx.indexerQueueDepthTooltip = indexerQueueDepthTooltip;
  ctx.latestIndexerStateQueueInflightFromEntries = latestIndexerStateQueueInflightFromEntries;
  ctx.gatewayServicePanelMiniHtml = gatewayServicePanelMiniHtml;
  ctx.vectorstoreServicePanelMiniHtml = vectorstoreServicePanelMiniHtml;
  ctx.chimeraBrokerServicePanelKvHtml = chimeraBrokerServicePanelKvHtml;
  ctx.buildGatewayCardIntroHtml = buildGatewayCardIntroHtml;
  ctx.buildBrokerCardIntroHtml = buildBrokerCardIntroHtml;
  ctx.buildVectorstoreCardIntroHtml = buildVectorstoreCardIntroHtml;
  ctx.buildIndexerCardIntroHtml = buildIndexerCardIntroHtml;
  ctx.renderExpandedService = renderExpandedService;
  ctx.buildServiceCard = buildServiceCard;
  ctx.summarizedServicesSectionHead = summarizedServicesSectionHead;
  ctx.indexerLatestSupervisedWaitFlat = indexerLatestSupervisedWaitFlat;
  ctx.entryIsGatewayUpstreamRelay = entryIsGatewayUpstreamRelay;
  ctx.entryRoutesToChimeraBrokerBucket = entryRoutesToChimeraBrokerBucket;
};

