package envfile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUpsertIfAbsent_createsFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, ".env")
	res, err := UpsertIfAbsent(p, "CHIMERA_GATEWAY_TOKEN", "abc123")
	if err != nil {
		t.Fatal(err)
	}
	if !res.Written || res.Reason != "created" {
		t.Fatalf("res: %+v", res)
	}
	raw, _ := os.ReadFile(p)
	if !strings.Contains(string(raw), "CHIMERA_GATEWAY_TOKEN=abc123") {
		t.Fatalf("file: %q", raw)
	}
}

func TestUpsertIfAbsent_skipsWhenSet(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, ".env")
	if err := os.WriteFile(p, []byte("CHIMERA_GATEWAY_TOKEN=existing\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	res, err := UpsertIfAbsent(p, "CHIMERA_GATEWAY_TOKEN", "new")
	if err != nil {
		t.Fatal(err)
	}
	if res.Written || res.Reason != "already_set" {
		t.Fatalf("res: %+v", res)
	}
	raw, _ := os.ReadFile(p)
	if string(raw) != "CHIMERA_GATEWAY_TOKEN=existing\n" {
		t.Fatalf("file changed: %q", raw)
	}
}

func TestUpsertIfAbsent_ignoresCommentedKey(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, ".env")
	if err := os.WriteFile(p, []byte("# CHIMERA_GATEWAY_TOKEN=\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	res, err := UpsertIfAbsent(p, "CHIMERA_GATEWAY_TOKEN", "tok")
	if err != nil {
		t.Fatal(err)
	}
	if !res.Written {
		t.Fatalf("res: %+v", res)
	}
}
