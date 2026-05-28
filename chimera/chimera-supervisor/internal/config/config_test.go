package config

import (
	"errors"
	"io"
	"testing"
	"time"
)

func TestParseDefaults(t *testing.T) {
	t.Parallel()
	cfg, err := Parse(nil, BuildInfo{})
	if err != nil {
		t.Fatalf("parse defaults: %v", err)
	}
	if cfg.Listen != "127.0.0.1:7710" {
		t.Fatalf("listen default mismatch: %s", cfg.Listen)
	}
	if cfg.WaitGateway != 60*time.Second {
		t.Fatalf("wait-gateway default mismatch: %v", cfg.WaitGateway)
	}
	if !cfg.LogJSON {
		t.Fatal("log-json default want true")
	}
	if cfg.ShutdownTimeout != 10*time.Second {
		t.Fatalf("shutdown-timeout default mismatch: %v", cfg.ShutdownTimeout)
	}
	if cfg.TerminateWait != 5*time.Second {
		t.Fatalf("terminate-wait default mismatch: %v", cfg.TerminateWait)
	}
}

func TestParseVersionEOF(t *testing.T) {
	t.Parallel()
	_, err := Parse([]string{"-version"}, BuildInfo{Version: "test"})
	if !errors.Is(err, io.EOF) {
		t.Fatalf("parse -version: %v want EOF", err)
	}
}
