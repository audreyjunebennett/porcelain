package bifrostadmin

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClient_GetProvider(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/providers/groq" || r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("Authorization") != "Bearer secret" {
			t.Errorf("missing bearer, got %q", r.Header.Get("Authorization"))
		}
		_, _ = w.Write([]byte(`{"name":"groq","keys":[]}`))
	}))
	defer ts.Close()

	c := &Client{BaseURL: ts.URL, BearerToken: "secret"}
	body, st, err := c.GetProvider(context.Background(), "groq")
	if err != nil {
		t.Fatal(err)
	}
	if st != http.StatusOK {
		t.Fatalf("status %d", st)
	}
	if string(body) != `{"name":"groq","keys":[]}` {
		t.Fatalf("body %s", body)
	}
}

func TestClient_PutProvider(t *testing.T) {
	var gotMethod, gotPath, gotCT, gotAuth string
	var gotBody []byte
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotCT = r.Header.Get("Content-Type")
		gotAuth = r.Header.Get("Authorization")
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	c := &Client{BaseURL: ts.URL, BearerToken: "tok"}
	st, body, err := c.PutProvider(context.Background(), "gemini", []byte(`{"keys":[]}`))
	if err != nil {
		t.Fatal(err)
	}
	if st != http.StatusOK {
		t.Fatalf("status %d", st)
	}
	_ = body
	if gotMethod != http.MethodPut || gotPath != "/api/providers/gemini" {
		t.Fatalf("got %s %s", gotMethod, gotPath)
	}
	if !strings.HasPrefix(gotCT, "application/json") {
		t.Fatalf("content-type %q", gotCT)
	}
	if gotAuth != "Bearer tok" {
		t.Fatalf("auth %q", gotAuth)
	}
	if string(gotBody) != `{"keys":[]}` {
		t.Fatalf("body %s", gotBody)
	}
}

func TestClient_GetProvider_emptyBase(t *testing.T) {
	c := &Client{}
	_, _, err := c.GetProvider(context.Background(), "groq")
	if err == nil {
		t.Fatal("expected error")
	}
}
