package embedui_test

import (
	"testing"

	"github.com/dop251/goja"
)

// loadFeedSmokeStack mounts cards + summarized render modules the way settings.html does
// (without summarizedFeed.js — smoke tests call ctx builders and Summarized.Render directly).
func loadFeedSmokeStack(t *testing.T, vm *goja.Runtime) {
	t.Helper()
	loadChimeraUIBase(t, vm)
	evalJS(t, vm, settingsUIPath(t, "util", "escape.js"))
	evalJS(t, vm, settingsUIPath(t, "util", "hash.js"))
	evalJS(t, vm, settingsUIPath(t, "util", "time.js"))
	evalJS(t, vm, settingsUIPath(t, "derive", "chimeraBrokerMetrics.js"))
	evalJS(t, vm, settingsUIPath(t, "derive", "gatewayUsageMetrics.js"))
	evalJS(t, vm, settingsUIPath(t, "derive", "gatewayCardModel.js"))
	evalJS(t, vm, settingsUIPath(t, "derive", "vectorstoreRagMetrics.js"))
	evalJS(t, vm, settingsUIPath(t, "derive", "conversationCardModel.js"))
	evalJS(t, vm, settingsUIPath(t, "derive", "logLineClassification.js"))
	evalJS(t, vm, settingsUIPath(t, "derive", "conversationAggregate.js"))
	evalJS(t, vm, settingsUIPath(t, "render", "sumEvlog.js"))
	evalJS(t, vm, settingsUIPath(t, "render", "cardChrome.js"))
	for _, f := range []string{
		"operatorFeedback.js", "configureEdit.js", "yamlEditor.js", "draftInput.js",
		"providerCredentials.js", "scopedEvlog.js", "adminAction.js", "editToolbar.js",
		"workspacePaths.js", "serviceHealth.js",
	} {
		evalJS(t, vm, sharedUIPath(t, f))
	}
	for _, f := range []string{
		"sharedFormat.js", "convCard.js", "serviceCard.js", "gatewayOverview.js", "gatewayUsage.js",
		"adminShared.js", "adminUsers.js", "adminProvider.js", "adminVirtualModels.js", "workspaceDraft.js",
		"feedLogConv.js",
	} {
		evalJS(t, vm, cardsUIPath(t, f))
	}
	evalServiceFeedModules(t, vm)
	for _, f := range []string{"indexerRun.js", "indexerWorkspace.js", "mount.js"} {
		evalJS(t, vm, cardsUIPath(t, f))
	}
	for _, f := range []string{"hash.js", "model.js", "aggregate.js", "renderHtml.js"} {
		evalJS(t, vm, settingsUIPath(t, "summarized", f))
	}
	evalJS(t, vm, settingsUIPath(t, "summarized", "rebuildPolicy.js"))

	_, err := vm.RunString(`
		var ctx = {
			escapeHtml: ChimeraSettings.escapeHtml,
			getFlat: function (p) { return (p && p.rawFlat) || {}; },
			entryCache: [],
			strHash: ChimeraSettings.strHash,
			entryInstant: function (e) {
				if (!e || e.ts == null || e.ts === "") return null;
				var d = e.ts instanceof Date ? e.ts : new Date(e.ts);
				return isNaN(d.getTime()) ? null : d;
			},
			humanDurationMs: ChimeraSettings.humanDurationMs,
			logSummaryHtml: function () { return ""; },
			tbody: null,
			sumEvlogRowTrHtml: function () { return "<tr></tr>"; },
			sumEvlogPanelHtml: function (o) { return (o && o.title) || ""; },
			sumEvlogBuildTbodyFromConvEvents: function () { return ""; },
			sumEvlogBuildTbodyFromServiceEntries: function () { return ""; },
			sumEvlogVisibleEntriesForService: function (_n, arr) { return arr || []; },
			sumEvlogCountWarnFailFromEntries: function () { return { warn: 0, fail: 0 }; },
			scopedEvlogTitle: function (t) { return String(t || ""); },
			contextGrowthStripHtml: function () { return ""; },
			RECENT_CARD_STATUS_N: 12,
			formatInt: function (n) { return String(n); },
			formatCompactTok: function (n) { return String(n); },
			formatUtcToMinute: function (s) { return s; },
			formatUtcToDay: function (s) { return s; },
			aggregateRollupRows: function () { return { models: 0, tokens: 0 }; },
			metricsRollupTableHtml: function () { return ""; },
			metricsEventsTableHtml: function () { return ""; },
			metricsCache: { metrics_store_open: true, rows: [], message: "" },
			gatewayOverviewCache: {
				virtual_model_id: "virtual/test",
				service_overview: { refreshed_at: "2026-01-01T12:00:00Z", services: [] }
			},
			tokenListCache: [],
			adminUserDrafts: [],
			virtualModelDrafts: [],
			adminProviderKeyDraft: {},
			adminVisibleProviderIds: [],
			adminOllamaUrlDraft: null,
			adminProviderModelsEditingId: null,
			adminProviderModelsDraft: {},
			adminProviderModelsCache: {},
			adminStateCache: { providers: {}, gateway: { virtual_models: [] } },
			tokenLabelByTenant: {},
			workspaceDrafts: [],
			lastIndexerOperatorWorkspacesNested: [],
			summarizedReqToConv: {},
			summarizedIndexRunToConv: {}
		};
		ChimeraSettings.Render.mountSumEvlog(ctx);
		ChimeraSettings.Render.Cards.mountAll(ctx);
		var C = ChimeraSettings.Render.Cards;
		if (typeof C.mountSummarizedFeedCards === "function") C.mountSummarizedFeedCards(ctx);
		globalThis.__feedSmokeCtx = ctx;
	`)
	if err != nil {
		t.Fatalf("feed smoke mount: %v", err)
	}
}

func TestFeedSmoke_buildServiceCard_chimeraBroker(t *testing.T) {
	vm := goja.New()
	loadFeedSmokeStack(t, vm)
	_, err := vm.RunString(`
		var ctx = globalThis.__feedSmokeCtx;
		if (typeof ctx.sgOpInsetWellOkFailHtml !== "function") {
			throw new Error("sgOpInsetWellOkFailHtml missing on ctx");
		}
		if (typeof ctx.entryIsGatewayUpstreamRelay !== "function") {
			throw new Error("entryIsGatewayUpstreamRelay missing on ctx");
		}
		if (typeof ctx.buildServiceCard !== "function") {
			throw new Error("buildServiceCard missing on ctx");
		}
		var relayEnt = {
			parsed: { shape: "chat.chimera-broker.request", rawFlat: { msg: "chat.chimera-broker.request" } },
			text: "",
			ts: "2026-01-01T12:00:00Z",
			source: "chimera-gateway"
		};
		if (!ctx.entryIsGatewayUpstreamRelay(relayEnt)) {
			throw new Error("entryIsGatewayUpstreamRelay should match broker relay lines");
		}
		var html = ctx.buildServiceCard("chimera-broker", [], { byRun: {}, partitionRegistry: {} });
		if (!html || String(html).indexOf("chimera-broker") < 0 && String(html).indexOf("broker") < 0) {
			throw new Error("buildServiceCard returned empty or unexpected html");
		}
		if (String(html).indexOf("<details") < 0) {
			throw new Error("buildServiceCard should return a details.sum-card");
		}
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestFeedSmoke_buildServiceCard_allCoreServices(t *testing.T) {
	vm := goja.New()
	loadFeedSmokeStack(t, vm)
	services := []string{"chimera-gateway", "chimera-broker", "chimera-indexer", "chimera-vectorstore"}
	for _, svc := range services {
		svc := svc
		t.Run(svc, func(t *testing.T) {
			_, err := vm.RunString(`
				var html = globalThis.__feedSmokeCtx.buildServiceCard("` + svc + `", [], { byRun: {}, partitionRegistry: {} });
				if (!html || String(html).length < 20) {
					throw new Error("buildServiceCard failed for ` + svc + `");
				}
			`)
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestFeedSmoke_renderSummarizedHtml_sections(t *testing.T) {
	vm := goja.New()
	loadFeedSmokeStack(t, vm)
	_, err := vm.RunString(`
		var ctx = globalThis.__feedSmokeCtx;
		var model = {
			cards: [
				{
					id: "gw-overview",
					kind: "gateway-overview",
					section: "overview",
					sortKey: "00",
					hash: "h1",
					summary: {},
					body: {},
					source: { cache: ctx.gatewayOverviewCache }
				},
				{
					id: "svc-broker",
					kind: "service",
					section: "services",
					sortKey: "chimera-broker",
					hash: "h2",
					summary: { service: "chimera-broker" },
					body: {},
					source: {
						name: "chimera-broker",
						events: [],
						svcCtx: { byRun: {}, partitionRegistry: {} }
					}
				}
			],
			meta: { hasThreads: true, hasWorkspaces: false, hasServices: true }
		};
		var html = ChimeraSettings.Summarized.Render.renderSummarizedHtml(model, {
			renderCard: function (card) {
				if (card.kind === "gateway-overview") return ctx.buildGatewayOverviewCardHtml();
				if (card.kind === "service") {
					return ctx.buildServiceCard(card.source.name, card.source.events, card.source.svcCtx);
				}
				return "";
			},
			workspacesSectionHead: function () {
				return '<div class="sum-feed-section-head"><span class="sum-feed-section-title">Workspaces</span></div>';
			},
			servicesSectionHead: function () {
				return '<div class="sum-feed-section-head"><span class="sum-feed-section-title">Core services</span></div>';
			},
			workspacesSectionIntro: function () { return ""; },
			buildWorkspacesCreateBtnHtml: function () { return ""; }
		});
		if (String(html).indexOf("sum-feed-section--workspaces") < 0) {
			throw new Error("missing workspaces section wrapper");
		}
		if (String(html).indexOf("Workspaces") < 0) {
			throw new Error("missing Workspaces section head");
		}
		if (String(html).indexOf("Core services") < 0) {
			throw new Error("missing Core services section head");
		}
		if (String(html).indexOf("svc-") < 0 && String(html).indexOf("chimera-broker") < 0) {
			throw new Error("missing service card html in feed");
		}
	`)
	if err != nil {
		t.Fatal(err)
	}
}
