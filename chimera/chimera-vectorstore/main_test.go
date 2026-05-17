package main

import (
	"strings"
	"testing"
	"time"

	"github.com/lynn/porcelain/chimera/internal/wrapper/contract"
)

func TestNextBackoffCaps(t *testing.T) {
	t.Parallel()
	cfg := vectorstoreConfig{
		BackoffInitial:    1 * time.Second,
		BackoffMultiplier: 2,
		BackoffMax:        30 * time.Second,
	}
	if d := nextBackoff(cfg, 0); d != 1*time.Second {
		t.Fatalf("attempt0=%v", d)
	}
	if d := nextBackoff(cfg, 3); d != 8*time.Second {
		t.Fatalf("attempt3=%v", d)
	}
	if d := nextBackoff(cfg, 10); d != 30*time.Second {
		t.Fatalf("attempt10=%v", d)
	}
}

func TestPrefixUpstreamMetrics(t *testing.T) {
	t.Parallel()
	in := `# HELP req_total requests
# TYPE req_total counter
req_total{code="200"} 1
plain_metric 2`
	out := prefixUpstreamMetrics(in)
	if !strings.Contains(out, "# HELP upstream_req_total requests") {
		t.Fatalf("missing HELP rewrite: %s", out)
	}
	if !strings.Contains(out, "# TYPE upstream_req_total counter") {
		t.Fatalf("missing TYPE rewrite: %s", out)
	}
	if !strings.Contains(out, "upstream_req_total{code=\"200\"} 1") {
		t.Fatalf("missing metric rewrite: %s", out)
	}
	if !strings.Contains(out, "upstream_plain_metric 2") {
		t.Fatalf("missing simple rewrite: %s", out)
	}
}

func TestIsLoopbackBind(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		addr string
		want bool
	}{
		{"127.0.0.1:1234", true},
		{"localhost:1234", true},
		{"[::1]:1234", true},
		{"0.0.0.0:1234", false},
		{"192.168.0.4:1234", false},
	} {
		if got := isLoopbackBind(tc.addr); got != tc.want {
			t.Fatalf("%s => %v (want %v)", tc.addr, got, tc.want)
		}
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
	if cfg.Backend != "qdrant" {
		t.Fatalf("backend default mismatch: %s", cfg.Backend)
	}
	if cfg.Endpoint != "127.0.0.1:6333" {
		t.Fatalf("endpoint default mismatch: %s", cfg.Endpoint)
	}
	if cfg.DataPath != "data/qdrant" {
		t.Fatalf("data path default mismatch: %s", cfg.DataPath)
	}
}

func TestParseEndpoint(t *testing.T) {
	t.Parallel()
	host, port, err := parseEndpoint("127.0.0.1:6333")
	if err != nil {
		t.Fatalf("parse endpoint: %v", err)
	}
	if host != "127.0.0.1" || port != 6333 {
		t.Fatalf("unexpected parse result %s:%d", host, port)
	}
	if _, _, err := parseEndpoint("bad-endpoint"); err == nil {
		t.Fatal("expected parse error for invalid endpoint")
	}
}

func TestWrapVectorstoreLineNormalizesToVectorstorePrefixes(t *testing.T) {
	t.Parallel()
	raw := `{"timestamp":"t","level":"INFO","fields":{"message":"Distributed mode disabled"},"target":"qdrant"}`
	out := wrapVectorstoreLine(raw)
	if strings.TrimSpace(out) == "" {
		t.Fatal("expected wrapped output")
	}
	if !strings.Contains(out, `"service":"vectorstore"`) {
		t.Fatalf("missing vectorstore service: %s", out)
	}
	if !strings.Contains(out, `"msg":"vectorstore.cluster.single_node"`) {
		t.Fatalf("missing vectorstore msg: %s", out)
	}
}
