package conversationmerge

import (
	"context"
	"database/sql"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/lynn/porcelain/chimera/internal/operatorstore"

	_ "modernc.org/sqlite"
)

func testOperatorMigrationsDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "migrations", "chimera-gateway", "operator"))
}

func TestStore_roundTrip(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "m.db")
	abs, err := filepath.Abs(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	dsn := "file:" + filepath.ToSlash(abs) + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := operatorstore.ApplyMigrations(db, testOperatorMigrationsDir(t), nil); err != nil {
		t.Fatal(err)
	}
	st := NewStore(db)
	ctx := context.Background()
	v := []float32{1, 0, 0}
	if err := st.UpsertConversation(ctx, "t1", "p", "f", "conv-a", v, "hi", "there", "fp1", time.Unix(1000, 0)); err != nil {
		t.Fatal(err)
	}
	if got := st.GetRollingFingerprint(ctx, "conv-a"); got != "fp1" {
		t.Fatalf("fp %q", got)
	}
	rows, err := st.ListCandidates(ctx, "t1", "p", "f", time.Time{}, 10)
	if err != nil || len(rows) != 1 {
		t.Fatalf("rows=%v err=%v", rows, err)
	}

	v2 := []float32{0, 1, 0}
	if err := st.UpsertUserSnapshotAtResolve(ctx, "t1", "p", "f", "conv-a", v2, "hello", time.Unix(2000, 0)); err != nil {
		t.Fatal(err)
	}
	rows2, err := st.ListCandidates(ctx, "t1", "p", "f", time.Time{}, 10)
	if err != nil || len(rows2) != 1 {
		t.Fatalf("rows2=%v err=%v", rows2, err)
	}
	if rows2[0].LastModelTextNormalized != "there" {
		t.Fatalf("model text was cleared: %q", rows2[0].LastModelTextNormalized)
	}
	if rows2[0].RollingFingerprint != "fp1" {
		t.Fatalf("fingerprint was cleared: %q", rows2[0].RollingFingerprint)
	}
}
