package indexer

import (
	"fmt"
	"sync/atomic"
)

// WorkKind discriminates queue items processed by the indexer worker pool.
type WorkKind uint8

const (
	WorkIngest WorkKind = iota
	WorkScan
	WorkFanoutList
)

// PriorityTier controls dequeue order: higher tier runs before lower tier.
// TierBulk=1 (scan, fan-out, bulk ingest), TierWrite=2, TierInteractive=3.
type PriorityTier uint8

const (
	TierBulk  PriorityTier = 1
	TierWrite PriorityTier = 2
	// TierInteractive covers create/delete (and future remove jobs).
	TierInteractive PriorityTier = 3
)

// Job is a single-file ingest unit (same shape as historical indexer.Job).
type Job struct {
	Root    Root
	RelPath string
	AbsPath string
}

// Key returns the deduplication key for ingest jobs: root id + relative path.
func (j Job) Key() string { return j.Root.ID + "\x00" + j.RelPath }

// TaggedCandidate is a walk candidate plus resolved scope for fair-share and logs.
type TaggedCandidate struct {
	Candidate
	Project string
	Flavor  string
}

// ScopeKey returns a stable key for (project, flavor) bucketing.
func ScopeKey(project, flavor string) string {
	return project + "\x00" + flavor
}

// FanoutMeta carries scan-derived scheduling metadata for logging and fan-out.
type FanoutMeta struct {
	NScopes               int
	PerScopeFanoutBudget  int
	QueueFanoutHWMPercent int
}

var fanoutIDSeq atomic.Uint64

func nextFanoutID() string {
	return fmt.Sprintf("%d", fanoutIDSeq.Add(1))
}

// WorkItem is one unit of work for the priority queue.
type WorkItem struct {
	Kind WorkKind
	Tier PriorityTier

	// Ingest (WorkIngest)
	Job Job

	// Bulk fan-out accounting: set when Kind==WorkIngest and enqueued from FanoutListJob.
	FromFanout   bool
	BulkScopeKey string

	// Scan (WorkScan)
	ScanID string

	// Fan-out list (WorkFanoutList)
	FanoutID   string
	Candidates []TaggedCandidate
	Meta       FanoutMeta
}

// Key returns a deduplication key. Ingest uses Job.Key; meta-jobs use distinct prefixes.
func (w WorkItem) Key() string {
	switch w.Kind {
	case WorkIngest:
		return w.Job.Key()
	case WorkScan:
		return "scan\x00" + w.ScanID
	case WorkFanoutList:
		return "fanout\x00" + w.FanoutID
	default:
		return ""
	}
}

// IngestEnqueue builds a tiered ingest work item.
func IngestEnqueue(j Job, tier PriorityTier, fromFanout bool, bulkScopeKey string) WorkItem {
	return WorkItem{
		Kind:         WorkIngest,
		Tier:         tier,
		Job:          j,
		FromFanout:   fromFanout,
		BulkScopeKey: bulkScopeKey,
	}
}
