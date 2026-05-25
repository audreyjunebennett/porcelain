package embedui_test

import (
	"testing"

	"github.com/dop251/goja"
)

func TestConvEvlogPrepareTurnEvents_hidesRagSpanWhenAttached(t *testing.T) {
	vm := goja.New()
	evalJS(t, vm, settingsUIPath(t, "testing", "loader.js"))
	evalJS(t, vm, settingsUIPath(t, "derive", "ragWorkspaceLabel.js"))
	evalJS(t, vm, settingsUIPath(t, "derive", "conversationEventLog.js"))

	fn, ok := goja.AssertFunction(vm.Get("ChimeraSettings").ToObject(vm).Get("Derive").ToObject(vm).Get("convEvlogPrepareTurnEvents"))
	if !ok {
		t.Fatal("missing convEvlogPrepareTurnEvents")
	}

	events := []map[string]any{
		{"seq": 1, "parsed": map[string]any{"rawFlat": map[string]any{"msg": "conversation.received", "turn_index": 1}}},
		{"seq": 2, "parsed": map[string]any{"rawFlat": map[string]any{"msg": "conversation.rag.span", "collection": "coll-a"}}},
		{"seq": 3, "parsed": map[string]any{"rawFlat": map[string]any{"msg": "conversation.rag.attached", "collection": "coll-a", "hits": 3}}},
		{"seq": 4, "parsed": map[string]any{"rawFlat": map[string]any{"msg": "conversation.delivered", "total_ms": 1000}}},
	}
	v, err := fn(goja.Undefined(), vm.ToValue(events), goja.Undefined())
	if err != nil {
		t.Fatal(err)
	}
	arr := v.Export().([]any)
	msgs := make([]string, 0, len(arr))
	for _, item := range arr {
		ev := item.(map[string]any)
		parsed := ev["parsed"].(map[string]any)
		raw := parsed["rawFlat"].(map[string]any)
		msgs = append(msgs, raw["msg"].(string))
	}
	for _, m := range msgs {
		if m == "conversation.rag.span" {
			t.Fatalf("rag.span should be hidden when attached present; msgs=%v", msgs)
		}
	}
	if len(msgs) != 3 {
		t.Fatalf("len=%d want 3 (received, attached, delivered); msgs=%v", len(msgs), msgs)
	}
}

func TestConvEvlogPrepareTurnEvents_attachesRagCoords(t *testing.T) {
	vm := goja.New()
	evalJS(t, vm, settingsUIPath(t, "testing", "loader.js"))
	evalJS(t, vm, settingsUIPath(t, "derive", "ragWorkspaceLabel.js"))
	evalJS(t, vm, settingsUIPath(t, "derive", "conversationEventLog.js"))

	fn, ok := goja.AssertFunction(vm.Get("ChimeraSettings").ToObject(vm).Get("Derive").ToObject(vm).Get("convEvlogPrepareTurnEvents"))
	if !ok {
		t.Fatal("missing convEvlogPrepareTurnEvents")
	}

	events := []map[string]any{
		{"seq": 1, "parsed": map[string]any{"rawFlat": map[string]any{"msg": "conversation.received", "turn_index": 1}}},
		{"seq": 2, "parsed": map[string]any{"rawFlat": map[string]any{
			"msg": "conversation.rag.span", "collection": "coll-a",
		}}},
		{"seq": 3, "parsed": map[string]any{"rawFlat": map[string]any{
			"msg": "rag.query", "tenant": "t1", "project": "workspacename", "collection": "coll-a",
		}}},
	}
	v, err := fn(goja.Undefined(), vm.ToValue(events), goja.Undefined())
	if err != nil {
		t.Fatal(err)
	}
	arr := v.Export().([]any)
	var meta map[string]any
	for _, item := range arr {
		ev := item.(map[string]any)
		if m, ok := ev["convEvlogMeta"].(map[string]any); ok {
			meta = m
			break
		}
	}
	if meta == nil {
		t.Fatal("missing convEvlogMeta on prepared events")
	}
	coords, ok := meta["ragCoords"].(map[string]any)
	if !ok {
		t.Fatalf("ragCoords missing from meta: %v", meta)
	}
	if coords["projectId"] != "workspacename" {
		t.Fatalf("projectId: got %v", coords["projectId"])
	}
}

func TestConvEvlogPrepareTurnEvents_storyOrderAndRoutingReposition(t *testing.T) {
	vm := goja.New()
	evalJS(t, vm, settingsUIPath(t, "testing", "loader.js"))
	evalJS(t, vm, settingsUIPath(t, "derive", "ragWorkspaceLabel.js"))
	evalJS(t, vm, settingsUIPath(t, "derive", "conversationEventLog.js"))

	fn, ok := goja.AssertFunction(vm.Get("ChimeraSettings").ToObject(vm).Get("Derive").ToObject(vm).Get("convEvlogPrepareTurnEvents"))
	if !ok {
		t.Fatal("missing convEvlogPrepareTurnEvents")
	}

	events := []map[string]any{
		{"seq": 1, "parsed": map[string]any{"rawFlat": map[string]any{"msg": "conversation.received", "turn_index": 1}}},
		{"seq": 2, "parsed": map[string]any{"rawFlat": map[string]any{"msg": "chat.chimera-broker.request", "upstreamModel": "groq/fail-model"}}},
		{"seq": 3, "parsed": map[string]any{"rawFlat": map[string]any{"msg": "chat.chimera-broker.response", "statusCode": 404, "upstreamModel": "groq/fail-model"}}},
		{"seq": 4, "parsed": map[string]any{"rawFlat": map[string]any{"msg": "conversation.routing.resolved", "upstreamModel": "groq/ok-model", "attempt": 2, "chainLen": 3, "clientModel": "Chimera-0.2.0"}}},
		{"seq": 5, "parsed": map[string]any{"rawFlat": map[string]any{"msg": "chat.chimera-broker.request", "upstreamModel": "groq/ok-model"}}},
		{"seq": 6, "parsed": map[string]any{"rawFlat": map[string]any{"msg": "chat.chimera-broker.response", "statusCode": 200, "upstreamModel": "groq/ok-model", "usageTotalTokens": 100}}},
		{"seq": 7, "parsed": map[string]any{"rawFlat": map[string]any{"msg": "conversation.delivered", "total_ms": 500}}},
	}
	v, err := fn(goja.Undefined(), vm.ToValue(events), goja.Undefined())
	if err != nil {
		t.Fatal(err)
	}
	arr := v.Export().([]any)
	msgs := make([]string, 0, len(arr))
	for _, item := range arr {
		ev := item.(map[string]any)
		parsed := ev["parsed"].(map[string]any)
		raw := parsed["rawFlat"].(map[string]any)
		msgs = append(msgs, raw["msg"].(string))
	}
	want := []string{
		"conversation.received",
		"chat.chimera-broker.request",
		"chat.chimera-broker.response",
		"conversation.routing.resolved",
		"chat.chimera-broker.request",
		"chat.chimera-broker.response",
		"conversation.delivered",
	}
	if len(msgs) != len(want) {
		t.Fatalf("len=%d want %d; msgs=%v", len(msgs), len(want), msgs)
	}
	for i := range want {
		if msgs[i] != want[i] {
			t.Fatalf("idx %d: got %q want %q; all=%v", i, msgs[i], want[i], msgs)
		}
	}
}
