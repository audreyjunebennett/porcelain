package indexer

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureSupervisedConfigFile_createsOnce(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "nested", "idx.yaml")
	if err := EnsureSupervisedConfigFile(p); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(b) < 20 || string(b) == "" {
		t.Fatalf("unexpected content: %q", b)
	}
	if err := EnsureSupervisedConfigFile(p); err != nil {
		t.Fatal(err)
	}
	b2, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != string(b2) {
		t.Fatal("second call should not overwrite")
	}
}
