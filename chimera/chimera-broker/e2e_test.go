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
	buildOnce                sync.Once
	buildErr                 error
	brokerBinPath            string
	fakeChimeraBrokerBinPath string
)

func ensureE2EBinaries(t *testing.T) (brokerBin, fakeBin string) {
	t.Helper()
	buildOnce.Do(func() {
		buildErr = buildE2EBinaries()
	})
	if buildErr != nil {
		t.Fatalf("build e2e binaries: %v", buildErr)
	}
	return brokerBinPath, fakeChimeraBrokerBinPath
}

func buildE2EBinaries() error {
	modRoot, err := findModuleRoot()
	if err != nil {
		return err
	}
	tmp, err := os.MkdirTemp("", naming.ProductBrokerName+"-e2e-*")
	if err != nil {
		return err
	}
	ext := ""
	if runtime.GOOS == "windows" {
		ext = ".exe"
	}
	brokerBinPath = filepath.Join(tmp, naming.ProductBrokerName+ext)
	fakeChimeraBrokerBinPath = filepath.Join(tmp, "fake-"+naming.ProductBrokerName+ext)
	if err := runCmd(modRoot, "go", "build", "-o", brokerBinPath, "./chimera/chimera-broker"); err != nil {
		return fmt.Errorf("build broker: %w", err)
	}
	fakeSrc := filepath.Join(modRoot, "chimera", "chimera-broker", "testdata", "fakechimerabroker")
	if err := runCmd(fakeSrc, "go", "build", "-o", fakeChimeraBrokerBinPath, "."); err != nil {
		return fmt.Errorf("build fake chimera-broker: %w", err)
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

type brokerProc struct {
	cmd    *exec.Cmd
	stdout bytes.Buffer
	stderr bytes.Buffer
}

func stopBroker(t *testing.T, p *brokerProc) {
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
	case <-time.After(6 * time.Second):
		_ = p.cmd.Process.Kill()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Log("broker process still running after forced kill timeout")
		}
	}
}

func startBrokerProcess(t *testing.T, brokerBin, fakeBin string, args []string, extraEnv map[string]string) *brokerProc {
	t.Helper()
	cfgPath := filepath.Join(t.TempDir(), "chimera-broker.config.json")
	if err := os.WriteFile(cfgPath, []byte(`{}`), 0o644); err != nil {
		t.Fatalf("write fake chimera-broker config: %v", err)
	}
	dataDir := t.TempDir()
	listen := "127.0.0.1:" + strconvPort(t)
	chimeraBrokerPort := strconvPort(t)

	base := []string{
		"-listen", listen,
		"-bin", fakeBin,
		"-chimera-broker-config", cfgPath,
		"-data-path", dataDir,
		"-endpoint", "127.0.0.1:" + chimeraBrokerPort,
		"-startup-timeout", "5s",
		"-terminate-wait", "1s",
	}
	base = append(base, args...)
	cmd := exec.Command(brokerBin, base...)
	cmd.Env = append([]string{}, os.Environ()...)
	for k, v := range extraEnv {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	p := &brokerProc{cmd: cmd}
	cmd.Stdout = &p.stdout
	cmd.Stderr = &p.stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("start broker: %v", err)
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

func waitForProcessExit(t *testing.T, p *brokerProc, timeout time.Duration) int {
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

func TestE2E_Broker_001_002_003_004_HappyStatusMetricsDebugDefault(t *testing.T) {
	brokerBin, fakeBin := ensureE2EBinaries(t)
	p := startBrokerProcess(t, brokerBin, fakeBin, nil, map[string]string{
		"FAKE_CHIMERA_BROKER_START_READY": "1",
	})
	t.Cleanup(func() { stopBroker(t, p) })
	listen := extractFlagValue(p.cmd.Args, "-listen")
	base := "http://" + listen

	waitForHTTPStatus(t, base+"/healthz", 200, 8*time.Second)
	waitForHTTPStatus(t, base+"/readyz", 200, 8*time.Second)

	doc := statusDoc(t, base)
	if doc["component"] != naming.ProductBrokerName || doc["backend_name"] != naming.ProductBrokerName || doc["backend_mode"] != "binary" {
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
		"upstream_req_total",
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

func TestE2E_Broker_005_DebugEnabledRedaction(t *testing.T) {
	brokerBin, fakeBin := ensureE2EBinaries(t)
	p := startBrokerProcess(t, brokerBin, fakeBin, []string{
		"-debug-enable-broker-logs",
	}, map[string]string{
		"FAKE_CHIMERA_BROKER_START_READY":   "1",
		"FAKE_CHIMERA_BROKER_STDOUT_SECRET": "TOKEN=my-secret-value",
	})
	t.Cleanup(func() { stopBroker(t, p) })
	base := "http://" + extractFlagValue(p.cmd.Args, "-listen")
	waitForHTTPStatus(t, base+"/readyz", 200, 8*time.Second)

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
		if strings.Contains(s, `"service":"`+naming.ProductBrokerName+`"`) && strings.Contains(s, `"msg":"broker.`) {
			foundWrapped = true
			break
		}
	}
	if !foundWrapped {
		t.Fatalf("expected wrapped broker structured line(s), got: %s", all)
	}
}

func TestE2E_Broker_006_DebugBindSafetyRejectsRemoteWithoutOverride(t *testing.T) {
	brokerBin, fakeBin := ensureE2EBinaries(t)
	cfgPath := filepath.Join(t.TempDir(), "chimera-broker.config.json")
	_ = os.WriteFile(cfgPath, []byte(`{}`), 0o644)
	cmd := exec.Command(brokerBin,
		"-listen", "0.0.0.0:"+strconvPort(t),
		"-debug-enable-broker-logs",
		"-bin", fakeBin,
		"-chimera-broker-config", cfgPath,
		"-data-path", t.TempDir(),
		"-endpoint", "127.0.0.1:"+strconvPort(t),
	)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start broker: %v", err)
	}
	p := &brokerProc{cmd: cmd}
	code := waitForProcessExit(t, p, 5*time.Second)
	if code != 10 {
		t.Fatalf("expected config exit 10, got %d", code)
	}
}

func TestE2E_Broker_007_DebugBindOverrideAllowsRemote(t *testing.T) {
	if runtime.GOOS == "windows" && strings.TrimSpace(os.Getenv("CHIMERA_E2E_REMOTE_BIND")) != "1" {
		t.Skip("skipping remote bind test on windows by default to avoid firewall prompt; set CHIMERA_E2E_REMOTE_BIND=1 to run")
	}
	brokerBin, fakeBin := ensureE2EBinaries(t)
	p := startBrokerProcess(t, brokerBin, fakeBin, []string{
		"-listen", "0.0.0.0:" + strconvPort(t),
		"-debug-enable-broker-logs",
		"-debug-allow-remote",
	}, map[string]string{"FAKE_CHIMERA_BROKER_START_READY": "1"})
	t.Cleanup(func() { stopBroker(t, p) })
	base := "http://127.0.0.1:" + strings.Split(extractFlagValue(p.cmd.Args, "-listen"), ":")[1]
	waitForHTTPStatus(t, base+"/healthz", 200, 8*time.Second)
}

func TestE2E_Broker_008_StartupTimeoutExit20(t *testing.T) {
	brokerBin, fakeBin := ensureE2EBinaries(t)
	p := startBrokerProcess(t, brokerBin, fakeBin, []string{
		"-startup-timeout", "1s",
	}, map[string]string{"FAKE_CHIMERA_BROKER_START_READY": "0"})
	code := waitForProcessExit(t, p, 6*time.Second)
	if code != 20 {
		t.Fatalf("expected startup failure exit 20, got %d", code)
	}
}

func TestE2E_Broker_009_RuntimeDegradedWhenUpstreamUnready(t *testing.T) {
	brokerBin, fakeBin := ensureE2EBinaries(t)
	p := startBrokerProcess(t, brokerBin, fakeBin, nil, map[string]string{"FAKE_CHIMERA_BROKER_START_READY": "1"})
	t.Cleanup(func() { stopBroker(t, p) })
	base := "http://" + extractFlagValue(p.cmd.Args, "-listen")
	waitForHTTPStatus(t, base+"/readyz", 200, 8*time.Second)
	bport := endpointPort(extractFlagValue(p.cmd.Args, "-endpoint"))
	admin := fmt.Sprintf("http://127.0.0.1:%s/admin/ready?value=0", bport)
	if _, err := http.Get(admin); err != nil {
		t.Fatalf("set upstream unready: %v", err)
	}
	waitForHTTPStatus(t, base+"/readyz", 503, 8*time.Second)
	doc := statusDoc(t, base)
	if doc["status"] != "degraded" {
		t.Fatalf("expected degraded status: %+v", doc)
	}
}

func TestE2E_Broker_010_RestartBackoffAndCounter(t *testing.T) {
	brokerBin, fakeBin := ensureE2EBinaries(t)
	p := startBrokerProcess(t, brokerBin, fakeBin, nil, map[string]string{"FAKE_CHIMERA_BROKER_START_READY": "1"})
	t.Cleanup(func() { stopBroker(t, p) })
	base := "http://" + extractFlagValue(p.cmd.Args, "-listen")
	waitForHTTPStatus(t, base+"/readyz", 200, 8*time.Second)
	bport := endpointPort(extractFlagValue(p.cmd.Args, "-endpoint"))

	start := time.Now()
	if _, err := http.Get(fmt.Sprintf("http://127.0.0.1:%s/admin/crash", bport)); err == nil {
		// connection may drop; ignore.
	}
	waitForHTTPStatus(t, base+"/readyz", 503, 8*time.Second)
	deadline := time.Now().Add(14 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(base + "/readyz")
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == 200 {
				if time.Since(start) < 900*time.Millisecond {
					t.Fatalf("service became ready too quickly, expected backoff >=1s")
				}
				doc := statusDoc(t, base)
				if rf, ok := doc["restarts"].(float64); !ok || int(rf) < 1 {
					t.Fatalf("expected restarts>=1, got %+v", doc["restarts"])
				}
				return
			}
		}
		time.Sleep(150 * time.Millisecond)
	}
	t.Fatalf("wrapper never recovered to ready after crash")
}

func TestE2E_Broker_011_GracefulShutdownExit0(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("signal-driven graceful shutdown semantics vary on windows")
	}
	brokerBin, fakeBin := ensureE2EBinaries(t)
	p := startBrokerProcess(t, brokerBin, fakeBin, nil, map[string]string{"FAKE_CHIMERA_BROKER_START_READY": "1"})
	base := "http://" + extractFlagValue(p.cmd.Args, "-listen")
	waitForHTTPStatus(t, base+"/readyz", 200, 8*time.Second)
	_ = p.cmd.Process.Signal(os.Interrupt)
	code := waitForProcessExit(t, p, 8*time.Second)
	if code != 0 {
		t.Fatalf("expected graceful exit 0, got %d", code)
	}
}

func TestE2E_Broker_012_ForcedShutdownExit30(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("forced-kill semantics are kill-first on windows")
	}
	brokerBin, fakeBin := ensureE2EBinaries(t)
	p := startBrokerProcess(t, brokerBin, fakeBin, nil, map[string]string{
		"FAKE_CHIMERA_BROKER_START_READY":      "1",
		"FAKE_CHIMERA_BROKER_IGNORE_TERMINATE": "1",
	})
	base := "http://" + extractFlagValue(p.cmd.Args, "-listen")
	waitForHTTPStatus(t, base+"/readyz", 200, 8*time.Second)
	_ = p.cmd.Process.Signal(os.Interrupt)
	code := waitForProcessExit(t, p, 12*time.Second)
	if code != 30 {
		t.Fatalf("expected forced shutdown exit 30, got %d", code)
	}
}

func TestE2E_Broker_013_UpstreamMetricsPrefixedNoMerge(t *testing.T) {
	brokerBin, fakeBin := ensureE2EBinaries(t)
	p := startBrokerProcess(t, brokerBin, fakeBin, nil, map[string]string{"FAKE_CHIMERA_BROKER_START_READY": "1"})
	t.Cleanup(func() { stopBroker(t, p) })
	base := "http://" + extractFlagValue(p.cmd.Args, "-listen")
	waitForHTTPStatus(t, base+"/readyz", 200, 8*time.Second)
	resp, err := http.Get(base + "/metrics")
	if err != nil {
		t.Fatalf("metrics request: %v", err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	text := string(b)
	if !strings.Contains(text, "chimera_wrapper_up") {
		t.Fatalf("missing wrapper metric")
	}
	if !strings.Contains(text, "upstream_chimera_wrapper_up") {
		t.Fatalf("missing prefixed upstream collision metric")
	}
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

func endpointPort(endpoint string) string {
	_, p, _ := strings.Cut(strings.TrimSpace(endpoint), ":")
	return p
}
