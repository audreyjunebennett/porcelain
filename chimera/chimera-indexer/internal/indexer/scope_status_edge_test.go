package indexer

import "testing"

func TestQueuePendingChangeReason_zeroToOne(t *testing.T) {
	if r := queuePendingChangeReason("queue_ingest_pending", 0, 1, 100); r != "queue_ingest_pending" {
		t.Fatalf("got %q", r)
	}
}

func TestQueuePendingChangeReason_noChange(t *testing.T) {
	if r := queuePendingChangeReason("queue_ingest_pending", 5, 5, 100); r != "" {
		t.Fatalf("got %q", r)
	}
}

func TestQueuePendingChangeReason_smallDelta(t *testing.T) {
	if r := queuePendingChangeReason("pending_bulk_tier1", 1, 2, 658); r != "" {
		t.Fatalf("small delta should not emit: got %q", r)
	}
}

func TestQueuePendingChangeReason_percentBucket(t *testing.T) {
	if r := queuePendingChangeReason("queue_ingest_pending", 5, 16, 100); r != "queue_ingest_pending" {
		t.Fatalf("crossed 10%% bucket: got %q", r)
	}
}

func TestScopePhaseEdgeBucket_uploadingVsBacklog(t *testing.T) {
	if scopePhaseEdgeBucket("uploading", 10, 1, true) != "draining" {
		t.Fatal("uploading with work should be draining")
	}
	if scopePhaseEdgeBucket("backlog", 10, 0, true) != "draining" {
		t.Fatal("backlog should be draining")
	}
	if scopePhaseEdgeBucket("watch_idle", 0, 0, true) != "watch_idle" {
		t.Fatal("idle queue should be watch_idle")
	}
}

func TestGlobalScopeStatusChangeReason_phaseMicroTransition(t *testing.T) {
	last := scopeStatusGlobals{Phase: "backlog", PhaseEdgeBucket: "draining"}
	cur := scopeStatusGlobals{Phase: "uploading", PhaseEdgeBucket: "draining"}
	if r := globalScopeStatusChangeReason(last, cur, 25); r != "" {
		t.Fatalf("micro phase flicker should not emit: got %q", r)
	}
}

func TestGlobalScopeStatusChangeReason_drainComplete(t *testing.T) {
	last := scopeStatusGlobals{PhaseEdgeBucket: "draining"}
	cur := scopeStatusGlobals{PhaseEdgeBucket: "watch_idle"}
	if r := globalScopeStatusChangeReason(last, cur, 25); r != "phase" {
		t.Fatalf("got %q", r)
	}
}

func TestGlobalScopeStatusChangeReason_ingestGate(t *testing.T) {
	last := scopeStatusGlobals{IngestGateClosed: false}
	cur := scopeStatusGlobals{IngestGateClosed: true, IngestGateReason: "embed_provider_down"}
	if r := globalScopeStatusChangeReason(last, cur, 25); r != "ingest_gate" {
		t.Fatalf("got %q", r)
	}
}
