package indexer

import (
	"context"
	"errors"
	"math/rand"
	"sync"
	"time"
)

// tierSlot tracks where a pending item lives for global dedup / tier upgrades.
type tierSlot struct {
	tier PriorityTier
	idx  int
}

// Queue is a bounded priority queue: dequeue prefers TierInteractive, then
// TierWrite, then TierBulk. Ingest jobs dedupe by Job.Key(); enqueueing a
// duplicate at a higher tier replaces the lower-tier pending entry.
type Queue struct {
	mu   sync.Mutex
	cond *sync.Cond

	tier3 []WorkItem // interactive (create/delete)
	tier2 []WorkItem // write
	tier1 []WorkItem // bulk

	pending map[string]tierSlot

	cap    int
	closed bool
}

// NewQueue creates a queue with the given capacity. Capacity <= 0 means
// unbounded (still recommended to set a value).
func NewQueue(capacity int) *Queue {
	q := &Queue{cap: capacity, pending: map[string]tierSlot{}}
	q.cond = sync.NewCond(&q.mu)
	return q
}

func (q *Queue) totalLenLocked() int {
	return len(q.tier3) + len(q.tier2) + len(q.tier1)
}

func tierSlice(q *Queue, tier PriorityTier) *[]WorkItem {
	switch tier {
	case TierInteractive:
		return &q.tier3
	case TierWrite:
		return &q.tier2
	case TierBulk:
		return &q.tier1
	default:
		return &q.tier1
	}
}

// removeAt removes work item at (tier, idx), fixing pending indices after swap-with-last.
func (q *Queue) removeAtLocked(tier PriorityTier, idx int) WorkItem {
	s := tierSlice(q, tier)
	n := len(*s)
	if idx < 0 || idx >= n {
		return WorkItem{}
	}
	w := (*s)[idx]
	last := n - 1
	(*s)[idx] = (*s)[last]
	*s = (*s)[:last]
	delete(q.pending, w.Key())
	if idx != last {
		moved := (*s)[idx]
		k := moved.Key()
		if loc, ok := q.pending[k]; ok {
			loc.idx = idx
			q.pending[k] = loc
		}
	}
	return w
}

// Enqueue adds work. Ingest keys dedupe; higher tier replaces lower. Returns false if full.
func (q *Queue) Enqueue(w WorkItem) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.closed {
		return false
	}
	key := w.Key()
	if key == "" {
		return false
	}

	if w.Kind != WorkIngest {
		if _, ok := q.pending[key]; ok {
			return true
		}
	}

	if w.Kind == WorkIngest {
		if loc, ok := q.pending[key]; ok {
			if w.Tier <= loc.tier {
				return true // coalesce duplicate at same or lower priority
			}
			q.removeAtLocked(loc.tier, loc.idx)
		}
	} else {
		if _, ok := q.pending[key]; ok {
			return true // meta job already pending (should not happen with unique ids)
		}
	}

	if q.cap > 0 && q.totalLenLocked() >= q.cap {
		return false
	}

	s := tierSlice(q, w.Tier)
	idx := len(*s)
	*s = append(*s, w)
	q.pending[key] = tierSlot{tier: w.Tier, idx: idx}
	q.cond.Signal()
	return true
}

// Dequeue blocks until work is available or the queue is closed.
func (q *Queue) Dequeue(ctx context.Context) (WorkItem, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	for q.totalLenLocked() == 0 && !q.closed {
		done := make(chan struct{})
		go func() {
			select {
			case <-ctx.Done():
				q.cond.Broadcast()
			case <-done:
			}
		}()
		q.cond.Wait()
		close(done)
		if ctx.Err() != nil {
			return WorkItem{}, false
		}
	}
	if q.totalLenLocked() == 0 {
		return WorkItem{}, false
	}
	var tier PriorityTier
	var s *[]WorkItem
	switch {
	case len(q.tier3) > 0:
		tier = TierInteractive
		s = &q.tier3
	case len(q.tier2) > 0:
		tier = TierWrite
		s = &q.tier2
	default:
		tier = TierBulk
		s = &q.tier1
	}
	w := (*s)[0]
	copy((*s)[0:], (*s)[1:])
	*s = (*s)[:len(*s)-1]
	delete(q.pending, w.Key())
	// Fix indices for the tier we popped from (all shifted down by 1).
	q.reindexTierLocked(tier)
	return w, true
}

func (q *Queue) reindexTierLocked(tier PriorityTier) {
	s := tierSlice(q, tier)
	for i := range *s {
		k := (*s)[i].Key()
		if loc, ok := q.pending[k]; ok {
			loc.idx = i
			q.pending[k] = loc
		}
	}
}

// Len returns the total queued items across tiers.
func (q *Queue) Len() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.totalLenLocked()
}

// LenByTier returns per-tier queue depths for observability.
func (q *Queue) LenByTier() (bulk, write, interactive int) {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.tier1), len(q.tier2), len(q.tier3)
}

// HasPendingKey reports whether key is pending in any tier (mostly for ingest dedup keys).
func (q *Queue) HasPendingKey(key string) bool {
	if key == "" {
		return false
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	_, ok := q.pending[key]
	return ok
}

// TallyScopeQueues counts queued WorkIngest jobs and FanoutList candidate rows by ScopeKey(project, flavor).
func (q *Queue) TallyScopeQueues(resolved Resolved) (ingestByScope map[string]int64, fanoutRowsByScope map[string]int64) {
	q.mu.Lock()
	defer q.mu.Unlock()
	ingestByScope = map[string]int64{}
	fanoutRowsByScope = map[string]int64{}
	walk := func(items []WorkItem) {
		for _, w := range items {
			switch w.Kind {
			case WorkIngest:
				p, f := resolved.IngestHeaders(w.Job.Root, w.Job.RelPath)
				sk := ScopeKey(p, f)
				ingestByScope[sk]++
			case WorkFanoutList:
				for _, tc := range w.Candidates {
					sk := ScopeKey(tc.Project, tc.Flavor)
					fanoutRowsByScope[sk]++
				}
			}
		}
	}
	walk(q.tier3)
	walk(q.tier2)
	walk(q.tier1)
	return ingestByScope, fanoutRowsByScope
}

// Cap returns the configured capacity (0 means unbounded).
func (q *Queue) Cap() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return q.cap
}

// Close wakes every blocked Dequeue caller; subsequent Enqueue calls fail.
func (q *Queue) Close() {
	q.mu.Lock()
	q.closed = true
	q.cond.Broadcast()
	q.mu.Unlock()
}

// Backoff computes the nth retry delay as base * 2^attempt, capped at max,
// with full-jitter randomization. attempt is 0-based.
func Backoff(attempt int, base, max time.Duration, rng *rand.Rand) time.Duration {
	if attempt < 0 {
		attempt = 0
	}
	d := base
	for i := 0; i < attempt && d < max; i++ {
		d *= 2
	}
	if d > max {
		d = max
	}
	if rng != nil {
		d = time.Duration(rng.Int63n(int64(d) + 1))
	}
	return d
}

// ErrPaused is returned by RunWithBackoff when the queue worker stops
// retrying and signals that the supervisor should pause and poll health.
var ErrPaused = errors.New("indexer: paused after exhausted retries")
