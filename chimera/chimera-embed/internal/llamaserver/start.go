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

// Config controls a llama-server child process for embedding-only mode.
type Config struct {
	Bin        string
	ModelPath  string
	CacheDir   string
	BindHost   string
	Port       int
	CtxSize    int
	NGPULayers int
	Pooling    string
	LogLevel   string
	Stdout     io.Writer
	Stderr     io.Writer
}

// Start launches llama-server with --embedding.
func Start(ctx context.Context, cfg Config, log *slog.Logger) (*exec.Cmd, error) {
	bin := strings.TrimSpace(cfg.Bin)
	if bin == "" {
		return nil, fmt.Errorf("llama-server: empty Bin")
	}
	var err error
	bin, err = absBinIfNeeded(bin)
	if err != nil {
		return nil, fmt.Errorf("resolve llama-server binary path: %w", err)
	}
	modelPath := strings.TrimSpace(cfg.ModelPath)
	if modelPath == "" {
		return nil, fmt.Errorf("llama-server: empty ModelPath")
	}
	modelPath, err = filepath.Abs(modelPath)
	if err != nil {
		return nil, fmt.Errorf("resolve model path: %w", err)
	}
	if st, err := os.Stat(modelPath); err != nil || st.IsDir() {
		if err != nil {
			return nil, fmt.Errorf("llama-server model missing at %s: %w", modelPath, err)
		}
		return nil, fmt.Errorf("llama-server model path is not a file: %s", modelPath)
	}
	cacheDir := strings.TrimSpace(cfg.CacheDir)
	if cacheDir != "" {
		if err := os.MkdirAll(cacheDir, 0o755); err != nil {
			return nil, fmt.Errorf("llama-server cache dir: %w", err)
		}
	}

	pool := strings.TrimSpace(cfg.Pooling)
	if pool == "" {
		pool = "mean"
	}
	ctxSize := cfg.CtxSize
	if ctxSize <= 0 {
		ctxSize = 2048
	}
	argv := []string{
		"-m", modelPath,
		"--embedding",
		"--host", cfg.BindHost,
		"--port", strconv.Itoa(cfg.Port),
		"-c", strconv.Itoa(ctxSize),
		"--n-gpu-layers", strconv.Itoa(cfg.NGPULayers),
		"--pooling", pool,
	}

	// Do not use CommandContext: wrapper shutdown uses TerminateThenKill.
	_ = ctx
	cmd := exec.Command(bin, argv...)
	if cacheDir != "" {
		cmd.Dir = cacheDir
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
		log.Info("starting llama-server subprocess",
			"msg", "embed.llama_server.starting",
			"bin", bin,
			"model", modelPath,
			"host", cfg.BindHost,
			"port", cfg.Port,
		)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start llama-server: %w", err)
	}
	return cmd, nil
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
