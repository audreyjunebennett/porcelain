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

// GatewayConfig controls a native chimera-gateway backend process.
type GatewayConfig struct {
	// Bin is the gateway executable (PATH or path, e.g. ./bin/chimera-gateway-backend).
	Bin string
	// ConfigPath is passed as -config to the gateway process.
	ConfigPath string
	// BindHost is translated into -listen host:port.
	BindHost string
	// Port is translated into -listen host:port.
	Port int
	// ExtraArgs are appended after -config/-listen.
	ExtraArgs []string
	// RawExec runs Bin with Args only (tests).
	RawExec bool
	Args    []string
	// Stdout and Stderr default to os.Stdout / os.Stderr when nil.
	Stdout io.Writer
	Stderr io.Writer
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

// StartGateway starts the gateway backend process.
func StartGateway(ctx context.Context, cfg GatewayConfig, log *slog.Logger) (*exec.Cmd, error) {
	bin := strings.TrimSpace(cfg.Bin)
	if bin == "" {
		return nil, fmt.Errorf("gateway: empty Bin")
	}
	var err error
	bin, err = absBinIfNeeded(bin)
	if err != nil {
		return nil, fmt.Errorf("resolve gateway binary path: %w", err)
	}
	var argv []string
	if cfg.RawExec {
		argv = append(argv, cfg.Args...)
	} else {
		listen := strings.TrimSpace(cfg.BindHost) + ":" + strconv.Itoa(cfg.Port)
		argv = []string{"-config", strings.TrimSpace(cfg.ConfigPath), "-listen", listen}
		argv = append(argv, cfg.ExtraArgs...)
	}
	cmd := exec.CommandContext(ctx, bin, argv...)
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
	if log != nil {
		if cfg.RawExec {
			log.Info("starting gateway subprocess", "msg", "gateway.supervisor.gateway.starting", "bin", bin, "raw", true)
		} else {
			log.Info("starting gateway subprocess", "msg", "gateway.supervisor.gateway.starting", "bin", bin, "config", cfg.ConfigPath, "host", cfg.BindHost, "port", cfg.Port)
		}
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start gateway: %w", err)
	}
	return cmd, nil
}
