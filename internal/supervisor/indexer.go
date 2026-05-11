package supervisor

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/lynn/claudia-gateway/internal/indexer"
)

// IndexerConfig starts claudia-index under claudia serve / desktop (v0.5).
type IndexerConfig struct {
	// Bin is the claudia-index executable (PATH or path).
	Bin string
	// ConfigPath is the absolute path passed as --config (single merged file).
	ConfigPath string
	// WorkDir is the process working directory (e.g. gateway repo root for .env).
	WorkDir string
	// GatewayURL is set as CLAUDIA_GATEWAY_URL for the child (overrides YAML).
	GatewayURL string
	// LogJSON adds --log-json for structured stderr (v0.5).
	LogJSON bool
	// Stdout and Stderr default to os.Stdout / os.Stderr when nil.
	Stdout io.Writer
	Stderr io.Writer

	// RawExec runs Bin with Args only (tests).
	RawExec bool
	Args    []string
}

// StartIndexer starts claudia-index with --config. ctx cancel kills the process.
func StartIndexer(ctx context.Context, cfg IndexerConfig, log *slog.Logger) (*exec.Cmd, error) {
	if cfg.RawExec {
		bin := strings.TrimSpace(cfg.Bin)
		if bin == "" {
			return nil, fmt.Errorf("indexer: empty Bin")
		}
		var err error
		bin, err = absBinIfNeeded(bin)
		if err != nil {
			return nil, fmt.Errorf("resolve indexer binary path: %w", err)
		}
		cmd := exec.CommandContext(ctx, bin, cfg.Args...)
		cmd.Dir = strings.TrimSpace(cfg.WorkDir)
		if cmd.Dir == "" {
			wd, err := os.Getwd()
			if err != nil {
				return nil, fmt.Errorf("indexer work dir: %w", err)
			}
			cmd.Dir = wd
		}
		cmd.Env = os.Environ()
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
		if err := cmd.Start(); err != nil {
			return nil, fmt.Errorf("indexer start: %w", err)
		}
		if log != nil {
			log.Debug("indexer supervised (raw exec)", "msg", "gateway.supervisor.indexer.raw_exec", "bin", bin, "args", cfg.Args)
		}
		return cmd, nil
	}

	bin := strings.TrimSpace(cfg.Bin)
	if bin == "" {
		return nil, fmt.Errorf("indexer: empty Bin")
	}
	var err error
	bin, err = absBinIfNeeded(bin)
	if err != nil {
		return nil, fmt.Errorf("resolve indexer binary path: %w", err)
	}
	cfgPath := strings.TrimSpace(cfg.ConfigPath)
	if cfgPath == "" {
		return nil, fmt.Errorf("indexer: empty ConfigPath")
	}
	cfgPath, err = filepath.Abs(cfgPath)
	if err != nil {
		return nil, fmt.Errorf("indexer config path: %w", err)
	}
	if err := indexer.EnsureSupervisedConfigFile(cfgPath); err != nil {
		return nil, fmt.Errorf("indexer config file: %w", err)
	}

	argv := []string{"--config", cfgPath}
	if cfg.LogJSON {
		argv = append(argv, "--log-json")
	}
	workDir := strings.TrimSpace(cfg.WorkDir)
	if workDir == "" {
		return nil, fmt.Errorf("indexer: empty WorkDir")
	}

	cmd := exec.CommandContext(ctx, bin, argv...)
	cmd.Dir = workDir
	env := map[string]string{}
	if u := strings.TrimSpace(cfg.GatewayURL); u != "" {
		env[indexer.EnvGatewayURL] = strings.TrimSuffix(u, "/")
	}
	cmd.Env = MergeEnv(env)

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

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("indexer start: %w", err)
	}
	if log != nil {
		log.Info("indexer supervised", "msg", "gateway.supervisor.indexer.starting", "bin", bin, "config", cfgPath, "workdir", workDir, "log_json", cfg.LogJSON)
	}
	return cmd, nil
}
