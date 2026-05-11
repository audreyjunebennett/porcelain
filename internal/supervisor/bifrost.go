package supervisor

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// BifrostConfig is the child process layout for supervised bifrost-http.
type BifrostConfig struct {
	// Bin is the BiFrost HTTP executable name or path (e.g. bifrost-http, or ./bin/bifrost-http; default "bifrost" on PATH).
	Bin string
	// ConfigJSON is the host path to bifrost.config.json (copied to DataDir/config.json).
	ConfigJSON string
	// DataDir is BiFrost working directory and SQLite/config parent (created if missing).
	DataDir string
	// BindHost is passed as -host (and APP_HOST for compatibility).
	BindHost string
	// Port is passed as -port (and APP_PORT for compatibility).
	Port int
	// LogLevel is -log-level for bifrost-http (empty → info).
	LogLevel string
	// LogStyle is -log-style for bifrost-http (empty → json).
	LogStyle string
	// ExtraArgs are appended after -app-dir, -host, -port, -log-level, -log-style.
	ExtraArgs []string
	// RawExec runs Bin with Args only (no bifrost-http flags). Used in tests.
	RawExec bool
	// Args is argv when RawExec is true (e.g. sleep, "60").
	Args []string
	// Stdout and Stderr default to os.Stdout / os.Stderr when nil (e.g. use io.MultiWriter for tee + UI buffer).
	Stdout io.Writer
	Stderr io.Writer
}

// CopyConfigJSON copies src to dstDir/config.json (overwrites).
func CopyConfigJSON(src, dstDir string) error {
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		return err
	}
	raw, err := os.ReadFile(src)
	if err != nil {
		return fmt.Errorf("read bifrost config: %w", err)
	}
	dst := filepath.Join(dstDir, "config.json")
	if err := os.WriteFile(dst, raw, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", dst, err)
	}
	return nil
}

// MergeEnv starts from os.Environ() and replaces keys in overrides (last wins).
func MergeEnv(overrides map[string]string) []string {
	m := make(map[string]string)
	for _, e := range os.Environ() {
		i := strings.IndexByte(e, '=')
		if i <= 0 {
			continue
		}
		m[e[:i]] = e[i+1:]
	}
	for k, v := range overrides {
		m[k] = v
	}
	out := make([]string, 0, len(m))
	for k, v := range m {
		out = append(out, k+"="+v)
	}
	return out
}

// absBinIfNeeded turns relative paths that include a directory (e.g. ./bin/bifrost-http) into absolute paths.
// A bare name like "bifrost" is left unchanged so os/exec uses LookPath. Kernel exec resolves relative binary
// paths against the process working directory, not cmd.Dir, so IDE/cwd mismatches otherwise yield ENOENT.
func absBinIfNeeded(bin string) (string, error) {
	if filepath.IsAbs(bin) {
		return bin, nil
	}
	if !strings.ContainsAny(bin, `/\`) {
		return bin, nil
	}
	return filepath.Abs(bin)
}

// StartBifrost starts bifrost-http with -app-dir, -host, -port, and logging flags (same as Docker entrypoint mapping).
// ctx cancel kills the process.
func StartBifrost(ctx context.Context, cfg BifrostConfig, log *slog.Logger) (*exec.Cmd, error) {
	if err := CopyConfigJSON(cfg.ConfigJSON, cfg.DataDir); err != nil {
		return nil, err
	}
	bin := strings.TrimSpace(cfg.Bin)
	if bin == "" {
		bin = "bifrost"
	}
	var err error
	bin, err = absBinIfNeeded(bin)
	if err != nil {
		return nil, fmt.Errorf("resolve bifrost binary path: %w", err)
	}
	var argv []string
	var absAppDir string
	if cfg.RawExec {
		argv = append(argv, cfg.Args...)
	} else {
		absAppDir, err = filepath.Abs(cfg.DataDir)
		if err != nil {
			return nil, fmt.Errorf("resolve bifrost data dir: %w", err)
		}
		ll := strings.TrimSpace(strings.ToLower(cfg.LogLevel))
		if ll == "" {
			ll = "info"
		}
		ls := strings.TrimSpace(strings.ToLower(cfg.LogStyle))
		if ls == "" {
			ls = "json"
		}
		argv = []string{
			"-app-dir", absAppDir,
			"-host", cfg.BindHost,
			"-port", strconv.Itoa(cfg.Port),
			"-log-level", ll,
			"-log-style", ls,
		}
		argv = append(argv, cfg.ExtraArgs...)
	}
	cmd := exec.CommandContext(ctx, bin, argv...)
	cmd.Dir = cfg.DataDir
	cmd.Env = MergeEnv(map[string]string{
		"APP_HOST": cfg.BindHost,
		"APP_PORT": strconv.Itoa(cfg.Port),
	})
	out := cfg.Stdout
	if out == nil {
		out = os.Stdout
	}
	errOut := cfg.Stderr
	if errOut == nil {
		errOut = os.Stderr
	}
	cmd.Stdout = out
	cmd.Stderr = errOut
	applyNoConsoleWindow(cmd)
	if log != nil {
		if cfg.RawExec {
			log.Info("starting bifrost subprocess", "msg", "gateway.supervisor.bifrost.starting", "bin", bin, "dir", cfg.DataDir, "raw", true)
		} else {
			log.Info("starting bifrost subprocess", "msg", "gateway.supervisor.bifrost.starting", "bin", bin, "app_dir", absAppDir, "host", cfg.BindHost, "port", cfg.Port)
		}
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start bifrost: %w", err)
	}
	return cmd, nil
}

// WaitHealthy polls GET healthURL until 2xx or ctx done / timeout.
// child is "qdrant" or "bifrost" (or empty to skip the success log line).
func WaitHealthy(ctx context.Context, healthURL string, timeout time.Duration, log *slog.Logger, child string) error {
	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: 2 * time.Second}
	interval := 200 * time.Millisecond
	for {
		if timeout > 0 && time.Now().After(deadline) {
			return fmt.Errorf("timeout waiting for %s", healthURL)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, healthURL, nil)
		if err != nil {
			return err
		}
		res, err := client.Do(req)
		if err == nil {
			_, _ = io.Copy(io.Discard, res.Body)
			res.Body.Close()
			if res.StatusCode >= 200 && res.StatusCode < 300 {
				if log != nil {
					switch child {
					case "qdrant":
						log.Info("qdrant health OK", "msg", "gateway.supervisor.qdrant.ready", "url", healthURL)
					case "bifrost":
						log.Info("bifrost health OK", "msg", "gateway.supervisor.bifrost.ready", "url", healthURL)
					}
				}
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(interval):
		}
	}
}
