package server

import (
	"context"
	"encoding/json"
	"github.com/lynn/porcelain/internal/naming"
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
	gwPath := filepath.Join(dir, naming.GatewayConfigFileTarget)
	writeGateway(t, gwPath, chimeraBrokerURL, []string{"m"}, "")
	tokPath := filepath.Join(dir, "api-keys.yaml")
	writeTokens(t, tokPath, "gw-ui-secret", "t1")
	routePath := filepath.Join(dir, "routing-policy.yaml")
	if err := os.WriteFile(routePath, []byte("rules: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rt := mustRuntime(t, gwPath)
	return rt
}

func TestUILogsAPI_unauthorizedWithoutSession(t *testing.T) {
	t.Setenv(naming.EnvBrokerAPIKeyTarget, "ukey")
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
	t.Setenv(naming.EnvBrokerAPIKeyTarget, "ukey")
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
	var body servicelogs.PollResponse
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
	var body2 servicelogs.PollResponse
	if err := json.NewDecoder(res2.Body).Decode(&body2); err != nil {
		t.Fatal(err)
	}
	if len(body2.Lines) != 1 || body2.Lines[0].Text != "beta" {
		t.Fatalf("since=1: %+v", body2.Lines)
	}
}

func TestUILogsPoll_limitReturnsTailWhenSinceZero(t *testing.T) {
	t.Setenv(naming.EnvBrokerAPIKeyTarget, "ukey")
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
	var body servicelogs.PollResponse
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
	t.Setenv(naming.EnvBrokerAPIKeyTarget, "ukey")
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
	var body servicelogs.PollResponse
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
	t.Setenv(naming.EnvBrokerAPIKeyTarget, "ukey")
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
	t.Setenv(naming.EnvBrokerAPIKeyTarget, "ukey")
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
		if c.Name == DefaultUICookieName {
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
	req.AddCookie(&http.Cookie{Name: DefaultUICookieName, Value: sessionValue})
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

func TestUISettingsPage_requiresAuth(t *testing.T) {
	t.Setenv(naming.EnvBrokerAPIKeyTarget, "ukey")
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
	res, err := noFollow.Get(front.URL + "/ui/settings")
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

func TestUIRoot_redirectsToShellWhenUIOptionsEnabled(t *testing.T) {
	t.Setenv(naming.EnvBrokerAPIKeyTarget, "ukey")
	up := chimeraBrokerStubForUILogs(t)
	t.Cleanup(up.Close)

	rt := runtimeForUILogs(t, up.URL)
	ui := NewUIOptions()
	front := httptest.NewServer(NewMux(rt, testLog(), nil, ui))
	t.Cleanup(front.Close)

	noFollow := &http.Client{CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	res, err := noFollow.Get(front.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusFound {
		t.Fatalf("status %d want 302", res.StatusCode)
	}
	if loc := res.Header.Get("Location"); loc != "/ui" {
		t.Fatalf("Location: %q want /ui", loc)
	}
}

func TestUIShellPage_requiresAuth(t *testing.T) {
	t.Setenv(naming.EnvBrokerAPIKeyTarget, "ukey")
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
	res, err := noFollow.Get(front.URL + "/ui")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusFound {
		t.Fatalf("status %d", res.StatusCode)
	}
}

func TestUISettingsPage_servesHTMLWhenAuthed(t *testing.T) {
	t.Setenv(naming.EnvBrokerAPIKeyTarget, "ukey")
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
	res, err := client.Get(front.URL + "/ui/settings")
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
		"Chimera — Settings",
		`href="/ui/assets/settings.css"`,
		`src="/ui/assets/settings/testing/loader.js"`,
		`src="/ui/assets/ui/util/escape.js"`,
		`src="/ui/assets/ui/components/Pill.js"`,
		`src="/ui/assets/ui/components/StatusIndicator.js"`,
		`src="/ui/assets/settings/models.js"`,
		`src="/ui/assets/settings/util/escape.js"`,
		`src="/ui/assets/settings/util/hash.js"`,
		`src="/ui/assets/settings/util/time.js"`,
		`src="/ui/assets/settings/parse/parseLogText.js"`,
		`src="/ui/assets/settings/transport/streaming.js"`,
		`src="/ui/assets/settings/contracts.js"`,
		`src="/ui/assets/settings/derive/conversationMetrics.js"`,
		`src="/ui/assets/settings/derive/conversationBroker.js"`,
		`src="/ui/assets/settings/derive/chimeraBrokerMetrics.js"`,
		`src="/ui/assets/settings/derive/sha1.js"`,
		`src="/ui/assets/settings/derive/vectorstoreRagMetrics.js"`,
		`src="/ui/assets/settings/derive/vectorstoreCollection.js"`,
		`src="/ui/assets/settings/render/sumEvlog.js"`,
		`src="/ui/assets/settings/render/cards/mount.js"`,
		`src="/ui/assets/settings/app/summarizedFeed.js"`,
		`src="/ui/assets/settings/handlers/evlog.js"`,
		`src="/ui/assets/settings/handlers/chrome.js"`,
		`src="/ui/assets/settings/handlers/admin.js"`,
		`src="/ui/assets/settings/app/wireHandlers.js"`,
		`src="/ui/assets/settings/derive/indexerMetrics.js"`,
		`src="/ui/assets/settings/derive/gatewayUsageMetrics.js"`,
		`src="/ui/assets/settings/derive/gatewayCardModel.js"`,
		`src="/ui/assets/settings/derive/conversationCardModel.js"`,
		`src="/ui/assets/settings/components/StatusLine.js"`,
		`src="/ui/assets/settings/components/KeyValueGrid.js"`,
		`src="/ui/assets/settings/components/Badge.js"`,
		`src="/ui/assets/settings/components/MetricPills.js"`,
		`src="/ui/assets/settings/main.js"`,
		`src="/ui/assets/settings.js"`,
		`id="logs-chrome"`,
		`id="status"`,
		`id="panel-summarized"`,
		`href="/ui/settings/gallery"`,
		`Component gallery`,
	}
	for _, w := range wantAll {
		if strings.Contains(page, w) {
			continue
		}
		snippet := page
		if len(snippet) > 600 {
			snippet = snippet[:600]
		}
		t.Fatalf("missing %q in /ui/settings HTML shell; snippet=%q", w, snippet)
	}
	if strings.Contains(page, "<style") {
		snippet := page
		if len(snippet) > 600 {
			snippet = snippet[:600]
		}
		t.Fatalf("expected CSS extracted (no inline <style>) in /ui/settings shell; snippet=%q", snippet)
	}
	for _, bad := range []string{"?focus=", "/ui/logs", "/ui/desktop"} {
		if strings.Contains(page, bad) {
			t.Fatalf("settings page must not reference %q", bad)
		}
	}
}

func TestUISettingsAssets_servesEntryJSWhenAuthed(t *testing.T) {
	t.Setenv(naming.EnvBrokerAPIKeyTarget, "ukey")
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
	res, err := client.Get(front.URL + "/ui/assets/settings.js")
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
		"ChimeraSettings",
		// settings.js is a bootstrap that relies on module assets for most behavior.
		"ChimeraSettings.Main",
	}
	for _, w := range want {
		if strings.Contains(js, w) {
			continue
		}
		snippet := js
		if len(snippet) > 600 {
			snippet = snippet[:600]
		}
		t.Fatalf("missing %q in settings.js; snippet=%q", w, snippet)
	}
}

func TestUILogsAssets_summarizedFeedContainsBrokerServiceSummary(t *testing.T) {
	t.Setenv(naming.EnvBrokerAPIKeyTarget, "ukey")
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
	res, err := client.Get(front.URL + "/ui/assets/settings/app/summarizedFeed.js")
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
		t.Fatal("expected chimera-broker service card KV class in summarized feed module")
	}
	if !strings.Contains(body, "sum-mini-row--chimera-broker-deck") {
		t.Fatal("expected chimera-broker summary deck layout class in summarized feed module")
	}
	if !strings.Contains(body, "Provider health") {
		t.Fatal("expected broker provider-health section label in summarized feed module")
	}
	if !strings.Contains(body, "Relay outcomes") {
		t.Fatal("expected broker relay-outcome section label in summarized feed module")
	}
	if !strings.Contains(body, "sum-bf-prov-health-root") {
		t.Fatal("expected provider-health strip root class in summarized feed module")
	}
	if !strings.Contains(body, "sum-timeline-bar--relay-outcome") {
		t.Fatal("expected relay-outcome strip class in summarized feed module")
	}
	if !strings.Contains(body, "/api/ui/chimera-broker/providers") {
		t.Fatal("expected summarized feed module to fetch live broker provider snapshot")
	}
	if !strings.Contains(body, "chimera-broker-provider-health-strip") {
		t.Fatal("expected stable id wrapper for broker provider-health strip patching")
	}
}

func TestUILogsAssets_servesLogsModuleWhenAuthed(t *testing.T) {
	t.Setenv(naming.EnvBrokerAPIKeyTarget, "ukey")
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

	res, err := client.Get(front.URL + "/ui/assets/settings/transport/streaming.js")
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

func TestUIShellPage_servesWhenAuthed(t *testing.T) {
	t.Setenv(naming.EnvBrokerAPIKeyTarget, "ukey")
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
	res, err := client.Get(front.URL + "/ui")
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
	if !strings.Contains(page, "f-main") || !strings.Contains(page, "/ui/chat") {
		t.Fatal("expected app shell with default chat route")
	}
	if !strings.Contains(page, "/ui/settings") || !strings.Contains(page, "material-symbols-outlined") {
		t.Fatal("expected shell settings control and Material refresh icon")
	}
	if strings.Contains(page, `data-tab="main"`) {
		t.Fatal("app shell should not include a separate Main tab")
	}
	if strings.Contains(page, "f-stats") || strings.Contains(page, `data-tab="stats"`) {
		t.Fatal("app shell should not include a Stats tab (metrics live under /ui/settings)")
	}
	if strings.Contains(page, `data-tab="indexer"`) || strings.Contains(page, "f-indexer") {
		t.Fatal("app shell should not include Indexer tab (workspaces live under /ui/settings)")
	}
	for _, bad := range []string{"/ui/desktop", "/ui/logs", "reload.svg"} {
		if strings.Contains(page, bad) {
			t.Fatalf("shell must not reference %q", bad)
		}
	}
}

func TestUIPWAPage_servesWhenAuthed(t *testing.T) {
	t.Setenv(naming.EnvBrokerAPIKeyTarget, "ukey")
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

func TestUILegacyRoutes_notFoundWhenAuthed(t *testing.T) {
	t.Setenv(naming.EnvBrokerAPIKeyTarget, "ukey")
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
	legacy := []string{
		"/ui/desktop", "/ui/logs", "/ui/panel", "/ui/metrics", "/ui/indexer",
		"/ui/gallery", "/ui/gallery/operator", "/ui/gallery/tokens",
		"/ui/assets/reload.svg",
	}
	for _, path := range legacy {
		res, err := client.Get(front.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != http.StatusNotFound {
			t.Fatalf("%s status %d want 404", path, res.StatusCode)
		}
	}
}

func TestUISettingsGallery_servesWhenAuthed(t *testing.T) {
	t.Setenv(naming.EnvBrokerAPIKeyTarget, "ukey")
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
	res, err := client.Get(front.URL + "/ui/settings/gallery")
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
	for _, w := range []string{
		"Settings gallery",
		`/ui/assets/gallery/gallery-shell.css`,
		`/ui/assets/gallery/gallery-card-fixtures.js`,
		`/ui/assets/gallery/gallery-parts.js`,
		`gallery-parts-toggle`,
		`card-parts-registry.md`,
		`/ui/assets/gallery/gallery-evlog-markup.js`,
		`/ui/assets/gallery/gallery-event-log-demo.js`,
		`gallery-fixture-overview`,
		`gallery-fixture-provider-groq`,
		`gallery-fixture-virtual-model`,
		`data-sum-evlog-cols="2"`,
		`sum-evlog__msg-meta`,
		`material-symbols-outlined`,
	} {
		if !strings.Contains(page, w) {
			t.Fatalf("missing %q in gallery HTML", w)
		}
	}
	for _, bad := range []string{"sample.html", "reload.svg", "/ui/gallery"} {
		if strings.Contains(page, bad) {
			t.Fatalf("gallery page must not reference %q", bad)
		}
	}
	res2, err := client.Get(front.URL + "/ui/assets/gallery/gallery-shell.css")
	if err != nil {
		t.Fatal(err)
	}
	defer res2.Body.Close()
	if res2.StatusCode != http.StatusOK {
		t.Fatalf("gallery css status %d", res2.StatusCode)
	}
}

func TestUIOperatorPages_serveWithoutLogStore(t *testing.T) {
	t.Setenv(naming.EnvBrokerAPIKeyTarget, "ukey")
	up := chimeraBrokerStubForUILogs(t)
	t.Cleanup(up.Close)

	rt := runtimeForUILogs(t, up.URL)
	ui := NewUIOptions()
	ui.LogStore = nil
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
	for _, path := range []string{"/ui", "/ui/chat", "/ui/pwa", "/ui/settings", "/ui/settings/gallery"} {
		res, err := client.Get(front.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		if res.StatusCode != http.StatusOK {
			res.Body.Close()
			t.Fatalf("%s status %d want 200", path, res.StatusCode)
		}
		res.Body.Close()
	}
	res, err := client.Get(front.URL + "/ui/assets/settings.js")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("settings.js status %d", res.StatusCode)
	}
	res2, err := client.Get(front.URL + "/api/ui/logs?since=0")
	if err != nil {
		t.Fatal(err)
	}
	defer res2.Body.Close()
	if res2.StatusCode != http.StatusNotFound {
		t.Fatalf("logs API without LogStore: status %d want 404", res2.StatusCode)
	}
}
