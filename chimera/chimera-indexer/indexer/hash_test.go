package indexer

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHashFile(t *testing.T) {
	dir := t.TempDir()
	body := []byte("hello world")
	p := filepath.Join(dir, "x.txt")
	if err := os.WriteFile(p, body, 0o644); err != nil {
		t.Fatal(err)
	}
	got, n, err := HashFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if n != int64(len(body)) {
		t.Fatalf("size %d != %d", n, len(body))
	}
	want := "sha256:" + hex.EncodeToString(sumOf(body))
	if got != want {
		t.Fatalf("hash %q != %q", got, want)
	}
	if !strings.HasPrefix(got, "sha256:") {
		t.Fatal("missing sha256 prefix")
	}
}

func sumOf(b []byte) []byte {
	h := sha256.Sum256(b)
	return h[:]
}
