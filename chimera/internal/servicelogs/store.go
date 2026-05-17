// Package servicelogs captures line-oriented process and gateway output for the operator UI.
package servicelogs

import (
	"bytes"
	"fmt"
	"io"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// DefaultMaxLines is the default ring buffer capacity (oldest dropped).
// Keep operator UI responsive: large snapshots block the WebView when rendering history.
// The logs UI loads a small tail first and backfills older entries on demand; total
// ring size stays bounded by DefaultMaxLines.
const DefaultMaxLines = 5000

// MaxIndexerLinesPerSource caps lines attributed to the indexer subprocess so verbose
// per-file logs cannot crowd out gateway, broker, vectorstore, and chat traffic.
const MaxIndexerLinesPerSource = DefaultMaxLines / 4

const sourceIndexer = SourceChimeraIndexer

// mirrorTimeLayout is RFC3339-like UTC with a fixed six-digit fractional second so
// mirrored disk lines stay column-aligned; sub-microsecond precision is truncated.
const mirrorTimeLayout = "2006-01-02T15:04:05.000000Z07:00"

func formatMirrorTimestamp(ts time.Time) string {
	return ts.UTC().Truncate(time.Microsecond).Format(mirrorTimeLayout)
}

// Entry is one logical log line with a stable sequence number for polling.
type Entry struct {
	Seq    uint64    `json:"seq"`
	Source string    `json:"source"`
	Text   string    `json:"text"`
	Time   time.Time `json:"ts"`
}

// Store is a bounded, thread-safe log buffer with optional SSE subscribers.
type Store struct {
	mu       sync.Mutex
	maxLines int
	lines    []Entry
	lastSeq  uint64

	mirrorMu sync.Mutex
	mirror   io.Writer // optional append-only sink (e.g. desktop disk log)

	subsMu sync.Mutex
	subs   map[uint64]chan Entry
	subID  uint64
}

// New creates a Store that retains at most maxLines entries.
func New(maxLines int) *Store {
	if maxLines < 1 {
		maxLines = DefaultMaxLines
	}
	return &Store{
		maxLines: maxLines,
		subs:     make(map[uint64]chan Entry),
	}
}

// Writer returns an io.Writer that splits on '\n' and records complete lines under source.
func (s *Store) Writer(source string) io.Writer {
	return &lineWriter{store: s, source: source}
}

func (s *Store) add(source, text string) {
	text = string(bytes.TrimSuffix([]byte(text), []byte{'\r'}))
	if text == "" {
		return
	}
	now := time.Now().UTC()
	seq := atomic.AddUint64(&s.lastSeq, 1)
	ent := Entry{Seq: seq, Source: source, Text: text, Time: now}

	s.mu.Lock()
	s.lines = append(s.lines, ent)
	idxCap := MaxIndexerLinesPerSource
	if s.maxLines < DefaultMaxLines {
		idxCap = s.maxLines / 4
		if idxCap < 1 {
			idxCap = 1
		}
	}
	if idxCap > s.maxLines {
		idxCap = s.maxLines
	}
	trimSourceToMax(&s.lines, sourceIndexer, idxCap)
	if len(s.lines) > s.maxLines {
		overflow := len(s.lines) - s.maxLines
		s.lines = append([]Entry(nil), s.lines[overflow:]...)
	}
	s.mu.Unlock()

	s.broadcast(ent)
	s.writeMirror(source, text, ent.Time)
}

// SetMirror directs every completed log line to w in addition to the ring buffer.
// Pass nil to disable.
func (s *Store) SetMirror(w io.Writer) {
	s.mirrorMu.Lock()
	s.mirror = w
	s.mirrorMu.Unlock()
}

func (s *Store) writeMirror(source, text string, ts time.Time) {
	s.mirrorMu.Lock()
	w := s.mirror
	s.mirrorMu.Unlock()
	if w == nil {
		return
	}
	t := strings.ReplaceAll(text, "\t", " ")
	t = strings.ReplaceAll(t, "\n", "\\n")
	_, _ = fmt.Fprintf(w, "%s\t%s\t%s\n", formatMirrorTimestamp(ts), source, t)
}

func (s *Store) broadcast(ent Entry) {
	s.subsMu.Lock()
	defer s.subsMu.Unlock()
	for _, ch := range s.subs {
		select {
		case ch <- ent:
		default:
			// Slow consumer: drop, never block logging path.
		}
	}
}

// Snapshot returns a copy of all buffered lines (oldest first).
func (s *Store) Snapshot() []Entry {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Entry, len(s.lines))
	copy(out, s.lines)
	return out
}

func isNoisyIndexerLine(text string) bool {
	return strings.Contains(text, "indexer queue snapshot") ||
		strings.Contains(text, "indexer state") ||
		strings.Contains(text, "indexer storage stats")
}

func removeFirstSourceLineMatching(lines *[]Entry, source string, pred func(string) bool) bool {
	sl := *lines
	for i, e := range sl {
		if e.Source != source || !pred(e.Text) {
			continue
		}
		*lines = append(sl[:i], sl[i+1:]...)
		return true
	}
	return false
}

func trimSourceToMax(lines *[]Entry, source string, max int) {
	if max < 1 {
		return
	}
	for countEntriesWithSource(*lines, source) > max {
		if source == sourceIndexer && removeFirstSourceLineMatching(lines, source, isNoisyIndexerLine) {
			continue
		}
		if !removeFirstWithSource(lines, source) {
			break
		}
	}
}

func countEntriesWithSource(lines []Entry, source string) int {
	n := 0
	for _, e := range lines {
		if e.Source == source {
			n++
		}
	}
	return n
}

func removeFirstWithSource(lines *[]Entry, source string) bool {
	sl := *lines
	for i, e := range sl {
		if e.Source == source {
			*lines = append(sl[:i], sl[i+1:]...)
			return true
		}
	}
	return false
}

// MinSeq returns the lowest Seq still in the ring buffer, or 0 if empty.
func (s *Store) MinSeq() uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.lines) == 0 {
		return 0
	}
	return s.lines[0].Seq
}

// EntriesBefore returns up to limit entries with Seq strictly less than beforeSeq.
func (s *Store) EntriesBefore(beforeSeq uint64, limit int) []Entry {
	if beforeSeq <= 1 || limit <= 0 {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var buf []Entry
	for _, e := range s.lines {
		if e.Seq > 0 && e.Seq < beforeSeq {
			buf = append(buf, e)
		}
	}
	slices.SortFunc(buf, func(a, b Entry) int {
		if a.Seq < b.Seq {
			return -1
		}
		if a.Seq > b.Seq {
			return 1
		}
		return 0
	})
	if len(buf) > limit {
		buf = buf[len(buf)-limit:]
	}
	return buf
}

// EntriesAfter returns entries with Seq > afterSeq, and the highest Seq in the buffer.
func (s *Store) EntriesAfter(afterSeq uint64) (entries []Entry, maxSeq uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	maxSeq = afterSeq
	for _, e := range s.lines {
		if e.Seq > maxSeq {
			maxSeq = e.Seq
		}
		if e.Seq > afterSeq {
			entries = append(entries, e)
		}
	}
	return entries, maxSeq
}

// Tail returns the last n entries (n <= 0 means all).
func (s *Store) Tail(n int) []Entry {
	s.mu.Lock()
	defer s.mu.Unlock()
	if n <= 0 || n >= len(s.lines) {
		out := make([]Entry, len(s.lines))
		copy(out, s.lines)
		return out
	}
	return append([]Entry(nil), s.lines[len(s.lines)-n:]...)
}

// Import inserts entries with preserved Seq, Source, Text, and Time (for supervisor fan-in).
// Duplicate seq values are skipped. Broadcasts each newly inserted entry to subscribers.
func (s *Store) Import(entries []Entry) {
	if len(entries) == 0 {
		return
	}
	sorted := append([]Entry(nil), entries...)
	slices.SortFunc(sorted, func(a, b Entry) int {
		if a.Seq < b.Seq {
			return -1
		}
		if a.Seq > b.Seq {
			return 1
		}
		return 0
	})

	s.mu.Lock()
	existing := make(map[uint64]struct{}, len(s.lines))
	for _, e := range s.lines {
		if e.Seq > 0 {
			existing[e.Seq] = struct{}{}
		}
	}
	var added []Entry
	for _, e := range sorted {
		if e.Seq == 0 || e.Text == "" {
			continue
		}
		if _, dup := existing[e.Seq]; dup {
			continue
		}
		existing[e.Seq] = struct{}{}
		if e.Seq > s.lastSeq {
			s.lastSeq = e.Seq
		}
		ent := Entry{
			Seq:    e.Seq,
			Source: e.Source,
			Text:   string(bytes.TrimSuffix([]byte(e.Text), []byte{'\r'})),
			Time:   e.Time,
		}
		if ent.Time.IsZero() {
			ent.Time = time.Now().UTC()
		}
		s.lines = append(s.lines, ent)
		added = append(added, ent)
	}
	idxCap := MaxIndexerLinesPerSource
	if s.maxLines < DefaultMaxLines {
		idxCap = s.maxLines / 4
		if idxCap < 1 {
			idxCap = 1
		}
	}
	if idxCap > s.maxLines {
		idxCap = s.maxLines
	}
	trimSourceToMax(&s.lines, sourceIndexer, idxCap)
	if len(s.lines) > s.maxLines {
		overflow := len(s.lines) - s.maxLines
		s.lines = append([]Entry(nil), s.lines[overflow:]...)
	}
	s.mu.Unlock()

	for _, ent := range added {
		s.broadcast(ent)
		s.writeMirror(ent.Source, ent.Text, ent.Time)
	}
}

// Subscribe registers a consumer for new entries after this call.
func (s *Store) Subscribe(buf int) (ch <-chan Entry, cancel func()) {
	if buf < 1 {
		buf = 16
	}
	c := make(chan Entry, buf)
	id := atomic.AddUint64(&s.subID, 1)

	s.subsMu.Lock()
	s.subs[id] = c
	s.subsMu.Unlock()

	return c, func() {
		s.subsMu.Lock()
		if ch2, ok := s.subs[id]; ok {
			delete(s.subs, id)
			close(ch2)
		}
		s.subsMu.Unlock()
	}
}

type lineWriter struct {
	store  *Store
	source string
	buf    []byte
	mu     sync.Mutex
}

func (w *lineWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.buf = append(w.buf, p...)
	for {
		i := bytes.IndexByte(w.buf, '\n')
		if i < 0 {
			break
		}
		line := string(w.buf[:i])
		w.store.add(w.source, line)
		w.buf = append([]byte(nil), w.buf[i+1:]...)
	}
	const maxFrag = 64 << 10
	if len(w.buf) > maxFrag {
		w.store.add(w.source, string(w.buf))
		w.buf = w.buf[:0]
	}
	return len(p), nil
}
