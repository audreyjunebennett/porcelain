package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSupervisorReachable_HealthzOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	if !supervisorReachable(srv.URL) {
		t.Fatalf("expected supervisor to be reachable")
	}
}

func TestSupervisorReachable_Unreachable(t *testing.T) {
	if supervisorReachable("http://127.0.0.1:1") {
		t.Fatalf("expected supervisor to be unreachable")
	}
}

func TestWaitForSupervisorReadiness_ReadyzOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/readyz":
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	ok, detail := waitForSupervisorReadiness(srv.URL, 400*time.Millisecond)
	if !ok {
		t.Fatalf("expected readiness true, got detail=%q", detail)
	}
}

func TestWaitForSupervisorReadiness_BootstrapStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/readyz":
			w.WriteHeader(http.StatusServiceUnavailable)
		case "/status":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"bootstrap":true}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	ok, detail := waitForSupervisorReadiness(srv.URL, 400*time.Millisecond)
	if !ok {
		t.Fatalf("expected bootstrap readiness true, got detail=%q", detail)
	}
}

func TestWaitForSupervisorReadiness_Timeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()
	ok, detail := waitForSupervisorReadiness(srv.URL, 450*time.Millisecond)
	if ok {
		t.Fatalf("expected readiness timeout")
	}
	if strings.TrimSpace(detail) == "" {
		t.Fatalf("expected non-empty timeout detail")
	}
}

func TestResolveDesktopEntryURL_Bootstrap(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/status" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"details":{"operator_ui":{"base_url":"http://127.0.0.1:3000","bootstrap":true}}}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	got := resolveDesktopEntryURL(srv.URL)
	if got != "http://127.0.0.1:3000/ui/setup" {
		t.Fatalf("want setup URL, got %s", got)
	}
}

func TestAcquireDesktopLaunchLock_ContentionAndRecovery(t *testing.T) {
	root := t.TempDir()
	unlock, err := acquireDesktopLaunchLock(root, 500*time.Millisecond)
	if err != nil {
		t.Fatalf("first lock failed: %v", err)
	}
	defer unlock()
	if _, err := acquireDesktopLaunchLock(root, 100*time.Millisecond); err == nil {
		t.Fatalf("expected contention error")
	}
	unlock()
	if unlock2, err := acquireDesktopLaunchLock(root, 200*time.Millisecond); err != nil {
		t.Fatalf("expected lock to recover after unlock: %v", err)
	} else {
		unlock2()
	}
}

func TestAcquireDesktopLaunchLock_ReapsStaleLock(t *testing.T) {
	root := t.TempDir()
	lockPath := desktopLaunchLockPath(root)
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		t.Fatalf("mkdir run dir: %v", err)
	}
	if err := os.WriteFile(lockPath, []byte("stale"), 0o644); err != nil {
		t.Fatalf("write stale lock: %v", err)
	}
	old := time.Now().Add(-3 * time.Minute)
	if err := os.Chtimes(lockPath, old, old); err != nil {
		t.Fatalf("set stale mtime: %v", err)
	}
	unlock, err := acquireDesktopLaunchLock(root, 500*time.Millisecond)
	if err != nil {
		t.Fatalf("expected stale lock reap, got err: %v", err)
	}
	unlock()
}

func TestRecordLifecycleEvent_WritesJSONL(t *testing.T) {
	root := t.TempDir()
	recordLifecycleEvent(root, desktopStateLaunchAttempt, "test launch event", map[string]any{
		"base_url": "http://127.0.0.1:7710",
	})
	path := filepath.Join(root, "run", "locus-desktop-events.jsonl")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read events file: %v", err)
	}
	s := string(raw)
	if !strings.Contains(s, "desktop.launch.attempt") {
		t.Fatalf("expected launch state in event log: %s", s)
	}
}

