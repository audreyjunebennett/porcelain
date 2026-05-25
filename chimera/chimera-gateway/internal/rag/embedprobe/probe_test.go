package embedprobe

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestProbe_HappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/embeddings" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(`{"data":[{"index":0,"embedding":[0.1,0.2,0.3]}]}`))
	}))
	t.Cleanup(srv.Close)

	if err := Probe(context.Background(), srv.URL+"/v1/embeddings", "internal/nomic-embed-text", 3); err != nil {
		t.Fatal(err)
	}
}

func TestProbe_DimMismatch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"index":0,"embedding":[0.1]}]}`))
	}))
	t.Cleanup(srv.Close)

	if err := Probe(context.Background(), srv.URL+"/v1/embeddings", "m", 3); err == nil {
		t.Fatal("expected dim mismatch")
	}
}
