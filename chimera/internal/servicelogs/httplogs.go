package servicelogs

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
)

// ReplayMode controls SSE history sent before live subscription.
type ReplayMode string

const (
	ReplayBuffer ReplayMode = "buffer" // full retained ring (oldest first)
	ReplayTail   ReplayMode = "tail"   // last N lines only
	ReplayNone   ReplayMode = "none"   // live only
)

const defaultReplayTailN = 200

// PollResponse is the JSON body for GET /logs.
type PollResponse struct {
	Lines         []Entry `json:"lines"`
	MaxSeq        uint64  `json:"max_seq"`
	BufferMinSeq  uint64  `json:"buffer_min_seq,omitempty"`
	HasOlderInBuf *bool   `json:"has_older_in_buffer,omitempty"`
}

// ParseReplayMode reads ?replay=buffer|tail|none (default buffer).
func ParseReplayMode(raw string) ReplayMode {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "tail":
		return ReplayTail
	case "none":
		return ReplayNone
	default:
		return ReplayBuffer
	}
}

// RegisterLogRoutes mounts loopback-only log poll and SSE on mux.
func RegisterLogRoutes(mux *http.ServeMux, store *Store) {
	if store == nil {
		return
	}
	mux.HandleFunc("GET /logs", loopbackOnly(func(w http.ResponseWriter, r *http.Request) {
		HandleLogsPoll(store, w, r)
	}))
	mux.HandleFunc("GET /logs/stream", loopbackOnly(func(w http.ResponseWriter, r *http.Request) {
		HandleLogsStream(store, w, r)
	}))
}

func loopbackOnly(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !requestFromLoopback(r) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		next(w, r)
	}
}

func requestFromLoopback(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	host = strings.Trim(host, "[]")
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// HandleLogsPoll serves GET /logs poll semantics (since, before_seq, limit).
func HandleLogsPoll(store *Store, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	hasSince := r.URL.Query().Get("since") != ""
	hasBefore := r.URL.Query().Get("before_seq") != ""
	if hasSince && hasBefore {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": "use either since or before_seq, not both"})
		return
	}

	var limit int
	if ls := r.URL.Query().Get("limit"); ls != "" {
		v, err := strconv.Atoi(ls)
		if err != nil || v < 0 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": "invalid limit"})
			return
		}
		limit = v
		if limit > DefaultMaxLines {
			limit = DefaultMaxLines
		}
	}

	bufMin := store.MinSeq()
	resp := PollResponse{BufferMinSeq: bufMin}

	if hasBefore {
		bs := r.URL.Query().Get("before_seq")
		beforeSeq, err := strconv.ParseUint(bs, 10, 64)
		if err != nil || beforeSeq <= 1 {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": "invalid before_seq"})
			return
		}
		if limit <= 0 {
			limit = 300
		}
		lines := store.EntriesBefore(beforeSeq, limit)
		resp.Lines = lines
		_, resp.MaxSeq = store.EntriesAfter(0)
		if len(lines) > 0 {
			v := lines[0].Seq > bufMin
			resp.HasOlderInBuf = &v
		}
		writePollJSON(w, resp)
		return
	}

	var since uint64
	if s := r.URL.Query().Get("since"); s != "" {
		var err error
		since, err = strconv.ParseUint(s, 10, 64)
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{"error": "invalid since"})
			return
		}
	}
	lines, maxSeq := store.EntriesAfter(since)
	if limit > 0 && len(lines) > limit {
		lines = balancedTail(lines, since, limit)
	}
	resp.Lines = lines
	resp.MaxSeq = maxSeq
	if since == 0 && len(lines) > 0 {
		v := lines[0].Seq > bufMin
		resp.HasOlderInBuf = &v
	}
	writePollJSON(w, resp)
}

func writePollJSON(w http.ResponseWriter, resp PollResponse) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func balancedTail(lines []Entry, since uint64, limit int) []Entry {
	if since != 0 {
		if len(lines) > limit {
			return lines[len(lines)-limit:]
		}
		return lines
	}
	bySrc := map[string][]Entry{}
	for _, e := range lines {
		bySrc[e.Source] = append(bySrc[e.Source], e)
	}
	if len(bySrc) <= 1 {
		return lines[len(lines)-limit:]
	}
	full := lines
	idx := bySrc[SourceChimeraIndexer]
	other := make([]Entry, 0, len(lines))
	for s, sl := range bySrc {
		if s == SourceChimeraIndexer {
			continue
		}
		other = append(other, sl...)
	}
	sort.Slice(other, func(i, j int) bool { return other[i].Seq < other[j].Seq })
	sort.Slice(idx, func(i, j int) bool { return idx[i].Seq < idx[j].Seq })

	idxBudget := (limit + 1) / 2
	if idxBudget < 120 {
		idxBudget = 120
	}
	if idxBudget > limit-1 {
		idxBudget = limit - 1
	}
	otherBudget := limit - idxBudget
	if otherBudget < 1 {
		otherBudget = 1
		idxBudget = limit - otherBudget
	}

	cand := make([]Entry, 0, limit)
	if len(idx) > idxBudget {
		cand = append(cand, idx[len(idx)-idxBudget:]...)
	} else {
		cand = append(cand, idx...)
	}
	if len(other) > otherBudget {
		cand = append(cand, other[len(other)-otherBudget:]...)
	} else {
		cand = append(cand, other...)
	}
	if len(cand) < limit && len(full) >= limit {
		need := limit - len(cand)
		cand = append(cand, full[len(full)-need:]...)
	}
	sort.Slice(cand, func(i, j int) bool { return cand[i].Seq < cand[j].Seq })
	dedup := cand[:0]
	var prev uint64
	for i, e := range cand {
		if i > 0 && e.Seq == prev {
			continue
		}
		dedup = append(dedup, e)
		prev = e.Seq
	}
	cand = dedup
	if len(cand) > limit {
		cand = cand[len(cand)-limit:]
	}
	return cand
}

// HandleLogsStream serves GET /logs/stream SSE with ?replay=buffer|tail|none.
func HandleLogsStream(store *Store, w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	replay := ParseReplayMode(r.URL.Query().Get("replay"))
	tailN := defaultReplayTailN
	if tn := r.URL.Query().Get("tail"); tn != "" {
		if v, err := strconv.Atoi(tn); err == nil && v > 0 {
			tailN = v
		}
	}

	rc := http.NewResponseController(w)
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	flush := func() { _ = rc.Flush() }
	flush()

	writeSSE := func(e Entry) {
		b, err := json.Marshal(e)
		if err != nil {
			return
		}
		_, _ = fmt.Fprintf(w, "data: %s\n\n", b)
	}

	switch replay {
	case ReplayBuffer:
		for _, e := range store.Snapshot() {
			writeSSE(e)
		}
	case ReplayTail:
		for _, e := range store.Tail(tailN) {
			writeSSE(e)
		}
	case ReplayNone:
	}
	flush()

	ch, cancel := store.Subscribe(64)
	defer cancel()

	for {
		select {
		case <-r.Context().Done():
			return
		case e, ok := <-ch:
			if !ok {
				return
			}
			writeSSE(e)
			flush()
		}
	}
}
