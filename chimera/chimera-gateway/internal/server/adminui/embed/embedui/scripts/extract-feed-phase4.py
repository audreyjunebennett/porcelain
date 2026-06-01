#!/usr/bin/env python3
"""Phase 4: extract log-feed card builders from summarizedFeed.js into render/cards/*.js"""
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
FEED = ROOT / "settings" / "app" / "summarizedFeed.js"
CARDS = ROOT / "settings" / "render" / "cards"

HEADER = """/**
 * Summarized feed card render (Phase 4 extraction).
 */
globalThis.ChimeraSettings = globalThis.ChimeraSettings || {};
globalThis.ChimeraSettings.Render = globalThis.ChimeraSettings.Render || {};
globalThis.ChimeraSettings.Render.Cards = globalThis.ChimeraSettings.Render.Cards || {};

"""

FILES = {
    "feedLogConv.js": {
        "mount": "mountFeedLogConv",
        "ranges": [(2086, 2611), (4174, 4306)],
        "ctx": [
            "conversationScopedLogSubject",
            "formatConversationCardTitle",
            "scrapeConversationMetrics",
            "conversationCardModelForGroup",
            "conversationRagRetrievalSummary",
            "conversationVectorsRetrievedMiniCardHtml",
            "conversationLifecycleBarHtml",
            "conversationCardChipsSummaryHtml",
            "conversationCardStatus",
            "timelineBarHtml",
            "renderExpandedConv",
            "buildConvCard",
        ],
    },
    "feedLogService.js": {
        "mount": "mountFeedLogService",
        "ranges": [(2560, 2879), (2880, 4173), (4307, 5223)],
        "ctx": [
            "recentServiceCardHasError",
            "serviceWindowMs",
            "chimeraBrokerProviderHealthStripHtml",
            "chimeraBrokerRelayOutcomeStripHtml",
            "chimeraBrokerShortModelLabel",
            "badgeForServicePanel",
            "buildIndexerEvlogWorkspaceLabelMap",
            "getIndexerEvlogWorkspaceLabelMap",
            "indexerEvlogFlatForEntry",
            "indexerEvlogLineIsProcessWide",
            "indexerEvlogUserLabelFromFlat",
            "indexerEvlogWorkspaceSourceLabel",
            "indexerHumanDeclaredState",
            "indexerLastFileEventTime",
            "indexerRelFromLatestFileLine",
            "indexerBuildCardSubtitle",
            "indexerWorkspaceFileCountFromMeta",
            "indexerWorkspaceEmbeddedChunksFromMeta",
            "indexerWorkspaceMetricWellHtml",
            "indexerWorkspaceCollapsedMetricsHtml",
            "indexerEventMixHistogramHtml",
            "indexerHistogramLegendHtml",
            "badgeForIndexerRunLine",
            "indexerRunProgressSubtitle",
            "indexerFlatMsg",
            "isIndexerStateFlat",
            "latestIndexerStateQueueInflightFromEntries",
            "latestIndexerQueueSnapshotMetaFromEntries",
            "gatewayHttpOkFailTooltip",
            "chimeraBrokerRelayOkFailTooltip",
            "indexerQueueDepthTooltip",
            "gatewayServicePanelMiniHtml",
            "rollupGatewayRagPipeline",
            "vectorstoreHttpPathRollup",
            "vectorstoreServicePanelMiniHtml",
            "chimeraBrokerServicePanelKvHtml",
            "flatLooksLikeIndexerRunStart",
            "flatLooksLikeIndexerRunDone",
            "flatLooksLikeIndexerRunProgress",
            "flatLooksLikeIndexerJobIngested",
            "indexerRecentEvalStatusForFlat",
            "buildIndexerRecentEvaluatedFilesHtml",
            "collectIndexerRunMeta",
            "buildGatewayCardIntroHtml",
            "buildBrokerCardIntroHtml",
            "buildVectorstoreCardIntroHtml",
            "buildIndexerCardIntroHtml",
            "aggregateIndexerManagedWorkspacesHtml",
            "indexerServiceSummaryConfigPathHtml",
            "indexerServiceSummaryWorkspacesHtml",
            "syncIndexerServiceSummaryDom",
            "scheduleIndexerServiceSummaryFetch",
            "hydrateIndexerServiceSummaryFromApi",
            "renderExpandedService",
            "buildServiceCard",
            "operatorCardChevronHtml",
            "summaryMetricsHtml",
            "serviceSummaryStatusPillHtml",
        ],
    },
}

# Small admin/workspace extractions (manual ranges)
SMALL = {
    "adminProvider.js": {
        "append": True,
        "ranges": [(75, 147)],
        "ctx": ["buildAdminProviderPickerHtml", "buildAdminProvidersSectionBreakHtml"],
        "extra_const": '  var ADMIN_PROVIDERS_INTRO_HTML =\n    \'<div class="sum-workspaces-intro"><p class="sum-workspaces-intro-lead">Providers drive upstream inference through chimera-broker; each card shows configuration, usage, and scoped log activity.</p></div>\';\n\n',
    },
    "workspaceDraft.js": {
        "append": True,
        "ranges": [(1584, 1694)],
        "ctx": ["buildWorkspacesCreateBtnHtml", "buildWorkspacesSectionIntroHtml"],
    },
}


def slice_lines(lines, ranges):
    chunks = []
    for a, b in ranges:
        chunks.append("".join(lines[a - 1 : b]))
    return chunks


def build_new_file(fname, spec):
    mount = spec["mount"]
    body = slice_lines(lines, spec["ranges"])
    out = HEADER + f"globalThis.ChimeraSettings.Render.Cards.{mount} = function (ctx) {{\n"
    out += "  var escapeHtml = ctx.escapeHtml;\n"
    out += "  var getFlat = ctx.getFlat;\n"
    out += "  var entryCache = ctx.entryCache;\n"
    out += "  var strHash = ctx.strHash;\n"
    out += "  var entryInstant = ctx.entryInstant;\n"
    out += "  var primaryLogMessage = ctx.primaryLogMessage;\n"
    out += "  var formatInt = ctx.formatInt;\n"
    out += "  var getViewMode = ctx.getViewMode;\n"
    out += "  var sumEvlogPanelHtml = ctx.sumEvlogPanelHtml;\n"
    out += "  var sumEvlogBuildTbodyFromConvEvents = ctx.sumEvlogBuildTbodyFromConvEvents;\n"
    out += "  var sumEvlogBuildTbodyFromServiceEntries = ctx.sumEvlogBuildTbodyFromServiceEntries;\n"
    out += "  var sumEvlogCountWarnFailFromEntries = ctx.sumEvlogCountWarnFailFromEntries;\n"
    out += "  var scopedEvlogTitle = ctx.scopedEvlogTitle;\n"
    out += "  var contextGrowthStripHtml = ctx.contextGrowthStripHtml;\n"
    out += "  var SHOW_CONV_EXPANDED_CONTEXT_STRIP = !!ctx.SHOW_CONV_EXPANDED_CONTEXT_STRIP;\n"
    out += "  var formatMergedConversationSubtitle = ctx.formatMergedConversationSubtitle;\n"
    out += "  var serviceAvatarClass = ctx.serviceAvatarClass;\n"
    out += "  var serviceAvatarInitials = ctx.serviceAvatarInitials;\n"
    out += "  var humanDurationMs = ctx.humanDurationMs;\n"
    out += "  var sgOpInsetWellOkFailHtml = ctx.sgOpInsetWellOkFailHtml;\n"
    out += "  var RECENT_CARD_STATUS_N = ctx.RECENT_CARD_STATUS_N;\n"
    out += "  var sliceRecent = ctx.sliceRecent;\n"
    if spec.get("extra_const"):
        out += spec["extra_const"]
    for chunk in body:
        out += chunk
        if not chunk.endswith("\n"):
            out += "\n"
    for fn in spec["ctx"]:
        out += f"  ctx.{fn} = {fn};\n"
    out += "};\n"
    return out


def append_to_mount_file(path, spec):
    text = path.read_text(encoding="utf-8")
    body = slice_lines(lines, spec["ranges"])
    insert_before = "  ctx.buildWorkspaceDraftCardHtml"
    if "buildAdminProviderCardHtml" in text:
        insert_before = "  ctx.buildAdminProviderCardHtml"
    idx = text.find(insert_before)
    if idx < 0:
        raise SystemExit(f"insert anchor not found in {path}")
    extra = spec.get("extra_const", "")
    if "mountAdminProvider" in text:
        extra += (
            "  function providerCatalogApi() {\n"
            "    return globalThis.ChimeraSettings &&\n"
            "      ChimeraSettings.Providers &&\n"
            "      ChimeraSettings.Providers.Catalog\n"
            "      ? ChimeraSettings.Providers.Catalog\n"
            "      : null;\n"
            "  }\n\n"
        )
    if "mountWorkspaceDraft" in text:
        extra += (
            "  var workspaceDesktopFeaturesAvailable =\n"
            "    typeof ctx.workspaceDesktopFeaturesAvailable === 'function'\n"
            "      ? ctx.workspaceDesktopFeaturesAvailable\n"
            "      : function () { return false; };\n"
            "  var wrapDesktopOnlyLockedControl =\n"
            "    typeof ctx.wrapDesktopOnlyLockedControl === 'function'\n"
            "      ? ctx.wrapDesktopOnlyLockedControl\n"
            "      : function (html) { return html; };\n\n"
        )
    chunk = extra + "".join(body)
    new_text = text[:idx] + chunk + text[idx:]
    close = new_text.rfind("};")
    binds = ""
    for fn in spec["ctx"]:
        if f"ctx.{fn} =" not in new_text:
            binds += f"  ctx.{fn} = {fn};\n"
    if binds and close >= 0:
        new_text = new_text[:close] + binds + new_text[close:]
    path.write_text(new_text, encoding="utf-8")


lines = FEED.read_text(encoding="utf-8").splitlines(keepends=True)

for fname, spec in FILES.items():
    path = CARDS / fname
    path.write_text(build_new_file(fname, spec), encoding="utf-8")
    print("wrote", path.name)

for fname, spec in SMALL.items():
    append_to_mount_file(CARDS / fname, spec)
    print("appended to", fname)

# Remove all extracted ranges from feed (merge first)
all_ranges = []
for spec in FILES.values():
    all_ranges.extend(spec["ranges"])
for spec in SMALL.values():
    all_ranges.extend(spec["ranges"])

merged = []
for a, b in sorted(all_ranges):
    if merged and a <= merged[-1][1] + 1:
        merged[-1] = (merged[-1][0], max(merged[-1][1], b))
    else:
        merged.append([a, b])

new_lines = lines[:]
for a, b in sorted(merged, key=lambda x: -x[0]):
    del new_lines[a - 1 : b]

# Insert feed log mounts before mountAll block
needle = "  ctx.workspaceDesktopFeaturesAvailable = workspaceDesktopFeaturesAvailable;"
insert_block = """
  if (globalThis.ChimeraSettings.Render && globalThis.ChimeraSettings.Render.Cards) {
    var FeedCards = globalThis.ChimeraSettings.Render.Cards;
    if (typeof FeedCards.mountFeedLogConv === "function") FeedCards.mountFeedLogConv(ctx);
    if (typeof FeedCards.mountFeedLogService === "function") FeedCards.mountFeedLogService(ctx);
  }
  var buildConvCard = ctx.buildConvCard;
  var buildServiceCard = ctx.buildServiceCard;
  var renderExpandedService = ctx.renderExpandedService;
  var chimeraBrokerProviderHealthStripHtml = ctx.chimeraBrokerProviderHealthStripHtml;
  var buildGatewayCardIntroHtml = ctx.buildGatewayCardIntroHtml;
  var buildBrokerCardIntroHtml = ctx.buildBrokerCardIntroHtml;
  var buildVectorstoreCardIntroHtml = ctx.buildVectorstoreCardIntroHtml;
  var buildIndexerCardIntroHtml = ctx.buildIndexerCardIntroHtml;
  var buildAdminProviderPickerHtml = ctx.buildAdminProviderPickerHtml;
  var buildAdminProvidersSectionBreakHtml = ctx.buildAdminProvidersSectionBreakHtml;
  var buildWorkspacesCreateBtnHtml = ctx.buildWorkspacesCreateBtnHtml;
  var buildWorkspacesSectionIntroHtml = ctx.buildWorkspacesSectionIntroHtml;
  var collectIndexerRunMeta = ctx.collectIndexerRunMeta;
  var buildIndexerRecentEvaluatedFilesHtml = ctx.buildIndexerRecentEvaluatedFilesHtml;
  var operatorCardChevronHtml = ctx.operatorCardChevronHtml;

"""
insert_at = None
for i, ln in enumerate(new_lines):
    if needle.strip() in ln:
        insert_at = i + 1
        break
if insert_at:
    new_lines.insert(insert_at, insert_block)

FEED.write_text("".join(new_lines), encoding="utf-8")
print("updated summarizedFeed.js, lines:", len(new_lines))
