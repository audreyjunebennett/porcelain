package control

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/lynn/porcelain/chimera/internal/servicelogs"
	"github.com/lynn/porcelain/chimera/internal/wrapper/contract"
)

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
		for _, le := range buckets {
			var bucketCount int64
			for _, v := range vals {
				if v <= le {
					bucketCount++
				}
			}
			fmt.Fprintf(&b, "chimera_request_duration_seconds_bucket{component=%q,endpoint=%q,le=%q} %d\n", component, endpoint, fmt.Sprintf("%.2f", le), bucketCount)
		}
		fmt.Fprintf(&b, "chimera_request_duration_seconds_bucket{component=%q,endpoint=%q,le=\"+Inf\"} %d\n", component, endpoint, count)
		fmt.Fprintf(&b, "chimera_request_duration_seconds_sum{component=%q,endpoint=%q} %f\n", component, endpoint, sum)
		fmt.Fprintf(&b, "chimera_request_duration_seconds_count{component=%q,endpoint=%q} %d\n", component, endpoint, count)
	}
	return b.String()
}

type captureStatusWriter struct {
	http.ResponseWriter
	code int
}

func (w *captureStatusWriter) WriteHeader(statusCode int) {
	w.code = statusCode
	w.ResponseWriter.WriteHeader(statusCode)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

// Handler returns the supervisor control-plane HTTP handler.
// onShutdown, when non-nil, is invoked by POST /shutdown to begin graceful teardown.
func Handler(state *State, logStore *servicelogs.Store, onShutdown func()) http.Handler {
	metrics := newRequestMetrics()
	withMetrics := func(endpoint string, h http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, req *http.Request) {
			start := time.Now()
			rw := &captureStatusWriter{ResponseWriter: w, code: http.StatusOK}
			h(rw, req)
			metrics.record(endpoint, rw.code, time.Since(start))
		}
	}

	mux := http.NewServeMux()
	mux.HandleFunc(contract.HealthPath, withMetrics("healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"status":    "ok",
			"component": contract.ComponentSupervisor,
		})
	}))
	mux.HandleFunc(contract.ReadyPath, withMetrics("readyz", func(w http.ResponseWriter, _ *http.Request) {
		s := state.Snapshot()
		if !Ready(s) {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{
				"status":    "degraded",
				"component": contract.ComponentSupervisor,
			})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"status":    "ok",
			"component": contract.ComponentSupervisor,
		})
	}))
	mux.HandleFunc("/status", withMetrics("status", func(w http.ResponseWriter, _ *http.Request) {
		s := state.Snapshot()
		restarts := s.BrokerRestarts + s.VectorstoreRestarts
		status := ContractStatus(s)
		details := map[string]any{
			"children": map[string]any{
				"broker": map[string]any{
					"required": s.BrokerRequired,
					"ready":    s.BrokerReady,
					"restarts": s.BrokerRestarts,
					"endpoint": s.BrokerEndpoint,
				},
				"vectorstore": map[string]any{
					"required": s.VectorstoreRequired,
					"ready":    s.VectorstoreReady,
					"restarts": s.VectorstoreRestarts,
					"endpoint": s.VectorstoreEndpoint,
				},
			},
		}
		payload := contract.StatusPayload{
			Component:   contract.ComponentSupervisor,
			BackendName: "custom",
			BackendMode: "binary",
			Status:      status,
			Timestamp:   time.Now().UTC(),
			Version: contract.Version{
				Wrapper:  s.WrapperVersion,
				BuildSHA: s.BuildCommit,
			},
			Message:  "supervisor wrapper orchestration state",
			Restarts: &restarts,
			Details:  details,
		}
		if strings.TrimSpace(s.LastError) != "" {
			payload.LastError = s.LastError
		}
		if ui := strings.TrimSpace(s.OperatorUIBaseURL); ui != "" {
			payload.Details["operator_ui"] = map[string]any{
				"base_url":  ui,
				"bootstrap": s.Bootstrap,
			}
		}
		if err := payload.Validate(); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{
				"status": "error",
				"detail": err.Error(),
			})
			return
		}
		code := http.StatusOK
		if payload.Status != "ok" {
			code = http.StatusServiceUnavailable
		}
		writeJSON(w, code, payload)
	}))
	mux.HandleFunc("/shutdown", withMetrics("shutdown", func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if onShutdown == nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{
				"status": "unavailable",
				"detail": "shutdown not configured",
			})
			return
		}
		writeJSON(w, http.StatusAccepted, map[string]any{
			"status":    "shutting_down",
			"component": contract.ComponentSupervisor,
		})
		go onShutdown()
	}))
	mux.HandleFunc(contract.MetricsPath, withMetrics("metrics", func(w http.ResponseWriter, _ *http.Request) {
		s := state.Snapshot()
		restarts := s.BrokerRestarts + s.VectorstoreRestarts
		backendUp := 1
		if !Ready(s) {
			backendUp = 0
		}
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		var b strings.Builder
		fmt.Fprintf(&b, "# HELP chimera_wrapper_up Wrapper process health.\n# TYPE chimera_wrapper_up gauge\nchimera_wrapper_up{component=%q} 1\n", contract.ComponentSupervisor)
		fmt.Fprintf(&b, "# HELP chimera_backend_up Upstream backend readiness.\n# TYPE chimera_backend_up gauge\nchimera_backend_up{component=%q} %d\n", contract.ComponentSupervisor, backendUp)
		fmt.Fprintf(&b, "# HELP chimera_backend_restarts_total Backend restart count.\n# TYPE chimera_backend_restarts_total counter\nchimera_backend_restarts_total{component=%q} %d\n", contract.ComponentSupervisor, restarts)
		b.WriteString(metrics.render(contract.ComponentSupervisor))
		_, _ = w.Write([]byte(b.String()))
	}))
	servicelogs.RegisterLogRoutes(mux, logStore)
	return mux
}
