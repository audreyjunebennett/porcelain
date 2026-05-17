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
)

var (
	supervisorBuildOnce sync.Once
	supervisorBuildErr  error
	supervisorBinPath   string
	fakeWrapperBinPath  string
)

func ensureSupervisorE2EBinaries(t *testing.T) (supervisorBin, fakeWrapperBin string) {
	t.Helper()
	supervisorBuildOnce.Do(func() {
		supervisorBuildErr = buildSupervisorE2EBinaries()
	})
	if supervisorBuildErr != nil {
		t.Fatalf("build supervisor e2e binaries: %v", supervisorBuildErr)
	}
	return supervisorBinPath, fakeWrapperBinPath
}

func buildSupervisorE2EBinaries() error {
	modRoot, err := findModuleRoot()
	if err != nil {
		return err
	}
	tmp, err := os.MkdirTemp("", "chimera-supervisor-e2e-*")
	if err != nil {
		return err
	}
	ext := ""
	if runtime.GOOS == "windows" {
		ext = ".exe"
	}
	supervisorBinPath = filepath.Join(tmp, "chimera-supervisor"+ext)
	fakeWrapperBinPath = filepath.Join(tmp, "fake-wrapper"+ext)
	if err := runCmd(modRoot, "go", "build", "-o", supervisorBinPath, "./chimera/chimera-supervisor"); err != nil {
		return fmt.Errorf("build supervisor: %w", err)
	}
	fakeDir := filepath.Join(tmp, "fake-wrapper-src")
	if err := os.MkdirAll(fakeDir, 0o755); err != nil {
		return err
	}
	src := filepath.Join(fakeDir, "main.go")
	if err := os.WriteFile(src, []byte(fakeWrapperSource), 0o644); err != nil {
		return err
	}
	if err := runCmd(fakeDir, "go", "build", "-o", fakeWrapperBinPath, src); err != nil {
		return fmt.Errorf("build fake wrapper: %w", err)
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

type supervisorProc struct {
	cmd    *exec.Cmd
	stdout bytes.Buffer
	stderr bytes.Buffer
}

func startSupervisorProcess(t *testing.T, supervisorBin, fakeWrapper string, args []string, extraEnv map[string]string) *supervisorProc {
	t.Helper()
	dir := t.TempDir()
	gatewayPath := filepath.Join(dir, "gateway.yaml")
	tokensPath := filepath.Join(dir, "api-keys.yaml")
	routingPath := filepath.Join(dir, "routing-policy.yaml")
	if err := os.WriteFile(tokensPath, []byte("api_keys:\n  - secret: \"tok\"\n    tenant_id: \"t\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(routingPath, []byte("rules: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	raw := "gateway:\n  semver: \"0.1.0\"\n  listen_port: 0\n  listen_host: \"127.0.0.1\"\n" +
		"upstream:\n  base_url: \"http://127.0.0.1:8080\"\n  api_key_env: \"CHIMERA_UPSTREAM_API_KEY\"\n" +
		"health:\n  timeout_ms: 1000\n  chat_timeout_ms: 60000\n" +
		"paths:\n  tokens: \"" + strings.ReplaceAll(tokensPath, "\\", "/") + "\"\n  routing_policy: \"" + strings.ReplaceAll(routingPath, "\\", "/") + "\"\n" +
		"routing:\n  fallback_chain:\n    - \"fake/model\"\n"
	if err := os.WriteFile(gatewayPath, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}

	listen := "127.0.0.1:" + allocPort(t)
	gatewayListen := "127.0.0.1:" + allocPort(t)
	brokerListen := "127.0.0.1:" + allocPort(t)
	vectorstoreListen := "127.0.0.1:" + allocPort(t)
	base := []string{
		"-config", gatewayPath,
		"-listen", listen,
		"-gateway-bin", fakeWrapper,
		"-gateway-listen", gatewayListen,
		"-wait-gateway", "5s",
		"-broker-bin", fakeWrapper,
		"-broker-listen", brokerListen,
		"-broker-endpoint", "127.0.0.1:" + allocPort(t),
		"-broker-data-dir", filepath.Join(dir, "broker-data"),
		"-vectorstore-bin", fakeWrapper,
		"-vectorstore-listen", vectorstoreListen,
		"-vectorstore-endpoint", "127.0.0.1:" + allocPort(t),
		"-vectorstore-data-path", filepath.Join(dir, "vectorstore-data"),
		"-wait-broker", "5s",
		"-wait-vectorstore", "5s",
	}
	base = append(base, args...)

	cmd := exec.Command(supervisorBin, base...)
	cmd.Env = append([]string{}, os.Environ()...)
	for k, v := range extraEnv {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	p := &supervisorProc{cmd: cmd}
	cmd.Stdout = &p.stdout
	cmd.Stderr = &p.stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("start supervisor: %v", err)
	}
	return p
}

func stopSupervisor(t *testing.T, p *supervisorProc) {
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
		return
	case <-time.After(8 * time.Second):
		_ = p.cmd.Process.Kill()
	}
}

func waitForStatus(t *testing.T, u string, code int, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		resp, err := http.Get(u)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == code {
				return
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("wait for %s status %d timed out", u, code)
}

func waitForProcessExitSupervisor(t *testing.T, p *supervisorProc, timeout time.Duration) int {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- p.cmd.Wait() }()
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
		_ = p.cmd.Process.Kill()
		t.Fatalf("process did not exit in %v", timeout)
	}
	return -1
}

func allocPort(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	_, p, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func flagValue(args []string, key string) string {
	out := ""
	for i := 0; i < len(args)-1; i++ {
		if args[i] == key {
			out = args[i+1]
		}
	}
	return out
}

func TestE2E_Supervisor_001_HealthReadyStatusMetrics(t *testing.T) {
	supervisorBin, fakeWrapper := ensureSupervisorE2EBinaries(t)
	p := startSupervisorProcess(t, supervisorBin, fakeWrapper, nil, map[string]string{
		"FAKE_WRAPPER_START_READY": "1",
	})
	t.Cleanup(func() { stopSupervisor(t, p) })

	base := "http://" + flagValue(p.cmd.Args, "-listen")
	waitForStatus(t, base+"/healthz", http.StatusOK, 10*time.Second)
	waitForStatus(t, base+"/readyz", http.StatusOK, 10*time.Second)
	waitForStatus(t, base+"/status", http.StatusOK, 10*time.Second)

	resp, err := http.Get(base + "/status")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var doc map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		t.Fatal(err)
	}
	if doc["component"] != "chimera-supervisor" {
		t.Fatalf("component=%v", doc["component"])
	}
	if doc["backend_name"] != "custom" || doc["backend_mode"] != "binary" {
		t.Fatalf("backend identity mismatch: %+v", doc)
	}

	met, err := http.Get(base + "/metrics")
	if err != nil {
		t.Fatal(err)
	}
	defer met.Body.Close()
	b, _ := io.ReadAll(met.Body)
	text := string(b)
	for _, m := range []string{
		"chimera_wrapper_up",
		"chimera_backend_up",
		"chimera_backend_restarts_total",
		"chimera_requests_total",
		"chimera_request_duration_seconds",
	} {
		if !strings.Contains(text, m) {
			t.Fatalf("metrics missing %s\n%s", m, text)
		}
	}
}

func TestE2E_Supervisor_002_ReadyzDegradedWhenBrokerUnready(t *testing.T) {
	supervisorBin, fakeWrapper := ensureSupervisorE2EBinaries(t)
	p := startSupervisorProcess(t, supervisorBin, fakeWrapper, nil, map[string]string{
		"FAKE_WRAPPER_START_READY": "0",
	})
	t.Cleanup(func() { stopSupervisor(t, p) })
	base := "http://" + flagValue(p.cmd.Args, "-listen")
	waitForStatus(t, base+"/healthz", http.StatusOK, 10*time.Second)
	waitForStatus(t, base+"/readyz", http.StatusServiceUnavailable, 10*time.Second)
	waitForStatus(t, base+"/status", http.StatusServiceUnavailable, 10*time.Second)
}

func TestE2E_Supervisor_003_BrokerStartFailureExitsNonZero(t *testing.T) {
	supervisorBin, fakeWrapper := ensureSupervisorE2EBinaries(t)
	p := startSupervisorProcess(t, supervisorBin, fakeWrapper, nil, map[string]string{
		"FAKE_WRAPPER_FAIL_START": "1",
	})
	code := waitForProcessExitSupervisor(t, p, 8*time.Second)
	if code == 0 {
		t.Fatalf("expected non-zero exit when broker wrapper fails start")
	}
}

const fakeWrapperSource = `package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"time"
)

func envBool(k string) bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv(k)))
	return v == "1" || v == "true" || v == "yes" || v == "on"
}

func main() {
	if envBool("FAKE_WRAPPER_FAIL_START") {
		fmt.Fprintln(os.Stderr, "forced startup failure")
		os.Exit(20)
	}
	var listen string
	var _bin string
	var _endpoint string
	var _data string
	var _config string
	var _upstream string
	fs := flag.NewFlagSet("fake-wrapper", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fs.StringVar(&listen, "listen", "127.0.0.1:0", "")
	fs.StringVar(&_bin, "bin", "", "")
	fs.StringVar(&_endpoint, "endpoint", "", "")
	fs.StringVar(&_data, "data-path", "", "")
	fs.StringVar(&_config, "config", "", "")
	fs.StringVar(&_upstream, "upstream-override", "", "")
	_ = fs.Bool("debug-forward-upstream", false, "")
	_ = fs.Parse(os.Args[1:])

	var ready uint32
	if envBool("FAKE_WRAPPER_START_READY") {
		atomic.StoreUint32(&ready, 1)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		if atomic.LoadUint32(&ready) == 1 {
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
			return
		}
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "degraded"})
	})
	mux.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		status := "degraded"
		if atomic.LoadUint32(&ready) == 1 {
			status = "ok"
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"component": "fake-wrapper",
			"status": status,
			"timestamp": time.Now().UTC().Format(time.RFC3339),
		})
	})
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		_, _ = w.Write([]byte("chimera_wrapper_up 1\n"))
	})

	srv := &http.Server{Addr: listen, Handler: mux}
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		os.Exit(1)
	}
}
`
