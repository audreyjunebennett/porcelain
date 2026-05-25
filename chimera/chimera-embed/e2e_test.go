package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/lynn/porcelain/internal/naming"
)

var (
	buildOnce     sync.Once
	buildErr      error
	embedBinPath  string
	fakeLlamaPath string
)

func ensureE2EBinaries(t *testing.T) (embedBin, fakeBin string) {
	t.Helper()
	buildOnce.Do(func() {
		buildErr = buildE2EBinaries()
	})
	if buildErr != nil {
		t.Fatalf("build e2e binaries: %v", buildErr)
	}
	return embedBinPath, fakeLlamaPath
}

func buildE2EBinaries() error {
	modRoot, err := findModuleRoot()
	if err != nil {
		return err
	}
	tmp, err := os.MkdirTemp("", naming.ProductEmbedName+"-e2e-*")
	if err != nil {
		return err
	}
	ext := ""
	if runtime.GOOS == "windows" {
		ext = ".exe"
	}
	embedBinPath = filepath.Join(tmp, naming.ProductEmbedName+ext)
	fakeLlamaPath = filepath.Join(tmp, "fake-"+naming.ProductLlamaServerBinName+ext)
	if err := runCmd(modRoot, "go", "build", "-o", embedBinPath, "./chimera/chimera-embed"); err != nil {
		return fmt.Errorf("build embed: %w", err)
	}
	fakeSrc := filepath.Join(modRoot, "chimera", "chimera-embed", "testdata", "fakellamaserver")
	if err := runCmd(fakeSrc, "go", "build", "-o", fakeLlamaPath, "."); err != nil {
		return fmt.Errorf("build fake llama-server: %w", err)
	}
	return nil
}

func runCmd(dir string, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s %v failed: %w: %s", name, args, err, strings.TrimSpace(string(out)))
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

type embedProc struct {
	cmd    *exec.Cmd
	stdout bytes.Buffer
	stderr bytes.Buffer
}

func stopEmbed(t *testing.T, p *embedProc) {
	t.Helper()
	if p == nil || p.cmd == nil || p.cmd.Process == nil {
		return
	}
	if p.cmd.ProcessState != nil && p.cmd.ProcessState.Exited() {
		return
	}
	done := make(chan error, 1)
	go func() { done <- p.cmd.Wait() }()
	if runtime.GOOS == "windows" {
		_ = exec.Command("taskkill", "/T", "/F", "/PID", fmt.Sprintf("%d", p.cmd.Process.Pid)).Run()
	} else {
		_ = p.cmd.Process.Signal(os.Interrupt)
	}
	select {
	case <-done:
	case <-time.After(6 * time.Second):
		_ = p.cmd.Process.Kill()
	}
}

func startEmbedProcess(t *testing.T, embedBin, fakeBin string, extraEnv map[string]string) *embedProc {
	t.Helper()
	dataDir := t.TempDir()
	modelPath := filepath.Join(dataDir, "model.gguf")
	if err := os.WriteFile(modelPath, []byte("fake-gguf"), 0o644); err != nil {
		t.Fatal(err)
	}
	listen := "127.0.0.1:" + strconvPort(t)
	backendPort := strconvPort(t)
	cmd := exec.Command(embedBin,
		"-listen", listen,
		"-bin", fakeBin,
		"-model-path", modelPath,
		"-cache-dir", dataDir,
		"-endpoint", "127.0.0.1:"+backendPort,
		"-startup-timeout", "8s",
		"-terminate-wait", "1s",
	)
	cmd.Env = append([]string{}, os.Environ()...)
	for k, v := range extraEnv {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	p := &embedProc{cmd: cmd}
	cmd.Stdout = &p.stdout
	cmd.Stderr = &p.stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("start embed: %v", err)
	}
	return p
}

func strconvPort(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("alloc port: %v", err)
	}
	defer ln.Close()
	_, p, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		t.Fatalf("split host port: %v", err)
	}
	return p
}

func waitForHTTPStatus(t *testing.T, u string, code int, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		resp, err := http.Get(u)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == code {
				return
			}
			lastErr = fmt.Errorf("status %d", resp.StatusCode)
		} else {
			lastErr = err
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("wait for %s status %d timed out: %v", u, code, lastErr)
}

func TestE2E_Embed_HappyPathAndEmbeddings(t *testing.T) {
	embedBin, fakeBin := ensureE2EBinaries(t)
	p := startEmbedProcess(t, embedBin, fakeBin, map[string]string{"FAKE_LLAMA_START_READY": "1"})
	t.Cleanup(func() { stopEmbed(t, p) })

	listen := ""
	for i := 0; i < len(p.cmd.Args)-1; i++ {
		if p.cmd.Args[i] == "-listen" {
			listen = p.cmd.Args[i+1]
		}
	}
	base := "http://" + listen
	waitForHTTPStatus(t, base+"/readyz", 200, 10*time.Second)

	endpoint := ""
	for i := 0; i < len(p.cmd.Args)-1; i++ {
		if p.cmd.Args[i] == "-endpoint" {
			endpoint = p.cmd.Args[i+1]
		}
	}
	body := `{"model":"internal/nomic-embed-text","input":["hello"],"encoding_format":"float"}`
	resp, err := http.Post("http://"+endpoint+"/v1/embeddings", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatalf("embed POST: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("embed status %d: %s", resp.StatusCode, raw)
	}
	var doc map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		t.Fatalf("decode: %v", err)
	}
	data, _ := doc["data"].([]any)
	if len(data) != 1 {
		t.Fatalf("data len: %+v", doc)
	}
	row, _ := data[0].(map[string]any)
	emb, _ := row["embedding"].([]any)
	if len(emb) != 768 {
		t.Fatalf("dim: got %d want 768", len(emb))
	}
}

func TestE2E_Embed_StartupTimeoutExit20(t *testing.T) {
	embedBin, fakeBin := ensureE2EBinaries(t)
	p := startEmbedProcess(t, embedBin, fakeBin, map[string]string{"FAKE_LLAMA_START_READY": "0"})
	done := make(chan error, 1)
	go func() { done <- p.cmd.Wait() }()
	select {
	case err := <-done:
		var ee *exec.ExitError
		if !errors.As(err, &ee) || ee.ExitCode() != 20 {
			t.Fatalf("expected exit 20, got %v", err)
		}
	case <-time.After(12 * time.Second):
		_ = p.cmd.Process.Kill()
		t.Fatal("timeout waiting for startup failure")
	}
}
