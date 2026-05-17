package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// ChimeraBrokerConfig is the child process layout for supervised chimera-broker.
type ChimeraBrokerConfig struct {
	// Bin is the Chimera Broker executable name or path.
	Bin string
	// ConfigJSON is the host path to chimera-broker.config.json (copied to DataDir/config.json).
	ConfigJSON string
	// DataDir is Chimera Broker working directory and SQLite/config parent (created if missing).
	DataDir string
	// BindHost is passed as -host (and APP_HOST for compatibility).
	BindHost string
	// Port is passed as -port (and APP_PORT for compatibility).
	Port int
	// LogLevel is -log-level for chimera-broker (empty -> info).
	LogLevel string
	// LogStyle is -log-style for chimera-broker (empty -> json).
	LogStyle string
	// ExtraArgs are appended after -app-dir, -host, -port, -log-level, -log-style.
	ExtraArgs []string
	// RawExec runs Bin with Args only (no chimera-broker flags). Used in tests.
	RawExec bool
	// Args is argv when RawExec is true.
	Args []string
	// Stdout and Stderr default to os.Stdout / os.Stderr when nil.
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
		return fmt.Errorf("read chimera-broker config: %w", err)
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

// absBinIfNeeded turns relative paths that include a directory into absolute paths.
func absBinIfNeeded(bin string) (string, error) {
	if filepath.IsAbs(bin) {
		return bin, nil
	}
	if !strings.ContainsAny(bin, `/\`) {
		return bin, nil
	}
	return filepath.Abs(bin)
}

// StartChimeraBroker starts chimera-broker with -app-dir, -host, -port, and logging flags.
// ctx cancel kills the process.
func StartChimeraBroker(ctx context.Context, cfg ChimeraBrokerConfig, log *slog.Logger) (*exec.Cmd, error) {
	if err := CopyConfigJSON(cfg.ConfigJSON, cfg.DataDir); err != nil {
		return nil, err
	}
	bin := strings.TrimSpace(cfg.Bin)
	if bin == "" {
		bin = "chimera-broker"
	}
	var err error
	bin, err = absBinIfNeeded(bin)
	if err != nil {
		return nil, fmt.Errorf("resolve chimera broker binary path: %w", err)
	}
	var argv []string
	var absAppDir string
	if cfg.RawExec {
		argv = append(argv, cfg.Args...)
	} else {
		absAppDir, err = filepath.Abs(cfg.DataDir)
		if err != nil {
			return nil, fmt.Errorf("resolve chimera broker data dir: %w", err)
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
			log.Info("starting chimera-broker subprocess", "msg", "gateway.supervisor.chimera-broker.starting", "bin", bin, "dir", cfg.DataDir, "raw", true)
		} else {
			log.Info("starting chimera-broker subprocess", "msg", "gateway.supervisor.chimera-broker.starting", "bin", bin, "app_dir", absAppDir, "host", cfg.BindHost, "port", cfg.Port)
		}
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start chimera-broker: %w", err)
	}
	return cmd, nil
}
