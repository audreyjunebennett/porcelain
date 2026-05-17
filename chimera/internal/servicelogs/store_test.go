package servicelogs

import (
	"testing"
	"time"
)

func TestStore_ImportPreservesSeq(t *testing.T) {
	s := New(100)
	ts := time.Now().UTC()
	s.Import([]Entry{
		{Seq: 10, Source: SourceChimeraGateway, Text: `{"msg":"gateway.startup.seed"}`, Time: ts},
		{Seq: 11, Source: SourceChimeraBroker, Text: `{"msg":"broker.ready"}`, Time: ts},
	})
	s.Import([]Entry{
		{Seq: 10, Source: SourceChimeraGateway, Text: `{"msg":"dup"}`, Time: ts},
		{Seq: 12, Source: SourceChimeraSupervisor, Text: `{"msg":"chimera-supervisor.startup.seed"}`, Time: ts},
	})
	lines, maxSeq := s.EntriesAfter(0)
	if maxSeq != 12 {
		t.Fatalf("maxSeq=%d", maxSeq)
	}
	if len(lines) != 3 {
		t.Fatalf("len=%d", len(lines))
	}
	if lines[0].Seq != 10 || lines[2].Source != SourceChimeraSupervisor {
		t.Fatalf("lines=%+v", lines)
	}
}
