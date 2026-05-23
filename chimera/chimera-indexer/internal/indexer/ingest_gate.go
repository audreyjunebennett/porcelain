package indexer

import (
	"context"
	"sync"
	"time"
)

type ingestGate struct {
	mu         sync.Mutex
	cond       sync.Cond
	closed     bool
	reasonCode string
	detail     string
	embedModel string
}

func newIngestGate() *ingestGate {
	g := &ingestGate{}
	g.cond.L = &g.mu
	return g
}

func (g *ingestGate) isClosed() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.closed
}

func (g *ingestGate) close(reasonCode, detail string) (opened bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	wasOpen := !g.closed
	g.closed = true
	g.reasonCode = reasonCode
	g.detail = detail
	return wasOpen
}

func (g *ingestGate) open(embedModel string) (wasClosed bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	wasClosed = g.closed
	g.closed = false
	g.embedModel = embedModel
	if wasClosed {
		g.cond.Broadcast()
	}
	return wasClosed
}

func (g *ingestGate) waitOpen(ctx context.Context) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	for g.closed {
		if err := ctx.Err(); err != nil {
			return err
		}
		g.mu.Unlock()
		select {
		case <-ctx.Done():
			g.mu.Lock()
			return ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
		g.mu.Lock()
	}
	return nil
}

func embedModelFromHealth(h *HealthStatus) string {
	if h == nil || h.Checks == nil {
		return ""
	}
	return h.Checks.Embedding.Model
}

func (ix *Indexer) ingestGateClosed() bool {
	if ix.ingestGate == nil {
		return false
	}
	return ix.ingestGate.isClosed()
}

func (ix *Indexer) closeIngestGate(reasonCode, detail string) {
	if ix.ingestGate == nil {
		return
	}
	if reasonCode == "" {
		reasonCode = "ingest_not_ready"
	}
	if ix.ingestGate.close(reasonCode, detail) {
		ix.logIngestGateClosed(reasonCode, detail)
		ix.FlushSkipSummaries()
		ix.FlushIngestSummaries()
		ix.MaybeEmitScopeStatusEdge("ingest_gate")
	}
}

func (ix *Indexer) openIngestGate(embedModel string) {
	if ix.ingestGate == nil {
		return
	}
	if ix.ingestGate.open(embedModel) {
		ix.logIngestGateOpen(embedModel)
		ix.MaybeEmitScopeStatusEdge("ingest_gate")
	}
}

func (ix *Indexer) waitIngestGateOpen(ctx context.Context) error {
	if ix.ingestGate == nil {
		return nil
	}
	return ix.ingestGate.waitOpen(ctx)
}

func (ix *Indexer) closeIngestGateFromHealth(ctx context.Context) {
	h, err := ix.client.CheckHealth(ctx)
	if err != nil {
		ix.closeIngestGate("health_probe_failed", err.Error())
		return
	}
	if h == nil {
		ix.closeIngestGate("health_probe_failed", "empty health response")
		return
	}
	if h.IngestReady() {
		return
	}
	rc := h.ReasonCode()
	if rc == "" {
		rc = "ingest_not_ready"
	}
	ix.closeIngestGate(rc, h.HealthDetail())
}

func (ix *Indexer) syncIngestGateFromHealth(ctx context.Context, h *HealthStatus) {
	if h == nil {
		return
	}
	if h.IngestReady() {
		ix.openIngestGate(embedModelFromHealth(h))
		return
	}
	rc := h.ReasonCode()
	if rc == "" {
		rc = "ingest_not_ready"
	}
	ix.closeIngestGate(rc, h.HealthDetail())
}

func (ix *Indexer) logIngestGateClosed(reasonCode, detail string) {
	if ix.log == nil {
		return
	}
	args := []any{
		"msg", "indexer.ingest.gate.closed",
		"reason_code", reasonCode,
		"queue_depth", ix.queue.Len(),
	}
	if detail != "" {
		args = append(args, "detail", detail)
	}
	ix.log.Info("ingest gate closed", args...)
}

func (ix *Indexer) logIngestGateOpen(embedModel string) {
	if ix.log == nil {
		return
	}
	args := []any{
		"msg", "indexer.ingest.gate.open",
		"queue_depth", ix.queue.Len(),
	}
	if embedModel != "" {
		args = append(args, "embed_model", embedModel)
	}
	ix.log.Info("ingest gate open", args...)
}
