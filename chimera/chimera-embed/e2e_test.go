package main

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestE2EEmbedWrapperServesSupervisedEmbeddings(t *testing.T) {
	moduleRoot, err := findModuleRoot()
	if err != nil {
		t.Fatal(err)
	}
	buildDir := t.TempDir()
	ext := ""
	if runtime.GOOS == "windows" {
		ext = ".exe"
	}
	embedBin := filepath.Join(buildDir, "chimera-embed"+ext)
	fakeBin := filepath.Join(buildDir, "llama-server"+ext)
	buildForE2E(t, moduleRoot, embedBin, "./chimera/chimera-embed")
	buildForE2E(t, moduleRoot, fakeBin, "./chimera/chimera-embed/testdata/fakellamaserver")
	modelPath := filepath.Join(buildDir, "nomic-embed-text.gguf")
	if err := os.WriteFile(modelPath, []byte("fake"), 0o644); err != nil {
		t.Fatal(err)
	}
	wrapperAddr := allocateAddr(t)
	backendAddr := allocateAddr(t)
	cmd := exec.Command(embedBin,
		"-listen", wrapperAddr,
		"-bin", fakeBin,
		"-endpoint", backendAddr,
		"-model-path", modelPath,
		"-cache-dir", filepath.Join(buildDir, "cache"),
		"-startup-timeout", "8s",
		"-terminate-wait", "1s",
	)
	cmd.Env = append(os.Environ(), "CHIMERA_LOG_JSON=1")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { stopE2EProcess(cmd) })
	waitForStatus(t, "http://"+wrapperAddr+"/readyz", http.StatusOK, 10*time.Second)

	response, err := http.Post("http://"+backendAddr+"/v1/embeddings", "application/json", strings.NewReader(`{"model":"internal/nomic-embed-text","input":["hello"]}`))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var body struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if len(body.Data) != 1 || len(body.Data[0].Embedding) != 3 {
		t.Fatalf("unexpected embedding response: %+v", body)
	}

	response, err = http.Get("http://" + wrapperAddr + "/status")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var status map[string]any
	if err := json.NewDecoder(response.Body).Decode(&status); err != nil {
		t.Fatal(err)
	}
	if status["component"] != "chimera-embed" || status["backend_name"] != "llama-server" || status["status"] != "ok" {
		t.Fatalf("unexpected status: %+v", status)
	}
}

func findModuleRoot() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for current, i := wd, 0; i < 8; i++ {
		if info, statErr := os.Stat(filepath.Join(current, "go.mod")); statErr == nil && !info.IsDir() {
			return current, nil
		}
		next := filepath.Dir(current)
		if next == current {
			break
		}
		current = next
	}
	return "", fmt.Errorf("go.mod not found from %s", wd)
}

func buildForE2E(t *testing.T, root, output, pkg string) {
	t.Helper()
	cmd := exec.Command("go", "build", "-o", output, pkg)
	cmd.Dir = root
	if combined, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build %s: %v: %s", pkg, err, combined)
	}
}

func allocateAddr(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	return listener.Addr().String()
}

func waitForStatus(t *testing.T, url string, want int, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		response, err := http.Get(url)
		if err == nil {
			_ = response.Body.Close()
			if response.StatusCode == want {
				return
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", url)
}

func stopE2EProcess(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil || cmd.ProcessState != nil {
		return
	}
	if runtime.GOOS == "windows" {
		_ = exec.Command("taskkill", "/T", "/F", "/PID", fmt.Sprintf("%d", cmd.Process.Pid)).Run()
	} else {
		_ = cmd.Process.Signal(os.Interrupt)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		_ = cmd.Process.Kill()
	}
}
