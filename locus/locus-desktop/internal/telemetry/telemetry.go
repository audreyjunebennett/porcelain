package telemetry

import (
	"encoding/json"
	"os"
	"strings"
	"time"

	"github.com/lynn/porcelain/internal/locus"
)

// Lifecycle states (consolidated taxonomy).
const (
	StateInit         = "desktop.init"
	StateLaunchAttach = "desktop.launch_attach"
	StateWaitReady    = "desktop.wait_ready"
	StateOpenUI       = "desktop.open_ui"
	StateFailed       = "desktop.failed"
	StateShutdown     = "desktop.shutdown"
	StateRuntimeLost  = "desktop.runtime_lost"
)

// Launch modes written to launch metadata.
type LaunchMode string

const (
	LaunchAttachExisting LaunchMode = "attach_existing"
	LaunchLaunchOwned    LaunchMode = "launch_owned"
	LaunchFailed         LaunchMode = "launch_failed"
)

// LaunchMetadata is persisted under run/locus-desktop-launch.json on failure or successful attach/launch.
type LaunchMetadata struct {
	TimestampUTC       string     `json:"timestamp_utc"`
	Mode               LaunchMode `json:"mode"`
	BaseURL            string     `json:"base_url"`
	SupervisorBin      string     `json:"supervisor_bin,omitempty"`
	SupervisorOwned    bool       `json:"supervisor_owned"`
	SupervisorPID      int        `json:"supervisor_pid,omitempty"`
	SupervisorWorkDir  string     `json:"supervisor_work_dir,omitempty"`
	SupervisorLogPath  string     `json:"supervisor_log_path,omitempty"`
	LaunchArgsRedacted []string   `json:"launch_args_redacted,omitempty"`
	Error              string     `json:"error,omitempty"`
}

// TraceEnabled reports whether lifecycle JSONL tracing is active.
func TraceEnabled() bool {
	v := strings.TrimSpace(os.Getenv(locus.EnvTrace))
	return v == "1" || strings.EqualFold(v, "true")
}

// RecordLifecycle appends one JSONL event when LOCUS_DESKTOP_TRACE is set.
func RecordLifecycle(runtimeRoot, state, detail string, fields map[string]any) {
	if !TraceEnabled() {
		return
	}
	entry := map[string]any{
		"timestamp_utc": time.Now().UTC().Format(time.RFC3339Nano),
		"state":         state,
		"detail":        detail,
	}
	for k, v := range fields {
		entry[k] = v
	}
	path := locus.LifecycleEventsPath(runtimeRoot)
	if err := os.MkdirAll(locus.RunDirPath(runtimeRoot), 0o755); err != nil {
		return
	}
	b, err := json.Marshal(entry)
	if err != nil {
		return
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.Write(append(b, '\n'))
}

// WriteLaunchMetadata writes run/locus-desktop-launch.json (typically once per run).
func WriteLaunchMetadata(runtimeRoot string, md LaunchMetadata) {
	if strings.TrimSpace(runtimeRoot) == "" {
		return
	}
	path := locus.LaunchMetadataPath(runtimeRoot)
	if err := os.MkdirAll(locus.RunDirPath(runtimeRoot), 0o755); err != nil {
		return
	}
	if md.TimestampUTC == "" {
		md.TimestampUTC = time.Now().UTC().Format(time.RFC3339Nano)
	}
	raw, err := json.MarshalIndent(md, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(path, raw, 0o644)
}

// LaunchLockPath returns the path used for single-instance launch coordination.
func LaunchLockPath(runtimeRoot string) string {
	return locus.LaunchLockPath(runtimeRoot)
}

// LaunchMetadataPath returns the path for launch diagnostics JSON.
func LaunchMetadataPath(runtimeRoot string) string {
	return locus.LaunchMetadataPath(runtimeRoot)
}
