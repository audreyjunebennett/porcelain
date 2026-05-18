package chimerabroker

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

	"github.com/lynn/porcelain/internal/naming"
)

// Config is the child process layout for supervised chimera-broker-http (BiFrost).
type Config struct {
	Bin        string
	ConfigJSON string
	DataDir    string
	BindHost   string
	Port       int
	LogLevel   string
	LogStyle   string
	ExtraArgs  []string
	RawExec    bool
	Args       []string
	Stdout     io.Writer
	Stderr     io.Writer
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

// Start launches chimera-broker-http with -app-dir, -host, -port, and logging flags.
func Start(ctx context.Context, cfg Config, log *slog.Logger) (*exec.Cmd, error) {
	if err := CopyConfigJSON(cfg.ConfigJSON, cfg.DataDir); err != nil {
		return nil, err
	}
	bin := strings.TrimSpace(cfg.Bin)
	if bin == "" {
		bin = naming.ProductBrokerHTTPBinName
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
	cmd.Env = mergeEnv(map[string]string{
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

func mergeEnv(overrides map[string]string) []string {
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

func absBinIfNeeded(bin string) (string, error) {
	if filepath.IsAbs(bin) {
		return bin, nil
	}
	if !strings.ContainsAny(bin, `/\`) {
		return bin, nil
	}
	return filepath.Abs(bin)
}
