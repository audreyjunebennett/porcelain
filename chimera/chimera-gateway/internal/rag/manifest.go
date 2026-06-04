package rag

import (
	"errors"
	"fmt"
	"strings"

	ichunk "github.com/lynn/porcelain/internal/chunk"
)

// ManifestChunk is one pre-chunked segment from the indexer.
type ManifestChunk struct {
	ChunkIndex    int    `json:"chunk_index"`
	Text          string `json:"text"`
	StartLine     int    `json:"start_line"`
	EndLine       int    `json:"end_line"`
	StartByte     int    `json:"start_byte"`
	EndByte       int    `json:"end_byte"`
	StartCh       int    `json:"start_ch"`
	EndCh         int    `json:"end_ch"`
	StartsMidLine bool   `json:"starts_mid_line"`
	Language      string `json:"language"`
}

// IngestManifest is the JSON body for POST /v1/ingest (manifest-only path).
type IngestManifest struct {
	Object            string          `json:"object"`
	Source            string          `json:"source"`
	ContentSHA256     string          `json:"content_sha256"`
	ClientContentHash string          `json:"client_content_hash"`
	ChunkSize         int             `json:"chunk_size"`
	ChunkOverlap      int             `json:"chunk_overlap"`
	ChunkSchema       int             `json:"chunk_schema"`
	LineCount         int             `json:"line_count"`
	FileBytes         int             `json:"file_bytes"`
	Chunks            []ManifestChunk `json:"chunks"`
}

// ValidateManifest checks required manifest ingest fields.
func ValidateManifest(m *IngestManifest) error {
	if m == nil {
		return errors.New("ingest: missing manifest")
	}
	if strings.TrimSpace(m.Object) != "" && m.Object != "ingest.manifest" {
		return fmt.Errorf("ingest: unsupported object %q", m.Object)
	}
	if strings.TrimSpace(m.Source) == "" {
		return errors.New("ingest: missing source")
	}
	if strings.TrimSpace(m.ContentSHA256) == "" {
		return errors.New("ingest: missing content_sha256")
	}
	if m.ChunkSchema != ichunk.SchemaV2 {
		return fmt.Errorf("ingest: unsupported chunk_schema %d", m.ChunkSchema)
	}
	if len(m.Chunks) == 0 {
		return errors.New("ingest: manifest has no chunks")
	}
	for i, c := range m.Chunks {
		if c.ChunkIndex != i {
			return fmt.Errorf("ingest: chunk_index %d out of order at %d", c.ChunkIndex, i)
		}
		if strings.TrimSpace(c.Text) == "" {
			return fmt.Errorf("ingest: empty chunk text at index %d", i)
		}
		if c.StartLine < 1 || c.EndLine < c.StartLine {
			return fmt.Errorf("ingest: invalid line span at chunk %d", i)
		}
	}
	return nil
}
