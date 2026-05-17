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

// QdrantConfig controls a native Qdrant process (default ports 6333/6334).
type QdrantConfig struct {
	// Bin is the Qdrant executable (PATH or path, e.g. ./bin/qdrant).
	Bin string
	// StorageDir is persisted vector data (QDRANT__STORAGE__STORAGE_PATH).
	StorageDir string
	// BindHost is QDRANT__SERVICE__HOST (e.g. 127.0.0.1).
	BindHost string
	// HTTPPort is QDRANT__SERVICE__HTTP_PORT (REST, default 6333).
	HTTPPort int
	// GRPCPort is QDRANT__SERVICE__GRPC_PORT (default 6334).
	GRPCPort int
	// LogLevel sets QDRANT__LOGGER__LOG_LEVEL when non-empty.
	LogLevel string
	// RawExec runs Bin with Args only (tests).
	RawExec bool
	Args    []string
	// Stdout and Stderr default to os.Stdout / os.Stderr when nil.
	Stdout io.Writer
	Stderr io.Writer
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

// StartQdrant starts the Qdrant binary with env-based config.
func StartQdrant(ctx context.Context, cfg QdrantConfig, log *slog.Logger) (*exec.Cmd, error) {
	bin := strings.TrimSpace(cfg.Bin)
	if bin == "" {
		return nil, fmt.Errorf("qdrant: empty Bin")
	}
	var err error
	bin, err = absBinIfNeeded(bin)
	if err != nil {
		return nil, fmt.Errorf("resolve qdrant binary path: %w", err)
	}
	var argv []string
	var absStorage string
	if cfg.RawExec {
		argv = append(argv, cfg.Args...)
	} else {
		if err := os.MkdirAll(cfg.StorageDir, 0o755); err != nil {
			return nil, fmt.Errorf("qdrant storage dir: %w", err)
		}
		absStorage, err = filepath.Abs(cfg.StorageDir)
		if err != nil {
			return nil, fmt.Errorf("resolve qdrant storage dir: %w", err)
		}
		argv = []string{}
	}
	cmd := exec.CommandContext(ctx, bin, argv...)
	if cfg.RawExec {
		cmd.Dir = cfg.StorageDir
		cmd.Env = os.Environ()
	} else {
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
		cmd.Env = MergeEnv(envOverrides)
	}
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
			log.Info("starting qdrant subprocess", "msg", "gateway.supervisor.qdrant.starting", "bin", bin, "raw", true)
		} else {
			log.Info("starting qdrant subprocess", "msg", "gateway.supervisor.qdrant.starting", "bin", bin, "storage", absStorage, "http_port", cfg.HTTPPort, "grpc_port", cfg.GRPCPort, "host", cfg.BindHost)
		}
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start qdrant: %w", err)
	}
	return cmd, nil
}
