package server

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lynn/porcelain/chimera/internal/servicelogs"
)

func chimeraBrokerStubForUILogs(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/health":
			w.WriteHeader(http.StatusOK)
		case "/api/providers/groq":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"name":"groq","keys":[]}`))
		default:
			http.NotFound(w, r)
		}
	}))
}

func runtimeForUILogs(t *testing.T, chimeraBrokerURL string) *Runtime {
	t.Helper()
	dir := t.TempDir()
	gwPath := filepath.Join(dir, "gateway.yaml")
	writeGateway(t, gwPath, chimeraBrokerURL, []string{"m"})
	tokPath := filepath.Join(dir, "api-keys.yaml")
	writeTokens(t, tokPath, "gw-ui-secret", "t1")
	routePath := filepath.Join(dir, "routing-policy.yaml")
	if err := os.WriteFile(routePath, []byte("rules: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rt, err := NewRuntime(gwPath, testLog())
	if err != nil {
		t.Fatal(err)
	}
	return rt
}

func TestUILogsAPI_unauthorizedWithoutSession(t *testing.T) {
	t.Setenv("CHIMERA_UPSTREAM_API_KEY", "ukey")
	up := chimeraBrokerStubForUILogs(t)
	t.Cleanup(up.Close)

	rt := runtimeForUILogs(t, up.URL)
	logStore := servicelogs.New(100)
	ui := NewUIOptions()
	ui.LogStore = logStore
	front := httptest.NewServer(NewMux(rt, testLog(), nil, ui))
	t.Cleanup(front.Close)

	res, err := http.Get(front.URL + "/api/ui/logs?since=0")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("logs poll: status %d", res.StatusCode)
	}

	res2, err := http.Get(front.URL + "/api/ui/logs/stream")
	if err != nil {
		t.Fatal(err)
	}
	defer res2.Body.Close()
	if res2.StatusCode != http.StatusUnauthorized {
		t.Fatalf("logs stream: status %d", res2.StatusCode)
	}
}

func TestUILogsPoll_returnsLinesAfterSince(t *testing.T) {
	t.Setenv("CHIMERA_UPSTREAM_API_KEY", "ukey")
	up := chimeraBrokerStubForUILogs(t)
	t.Cleanup(up.Close)

	rt := runtimeForUILogs(t, up.URL)
	logStore := servicelogs.New(100)
	_, _ = io.WriteString(logStore.Writer("unit"), "alpha\nbeta\n")

	ui := NewUIOptions()
	ui.LogStore = logStore
	front := httptest.NewServer(NewMux(rt, testLog(), nil, ui))
	t.Cleanup(front.Close)

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Jar: jar}
	if _, err := client.Post(front.URL+"/api/ui/login", "application/json", strings.NewReader(`{"token":"gw-ui-secret"}`)); err != nil {
		t.Fatal(err)
	}

	res, err := client.Get(front.URL + "/api/ui/logs?since=0")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status %d", res.StatusCode)
	}
	var body logsPollResponse
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if len(body.Lines) != 2 {
		t.Fatalf("lines: %+v", body.Lines)
	}
	if body.Lines[0].Text != "alpha" || body.Lines[1].Text != "beta" {
		t.Fatalf("content: %+v", body.Lines)
	}
	if body.MaxSeq != 2 {
		t.Fatalf("max_seq %d", body.MaxSeq)
	}

	res2, err := client.Get(front.URL + "/api/ui/logs?since=1")
	if err != nil {
		t.Fatal(err)
	}
	defer res2.Body.Close()
	var body2 logsPollResponse
	if err := json.NewDecoder(res2.Body).Decode(&body2); err != nil {
		t.Fatal(err)
	}
	if len(body2.Lines) != 1 || body2.Lines[0].Text != "beta" {
		t.Fatalf("since=1: %+v", body2.Lines)
	}
}

func TestUILogsPoll_limitReturnsTailWhenSinceZero(t *testing.T) {
	t.Setenv("CHIMERA_UPSTREAM_API_KEY", "ukey")
	up := chimeraBrokerStubForUILogs(t)
	t.Cleanup(up.Close)

	rt := runtimeForUILogs(t, up.URL)
	logStore := servicelogs.New(100)
	for _, line := range []string{"a\n", "b\n", "c\n", "d\n", "e\n"} {
		_, _ = io.WriteString(logStore.Writer("unit"), line)
	}

	ui := NewUIOptions()
	ui.LogStore = logStore
	front := httptest.NewServer(NewMux(rt, testLog(), nil, ui))
	t.Cleanup(front.Close)

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Jar: jar}
	if _, err := client.Post(front.URL+"/api/ui/login", "application/json", strings.NewReader(`{"token":"gw-ui-secret"}`)); err != nil {
		t.Fatal(err)
	}

	res, err := client.Get(front.URL + "/api/ui/logs?since=0&limit=2")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status %d", res.StatusCode)
	}
	var body logsPollResponse
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if len(body.Lines) != 2 {
		t.Fatalf("want 2 lines, got %d %+v", len(body.Lines), body.Lines)
	}
	if body.Lines[0].Text != "d" || body.Lines[1].Text != "e" {
		t.Fatalf("want tail d,e got %+v", body.Lines)
	}
	if body.MaxSeq != 5 {
		t.Fatalf("max_seq want 5 got %d", body.MaxSeq)
	}
}

func TestUILogsPoll_beforeSeq_returnsOlderChunk(t *testing.T) {
	t.Setenv("CHIMERA_UPSTREAM_API_KEY", "ukey")
	up := chimeraBrokerStubForUILogs(t)
	t.Cleanup(up.Close)

	rt := runtimeForUILogs(t, up.URL)
	logStore := servicelogs.New(100)
	for _, line := range []string{"a\n", "b\n", "c\n", "d\n", "e\n"} {
		_, _ = io.WriteString(logStore.Writer("unit"), line)
	}

	ui := NewUIOptions()
	ui.LogStore = logStore
	front := httptest.NewServer(NewMux(rt, testLog(), nil, ui))
	t.Cleanup(front.Close)

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Jar: jar}
	if _, err := client.Post(front.URL+"/api/ui/login", "application/json", strings.NewReader(`{"token":"gw-ui-secret"}`)); err != nil {
		t.Fatal(err)
	}

	res, err := client.Get(front.URL + "/api/ui/logs?before_seq=6&limit=2")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status %d", res.StatusCode)
	}
	var body logsPollResponse
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if len(body.Lines) != 2 {
		t.Fatalf("want 2 lines, got %d %+v", len(body.Lines), body.Lines)
	}
	if body.Lines[0].Text != "d" || body.Lines[1].Text != "e" {
		t.Fatalf("want d,e got %+v", body.Lines)
	}
	if body.HasOlderInBuf == nil || !*body.HasOlderInBuf {
		t.Fatalf("expected has_older_in_buffer true (more lines below chunk)")
	}
}

func TestUILogsPoll_sinceAndBeforeRejected(t *testing.T) {
	t.Setenv("CHIMERA_UPSTREAM_API_KEY", "ukey")
	up := chimeraBrokerStubForUILogs(t)
	t.Cleanup(up.Close)

	rt := runtimeForUILogs(t, up.URL)
	logStore := servicelogs.New(10)
	ui := NewUIOptions()
	ui.LogStore = logStore
	front := httptest.NewServer(NewMux(rt, testLog(), nil, ui))
	t.Cleanup(front.Close)

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Jar: jar}
	if _, err := client.Post(front.URL+"/api/ui/login", "application/json", strings.NewReader(`{"token":"gw-ui-secret"}`)); err != nil {
		t.Fatal(err)
	}

	res, err := client.Get(front.URL + "/api/ui/logs?since=0&before_seq=9")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("want 400 got %d", res.StatusCode)
	}
}

func TestUILogsStream_replaysTailOnConnect(t *testing.T) {
	t.Setenv("CHIMERA_UPSTREAM_API_KEY", "ukey")
	up := chimeraBrokerStubForUILogs(t)
	t.Cleanup(up.Close)

	rt := runtimeForUILogs(t, up.URL)
	logStore := servicelogs.New(100)
	_, _ = io.WriteString(logStore.Writer("unit"), "sse-seed\n")

	ui := NewUIOptions()
	ui.LogStore = logStore
	h := NewMux(rt, testLog(), nil, ui)
	front := httptest.NewServer(h)
	t.Cleanup(front.Close)

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Jar: jar}
	if _, err := client.Post(front.URL+"/api/ui/login", "application/json", strings.NewReader(`{"token":"gw-ui-secret"}`)); err != nil {
		t.Fatal(err)
	}
	u, err := url.Parse(front.URL)
	if err != nil {
		t.Fatal(err)
	}
	var sessionValue string
	for _, c := range jar.Cookies(u) {
		if c.Name == defaultUICookieName {
			sessionValue = c.Value
			break
		}
	}
	if sessionValue == "" {
		t.Fatal("no session cookie after login")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		time.Sleep(80 * time.Millisecond)
		cancel()
	}()

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/ui/logs/stream", nil)
	req = req.WithContext(ctx)
	req.AddCookie(&http.Cookie{Name: defaultUICookieName, Value: sessionValue})
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("stream status %d body %s", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "text/event-stream") {
		t.Fatalf("content-type %q", ct)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "sse-seed") {
		t.Fatalf("expected SSE replay, got %q", body[:min(400, len(body))])
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func TestUILogsPage_requiresAuth(t *testing.T) {
	t.Setenv("CHIMERA_UPSTREAM_API_KEY", "ukey")
	up := chimeraBrokerStubForUILogs(t)
	t.Cleanup(up.Close)

	rt := runtimeForUILogs(t, up.URL)
	ui := NewUIOptions()
	ui.LogStore = servicelogs.New(10)
	front := httptest.NewServer(NewMux(rt, testLog(), nil, ui))
	t.Cleanup(front.Close)

	noFollow := &http.Client{CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	res, err := noFollow.Get(front.URL + "/ui/logs")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusFound {
		t.Fatalf("status %d", res.StatusCode)
	}
	loc := res.Header.Get("Location")
	if !strings.Contains(loc, "/ui/login") || !strings.Contains(loc, "next=") {
		t.Fatalf("location %q", loc)
	}
}

func TestUIDesktopPage_requiresAuth(t *testing.T) {
	t.Setenv("CHIMERA_UPSTREAM_API_KEY", "ukey")
	up := chimeraBrokerStubForUILogs(t)
	t.Cleanup(up.Close)

	rt := runtimeForUILogs(t, up.URL)
	ui := NewUIOptions()
	ui.LogStore = servicelogs.New(10)
	front := httptest.NewServer(NewMux(rt, testLog(), nil, ui))
	t.Cleanup(front.Close)

	noFollow := &http.Client{CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	res, err := noFollow.Get(front.URL + "/ui/desktop")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusFound {
		t.Fatalf("status %d", res.StatusCode)
	}
}

func TestUILogsPage_servesLogsHTMLWhenAuthed(t *testing.T) {
	t.Setenv("CHIMERA_UPSTREAM_API_KEY", "ukey")
	up := chimeraBrokerStubForUILogs(t)
	t.Cleanup(up.Close)

	rt := runtimeForUILogs(t, up.URL)
	ui := NewUIOptions()
	ui.LogStore = servicelogs.New(10)
	front := httptest.NewServer(NewMux(rt, testLog(), nil, ui))
	t.Cleanup(front.Close)

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Jar: jar}
	if _, err := client.Post(front.URL+"/api/ui/login", "application/json", strings.NewReader(`{"token":"gw-ui-secret"}`)); err != nil {
		t.Fatal(err)
	}
	res, err := client.Get(front.URL + "/ui/logs")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status %d", res.StatusCode)
	}
	b, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	page := string(b)
	// Shell contract: keep these stable so the logs view can be refactored safely
	// (CSS/JS extraction, modularization, and componentization) without breaking the page structure.
	wantAll := []string{
		"Chimera — Logs",
		`href="/ui/assets/logs.css"`,
		`src="/ui/assets/logs/testing/loader.js"`,
		`src="/ui/assets/logs/util/escape.js"`,
		`src="/ui/assets/logs/util/hash.js"`,
		`src="/ui/assets/logs/util/time.js"`,
		`src="/ui/assets/logs/parse/parseLogText.js"`,
		`src="/ui/assets/logs/transport/streaming.js"`,
		`src="/ui/assets/logs/derive/conversationMetrics.js"`,
		`src="/ui/assets/logs/derive/conversationBifrost.js"`,
		`src="/ui/assets/logs/derive/bifrostMetrics.js"`,
		`src="/ui/assets/logs/derive/sha1.js"`,
		`src="/ui/assets/logs/derive/qdrantRagMetrics.js"`,
		`src="/ui/assets/logs/derive/qdrantCollection.js"`,
		`src="/ui/assets/logs/derive/indexerMetrics.js"`,
		`src="/ui/assets/logs/derive/gatewayUsageMetrics.js"`,
		`src="/ui/assets/logs/derive/gatewayCardModel.js"`,
		`src="/ui/assets/logs/derive/conversationCardModel.js"`,
		`src="/ui/assets/logs/components/StatusLine.js"`,
		`src="/ui/assets/logs/components/KeyValueGrid.js"`,
		`src="/ui/assets/logs/components/Badge.js"`,
		`src="/ui/assets/logs/components/MetricPills.js"`,
		`src="/ui/assets/logs/main.js"`,
		`src="/ui/assets/logs.js"`,
		`id="logs-chrome"`,
		`id="status"`,
		`id="panel-summarized"`,
	}
	for _, w := range wantAll {
		if strings.Contains(page, w) {
			continue
		}
		snippet := page
		if len(snippet) > 600 {
			snippet = snippet[:600]
		}
		t.Fatalf("missing %q in /ui/logs HTML shell; snippet=%q", w, snippet)
	}
	if strings.Contains(page, "<style") {
		snippet := page
		if len(snippet) > 600 {
			snippet = snippet[:600]
		}
		t.Fatalf("expected CSS extracted (no inline <style>) in /ui/logs shell; snippet=%q", snippet)
	}
}

func TestUILogsAssets_servesLogsJSWhenAuthed(t *testing.T) {
	t.Setenv("CHIMERA_UPSTREAM_API_KEY", "ukey")
	up := chimeraBrokerStubForUILogs(t)
	t.Cleanup(up.Close)

	rt := runtimeForUILogs(t, up.URL)
	ui := NewUIOptions()
	ui.LogStore = servicelogs.New(10)
	front := httptest.NewServer(NewMux(rt, testLog(), nil, ui))
	t.Cleanup(front.Close)

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Jar: jar}
	if _, err := client.Post(front.URL+"/api/ui/login", "application/json", strings.NewReader(`{"token":"gw-ui-secret"}`)); err != nil {
		t.Fatal(err)
	}
	res, err := client.Get(front.URL + "/ui/assets/logs.js")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status %d", res.StatusCode)
	}
	if ct := res.Header.Get("Content-Type"); !strings.Contains(ct, "javascript") {
		t.Fatalf("content-type %q", ct)
	}
	b, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	js := string(b)
	want := []string{
		"ChimeraLogs",
		// logs.js is now a bootstrap that relies on module assets for most behavior.
		"ChimeraLogs.Main",
	}
	for _, w := range want {
		if strings.Contains(js, w) {
			continue
		}
		snippet := js
		if len(snippet) > 600 {
			snippet = snippet[:600]
		}
		t.Fatalf("missing %q in logs.js; snippet=%q", w, snippet)
	}
}

func TestUILogsAssets_logsMainContainsBrokerServiceSummary(t *testing.T) {
	t.Setenv("CHIMERA_UPSTREAM_API_KEY", "ukey")
	up := chimeraBrokerStubForUILogs(t)
	t.Cleanup(up.Close)

	rt := runtimeForUILogs(t, up.URL)
	ui := NewUIOptions()
	ui.LogStore = servicelogs.New(10)
	front := httptest.NewServer(NewMux(rt, testLog(), nil, ui))
	t.Cleanup(front.Close)

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Jar: jar}
	if _, err := client.Post(front.URL+"/api/ui/login", "application/json", strings.NewReader(`{"token":"gw-ui-secret"}`)); err != nil {
		t.Fatal(err)
	}
	res, err := client.Get(front.URL + "/ui/assets/logs/main.js")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status %d", res.StatusCode)
	}
	b, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	body := string(b)
	if !strings.Contains(body, "indexer-run-kv--chimera-broker-summary") {
		t.Fatal("expected chimera-broker service card KV class in logs main bundle")
	}
	if !strings.Contains(body, "sum-mini-row--chimera-broker-deck") {
		t.Fatal("expected chimera-broker summary deck layout class in logs main bundle")
	}
	if !strings.Contains(body, "Provider health") {
		t.Fatal("expected broker provider-health section label in logs main bundle")
	}
	if !strings.Contains(body, "Relay outcomes") {
		t.Fatal("expected BiFrost relay-outcome section label in logs main bundle")
	}
	if !strings.Contains(body, "sum-bf-prov-health-root") {
		t.Fatal("expected provider-health strip root class in logs main bundle")
	}
	if !strings.Contains(body, "sum-timeline-bar--relay-outcome") {
		t.Fatal("expected relay-outcome strip class in logs main bundle")
	}
	if !strings.Contains(body, "/api/ui/chimera-broker/providers") {
		t.Fatal("expected logs main bundle to fetch live broker provider snapshot")
	}
	if !strings.Contains(body, "chimera-broker-provider-health-strip") {
		t.Fatal("expected stable id wrapper for broker provider-health strip patching")
	}
}

func TestUILogsAssets_servesLogsModuleWhenAuthed(t *testing.T) {
	t.Setenv("CHIMERA_UPSTREAM_API_KEY", "ukey")
	up := chimeraBrokerStubForUILogs(t)
	t.Cleanup(up.Close)

	rt := runtimeForUILogs(t, up.URL)
	ui := NewUIOptions()
	ui.LogStore = servicelogs.New(10)
	front := httptest.NewServer(NewMux(rt, testLog(), nil, ui))
	t.Cleanup(front.Close)

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Jar: jar}
	if _, err := client.Post(front.URL+"/api/ui/login", "application/json", strings.NewReader(`{"token":"gw-ui-secret"}`)); err != nil {
		t.Fatal(err)
	}

	res, err := client.Get(front.URL + "/ui/assets/logs/transport/streaming.js")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status %d", res.StatusCode)
	}
	if ct := res.Header.Get("Content-Type"); !strings.Contains(ct, "javascript") {
		t.Fatalf("content-type %q", ct)
	}
	b, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "Transport") {
		t.Fatalf("unexpected module body: %q", string(b)[:min(200, len(b))])
	}
}

func TestUIDesktopPage_servesShellWhenAuthed(t *testing.T) {
	t.Setenv("CHIMERA_UPSTREAM_API_KEY", "ukey")
	up := chimeraBrokerStubForUILogs(t)
	t.Cleanup(up.Close)

	rt := runtimeForUILogs(t, up.URL)
	ui := NewUIOptions()
	ui.LogStore = servicelogs.New(10)
	front := httptest.NewServer(NewMux(rt, testLog(), nil, ui))
	t.Cleanup(front.Close)

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Jar: jar}
	if _, err := client.Post(front.URL+"/api/ui/login", "application/json", strings.NewReader(`{"token":"gw-ui-secret"}`)); err != nil {
		t.Fatal(err)
	}
	res, err := client.Get(front.URL + "/ui/desktop")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status %d", res.StatusCode)
	}
	b, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	page := string(b)
	if !strings.Contains(page, "f-main") || !strings.Contains(page, "/ui/pwa") {
		t.Fatal("expected desktop shell with default PWA page")
	}
	if strings.Contains(page, `data-tab="main"`) {
		t.Fatal("desktop shell should not include a separate Main tab")
	}
	if strings.Contains(page, "f-stats") || strings.Contains(page, `data-tab="stats"`) {
		t.Fatal("desktop shell should not include a Stats tab (metrics live under /ui/logs summarized view)")
	}
	if strings.Contains(page, `data-tab="indexer"`) || strings.Contains(page, "f-indexer") {
		t.Fatal("desktop shell should not include Indexer tab (workspaces live under Logs)")
	}
}

func TestUIPWAPage_servesWhenAuthed(t *testing.T) {
	t.Setenv("CHIMERA_UPSTREAM_API_KEY", "ukey")
	up := chimeraBrokerStubForUILogs(t)
	t.Cleanup(up.Close)

	rt := runtimeForUILogs(t, up.URL)
	ui := NewUIOptions()
	ui.LogStore = servicelogs.New(10)
	front := httptest.NewServer(NewMux(rt, testLog(), nil, ui))
	t.Cleanup(front.Close)

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Jar: jar}
	if _, err := client.Post(front.URL+"/api/ui/login", "application/json", strings.NewReader(`{"token":"gw-ui-secret"}`)); err != nil {
		t.Fatal(err)
	}
	res, err := client.Get(front.URL + "/ui/pwa")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status %d", res.StatusCode)
	}
	b, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	page := string(b)
	if !strings.Contains(page, ">PWA<") {
		t.Fatal("expected PWA landing content")
	}
}

func TestUIIndexer_redirectsToLogsWhenAuthed(t *testing.T) {
	t.Setenv("CHIMERA_UPSTREAM_API_KEY", "ukey")
	up := chimeraBrokerStubForUILogs(t)
	t.Cleanup(up.Close)

	rt := runtimeForUILogs(t, up.URL)
	ui := NewUIOptions()
	ui.LogStore = servicelogs.New(10)
	front := httptest.NewServer(NewMux(rt, testLog(), nil, ui))
	t.Cleanup(front.Close)

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{
		Jar: jar,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	if _, err := client.Post(front.URL+"/api/ui/login", "application/json", strings.NewReader(`{"token":"gw-ui-secret"}`)); err != nil {
		t.Fatal(err)
	}
	res, err := client.Get(front.URL + "/ui/indexer")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusFound {
		t.Fatalf("status %d want %d", res.StatusCode, http.StatusFound)
	}
	if loc := res.Header.Get("Location"); loc != "/ui/logs" {
		t.Fatalf("Location %q want %q", loc, "/ui/logs")
	}
}

func TestUIPanel_redirectsToLogsAdminWhenAuthed(t *testing.T) {
	t.Setenv("CHIMERA_UPSTREAM_API_KEY", "ukey")
	up := chimeraBrokerStubForUILogs(t)
	t.Cleanup(up.Close)

	rt := runtimeForUILogs(t, up.URL)
	ui := NewUIOptions()
	ui.LogStore = servicelogs.New(10)
	front := httptest.NewServer(NewMux(rt, testLog(), nil, ui))
	t.Cleanup(front.Close)

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{
		Jar: jar,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	if _, err := client.Post(front.URL+"/api/ui/login", "application/json", strings.NewReader(`{"token":"gw-ui-secret"}`)); err != nil {
		t.Fatal(err)
	}
	res, err := client.Get(front.URL + "/ui/panel")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusFound {
		t.Fatalf("status %d want %d", res.StatusCode, http.StatusFound)
	}
	if loc := res.Header.Get("Location"); loc != "/ui/logs?focus=admin" {
		t.Fatalf("Location %q want %q", loc, "/ui/logs?focus=admin")
	}
}
