package brokerclient

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestGatewayHealthUpstreamState_firstAndThrottle(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	var st gatewayHealthUpstreamState
	min := 50 * time.Millisecond

	st.observe(log, "http://127.0.0.1:9/health", true, 200, "", min)
	if !strings.Contains(buf.String(), "gateway.health.upstream") || !strings.Contains(buf.String(), "ok=true") {
		t.Fatalf("first observation: %q", buf.String())
	}

	buf.Reset()
	st.observe(log, "http://127.0.0.1:9/health", true, 200, "", min)
	if buf.Len() != 0 {
		t.Fatalf("same state should not log: %q", buf.String())
	}

	buf.Reset()
	st.observe(log, "http://127.0.0.1:9/health", false, 503, "down", min)
	if buf.Len() != 0 {
		t.Fatalf("flip too soon should be throttled: %q", buf.String())
	}

	time.Sleep(60 * time.Millisecond)
	st.observe(log, "http://127.0.0.1:9/health", false, 503, "down", min)
	if !strings.Contains(buf.String(), "gateway.health.upstream") || !strings.Contains(buf.String(), "ok=false") {
		t.Fatalf("after throttle window: %q", buf.String())
	}
}

func TestSupervisedChildHealthState_seedHealthySuppressesFirstOK(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	var st supervisedChildHealthState
	st.seedHealthy = true
	min := time.Millisecond

	st.observe(log, "gateway.health.chimera-broker", "http://h/health", true, 200, "", min)
	if buf.Len() != 0 {
		t.Fatalf("expected suppress first OK when seed healthy: %q", buf.String())
	}

	buf.Reset()
	st.observe(log, "gateway.health.chimera-broker", "http://h/health", false, 503, "no", min)
	if !strings.Contains(buf.String(), "gateway.health.chimera-broker") || !strings.Contains(buf.String(), "ok=false") {
		t.Fatalf("unhealthy after seed: %q", buf.String())
	}
}

func TestProbeHealth_transportErrorStillWarns(t *testing.T) {
	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelWarn}))
	ctx := context.Background()
	_, _, _ = ProbeHealth(ctx, "http://127.0.0.1:1/health", "", 200*time.Millisecond, log)
	if !strings.Contains(buf.String(), "upstream.health.probe_failed") {
		t.Fatalf("expected warn log, got %q", buf.String())
	}
}
