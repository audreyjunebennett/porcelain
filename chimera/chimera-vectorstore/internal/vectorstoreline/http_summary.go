package vectorstoreline

import (
	"encoding/json"
	"sort"
	"strings"
	"sync"
	"time"

	wline "github.com/lynn/porcelain/chimera/internal/wrapper/line"
	"github.com/lynn/porcelain/internal/naming"
)

const defaultHTTPSummaryMinInterval = 5 * time.Second

type httpSummaryWindow struct {
	collections map[string]struct{}
	upsertsOK   int64
	deleteOK    int64
	searchOK    int64
	windowStart time.Time
	lastEmit    time.Time
}

func (w *httpSummaryWindow) hasActivity() bool {
	return w != nil && (w.upsertsOK > 0 || w.deleteOK > 0 || w.searchOK > 0)
}

func (w *httpSummaryWindow) resetAfterEmit(now time.Time) {
	if w == nil {
		return
	}
	w.collections = map[string]struct{}{}
	w.upsertsOK = 0
	w.deleteOK = 0
	w.searchOK = 0
	w.windowStart = now
	w.lastEmit = now
}

type httpSummaryTracker struct {
	mu          sync.Mutex
	window      *httpSummaryWindow
	dsts        []ioWriterRef
	minInterval time.Duration
}

type ioWriterRef struct {
	mu sync.Mutex
	w  interface {
		Write([]byte) (int, error)
	}
}

var (
	httpSummaryOnce sync.Once
	httpSummary     *httpSummaryTracker
)

func globalHTTPSummary() *httpSummaryTracker {
	httpSummaryOnce.Do(func() {
		httpSummary = &httpSummaryTracker{
			window:      &httpSummaryWindow{collections: map[string]struct{}{}, windowStart: time.Now().UTC()},
			minInterval: defaultHTTPSummaryMinInterval,
		}
		go httpSummary.runLoop()
	})
	return httpSummary
}

func registerHTTPSummaryDst(dst interface {
	Write([]byte) (int, error)
}) {
	if dst == nil {
		return
	}
	t := globalHTTPSummary()
	t.mu.Lock()
	t.dsts = append(t.dsts, ioWriterRef{w: dst})
	t.mu.Unlock()
}

func (t *httpSummaryTracker) note(collection, slug string, httpStatus int) {
	if t == nil || httpStatus != 200 {
		return
	}
	coll := strings.TrimSpace(collection)
	switch slug {
	case "vectorstore.http.points_upsert_ok":
	case "vectorstore.http.points_delete":
	case "vectorstore.http.vector_search":
	default:
		return
	}

	now := time.Now().UTC()
	t.mu.Lock()
	defer t.mu.Unlock()
	w := t.window
	if w.collections == nil {
		w.collections = map[string]struct{}{}
	}
	if w.windowStart.IsZero() {
		w.windowStart = now
	}
	if coll != "" {
		w.collections[coll] = struct{}{}
	}
	switch slug {
	case "vectorstore.http.points_upsert_ok":
		w.upsertsOK++
	case "vectorstore.http.points_delete":
		w.deleteOK++
	case "vectorstore.http.vector_search":
		w.searchOK++
	}
}

func (t *httpSummaryTracker) runLoop() {
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for range tick.C {
		t.emitDue(false)
	}
}

func (t *httpSummaryTracker) emitDue(force bool) {
	if t == nil {
		return
	}
	now := time.Now().UTC()
	minGap := t.minInterval
	if minGap <= 0 {
		minGap = defaultHTTPSummaryMinInterval
	}

	var emitLine []byte
	var dsts []ioWriterRef

	t.mu.Lock()
	w := t.window
	if w != nil && w.hasActivity() {
		if force || now.Sub(w.windowStart) >= minGap {
			windowMs := int64(0)
			if !w.windowStart.IsZero() {
				windowMs = now.Sub(w.windowStart).Milliseconds()
			}
			emitLine = marshalHTTPSummaryLine(w, windowMs, now)
			w.resetAfterEmit(now)
		}
	}
	if len(emitLine) > 0 {
		dsts = append([]ioWriterRef(nil), t.dsts...)
	}
	t.mu.Unlock()

	if len(emitLine) == 0 {
		return
	}
	line := append(emitLine, '\n')
	for i := range dsts {
		ref := &dsts[i]
		ref.mu.Lock()
		_, _ = ref.w.Write(line)
		ref.mu.Unlock()
	}
}

func marshalHTTPSummaryLine(w *httpSummaryWindow, windowMs int64, now time.Time) []byte {
	colls := make([]string, 0, len(w.collections))
	for c := range w.collections {
		colls = append(colls, c)
	}
	sort.Strings(colls)
	collections := strings.Join(colls, ",")

	out := map[string]any{
		"timestamp":     wline.NormalizeTimestampUTC(now.Format(time.RFC3339)),
		"level":         "INFO",
		"service":       naming.ProductVectorstoreName,
		"msg":           "vectorstore.http.upsert.summary",
		"collections":   collections,
		"upserts_ok":    w.upsertsOK,
		"deletes_ok":    w.deleteOK,
		"searches_ok":   w.searchOK,
		"window_ms":     windowMs,
		"_chimera_norm": 1,
	}
	if len(colls) == 1 {
		out["collection"] = colls[0]
	}
	b, _ := json.Marshal(out)
	return wline.ReorderNormalizedJSONBytes(b)
}

func postProcessNormalizedLine(b []byte) []byte {
	if len(b) == 0 {
		return b
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(b, &fields); err != nil {
		return b
	}
	slug := strings.TrimSpace(wline.JSONString(fields, "msg"))
	if slug == "vectorstore.http.upsert.summary" {
		return b
	}
	status := wline.IntFromJSON(fields, "http_status")
	coll := wline.JSONString(fields, "collection")

	switch slug {
	case "vectorstore.http.points_upsert_ok",
		"vectorstore.http.points_delete",
		"vectorstore.http.vector_search":
		if status == 200 {
			globalHTTPSummary().note(coll, slug, status)
			return postProcessErrorStormLine(postProcessStartupChatterLine(demoteLineLevel(b, fields, "DEBUG")))
		}
	case "vectorstore.http.collection_meta":
		// Gateway probes collection existence before every ingest; per-file meta lines
		// are trace-only — upsert.summary and collection_create cover operator visibility.
		if status == 200 || status == 404 {
			return postProcessErrorStormLine(postProcessStartupChatterLine(demoteLineLevel(b, fields, "DEBUG")))
		}
	}
	return postProcessErrorStormLine(postProcessStartupChatterLine(b))
}

func demoteLineLevel(b []byte, fields map[string]json.RawMessage, level string) []byte {
	if fields == nil {
		var m map[string]any
		if json.Unmarshal(b, &m) != nil {
			return b
		}
		m["level"] = level
		out, err := json.Marshal(m)
		if err != nil {
			return b
		}
		return wline.ReorderNormalizedJSONBytes(out)
	}
	var m map[string]any
	if json.Unmarshal(b, &m) != nil {
		return b
	}
	m["level"] = level
	out, err := json.Marshal(m)
	if err != nil {
		return b
	}
	return wline.ReorderNormalizedJSONBytes(out)
}
