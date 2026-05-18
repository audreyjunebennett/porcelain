package config

import (
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/lynn/porcelain/chimera/internal/wrapper/contract"
)

func TestParseDefaults(t *testing.T) {
	t.Parallel()
	cfg, err := Parse(nil, BuildInfo{})
	if err != nil {
		t.Fatalf("parse defaults: %v", err)
	}
	if cfg.StartupTimeout != contract.DefaultStartupTimeout {
		t.Fatalf("startup timeout default mismatch: %v", cfg.StartupTimeout)
	}
	if cfg.ShutdownTimeout != contract.DefaultShutdownTimeout {
		t.Fatalf("shutdown timeout default mismatch: %v", cfg.ShutdownTimeout)
	}
	if strings.TrimSpace(cfg.Listen) == "" {
		t.Fatal("wrapper listen default should be set")
	}
}

func TestParseVersionEOF(t *testing.T) {
	t.Parallel()
	_, err := Parse([]string{"--version"}, BuildInfo{Version: "test"})
	if !errors.Is(err, io.EOF) {
		t.Fatalf("parse -version: %v want EOF", err)
	}
}
