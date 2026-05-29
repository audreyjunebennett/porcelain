package rag

import (
	"encoding/base64"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/vectorstore"
	"github.com/lynn/porcelain/internal/naming"
)

func TestSummarizeHits_preview(t *testing.T) {
	long := strings.Repeat("a", 1800)
	got := SummarizeHits([]vectorstore.Hit{{
		Score: 0.91,
		Payload: vectorstore.Payload{
			Source: "docs/guide.md",
			Text:   long,
		},
	}})
	if len(got) != 1 {
		t.Fatalf("len=%d", len(got))
	}
	if got[0].Source != "docs/guide.md" {
		t.Fatalf("source=%q", got[0].Source)
	}
	if got[0].Language != "markdown" {
		t.Fatalf("language=%q", got[0].Language)
	}
	if !strings.HasSuffix(got[0].Text, "…") {
		t.Fatalf("expected truncated text")
	}
}

func TestSummarizeHits_preservesNewlines(t *testing.T) {
	got := SummarizeHits([]vectorstore.Hit{{
		Payload: vectorstore.Payload{
			Source: "main.go",
			Text:   "func main() {\n\tfmt.Println(\"hi\")\n}",
		},
	}})
	if len(got) != 1 {
		t.Fatalf("len=%d", len(got))
	}
	if got[0].Language != "go" {
		t.Fatalf("language=%q", got[0].Language)
	}
	if !strings.Contains(got[0].Text, "\n") {
		t.Fatalf("expected preserved newlines: %q", got[0].Text)
	}
}

func TestLanguageFromSource(t *testing.T) {
	if LanguageFromSource("pkg/foo.go") != "go" {
		t.Fatal("go ext")
	}
	if LanguageFromSource("README") != "" {
		t.Fatal("no ext")
	}
}

func TestWriteResponseHeaders(t *testing.T) {
	rec := httptest.NewRecorder()
	WriteResponseHeaders(rec, "groq/llama", []vectorstore.Hit{{
		Score:   0.5,
		Payload: vectorstore.Payload{Source: "a.txt", Text: "hello"},
	}})
	if got := rec.Header().Get(naming.HeaderUpstreamModelTarget); got != "groq/llama" {
		t.Fatalf("upstream=%q", got)
	}
	raw := rec.Header().Get(naming.HeaderRAGHitsTarget)
	if raw == "" {
		t.Fatal("missing rag hits header")
	}
	if strings.Contains(raw, "a.txt") {
		t.Fatalf("expected base64 header, got raw json: %q", raw)
	}
	decoded, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !strings.Contains(string(decoded), "a.txt") {
		t.Fatalf("decoded=%q", decoded)
	}
}

func TestWriteResponseHeaders_preservesUnicode(t *testing.T) {
	rec := httptest.NewRecorder()
	want := "Phase 1 — Embedding Model Selection"
	WriteResponseHeaders(rec, "vm", []vectorstore.Hit{{
		Score:   0.9,
		Payload: vectorstore.Payload{Source: "docs/plan.md", Text: want},
	}})
	raw := rec.Header().Get(naming.HeaderRAGHitsTarget)
	if strings.Contains(raw, "—") {
		t.Fatalf("header must be ASCII/base64, got raw unicode: %q", raw)
	}
	decoded, err := base64.StdEncoding.DecodeString(raw)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	got := SummarizeHits([]vectorstore.Hit{{
		Payload: vectorstore.Payload{Source: "docs/plan.md", Text: want},
	}})
	if len(got) != 1 || got[0].Text != want {
		t.Fatalf("summarize text=%q want=%q", got[0].Text, want)
	}
	if !strings.Contains(string(decoded), want) {
		t.Fatalf("decoded json missing unicode text: %q", decoded)
	}
}
