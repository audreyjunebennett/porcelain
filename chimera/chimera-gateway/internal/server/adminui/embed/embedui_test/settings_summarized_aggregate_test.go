package embedui_test

import (
	"testing"

	"github.com/dop251/goja"
)

func loadSummarizedAggregateCtx(t *testing.T, vm *goja.Runtime) {
	t.Helper()
	evalJS(t, vm, settingsUIPath(t, "testing", "loader.js"))
	evalJS(t, vm, settingsUIPath(t, "util", "hash.js"))
	evalJS(t, vm, settingsUIPath(t, "derive", "logLineClassification.js"))
	evalJS(t, vm, settingsUIPath(t, "derive", "conversationAggregate.js"))
	evalJS(t, vm, settingsUIPath(t, "summarized", "aggregate.js"))
}

func TestSummarizedAggregate_groupsDirectConversation(t *testing.T) {
	vm := goja.New()
	loadSummarizedAggregateCtx(t, vm)

	_, err := vm.RunString(`
		var entryCache = [{
			seq: 1,
			ts: "2026-01-01T12:00:00Z",
			text: "chat",
			parsed: {
				rawFlat: {
					msg: "chat.request",
					conversation_id: "conv-1",
					principal_id: "tenant-a"
				}
			}
		}];
		var agg = ChimeraSettings.Summarized.buildAggregateState(entryCache, {
			getFlat: function (p) { return (p && p.rawFlat) || {}; },
			entryInstant: function (o) { return o && o.ts ? new Date(o.ts) : null; },
			normalizeServiceBucketKey: function () { return "chimera-gateway"; },
			collectIndexerRunMeta: function () { return null; },
			indexerRootScopeByRootId: {},
			operatorWsFullLogCtx: {},
			lastIndexerOperatorWorkspacesNested: []
		});
		if (!agg.mergedConv || agg.mergedConv.length !== 1) {
			throw new Error("expected one merged conversation, got " + (agg.mergedConv ? agg.mergedConv.length : 0));
		}
		if (agg.mergedConv[0].cid !== "conv-1") throw new Error("wrong cid");
		if (!agg.buckets["chimera-gateway"] || agg.buckets["chimera-gateway"].length !== 1) {
			throw new Error("expected gateway bucket line");
		}
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestSummarizedAggregate_brokerRelayBucketsUnderChimeraBroker(t *testing.T) {
	vm := goja.New()
	loadSummarizedAggregateCtx(t, vm)

	_, err := vm.RunString(`
		var entryCache = [{
			seq: 2,
			ts: "2026-01-01T12:00:01Z",
			text: "relay",
			parsed: {
				rawFlat: { msg: "chat.chimera-broker.request", service: "gateway" }
			}
		}];
		var agg = ChimeraSettings.Summarized.buildAggregateState(entryCache, {
			getFlat: function (p) { return (p && p.rawFlat) || {}; },
			entryInstant: function (o) { return o && o.ts ? new Date(o.ts) : null; },
			normalizeServiceBucketKey: function () { return "chimera-gateway"; },
			collectIndexerRunMeta: function () { return null; },
			indexerRootScopeByRootId: {},
			operatorWsFullLogCtx: {},
			lastIndexerOperatorWorkspacesNested: []
		});
		if (!agg.buckets["chimera-broker"] || agg.buckets["chimera-broker"].length !== 1) {
			throw new Error("expected chimera-broker bucket");
		}
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestConversationAggregate_requestIdCorrelation(t *testing.T) {
	vm := goja.New()
	loadSummarizedAggregateCtx(t, vm)

	_, err := vm.RunString(`
		var reqToConv = {};
		ChimeraSettings.Derive.tryRegisterRequestConversationCorrelationPrimary(reqToConv, {
			request_id: "req-1",
			conversation_id: "c-1",
			principal_id: "p-1",
			msg: "chat.request"
		});
		if (!reqToConv["req-1"] || reqToConv["req-1"].cid !== "c-1") {
			throw new Error("primary correlation failed");
		}
	`)
	if err != nil {
		t.Fatal(err)
	}
}
