package server

import (
	"bytes"
	"encoding/json"
	"github.com/lynn/porcelain/internal/naming"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/lynn/porcelain/chimera/internal/config"
	"github.com/lynn/porcelain/chimera/internal/tokens"
)

func TestBootstrapMode_missingTokensFile(t *testing.T) {
	t.Setenv(naming.EnvBrokerAPIKeyTarget, "ukey")
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(up.Close)

	dir := t.TempDir()
	gwPath := filepath.Join(dir, naming.GatewayConfigFileTarget)
	writeGateway(t, gwPath, up.URL, []string{"m"}, "")
	routePath := filepath.Join(dir, "routing-policy.yaml")
	if err := os.WriteFile(routePath, []byte("rules: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	rt := mustRuntime(t, gwPath)
	if !BootstrapMode(rt) {
		t.Fatal("expected bootstrap with no api-keys.yaml")
	}
}

func TestNewBootstrapMux_setupTokenThenNotBootstrap(t *testing.T) {
	t.Setenv(naming.EnvBrokerAPIKeyTarget, "ukey")
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(up.Close)

	dir := t.TempDir()
	gwPath := filepath.Join(dir, naming.GatewayConfigFileTarget)
	writeGateway(t, gwPath, up.URL, []string{"m"}, "")
	routePath := filepath.Join(dir, "routing-policy.yaml")
	if err := os.WriteFile(routePath, []byte("rules: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	rt := mustRuntime(t, gwPath)
	h := NewBootstrapMux(rt, testLog(), &StatusOverlay{EffectiveListen: "127.0.0.1:9"})
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)

	res, err := http.Get(ts.URL + "/health")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("health %d", res.StatusCode)
	}

	body := bytes.NewBufferString(`{"label":"test-admin"}`)
	res2, err := http.Post(ts.URL+"/api/ui/setup/token", "application/json", body)
	if err != nil {
		t.Fatal(err)
	}
	defer res2.Body.Close()
	if res2.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(res2.Body)
		t.Fatalf("setup %d: %s", res2.StatusCode, b)
	}
	var out map[string]any
	if err := json.NewDecoder(res2.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	tok, _ := out["token"].(string)
	if len(tok) < 16 {
		t.Fatalf("token: %q", tok)
	}

	if BootstrapMode(rt) {
		t.Fatal("expected bootstrap mode off after token file written")
	}
	if tokens.IsBootstrapMode(filepath.Join(dir, "api-keys.yaml")) {
		t.Fatal("tokens file should be valid")
	}

	res3, err := http.Post(ts.URL+"/api/ui/setup/token", "application/json", bytes.NewBufferString(`{"label":"x"}`))
	if err != nil {
		t.Fatal(err)
	}
	defer res3.Body.Close()
	if res3.StatusCode != http.StatusNotFound {
		t.Fatalf("second setup want 404, got %d", res3.StatusCode)
	}
}

func TestBootstrapListenPort_respectsFlag(t *testing.T) {
	res := &config.Resolved{ListenPort: 3000, ListenHost: "0.0.0.0"}
	p := BootstrapListenPort(res, ":4444")
	if p != 4444 {
		t.Fatalf("port %d", p)
	}
}
