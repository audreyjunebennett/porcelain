package adapter

import (
	"strings"
	"testing"
)

func TestWrapUpstreamLineNormalizesIndexerState(t *testing.T) {
	t.Parallel()
	raw := `{"msg":"indexer.state","service":"indexer","state":"watch_idle","recovery":false}`
	out := WrapUpstreamLine(raw)
	if strings.TrimSpace(out) == "" {
		t.Fatal("expected wrapped output")
	}
	if !strings.Contains(out, `"service":"chimera-indexer"`) {
		t.Fatalf("missing indexer service: %s", out)
	}
	if !strings.Contains(out, `"msg":"indexer.state"`) {
		t.Fatalf("missing indexer.state msg: %s", out)
	}
}

func TestWrapUpstreamLineRunStart(t *testing.T) {
	t.Parallel()
	raw := `{"msg":"chimera-indexer.run.start","service":"chimera-indexer"}`
	out := WrapUpstreamLine(raw)
	if strings.TrimSpace(out) == "" {
		t.Fatal("expected wrapped output")
	}
	if !strings.Contains(out, `"service":"chimera-indexer"`) {
		t.Fatalf("missing chimera-indexer service in wrapped line: %s", out)
	}
}
