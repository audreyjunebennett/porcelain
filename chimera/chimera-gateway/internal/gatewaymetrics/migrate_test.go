package gatewaymetrics

import (
	"database/sql"
	"path/filepath"
	"runtime"
	"testing"

	_ "modernc.org/sqlite"
)

func testMigrationsDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "migrations", "chimera-gateway", "metrics"))
}

func TestApplyMigrations_idempotent(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "m.db")

	dsn := sqliteDSN(dbPath)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	migDir := testMigrationsDir(t)
	if err := ApplyMigrations(db, migDir, nil); err != nil {
		t.Fatal(err)
	}
	if err := ApplyMigrations(db, migDir, nil); err != nil {
		t.Fatal(err)
	}

	var v int
	if err := db.QueryRow(`SELECT COUNT(*) FROM gateway_migrations`).Scan(&v); err != nil {
		t.Fatal(err)
	}
	if v != 1 {
		t.Fatalf("gateway_migrations rows = %d, want 1", v)
	}
}
