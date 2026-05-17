package main

import (
	"io"
	"strings"
	"testing"

	"github.com/lynn/porcelain/chimera/internal/wrapper/contract"
)

func TestParseConfigDefaults(t *testing.T) {
	t.Parallel()
	cfg, err := parseConfig(nil)
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

func TestParseConfigVersion(t *testing.T) {
	t.Parallel()
	_, err := parseConfig([]string{"--version"})
	if err == nil {
		t.Fatal("expected io.EOF for version")
	}
	if err != io.EOF {
		t.Fatalf("expected io.EOF, got %v", err)
	}
}

func TestWrapIndexerLineNormalizes(t *testing.T) {
	t.Parallel()
	raw := `{"msg":"indexer.run.start","service":"indexer"}`
	out := wrapIndexerLine(raw)
	if strings.TrimSpace(out) == "" {
		t.Fatal("expected wrapped output")
	}
	if !strings.Contains(out, `"service":"indexer"`) {
		t.Fatalf("missing indexer service in wrapped line: %s", out)
	}
}
