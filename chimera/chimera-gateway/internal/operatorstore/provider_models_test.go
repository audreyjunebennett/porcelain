package operatorstore

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/lynn/porcelain/chimera/internal/config"
	"github.com/lynn/porcelain/chimera/internal/providerfreetier"
)

func writeFreeTierYAML(t *testing.T, dir string) string {
	t.Helper()
	p := filepath.Join(dir, "provider-free-tier.yaml")
	raw := `format_version: 1
effective_date: "2026-01-01"
models:
  - groq/free-model
  - gemini/gemini-flash
`
	if err := os.WriteFile(p, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestProviderModelAvailability_CRUDAndTenantIsolation(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	if err := s.ReplaceProviderModelAvailability(ctx, "tenant-a", "groq", map[string]bool{
		"groq/a": true,
		"groq/b": false,
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.ReplaceProviderModelAvailability(ctx, "tenant-b", "groq", map[string]bool{
		"groq/a": false,
	}); err != nil {
		t.Fatal(err)
	}

	availA, err := s.IsModelAvailableForTenant(ctx, "tenant-a", "groq/b")
	if err != nil || availA {
		t.Fatalf("tenant-a groq/b available=%v err=%v", availA, err)
	}
	availB, err := s.IsModelAvailableForTenant(ctx, "tenant-b", "groq/a")
	if err != nil || availB {
		t.Fatalf("tenant-b groq/a available=%v err=%v", availB, err)
	}
	availDefault, err := s.IsModelAvailableForTenant(ctx, "tenant-a", "groq/new-model")
	if err != nil || !availDefault {
		t.Fatalf("new model default available=%v err=%v", availDefault, err)
	}

	rows, err := s.ListProviderModelAvailability(ctx, "tenant-a", "groq")
	if err != nil || len(rows) != 2 {
		t.Fatalf("list tenant-a: len=%d err=%v", len(rows), err)
	}
	merged := MergeProviderModelAvailability([]string{"groq/a", "groq/b", "groq/c"}, rows)
	if len(merged) != 3 || !merged[0].Available || merged[1].Available || !merged[2].Available {
		t.Fatalf("merge=%+v", merged)
	}
	if merged[2].Explicit {
		t.Fatalf("expected implicit default for groq/c, got %+v", merged[2])
	}

	configured, err := s.ProviderModelConfigConfigured(ctx, "tenant-a", "groq")
	if err != nil || !configured {
		t.Fatalf("configured=%v err=%v", configured, err)
	}
	if err := s.ClearProviderModelAvailability(ctx, "tenant-a", "groq"); err != nil {
		t.Fatal(err)
	}
	configured, err = s.ProviderModelConfigConfigured(ctx, "tenant-a", "groq")
	if err != nil || configured {
		t.Fatalf("after clear configured=%v err=%v", configured, err)
	}
}

func TestBootstrapProviderModelAvailability_fromYAML(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	dir := t.TempDir()
	ftPath := writeFreeTierYAML(t, dir)

	res := &config.Resolved{ProviderFreeTierPath: ftPath}
	catalog := []string{
		"groq/free-model",
		"groq/paid-model",
		"gemini/gemini-flash",
		"gemini/gemini-pro",
		"ollama/llama3",
	}

	if err := BootstrapProviderModelAvailability(ctx, s, res, catalog, nil); err != nil {
		t.Fatal(err)
	}
	if err := BootstrapProviderModelAvailability(ctx, s, res, catalog, nil); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		model string
		want  bool
	}{
		{"groq/free-model", true},
		{"groq/paid-model", false},
		{"gemini/gemini-flash", true},
		{"gemini/gemini-pro", false},
		{"ollama/llama3", true},
	} {
		got, err := s.IsModelAvailableForTenant(ctx, "", tc.model)
		if err != nil || got != tc.want {
			t.Fatalf("%s available=%v want=%v err=%v", tc.model, got, tc.want, err)
		}
	}

	has, err := s.HasProviderModelAvailabilityRows(ctx)
	if err != nil || !has {
		t.Fatalf("has rows=%v err=%v", has, err)
	}
}

func TestAvailabilityFromFreeTierSpec_ollamaNoOp(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "p.yaml")
	if err := os.WriteFile(p, []byte(`format_version: 1
models:
  - groq/free
`), 0o644); err != nil {
		t.Fatal(err)
	}
	spec, err := providerfreetier.Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if got := AvailabilityFromFreeTierSpec(spec, "ollama", []string{"ollama/a"}); got != nil {
		t.Fatalf("ollama no-op: %#v", got)
	}
	groq := AvailabilityFromFreeTierSpec(spec, "groq", []string{"groq/free", "groq/paid"})
	if len(groq) != 2 || !groq["groq/free"] || groq["groq/paid"] {
		t.Fatalf("groq=%#v", groq)
	}
}

func TestLoadProviderModelAvailabilityIndex(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	if err := s.UpsertProviderModelAvailability(ctx, "t1", "groq", "groq/x", false, "{}"); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertProviderModelAvailability(ctx, "t2", "gemini", "gemini/y", false, "{}"); err != nil {
		t.Fatal(err)
	}
	idx, err := s.LoadProviderModelAvailabilityIndex(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := idx["t1"]["groq/x"]; !ok {
		t.Fatalf("index=%#v", idx)
	}
	if _, ok := idx["t2"]["gemini/y"]; !ok {
		t.Fatalf("index=%#v", idx)
	}
}
