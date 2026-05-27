package embedui_test

import (
	"testing"

	"github.com/dop251/goja"
)

func loadOperatorMessageCtx(t *testing.T, vm *goja.Runtime) {
	t.Helper()
	evalJS(t, vm, settingsUIPath(t, "testing", "loader.js"))
	evalJS(t, vm, settingsUIPath(t, "operator_copy.js"))
	evalJS(t, vm, settingsUIPath(t, "derive", "ragWorkspaceLabel.js"))
	evalJS(t, vm, settingsUIPath(t, "render", "operatorMessage.js"))
	evalJS(t, vm, settingsUIPath(t, "render", "operatorMessageServices.js"))
	evalJS(t, vm, settingsUIPath(t, "render", "operatorMessageIndexer.js"))
	mountTestRagWorkspaceLabel(t, vm)
}

func TestOperatorCopy_inferShapeForFlat_registry(t *testing.T) {
	vm := goja.New()
	loadOperatorMessageCtx(t, vm)
	oc := vm.Get("ChimeraSettings").ToObject(vm).Get("OperatorCopy").ToObject(vm)
	fn, ok := goja.AssertFunction(oc.Get("inferShapeForFlat"))
	if !ok {
		t.Fatal("missing inferShapeForFlat")
	}
	v, err := fn(goja.Undefined(), vm.ToValue(map[string]any{
		"msg": "chat.request", "method": "POST", "path": "/v1/chat/completions",
	}))
	if err != nil {
		t.Fatal(err)
	}
	if v.String() != "chat.request" {
		t.Fatalf("chat.request shape: got %q", v.String())
	}
	v2, err := fn(goja.Undefined(), vm.ToValue(map[string]any{
		"msg": "http response", "method": "GET", "path": "/health", "statusCode": 200,
	}))
	if err != nil {
		t.Fatal(err)
	}
	if v2.String() != "http.access" {
		t.Fatalf("http.access shape: got %q", v2.String())
	}
}

func TestOperatorCopy_metricsCounterForFlat_gatewayCard(t *testing.T) {
	vm := goja.New()
	loadOperatorMessageCtx(t, vm)
	oc := vm.Get("ChimeraSettings").ToObject(vm).Get("OperatorCopy").ToObject(vm)
	mcFn, ok := goja.AssertFunction(oc.Get("metricsCounterForFlat"))
	if !ok {
		t.Fatal("missing metricsCounterForFlat")
	}
	v, err := mcFn(goja.Undefined(), vm.ToValue(map[string]any{"msg": "rag.query"}))
	if err != nil {
		t.Fatal(err)
	}
	if v.String() != "ragQuery" {
		t.Fatalf("got %q want ragQuery", v.String())
	}
}

func TestOperatorMessage_resolveCanonicalSlug_aliases(t *testing.T) {
	vm := goja.New()
	loadOperatorMessageCtx(t, vm)
	fn, ok := goja.AssertFunction(vm.Get("ChimeraSettings").ToObject(vm).Get("Render").ToObject(vm).Get("resolveCanonicalSlug"))
	if !ok {
		t.Fatal("missing resolveCanonicalSlug")
	}
	v, err := fn(goja.Undefined(), vm.ToValue(map[string]any{"msg": "conversation.routing.resolve"}))
	if err != nil {
		t.Fatal(err)
	}
	if v.String() != "conversation.routing.resolved" {
		t.Fatalf("got %q", v.String())
	}
}

func TestOperatorMessage_gatewaySlugs(t *testing.T) {
	cases := []struct {
		name string
		flat map[string]any
		opts map[string]any
		want string
	}{
		{
			name: "conversation_received",
			flat: map[string]any{"msg": "conversation.received", "turn_index": 1, "clientModel": "Claudia-0.2.0", "message_count": 2},
			want: "Turn 1 started · new conversation · client asked for Claudia-0.2.0 · 2 messages in prompt.",
		},
		{
			name: "conversation_delivered",
			flat: map[string]any{"msg": "conversation.delivered", "statusCode": 200, "total_ms": 2200},
			opts: map[string]any{"forEventLog": true},
			want: "Turn completed · 2.2 s · response delivered to client.",
		},
		{
			name: "conversation_delivered_non_evlog",
			flat: map[string]any{"msg": "conversation.delivered", "statusCode": 200, "total_ms": 142},
			want: "Completion delivered to the client (this turn finished successfully). · 142 ms",
		},
		{
			name: "routing_virtual",
			flat: map[string]any{
				"msg":           "conversation.routing.resolve",
				"clientModel":   "Chimera-0.2.0",
				"upstreamModel": "groq/meta-llama/llama-4-scout-17b-16e-instruct",
				"attempt":       4,
				"chainLen":      24,
			},
			want: "Routed virtual model Chimera-0.2.0 → llama-4-scout-17b-16e-instruct · attempt 4 of 24.",
		},
		{
			name: "rag_attached_evlog",
			flat: map[string]any{
				"msg":        "conversation.rag.attached",
				"collection": "chimera-lynn-task-orchestrator-_-abc",
				"tenant":     "tenant-1",
				"project":    "task-orchestrator",
				"hits":       8,
			},
			opts: map[string]any{"forEventLog": true},
			want: "Retrieved context · from lynn:task-orchestrator · 8 chunks injected into the request.",
		},
		{
			name: "rag_span_unknown_workspace_evlog",
			flat: map[string]any{
				"msg":        "conversation.rag.span",
				"collection": "chimera-lynn-workspacename-_-79692145",
			},
			opts: map[string]any{
				"forEventLog": true,
				"convEvlogMeta": map[string]any{
					"ragCoords": map[string]any{
						"tenantId":  "tenant-1",
						"projectId": "workspacename",
						"flavorId":  "",
					},
				},
			},
			want: "RAG search for workspace lynn:workspacename - missing or undefined.",
		},
		{
			name: "rag_attached_unknown_workspace_evlog",
			flat: map[string]any{
				"msg":        "conversation.rag.attached",
				"tenant":     "tenant-1",
				"project":    "workspacename",
				"collection": "chimera-lynn-workspacename-_-79692145",
				"hits":       2,
			},
			opts: map[string]any{"forEventLog": true},
			want: "Retrieved context · from lynn:workspacename - missing or undefined · 2 chunks injected into the request.",
		},
		{
			name: "model_not_found_will_retry",
			flat: map[string]any{
				"msg":           "conversation.fallback.model_not_found",
				"upstreamModel": "google/gemini-3.1-flash-live-preview",
				"attempt":       1,
				"chainLen":      22,
				"willRetry":     true,
			},
			opts: map[string]any{"forEventLog": true},
			want: "gemini-3.1-flash-live-preview not found (404) · trying next in chain (attempt 1 of 22).",
		},
		{
			name: "rag_retrieve_error_context_evlog",
			flat: map[string]any{
				"msg": "rag.retrieve.error",
				"err": "Embedding input too long for the model context window.",
			},
			opts: map[string]any{"forEventLog": true},
			want: "Unable to insert indexed samples from the workspace because they are too long for the model context window.",
		},
		{
			name: "model_not_found_exhausted",
			flat: map[string]any{
				"msg":           "conversation.fallback.model_not_found",
				"upstreamModel": "google/gemini-3.1-flash-live-preview",
				"attempt":       22,
				"chainLen":      22,
				"willRetry":     false,
			},
			opts: map[string]any{"forEventLog": true},
			want: "No model in the fallback chain could serve this request · last attempt: gemini-3.1-flash-live-preview (404).",
		},
		{
			name: "gateway_auth_reloaded",
			flat: map[string]any{"msg": "gateway.auth.reloaded", "count": 4},
			want: "Client credentials reloaded from disk. Active keys: 4.",
		},
		{
			name: "ingest_complete",
			flat: map[string]any{"msg": "ingest.complete", "chunks": 3, "source": "doc.pdf", "tenant": "acme"},
			want: "Ingest finished — document indexed. · 3 chunks · source: doc.pdf · tenant acme",
		},
		{
			name: "supervisor_broker_ready_alias",
			flat: map[string]any{"msg": "chimera-supervisor.chimera-broker.ready", "url": "http://127.0.0.1:8081/health"},
			want: "chimera-broker passed health check — ready. · 127.0.0.1:8081/health",
		},
		{
			name: "provider_limits_context_window",
			flat: map[string]any{
				"msg":            "chat.provider_limits.blocked",
				"upstreamModel":  "groq/llama-3.3-70b-versatile",
				"reason":         "context_window",
				"outgoingTokens": 9500,
				"max_tokens":     512,
				"context_cap":    8192,
			},
			want: "Blocked by provider limits · llama-3.3-70b-versatile · context window · 9500 prompt + 512 max_tokens > cap 8192",
		},
		{
			name: "provider_limits_tpm",
			flat: map[string]any{
				"msg":           "chat.provider_limits.blocked",
				"upstreamModel": "groq/llama-3.1-8b-instant",
				"reason":        "tpm",
			},
			want: "Blocked by provider limits · llama-3.1-8b-instant · TPM quota",
		},
		{
			name: "provider_limits_body_bytes",
			flat: map[string]any{
				"msg":            "chat.provider_limits.blocked",
				"upstreamModel":  "groq/groq/compound-mini",
				"reason":         "request_body_bytes",
				"body_bytes":     4000000,
				"max_body_bytes": 3500000,
			},
			want: "Blocked by provider limits · groq/compound-mini · body size · 4000000 bytes > cap 3500000",
		},
		{
			name: "unknown_slug",
			flat: map[string]any{"msg": "gateway.metrics.init_failed"},
			want: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			vm := goja.New()
			loadOperatorMessageCtx(t, vm)
			fn, ok := goja.AssertFunction(vm.Get("ChimeraSettings").ToObject(vm).Get("Render").ToObject(vm).Get("operatorMessage"))
			if !ok {
				t.Fatal("missing operatorMessage")
			}
			v, err := fn(goja.Undefined(), vm.ToValue(tc.flat), vm.ToValue(tc.opts))
			if err != nil {
				t.Fatal(err)
			}
			if got := v.String(); got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
}

func TestOperatorMessage_broker_ready_canonicalSlug(t *testing.T) {
	vm := goja.New()
	loadOperatorMessageCtx(t, vm)
	fn, ok := goja.AssertFunction(vm.Get("ChimeraSettings").ToObject(vm).Get("Render").ToObject(vm).Get("operatorMessage"))
	if !ok {
		t.Fatal("missing operatorMessage")
	}
	v, err := fn(goja.Undefined(), vm.ToValue(map[string]any{
		"service":    "chimera-broker",
		"msg":        "broker.ready",
		"listen_url": "http://127.0.0.1:8899/ui",
	}))
	if err != nil {
		t.Fatal(err)
	}
	want := "Ready · UI at http://127.0.0.1:8899/ui"
	if v.String() != want {
		t.Fatalf("got %q want %q", v.String(), want)
	}
}

func TestOperatorMessage_vectorstore_version_canonicalSlug(t *testing.T) {
	vm := goja.New()
	loadOperatorMessageCtx(t, vm)
	fn, ok := goja.AssertFunction(vm.Get("ChimeraSettings").ToObject(vm).Get("Render").ToObject(vm).Get("operatorMessage"))
	if !ok {
		t.Fatal("missing operatorMessage")
	}
	v, err := fn(goja.Undefined(), vm.ToValue(map[string]any{
		"service":        "qdrant",
		"msg":            "vectorstore.version",
		"qdrant_version": "1.12.4",
	}))
	if err != nil {
		t.Fatal(err)
	}
	want := "Component: chimera-vectorstore · Backend: Qdrant 1.12.4"
	if v.String() != want {
		t.Fatalf("got %q want %q", v.String(), want)
	}
}

func TestOperatorMessage_indexer_state_humanTitle(t *testing.T) {
	vm := goja.New()
	loadOperatorMessageCtx(t, vm)
	fn, ok := goja.AssertFunction(vm.Get("ChimeraSettings").ToObject(vm).Get("Render").ToObject(vm).Get("operatorMessage"))
	if !ok {
		t.Fatal("missing operatorMessage")
	}
	v, err := fn(goja.Undefined(), vm.ToValue(map[string]any{
		"service": "indexer", "msg": "indexer state", "state": "watch_idle", "queue_depth": 0,
	}))
	if err != nil {
		t.Fatal(err)
	}
	got := v.String()
	if got == "" || !containsAll(got, "Waiting for file changes", "queue depth 0") {
		t.Fatalf("got %q", got)
	}
}

func TestOperatorMessage_indexer_job_failed_shortErr(t *testing.T) {
	vm := goja.New()
	loadOperatorMessageCtx(t, vm)
	fn, ok := goja.AssertFunction(vm.Get("ChimeraSettings").ToObject(vm).Get("Render").ToObject(vm).Get("operatorMessage"))
	if !ok {
		t.Fatal("missing operatorMessage")
	}
	v, err := fn(goja.Undefined(), vm.ToValue(map[string]any{
		"service": "indexer",
		"msg":     "indexer.job.failed",
		"rel":     "spotify/track.json",
		"err":     `/v1/ingest/session/x/complete: status 404: {"error":{"message":"unknown or expired session"}}`,
	}))
	if err != nil {
		t.Fatal(err)
	}
	got := v.String()
	if contains(got, "/v1/ingest/session") {
		t.Fatalf("raw path leaked: %q", got)
	}
	if !contains(got, "chunked upload session lost") {
		t.Fatalf("got %q", got)
	}
}

func contains(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && (s == sub || len(s) > 0 && stringIndex(s, sub) >= 0))
}

func stringIndex(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func containsAll(s string, parts ...string) bool {
	for _, p := range parts {
		if !contains(s, p) {
			return false
		}
	}
	return true
}

func TestOperatorMessage_operatorFriendlyGatewayMsg_compat(t *testing.T) {
	vm := goja.New()
	loadOperatorMessageCtx(t, vm)
	fn, ok := goja.AssertFunction(vm.Get("ChimeraSettings").ToObject(vm).Get("Render").ToObject(vm).Get("operatorFriendlyGatewayMsg"))
	if !ok {
		t.Fatal("missing operatorFriendlyGatewayMsg")
	}
	v, err := fn(goja.Undefined(), vm.ToValue(map[string]any{"msg": "chat.request"}))
	if err != nil {
		t.Fatal(err)
	}
	want := "Chat completion request accepted and prepared for upstream routing."
	if v.String() != want {
		t.Fatalf("got %q want %q", v.String(), want)
	}
}
