package embed

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestEmbedBatch_HappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer key123" {
			t.Errorf("missing auth header: %q", got)
		}
		var in struct {
			Model string   `json:"model"`
			Input []string `json:"input"`
		}
		_ = json.NewDecoder(r.Body).Decode(&in)
		if in.Model != "test-embed" {
			t.Errorf("model: %q", in.Model)
		}
		out := struct {
			Data []struct {
				Index     int       `json:"index"`
				Embedding []float32 `json:"embedding"`
			} `json:"data"`
		}{}
		for i := range in.Input {
			out.Data = append(out.Data, struct {
				Index     int       `json:"index"`
				Embedding []float32 `json:"embedding"`
			}{Index: i, Embedding: []float32{float32(i + 1), 0.0, 0.0}})
		}
		_ = json.NewEncoder(w).Encode(out)
	}))
	defer srv.Close()

	c := New(srv.URL, "key123", "test-embed")
	vecs, err := c.EmbedBatch(context.Background(), []string{"a", "b"})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(vecs) != 2 || vecs[0][0] != 1 || vecs[1][0] != 2 {
		t.Fatalf("vecs: %v", vecs)
	}
}

func TestEmbedBatch_ErrorBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":{"message":"nope"}}`, http.StatusBadRequest)
	}))
	defer srv.Close()
	c := New(srv.URL, "", "m")
	_, err := c.EmbedBatch(context.Background(), []string{"x"})
	if err == nil || !strings.Contains(err.Error(), "status 400") {
		t.Fatalf("expected 400 error, got %v", err)
	}
}

func TestEmbedBatch_EmptyInputs(t *testing.T) {
	c := New("http://x", "", "m")
	v, err := c.EmbedBatch(context.Background(), nil)
	if err != nil || v != nil {
		t.Fatalf("unexpected: %v %v", v, err)
	}
}
