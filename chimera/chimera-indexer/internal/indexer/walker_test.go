package indexer

import (
	"os"
	"path/filepath"
	"sort"
	"testing"
)

func TestWalk_RespectsIgnoresAndBinary(t *testing.T) {
	root := t.TempDir()
	mustWrite(t, filepath.Join(root, "src", "main.go"), "package main\n")
	mustWrite(t, filepath.Join(root, "docs", "readme.md"), "# hi\n")
	mustWrite(t, filepath.Join(root, ".env"), "SECRET=1\n")
	mustWrite(t, filepath.Join(root, "node_modules", "x.js"), "module.exports={}\n")
	mustWriteBytes(t, filepath.Join(root, "image.dat"), []byte{0x00, 0x01, 0x02})
	mustWrite(t, filepath.Join(root, ".gitignore"), "scratch/\n")
	mustWrite(t, filepath.Join(root, "scratch", "tmp.txt"), "x\n")

	m, err := NewMatcher(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	r := Root{ID: "r", AbsPath: root}
	cands, err := Walk(r, WalkOptions{
		Matcher:              m,
		MaxFileBytes:         1 << 20,
		BinaryNullByteSample: 1024,
		BinaryNullByteRatio:  0.001,
	})
	if err != nil {
		t.Fatal(err)
	}
	got := make([]string, 0, len(cands))
	for _, c := range cands {
		got = append(got, c.RelPath)
	}
	sort.Strings(got)
	want := []string{"docs/readme.md", "src/main.go"}
	if len(got) != len(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("got %v want %v", got, want)
		}
	}
}

func TestWalk_SkipsOversize(t *testing.T) {
	root := t.TempDir()
	big := make([]byte, 10*1024)
	for i := range big {
		big[i] = 'a'
	}
	mustWriteBytes(t, filepath.Join(root, "big.txt"), big)
	mustWrite(t, filepath.Join(root, "small.txt"), "ok\n")
	m, _ := NewMatcher(root, nil)
	skipped := map[string]string{}
	cands, err := Walk(Root{ID: "r", AbsPath: root}, WalkOptions{
		Matcher:              m,
		MaxFileBytes:         100,
		BinaryNullByteSample: 1024,
		BinaryNullByteRatio:  0.001,
		OnSkip:               func(rel, reason string) { skipped[rel] = reason },
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(cands) != 1 || cands[0].RelPath != "small.txt" {
		t.Fatalf("cands=%v", cands)
	}
	if skipped["big.txt"] == "" {
		t.Fatalf("expected big.txt skipped, got %v", skipped)
	}
}

func mustWrite(t *testing.T, path, body string) {
	mustWriteBytes(t, path, []byte(body))
}

func mustWriteBytes(t *testing.T, path string, body []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err)
	}
}
