package freecatalog

import (
	"context"
	"fmt"
	"net/http"
)

// FetchPublicEntries downloads and parses the current public Groq free-plan limits and Gemini
// pricing pages. It does not assert account access; callers must intersect with a live catalog.
func FetchPublicEntries(ctx context.Context, client *http.Client, groqURL, geminiURL string) ([]Entry, error) {
	groqBody, err := FetchURL(ctx, client, groqURL)
	if err != nil {
		return nil, fmt.Errorf("fetch groq: %w", err)
	}
	geminiBody, err := FetchURL(ctx, client, geminiURL)
	if err != nil {
		return nil, fmt.Errorf("fetch gemini: %w", err)
	}

	var entries []Entry
	for _, row := range ParseGroqRateLimitRows(string(groqBody)) {
		bf := ToGroqBiFrost(row.SourceID)
		if bf == "" {
			continue
		}
		limits := row.GroqLimits
		entries = append(entries, Entry{
			Provider:   "groq",
			SourceID:   row.SourceID,
			BiFrostID:  bf,
			SourcePage: groqURL,
			Groq:       &limits,
		})
	}
	for _, sourceID := range ParseGeminiPricingFreeInputModels(string(geminiBody)) {
		bf := ToGeminiBiFrost(sourceID)
		if bf == "" {
			continue
		}
		entries = append(entries, Entry{
			Provider:   "gemini",
			SourceID:   sourceID,
			BiFrostID:  bf,
			SourcePage: geminiURL,
		})
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("no models extracted (provider page layout may have changed)")
	}
	return entries, nil
}
