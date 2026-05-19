package embedui_test

import (
	"testing"

	"github.com/dop251/goja"
)

func loadSummarizedPatchCtx(t *testing.T, vm *goja.Runtime) {
	t.Helper()
	evalJS(t, vm, logsUIPath(t, "testing", "loader.js"))
	evalJS(t, vm, logsUIPath(t, "util", "hash.js"))
	evalJS(t, vm, logsUIPath(t, "summarized", "hash.js"))
	evalJS(t, vm, logsUIPath(t, "summarized", "patch.js"))
}

func TestSummarizedPatch_diffReplaceCardWhenHashChanges(t *testing.T) {
	vm := goja.New()
	loadSummarizedPatchCtx(t, vm)

	_, err := vm.RunString(`
		var prev = {
			cards: [{ id: "a", kind: "conversation", hash: "h1", summary: {}, body: {} }],
			meta: { hasThreads: true }
		};
		var next = {
			cards: [{ id: "a", kind: "conversation", hash: "h2", summary: { n: 1 }, body: {} }],
			meta: { hasThreads: true }
		};
		var ops = ChimeraLogs.Summarized.Patch.diffSummarizedModels(prev, next);
	`)
	if err != nil {
		t.Fatal(err)
	}

	v, err := vm.RunString(`ops.length === 1 && ops[0].op === "replaceCard" && ops[0].id === "a"`)
	if err != nil {
		t.Fatal(err)
	}
	if !v.ToBoolean() {
		t.Fatalf("expected single replaceCard op, got %v", vm.Get("ops").Export())
	}
}

func TestSummarizedPatch_diffReplaceFeedOnStructureChange(t *testing.T) {
	vm := goja.New()
	loadSummarizedPatchCtx(t, vm)

	_, err := vm.RunString(`
		var prev = {
			cards: [{ id: "a", kind: "conversation", hash: "h1", summary: {}, body: {} }],
			meta: { hasThreads: true }
		};
		var next = {
			cards: [
				{ id: "a", kind: "conversation", hash: "h1", summary: {}, body: {} },
				{ id: "b", kind: "conversation", hash: "h2", summary: {}, body: {} }
			],
			meta: { hasThreads: true }
		};
		var ops = ChimeraLogs.Summarized.Patch.diffSummarizedModels(prev, next);
	`)
	if err != nil {
		t.Fatal(err)
	}

	v, err := vm.RunString(`ops.length === 1 && ops[0].op === "replaceFeed"`)
	if err != nil {
		t.Fatal(err)
	}
	if !v.ToBoolean() {
		t.Fatalf("expected replaceFeed on card add, got %v", vm.Get("ops").Export())
	}
}

func TestSummarizedPatch_diffOnlyCardIds(t *testing.T) {
	vm := goja.New()
	loadSummarizedPatchCtx(t, vm)

	_, err := vm.RunString(`
		var prev = {
			cards: [
				{ id: "a", kind: "conversation", hash: "h1", summary: {}, body: {} },
				{ id: "b", kind: "conversation", hash: "h1", summary: {}, body: {} }
			],
			meta: { hasThreads: true }
		};
		var next = {
			cards: [
				{ id: "a", kind: "conversation", hash: "h2", summary: {}, body: {} },
				{ id: "b", kind: "conversation", hash: "h3", summary: {}, body: {} }
			],
			meta: { hasThreads: true }
		};
		var only = { a: true };
		var ops = ChimeraLogs.Summarized.Patch.diffSummarizedModels(prev, next, { onlyCardIds: only });
	`)
	if err != nil {
		t.Fatal(err)
	}

	v, err := vm.RunString(`ops.length === 1 && ops[0].id === "a"`)
	if err != nil {
		t.Fatal(err)
	}
	if !v.ToBoolean() {
		t.Fatalf("expected only card a patched, got %v", vm.Get("ops").Export())
	}
}

func TestSummarizedPatch_applyReplaceCardOnDomStub(t *testing.T) {
	vm := goja.New()
	loadSummarizedPatchCtx(t, vm)

	_, err := vm.RunString(`
		var replaced = [];
		var nodes = {
			"a": {
				id: "a",
				tagName: "DETAILS",
				open: true,
				parentNode: { replaceChild: function (n, o) { replaced.push({ id: n.id, open: n.open, hash: n.getAttribute("data-card-hash") }); return n; } },
				querySelectorAll: function () { return []; },
				setAttribute: function (k, v) { this["_" + k] = v; },
				getAttribute: function (k) { return this["_" + k] || null; }
			}
		};
		var container = { id: "panel-summarized" };
		globalThis.document = {
			getElementById: function (id) { return nodes[id] || null; },
			createElement: function (tag) {
				return {
					tagName: String(tag || "div").toUpperCase(),
					innerHTML: "",
					firstElementChild: null,
					set innerHTML(html) {
						this.firstElementChild = {
							id: "a",
							tagName: "DETAILS",
							open: false,
							parentNode: null,
							querySelectorAll: function () { return []; },
							setAttribute: function (k, v) { this["_" + k] = v; },
							getAttribute: function (k) { return this["_" + k] || null; }
						};
					}
				};
			}
		};
		var ops = [{ op: "replaceCard", id: "a", card: { id: "a", hash: "h2" }, nextHash: "h2" }];
		var result = ChimeraLogs.Summarized.Patch.applySummarizedPatches(container, ops, {
			renderCard: function () { return '<details id="a"></details>'; }
		}, {
			replaceCard: function (id, html, opts) {
				var wrap = document.createElement("div");
				wrap.innerHTML = html;
				var newEl = wrap.firstElementChild;
				if (opts && opts.cardHash) newEl.setAttribute("data-card-hash", opts.cardHash);
				newEl.open = nodes[id].open;
				nodes[id].parentNode.replaceChild(newEl, nodes[id]);
				nodes[id] = newEl;
				return true;
			}
		});
	`)
	if err != nil {
		t.Fatal(err)
	}

	ok, err := vm.RunString(`result.ok && result.applied === 1 && replaced.length === 1 && replaced[0].hash === "h2"`)
	if err != nil {
		t.Fatal(err)
	}
	if !ok.ToBoolean() {
		t.Fatalf("apply patch failed: result=%v replaced=%v", vm.Get("result").Export(), vm.Get("replaced").Export())
	}
}
