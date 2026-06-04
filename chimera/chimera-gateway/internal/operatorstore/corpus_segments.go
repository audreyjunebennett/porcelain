package operatorstore

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// CorpusSegmentRow is one indexed chunk stored for tooling lookups.
type CorpusSegmentRow struct {
	SegmentID     string
	TenantID      string
	ProjectID     string
	FlavorID      string
	Source        string
	ContentSHA256 string
	ChunkIndex    int
	ChunkCount    int
	StartLine     int
	EndLine       int
	StartByte     int
	EndByte       int
	StartCh       int
	EndCh         int
	StartsMidLine bool
	VectorPointID string
	Language      string
	CreatedAt     time.Time
}

// ReplaceCorpusSegmentsForSource deletes prior segment rows for source (all hashes) and inserts new rows.
func (s *Store) ReplaceCorpusSegmentsForSource(ctx context.Context, tenantID, projectID, flavorID, source string, rows []CorpusSegmentRow) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("operator store unavailable")
	}
	source = strings.TrimSpace(source)
	if source == "" {
		return fmt.Errorf("source required")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `
DELETE FROM corpus_segments
WHERE tenant_id = ? AND project_id = ? AND flavor_id = ? AND source = ?`,
		tenantID, projectID, flavorID, source); err != nil {
		return err
	}
	if len(rows) == 0 {
		return tx.Commit()
	}
	stmt, err := tx.PrepareContext(ctx, `
INSERT INTO corpus_segments (
	segment_id, tenant_id, project_id, flavor_id, source, content_sha256,
	chunk_index, chunk_count, start_line, end_line, start_byte, end_byte,
	start_ch, end_ch, starts_mid_line, vector_point_id, language, created_at
) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, r := range rows {
		mid := 0
		if r.StartsMidLine {
			mid = 1
		}
		ca := r.CreatedAt.UTC().Format(time.RFC3339Nano)
		if ca == "" || ca == "0001-01-01T00:00:00Z" {
			ca = s.nowRFC3339()
		}
		if _, err := stmt.ExecContext(ctx,
			r.SegmentID, tenantID, projectID, flavorID, source, r.ContentSHA256,
			r.ChunkIndex, r.ChunkCount, r.StartLine, r.EndLine, r.StartByte, r.EndByte,
			r.StartCh, r.EndCh, mid, r.VectorPointID, r.Language, ca,
		); err != nil {
			return err
		}
	}
	return tx.Commit()
}
