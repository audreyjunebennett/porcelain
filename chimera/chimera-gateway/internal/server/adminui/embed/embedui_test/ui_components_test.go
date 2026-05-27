package embedui_test

import (
	"strings"
	"testing"

	"github.com/dop251/goja"
)

func TestUIComponents_Pill_httpStatusClass(t *testing.T) {
	vm := goja.New()
	loadChimeraUIBase(t, vm)
	evalJS(t, vm, uiEmbedPath(t, "components", "Pill.js"))

	pill := vm.Get("ChimeraUI").ToObject(vm).Get("Pill").ToObject(vm)
	fn, ok := goja.AssertFunction(pill.Get("httpStatusClass"))
	if !ok {
		t.Fatal("missing Pill.httpStatusClass")
	}
	v, err := fn(pill, vm.ToValue(404))
	if err != nil {
		t.Fatal(err)
	}
	if v.String() != "pill-4xx" {
		t.Fatalf("got %q", v.String())
	}
}

func TestUIComponents_Pill_renderHttpStatus_escapes(t *testing.T) {
	vm := goja.New()
	loadChimeraUIBase(t, vm)
	evalJS(t, vm, uiEmbedPath(t, "components", "Pill.js"))

	pill := vm.Get("ChimeraUI").ToObject(vm).Get("Pill").ToObject(vm)
	fn, ok := goja.AssertFunction(pill.Get("renderHttpStatus"))
	if !ok {
		t.Fatal("missing renderHttpStatus")
	}
	v, err := fn(pill, vm.ToValue(200))
	if err != nil {
		t.Fatal(err)
	}
	html := v.String()
	if !strings.Contains(html, "pill-2xx") || !strings.Contains(html, "200") {
		t.Fatalf("got %q", html)
	}
}

func TestUIComponents_StatusIndicator_evlogRow(t *testing.T) {
	vm := goja.New()
	loadChimeraUIBase(t, vm)
	evalJS(t, vm, uiEmbedPath(t, "components", "Pill.js"))
	evalJS(t, vm, uiEmbedPath(t, "components", "StatusIndicator.js"))

	si := vm.Get("ChimeraUI").ToObject(vm).Get("StatusIndicator").ToObject(vm)
	fn, ok := goja.AssertFunction(si.Get("evlogRow"))
	if !ok {
		t.Fatal("missing evlogRow")
	}
	v, err := fn(si, vm.ToValue(map[string]any{"levelKey": "ERROR", "http": 500}))
	if err != nil {
		t.Fatal(err)
	}
	html := v.String()
	if !strings.Contains(html, "sum-evlog-status__lvl--ERROR") || !strings.Contains(html, "pill-5xx") {
		t.Fatalf("got %q", html)
	}
}

func TestUIComponents_Chip_renderRow(t *testing.T) {
	vm := goja.New()
	loadChimeraUIBase(t, vm)
	evalJS(t, vm, uiEmbedPath(t, "components", "Chip.js"))

	chip := vm.Get("ChimeraUI").ToObject(vm).Get("Chip").ToObject(vm)
	fn, ok := goja.AssertFunction(chip.Get("renderRow"))
	if !ok {
		t.Fatal("missing renderRow")
	}
	v, err := fn(chip, vm.ToValue([]string{"a", "b<"}))
	if err != nil {
		t.Fatal(err)
	}
	html := v.String()
	if !strings.Contains(html, "service-chips") || strings.Contains(html, "<b") {
		t.Fatalf("got %q", html)
	}
}

func TestUIComponents_Chip_render_ignoresFunctionText(t *testing.T) {
	vm := goja.New()
	loadChimeraUIBase(t, vm)
	evalJS(t, vm, uiEmbedPath(t, "components", "Chip.js"))

	chip := vm.Get("ChimeraUI").ToObject(vm).Get("Chip").ToObject(vm)
	rowFn, ok := goja.AssertFunction(chip.Get("renderRow"))
	if !ok {
		t.Fatal("missing renderRow")
	}
	v, err := rowFn(chip, vm.ToValue([]any{vm.Get("ChimeraUI").ToObject(vm).Get("escapeHtml")}))
	if err != nil {
		t.Fatal(err)
	}
	html := v.String()
	if strings.Contains(html, "function escapeHtml") {
		t.Fatalf("function text leaked into chip row html: %q", html)
	}
	if html != "" {
		t.Fatalf("expected empty row for function-only parts, got %q", html)
	}
}

func TestUIComponents_KeyValueGrid_oddExtrasUsesColspan(t *testing.T) {
	vm := goja.New()
	loadChimeraUIBase(t, vm)
	evalJS(t, vm, uiEmbedPath(t, "components", "KeyValueGrid.js"))

	fn, ok := goja.AssertFunction(vm.Get("ChimeraUI").ToObject(vm).Get("KeyValueGrid"))
	if !ok {
		t.Fatal("missing KeyValueGrid")
	}
	extras := []map[string]string{{"k": "msg", "v": `hi<script>alert(1)</script>`}}
	v, err := fn(goja.Undefined(), vm.ToValue(extras))
	if err != nil {
		t.Fatal(err)
	}
	html := v.String()
	if !strings.Contains(html, `colspan="3"`) {
		t.Fatalf("expected colspan=3, got %q", html)
	}
}

func TestUIComponents_Badge_rendersClassAndTitle(t *testing.T) {
	vm := goja.New()
	loadChimeraUIBase(t, vm)
	evalJS(t, vm, uiEmbedPath(t, "components", "Badge.js"))

	badgeFn, ok := goja.AssertFunction(vm.Get("ChimeraUI").ToObject(vm).Get("Badge"))
	if !ok {
		t.Fatal("missing Badge")
	}
	model := map[string]any{"text": "chimera-vectorstore", "variant": "svc-chimera-vectorstore", "title": "vector store"}
	v, err := badgeFn(goja.Undefined(), vm.ToValue(model))
	if err != nil {
		t.Fatal(err)
	}
	html := v.String()
	if !strings.Contains(html, `sum-svc-chimera-vectorstore`) {
		t.Fatalf("expected svc class, got %q", html)
	}
}

func TestUIComponents_MetricPillsRow_renders(t *testing.T) {
	vm := goja.New()
	loadChimeraUIBase(t, vm)
	evalJS(t, vm, uiEmbedPath(t, "components", "MetricPillsRow.js"))

	fn, ok := goja.AssertFunction(vm.Get("ChimeraUI").ToObject(vm).Get("MetricPillsRow"))
	if !ok {
		t.Fatal("missing MetricPillsRow")
	}
	v, err := fn(goja.Undefined(), vm.ToValue([]map[string]string{{"text": "12 ms"}}))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(v.String(), "sum-metric") {
		t.Fatalf("got %q", v.String())
	}
}

func TestUIComponents_TimelineBar_segments(t *testing.T) {
	vm := goja.New()
	loadChimeraUIBase(t, vm)
	evalJS(t, vm, uiEmbedPath(t, "components", "TimelineBar.js"))

	tb := vm.Get("ChimeraUI").ToObject(vm).Get("TimelineBar").ToObject(vm)
	fn, ok := goja.AssertFunction(tb.Get("segments"))
	if !ok {
		t.Fatal("missing segments")
	}
	segs := []map[string]any{{"pct": 50, "bg": "#fff"}, {"pct": 0.01, "bg": "#000"}}
	v, err := fn(tb, vm.ToValue(segs))
	if err != nil {
		t.Fatal(err)
	}
	html := v.String()
	if !strings.Contains(html, "sum-timeline-bar") || !strings.Contains(html, "50.0%") {
		t.Fatalf("got %q", html)
	}
}

func TestUIComponents_Button_render(t *testing.T) {
	vm := goja.New()
	loadChimeraUIBase(t, vm)
	evalJS(t, vm, uiEmbedPath(t, "components", "Button.js"))

	btn := vm.Get("ChimeraUI").ToObject(vm).Get("Button").ToObject(vm)
	fn, ok := goja.AssertFunction(btn.Get("render"))
	if !ok {
		t.Fatal("missing Button.render")
	}
	v, err := fn(btn, vm.ToValue(map[string]any{"label": "Save", "variant": "primary"}))
	if err != nil {
		t.Fatal(err)
	}
	html := v.String()
	if !strings.Contains(html, "btn--primary") || !strings.Contains(html, "Save") {
		t.Fatalf("got %q", html)
	}
}

func TestUIComponents_Callout_render(t *testing.T) {
	vm := goja.New()
	loadChimeraUIBase(t, vm)
	evalJS(t, vm, uiEmbedPath(t, "components", "Callout.js"))

	co := vm.Get("ChimeraUI").ToObject(vm).Get("Callout").ToObject(vm)
	fn, ok := goja.AssertFunction(co.Get("render"))
	if !ok {
		t.Fatal("missing Callout.render")
	}
	v, err := fn(co, vm.ToValue("hello"), vm.ToValue(map[string]any{"escape": true}))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(v.String(), "callout") {
		t.Fatalf("got %q", v.String())
	}
}

func TestUIComponents_LogsShim_reexportsBadge(t *testing.T) {
	vm := goja.New()
	loadChimeraUIBase(t, vm)
	evalJS(t, vm, uiEmbedPath(t, "components", "Badge.js"))
	evalJS(t, vm, settingsUIPath(t, "components", "Badge.js"))

	fn := getFn(t, vm, "Badge")
	v, err := fn(goja.Undefined(), vm.ToValue(map[string]any{"text": "x", "variant": "svc-chimera-broker"}))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(v.String(), "sum-svc-broker") {
		t.Fatalf("shim failed: %q", v.String())
	}
}
