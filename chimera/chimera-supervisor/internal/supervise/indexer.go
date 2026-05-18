package supervise

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/lynn/porcelain/chimera/chimera-indexer/adapter"
	"github.com/lynn/porcelain/chimera/chimera-supervisor/internal/proc"
)

// IndexerConfig configures a supervised chimera-indexer child.
type IndexerConfig struct {
	Bin          string
	ConfigPath   string
	WorkDir      string
	GatewayURL   string
	GatewayToken string
	LogJSON      bool
	Stdout       io.Writer
	Stderr       io.Writer
	Env          map[string]string
	RawExec      bool
	Args         []string
}

// StartIndexer launches chimera-indexer under the given context.
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
		env := map[string]string{}
		if u := strings.TrimSpace(cfg.GatewayURL); u != "" {
			env[adapter.EnvGatewayURL] = strings.TrimSuffix(u, "/")
		}
		if t := strings.TrimSpace(cfg.GatewayToken); t != "" {
			env[adapter.EnvGatewayToken] = t
		}
		for k, v := range cfg.Env {
			env[k] = v
		}
		cmd.Env = mergeEnv(env)
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
		proc.ApplyNoConsoleWindow(cmd)
		if err := cmd.Start(); err != nil {
			return nil, fmt.Errorf("indexer start: %w", err)
		}
		if log != nil {
			log.Debug("indexer supervised (raw exec)", "msg", "chimera-supervisor.indexer.raw_exec", "bin", bin, "args", cfg.Args)
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
	if err := adapter.EnsureSupervisedConfigFile(cfgPath); err != nil {
		return nil, fmt.Errorf("indexer config file: %w", err)
	}

	argv := []string{"--indexer-backend", "--config", cfgPath}
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
		env[adapter.EnvGatewayURL] = strings.TrimSuffix(u, "/")
	}
	if t := strings.TrimSpace(cfg.GatewayToken); t != "" {
		env[adapter.EnvGatewayToken] = t
	}
	for k, v := range cfg.Env {
		env[k] = v
	}
	cmd.Env = mergeEnv(env)
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
	proc.ApplyNoConsoleWindow(cmd)
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("indexer start: %w", err)
	}
	if log != nil {
		log.Info("indexer supervised", "msg", "chimera-supervisor.indexer.starting", "bin", bin, "config", cfgPath, "workdir", workDir, "log_json", cfg.LogJSON)
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
