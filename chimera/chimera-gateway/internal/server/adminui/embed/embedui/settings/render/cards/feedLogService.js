/**
 * Summarized feed card render (Phase 4 extraction).
 */
globalThis.ChimeraSettings = globalThis.ChimeraSettings || {};
globalThis.ChimeraSettings.Render = globalThis.ChimeraSettings.Render || {};
globalThis.ChimeraSettings.Render.Cards = globalThis.ChimeraSettings.Render.Cards || {};

globalThis.ChimeraSettings.Render.Cards.mountFeedLogService = function (ctx) {
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
  function serviceDisplayLabel(key) {
    return typeof ctx.serviceDisplayLabel === "function"
      ? ctx.serviceDisplayLabel(key)
      : String(key || "").trim();
  }

  function inferServiceBadge(ev) {
    return typeof ctx.inferServiceBadge === "function" ? ctx.inferServiceBadge(ev) : null;
  }

  function serviceAvatarClass(name) {
    return typeof ctx.serviceAvatarClass === "function" ? ctx.serviceAvatarClass(name) : "sum-av-a";
  }

  function serviceAvatarInitials(name) {
    return typeof ctx.serviceAvatarInitials === "function" ? ctx.serviceAvatarInitials(name) : "??";
  }
  function humanDurationMs(ms) {
    if (typeof ctx.humanDurationMs === "function") return ctx.humanDurationMs(ms);
    if (
      globalThis.ChimeraSettings &&
      typeof ChimeraSettings.humanDurationMs === "function"
    ) {
      return ChimeraSettings.humanDurationMs(ms);
    }
    return ms != null ? String(ms) : "—";
  }
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

  function vectorstoreCollectionScopeLabelForLogs(collRaw) {
    if (typeof ctx.vectorstoreCollectionScopeLabelForLogs === "function") {
      return ctx.vectorstoreCollectionScopeLabelForLogs(collRaw);
    }
    return collRaw != null ? String(collRaw).trim() : "";
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

  /** Gateway logs upstream relay with service=gateway; bucket under chimera-broker (see summarizedFeed). */
  function entryIsGatewayUpstreamRelay(ent) {
    if (!ent || !ent.parsed) return false;
    var f = getFlat(ent.parsed);
    var msg = String(f.msg != null ? f.msg : f.message != null ? f.message : "").trim();
    if (
      msg === "chat.chimera-broker.request" ||
      msg === "upstream chat response" ||
      msg === "chat.chimera-broker.response" ||
      msg === "chat.chimera-broker.error" ||
      msg.indexOf("chimera-broker.error") >= 0
    ) {
      return true;
    }
    var sh = ent.parsed.shape || "";
    if (sh === "chat.chimera-broker" || sh.indexOf("chat.chimera-broker.") === 0) return true;
    return false;
  }

  function entryRoutesToChimeraBrokerBucket(ent) {
    if (entryIsGatewayUpstreamRelay(ent)) return true;
    var f = getFlat(ent.parsed);
    var msg = String(f.msg != null ? f.msg : f.message != null ? f.message : "").trim();
    if (msg === "chat.chimera-broker.available_models") return true;
    if (msg === "chat.routing.fallback") return true;
    if (msg === "chat.routing.attempt") return true;
    if (msg === "chat.routing.resolved") return true;
    if (msg === "chat.provider_limits.blocked") return true;
    if (msg.indexOf("virtual model fallback attempt") >= 0) return true;
    if (msg.indexOf("virtual model routing resolved") >= 0) return true;
    return false;
  }

  function badgeForServicePanel(name, ev) {
    if (name === "chimera-broker") {
      var w = { parsed: ev.parsed, text: ev.text, ts: ev.ts, source: ev.source };
      if (entryIsGatewayUpstreamRelay(w)) {
        return {
          cls: "sum-svc-broker sum-svc-badge-filled sum-svc-broker-filled",
          key: "chimera-broker",
          lab: serviceDisplayLabel("chimera-broker")
        };
      }
      return null;
    }
    if (name === "chimera-indexer") {
      var ixLab = indexerEvlogWorkspaceSourceLabel(ev);
      if (!ixLab) return null;
      return { kind: "indexer-workspace", lab: ixLab, key: ixLab };
    }
    return inferServiceBadge(ev);
  }

  var indexerEvlogWorkspaceLabelMapCacheKey = null;
  var indexerEvlogWorkspaceLabelMapCache = null;

  function indexerEvlogWorkspaceLabelMapFingerprint() {
    return [
      ctx.lastIndexerSummarizeByRun,
      ctx.lastIndexerSummarizePartitionRegistry,
      ctx.lastIndexerOperatorWorkspacesFingerprint || "",
      resolveLogsOperatorUserLabel()
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
        meta = mergePersistedIndexerWatchRoots(meta, run.events, run.id);
        var label = indexerCardTitleSortLabel(meta);
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
          byProjectFlavor[proj + "\u0000" + normalizeFlavorMatch(flav)] = label;
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
      var wsLabel = operatorManagedWorkspaceTitleText(ws);
      var wsId = canonicalWorkspaceRowIdKey(ws.id);
      var wsNumFn = ctx.operatorWorkspaceNumericId;
      var wsNum = typeof wsNumFn === "function" ? String(wsNumFn(ws)) : "";
      indexerEvlogRegisterWorkspaceLabel(map, wsLabel, [wsId, wsNum]);
      if (wsId) byWorkspaceId[wsId] = wsLabel;
      var wp = String(ws.project_id || "").trim();
      var wf = normalizeFlavorMatch(ws.flavor_id);
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
    return resolveLogsOperatorUserLabel();
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
    var flav = normalizeFlavorMatch(f.flavor_id);
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
      globalThis.ChimeraSettings &&
        ChimeraSettings.Derive &&
        typeof ChimeraSettings.Derive.indexerSlugHistogramBucket === "function"
        ? function (msg) {
          return ChimeraSettings.Derive.indexerSlugHistogramBucket(msg);
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
      ["statestats", "state / vectorstore stats"],
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

  function sliceRecent(arr, n) {
    if (!arr || !arr.length) return [];
    var take = Math.min(n, arr.length);
    return arr.slice(-take);
  }

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

  function recentServiceCardHasError(name, arr) {
    var hasErr = ctx.entryHasErrorStatus;
    var hasRl = ctx.chimeraBrokerEntryHasRateLimit;
    var slice = sliceRecent(arr, RECENT_CARD_STATUS_N);
    for (var i = 0; i < slice.length; i++) {
      if (typeof hasErr === "function" && hasErr(slice[i])) return true;
      if (name === "chimera-broker" && typeof hasRl === "function" && hasRl(slice[i])) return true;
    }
    return false;
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

  /** Gateway DEBUG traces for RAG (counts entire buffer — same window as service cards). */
  function rollupGatewayRagPipeline() {
    if (globalThis.ChimeraSettings && globalThis.ChimeraSettings.Derive && globalThis.ChimeraSettings.Derive.rollupGatewayRagPipeline) {
      return globalThis.ChimeraSettings.Derive.rollupGatewayRagPipeline(entryCache, function (p) { return getFlat(p); });
    }
    return { ragQuery: 0, ragEmbed: 0, ragHitLines: 0, embedMsSum: 0 };
  }

  function vectorstoreHttpPathRollup(arr) {
    if (globalThis.ChimeraSettings && globalThis.ChimeraSettings.Derive && globalThis.ChimeraSettings.Derive.vectorstoreHttpPathRollup) {
      return globalThis.ChimeraSettings.Derive.vectorstoreHttpPathRollup(arr, function (p) { return getFlat(p); });
    }
    return { searchN: 0, upsertN: 0, scrollN: 0 };
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
      M = ChimeraSettings.Derive.vectorstoreCardModel(arr, getFlat, vectorstoreCollectionScopeLabelForLogs);
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


  /** Operator-store workspace label for summary links (USER:PROJECT[:FLAVOR], no row id). */
  function formatIndexerSupervisedRootLabel(row) {
    if (!row || typeof row !== "object") return "—";
    var fv =
      row.flavor_id != null && String(row.flavor_id).trim() !== ""
        ? String(row.flavor_id).trim()
        : "—";
    return indexerCardTitleSortLabel({
      userLabel: resolveLogsOperatorUserLabel(),
      projectId: row.project_id != null ? String(row.project_id).trim() : "—",
      flavorId: fv
    });
  }

  function indexerCardDomIdFromMeta(meta, bucketId) {
    return "ix-" + strHash(indexerRunTimelineDedupeKey(meta, bucketId));
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
    meta = mergePersistedIndexerWatchRoots(meta, run.events, run.id);
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

  /** Same title line as IX / stale / managed WS cards (USER:PROJECT[:FLAVOR]). */
  function workspaceCardTitleFromIndexerMeta(meta) {
    return indexerCardTitleSortLabel(meta);
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
      meta = mergePersistedIndexerWatchRoots(meta, run.events, run.id);
      var lab = indexerCardTitleSortLabel(meta);
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
        indexerServiceSummaryWorkspacesHtml(svcCtx) +
        '</dd><dt>Indexer config file</dt><dd id="svc-indexer-summary-config-path">' +
        indexerServiceSummaryConfigPathHtml() +
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
      title: typeof scopedEvlogTitle === "function" ? scopedEvlogTitle(serviceDisplayLabel(name)) : "Scoped log"
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

  function sgOpInsetWellOkFailHtml(okN, failN, prefix, opts) {
    opts = opts || {};
    var lead = "";
    if (opts.leadIcon) {
      lead =
        '<span class="material-symbols-outlined material-symbols-outlined--sm" aria-hidden="true">' +
        escapeHtml(String(opts.leadIcon)) +
        "</span> ";
    } else if (prefix) {
      lead = escapeHtml(String(prefix)) + " ";
    }
    var titleAttr =
      opts.title != null && String(opts.title).trim() !== ""
        ? ' title="' + escapeHtml(String(opts.title)) + '"'
        : "";
    var out = '<span class="sg-op-inset-well"' + titleAttr + ">" + lead;
    if (opts.okIcon !== false) {
      out +=
        '<span class="material-symbols-outlined material-symbols-outlined--sm" aria-hidden="true">check_circle</span> ';
    }
    out += escapeHtml(formatInt(okN)) + " " + escapeHtml(formatInt(failN));
    if (opts.errorIcon !== false) {
      out += ' <span class="material-symbols-outlined material-symbols-outlined--sm" aria-hidden="true">error</span>';
    }
    return out + "</span>";
  }

  /** Append trailing summary chips/pills into one .sum-metrics cluster (user-card parity). */
  function summaryMetricsHtml(innerHtml, extraHtml) {
    innerHtml = innerHtml != null ? String(innerHtml) : "";
    extraHtml = extraHtml != null ? String(extraHtml) : "";
    if (!innerHtml && !extraHtml) return "";
    if (innerHtml.indexOf('class="sum-metrics"') >= 0) {
      if (!extraHtml) return innerHtml;
      return innerHtml.replace(/<\/span>\s*$/, extraHtml + "</span>");
    }
    return '<span class="sum-metrics">' + innerHtml + extraHtml + "</span>";
  }

  function serviceSummaryStatusPillHtml(st) {
    st = st || {};
    var label = st.st != null ? String(st.st) : "";
    var okStates = { active: 1, complete: 1, idle: 1, waiting: 1 };
    var variant = okStates[label] ? "ok" : "";
    var pulse = st.cls && String(st.cls).indexOf("sum-pulse") >= 0;
    if (typeof ctx.sgOpHealthPillHtml === "function") {
      return ctx.sgOpHealthPillHtml(label, variant, { pulse: pulse });
    }
    return '<span class="sum-status ' + (st.cls || "") + '">' + escapeHtml(label) + "</span>";
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
      qdrCardModel = ChimeraSettings.Derive.vectorstoreCardModel(arr, getFlat, vectorstoreCollectionScopeLabelForLogs);
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
    var ini = serviceAvatarInitials(name);
    var av = serviceAvatarClass(name);
    /** Single outer .sum-title only — avoid nesting .sum-title (was hiding pills / breaking layout). */
    var titleClass = "sum-title";
    var displayServiceName = serviceDisplayLabel(name);
    var titleBlock = escapeHtml(displayServiceName);
    var wms = serviceWindowMs(arr);
    var metrics;
    if (isChimeraBroker) {
      var bxC = chimeraBrokerCardMetrics(arr);
      metrics =
        '<span class="sum-metrics">' +
        sgOpInsetWellOkFailHtml(bxC.relayOk, bxC.relayFail, "", {
          title: chimeraBrokerRelayOkFailTooltip(bxC)
        }) +
        chimeraBrokerProviderHealthStripHtml(arr, { compact: true }) +
        "</span>";
    } else if (name === "chimera-vectorstore") {
      if (qdrCardModel) {
        var vm = qdrCardModel;
        metrics =
          '<span class="sum-metrics">' +
          sgOpInsetWellOkFailHtml(vm.upsertOk || 0, vm.upsertFail || 0, "", {
            leadIcon: "database_upload",
            title: "Upserts · success / fail (not HTTP 200)",
            okIcon: false
          }) +
          sgOpInsetWellOkFailHtml(vm.searchOk || 0, vm.searchFail || 0, "", {
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
          sgOpInsetWellOkFailHtml(gc.http2xx || 0, gc.httpNot2xx || 0, "", {
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
        (sumMs ? '<span class="sum-metric">' + escapeHtml(humanDurationMs(sumMs)) + " Σ</span>" : "") +
        "</span>";
    }
    var statusHtml = isChimeraBroker ? "" : serviceSummaryStatusPillHtml(st);
    metrics = summaryMetricsHtml(metrics, statusHtml);
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
  ctx.recentServiceCardHasError = recentServiceCardHasError;
  ctx.serviceWindowMs = serviceWindowMs;
  ctx.chimeraBrokerProviderHealthStripHtml = chimeraBrokerProviderHealthStripHtml;
  ctx.chimeraBrokerRelayOutcomeStripHtml = chimeraBrokerRelayOutcomeStripHtml;
  ctx.chimeraBrokerShortModelLabel = chimeraBrokerShortModelLabel;
  ctx.chimeraBrokerAvailableModelCountLabel = chimeraBrokerAvailableModelCountLabel;
  ctx.chimeraBrokerAvailableModelCountResolve = chimeraBrokerAvailableModelCountResolve;
  ctx.chimeraBrokerCollapsedCardSubtitle = chimeraBrokerCollapsedCardSubtitle;
  ctx.badgeForServicePanel = badgeForServicePanel;
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
  ctx.indexerEventMixHistogramHtml = indexerEventMixHistogramHtml;
  ctx.indexerHistogramLegendHtml = indexerHistogramLegendHtml;
  ctx.badgeForIndexerRunLine = badgeForIndexerRunLine;
  ctx.indexerRunProgressSubtitle = indexerRunProgressSubtitle;
  ctx.indexerFlatMsg = indexerFlatMsg;
  ctx.isIndexerStateFlat = isIndexerStateFlat;
  ctx.latestIndexerStateQueueInflightFromEntries = latestIndexerStateQueueInflightFromEntries;
  ctx.latestIndexerQueueSnapshotMetaFromEntries = latestIndexerQueueSnapshotMetaFromEntries;
  ctx.gatewayHttpOkFailTooltip = gatewayHttpOkFailTooltip;
  ctx.chimeraBrokerRelayOkFailTooltip = chimeraBrokerRelayOkFailTooltip;
  ctx.indexerQueueDepthTooltip = indexerQueueDepthTooltip;
  ctx.gatewayServicePanelMiniHtml = gatewayServicePanelMiniHtml;
  ctx.rollupGatewayRagPipeline = rollupGatewayRagPipeline;
  ctx.vectorstoreHttpPathRollup = vectorstoreHttpPathRollup;
  ctx.vectorstoreServicePanelMiniHtml = vectorstoreServicePanelMiniHtml;
  ctx.chimeraBrokerServicePanelKvHtml = chimeraBrokerServicePanelKvHtml;
  ctx.flatLooksLikeIndexerRunStart = flatLooksLikeIndexerRunStart;
  ctx.flatLooksLikeIndexerRunDone = flatLooksLikeIndexerRunDone;
  ctx.flatLooksLikeIndexerRunProgress = flatLooksLikeIndexerRunProgress;
  ctx.flatLooksLikeIndexerJobIngested = flatLooksLikeIndexerJobIngested;
  ctx.indexerRecentEvalStatusForFlat = indexerRecentEvalStatusForFlat;
  ctx.buildIndexerRecentEvaluatedFilesHtml = buildIndexerRecentEvaluatedFilesHtml;
  ctx.collectIndexerRunMeta = collectIndexerRunMeta;
  ctx.indexerCardDomIdFromMeta = indexerCardDomIdFromMeta;
  ctx.workspaceCardTitleFromIndexerMeta = workspaceCardTitleFromIndexerMeta;
  ctx.operatorManagedWorkspaceTitleText = operatorManagedWorkspaceTitleText;
  ctx.workspaceDraftComparableManagedTitle = workspaceDraftComparableManagedTitle;
  ctx.dedupeOperatorWorkspacesNested = dedupeOperatorWorkspacesNested;
  ctx.canonicalWorkspaceRowIdKey = canonicalWorkspaceRowIdKey;
  ctx.normalizeFlavorMatch = normalizeFlavorMatch;
  ctx.resolveLogsOperatorUserLabel = resolveLogsOperatorUserLabel;
  ctx.buildIndexerManagedWorkspaceSummaryRowsFromOperatorStore =
    buildIndexerManagedWorkspaceSummaryRowsFromOperatorStore;
  ctx.buildGatewayCardIntroHtml = buildGatewayCardIntroHtml;
  ctx.buildBrokerCardIntroHtml = buildBrokerCardIntroHtml;
  ctx.buildVectorstoreCardIntroHtml = buildVectorstoreCardIntroHtml;
  ctx.buildIndexerCardIntroHtml = buildIndexerCardIntroHtml;
  ctx.aggregateIndexerManagedWorkspacesHtml = aggregateIndexerManagedWorkspacesHtml;
  ctx.indexerServiceSummaryConfigPathHtml = indexerServiceSummaryConfigPathHtml;
  ctx.indexerServiceSummaryWorkspacesHtml = indexerServiceSummaryWorkspacesHtml;
  ctx.syncIndexerServiceSummaryDom = syncIndexerServiceSummaryDom;
  ctx.scheduleIndexerServiceSummaryFetch = scheduleIndexerServiceSummaryFetch;
  ctx.hydrateIndexerServiceSummaryFromApi = hydrateIndexerServiceSummaryFromApi;
  ctx.renderExpandedService = renderExpandedService;
  ctx.buildServiceCard = buildServiceCard;
  ctx.summaryMetricsHtml = summaryMetricsHtml;
  ctx.serviceSummaryStatusPillHtml = serviceSummaryStatusPillHtml;
  ctx.recentServiceCardHasError = recentServiceCardHasError;
  ctx.indexerLatestSupervisedWaitFlat = indexerLatestSupervisedWaitFlat;
  ctx.sliceRecent = sliceRecent;
  ctx.sgOpInsetWellOkFailHtml = sgOpInsetWellOkFailHtml;
  ctx.humanDurationMs = humanDurationMs;
  ctx.entryIsGatewayUpstreamRelay = entryIsGatewayUpstreamRelay;
  ctx.entryRoutesToChimeraBrokerBucket = entryRoutesToChimeraBrokerBucket;
};
