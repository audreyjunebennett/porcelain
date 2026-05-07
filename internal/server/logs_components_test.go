package server

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

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
		{"text": "", "parsed": map[string]any{"shape": "chat.bifrost", "rawFlat": map[string]any{"msg": "upstream chat response", "usageTotalTokens": 20, "responseBytes": 100, "statusCode": 200}}},
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
	// legacy behavior: only counts statusCode for http.access, upstream chat response, and bifrost.error lines
	// (request lines don't contribute to scErr even if they contain 429 strings).
	if obj.Get("scErr").Export() != int64(1) {
		t.Fatalf("scErr=%v", obj.Get("scErr").Export())
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
