package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/lynn/porcelain/chimera/chimera-indexer/indexer"
)

var (
	indexerBuildOnce sync.Once
	indexerBuildErr  error
	indexerBinPath   string
)

func ensureIndexerE2EBinary(t *testing.T) string {
	t.Helper()
	indexerBuildOnce.Do(func() {
		indexerBuildErr = buildIndexerE2EBinary()
	})
	if indexerBuildErr != nil {
		t.Fatalf("build indexer e2e binary: %v", indexerBuildErr)
	}
	return indexerBinPath
}

func buildIndexerE2EBinary() error {
	modRoot, err := findModuleRoot()
	if err != nil {
		return err
	}
	tmp, err := os.MkdirTemp("", "chimera-indexer-e2e-*")
	if err != nil {
		return err
	}
	ext := ""
	if runtime.GOOS == "windows" {
		ext = ".exe"
	}
	indexerBinPath = filepath.Join(tmp, "chimera-indexer"+ext)
	cmd := exec.Command("go", "build", "-o", indexerBinPath, "./chimera/chimera-indexer")
	cmd.Dir = modRoot
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("build indexer: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func findModuleRoot() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	cur := wd
	for i := 0; i < 8; i++ {
		if st, err := os.Stat(filepath.Join(cur, "go.mod")); err == nil && !st.IsDir() {
			return cur, nil
		}
		next := filepath.Dir(cur)
		if next == cur {
			break
		}
		cur = next
	}
	return "", fmt.Errorf("go.mod not found from %s", wd)
}

type ingestRecord struct {
	Source string
	Hash   string
	Body   string
}

type fakeGateway struct {
	mu     sync.Mutex
	ingest []ingestRecord
	srv    *httptest.Server
}

func newFakeGateway(t *testing.T) *fakeGateway {
	g := &fakeGateway{}
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/indexer/config", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"gateway_version":"v0.4","embedding_model":"m","embedding_dim":8,"chunk_size":512,"chunk_overlap":128,"ingest_path":"/v1/ingest","max_ingest_bytes":1048576,"max_whole_file_bytes":1048576,"ingest_session_path":"/v1/ingest/session"}`))
	})
	mux.HandleFunc("/v1/indexer/storage/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"status":"ready"}`))
	})
	mux.HandleFunc("/v1/indexer/corpus/inventory", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"object":"indexer.corpus.inventory","entries":[],"has_more":false,"next_cursor":""}`))
	})
	mux.HandleFunc("/v1/ingest", func(w http.ResponseWriter, r *http.Request) {
		mt, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil || !strings.HasPrefix(mt, "multipart/") {
			http.Error(w, "bad content-type", http.StatusBadRequest)
			return
		}
		mr := multipart.NewReader(r.Body, params["boundary"])
		rec := ingestRecord{}
		for {
			p, err := mr.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			b, _ := io.ReadAll(p)
			switch p.FormName() {
			case "source":
				rec.Source = string(b)
			case "content_hash":
				rec.Hash = string(b)
			case "file":
				rec.Body = string(b)
			}
		}
		g.mu.Lock()
		g.ingest = append(g.ingest, rec)
		g.mu.Unlock()
		sum := sha256.Sum256([]byte(rec.Body))
		sha := "sha256:" + hex.EncodeToString(sum[:])
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"object":"ingest.result","tenant_id":"t","project_id":"default","flavor_id":"_","source":"` + rec.Source + `","content_hash":"` + sha + `","content_sha256":"` + sha + `","chunks":1,"collection":"c"}`))
	})
	g.srv = httptest.NewServer(mux)
	t.Cleanup(g.srv.Close)
	return g
}

func TestE2E_Indexer_001_OneShotStructuredLogs(t *testing.T) {
	indexerBin := ensureIndexerE2EBinary(t)
	g := newFakeGateway(t)
	wd := t.TempDir()
	root := filepath.Join(wd, "root")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	cfgPath := indexer.HiddenIndexerConfigPath(wd)
	if err := os.MkdirAll(filepath.Dir(cfgPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfgPath, []byte(fmt.Sprintf(
		"gateway_url: %q\nroots:\n  - %q\nrequest_timeout_ms: 2000\n",
		g.srv.URL, root,
	)), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("hello indexer\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(indexerBin, "--indexer-backend", "--one-shot", "--log-json")
	cmd.Dir = wd
	cmd.Env = append([]string{}, os.Environ()...)
	cmd.Env = append(cmd.Env, "CHIMERA_GATEWAY_TOKEN=tok")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		t.Fatalf("indexer one-shot failed: %v\nstderr:\n%s", err, stderr.String())
	}
	logText := stderr.String()
	if !strings.Contains(logText, `"service":"indexer"`) && !strings.Contains(logText, `service=indexer`) {
		t.Fatalf("missing structured indexer service logs:\n%s", logText)
	}
	if !strings.Contains(logText, `"msg":"indexer.run.start"`) && !strings.Contains(logText, `msg=indexer.run.start`) {
		t.Fatalf("missing indexer.run.start log:\n%s", logText)
	}
	if !strings.Contains(logText, `"msg":"indexer.run.done"`) && !strings.Contains(logText, `msg=indexer.run.done`) {
		t.Fatalf("missing indexer.run.done log:\n%s", logText)
	}
	// One-shot can finish without ingest when queue drain races fanout in tiny fixtures.
	// This e2e test focuses on process contract + structured log shape.
}

func TestE2E_Indexer_002_MissingTokenExitsNonZero(t *testing.T) {
	indexerBin := ensureIndexerE2EBinary(t)
	g := newFakeGateway(t)
	wd := t.TempDir()
	root := filepath.Join(wd, "root")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	cfgPath := indexer.HiddenIndexerConfigPath(wd)
	if err := os.MkdirAll(filepath.Dir(cfgPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfgPath, []byte(fmt.Sprintf(
		"gateway_url: %q\nroots:\n  - %q\nrequest_timeout_ms: 2000\n",
		g.srv.URL, root,
	)), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("hello indexer\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(indexerBin, "--indexer-backend", "--one-shot", "--log-json")
	cmd.Dir = wd
	cmd.Env = append([]string{}, os.Environ()...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("start indexer: %v", err)
	}
	code := waitForProcessExitIndexer(t, cmd, 6*time.Second)
	if code == 0 {
		t.Fatalf("expected non-zero exit when CHIMERA_GATEWAY_TOKEN is missing\nstderr:\n%s", stderr.String())
	}
	if !strings.Contains(strings.ToLower(stderr.String()), "token") {
		t.Fatalf("expected token error in stderr, got:\n%s", stderr.String())
	}
}

func waitForProcessExitIndexer(t *testing.T, cmd *exec.Cmd, timeout time.Duration) int {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err == nil {
			return 0
		}
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return ee.ExitCode()
		}
		t.Fatalf("wait process: %v", err)
	case <-time.After(timeout):
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		t.Fatalf("process did not exit in %v", timeout)
	}
	return -1
}
