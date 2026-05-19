package embedui_test

import (
	"strings"
	"testing"

	"github.com/dop251/goja"
)

func loadSummarizedModelCtx(t *testing.T, vm *goja.Runtime) {
	t.Helper()
	evalJS(t, vm, logsUIPath(t, "testing", "loader.js"))
	evalJS(t, vm, logsUIPath(t, "util", "hash.js"))
	evalJS(t, vm, logsUIPath(t, "summarized", "hash.js"))
	evalJS(t, vm, logsUIPath(t, "summarized", "model.js"))
	evalJS(t, vm, logsUIPath(t, "summarized", "renderHtml.js"))
}

func TestSummarizedModel_buildsConversationServiceGatewayCards(t *testing.T) {
	vm := goja.New()
	loadSummarizedModelCtx(t, vm)

	_, err := vm.RunString(`
		var deps = {
			strHash: ChimeraLogs.strHash,
			conversationDomIdForGroup: function (g) {
				return ChimeraLogs.strHash(g.pid + "\0" + g.cid);
			},
			convLastTs: function () { return 1000; },
			primaryLogMessage: function () { return "hello"; },
			conversationCardModelForGroup: function () { return { progress: [] }; },
			conversationCardStatus: function () { return { st: "active" }; },
			indexerPartitionMetaForRun: function () { return null; },
			collectIndexerRunMeta: function () { return {}; },
			mergePersistedIndexerWatchRoots: function (m) { return m; },
			indexerRunTimelineDedupeKey: function (_m, id) { return id; },
			pickCanonicalIndexerRun: function (r) { return r[0]; },
			workspaceCardTitleFromIndexerMeta: function () { return "ws"; },
			indexerCardTitleSortLabel: function () { return "a"; },
			indexerCardDomIdFromMeta: function (_m, id) { return "ix-" + ChimeraLogs.strHash(id); },
			indexerCardIdentityKey: function () { return "k"; },
			indexerCardIdentityKeyFromSnap: function () { return "k"; },
			loadIndexerWatchRootsStore: function () { return { snapshots: {} }; },
			dedupeOperatorWorkspacesNested: function (x) { return x; },
			canonicalWorkspaceRowIdKey: function (id) { return String(id); },
			workspaceDraftComparableManagedTitle: function () { return ""; },
			operatorManagedWorkspaceTitleText: function () { return ""; },
			operatorWorkspaceCoveredByIndexerRuns: function () { return false; },
			adminProvidersSectionBreakHtml: function () { return ""; },
			adminRoutingSectionBreakHtml: function () { return ""; }
		};
		var state = {
			agg: {
				mergedConv: [{
					pid: "tenant-a",
					cid: "conv-1",
					events: [{ seq: 1, parsed: { rawFlat: { msg: "chat.request" } }, text: "x" }]
				}],
				buckets: {
					"chimera-gateway": [{ seq: 2, parsed: { rawFlat: { msg: "gateway.http.access" } }, text: "y" }]
				},
				byRun: {},
				partitionRegistry: {}
			},
			gatewayOverviewCache: { semver: "1.0.0", virtual_model_id: "v/test", service_overview: { services: [] } },
			metricsCache: { metrics_store_open: true, rows: [] },
			adminStateCache: { providers: { groq: { keys: [], ok: true } }, gateway: {} },
			tokenListCache: [],
			workspaceDrafts: [],
			adminProviderSpecs: [{ id: "groq", title: "Groq", avatar: "Gq", subtitle: "sub" }],
			lastIndexerOperatorWorkspacesNested: []
		};
		var model = ChimeraLogs.Summarized.Model.buildSummarizedModel(deps, state);
	`)
	if err != nil {
		t.Fatal(err)
	}

	v, err := vm.RunString(`
		(function () {
			var ids = model.cards.map(function (c) { return c.id; });
			return {
				hasGw: ids.indexOf("gw-overview") >= 0,
				hasGroq: ids.indexOf("admin-provider-groq") >= 0,
				convKind: model.cards.filter(function (c) { return c.kind === "conversation"; }).length,
				svcKind: model.cards.filter(function (c) { return c.kind === "service"; }).length,
				convHash: model.cards.filter(function (c) { return c.kind === "conversation"; })[0].hash
			};
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}
	obj := v.ToObject(vm)
	if !obj.Get("hasGw").ToBoolean() {
		t.Fatal("missing gw-overview card")
	}
	if !obj.Get("hasGroq").ToBoolean() {
		t.Fatal("missing admin-provider-groq card")
	}
	if obj.Get("convKind").ToInteger() != 1 {
		t.Fatalf("conversation cards=%v", obj.Get("convKind").Export())
	}
	if obj.Get("svcKind").ToInteger() != 1 {
		t.Fatalf("service cards=%v", obj.Get("svcKind").Export())
	}
	if obj.Get("convHash").Export() == "" {
		t.Fatal("conversation card missing hash")
	}
}

func TestSummarizedModel_hashChangesWhenEventAppended(t *testing.T) {
	vm := goja.New()
	loadSummarizedModelCtx(t, vm)

	_, err := vm.RunString(`
		var deps = {
			strHash: ChimeraLogs.strHash,
			conversationDomIdForGroup: function (g) { return ChimeraLogs.strHash(g.pid + "\0" + g.cid); },
			convLastTs: function () { return 1; },
			primaryLogMessage: function () { return "m"; },
			conversationCardModelForGroup: function () { return {}; },
			conversationCardStatus: function () { return { st: "active" }; },
			indexerPartitionMetaForRun: function () { return null; },
			collectIndexerRunMeta: function () { return {}; },
			mergePersistedIndexerWatchRoots: function (m) { return m; },
			indexerRunTimelineDedupeKey: function (_m, id) { return id; },
			pickCanonicalIndexerRun: function (r) { return r[0]; },
			workspaceCardTitleFromIndexerMeta: function () { return ""; },
			indexerCardTitleSortLabel: function () { return ""; },
			indexerCardDomIdFromMeta: function (_m, id) { return "ix-" + id; },
			indexerCardIdentityKey: function () { return ""; },
			indexerCardIdentityKeyFromSnap: function () { return ""; },
			loadIndexerWatchRootsStore: function () { return { snapshots: {} }; },
			dedupeOperatorWorkspacesNested: function (x) { return x; },
			canonicalWorkspaceRowIdKey: function (id) { return String(id); },
			workspaceDraftComparableManagedTitle: function () { return ""; },
			operatorManagedWorkspaceTitleText: function () { return ""; },
			operatorWorkspaceCoveredByIndexerRuns: function () { return false; },
			adminProvidersSectionBreakHtml: function () { return ""; },
			adminRoutingSectionBreakHtml: function () { return ""; }
		};
		var baseEvents = [{ seq: 1, parsed: { rawFlat: {} }, text: "a" }];
		var state1 = {
			agg: { mergedConv: [{ pid: "p", cid: "c", events: baseEvents }], buckets: {}, byRun: {}, partitionRegistry: {} },
			gatewayOverviewCache: {}, metricsCache: {}, adminStateCache: { providers: {}, gateway: {} },
			tokenListCache: [], workspaceDrafts: [], adminProviderSpecs: [], lastIndexerOperatorWorkspacesNested: []
		};
		var m1 = ChimeraLogs.Summarized.Model.buildSummarizedModel(deps, state1);
		var h1 = m1.cards.filter(function (c) { return c.kind === "conversation"; })[0].hash;
		baseEvents.push({ seq: 2, parsed: { rawFlat: {} }, text: "b" });
		var m2 = ChimeraLogs.Summarized.Model.buildSummarizedModel(deps, state1);
		var h2 = m2.cards.filter(function (c) { return c.kind === "conversation"; })[0].hash;
	`)
	if err != nil {
		t.Fatal(err)
	}

	v, err := vm.RunString(`h1 !== h2`)
	if err != nil {
		t.Fatal(err)
	}
	if !v.ToBoolean() {
		t.Fatal("expected conversation card hash to change when events appended")
	}
}

func TestSummarizedRenderHtml_includesSectionWrappers(t *testing.T) {
	vm := goja.New()
	loadSummarizedModelCtx(t, vm)

	_, err := vm.RunString(`
		var model = {
			cards: [
				{ id: "gw-overview", kind: "gateway-overview", section: "overview", sortKey: "a", hash: "h", summary: {}, body: {}, source: {} }
			],
			meta: { hasThreads: true }
		};
		var html = ChimeraLogs.Summarized.Render.renderSummarizedHtml(model, {
			renderCard: function () { return '<details id="gw-overview"></details>'; },
			workspacesSectionIntro: function () { return '<p class="intro">i</p>'; }
		});
	`)
	if err != nil {
		t.Fatal(err)
	}
	out := vm.Get("html").String()
	if !strings.Contains(out, `sum-feed-section--workspaces`) {
		t.Fatalf("missing workspaces section: %q", out)
	}
	if !strings.Contains(out, `id="gw-overview"`) {
		t.Fatalf("missing card html: %q", out)
	}
}
