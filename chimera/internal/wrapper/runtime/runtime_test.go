package runtime

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestTerminateThenKillForceKillError(t *testing.T) {
	if runtimeIsWindows() {
		t.Skip("windows uses kill-first terminate")
	}
	cmd := exec.Command("go", "test", "-run", "TestHelperProcessSleep", "./internal/wrapper/runtime", "--", "sleep")
	cmd.Env = append(os.Environ(), "WRAPPER_TEST_HELPER=1")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start helper: %v", err)
	}
	defer func() { _ = cmd.Process.Kill() }()
	err := TerminateThenKill(cmd, 20*time.Millisecond)
	if !errors.Is(err, errBackendForcedKill) {
		t.Fatalf("expected forced kill error, got %v", err)
	}
}

func TestRunLogsIncludeUniformFields(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a := &fakeAdapter{}
	go func() {
		time.Sleep(150 * time.Millisecond)
		cancel()
	}()
	_ = Run(ctx, Config{
		Component:              "chimera-broker",
		ComponentLabel:         "chimera-broker",
		BackendLabel:           "bifrost",
		ModeLabel:              "binary",
		BackendMode:            "binary",
		Listen:                 "127.0.0.1:0",
		StartupTimeout:         500 * time.Millisecond,
		ShutdownTimeout:        500 * time.Millisecond,
		TerminateWait:          10 * time.Millisecond,
		BackoffInitial:         10 * time.Millisecond,
		BackoffMultiplier:      2,
		BackoffMax:             20 * time.Millisecond,
		BackoffResetAfter:      30 * time.Millisecond,
		WrapperVersion:         "test",
		ReadyMessage:           "broker.ready",
		UpstreamLineMessage:    "broker.upstream.line",
		HTTPServerErrorMessage: "broker.http.server_error",
	}, a, logger)
	out := buf.String()
	for _, key := range []string{"component=chimera-broker", "backend_name=bifrost", "backend_mode=binary"} {
		if !strings.Contains(out, key) {
			t.Fatalf("expected log output to include %s: %s", key, out)
		}
	}
}

func TestUpstreamLineWrapperAppliesBeforeDebugEmitAndRing(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a := &fakeAdapterOutput{
		output: "line one\n",
	}
	go func() {
		time.Sleep(150 * time.Millisecond)
		cancel()
	}()
	_ = Run(ctx, Config{
		Component:              "chimera-vectorstore",
		ComponentLabel:         "chimera-vectorstore",
		BackendLabel:           "qdrant",
		ModeLabel:              "binary",
		BackendMode:            "binary",
		Listen:                 "127.0.0.1:0",
		StartupTimeout:         500 * time.Millisecond,
		ShutdownTimeout:        500 * time.Millisecond,
		TerminateWait:          10 * time.Millisecond,
		BackoffInitial:         10 * time.Millisecond,
		BackoffMultiplier:      2,
		BackoffMax:             20 * time.Millisecond,
		BackoffResetAfter:      30 * time.Millisecond,
		DebugEnableUpstream:    true,
		ForwardUpstreamInDebug: true,
		WrapperVersion:         "test",
		ReadyMessage:           "vectorstore.ready",
		UpstreamLineMessage:    "vectorstore.upstream.line",
		HTTPServerErrorMessage: "vectorstore.http.server_error",
		UpstreamLineWrapper: func(in string) string {
			return "wrapped:" + in
		},
	}, a, logger)
	out := buf.String()
	if !strings.Contains(out, `upstream_wrapped="wrapped:line one"`) {
		t.Fatalf("expected wrapped debug field, got: %s", out)
	}
}

func TestRunSupportsWorkersWithoutReadyURL(t *testing.T) {
	buf := &bytes.Buffer{}
	logger := slog.New(slog.NewTextHandler(buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a := &fakeAdapterNoReadyURL{}
	go func() {
		time.Sleep(120 * time.Millisecond)
		cancel()
	}()
	if err := Run(ctx, Config{
		Component:              "chimera-indexer",
		ComponentLabel:         "chimera-indexer",
		BackendLabel:           "custom",
		ModeLabel:              "binary",
		BackendMode:            "binary",
		Listen:                 "127.0.0.1:0",
		StartupTimeout:         500 * time.Millisecond,
		ShutdownTimeout:        500 * time.Millisecond,
		TerminateWait:          10 * time.Millisecond,
		BackoffInitial:         10 * time.Millisecond,
		BackoffMultiplier:      2,
		BackoffMax:             20 * time.Millisecond,
		BackoffResetAfter:      30 * time.Millisecond,
		WrapperVersion:         "test",
		ReadyMessage:           "indexer.ready",
		UpstreamLineMessage:    "indexer.upstream.line",
		HTTPServerErrorMessage: "indexer.http.server_error",
	}, a, logger); err != nil {
		t.Fatalf("run should succeed without ready URL: %v", err)
	}
	if out := buf.String(); !strings.Contains(out, "indexer.ready") {
		t.Fatalf("expected indexer ready log, got: %s", out)
	}
}

func TestHelperProcessSleep(t *testing.T) {
	if os.Getenv("WRAPPER_TEST_HELPER") != "1" {
		return
	}
	if len(os.Args) >= 2 && os.Args[len(os.Args)-1] == "sleep" {
		time.Sleep(5 * time.Second)
		os.Exit(0)
	}
}

type fakeAdapter struct{}

func (f *fakeAdapter) Start(ctx context.Context, capture io.Writer, log *slog.Logger) (*exec.Cmd, error) {
	cmd := exec.CommandContext(ctx, "go", "test", "-run", "TestHelperProcessSleep", "./internal/wrapper/runtime", "--", "sleep")
	cmd.Env = append(os.Environ(), "WRAPPER_TEST_HELPER=1")
	cmd.Stdout = capture
	cmd.Stderr = capture
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return cmd, nil
}

func (f *fakeAdapter) ReadyURL() string   { return "http://127.0.0.1:1/ready" }
func (f *fakeAdapter) MetricsURL() string { return "http://127.0.0.1:1/metrics" }
func (f *fakeAdapter) BackendName() string {
	return "bifrost"
}

type fakeAdapterOutput struct {
	output string
}

func (f *fakeAdapterOutput) Start(ctx context.Context, capture io.Writer, _ *slog.Logger) (*exec.Cmd, error) {
	cmd := exec.CommandContext(ctx, "go", "test", "-run", "TestHelperProcessSleep", "./internal/wrapper/runtime", "--", "sleep")
	cmd.Env = append(os.Environ(), "WRAPPER_TEST_HELPER=1")
	cmd.Stdout = capture
	cmd.Stderr = capture
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(f.output) != "" {
		_, _ = io.WriteString(capture, f.output)
	}
	return cmd, nil
}

func (f *fakeAdapterOutput) ReadyURL() string   { return "http://127.0.0.1:1/ready" }
func (f *fakeAdapterOutput) MetricsURL() string { return "http://127.0.0.1:1/metrics" }
func (f *fakeAdapterOutput) BackendName() string {
	return "qdrant"
}

type fakeAdapterNoReadyURL struct{}

func (f *fakeAdapterNoReadyURL) Start(ctx context.Context, capture io.Writer, _ *slog.Logger) (*exec.Cmd, error) {
	cmd := exec.CommandContext(ctx, "go", "test", "-run", "TestHelperProcessSleep", "./internal/wrapper/runtime", "--", "sleep")
	cmd.Env = append(os.Environ(), "WRAPPER_TEST_HELPER=1")
	cmd.Stdout = capture
	cmd.Stderr = capture
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return cmd, nil
}

func (f *fakeAdapterNoReadyURL) ReadyURL() string   { return "" }
func (f *fakeAdapterNoReadyURL) MetricsURL() string { return "" }
func (f *fakeAdapterNoReadyURL) BackendName() string {
	return "custom"
}

func runtimeIsWindows() bool {
	return strings.EqualFold(os.Getenv("OS"), "Windows_NT")
}
