package llamaserver

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

type Config struct {
	Bin        string
	ModelPath  string
	CacheDir   string
	BindHost   string
	Port       int
	CtxSize    int
	NGPULayers int
	Pooling    string
	Stdout     io.Writer
	Stderr     io.Writer
}

// Command resolves and validates the operator-supplied runtime and model.
func Command(cfg Config) (*exec.Cmd, error) {
	bin := strings.TrimSpace(cfg.Bin)
	if bin == "" {
		return nil, fmt.Errorf("llama-server: empty binary")
	}
	if strings.ContainsAny(bin, `/\`) && !filepath.IsAbs(bin) {
		var err error
		bin, err = filepath.Abs(bin)
		if err != nil {
			return nil, fmt.Errorf("resolve llama-server binary: %w", err)
		}
	}
	modelPath := strings.TrimSpace(cfg.ModelPath)
	if modelPath == "" {
		return nil, fmt.Errorf("llama-server: empty model path")
	}
	var err error
	modelPath, err = filepath.Abs(modelPath)
	if err != nil {
		return nil, fmt.Errorf("resolve model path: %w", err)
	}
	if info, statErr := os.Stat(modelPath); statErr != nil {
		return nil, fmt.Errorf("llama-server model missing at %s: %w", modelPath, statErr)
	} else if info.IsDir() {
		return nil, fmt.Errorf("llama-server model path is not a file: %s", modelPath)
	}

	ctxSize := cfg.CtxSize
	if ctxSize <= 0 {
		ctxSize = 2048
	}
	pooling := strings.TrimSpace(cfg.Pooling)
	if pooling == "" {
		pooling = "mean"
	}
	args := []string{
		"-m", modelPath,
		"--embedding",
		"--host", strings.TrimSpace(cfg.BindHost),
		"--port", strconv.Itoa(cfg.Port),
		"-c", strconv.Itoa(ctxSize),
		"--n-gpu-layers", strconv.Itoa(cfg.NGPULayers),
		"--pooling", pooling,
		"--metrics",
	}
	cmd := exec.Command(bin, args...)
	cacheDir := strings.TrimSpace(cfg.CacheDir)
	if cacheDir != "" {
		cacheDir, err = filepath.Abs(cacheDir)
		if err != nil {
			return nil, fmt.Errorf("resolve cache dir: %w", err)
		}
		if err := os.MkdirAll(cacheDir, 0o755); err != nil {
			return nil, fmt.Errorf("create llama-server cache dir: %w", err)
		}
		cmd.Dir = cacheDir
	}
	return cmd, nil
}

func Start(ctx context.Context, cfg Config, log *slog.Logger) (*exec.Cmd, error) {
	_ = ctx // shared wrapper runtime owns graceful termination.
	cmd, err := Command(cfg)
	if err != nil {
		return nil, err
	}
	cmd.Stdout = cfg.Stdout
	if cmd.Stdout == nil {
		cmd.Stdout = os.Stdout
	}
	cmd.Stderr = cfg.Stderr
	if cmd.Stderr == nil {
		cmd.Stderr = os.Stderr
	}
	applyNoConsoleWindow(cmd)
	if log != nil {
		log.Info("starting llama-server embedding backend", "msg", "embed.llama_server.starting", "bin", cmd.Path, "model", cfg.ModelPath, "host", cfg.BindHost, "port", cfg.Port)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start llama-server: %w", err)
	}
	return cmd, nil
}
