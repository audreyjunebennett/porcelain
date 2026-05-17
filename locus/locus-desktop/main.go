package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/joho/godotenv"
)

const desktopAttachStartupTimeout = 60 * time.Second
const desktopLaunchLockTimeout = 4 * time.Second
const desktopStartupLivenessTimeout = 20 * time.Second
const desktopStartupReadinessTimeout = 45 * time.Second
const desktopRuntimeHealthPollInterval = 2 * time.Second
const desktopRuntimeHealthMissThreshold = 3
const desktopSupervisorLogFileName = "locus-supervisor.log"

type launchMode string

const (
	launchModeAttachExisting launchMode = "attach_existing"
	launchModeLaunchOwned    launchMode = "launch_owned"
	launchModeLaunchFailed   launchMode = "launch_failed"
)

type launchMetadata struct {
	TimestampUTC       string     `json:"timestamp_utc"`
	Mode               launchMode `json:"mode"`
	BaseURL            string     `json:"base_url"`
	SupervisorBin      string     `json:"supervisor_bin,omitempty"`
	SupervisorOwned    bool       `json:"supervisor_owned"`
	SupervisorPID      int        `json:"supervisor_pid,omitempty"`
	SupervisorWorkDir  string     `json:"supervisor_work_dir,omitempty"`
	SupervisorLogPath  string     `json:"supervisor_log_path,omitempty"`
	LaunchArgsRedacted []string   `json:"launch_args_redacted,omitempty"`
	Error              string     `json:"error,omitempty"`
}

type desktopLifecycleState string

const (
	desktopStateInit                desktopLifecycleState = "desktop.init"
	desktopStateAttachAttempt       desktopLifecycleState = "desktop.attach.attempt"
	desktopStateAttachSuccess       desktopLifecycleState = "desktop.attach.success"
	desktopStateLaunchAttempt       desktopLifecycleState = "desktop.launch.attempt"
	desktopStateLaunchSuccess       desktopLifecycleState = "desktop.launch.success"
	desktopStateLivenessWait        desktopLifecycleState = "desktop.liveness.wait"
	desktopStateLivenessTimeout     desktopLifecycleState = "desktop.liveness.timeout"
	desktopStateReadinessWait       desktopLifecycleState = "desktop.readiness.wait"
	desktopStateReadinessTimeout    desktopLifecycleState = "desktop.readiness.timeout"
	desktopStateEntryResolved       desktopLifecycleState = "desktop.entry.resolved"
	desktopStateUnreachableDisplayed desktopLifecycleState = "desktop.unreachable.displayed"
	desktopStateRuntimeLost         desktopLifecycleState = "desktop.runtime.lost"
	desktopStateShutdown            desktopLifecycleState = "desktop.shutdown"
)

func main() {
	_ = godotenv.Load("env")
	_ = godotenv.Load(".env")

	args := os.Args[1:]
	headless := false
	for len(args) > 0 && (args[0] == "--headless" || args[0] == "-headless") {
		headless = true
		args = args[1:]
	}
	if len(args) > 0 && (args[0] == "-version" || args[0] == "--version") {
		fmt.Printf("locus-desktop %s\ncommit %s\nbuild date %s\n", version, commit, date)
		return
	}
	for _, a := range args {
		if a == "-h" || a == "--help" {
			printHelp()
			return
		}
	}
	runDesktopLauncher(args, !headless)
}

func printHelp() {
	fmt.Printf(`Locus desktop launcher

Usage:
  locus-desktop [flags]
  locus-desktop --headless [flags]
  locus-desktop -version

This binary launches or attaches to chimera-supervisor and opens desktop UI.
Launcher-only flags:
  -log-dir <path>   Directory for locus-desktop/supervisor logs (default: ../data)
`)
}

func runDesktopLauncher(args []string, openWebview bool) {
	runtimeRoot := resolveRuntimeRoot()
	launchArgs, logDir := launcherArgs(args, runtimeRoot)
	baseURL := desktopSupervisorBaseURL(args, runtimeRoot)
	rootCtx, stopRoot := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopRoot()
	recordLifecycleEvent(runtimeRoot, desktopStateInit, "desktop launcher start", map[string]any{
		"base_url": baseURL,
	})

	ownedSupervisor, owned := (*exec.Cmd)(nil), false
	var supervisorLogFile *os.File
	defer func() {
		if supervisorLogFile != nil {
			_ = supervisorLogFile.Close()
		}
	}()
	if !supervisorReachable(baseURL) {
		recordLifecycleEvent(runtimeRoot, desktopStateAttachAttempt, "attach check failed; launch path", map[string]any{
			"base_url": baseURL,
		})
		unlock, lockErr := acquireDesktopLaunchLock(runtimeRoot, desktopLaunchLockTimeout)
		if lockErr != nil {
			fmt.Fprintf(os.Stderr, "locus-desktop: startup lock: %v\n", lockErr)
			os.Exit(1)
		}
		defer unlock()
		if supervisorReachable(baseURL) {
			recordLifecycleEvent(runtimeRoot, desktopStateAttachSuccess, "attached while waiting for launch lock", map[string]any{
				"base_url": baseURL,
			})
			recordLaunchMetadata(runtimeRoot, launchMetadata{
				TimestampUTC:    time.Now().UTC().Format(time.RFC3339Nano),
				Mode:            launchModeAttachExisting,
				BaseURL:         baseURL,
				SupervisorOwned: false,
			})
		} else {
		bin, err := resolveLauncherSupervisorBinary()
		if err != nil {
			recordLaunchMetadata(runtimeRoot, launchMetadata{
				TimestampUTC:    time.Now().UTC().Format(time.RFC3339Nano),
				Mode:            launchModeLaunchFailed,
				BaseURL:         baseURL,
				SupervisorOwned: false,
				Error:           "resolve chimera-supervisor: " + err.Error(),
			})
			fmt.Fprintf(os.Stderr, "locus-desktop: resolve chimera-supervisor: %v\n", err)
			os.Exit(1)
		}
		recordLifecycleEvent(runtimeRoot, desktopStateLaunchAttempt, "starting owned supervisor", map[string]any{
			"supervisor_bin": bin,
			"log_dir":        logDir,
		})
		supervisorLogFile, err = openSupervisorLogFile(logDir)
		if err != nil {
			recordLaunchMetadata(runtimeRoot, launchMetadata{
				TimestampUTC:       time.Now().UTC().Format(time.RFC3339Nano),
				Mode:               launchModeLaunchFailed,
				BaseURL:            baseURL,
				SupervisorBin:      bin,
				SupervisorOwned:    false,
				SupervisorWorkDir:  runtimeRoot,
				LaunchArgsRedacted: redactLaunchArgs(launchArgs),
				Error:              "open supervisor log file: " + err.Error(),
			})
			fmt.Fprintf(os.Stderr, "locus-desktop: open supervisor log file: %v\n", err)
			os.Exit(1)
		}
		ownedSupervisor = exec.Command(bin, launchArgs...)
		ownedSupervisor.Dir = runtimeRoot
		ownedSupervisor.Stdout = supervisorLogFile
		ownedSupervisor.Stderr = supervisorLogFile
		if err := ownedSupervisor.Start(); err != nil {
			recordLaunchMetadata(runtimeRoot, launchMetadata{
				TimestampUTC:       time.Now().UTC().Format(time.RFC3339Nano),
				Mode:               launchModeLaunchFailed,
				BaseURL:            baseURL,
				SupervisorBin:      bin,
				SupervisorOwned:    false,
				SupervisorWorkDir:  ownedSupervisor.Dir,
				SupervisorLogPath:  supervisorLogFile.Name(),
				LaunchArgsRedacted: redactLaunchArgs(launchArgs),
				Error:              "start chimera-supervisor: " + err.Error(),
			})
			fmt.Fprintf(os.Stderr, "locus-desktop: start chimera-supervisor: %v\n", err)
			os.Exit(1)
		}
		owned = true
		recordLifecycleEvent(runtimeRoot, desktopStateLaunchSuccess, "owned supervisor started", map[string]any{
			"supervisor_pid": ownedSupervisor.Process.Pid,
		})
		recordLaunchMetadata(runtimeRoot, launchMetadata{
			TimestampUTC:       time.Now().UTC().Format(time.RFC3339Nano),
			Mode:               launchModeLaunchOwned,
			BaseURL:            baseURL,
			SupervisorBin:      bin,
			SupervisorOwned:    true,
			SupervisorPID:      ownedSupervisor.Process.Pid,
			SupervisorWorkDir:  ownedSupervisor.Dir,
			SupervisorLogPath:  supervisorLogFile.Name(),
			LaunchArgsRedacted: redactLaunchArgs(launchArgs),
		})
		recordLifecycleEvent(runtimeRoot, desktopStateLivenessWait, "waiting for supervisor liveness", map[string]any{
			"timeout_ms": desktopAttachStartupTimeout.Milliseconds(),
		})
		if !waitForSupervisorAttach(baseURL, desktopAttachStartupTimeout) {
			_ = stopOwnedDesktopSupervisor(ownedSupervisor)
			recordLifecycleEvent(runtimeRoot, desktopStateLivenessTimeout, "supervisor liveness timeout", map[string]any{
				"base_url": baseURL,
			})
			recordLaunchMetadata(runtimeRoot, launchMetadata{
				TimestampUTC:       time.Now().UTC().Format(time.RFC3339Nano),
				Mode:               launchModeLaunchFailed,
				BaseURL:            baseURL,
				SupervisorBin:      bin,
				SupervisorOwned:    false,
				SupervisorWorkDir:  ownedSupervisor.Dir,
				SupervisorLogPath:  supervisorLogFile.Name(),
				LaunchArgsRedacted: redactLaunchArgs(launchArgs),
				Error:              "timed out waiting for chimera-supervisor",
			})
			showUnreachableOrExit(openWebview, baseURL, true, "Timed out waiting for chimera-supervisor startup", rootCtx, stopRoot, runtimeRoot)
			return
		}
	}
	}
	if !owned {
		recordLifecycleEvent(runtimeRoot, desktopStateAttachSuccess, "attached to existing supervisor", map[string]any{
			"base_url": baseURL,
		})
		recordLaunchMetadata(runtimeRoot, launchMetadata{
			TimestampUTC:    time.Now().UTC().Format(time.RFC3339Nano),
			Mode:            launchModeAttachExisting,
			BaseURL:         baseURL,
			SupervisorOwned: false,
		})
	}

	recordLifecycleEvent(runtimeRoot, desktopStateLivenessWait, "waiting for liveness", map[string]any{
		"timeout_ms": desktopStartupLivenessTimeout.Milliseconds(),
	})
	if !waitForSupervisorLiveness(baseURL, desktopStartupLivenessTimeout) {
		recordLifecycleEvent(runtimeRoot, desktopStateLivenessTimeout, "liveness wait timeout", map[string]any{
			"base_url": baseURL,
		})
		showUnreachableOrExit(openWebview, baseURL, owned, "Supervisor is not reachable on health endpoints", rootCtx, stopRoot, runtimeRoot)
		return
	}

	recordLifecycleEvent(runtimeRoot, desktopStateReadinessWait, "waiting for readiness", map[string]any{
		"timeout_ms": desktopStartupReadinessTimeout.Milliseconds(),
	})
	ready, readinessDetail := waitForSupervisorReadiness(baseURL, desktopStartupReadinessTimeout)
	if !ready {
		recordLifecycleEvent(runtimeRoot, desktopStateReadinessTimeout, "readiness wait timeout", map[string]any{
			"detail": readinessDetail,
		})
		showUnreachableOrExit(openWebview, baseURL, owned, readinessDetail, rootCtx, stopRoot, runtimeRoot)
		return
	}

	entryURL := resolveDesktopEntryURL(baseURL)
	recordLifecycleEvent(runtimeRoot, desktopStateEntryResolved, "entry route selected", map[string]any{
		"entry_url": entryURL,
	})

	runtimeLossCh := make(chan string, 1)
	go monitorRuntimeLoss(rootCtx, baseURL, runtimeLossCh)
	if owned {
		defer func() {
			_ = stopOwnedDesktopSupervisor(ownedSupervisor)
			recordLifecycleEvent(runtimeRoot, desktopStateShutdown, "owned supervisor stopped on desktop close", nil)
		}()
	}
	if openWebview {
		runDesktopWebview(true, entryURL, runtimeLossCh, baseURL, stopRoot, rootCtx)
		return
	}
	select {
	case <-rootCtx.Done():
	case reason := <-runtimeLossCh:
		recordLifecycleEvent(runtimeRoot, desktopStateRuntimeLost, reason, map[string]any{"base_url": baseURL})
		fmt.Fprintf(os.Stderr, "locus-desktop: supervisor runtime lost: %s\n", reason)
	}
}

func resolveLauncherSupervisorBinary() (string, error) {
	exeDir := executableDir()
	if exeDir != "" {
		names := []string{"chimera-supervisor"}
		if runtime.GOOS == "windows" {
			names = []string{"chimera-supervisor.exe", "chimera-supervisor"}
		}
		if p := firstExistingInSearchDirs(exeDir, names); p != "" {
			return p, nil
		}
	}
	if runtime.GOOS == "windows" {
		return "", errors.New("chimera-supervisor.exe not found next to locus-desktop")
	}
	return "", errors.New("chimera-supervisor not found next to locus-desktop")
}

func desktopSupervisorBaseURL(args []string, runtimeRoot string) string {
	addr := "127.0.0.1:7710"
	for i := 0; i < len(args); i++ {
		raw := strings.TrimSpace(args[i])
		lower := strings.ToLower(raw)
		switch {
		case lower == "-listen" || lower == "--listen":
			if i+1 < len(args) {
				addr = strings.TrimSpace(args[i+1])
				i++
			}
		case strings.HasPrefix(lower, "-listen=") || strings.HasPrefix(lower, "--listen="):
			if idx := strings.IndexByte(raw, '='); idx >= 0 {
				addr = strings.TrimSpace(raw[idx+1:])
			}
		}
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return "http://127.0.0.1:7710"
	}
	host = strings.TrimSpace(host)
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	if strings.Contains(host, ":") && !strings.HasPrefix(host, "[") {
		host = "[" + host + "]"
	}
	return "http://" + host + ":" + port
}

func supervisorReachable(baseURL string) bool {
	client := &http.Client{Timeout: 1200 * time.Millisecond}
	for _, path := range []string{"/healthz", "/health"} {
		resp, err := client.Get(baseURL + path)
		if err != nil {
			continue
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			return true
		}
	}
	return false
}

func waitForSupervisorAttach(baseURL string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if supervisorReachable(baseURL) {
			return true
		}
		time.Sleep(350 * time.Millisecond)
	}
	return false
}

func waitForSupervisorLiveness(baseURL string, timeout time.Duration) bool {
	return waitForSupervisorAttach(baseURL, timeout)
}

func waitForSupervisorReadiness(baseURL string, timeout time.Duration) (bool, string) {
	deadline := time.Now().Add(timeout)
	last := "readiness endpoint unavailable"
	for time.Now().Before(deadline) {
		ready, detail := supervisorReady(baseURL)
		if ready {
			return true, ""
		}
		if strings.TrimSpace(detail) != "" {
			last = detail
		}
		time.Sleep(350 * time.Millisecond)
	}
	return false, last
}

func supervisorReady(baseURL string) (bool, string) {
	client := &http.Client{Timeout: 1500 * time.Millisecond}
	resp, err := client.Get(baseURL + "/readyz")
	if err == nil {
		if resp.StatusCode == http.StatusServiceUnavailable {
			_ = resp.Body.Close()
			resp, err = client.Get(baseURL + "/status")
			if err != nil {
				return false, "supervisor reports degraded readiness"
			}
			defer resp.Body.Close()
			raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
			if resp.StatusCode == http.StatusOK {
				var body map[string]any
				if json.Unmarshal(raw, &body) == nil {
					if bootstrap, ok := body["bootstrap"].(bool); ok && bootstrap {
						return true, ""
					}
					if upstream, ok := body["upstream"].(map[string]any); ok {
						if okv, ok2 := upstream["ok"].(bool); ok2 && okv {
							return true, ""
						}
					}
				}
			}
			return false, "supervisor reports degraded readiness"
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			return true, ""
		}
		return false, fmt.Sprintf("readyz returned status %d", resp.StatusCode)
	}
	resp, err = client.Get(baseURL + "/status")
	if err != nil {
		return false, "status endpoint unavailable"
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode == http.StatusOK {
		var body map[string]any
		if json.Unmarshal(raw, &body) == nil {
			if upstream, ok := body["upstream"].(map[string]any); ok {
				if okv, ok2 := upstream["ok"].(bool); ok2 && okv {
					return true, ""
				}
				if detail, ok2 := upstream["detail"].(string); ok2 && strings.TrimSpace(detail) != "" {
					return false, detail
				}
			}
			if bootstrap, ok := body["bootstrap"].(bool); ok && bootstrap {
				return true, ""
			}
		}
		return true, ""
	}
	return false, fmt.Sprintf("status returned %d", resp.StatusCode)
}

// defaultDesktopLoginNext is the post-auth route for Locus (unified logs + admin cards).
const defaultDesktopLoginNext = "/ui/logs?focus=admin"

func operatorUIFromSupervisorStatus(supervisorBase string) (uiBase string, bootstrap bool) {
	supervisorBase = strings.TrimRight(strings.TrimSpace(supervisorBase), "/")
	if supervisorBase == "" {
		return "", false
	}
	client := &http.Client{Timeout: 1500 * time.Millisecond}
	resp, err := client.Get(supervisorBase + "/status")
	if err != nil {
		return "", false
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var body map[string]any
	if json.Unmarshal(raw, &body) != nil {
		return "", false
	}
	det, _ := body["details"].(map[string]any)
	if det == nil {
		return "", false
	}
	ui, _ := det["operator_ui"].(map[string]any)
	if ui == nil {
		return "", false
	}
	s, _ := ui["base_url"].(string)
	s = strings.TrimRight(strings.TrimSpace(s), "/")
	if s == "" {
		return "", false
	}
	b, _ := ui["bootstrap"].(bool)
	return s, b
}

func resolveDesktopEntryURL(supervisorBase string) string {
	uiBase, bootstrap := operatorUIFromSupervisorStatus(supervisorBase)
	if uiBase == "" {
		uiBase = strings.TrimRight(strings.TrimSpace(supervisorBase), "/")
	}
	if bootstrap {
		return uiBase + "/ui/setup"
	}
	return uiBase + "/ui/login?next=" + url.PathEscape(defaultDesktopLoginNext)
}

func showUnreachableOrExit(openWebview bool, baseURL string, owned bool, reason string, rootCtx context.Context, stopRoot context.CancelFunc, runtimeRoot string) {
	recordLifecycleEvent(runtimeRoot, desktopStateUnreachableDisplayed, reason, map[string]any{
		"base_url": baseURL,
		"owned":    owned,
	})
	if !openWebview {
		fmt.Fprintf(os.Stderr, "locus-desktop: cannot connect to supervisor: %s\n", reason)
		os.Exit(1)
	}
	unreachableURL := buildUnreachableURL(baseURL, reason, owned)
	runDesktopWebview(true, unreachableURL, nil, baseURL, stopRoot, rootCtx)
}

func buildUnreachableURL(baseURL, reason string, owned bool) string {
	mode := "attached to existing supervisor"
	if owned {
		mode = "started by this desktop instance"
	}
	html := "<!doctype html><html><head><meta charset=\"utf-8\"><title>Cannot connect to supervisor</title>" +
		"<style>body{font-family:Segoe UI,Arial,sans-serif;background:#0f1115;color:#e7e9ee;margin:0;padding:24px;}h1{margin:0 0 8px 0;font-size:22px;}p{margin:8px 0;line-height:1.45;}code{background:#1b1f29;padding:2px 6px;border-radius:4px;} .box{border:1px solid #2f3545;border-radius:8px;padding:14px;margin-top:14px;background:#151925;}</style></head><body>" +
		"<h1>Cannot connect to supervisor</h1>" +
		"<p>Locus desktop could not establish a healthy connection to <code>" + escapeHTML(baseURL) + "</code>.</p>" +
		"<div class=\"box\"><p><strong>Detail:</strong> " + escapeHTML(reason) + "</p>" +
		"<p><strong>Ownership:</strong> " + escapeHTML(mode) + "</p></div>" +
		"<div class=\"box\"><p><strong>Try:</strong></p><p>1) Verify <code>chimera-supervisor</code>, <code>chimera-broker</code>, and <code>chimera-vectorstore</code> binaries exist in runtime paths.</p>" +
		"<p>2) Check for port conflicts on 3000, 6333, 6334, 7710, 7720, 7730, 7740, 7750, and 8080.</p>" +
		"<p>3) Relaunch desktop after stopping stale local runtime processes.</p></div></body></html>"
	return "data:text/html;charset=utf-8," + url.PathEscape(html)
}

func monitorRuntimeLoss(ctx context.Context, baseURL string, out chan<- string) {
	if out == nil {
		return
	}
	t := time.NewTicker(desktopRuntimeHealthPollInterval)
	defer t.Stop()
	misses := 0
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if supervisorReachable(baseURL) {
				misses = 0
				continue
			}
			misses++
			if misses >= desktopRuntimeHealthMissThreshold {
				select {
				case out <- "runtime health checks failed":
				default:
				}
				return
			}
		}
	}
}

func recordLifecycleEvent(runtimeRoot string, state desktopLifecycleState, detail string, fields map[string]any) {
	entry := map[string]any{
		"timestamp_utc": time.Now().UTC().Format(time.RFC3339Nano),
		"state":         state,
		"detail":        detail,
	}
	for k, v := range fields {
		entry[k] = v
	}
	path := filepath.Join(runtimeRoot, "run", "locus-desktop-events.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return
	}
	b, err := json.Marshal(entry)
	if err != nil {
		return
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.Write(append(b, '\n'))
}

func escapeHTML(s string) string {
	r := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		"\"", "&quot;",
		"'", "&#39;",
	)
	return r.Replace(s)
}

func stopOwnedDesktopSupervisor(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	waitCh := make(chan error, 1)
	go func() {
		waitCh <- cmd.Wait()
	}()
	if err := cmd.Process.Signal(os.Interrupt); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return err
	}
	select {
	case err := <-waitCh:
		return err
	case <-time.After(15 * time.Second):
		if err := cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
			return err
		}
		<-waitCh
		return nil
	}
}

func executableDir() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	return filepath.Dir(exe)
}

func resolveSupervisorWorkingDir() string {
	return resolveRuntimeRoot()
}

func resolveRuntimeRoot() string {
	exeDir := executableDir()
	if exeDir != "" {
		if strings.EqualFold(filepath.Base(exeDir), "bin") {
			return filepath.Dir(exeDir)
		}
		parent := filepath.Dir(exeDir)
		if strings.EqualFold(filepath.Base(parent), "bin") {
			return filepath.Dir(parent)
		}
	}
	if wd, err := os.Getwd(); err == nil {
		if st, serr := os.Stat(filepath.Join(wd, "config")); serr == nil && st.IsDir() {
			return wd
		}
		return wd
	}
	if exeDir != "" {
		return filepath.Dir(exeDir)
	}
	return "."
}

func supervisorLaunchArgs(args []string, runtimeRoot string) []string {
	out := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		raw := strings.TrimSpace(args[i])
		lower := strings.ToLower(raw)
		if lower == "desktop" {
			continue
		}
		if lower == "-log-dir" || lower == "--log-dir" {
			if i+1 < len(args) {
				i++
			}
			continue
		}
		if strings.HasPrefix(lower, "-log-dir=") || strings.HasPrefix(lower, "--log-dir=") {
			continue
		}
		out = append(out, args[i])
	}
	return out
}

func launcherArgs(args []string, runtimeRoot string) ([]string, string) {
	return supervisorLaunchArgs(args, runtimeRoot), resolveDesktopLogDir(args, runtimeRoot)
}

func resolveDesktopLogDir(args []string, runtimeRoot string) string {
	dir := strings.TrimSpace(os.Getenv("LOCUS_DESKTOP_LOG_DIR"))
	if dir == "" {
		dir = filepath.Join(runtimeRoot, "data")
	}
	for i := 0; i < len(args); i++ {
		raw := strings.TrimSpace(args[i])
		lower := strings.ToLower(raw)
		switch {
		case lower == "-log-dir" || lower == "--log-dir":
			if i+1 < len(args) {
				dir = strings.TrimSpace(args[i+1])
				i++
			}
		case strings.HasPrefix(lower, "-log-dir=") || strings.HasPrefix(lower, "--log-dir="):
			if idx := strings.IndexByte(raw, '='); idx >= 0 {
				dir = strings.TrimSpace(raw[idx+1:])
			}
		}
	}
	if strings.TrimSpace(dir) == "" {
		dir = filepath.Join(runtimeRoot, "data")
	}
	if !isRootedPath(dir) {
		dir = filepath.Join(runtimeRoot, dir)
	}
	return filepath.Clean(dir)
}

func isRootedPath(p string) bool {
	if filepath.IsAbs(p) {
		return true
	}
	if strings.HasPrefix(p, string(os.PathSeparator)) {
		return true
	}
	if runtime.GOOS == "windows" && strings.HasPrefix(p, "/") {
		return true
	}
	return false
}

func openSupervisorLogFile(logDir string) (*os.File, error) {
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return nil, fmt.Errorf("mkdir log dir %s: %w", logDir, err)
	}
	logPath := filepath.Join(logDir, desktopSupervisorLogFileName)
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", logPath, err)
	}
	_, _ = fmt.Fprintf(f, "\n[%s] locus-desktop launching supervisor\n", time.Now().UTC().Format(time.RFC3339Nano))
	return f, nil
}

func desktopLaunchLockPath(runtimeRoot string) string {
	return filepath.Join(runtimeRoot, "run", "locus-desktop-launch.lock")
}

func desktopLaunchMetadataPath(runtimeRoot string) string {
	return filepath.Join(runtimeRoot, "run", "locus-desktop-launch.json")
}

func acquireDesktopLaunchLock(runtimeRoot string, timeout time.Duration) (func(), error) {
	if timeout <= 0 {
		timeout = desktopLaunchLockTimeout
	}
	lockPath := desktopLaunchLockPath(runtimeRoot)
	if err := os.MkdirAll(filepath.Dir(lockPath), 0755); err != nil {
		return nil, err
	}
	deadline := time.Now().Add(timeout)
	for {
		f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
		if err == nil {
			_, _ = fmt.Fprintf(f, "pid=%d\nstarted=%s\n", os.Getpid(), time.Now().UTC().Format(time.RFC3339Nano))
			_ = f.Close()
			return func() {
				_ = os.Remove(lockPath)
			}, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, err
		}
		if st, statErr := os.Stat(lockPath); statErr == nil && time.Since(st.ModTime()) > 2*time.Minute {
			_ = os.Remove(lockPath)
			continue
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("another desktop launch is already in progress (%s)", lockPath)
		}
		time.Sleep(120 * time.Millisecond)
	}
}

func recordLaunchMetadata(runtimeRoot string, md launchMetadata) {
	path := desktopLaunchMetadataPath(runtimeRoot)
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return
	}
	raw, err := json.MarshalIndent(md, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(path, raw, 0644)
}

func redactLaunchArgs(args []string) []string {
	out := make([]string, 0, len(args))
	redactNext := false
	for _, a := range args {
		if redactNext {
			out = append(out, "<redacted>")
			redactNext = false
			continue
		}
		lower := strings.ToLower(strings.TrimSpace(a))
		if lower == "" {
			out = append(out, a)
			continue
		}
		if strings.HasPrefix(lower, "-") {
			k := lower
			if idx := strings.IndexByte(lower, '='); idx >= 0 {
				key := lower[:idx]
				if isSensitiveArgKey(key) {
					out = append(out, key+"=<redacted>")
					continue
				}
			}
			if isSensitiveArgKey(k) {
				out = append(out, a)
				redactNext = true
				continue
			}
		}
		out = append(out, a)
	}
	return out
}

func isSensitiveArgKey(k string) bool {
	key := strings.ToLower(strings.TrimLeft(strings.TrimSpace(k), "-"))
	return strings.Contains(key, "token") ||
		strings.Contains(key, "secret") ||
		strings.Contains(key, "pass") ||
		strings.Contains(key, "apikey") ||
		strings.Contains(key, "api-key") ||
		strings.Contains(key, "key")
}

func firstExistingFile(dir string, names []string) string {
	for _, n := range names {
		p := filepath.Join(dir, n)
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
	}
	return ""
}

func firstExistingInSearchDirs(exeDir string, names []string) string {
	if exeDir == "" {
		return ""
	}
	parent := filepath.Dir(exeDir)
	grandparent := filepath.Dir(parent)
	greatGrandparent := filepath.Dir(grandparent)
	for _, d := range []string{
		exeDir,
		filepath.Join(exeDir, "bin"),
		filepath.Join(exeDir, "chimera", "bin"),
		parent,
		filepath.Join(parent, "bin"),
		filepath.Join(parent, "chimera", "bin"),
		grandparent,
		filepath.Join(grandparent, "bin"),
		filepath.Join(grandparent, "chimera", "bin"),
		greatGrandparent,
		filepath.Join(greatGrandparent, "bin"),
		filepath.Join(greatGrandparent, "chimera", "bin"),
	} {
		if p := firstExistingFile(d, names); p != "" {
			return p
		}
	}
	return ""
}
