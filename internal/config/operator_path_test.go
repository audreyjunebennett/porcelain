package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOperatorSQLitePaths_default(t *testing.T) {
	dir := t.TempDir()
	gw := filepath.Join(dir, "config", "gateway.yaml")
	if err := os.MkdirAll(filepath.Dir(gw), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(gw, []byte("gateway:\n  semver: 0.1.0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := LoadGatewayYAML(gw, nil)
	if err != nil {
		t.Fatal(err)
	}
	wantDB := filepath.Join(dir, "data", "gateway", "operator.sqlite")
	if res.OperatorSQLitePath != wantDB {
		t.Fatalf("OperatorSQLitePath=%q want %q", res.OperatorSQLitePath, wantDB)
	}
	wantMig := filepath.Join(dir, "migrations", "operator")
	if res.OperatorMigrationsDir != wantMig {
		t.Fatalf("OperatorMigrationsDir=%q want %q", res.OperatorMigrationsDir, wantMig)
	}
}
