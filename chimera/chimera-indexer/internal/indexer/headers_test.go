package indexer

import (
	"testing"

	"github.com/lynn/porcelain/internal/naming"
)

func TestScopeHTTPHeaders(t *testing.T) {
	if ScopeHTTPHeaders("", "") != nil {
		t.Fatal("expected nil for empty scope")
	}
	h := ScopeHTTPHeaders("acme", "prod")
	if h[naming.HeaderProjectTarget] != "acme" {
		t.Fatalf("project=%q", h[naming.HeaderProjectTarget])
	}
	if h[naming.HeaderFlavorTarget] != "prod" {
		t.Fatalf("flavor=%q", h[naming.HeaderFlavorTarget])
	}
}
