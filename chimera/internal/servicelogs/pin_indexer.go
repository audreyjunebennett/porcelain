package servicelogs

import (
	"encoding/json"
	"strings"
)

const DefaultIndexerPinnedLinesMax = 64

func indexerPinKey(text string) (string, bool) {
	if !strings.Contains(text, "indexer.") {
		return "", false
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal([]byte(text), &raw); err != nil {
		return "", false
	}
	msg := jsonStringField(raw, "msg")
	if msg == "" {
		return "", false
	}
	runID := jsonStringField(raw, "index_run_id")
	itk := jsonStringField(raw, "indexer_target_key")
	switch msg {
	case "indexer.run.start":
		if runID == "" {
			return "", false
		}
		return "start:" + runID, true
	case "indexer.run.done":
		if runID == "" {
			return "", false
		}
		return "done:" + runID, true
	case "indexer.scope.status":
		if runID == "" || itk == "" {
			return "", false
		}
		return "scope:" + runID + ":" + itk, true
	case "indexer.ingest.gate.closed", "indexer.ingest.gate.open":
		if runID == "" {
			return "", false
		}
		return "gate:" + runID, true
	case "indexer.job.skipped.summary":
		if runID == "" || itk == "" {
			return "", false
		}
		return "skip:" + runID + ":" + itk, true
	case "indexer.job.ingested.summary":
		if runID == "" || itk == "" {
			return "", false
		}
		return "ingest:" + runID + ":" + itk, true
	default:
		return "", false
	}
}

func jsonStringField(raw map[string]json.RawMessage, key string) string {
	v, ok := raw[key]
	if !ok || len(v) == 0 {
		return ""
	}
	var s string
	if err := json.Unmarshal(v, &s); err != nil {
		return ""
	}
	return strings.TrimSpace(s)
}

func entrySeqPinned(seq uint64, pinSeq map[string]uint64) bool {
	for _, ps := range pinSeq {
		if ps == seq {
			return true
		}
	}
	return false
}

func countUnpinnedSource(lines []Entry, source string, pinSeq map[string]uint64) int {
	n := 0
	for _, e := range lines {
		if e.Source != source {
			continue
		}
		if entrySeqPinned(e.Seq, pinSeq) {
			continue
		}
		n++
	}
	return n
}

func removeFirstUnpinnedMatching(lines *[]Entry, source string, pinSeq map[string]uint64, pred func(string) bool) bool {
	sl := *lines
	for i, e := range sl {
		if e.Source != source || entrySeqPinned(e.Seq, pinSeq) || !pred(e.Text) {
			continue
		}
		*lines = append(sl[:i], sl[i+1:]...)
		return true
	}
	return false
}

func removeFirstUnpinnedWithSource(lines *[]Entry, source string, pinSeq map[string]uint64) bool {
	sl := *lines
	for i, e := range sl {
		if e.Source != source || entrySeqPinned(e.Seq, pinSeq) {
			continue
		}
		*lines = append(sl[:i], sl[i+1:]...)
		return true
	}
	return false
}

func removeEntryBySeq(lines *[]Entry, seq uint64) bool {
	sl := *lines
	for i, e := range sl {
		if e.Seq == seq {
			*lines = append(sl[:i], sl[i+1:]...)
			return true
		}
	}
	return false
}

func trimPinnedIndexerLines(lines *[]Entry, pinSeq map[string]uint64, maxPinned int) {
	if maxPinned < 1 || len(pinSeq) <= maxPinned {
		return
	}
	for len(pinSeq) > maxPinned {
		var oldestSeq uint64
		var oldestKey string
		for k, seq := range pinSeq {
			if oldestKey == "" || seq < oldestSeq {
				oldestKey = k
				oldestSeq = seq
			}
		}
		if oldestKey == "" {
			return
		}
		removeEntryBySeq(lines, oldestSeq)
		delete(pinSeq, oldestKey)
	}
}
