package tokens

import (
	"path/filepath"
	"testing"
)

func TestAppendToken_createsFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "api-keys.yaml")
	tok, tenant, err := AppendToken(p, "My Admin")
	if err != nil {
		t.Fatal(err)
	}
	if len(tok) < 16 {
		t.Fatalf("token too short: %q", tok)
	}
	if tenant != "my-admin" {
		t.Fatalf("tenant: got %q", tenant)
	}
	if IsBootstrapMode(p) {
		t.Fatal("file should have valid token after append")
	}
	meta, err := ListTokenMeta(p)
	if err != nil || len(meta) != 1 {
		t.Fatalf("meta: %v err %v", meta, err)
	}
	if meta[0].Label != "My Admin" || meta[0].TenantID != "my-admin" || meta[0].Index != 0 {
		t.Fatalf("meta row: %+v", meta[0])
	}
}

func TestAppendToken_appends(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "api-keys.yaml")
	_, _, err := AppendToken(p, "a")
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = AppendToken(p, "b")
	if err != nil {
		t.Fatal(err)
	}
	meta, err := ListTokenMeta(p)
	if err != nil || len(meta) != 2 {
		t.Fatalf("meta: %v err %v", meta, err)
	}
}

func TestRemoveTokenAt(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "api-keys.yaml")
	_, _, err := AppendToken(p, "one")
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = AppendToken(p, "two")
	if err != nil {
		t.Fatal(err)
	}
	if err := RemoveTokenAt(p, 0); err != nil {
		t.Fatal(err)
	}
	meta, err := ListTokenMeta(p)
	if err != nil || len(meta) != 1 {
		t.Fatalf("after remove: %v err %v", meta, err)
	}
	if meta[0].Label != "two" {
		t.Fatalf("expected second row to remain, got %+v", meta[0])
	}
}

func TestTenantIDFromLabel(t *testing.T) {
	if got := TenantIDFromLabel(""); got != "default" {
		t.Fatalf("empty: %q", got)
	}
	if got := TenantIDFromLabel("  Foo Bar!  "); got != "foo-bar" {
		t.Fatalf("slug: %q", got)
	}
}
