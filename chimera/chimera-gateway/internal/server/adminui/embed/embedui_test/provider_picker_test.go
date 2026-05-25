package embedui_test

import (
	"testing"

	"github.com/dop251/goja"
)

func loadProviderCatalogCtx(t *testing.T, vm *goja.Runtime) {
	t.Helper()
	evalJS(t, vm, settingsUIPath(t, "testing", "loader.js"))
	evalJS(t, vm, settingsUIPath(t, "providers", "catalog.js"))
	_, err := vm.RunString(`
		ChimeraSettings.Providers.Catalog.installCatalogEntries([
			{ id: "groq", title: "Groq", avatar: "Gq", subtitle: "Fast" },
			{ id: "gemini", title: "Gemini", avatar: "Gm", subtitle: "Google" },
			{ id: "ollama", title: "Ollama", avatar: "Ol", subtitle: "Local" }
		]);
		var ctx = { adminVisibleProviderIds: ["ollama"], adminVisibleProviderIdsSeeded: true };
	`)
	if err != nil {
		t.Fatalf("catalog ctx: %v", err)
	}
}

func TestProviderCatalog_addVisibleProviderId(t *testing.T) {
	vm := goja.New()
	loadProviderCatalogCtx(t, vm)

	v, err := vm.RunString(`
		(function () {
			var cat = ChimeraSettings.Providers.Catalog;
			var added = cat.addVisibleProviderId(ctx, "groq");
			return {
				added: added,
				visible: ctx.adminVisibleProviderIds.slice(),
				addable: cat.addableProviderEntries(ctx).map(function (e) { return e.id; }),
				canAdd: cat.hasAddableProviders(ctx)
			};
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}
	obj := v.ToObject(vm)
	if !obj.Get("added").ToBoolean() {
		t.Fatal("expected groq to be added")
	}
	visible := obj.Get("visible").Export().([]any)
	if len(visible) != 2 {
		t.Fatalf("visible=%v", visible)
	}
	addable := obj.Get("addable").Export().([]any)
	if len(addable) != 1 || addable[0] != "gemini" {
		t.Fatalf("addable=%v", addable)
	}
	if !obj.Get("canAdd").ToBoolean() {
		t.Fatal("expected hasAddableProviders true")
	}
}
