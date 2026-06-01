#!/usr/bin/env python3
"""Phase 4b: extract indexer card builders from summarizedFeed.js."""
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
FEED = ROOT / "settings" / "app" / "summarizedFeed.js"
CARDS = ROOT / "settings" / "render" / "cards"

HEADER = """/**
 * Summarized feed indexer cards (Phase 4b).
 */
globalThis.ChimeraSettings = globalThis.ChimeraSettings || {};
globalThis.ChimeraSettings.Render = globalThis.ChimeraSettings.Render || {};
globalThis.ChimeraSettings.Render.Cards = globalThis.ChimeraSettings.Render.Cards || {};

"""

INDEXER_SCOPE_PROGRESS_FN = """
  function indexerScopeProgressTimelineBarHtml(pRem, qTot, doneSeen) {
    var timelineSegmentsHtml = ctx.timelineSegmentsHtml;
    if (typeof timelineSegmentsHtml !== "function") return "";
    var orange = "#ffa726";
    if (doneSeen) {
      return timelineSegmentsHtml([{ pct: 100, bg: orange }]);
    }
    if (
      pRem !== null &&
      !isNaN(Number(pRem)) &&
      Number(pRem) === 0 &&
      qTot !== null &&
      !isNaN(Number(qTot)) &&
      Number(qTot) >= 0
    ) {
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

"""

FILES = {
    "indexerRun.js": {
        "mount": "mountFeedLogIndexerRun",
        "ranges": [(1830, 2385), (2738, 2910)],
        "extra": INDEXER_SCOPE_PROGRESS_FN,
        "ctx": [
            "indexerExpandedSummaryKvInnerHtml",
            "renderExpandedIndexer",
            "emptyIndexerWatchRootsStore",
            "normalizeIndexerWatchRootsStore",
            "loadIndexerWatchRootsStore",
            "saveIndexerWatchRootsStore",
            "latestIndexRunIdFromEvs",
            "indexerScopeKeyFromMetaAndEvs",
            "indexerRunTimelineDedupeKey",
            "pickCanonicalIndexerRun",
            "indexerCardIdentityKey",
            "indexerCardIdentityKeyFromSnap",
            "indexerCardTitleSortLabel",
            "persistIndexerWatchRoots",
            "rememberIndexerCardSnapshot",
            "buildIndexerStaleSnapshotCard",
            "mergePersistedIndexerWatchRoots",
            "indexerMetaForBucketDedup",
            "indexerScopeProgressTimelineBarHtml",
            "buildIndexerCard",
        ],
    },
    "indexerWorkspace.js": {
        "mount": "mountFeedLogIndexerWorkspace",
        "ranges": [(1361, 1405), (2386, 2475), (2659, 2736)],
        "ctx": [
            "dirBasenameForWorkspace",
            "formatWatchPathDisplayLine",
            "formatWatchPathsPreHtml",
            "applyOperatorWorkspacePathsToMeta",
            "mergeOperatorStorePathsIntoIndexerMeta",
            "operatorWorkspaceCoveredByIndexerRuns",
            "operatorWorkspaceNumericId",
            "findOperatorWorkspaceByNumericId",
            "findOperatorWorkspaceMatchingIndexerMeta",
            "buildIndexerOperatorWorkspaceCard",
        ],
    },
}

REMOVE_RANGES = [
    (1319, 1327),
    (1361, 1405),
    (1830, 2385),
    (2738, 2910),
    (2386, 2475),
    (2659, 2736),
]


def slice_lines(lines, ranges):
    chunks = []
    for a, b in ranges:
        chunks.append("".join(lines[a - 1 : b]))
    return chunks


def build_new_file(spec):
    mount = spec["mount"]
    body = slice_lines(lines, spec["ranges"])
    out = HEADER + f"globalThis.ChimeraSettings.Render.Cards.{mount} = function (ctx) {{\n"
    out += "  var escapeHtml = ctx.escapeHtml;\n"
    out += "  var getFlat = ctx.getFlat;\n"
    out += "  var entryCache = ctx.entryCache;\n"
    out += "  var strHash = ctx.strHash;\n"
    out += "  var entryInstant = ctx.entryInstant;\n"
    out += "  var formatInt = ctx.formatInt;\n"
    out += "  var sumEvlogPanelHtml = ctx.sumEvlogPanelHtml;\n"
    out += "  var sumEvlogBuildTbodyFromServiceEntries = ctx.sumEvlogBuildTbodyFromServiceEntries;\n"
    out += "  var sumEvlogCountWarnFailFromEntries = ctx.sumEvlogCountWarnFailFromEntries;\n"
    out += "  var scopedEvlogTitle = ctx.scopedEvlogTitle;\n"
    out += "  var RECENT_CARD_STATUS_N = ctx.RECENT_CARD_STATUS_N;\n"
    out += "  var sliceRecent = ctx.sliceRecent;\n"
    out += "  var countErrorSignalsInEntries = ctx.countErrorSignalsInEntries;\n"
    out += "  var collectIndexerRunMeta = ctx.collectIndexerRunMeta;\n"
    out += "  var indexerBuildCardSubtitle = ctx.indexerBuildCardSubtitle;\n"
    out += "  var indexerWorkspaceCollapsedMetricsHtml = ctx.indexerWorkspaceCollapsedMetricsHtml;\n"
    out += "  var operatorCardChevronHtml = ctx.operatorCardChevronHtml;\n"
    out += "  var serviceSummaryStatusPillHtml = ctx.serviceSummaryStatusPillHtml;\n"
    out += "  var buildIndexerRecentEvaluatedFilesHtml = ctx.buildIndexerRecentEvaluatedFilesHtml;\n"
    out += "  var indexerCardDomIdFromMeta = ctx.indexerCardDomIdFromMeta;\n"
    out += "  var workspaceCardTitleFromIndexerMeta = ctx.workspaceCardTitleFromIndexerMeta;\n"
    out += "  var resolveLogsOperatorUserLabel = ctx.resolveLogsOperatorUserLabel;\n"
    out += "  var canonicalWorkspaceRowIdKey = ctx.canonicalWorkspaceRowIdKey;\n"
    out += "  var operatorWorkspacePaths = ctx.operatorWorkspacePaths;\n"
    out += "  var pathsSetEqualForIndexerRoots = ctx.pathsSetEqualForIndexerRoots;\n"
    out += "  var normalizeFlavorMatch = ctx.normalizeFlavorMatch;\n"
    out += "  var filterEventsForIndexerScopeFullLog = ctx.filterEventsForIndexerScopeFullLog;\n"
    if spec.get("extra"):
        out += spec["extra"]
    for chunk in body:
        out += chunk
        if not chunk.endswith("\n"):
            out += "\n"
    for fn in spec["ctx"]:
        out += f"  ctx.{fn} = {fn};\n"
    out += "};\n"
    return out


lines = FEED.read_text(encoding="utf-8").splitlines(keepends=True)

for fname, spec in FILES.items():
    path = CARDS / fname
    path.write_text(build_new_file(spec), encoding="utf-8")
    print("wrote", path.name)

merged = []
for a, b in sorted(REMOVE_RANGES):
    if merged and a <= merged[-1][1] + 1:
        merged[-1] = (merged[-1][0], max(merged[-1][1], b))
    else:
        merged.append([a, b])

new_lines = lines[:]
for a, b in sorted(merged, key=lambda x: -x[0]):
    del new_lines[a - 1 : b]

early_mount = """  if (
    globalThis.ChimeraSettings.Render &&
    globalThis.ChimeraSettings.Render.Cards &&
    typeof ChimeraSettings.Render.Cards.mountFeedLogIndexerWorkspace === "function"
  ) {
    ChimeraSettings.Render.Cards.mountFeedLogIndexerWorkspace(ctx);
  }

"""
rebuild_needle = "  if (\n    globalThis.ChimeraSettings.Summarized &&\n    typeof ChimeraSettings.Summarized.mountRebuildPolicy === \"function\"\n  ) {"
for i, ln in enumerate(new_lines):
    if rebuild_needle in "".join(new_lines[i : i + 4]):
        new_lines.insert(i, early_mount)
        break

late_needle = "    if (typeof FeedCards.mountFeedLogService === \"function\") FeedCards.mountFeedLogService(ctx);"
late_insert = """    if (typeof FeedCards.mountFeedLogIndexerRun === "function") FeedCards.mountFeedLogIndexerRun(ctx);
"""
for i, ln in enumerate(new_lines):
    if late_needle in ln:
        new_lines.insert(i + 1, late_insert)
        break

alias_needle = "  var indexerLatestSupervisedWaitFlat = ctx.indexerLatestSupervisedWaitFlat;"
alias_block = """  var buildIndexerCard = ctx.buildIndexerCard;
  var buildIndexerStaleSnapshotCard = ctx.buildIndexerStaleSnapshotCard;
  var buildIndexerOperatorWorkspaceCard = ctx.buildIndexerOperatorWorkspaceCard;
  var mergePersistedIndexerWatchRoots = ctx.mergePersistedIndexerWatchRoots;
  var indexerRunTimelineDedupeKey = ctx.indexerRunTimelineDedupeKey;
  var pickCanonicalIndexerRun = ctx.pickCanonicalIndexerRun;
  var indexerCardDomIdFromMeta = ctx.indexerCardDomIdFromMeta;
  var loadIndexerWatchRootsStore = ctx.loadIndexerWatchRootsStore;
  var indexerCardIdentityKey = ctx.indexerCardIdentityKey;
  var indexerCardIdentityKeyFromSnap = ctx.indexerCardIdentityKeyFromSnap;
  var operatorWorkspaceCoveredByIndexerRuns = ctx.operatorWorkspaceCoveredByIndexerRuns;
  var findOperatorWorkspaceMatchingIndexerMeta = ctx.findOperatorWorkspaceMatchingIndexerMeta;
  var renderExpandedIndexer = ctx.renderExpandedIndexer;
"""
for i, ln in enumerate(new_lines):
    if alias_needle in ln:
        new_lines.insert(i + 1, alias_block)
        break

# operatorWorkspaceNumericId now from workspace mount
num_needle = "  ctx.operatorWorkspaceNumericId = operatorWorkspaceNumericId;"
for i, ln in enumerate(new_lines):
    if num_needle in ln:
        del new_lines[i]
        break

FEED.write_text("".join(new_lines), encoding="utf-8")
print("updated summarizedFeed.js, lines:", len(new_lines))
