package tokens

import (
	"os"
	"path/filepath"
	"testing"
)

func TestIsBootstrapMode_missing(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "api-keys.yaml")
	if !IsBootstrapMode(p) {
		t.Fatal("missing file should bootstrap")
	}
}

func TestIsBootstrapMode_emptyDoc(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "api-keys.yaml")
	if err := os.WriteFile(p, []byte("api_keys: []\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if !IsBootstrapMode(p) {
		t.Fatal("empty tokens list should bootstrap")
	}
}

func TestIsBootstrapMode_invalidRows(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "api-keys.yaml")
	y := "api_keys:\n  - label: x\n    secret: \"\"\n    tenant_id: \"t1\"\n"
	if err := os.WriteFile(p, []byte(y), 0o600); err != nil {
		t.Fatal(err)
	}
	if !IsBootstrapMode(p) {
		t.Fatal("no valid rows should bootstrap")
	}
}

func TestIsBootstrapMode_valid(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "api-keys.yaml")
	y := "api_keys:\n  - label: admin\n    secret: \"secret-token\"\n    tenant_id: \"admin\"\n"
	if err := os.WriteFile(p, []byte(y), 0o600); err != nil {
		t.Fatal(err)
	}
	if IsBootstrapMode(p) {
		t.Fatal("valid token should not bootstrap")
	}
}

func TestIsBootstrapMode_badYAML(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "api-keys.yaml")
	if err := os.WriteFile(p, []byte("api_keys: [\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if !IsBootstrapMode(p) {
		t.Fatal("unparseable yaml should bootstrap")
	}
}
