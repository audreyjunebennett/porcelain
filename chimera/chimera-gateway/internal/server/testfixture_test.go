package server

import (
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"log/slog"

	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/testsupport"
	"github.com/lynn/porcelain/internal/naming"
)

func testLog() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError + 1}))
}

// mustRuntime opens a gateway runtime and closes SQLite stores on test cleanup (Windows file locks).
func mustRuntime(t *testing.T, gwPath string) *Runtime {
	return mustRuntimeLog(t, gwPath, testLog())
}

func mustRuntimeLog(t *testing.T, gwPath string, log *slog.Logger) *Runtime {
	t.Helper()
	if log == nil {
		log = testLog()
	}
	rt, err := NewRuntime(gwPath, log)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		rt.CloseOperator()
		rt.CloseMetrics()
	})
	return rt
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
		"broker:\n  base_url: \"" + upstream + "\"\n  api_key_env: \"" + naming.EnvBrokerAPIKeyTarget + "\"\n" +
		"health:\n  timeout_ms: 2000\n  chat_timeout_ms: 60000\n" +
		"paths:\n  tokens: \"./" + naming.APIKeysFileTarget + "\"\n  routing_policy: \"./" + naming.RoutingPolicyFileTarget + "\"\n" +
		"routing:\n  fallback_chain:\n" + chainYAML
	if qdrantURL != "" {
		raw += "vectorstore:\n  url: \"" + qdrantURL + "\"\n" +
			"rag:\n  enabled: true\n" +
			"  embedding:\n    model: \"test-embed\"\n    dim: 8\n" +
			"  chunking:\n    size: 128\n    overlap: 32\n" +
			"  ingest:\n    max_bytes: 10485760\n" +
			"  defaults:\n    project_id: \"default\"\n"
	}
	opMig := testsupport.GatewayOperatorMigrationsDir(t)
	opMigSlash := strings.ReplaceAll(filepath.ToSlash(opMig), `\`, `/`)
	raw += "operator:\n  sqlite_path: \"./operator.sqlite\"\n  migrations_dir: \"" + opMigSlash + "\"\n"
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
	return mustRuntime(t, gwPath)
}
