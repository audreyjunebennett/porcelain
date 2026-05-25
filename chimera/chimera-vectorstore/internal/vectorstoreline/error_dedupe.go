package vectorstoreline

import (
	"encoding/json"
	"strings"
	"sync"
	"time"

	wline "github.com/lynn/porcelain/chimera/internal/wrapper/line"
)

const defaultErrorDedupeWindow = 30 * time.Second

var errorDedupeNow = time.Now

type errorDedupeTracker struct {
	mu     sync.Mutex
	window time.Duration
	last   map[string]time.Time
}

var (
	errorDedupeOnce sync.Once
	errorDedupe     *errorDedupeTracker
)

func globalErrorDedupe() *errorDedupeTracker {
	errorDedupeOnce.Do(func() {
		errorDedupe = &errorDedupeTracker{
			window: defaultErrorDedupeWindow,
			last:   map[string]time.Time{},
		}
	})
	return errorDedupe
}

func resetErrorDedupeForTest() {
	t := globalErrorDedupe()
	t.mu.Lock()
	defer t.mu.Unlock()
	t.last = map[string]time.Time{}
}

func (t *errorDedupeTracker) allowOperator(key string, now time.Time) bool {
	if t == nil || key == "" {
		return true
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if last, ok := t.last[key]; ok && now.Sub(last) < t.window {
		return false
	}
	t.last[key] = now
	return true
}

func postProcessErrorStormLine(b []byte) []byte {
	if len(b) == 0 {
		return b
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(b, &fields); err != nil {
		return b
	}
	slug := strings.TrimSpace(wline.JSONString(fields, "msg"))
	switch slug {
	case "vectorstore.runtime.panic", "vectorstore.trace.other":
	default:
		return b
	}

	detail := strings.TrimSpace(wline.JSONString(fields, "progress_detail"))
	if detail == "" {
		return b
	}

	if slug == "vectorstore.runtime.panic" && strings.Contains(detail, "Panic backtrace") {
		return demoteLineLevel(b, fields, "DEBUG")
	}

	root := incidentRootCause(detail)
	if root == "" {
		return b
	}

	coll := strings.TrimSpace(wline.JSONString(fields, "collection"))
	target := strings.TrimSpace(wline.JSONString(fields, "qdrant_target"))
	key := incidentDedupeKey(coll, target, root)
	if !globalErrorDedupe().allowOperator(key, errorDedupeNow()) {
		return demoteLineLevel(b, fields, "DEBUG")
	}

	if slug == "vectorstore.runtime.panic" && strings.Contains(detail, "Panic occurred") {
		return setProgressDetail(b, compactPanicOccurredDetail(detail))
	}
	return b
}

func incidentDedupeKey(collection, target, root string) string {
	return collection + "\x00" + target + "\x00" + root
}

func incidentRootCause(detail string) string {
	d := strings.TrimSpace(detail)
	if d == "" {
		return ""
	}
	const unstablePrefix = "Optimization task panicked, collection may be in unstable state: "
	if strings.HasPrefix(d, unstablePrefix) {
		d = strings.TrimSpace(d[len(unstablePrefix):])
	}
	const panicPrefix = "Panic occurred in file "
	if strings.HasPrefix(d, panicPrefix) {
		if i := strings.LastIndex(d, ": "); i >= 0 {
			d = strings.TrimSpace(d[i+2:])
		}
	}
	return wline.TrimRunes(d, 256)
}

func compactPanicOccurredDetail(detail string) string {
	const prefix = "Panic occurred in file "
	if !strings.HasPrefix(detail, prefix) {
		return wline.TrimRunes(detail, 512)
	}
	rest := strings.TrimPrefix(detail, prefix)
	fileLine, root := rest, ""
	if i := strings.LastIndex(rest, ": "); i >= 0 {
		fileLine = strings.TrimSpace(rest[:i])
		root = strings.TrimSpace(rest[i+2:])
	}
	fileLine = strings.ReplaceAll(fileLine, "\\", "/")
	if j := strings.LastIndex(fileLine, "/"); j >= 0 {
		fileLine = fileLine[j+1:]
	}
	if root != "" {
		return wline.TrimRunes(fileLine+": "+root, 512)
	}
	return wline.TrimRunes(fileLine, 512)
}

func setProgressDetail(b []byte, detail string) []byte {
	var m map[string]any
	if json.Unmarshal(b, &m) != nil {
		return b
	}
	m["progress_detail"] = detail
	out, err := json.Marshal(m)
	if err != nil {
		return b
	}
	return wline.ReorderNormalizedJSONBytes(out)
}
