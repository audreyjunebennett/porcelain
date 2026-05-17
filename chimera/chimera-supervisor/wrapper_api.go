package main

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

type supervisorControlState struct {
	mu                  sync.RWMutex
	brokerRequired      bool
	vectorstoreRequired bool
	brokerReady         bool
	vectorstoreReady    bool
	brokerRestarts      int
	vectorstoreRestarts int
	lastError           string
	wrapperVersion      string
	buildCommit         string
	brokerEndpoint      string
	vectorstoreEndpoint string
	operatorUIBaseURL   string
	bootstrap           bool
}

func newSupervisorControlState() *supervisorControlState {
	return &supervisorControlState{}
}

func (s *supervisorControlState) setRequired(brokerRequired, vectorstoreRequired bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.brokerRequired = brokerRequired
	s.vectorstoreRequired = vectorstoreRequired
}

func (s *supervisorControlState) setVersions(wrapperVersion, buildCommit string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.wrapperVersion = strings.TrimSpace(wrapperVersion)
	s.buildCommit = strings.TrimSpace(buildCommit)
}

func (s *supervisorControlState) setEndpoints(brokerEndpoint, vectorstoreEndpoint string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.brokerEndpoint = strings.TrimSpace(brokerEndpoint)
	s.vectorstoreEndpoint = strings.TrimSpace(vectorstoreEndpoint)
}

func (s *supervisorControlState) setBrokerReady(v bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.brokerReady = v
}

func (s *supervisorControlState) setVectorstoreReady(v bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.vectorstoreReady = v
}

func (s *supervisorControlState) incBrokerRestarts() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.brokerRestarts++
}

func (s *supervisorControlState) incVectorstoreRestarts() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.vectorstoreRestarts++
}

func (s *supervisorControlState) setLastError(err string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastError = strings.TrimSpace(err)
}

func (s *supervisorControlState) setOperatorUI(baseURL string, bootstrap bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.operatorUIBaseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	s.bootstrap = bootstrap
}

type supervisorSnapshot struct {
	brokerRequired      bool
	vectorstoreRequired bool
	brokerReady         bool
	vectorstoreReady    bool
	brokerRestarts      int
	vectorstoreRestarts int
	lastError           string
	wrapperVersion      string
	buildCommit         string
	brokerEndpoint      string
	vectorstoreEndpoint string
	operatorUIBaseURL   string
	bootstrap           bool
}

func (s *supervisorControlState) snapshot() supervisorSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return supervisorSnapshot{
		brokerRequired:      s.brokerRequired,
		vectorstoreRequired: s.vectorstoreRequired,
		brokerReady:         s.brokerReady,
		vectorstoreReady:    s.vectorstoreReady,
		brokerRestarts:      s.brokerRestarts,
		vectorstoreRestarts: s.vectorstoreRestarts,
		lastError:           s.lastError,
		wrapperVersion:      s.wrapperVersion,
		buildCommit:         s.buildCommit,
		brokerEndpoint:      s.brokerEndpoint,
		vectorstoreEndpoint: s.vectorstoreEndpoint,
		operatorUIBaseURL:   s.operatorUIBaseURL,
		bootstrap:           s.bootstrap,
	}
}

func supervisorContractStatus(s supervisorSnapshot) string {
	if s.brokerRequired && !s.brokerReady {
		return "degraded"
	}
	if s.vectorstoreRequired && !s.vectorstoreReady {
		return "degraded"
	}
	return "ok"
}

func supervisorReady(s supervisorSnapshot) bool {
	return supervisorContractStatus(s) == "ok"
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

func buildWrapperControlMux(state *supervisorControlState, logStore *servicelogs.Store) http.Handler {
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
		s := state.snapshot()
		if !supervisorReady(s) {
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
		s := state.snapshot()
		restarts := s.brokerRestarts + s.vectorstoreRestarts
		status := supervisorContractStatus(s)
		details := map[string]any{
			"children": map[string]any{
				"broker": map[string]any{
					"required": s.brokerRequired,
					"ready":    s.brokerReady,
					"restarts": s.brokerRestarts,
					"endpoint": s.brokerEndpoint,
				},
				"vectorstore": map[string]any{
					"required": s.vectorstoreRequired,
					"ready":    s.vectorstoreReady,
					"restarts": s.vectorstoreRestarts,
					"endpoint": s.vectorstoreEndpoint,
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
				Wrapper:  s.wrapperVersion,
				BuildSHA: s.buildCommit,
			},
			Message:  "supervisor wrapper orchestration state",
			Restarts: &restarts,
			Details:  details,
		}
		if strings.TrimSpace(s.lastError) != "" {
			payload.LastError = s.lastError
		}
		if ui := strings.TrimSpace(s.operatorUIBaseURL); ui != "" {
			payload.Details["operator_ui"] = map[string]any{
				"base_url":  ui,
				"bootstrap": s.bootstrap,
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
	mux.HandleFunc(contract.MetricsPath, withMetrics("metrics", func(w http.ResponseWriter, _ *http.Request) {
		s := state.snapshot()
		restarts := s.brokerRestarts + s.vectorstoreRestarts
		backendUp := 1
		if !supervisorReady(s) {
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
