package supervisor

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
	// LogLevel sets QDRANT__LOGGER__LOG_LEVEL when non-empty (e.g. DEBUG, INFO).
	LogLevel string
	// RawExec runs Bin with Args only (tests).
	RawExec bool
	Args    []string
	// Stdout and Stderr default to os.Stdout / os.Stderr when nil.
	Stdout io.Writer
	Stderr io.Writer
}

// StartQdrant starts the Qdrant binary with env-based config (see https://qdrant.tech/documentation/guides/configuration/).
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
		argv = []string{} // Qdrant reads config from env
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
			// One JSON object per line on stdout/stderr — see https://qdrant.tech/documentation/ops-configuration/configuration/
			"QDRANT__LOGGER__FORMAT": "json",
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
