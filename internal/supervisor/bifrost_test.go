package supervisor

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func runHealthServer(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(http.StatusOK)
			return
		}
		http.NotFound(w, r)
	}))
}

func TestAbsBinIfNeeded(t *testing.T) {
	if got, err := absBinIfNeeded("bifrost"); err != nil || got != "bifrost" {
		t.Fatalf("bare name: got %q err %v", got, err)
	}
	dir := t.TempDir()
	sub := filepath.Join(dir, "fakebin")
	if err := os.WriteFile(sub, []byte{}, 0o755); err != nil {
		t.Fatal(err)
	}
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(wd) })
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	// filepath.Join(".", "fakebin") may clean to "fakebin" (no path sep); use explicit ./ for absBinIfNeeded.
	got, err := absBinIfNeeded("./fakebin")
	if err != nil {
		t.Fatal(err)
	}
	want, err := filepath.Abs(sub)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestMergeEnv(t *testing.T) {
	t.Setenv("ZZZ_SUP_TEST_A", "from_parent")
	out := MergeEnv(map[string]string{
		"ZZZ_SUP_TEST_A": "override",
		"ZZZ_SUP_TEST_B": "new",
	})
	found := make(map[string]string)
	for _, e := range out {
		i := strings.IndexByte(e, '=')
		if i > 0 {
			found[e[:i]] = e[i+1:]
		}
	}
	if found["ZZZ_SUP_TEST_A"] != "override" || found["ZZZ_SUP_TEST_B"] != "new" {
		t.Fatalf("%v", found)
	}
}

func TestCopyConfigJSON(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "in.json")
	if err := os.WriteFile(src, []byte(`{"x":1}`), 0o644); err != nil {
		t.Fatal(err)
	}
	dstDir := filepath.Join(dir, "data")
	if err := CopyConfigJSON(src, dstDir); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dstDir, "config.json"))
	if err != nil || string(b) != `{"x":1}` {
		t.Fatalf("%s %v", b, err)
	}
}

func TestWaitHealthy_httptest(t *testing.T) {
	srv := runHealthServer(t)
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := WaitHealthy(ctx, srv.URL+"/health", time.Second, nil, "bifrost"); err != nil {
		t.Fatal(err)
	}
}

func TestStartBifrost_KillOnContext(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sleep not used on windows CI")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cfg := BifrostConfig{
		Bin:        "sleep",
		RawExec:    true,
		Args:       []string{"60"},
		ConfigJSON: writeFakeConfig(t),
		DataDir:    t.TempDir(),
		BindHost:   "127.0.0.1",
		Port:       65533,
	}
	cmd, err := StartBifrost(ctx, cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	waitDone := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(waitDone)
	}()
	select {
	case <-waitDone:
	case <-time.After(3 * time.Second):
		t.Fatal("process did not exit after context cancel")
	}
}

func writeFakeConfig(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "bifrost.config.json")
	if err := os.WriteFile(p, []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// TestStartBifrost_customStdout verifies optional Stdout/Stderr (tee / UI buffer integration).
func TestStartBifrost_customStdout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	var buf bytes.Buffer
	cfg := BifrostConfig{
		ConfigJSON: writeFakeConfig(t),
		DataDir:    t.TempDir(),
		BindHost:   "127.0.0.1",
		Port:       65532,
		RawExec:    true,
		Stdout:     &buf,
		Stderr:     &buf,
	}
	if runtime.GOOS == "windows" {
		cfg.Bin = "cmd"
		cfg.Args = []string{"/c", "echo", "bifrost-writer-test"}
	} else {
		cfg.Bin = "/bin/sh"
		cfg.Args = []string{"-c", "echo bifrost-writer-test"}
	}

	cmd, err := StartBifrost(ctx, cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "bifrost-writer-test") {
		t.Fatalf("stdout capture: %q", out)
	}
}
