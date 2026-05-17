package freecatalog

import "testing"

func TestParseCatalogIntersectJSON(t *testing.T) {
	raw := []byte(`{"object":"list","data":[{"id":"groq/a"},{"id":"gemini/b"}]}`)
	got, err := ParseCatalogIntersect(raw)
	if err != nil || len(got) != 2 || got[0] != "groq/a" || got[1] != "gemini/b" {
		t.Fatalf("err=%v got=%#v", err, got)
	}
}

func TestParseCatalogIntersectYAML(t *testing.T) {
	raw := []byte(`# header comment
format_version: 1
data:
  - id: groq/x
    name: X
  - id: gemini/y
    other: 1
`)
	got, err := ParseCatalogIntersect(raw)
	if err != nil || len(got) != 2 || got[0] != "groq/x" || got[1] != "gemini/y" {
		t.Fatalf("err=%v got=%#v", err, got)
	}
}

func TestParseCatalogIntersectInvalid(t *testing.T) {
	_, err := ParseCatalogIntersect([]byte(`not: yaml: [[[`))
	if err == nil {
		t.Fatal("expected error")
	}
}
