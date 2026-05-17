package gatewaymetrics

import (
	"context"
	"database/sql"
	"fmt"
)

// UsageRollup is one row from minute or day rollup tables (UTC buckets).
type UsageRollup struct {
	Provider  string `json:"provider"`
	ModelID   string `json:"model_id"`
	Status    int    `json:"status"`
	Calls     int    `json:"calls"`
	EstTokens int    `json:"est_tokens"`
}

// CallEvent is a recent row from upstream_call_events.
type CallEvent struct {
	OccurredAt string `json:"occurred_at"`
	Provider   string `json:"provider"`
	ModelID    string `json:"model_id"`
	Status     int    `json:"status"`
	EstTokens  int    `json:"est_tokens"`
}

// QueryMinuteRollups returns rollup rows for the given UTC minute key (format 2006-01-02T15:04).
func (s *Store) QueryMinuteRollups(ctx context.Context, minuteUTC string, limit int) ([]UsageRollup, error) {
	if s == nil || s.db == nil {
		return nil, nil
	}
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	q := `SELECT provider, model_id, status, calls, est_tokens
FROM upstream_rollup_minute WHERE minute_utc = ? ORDER BY calls DESC, model_id LIMIT ?`
	rows, err := s.db.QueryContext(ctx, q, minuteUTC, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRollups(rows)
}

// QueryDayRollups returns rollup rows for the given UTC calendar day (format 2006-01-02).
func (s *Store) QueryDayRollups(ctx context.Context, dayUTC string, limit int) ([]UsageRollup, error) {
	if s == nil || s.db == nil {
		return nil, nil
	}
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	q := `SELECT provider, model_id, status, calls, est_tokens
FROM upstream_rollup_day WHERE day_utc = ? ORDER BY calls DESC, model_id LIMIT ?`
	rows, err := s.db.QueryContext(ctx, q, dayUTC, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRollups(rows)
}

func scanRollups(rows *sql.Rows) ([]UsageRollup, error) {
	var out []UsageRollup
	for rows.Next() {
		var r UsageRollup
		if err := rows.Scan(&r.Provider, &r.ModelID, &r.Status, &r.Calls, &r.EstTokens); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// QueryRecentEvents returns the most recent call events (newest first).
func (s *Store) QueryRecentEvents(ctx context.Context, limit int) ([]CallEvent, error) {
	if s == nil || s.db == nil {
		return nil, nil
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	q := `SELECT occurred_at, provider, model_id, status, est_tokens
FROM upstream_call_events ORDER BY id DESC LIMIT ?`
	rows, err := s.db.QueryContext(ctx, q, limit)
	if err != nil {
		return nil, fmt.Errorf("recent events: %w", err)
	}
	defer rows.Close()
	var out []CallEvent
	for rows.Next() {
		var e CallEvent
		if err := rows.Scan(&e.OccurredAt, &e.Provider, &e.ModelID, &e.Status, &e.EstTokens); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
