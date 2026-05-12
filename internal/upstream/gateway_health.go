package upstream

import (
	"context"
	"log/slog"
	"strings"
	"time"
)

const (
	defaultGatewayHealthPollInterval = 15 * time.Second
	defaultGatewayHealthMinEmitGap   = 30 * time.Second
)

// gatewayHealthUpstreamState tracks emitted gateway.health.upstream lines (P4).
type gatewayHealthUpstreamState struct {
	initialized   bool
	lastEmittedOK bool
	lastEmit      time.Time
}

func (s *gatewayHealthUpstreamState) observe(log *slog.Logger, target string, ok bool, status int, detail string, minGap time.Duration) {
	if log == nil || strings.TrimSpace(target) == "" {
		return
	}
	if minGap <= 0 {
		minGap = defaultGatewayHealthMinEmitGap
	}
	now := time.Now()
	args := []any{
		"msg", "gateway.health.upstream",
		"target", target,
		"ok", ok,
		"status", status,
		"service", "gateway",
	}
	if detail != "" {
		args = append(args, "detail", detail)
	}
	if !s.initialized {
		s.initialized = true
		s.lastEmittedOK = ok
		s.lastEmit = now
		log.Info("gateway upstream health", args...)
		return
	}
	if ok == s.lastEmittedOK {
		return
	}
	if now.Sub(s.lastEmit) < minGap {
		return
	}
	s.lastEmittedOK = ok
	s.lastEmit = now
	log.Info("gateway upstream health", args...)
}

// RunGatewayUpstreamHealthMonitor polls upstream.base_url health until ctx is cancelled.
// It emits gateway.health.upstream on first observation and whenever ok flips, throttled
// by minEmitGap (defaults to 30s when <=0). resolver returns health URL, API key, per-probe
// timeout, and false when monitoring should skip this tick (e.g. unresolved config).
func RunGatewayUpstreamHealthMonitor(ctx context.Context, log *slog.Logger, tick, minEmitGap time.Duration, resolver func(context.Context) (healthURL, apiKey string, timeout time.Duration, ok bool)) {
	if log == nil {
		return
	}
	if tick <= 0 {
		tick = defaultGatewayHealthPollInterval
	}
	go func() {
		var st gatewayHealthUpstreamState
		run := func() {
			pctx, cancel := context.WithTimeout(ctx, 25*time.Second)
			defer cancel()
			url, key, to, ok := resolver(pctx)
			if !ok || strings.TrimSpace(url) == "" {
				return
			}
			if to <= 0 {
				to = 5 * time.Second
			}
			probeCtx, pCancel := context.WithTimeout(pctx, to)
			hok, stCode, det, _ := probeHealthHTTP(probeCtx, url, key, to)
			pCancel()
			st.observe(log, url, hok, stCode, det, minEmitGap)
		}
		run()
		t := time.NewTicker(tick)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				run()
			}
		}
	}()
}

// supervisedChildHealthState tracks gateway.health.bifrost / gateway.health.qdrant (P4).
type supervisedChildHealthState struct {
	seedHealthy bool
	outOnce     bool
	lastOutOK   bool
	lastEmit    time.Time
}

func (s *supervisedChildHealthState) observe(log *slog.Logger, msg, url string, ok bool, status int, detail string, minGap time.Duration) {
	if log == nil || strings.TrimSpace(url) == "" {
		return
	}
	if minGap <= 0 {
		minGap = defaultGatewayHealthMinEmitGap
	}
	now := time.Now()
	args := []any{
		"msg", msg,
		"url", url,
		"ok", ok,
		"status", status,
		"service", "gateway",
	}
	if detail != "" {
		args = append(args, "detail", detail)
	}
	if !s.outOnce {
		if s.seedHealthy && ok {
			s.lastOutOK = true
			s.outOnce = true
			return
		}
		log.Info("supervised child health", args...)
		s.lastOutOK = ok
		s.outOnce = true
		s.lastEmit = now
		return
	}
	if ok == s.lastOutOK {
		return
	}
	if now.Sub(s.lastEmit) < minGap {
		return
	}
	log.Info("supervised child health", args...)
	s.lastOutOK = ok
	s.lastEmit = now
}

// RunSupervisedChildHealthMonitor polls healthURL until ctx is cancelled and emits
// gateway.health.{child} when HTTP health flips (child must be "bifrost" or "qdrant").
// When waitVerifiedOK is true, the first successful probe is suppressed (WaitHealthy already ran).
func RunSupervisedChildHealthMonitor(ctx context.Context, log *slog.Logger, child, healthURL string, tick, minEmitGap time.Duration, waitVerifiedOK bool) {
	if log == nil || strings.TrimSpace(healthURL) == "" {
		return
	}
	if tick <= 0 {
		tick = defaultGatewayHealthPollInterval
	}
	msg := "gateway.health." + child
	go func() {
		var st supervisedChildHealthState
		st.seedHealthy = waitVerifiedOK
		run := func() {
			pctx, cancel := context.WithTimeout(ctx, 8*time.Second)
			defer cancel()
			hok, stCode, det, _ := probeHealthHTTP(pctx, healthURL, "", 6*time.Second)
			st.observe(log, msg, healthURL, hok, stCode, det, minEmitGap)
		}
		run()
		t := time.NewTicker(tick)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				run()
			}
		}
	}()
}
