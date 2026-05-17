package main

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestDesktopSupervisorBaseURL_Default(t *testing.T) {
	got := desktopSupervisorBaseURL(nil, filepath.FromSlash("/tmp/none"))
	if got == "" {
		t.Fatalf("expected non-empty URL")
	}
}

func TestDesktopSupervisorBaseURL_ListenOverride(t *testing.T) {
	got := desktopSupervisorBaseURL([]string{"-listen", "0.0.0.0:4123"}, filepath.FromSlash("/tmp/none"))
	want := "http://127.0.0.1:4123"
	if got != want {
		t.Fatalf("want %s, got %s", want, got)
	}
}

func TestRedactLaunchArgs_SensitiveValues(t *testing.T) {
	in := []string{
		"--headless",
		"-gateway-token", "secret-token-value",
		"--api-key=abc123",
		"-listen", "127.0.0.1:7710",
	}
	got := redactLaunchArgs(in)
	want := []string{
		"--headless",
		"-gateway-token", "<redacted>",
		"--api-key=<redacted>",
		"-listen", "127.0.0.1:7710",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("want %v, got %v", want, got)
	}
}

func TestLaunchLockPaths(t *testing.T) {
	root := filepath.FromSlash("/tmp/porcelain")
	gotLock := desktopLaunchLockPath(root)
	gotMeta := desktopLaunchMetadataPath(root)
	if filepath.Base(gotLock) != "locus-desktop-launch.lock" {
		t.Fatalf("unexpected lock path: %s", gotLock)
	}
	if filepath.Base(gotMeta) != "locus-desktop-launch.json" {
		t.Fatalf("unexpected metadata path: %s", gotMeta)
	}
}

func TestResolveDesktopEntryURL_Default(t *testing.T) {
	got := resolveDesktopEntryURL("http://127.0.0.1:7710")
	if !strings.Contains(got, "/ui/login") {
		t.Fatalf("expected login route, got %s", got)
	}
	if !strings.Contains(got, "focus=admin") {
		t.Fatalf("expected admin logs focus, got %s", got)
	}
}

func TestBuildUnreachableURL(t *testing.T) {
	got := buildUnreachableURL("http://127.0.0.1:7710", "timeout", true)
	if !strings.HasPrefix(got, "data:text/html") {
		t.Fatalf("expected data URL, got %s", got)
	}
	if !strings.Contains(got, "Cannot%20connect%20to%20supervisor") {
		t.Fatalf("missing unreachable heading")
	}
}

func TestSupervisorLaunchArgs_NoImplicitHeadless(t *testing.T) {
	in := []string{"desktop", "-listen", "127.0.0.1:7710", "--log-dir", "custom/logs"}
	got := supervisorLaunchArgs(in, filepath.FromSlash("/tmp/none"))
	want := []string{"-listen", "127.0.0.1:7710"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("want %v, got %v", want, got)
	}
}

func TestResolveDesktopLogDir_Default(t *testing.T) {
	root := filepath.Clean(filepath.FromSlash("/tmp/porcelain"))
	got := resolveDesktopLogDir(nil, root)
	want := filepath.Clean(filepath.Join(root, "data"))
	if got != want {
		t.Fatalf("want %s, got %s", want, got)
	}
}

func TestResolveDesktopLogDir_FlagOverride(t *testing.T) {
	root := filepath.Clean(filepath.FromSlash("/tmp/porcelain"))
	got := resolveDesktopLogDir([]string{"--log-dir", "runtime-logs"}, root)
	want := filepath.Clean(filepath.Join(root, "runtime-logs"))
	if got != want {
		t.Fatalf("want %s, got %s", want, got)
	}
}
