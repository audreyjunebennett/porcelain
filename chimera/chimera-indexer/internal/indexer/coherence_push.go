package indexer

import (
	"context"
	"math/rand"
	"strings"
)

// PushStaleSources reports stale sources to the gateway per ingest scope.
func PushStaleSources(ctx context.Context, client *GatewayClient, cfg Resolved, gw *IndexerConfig, st *SyncState, roots []Root, reporter *CoherenceReporter) {
	if client == nil || st == nil || len(roots) == 0 {
		return
	}
	stale := CollectStaleSources(cfg, gw, st, roots)
	if len(stale) == 0 {
		return
	}
	type scopeKey struct {
		proj, flav string
	}
	byScope := map[scopeKey][]StaleSource{}
	for _, s := range stale {
		if reporter != nil {
			reporter.LogStale(s)
		}
		k := scopeKey{proj: strings.TrimSpace(s.ProjectID), flav: strings.TrimSpace(s.FlavorID)}
		byScope[k] = append(byScope[k], s)
	}
	pol := SessionRetryPolicy{MaxAttempts: 2, BaseDelay: 200, MaxDelay: 2}
	rng := rand.New(rand.NewSource(1))
	for k, list := range byScope {
		hdrs := map[string]string{}
		if k.proj != "" {
			hdrs["X-Chimera-Project"] = k.proj
		}
		if k.flav != "" {
			hdrs["X-Chimera-Flavor-Id"] = k.flav
		}
		entries := make([]CorpusStaleEntry, 0, len(list))
		for _, s := range list {
			entries = append(entries, CorpusStaleEntry{
				Source:        s.Source,
				IndexedSHA256: s.IndexedSHA256,
				LiveSHA256:    s.LiveSHA256,
			})
		}
		_ = client.PutCorpusStale(ctx, entries, hdrs, pol, rng)
	}
}
