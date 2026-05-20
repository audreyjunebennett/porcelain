package embed

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/lynn/porcelain/internal/naming"
)

func embedPackageDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Dir(file)
}

func resetAssetsCache(t *testing.T) {
	t.Helper()
	assetsMu.Lock()
	assetsCached = nil
	assetsEnv = ""
	gatewayListen = ""
	assetsMu.Unlock()
}

func TestReadFile_embeddedDefault(t *testing.T) {
	resetAssetsCache(t)
	t.Setenv(naming.EnvAdminUIRoot, "")
	b, err := ReadFile("embedui/logs.html")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if len(b) == 0 {
		t.Fatal("expected non-empty logs.html")
	}
	if AssetsFromDisk() {
		t.Fatal("expected embedded assets by default")
	}
}

func TestAdminUIRootCandidate_valid(t *testing.T) {
	root := embedPackageDir(t)
	got, ok := adminUIRootCandidate(root)
	if !ok {
		t.Fatalf("adminUIRootCandidate(%q) = false", root)
	}
	if got != root {
		t.Fatalf("got root %q want %q", got, root)
	}
}

func TestResolveAdminUIRoot_embeduiDir(t *testing.T) {
	root := embedPackageDir(t)
	embedui := filepath.Join(root, "embedui")
	got, ok := resolveAdminUIRoot(embedui)
	if !ok || got != root {
		t.Fatalf("resolve from embedui dir: got (%q, %v) want (%q, true)", got, ok, root)
	}
}

func TestReadFile_fromDisk(t *testing.T) {
	root := embedPackageDir(t)
	resetAssetsCache(t)
	SetGatewayListenAddr("127.0.0.1:3000")
	t.Setenv(naming.EnvAdminUIRoot, root)

	marker := filepath.Join(root, "embedui", ".dev_mode_test_marker")
	if err := os.WriteFile(marker, []byte("disk-ok"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(marker) })

	if !AssetsFromDisk() {
		t.Fatal("expected disk assets")
	}
	if DiskRoot() != root {
		t.Fatalf("DiskRoot=%q want %q", DiskRoot(), root)
	}
	b, err := ReadFile("embedui/.dev_mode_test_marker")
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(b) != "disk-ok" {
		t.Fatalf("got %q", b)
	}
}

func TestFilesystemAssets_deniedOnRemoteBind(t *testing.T) {
	root := embedPackageDir(t)
	resetAssetsCache(t)
	SetGatewayListenAddr("0.0.0.0:3000")
	t.Setenv(naming.EnvAdminUIRoot, root)
	if AssetsFromDisk() {
		t.Fatal("expected embedded assets when listen is not loopback")
	}
}

func TestIsLoopbackListen(t *testing.T) {
	cases := []struct {
		addr string
		ok   bool
	}{
		{"127.0.0.1:3000", true},
		{"localhost:3000", true},
		{"[::1]:3000", true},
		{"0.0.0.0:3000", false},
		{"192.168.1.1:3000", false},
	}
	for _, tc := range cases {
		if got := isLoopbackListen(tc.addr); got != tc.ok {
			t.Errorf("isLoopbackListen(%q)=%v want %v", tc.addr, got, tc.ok)
		}
	}
}

func TestServeAsset_fromDisk(t *testing.T) {
	root := embedPackageDir(t)
	resetAssetsCache(t)
	SetGatewayListenAddr("127.0.0.1:3000")
	t.Setenv(naming.EnvAdminUIRoot, root)

	marker := filepath.Join(root, "embedui", "logs", ".dev_mode_serve_marker")
	if err := os.WriteFile(marker, []byte("serve-ok"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Remove(marker) })

	req := httptest.NewRequest(http.MethodGet, "/ui/assets/logs/.dev_mode_serve_marker", nil)
	rec := httptest.NewRecorder()
	ServePathPrefix("embedui/logs/", "/ui/assets/logs/", "application/javascript; charset=utf-8")(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if got := rec.Body.String(); got != "serve-ok" {
		t.Fatalf("body=%q", got)
	}
}
