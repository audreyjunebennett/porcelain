package indexer

import (
	"context"
	"net/http"
	"testing"
	"time"
)

func TestIsEmbedClassifiedHTTPError(t *testing.T) {
	if !IsEmbedClassifiedHTTPError(&HTTPError{Status: http.StatusBadGateway, Body: "embed: down"}) {
		t.Fatal("502 should classify as embed")
	}
	if IsEmbedClassifiedHTTPError(&HTTPError{Status: http.StatusInternalServerError, Body: "busy"}) {
		t.Fatal("500 without embed hint should not classify")
	}
	if !IsEmbedClassifiedHTTPError(&HTTPError{Status: http.StatusServiceUnavailable, Body: `{"error":"embed failed"}`}) {
		t.Fatal("503 with embed body should classify")
	}
}

func TestIngestGateOpenClose(t *testing.T) {
	g := newIngestGate()
	if g.isClosed() {
		t.Fatal("new gate should be open")
	}
	if !g.close("embed_provider_down", "ollama down") {
		t.Fatal("expected transition to closed")
	}
	if g.close("x", "y") {
		t.Fatal("already closed")
	}
	if !g.isClosed() {
		t.Fatal("gate should be closed")
	}
	if !g.open("m") {
		t.Fatal("expected transition to open")
	}
	if g.isClosed() {
		t.Fatal("gate should be open")
	}
}

func TestIngestGateWaitOpenRespectsContext(t *testing.T) {
	g := newIngestGate()
	g.close("test", "blocked")
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if err := g.waitOpen(ctx); err == nil {
		t.Fatal("expected context error while gate closed")
	}
}
