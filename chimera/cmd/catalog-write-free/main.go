// Fetches public Groq + Gemini documentation, extracts model ids (Groq: full Free Plan limit columns),
// maps them to BiFrost-style provider/model strings, and writes YAML with source + limits in comments.
// Optional -provider-free-tier-out (with -intersect) writes gateway provider-free-tier.yaml shape including patterns ollama/*.
// make config-provider-free-tier runs catalog-available then this command with defaults.
package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/lynn/porcelain/chimera/internal/freecatalog"
)

const (
	defaultGroqURL   = "https://console.groq.com/docs/rate-limits"
	defaultGeminiURL = "https://ai.google.dev/gemini-api/docs/pricing"
	defaultOut       = "config/free-tier-catalog.snapshot.yaml"
)

func main() {
	outPath := flag.String("out", defaultOut, "output YAML path")
	groqURL := flag.String("groq-url", defaultGroqURL, "Groq rate limits documentation URL")
	geminiURL := flag.String("gemini-url", defaultGeminiURL, "Gemini API pricing documentation URL")
	intersectPath := flag.String("intersect", "", "optional BiFrost models list (JSON or YAML, OpenAI-style data[].id); keep only entries that fuzzy-match, align ids to catalog spelling")
	providerFreeTierOut := flag.String("provider-free-tier-out", "", "optional path to write config/provider-free-tier.yaml (format_version + effective_date + models + patterns); use with -intersect for groq/gemini ids from catalog")
	ollamaPattern := flag.Bool("ollama-pattern", true, "when -provider-free-tier-out is set, add patterns entry ollama/* (all upstream ollama/... ids when filter is on)")
	extraPatterns := flag.String("extra-patterns", "", "comma-separated extra patterns for -provider-free-tier-out (in addition to -ollama-pattern when true)")
	effectiveDate := flag.String("effective-date", "", "YYYY-MM-DD for provider-free-tier-out effective_date (default: today UTC)")
	timeout := flag.Duration("timeout", 60*time.Second, "per-fetch timeout")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), *timeout*3)
	defer cancel()

	client := &http.Client{Timeout: *timeout}

	groqBody, err := freecatalog.FetchURL(ctx, client, *groqURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "catalog-write-free: fetch groq: %v\n", err)
		os.Exit(1)
	}
	gemBody, err := freecatalog.FetchURL(ctx, client, *geminiURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "catalog-write-free: fetch gemini: %v\n", err)
		os.Exit(1)
	}

	var entries []freecatalog.Entry
	for _, row := range freecatalog.ParseGroqRateLimitRows(string(groqBody)) {
		bf := freecatalog.ToGroqBiFrost(row.SourceID)
		if bf == "" {
			continue
		}
		lim := row.GroqLimits
		entries = append(entries, freecatalog.Entry{
			Provider:   "groq",
			SourceID:   row.SourceID,
			BiFrostID:  bf,
			SourcePage: *groqURL,
			Groq:       &lim,
		})
	}
	for _, src := range freecatalog.ParseGeminiPricingFreeInputModels(string(gemBody)) {
		bf := freecatalog.ToGeminiBiFrost(src)
		if bf == "" {
			continue
		}
		entries = append(entries, freecatalog.Entry{
			Provider:   "gemini",
			SourceID:   src,
			BiFrostID:  bf,
			SourcePage: *geminiURL,
		})
	}
	if len(entries) == 0 {
		fmt.Fprintf(os.Stderr, "catalog-write-free: no models extracted (page layout may have changed)\n")
		os.Exit(1)
	}

	if *intersectPath != "" {
		raw, err := os.ReadFile(*intersectPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "catalog-write-free: read intersect catalog: %v\n", err)
			os.Exit(1)
		}
		catIDs, err := freecatalog.ParseCatalogIntersect(raw)
		if err != nil {
			fmt.Fprintf(os.Stderr, "catalog-write-free: parse intersect catalog: %v\n", err)
			os.Exit(1)
		}
		entries = freecatalog.FilterEntriesByCatalog(entries, catIDs)
		if len(entries) == 0 {
			fmt.Fprintf(os.Stderr, "catalog-write-free: intersect removed all models (check intersect file or parsing)\n")
			os.Exit(1)
		}
		entries = freecatalog.AlignEntriesToCatalog(entries, catIDs)
	}

	n, err := freecatalog.WriteCatalogYAML(*outPath, time.Now(), *groqURL, *geminiURL, entries)
	if err != nil {
		fmt.Fprintf(os.Stderr, "catalog-write-free: write: %v\n", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "catalog-write-free: wrote %d models -> %s\n", n, *outPath)

	if strings.TrimSpace(*providerFreeTierOut) != "" {
		if strings.TrimSpace(*intersectPath) == "" {
			fmt.Fprintf(os.Stderr, "catalog-write-free: -provider-free-tier-out requires -intersect (catalog snapshot) so groq/gemini ids match your BiFrost listing\n")
			os.Exit(1)
		}
		var patterns []string
		if *ollamaPattern {
			patterns = append(patterns, "ollama/*")
		}
		for _, p := range strings.Split(*extraPatterns, ",") {
			p = strings.TrimSpace(p)
			if p != "" {
				patterns = append(patterns, p)
			}
		}
		ed := time.Now().UTC()
		if s := strings.TrimSpace(*effectiveDate); s != "" {
			t, err := time.Parse("2006-01-02", s)
			if err != nil {
				fmt.Fprintf(os.Stderr, "catalog-write-free: -effective-date: %v\n", err)
				os.Exit(1)
			}
			ed = t
		}
		if err := freecatalog.WriteProviderFreeTierYAML(*providerFreeTierOut, ed, entries, patterns); err != nil {
			fmt.Fprintf(os.Stderr, "catalog-write-free: write provider-free-tier: %v\n", err)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "catalog-write-free: wrote provider-free-tier -> %s\n", *providerFreeTierOut)
	}
}
