package server

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/lynn/claudia-gateway/internal/gatewaymetrics"
)

func (a *adminUI) handleMetricsGET(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	limit := 100
	if q := r.URL.Query().Get("limit"); q != "" {
		if n, err := strconv.Atoi(q); err == nil && n > 0 && n <= 500 {
			limit = n
		}
	}

	a.rt.Sync()
	res, _, _ := a.rt.Snapshot()
	now := time.Now().UTC()
	minute := now.Format("2006-01-02T15:04")
	day := now.Format("2006-01-02")

	st := a.rt.MetricsStore()
	out := map[string]any{
		"ok":                     true,
		"metrics_store_open":     st != nil,
		"metrics_config_enabled": res != nil && res.MetricsEnabled,
		"sqlite_path":            "",
		"now_utc":                now.Format(time.RFC3339),
		"current_minute_utc":     minute,
		"current_day_utc":        day,
		"buckets_note":           "Minute and day buckets are UTC calendar boundaries (see docs/plans/version-v0.1.1.md §3.6.4).",
		"estimator_note":         "est_tokens uses tiktoken cl100k_base on the proxied JSON body (same family as outgoingTokens logs).",
	}
	if res != nil {
		out["sqlite_path"] = res.MetricsSQLitePath
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	if st == nil {
		out["minute_rollups"] = []gatewaymetrics.UsageRollup{}
		out["day_rollups"] = []gatewaymetrics.UsageRollup{}
		out["recent_events"] = []gatewaymetrics.CallEvent{}
		out["message"] = "Metrics SQLite is not open (disabled in gateway.yaml, init failure, or metrics.enabled: false). Check gateway startup logs."
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(out)
		return
	}

	minRows, err := st.QueryMinuteRollups(ctx, minute, limit)
	if err != nil {
		writeMetricsError(w, err)
		return
	}
	dayRows, err := st.QueryDayRollups(ctx, day, limit)
	if err != nil {
		writeMetricsError(w, err)
		return
	}
	events, err := st.QueryRecentEvents(ctx, limit)
	if err != nil {
		writeMetricsError(w, err)
		return
	}

	out["minute_rollups"] = minRows
	out["day_rollups"] = dayRows
	out["recent_events"] = events
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

func writeMetricsError(w http.ResponseWriter, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusInternalServerError)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":    false,
		"error": err.Error(),
	})
}
