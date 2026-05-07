package servicelogs

import (
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestFormatMirrorTimestamp_fixedWidthMicroseconds(t *testing.T) {
	ts := time.Date(2026, 5, 6, 17, 4, 43, 592559623, time.UTC) // would vary with RFC3339Nano
	got := formatMirrorTimestamp(ts)
	want := "2026-05-06T17:04:43.592559Z"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	if len(got) != len(want) {
		t.Fatalf("length %d", len(got))
	}
	zero := formatMirrorTimestamp(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	if zero != "2026-01-01T00:00:00.000000Z" {
		t.Fatalf("zero frac: %q", zero)
	}
}

func TestSetMirror_writesTabSeparatedLines(t *testing.T) {
	var b strings.Builder
	s := New(10)
	s.SetMirror(&b)
	w := s.Writer("gateway")
	_, _ = io.WriteString(w, "hello\n")
	s.SetMirror(nil)
	got := b.String()
	if !strings.Contains(got, "\tgateway\thello\n") {
		t.Fatalf("mirror output %q", got)
	}
	if strings.Count(got, "\n") != 1 {
		t.Fatalf("expected single line, got %q", got)
	}
}

func TestLineWriter_splitsLinesAndCarriageReturn(t *testing.T) {
	s := New(100)
	w := s.Writer("test")
	_, _ = io.WriteString(w, "hello\r\nworld\n")
	lines := s.Snapshot()
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2: %#v", len(lines), lines)
	}
	if lines[0].Source != "test" || lines[0].Text != "hello" {
		t.Fatalf("line0: %+v", lines[0])
	}
	if lines[1].Text != "world" {
		t.Fatalf("line1: %+v", lines[1])
	}
}

func TestLineWriter_splitAcrossWrites(t *testing.T) {
	s := New(100)
	w := s.Writer("a")
	_, _ = w.Write([]byte("part1"))
	_, _ = w.Write([]byte("end\nnext\n"))
	got := s.Snapshot()
	if len(got) != 2 || got[0].Text != "part1end" || got[1].Text != "next" {
		t.Fatalf("got %+v", got)
	}
}

func TestStore_maxLinesEvictsOldest(t *testing.T) {
	s := New(3)
	for i := range 5 {
		s.add("g", fmt.Sprintf("L%d", i))
	}
	got := s.Snapshot()
	if len(got) != 3 {
		t.Fatalf("len=%d want 3", len(got))
	}
	if got[0].Text != "L2" || got[2].Text != "L4" {
		t.Fatalf("expected L2,L3,L4 got %#v", got)
	}
}

func TestIndexerCap_prefersTrimmingNoisyIndexerLines(t *testing.T) {
	s := New(50)
	wIdx := s.Writer("indexer")
	// One high-value line that must survive heavy noisy traffic.
	_, _ = io.WriteString(wIdx, `{"msg":"indexer.run.start","service":"indexer","root_scopes":[]}`+"\n")
	for i := range 40 {
		_, _ = io.WriteString(wIdx, fmt.Sprintf("indexer queue snapshot #%d\n", i))
	}
	_, _ = io.WriteString(wIdx, `{"msg":"indexer.job.skipped","service":"indexer","rel":"keep/this.vue"}`+"\n")
	got := s.Snapshot()
	var hasStart, hasJob bool
	for _, e := range got {
		if e.Source != "indexer" {
			continue
		}
		if strings.Contains(e.Text, "indexer.run.start") {
			hasStart = true
		}
		if strings.Contains(e.Text, "keep/this.vue") {
			hasJob = true
		}
	}
	if !hasStart {
		t.Fatal("expected indexer.run.start to survive when noisy lines are trimmed first")
	}
	if !hasJob {
		t.Fatal("expected last job line to remain in buffer")
	}
}

func TestIndexerCap_trimsOldestIndexerLines(t *testing.T) {
	s := New(100)
	wIdx := s.Writer("indexer")
	wGw := s.Writer("gateway")
	for i := range 80 {
		_, _ = io.WriteString(wIdx, fmt.Sprintf("ix-%d\n", i))
	}
	for i := range 80 {
		_, _ = io.WriteString(wGw, fmt.Sprintf("gw-%d\n", i))
	}
	got := s.Snapshot()
	idx := 0
	gw := 0
	for _, e := range got {
		if e.Source == "indexer" {
			idx++
		}
		if e.Source == "gateway" {
			gw++
		}
	}
	if idx > 25 {
		t.Fatalf("indexer lines %d > small-store cap 25", idx)
	}
	if gw < 20 {
		t.Fatalf("gateway crowded out: only %d gateway lines", gw)
	}
	if len(got) != 100 {
		t.Fatalf("want len 100 got %d", len(got))
	}
}

func TestEntriesBefore_chunk(t *testing.T) {
	s := New(100)
	for i := range 10 {
		s.add("x", fmt.Sprintf("L%d", i))
	}
	// seq 1..10; ask for lines before 8 with limit 3 => seq 5,6,7
	got := s.EntriesBefore(8, 3)
	if len(got) != 3 {
		t.Fatalf("got %#v", got)
	}
	if got[0].Text != "L4" || got[1].Text != "L5" || got[2].Text != "L6" {
		t.Fatalf("want L4,L5,L6 got %#v", got)
	}
}

func TestEntriesAfter_cursor(t *testing.T) {
	s := New(100)
	s.add("x", "a")
	s.add("x", "b")
	ent, maxSeq := s.EntriesAfter(0)
	if len(ent) != 2 || maxSeq != 2 {
		t.Fatalf("after 0: n=%d max=%d", len(ent), maxSeq)
	}
	ent2, _ := s.EntriesAfter(1)
	if len(ent2) != 1 || ent2[0].Text != "b" {
		t.Fatalf("after 1: %+v", ent2)
	}
}

func TestSubscribe_deliversNewEntries(t *testing.T) {
	s := New(100)
	ch, cancel := s.Subscribe(8)
	defer cancel()

	s.add("gw", "one")
	select {
	case e := <-ch:
		if e.Text != "one" || e.Source != "gw" {
			t.Fatalf("got %+v", e)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for broadcast")
	}
}

func TestSubscribe_slowConsumerDoesNotBlockAdd(t *testing.T) {
	s := New(100)
	_, cancel := s.Subscribe(1)
	defer cancel()

	done := make(chan struct{})
	go func() {
		for i := 0; i < 200; i++ {
			s.add("x", "line")
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("add blocked on slow subscriber")
	}
}

func TestConcurrentWriters_noPanic(t *testing.T) {
	s := New(500)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			w := s.Writer(fmt.Sprintf("w%d", id))
			for j := 0; j < 50; j++ {
				_, _ = io.WriteString(w, fmt.Sprintf("line-%d-%d\n", id, j))
			}
		}(i)
	}
	wg.Wait()
	if n := len(s.Snapshot()); n != 400 {
		t.Fatalf("expected 400 lines, got %d", n)
	}
}

func TestWriter_largeLineWithoutNewline_flushedWhenOverCap(t *testing.T) {
	s := New(100)
	w := s.Writer("big")
	huge := strings.Repeat("x", 70<<10)
	_, err := io.WriteString(w, huge)
	if err != nil {
		t.Fatal(err)
	}
	lines := s.Snapshot()
	if len(lines) < 1 {
		t.Fatal("expected flush of oversized fragment")
	}
}
