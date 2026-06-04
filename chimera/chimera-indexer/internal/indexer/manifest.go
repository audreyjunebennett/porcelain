package indexer

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"strings"

	ichunk "github.com/lynn/porcelain/internal/chunk"
)

const manifestChunkSchema = ichunk.SchemaV2

// ManifestChunk is one segment in an ingest manifest payload.
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

// IngestManifest is the JSON body for POST /v1/ingest.
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

// ReadNormalizeFile reads path, normalizes newlines, and returns text + sha256 digest.
func ReadNormalizeFile(path string) (normalized string, hash string, err error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", "", err
	}
	normalized = ichunk.NormalizeNewlines(string(b))
	sum := sha256.Sum256([]byte(normalized))
	hash = "sha256:" + hex.EncodeToString(sum[:])
	return normalized, hash, nil
}

// BuildManifest constructs an ingest.manifest from normalized file text.
func BuildManifest(source, normalized, clientHash string, chunkSize, chunkOverlap int) (IngestManifest, error) {
	source = strings.TrimSpace(source)
	if source == "" {
		return IngestManifest{}, fmt.Errorf("manifest: empty source")
	}
	if strings.TrimSpace(normalized) == "" {
		return IngestManifest{}, fmt.Errorf("manifest: empty text")
	}
	lang := ichunk.LanguageFromPath(source)
	segs := ichunk.Split(normalized, chunkSize, chunkOverlap)
	if len(segs) == 0 {
		return IngestManifest{}, fmt.Errorf("manifest: no chunks for %s", source)
	}
	sum := sha256.Sum256([]byte(normalized))
	serverHash := "sha256:" + hex.EncodeToString(sum[:])
	lineCount := 1
	if len(normalized) > 0 {
		lineCount = strings.Count(normalized, "\n") + 1
	}
	chunks := make([]ManifestChunk, len(segs))
	for i, s := range segs {
		chunks[i] = ManifestChunk{
			ChunkIndex:    i,
			Text:          s.Text,
			StartLine:     s.StartLine,
			EndLine:       s.EndLine,
			StartByte:     s.StartByte,
			EndByte:       s.EndByte,
			StartCh:       s.StartCh,
			EndCh:         s.EndCh,
			StartsMidLine: s.StartsMidLine,
			Language:      lang,
		}
	}
	return IngestManifest{
		Object:            "ingest.manifest",
		Source:            source,
		ContentSHA256:     serverHash,
		ClientContentHash: clientHash,
		ChunkSize:         chunkSize,
		ChunkOverlap:      chunkOverlap,
		ChunkSchema:       manifestChunkSchema,
		LineCount:         lineCount,
		FileBytes:         len(normalized),
		Chunks:            chunks,
	}, nil
}
