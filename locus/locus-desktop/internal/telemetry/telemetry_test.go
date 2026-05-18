package telemetry

import (
	"os"
	"strings"
	"testing"

	"github.com/lynn/porcelain/internal/locus"
)

func TestRecordLifecycle_WritesJSONLWhenTraceEnabled(t *testing.T) {
	t.Setenv(locus.EnvTrace, "1")
	root := t.TempDir()
	RecordLifecycle(root, StateLaunchAttach, "test launch event", map[string]any{
		"base_url": "http://127.0.0.1:7710",
	})
	path := locus.LifecycleEventsPath(root)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read events file: %v", err)
	}
	s := string(raw)
	if !strings.Contains(s, StateLaunchAttach) {
		t.Fatalf("expected launch state in event log: %s", s)
	}
}

func TestRecordLifecycle_SkipsWithoutTrace(t *testing.T) {
	t.Setenv(locus.EnvTrace, "")
	root := t.TempDir()
	RecordLifecycle(root, StateLaunchAttach, "test", nil)
	path := locus.LifecycleEventsPath(root)
	if _, err := os.Stat(path); err == nil {
		t.Fatalf("expected no events file without %s", locus.EnvTrace)
	}
}
