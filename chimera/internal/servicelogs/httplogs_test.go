package servicelogs

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHandleLogsStream_replayBuffer(t *testing.T) {
	store := New(50)
	_, _ = store.Writer(SourceChimeraGateway).Write([]byte("a\n"))
	_, _ = store.Writer(SourceChimeraBroker).Write([]byte("b\n"))

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	req := httptest.NewRequest(http.MethodGet, "/logs/stream?replay=buffer", nil).WithContext(ctx)
	req.RemoteAddr = "127.0.0.1:12345"
	rec := httptest.NewRecorder()
	HandleLogsStream(store, rec, req)
	body := rec.Body.String()
	if !strings.Contains(body, `"source":"chimera-gateway"`) {
		t.Fatalf("missing gateway line: %s", body)
	}
	if !strings.Contains(body, `"source":"chimera-broker"`) {
		t.Fatalf("missing broker line: %s", body)
	}
}

func TestRegisterLogRoutes_loopbackOnly(t *testing.T) {
	store := New(10)
	mux := http.NewServeMux()
	RegisterLogRoutes(mux, store)

	req := httptest.NewRequest(http.MethodGet, "/logs?since=0", nil)
	req.RemoteAddr = "10.0.0.1:9999"
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("code=%d", rec.Code)
	}

	req2 := httptest.NewRequest(http.MethodGet, "/logs?since=0", nil)
	req2.RemoteAddr = "127.0.0.1:4321"
	rec2 := httptest.NewRecorder()
	mux.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("code=%d body=%s", rec2.Code, rec2.Body.String())
	}
	var resp PollResponse
	if err := json.Unmarshal(rec2.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
}
