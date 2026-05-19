package line

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestReorderNormalizedJSONPreservesExtraFields(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		raw  string
		want map[string]any
	}{
		{
			name: "gateway_http_access",
			raw:  `{"timestamp":"2026-05-14T12:34:56Z","level":"INFO","service":"chimera-gateway","msg":"gateway.http.access","method":"GET","path":"/health","statusCode":200,"responseTimeMs":12,"_chimera_norm":1}`,
			want: map[string]any{
				"method":     "GET",
				"path":       "/health",
				"statusCode": float64(200),
			},
		},
		{
			name: "vectorstore_trace",
			raw:  `{"level":"INFO","service":"chimera-vectorstore","msg":"vectorstore.trace.other","progress_detail":"Loading shard 1/3","qdrant_target":"storage::toc","_chimera_norm":1}`,
			want: map[string]any{
				"progress_detail": "Loading shard 1/3",
				"qdrant_target":   "storage::toc",
			},
		},
		{
			name: "broker_http",
			raw:  `{"timestamp":"t","level":"INFO","service":"chimera-broker","msg":"broker.http.access","http_method":"POST","http_target":"/v1/chat","http_status":200,"catalog_model_count":42,"listen_url":"http://127.0.0.1:8080","_chimera_norm":1}`,
			want: map[string]any{
				"http_method":         "POST",
				"http_target":         "/v1/chat",
				"http_status":         float64(200),
				"catalog_model_count": float64(42),
				"listen_url":          "http://127.0.0.1:8080",
			},
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assertReorderPreserves(t, tc.raw, tc.want)
			first, ok := ReorderNormalizedJSON([]byte(tc.raw))
			if !ok {
				t.Fatal("first reorder failed")
			}
			second, ok := ReorderNormalizedJSON(first)
			if !ok {
				t.Fatal("second reorder failed")
			}
			var got map[string]any
			if err := json.Unmarshal(second, &got); err != nil {
				t.Fatal(err)
			}
			for k, want := range tc.want {
				if got[k] != want {
					t.Fatalf("after double reorder %s=%v want %v full=%v", k, got[k], want, got)
				}
			}
		})
	}
}

func assertReorderPreserves(t *testing.T, raw string, want map[string]any) {
	t.Helper()
	out, ok := ReorderNormalizedJSON([]byte(raw))
	if !ok {
		t.Fatal("expected reorder ok")
	}
	var got map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatal(err)
	}
	for k, v := range want {
		if got[k] != v {
			t.Fatalf("%s=%v want %v; out=%s", k, got[k], v, out)
		}
	}
	if got["_chimera_norm"] != float64(1) {
		t.Fatalf("_chimera_norm=%v", got["_chimera_norm"])
	}
	idxNorm := strings.LastIndex(string(out), `"_chimera_norm"`)
	if idxNorm < 0 {
		t.Fatal("missing _chimera_norm")
	}
	for k := range want {
		if strings.Index(string(out), `"`+k+`"`) > idxNorm {
			t.Fatalf("expected %q before _chimera_norm", k)
		}
	}
}

func TestReorderNormalizedJSONTwiceIdempotentShape(t *testing.T) {
	raw := []byte(`{"_chimera_norm":1,"msg":"broker.ready","service":"chimera-broker","level":"INFO","timestamp":"2026-05-16T12:00:00Z","listen_port":8080}`)
	first, ok := ReorderNormalizedJSON(raw)
	if !ok {
		t.Fatal("expected reorder")
	}
	second, ok := ReorderNormalizedJSON(first)
	if !ok {
		t.Fatal("expected second reorder")
	}
	var m map[string]any
	if err := json.Unmarshal(second, &m); err != nil {
		t.Fatal(err)
	}
	if m["msg"] != "broker.ready" {
		t.Fatalf("msg=%v", m["msg"])
	}
	if int(m["listen_port"].(float64)) != 8080 {
		t.Fatalf("listen_port=%v", m["listen_port"])
	}
}
