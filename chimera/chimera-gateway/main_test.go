package main

import (
	"strings"
	"testing"
	"time"

	"github.com/lynn/porcelain/chimera/internal/wrapper/contract"
)

func TestParseListenHostPort(t *testing.T) {
	t.Parallel()
	host, port, err := parseListenHostPort("127.0.0.1:3000")
	if err != nil {
		t.Fatalf("parse hostport: %v", err)
	}
	if host != "127.0.0.1" || port != 3000 {
		t.Fatalf("unexpected parse result %s:%d", host, port)
	}
	if _, _, err := parseListenHostPort("bad"); err == nil {
		t.Fatal("expected parse error for invalid listen address")
	}
}

func TestDefaultGatewayBackendBin(t *testing.T) {
	t.Setenv("GATEWAY__BACKEND_BIN_DEFAULT", "custom-gateway")
	if got := defaultGatewayBackendBin(); got != "custom-gateway" {
		t.Fatalf("default backend bin=%q", got)
	}
}

func TestUseEmbeddedBackend(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		bin  string
		want bool
	}{
		{name: "empty", bin: "", want: true},
		{name: "supervisor", bin: "chimera-supervisor", want: true},
		{name: "gateway self", bin: "chimera-gateway", want: true},
		{name: "normal backend", bin: "custom-gateway-backend", want: false},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := useEmbeddedBackend(tc.bin); got != tc.want {
				t.Fatalf("useEmbeddedBackend(%q)=%v want %v", tc.bin, got, tc.want)
			}
		})
	}
}

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
	if cfg.BackoffInitial != contract.DefaultBackoffInitial {
		t.Fatalf("backoff initial mismatch: %v", cfg.BackoffInitial)
	}
	if strings.TrimSpace(cfg.Listen) == "" {
		t.Fatal("wrapper listen default should be set")
	}
}

func TestEnvDurationFallback(t *testing.T) {
	t.Setenv("GW_TEST_DURATION", "2s")
	if d := envDuration("GW_TEST_DURATION", time.Second); d != 2*time.Second {
		t.Fatalf("env duration=%v", d)
	}
	t.Setenv("GW_TEST_DURATION", "bad")
	if d := envDuration("GW_TEST_DURATION", time.Second); d != time.Second {
		t.Fatalf("fallback duration=%v", d)
	}
}

func TestWrapGatewayLineNormalizes(t *testing.T) {
	t.Parallel()
	raw := `{"time":"2026-05-14T12:34:56Z","level":"INFO","msg":"gateway.http.access","method":"GET","path":"/health","statusCode":200}`
	out := wrapGatewayLine(raw)
	if strings.TrimSpace(out) == "" {
		t.Fatal("expected wrapped output")
	}
	if !strings.Contains(out, `"service":"gateway"`) {
		t.Fatalf("missing gateway service: %s", out)
	}
	if !strings.Contains(out, `"msg":"gateway.http.access"`) {
		t.Fatalf("missing gateway msg: %s", out)
	}
}
