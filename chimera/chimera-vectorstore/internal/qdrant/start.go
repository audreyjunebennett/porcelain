package qdrant

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

// Config controls a native Qdrant child process (default ports 6333/6334).
type Config struct {
	Bin        string
	StorageDir string
	BindHost   string
	HTTPPort   int
	GRPCPort   int
	LogLevel   string
	Stdout     io.Writer
	Stderr     io.Writer
}

// Start launches the Qdrant binary with env-based configuration.
func Start(ctx context.Context, cfg Config, log *slog.Logger) (*exec.Cmd, error) {
	bin := strings.TrimSpace(cfg.Bin)
	if bin == "" {
		return nil, fmt.Errorf("qdrant: empty Bin")
	}
	var err error
	bin, err = absBinIfNeeded(bin)
	if err != nil {
		return nil, fmt.Errorf("resolve qdrant binary path: %w", err)
	}
	if err := os.MkdirAll(cfg.StorageDir, 0o755); err != nil {
		return nil, fmt.Errorf("qdrant storage dir: %w", err)
	}
	absStorage, err := filepath.Abs(cfg.StorageDir)
	if err != nil {
		return nil, fmt.Errorf("resolve qdrant storage dir: %w", err)
	}

	cmd := exec.CommandContext(ctx, bin)
	cmd.Dir = absStorage
	envOverrides := map[string]string{
		"QDRANT__STORAGE__STORAGE_PATH": absStorage,
		"QDRANT__SERVICE__HOST":         cfg.BindHost,
		"QDRANT__SERVICE__HTTP_PORT":    strconv.Itoa(cfg.HTTPPort),
		"QDRANT__SERVICE__GRPC_PORT":    strconv.Itoa(cfg.GRPCPort),
		"QDRANT__LOGGER__FORMAT":        "json",
	}
	if ll := strings.TrimSpace(cfg.LogLevel); ll != "" {
		envOverrides["QDRANT__LOGGER__LOG_LEVEL"] = strings.ToUpper(strings.ToLower(ll))
	}
	cmd.Env = mergeEnv(envOverrides)

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
		log.Info("starting qdrant subprocess",
			"msg", "vectorstore.qdrant.starting",
			"bin", bin,
			"storage", absStorage,
			"http_port", cfg.HTTPPort,
			"grpc_port", cfg.GRPCPort,
			"host", cfg.BindHost,
		)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start qdrant: %w", err)
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
