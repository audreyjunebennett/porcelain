package indexer

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMatcher_Defaults(t *testing.T) {
	m, err := NewMatcher(t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		path string
		want bool
	}{
		{".env", true},
		{"node_modules/foo/index.js", true},
		{".git/config", true},
		{"src/main.go", false},
		{"docs/readme.md", false},
		{"image.png", true},
	}
	for _, c := range cases {
		if got := m.Match(c.path); got != c.want {
			t.Fatalf("Match(%q) = %v, want %v", c.path, got, c.want)
		}
	}
}

func TestMatcher_GitignoreAndChimeraignore(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte("secret/\n*.tmp\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".chimeraignore"), []byte("docs/private/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := NewMatcher(root, []string{"*.bak"})
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"secret/keys.txt", "scratch.tmp", "docs/private/note.md", "old.bak"} {
		if !m.Match(p) {
			t.Fatalf("expected %q to match", p)
		}
	}
	for _, p := range []string{"src/app.go", "docs/public/note.md"} {
		if m.Match(p) {
			t.Fatalf("expected %q NOT to match", p)
		}
	}
}

func TestLooksBinary(t *testing.T) {
	text := []byte("hello world\nthis is text\n")
	if looksBinary(text, 0.001) {
		t.Fatal("text flagged as binary")
	}
	bin := append([]byte("plain"), 0x00, 0x01, 0x02)
	if !looksBinary(bin, 0.001) {
		t.Fatal("binary not detected")
	}
}

func TestIsBinaryFile(t *testing.T) {
	dir := t.TempDir()
	good := filepath.Join(dir, "a.txt")
	bad := filepath.Join(dir, "a.bin")
	if err := os.WriteFile(good, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bad, []byte{0x00, 0x01, 0x02, 0x03}, 0o644); err != nil {
		t.Fatal(err)
	}
	if b, err := IsBinaryFile(good, 1024, 0.001); err != nil || b {
		t.Fatalf("good: bin=%v err=%v", b, err)
	}
	if b, err := IsBinaryFile(bad, 1024, 0.001); err != nil || !b {
		t.Fatalf("bad: bin=%v err=%v", b, err)
	}
}
