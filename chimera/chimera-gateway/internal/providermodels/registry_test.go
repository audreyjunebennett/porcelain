package providermodels_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/operatorstore"
	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/providermodels"
	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/testsupport"
)

func openStore(t *testing.T) *operatorstore.Store {
	t.Helper()
	dir := t.TempDir()
	s, err := operatorstore.Open(filepath.Join(dir, "operator.sqlite"), testsupport.GatewayOperatorMigrationsDir(t), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestRegistry_ReloadAndFilter(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()
	if err := s.UpsertProviderModelAvailability(ctx, "tenant-a", "groq", "groq/blocked", false, "{}"); err != nil {
		t.Fatal(err)
	}

	reg := providermodels.NewRegistry()
	if err := reg.Reload(ctx, s); err != nil {
		t.Fatal(err)
	}
	if reg.Revision() != 1 {
		t.Fatalf("revision=%d", reg.Revision())
	}
	if !reg.IsModelAvailable("tenant-a", "groq/ok") || reg.IsModelAvailable("tenant-a", "groq/blocked") {
		t.Fatal("tenant-a availability mismatch")
	}
	if !reg.IsModelAvailable("tenant-b", "groq/blocked") {
		t.Fatal("unknown tenant should default available")
	}

	filtered := reg.FilterAvailable("tenant-a", []string{"groq/ok", "groq/blocked", "groq/also-ok"})
	if len(filtered) != 2 || filtered[0] != "groq/ok" || filtered[1] != "groq/also-ok" {
		t.Fatalf("filtered=%#v", filtered)
	}

	if err := reg.Reload(ctx, s); err != nil {
		t.Fatal(err)
	}
	if reg.Revision() != 2 {
		t.Fatalf("revision after reload=%d", reg.Revision())
	}
}
