/**
 * Summarized feed view model (Phase 4): card list, order, and content hashes (no HTML).
 */
globalThis.ChimeraSettings = globalThis.ChimeraSettings || {};
globalThis.ChimeraSettings.Summarized = globalThis.ChimeraSettings.Summarized || {};
globalThis.ChimeraSettings.Summarized.Model = globalThis.ChimeraSettings.Summarized.Model || {};

(function () {
  var Render = globalThis.ChimeraSettings.Summarized.Render;
  var Hash = globalThis.ChimeraSettings.Summarized.Hash;
  var SECTION_OVERVIEW = Render ? Render.SECTION_OVERVIEW : "overview";
  var SECTION_CONVERSATIONS = Render ? Render.SECTION_CONVERSATIONS : "conversations";
  var SECTION_WORKSPACES = Render ? Render.SECTION_WORKSPACES : "workspaces";
  var SECTION_SERVICES = Render ? Render.SECTION_SERVICES : "services";

  function makeCard(id, kind, section, sortKey, summary, body, source) {
    var hash = Hash.cardContentHash(
      depsOrThrow().strHash,
      summary,
      body
    );
    return {
      id: id,
      kind: kind,
      section: section,
      sortKey: sortKey,
      hash: hash,
      summary: summary,
      body: body,
      source: source
    };
  }

  var _depsRef = null;
  function depsOrThrow() {
    if (!_depsRef) throw new Error("buildSummarizedModel: missing deps");
    return _depsRef;
  }

  function pushSectionBreak(cards, html, sortKey) {
    cards.push({
      id: "section-break-" + String(sortKey),
      kind: "section-break",
      section: SECTION_OVERVIEW,
      sortKey: sortKey,
      hash: Hash.cardContentHash(depsOrThrow().strHash, { html: html }, {}),
      summary: { html: html },
      body: {},
      source: null
    });
  }

  function buildOverviewCards(cards, state, deps) {
    var gwOv = state.gatewayOverviewCache || {};
    var metrics = state.metricsCache || {};
    cards.push(
      makeCard(
        "gw-overview",
        "gateway-overview",
        SECTION_OVERVIEW,
        "00-gw-overview",
        {
          semver: gwOv.semver,
          virtualModelId: gwOv.virtual_model_id,
          refreshedAt: gwOv.service_overview && gwOv.service_overview.refreshed_at
        },
        { serviceCount: gwOv.service_overview && gwOv.service_overview.services ? gwOv.service_overview.services.length : 0 },
        { cache: gwOv }
      )
    );
    cards.push(
      makeCard(
        "gw-usage-metrics",
        "gateway-usage",
        SECTION_OVERVIEW,
        "01-gw-usage",
        { storeOpen: metrics.metrics_store_open, rowCount: metrics.rows ? metrics.rows.length : 0 },
        { message: metrics.message || "" },
        { cache: metrics }
      )
    );
    cards.push(
      makeCard(
        "admin-users",
        "admin-users",
        SECTION_OVERVIEW,
        "02-admin-users",
        { tokenCount: state.tokenListCache ? state.tokenListCache.length : 0 },
        { drafts: state.adminUserDrafts ? state.adminUserDrafts.length : 0 },
        { tokens: state.tokenListCache }
      )
    );
    if (deps.adminProvidersSectionBreakHtml) {
      pushSectionBreak(cards, deps.adminProvidersSectionBreakHtml(), "03-providers-label");
    }
    var specs = state.adminProviderSpecs || [];
    for (var pi = 0; pi < specs.length; pi++) {
      var spec = specs[pi];
      var prow =
        state.adminStateCache &&
        state.adminStateCache.providers &&
        state.adminStateCache.providers[spec.id]
          ? state.adminStateCache.providers[spec.id]
          : {};
      var keys = prow.keys && Array.isArray(prow.keys) ? prow.keys.length : 0;
      cards.push(
        makeCard(
          "admin-provider-" + spec.id,
          "admin-provider",
          SECTION_OVERVIEW,
          "04-provider-" + spec.id,
          {
            providerId: spec.id,
            keyCount: keys,
            ok: !!prow.ok,
            modelsAvailable: prow.models_available_count != null ? prow.models_available_count : 0,
            modelsConfigured: !!prow.models_configured,
            modelsEditing: state.adminProviderModelsEditingId === spec.id
          },
          { spec: spec },
          { providerId: spec.id, spec: spec }
        )
      );
    }
    var gw = (state.adminStateCache && state.adminStateCache.gateway) || {};
    var vms = gw.virtual_models && Array.isArray(gw.virtual_models) ? gw.virtual_models : [];
    var vmDrafts =
      state.virtualModelDrafts && Array.isArray(state.virtualModelDrafts) ? state.virtualModelDrafts : [];
    if (deps.virtualModelsSectionBreakHtml) {
      pushSectionBreak(cards, deps.virtualModelsSectionBreakHtml(vms.length), "05-virtual-models-label");
    }
    for (var vdi = 0; vdi < vmDrafts.length; vdi++) {
      var vdraft = vmDrafts[vdi];
      if (!vdraft || vdraft.id == null) continue;
      cards.push(
        makeCard(
          "virtual-model-draft-" + String(vdraft.id),
          "virtual-model-draft",
          SECTION_OVERVIEW,
          "05-vm-draft-" + String(vdraft.id),
          { draftId: vdraft.id, name: vdraft.name, version: vdraft.version },
          { draft: vdraft },
          { draft: vdraft }
        )
      );
    }
    for (var vi = 0; vi < vms.length; vi++) {
      var vm = vms[vi];
      cards.push(
        makeCard(
          "virtual-model-" + String(vm.id),
          "virtual-model",
          SECTION_OVERVIEW,
          "05-vm-" + String(vm.id),
          {
            modelId: vm.model_id,
            enabled: !!vm.enabled,
            fallbackDepth: vm.fallback_depth,
            routing: !!vm.routing_policy_enabled,
            toolRouter: !!vm.tool_router_enabled
          },
          { vm: vm },
          { vm: vm }
        )
      );
    }
  }

  function buildConversationCards(cards, agg, deps) {
    var mergedConv = agg.mergedConv || [];
    for (var ci = 0; ci < mergedConv.length; ci++) {
      var g = mergedConv[ci];
      var id = deps.conversationDomIdForGroup(g);
      var seqSig = Hash.eventSeqSignature(g.events);
      var lastEv = g.events.length ? g.events[g.events.length - 1] : null;
      var lastMsg = lastEv && deps.primaryLogMessage ? deps.primaryLogMessage(lastEv.parsed, lastEv.text) : "";
      var cardModel = deps.conversationCardModelForGroup ? deps.conversationCardModelForGroup(g.events) : {};
      var st = deps.conversationCardStatus ? deps.conversationCardStatus(g, null, cardModel) : { st: "active" };
      cards.push(
        makeCard(
          id,
          "conversation",
          SECTION_CONVERSATIONS,
          deps.convLastTs ? deps.convLastTs(g) : 0,
          {
            pid: g.pid,
            cid: g.cid,
            status: st.st,
            lastMsg: lastMsg,
            eventCount: seqSig.count,
            lastSeq: seqSig.lastSeq
          },
          seqSig,
          g
        )
      );
    }
  }

  function buildServiceCards(cards, agg, deps) {
    var order =
      globalThis.ChimeraSettings.Summarized.SERVICE_BUCKET_ORDER ||
      deps.serviceBucketOrder ||
      ["chimera-broker", "chimera-gateway", "chimera-indexer", "chimera-vectorstore"];
    var buckets = agg.buckets || {};
    for (var oi = 0; oi < order.length; oi++) {
      var nm = order[oi];
      var arr = buckets[nm];
      if (!arr || !arr.length) continue;
      var id = "svc-" + deps.strHash(nm);
      var seqSig = Hash.eventSeqSignature(arr);
      var last = arr[arr.length - 1];
      var lastMsg =
        last && deps.primaryLogMessage ? deps.primaryLogMessage(last.parsed, last.text) : "";
      cards.push(
        makeCard(
          id,
          "service",
          SECTION_SERVICES,
          nm,
          { service: nm, lineCount: arr.length, lastMsg: lastMsg, lastSeq: seqSig.lastSeq },
          seqSig,
          { name: nm, events: arr, svcCtx: { byRun: agg.byRun, partitionRegistry: agg.partitionRegistry } }
        )
      );
    }
  }

  function buildWorkspaceCards(cards, agg, state, deps) {
    var byRun = agg.byRun || {};
    var partitionRegistry = agg.partitionRegistry || {};
    var dedupeGroups = {};
    var seenIndexerBuckets = {};
    var liveIndexerIdentities = {};
    var rks = Object.keys(byRun);
    var rj;
    for (rj = 0; rj < rks.length; rj++) {
      var runG = byRun[rks[rj]];
      if (!runG) continue;
      if (
        deps.indexerRunQualifiesForWorkspaceCard &&
        !deps.indexerRunQualifiesForWorkspaceCard(runG, partitionRegistry)
      ) {
        continue;
      }
      var pmetaG = deps.indexerPartitionMetaForRun
        ? deps.indexerPartitionMetaForRun(partitionRegistry, runG.id, runG.events)
        : null;
      var metaG = deps.collectIndexerRunMeta(runG.id, runG.events, pmetaG);
      metaG = deps.mergePersistedIndexerWatchRoots(metaG, runG.events, runG.id);
      var dk = deps.indexerRunTimelineDedupeKey(metaG, runG.id);
      if (!dedupeGroups[dk]) dedupeGroups[dk] = [];
      dedupeGroups[dk].push(runG);
    }
    var headlinesWithIndexerOrStaleCard = Object.create(null);
    var dkIter;
    for (dkIter in dedupeGroups) {
      if (!Object.prototype.hasOwnProperty.call(dedupeGroups, dkIter)) continue;
      var grpRuns = dedupeGroups[dkIter];
      var run = deps.pickCanonicalIndexerRun(grpRuns);
      if (!run) continue;
      if (
        deps.indexerRunQualifiesForWorkspaceCard &&
        !deps.indexerRunQualifiesForWorkspaceCard(run, partitionRegistry)
      ) {
        continue;
      }
      var gi;
      for (gi = 0; gi < grpRuns.length; gi++) {
        seenIndexerBuckets[grpRuns[gi].id] = true;
      }
      var pmetaLive = deps.indexerPartitionMetaForRun
        ? deps.indexerPartitionMetaForRun(partitionRegistry, run.id, run.events)
        : null;
      var metaLive = deps.collectIndexerRunMeta(run.id, run.events, pmetaLive);
      metaLive = deps.mergePersistedIndexerWatchRoots(metaLive, run.events, run.id);
      liveIndexerIdentities[deps.indexerCardIdentityKey(metaLive)] = true;
      var ixHead = deps.workspaceCardTitleFromIndexerMeta(metaLive);
      if (ixHead) headlinesWithIndexerOrStaleCard[ixHead] = true;
      var domId = deps.indexerCardDomIdFromMeta(metaLive, run.id);
      var seqSig = Hash.eventSeqSignature(run.events);
      cards.push(
        makeCard(
          domId,
          "indexer",
          SECTION_WORKSPACES,
          deps.indexerCardTitleSortLabel(metaLive) + "\u0001" + String(run.id || ""),
          {
            runId: run.id,
            title: ixHead,
            doneSeen: !!metaLive.doneSeen,
            eventCount: seqSig.count,
            workspaceEditing: deps.indexerWorkspaceEditActiveForMeta
              ? !!deps.indexerWorkspaceEditActiveForMeta(metaLive)
              : false
          },
          seqSig,
          { run: run, partitionRegistry: partitionRegistry }
        )
      );
    }
    var snapStore = deps.loadIndexerWatchRootsStore ? deps.loadIndexerWatchRootsStore() : { snapshots: {} };
    if (snapStore.snapshots) {
      for (var sbi in snapStore.snapshots) {
        if (!Object.prototype.hasOwnProperty.call(snapStore.snapshots, sbi)) continue;
        if (seenIndexerBuckets[sbi]) continue;
        var sn = snapStore.snapshots[sbi];
        if (liveIndexerIdentities[deps.indexerCardIdentityKeyFromSnap(sn)]) continue;
        var staleHead = deps.workspaceCardTitleFromIndexerMeta({
          userLabel: sn.userLabel,
          projectId: sn.projectId,
          flavorId: sn.flavorId
        });
        if (staleHead) headlinesWithIndexerOrStaleCard[staleHead] = true;
        var staleId = "ix-stale-" + deps.strHash(String(sbi));
        cards.push(
          makeCard(
            staleId,
            "indexer-stale",
            SECTION_WORKSPACES,
            deps.indexerCardTitleSortLabel(sn) + "\u0001" + String(sbi),
            { bucketId: sbi, title: staleHead },
            {},
            { bucketId: sbi, snap: sn }
          )
        );
      }
    }
    var wsn = deps.dedupeOperatorWorkspacesNested
      ? deps.dedupeOperatorWorkspacesNested((state.lastIndexerOperatorWorkspacesNested || []).slice())
      : [];
    wsn.sort(function (a, b) {
      var ak = deps.canonicalWorkspaceRowIdKey(a.id);
      var bk = deps.canonicalWorkspaceRowIdKey(b.id);
      var an = parseInt(ak, 10);
      var bn = parseInt(bk, 10);
      if (/^\d+$/.test(ak) && /^\d+$/.test(bk) && !isNaN(an) && !isNaN(bn)) return an - bn;
      return String(ak).localeCompare(String(bk));
    });
    var seenManagedWsTitle = Object.create(null);
    var drafts = state.workspaceDrafts || [];
    var wdx;
    for (wdx = 0; wdx < drafts.length; wdx++) {
      var draftHead = deps.workspaceDraftComparableManagedTitle(drafts[wdx]);
      if (draftHead) seenManagedWsTitle[draftHead] = true;
      var draftId = "ws-draft-" + String(drafts[wdx].id != null ? drafts[wdx].id : wdx);
      cards.push(
        makeCard(
          draftId,
          "workspace-draft",
          SECTION_WORKSPACES,
          "draft-" + String(wdx),
          { draftId: drafts[wdx].id, title: draftHead },
          {},
          drafts[wdx]
        )
      );
    }
    if (wsn && wsn.length) {
      for (var owi = 0; owi < wsn.length; owi++) {
        var ows = wsn[owi];
        if (!ows || ows.id == null) continue;
        var headTtl = deps.operatorManagedWorkspaceTitleText(ows);
        if (seenManagedWsTitle[headTtl]) continue;
        if (headlinesWithIndexerOrStaleCard[headTtl]) continue;
        seenManagedWsTitle[headTtl] = true;
        if (deps.operatorWorkspaceCoveredByIndexerRuns(ows, byRun, partitionRegistry)) continue;
        var opId = "ix-opws-" + deps.strHash(String(ows.id));
        var wsNumOp =
          typeof deps.operatorWorkspaceNumericId === "function" ? deps.operatorWorkspaceNumericId(ows) : 0;
        var wsEditing =
          state.workspaceManagedEditId != null &&
          wsNumOp > 0 &&
          state.workspaceManagedEditId === wsNumOp;
        cards.push(
          makeCard(
            opId,
            "indexer-operator-workspace",
            SECTION_WORKSPACES,
            headTtl + "\u0001opws-" + deps.canonicalWorkspaceRowIdKey(ows.id),
            { workspaceId: ows.id, title: headTtl, editing: wsEditing },
            {},
            { workspace: ows, partitionRegistry: partitionRegistry }
          )
        );
      }
    }
  }

  /**
   * @param {object} deps helpers from summarizedFeed mount
   * @param {{ agg: object, gatewayOverviewCache, metricsCache, adminStateCache, tokenListCache, workspaceDrafts, adminProviderSpecs, lastIndexerOperatorWorkspacesNested }} state
   * @returns {{ cards: object[], meta: object }}
   */
  function buildSummarizedModel(deps, state) {
    _depsRef = deps;
    state = state || {};
    var agg = state.agg || {};
    var cards = [];
    buildOverviewCards(cards, state, deps);
    buildConversationCards(cards, agg, deps);
    buildWorkspaceCards(cards, agg, state, deps);
    buildServiceCards(cards, agg, deps);
    _depsRef = null;

    var hasConversations = false;
    var hasWorkspaces = false;
    var hasServices = false;
    var ci;
    for (ci = 0; ci < cards.length; ci++) {
      if (cards[ci].section === SECTION_CONVERSATIONS) hasConversations = true;
      if (cards[ci].section === SECTION_WORKSPACES && cards[ci].kind !== "section-break") hasWorkspaces = true;
      if (cards[ci].section === SECTION_SERVICES) hasServices = true;
    }
    var hasThreads =
      hasConversations || hasWorkspaces || hasServices || (state.workspaceDrafts && state.workspaceDrafts.length > 0);

    return {
      cards: cards,
      meta: {
        hasThreads: hasThreads,
        cardCount: cards.length,
        builtAt: Date.now()
      }
    };
  }

  globalThis.ChimeraSettings.Summarized.Model.buildSummarizedModel = buildSummarizedModel;
  globalThis.ChimeraSettings.Summarized.Model.makeCard = makeCard;
})();
