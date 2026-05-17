package contract

import (
	"strings"
	"testing"
	"time"
)

func TestStatusPayloadValidate_valid(t *testing.T) {
	t.Parallel()
	s := StatusPayload{
		Component:   ComponentVectorstore,
		BackendName: "qdrant",
		BackendMode: "binary",
		Status:      "ok",
		Timestamp:   time.Now().UTC(),
		Version: Version{
			Wrapper:  "0.3.1",
			Upstream: "",
			BuildSHA: "abc123",
		},
	}
	if err := s.Validate(); err != nil {
		t.Fatalf("unexpected validate error: %v", err)
	}
}

func TestStatusPayloadValidate_rejectsMissingRequireds(t *testing.T) {
	t.Parallel()
	s := StatusPayload{}
	err := s.Validate()
	if err == nil {
		t.Fatal("expected validation error")
	}
	msg := err.Error()
	for _, want := range []string{
		"invalid component",
		"invalid backend_name",
		"invalid backend_mode",
		"invalid status",
		"missing timestamp",
		"missing version.wrapper",
	} {
		if !strings.Contains(msg, want) {
			t.Fatalf("expected %q in error %q", want, msg)
		}
	}
}

func TestStatusPayloadValidate_restartsNonNegative(t *testing.T) {
	t.Parallel()
	n := -1
	s := StatusPayload{
		Component:   ComponentBroker,
		BackendName: "chimera-broker",
		BackendMode: "binary",
		Status:      "degraded",
		Timestamp:   time.Now().UTC(),
		Version:     Version{Wrapper: "0.3.1"},
		Restarts:    &n,
	}
	if err := s.Validate(); err == nil || !strings.Contains(err.Error(), "restarts must be >= 0") {
		t.Fatalf("expected restarts validation error, got: %v", err)
	}
}

func TestReadyLogLineFormat(t *testing.T) {
	t.Parallel()
	got := ReadyLogLine("chimera-vectorstore", "qdrant", "binary", "0.3.1", "1.9.0")
	want := "READY: component=<chimera-vectorstore> backend=<qdrant> mode=<binary> version=<0.3.1> upstream=<1.9.0>"
	if got != want {
		t.Fatalf("ready line mismatch\nwant: %s\ngot:  %s", want, got)
	}
}

func TestDebugBindPolicy(t *testing.T) {
	t.Parallel()
	if !DebugMustBindLoopback(false) {
		t.Fatal("expected loopback-only when allow remote is false")
	}
	if DebugMustBindLoopback(true) {
		t.Fatal("expected remote bind allowed when allow remote is true")
	}
}

func TestEndpointMetricLabelBounded(t *testing.T) {
	t.Parallel()
	label, ok := EndpointMetricLabel("/healthz")
	if !ok || label != "healthz" {
		t.Fatalf("healthz label unexpected: %q ok=%v", label, ok)
	}
	if _, ok := EndpointMetricLabel("/dynamic/123"); ok {
		t.Fatal("expected dynamic path to be rejected for bounded endpoint labels")
	}
}

func TestContractConstants(t *testing.T) {
	t.Parallel()
	if LegacyCompatibilitySupported {
		t.Fatal("legacy compatibility must be hard-break false")
	}
	if DebugAllowRemoteEnv != "DEBUG__ALLOW_REMOTE" {
		t.Fatalf("unexpected debug override env: %s", DebugAllowRemoteEnv)
	}
	if DebugAllowRemoteFlag != "--debug-allow-remote" {
		t.Fatalf("unexpected debug override flag: %s", DebugAllowRemoteFlag)
	}
}
