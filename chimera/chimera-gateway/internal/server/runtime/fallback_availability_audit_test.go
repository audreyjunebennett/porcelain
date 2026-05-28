package runtime

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/testsupport"
	"github.com/lynn/porcelain/chimera/internal/config"
)

func TestAuditConfiguredFallbackAvailability_warnsOnUnavailableChainModel(t *testing.T) {
	ctx := context.Background()
	rt := testRuntimeWithProviderAvailability(t)
	t.Cleanup(func() { rt.CloseOperator() })
	st := rt.OperatorStore()
	if err := st.ReplaceProviderModelAvailability(ctx, "tenant-a", "groq", map[string]bool{
		"groq/free": true,
		"groq/paid": false,
	}); err != nil {
		t.Fatal(err)
	}
	if err := rt.ReloadProviderModelAvailability(ctx); err != nil {
		t.Fatal(err)
	}

	res, _, _ := rt.Snapshot()
	res = config.CloneResolved(res)
	res.FallbackChain = []string{"groq/free", "groq/paid"}

	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	auditConfiguredFallbackAvailability(ctx, rt, res, log)

	out := buf.String()
	if !strings.Contains(out, "gateway.catalog.fallback_unavailable_model") {
		t.Fatalf("expected catalog audit log, got: %s", out)
	}
	if !strings.Contains(out, "groq/paid") || !strings.Contains(out, "tenant-a") {
		t.Fatalf("expected tenant+model in log, got: %s", out)
	}
}

func TestAuditConfiguredFallbackAvailability_skipsWhenAllAvailable(t *testing.T) {
	ctx := context.Background()
	rt := testRuntimeWithProviderAvailability(t)
	t.Cleanup(func() { rt.CloseOperator() })
	st := rt.OperatorStore()
	if err := st.ReplaceProviderModelAvailability(ctx, "tenant-a", "groq", map[string]bool{
		"groq/free": true,
		"groq/paid": true,
	}); err != nil {
		t.Fatal(err)
	}
	if err := rt.ReloadProviderModelAvailability(ctx); err != nil {
		t.Fatal(err)
	}

	res, _, _ := rt.Snapshot()
	res = config.CloneResolved(res)
	res.FallbackChain = []string{"groq/free", "groq/paid"}

	var buf bytes.Buffer
	log := slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	auditConfiguredFallbackAvailability(ctx, rt, res, log)
	if strings.Contains(buf.String(), "gateway.catalog.fallback_unavailable_model") {
		t.Fatalf("unexpected audit log: %s", buf.String())
	}
}

func testRuntimeWithProviderAvailability(t *testing.T) *Runtime {
	t.Helper()
	dir := t.TempDir()
	gwPath := filepath.Join(dir, "gateway.yaml")
	opMig := testsupport.GatewayOperatorMigrationsDir(t)
	opMigSlash := strings.ReplaceAll(filepath.ToSlash(opMig), `\`, `/`)
	raw := "gateway:\n  semver: \"0.1.0\"\n  listen_port: 0\n  listen_host: \"127.0.0.1\"\n" +
		"broker:\n  base_url: \"http://127.0.0.1:9\"\n  api_key_env: \"CHIMERA_BROKER_API_KEY\"\n" +
		"health:\n  timeout_ms: 2000\n  chat_timeout_ms: 60000\n" +
		"paths:\n  tokens: \"./api-keys.yaml\"\n  routing_policy: \"./routing-policy.yaml\"\n" +
		"routing:\n  fallback_chain:\n    - \"groq/free\"\n" +
		"operator:\n  sqlite_path: \"./operator.sqlite\"\n  migrations_dir: \"" + opMigSlash + "\"\n"
	if err := os.WriteFile(gwPath, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "api-keys.yaml"), []byte("api_keys:\n  - secret: tok\n    tenant_id: tenant-a\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "routing-policy.yaml"), []byte("rules: []\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rt, err := NewRuntime(gwPath, slog.New(slog.NewTextHandler(ioDiscard{}, &slog.HandlerOptions{Level: slog.LevelError + 1})))
	if err != nil {
		t.Fatal(err)
	}
	if rt.OperatorStore() == nil {
		t.Fatal("operator store required")
	}
	return rt
}

type ioDiscard struct{}

func (ioDiscard) Write(p []byte) (int, error) { return len(p), nil }
