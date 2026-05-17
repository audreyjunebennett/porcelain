package runtime

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lynn/porcelain/chimera/internal/logfmt"
	"github.com/lynn/porcelain/chimera/internal/wrapper/contract"
)

type ExitError struct {
	Code int
	Err  error
}

// Backward-compatible aliases for migration safety.
type RuntimeConfig = Config
type RuntimeBackoffConfig = BackoffConfig

func (e *ExitError) Error() string { return e.Err.Error() }

func WrapExitError(code int, err error) error {
	if err == nil {
		return nil
	}
	return &ExitError{Code: code, Err: err}
}

func ExitCodeForError(err error) int {
	var ee *ExitError
	if errors.As(err, &ee) {
		return ee.Code
	}
	return contract.ExitInternal
}

type Config struct {
	Component              string
	BackendMode            string
	Listen                 string
	StartupTimeout         time.Duration
	ShutdownTimeout        time.Duration
	TerminateWait          time.Duration
	BackoffInitial         time.Duration
	BackoffMultiplier      float64
	BackoffMax             time.Duration
	BackoffResetAfter      time.Duration
	DebugEnableUpstream    bool
	DebugAllowRemote       bool
	ForwardUpstreamInDebug bool
	UpstreamVersion        string
	WrapperVersion         string
	BuildCommit            string
	ReadyMessage           string
	UpstreamLineMessage    string
	HTTPServerErrorMessage string
	ComponentLabel         string
	BackendLabel           string
	ModeLabel              string
	UpstreamLineWrapper    func(string) string
}

type BackoffConfig struct {
	Initial    time.Duration
	Multiplier float64
	Max        time.Duration
}

type Adapter interface {
	Start(ctx context.Context, capture io.Writer, log *slog.Logger) (*exec.Cmd, error)
	ReadyURL() string
	MetricsURL() string
	BackendName() string
}

type Runtime struct {
	cfg     Config
	log     *slog.Logger
	adapter Adapter
	state   *runtimeState
	ring    *lineRing
	metrics *requestMetrics
	killed  atomic.Bool
}

func Run(rootCtx context.Context, cfg Config, adapter Adapter, log *slog.Logger) error {
	if strings.TrimSpace(cfg.ComponentLabel) == "" {
		cfg.ComponentLabel = cfg.Component
	}
	if strings.TrimSpace(cfg.BackendLabel) == "" {
		cfg.BackendLabel = adapter.BackendName()
	}
	if strings.TrimSpace(cfg.ModeLabel) == "" {
		cfg.ModeLabel = cfg.BackendMode
	}
	if strings.TrimSpace(cfg.BackendMode) == "" {
		cfg.BackendMode = "binary"
	}
	if log == nil {
		log = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	}
	if cfg.DebugEnableUpstream && !cfg.DebugAllowRemote && !IsLoopbackBind(cfg.Listen) {
		return WrapExitError(
			contract.ExitConfigError,
			fmt.Errorf(
				"refusing non-loopback debug bind for %q; use %s=true or %s",
				cfg.Listen,
				contract.DebugAllowRemoteEnv,
				contract.DebugAllowRemoteFlag,
			),
		)
	}
	rt := &Runtime{
		cfg:     cfg,
		log:     log,
		adapter: adapter,
		state:   &runtimeState{status: "degraded", message: "initializing"},
		ring: newLineRing(
			contract.DefaultDebugRingBufferMaxLines,
			contract.DefaultDebugRingBufferMaxBytes,
		),
		metrics: newRequestMetrics(),
	}
	rt.log = log.With(
		"component", cfg.ComponentLabel,
		"backend_name", cfg.BackendLabel,
		"backend_mode", cfg.ModeLabel,
	)

	backendCtx, backendCancel := context.WithCancel(rootCtx)
	defer backendCancel()
	backendErr := make(chan error, 1)
	go func() { backendErr <- rt.runBackendLoop(backendCtx) }()

	srv := &http.Server{Addr: cfg.Listen, Handler: rt.routes()}
	listener, err := net.Listen("tcp", cfg.Listen)
	if err != nil {
		return WrapExitError(contract.ExitDependency, fmt.Errorf("listen %s: %w", cfg.Listen, err))
	}
	go func() {
		<-rootCtx.Done()
		shCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer cancel()
		_ = srv.Shutdown(shCtx)
	}()
	go func() {
		if err := srv.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			rt.log.Error("wrapper http server exit", "msg", cfg.HTTPServerErrorMessage, "status", "error", "err", err)
		}
	}()

	err = <-backendErr
	if err != nil {
		return err
	}
	return nil
}

func (r *Runtime) runBackendLoop(ctx context.Context) error {
	attempt := 0
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}
		r.state.setStartup()
		r.log.Info("starting backend", "msg", "wrapper.backend.starting", "status", "degraded")
		start := time.Now()
		capture := &upstreamCaptureWriter{
			log:       r.log,
			ring:      r.ring,
			debugEmit: r.cfg.ForwardUpstreamInDebug || logfmt.SupervisedMode(),
			component: r.cfg.Component,
			lineMsg:   r.cfg.UpstreamLineMessage,
			lineWrap:  r.cfg.UpstreamLineWrapper,
			statusFn: func() string {
				_, st, _, _, _, _ := r.state.snapshot()
				return st
			},
		}
		cmd, err := r.adapter.Start(ctx, capture, r.log)
		if err != nil {
			return WrapExitError(contract.ExitBackendStartup, fmt.Errorf("start %s: %w", r.adapter.BackendName(), err))
		}
		waitCh := make(chan error, 1)
		go func() { waitCh <- cmd.Wait() }()
		pid := 0
		if cmd.Process != nil {
			pid = cmd.Process.Pid
		}

		readyCtx, cancelReady := context.WithTimeout(ctx, r.cfg.StartupTimeout)
		err = waitBackendReady(readyCtx, r.adapter.ReadyURL())
		cancelReady()
		if err != nil {
			if terr := TerminateThenKill(cmd, r.cfg.TerminateWait); errors.Is(terr, errBackendForcedKill) {
				r.killed.Store(true)
			}
			<-waitCh
			r.state.setDegraded("startup readiness failed", err.Error())
			r.log.Error("startup readiness failed", "msg", "wrapper.startup.readiness_failed", "status", "error", "err", err)
			return WrapExitError(contract.ExitBackendStartup, fmt.Errorf("startup readiness failed: %w", err))
		}

		r.state.setReady("backend healthy", pid)
		r.log.Info(
			contract.ReadyLogLine(
				r.cfg.Component,
				r.adapter.BackendName(),
				r.cfg.BackendMode,
				r.cfg.WrapperVersion,
				r.cfg.UpstreamVersion,
			),
			"msg",
			r.cfg.ReadyMessage,
			"status",
			"ok",
		)
		monitorCtx, monitorCancel := context.WithCancel(ctx)
		defer monitorCancel()
		go r.monitorReadiness(monitorCtx, r.adapter.ReadyURL())
		var werr error
		select {
		case werr = <-waitCh:
		case <-ctx.Done():
			if terr := TerminateThenKill(cmd, r.cfg.TerminateWait); errors.Is(terr, errBackendForcedKill) {
				r.killed.Store(true)
			}
			werr = <-waitCh
		}
		if ctx.Err() != nil {
			if r.killed.Load() {
				r.log.Error("forced backend shutdown", "msg", "wrapper.shutdown.forced_kill", "status", "error")
				return WrapExitError(contract.ExitBackendRuntime, errBackendForcedKill)
			}
			return nil
		}
		r.state.setDegraded("backend exited; restarting", errorText(werr))
		r.log.Warn("backend exited; restarting", "msg", "wrapper.backend.restarting", "status", "degraded", "err", werr)
		r.state.restartInc()
		uptime := time.Since(start)
		if uptime >= r.cfg.BackoffResetAfter {
			attempt = 0
		}
		delay := NextBackoff(
			BackoffConfig{
				Initial:    r.cfg.BackoffInitial,
				Multiplier: r.cfg.BackoffMultiplier,
				Max:        r.cfg.BackoffMax,
			},
			attempt,
		)
		attempt++
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil
		case <-timer.C:
		}
	}
}

func waitBackendReady(ctx context.Context, u string) error {
	if strings.TrimSpace(u) == "" {
		return nil
	}
	client := &http.Client{Timeout: 2 * time.Second}
	t := time.NewTicker(200 * time.Millisecond)
	defer t.Stop()
	for {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		resp, err := client.Do(req)
		if err == nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
		}
	}
}

func (r *Runtime) monitorReadiness(ctx context.Context, u string) {
	if strings.TrimSpace(u) == "" {
		return
	}
	client := &http.Client{Timeout: 2 * time.Second}
	t := time.NewTicker(2 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
			resp, err := client.Do(req)
			ok := err == nil && resp != nil && resp.StatusCode == http.StatusOK
			if resp != nil {
				_, _ = io.Copy(io.Discard, resp.Body)
				resp.Body.Close()
			}
			if ok {
				_, _, _, _, pid, _ := r.state.snapshot()
				r.state.setReady("backend healthy", pid)
			} else {
				r.state.setDegraded("backend readiness check failed", errorText(err))
				r.log.Warn("backend readiness check failed", "msg", "wrapper.backend.readiness_failed", "status", "degraded", "err", err)
			}
		}
	}
}

func NextBackoff(cfg BackoffConfig, attempt int) time.Duration {
	if attempt < 0 {
		attempt = 0
	}
	pow := math.Pow(cfg.Multiplier, float64(attempt))
	d := time.Duration(float64(cfg.Initial) * pow)
	if d > cfg.Max {
		return cfg.Max
	}
	return d
}

func TerminateThenKill(cmd *exec.Cmd, wait time.Duration) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	_ = sendGracefulTerminate(cmd.Process)
	killed := false
	select {
	case <-time.After(wait):
		if cmd.ProcessState == nil || !cmd.ProcessState.Exited() {
			killed = true
			if err := cmd.Process.Kill(); err != nil {
				return err
			}
		}
	}
	if killed {
		return errBackendForcedKill
	}
	return nil
}

var errBackendForcedKill = errors.New("backend required force kill during shutdown")

func IsLoopbackBind(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	h := strings.Trim(strings.TrimSpace(host), "[]")
	return h == "127.0.0.1" || h == "localhost" || h == "::1"
}

func EnvBool(key string) bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	return v == "1" || v == "true" || v == "yes" || v == "on"
}

func (r *Runtime) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc(contract.HealthPath, r.withMetrics("healthz", func(w http.ResponseWriter, req *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "component": r.cfg.Component})
	}))
	mux.HandleFunc(contract.ReadyPath, r.withMetrics("readyz", func(w http.ResponseWriter, req *http.Request) {
		ready, _, _, _, _, _ := r.state.snapshot()
		if !ready {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{"status": "degraded", "component": r.cfg.Component})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"status": "ok", "component": r.cfg.Component})
	}))
	mux.HandleFunc("/status", r.withMetrics("status", func(w http.ResponseWriter, req *http.Request) {
		ready, st, msg, lastErr, pid, restarts := r.state.snapshot()
		payload := contract.StatusPayload{
			Component:   r.cfg.Component,
			BackendName: r.adapter.BackendName(),
			BackendMode: r.cfg.BackendMode,
			Status:      st,
			Timestamp:   time.Now().UTC(),
			Version: contract.Version{
				Wrapper:  r.cfg.WrapperVersion,
				Upstream: r.cfg.UpstreamVersion,
				BuildSHA: r.cfg.BuildCommit,
			},
			Message:   msg,
			Restarts:  &restarts,
			LastError: lastErr,
		}
		if pid > 0 {
			payload.PID = &pid
		}
		if !ready && payload.Status == "ok" {
			payload.Status = "degraded"
		}
		code := http.StatusOK
		if payload.Status != "ok" {
			code = http.StatusServiceUnavailable
		}
		if err := payload.Validate(); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"status": "error", "detail": err.Error()})
			return
		}
		writeJSON(w, code, payload)
	}))
	mux.HandleFunc(contract.MetricsPath, r.withMetrics("metrics", r.handleMetrics))
	mux.HandleFunc(contract.DebugUpstreamLogsPath, r.withMetrics("debug_upstream_logs", r.handleDebugUpstreamLogs))
	return mux
}

func (r *Runtime) withMetrics(endpoint string, h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		start := time.Now()
		rw := &captureStatusWriter{ResponseWriter: w, code: http.StatusOK}
		h(rw, req)
		r.metrics.record(endpoint, rw.code, time.Since(start))
	}
}

type captureStatusWriter struct {
	http.ResponseWriter
	code int
}

func (w *captureStatusWriter) WriteHeader(statusCode int) {
	w.code = statusCode
	w.ResponseWriter.WriteHeader(statusCode)
}

func (r *Runtime) handleDebugUpstreamLogs(w http.ResponseWriter, _ *http.Request) {
	if !r.cfg.DebugEnableUpstream {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"component": r.cfg.Component,
		"lines":     r.ring.Snapshot(),
	})
}

func (r *Runtime) handleMetrics(w http.ResponseWriter, _ *http.Request) {
	ready, _, _, _, _, restarts := r.state.snapshot()
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	var b strings.Builder
	fmt.Fprintf(&b, "# HELP chimera_wrapper_up Wrapper process health.\n# TYPE chimera_wrapper_up gauge\nchimera_wrapper_up{component=%q} 1\n", r.cfg.Component)
	backendUp := 0
	if ready {
		backendUp = 1
	}
	fmt.Fprintf(&b, "# HELP chimera_backend_up Upstream backend readiness.\n# TYPE chimera_backend_up gauge\nchimera_backend_up{component=%q} %d\n", r.cfg.Component, backendUp)
	fmt.Fprintf(&b, "# HELP chimera_backend_restarts_total Backend restart count.\n# TYPE chimera_backend_restarts_total counter\nchimera_backend_restarts_total{component=%q} %d\n", r.cfg.Component, restarts)
	b.WriteString(r.metrics.render(r.cfg.Component))
	up := FetchUpstreamMetrics(r.adapter.MetricsURL())
	if up != "" {
		b.WriteString(PrefixUpstreamMetrics(up))
	}
	_, _ = io.WriteString(w, b.String())
}

func FetchUpstreamMetrics(u string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ""
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return ""
	}
	return string(raw)
}

func PrefixUpstreamMetrics(raw string) string {
	var out strings.Builder
	sc := bufio.NewScanner(strings.NewReader(raw))
	for sc.Scan() {
		line := sc.Text()
		trim := strings.TrimSpace(line)
		if trim == "" {
			continue
		}
		if strings.HasPrefix(trim, "# HELP ") {
			parts := strings.SplitN(trim, " ", 4)
			if len(parts) == 4 {
				out.WriteString("# HELP " + contract.UpstreamMetricsPrefix + parts[2] + " " + parts[3] + "\n")
			}
			continue
		}
		if strings.HasPrefix(trim, "# TYPE ") {
			parts := strings.SplitN(trim, " ", 4)
			if len(parts) == 4 {
				out.WriteString("# TYPE " + contract.UpstreamMetricsPrefix + parts[2] + " " + parts[3] + "\n")
			}
			continue
		}
		if strings.HasPrefix(trim, "#") {
			continue
		}
		nameRest := trim
		name := nameRest
		rest := ""
		if i := strings.IndexAny(nameRest, "{ "); i >= 0 {
			name = nameRest[:i]
			rest = nameRest[i:]
		}
		out.WriteString(contract.UpstreamMetricsPrefix + name + rest + "\n")
	}
	return out.String()
}

type runtimeState struct {
	mu        sync.RWMutex
	ready     bool
	status    string
	message   string
	lastError string
	pid       int
	restarts  int
}

func (s *runtimeState) setStartup() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ready = false
	s.status = "degraded"
	s.message = "starting upstream backend"
}

func (s *runtimeState) setReady(msg string, pid int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ready = true
	s.status = "ok"
	s.message = msg
	s.pid = pid
}

func (s *runtimeState) setDegraded(msg, errText string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ready = false
	s.status = "degraded"
	s.message = msg
	if strings.TrimSpace(errText) != "" {
		s.lastError = errText
	}
}

func (s *runtimeState) restartInc() {
	s.mu.Lock()
	s.restarts++
	s.mu.Unlock()
}

func (s *runtimeState) snapshot() (ready bool, status, message, lastError string, pid, restarts int) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.ready, s.status, s.message, s.lastError, s.pid, s.restarts
}

type lineRing struct {
	mu       sync.Mutex
	maxLines int
	maxBytes int
	lines    []string
	bytes    int
}

func newLineRing(maxLines, maxBytes int) *lineRing {
	return &lineRing{maxLines: maxLines, maxBytes: maxBytes}
}

func (r *lineRing) Add(line string) {
	line = strings.TrimSpace(line)
	if line == "" {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lines = append(r.lines, line)
	r.bytes += len(line)
	for len(r.lines) > r.maxLines || r.bytes > r.maxBytes {
		if len(r.lines) == 0 {
			break
		}
		r.bytes -= len(r.lines[0])
		r.lines = r.lines[1:]
	}
}

func (r *lineRing) Snapshot() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]string, len(r.lines))
	copy(out, r.lines)
	return out
}

type upstreamCaptureWriter struct {
	log       *slog.Logger
	ring      *lineRing
	debugEmit bool
	component string
	lineMsg   string
	lineWrap  func(string) string
	statusFn  func() string
	buf       strings.Builder
	bufMu     sync.Mutex
}

func (w *upstreamCaptureWriter) Write(p []byte) (int, error) {
	w.bufMu.Lock()
	defer w.bufMu.Unlock()
	w.buf.Write(p)
	sc := bufio.NewScanner(strings.NewReader(w.buf.String()))
	var consumed int
	for sc.Scan() {
		line := sc.Text()
		consumed += len(line) + 1
		raw := redactSecrets(line)
		wrapped := raw
		if w.lineWrap != nil {
			wrapped = strings.TrimSpace(w.lineWrap(raw))
		}
		if wrapped == "" {
			continue
		}
		w.ring.Add(wrapped)
		if w.debugEmit && w.log != nil {
			status := "degraded"
			if w.statusFn != nil {
				status = w.statusFn()
			}
			fields := []any{"msg", w.lineMsg, "component", w.component, "status", status, "upstream_raw", raw}
			if wrapped != raw {
				fields = append(fields, "upstream_wrapped", wrapped)
			}
			w.log.Info("upstream line", fields...)
		}
	}
	rest := w.buf.String()
	if consumed > len(rest) {
		consumed = len(rest)
	}
	w.buf.Reset()
	w.buf.WriteString(rest[consumed:])
	return len(p), nil
}

func redactSecrets(in string) string {
	out := in
	for _, token := range contract.RedactedSecretTokens {
		up := strings.ToUpper(token)
		if strings.Contains(strings.ToUpper(out), up) {
			out = "[redacted line: contains secret token]"
			return out
		}
	}
	return out
}

type requestMetrics struct {
	mu        sync.Mutex
	reqTotal  map[string]map[string]int64
	durations map[string][]float64
}

func newRequestMetrics() *requestMetrics {
	return &requestMetrics{
		reqTotal:  map[string]map[string]int64{},
		durations: map[string][]float64{},
	}
}

func (m *requestMetrics) record(endpointLabel string, code int, d time.Duration) {
	status := strconv.Itoa(code)
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.reqTotal[endpointLabel]; !ok {
		m.reqTotal[endpointLabel] = map[string]int64{}
	}
	m.reqTotal[endpointLabel][status]++
	m.durations[endpointLabel] = append(m.durations[endpointLabel], d.Seconds())
}

func (m *requestMetrics) render(component string) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	var b strings.Builder
	b.WriteString("# HELP chimera_requests_total Wrapper HTTP requests total.\n")
	b.WriteString("# TYPE chimera_requests_total counter\n")
	for endpoint, byStatus := range m.reqTotal {
		for status, cnt := range byStatus {
			fmt.Fprintf(&b, "chimera_requests_total{component=%q,endpoint=%q,status=%q} %d\n", component, endpoint, status, cnt)
		}
	}
	b.WriteString("# HELP chimera_request_duration_seconds Wrapper HTTP request duration in seconds.\n")
	b.WriteString("# TYPE chimera_request_duration_seconds histogram\n")
	buckets := []float64{0.01, 0.05, 0.1, 0.5, 1, 2, 5}
	for endpoint, vals := range m.durations {
		var sum float64
		var count int64
		for _, v := range vals {
			sum += v
			count++
		}
		var running int64
		for _, le := range buckets {
			for _, v := range vals {
				if v <= le {
					running++
				}
			}
			fmt.Fprintf(&b, "chimera_request_duration_seconds_bucket{component=%q,endpoint=%q,le=%q} %d\n", component, endpoint, fmt.Sprintf("%.2f", le), running)
			running = 0
		}
		fmt.Fprintf(&b, "chimera_request_duration_seconds_bucket{component=%q,endpoint=%q,le=\"+Inf\"} %d\n", component, endpoint, count)
		fmt.Fprintf(&b, "chimera_request_duration_seconds_sum{component=%q,endpoint=%q} %f\n", component, endpoint, sum)
		fmt.Fprintf(&b, "chimera_request_duration_seconds_count{component=%q,endpoint=%q} %d\n", component, endpoint, count)
	}
	return b.String()
}

func errorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}
