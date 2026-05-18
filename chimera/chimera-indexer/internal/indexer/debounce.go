package indexer

import (
	"sync"
	"time"
)

// debouncer collapses a burst of events on the same key into one delayed
// callback. It is safe for concurrent use. Tier carries the maximum priority
// seen for that key before the callback fires (coalesce upgrades).
type debouncer struct {
	mu          sync.Mutex
	timers      map[string]*time.Timer
	pendingTier map[string]PriorityTier
	delay       time.Duration
	fn          func(string, PriorityTier)
	closed      bool
}

func newDebouncer(delay time.Duration, fn func(string, PriorityTier)) *debouncer {
	return &debouncer{
		timers:      map[string]*time.Timer{},
		pendingTier: map[string]PriorityTier{},
		delay:       delay,
		fn:          fn,
	}
}

func maxPriorityTier(a, b PriorityTier) PriorityTier {
	if a > b {
		return a
	}
	return b
}

// Trigger schedules fn(key, tier) after the configured delay. If Trigger is
// called again with the same key before fn fires, the timer is reset and tier
// is the max of prior and new.
func (d *debouncer) Trigger(key string, tier PriorityTier) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return
	}
	if prev, ok := d.pendingTier[key]; ok {
		tier = maxPriorityTier(prev, tier)
	}
	d.pendingTier[key] = tier
	if t, ok := d.timers[key]; ok {
		t.Stop()
	}
	d.timers[key] = time.AfterFunc(d.delay, func() {
		d.mu.Lock()
		delete(d.timers, key)
		tier := d.pendingTier[key]
		delete(d.pendingTier, key)
		d.mu.Unlock()
		d.fn(key, tier)
	})
}

// Close stops every pending timer and prevents future triggers.
func (d *debouncer) Close() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.closed = true
	for _, t := range d.timers {
		t.Stop()
	}
	d.timers = nil
	d.pendingTier = nil
}
