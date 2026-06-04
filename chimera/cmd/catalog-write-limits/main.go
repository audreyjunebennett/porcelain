// Seeds context_window in provider-model-limits.yaml from catalog-available.snapshot.yaml.
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/lynn/porcelain/chimera/internal/cataloglimits"
	"github.com/lynn/porcelain/chimera/internal/providerlimits"
)

const (
	defaultCatalog = "config/catalog-available.snapshot.yaml"
	defaultLimits  = "config/provider-model-limits.yaml"
)

func main() {
	catalogPath := flag.String("catalog", defaultCatalog, "input catalog-available.snapshot.yaml (from make catalog-available)")
	limitsPath := flag.String("limits", defaultLimits, "provider-model-limits.yaml to patch in place")
	ensureFlag := flag.String("ensure", "", "comma-separated upstream model ids to ensure exist in limits (may repeat)")
	ensureFromCatalog := flag.Bool("ensure-from-catalog", false, "ensure every model id listed in the catalog snapshot")
	operatorSQLite := flag.String("operator-sqlite", "", "optional operator SQLite path; ensure models from enabled virtual model fallback chains")
	force := flag.Bool("force", false, "overwrite existing context_window values")
	flag.Parse()

	if _, err := os.Stat(*catalogPath); err != nil {
		fmt.Fprintf(os.Stderr, "catalog-write-limits: catalog file: %v\n", err)
		os.Exit(1)
	}

	catalog, err := cataloglimits.LoadCatalogContextLengths(*catalogPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "catalog-write-limits: %v\n", err)
		os.Exit(1)
	}

	cfg, err := providerlimits.Load(*limitsPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "catalog-write-limits: load limits: %v\n", err)
		os.Exit(1)
	}

	var ensureParts [][]string
	if ids := parseEnsureList(*ensureFlag); len(ids) > 0 {
		ensureParts = append(ensureParts, ids)
	}
	if *ensureFromCatalog {
		ids, err := cataloglimits.CatalogModelIDs(*catalogPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "catalog-write-limits: catalog ensure: %v\n", err)
			os.Exit(1)
		}
		ensureParts = append(ensureParts, ids)
	}
	if strings.TrimSpace(*operatorSQLite) != "" {
		ids, err := cataloglimits.LoadEnsureModelsFromOperatorSQLite(*operatorSQLite)
		if err != nil {
			fmt.Fprintf(os.Stderr, "catalog-write-limits: operator sqlite: %v\n", err)
			os.Exit(1)
		}
		ensureParts = append(ensureParts, ids)
	}
	ensure := cataloglimits.MergeEnsureModels(ensureParts...)

	rep := cataloglimits.ApplyContextWindows(cfg, catalog, ensure, cataloglimits.ApplyOptions{Force: *force})
	for _, id := range rep.Skipped {
		fmt.Fprintf(os.Stderr, "catalog-write-limits: skip %s (no context_length in catalog; no ollama default)\n", id)
	}

	if err := cataloglimits.WriteLimitsFile(*limitsPath, cfg); err != nil {
		fmt.Fprintf(os.Stderr, "catalog-write-limits: write: %v\n", err)
		os.Exit(1)
	}

	fmt.Fprintf(os.Stderr, "catalog-write-limits: wrote %s (updated=%d added=%d skipped=%d)\n",
		*limitsPath, len(rep.Updated), len(rep.Added), len(rep.Skipped))
	if len(rep.Updated) > 0 {
		fmt.Fprintf(os.Stderr, "  updated: %s\n", strings.Join(rep.Updated, ", "))
	}
	if len(rep.Added) > 0 {
		fmt.Fprintf(os.Stderr, "  added: %s\n", strings.Join(rep.Added, ", "))
	}
}

func parseEnsureList(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var out []string
	for _, part := range strings.Split(raw, ",") {
		if id := strings.TrimSpace(part); id != "" {
			out = append(out, id)
		}
	}
	return out
}
