package metrics

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/server/adminui/handler"
	"github.com/lynn/porcelain/internal/naming"
	"github.com/lynn/porcelain/internal/operatorapi"
)

func handleMetricsGET(h *handler.Handler, w http.ResponseWriter, r *http.Request) {
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

	h.RT.Sync()
	res, _ := h.RT.Snapshot()
	now := time.Now().UTC()
	minute := now.Format("2006-01-02T15:04")
	day := now.Format("2006-01-02")

	st := h.RT.MetricsStore()
	out := operatorapi.MetricsResponse{
		OK:                   true,
		MetricsStoreOpen:     st != nil,
		MetricsConfigEnabled: res != nil && res.MetricsEnabled,
		NowUTC:               now.Format(time.RFC3339),
		CurrentMinuteUTC:     minute,
		CurrentDayUTC:        day,
		BucketsNote:          "Minute and day buckets are UTC calendar boundaries (see docs/plans/version-v0.1.1.md §3.6.4).",
		EstimatorNote:        "est_tokens uses tiktoken cl100k_base on the proxied JSON body (same family as outgoingTokens logs).",
		MinuteRollups:        []operatorapi.UsageRollup{},
		DayRollups:           []operatorapi.UsageRollup{},
		RecentEvents:         []operatorapi.CallEvent{},
	}
	if res != nil {
		out.SQLitePath = res.MetricsSQLitePath
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	if st == nil {
		out.Message = "Metrics SQLite is not open (disabled in " + naming.GatewayConfigFileTarget + ", init failure, or metrics.enabled: false). Check gateway startup logs."
		writeMetrics(w, out)
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

	out.MinuteRollups = toOperatorRollups(minRows)
	out.DayRollups = toOperatorRollups(dayRows)
	out.RecentEvents = toOperatorEvents(events)
	writeMetrics(w, out)
}

func writeMetrics(w http.ResponseWriter, out operatorapi.MetricsResponse) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

func writeMetricsError(w http.ResponseWriter, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusInternalServerError)
	_ = json.NewEncoder(w).Encode(operatorapi.MetricsResponse{
		OK:    false,
		Error: err.Error(),
	})
}
