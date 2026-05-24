package launcher

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestRedactArgs_SensitiveValues(t *testing.T) {
	in := []string{
		"--headless",
		"-gateway-token", "secret-token-value",
		"--api-key=abc123",
		"-listen", "127.0.0.1:7710",
	}
	got := RedactArgs(in)
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

func TestFilterSupervisorArgs(t *testing.T) {
	in := []string{"desktop", "-listen", "127.0.0.1:7710", "--log-dir", "custom/logs"}
	got := FilterSupervisorArgs(in)
	want := []string{"-listen", "127.0.0.1:7710"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("want %v, got %v", want, got)
	}
}

func TestLogDir_Default(t *testing.T) {
	root := filepath.Clean(filepath.FromSlash("/tmp/porcelain"))
	got := LogDir(nil, root)
	want := filepath.Clean(filepath.Join(root, "data", "chimera-supervisor"))
	if got != want {
		t.Fatalf("want %s, got %s", want, got)
	}
}

func TestLogDir_FlagOverride(t *testing.T) {
	root := filepath.Clean(filepath.FromSlash("/tmp/porcelain"))
	got := LogDir([]string{"--log-dir", "runtime-logs"}, root)
	want := filepath.Clean(filepath.Join(root, "runtime-logs"))
	if got != want {
		t.Fatalf("want %s, got %s", want, got)
	}
}
