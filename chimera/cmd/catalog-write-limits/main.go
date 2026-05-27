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
	defaultGateway = "config/gateway.yaml"
)

func main() {
	catalogPath := flag.String("catalog", defaultCatalog, "input catalog-available.snapshot.yaml (from make catalog-available)")
	limitsPath := flag.String("limits", defaultLimits, "provider-model-limits.yaml to patch in place")
	gatewayPath := flag.String("gateway", defaultGateway, "gateway.yaml with routing.fallback_chain models to ensure exist")
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

	var ensure []string
	if *gatewayPath != "" {
		if _, err := os.Stat(*gatewayPath); err == nil {
			ensure, err = cataloglimits.LoadFallbackChain(*gatewayPath)
			if err != nil {
				fmt.Fprintf(os.Stderr, "catalog-write-limits: %v\n", err)
				os.Exit(1)
			}
		}
	}

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
