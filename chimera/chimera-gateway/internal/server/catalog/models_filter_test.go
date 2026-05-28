package catalog

import (
	"testing"

	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/providermodels"
)

func TestFilterOpenAIModelDataByAvailability(t *testing.T) {
	data := []any{
		map[string]any{"id": "groq/a"},
		map[string]any{"id": "groq/b"},
		map[string]any{"id": "gemini/c"},
	}
	snap := &providermodels.TenantSnapshot{}
	snap.SetUnavailableForTest(map[string]struct{}{
		"groq/b": {},
	})
	got := FilterOpenAIModelDataByAvailability(data, snap)
	if len(got) != 2 {
		t.Fatalf("len=%d want 2", len(got))
	}
	m0, _ := got[0].(map[string]any)
	m1, _ := got[1].(map[string]any)
	if m0["id"] != "groq/a" || m1["id"] != "gemini/c" {
		t.Fatalf("got=%#v", got)
	}
	if len(FilterOpenAIModelDataByAvailability(data, nil)) != 3 {
		t.Fatal("nil snapshot should not filter")
	}
}
