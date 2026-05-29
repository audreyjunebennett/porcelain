package conversations_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/operatorstore"
	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/server/adminui/api/conversations"
	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/server/adminui/handler"
	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/server/adminui/session"
	gruntime "github.com/lynn/porcelain/chimera/chimera-gateway/internal/server/runtime"
	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/testsupport"
)

func testEnv(t *testing.T) (*http.ServeMux, *handler.Handler, *operatorstore.Store, string) {
	t.Helper()
	dir := t.TempDir()
	store, err := operatorstore.Open(filepath.Join(dir, "op.sqlite"), testsupport.GatewayOperatorMigrationsDir(t), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ui := session.NewUIOptions()
	rt := &gruntime.Runtime{}
	rt.SetOperatorStoreForTest(store)
	h := handler.New(rt, nil, ui)
	mux := http.NewServeMux()
	conversations.Register(mux, h)
	const principal = "tenant-a"
	sid, err := ui.Sessions.Issue(principal)
	if err != nil {
		t.Fatal(err)
	}
	return mux, h, store, sid
}

func authedRequest(method, path, sid, cookieName string, body []byte) *http.Request {
	var r *http.Request
	if body != nil {
		r = httptest.NewRequest(method, path, bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
	} else {
		r = httptest.NewRequest(method, path, nil)
	}
	r.AddCookie(&http.Cookie{Name: cookieName, Value: sid})
	return r
}

func TestConversationsAPI_listDetailPatchFlagDelete(t *testing.T) {
	mux, h, store, sid := testEnv(t)
	ctx := context.Background()
	const cid = "conv-api-1"
	if err := store.EnsureConversation(ctx, "tenant-a", cid, "hello", operatorstore.ConversationWorkspaceSnapshot{}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AppendTurn(ctx, "tenant-a", cid, operatorstore.AppendTurnInput{Role: "user", Content: "hello"}); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, authedRequest(http.MethodGet, "/api/ui/conversations?limit=10", sid, h.CookieName(), nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, authedRequest(http.MethodGet, "/api/ui/conversations/"+cid, sid, h.CookieName(), nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("detail status=%d", rec.Code)
	}

	patchBody, _ := json.Marshal(map[string]string{"title": "Renamed"})
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, authedRequest(http.MethodPatch, "/api/ui/conversations/"+cid, sid, h.CookieName(), patchBody))
	if rec.Code != http.StatusOK {
		t.Fatalf("patch status=%d body=%s", rec.Code, rec.Body.String())
	}

	flagBody, _ := json.Marshal(map[string]bool{"flagged": true})
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, authedRequest(http.MethodPost, "/api/ui/conversations/"+cid+"/flag", sid, h.CookieName(), flagBody))
	if rec.Code != http.StatusOK {
		t.Fatalf("flag status=%d", rec.Code)
	}

	ui := h.Opts
	otherSid, err := ui.Sessions.Issue("tenant-b")
	if err != nil {
		t.Fatal(err)
	}
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, authedRequest(http.MethodGet, "/api/ui/conversations/"+cid, otherSid, h.CookieName(), nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("cross principal status=%d", rec.Code)
	}

	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, authedRequest(http.MethodDelete, "/api/ui/conversations/"+cid, sid, h.CookieName(), nil))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete status=%d", rec.Code)
	}
}

func TestConversationsAPI_unauthorized(t *testing.T) {
	mux, _, _, _ := testEnv(t)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/ui/conversations", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d", rec.Code)
	}
}

func TestConversationsAPI_flaggedFilter(t *testing.T) {
	mux, h, store, sid := testEnv(t)
	ctx := context.Background()
	const cid = "conv-flag"
	if err := store.EnsureConversation(ctx, "tenant-a", cid, "x", operatorstore.ConversationWorkspaceSnapshot{}); err != nil {
		t.Fatal(err)
	}
	if err := store.SetConversationFlagged(ctx, "tenant-a", cid, true); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, authedRequest(http.MethodGet, "/api/ui/conversations?flagged=1", sid, h.CookieName(), nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d", rec.Code)
	}
	var body struct {
		Conversations []struct {
			ConversationID string `json:"conversation_id"`
		} `json:"conversations"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Conversations) != 1 || body.Conversations[0].ConversationID != cid {
		t.Fatalf("body=%+v", body)
	}
}
