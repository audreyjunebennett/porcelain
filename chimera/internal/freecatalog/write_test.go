package freecatalog

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWriteCatalogYAML_comments(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "out.yaml")
	groqLim := GroqLimits{RPM: "30", RPD: "14.4K", TPM: "6K", TPD: "500K", ASH: "-", ASD: "-"}
	n, err := WriteCatalogYAML(p, time.Unix(0, 0).UTC(), "https://groq.example/rate", "https://gemini.example/price", []Entry{
		{Provider: "groq", SourceID: "llama-3.1-8b-instant", BiFrostID: "groq/llama-3.1-8b-instant", Groq: &groqLim},
		{Provider: "gemini", SourceID: "gemini-2.0-flash", BiFrostID: "gemini/gemini-2.0-flash"},
	})
	if err != nil || n != 2 {
		t.Fatalf("n=%d err=%v", n, err)
	}
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if !strings.Contains(s, "groq/llama-3.1-8b-instant") || !strings.Contains(s, "# llama-3.1-8b-instant | 30 | 14.4K | 6K | 500K | - | -") {
		t.Fatalf("%s", s)
	}
}
