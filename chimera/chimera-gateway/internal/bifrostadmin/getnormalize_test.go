package bifrostadmin

import (
	"net/http"
	"testing"
)

func TestIsProviderMissingGET(t *testing.T) {
	env := []byte(`{"is_chimera_broker_error":false,"status_code":404,"error":{"message":"Provider not found: not found"},"extra_fields":{}}`)
	if !IsProviderMissingGET(http.StatusNotFound, []byte("{}")) {
		t.Fatal("want 404 missing")
	}
	if !IsProviderMissingGET(http.StatusOK, env) {
		t.Fatal("want envelope missing")
	}
	if IsProviderMissingGET(http.StatusOK, []byte(`{"name":"ollama","keys":[]}`)) {
		t.Fatal("real provider should not be missing")
	}
}

func TestNormalizeProviderGETForMerge(t *testing.T) {
	env := []byte(`{"is_chimera_broker_error":false,"status_code":404,"error":{"message":"Provider not found: not found"},"extra_fields":{}}`)
	b, ok := NormalizeProviderGETForMerge(http.StatusOK, env)
	if !ok || string(b) != "{}" {
		t.Fatalf("got %q ok=%v", b, ok)
	}
}
