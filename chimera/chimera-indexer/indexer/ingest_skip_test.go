package indexer

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFileHasNoIngestableText(t *testing.T) {
	dir := t.TempDir()
	tests := []struct {
		name    string
		body    string
		wantYes bool
	}{
		{"empty", "", true},
		{"only_newlines", "\n\n\r\n", true},
		{"only_spaces", "   \t  ", true},
		{"gitkeep_like", "\n", true},
		{"content", "x", false},
		{"trimmed_content", "\n  hello\n", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p := filepath.Join(dir, tc.name)
			if err := os.WriteFile(p, []byte(tc.body), 0o644); err != nil {
				t.Fatal(err)
			}
			got, err := fileHasNoIngestableText(p)
			if err != nil {
				t.Fatal(err)
			}
			if got != tc.wantYes {
				t.Fatalf("fileHasNoIngestableText(%q) = %v, want %v", tc.body, got, tc.wantYes)
			}
		})
	}
}
