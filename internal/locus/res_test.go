package locus

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/lynn/porcelain/internal/naming"
)

func TestBinariesMatchNamingContracts(t *testing.T) {
	if BinDesktop != naming.ProductDesktopName {
		t.Fatalf("desktop bin mismatch")
	}
	if BinSupervisor != naming.ProductSupervisorName {
		t.Fatalf("supervisor bin mismatch")
	}
	if BinBroker != naming.ProductBrokerName {
		t.Fatalf("broker bin mismatch")
	}
	if BinBrokerHTTP != naming.ProductBrokerHTTPBinName {
		t.Fatalf("broker http bin mismatch")
	}
	if BinQdrant != naming.ProductQdrantBinName {
		t.Fatalf("qdrant bin mismatch")
	}
	if EnvTrace != naming.EnvDesktopTrace {
		t.Fatalf("trace env mismatch")
	}
}

func TestSupervisorSearchNames_Windows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows-only")
	}
	got := SupervisorSearchNames()
	want0 := naming.ProductSupervisorName + ".exe"
	if len(got) != 2 || got[0] != want0 {
		t.Fatalf("unexpected names: %v", got)
	}
}

func TestRuntimePaths(t *testing.T) {
	root := filepath.FromSlash("/tmp/porcelain")
	wantDesktopDir := filepath.Join(root, DirData, DirDesktopState)
	if DesktopStateDirPath(root) != wantDesktopDir {
		t.Fatalf("desktop state dir: got %s want %s", DesktopStateDirPath(root), wantDesktopDir)
	}
	wantSupervisorDir := filepath.Join(root, DirData, DirSupervisorState)
	if SupervisorStateDirPath(root) != wantSupervisorDir {
		t.Fatalf("supervisor state dir: got %s want %s", SupervisorStateDirPath(root), wantSupervisorDir)
	}
	if filepath.Base(LaunchLockPath(root)) != FileLaunchLock {
		t.Fatalf("lock path: %s", LaunchLockPath(root))
	}
	if filepath.Base(LifecycleEventsPath(root)) != FileLifecycleLog {
		t.Fatalf("events path: %s", LifecycleEventsPath(root))
	}
}
