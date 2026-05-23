package servicelogs

import (
	"strings"
	"testing"
)

func TestIndexerPinKey_runStart(t *testing.T) {
	text := `{"msg":"indexer.run.start","index_run_id":"abc","service":"indexer"}`
	key, ok := indexerPinKey(text)
	if !ok || key != "start:abc" {
		t.Fatalf("got %q ok=%v", key, ok)
	}
}

func TestIndexerPinKey_scopeStatus(t *testing.T) {
	text := `{"msg":"indexer.scope.status","index_run_id":"r1","indexer_target_key":"itk1","change_reason":"queue_ingest_pending"}`
	key, ok := indexerPinKey(text)
	if !ok || key != "scope:r1:itk1" {
		t.Fatalf("got %q ok=%v", key, ok)
	}
}

func TestStore_IndexerPinningRetainsRunStart(t *testing.T) {
	s := New(20)
	s.SetIndexerPinnedLinesMax(8)
	w := s.Writer(SourceChimeraIndexer)
	for i := 0; i < 30; i++ {
		_, _ = w.Write([]byte(`{"msg":"indexer.job.skipped","rel":"f.go","index_run_id":"run1"}` + "\n"))
	}
	_, _ = w.Write([]byte(`{"msg":"indexer.run.start","index_run_id":"run1","root_scopes":"[]"}` + "\n"))
	foundStart := false
	for _, e := range s.Snapshot() {
		if strings.Contains(e.Text, `"msg":"indexer.run.start"`) && strings.Contains(e.Text, "run1") {
			foundStart = true
			break
		}
	}
	if !foundStart {
		t.Fatal("pinned indexer.run.start evicted by verbose traffic")
	}
}
