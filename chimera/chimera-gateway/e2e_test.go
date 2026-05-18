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
	buildOnce      sync.Once
	buildErr       error
	wrapperBinPath string
	backendBinPath string
)

func ensureE2EBinaries(t *testing.T) (wrapperBin, backendBin string) {
	t.Helper()
	buildOnce.Do(func() {
		buildErr = buildE2EBinaries()
	})
	if buildErr != nil {
		t.Fatalf("build e2e binaries: %v", buildErr)
	}
	return wrapperBinPath, backendBinPath
}

func buildE2EBinaries() error {
	modRoot, err := findModuleRoot()
	if err != nil {
		return err
	}
	tmp, err := os.MkdirTemp("", "chimera-gateway-e2e-*")
	if err != nil {
		return err
	}
	ext := ""
	if runtime.GOOS == "windows" {
		ext = ".exe"
	}
	wrapperBinPath = filepath.Join(tmp, "chimera-gateway"+ext)
	backendBinPath = filepath.Join(tmp, "fake-gateway-backend"+ext)
	if err := runCmd(modRoot, "go", "build", "-o", wrapperBinPath, "./chimera/chimera-gateway"); err != nil {
		return fmt.Errorf("build gateway wrapper: %w", err)
	}
	fakeDir := filepath.Join(tmp, "fake-gateway-backend-src")
	if err := os.MkdirAll(fakeDir, 0o755); err != nil {
		return err
	}
	src := filepath.Join(fakeDir, "main.go")
	if err := os.WriteFile(src, []byte(fakeGatewayBackendSource), 0o644); err != nil {
		return err
	}
	if err := runCmd(fakeDir, "go", "build", "-o", backendBinPath, src); err != nil {
		return fmt.Errorf("build fake gateway backend: %w", err)
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

type gatewayProc struct {
	cmd    *exec.Cmd
	stdout bytes.Buffer
	stderr bytes.Buffer
}

func startGatewayProcess(t *testing.T, wrapperBin, backendBin string, args []string, extraEnv map[string]string) *gatewayProc {
	t.Helper()
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, naming.GatewayConfigFileTarget)
	tokensPath := filepath.Join(dir, "api-keys.yaml")
	routingPath := filepath.Join(dir, "routing-policy.yaml")
	if err := os.WriteFile(tokensPath, []byte("api_keys:\n  - secret: \"gw-secret\"\n    tenant_id: \"t1\"\n"), 0o644); err != nil {
		t.Fatalf("write tokens: %v", err)
	}
	if err := os.WriteFile(routingPath, []byte("rules: []\n"), 0o644); err != nil {
		t.Fatalf("write routing: %v", err)
	}
	raw := "gateway:\n  semver: \"0.1.0\"\n  listen_port: " + allocPort(t) + "\n  listen_host: \"127.0.0.1\"\n" +
		"upstream:\n  base_url: \"http://127.0.0.1:1\"\n  api_key_env: \"" + naming.EnvBrokerAPIKeyTarget + "\"\n" +
		"health:\n  timeout_ms: 1000\n  chat_timeout_ms: 60000\n" +
		"paths:\n  api_keys: \"" + strings.ReplaceAll(tokensPath, "\\", "/") + "\"\n  routing_policy: \"" + strings.ReplaceAll(routingPath, "\\", "/") + "\"\n" +
		"routing:\n  fallback_chain:\n    - \"fake/model\"\n"
	if err := os.WriteFile(cfgPath, []byte(raw), 0o644); err != nil {
		t.Fatalf("write gateway yaml: %v", err)
	}

	wlisten := "127.0.0.1:" + allocPort(t)
	glisten := "127.0.0.1:" + allocPort(t)
	cmdArgs := []string{
		"-listen", wlisten,
		"-config", cfgPath,
		"-gateway-listen", glisten,
		"-startup-timeout", "8s",
		"-terminate-wait", "1s",
	}
	if strings.TrimSpace(backendBin) != "" {
		cmdArgs = append(cmdArgs, "-bin", backendBin)
	}
	cmdArgs = append(cmdArgs, args...)
	cmd := exec.Command(wrapperBin, cmdArgs...)
	cmd.Env = append([]string{}, os.Environ()...)
	cmd.Env = append(cmd.Env, naming.EnvBrokerAPIKeyTarget+"=test-upstream-key", "FAKE_GATEWAY_CONFIG_PATH="+cfgPath)
	for k, v := range extraEnv {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	p := &gatewayProc{cmd: cmd}
	cmd.Stdout = &p.stdout
	cmd.Stderr = &p.stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("start gateway wrapper: %v", err)
	}
	return p
}

func allocPort(t *testing.T) string {
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

func stopGateway(t *testing.T, p *gatewayProc) {
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
	case <-time.After(8 * time.Second):
		_ = p.cmd.Process.Kill()
	}
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

func waitForProcessExit(t *testing.T, p *gatewayProc, timeout time.Duration) int {
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

func extractFlagValue(args []string, key string) string {
	out := ""
	for i := 0; i < len(args)-1; i++ {
		if args[i] == key {
			out = args[i+1]
		}
	}
	return out
}

func statusDoc(t *testing.T, baseURL string) map[string]any {
	t.Helper()
	resp, err := http.Get(baseURL + "/status")
	if err != nil {
		t.Fatalf("get status: %v", err)
	}
	defer resp.Body.Close()
	var doc map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	return doc
}

func TestE2E_Gateway_001_002_003_004_HappyStatusMetricsDebugDefault(t *testing.T) {
	wrapperBin, backendBin := ensureE2EBinaries(t)
	p := startGatewayProcess(t, wrapperBin, backendBin, nil, map[string]string{
		"FAKE_GATEWAY_START_READY": "1",
	})
	t.Cleanup(func() { stopGateway(t, p) })
	base := "http://" + extractFlagValue(p.cmd.Args, "-listen")

	waitForHTTPStatus(t, base+"/healthz", 200, 12*time.Second)
	waitForHTTPStatus(t, base+"/readyz", 200, 12*time.Second)

	doc := statusDoc(t, base)
	if doc["component"] != "chimera-gateway" || doc["backend_name"] != "custom" || doc["backend_mode"] != "binary" {
		t.Fatalf("status identity mismatch: %+v", doc)
	}
	if _, ok := doc["version"]; !ok {
		t.Fatalf("missing version object: %+v", doc)
	}
	met, err := http.Get(base + "/metrics")
	if err != nil {
		t.Fatalf("get metrics: %v", err)
	}
	defer met.Body.Close()
	raw, _ := io.ReadAll(met.Body)
	text := string(raw)
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
	dbg, err := http.Get(base + "/debug/broker/logs")
	if err != nil {
		t.Fatalf("get debug endpoint: %v", err)
	}
	defer dbg.Body.Close()
	if dbg.StatusCode != http.StatusNotFound {
		t.Fatalf("debug endpoint should be disabled by default, got %d", dbg.StatusCode)
	}
}

func TestE2E_Gateway_005_DebugEnabledRedaction(t *testing.T) {
	wrapperBin, backendBin := ensureE2EBinaries(t)
	p := startGatewayProcess(t, wrapperBin, backendBin, []string{
		"-debug-enable-broker-logs",
	}, map[string]string{
		"FAKE_GATEWAY_START_READY":   "1",
		"FAKE_GATEWAY_STDOUT_SECRET": "TOKEN=my-secret-value",
	})
	t.Cleanup(func() { stopGateway(t, p) })
	base := "http://" + extractFlagValue(p.cmd.Args, "-listen")
	waitForHTTPStatus(t, base+"/readyz", 200, 12*time.Second)

	resp, err := http.Get(base + "/debug/broker/logs")
	if err != nil {
		t.Fatalf("get debug logs: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("debug logs status %d", resp.StatusCode)
	}
	var doc map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		t.Fatalf("decode debug logs: %v", err)
	}
	lines, _ := doc["lines"].([]any)
	if len(lines) == 0 {
		t.Fatalf("expected upstream lines in debug logs")
	}
	all := fmt.Sprint(lines)
	if strings.Contains(all, "my-secret-value") {
		t.Fatalf("secret should be redacted: %s", all)
	}
	foundWrapped := false
	for _, line := range lines {
		s := fmt.Sprint(line)
		if strings.Contains(s, `"service":"chimera-gateway"`) && strings.Contains(s, `"msg":"gateway.`) {
			foundWrapped = true
			break
		}
	}
	if !foundWrapped {
		t.Fatalf("expected wrapped gateway structured line(s), got: %s", all)
	}
}

func TestE2E_Gateway_006_DebugBindSafetyRejectsRemoteWithoutOverride(t *testing.T) {
	wrapperBin, backendBin := ensureE2EBinaries(t)
	p := startGatewayProcess(t, wrapperBin, backendBin, []string{
		"-listen", "0.0.0.0:" + allocPort(t),
		"-debug-enable-broker-logs",
	}, map[string]string{"FAKE_GATEWAY_START_READY": "1"})
	code := waitForProcessExit(t, p, 8*time.Second)
	if code != 10 {
		t.Fatalf("expected config exit 10, got %d", code)
	}
}

func TestE2E_Gateway_007_DebugBindOverrideAllowsRemote(t *testing.T) {
	if runtime.GOOS == "windows" && strings.TrimSpace(os.Getenv("CHIMERA_E2E_REMOTE_BIND")) != "1" {
		t.Skip("skipping remote bind test on windows by default to avoid firewall prompt; set CHIMERA_E2E_REMOTE_BIND=1 to run")
	}
	wrapperBin, backendBin := ensureE2EBinaries(t)
	p := startGatewayProcess(t, wrapperBin, backendBin, []string{
		"-listen", "0.0.0.0:" + allocPort(t),
		"-debug-enable-broker-logs",
		"-debug-allow-remote",
	}, map[string]string{"FAKE_GATEWAY_START_READY": "1"})
	t.Cleanup(func() { stopGateway(t, p) })
	base := "http://127.0.0.1:" + strings.Split(extractFlagValue(p.cmd.Args, "-listen"), ":")[1]
	waitForHTTPStatus(t, base+"/healthz", 200, 12*time.Second)
}

func TestE2E_Gateway_008_StartupTimeoutExit20(t *testing.T) {
	wrapperBin, backendBin := ensureE2EBinaries(t)
	p := startGatewayProcess(t, wrapperBin, backendBin, []string{
		"-startup-timeout", "1s",
	}, map[string]string{"FAKE_GATEWAY_START_READY": "0"})
	code := waitForProcessExit(t, p, 10*time.Second)
	if code != 20 {
		t.Fatalf("expected startup failure exit 20, got %d", code)
	}
}

func TestE2E_Gateway_009_RuntimeDegradedWhenBackendUnready(t *testing.T) {
	wrapperBin, backendBin := ensureE2EBinaries(t)
	p := startGatewayProcess(t, wrapperBin, backendBin, nil, map[string]string{"FAKE_GATEWAY_START_READY": "1"})
	t.Cleanup(func() { stopGateway(t, p) })
	base := "http://" + extractFlagValue(p.cmd.Args, "-listen")
	waitForHTTPStatus(t, base+"/readyz", 200, 12*time.Second)
	backendBase := "http://" + extractFlagValue(p.cmd.Args, "-gateway-listen")
	if _, err := http.Get(backendBase + "/admin/ready?value=0"); err != nil {
		t.Fatalf("set backend unready: %v", err)
	}
	waitForHTTPStatus(t, base+"/readyz", 503, 12*time.Second)
	doc := statusDoc(t, base)
	if doc["status"] != "degraded" {
		t.Fatalf("expected degraded status: %+v", doc)
	}
}

func TestE2E_Gateway_010_SupervisorBinIsIgnoredAndGatewayStillRuns(t *testing.T) {
	wrapperBin, _ := ensureE2EBinaries(t)
	p := startGatewayProcess(t, wrapperBin, "chimera-supervisor", nil, map[string]string{
		"FAKE_GATEWAY_START_READY": "1",
	})
	t.Cleanup(func() { stopGateway(t, p) })
	base := "http://" + extractFlagValue(p.cmd.Args, "-listen")
	waitForHTTPStatus(t, base+"/readyz", 200, 12*time.Second)
}

const fakeGatewayBackendSource = `package main

import (
	"encoding/json"
	"flag"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
)

func envBool(k string) bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv(k)))
	return v == "1" || v == "true" || v == "yes" || v == "on"
}

func main() {
	var listen string
	flag.StringVar(&listen, "listen", "127.0.0.1:0", "")
	var _config string
	flag.StringVar(&_config, "config", "", "")
	flag.Parse()
	var ready uint32
	if envBool("FAKE_GATEWAY_START_READY") {
		atomic.StoreUint32(&ready, 1)
	}
	if s := strings.TrimSpace(os.Getenv("FAKE_GATEWAY_STDOUT_SECRET")); s != "" {
		_, _ = os.Stdout.WriteString(s + "\n")
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		if atomic.LoadUint32(&ready) == 1 {
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "ok"})
			return
		}
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "degraded"})
	})
	mux.HandleFunc("/v1/models", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"object": "list",
			"data": []map[string]any{{"id": "fake/model", "object": "model", "owned_by": "fake"}},
		})
	})
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		_, _ = w.Write([]byte("# HELP req_total requests\n# TYPE req_total counter\nreq_total{code=\"200\"} 1\n"))
	})
	mux.HandleFunc("/admin/ready", func(w http.ResponseWriter, r *http.Request) {
		val := r.URL.Query().Get("value")
		b := val == "1" || strings.EqualFold(val, "true")
		if b {
			atomic.StoreUint32(&ready, 1)
		} else {
			atomic.StoreUint32(&ready, 0)
		}
		_, _ = w.Write([]byte("ok"))
	})
	_ = http.ListenAndServe(listen, mux)
}
`
