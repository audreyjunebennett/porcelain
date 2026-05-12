package server

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/dop251/goja"
)

func logsUIPath(t *testing.T, rel ...string) string {
	t.Helper()
	// Derive repo-relative path from this test file location, not from the process CWD.
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	// thisFile = .../internal/server/logs_components_test.go
	base := filepath.Join(filepath.Dir(thisFile), "embedui", "logs")
	return filepath.Join(append([]string{base}, rel...)...)
}

func mustReadFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

func evalJS(t *testing.T, vm *goja.Runtime, path string) {
	t.Helper()
	_, err := vm.RunString(mustReadFile(t, path))
	if err != nil {
		t.Fatalf("eval %s: %v", path, err)
	}
}

func getFn(t *testing.T, vm *goja.Runtime, name string) goja.Callable {
	t.Helper()
	obj := vm.Get("ClaudiaLogs").ToObject(vm)
	fn, ok := goja.AssertFunction(obj.Get(name))
	if !ok {
		t.Fatalf("missing ClaudiaLogs.%s", name)
	}
	return fn
}

func TestLogsComponents_escapeHtml(t *testing.T) {
	vm := goja.New()
	evalJS(t, vm, logsUIPath(t, "testing", "loader.js"))
	evalJS(t, vm, logsUIPath(t, "util", "escape.js"))

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
	evalJS(t, vm, logsUIPath(t, "testing", "loader.js"))
	evalJS(t, vm, logsUIPath(t, "util", "escape.js"))
	evalJS(t, vm, logsUIPath(t, "components", "KeyValueGrid.js"))

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
	evalJS(t, vm, logsUIPath(t, "testing", "loader.js"))
	evalJS(t, vm, logsUIPath(t, "util", "escape.js"))
	evalJS(t, vm, logsUIPath(t, "components", "Badge.js"))

	badge := getFn(t, vm, "Badge")

	model := map[string]any{"text": "Qdrant", "variant": "svc-qdrant", "title": "vector store"}
	v, err := badge(goja.Undefined(), vm.ToValue(model))
	if err != nil {
		t.Fatal(err)
	}
	html := v.String()
	if !strings.Contains(html, `sum-svc-qdrant`) {
		t.Fatalf("expected svc class, got %q", html)
	}
	if !strings.Contains(html, `title="vector store"`) {
		t.Fatalf("expected title, got %q", html)
	}
}

func TestLogsDerive_scrapeConversationMetrics_totalTokensAndVec(t *testing.T) {
	vm := goja.New()
	// load minimal runtime (no DOM required)
	evalJS(t, vm, logsUIPath(t, "testing", "loader.js"))
	evalJS(t, vm, logsUIPath(t, "derive", "conversationMetrics.js"))

	claudia := vm.Get("ClaudiaLogs").ToObject(vm)
	derive := claudia.Get("Derive").ToObject(vm)
	scrape, ok := goja.AssertFunction(derive.Get("scrapeConversationMetrics"))
	if !ok {
		t.Fatal("missing ClaudiaLogs.Derive.scrapeConversationMetrics")
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

func TestLogsDerive_conversationBifrostRelayCount_andFlat(t *testing.T) {
	vm := goja.New()
	evalJS(t, vm, logsUIPath(t, "testing", "loader.js"))
	evalJS(t, vm, logsUIPath(t, "derive", "conversationBifrost.js"))

	derive := vm.Get("ClaudiaLogs").ToObject(vm).Get("Derive").ToObject(vm)
	countFn, ok := goja.AssertFunction(derive.Get("conversationBifrostRelayCount"))
	if !ok {
		t.Fatal("missing conversationBifrostRelayCount")
	}
	flatFn, ok := goja.AssertFunction(derive.Get("conversationBifrostTimelineFlat"))
	if !ok {
		t.Fatal("missing conversationBifrostTimelineFlat")
	}

	events := []map[string]any{
		{"parsed": map[string]any{"rawFlat": map[string]any{"msg": "chat.bifrost.request"}}},
		{"parsed": map[string]any{"rawFlat": map[string]any{"msg": "chat.bifrost.response"}}},
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

	vAvail, err := flatFn(goja.Undefined(), vm.ToValue(map[string]any{"msg": "chat.bifrost.available_models"}))
	if err != nil {
		t.Fatal(err)
	}
	if b, ok := vAvail.Export().(bool); !ok || b {
		t.Fatalf("available_models flat=%v want false", vAvail.Export())
	}

	vFb, err := flatFn(goja.Undefined(), vm.ToValue(map[string]any{"msg": "chat.bifrost.request"}))
	if err != nil {
		t.Fatal(err)
	}
	if b, ok := vFb.Export().(bool); !ok || !b {
		t.Fatalf("request flat=%v want true", vFb.Export())
	}
}

func TestLogsDerive_conversationCardModel_joinAndProgress(t *testing.T) {
	vm := goja.New()
	evalJS(t, vm, logsUIPath(t, "testing", "loader.js"))
	evalJS(t, vm, logsUIPath(t, "derive", "conversationBifrost.js"))
	evalJS(t, vm, logsUIPath(t, "derive", "conversationCardModel.js"))

	derive := vm.Get("ClaudiaLogs").ToObject(vm).Get("Derive").ToObject(vm)
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
		{"ts": "2026-05-09T12:00:04.000Z", "parsed": map[string]any{"rawFlat": map[string]any{"msg": "conversation.upstream.started"}}},
		{"ts": "2026-05-09T12:00:05.000Z", "parsed": map[string]any{"rawFlat": map[string]any{"msg": "conversation.upstream.completed"}}},
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
	if prog.Get("upstream").String() != "done" {
		t.Fatalf("upstream=%q want done", prog.Get("upstream").String())
	}
	if prog.Get("delivered").String() != "done" {
		t.Fatalf("delivered=%q want done", prog.Get("delivered").String())
	}
}

func TestLogsDerive_conversationCardModel_toolsChipCounts(t *testing.T) {
	vm := goja.New()
	evalJS(t, vm, logsUIPath(t, "testing", "loader.js"))
	evalJS(t, vm, logsUIPath(t, "derive", "conversationCardModel.js"))

	derive := vm.Get("ClaudiaLogs").ToObject(vm).Get("Derive").ToObject(vm)
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
	evalJS(t, vm, logsUIPath(t, "testing", "loader.js"))
	evalJS(t, vm, logsUIPath(t, "derive", "conversationCardModel.js"))

	derive := vm.Get("ClaudiaLogs").ToObject(vm).Get("Derive").ToObject(vm)
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
	evalJS(t, vm, logsUIPath(t, "testing", "loader.js"))
	evalJS(t, vm, logsUIPath(t, "derive", "conversationCardModel.js"))

	derive := vm.Get("ClaudiaLogs").ToObject(vm).Get("Derive").ToObject(vm)
	matchFn, ok := goja.AssertFunction(derive.Get("joinQdrantLineConversationMatch"))
	if !ok {
		t.Fatal("missing joinQdrantLineConversationMatch")
	}

	path := filepath.Join("testdata", "correlation", "phase5-qdrant-tier4b.example.log")
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
	evalJS(t, vm, logsUIPath(t, "testing", "loader.js"))
	evalJS(t, vm, logsUIPath(t, "derive", "conversationCardModel.js"))

	derive := vm.Get("ClaudiaLogs").ToObject(vm).Get("Derive").ToObject(vm)
	groupFn, ok := goja.AssertFunction(derive.Get("conversationTurnGroupsForExpanded"))
	if !ok {
		t.Fatal("missing conversationTurnGroupsForExpanded")
	}

	path := filepath.Join("testdata", "correlation", "phase6-multi-turn.example.log")
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
	evalJS(t, vm, logsUIPath(t, "testing", "loader.js"))
	evalJS(t, vm, logsUIPath(t, "derive", "conversationCardModel.js"))

	derive := vm.Get("ClaudiaLogs").ToObject(vm).Get("Derive").ToObject(vm)
	groupFn, ok := goja.AssertFunction(derive.Get("conversationTurnGroupsForExpanded"))
	if !ok {
		t.Fatal("missing conversationTurnGroupsForExpanded")
	}

	events := []map[string]any{
		{"ts": "2026-05-09T12:00:00.000Z", "seq": 1, "parsed": map[string]any{"rawFlat": map[string]any{"msg": "conversation.merge.resolve_failed"}}},
		{"ts": "2026-05-09T12:00:00.010Z", "seq": 2, "parsed": map[string]any{"rawFlat": map[string]any{"msg": "conversation.received", "turn_index": 1}}},
		{"ts": "2026-05-09T12:00:00.020Z", "seq": 3, "parsed": map[string]any{"rawFlat": map[string]any{"msg": "chat.bifrost.request"}}},
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
		t.Fatalf("turn 1 events=%d want 3 (received + inherited bifrost + qdrant)", secondEvs.Get("length").ToInteger())
	}
	if got := secondEvs.Get("1").ToObject(vm).Get("parsed").ToObject(vm).Get("rawFlat").ToObject(vm).Get("msg").String(); got != "chat.bifrost.request" {
		t.Fatalf("turn 1 second msg=%q want chat.bifrost.request", got)
	}
	if got := secondEvs.Get("2").ToObject(vm).Get("parsed").ToObject(vm).Get("rawFlat").ToObject(vm).Get("msg").String(); got != "qdrant.http.vector_search" {
		t.Fatalf("turn 1 third msg=%q want qdrant.http.vector_search", got)
	}
}

func TestLogsDerive_conversationRequestIdTier2Eligible_narrowChatPrefixes(t *testing.T) {
	vm := goja.New()
	evalJS(t, vm, logsUIPath(t, "testing", "loader.js"))
	evalJS(t, vm, logsUIPath(t, "derive", "conversationBifrost.js"))
	evalJS(t, vm, logsUIPath(t, "derive", "conversationCardModel.js"))
	elig, ok := goja.AssertFunction(vm.Get("ClaudiaLogs").ToObject(vm).Get("Derive").ToObject(vm).Get("conversationRequestIdTier2Eligible"))
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

func TestLogsDerive_bifrostCardMetrics_countsAndRateLimit(t *testing.T) {
	vm := goja.New()
	evalJS(t, vm, logsUIPath(t, "testing", "loader.js"))
	evalJS(t, vm, logsUIPath(t, "derive", "bifrostMetrics.js"))

	claudia := vm.Get("ClaudiaLogs").ToObject(vm)
	derive := claudia.Get("Derive").ToObject(vm)
	metricFn, ok := goja.AssertFunction(derive.Get("bifrostCardMetrics"))
	if !ok {
		t.Fatal("missing ClaudiaLogs.Derive.bifrostCardMetrics")
	}

	arr := []map[string]any{
		{"text": "429 rate limit", "parsed": map[string]any{"shape": "chat.bifrost", "rawFlat": map[string]any{"msg": "chat.bifrost.request", "outgoingTokens": 10, "stream": true, "upstreamModel": "groq/x", "statusCode": 429}}},
		{"text": "", "parsed": map[string]any{"shape": "chat.bifrost", "rawFlat": map[string]any{"msg": "chat.bifrost.response", "usageTotalTokens": 20, "responseBytes": 100, "statusCode": 200}}},
		{"text": "", "parsed": map[string]any{"shape": "chat.bifrost", "rawFlat": map[string]any{"msg": "chat.bifrost.error", "statusCode": 500}}},
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
	// legacy behavior: only counts statusCode for http.access, chat.bifrost.response (legacy: upstream chat response), and bifrost.error lines
	// (request lines don't contribute to scErr even if they contain 429 strings).
	if obj.Get("scErr").Export() != int64(1) {
		t.Fatalf("scErr=%v", obj.Get("scErr").Export())
	}
}

func TestLogsDerive_bifrostOperatorLine(t *testing.T) {
	vm := goja.New()
	evalJS(t, vm, logsUIPath(t, "testing", "loader.js"))
	evalJS(t, vm, logsUIPath(t, "derive", "bifrostMetrics.js"))

	derive := vm.Get("ClaudiaLogs").ToObject(vm).Get("Derive").ToObject(vm)
	fn, ok := goja.AssertFunction(derive.Get("bifrostOperatorLine"))
	if !ok {
		t.Fatal("missing ClaudiaLogs.Derive.bifrostOperatorLine")
	}

	cases := []struct {
		flat map[string]any
		want string
	}{
		{
			flat: map[string]any{"service": "bifrost", "msg": "bifrost.http.access", "http_method": "GET", "http_target": "http://localhost:8081/v1/models", "http_status": 200, "http_duration_ms": 3.2},
			want: "Inbound · GET /v1/models · → 200 · 3 ms",
		},
		{
			flat: map[string]any{"service": "bifrost", "msg": "bifrost.rate_limit", "http_method": "POST", "http_target": "/v1/embeddings", "http_status": 429, "http_duration_ms": 10},
			want: "Rate limited · POST /v1/embeddings · → 429 · 10 ms",
		},
		{
			flat: map[string]any{"service": "bifrost", "msg": "bifrost.catalog.sync", "catalog_model_count": 42},
			want: "Model catalog updated · 42 models",
		},
		{
			flat: map[string]any{"service": "gateway", "msg": "chat.bifrost.available_models", "catalog_model_count": 100},
			want: "Model list for routing · 100 models",
		},
		{
			flat: map[string]any{"msg": "chat.bifrost.request", "upstreamModel": "groq/openai/gpt-oss-20b", "stream": true, "outgoingTokens": 128},
			want: "Relay request · gpt-oss-20b · streaming on · 128 tok out",
		},
		{
			flat: map[string]any{"msg": "chat.bifrost.response", "statusCode": 200, "usageTotalTokens": 50, "responseBytes": 1200},
			want: "Relay response · HTTP 200 · 50 usage tok · 1200 B",
		},
		{
			flat: map[string]any{"msg": "upstream chat response", "statusCode": 201, "usageTotalTokens": 1, "responseBytes": 10},
			want: "Relay response · HTTP 201 · 1 usage tok · 10 B",
		},
		{
			flat: map[string]any{"msg": "chat.routing.attempt", "attempt": 2, "upstreamModel": "x/y/z", "chainLen": 3},
			want: "Routing attempt · z · attempt 2/3",
		},
		{
			flat: map[string]any{"msg": "chat.bifrost.error", "err": "connection reset"},
			want: "Relay failed · connection reset",
		},
		{
			flat: map[string]any{"service": "bifrost", "msg": "bifrost.ready", "listen_url": "http://127.0.0.1:8899/ui"},
			want: "Ready · UI at http://127.0.0.1:8899/ui",
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
			flat: map[string]any{"service": "bifrost", "msg": "bifrost.http.access", "http_method": "GET", "http_target": "http://localhost:8081/v1/models", "http_status": 200, "http_duration_ms": 3.2},
			want: "Inbound · GET /v1/models · 3 ms",
		},
		{
			flat: map[string]any{"msg": "chat.bifrost.response", "statusCode": 200, "usageTotalTokens": 50, "responseBytes": 1200},
			want: "Relay response · 50 usage tok · 1200 B",
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
		t.Fatalf("non-bifrost msg should return empty got %q", vSilent.String())
	}
}

func TestLogsDerive_bifrostSliceSinceLastBanner_resetsCounters(t *testing.T) {
	vm := goja.New()
	evalJS(t, vm, logsUIPath(t, "testing", "loader.js"))
	evalJS(t, vm, logsUIPath(t, "derive", "bifrostMetrics.js"))

	derive := vm.Get("ClaudiaLogs").ToObject(vm).Get("Derive").ToObject(vm)
	sliceFn, ok := goja.AssertFunction(derive.Get("bifrostSliceSinceLastBanner"))
	if !ok {
		t.Fatal("missing bifrostSliceSinceLastBanner")
	}
	metricFn, ok := goja.AssertFunction(derive.Get("bifrostCardMetrics"))
	if !ok {
		t.Fatal("missing bifrostCardMetrics")
	}

	arr := []map[string]any{
		{"text": "", "parsed": map[string]any{"rawFlat": map[string]any{"msg": "bifrost.provider.loaded", "service": "bifrost", "provider_id": "old"}}},
		{"text": "", "parsed": map[string]any{"rawFlat": map[string]any{"msg": "bifrost.startup.banner", "service": "bifrost"}}},
		{"text": "", "parsed": map[string]any{"rawFlat": map[string]any{"msg": "bifrost.provider.loaded", "service": "bifrost", "provider_id": "new"}}},
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

func TestLogsDerive_bifrostCardModel_kv(t *testing.T) {
	vm := goja.New()
	evalJS(t, vm, logsUIPath(t, "testing", "loader.js"))
	evalJS(t, vm, logsUIPath(t, "derive", "bifrostMetrics.js"))

	fn, ok := goja.AssertFunction(vm.Get("ClaudiaLogs").ToObject(vm).Get("Derive").ToObject(vm).Get("bifrostCardModel"))
	if !ok {
		t.Fatal("missing bifrostCardModel")
	}
	arr := []map[string]any{
		{"text": `{"msg":"bifrost.version","service":"bifrost","bifrost_version":"1.2.3"}`, "parsed": map[string]any{"rawFlat": map[string]any{"msg": "bifrost.version", "service": "bifrost", "bifrost_version": "1.2.3"}}},
		{"text": "", "parsed": map[string]any{"rawFlat": map[string]any{"msg": "bifrost.config.loaded", "service": "bifrost"}}},
		{"text": "", "parsed": map[string]any{"rawFlat": map[string]any{"msg": "bifrost.ready", "service": "bifrost", "listen_port": 8080}}},
		{"text": "", "parsed": map[string]any{"rawFlat": map[string]any{"msg": "bifrost.auth.token_refresh", "service": "bifrost"}}},
		{"text": "", "parsed": map[string]any{"rawFlat": map[string]any{"msg": "chat.bifrost.request", "upstreamModel": "groq/llama"}}},
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
		t.Fatalf("auth=%q want jwt from bifrost.auth.token_refresh", o.Get("auth").String())
	}
}

func TestLogsDerive_bifrostProviderHealthList_pickLatest(t *testing.T) {
	vm := goja.New()
	evalJS(t, vm, logsUIPath(t, "testing", "loader.js"))
	evalJS(t, vm, logsUIPath(t, "derive", "bifrostMetrics.js"))

	derive := vm.Get("ClaudiaLogs").ToObject(vm).Get("Derive").ToObject(vm)
	fn, ok := goja.AssertFunction(derive.Get("bifrostProviderHealthList"))
	if !ok {
		t.Fatal("missing bifrostProviderHealthList")
	}

	arr := []map[string]any{
		{"parsed": map[string]any{"rawFlat": map[string]any{"msg": "bifrost.provider.loaded", "service": "bifrost", "provider_id": "groq"}}},
		{"parsed": map[string]any{"rawFlat": map[string]any{"msg": "bifrost.provider.loaded", "service": "bifrost", "provider_id": "openai"}}},
		{"parsed": map[string]any{"rawFlat": map[string]any{"msg": "bifrost.provider.loaded", "service": "bifrost", "provider_id": "gemini"}}},
		{"parsed": map[string]any{"rawFlat": map[string]any{"msg": "bifrost.provider.health.fail", "service": "bifrost", "provider_id": "gemini"}}},
		{"parsed": map[string]any{"rawFlat": map[string]any{"msg": "bifrost.provider.health.ok", "service": "bifrost", "provider_id": "gemini"}}},
		{"parsed": map[string]any{"rawFlat": map[string]any{"msg": "bifrost.provider.health.fail", "service": "bifrost", "provider_id": "openai"}}},
		{"parsed": map[string]any{"rawFlat": map[string]any{"msg": "bifrost.provider.key_missing", "service": "bifrost", "provider_id": "anthropic"}}},
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
		"groq":      "up",
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

func TestLogsDerive_bifrostRelayOutcomeBuckets_httpAndErrors(t *testing.T) {
	vm := goja.New()
	evalJS(t, vm, logsUIPath(t, "testing", "loader.js"))
	evalJS(t, vm, logsUIPath(t, "derive", "bifrostMetrics.js"))

	derive := vm.Get("ClaudiaLogs").ToObject(vm).Get("Derive").ToObject(vm)
	fn, ok := goja.AssertFunction(derive.Get("bifrostRelayOutcomeBuckets"))
	if !ok {
		t.Fatal("missing bifrostRelayOutcomeBuckets")
	}

	arr := []map[string]any{
		{"parsed": map[string]any{"rawFlat": map[string]any{"msg": "bifrost.startup.banner", "service": "bifrost"}}},
		{"parsed": map[string]any{"rawFlat": map[string]any{"msg": "bifrost.ready", "service": "bifrost"}}},
		{"parsed": map[string]any{"rawFlat": map[string]any{"msg": "chat.bifrost.request", "upstreamModel": "groq/x"}}},
		{"parsed": map[string]any{"rawFlat": map[string]any{"msg": "chat.bifrost.response", "statusCode": 200}}},
		{"parsed": map[string]any{"rawFlat": map[string]any{"msg": "chat.bifrost.request", "upstreamModel": "groq/x"}}},
		{"parsed": map[string]any{"rawFlat": map[string]any{"msg": "chat.bifrost.response", "statusCode": 429}}},
		{"parsed": map[string]any{"rawFlat": map[string]any{"msg": "chat.bifrost.request", "upstreamModel": "groq/x"}}},
		{"parsed": map[string]any{"rawFlat": map[string]any{"msg": "chat.bifrost.response", "statusCode": 401}}},
		{"parsed": map[string]any{"rawFlat": map[string]any{"msg": "chat.bifrost.request", "upstreamModel": "groq/x"}}},
		{"parsed": map[string]any{"rawFlat": map[string]any{"msg": "chat.bifrost.response", "statusCode": 503}}},
		{"parsed": map[string]any{"rawFlat": map[string]any{"msg": "chat.bifrost.request", "upstreamModel": "groq/x"}}},
		{"parsed": map[string]any{"rawFlat": map[string]any{"msg": "chat.bifrost.error", "err": "connection reset"}}},
		{"parsed": map[string]any{"rawFlat": map[string]any{"msg": "chat.bifrost.request", "upstreamModel": "groq/x"}}},
		{"parsed": map[string]any{"rawFlat": map[string]any{"msg": "bifrost.rate_limit", "service": "bifrost", "http_status": 429}}},
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

func TestLogsDerive_bifrostCardMetrics_catalogModelCount_gatewayLine(t *testing.T) {
	vm := goja.New()
	evalJS(t, vm, logsUIPath(t, "testing", "loader.js"))
	evalJS(t, vm, logsUIPath(t, "derive", "bifrostMetrics.js"))
	metricFn, ok := goja.AssertFunction(vm.Get("ClaudiaLogs").ToObject(vm).Get("Derive").ToObject(vm).Get("bifrostCardMetrics"))
	if !ok {
		t.Fatal("missing bifrostCardMetrics")
	}
	arr := []map[string]any{
		{"text": "", "parsed": map[string]any{"rawFlat": map[string]any{
			"msg": "chat.bifrost.available_models", "service": "gateway", "catalog_model_count": 42,
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

func TestLogsDerive_qdrantCollection_nameGolden(t *testing.T) {
	vm := goja.New()
	evalJS(t, vm, logsUIPath(t, "testing", "loader.js"))
	evalJS(t, vm, logsUIPath(t, "derive", "sha1.js"))
	evalJS(t, vm, logsUIPath(t, "derive", "qdrantCollection.js"))

	derive := vm.Get("ClaudiaLogs").ToObject(vm).Get("Derive").ToObject(vm)
	fn, ok := goja.AssertFunction(derive.Get("qdrantCollectionName"))
	if !ok {
		t.Fatal("missing qdrantCollectionName")
	}
	v, err := fn(goja.Undefined(), vm.ToValue("default"), vm.ToValue("assistants"), vm.ToValue(""))
	if err != nil {
		t.Fatal(err)
	}
	if v.String() != "claudia-default-assistants-_-be0adbc3" {
		t.Fatalf("got %q", v.String())
	}
	v2, err := fn(goja.Undefined(), vm.ToValue("default"), vm.ToValue("clone"), vm.ToValue(""))
	if err != nil {
		t.Fatal(err)
	}
	if v2.String() != "claudia-default-clone-_-0ef54bfe" {
		t.Fatalf("clone got %q", v2.String())
	}
}

func TestLogsDerive_qdrantOperatorLine_forEventLog(t *testing.T) {
	vm := goja.New()
	evalJS(t, vm, logsUIPath(t, "testing", "loader.js"))
	evalJS(t, vm, logsUIPath(t, "derive", "qdrantCollection.js"))

	derive := vm.Get("ClaudiaLogs").ToObject(vm).Get("Derive").ToObject(vm)
	fn, ok := goja.AssertFunction(derive.Get("qdrantOperatorLine"))
	if !ok {
		t.Fatal("missing qdrantOperatorLine")
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
}

func TestLogsDerive_qdrantRag_rollups(t *testing.T) {
	vm := goja.New()
	evalJS(t, vm, logsUIPath(t, "testing", "loader.js"))
	evalJS(t, vm, logsUIPath(t, "derive", "qdrantRagMetrics.js"))

	claudia := vm.Get("ClaudiaLogs").ToObject(vm)
	derive := claudia.Get("Derive").ToObject(vm)

	ragFn, ok := goja.AssertFunction(derive.Get("rollupGatewayRagPipeline"))
	if !ok {
		t.Fatal("missing rollupGatewayRagPipeline")
	}
	qFn, ok := goja.AssertFunction(derive.Get("qdrantHttpPathRollup"))
	if !ok {
		t.Fatal("missing qdrantHttpPathRollup")
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
	evalJS(t, vm, logsUIPath(t, "testing", "loader.js"))
	evalJS(t, vm, logsUIPath(t, "derive", "indexerMetrics.js"))

	claudia := vm.Get("ClaudiaLogs").ToObject(vm)
	derive := claudia.Get("Derive").ToObject(vm)
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

func TestLogsDerive_gatewayCardModel_kvCountersHideRow(t *testing.T) {
	vm := goja.New()
	evalJS(t, vm, logsUIPath(t, "testing", "loader.js"))
	evalJS(t, vm, logsUIPath(t, "derive", "gatewayCardModel.js"))

	derive := vm.Get("ClaudiaLogs").ToObject(vm).Get("Derive").ToObject(vm)
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
			"msg": "gateway.startup.listening", "addr": ":8080", "upstream": "http://bifrost", "bifrost_data": "/data/b",
			"qdrant_supervised": true, "indexer_supervised": false, "config": "/x/gateway.yaml",
		}}},
		{"parsed": map[string]any{"rawFlat": map[string]any{"msg": "gateway.auth.reloaded", "count": 2}}},
		{"parsed": map[string]any{"rawFlat": map[string]any{"msg": "routing.policy.reloaded", "rules": 5}}},
		{"parsed": map[string]any{"rawFlat": map[string]any{"msg": "chat.request"}}},
		{"parsed": map[string]any{"rawFlat": map[string]any{"msg": "chat.bifrost.response", "statusCode": 200}}},
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
	if kv.Get("upstream").String() != "http://bifrost" {
		t.Fatalf("upstream=%q", kv.Get("upstream").String())
	}
	if kv.Get("apiKeys").String() != "2" {
		t.Fatalf("apiKeys=%q", kv.Get("apiKeys").String())
	}
	if kv.Get("routingRules").String() != "5" {
		t.Fatalf("routingRules=%q", kv.Get("routingRules").String())
	}
	sup := kv.Get("supervised").String()
	if !strings.Contains(sup, "qdrant") || !strings.Contains(sup, "bifrost") {
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

	ent := map[string]any{
		"parsed": map[string]any{"shape": "http.access", "rawFlat": map[string]any{"path": "/api/ui/logs", "statusCode": 200}},
	}
	vh, err := hideFn(goja.Undefined(), vm.ToValue(ent), goja.Undefined())
	if err != nil {
		t.Fatal(err)
	}
	if vh.Export().(bool) != true {
		t.Fatalf("hide probe logs=%v", vh.Export())
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
	evalJS(t, vm, logsUIPath(t, "testing", "loader.js"))
	evalJS(t, vm, logsUIPath(t, "derive", "gatewayUsageMetrics.js"))

	claudia := vm.Get("ClaudiaLogs").ToObject(vm)
	derive := claudia.Get("Derive").ToObject(vm)
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
	evalJS(t, vm, logsUIPath(t, "testing", "loader.js"))
	evalJS(t, vm, logsUIPath(t, "derive", "indexerPresent.js"))
	derive := vm.Get("ClaudiaLogs").ToObject(vm).Get("Derive").ToObject(vm)
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

func TestLogsDerive_indexerPresent_proseStateAndStats(t *testing.T) {
	vm := goja.New()
	evalJS(t, vm, logsUIPath(t, "testing", "loader.js"))
	evalJS(t, vm, logsUIPath(t, "derive", "indexerPresent.js"))
	derive := vm.Get("ClaudiaLogs").ToObject(vm).Get("Derive").ToObject(vm)
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
	statsFlat := map[string]any{"service": "indexer", "msg": "indexer.storage.stats", "qdrant_points": 120, "available": true, "collection": "c-test"}
	pg, err := proseFn(goja.Undefined(), vm.ToValue(statsFlat))
	if err != nil {
		t.Fatal(err)
	}
	g := pg.String()
	if !strings.Contains(g, "120") || !strings.Contains(g, "Indexed vectors") {
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
	if !strings.Contains(ps2Str, "12") || !strings.Contains(ps2Str, "files in workspace") ||
		!strings.Contains(ps2Str, "waiting to embed") || !strings.Contains(ps2Str, "discovery queue") {
		t.Fatalf("scope status prose: %q", ps2Str)
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
	if !strings.Contains(fs, "chunked upload session missing") {
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
}

func TestLogsDerive_collectIndexerRunMeta_scopeStatus(t *testing.T) {
	vm := goja.New()
	evalJS(t, vm, logsUIPath(t, "testing", "loader.js"))
	evalJS(t, vm, logsUIPath(t, "derive", "indexerPresent.js"))
	evalJS(t, vm, logsUIPath(t, "derive", "indexerMetrics.js"))

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
		ClaudiaLogs.Derive.collectIndexerRunMeta("ik_one", evs, { getFlat: getFlat });
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
	evalJS(t, vm, logsUIPath(t, "testing", "loader.js"))
	evalJS(t, vm, logsUIPath(t, "derive", "indexerPresent.js"))
	derive := vm.Get("ClaudiaLogs").ToObject(vm).Get("Derive").ToObject(vm)
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
	evalJS(t, vm, logsUIPath(t, "testing", "loader.js"))
	evalJS(t, vm, logsUIPath(t, "derive", "indexerPresent.js"))
	evalJS(t, vm, logsUIPath(t, "derive", "indexerPartition.js"))

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

	obj := vm.Get("ClaudiaLogs").ToObject(vm).Get("Derive").ToObject(vm)
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

func TestLogsDerive_indexerPartition_sourceOnlyNoServiceField(t *testing.T) {
	vm := goja.New()
	evalJS(t, vm, logsUIPath(t, "testing", "loader.js"))
	evalJS(t, vm, logsUIPath(t, "derive", "indexerPresent.js"))
	evalJS(t, vm, logsUIPath(t, "derive", "indexerPartition.js"))

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

	obj := vm.Get("ClaudiaLogs").ToObject(vm).Get("Derive").ToObject(vm)
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
	evalJS(t, vm, logsUIPath(t, "testing", "loader.js"))
	evalJS(t, vm, logsUIPath(t, "derive", "indexerPresent.js"))
	evalJS(t, vm, logsUIPath(t, "derive", "indexerPartition.js"))

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

	obj := vm.Get("ClaudiaLogs").ToObject(vm).Get("Derive").ToObject(vm)
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
