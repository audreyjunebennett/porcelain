package gatewaymetrics

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// ModelUsage is a pair of counters (calls, est tokens) for a single model over a time window.
type ModelUsage struct {
	Calls     int64
	EstTokens int64
}

// UsageForModelWindow returns (calls, est_tokens) summed across all statuses for modelID between
// [start, end) using the upstream_call_events log. The caller chooses start/end — typically the
// UTC minute boundary for RPM/TPM checks or a vendor-local day window for RPD/TPD checks. Both
// bounds are applied against the RFC3339Nano UTC timestamp stored at insert time.
func (s *Store) UsageForModelWindow(ctx context.Context, modelID string, start, end time.Time) (ModelUsage, error) {
	if s == nil || s.db == nil {
		return ModelUsage{}, nil
	}
	q := `SELECT COALESCE(COUNT(*),0), COALESCE(SUM(est_tokens),0)
FROM upstream_call_events
WHERE model_id = ? AND occurred_at >= ? AND occurred_at < ?`
	var calls int64
	var tokens sql.NullInt64
	startStr := start.UTC().Format(time.RFC3339Nano)
	endStr := end.UTC().Format(time.RFC3339Nano)
	row := s.db.QueryRowContext(ctx, q, modelID, startStr, endStr)
	if err := row.Scan(&calls, &tokens); err != nil {
		return ModelUsage{}, fmt.Errorf("usage window: %w", err)
	}
	u := ModelUsage{Calls: calls}
	if tokens.Valid {
		u.EstTokens = tokens.Int64
	}
	return u, nil
}
