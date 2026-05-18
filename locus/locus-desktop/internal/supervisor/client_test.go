package supervisor

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestBaseURL_Default(t *testing.T) {
	got := BaseURL(nil)
	if got == "" {
		t.Fatal("expected non-empty URL")
	}
}

func TestBaseURL_ListenOverride(t *testing.T) {
	got := BaseURL([]string{"-listen", "0.0.0.0:4123"})
	want := "http://127.0.0.1:4123"
	if got != want {
		t.Fatalf("want %s, got %s", want, got)
	}
}

func TestReachable_HealthzOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	if !Reachable(srv.URL) {
		t.Fatal("expected supervisor to be reachable")
	}
}

func TestReachable_Unreachable(t *testing.T) {
	if Reachable("http://127.0.0.1:1") {
		t.Fatal("expected supervisor to be unreachable")
	}
}

func TestWaitReady_ReadyzOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/readyz":
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	ok, detail := WaitReady(srv.URL, 400*time.Millisecond)
	if !ok {
		t.Fatalf("expected readiness true, got detail=%q", detail)
	}
}

func TestWaitReady_BootstrapStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/readyz":
			w.WriteHeader(http.StatusServiceUnavailable)
		case "/status":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"bootstrap":true}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	ok, detail := WaitReady(srv.URL, 400*time.Millisecond)
	if !ok {
		t.Fatalf("expected bootstrap readiness true, got detail=%q", detail)
	}
}

func TestWaitReady_Timeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()
	ok, detail := WaitReady(srv.URL, 450*time.Millisecond)
	if ok {
		t.Fatal("expected readiness timeout")
	}
	if strings.TrimSpace(detail) == "" {
		t.Fatal("expected non-empty timeout detail")
	}
}

func TestEntryURL_Default(t *testing.T) {
	got := EntryURL("http://127.0.0.1:7710")
	if !strings.Contains(got, "/ui/login") {
		t.Fatalf("expected login route, got %s", got)
	}
	if !strings.Contains(got, "focus=admin") {
		t.Fatalf("expected admin logs focus, got %s", got)
	}
}

func TestEntryURL_Bootstrap(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/status" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"details":{"operator_ui":{"base_url":"http://127.0.0.1:3000","bootstrap":true}}}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()
	got := EntryURL(srv.URL)
	if got != "http://127.0.0.1:3000/ui/setup" {
		t.Fatalf("want setup URL, got %s", got)
	}
}
