package embedui_test

import (
	"bytes"
	"encoding/json"
	"os"
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
	"conversationCardModelForGroup":  "conversationCardModelForGroup",
	"conversationCardStatus":         "conversationCardStatus",
	"collectIndexerRunMeta":            "collectIndexerRunMeta",
	"mergePersistedIndexerWatchRoots":  "mergePersistedIndexerWatchRoots",
	"operatorWorkspaceCoveredByIndexerRuns": "operatorWorkspaceCoveredByIndexerRuns",
	"operatorWorkspaceNumericId":       "operatorWorkspaceNumericId",
	"adminProvidersSectionBreakHtml": "buildAdminProvidersSectionBreakHtml",
}

func TestSummarizedModelDeps_cardRefsUseCtx(t *testing.T) {
	t.Helper()
	path := settingsUIPath(t, "app", "summarizedFeed.js")
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
	path := cardsUIPath(t, "feedLogService.js")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(body, []byte("ctx.entryIsGatewayUpstreamRelay = entryIsGatewayUpstreamRelay")) {
		t.Fatal("feedLogService.js must export entryIsGatewayUpstreamRelay on ctx")
	}
}

func TestFeedLogService_sumEvlogUsesCtx(t *testing.T) {
	t.Helper()
	path := cardsUIPath(t, "feedLogService.js")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if pat := regexp.MustCompile(`[^.]sumEvlogVisibleEntriesForService\s*\(`); pat.Find(body) != nil {
		t.Fatal("feedLogService.js must call ctx.sumEvlogVisibleEntriesForService, not bare sumEvlogVisibleEntriesForService")
	}
	if pat := regexp.MustCompile(`[^.]countWarnErrorInEntries\s*\(`); pat.Find(body) != nil {
		t.Fatal("feedLogService.js must call ctx.countWarnErrorInEntries, not bare countWarnErrorInEntries")
	}
}

func TestFeedLogService_serviceLabelDelegatesToCtx(t *testing.T) {
	t.Helper()
	path := cardsUIPath(t, "feedLogService.js")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(body, []byte("function serviceDisplayLabel(key)")) ||
		!bytes.Contains(body, []byte("ctx.serviceDisplayLabel(key)")) {
		t.Fatal("feedLogService.js must wrap serviceDisplayLabel and delegate to ctx")
	}
	if !bytes.Contains(body, []byte("function inferServiceBadge(ev)")) ||
		!bytes.Contains(body, []byte("ctx.inferServiceBadge(ev)")) {
		t.Fatal("feedLogService.js must wrap inferServiceBadge and delegate to ctx")
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
		"sharedFormat.js", "serviceCard.js", "feedLogService.js", "gatewayUsage.js", "mount.js",
	} {
		evalJS(t, vm, cardsUIPath(t, f))
	}
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
		if (typeof ctx.chimeraBrokerShortModelLabel !== "function") {
			throw new Error("mountAll: chimeraBrokerShortModelLabel missing before gateway usage build");
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
