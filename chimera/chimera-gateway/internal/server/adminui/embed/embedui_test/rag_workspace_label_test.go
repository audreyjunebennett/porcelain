package embedui_test

import (
	"testing"

	"github.com/dop251/goja"
)

func mountTestRagWorkspaceLabel(t *testing.T, vm *goja.Runtime) {
	t.Helper()
	_, err := vm.RunString(`
ChimeraSettings.Derive.mountRagWorkspaceLabel({
  getOperatorWorkspaces: function () {
    return [{ project_id: "task-orchestrator", flavor_id: "" }];
  },
  tenantUserLabel: function () { return "lynn"; },
  operatorWorkspaceTitle: function (ws) { return "lynn:" + String(ws.project_id || ""); },
  workspaceTitleFromParts: function (user, project) { return user + ":" + project; },
  normalizeFlavor: function (v) {
    if (v == null || v === "—") return "";
    return String(v).trim();
  }
});
`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestRagWorkspaceLabel_resolveKnownAndMissing(t *testing.T) {
	vm := goja.New()
	evalJS(t, vm, settingsUIPath(t, "derive", "ragWorkspaceLabel.js"))
	mountTestRagWorkspaceLabel(t, vm)

	fn, ok := goja.AssertFunction(vm.Get("ChimeraSettings").ToObject(vm).Get("Derive").ToObject(vm).Get("resolveRagWorkspaceLabel"))
	if !ok {
		t.Fatal("missing resolveRagWorkspaceLabel")
	}

	vKnown, err := fn(goja.Undefined(), vm.ToValue("tenant-1"), vm.ToValue("task-orchestrator"), vm.ToValue(""))
	if err != nil {
		t.Fatal(err)
	}
	known := vKnown.ToObject(vm)
	if known.Get("known").Export().(bool) != true {
		t.Fatalf("expected known workspace, got %v", known.Export())
	}
	if got := known.Get("label").String(); got != "lynn:task-orchestrator" {
		t.Fatalf("known label: got %q", got)
	}

	vMissing, err := fn(goja.Undefined(), vm.ToValue("tenant-1"), vm.ToValue("workspacename"), vm.ToValue(""))
	if err != nil {
		t.Fatal(err)
	}
	missing := vMissing.ToObject(vm)
	if missing.Get("known").Export().(bool) != false {
		t.Fatalf("expected unknown workspace, got %v", missing.Export())
	}
	wantMissing := "lynn:workspacename - missing or undefined"
	if got := missing.Get("label").String(); got != wantMissing {
		t.Fatalf("missing label: got %q want %q", got, wantMissing)
	}
}

func TestRagWorkspaceLabel_extractCoordsFromEvents(t *testing.T) {
	vm := goja.New()
	evalJS(t, vm, settingsUIPath(t, "derive", "ragWorkspaceLabel.js"))

	fn, ok := goja.AssertFunction(vm.Get("ChimeraSettings").ToObject(vm).Get("Derive").ToObject(vm).Get("extractRagCoordsFromEvents"))
	if !ok {
		t.Fatal("missing extractRagCoordsFromEvents")
	}
	getFlat := vm.ToValue(func(call goja.FunctionCall) goja.Value {
		p := call.Argument(0).Export()
		m, _ := p.(map[string]any)
		if m == nil {
			return vm.ToValue(map[string]any{})
		}
		if rf, ok := m["rawFlat"].(map[string]any); ok {
			return vm.ToValue(rf)
		}
		return vm.ToValue(map[string]any{})
	})
	events := []map[string]any{
		{"parsed": map[string]any{"rawFlat": map[string]any{"msg": "conversation.rag.span", "collection": "coll-x"}}},
		{"parsed": map[string]any{"rawFlat": map[string]any{
			"msg": "conversation.rag.attached", "tenant": "t1", "project": "workspacename", "collection": "coll-x",
		}}},
	}
	v, err := fn(goja.Undefined(), vm.ToValue(events), getFlat)
	if err != nil {
		t.Fatal(err)
	}
	coords := v.ToObject(vm)
	if coords.Get("projectId").String() != "workspacename" {
		t.Fatalf("projectId: got %q", coords.Get("projectId").String())
	}
	if coords.Get("tenantId").String() != "t1" {
		t.Fatalf("tenantId: got %q", coords.Get("tenantId").String())
	}
}
