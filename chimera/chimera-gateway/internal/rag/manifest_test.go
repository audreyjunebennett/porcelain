package rag

import (
	"testing"

	ichunk "github.com/lynn/porcelain/internal/chunk"
)

func TestValidateManifest_ok(t *testing.T) {
	m := &IngestManifest{
		Object:        "ingest.manifest",
		Source:        "a.go",
		ContentSHA256: "sha256:abc",
		ChunkSchema:   ichunk.SchemaV2,
		Chunks: []ManifestChunk{{
			ChunkIndex: 0, Text: "hello", StartLine: 1, EndLine: 1,
			StartByte: 0, EndByte: 5, StartCh: 0, EndCh: 5,
		}},
	}
	if err := ValidateManifest(m); err != nil {
		t.Fatal(err)
	}
}

func TestValidateManifest_rejectsLegacy(t *testing.T) {
	m := &IngestManifest{Source: "a", ContentSHA256: "sha256:x", ChunkSchema: 1, Chunks: []ManifestChunk{{ChunkIndex: 0, Text: "x", StartLine: 1, EndLine: 1}}}
	if err := ValidateManifest(m); err == nil {
		t.Fatal("expected schema error")
	}
}
