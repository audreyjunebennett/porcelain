package launcher

import (
	"os"
	"testing"
	"time"

	"github.com/lynn/porcelain/internal/locus"
)

func TestAcquireLaunchLock_ContentionAndRecovery(t *testing.T) {
	root := t.TempDir()
	unlock, err := AcquireLaunchLock(root, 500*time.Millisecond)
	if err != nil {
		t.Fatalf("first lock failed: %v", err)
	}
	defer unlock()
	if _, err := AcquireLaunchLock(root, 100*time.Millisecond); err == nil {
		t.Fatal("expected contention error")
	}
	unlock()
	if unlock2, err := AcquireLaunchLock(root, 200*time.Millisecond); err != nil {
		t.Fatalf("expected lock to recover after unlock: %v", err)
	} else {
		unlock2()
	}
}

func TestAcquireLaunchLock_ReapsStaleLock(t *testing.T) {
	root := t.TempDir()
	lockPath := locus.LaunchLockPath(root)
	if err := os.MkdirAll(locus.RunDirPath(root), 0o755); err != nil {
		t.Fatalf("mkdir run dir: %v", err)
	}
	if err := os.WriteFile(lockPath, []byte("stale"), 0o644); err != nil {
		t.Fatalf("write stale lock: %v", err)
	}
	old := time.Now().Add(-3 * time.Minute)
	if err := os.Chtimes(lockPath, old, old); err != nil {
		t.Fatalf("set stale mtime: %v", err)
	}
	unlock, err := AcquireLaunchLock(root, 500*time.Millisecond)
	if err != nil {
		t.Fatalf("expected stale lock reap, got err: %v", err)
	}
	unlock()
}

func TestLaunchLockPaths(t *testing.T) {
	root := t.TempDir()
	if filepathBase(locus.LaunchLockPath(root)) != locus.FileLaunchLock {
		t.Fatalf("unexpected lock file name")
	}
	if filepathBase(locus.LaunchMetadataPath(root)) != locus.FileLaunchMetadata {
		t.Fatalf("unexpected metadata file name")
	}
}

func filepathBase(p string) string {
	for i := len(p) - 1; i >= 0; i-- {
		if p[i] == '/' || p[i] == '\\' {
			return p[i+1:]
		}
	}
	return p
}
