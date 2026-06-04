package embedui_test

import (
	"testing"

	"github.com/dop251/goja"
)

func loadIndexerScopeCtx(t *testing.T, vm *goja.Runtime) {
	t.Helper()
	evalJS(t, vm, settingsUIPath(t, "testing", "loader.js"))
	evalJS(t, vm, settingsUIPath(t, "derive", "logLineClassification.js"))
	evalJS(t, vm, settingsUIPath(t, "derive", "indexerScopeFullLog.js"))
}

func TestIndexerScope_normalizeFlavor(t *testing.T) {
	vm := goja.New()
	loadIndexerScopeCtx(t, vm)

	_, err := vm.RunString(`
		var n = ChimeraSettings.Derive.normalizeIndexerScopeFlavor;
		if (n("—") !== "") throw new Error("em dash");
		if (n("dev") !== "dev") throw new Error("dev");
		if (n("  none  ") !== "") throw new Error("none");
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestIndexerScope_filterEmptyBucketReturnsAll(t *testing.T) {
	vm := goja.New()
	loadIndexerScopeCtx(t, vm)

	_, err := vm.RunString(`
		var evs = [{ parsed: { rawFlat: { msg: "indexer.run.start" } } }];
		var out = ChimeraSettings.Derive.filterEventsForIndexerScopeFullLog(
			evs,
			"",
			{},
			evs,
			function (p) { return (p && p.rawFlat) || {}; },
			{ indexerRootScopeByRootId: {}, operatorWsFullLogCtx: {}, lastIndexerOperatorWorkspacesNested: [] },
			{
				canonicalWorkspaceRowIdKey: function (id) { return String(id); },
				operatorWorkspaceNumericId: function () { return 0; },
				operatorWorkspacePaths: function () { return []; }
			}
		);
		if (out.length !== 1) throw new Error("empty bucket id should include all lines");
	`)
	if err != nil {
		t.Fatal(err)
	}
}
