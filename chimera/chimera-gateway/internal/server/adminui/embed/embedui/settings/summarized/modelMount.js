/**
 * Summarized model deps/state, card render dispatch, buildSummarizedModelForAgg.
 * Exports: ChimeraSettings.Summarized.mountModelGlue(ctx, bridge)
 */
globalThis.ChimeraSettings = globalThis.ChimeraSettings || {};
globalThis.ChimeraSettings.Summarized = globalThis.ChimeraSettings.Summarized || {};

globalThis.ChimeraSettings.Summarized.mountModelGlue = function (ctx, bridge) {
  var strHash = bridge.strHash;
  var entryInstant = bridge.entryInstant;
  var primaryLogMessage = bridge.primaryLogMessage;
  var getFlat = bridge.getFlat;
  var adminProviderSpecsFromVisible = bridge.adminProviderSpecsFromVisible;

  function summarizedModelState(agg) {
    return {
      agg: agg,
      gatewayOverviewCache: ctx.gatewayOverviewCache,
      metricsCache: ctx.metricsCache,
      adminStateCache: ctx.adminStateCache,
      tokenListCache: ctx.tokenListCache,
      workspaceDrafts: ctx.workspaceDrafts,
      adminProviderSpecs: adminProviderSpecsFromVisible(),
      virtualModelDrafts: ctx.virtualModelDrafts,
      adminProviderModelsEditingId: ctx.adminProviderModelsEditingId,
      workspaceManagedEditId: ctx.workspaceManagedEditId,
      lastIndexerOperatorWorkspacesNested: ctx.lastIndexerOperatorWorkspacesNested
    };
  }

  function summarizedModelDeps() {
    return {
      strHash: strHash,
      conversationDomIdForGroup: ctx.conversationDomIdForGroup,
      convLastTs: function (g) {
        if (
          globalThis.ChimeraSettings &&
          ChimeraSettings.Derive &&
          typeof ChimeraSettings.Derive.convLastTs === "function"
        ) {
          return ChimeraSettings.Derive.convLastTs(g, entryInstant);
        }
        return 0;
      },
      primaryLogMessage: primaryLogMessage,
      conversationCardModelForGroup: ctx.conversationCardModelForGroup,
      conversationCardStatus: ctx.conversationCardStatus,
      indexerPartitionMetaForRun: function (partitionRegistry, runId, events) {
        if (
          partitionRegistry &&
          globalThis.ChimeraSettings &&
          ChimeraSettings.Derive &&
          typeof ChimeraSettings.Derive.indexerPartitionMetaForRun === "function"
        ) {
          return ChimeraSettings.Derive.indexerPartitionMetaForRun(partitionRegistry, runId, events, getFlat);
        }
        return null;
      },
      collectIndexerRunMeta: ctx.collectIndexerRunMeta,
      mergePersistedIndexerWatchRoots: ctx.mergePersistedIndexerWatchRoots,
      indexerRunTimelineDedupeKey: ctx.indexerRunTimelineDedupeKey,
      pickCanonicalIndexerRun: ctx.pickCanonicalIndexerRun,
      workspaceCardTitleFromIndexerMeta: ctx.workspaceCardTitleFromIndexerMeta,
      indexerCardTitleSortLabel: ctx.indexerCardTitleSortLabel,
      indexerCardDomIdFromMeta: ctx.indexerCardDomIdFromMeta,
      indexerCardIdentityKey: ctx.indexerCardIdentityKey,
      indexerCardIdentityKeyFromSnap: ctx.indexerCardIdentityKeyFromSnap,
      loadIndexerWatchRootsStore: ctx.loadIndexerWatchRootsStore,
      dedupeOperatorWorkspacesNested: ctx.dedupeOperatorWorkspacesNested,
      canonicalWorkspaceRowIdKey: ctx.canonicalWorkspaceRowIdKey,
      workspaceDraftComparableManagedTitle: ctx.workspaceDraftComparableManagedTitle,
      operatorManagedWorkspaceTitleText: ctx.operatorManagedWorkspaceTitleText,
      operatorWorkspaceCoveredByIndexerRuns: ctx.operatorWorkspaceCoveredByIndexerRuns,
      operatorWorkspaceNumericId: ctx.operatorWorkspaceNumericId,
      indexerWorkspaceEditActiveForMeta: function (meta) {
        if (ctx.workspaceManagedEditId == null || !ctx.workspaceManagedStaging) return false;
        var opWs =
          typeof ctx.findOperatorWorkspaceMatchingIndexerMeta === "function"
            ? ctx.findOperatorWorkspaceMatchingIndexerMeta(meta)
            : null;
        if (!opWs) return false;
        return typeof ctx.operatorWorkspaceNumericId === "function"
          ? ctx.operatorWorkspaceNumericId(opWs) === ctx.workspaceManagedEditId
          : false;
      },
      indexerRunQualifiesForWorkspaceCard: function (run, partitionRegistry) {
        if (
          globalThis.ChimeraSettings &&
          ChimeraSettings.Derive &&
          typeof ChimeraSettings.Derive.indexerRunQualifiesForWorkspaceCard === "function"
        ) {
          return ChimeraSettings.Derive.indexerRunQualifiesForWorkspaceCard(
            run,
            partitionRegistry,
            getFlat,
            function (runId, evs, opts) {
              return typeof ctx.collectIndexerRunMeta === "function"
                ? ctx.collectIndexerRunMeta(runId, evs, opts && opts.partitionMeta)
                : null;
            },
            {
              tokenLabelByTenant: ctx.tokenLabelByTenant,
              indexerFlatMsg: function (fl) {
                return typeof ctx.indexerFlatMsg === "function" ? ctx.indexerFlatMsg(fl) : "";
              },
              flatLooksLikeIndexerRunStart: function (fl) {
                return typeof ctx.flatLooksLikeIndexerRunStart === "function"
                  ? ctx.flatLooksLikeIndexerRunStart(fl)
                  : false;
              },
              flatLooksLikeIndexerRunDone: function (fl) {
                return typeof ctx.flatLooksLikeIndexerRunDone === "function"
                  ? ctx.flatLooksLikeIndexerRunDone(fl)
                  : false;
              },
              flatLooksLikeIndexerRunProgress: function (fl) {
                return typeof ctx.flatLooksLikeIndexerRunProgress === "function"
                  ? ctx.flatLooksLikeIndexerRunProgress(fl)
                  : false;
              },
              flatLooksLikeIndexerJobIngested: function (fl) {
                return typeof ctx.flatLooksLikeIndexerJobIngested === "function"
                  ? ctx.flatLooksLikeIndexerJobIngested(fl)
                  : false;
              }
            }
          );
        }
        return true;
      },
      adminProvidersSectionBreakHtml: ctx.buildAdminProvidersSectionBreakHtml,
      virtualModelsSectionBreakHtml: function (count) {
        if (typeof ctx.buildVirtualModelsSectionBreakHtml === "function") {
          return ctx.buildVirtualModelsSectionBreakHtml(count);
        }
        if (typeof ctx.buildVirtualModelsSectionIntroHtml === "function") {
          return (
            '<div class="sum-section-label sum-feed-section-title">Virtual models</div>' +
            ctx.buildVirtualModelsSectionIntroHtml(count)
          );
        }
        return '<div class="sum-section-label sum-feed-section-title">Virtual models</div>';
      }
    };
  }

  function renderSummarizedCardFromModel(card) {
    if (!card || card.kind === "section-break") return null;
    var src = card.source;
    switch (card.kind) {
      case "gateway-overview":
        return ctx.buildGatewayOverviewCardHtml();
      case "gateway-usage":
        return ctx.buildGatewayUsageCardHtml();
      case "admin-users":
        return ctx.buildAdminUsersCardHtml();
      case "admin-provider":
        return ctx.buildAdminProviderCardHtml(src.spec.id, src.spec.title, src.spec.avatar, src.spec.subtitle);
      case "virtual-model":
        return typeof ctx.buildVirtualModelCardHtml === "function" ? ctx.buildVirtualModelCardHtml(src.vm) : null;
      case "virtual-model-draft":
        return typeof ctx.buildVirtualModelDraftCardHtml === "function"
          ? ctx.buildVirtualModelDraftCardHtml(src.draft)
          : null;
      case "conversation":
        return ctx.buildConvCard(src);
      case "service":
        return ctx.buildServiceCard(src.name, src.events, src.svcCtx);
      case "indexer":
        return ctx.buildIndexerCard(src.run, src.partitionRegistry);
      case "indexer-stale":
        return ctx.buildIndexerStaleSnapshotCard(src.bucketId, src.snap);
      case "workspace-draft":
        return ctx.buildWorkspaceDraftCardHtml(src);
      case "indexer-operator-workspace":
        return ctx.buildIndexerOperatorWorkspaceCard(src.workspace, src.partitionRegistry);
      default:
        return null;
    }
  }

  function buildSummarizedModelForAgg(agg) {
    if (
      !globalThis.ChimeraSettings ||
      !ChimeraSettings.Summarized ||
      !ChimeraSettings.Summarized.Model ||
      typeof ChimeraSettings.Summarized.Model.buildSummarizedModel !== "function"
    ) {
      return null;
    }
    return ChimeraSettings.Summarized.Model.buildSummarizedModel(
      summarizedModelDeps(),
      summarizedModelState(agg)
    );
  }

  function buildSummarizedHtmlRenderers() {
    return {
      renderCard: renderSummarizedCardFromModel,
      conversationsSectionHead: ctx.summarizedConversationsSectionHead,
      workspacesSectionHead: ctx.summarizedWorkspacesSectionHead,
      servicesSectionHead: ctx.summarizedServicesSectionHead,
      workspacesSectionIntro: ctx.buildWorkspacesSectionIntroHtml,
      buildWorkspacesCreateBtnHtml: ctx.buildWorkspacesCreateBtnHtml,
      emptyFeedMessage: ctx.summarizedEmptyFeedMessage
    };
  }

  ctx.buildSummarizedModelForAgg = buildSummarizedModelForAgg;
  ctx.renderSummarizedCardFromModel = renderSummarizedCardFromModel;
  ctx.buildSummarizedHtmlRenderers = buildSummarizedHtmlRenderers;

  return {
    summarizedModelDeps: summarizedModelDeps,
    summarizedModelState: summarizedModelState,
    buildSummarizedModelForAgg: buildSummarizedModelForAgg,
    renderSummarizedCardFromModel: renderSummarizedCardFromModel,
    buildSummarizedHtmlRenderers: buildSummarizedHtmlRenderers
  };
};
