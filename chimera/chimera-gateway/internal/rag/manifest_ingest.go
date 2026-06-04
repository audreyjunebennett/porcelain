package rag

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/operatorstore"
	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/vectorstore"
)

// ManifestIngestRequest is one manifest document ingest.
type ManifestIngestRequest struct {
	Coords         vectorstore.Coords
	Manifest       IngestManifest
	RequestID      string
	ConversationID string
	IndexRunID     string
}

// IngestManifest embeds manifest chunk texts and upserts Qdrant points + corpus_segments.
func (s *Service) IngestManifest(ctx context.Context, m IngestManifest, coords vectorstore.Coords, op *operatorstore.Store, meta ManifestIngestRequest) (IngestResult, error) {
	_ = meta
	res := IngestResult{Source: strings.TrimSpace(m.Source)}
	if err := ValidateManifest(&m); err != nil {
		return res, err
	}
	if coords.TenantID == "" {
		return res, fmt.Errorf("ingest: empty tenant_id")
	}
	collection := vectorstore.CollectionName(coords)
	res.Collection = collection
	if err := s.store.EnsureCollection(ctx, collection, s.embedDim); err != nil {
		return res, fmt.Errorf("ensure collection %s: %w", collection, err)
	}

	inputs := make([]string, len(m.Chunks))
	for i, c := range m.Chunks {
		inputs[i] = c.Text
	}
	vectors, err := s.embedder.EmbedBatch(ctx, inputs)
	if err != nil {
		return res, fmt.Errorf("embed: %w", err)
	}
	if len(vectors) != len(m.Chunks) {
		return res, fmt.Errorf("embed returned %d vectors for %d chunks", len(vectors), len(m.Chunks))
	}

	if err := s.store.DeleteBySource(ctx, collection, res.Source); err != nil && s.log != nil {
		s.log.Debug("delete-by-source pre-ingest failed (likely empty collection)",
			"msg", "rag.ingest.delete_pre_failed", "source", res.Source, "err", err)
	}

	serverHash := strings.TrimSpace(m.ContentSHA256)
	clientHash := strings.TrimSpace(m.ClientContentHash)
	now := time.Now().Unix()
	chunkCount := len(m.Chunks)
	pts := make([]vectorstore.Point, 0, chunkCount)
	segRows := make([]operatorstore.CorpusSegmentRow, 0, chunkCount)
	for i, c := range m.Chunks {
		if len(vectors[i]) != s.embedDim {
			return res, fmt.Errorf("embed dim mismatch at chunk %d", i)
		}
		pid := vectorstore.PointID(coords, res.Source, i)
		lang := strings.TrimSpace(c.Language)
		if lang == "" {
			lang = strings.TrimSpace(m.Chunks[0].Language)
		}
		pts = append(pts, vectorstore.Point{
			ID:     pid,
			Vector: vectors[i],
			Payload: vectorstore.Payload{
				TenantID:          coords.TenantID,
				ProjectID:         coords.ProjectID,
				FlavorID:          coords.FlavorID,
				Text:              c.Text,
				Source:            res.Source,
				CreatedAt:         now,
				ContentSHA256:     serverHash,
				ClientContentHash: clientHash,
				ChunkIndex:        i,
				ChunkCount:        chunkCount,
				StartLine:         c.StartLine,
				EndLine:           c.EndLine,
				StartByte:         c.StartByte,
				EndByte:           c.EndByte,
				StartCh:           c.StartCh,
				EndCh:             c.EndCh,
				StartsMidLine:     c.StartsMidLine,
				LineCount:         m.LineCount,
				FileBytes:         m.FileBytes,
				ChunkSchema:       m.ChunkSchema,
				Language:          lang,
			},
		})
		if op != nil {
			segRows = append(segRows, operatorstore.CorpusSegmentRow{
				SegmentID:     pid,
				Source:        res.Source,
				ContentSHA256: serverHash,
				ChunkIndex:    i,
				ChunkCount:    chunkCount,
				StartLine:     c.StartLine,
				EndLine:       c.EndLine,
				StartByte:     c.StartByte,
				EndByte:       c.EndByte,
				StartCh:       c.StartCh,
				EndCh:         c.EndCh,
				StartsMidLine: c.StartsMidLine,
				VectorPointID: pid,
				Language:      lang,
				CreatedAt:     time.Unix(now, 0).UTC(),
			})
		}
	}
	if err := s.store.Upsert(ctx, collection, pts); err != nil {
		return res, fmt.Errorf("upsert: %w", err)
	}
	if op != nil {
		if err := op.ReplaceCorpusSegmentsForSource(ctx, coords.TenantID, coords.ProjectID, coords.FlavorID, res.Source, segRows); err != nil {
			return res, fmt.Errorf("corpus_segments: %w", err)
		}
	}
	res.Chunks = chunkCount
	res.ClientContentHash = clientHash
	res.ContentSHA256 = serverHash
	res.ContentHash = serverHash
	return res, nil
}
