package embedui_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/dop251/goja"
)

func searchUIPath(t *testing.T, rel ...string) string {
	t.Helper()
	base := filepath.Join(embeduiRoot(t), "search")
	return filepath.Join(append([]string{base}, rel...)...)
}

func TestSearchHTML_layoutAndAssets(t *testing.T) {
	html := mustReadFile(t, embeduiRoot(t)+"/search.html")
	css := mustReadFile(t, embeduiRoot(t)+"/styles/search.css")
	for _, needle := range []string{
		`class="search-page embed-page"`,
		`id="search-query"`,
		`id="search-workspace"`,
		`id="search-threshold"`,
		`id="search-threshold-slider"`,
		`readiness_score`,
		`id="search-results"`,
		`class="search-controls"`,
		`class="search-results"`,
		"/ui/assets/styles/chat.css",
		"/ui/assets/styles/search.css",
		"/ui/assets/search/app.js",
		"/ui/assets/chat/render/snippet.js",
	} {
		if !strings.Contains(html, needle) {
			t.Fatalf("search.html missing %q", needle)
		}
	}
	if strings.Contains(html, `class="chat-composer"`) {
		t.Fatal("search.html should use search-controls, not chat-composer")
	}
	if !strings.Contains(css, ".search-controls") || !strings.Contains(css, ".search-results") {
		t.Fatal("search.css must define top controls and scrollable results")
	}
	if !strings.Contains(css, ".search-controls__threshold-slider") {
		t.Fatal("search.css must define threshold slider")
	}
	if !strings.Contains(css, "flex: 1 1 auto") || !strings.Contains(css, "min-height: 0") {
		t.Fatal("search.css must use flex scroll column for results viewport")
	}
}

func TestSearchHighlight_wrapsTerms(t *testing.T) {
	vm := goja.New()
	evalJS(t, vm, uiEmbedPath(t, "util", "escape.js"))
	evalJS(t, vm, searchUIPath(t, "render", "highlight.js"))

	fn, ok := goja.AssertFunction(vm.Get("ChimeraSearch").ToObject(vm).Get("Render").ToObject(vm).Get("Highlight").ToObject(vm).Get("highlightPlain"))
	if !ok {
		t.Fatal("missing highlightPlain")
	}
	v, err := fn(goja.Undefined(), vm.ToValue("hello world fixture"), vm.ToValue("world fix"))
	if err != nil {
		t.Fatal(err)
	}
	out := v.String()
	if !strings.Contains(out, `<mark class="search-hl">world</mark>`) {
		t.Fatalf("expected world highlight, got %q", out)
	}
	if !strings.Contains(out, `<mark class="search-hl">fix</mark>`) {
		t.Fatalf("expected fix highlight, got %q", out)
	}
}

func TestSearchResults_zeroHitsWithHint(t *testing.T) {
	vm := goja.New()
	evalJS(t, vm, uiEmbedPath(t, "util", "escape.js"))
	evalJS(t, vm, searchUIPath(t, "render", "results.js"))

	fn, ok := goja.AssertFunction(vm.Get("ChimeraSearch").ToObject(vm).Get("Render").ToObject(vm).Get("Results").ToObject(vm).Get("renderResultsView"))
	if !ok {
		t.Fatal("missing renderResultsView")
	}
	payload := map[string]any{
		"lastQuery": "alpha",
		"lastResponse": map[string]any{
			"hits":            []any{},
			"indexer_hint":    "empty_collection",
			"score_threshold": 0.72,
			"top_k":           8,
		},
	}
	v, err := fn(goja.Undefined(), vm.ToValue(payload))
	if err != nil {
		t.Fatal(err)
	}
	out := v.String()
	for _, needle := range []string{"No results", "search-hint", "No indexed content", "search-empty", "threshold 72%"} {
		if !strings.Contains(out, needle) {
			t.Fatalf("missing %q in %q", needle, out)
		}
	}
}
