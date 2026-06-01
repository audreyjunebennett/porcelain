package embedui_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/dop251/goja"
)

// embeduiRoot returns the on-disk embedui/ directory (sibling of this test package).
func testPkgDir(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Dir(thisFile)
}

func embeduiRoot(t *testing.T) string {
	t.Helper()
	return filepath.Join(testPkgDir(t), "..", "embedui")
}

// serverTestdataPath resolves fixtures under internal/server/testdata/.
func serverTestdataPath(t *testing.T, rel ...string) string {
	t.Helper()
	base := filepath.Join(testPkgDir(t), "..", "..", "..", "testdata")
	return filepath.Join(append([]string{base}, rel...)...)
}

// settingsUIPath resolves modules under embedui/settings/ (operator settings + log feed UI).
func settingsUIPath(t *testing.T, rel ...string) string {
	t.Helper()
	base := filepath.Join(embeduiRoot(t), "settings")
	return filepath.Join(append([]string{base}, rel...)...)
}

func uiEmbedPath(t *testing.T, rel ...string) string {
	t.Helper()
	base := filepath.Join(embeduiRoot(t), "ui")
	return filepath.Join(append([]string{base}, rel...)...)
}

func cardsUIPath(t *testing.T, rel ...string) string {
	t.Helper()
	return settingsUIPath(t, append([]string{"render", "cards"}, rel...)...)
}

func sharedUIPath(t *testing.T, rel ...string) string {
	t.Helper()
	base := filepath.Join(embeduiRoot(t), "shared")
	return filepath.Join(append([]string{base}, rel...)...)
}

func mustReadFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

func evalJS(t *testing.T, vm *goja.Runtime, path string) {
	t.Helper()
	_, err := vm.RunString(mustReadFile(t, path))
	if err != nil {
		t.Fatalf("eval %s: %v", path, err)
	}
}

func getFn(t *testing.T, vm *goja.Runtime, name string) goja.Callable {
	t.Helper()
	obj := vm.Get("ChimeraSettings").ToObject(vm)
	fn, ok := goja.AssertFunction(obj.Get(name))
	if !ok {
		t.Fatalf("missing ChimeraSettings.%s", name)
	}
	return fn
}

func loadChimeraUIBase(t *testing.T, vm *goja.Runtime) {
	t.Helper()
	evalJS(t, vm, settingsUIPath(t, "testing", "loader.js"))
	evalJS(t, vm, uiEmbedPath(t, "util", "escape.js"))
}

func loadCardTestCtx(t *testing.T, vm *goja.Runtime) {
	t.Helper()
	evalJS(t, vm, settingsUIPath(t, "testing", "loader.js"))
	evalJS(t, vm, uiEmbedPath(t, "util", "escape.js"))
	evalJS(t, vm, settingsUIPath(t, "util", "escape.js"))
	evalJS(t, vm, settingsUIPath(t, "util", "hash.js"))
	evalJS(t, vm, settingsUIPath(t, "util", "time.js"))
	evalJS(t, vm, settingsUIPath(t, "derive", "chimeraBrokerMetrics.js"))
	evalJS(t, vm, settingsUIPath(t, "render", "sumEvlog.js"))
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
		"feedLogConv.js", "feedLogService.js", "indexerRun.js", "indexerWorkspace.js", "mount.js",
	} {
		evalJS(t, vm, cardsUIPath(t, f))
	}
	evalJS(t, vm, settingsUIPath(t, "summarized", "rebuildPolicy.js"))

	_, err := vm.RunString(`
		var ctx = {
			escapeHtml: ChimeraSettings.escapeHtml,
			getFlat: function (p) { return (p && p.rawFlat) || {}; },
			entryCache: [],
			strHash: ChimeraSettings.strHash,
			entryInstant: function () { return null; },
			humanDurationMs: ChimeraSettings.humanDurationMs,
			logSummaryHtml: function () { return ""; },
			tbody: null,
			sumEvlogRowTrHtml: function () { return ""; },
			sumEvlogPanelHtml: function (o) { return o.title || ""; },
			inferServiceBadge: function () { return "svc"; },
			avatarInitials: function (label) {
				var s = String(label || "?").trim();
				return s.slice(0, 2).toUpperCase() || "??";
			},
			avatarHueClass: function () { return "sum-av-a"; },
			chimeraBrokerShortModelLabel: function (id) { return String(id || "—"); },
			metricsCache: null,
			gatewayOverviewCache: {
				semver: "9.9.9-test",
				virtual_model_id: "virtual/test",
				service_overview: { refreshed_at: "2026-01-01T12:00:00Z", services: [] }
			},
			tokenListCache: [{ tenant_id: "tenant-a", label: "Alice", index: 0 }],
			adminUserDrafts: [],
			virtualModelDrafts: [],
			nextVirtualModelDraftId: 1,
			adminProviderKeyDraft: {},
			adminVisibleProviderIds: ["groq", "ollama"],
			adminOllamaUrlDraft: null,
			adminProviderModelsEditingId: null,
			adminProviderModelsDraft: {},
			adminProviderModelsCache: {},
			adminStateCache: {
				providers: {
					groq: { keys: [], ok: true },
					ollama: { keys: [], ok: true, ollama_base_url: "http://127.0.0.1:11434" }
				}
			},
			tokenLabelByTenant: { "tenant-a": "Alice" },
			adminCreatedTokenByTenant: {}
		};
		ChimeraSettings.Render.mountSumEvlog(ctx);
		ChimeraSettings.Render.Cards.mountAll(ctx);
		var C = ChimeraSettings.Render.Cards;
		if (typeof C.mountFeedLogConv === "function") C.mountFeedLogConv(ctx);
		if (typeof C.mountFeedLogService === "function") C.mountFeedLogService(ctx);
		if (typeof C.mountFeedLogIndexerRun === "function") C.mountFeedLogIndexerRun(ctx);
		if (typeof C.mountFeedLogIndexerWorkspace === "function") C.mountFeedLogIndexerWorkspace(ctx);
		if (typeof ctx.pickFolderForWorkspaceDraft !== "function") throw new Error("pickFolderForWorkspaceDraft missing on ctx");
		if (typeof ctx.findWorkspaceDraft !== "function") throw new Error("findWorkspaceDraft missing on ctx");
		globalThis.__cardTestCtx = ctx;
	`)
	if err != nil {
		t.Fatalf("mount card ctx: %v", err)
	}
}
