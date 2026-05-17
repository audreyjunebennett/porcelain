package freecatalog

import "testing"

func TestAlignEntriesToCatalog_prefersLongestMatch(t *testing.T) {
	e := Entry{
		Provider:  "gemini",
		SourceID:  "gemini-3-flash",
		BiFrostID: "gemini/gemini-3-flash",
	}
	// Catalog omits the short id so alignment picks the longest fuzzy match under gemini/.
	cat := []string{"gemini/gemini-3-flash-preview"}
	out := AlignEntriesToCatalog([]Entry{e}, cat)
	if out[0].BiFrostID != "gemini/gemini-3-flash-preview" {
		t.Fatalf("got %q", out[0].BiFrostID)
	}
}

func TestFilterEntriesByCatalog(t *testing.T) {
	entries := []Entry{
		{Provider: "groq", SourceID: "llama-3.1-8b-instant", BiFrostID: "groq/llama-3.1-8b-instant"},
		{Provider: "groq", SourceID: "missing-from-catalog", BiFrostID: "groq/missing-from-catalog"},
	}
	cat := []string{"groq/llama-3.1-8b-instant"}
	got := FilterEntriesByCatalog(entries, cat)
	if len(got) != 1 || got[0].SourceID != "llama-3.1-8b-instant" {
		t.Fatalf("%#v", got)
	}
}
