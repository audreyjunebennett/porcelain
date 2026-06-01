package embedui_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/dop251/goja"
)

// requiredCtxExportsAfterCardMount lists APIs handlers and summarizedFeed read from ctx
// after mountAll + feed-log card mounts (see summarizedFeed.js mount order).
var requiredCtxExportsAfterCardMount = []string{
	"pickFolderForWorkspaceDraft",
	"findWorkspaceDraft",
	"removeWorkspaceDraft",
	"appendWorkspaceDraftPath",
	"saveWorkspaceDraftById",
	"notifyWorkspaceDraftMsg",
	"buildWorkspaceDraftCardHtml",
	"buildConvCard",
	"buildServiceCard",
	"buildIndexerCard",
	"collectIndexerRunMeta",
	"workspaceCardTitleFromIndexerMeta",
	"indexerCardDomIdFromMeta",
	"indexerRunTimelineDedupeKey",
	"pickCanonicalIndexerRun",
	"normalizeFlavorMatch",
	"resolveLogsOperatorUserLabel",
	"operatorManagedWorkspaceTitleText",
	"ragCollectionLabelForUi",
	"vectorstoreCollectionScopeLabelForLogs",
	"hydrateIndexerServiceSummaryFromApi",
	"buildGatewayOverviewCardHtml",
	"buildAdminProviderCardHtml",
	"buildVirtualModelCardHtml",
	"conversationCardModelForGroup",
	"conversationCardStatus",
	"buildAdminProvidersSectionBreakHtml",
	"chimeraBrokerShortModelLabel",
	"chimeraBrokerAvailableModelCountLabel",
	"chimeraBrokerCollapsedCardSubtitle",
	"chimeraBrokerProviderHealthStripHtml",
	"syncIndexerServiceSummaryDom",
	"scheduleIndexerServiceSummaryFetch",
	"avatarInitials",
	"avatarHueClass",
	"tryRegisterRequestConversationCorrelationPrimary",
	"tryRegisterRequestConversationCorrelationRagFallback",
	"pushConversationGroupedEvent",
	"conversationRequestIdTier2EligibleLocal",
	"conversationIndexRunTier3EligibleLocal",
	"entryIsVectorstoreSubprocessForConvJoin",
	"serviceDisplayLabel",
	"inferServiceBadge",
	"sgOpInsetWellOkFailHtml",
	"humanDurationMs",
	"sumEvlogVisibleEntriesForService",
	"countWarnErrorInEntries",
	"entryIsGatewayUpstreamRelay",
	"entryRoutesToChimeraBrokerBucket",
}

func TestFeedMount_requiredCtxExports(t *testing.T) {
	vm := goja.New()
	loadCardTestCtx(t, vm)

	keysJSON, err := json.Marshal(requiredCtxExportsAfterCardMount)
	if err != nil {
		t.Fatal(err)
	}
	_, err = vm.RunString(`
		var missing = [];
		var required = ` + string(keysJSON) + `;
		for (var i = 0; i < required.length; i++) {
			var k = required[i];
			if (typeof ctx[k] !== "function") missing.push(k);
		}
		if (missing.length) {
			throw new Error("missing ctx exports after card mount: " + missing.join(", "));
		}
	`)
	if err != nil {
		t.Fatal(err)
	}
}

// summarizedFeed must not assign ctx.KEY = KEY for symbols moved to card modules (undefined locals).
func TestSummarizedFeed_noGhostCtxReexports(t *testing.T) {
	t.Helper()
	path := settingsUIPath(t, "app", "summarizedFeed.js")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	cardOwned := []string{
		"pickFolderForWorkspaceDraft",
		"appendWorkspaceDraftPath",
		"saveWorkspaceDraftById",
		"workspaceCardTitleFromIndexerMeta",
		"indexerCardTitleSortLabel",
		"buildIndexerManagedWorkspaceSummaryRowsFromOperatorStore",
		"indexerServiceSummaryWorkspacesHtml",
		"ragCollectionLabelForUi",
		"vectorstoreCollectionScopeLabelForLogs",
		"findWorkspaceDraft",
		"removeWorkspaceDraft",
	}
	for _, sym := range cardOwned {
		pat := regexp.MustCompile(`ctx\.` + regexp.QuoteMeta(sym) + `\s*=\s*` + regexp.QuoteMeta(sym) + `\s*;`)
		if loc := pat.FindIndex(body); loc != nil {
			t.Fatalf("summarizedFeed.js ghost re-export ctx.%s = %s at byte %d", sym, sym, loc[0])
		}
	}
}

// modelDepsBareRHS maps summarizedModelDeps property keys to identifiers that must not
// appear bare on the RHS (card modules export them on ctx after mount).
var modelDepsBareRHS = map[string]string{
	"conversationCardModelForGroup":         "conversationCardModelForGroup",
	"conversationCardStatus":                "conversationCardStatus",
	"collectIndexerRunMeta":                 "collectIndexerRunMeta",
	"mergePersistedIndexerWatchRoots":       "mergePersistedIndexerWatchRoots",
	"operatorWorkspaceCoveredByIndexerRuns": "operatorWorkspaceCoveredByIndexerRuns",
	"operatorWorkspaceNumericId":            "operatorWorkspaceNumericId",
	"adminProvidersSectionBreakHtml":        "buildAdminProvidersSectionBreakHtml",
}

func TestSummarizedModelDeps_cardRefsUseCtx(t *testing.T) {
	t.Helper()
	path := settingsUIPath(t, "summarized", "modelMount.js")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	start := bytes.Index(body, []byte("function summarizedModelDeps()"))
	if start < 0 {
		t.Fatal("summarizedModelDeps not found")
	}
	rest := body[start:]
	endRel := bytes.Index(rest, []byte("\n  function buildSummarizedModelForAgg"))
	if endRel < 0 {
		t.Fatal("buildSummarizedModelForAgg not found after summarizedModelDeps")
	}
	block := rest[:endRel]
	for prop, bare := range modelDepsBareRHS {
		pat := regexp.MustCompile(regexp.QuoteMeta(prop) + `:\s*` + regexp.QuoteMeta(bare) + `\s*,`)
		if loc := pat.FindIndex(block); loc != nil {
			t.Fatalf("summarizedModelDeps uses bare %s for %s (use ctx.%s) at offset %d in block",
				bare, prop, bare, loc[0])
		}
	}
}

func TestFeedLogService_brokerRelayHelperOnCtx(t *testing.T) {
	t.Helper()
	path := cardsUIPath(t, "serviceFeed.js")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(body, []byte("ctx.entryIsGatewayUpstreamRelay = entryIsGatewayUpstreamRelay")) {
		t.Fatal("serviceFeed.js must export entryIsGatewayUpstreamRelay on ctx")
	}
}

func TestFeedLogService_sumEvlogUsesCtx(t *testing.T) {
	t.Helper()
	path := cardsUIPath(t, "serviceFeed.js")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if pat := regexp.MustCompile(`[^.]sumEvlogVisibleEntriesForService\s*\(`); pat.Find(body) != nil {
		t.Fatal("serviceFeed.js must call ctx.sumEvlogVisibleEntriesForService, not bare sumEvlogVisibleEntriesForService")
	}
	if pat := regexp.MustCompile(`[^.]countWarnErrorInEntries\s*\(`); pat.Find(body) != nil {
		t.Fatal("serviceFeed.js must call ctx.countWarnErrorInEntries, not bare countWarnErrorInEntries")
	}
}

func TestFeedLogService_noBareIndexerCrossModuleCalls(t *testing.T) {
	t.Helper()
	path := cardsUIPath(t, "serviceFeed.js")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	badPat := []*regexp.Regexp{
		regexp.MustCompile(`[^.]mergePersistedIndexerWatchRoots\s*\(`),
		regexp.MustCompile(`[^.]indexerCardTitleSortLabel\s*\(`),
		regexp.MustCompile(`[^.]indexerRunTimelineDedupeKey\s*\(`),
	}
	for _, pat := range badPat {
		if loc := pat.FindIndex(body); loc != nil {
			t.Fatalf("serviceFeed.js must use ctx for indexer cross-module API at byte %d", loc[0])
		}
	}
}

func TestFeedLogIndexerPhase4_workspaceAndRunExports(t *testing.T) {
	t.Helper()
	wsBody, err := os.ReadFile(cardsUIPath(t, "indexerWorkspace.js"))
	if err != nil {
		t.Fatal(err)
	}
	for _, sym := range []string{
		"ctx.operatorWorkspacePaths = operatorWorkspacePaths",
		"ctx.pathsSetEqualForIndexerRoots = pathsSetEqualForIndexerRoots",
		"ctx.hydrateIndexerServiceSummaryFromApi = hydrateIndexerServiceSummaryFromApi",
	} {
		if !bytes.Contains(wsBody, []byte(sym)) {
			t.Fatalf("indexerWorkspace.js must export %s", sym)
		}
	}
	runBody, err := os.ReadFile(cardsUIPath(t, "indexerRun.js"))
	if err != nil {
		t.Fatal(err)
	}
	for _, sym := range []string{
		"ctx.collectIndexerRunMeta = collectIndexerRunMeta",
		"ctx.workspaceCardTitleFromIndexerMeta = workspaceCardTitleFromIndexerMeta",
	} {
		if !bytes.Contains(runBody, []byte(sym)) {
			t.Fatalf("indexerRun.js must export %s", sym)
		}
	}
	if bytes.Contains(runBody, []byte("indexerFeedHelpers")) {
		t.Fatal("indexerRun.js must not reference deleted indexerFeedHelpers bridge")
	}
}

func TestFeedLogService_phase3ServiceOnlyModule(t *testing.T) {
	t.Helper()
	path := cardsUIPath(t, "serviceFeed.js")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	forbidden := []string{
		"collectIndexerRunMeta",
		"buildIndexerEvlogWorkspaceLabelMap",
		"dedupeOperatorWorkspacesNested",
		"hydrateIndexerServiceSummaryFromApi",
	}
	for _, sym := range forbidden {
		if bytes.Contains(body, []byte("function "+sym)) {
			t.Fatalf("serviceFeed.js must not define %s (belongs in indexerRun/indexerWorkspace)", sym)
		}
	}
	if !bytes.Contains(body, []byte("function buildServiceCard")) {
		t.Fatal("serviceFeed.js must define buildServiceCard")
	}
	n := bytes.Count(body, []byte("\n")) + 1
	if n > 1100 {
		t.Fatalf("serviceFeed.js still too large after phase 3 split: %d lines (target ~650 after broker trim in follow-up)", n)
	}
}

func TestFeedCardShared_singleSliceRecentDefinition(t *testing.T) {
	t.Helper()
	settingsRoot := filepath.Join(embeduiRoot(t), "settings", "render")
	var hits []string
	err := filepath.Walk(settingsRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || filepath.Ext(path) != ".js" {
			return err
		}
		body, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if bytes.Contains(body, []byte("function sliceRecent")) {
			hits = append(hits, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 1 {
		t.Fatalf("expected exactly one function sliceRecent under settings/render, got %d: %v", len(hits), hits)
	}
	if !bytes.Contains([]byte(hits[0]), []byte("cardChrome.js")) {
		t.Fatalf("sliceRecent should live in cardChrome.js, found in %s", hits[0])
	}
}

func TestFeedLogService_phase1DeadCodeRemoved(t *testing.T) {
	t.Helper()
	path := cardsUIPath(t, "serviceFeed.js")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	removed := []string{
		"indexerEventMixHistogramHtml",
		"indexerHistogramLegendHtml",
		"latestIndexerQueueSnapshotMetaFromEntries",
		"ctx.rollupGatewayRagPipeline",
		"ctx.vectorstoreHttpPathRollup",
		"sumEvlogBuildTbodyFromConvEvents",
		"SHOW_CONV_EXPANDED_CONTEXT_STRIP",
	}
	for _, sym := range removed {
		if bytes.Contains(body, []byte(sym)) {
			t.Fatalf("serviceFeed.js still contains removed symbol %q", sym)
		}
	}
}

func TestFeedLogService_serviceLabelDelegatesToCtx(t *testing.T) {
	t.Helper()
	path := cardsUIPath(t, "serviceFeed.js")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(body, []byte("function serviceDisplayLabel(key)")) {
		t.Fatal("serviceFeed.js must call ctx.serviceDisplayLabel directly, not wrap serviceDisplayLabel")
	}
	if bytes.Contains(body, []byte("function inferServiceBadge(ev)")) {
		t.Fatal("serviceFeed.js must call ctx.inferServiceBadge directly, not wrap inferServiceBadge")
	}
}

func TestSummarizedFeed_noBareConvAggregateCalls(t *testing.T) {
	t.Helper()
	path := settingsUIPath(t, "app", "summarizedFeed.js")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	badPat := []*regexp.Regexp{
		regexp.MustCompile(`[^.]tryRegisterRequestConversationCorrelationPrimary\s*\(`),
		regexp.MustCompile(`[^.]tryRegisterRequestConversationCorrelationRagFallback\s*\(`),
		regexp.MustCompile(`[^.]pushConversationGroupedEvent\s*\(`),
		regexp.MustCompile(`[^.]conversationRequestIdTier2EligibleLocal\s*\(`),
		regexp.MustCompile(`[^.]conversationIndexRunTier3EligibleLocal\s*\(`),
		regexp.MustCompile(`[^.]entryIsVectorstoreSubprocessForConvJoin\s*\(`),
	}
	for _, pat := range badPat {
		if loc := pat.FindIndex(body); loc != nil {
			t.Fatalf("summarizedFeed.js must use ctx for conv aggregate API at byte %d", loc[0])
		}
	}
}

func TestSummarizedFeed_noBareIndexerSummaryCalls(t *testing.T) {
	t.Helper()
	path := settingsUIPath(t, "app", "summarizedFeed.js")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	badPat := []*regexp.Regexp{
		regexp.MustCompile(`[^.]syncIndexerServiceSummaryDom\s*\(`),
		regexp.MustCompile(`[^.]scheduleIndexerServiceSummaryFetch\s*\(`),
		regexp.MustCompile(`[^.]hydrateIndexerServiceSummaryFromApi\s*\(`),
	}
	for _, pat := range badPat {
		if loc := pat.FindIndex(body); loc != nil {
			t.Fatalf("summarizedFeed.js must use ctx for indexer summary API at byte %d", loc[0])
		}
	}
}

func TestAdminUsers_noStaleAvatarCapture(t *testing.T) {
	t.Helper()
	path := cardsUIPath(t, "adminUsers.js")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if pat := regexp.MustCompile(`var\s+avatarInitials\s*=\s*ctx\.avatarInitials`); pat.Find(body) != nil {
		t.Fatal("adminUsers.js must resolve avatarInitials from ctx at call time")
	}
}

func TestMountAll_serviceCardAvatarInitials(t *testing.T) {
	vm := goja.New()
	loadChimeraUIBase(t, vm)
	evalJS(t, vm, settingsUIPath(t, "util", "hash.js"))
	evalJS(t, vm, cardsUIPath(t, "serviceCard.js"))
	evalJS(t, vm, cardsUIPath(t, "mount.js"))
	_, err := vm.RunString(`
		var ctx = { strHash: ChimeraSettings.strHash, escapeHtml: function (s) { return String(s); } };
		ChimeraSettings.Render.Cards.mountAll(ctx);
		if (typeof ctx.avatarInitials !== "function") throw new Error("avatarInitials missing after mountAll");
		if (ctx.avatarInitials("Alice Bob") !== "AB") throw new Error("avatarInitials unexpected: " + ctx.avatarInitials("Alice Bob"));
		if (typeof ctx.serviceDisplayLabel !== "function") throw new Error("serviceDisplayLabel missing after mountAll");
		if (ctx.serviceDisplayLabel("chimera-broker") !== "broker") throw new Error("serviceDisplayLabel unexpected");
		if (typeof ctx.serviceAvatarInitials !== "function") throw new Error("serviceAvatarInitials missing");
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestGatewayUsage_noStaleBrokerLabelCapture(t *testing.T) {
	t.Helper()
	path := cardsUIPath(t, "gatewayUsage.js")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if pat := regexp.MustCompile(`var\s+chimeraBrokerShortModelLabel\s*=\s*ctx\.chimeraBrokerShortModelLabel`); pat.Find(body) != nil {
		t.Fatal("gatewayUsage.js must resolve chimeraBrokerShortModelLabel from ctx at call time, not capture at mount")
	}
}

func TestMountAll_gatewayUsageCardHtml(t *testing.T) {
	vm := goja.New()
	loadChimeraUIBase(t, vm)
	evalJS(t, vm, settingsUIPath(t, "util", "escape.js"))
	evalJS(t, vm, settingsUIPath(t, "util", "hash.js"))
	evalJS(t, vm, settingsUIPath(t, "derive", "chimeraBrokerMetrics.js"))
	evalJS(t, vm, settingsUIPath(t, "derive", "gatewayUsageMetrics.js"))
	for _, f := range []string{
		"sharedFormat.js", "serviceCard.js", "serviceFeed.js", "gatewayUsage.js", "mount.js",
	} {
		evalJS(t, vm, cardsUIPath(t, f))
	}
	evalJS(t, vm, settingsUIPath(t, "render", "cardChrome.js"))
	_, err := vm.RunString(`
		var ctx = {
			escapeHtml: ChimeraSettings.escapeHtml,
			strHash: ChimeraSettings.strHash,
			formatInt: function (n) { return String(n); },
			formatCompactTok: function (n) { return String(n); },
			formatUtcToMinute: function (s) { return s; },
			formatUtcToDay: function (s) { return s; },
			aggregateRollupRows: function () { return { models: 0, tokens: 0 }; },
			metricsRollupTableHtml: function () { return ""; },
			metricsEventsTableHtml: function () { return ""; },
			metricsCache: { metrics_store_open: true, rows: [], message: "" }
		};
		ChimeraSettings.Render.Cards.mountAll(ctx);
		if (typeof ctx.chimeraBrokerShortModelLabel === "function") {
			throw new Error("mountAll must not mount serviceFeed");
		}
		ChimeraSettings.Render.Cards.mountSummarizedFeedCards(ctx);
		if (typeof ctx.chimeraBrokerShortModelLabel !== "function") {
			throw new Error("mountSummarizedFeedCards: chimeraBrokerShortModelLabel missing");
		}
		if (typeof ctx.buildGatewayUsageCardHtml !== "function") {
			throw new Error("buildGatewayUsageCardHtml missing");
		}
		var html = ctx.buildGatewayUsageCardHtml();
		if (!html || html.indexOf("gw-usage-metrics") < 0) {
			throw new Error("buildGatewayUsageCardHtml returned unexpected html");
		}
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestMount_summarizedFeedCards_order(t *testing.T) {
	t.Helper()
	body, err := os.ReadFile(cardsUIPath(t, "mount.js"))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"mountCardChrome",
		"mountFeedLogIndexerRun",
		"mountFeedLogIndexerWorkspace",
		"mountFeedLogConv",
		"mountServiceFeed",
	}
	start := bytes.Index(body, []byte("mountSummarizedFeedCards"))
	if start < 0 {
		t.Fatal("mount.js must define mountSummarizedFeedCards")
	}
	chunk := body[start:]
	last := -1
	for _, sym := range want {
		pos := bytes.Index(chunk, []byte(sym))
		if pos < 0 {
			t.Fatalf("mountSummarizedFeedCards missing %s", sym)
		}
		if pos <= last {
			t.Fatalf("mountSummarizedFeedCards order: %s should follow prior mounts", sym)
		}
		last = pos
	}
	allStart := bytes.Index(body, []byte("mountAll = function"))
	allEnd := bytes.Index(body[allStart:], []byte("mountSummarizedFeedCards"))
	if allStart >= 0 && allEnd > 0 {
		allBody := body[allStart : allStart+allEnd]
		if bytes.Contains(allBody, []byte("mountServiceFeed")) {
			t.Fatal("mountAll must not call mountServiceFeed")
		}
		if bytes.Contains(allBody, []byte("mountCardChrome")) {
			t.Fatal("mountAll must not call mountCardChrome")
		}
	}
}

func TestSummarizedFeed_mountSummarizedFeedCardsOnly(t *testing.T) {
	t.Helper()
	body, err := os.ReadFile(settingsUIPath(t, "app", "summarizedFeed.js"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(body, []byte("mountSummarizedFeedCards(ctx)")) {
		t.Fatal("summarizedFeed.js must call mountSummarizedFeedCards(ctx)")
	}
	for _, sym := range []string{"mountFeedCardShared", "mountCardChrome", "mountFeedLogService", "mountServiceFeed"} {
		if bytes.Contains(body, []byte(sym+"(ctx)")) {
			t.Fatalf("summarizedFeed.js must not call %s directly; use mountSummarizedFeedCards", sym)
		}
	}
}

func TestServiceFeed_requiredExternalCtxConsumers(t *testing.T) {
	t.Helper()
	settingsRoot := filepath.Join(embeduiRoot(t), "settings")
	mustConsumeOutside := []string{
		"buildServiceCard",
		"chimeraBrokerShortModelLabel",
	}
	searchRoots := []string{settingsRoot, filepath.Join(embeduiRoot(t), "..", "embedui_test")}
	for _, sym := range mustConsumeOutside {
		needle := []byte("ctx." + sym + "(")
		found := false
		for _, root := range searchRoots {
			_ = filepath.Walk(root, func(p string, info os.FileInfo, walkErr error) error {
				if walkErr != nil || info.IsDir() || filepath.Base(p) == "serviceFeed.js" {
					return walkErr
				}
				if filepath.Ext(p) != ".js" && filepath.Ext(p) != ".go" {
					return nil
				}
				b, readErr := os.ReadFile(p)
				if readErr != nil {
					return readErr
				}
				if bytes.Contains(b, needle) {
					found = true
					return filepath.SkipAll
				}
				return nil
			})
			if found {
				break
			}
		}
		if !found {
			t.Fatalf("expected ctx.%s( consumer outside serviceFeed.js under settings/", sym)
		}
	}
}
