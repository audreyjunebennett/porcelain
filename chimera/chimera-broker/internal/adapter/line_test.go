package adapter

import (
	"strings"
	"testing"

	"github.com/lynn/porcelain/internal/naming"
)

func TestWrapUpstreamLineNormalizesToBrokerPrefixes(t *testing.T) {
	t.Parallel()
	raw := `{"level":"info","time":"2026-05-08T14:15:51-05:00","message":"successfully started chimera-broker, serving UI on http://127.0.0.1:8080"}`
	out := WrapUpstreamLine(raw)
	if strings.TrimSpace(out) == "" {
		t.Fatal("expected wrapped output")
	}
	if !strings.Contains(out, `"service":"`+naming.ProductBrokerName+`"`) {
		t.Fatalf("missing chimera-broker service: %s", out)
	}
	if !strings.Contains(out, `"msg":"broker.ready"`) {
		t.Fatalf("missing broker msg: %s", out)
	}
}
