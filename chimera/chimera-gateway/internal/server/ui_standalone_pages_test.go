package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/server/adminui/embed"
	"github.com/lynn/porcelain/internal/naming"
)

func TestStandalonePages_embedHTMLUsesSharedCSS(t *testing.T) {
	for _, name := range []string{"embedui/login.html", "embedui/setup.html"} {
		b, err := embed.ReadFile(name)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		s := string(b)
		for _, want := range []string{
			"/ui/assets/theme-tokens.css",
			"/ui/assets/ui.css",
			`class="embed-page`,
		} {
			if !strings.Contains(s, want) {
				t.Fatalf("%s: missing %q", name, want)
			}
		}
		if strings.Contains(s, "<style>") {
			t.Fatalf("%s: should not use inline <style> blocks", name)
		}
	}
}

func TestPublicPrimitiveCSS_servedWithoutSession(t *testing.T) {
	t.Setenv(naming.EnvBrokerAPIKeyTarget, "ukey")
	up := chimeraBrokerStubForUILogs(t)
	t.Cleanup(up.Close)

	rt := runtimeForUILogs(t, up.URL)
	ui := NewUIOptions()
	ui.LogStore = nil // login/CSS must work when logs routes are omitted
	front := httptest.NewServer(NewMux(rt, testLog(), nil, ui))
	t.Cleanup(front.Close)

	for _, path := range []string{"/ui/assets/ui.css", "/ui/assets/theme-tokens.css"} {
		res, err := http.Get(front.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(res.Body)
		res.Body.Close()
		if res.StatusCode != http.StatusOK {
			t.Fatalf("%s: status %d body %s", path, res.StatusCode, body)
		}
		if ct := res.Header.Get("Content-Type"); !strings.Contains(ct, "text/css") {
			t.Fatalf("%s: Content-Type %q", path, ct)
		}
	}
}

func TestBootstrapSetup_servesSharedCSS(t *testing.T) {
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

	rt, err := NewRuntime(gwPath, testLog())
	if err != nil {
		t.Fatal(err)
	}
	h := NewBootstrapMux(rt, testLog(), &StatusOverlay{EffectiveListen: "127.0.0.1:9"})
	ts := httptest.NewServer(h)
	t.Cleanup(ts.Close)

	res, err := http.Get(ts.URL + "/ui/assets/ui.css")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("ui.css status %d", res.StatusCode)
	}
}
