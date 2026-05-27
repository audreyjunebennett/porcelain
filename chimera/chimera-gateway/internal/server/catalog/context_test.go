package catalog

import (
	"testing"
	"time"
)

func TestCatalogSnapshot_ContextLength(t *testing.T) {
	t.Parallel()
	var nilSnap *CatalogSnapshot
	if n, ok := nilSnap.ContextLength("groq/x"); ok || n != 0 {
		t.Fatalf("nil snapshot: n=%d ok=%v", n, ok)
	}
	snap := NewTestSnapshotWithModelContext(time.Now(), map[string]int64{
		"groq/x": 131072,
	})
	if n, ok := snap.ContextLength("groq/x"); !ok || n != 131072 {
		t.Fatalf("got %d ok=%v", n, ok)
	}
	if _, ok := snap.ContextLength("missing"); ok {
		t.Fatal("expected missing model")
	}
}

func TestInt64FromCatalogField(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in   any
		want int64
		ok   bool
	}{
		{131072, 131072, true},
		{float64(1.31072e5), 131072, true},
		{0, 0, false},
		{"nope", 0, false},
	}
	for _, tc := range cases {
		got, ok := int64FromCatalogField(tc.in)
		if got != tc.want || ok != tc.ok {
			t.Fatalf("%v -> (%d,%v) want (%d,%v)", tc.in, got, ok, tc.want, tc.ok)
		}
	}
}
