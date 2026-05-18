package server

import (
	"io"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"log/slog"

	"github.com/lynn/porcelain/internal/naming"
)

func testLog() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError + 1}))
}

// writeGateway writes a minimal gateway.yaml for tests. When qdrantURL is non-empty, RAG is enabled.
func writeGateway(t *testing.T, path, upstream string, chain []string, qdrantURL string) {
	t.Helper()
	chainYAML := ""
	for _, m := range chain {
		chainYAML += "    - \"" + m + "\"\n"
	}
	semver := "0.1.0"
	if qdrantURL != "" {
		semver = "0.2.0"
	}
	raw := "gateway:\n  semver: \"" + semver + "\"\n  listen_port: 0\n  listen_host: \"127.0.0.1\"\n" +
		"upstream:\n  base_url: \"" + upstream + "\"\n  api_key_env: \"" + naming.EnvBrokerAPIKeyTarget + "\"\n" +
		"health:\n  timeout_ms: 2000\n  chat_timeout_ms: 60000\n" +
		"paths:\n  tokens: \"./" + naming.APIKeysFileTarget + "\"\n  routing_policy: \"./" + naming.RoutingPolicyFileTarget + "\"\n" +
		"routing:\n  fallback_chain:\n" + chainYAML
	if qdrantURL != "" {
		raw += "rag:\n  enabled: true\n  qdrant:\n    url: \"" + qdrantURL + "\"\n" +
			"  embedding:\n    model: \"test-embed\"\n    dim: 8\n" +
			"  chunking:\n    size: 128\n    overlap: 32\n" +
			"  ingest:\n    max_bytes: 10485760\n" +
			"  defaults:\n    project_id: \"default\"\n"
	}
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeTokens(t *testing.T, path, token, tenant string) {
	t.Helper()
	raw := "api_keys:\n  - secret: \"" + token + "\"\n    tenant_id: \"" + tenant + "\"\n"
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeRouting(t *testing.T, path, model string, minChars int) {
	t.Helper()
	raw := "ambiguous_default_model: \"" + model + "\"\nrules:\n  - name: x\n    when:\n      min_message_chars: " +
		strconv.Itoa(minChars) + "\n    models:\n      - \"" + model + "\"\n"
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
}

// runtimeForCatalogTest writes gateway + api-keys + routing-policy and returns a loaded Runtime.
func runtimeForCatalogTest(t *testing.T, upstreamURL string) *Runtime {
	t.Helper()
	dir := t.TempDir()
	gwPath := filepath.Join(dir, naming.GatewayConfigFileTarget)
	tokPath := filepath.Join(dir, naming.APIKeysFileTarget)
	routePath := filepath.Join(dir, naming.RoutingPolicyFileTarget)
	writeGateway(t, gwPath, upstreamURL, []string{"m"}, "")
	writeTokens(t, tokPath, "tok", "tenant")
	if err := os.WriteFile(routePath, []byte("rules: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rt, err := NewRuntime(gwPath, testLog())
	if err != nil {
		t.Fatal(err)
	}
	return rt
}
