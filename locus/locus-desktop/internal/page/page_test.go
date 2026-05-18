package page

import (
	"strings"
	"testing"
)

func TestUnreachableDataURL(t *testing.T) {
	got := UnreachableDataURL("http://127.0.0.1:7710", "timeout", true)
	if !strings.HasPrefix(got, "data:text/html") {
		t.Fatalf("expected data URL, got %s", got)
	}
	if !strings.Contains(got, "Cannot%20connect%20to%20supervisor") {
		t.Fatal("missing unreachable heading")
	}
}

func TestRuntimeLossDataURL(t *testing.T) {
	got := RuntimeLossDataURL("http://127.0.0.1:7710", "health checks failed")
	if !strings.HasPrefix(got, "data:text/html") {
		t.Fatalf("expected data URL, got %s", got)
	}
	if !strings.Contains(got, "connection%20lost") {
		t.Fatal("missing runtime loss heading")
	}
}
