package adapter

import (
	"strings"
	"testing"
)

func TestWrapUpstreamLineNormalizesToVectorstorePrefixes(t *testing.T) {
	t.Parallel()
	raw := `{"timestamp":"t","level":"INFO","fields":{"message":"Distributed mode disabled"},"target":"qdrant"}`
	out := WrapUpstreamLine(raw)
	if strings.TrimSpace(out) == "" {
		t.Fatal("expected wrapped output")
	}
	if !strings.Contains(out, `"service":"chimera-vectorstore"`) {
		t.Fatalf("missing chimera-vectorstore service: %s", out)
	}
	if !strings.Contains(out, `"msg":"vectorstore.cluster.single_node"`) {
		t.Fatalf("missing vectorstore msg: %s", out)
	}
}
