package control

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/lynn/porcelain/chimera/internal/servicelogs"
	"github.com/lynn/porcelain/chimera/internal/wrapper/contract"
)

func TestHandler_StatusAndMetrics(t *testing.T) {
	t.Parallel()
	st := NewState()
	st.SetRequired(true, true)
	st.SetVersions("test-version", "abc123")
	st.SetEndpoints("127.0.0.1:8080", "127.0.0.1:6333")
	st.SetBrokerReady(true)
	st.SetVectorstoreReady(true)
	st.IncBrokerRestarts()
	st.IncVectorstoreRestarts()
	st.SetLastError("last err")
	st.SetOperatorUI("http://127.0.0.1:3000", false)

	srv := httptest.NewServer(Handler(st, servicelogs.New(10), nil))
	t.Cleanup(srv.Close)

	res, err := http.Get(srv.URL + "/status")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status code=%d want 200", res.StatusCode)
	}
	var doc map[string]any
	if err := json.NewDecoder(res.Body).Decode(&doc); err != nil {
		t.Fatal(err)
	}
	if doc["component"] != contract.ComponentSupervisor {
		t.Fatalf("component=%v", doc["component"])
	}
	if doc["backend_name"] != "custom" {
		t.Fatalf("backend_name=%v", doc["backend_name"])
	}
	if doc["backend_mode"] != "binary" {
		t.Fatalf("backend_mode=%v", doc["backend_mode"])
	}
	if doc["status"] != "ok" {
		t.Fatalf("status=%v", doc["status"])
	}
	if doc["last_error"] != "last err" {
		t.Fatalf("last_error=%v", doc["last_error"])
	}
	versionObj, _ := doc["version"].(map[string]any)
	if versionObj["wrapper"] != "test-version" {
		t.Fatalf("version.wrapper=%v", versionObj["wrapper"])
	}
	if versionObj["build_sha"] != "abc123" {
		t.Fatalf("version.build_sha=%v", versionObj["build_sha"])
	}
	restarts, _ := doc["restarts"].(float64)
	if int(restarts) != 2 {
		t.Fatalf("restarts=%v", doc["restarts"])
	}
	details, _ := doc["details"].(map[string]any)
	ui, _ := details["operator_ui"].(map[string]any)
	if ui["base_url"] != "http://127.0.0.1:3000" {
		t.Fatalf("operator_ui.base_url=%v", ui["base_url"])
	}

	met, err := http.Get(srv.URL + "/metrics")
	if err != nil {
		t.Fatal(err)
	}
	defer met.Body.Close()
	if met.StatusCode != http.StatusOK {
		t.Fatalf("metrics status=%d", met.StatusCode)
	}
	b, _ := io.ReadAll(met.Body)
	text := string(b)
	for _, m := range []string{
		"chimera_wrapper_up",
		"chimera_backend_up",
		"chimera_backend_restarts_total",
		"chimera_requests_total",
		"chimera_request_duration_seconds",
	} {
		if !strings.Contains(text, m) {
			t.Fatalf("metrics missing %s\n%s", m, text)
		}
	}
}

func TestHandler_ReadyDegraded(t *testing.T) {
	t.Parallel()
	st := NewState()
	st.SetRequired(true, false)
	st.SetVersions("test-version", "")
	st.SetBrokerReady(false)

	srv := httptest.NewServer(Handler(st, servicelogs.New(10), nil))
	t.Cleanup(srv.Close)

	res, err := http.Get(srv.URL + "/readyz")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("readyz status=%d want 503", res.StatusCode)
	}

	res2, err := http.Get(srv.URL + "/status")
	if err != nil {
		t.Fatal(err)
	}
	defer res2.Body.Close()
	if res2.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status code=%d want 503", res2.StatusCode)
	}
}

func TestHandler_Shutdown(t *testing.T) {
	t.Parallel()
	st := NewState()
	called := make(chan struct{}, 1)
	onShutdown := func() {
		select {
		case called <- struct{}{}:
		default:
		}
	}
	srv := httptest.NewServer(Handler(st, servicelogs.New(10), onShutdown))
	t.Cleanup(srv.Close)

	req, err := http.NewRequest(http.MethodPost, srv.URL+"/shutdown", nil)
	if err != nil {
		t.Fatal(err)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusAccepted {
		t.Fatalf("shutdown status=%d want 202", res.StatusCode)
	}
	select {
	case <-called:
	case <-time.After(2 * time.Second):
		t.Fatal("onShutdown was not invoked")
	}
}
