package embedui_test

import (
	"testing"

	"github.com/dop251/goja"
)

func loadSummarizedDirtyRoutingCtx(t *testing.T, vm *goja.Runtime) {
	t.Helper()
	evalJS(t, vm, settingsUIPath(t, "testing", "loader.js"))
	evalJS(t, vm, settingsUIPath(t, "util", "hash.js"))
	evalJS(t, vm, settingsUIPath(t, "derive", "logLineClassification.js"))
	evalJS(t, vm, settingsUIPath(t, "app", "summarizedDirtyRouting.js"))
	_, err := vm.RunString(`
		var deps = {
			getFlat: function (p) { return (p && p.rawFlat) || {}; },
			strHash: ChimeraSettings.strHash,
			getAdminProviderIds: function () { return ["groq"]; },
			normalizeServiceBucketKey: function (svc, src) {
				var s = String(svc || src || "").toLowerCase();
				if (s.indexOf("broker") >= 0 || s === "bifrost") return "chimera-broker";
				if (s.indexOf("vectorstore") >= 0 || s === "qdrant") return "chimera-vectorstore";
				if (s.indexOf("indexer") >= 0) return "chimera-indexer";
				if (s.indexOf("gateway") >= 0) return "chimera-gateway";
				return s || "chimera-gateway";
			}
		};
	`)
	if err != nil {
		t.Fatalf("dirty routing deps: %v", err)
	}
}

func TestSummarizedDirtyRouting_conversationAndService(t *testing.T) {
	vm := goja.New()
	loadSummarizedDirtyRoutingCtx(t, vm)

	fn, ok := goja.AssertFunction(vm.Get("ChimeraSettings").ToObject(vm).Get("Summarized").ToObject(vm).Get("dirtyTargetsForEntry"))
	if !ok {
		t.Fatal("missing ChimeraSettings.Summarized.dirtyTargetsForEntry")
	}

	v, err := fn(
		goja.Undefined(),
		vm.ToValue(map[string]any{
			"parsed": map[string]any{
				"rawFlat": map[string]any{
					"conversation_id": "conv-1",
					"principal_id":    "tenant-a",
					"service":         "chimera-gateway",
					"msg":             "chat.request",
				},
			},
		}),
		vm.ToValue(map[string]any{}),
		vm.Get("deps"),
	)
	if err != nil {
		t.Fatal(err)
	}
	obj := v.ToObject(vm)
	cardIds := obj.Get("cardIds").Export().([]any)
	if len(cardIds) < 2 {
		t.Fatalf("expected conversation + service card ids, got %v", cardIds)
	}
	convID, err := vm.RunString(`ChimeraSettings.Summarized.conversationCardIdForPrincipalAndCid("tenant-a", "conv-1", ChimeraSettings.strHash)`)
	if err != nil {
		t.Fatal(err)
	}
	foundConv := false
	for _, id := range cardIds {
		if id == convID.String() {
			foundConv = true
			break
		}
	}
	if !foundConv {
		t.Fatalf("missing conversation card id %q in %v", convID.String(), cardIds)
	}
	svcID, err := vm.RunString(`ChimeraSettings.Summarized.serviceCardIdForBucketKey("chimera-gateway", ChimeraSettings.strHash)`)
	if err != nil {
		t.Fatal(err)
	}
	foundSvc := false
	for _, id := range cardIds {
		if id == svcID.String() {
			foundSvc = true
			break
		}
	}
	if !foundSvc {
		t.Fatalf("missing service card id %q in %v", svcID.String(), cardIds)
	}
}

func TestSummarizedDirtyRouting_requestIdCorrelation(t *testing.T) {
	vm := goja.New()
	loadSummarizedDirtyRoutingCtx(t, vm)

	fn, ok := goja.AssertFunction(vm.Get("ChimeraSettings").ToObject(vm).Get("Summarized").ToObject(vm).Get("dirtyTargetsForEntry"))
	if !ok {
		t.Fatal("missing dirtyTargetsForEntry")
	}

	v, err := fn(
		goja.Undefined(),
		vm.ToValue(map[string]any{
			"parsed": map[string]any{
				"rawFlat": map[string]any{
					"request_id": "req-99",
					"msg":        "chat.chimera-broker.response",
				},
			},
		}),
		vm.ToValue(map[string]any{
			"reqToConv": map[string]any{
				"req-99": map[string]any{"pid": "tenant-b", "cid": "conv-b"},
			},
		}),
		vm.Get("deps"),
	)
	if err != nil {
		t.Fatal(err)
	}
	cardIds := v.ToObject(vm).Get("cardIds").Export().([]any)
	convID, err := vm.RunString(`ChimeraSettings.Summarized.conversationCardIdForPrincipalAndCid("tenant-b", "conv-b", ChimeraSettings.strHash)`)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, id := range cardIds {
		if id == convID.String() {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected correlated conv id %q in %v", convID.String(), cardIds)
	}
}

func TestSummarizedDirtyRouting_skipsProviderNotInVisibleList(t *testing.T) {
	vm := goja.New()
	loadSummarizedDirtyRoutingCtx(t, vm)

	_, err := vm.RunString(`
		deps.getAdminProviderIds = function () { return ["ollama"]; };
	`)
	if err != nil {
		t.Fatal(err)
	}

	fn, ok := goja.AssertFunction(vm.Get("ChimeraSettings").ToObject(vm).Get("Summarized").ToObject(vm).Get("dirtyTargetsForEntry"))
	if !ok {
		t.Fatal("missing dirtyTargetsForEntry")
	}

	v, err := fn(
		goja.Undefined(),
		vm.ToValue(map[string]any{
			"parsed": map[string]any{
				"rawFlat": map[string]any{
					"provider_id": "groq",
					"msg":         "chat.chimera-broker.request",
					"model":       "groq/llama",
				},
			},
		}),
		vm.ToValue(map[string]any{}),
		vm.Get("deps"),
	)
	if err != nil {
		t.Fatal(err)
	}
	cardIds := v.ToObject(vm).Get("cardIds").Export().([]any)
	for _, id := range cardIds {
		if id == "admin-provider-groq" {
			t.Fatalf("groq should not dirty-route when not visible: %v", cardIds)
		}
	}
}

func TestSummarizedDirtyRouting_adminProviderScoped(t *testing.T) {
	vm := goja.New()
	loadSummarizedDirtyRoutingCtx(t, vm)

	fn, ok := goja.AssertFunction(vm.Get("ChimeraSettings").ToObject(vm).Get("Summarized").ToObject(vm).Get("dirtyTargetsForEntry"))
	if !ok {
		t.Fatal("missing dirtyTargetsForEntry")
	}

	v, err := fn(
		goja.Undefined(),
		vm.ToValue(map[string]any{
			"parsed": map[string]any{
				"rawFlat": map[string]any{
					"provider_id": "groq",
					"msg":         "chat.chimera-broker.request",
					"model":       "groq/llama",
				},
			},
		}),
		vm.ToValue(map[string]any{}),
		vm.Get("deps"),
	)
	if err != nil {
		t.Fatal(err)
	}
	cardIds := v.ToObject(vm).Get("cardIds").Export().([]any)
	found := false
	for _, id := range cardIds {
		if id == "admin-provider-groq" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected admin-provider-groq in %v", cardIds)
	}
}

func TestSummarizedDirtyRouting_brokerRelayBucketsToBrokerCard(t *testing.T) {
	vm := goja.New()
	loadSummarizedDirtyRoutingCtx(t, vm)

	fn, ok := goja.AssertFunction(vm.Get("ChimeraSettings").ToObject(vm).Get("Summarized").ToObject(vm).Get("dirtyTargetsForEntry"))
	if !ok {
		t.Fatal("missing dirtyTargetsForEntry")
	}

	v, err := fn(
		goja.Undefined(),
		vm.ToValue(map[string]any{
			"parsed": map[string]any{
				"rawFlat": map[string]any{
					"service": "chimera-gateway",
					"msg":     "chat.chimera-broker.request",
				},
			},
		}),
		vm.ToValue(map[string]any{}),
		vm.Get("deps"),
	)
	if err != nil {
		t.Fatal(err)
	}
	cardIds := v.ToObject(vm).Get("cardIds").Export().([]any)
	brokerID, err := vm.RunString(`ChimeraSettings.Summarized.serviceCardIdForBucketKey("chimera-broker", ChimeraSettings.strHash)`)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, id := range cardIds {
		if id == brokerID.String() {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected broker service card %q in %v", brokerID.String(), cardIds)
	}
}

func TestSummarizedDirtyRouting_indexerSummaryDirtiesWorkspaceNotGateway(t *testing.T) {
	vm := goja.New()
	loadSummarizedDirtyRoutingCtx(t, vm)

	fn, ok := goja.AssertFunction(vm.Get("ChimeraSettings").ToObject(vm).Get("Summarized").ToObject(vm).Get("dirtyTargetsForEntry"))
	if !ok {
		t.Fatal("missing dirtyTargetsForEntry")
	}

	v, err := fn(
		goja.Undefined(),
		vm.ToValue(map[string]any{
			"parsed": map[string]any{
				"rawFlat": map[string]any{
					"service":            "chimera-indexer",
					"msg":                "indexer.job.ingested.summary",
					"index_run_id":       "run-1",
					"indexer_target_key": "itk_a",
					"ingest_succeeded":   12,
				},
			},
		}),
		vm.ToValue(map[string]any{}),
		vm.Get("deps"),
	)
	if err != nil {
		t.Fatal(err)
	}
	obj := v.ToObject(vm)
	cardIds := obj.Get("cardIds").Export().([]any)
	buckets := obj.Get("indexerBucketIds").Export().([]any)
	if len(buckets) != 1 || buckets[0] != "itk_a" {
		t.Fatalf("expected indexer bucket itk_a, got %v", buckets)
	}
	gatewayID, err := vm.RunString(`ChimeraSettings.Summarized.serviceCardIdForBucketKey("chimera-gateway", ChimeraSettings.strHash)`)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range cardIds {
		if id == gatewayID.String() {
			t.Fatalf("gateway service card should not be dirty for indexer summary, got %v", cardIds)
		}
	}
	indexerSvcID, err := vm.RunString(`ChimeraSettings.Summarized.serviceCardIdForBucketKey("chimera-indexer", ChimeraSettings.strHash)`)
	if err != nil {
		t.Fatal(err)
	}
	foundIndexerSvc := false
	for _, id := range cardIds {
		if id == indexerSvcID.String() {
			foundIndexerSvc = true
			break
		}
	}
	if !foundIndexerSvc {
		t.Fatalf("expected chimera-indexer service card in %v", cardIds)
	}
}

func TestSummarizedDirtyRouting_indexerQueueSnapshotSkipsDirty(t *testing.T) {
	vm := goja.New()
	loadSummarizedDirtyRoutingCtx(t, vm)

	fn, ok := goja.AssertFunction(vm.Get("ChimeraSettings").ToObject(vm).Get("Summarized").ToObject(vm).Get("dirtyTargetsForEntry"))
	if !ok {
		t.Fatal("missing dirtyTargetsForEntry")
	}

	v, err := fn(
		goja.Undefined(),
		vm.ToValue(map[string]any{
			"parsed": map[string]any{
				"rawFlat": map[string]any{
					"service":          "chimera-indexer",
					"msg":              "indexer.queue.snapshot",
					"index_run_id":     "run-1",
					"queue_depth":      0,
					"ingest_completed": 12,
				},
			},
		}),
		vm.ToValue(map[string]any{}),
		vm.Get("deps"),
	)
	if err != nil {
		t.Fatal(err)
	}
	obj := v.ToObject(vm)
	if len(obj.Get("cardIds").Export().([]any)) != 0 {
		t.Fatalf("queue snapshot should not dirty service cards, got %v", obj.Get("cardIds").Export())
	}
	if len(obj.Get("indexerBucketIds").Export().([]any)) != 0 {
		t.Fatalf("queue snapshot should not dirty workspace buckets, got %v", obj.Get("indexerBucketIds").Export())
	}
}

func TestSummarizedDirtyRouting_scopeStatusHeartbeatSkipsDirty(t *testing.T) {
	vm := goja.New()
	loadSummarizedDirtyRoutingCtx(t, vm)

	fn, ok := goja.AssertFunction(vm.Get("ChimeraSettings").ToObject(vm).Get("Summarized").ToObject(vm).Get("dirtyTargetsForEntry"))
	if !ok {
		t.Fatal("missing dirtyTargetsForEntry")
	}

	v, err := fn(
		goja.Undefined(),
		vm.ToValue(map[string]any{
			"parsed": map[string]any{
				"rawFlat": map[string]any{
					"service":            "chimera-indexer",
					"msg":                "indexer.scope.status",
					"change_reason":      "heartbeat",
					"index_run_id":       "run-1",
					"indexer_target_key": "itk_a",
				},
			},
		}),
		vm.ToValue(map[string]any{}),
		vm.Get("deps"),
	)
	if err != nil {
		t.Fatal(err)
	}
	obj := v.ToObject(vm)
	if len(obj.Get("cardIds").Export().([]any)) != 0 || len(obj.Get("indexerBucketIds").Export().([]any)) != 0 {
		t.Fatalf("heartbeat scope.status should not dirty cards, got cards=%v buckets=%v",
			obj.Get("cardIds").Export(), obj.Get("indexerBucketIds").Export())
	}
}

func TestSummarizedDirtyRouting_scopeStatusPhaseDirtiesWorkspace(t *testing.T) {
	vm := goja.New()
	loadSummarizedDirtyRoutingCtx(t, vm)

	fn, ok := goja.AssertFunction(vm.Get("ChimeraSettings").ToObject(vm).Get("Summarized").ToObject(vm).Get("dirtyTargetsForEntry"))
	if !ok {
		t.Fatal("missing dirtyTargetsForEntry")
	}

	v, err := fn(
		goja.Undefined(),
		vm.ToValue(map[string]any{
			"parsed": map[string]any{
				"rawFlat": map[string]any{
					"service":            "chimera-indexer",
					"msg":                "indexer.scope.status",
					"change_reason":      "phase",
					"index_run_id":       "run-1",
					"indexer_target_key": "itk_a",
				},
			},
		}),
		vm.ToValue(map[string]any{}),
		vm.Get("deps"),
	)
	if err != nil {
		t.Fatal(err)
	}
	obj := v.ToObject(vm)
	buckets := obj.Get("indexerBucketIds").Export().([]any)
	if len(buckets) != 1 || buckets[0] != "itk_a" {
		t.Fatalf("expected workspace bucket dirty on phase change, got %v", buckets)
	}
}

func TestSummarizedDirtyRouting_indexerPipelineSkipsGatewayServiceCard(t *testing.T) {
	vm := goja.New()
	loadSummarizedDirtyRoutingCtx(t, vm)

	fn, ok := goja.AssertFunction(vm.Get("ChimeraSettings").ToObject(vm).Get("Summarized").ToObject(vm).Get("dirtyTargetsForEntry"))
	if !ok {
		t.Fatal("missing dirtyTargetsForEntry")
	}

	v, err := fn(
		goja.Undefined(),
		vm.ToValue(map[string]any{
			"parsed": map[string]any{
				"rawFlat": map[string]any{
					"service":       "chimera-gateway",
					"msg":           "ingest.complete",
					"timeline_kind": "indexer",
					"index_run_id":  "run-1",
					"source":        "src/main.go",
				},
			},
		}),
		vm.ToValue(map[string]any{}),
		vm.Get("deps"),
	)
	if err != nil {
		t.Fatal(err)
	}
	obj := v.ToObject(vm)
	if len(obj.Get("cardIds").Export().([]any)) != 0 {
		t.Fatalf("indexer pipeline ingest.complete should not dirty gateway card, got %v", obj.Get("cardIds").Export())
	}
}
