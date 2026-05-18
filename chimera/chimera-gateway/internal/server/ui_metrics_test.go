package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/testsupport"
	"github.com/lynn/porcelain/internal/naming"
)

func TestMetricsAPI_unauthorizedWithoutSession(t *testing.T) {
	t.Setenv(naming.EnvBrokerAPIKeyTarget, "ukey")
	up := chimeraBrokerStubForUILogs(t)
	t.Cleanup(up.Close)

	rt := runtimeForUILogs(t, up.URL)
	ui := NewUIOptions()
	ui.LogStore = nil
	front := httptest.NewServer(NewMux(rt, testLog(), nil, ui))
	t.Cleanup(front.Close)

	res, err := http.Get(front.URL + "/api/ui/metrics")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusUnauthorized {
		t.Fatalf("metrics: status %d", res.StatusCode)
	}
}

func TestMetricsAPI_returnsJSONWhenAuthed(t *testing.T) {
	t.Setenv(naming.EnvBrokerAPIKeyTarget, "ukey")
	up := chimeraBrokerStubForUILogs(t)
	t.Cleanup(up.Close)

	rt := runtimeForUILogs(t, up.URL)
	ui := NewUIOptions()
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

	res, err := client.Get(front.URL + "/api/ui/metrics")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("status %d body %s", res.StatusCode, b)
	}
	var doc map[string]any
	if err := json.NewDecoder(res.Body).Decode(&doc); err != nil {
		t.Fatal(err)
	}
	if doc["ok"] != true {
		t.Fatalf("ok: %+v", doc)
	}
	if _, ok := doc["minute_rollups"]; !ok {
		t.Fatal("missing minute_rollups")
	}
	if _, ok := doc["day_rollups"]; !ok {
		t.Fatal("missing day_rollups")
	}
	if _, ok := doc["recent_events"]; !ok {
		t.Fatal("missing recent_events")
	}
}

func TestMetricsPage_redirectsToLoginWithoutSession(t *testing.T) {
	t.Setenv(naming.EnvBrokerAPIKeyTarget, "ukey")
	up := chimeraBrokerStubForUILogs(t)
	t.Cleanup(up.Close)

	rt := runtimeForUILogs(t, up.URL)
	ui := NewUIOptions()
	front := httptest.NewServer(NewMux(rt, testLog(), nil, ui))
	t.Cleanup(front.Close)

	client := &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	res, err := client.Get(front.URL + "/ui/metrics")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusFound {
		t.Fatalf("status %d", res.StatusCode)
	}
	loc := res.Header.Get("Location")
	if !strings.Contains(loc, "/ui/login") {
		t.Fatalf("location %q", loc)
	}
}

func TestMetricsWithWorkingStore_queryRollups(t *testing.T) {
	t.Setenv(naming.EnvBrokerAPIKeyTarget, "ukey")
	up := chimeraBrokerStubForUILogs(t)
	t.Cleanup(up.Close)

	dir := t.TempDir()
	migSrc := testsupport.GatewayMetricsMigrationsDir(t)
	migDst := filepath.Join(dir, "migrations", "chimera-gateway", "metrics")
	if err := os.MkdirAll(migDst, 0o755); err != nil {
		t.Fatal(err)
	}
	ents, err := os.ReadDir(migSrc)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range ents {
		if e.IsDir() {
			continue
		}
		b, err := os.ReadFile(filepath.Join(migSrc, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(migDst, e.Name()), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	gwPath := filepath.Join(dir, naming.GatewayConfigFileTarget)
	raw := `gateway:
  semver: "0.1.0"
  listen_port: 0
  listen_host: "127.0.0.1"
upstream:
  base_url: "` + up.URL + `"
  api_key_env: "` + naming.EnvBrokerAPIKeyTarget + `"
health:
  timeout_ms: 2000
  chat_timeout_ms: 60000
paths:
  api_keys: "./` + naming.APIKeysFileTarget + `"
  routing_policy: "./` + naming.RoutingPolicyFileTarget + `"
routing:
  fallback_chain:
    - "groq/x"
metrics:
  sqlite_path: "./data/gateway/metrics.sqlite"
  migrations_dir: "./migrations/chimera-gateway/metrics"
`
	if err := os.WriteFile(gwPath, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	writeTokens(t, filepath.Join(dir, naming.APIKeysFileTarget), "gw-ui-secret", "t1")
	if err := os.WriteFile(filepath.Join(dir, naming.RoutingPolicyFileTarget), []byte("rules: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	rt, err := NewRuntime(gwPath, testLog())
	if err != nil {
		t.Fatal(err)
	}
	st := rt.MetricsStore()
	if st == nil {
		t.Fatal("expected metrics store open")
	}
	st.RecordBrokerResponse(time.Now().UTC(), "groq/test-model", 200, 7)

	ui := NewUIOptions()
	front := httptest.NewServer(NewMux(rt, testLog(), nil, ui))
	t.Cleanup(func() {
		front.Close()
		rt.CloseMetrics()
	})

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Jar: jar}
	if _, err := client.Post(front.URL+"/api/ui/login", "application/json", strings.NewReader(`{"token":"gw-ui-secret"}`)); err != nil {
		t.Fatal(err)
	}
	res, err := client.Get(front.URL + "/api/ui/metrics")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(res.Body)
		t.Fatalf("status %d %s", res.StatusCode, b)
	}
	var doc map[string]any
	if err := json.NewDecoder(res.Body).Decode(&doc); err != nil {
		t.Fatal(err)
	}
	if doc["metrics_store_open"] != true {
		t.Fatalf("expected store open: %+v", doc)
	}
}
