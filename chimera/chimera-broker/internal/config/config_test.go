package config

import (
	"errors"
	"io"
	"testing"

	"github.com/lynn/porcelain/chimera/internal/wrapper/contract"
	"github.com/lynn/porcelain/internal/naming"
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
	if cfg.BackoffInitial != contract.DefaultBackoffInitial {
		t.Fatalf("backoff initial mismatch: %v", cfg.BackoffInitial)
	}
	if cfg.Backend != naming.ProductBrokerName {
		t.Fatalf("backend default mismatch: %s", cfg.Backend)
	}
	if cfg.Endpoint != naming.DefaultBrokerEndpoint {
		t.Fatalf("endpoint default mismatch: %s", cfg.Endpoint)
	}
	if cfg.DataPath != naming.DefaultBrokerDataPath {
		t.Fatalf("data path default mismatch: %s", cfg.DataPath)
	}
	if cfg.Bin != naming.ProductBrokerHTTPBinName {
		t.Fatalf("bin default mismatch: %s", cfg.Bin)
	}
}

func TestParseEndpoint(t *testing.T) {
	t.Parallel()
	host, port, err := ParseEndpoint("127.0.0.1:8080")
	if err != nil {
		t.Fatalf("parse endpoint: %v", err)
	}
	if host != "127.0.0.1" || port != 8080 {
		t.Fatalf("unexpected parse result %s:%d", host, port)
	}
	if _, _, err := ParseEndpoint("bad-endpoint"); err == nil {
		t.Fatal("expected parse error for invalid endpoint")
	}
}

func TestParseVersionEOF(t *testing.T) {
	t.Parallel()
	_, err := Parse([]string{"-version"}, BuildInfo{Version: "1.0"})
	if !errors.Is(err, io.EOF) {
		t.Fatalf("expected EOF for version flag, got %v", err)
	}
}
