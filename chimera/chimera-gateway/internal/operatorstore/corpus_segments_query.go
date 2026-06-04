package operatorstore

import (
	"context"
	"fmt"
	"strings"
)

// ListCorpusSegments returns segment rows for one file version in scope.
func (s *Store) ListCorpusSegments(ctx context.Context, tenantID, projectID, flavorID, source, contentSHA256 string) ([]CorpusSegmentRow, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("operator store unavailable")
	}
	source = strings.TrimSpace(source)
	contentSHA256 = strings.TrimSpace(contentSHA256)
	if source == "" || contentSHA256 == "" {
		return nil, fmt.Errorf("source and content_sha256 required")
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT segment_id, tenant_id, project_id, flavor_id, source, content_sha256,
	chunk_index, chunk_count, start_line, end_line, start_byte, end_byte,
	start_ch, end_ch, starts_mid_line, vector_point_id, language, created_at
FROM corpus_segments
WHERE tenant_id = ? AND project_id = ? AND flavor_id = ? AND source = ? AND content_sha256 = ?
ORDER BY chunk_index ASC`,
		tenantID, projectID, flavorID, source, contentSHA256)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanCorpusSegmentRows(rows)
}

// ListCorpusSegmentsAdjacent returns segments within chunk_index ± radius.
func (s *Store) ListCorpusSegmentsAdjacent(ctx context.Context, tenantID, projectID, flavorID, source, contentSHA256 string, chunkIndex, radius int) ([]CorpusSegmentRow, error) {
	if radius < 0 {
		radius = 0
	}
	all, err := s.ListCorpusSegments(ctx, tenantID, projectID, flavorID, source, contentSHA256)
	if err != nil {
		return nil, err
	}
	lo := chunkIndex - radius
	hi := chunkIndex + radius
	var out []CorpusSegmentRow
	for _, r := range all {
		if r.ChunkIndex >= lo && r.ChunkIndex <= hi {
			out = append(out, r)
		}
	}
	return out, nil
}

// CorpusSegmentByPointID loads one row by Qdrant point id.
func (s *Store) CorpusSegmentByPointID(ctx context.Context, tenantID, projectID, flavorID, pointID string) (*CorpusSegmentRow, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("operator store unavailable")
	}
	pointID = strings.TrimSpace(pointID)
	if pointID == "" {
		return nil, fmt.Errorf("point_id required")
	}
	var r CorpusSegmentRow
	var mid int
	var ca string
	err := s.db.QueryRowContext(ctx, `
SELECT segment_id, tenant_id, project_id, flavor_id, source, content_sha256,
	chunk_index, chunk_count, start_line, end_line, start_byte, end_byte,
	start_ch, end_ch, starts_mid_line, vector_point_id, language, created_at
FROM corpus_segments
WHERE tenant_id = ? AND project_id = ? AND flavor_id = ? AND vector_point_id = ?`,
		tenantID, projectID, flavorID, pointID).Scan(
		&r.SegmentID, &r.TenantID, &r.ProjectID, &r.FlavorID, &r.Source, &r.ContentSHA256,
		&r.ChunkIndex, &r.ChunkCount, &r.StartLine, &r.EndLine, &r.StartByte, &r.EndByte,
		&r.StartCh, &r.EndCh, &mid, &r.VectorPointID, &r.Language, &ca)
	if err != nil {
		return nil, err
	}
	r.StartsMidLine = mid == 1
	r.CreatedAt = parseTime(ca)
	return &r, nil
}

func scanCorpusSegmentRows(rows interface {
	Next() bool
	Scan(dest ...any) error
	Err() error
}) ([]CorpusSegmentRow, error) {
	var out []CorpusSegmentRow
	for rows.Next() {
		var r CorpusSegmentRow
		var mid int
		var ca string
		if err := rows.Scan(
			&r.SegmentID, &r.TenantID, &r.ProjectID, &r.FlavorID, &r.Source, &r.ContentSHA256,
			&r.ChunkIndex, &r.ChunkCount, &r.StartLine, &r.EndLine, &r.StartByte, &r.EndByte,
			&r.StartCh, &r.EndCh, &mid, &r.VectorPointID, &r.Language, &ca,
		); err != nil {
			return nil, err
		}
		r.StartsMidLine = mid == 1
		r.CreatedAt = parseTime(ca)
		out = append(out, r)
	}
	return out, rows.Err()
}
