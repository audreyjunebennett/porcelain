package embedui_test

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/dop251/goja"
)

func TestLogsComponents_escapeHtml(t *testing.T) {
	vm := goja.New()
	evalJS(t, vm, settingsUIPath(t, "testing", "loader.js"))
	evalJS(t, vm, uiEmbedPath(t, "util", "escape.js"))
	evalJS(t, vm, settingsUIPath(t, "util", "escape.js"))

	escapeHtml := getFn(t, vm, "escapeHtml")
	v, err := escapeHtml(goja.Undefined(), vm.ToValue(`x<y & "z"`))
	if err != nil {
		t.Fatal(err)
	}
	if got := v.String(); got != `x&lt;y &amp; &quot;z&quot;` {
		t.Fatalf("escapeHtml: %q", got)
	}
}

func TestLogsComponents_KeyValueGrid_oddExtrasUsesColspan(t *testing.T) {
	vm := goja.New()
	evalJS(t, vm, settingsUIPath(t, "testing", "loader.js"))
	evalJS(t, vm, uiEmbedPath(t, "util", "escape.js"))
	evalJS(t, vm, uiEmbedPath(t, "components", "KeyValueGrid.js"))
	evalJS(t, vm, settingsUIPath(t, "components", "KeyValueGrid.js"))

	keyValueGrid := getFn(t, vm, "KeyValueGrid")

	// One extra: should produce a row with colspan=3 for the value cell.
	extras := []map[string]string{{"k": "msg", "v": `hi<script>alert(1)</script>`}}
	v, err := keyValueGrid(goja.Undefined(), vm.ToValue(extras))
	if err != nil {
		t.Fatal(err)
	}
	html := v.String()
	if !strings.Contains(html, `colspan="3"`) {
		t.Fatalf("expected colspan=3, got %q", html)
	}
	if strings.Contains(html, "<script") {
		t.Fatalf("should be escaped: %q", html)
	}
}

func TestLogsComponents_Badge_rendersClassAndTitle(t *testing.T) {
	vm := goja.New()
	evalJS(t, vm, settingsUIPath(t, "testing", "loader.js"))
	evalJS(t, vm, uiEmbedPath(t, "util", "escape.js"))
	evalJS(t, vm, uiEmbedPath(t, "components", "Badge.js"))
	evalJS(t, vm, settingsUIPath(t, "components", "Badge.js"))

	badge := getFn(t, vm, "Badge")

	model := map[string]any{"text": "chimera-vectorstore", "variant": "svc-chimera-vectorstore", "title": "vector store"}
	v, err := badge(goja.Undefined(), vm.ToValue(model))
	if err != nil {
		t.Fatal(err)
	}
	html := v.String()
	if !strings.Contains(html, `sum-svc-chimera-vectorstore`) {
		t.Fatalf("expected svc class, got %q", html)
	}
	if !strings.Contains(html, `title="vector store"`) {
		t.Fatalf("expected title, got %q", html)
	}
}

func TestLogsDerive_scrapeConversationMetrics_totalTokensAndVec(t *testing.T) {
	vm := goja.New()
	// load minimal runtime (no DOM required)
	evalJS(t, vm, settingsUIPath(t, "testing", "loader.js"))
	evalJS(t, vm, settingsUIPath(t, "derive", "conversationMetrics.js"))

	chimera := vm.Get("ChimeraSettings").ToObject(vm)
	derive := chimera.Get("Derive").ToObject(vm)
	scrape, ok := goja.AssertFunction(derive.Get("scrapeConversationMetrics"))
	if !ok {
		t.Fatal("missing ChimeraSettings.Derive.scrapeConversationMetrics")
	}

	events := []map[string]any{
		{"parsed": map[string]any{"rawFlat": map[string]any{"usageTotalTokens": 123, "rag_hits": 9}}},
		{"parsed": map[string]any{"rawFlat": map[string]any{"usageTotalTokens": 5}}},
	}

	// use default getFlat (reads parsed.rawFlat)
	v, err := scrape(goja.Undefined(), vm.ToValue(events), goja.Undefined())
	if err != nil {
		t.Fatal(err)
	}
	obj := v.ToObject(vm)
	if tok := obj.Get("tok"); tok.Export() != int64(128) {
		t.Fatalf("tok=%v", tok.Export())
	}
	if vec := obj.Get("vec"); vec.Export() != int64(9) {
		t.Fatalf("vec=%v", vec.Export())
	}
}

func TestLogsDerive_conversationBrokerRelayCount_andFlat(t *testing.T) {
	vm := goja.New()
	evalJS(t, vm, settingsUIPath(t, "testing", "loader.js"))
	evalJS(t, vm, settingsUIPath(t, "derive", "conversationBroker.js"))

	derive := vm.Get("ChimeraSettings").ToObject(vm).Get("Derive").ToObject(vm)
	countFn, ok := goja.AssertFunction(derive.Get("conversationBrokerRelayCount"))
	if !ok {
		t.Fatal("missing conversationBrokerRelayCount")
	}
	flatFn, ok := goja.AssertFunction(derive.Get("conversationBrokerTimelineFlat"))
	if !ok {
		t.Fatal("missing conversationBrokerTimelineFlat")
	}

	events := []map[string]any{
		{"parsed": map[string]any{"rawFlat": map[string]any{"msg": "chat.chimera-broker.request"}}},
		{"parsed": map[string]any{"rawFlat": map[string]any{"msg": "chat.chimera-broker.response"}}},
		{"parsed": map[string]any{"rawFlat": map[string]any{"msg": "chat.routing.fallback"}}},
		{"parsed": map[string]any{"rawFlat": map[string]any{"msg": "rag.query"}}},
	}
	v, err := countFn(goja.Undefined(), vm.ToValue(events), goja.Undefined())
	if err != nil {
		t.Fatal(err)
	}
	if v.Export().(int64) != 3 {
		t.Fatalf("count=%v want 3", v.Export())
	}

	vAvail, err := flatFn(goja.Undefined(), vm.ToValue(map[string]any{"msg": "chat.chimera-broker.available_models"}))
	if err != nil {
		t.Fatal(err)
	}
	if b, ok := vAvail.Export().(bool); !ok || b {
		t.Fatalf("available_models flat=%v want false", vAvail.Export())
	}

	vFb, err := flatFn(goja.Undefined(), vm.ToValue(map[string]any{"msg": "chat.chimera-broker.request"}))
	if err != nil {
		t.Fatal(err)
	}
	if b, ok := vFb.Export().(bool); !ok || !b {
		t.Fatalf("request flat=%v want true", vFb.Export())
	}
}

func TestLogsDerive_conversationCardModel_joinAndProgress(t *testing.T) {
	vm := goja.New()
	evalJS(t, vm, settingsUIPath(t, "testing", "loader.js"))
	evalJS(t, vm, settingsUIPath(t, "derive", "conversationBroker.js"))
	evalJS(t, vm, settingsUIPath(t, "derive", "conversationCardModel.js"))

	derive := vm.Get("ChimeraSettings").ToObject(vm).Get("Derive").ToObject(vm)
	joinFn, ok := goja.AssertFunction(derive.Get("joinQdrantLineConversationTier"))
	if !ok {
		t.Fatal("missing joinQdrantLineConversationTier")
	}
	matchFn, ok := goja.AssertFunction(derive.Get("joinQdrantLineConversationMatch"))
	if !ok {
		t.Fatal("missing joinQdrantLineConversationMatch")
	}
	modelFn, ok := goja.AssertFunction(derive.Get("buildConversationCardModel"))
	if !ok {
		t.Fatal("missing buildConversationCardModel")
	}

	ms0 := time.Date(2026, 5, 9, 12, 0, 0, 0, time.UTC).UnixMilli()
	ms3 := ms0 + 3000
	ms7 := ms0 + 7000
	ms10 := ms0 + 10000
	ms11 := ms0 + 11000

	legacyEvs := []map[string]any{
		{
			"ts":     "2026-05-09T12:00:00.000Z",
			"parsed": map[string]any{"rawFlat": map[string]any{"msg": "rag.query", "collection": "coll-x"}},
		},
	}
	vNear, err := joinFn(goja.Undefined(), vm.ToValue(legacyEvs), goja.Undefined(), vm.ToValue(map[string]any{"collection": "coll-x"}), vm.ToValue(ms3))
	if err != nil {
		t.Fatal(err)
	}
	if vNear.String() != "inferred" {
		t.Fatalf("near legacy join=%q want inferred", vNear.String())
	}
	vFar, err := joinFn(goja.Undefined(), vm.ToValue(legacyEvs), goja.Undefined(), vm.ToValue(map[string]any{"collection": "coll-x"}), vm.ToValue(ms7))
	if err != nil {
		t.Fatal(err)
	}
	if goja.IsNull(vFar) || goja.IsUndefined(vFar) {
		// ok
	} else if s := vFar.String(); s != "" && s != "null" {
		t.Fatalf("far join=%q want null", s)
	}

	spanEvs := []map[string]any{
		{
			"ts":     "2026-05-09T12:00:00.000Z",
			"parsed": map[string]any{"rawFlat": map[string]any{"msg": "conversation.rag.span", "collection": "coll-x"}},
		},
	}
	vAnch, err := joinFn(goja.Undefined(), vm.ToValue(spanEvs), goja.Undefined(), vm.ToValue(map[string]any{"collection": "coll-x"}), vm.ToValue(ms10))
	if err != nil {
		t.Fatal(err)
	}
	if vAnch.String() != "anchored_inferred" {
		t.Fatalf("span join=%q want anchored_inferred", vAnch.String())
	}
	vBeyond, err := joinFn(goja.Undefined(), vm.ToValue(spanEvs), goja.Undefined(), vm.ToValue(map[string]any{"collection": "coll-x"}), vm.ToValue(ms11))
	if err != nil {
		t.Fatal(err)
	}
	if !goja.IsNull(vBeyond) && !goja.IsUndefined(vBeyond) {
		t.Fatalf("span default window join=%q want null after 10s", vBeyond.String())
	}

	overlapEvs := []map[string]any{
		{
			"ts":     "2026-05-09T12:00:00.000Z",
			"parsed": map[string]any{"rawFlat": map[string]any{"msg": "conversation.rag.span", "collection": "coll-x", "span_id": "span-old", "window_ms": 10000, "turn_index": 1}},
		},
		{
			"ts":     "2026-05-09T12:00:05.000Z",
			"parsed": map[string]any{"rawFlat": map[string]any{"msg": "conversation.rag.span", "collection": "coll-x", "span_id": "span-new", "window_ms": 10000, "turn_index": 2}},
		},
	}
	vMatch, err := matchFn(goja.Undefined(), vm.ToValue(overlapEvs), goja.Undefined(), vm.ToValue(map[string]any{"collection": "coll-x"}), vm.ToValue(ms7))
	if err != nil {
		t.Fatal(err)
	}
	match := vMatch.ToObject(vm)
	if got := match.Get("tier").String(); got != "anchored_inferred" {
		t.Fatalf("match tier=%q want anchored_inferred", got)
	}
	if got := match.Get("span_id").String(); got != "span-new" {
		t.Fatalf("span_id=%q want span-new", got)
	}
	if got := match.Get("turn_index").Export(); got != int64(2) {
		t.Fatalf("turn_index=%v want 2", got)
	}

	lifecycleEvs := []map[string]any{
		{"ts": "2026-05-09T12:00:00.000Z", "parsed": map[string]any{"rawFlat": map[string]any{"msg": "conversation.received", "clientModel": "c1", "turn_index": 1}}},
		{"ts": "2026-05-09T12:00:01.000Z", "parsed": map[string]any{"rawFlat": map[string]any{"msg": "conversation.routing.resolved", "upstreamModel": "m1"}}},
		{"ts": "2026-05-09T12:00:02.000Z", "parsed": map[string]any{"rawFlat": map[string]any{"msg": "conversation.rag.span", "collection": "cx"}}},
		{"ts": "2026-05-09T12:00:03.000Z", "parsed": map[string]any{"rawFlat": map[string]any{"msg": "conversation.rag.skipped", "reason": "no_hits"}}},
		{"ts": "2026-05-09T12:00:04.000Z", "parsed": map[string]any{"rawFlat": map[string]any{"msg": "conversation.broker.started"}}},
		{"ts": "2026-05-09T12:00:05.000Z", "parsed": map[string]any{"rawFlat": map[string]any{"msg": "conversation.broker.completed"}}},
		{"ts": "2026-05-09T12:00:06.000Z", "parsed": map[string]any{"rawFlat": map[string]any{"msg": "conversation.delivered"}}},
	}
	vMod, err := modelFn(goja.Undefined(), vm.ToValue(lifecycleEvs), goja.Undefined())
	if err != nil {
		t.Fatal(err)
	}
	mod := vMod.ToObject(vm)
	prog := mod.Get("progress").ToObject(vm)
	if prog.Get("rag").String() != "skipped" {
		t.Fatalf("rag=%q want skipped", prog.Get("rag").String())
	}
	if prog.Get("broker").String() != "done" {
		t.Fatalf("broker=%q want done", prog.Get("broker").String())
	}
	if prog.Get("delivered").String() != "done" {
		t.Fatalf("delivered=%q want done", prog.Get("delivered").String())
	}
}

func TestLogsDerive_conversationCardModel_toolsChipCounts(t *testing.T) {
	vm := goja.New()
	evalJS(t, vm, settingsUIPath(t, "testing", "loader.js"))
	evalJS(t, vm, settingsUIPath(t, "derive", "conversationCardModel.js"))

	derive := vm.Get("ChimeraSettings").ToObject(vm).Get("Derive").ToObject(vm)
	modelFn, ok := goja.AssertFunction(derive.Get("buildConversationCardModel"))
	if !ok {
		t.Fatal("missing buildConversationCardModel")
	}

	toolEvs := []map[string]any{
		{"ts": "2026-05-09T12:00:00.000Z", "parsed": map[string]any{"rawFlat": map[string]any{"msg": "conversation.tool.call_started", "turn_index": 1}}},
		{"ts": "2026-05-09T12:00:01.000Z", "parsed": map[string]any{"rawFlat": map[string]any{"msg": "conversation.tool.call_completed", "turn_index": 1}}},
		{"ts": "2026-05-09T12:00:02.000Z", "parsed": map[string]any{"rawFlat": map[string]any{"msg": "conversation.tool.call_failed", "turn_index": 1}}},
		{"ts": "2026-05-09T12:00:03.000Z", "parsed": map[string]any{"rawFlat": map[string]any{"msg": "chat.tool_router.applied", "turn_index": 1}}},
		{"ts": "2026-05-09T12:00:04.000Z", "parsed": map[string]any{"rawFlat": map[string]any{"msg": "conversation.tool.router", "turn_index": 1}}},
	}
	vMod, err := modelFn(goja.Undefined(), vm.ToValue(toolEvs), goja.Undefined())
	if err != nil {
		t.Fatal(err)
	}
	mod := vMod.ToObject(vm)
	chips := mod.Get("chips").ToObject(vm)
	if got := chips.Get("tools").Export(); got != int64(3) {
		t.Fatalf("tools chip=%v want 3 (completed+failed+chat.tool_router; call_started and conversation.tool.router excluded)", got)
	}
}

func TestLogsDerive_buildConversationCardModel_witnessFlags(t *testing.T) {
	vm := goja.New()
	evalJS(t, vm, settingsUIPath(t, "testing", "loader.js"))
	evalJS(t, vm, settingsUIPath(t, "derive", "conversationCardModel.js"))

	derive := vm.Get("ChimeraSettings").ToObject(vm).Get("Derive").ToObject(vm)
	modelFn, ok := goja.AssertFunction(derive.Get("buildConversationCardModel"))
	if !ok {
		t.Fatal("missing buildConversationCardModel")
	}
	evs := []map[string]any{
		{"ts": "2026-05-09T12:00:00.000Z", "parsed": map[string]any{"rawFlat": map[string]any{"msg": "conversation.received", "turn_index": 1}}},
		{"ts": "2026-05-09T12:00:01.000Z", "parsed": map[string]any{"rawFlat": map[string]any{"msg": "conversation.request.witness", "turn_index": 1}}},
		{"ts": "2026-05-09T12:00:02.000Z", "parsed": map[string]any{"rawFlat": map[string]any{"msg": "conversation.response.witness", "turn_index": 1}}},
		{"ts": "2026-05-09T12:00:03.000Z", "parsed": map[string]any{"rawFlat": map[string]any{"msg": "conversation.delivered", "turn_index": 1}}},
	}
	vMod, err := modelFn(goja.Undefined(), vm.ToValue(evs), goja.Undefined())
	if err != nil {
		t.Fatal(err)
	}
	mod := vMod.ToObject(vm)
	w := mod.Get("witness").ToObject(vm)
	if !w.Get("request").ToBoolean() {
		t.Fatal("expected witness.request true")
	}
	if !w.Get("response").ToBoolean() {
		t.Fatal("expected witness.response true")
	}
	if mod.Get("stateLabel").String() == "request witness" {
		t.Fatalf("state pill should not use witness msg; got %q", mod.Get("stateLabel").String())
	}
}

func TestLogsDerive_conversationCardModel_phase5QdrantFixture(t *testing.T) {
	vm := goja.New()
	evalJS(t, vm, settingsUIPath(t, "testing", "loader.js"))
	evalJS(t, vm, settingsUIPath(t, "derive", "conversationCardModel.js"))

	derive := vm.Get("ChimeraSettings").ToObject(vm).Get("Derive").ToObject(vm)
	matchFn, ok := goja.AssertFunction(derive.Get("joinQdrantLineConversationMatch"))
	if !ok {
		t.Fatal("missing joinQdrantLineConversationMatch")
	}

	path := serverTestdataPath(t, "correlation", "phase5-qdrant-tier4b.example.log")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var spanEvs []map[string]any
	var qFlat map[string]any
	var qTimeMs int64
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		flat := make(map[string]any)
		ts := ""
		for _, field := range fields {
			kv := strings.SplitN(field, "=", 2)
			if len(kv) != 2 {
				continue
			}
			if kv[0] == "time" {
				ts = kv[1]
				continue
			}
			flat[kv[0]] = kv[1]
		}
		switch flat["msg"] {
		case "conversation.rag.span":
			spanEvs = append(spanEvs, map[string]any{
				"ts":     ts,
				"parsed": map[string]any{"rawFlat": flat},
			})
		case "qdrant.http.vector_search":
			qFlat = flat
			tm, err := time.Parse(time.RFC3339Nano, ts)
			if err != nil {
				t.Fatalf("parse fixture qdrant time: %v", err)
			}
			qTimeMs = tm.UnixMilli()
		}
	}
	if len(spanEvs) != 2 || qFlat == nil || qTimeMs == 0 {
		t.Fatalf("fixture did not yield two spans and one qdrant row: spans=%d qFlat=%v qTimeMs=%d", len(spanEvs), qFlat, qTimeMs)
	}
	vMatch, err := matchFn(goja.Undefined(), vm.ToValue(spanEvs), goja.Undefined(), vm.ToValue(qFlat), vm.ToValue(qTimeMs))
	if err != nil {
		t.Fatal(err)
	}
	match := vMatch.ToObject(vm)
	if got := match.Get("tier").String(); got != "anchored_inferred" {
		t.Fatalf("tier=%q want anchored_inferred", got)
	}
	if got := match.Get("span_id").String(); got != "span-new" {
		t.Fatalf("span_id=%q want span-new", got)
	}
	if got := match.Get("turn_index").Export(); got != int64(2) {
		t.Fatalf("turn_index=%v want 2", got)
	}
}

func TestLogsDerive_conversationTurnGroupsForExpanded_multiTurnFixture(t *testing.T) {
	vm := goja.New()
	evalJS(t, vm, settingsUIPath(t, "testing", "loader.js"))
	evalJS(t, vm, settingsUIPath(t, "derive", "conversationCardModel.js"))

	derive := vm.Get("ChimeraSettings").ToObject(vm).Get("Derive").ToObject(vm)
	groupFn, ok := goja.AssertFunction(derive.Get("conversationTurnGroupsForExpanded"))
	if !ok {
		t.Fatal("missing conversationTurnGroupsForExpanded")
	}

	path := serverTestdataPath(t, "correlation", "phase6-multi-turn.example.log")
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var events []map[string]any
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		flat := make(map[string]any)
		ts := ""
		for _, field := range fields {
			kv := strings.SplitN(field, "=", 2)
			if len(kv) != 2 {
				continue
			}
			if kv[0] == "time" {
				ts = kv[1]
				continue
			}
			flat[kv[0]] = kv[1]
		}
		events = append(events, map[string]any{
			"ts":     ts,
			"parsed": map[string]any{"rawFlat": flat},
		})
	}
	if len(events) < 14 {
		t.Fatalf("fixture parsed only %d events", len(events))
	}

	vGroups, err := groupFn(goja.Undefined(), vm.ToValue(events), goja.Undefined())
	if err != nil {
		t.Fatal(err)
	}
	groupsArr := vGroups.ToObject(vm)
	n := int(groupsArr.Get("length").ToInteger())
	if n != 2 {
		t.Fatalf("groups=%d want 2", n)
	}

	type turnSnapshot struct {
		turnIndex int
		label     string
		count     int
		firstMsg  string
		lastMsg   string
	}
	snapshots := make([]turnSnapshot, 0, n)
	for i := 0; i < n; i++ {
		grp := groupsArr.Get(strconv.Itoa(i)).ToObject(vm)
		ti := int(grp.Get("turnIndex").ToInteger())
		label := grp.Get("label").String()
		evsArr := grp.Get("events").ToObject(vm)
		count := int(evsArr.Get("length").ToInteger())
		first := evsArr.Get("0").ToObject(vm).Get("parsed").ToObject(vm).Get("rawFlat").ToObject(vm).Get("msg").String()
		last := evsArr.Get(strconv.Itoa(count - 1)).ToObject(vm).Get("parsed").ToObject(vm).Get("rawFlat").ToObject(vm).Get("msg").String()
		snapshots = append(snapshots, turnSnapshot{turnIndex: ti, label: label, count: count, firstMsg: first, lastMsg: last})
	}

	if snapshots[0].turnIndex != 2 || snapshots[1].turnIndex != 1 {
		t.Fatalf("groups=%v want most-recent-turn-first (2,1)", snapshots)
	}
	if snapshots[0].label != "Turn 2" || snapshots[1].label != "Turn 1" {
		t.Fatalf("labels=%q,%q want Turn 2,Turn 1", snapshots[0].label, snapshots[1].label)
	}
	for _, s := range snapshots {
		if s.count != 7 {
			t.Fatalf("turn %d events=%d want 7", s.turnIndex, s.count)
		}
		if s.firstMsg != "conversation.received" {
			t.Fatalf("turn %d first=%q want conversation.received (ascending order)", s.turnIndex, s.firstMsg)
		}
		if s.lastMsg != "conversation.delivered" {
			t.Fatalf("turn %d last=%q want conversation.delivered", s.turnIndex, s.lastMsg)
		}
	}
}

func TestLogsDerive_conversationTurnGroupsForExpanded_inheritsAndUnattributed(t *testing.T) {
	vm := goja.New()
	evalJS(t, vm, settingsUIPath(t, "testing", "loader.js"))
	evalJS(t, vm, settingsUIPath(t, "derive", "conversationCardModel.js"))

	derive := vm.Get("ChimeraSettings").ToObject(vm).Get("Derive").ToObject(vm)
	groupFn, ok := goja.AssertFunction(derive.Get("conversationTurnGroupsForExpanded"))
	if !ok {
		t.Fatal("missing conversationTurnGroupsForExpanded")
	}

	events := []map[string]any{
		{"ts": "2026-05-09T12:00:00.000Z", "seq": 1, "parsed": map[string]any{"rawFlat": map[string]any{"msg": "conversation.merge.resolve_failed"}}},
		{"ts": "2026-05-09T12:00:00.010Z", "seq": 2, "parsed": map[string]any{"rawFlat": map[string]any{"msg": "conversation.received", "turn_index": 1}}},
		{"ts": "2026-05-09T12:00:00.020Z", "seq": 3, "parsed": map[string]any{"rawFlat": map[string]any{"msg": "chat.chimera-broker.request"}}},
		{"ts": "2026-05-09T12:00:00.030Z", "seq": 4, "qdrantTurnIndex": 1, "parsed": map[string]any{"rawFlat": map[string]any{"msg": "qdrant.http.vector_search"}}},
	}

	vGroups, err := groupFn(goja.Undefined(), vm.ToValue(events), goja.Undefined())
	if err != nil {
		t.Fatal(err)
	}
	groupsArr := vGroups.ToObject(vm)
	if int(groupsArr.Get("length").ToInteger()) != 2 {
		t.Fatalf("groups=%d want 2 (Unattributed + Turn 1)", groupsArr.Get("length").ToInteger())
	}
	first := groupsArr.Get("0").ToObject(vm)
	if !goja.IsNull(first.Get("turnIndex")) {
		t.Fatalf("first group turnIndex=%v want null (Unattributed before any turn_index)", first.Get("turnIndex").Export())
	}
	if first.Get("label").String() != "Unattributed" {
		t.Fatalf("first label=%q want Unattributed", first.Get("label").String())
	}
	second := groupsArr.Get("1").ToObject(vm)
	if int(second.Get("turnIndex").ToInteger()) != 1 {
		t.Fatalf("second turnIndex=%v want 1", second.Get("turnIndex").Export())
	}
	secondEvs := second.Get("events").ToObject(vm)
	if int(secondEvs.Get("length").ToInteger()) != 3 {
		t.Fatalf("turn 1 events=%d want 3 (received + inherited chimera broker + qdrant)", secondEvs.Get("length").ToInteger())
	}
	if got := secondEvs.Get("1").ToObject(vm).Get("parsed").ToObject(vm).Get("rawFlat").ToObject(vm).Get("msg").String(); got != "chat.chimera-broker.request" {
		t.Fatalf("turn 1 second msg=%q want chat.chimera-broker.request", got)
	}
	if got := secondEvs.Get("2").ToObject(vm).Get("parsed").ToObject(vm).Get("rawFlat").ToObject(vm).Get("msg").String(); got != "qdrant.http.vector_search" {
		t.Fatalf("turn 1 third msg=%q want qdrant.http.vector_search", got)
	}
}

func TestLogsDerive_conversationRequestIdTier2Eligible_narrowChatPrefixes(t *testing.T) {
	vm := goja.New()
	evalJS(t, vm, settingsUIPath(t, "testing", "loader.js"))
	evalJS(t, vm, settingsUIPath(t, "derive", "conversationBroker.js"))
	evalJS(t, vm, settingsUIPath(t, "derive", "conversationCardModel.js"))
	elig, ok := goja.AssertFunction(vm.Get("ChimeraSettings").ToObject(vm).Get("Derive").ToObject(vm).Get("conversationRequestIdTier2Eligible"))
	if !ok {
		t.Fatal("missing conversationRequestIdTier2Eligible")
	}
	call := func(flat map[string]any) bool {
		t.Helper()
		v, err := elig(goja.Undefined(), vm.ToValue(flat), goja.Undefined())
		if err != nil {
			t.Fatal(err)
		}
		b, ok := v.Export().(bool)
		if !ok {
			t.Fatalf("want bool got %T", v.Export())
		}
		return b
	}
	if !call(map[string]any{"msg": "rag.query"}) {
		t.Fatal("rag.query should be eligible")
	}
	if !call(map[string]any{"msg": "chat.routing.resolved"}) {
		t.Fatal("chat.routing.* should be eligible")
	}
	if !call(map[string]any{"msg": "chat.tool_router.applied"}) {
		t.Fatal("chat.tool_router.* should be eligible")
	}
	if call(map[string]any{"msg": "chat.noise.synthetic"}) {
		t.Fatal("unknown chat.* should not be blanket-eligible")
	}
	if call(map[string]any{"msg": "ingest.complete"}) {
		t.Fatal("ingest.* should not use tier 2")
	}
}

func TestLogsDerive_chimeraBrokerCardMetrics_countsAndRateLimit(t *testing.T) {
	vm := goja.New()
	evalJS(t, vm, settingsUIPath(t, "testing", "loader.js"))
	evalJS(t, vm, settingsUIPath(t, "derive", "chimeraBrokerMetrics.js"))

	chimera := vm.Get("ChimeraSettings").ToObject(vm)
	derive := chimera.Get("Derive").ToObject(vm)
	metricFn, ok := goja.AssertFunction(derive.Get("chimeraBrokerCardMetrics"))
	if !ok {
		t.Fatal("missing ChimeraSettings.Derive.chimeraBrokerCardMetrics")
	}

	arr := []map[string]any{
		{"text": "429 rate limit", "parsed": map[string]any{"shape": "chat.chimera-broker", "rawFlat": map[string]any{"msg": "chat.chimera-broker.request", "outgoingTokens": 10, "stream": true, "upstreamModel": "groq/x", "statusCode": 429}}},
		{"text": "", "parsed": map[string]any{"shape": "chat.chimera-broker", "rawFlat": map[string]any{"msg": "chat.chimera-broker.response", "usageTotalTokens": 20, "responseBytes": 100, "statusCode": 200}}},
		{"text": "", "parsed": map[string]any{"shape": "chat.chimera-broker", "rawFlat": map[string]any{"msg": "chat.chimera-broker.error", "statusCode": 500}}},
	}

	v, err := metricFn(goja.Undefined(), vm.ToValue(arr), goja.Undefined())
	if err != nil {
		t.Fatal(err)
	}
	obj := v.ToObject(vm)
	if obj.Get("reqN").Export() != int64(1) {
		t.Fatalf("reqN=%v", obj.Get("reqN").Export())
	}
	if obj.Get("resN").Export() != int64(1) {
		t.Fatalf("resN=%v", obj.Get("resN").Export())
	}
	if obj.Get("errN").Export() != int64(1) {
		t.Fatalf("errN=%v", obj.Get("errN").Export())
	}
	if obj.Get("rlN").Export() != int64(1) {
		t.Fatalf("rlN=%v", obj.Get("rlN").Export())
	}
	if obj.Get("sc2xx").Export() != int64(1) {
		t.Fatalf("sc2xx=%v", obj.Get("sc2xx").Export())
	}
	// legacy behavior: only counts statusCode for http.access, chat.chimera-broker.response (legacy: upstream chat response), and chimera-broker.error lines
	// (request lines don't contribute to scErr even if they contain 429 strings).
	if obj.Get("scErr").Export() != int64(1) {
		t.Fatalf("scErr=%v", obj.Get("scErr").Export())
	}
}

func TestLogsDerive_chimeraBrokerOperatorLine(t *testing.T) {
	vm := goja.New()
	evalJS(t, vm, settingsUIPath(t, "testing", "loader.js"))
	evalJS(t, vm, settingsUIPath(t, "operator_copy.js"))
	evalJS(t, vm, settingsUIPath(t, "render", "operatorMessage.js"))
	evalJS(t, vm, settingsUIPath(t, "render", "operatorMessageServices.js"))
	evalJS(t, vm, settingsUIPath(t, "derive", "chimeraBrokerMetrics.js"))

	derive := vm.Get("ChimeraSettings").ToObject(vm).Get("Derive").ToObject(vm)
	fn, ok := goja.AssertFunction(derive.Get("chimeraBrokerOperatorLine"))
	if !ok {
		t.Fatal("missing ChimeraSettings.Derive.chimeraBrokerOperatorLine")
	}

	cases := []struct {
		flat map[string]any
		want string
	}{
		{
			flat: map[string]any{"service": "chimera-broker", "msg": "chimera-broker.http.access", "http_method": "GET", "http_target": "http://localhost:8081/v1/models", "http_status": 200, "http_duration_ms": 3.2},
			want: "Inbound · GET /v1/models · → 200 · 3 ms",
		},
		{
			flat: map[string]any{"service": "chimera-broker", "msg": "chimera-broker.rate_limit", "http_method": "POST", "http_target": "/v1/embeddings", "http_status": 429, "http_duration_ms": 10},
			want: "Rate limited · POST /v1/embeddings · → 429 · 10 ms",
		},
		{
			flat: map[string]any{"service": "chimera-broker", "msg": "chimera-broker.catalog.sync", "catalog_model_count": 42},
			want: "Model catalog updated · 42 models",
		},
		{
			flat: map[string]any{"service": "gateway", "msg": "chat.chimera-broker.available_models", "catalog_model_count": 100},
			want: "Model list for routing · 100 models",
		},
		{
			flat: map[string]any{"msg": "chat.chimera-broker.request", "upstreamModel": "groq/openai/gpt-oss-20b", "stream": true, "outgoingTokens": 128},
			want: "Relay request · gpt-oss-20b · 128 input tok est",
		},
		{
			flat: map[string]any{"msg": "chat.chimera-broker.response", "statusCode": 200, "usageTotalTokens": 50, "responseBytes": 1200},
			want: "Relay response · HTTP 200 · 50 usage tok",
		},
		{
			flat: map[string]any{"msg": "upstream chat response", "statusCode": 201, "usageTotalTokens": 1, "responseBytes": 10},
			want: "Relay response · HTTP 201 · 1 usage tok",
		},
		{
			flat: map[string]any{"msg": "chat.routing.attempt", "attempt": 2, "upstreamModel": "x/y/z", "chainLen": 3},
			want: "Routing attempt · z · attempt 2/3",
		},
		{
			flat: map[string]any{"msg": "chat.chimera-broker.error", "err": "connection reset"},
			want: "Relay failed · connection reset",
		},
		{
			flat: map[string]any{"service": "chimera-broker", "msg": "chimera-broker.ready", "listen_url": "http://127.0.0.1:8899/ui"},
			want: "Ready · UI at http://127.0.0.1:8899/ui",
		},
		{
			flat: map[string]any{"service": "chimera-broker", "msg": "chimera-broker.version", "chimera_broker_version": "1.2.3"},
			want: "chimera-broker version 1.2.3 (backend: BiFrost)",
		},
		{
			flat: map[string]any{"service": "chimera-broker", "msg": "chimera-broker.startup.banner"},
			want: "chimera-broker starting (backend: BiFrost)",
		},
	}

	for i, tc := range cases {
		v, err := fn(goja.Undefined(), vm.ToValue(tc.flat))
		if err != nil {
			t.Fatalf("case %d: %v", i, err)
		}
		got := v.String()
		if got != tc.want {
			t.Fatalf("case %d: got %q want %q", i, got, tc.want)
		}
	}

	evCases := []struct {
		flat map[string]any
		want string
	}{
		{
			flat: map[string]any{"service": "chimera-broker", "msg": "chimera-broker.http.access", "http_method": "GET", "http_target": "http://localhost:8081/v1/models", "http_status": 200, "http_duration_ms": 3.2},
			want: "Inbound · GET /v1/models · 3 ms",
		},
		{
			flat: map[string]any{"msg": "chat.chimera-broker.response", "statusCode": 200, "usageTotalTokens": 50, "responseBytes": 1200, "finish_reason": "stop"},
			want: "Provider responded · 50 tokens used (prompt + completion) · finish: stop",
		},
	}
	opts := map[string]any{"forEventLog": true}
	for i, tc := range evCases {
		v, err := fn(goja.Undefined(), vm.ToValue(tc.flat), vm.ToValue(opts))
		if err != nil {
			t.Fatalf("forEventLog case %d: %v", i, err)
		}
		got := v.String()
		if got != tc.want {
			t.Fatalf("forEventLog case %d: got %q want %q", i, got, tc.want)
		}
	}

	vSilent, err := fn(goja.Undefined(), vm.ToValue(map[string]any{"msg": "gateway.startup.banner"}))
	if err != nil {
		t.Fatal(err)
	}
	if vSilent.String() != "" {
		t.Fatalf("non-chimera-broker msg should return empty got %q", vSilent.String())
	}
}

func TestLogsDerive_chimeraBrokerSliceSinceLastBanner_resetsCounters(t *testing.T) {
	vm := goja.New()
	evalJS(t, vm, settingsUIPath(t, "testing", "loader.js"))
	evalJS(t, vm, settingsUIPath(t, "derive", "chimeraBrokerMetrics.js"))

	derive := vm.Get("ChimeraSettings").ToObject(vm).Get("Derive").ToObject(vm)
	sliceFn, ok := goja.AssertFunction(derive.Get("chimeraBrokerSliceSinceLastBanner"))
	if !ok {
		t.Fatal("missing chimeraBrokerSliceSinceLastBanner")
	}
	metricFn, ok := goja.AssertFunction(derive.Get("chimeraBrokerCardMetrics"))
	if !ok {
		t.Fatal("missing chimeraBrokerCardMetrics")
	}

	arr := []map[string]any{
		{"text": "", "parsed": map[string]any{"rawFlat": map[string]any{"msg": "chimera-broker.provider.loaded", "service": "chimera-broker", "provider_id": "old"}}},
		{"text": "", "parsed": map[string]any{"rawFlat": map[string]any{"msg": "chimera-broker.startup.banner", "service": "chimera-broker"}}},
		{"text": "", "parsed": map[string]any{"rawFlat": map[string]any{"msg": "chimera-broker.provider.loaded", "service": "chimera-broker", "provider_id": "new"}}},
	}
	sv, err := sliceFn(goja.Undefined(), vm.ToValue(arr), goja.Undefined())
	if err != nil {
		t.Fatal(err)
	}
	slice := sv.Export().([]any)
	if len(slice) != 2 {
		t.Fatalf("slice len=%d want 2", len(slice))
	}

	v, err := metricFn(goja.Undefined(), vm.ToValue(arr), goja.Undefined())
	if err != nil {
		t.Fatal(err)
	}
	obj := v.ToObject(vm)
	if obj.Get("providersTotal").Export() != int64(1) {
		t.Fatalf("providersTotal=%v", obj.Get("providersTotal").Export())
	}
}

func TestLogsDerive_chimeraBrokerCardModel_kv(t *testing.T) {
	vm := goja.New()
	evalJS(t, vm, settingsUIPath(t, "testing", "loader.js"))
	evalJS(t, vm, settingsUIPath(t, "derive", "chimeraBrokerMetrics.js"))

	fn, ok := goja.AssertFunction(vm.Get("ChimeraSettings").ToObject(vm).Get("Derive").ToObject(vm).Get("chimeraBrokerCardModel"))
	if !ok {
		t.Fatal("missing chimeraBrokerCardModel")
	}
	arr := []map[string]any{
		{"text": `{"msg":"chimera-broker.version","service":"chimera-broker","chimera_broker_version":"1.2.3"}`, "parsed": map[string]any{"rawFlat": map[string]any{"msg": "chimera-broker.version", "service": "chimera-broker", "chimera_broker_version": "1.2.3"}}},
		{"text": "", "parsed": map[string]any{"rawFlat": map[string]any{"msg": "chimera-broker.config.loaded", "service": "chimera-broker"}}},
		{"text": "", "parsed": map[string]any{"rawFlat": map[string]any{"msg": "chimera-broker.ready", "service": "chimera-broker", "listen_port": 8080}}},
		{"text": "", "parsed": map[string]any{"rawFlat": map[string]any{"msg": "chimera-broker.auth.token_refresh", "service": "chimera-broker"}}},
		{"text": "", "parsed": map[string]any{"rawFlat": map[string]any{"msg": "chat.chimera-broker.request", "upstreamModel": "groq/llama"}}},
	}
	v, err := fn(goja.Undefined(), vm.ToValue(arr), goja.Undefined())
	if err != nil {
		t.Fatal(err)
	}
	o := v.ToObject(vm)
	if o.Get("version").String() != "1.2.3" {
		t.Fatalf("version=%q", o.Get("version").String())
	}
	if o.Get("configuration").String() != "supervised" {
		t.Fatalf("configuration=%q", o.Get("configuration").String())
	}
	if o.Get("port").String() != "8080" {
		t.Fatalf("port=%q", o.Get("port").String())
	}
	if o.Get("lastModel").String() != "groq/llama" {
		t.Fatalf("lastModel=%q", o.Get("lastModel").String())
	}
	if o.Get("auth").String() != "jwt" {
		t.Fatalf("auth=%q want jwt from chimera-broker.auth.token_refresh", o.Get("auth").String())
	}
}

func TestLogsDerive_chimeraBrokerCardModel_canonicalSlugs(t *testing.T) {
	vm := goja.New()
	evalJS(t, vm, settingsUIPath(t, "testing", "loader.js"))
	evalJS(t, vm, settingsUIPath(t, "derive", "chimeraBrokerMetrics.js"))

	fn, ok := goja.AssertFunction(vm.Get("ChimeraSettings").ToObject(vm).Get("Derive").ToObject(vm).Get("chimeraBrokerCardModel"))
	if !ok {
		t.Fatal("missing chimeraBrokerCardModel")
	}
	arr := []map[string]any{
		{"parsed": map[string]any{"rawFlat": map[string]any{"msg": "broker.config.loaded", "service": "chimera-broker"}}},
		{"parsed": map[string]any{"rawFlat": map[string]any{"msg": "broker.ready", "service": "chimera-broker", "listen_port": 8081}}},
		{"parsed": map[string]any{"rawFlat": map[string]any{"msg": "broker.governance.startup", "service": "chimera-broker"}}},
		{"parsed": map[string]any{"rawFlat": map[string]any{"msg": "broker.mcp.startup", "service": "chimera-broker"}}},
	}
	v, err := fn(goja.Undefined(), vm.ToValue(arr), goja.Undefined())
	if err != nil {
		t.Fatal(err)
	}
	o := v.ToObject(vm)
	if o.Get("configuration").String() != "supervised" {
		t.Fatalf("configuration=%q", o.Get("configuration").String())
	}
	if o.Get("port").String() != "8081" {
		t.Fatalf("port=%q", o.Get("port").String())
	}
	if o.Get("governance").String() != "enabled" {
		t.Fatalf("governance=%q", o.Get("governance").String())
	}
	if o.Get("mcp").String() != "enabled" {
		t.Fatalf("mcp=%q", o.Get("mcp").String())
	}
}

func TestLogsDerive_vectorstoreCardModel_canonicalSlugs(t *testing.T) {
	vm := goja.New()
	evalJS(t, vm, settingsUIPath(t, "testing", "loader.js"))
	evalJS(t, vm, settingsUIPath(t, "derive", "vectorstoreCollection.js"))

	fn, ok := goja.AssertFunction(vm.Get("ChimeraSettings").ToObject(vm).Get("Derive").ToObject(vm).Get("vectorstoreCardModel"))
	if !ok {
		t.Fatal("missing vectorstoreCardModel")
	}
	arr := []map[string]any{
		{"parsed": map[string]any{"rawFlat": map[string]any{
			"msg": "vectorstore.version", "service": "chimera-vectorstore", "qdrant_version": "1.12.0",
		}}},
		{"parsed": map[string]any{"rawFlat": map[string]any{
			"msg": "vectorstore.listen.http", "service": "chimera-vectorstore", "rest_port": 6333,
		}}},
		{"parsed": map[string]any{"rawFlat": map[string]any{
			"msg": "vectorstore.http.points_upsert_ok", "service": "chimera-vectorstore", "http_status": 200,
		}}},
		{"parsed": map[string]any{"rawFlat": map[string]any{
			"msg": "vectorstore.collection.loading", "service": "chimera-vectorstore",
		}}},
		{"parsed": map[string]any{"rawFlat": map[string]any{
			"msg": "vectorstore.shard.recovered", "service": "chimera-vectorstore",
		}}},
	}
	v, err := fn(goja.Undefined(), vm.ToValue(arr), goja.Undefined())
	if err != nil {
		t.Fatal(err)
	}
	o := v.ToObject(vm)
	if o.Get("version").String() != "1.12.0" {
		t.Fatalf("version=%q", o.Get("version").String())
	}
	if o.Get("restPort").Export() != int64(6333) {
		t.Fatalf("restPort=%v", o.Get("restPort").Export())
	}
	if o.Get("upsertOk").Export() != int64(1) {
		t.Fatalf("upsertOk=%v", o.Get("upsertOk").Export())
	}
	if o.Get("collTotal").Export() != int64(1) || o.Get("collLoaded").Export() != int64(1) {
		t.Fatalf("collections=%v/%v", o.Get("collTotal").Export(), o.Get("collLoaded").Export())
	}
}

func TestLogsDerive_wrapperBackendPanelLabel(t *testing.T) {
	vm := goja.New()
	evalJS(t, vm, settingsUIPath(t, "testing", "loader.js"))
	evalJS(t, vm, settingsUIPath(t, "derive", "vectorstoreCollection.js"))

	fn, ok := goja.AssertFunction(vm.Get("ChimeraSettings").ToObject(vm).Get("Derive").ToObject(vm).Get("wrapperBackendPanelLabel"))
	if !ok {
		t.Fatal("missing wrapperBackendPanelLabel")
	}
	v, err := fn(goja.Undefined(), vm.ToValue("qdrant"), vm.ToValue("binary"))
	if err != nil {
		t.Fatal(err)
	}
	if v.String() != "qdrant (binary)" {
		t.Fatalf("label=%q", v.String())
	}
	v2, err := fn(goja.Undefined(), vm.ToValue(""), vm.ToValue(""))
	if err != nil {
		t.Fatal(err)
	}
	if v2.String() != "—" {
		t.Fatalf("empty label=%q want em dash", v2.String())
	}
}

func TestLogsDerive_vectorstoreCardModel_wrapperBackend(t *testing.T) {
	vm := goja.New()
	evalJS(t, vm, settingsUIPath(t, "testing", "loader.js"))
	evalJS(t, vm, settingsUIPath(t, "derive", "vectorstoreCollection.js"))

	fn, ok := goja.AssertFunction(vm.Get("ChimeraSettings").ToObject(vm).Get("Derive").ToObject(vm).Get("vectorstoreCardModel"))
	if !ok {
		t.Fatal("missing vectorstoreCardModel")
	}
	arr := []map[string]any{
		{"parsed": map[string]any{"rawFlat": map[string]any{
			"msg": "vectorstore.ready", "service": "chimera-vectorstore",
			"backend_name": "qdrant", "backend_mode": "binary",
		}}},
	}
	v, err := fn(goja.Undefined(), vm.ToValue(arr), goja.Undefined(), goja.Undefined())
	if err != nil {
		t.Fatal(err)
	}
	o := v.ToObject(vm)
	if o.Get("backendName").String() != "qdrant" {
		t.Fatalf("backendName=%q", o.Get("backendName").String())
	}
	if o.Get("backendMode").String() != "binary" {
		t.Fatalf("backendMode=%q", o.Get("backendMode").String())
	}
}

func TestLogsDerive_chimeraBrokerCardModel_wrapperBackend(t *testing.T) {
	vm := goja.New()
	evalJS(t, vm, settingsUIPath(t, "testing", "loader.js"))
	evalJS(t, vm, settingsUIPath(t, "derive", "chimeraBrokerMetrics.js"))
	evalJS(t, vm, settingsUIPath(t, "derive", "vectorstoreCollection.js"))

	fn, ok := goja.AssertFunction(vm.Get("ChimeraSettings").ToObject(vm).Get("Derive").ToObject(vm).Get("chimeraBrokerCardModel"))
	if !ok {
		t.Fatal("missing chimeraBrokerCardModel")
	}
	arr := []map[string]any{
		{"parsed": map[string]any{"rawFlat": map[string]any{
			"msg": "broker.ready", "service": "chimera-broker",
			"backend_name": "bifrost", "backend_mode": "binary",
		}}},
	}
	v, err := fn(goja.Undefined(), vm.ToValue(arr), goja.Undefined())
	if err != nil {
		t.Fatal(err)
	}
	o := v.ToObject(vm)
	if o.Get("backendName").String() != "bifrost" {
		t.Fatalf("backendName=%q", o.Get("backendName").String())
	}
	if o.Get("backendMode").String() != "binary" {
		t.Fatalf("backendMode=%q", o.Get("backendMode").String())
	}
}

func TestLogsDerive_vectorstoreCardModel_httpUpsertSummary(t *testing.T) {
	vm := goja.New()
	evalJS(t, vm, settingsUIPath(t, "testing", "loader.js"))
	evalJS(t, vm, settingsUIPath(t, "derive", "vectorstoreCollection.js"))

	fn, ok := goja.AssertFunction(vm.Get("ChimeraSettings").ToObject(vm).Get("Derive").ToObject(vm).Get("vectorstoreCardModel"))
	if !ok {
		t.Fatal("missing vectorstoreCardModel")
	}
	arr := []map[string]any{
		{"parsed": map[string]any{"rawFlat": map[string]any{
			"msg": "vectorstore.version", "service": "chimera-vectorstore", "qdrant_version": "1.12.0",
		}}},
		{"parsed": map[string]any{"rawFlat": map[string]any{
			"msg": "vectorstore.http.upsert.summary", "service": "chimera-vectorstore",
			"upserts_ok": 42, "deletes_ok": 1, "searches_ok": 3,
		}}},
	}
	v, err := fn(goja.Undefined(), vm.ToValue(arr), goja.Undefined())
	if err != nil {
		t.Fatal(err)
	}
	o := v.ToObject(vm)
	if o.Get("upsertOk").Export() != int64(42) {
		t.Fatalf("upsertOk=%v", o.Get("upsertOk").Export())
	}
	if o.Get("deleteOk").Export() != int64(1) {
		t.Fatalf("deleteOk=%v", o.Get("deleteOk").Export())
	}
	if o.Get("searchOk").Export() != int64(3) {
		t.Fatalf("searchOk=%v", o.Get("searchOk").Export())
	}
}

func TestLogsDerive_chimeraBrokerProviderHealthList_pickLatest(t *testing.T) {
	vm := goja.New()
	evalJS(t, vm, settingsUIPath(t, "testing", "loader.js"))
	evalJS(t, vm, settingsUIPath(t, "derive", "chimeraBrokerMetrics.js"))

	derive := vm.Get("ChimeraSettings").ToObject(vm).Get("Derive").ToObject(vm)
	fn, ok := goja.AssertFunction(derive.Get("chimeraBrokerProviderHealthList"))
	if !ok {
		t.Fatal("missing chimeraBrokerProviderHealthList")
	}

	arr := []map[string]any{
		{"parsed": map[string]any{"rawFlat": map[string]any{"msg": "chimera-broker.provider.loaded", "service": "chimera-broker", "provider_id": "groq"}}},
		{"parsed": map[string]any{"rawFlat": map[string]any{"msg": "chimera-broker.provider.loaded", "service": "chimera-broker", "provider_id": "openai"}}},
		{"parsed": map[string]any{"rawFlat": map[string]any{"msg": "chimera-broker.provider.loaded", "service": "chimera-broker", "provider_id": "gemini"}}},
		{"parsed": map[string]any{"rawFlat": map[string]any{"msg": "chimera-broker.provider.health.fail", "service": "chimera-broker", "provider_id": "gemini"}}},
		{"parsed": map[string]any{"rawFlat": map[string]any{"msg": "chimera-broker.provider.health.ok", "service": "chimera-broker", "provider_id": "gemini"}}},
		{"parsed": map[string]any{"rawFlat": map[string]any{"msg": "chimera-broker.provider.health.fail", "service": "chimera-broker", "provider_id": "openai"}}},
		{"parsed": map[string]any{"rawFlat": map[string]any{"msg": "chimera-broker.provider.key_missing", "service": "chimera-broker", "provider_id": "anthropic"}}},
	}
	v, err := fn(goja.Undefined(), vm.ToValue(arr), goja.Undefined())
	if err != nil {
		t.Fatal(err)
	}
	list, ok := v.Export().([]any)
	if !ok {
		t.Fatalf("expected []any, got %T", v.Export())
	}
	if len(list) != 4 {
		t.Fatalf("len=%d want 4", len(list))
	}

	want := map[string]string{
		"anthropic": "key_missing",
		"gemini":    "up",
		"groq":      "unknown",
		"openai":    "down",
	}
	gotIDs := []string{}
	for _, raw := range list {
		entry, _ := raw.(map[string]any)
		id, _ := entry["id"].(string)
		state, _ := entry["state"].(string)
		gotIDs = append(gotIDs, id)
		if want[id] != state {
			t.Fatalf("provider %q: state=%q want %q", id, state, want[id])
		}
	}
	for i := 1; i < len(gotIDs); i++ {
		if gotIDs[i-1] >= gotIDs[i] {
			t.Fatalf("ids not sorted: %v", gotIDs)
		}
	}
}

func TestLogsDerive_chimeraBrokerProviderHealthList_catalogLiveness(t *testing.T) {
	vm := goja.New()
	evalJS(t, vm, settingsUIPath(t, "testing", "loader.js"))
	evalJS(t, vm, settingsUIPath(t, "derive", "chimeraBrokerMetrics.js"))

	derive := vm.Get("ChimeraSettings").ToObject(vm).Get("Derive").ToObject(vm)
	fn, ok := goja.AssertFunction(derive.Get("chimeraBrokerProviderHealthList"))
	if !ok {
		t.Fatal("missing chimeraBrokerProviderHealthList")
	}

	freshAt := time.Now().UTC().Format(time.RFC3339)
	arr := []map[string]any{
		{"parsed": map[string]any{"rawFlat": map[string]any{"msg": "chimera-broker.provider.loaded", "service": "chimera-broker", "provider_id": "groq"}}},
		{"parsed": map[string]any{"rawFlat": map[string]any{"msg": "chimera-broker.provider.loaded", "service": "chimera-broker", "provider_id": "ollama"}}},
		{"parsed": map[string]any{"rawFlat": map[string]any{
			"msg":                 "chat.chimera-broker.available_models",
			"service":             "gateway",
			"ok":                  true,
			"fetched_at":          freshAt,
			"providers":           []any{"groq", "gemini"},
			"catalog_model_count": 3,
		}}},
	}
	v, err := fn(goja.Undefined(), vm.ToValue(arr), goja.Undefined())
	if err != nil {
		t.Fatal(err)
	}
	list, ok := v.Export().([]any)
	if !ok {
		t.Fatalf("expected []any, got %T", v.Export())
	}
	want := map[string]string{"groq": "up", "ollama": "down"}
	if len(list) != len(want) {
		t.Fatalf("len=%d want %d", len(list), len(want))
	}
	for _, raw := range list {
		entry, _ := raw.(map[string]any)
		id, _ := entry["id"].(string)
		state, _ := entry["state"].(string)
		if want[id] != state {
			t.Fatalf("provider %q: state=%q want %q", id, state, want[id])
		}
	}
}

func TestLogsDerive_chimeraBrokerProviderHealthList_modelDiscoveryOverridesCatalog(t *testing.T) {
	vm := goja.New()
	evalJS(t, vm, settingsUIPath(t, "testing", "loader.js"))
	evalJS(t, vm, settingsUIPath(t, "derive", "chimeraBrokerMetrics.js"))

	fn, ok := goja.AssertFunction(vm.Get("ChimeraSettings").ToObject(vm).Get("Derive").ToObject(vm).Get("chimeraBrokerProviderHealthList"))
	if !ok {
		t.Fatal("missing chimeraBrokerProviderHealthList")
	}

	freshAt := time.Now().UTC().Format(time.RFC3339)
	arr := []map[string]any{
		{"parsed": map[string]any{"rawFlat": map[string]any{"msg": "chimera-broker.provider.loaded", "service": "chimera-broker", "provider_id": "ollama"}}},
		{"parsed": map[string]any{"rawFlat": map[string]any{
			"msg": "chat.chimera-broker.available_models", "service": "gateway", "ok": true,
			"fetched_at": freshAt, "providers": []any{"ollama"},
		}}},
		{"parsed": map[string]any{"rawFlat": map[string]any{
			"msg": "chimera-broker.provider.model_discovery.fail", "service": "chimera-broker", "provider_id": "ollama",
		}}},
	}
	v, err := fn(goja.Undefined(), vm.ToValue(arr), goja.Undefined())
	if err != nil {
		t.Fatal(err)
	}
	list := v.Export().([]any)
	entry := list[0].(map[string]any)
	if entry["state"] != "down" {
		t.Fatalf("state=%v want down after model_discovery.fail", entry["state"])
	}
}

func TestLogsDerive_chimeraBrokerProviderHealthList_catalogFail(t *testing.T) {
	vm := goja.New()
	evalJS(t, vm, settingsUIPath(t, "testing", "loader.js"))
	evalJS(t, vm, settingsUIPath(t, "derive", "chimeraBrokerMetrics.js"))

	fn, ok := goja.AssertFunction(vm.Get("ChimeraSettings").ToObject(vm).Get("Derive").ToObject(vm).Get("chimeraBrokerProviderHealthList"))
	if !ok {
		t.Fatal("missing chimeraBrokerProviderHealthList")
	}

	freshAt := time.Now().UTC().Format(time.RFC3339)
	arr := []map[string]any{
		{"parsed": map[string]any{"rawFlat": map[string]any{"msg": "chimera-broker.provider.loaded", "service": "chimera-broker", "provider_id": "ollama"}}},
		{"parsed": map[string]any{"rawFlat": map[string]any{
			"msg": "chat.chimera-broker.available_models", "service": "gateway", "ok": false,
			"fetched_at": freshAt, "err": "fetch /v1/models failed (status=400)",
		}}},
	}
	v, err := fn(goja.Undefined(), vm.ToValue(arr), goja.Undefined())
	if err != nil {
		t.Fatal(err)
	}
	list := v.Export().([]any)
	if len(list) != 1 {
		t.Fatalf("len=%d want 1", len(list))
	}
	entry := list[0].(map[string]any)
	if entry["state"] != "down" {
		t.Fatalf("state=%v want down after catalog fail", entry["state"])
	}
}

func TestLogsDerive_chimeraBrokerRelayOutcomeBuckets_httpAndErrors(t *testing.T) {
	vm := goja.New()
	evalJS(t, vm, settingsUIPath(t, "testing", "loader.js"))
	evalJS(t, vm, settingsUIPath(t, "derive", "chimeraBrokerMetrics.js"))

	derive := vm.Get("ChimeraSettings").ToObject(vm).Get("Derive").ToObject(vm)
	fn, ok := goja.AssertFunction(derive.Get("chimeraBrokerRelayOutcomeBuckets"))
	if !ok {
		t.Fatal("missing chimeraBrokerRelayOutcomeBuckets")
	}

	arr := []map[string]any{
		{"parsed": map[string]any{"rawFlat": map[string]any{"msg": "chimera-broker.startup.banner", "service": "chimera-broker"}}},
		{"parsed": map[string]any{"rawFlat": map[string]any{"msg": "chimera-broker.ready", "service": "chimera-broker"}}},
		{"parsed": map[string]any{"rawFlat": map[string]any{"msg": "chat.chimera-broker.request", "upstreamModel": "groq/x"}}},
		{"parsed": map[string]any{"rawFlat": map[string]any{"msg": "chat.chimera-broker.response", "statusCode": 200}}},
		{"parsed": map[string]any{"rawFlat": map[string]any{"msg": "chat.chimera-broker.request", "upstreamModel": "groq/x"}}},
		{"parsed": map[string]any{"rawFlat": map[string]any{"msg": "chat.chimera-broker.response", "statusCode": 429}}},
		{"parsed": map[string]any{"rawFlat": map[string]any{"msg": "chat.chimera-broker.request", "upstreamModel": "groq/x"}}},
		{"parsed": map[string]any{"rawFlat": map[string]any{"msg": "chat.chimera-broker.response", "statusCode": 401}}},
		{"parsed": map[string]any{"rawFlat": map[string]any{"msg": "chat.chimera-broker.request", "upstreamModel": "groq/x"}}},
		{"parsed": map[string]any{"rawFlat": map[string]any{"msg": "chat.chimera-broker.response", "statusCode": 503}}},
		{"parsed": map[string]any{"rawFlat": map[string]any{"msg": "chat.chimera-broker.request", "upstreamModel": "groq/x"}}},
		{"parsed": map[string]any{"rawFlat": map[string]any{"msg": "chat.chimera-broker.error", "err": "connection reset"}}},
		{"parsed": map[string]any{"rawFlat": map[string]any{"msg": "chat.chimera-broker.request", "upstreamModel": "groq/x"}}},
		{"parsed": map[string]any{"rawFlat": map[string]any{"msg": "chimera-broker.rate_limit", "service": "chimera-broker", "http_status": 429}}},
	}
	v, err := fn(goja.Undefined(), vm.ToValue(arr), goja.Undefined())
	if err != nil {
		t.Fatal(err)
	}
	o := v.ToObject(vm)
	expect := map[string]int64{
		"ok":          1,
		"redirect":    0,
		"rateLimit":   1,
		"clientErr":   1,
		"serverErr":   1,
		"errorNoResp": 1,
		"inFlight":    1,
		"requestN":    6,
		"responseN":   4,
		"total":       6,
	}
	for k, want := range expect {
		got, _ := o.Get(k).Export().(int64)
		if got != want {
			t.Fatalf("%s=%d want %d", k, got, want)
		}
	}
}

func TestLogsDerive_chimeraBrokerCardMetrics_catalogModelCount_gatewayLine(t *testing.T) {
	vm := goja.New()
	evalJS(t, vm, settingsUIPath(t, "testing", "loader.js"))
	evalJS(t, vm, settingsUIPath(t, "derive", "chimeraBrokerMetrics.js"))
	metricFn, ok := goja.AssertFunction(vm.Get("ChimeraSettings").ToObject(vm).Get("Derive").ToObject(vm).Get("chimeraBrokerCardMetrics"))
	if !ok {
		t.Fatal("missing chimeraBrokerCardMetrics")
	}
	arr := []map[string]any{
		{"text": "", "parsed": map[string]any{"rawFlat": map[string]any{
			"msg": "chat.chimera-broker.available_models", "service": "gateway", "catalog_model_count": 42,
		}}},
	}
	v, err := metricFn(goja.Undefined(), vm.ToValue(arr), goja.Undefined())
	if err != nil {
		t.Fatal(err)
	}
	obj := v.ToObject(vm)
	if obj.Get("catalogModelCount").Export() != int64(42) {
		t.Fatalf("catalogModelCount=%v", obj.Get("catalogModelCount").Export())
	}
}

func TestLogsDerive_vectorstoreCollection_nameGolden(t *testing.T) {
	vm := goja.New()
	evalJS(t, vm, settingsUIPath(t, "testing", "loader.js"))
	evalJS(t, vm, settingsUIPath(t, "derive", "sha1.js"))
	evalJS(t, vm, settingsUIPath(t, "derive", "vectorstoreCollection.js"))

	derive := vm.Get("ChimeraSettings").ToObject(vm).Get("Derive").ToObject(vm)
	fn, ok := goja.AssertFunction(derive.Get("vectorstoreCollectionName"))
	if !ok {
		t.Fatal("missing vectorstoreCollectionName")
	}
	v, err := fn(goja.Undefined(), vm.ToValue("default"), vm.ToValue("assistants"), vm.ToValue(""))
	if err != nil {
		t.Fatal(err)
	}
	if v.String() != "chimera-default-assistants-_-be0adbc3" {
		t.Fatalf("got %q", v.String())
	}
	v2, err := fn(goja.Undefined(), vm.ToValue("default"), vm.ToValue("clone"), vm.ToValue(""))
	if err != nil {
		t.Fatal(err)
	}
	if v2.String() != "chimera-default-clone-_-0ef54bfe" {
		t.Fatalf("clone got %q", v2.String())
	}
}

func TestLogsDerive_vectorstoreOperatorLine_forEventLog(t *testing.T) {
	vm := goja.New()
	evalJS(t, vm, settingsUIPath(t, "testing", "loader.js"))
	evalJS(t, vm, settingsUIPath(t, "operator_copy.js"))
	evalJS(t, vm, settingsUIPath(t, "render", "operatorMessage.js"))
	evalJS(t, vm, settingsUIPath(t, "render", "operatorMessageServices.js"))
	evalJS(t, vm, settingsUIPath(t, "derive", "vectorstoreCollection.js"))

	derive := vm.Get("ChimeraSettings").ToObject(vm).Get("Derive").ToObject(vm)
	fn, ok := goja.AssertFunction(derive.Get("vectorstoreOperatorLine"))
	if !ok {
		t.Fatal("missing vectorstoreOperatorLine")
	}
	flat := map[string]any{
		"msg":         "qdrant.http.collection_meta",
		"collection":  "lynn:rimworld",
		"http_status": 200,
	}
	v, err := fn(goja.Undefined(), vm.ToValue(flat), goja.Undefined(), goja.Undefined())
	if err != nil {
		t.Fatal(err)
	}
	if v.String() != "Reading collection lynn:rimworld · 200" {
		t.Fatalf("default got %q", v.String())
	}
	opts := map[string]any{"forEventLog": true}
	v2, err := fn(goja.Undefined(), vm.ToValue(flat), goja.Undefined(), vm.ToValue(opts))
	if err != nil {
		t.Fatal(err)
	}
	if v2.String() != "Reading collection lynn:rimworld" {
		t.Fatalf("forEventLog got %q", v2.String())
	}

	vVer, err := fn(goja.Undefined(), vm.ToValue(map[string]any{
		"msg":            "qdrant.version",
		"qdrant_version": "1.13.0",
	}), goja.Undefined(), goja.Undefined())
	if err != nil {
		t.Fatal(err)
	}
	if vVer.String() != "Component: chimera-vectorstore · Backend: Qdrant 1.13.0" {
		t.Fatalf("version line got %q", vVer.String())
	}
}

func TestLogsDerive_vectorstoreRag_rollups(t *testing.T) {
	vm := goja.New()
	evalJS(t, vm, settingsUIPath(t, "testing", "loader.js"))
	evalJS(t, vm, settingsUIPath(t, "derive", "vectorstoreRagMetrics.js"))

	chimera := vm.Get("ChimeraSettings").ToObject(vm)
	derive := chimera.Get("Derive").ToObject(vm)

	ragFn, ok := goja.AssertFunction(derive.Get("rollupGatewayRagPipeline"))
	if !ok {
		t.Fatal("missing rollupGatewayRagPipeline")
	}
	qFn, ok := goja.AssertFunction(derive.Get("vectorstoreHttpPathRollup"))
	if !ok {
		t.Fatal("missing vectorstoreHttpPathRollup")
	}

	entries := []map[string]any{
		{"parsed": map[string]any{"rawFlat": map[string]any{"msg": "rag.query"}}},
		{"parsed": map[string]any{"rawFlat": map[string]any{"msg": "rag.embed", "elapsed_ms": 12}}},
		{"parsed": map[string]any{"rawFlat": map[string]any{"msg": "rag.embed", "elapsedMs": 8}}},
		{"parsed": map[string]any{"rawFlat": map[string]any{"msg": "rag.hit"}}},
	}
	rv, err := ragFn(goja.Undefined(), vm.ToValue(entries), goja.Undefined())
	if err != nil {
		t.Fatal(err)
	}
	robj := rv.ToObject(vm)
	if robj.Get("ragQuery").Export() != int64(1) {
		t.Fatalf("ragQuery=%v", robj.Get("ragQuery").Export())
	}
	if robj.Get("ragEmbed").Export() != int64(2) {
		t.Fatalf("ragEmbed=%v", robj.Get("ragEmbed").Export())
	}
	if robj.Get("ragHitLines").Export() != int64(1) {
		t.Fatalf("ragHitLines=%v", robj.Get("ragHitLines").Export())
	}
	if robj.Get("embedMsSum").Export() != int64(20) {
		t.Fatalf("embedMsSum=%v", robj.Get("embedMsSum").Export())
	}

	arr := []map[string]any{
		{"parsed": map[string]any{"shape": "http.access", "rawFlat": map[string]any{"path": "/collections/x/points/search", "method": "POST"}}},
		{"parsed": map[string]any{"shape": "http.access", "rawFlat": map[string]any{"path": "/collections/x/points/scroll", "method": "POST"}}},
		{"parsed": map[string]any{"shape": "http.access", "rawFlat": map[string]any{"path": "/collections/x/points", "method": "PUT"}}},
	}
	qv, err := qFn(goja.Undefined(), vm.ToValue(arr), goja.Undefined())
	if err != nil {
		t.Fatal(err)
	}
	qobj := qv.ToObject(vm)
	if qobj.Get("searchN").Export() != int64(1) {
		t.Fatalf("searchN=%v", qobj.Get("searchN").Export())
	}
	if qobj.Get("scrollN").Export() != int64(1) {
		t.Fatalf("scrollN=%v", qobj.Get("scrollN").Export())
	}
	if qobj.Get("upsertN").Export() != int64(1) {
		t.Fatalf("upsertN=%v", qobj.Get("upsertN").Export())
	}
}

func TestLogsDerive_indexer_collectIndexerRunMeta_countsAndVectors(t *testing.T) {
	vm := goja.New()
	evalJS(t, vm, settingsUIPath(t, "testing", "loader.js"))
	evalJS(t, vm, settingsUIPath(t, "derive", "indexerMetrics.js"))

	chimera := vm.Get("ChimeraSettings").ToObject(vm)
	derive := chimera.Get("Derive").ToObject(vm)
	fn, ok := goja.AssertFunction(derive.Get("collectIndexerRunMeta"))
	if !ok {
		t.Fatal("missing collectIndexerRunMeta")
	}

	evs := []map[string]any{
		{"parsed": map[string]any{"rawFlat": map[string]any{"msg": "indexer.run.start", "service": "indexer", "scope_project_id": "p1", "root_ids": "r1", "roots": 1}}},
		{"parsed": map[string]any{"rawFlat": map[string]any{"msg": "indexer.job.ingested", "service": "indexer", "chunks": 3}}},
		{"parsed": map[string]any{"rawFlat": map[string]any{"msg": "indexer.job.ingested", "service": "indexer", "chunks": 2}}},
		{"parsed": map[string]any{"rawFlat": map[string]any{"msg": "indexer.job.failed", "service": "indexer"}}},
		{"parsed": map[string]any{"rawFlat": map[string]any{"msg": "indexer.run.done", "service": "indexer", "ingest_completed": 10, "ingest_failed_dropped": 4}}},
	}
	opts := map[string]any{
		"tokenLabelByTenant": map[string]any{"t1": "Tenant One"},
		"getFlat": func(call goja.FunctionCall) goja.Value {
			p := call.Argument(0).ToObject(vm)
			return p.Get("rawFlat")
		},
	}

	v, err := fn(goja.Undefined(), vm.ToValue("run-1"), vm.ToValue(evs), vm.ToValue(opts))
	if err != nil {
		t.Fatal(err)
	}
	obj := v.ToObject(vm)
	if obj.Get("vectorsStored").Export() != int64(5) {
		t.Fatalf("vectorsStored=%v", obj.Get("vectorsStored").Export())
	}
	// doneFlat present -> use ingest_completed/ingest_failed_dropped.
	if obj.Get("okCount").Export() != int64(10) {
		t.Fatalf("okCount=%v", obj.Get("okCount").Export())
	}
	if obj.Get("failCount").Export() != int64(4) {
		t.Fatalf("failCount=%v", obj.Get("failCount").Export())
	}
}

func loadGatewayCardCtx(t *testing.T, vm *goja.Runtime) {
	t.Helper()
	evalJS(t, vm, settingsUIPath(t, "testing", "loader.js"))
	evalJS(t, vm, settingsUIPath(t, "operator_copy.js"))
	evalJS(t, vm, settingsUIPath(t, "derive", "gatewayCardModel.js"))
}

func TestLogsDerive_gatewayCardModel_kvCountersHideRow(t *testing.T) {
	vm := goja.New()
	loadGatewayCardCtx(t, vm)

	derive := vm.Get("ChimeraSettings").ToObject(vm).Get("Derive").ToObject(vm)
	modelFn, ok := goja.AssertFunction(derive.Get("gatewayCardModel"))
	if !ok {
		t.Fatal("missing gatewayCardModel")
	}
	hideFn, ok := goja.AssertFunction(derive.Get("gatewayPanelHideRow"))
	if !ok {
		t.Fatal("missing gatewayPanelHideRow")
	}

	arr := []map[string]any{
		{"parsed": map[string]any{
			"shape":   "http.access",
			"rawFlat": map[string]any{"msg": "gateway.http.access", "path": "/health", "statusCode": 200, "method": "GET"},
		}},
		{"parsed": map[string]any{
			"shape":   "http.access",
			"rawFlat": map[string]any{"msg": "gateway.http.access", "path": "/v1/chat", "statusCode": 200, "method": "POST"},
		}},
		{"parsed": map[string]any{"rawFlat": map[string]any{
			"msg": "gateway.startup.listening", "addr": ":8080", "broker": "http://bifrost", "chimera_broker_data": "/data/b",
			"vectorstore_supervised": true, "indexer_supervised": true, "config": "/x/gateway.yaml",
		}}},
		{"parsed": map[string]any{"rawFlat": map[string]any{"msg": "gateway.auth.reloaded", "count": 2}}},
		{"parsed": map[string]any{"rawFlat": map[string]any{"msg": "routing.policy.reloaded", "rules": 5}}},
		{"parsed": map[string]any{"rawFlat": map[string]any{"msg": "chat.request"}}},
		{"parsed": map[string]any{"rawFlat": map[string]any{"msg": "chat.chimera-broker.response", "statusCode": 200}}},
		{"parsed": map[string]any{"rawFlat": map[string]any{"msg": "rag.query"}}},
		{"parsed": map[string]any{"rawFlat": map[string]any{"msg": "rag.hit"}}},
		{"parsed": map[string]any{"rawFlat": map[string]any{"msg": "ingest.complete"}}},
		{"parsed": map[string]any{"rawFlat": map[string]any{"msg": "ingest.failed"}}},
	}

	v, err := modelFn(goja.Undefined(), vm.ToValue(arr), goja.Undefined())
	if err != nil {
		t.Fatal(err)
	}
	o := v.ToObject(vm)
	if o.Get("subtitle").String() != "Client credentials reloaded" {
		t.Fatalf("subtitle=%q", o.Get("subtitle").String())
	}
	kv := o.Get("kv").ToObject(vm)
	if kv.Get("listening").String() != ":8080" {
		t.Fatalf("listening=%q", kv.Get("listening").String())
	}
	if kv.Get("broker").String() != "http://bifrost" {
		t.Fatalf("broker=%q", kv.Get("broker").String())
	}
	if kv.Get("apiKeys").String() != "2" {
		t.Fatalf("apiKeys=%q", kv.Get("apiKeys").String())
	}
	if kv.Get("routingRules").String() != "5" {
		t.Fatalf("routingRules=%q", kv.Get("routingRules").String())
	}
	sup := kv.Get("supervised").String()
	if !strings.Contains(sup, "chimera-vectorstore") || !strings.Contains(sup, "chimera-broker") {
		t.Fatalf("supervised=%q", sup)
	}

	co := o.Get("counters").ToObject(vm)
	if co.Get("http2xx").Export().(int64) != 2 {
		t.Fatalf("http2xx=%v", co.Get("http2xx").Export())
	}
	if co.Get("chatReq").Export().(int64) != 1 {
		t.Fatalf("chatReq=%v", co.Get("chatReq").Export())
	}
	if co.Get("ragQuery").Export().(int64) != 1 || co.Get("ragHit").Export().(int64) != 1 {
		t.Fatalf("ragQuery/Hit=%v/%v", co.Get("ragQuery").Export(), co.Get("ragHit").Export())
	}
	if co.Get("ingestOk").Export().(int64) != 1 || co.Get("ingestFail").Export().(int64) != 1 {
		t.Fatalf("ingest=%v/%v", co.Get("ingestOk").Export(), co.Get("ingestFail").Export())
	}

	for _, tc := range []struct {
		path string
		hide bool
	}{
		{"/healthz", true},
		{"/readyz", true},
		{"/api/ui/logs", true},
		{"/ui/settings", true},
		{"/ui/logs", false},
	} {
		ent := map[string]any{
			"parsed": map[string]any{"shape": "http.access", "rawFlat": map[string]any{"path": tc.path, "statusCode": 200}},
		}
		vh, err := hideFn(goja.Undefined(), vm.ToValue(ent), goja.Undefined())
		if err != nil {
			t.Fatal(err)
		}
		if vh.Export().(bool) != tc.hide {
			t.Fatalf("gatewayPanelHideRow path=%q hide=%v want %v", tc.path, vh.Export(), tc.hide)
		}
	}
	ent2 := map[string]any{
		"parsed": map[string]any{"shape": "http.access", "rawFlat": map[string]any{"path": "/other", "statusCode": 200}},
	}
	vh2, err := hideFn(goja.Undefined(), vm.ToValue(ent2), goja.Undefined())
	if err != nil {
		t.Fatal(err)
	}
	if vh2.Export().(bool) != false {
		t.Fatalf("hide non-probe=%v", vh2.Export())
	}
}

func TestLogsDerive_gatewayUsage_gatewayUsageCardModel(t *testing.T) {
	vm := goja.New()
	evalJS(t, vm, settingsUIPath(t, "testing", "loader.js"))
	evalJS(t, vm, settingsUIPath(t, "derive", "gatewayUsageMetrics.js"))

	chimera := vm.Get("ChimeraSettings").ToObject(vm)
	derive := chimera.Get("Derive").ToObject(vm)
	fn, ok := goja.AssertFunction(derive.Get("gatewayUsageCardModel"))
	if !ok {
		t.Fatal("missing gatewayUsageCardModel")
	}

	data := map[string]any{
		"metrics_store_open": true,
		"current_minute_utc": "2026-05-05T12:34",
		"current_day_utc":    "2026-05-05",
		"recent_events":      []map[string]any{{"model_id": "groq/llama3"}},
		"minute_rollups":     []map[string]any{{"provider": "x", "model_id": "m", "status": "200", "calls": 2, "est_tokens": 100}},
		"day_rollups":        []map[string]any{{"provider": "x", "model_id": "m", "status": "200", "calls": 3, "est_tokens": 500}},
	}

	agg := func(call goja.FunctionCall) goja.Value {
		rows := call.Argument(0).Export()
		// pretend there is exactly 1 distinct model and tokens=999 for any input
		_ = rows
		return vm.ToValue(map[string]any{"models": 1, "tokens": 999})
	}
	shortLabel := func(call goja.FunctionCall) goja.Value {
		return vm.ToValue("short:" + call.Argument(0).String())
	}

	v, err := fn(goja.Undefined(), vm.ToValue(data), vm.ToValue(agg), vm.ToValue(shortLabel))
	if err != nil {
		t.Fatal(err)
	}
	obj := v.ToObject(vm)
	if obj.Get("loading").Export() != false {
		t.Fatalf("loading=%v", obj.Get("loading").Export())
	}
	if obj.Get("storeOpen").Export() != true {
		t.Fatalf("storeOpen=%v", obj.Get("storeOpen").Export())
	}
	if obj.Get("lastModelLabel").String() != "short:groq/llama3" {
		t.Fatalf("lastModelLabel=%v", obj.Get("lastModelLabel").Export())
	}
	minAgg := obj.Get("minAgg").ToObject(vm)
	if minAgg.Get("tokens").Export() != int64(999) {
		t.Fatalf("minAgg.tokens=%v", minAgg.Get("tokens").Export())
	}
}

func TestLogsDerive_indexerPresent_histogramBucket(t *testing.T) {
	vm := goja.New()
	evalJS(t, vm, settingsUIPath(t, "testing", "loader.js"))
	evalJS(t, vm, settingsUIPath(t, "derive", "indexerPresent.js"))
	derive := vm.Get("ChimeraSettings").ToObject(vm).Get("Derive").ToObject(vm)
	bucket, ok := goja.AssertFunction(derive.Get("indexerSlugHistogramBucket"))
	if !ok {
		t.Fatal("missing indexerSlugHistogramBucket")
	}
	tests := []struct {
		msg, want string
	}{
		{"indexer.state", "statestats"},
		{"indexer.storage.stats", "statestats"},
		{"indexer.job.upload", "jobs"},
		{"indexer.queue.snapshot", "queue"},
		{"indexer.discovery.summary", "discovery"},
		{"indexer.run.start", "lifecycle"},
		{"gateway.indexer.config", "config"},
		{"indexer.retry.scheduled", "recovery"},
		{"indexer.scope.status", "statestats"},
		{"telemetry.other", "other"},
	}
	for _, tc := range tests {
		v, err := bucket(goja.Undefined(), vm.ToValue(tc.msg))
		if err != nil {
			t.Fatalf("%q: %v", tc.msg, err)
		}
		if v.String() != tc.want {
			t.Fatalf("%q: got %q want %q", tc.msg, v.String(), tc.want)
		}
	}
}

func loadIndexerPresentCtx(t *testing.T, vm *goja.Runtime) {
	t.Helper()
	evalJS(t, vm, settingsUIPath(t, "testing", "loader.js"))
	evalJS(t, vm, settingsUIPath(t, "operator_copy.js"))
	evalJS(t, vm, settingsUIPath(t, "render", "operatorMessage.js"))
	evalJS(t, vm, settingsUIPath(t, "render", "operatorMessageServices.js"))
	evalJS(t, vm, settingsUIPath(t, "render", "operatorMessageIndexer.js"))
	evalJS(t, vm, settingsUIPath(t, "derive", "indexerPresent.js"))
}

func TestLogsDerive_indexerPresent_proseStateAndStats(t *testing.T) {
	vm := goja.New()
	loadIndexerPresentCtx(t, vm)
	derive := vm.Get("ChimeraSettings").ToObject(vm).Get("Derive").ToObject(vm)
	proseFn, ok := goja.AssertFunction(derive.Get("indexerProseSummary"))
	if !ok {
		t.Fatal("missing indexerProseSummary")
	}
	labelFn, ok := goja.AssertFunction(derive.Get("indexerDeclaredStateLabel"))
	if !ok {
		t.Fatal("missing indexerDeclaredStateLabel")
	}
	v, err := labelFn(goja.Undefined(), vm.ToValue("watch_idle"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(v.String(), "Waiting for file changes") {
		t.Fatalf("label: %q", v.String())
	}
	stateFlat := map[string]any{"service": "indexer", "msg": "indexer.state", "state": "watch_idle", "queue_depth": 0, "qdrant_points_reported": 42}
	ps, err := proseFn(goja.Undefined(), vm.ToValue(stateFlat))
	if err != nil {
		t.Fatal(err)
	}
	psStr := ps.String()
	if !strings.Contains(psStr, "Waiting for file changes") || !strings.Contains(psStr, "queue depth 0") {
		t.Fatalf("state prose: %q", psStr)
	}
	statsFlat := map[string]any{
		"service": "indexer", "msg": "indexer.storage.stats",
		"qdrant_points": 120, "available": true, "collection": "c-test", "ingest_project": "task-orchestrator",
	}
	pg, err := proseFn(goja.Undefined(), vm.ToValue(statsFlat))
	if err != nil {
		t.Fatal(err)
	}
	g := pg.String()
	if !strings.Contains(g, "120") || !strings.Contains(g, "Stored search index looks healthy") ||
		!strings.Contains(g, "saved and ready for retrieval") {
		t.Fatalf("stats prose: %q", g)
	}
	scopeFlat := map[string]any{
		"service": "indexer", "msg": "indexer.scope.status",
		"workspace_files_total": 12, "queue_ingest_pending": 3, "queue_fanout_files_pending": 4,
	}
	ps2, err := proseFn(goja.Undefined(), vm.ToValue(scopeFlat))
	if err != nil {
		t.Fatal(err)
	}
	ps2Str := ps2.String()
	if !strings.Contains(ps2Str, "12") || !strings.Contains(ps2Str, "waiting to be examined") ||
		!strings.Contains(ps2Str, "waiting to be embedded") || strings.Contains(ps2Str, "Pipeline") {
		t.Fatalf("scope status prose: %q", ps2Str)
	}
	skipFlat := map[string]any{
		"service": "indexer", "msg": "indexer.job.skipped.summary",
		"files_evaluated": 1476, "skip_unchanged_local_sync": 1476,
		"ingest_succeeded": 0, "window_ms": 5000, "ingest_project": "task-orchestrator",
	}
	psSkip, err := proseFn(goja.Undefined(), vm.ToValue(skipFlat))
	if err != nil {
		t.Fatal(err)
	}
	psSkipStr := psSkip.String()
	if !strings.Contains(psSkipStr, "everything was already indexed") || !strings.Contains(psSkipStr, "1476") ||
		strings.Contains(psSkipStr, "task-orchestrator") || !strings.Contains(psSkipStr, "5 seconds") {
		t.Fatalf("skipped summary prose: %q", psSkipStr)
	}
	skipNewFlat := map[string]any{
		"service": "indexer", "msg": "indexer.job.skipped.summary",
		"files_evaluated": 9, "skip_unchanged_local_sync": 0,
		"ingest_succeeded": 9, "window_ms": 5000,
	}
	psSkipNew, err := proseFn(goja.Undefined(), vm.ToValue(skipNewFlat))
	if err != nil {
		t.Fatal(err)
	}
	psSkipNewStr := psSkipNew.String()
	if !strings.Contains(psSkipNewStr, "embedded 9 new file") || strings.Contains(psSkipNewStr, "0 unchanged") {
		t.Fatalf("skipped summary new embed prose: %q", psSkipNewStr)
	}
	hotFlat := map[string]any{"service": "indexer", "msg": "indexer.supervised.hot_reload", "n": 2}
	psHot, err := proseFn(goja.Undefined(), vm.ToValue(hotFlat))
	if err != nil {
		t.Fatal(err)
	}
	psHotStr := psHot.String()
	if !strings.Contains(psHotStr, "Indexer settings changed") || !strings.Contains(psHotStr, "restarting the watch session") {
		t.Fatalf("hot reload prose: %q", psHotStr)
	}
	queueFlat := map[string]any{
		"service": "indexer", "msg": "indexer.queue.snapshot",
		"queue_depth": 0, "ingest_completed": 0, "phase": "worker_drain_tick",
	}
	pq, err := proseFn(goja.Undefined(), vm.ToValue(queueFlat))
	if err != nil {
		t.Fatal(err)
	}
	pqStr := pq.String()
	if !strings.Contains(pqStr, "Ingest workers idle") || !strings.Contains(pqStr, "embed queue") {
		t.Fatalf("queue snapshot prose: %q", pqStr)
	}
	scanFlat := map[string]any{
		"service": "indexer", "msg": "indexer.run.progress",
		"phase": "scan_scheduled", "roots": 6, "ingest_project": "task-orchestrator",
	}
	psScan, err := proseFn(goja.Undefined(), vm.ToValue(scanFlat))
	if err != nil {
		t.Fatal(err)
	}
	psScanStr := psScan.String()
	if !strings.Contains(psScanStr, "Starting a scan of") || !strings.Contains(psScanStr, "6 directories for files") {
		t.Fatalf("scan_scheduled prose: %q", psScanStr)
	}
	discFlat := map[string]any{
		"service": "indexer", "msg": "indexer.discovery.summary.scope",
		"candidates_discovered": 1489, "ingest_project": "task-orchestrator", "flavor_id": "default",
	}
	psDisc, err := proseFn(goja.Undefined(), vm.ToValue(discFlat))
	if err != nil {
		t.Fatal(err)
	}
	psDiscStr := psDisc.String()
	if !strings.Contains(psDiscStr, "1489") || !strings.Contains(psDiscStr, "workspace's directories") ||
		strings.Contains(psDiscStr, "default") || strings.Contains(psDiscStr, "watch root") {
		t.Fatalf("discovery scope prose: %q", psDiscStr)
	}
	scanCompleteFlat := map[string]any{
		"service": "indexer", "msg": "indexer.scan.complete",
		"n_scopes": 3, "per_scope_fanout_budget": 50, "queue_cap": 200,
	}
	psFan, err := proseFn(goja.Undefined(), vm.ToValue(scanCompleteFlat))
	if err != nil {
		t.Fatal(err)
	}
	psFanStr := psFan.String()
	if !strings.Contains(psFanStr, "pending file slot") || !strings.Contains(psFanStr, "total queue capacity") ||
		strings.Contains(psFanStr, "scope(s)") {
		t.Fatalf("scan complete prose: %q", psFanStr)
	}
	doneFlat := map[string]any{
		"service": "indexer", "msg": "indexer.run.progress",
		"phase": "initial_scan", "candidates_total": 1476, "ingest_project": "task-orchestrator",
	}
	psDone, err := proseFn(goja.Undefined(), vm.ToValue(doneFlat))
	if err != nil {
		t.Fatal(err)
	}
	psDoneStr := psDone.String()
	if !strings.Contains(psDoneStr, "Initial scan finished") || !strings.Contains(psDoneStr, "1476") {
		t.Fatalf("initial_scan prose: %q", psDoneStr)
	}
	startFlat := map[string]any{
		"service": "indexer", "msg": "indexer.run.start",
		"roots": 3, "ingest_project": "task-orchestrator",
		"watch_root_paths": []any{`C:\repo\alpha`, `C:\repo\beta`, `C:\repo\gamma`},
	}
	psStart, err := proseFn(goja.Undefined(), vm.ToValue(startFlat))
	if err != nil {
		t.Fatal(err)
	}
	psStartStr := psStart.String()
	if !strings.Contains(psStartStr, "Watching") || !strings.Contains(psStartStr, "3 directories") ||
		!strings.Contains(psStartStr, "alpha") || strings.Contains(psStartStr, "Indexer is now") {
		t.Fatalf("run.start prose: %q", psStartStr)
	}
	failFlat := map[string]any{
		"service": "indexer", "msg": "indexer.job.failed", "rel": "spotify/track.json",
		"err": `/v1/ingest/session/df9a33cd774ef920acdb8c9562b01394/complete: status 404: {"error":{"message":"unknown or expired session"}}`,
	}
	pf, err := proseFn(goja.Undefined(), vm.ToValue(failFlat))
	if err != nil {
		t.Fatal(err)
	}
	fs := pf.String()
	if strings.Contains(fs, "/v1/ingest/session") {
		t.Fatalf("ingest failure prose should not repeat raw HTTP path: %q", fs)
	}
	if !strings.Contains(fs, "chunked upload session lost") {
		t.Fatalf("ingest failure prose: %q", fs)
	}
	waitFlat := map[string]any{
		"service": "indexer", "msg": "indexer.supervised.wait_roots", "type": "indexer.supervised.wait_roots",
		"config_path": "/tmp/data/gateway/indexer.supervised.yaml",
	}
	pw, err := proseFn(goja.Undefined(), vm.ToValue(waitFlat))
	if err != nil {
		t.Fatal(err)
	}
	pwStr := pw.String()
	if !strings.Contains(pwStr, "Waiting for at least one watch root") || !strings.Contains(pwStr, "indexer.supervised.yaml") {
		t.Fatalf("wait_roots prose: %q", pwStr)
	}
	changedFlat := map[string]any{
		"service": "indexer", "msg": "indexer.supervised.workspaces_changed",
		"roots": 3, "new_paths_hash": "abc123", "watch_root_paths": []any{`C:\repo\alpha`, `C:\repo\beta`, `C:\repo\new`},
	}
	pc, err := proseFn(goja.Undefined(), vm.ToValue(changedFlat))
	if err != nil {
		t.Fatal(err)
	}
	pcStr := pc.String()
	if !strings.Contains(pcStr, "Workspace paths changed") || !strings.Contains(pcStr, "3 watch root") || !strings.Contains(pcStr, "new") {
		t.Fatalf("workspaces_changed prose: %q", pcStr)
	}
}

func TestLogsDerive_collectIndexerRunMeta_scopeStatus(t *testing.T) {
	vm := goja.New()
	loadIndexerPresentCtx(t, vm)
	evalJS(t, vm, settingsUIPath(t, "derive", "indexerMetrics.js"))

	rs := `[{"root_id":"r1","path":"/a","ingest_project":"p1","flavor_id":"","indexer_target_key":"ik_one"}]`
	script := fmt.Sprintf(`
		var evs = [
			{ parsed: { rawFlat: {
				service: "indexer", index_run_id: "runZ", msg: "indexer.run.start",
				root_ids: "r1", watch_root_paths: ["/a"], root_scopes: %s
			}}},
			{ parsed: { rawFlat: {
				service: "indexer", index_run_id: "runZ", msg: "indexer.scope.status",
				indexer_target_key: "ik_one", workspace_files_total: 7,
				queue_ingest_pending: 2, queue_fanout_files_pending: 3, pending_bulk_tier1: 1
			}}}
		];
		function getFlat(p) { return (p && p.rawFlat) || {}; }
		ChimeraSettings.Derive.collectIndexerRunMeta("ik_one", evs, { getFlat: getFlat });
	`, strconv.Quote(rs))
	v, err := vm.RunString(script)
	if err != nil {
		t.Fatal(err)
	}
	o := v.ToObject(vm)
	if n := o.Get("scopeWorkspaceTotal"); n == nil || n.Export() != int64(7) {
		t.Fatalf("scopeWorkspaceTotal=%v", n)
	}
	if n := o.Get("scopeQueueIngestPending"); n == nil || n.Export() != int64(2) {
		t.Fatalf("scopeQueueIngestPending=%v", n)
	}
}

func TestLogsDerive_indexerPresent_groupKey(t *testing.T) {
	vm := goja.New()
	loadIndexerPresentCtx(t, vm)
	derive := vm.Get("ChimeraSettings").ToObject(vm).Get("Derive").ToObject(vm)
	gfn, ok := goja.AssertFunction(derive.Get("indexerGroupKeyFromFlat"))
	if !ok {
		t.Fatal("missing indexerGroupKeyFromFlat")
	}
	vTgt, err := gfn(goja.Undefined(), vm.ToValue(map[string]any{"indexer_target_key": "ik_tgt", "indexer_key": "ik_abc", "index_run_id": "run-99"}))
	if err != nil {
		t.Fatal(err)
	}
	if vTgt.String() != "ik_tgt" {
		t.Fatalf("indexer_target_key priority got %q", vTgt.String())
	}
	v, err := gfn(goja.Undefined(), vm.ToValue(map[string]any{"indexer_key": "ik_abc", "index_run_id": "run-99"}))
	if err != nil {
		t.Fatal(err)
	}
	if v.String() != "ik_abc" {
		t.Fatalf("got %q", v.String())
	}
	v2, err := gfn(goja.Undefined(), vm.ToValue(map[string]any{"index_run_id": "only-run"}))
	if err != nil {
		t.Fatal(err)
	}
	if v2.String() != "only-run" {
		t.Fatalf("fallback got %q", v2.String())
	}
	v3, err := gfn(goja.Undefined(), vm.ToValue(map[string]any{
		"principal_id":        "t1",
		"defaults_project_id": "myproj",
		"defaults_flavor_id":  "f1",
		"index_run_id":        "run-x",
	}))
	if err != nil {
		t.Fatal(err)
	}
	wantIG := "ig\x1et1\x1emyproj\x1ef1"
	if v3.String() != wantIG {
		t.Fatalf("tenant+proj+flav got %q want %q", v3.String(), wantIG)
	}
}

func TestLogsDerive_indexerPartition_humanStartMsgSplitsBuckets(t *testing.T) {
	vm := goja.New()
	loadIndexerPresentCtx(t, vm)
	evalJS(t, vm, settingsUIPath(t, "derive", "indexerPartition.js"))

	if _, err := vm.RunString(`function __getFlat(p) { return (p && p.rawFlat) || {}; }`); err != nil {
		t.Fatal(err)
	}

	rs := `[{"root_id":"r1","path":"/a","ingest_project":"p1","flavor_id":"","indexer_target_key":"ik_one"},{"root_id":"r2","path":"/b","ingest_project":"p2","flavor_id":"","indexer_target_key":"ik_two"}]`
	cache := []any{
		map[string]any{
			"parsed": map[string]any{"rawFlat": map[string]any{
				"service":              "indexer",
				"index_run_id":         "run1",
				"msg":                  "indexer run start",
				"root_ids":             "r1,r2",
				"watch_root_paths":     []any{"/a", "/b"},
				"root_scopes":          rs,
				"indexer_multi_target": true,
			}},
		},
		map[string]any{
			"parsed": map[string]any{"rawFlat": map[string]any{
				"service":              "indexer",
				"index_run_id":         "run1",
				"msg":                  "indexer.queue.snapshot",
				"indexer_multi_target": true,
			}},
		},
	}

	obj := vm.Get("ChimeraSettings").ToObject(vm).Get("Derive").ToObject(vm)
	bfn, ok := goja.AssertFunction(obj.Get("indexerBucketsFromCache"))
	if !ok {
		t.Fatal("missing indexerBucketsFromCache")
	}
	getFlat := vm.Get("__getFlat")
	v, err := bfn(goja.Undefined(), vm.ToValue(cache), getFlat)
	if err != nil {
		t.Fatal(err)
	}
	o := v.ToObject(vm)
	got := o.Get("buckets").ToObject(vm)
	if got == nil || goja.IsUndefined(got) {
		t.Fatal("missing buckets")
	}
	if got.Get("ik_one") == nil || got.Get("ik_two") == nil {
		t.Fatalf("want ik_one and ik_two buckets, buckets=%v", got)
	}
}

func TestLogsDerive_indexerPartition_queueSnapshotExcludedFromWorkspaceBuckets(t *testing.T) {
	vm := goja.New()
	loadIndexerPresentCtx(t, vm)
	evalJS(t, vm, settingsUIPath(t, "derive", "indexerPartition.js"))

	if _, err := vm.RunString(`function __getFlat(p) { return (p && p.rawFlat) || {}; }`); err != nil {
		t.Fatal(err)
	}

	rs := `[{"root_id":"r1","path":"/a","ingest_project":"p1","flavor_id":"","indexer_target_key":"ik_one"}]`
	cache := []any{
		map[string]any{
			"parsed": map[string]any{"rawFlat": map[string]any{
				"service":          "indexer",
				"index_run_id":     "run1",
				"msg":              "indexer.run.start",
				"root_ids":         "r1",
				"watch_root_paths": []any{"/a"},
				"root_scopes":      rs,
			}},
		},
		map[string]any{
			"parsed": map[string]any{"rawFlat": map[string]any{
				"service":                 "indexer",
				"index_run_id":            "run1",
				"msg":                     "indexer.scan.complete",
				"n_scopes":                3,
				"per_scope_fanout_budget": 50,
				"queue_cap":               200,
			}},
		},
		map[string]any{
			"parsed": map[string]any{"rawFlat": map[string]any{
				"service":          "indexer",
				"index_run_id":     "run1",
				"msg":              "indexer.queue.snapshot",
				"queue_depth":      0,
				"phase":            "worker_drain_tick",
				"ingest_completed": 0,
			}},
		},
	}

	obj := vm.Get("ChimeraSettings").ToObject(vm).Get("Derive").ToObject(vm)
	bfn, ok := goja.AssertFunction(obj.Get("indexerBucketsFromCache"))
	if !ok {
		t.Fatal("missing indexerBucketsFromCache")
	}
	v, err := bfn(goja.Undefined(), vm.ToValue(cache), vm.Get("__getFlat"))
	if err != nil {
		t.Fatal(err)
	}
	got := v.ToObject(vm).Get("buckets").ToObject(vm)
	one := got.Get("ik_one")
	if one == nil || goja.IsUndefined(one) {
		t.Fatal("missing ik_one bucket")
	}
	arr, ok := one.Export().([]any)
	if !ok {
		t.Fatalf("ik_one bucket type %T", one.Export())
	}
	if len(arr) != 1 {
		t.Fatalf("ik_one should only contain run.start, got %d events", len(arr))
	}
}

func TestLogsDerive_indexerFlatOmitFromWorkspaceScopedLog(t *testing.T) {
	vm := goja.New()
	evalJS(t, vm, settingsUIPath(t, "derive", "indexerPartition.js"))
	obj := vm.Get("ChimeraSettings").ToObject(vm).Get("Derive").ToObject(vm)
	fn, ok := goja.AssertFunction(obj.Get("indexerFlatOmitFromWorkspaceScopedLog"))
	if !ok {
		t.Fatal("missing indexerFlatOmitFromWorkspaceScopedLog")
	}
	cases := []struct {
		flat map[string]any
		want bool
	}{
		{map[string]any{"msg": "indexer.queue.snapshot", "queue_depth": 0}, true},
		{map[string]any{"msg": "indexer.scope.status", "change_reason": "heartbeat"}, true},
		{map[string]any{"msg": "indexer.scope.status", "change_reason": "phase"}, false},
		{map[string]any{"msg": "indexer.job.ingested", "rel": "a.go"}, false},
	}
	for _, tc := range cases {
		v, err := fn(goja.Undefined(), vm.ToValue(tc.flat))
		if err != nil {
			t.Fatal(err)
		}
		if v.ToBoolean() != tc.want {
			t.Fatalf("flat=%v want omit=%v got %v", tc.flat, tc.want, v.ToBoolean())
		}
	}
}

func TestLogsDerive_indexerPartition_sourceOnlyNoServiceField(t *testing.T) {
	vm := goja.New()
	loadIndexerPresentCtx(t, vm)
	evalJS(t, vm, settingsUIPath(t, "derive", "indexerPartition.js"))

	if _, err := vm.RunString(`function __getFlat(p) { return (p && p.rawFlat) || {}; }`); err != nil {
		t.Fatal(err)
	}

	rs := `[{"root_id":"r1","path":"/a","ingest_project":"p1","flavor_id":"","indexer_target_key":"ik_one"},{"root_id":"r2","path":"/b","ingest_project":"p2","flavor_id":"","indexer_target_key":"ik_two"}]`
	cache := []any{
		map[string]any{
			"source": "indexer",
			"parsed": map[string]any{"rawFlat": map[string]any{
				"index_run_id":         "run1",
				"msg":                  "indexer run start",
				"root_ids":             "r1,r2",
				"watch_root_paths":     []any{"/a", "/b"},
				"root_scopes":          rs,
				"indexer_multi_target": true,
			}},
		},
	}

	obj := vm.Get("ChimeraSettings").ToObject(vm).Get("Derive").ToObject(vm)
	bfn, ok := goja.AssertFunction(obj.Get("indexerBucketsFromCache"))
	if !ok {
		t.Fatal("missing indexerBucketsFromCache")
	}
	getFlat := vm.Get("__getFlat")
	v, err := bfn(goja.Undefined(), vm.ToValue(cache), getFlat)
	if err != nil {
		t.Fatal(err)
	}
	got := v.ToObject(vm).Get("buckets").ToObject(vm)
	if got.Get("ik_one") == nil || got.Get("ik_two") == nil {
		t.Fatalf("expected buckets with source-only indexer lines, got %+v", got)
	}
}

func TestLogsDerive_indexerPartition_syntheticJobsWhenStartMissing(t *testing.T) {
	vm := goja.New()
	loadIndexerPresentCtx(t, vm)
	evalJS(t, vm, settingsUIPath(t, "derive", "indexerPartition.js"))

	if _, err := vm.RunString(`function __getFlat(p) { return (p && p.rawFlat) || {}; }`); err != nil {
		t.Fatal(err)
	}

	t1 := "tenant-x"
	k1 := "ig\x1e" + t1 + "\x1ep1\x1e"
	k2 := "ig\x1e" + t1 + "\x1ep2\x1e"
	cache := []any{
		map[string]any{
			"source": "indexer",
			"parsed": map[string]any{"rawFlat": map[string]any{
				"service":        "indexer",
				"index_run_id":   "run99",
				"msg":            "indexer.job.ingested",
				"tenant_id":      t1,
				"ingest_project": "p1",
				"flavor_id":      "",
			}},
		},
		map[string]any{
			"source": "indexer",
			"parsed": map[string]any{"rawFlat": map[string]any{
				"service":        "indexer",
				"index_run_id":   "run99",
				"msg":            "indexer.job.ingested",
				"tenant_id":      t1,
				"ingest_project": "p2",
				"flavor_id":      "",
			}},
		},
	}

	obj := vm.Get("ChimeraSettings").ToObject(vm).Get("Derive").ToObject(vm)
	bfn, ok := goja.AssertFunction(obj.Get("indexerBucketsFromCache"))
	if !ok {
		t.Fatal("missing indexerBucketsFromCache")
	}
	getFlat := vm.Get("__getFlat")
	v, err := bfn(goja.Undefined(), vm.ToValue(cache), getFlat)
	if err != nil {
		t.Fatal(err)
	}
	got := v.ToObject(vm).Get("buckets").ToObject(vm)
	if got.Get(k1) == nil || got.Get(k2) == nil {
		t.Fatalf("want synthetic keys %q and %q, have object", k1, k2)
	}
}

func TestLogsDerive_indexerPartition_stateGoesToServiceOnly(t *testing.T) {
	vm := goja.New()
	loadIndexerPresentCtx(t, vm)
	evalJS(t, vm, settingsUIPath(t, "derive", "indexerPartition.js"))
	if _, err := vm.RunString(`function __getFlat(p) { return (p && p.rawFlat) || {}; }`); err != nil {
		t.Fatal(err)
	}
	cache := []any{
		map[string]any{
			"parsed": map[string]any{"rawFlat": map[string]any{
				"service":      "chimera-indexer",
				"index_run_id": "run-a",
				"msg":          "indexer.state",
				"state":        "watch_idle",
				"indexer_key":  "ik_orphan",
				"tenant_id":    "lynn",
				"user_label":   "lynn",
			}},
		},
		map[string]any{
			"parsed": map[string]any{"rawFlat": map[string]any{
				"service":            "chimera-indexer",
				"index_run_id":       "run-a",
				"msg":                "indexer.storage.stats",
				"indexer_target_key": "ik_orphan",
				"ingest_project":     "porcelain",
				"flavor_id":          "",
				"tenant_id":          "lynn",
				"qdrant_points":      42,
			}},
		},
	}
	obj := vm.Get("ChimeraSettings").ToObject(vm).Get("Derive").ToObject(vm)
	bfn, ok := goja.AssertFunction(obj.Get("indexerBucketsFromCache"))
	if !ok {
		t.Fatal("missing indexerBucketsFromCache")
	}
	v, err := bfn(goja.Undefined(), vm.ToValue(cache), vm.Get("__getFlat"))
	if err != nil {
		t.Fatal(err)
	}
	got := v.ToObject(vm).Get("buckets").ToObject(vm)
	if got == nil {
		t.Fatal("missing buckets")
	}
	if !goja.IsUndefined(got.Get("ik_orphan")) && got.Get("ik_orphan") != nil {
		t.Fatalf("orphan storage.stats should not create workspace bucket alone, got keys")
	}
}

func TestLogsDerive_indexerPartition_storageStatsJoinsExistingScopeBucket(t *testing.T) {
	vm := goja.New()
	loadIndexerPresentCtx(t, vm)
	evalJS(t, vm, settingsUIPath(t, "derive", "indexerPartition.js"))
	if _, err := vm.RunString(`function __getFlat(p) { return (p && p.rawFlat) || {}; }`); err != nil {
		t.Fatal(err)
	}
	t1 := "lynn"
	igKey := "ig\x1e" + t1 + "\x1eporcelain\x1e"
	ikKey := "ik_porcelain_scope"
	cache := []any{
		map[string]any{
			"parsed": map[string]any{"rawFlat": map[string]any{
				"service":            "chimera-indexer",
				"index_run_id":       "run-b",
				"msg":                "indexer.job.ingested",
				"tenant_id":          t1,
				"ingest_project":     "porcelain",
				"flavor_id":          "",
				"indexer_target_key": ikKey,
				"chunks":             3,
			}},
		},
		map[string]any{
			"parsed": map[string]any{"rawFlat": map[string]any{
				"service":            "chimera-indexer",
				"index_run_id":       "run-b",
				"msg":                "indexer.storage.stats",
				"tenant_id":          t1,
				"ingest_project":     "porcelain",
				"flavor_id":          "",
				"indexer_target_key": ikKey,
				"qdrant_points":      10711,
			}},
		},
	}
	obj := vm.Get("ChimeraSettings").ToObject(vm).Get("Derive").ToObject(vm)
	bfn, ok := goja.AssertFunction(obj.Get("indexerBucketsFromCache"))
	if !ok {
		t.Fatal("missing indexerBucketsFromCache")
	}
	v, err := bfn(goja.Undefined(), vm.ToValue(cache), vm.Get("__getFlat"))
	if err != nil {
		t.Fatal(err)
	}
	got := v.ToObject(vm).Get("buckets").ToObject(vm)
	merged := got.Get(ikKey)
	if merged == nil || goja.IsUndefined(merged) {
		merged = got.Get(igKey)
	}
	if merged == nil || goja.IsUndefined(merged) {
		t.Fatalf("want merged scope bucket ik or ig, buckets=%v", got.Keys())
	}
	arr := merged.Export().([]any)
	if len(arr) != 2 {
		t.Fatalf("want job + stats in one bucket, len=%d", len(arr))
	}
}

func TestLogsDerive_indexerPartition_mergesIgIntoIkAlias(t *testing.T) {
	vm := goja.New()
	loadIndexerPresentCtx(t, vm)
	evalJS(t, vm, settingsUIPath(t, "derive", "indexerPartition.js"))
	if _, err := vm.RunString(`function __getFlat(p) { return (p && p.rawFlat) || {}; }`); err != nil {
		t.Fatal(err)
	}
	t1 := "lynn"
	igKey := "ig\x1e" + t1 + "\x1eporcelain\x1e"
	ikKey := "ik_porcelain_scope"
	cache := []any{
		map[string]any{
			"parsed": map[string]any{"rawFlat": map[string]any{
				"service":        "indexer",
				"index_run_id":   "run-c",
				"msg":            "indexer.job.ingested",
				"tenant_id":      t1,
				"ingest_project": "porcelain",
				"flavor_id":      "",
				"chunks":         1,
			}},
		},
		map[string]any{
			"parsed": map[string]any{"rawFlat": map[string]any{
				"service":            "indexer",
				"index_run_id":       "run-c",
				"msg":                "indexer.storage.stats",
				"tenant_id":          t1,
				"ingest_project":     "porcelain",
				"flavor_id":          "",
				"indexer_target_key": ikKey,
				"qdrant_points":      9,
			}},
		},
	}
	obj := vm.Get("ChimeraSettings").ToObject(vm).Get("Derive").ToObject(vm)
	bfn, ok := goja.AssertFunction(obj.Get("indexerBucketsFromCache"))
	if !ok {
		t.Fatal("missing indexerBucketsFromCache")
	}
	v, err := bfn(goja.Undefined(), vm.ToValue(cache), vm.Get("__getFlat"))
	if err != nil {
		t.Fatal(err)
	}
	got := v.ToObject(vm).Get("buckets").ToObject(vm)
	if got.Get(igKey) != nil && !goja.IsUndefined(got.Get(igKey)) {
		t.Fatalf("expected ig bucket merged away, still have %q", igKey)
	}
	if got.Get(ikKey) == nil || goja.IsUndefined(got.Get(ikKey)) {
		t.Fatalf("expected canonical ik bucket %q", ikKey)
	}
}

func TestLogsDerive_collectIndexerRunMeta_chimeraIndexerServiceReadsProject(t *testing.T) {
	vm := goja.New()
	evalJS(t, vm, settingsUIPath(t, "testing", "loader.js"))
	evalJS(t, vm, settingsUIPath(t, "derive", "indexerMetrics.js"))
	derive := vm.Get("ChimeraSettings").ToObject(vm).Get("Derive").ToObject(vm)
	fn, ok := goja.AssertFunction(derive.Get("collectIndexerRunMeta"))
	if !ok {
		t.Fatal("missing collectIndexerRunMeta")
	}
	evs := []map[string]any{
		{"parsed": map[string]any{"rawFlat": map[string]any{
			"service":        "chimera-indexer",
			"msg":            "indexer.storage.stats",
			"user_label":     "lynn",
			"tenant_id":      "lynn",
			"ingest_project": "porcelain",
			"flavor_id":      "",
			"qdrant_points":  10711,
		}}},
	}
	opts := map[string]any{
		"getFlat": func(call goja.FunctionCall) goja.Value {
			p := call.Argument(0).ToObject(vm)
			return p.Get("rawFlat")
		},
	}
	v, err := fn(goja.Undefined(), vm.ToValue("ik_test"), vm.ToValue(evs), vm.ToValue(opts))
	if err != nil {
		t.Fatal(err)
	}
	obj := v.ToObject(vm)
	if obj.Get("projectId").String() != "porcelain" {
		t.Fatalf("projectId=%q want porcelain", obj.Get("projectId").String())
	}
	if obj.Get("userLabel").String() != "lynn" {
		t.Fatalf("userLabel=%q want lynn", obj.Get("userLabel").String())
	}
}

func TestLogsApp_normalizeServiceBucketKey_fullChimeraNames(t *testing.T) {
	t.Parallel()
	vm := goja.New()
	_, err := vm.RunString(`
		function normalizeServiceBucketKey(svc, source) {
			var s = String(svc || "").trim().toLowerCase();
			var src = String(source || "").trim().toLowerCase();
			var alias = {
				gateway: "chimera-gateway",
				vectorstore: "chimera-vectorstore",
				broker: "chimera-broker",
				indexer: "chimera-indexer",
				"chimera-gateway": "chimera-gateway",
				"chimera-vectorstore": "chimera-vectorstore",
				"chimera-broker": "chimera-broker",
				"chimera-indexer": "chimera-indexer",
			};
			if (alias[s]) return alias[s];
			if (alias[src]) return alias[src];
			return "";
		}
	`)
	if err != nil {
		t.Fatal(err)
	}
	fn, ok := goja.AssertFunction(vm.Get("normalizeServiceBucketKey"))
	if !ok {
		t.Fatal("missing normalizeServiceBucketKey")
	}
	cases := []struct {
		svc, src, want string
	}{
		{"chimera-vectorstore", "", "chimera-vectorstore"},
		{"", "chimera-vectorstore", "chimera-vectorstore"},
		{"vectorstore", "", "chimera-vectorstore"},
		{"chimera-gateway", "chimera-gateway", "chimera-gateway"},
		{"unknown-svc", "", ""},
	}
	for _, tc := range cases {
		got, err := fn(goja.Undefined(), vm.ToValue(tc.svc), vm.ToValue(tc.src))
		if err != nil {
			t.Fatalf("%q/%q: %v", tc.svc, tc.src, err)
		}
		if got.String() != tc.want {
			t.Fatalf("%q/%q: got %q want %q", tc.svc, tc.src, got.String(), tc.want)
		}
	}
}

func TestLogsRender_sumEvlog_workspaceScopedLogOmitsIndexerBadge(t *testing.T) {
	vm := goja.New()
	evalJS(t, vm, settingsUIPath(t, "testing", "loader.js"))
	evalJS(t, vm, settingsUIPath(t, "util", "escape.js"))
	evalJS(t, vm, settingsUIPath(t, "render", "sumEvlog.js"))
	_, err := vm.RunString(`
		var ctx = {
			getFlat: function (p) { return (p && p.rawFlat) || {}; },
			escapeHtml: ChimeraSettings.escapeHtml,
			primaryLogMessage: function () { return "Indexed example.go"; },
			formatLogDateTimeLocal: function () { return "12:00:00"; },
			formatLogRelativeAgo: function () { return "just now"; },
			toIsoDatetimeAttr: function () { return "2026-05-22T12:00:00"; },
			strHash: function (s) { return String(s); },
			badgeForIndexerRunLine: function () {
				return { cls: "sum-svc-chimera-indexer", lab: "chimera-indexer" };
			}
		};
		ChimeraSettings.Render.mountSumEvlog(ctx);
		globalThis.__sumEvlogBuild = ctx.sumEvlogBuildTbodyFromServiceEntries;
	`)
	if err != nil {
		t.Fatal(err)
	}
	fn, ok := goja.AssertFunction(vm.Get("__sumEvlogBuild"))
	if !ok {
		t.Fatal("missing sumEvlogBuildTbodyFromServiceEntries")
	}
	entries := []any{
		map[string]any{
			"parsed": map[string]any{"rawFlat": map[string]any{"msg": "indexer.job.ingested", "rel": "example.go"}},
			"text":   "",
			"ts":     "2026-05-22T12:00:00Z",
			"source": "indexer",
		},
	}
	v, err := fn(
		goja.Undefined(),
		vm.ToValue("indexer"),
		vm.ToValue(entries),
		vm.ToValue(map[string]any{"indexerRunLine": true, "suppressIndexerBadge": true, "cardScope": "ws-test"}),
	)
	if err != nil {
		t.Fatal(err)
	}
	html := v.String()
	if strings.Contains(html, "chimera-indexer") {
		t.Fatalf("workspace scoped log should omit chimera-indexer badge, got %q", html)
	}
	if !strings.Contains(html, "Indexed example.go") {
		t.Fatalf("expected message body, got %q", html)
	}
}
