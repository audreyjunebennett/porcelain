// Package server: live Chimera Broker catalog snapshot — periodic poll of `/v1/models` cached on
// the gateway runtime so health, routing-rule auditors, fallback-chain checks, and embedding /
// router model presence checks can read one consistent picture.
//
// The supervised `chimera-broker-http` subprocess prunes providers from `/v1/models` as soon as it
// can't reach the underlying upstream (e.g. the local ollama daemon being down removes every
// `ollama/...` model from the response). That makes the merged model list a strong runtime
// liveness signal — much stronger than the static `GET /api/providers/{name}` config view —
// and a natural anchor for catalog-driven health checks.
//
// Future auditors should attach via [RegisterCatalogAuditor]: they receive the freshly built
// snapshot plus the resolved gateway config and run after every refresh.
package server

import (
	"context"
	"encoding/json"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/lynn/porcelain/chimera/internal/config"
	"github.com/lynn/porcelain/chimera/internal/upstream"
)

// Successful merged-catalog polls log at Info only for the first success in a process;
// later polls use Debug so default operator streams are not dominated by the catalog interval
// (log-gateway.md P6 demotion).
var (
	catalogMergeLogMu       sync.Mutex
	catalogMergeInfoEmitted bool
)

// CatalogSnapshot captures one point-in-time view of BiFrost's merged model catalog.
//
// Consumers MUST treat the value as read-only — copies returned from [Runtime.CatalogSnapshot]
// share the same backing slices and maps. A nil snapshot means "no poll has succeeded yet";
// callers should fall back to config-only behavior in that case.
type CatalogSnapshot struct {
	// FetchedAt is when the snapshot was constructed (UTC, monotonic-friendly via time.Now).
	FetchedAt time.Time
	// OK is true when the upstream `/v1/models` call returned a parseable response.
	OK bool
	// CatalogModelCount is `1 + len(filtered upstream data)` — same shape as the gateway home
	// page and the historical `chat.chimera-broker.available_models` count.
	CatalogModelCount int
	// Providers is the distinct set of provider prefixes derived from the model ids
	// (ASCII-sorted). When ollama is offline, ollama disappears from this list because
	// BiFrost stops listing its models in `/v1/models`.
	Providers   []string
	providerSet map[string]struct{}
	// ModelIDs is the full filtered upstream id list (without the virtual Chimera id),
	// retained so future auditors can answer "is model X in the live catalog?" without
	// re-fetching.
	ModelIDs []string
	modelSet map[string]struct{}
	// FetchErr is a short error string when OK is false (e.g. transport failure). Empty otherwise.
	FetchErr string
}

// HasProvider reports whether the live catalog includes any model under the given provider id.
// Returns false on a nil snapshot.
func (s *CatalogSnapshot) HasProvider(id string) bool {
	if s == nil {
		return false
	}
	_, ok := s.providerSet[strings.TrimSpace(id)]
	return ok
}

// HasModel reports whether the live catalog includes the given upstream model id.
// Returns false on a nil snapshot.
func (s *CatalogSnapshot) HasModel(id string) bool {
	if s == nil {
		return false
	}
	_, ok := s.modelSet[strings.TrimSpace(id)]
	return ok
}

// IsFresh reports whether the snapshot was taken within maxAge of now. Use this before trusting
// "absence" as a real signal — a stale snapshot is just historical evidence, not current state.
func (s *CatalogSnapshot) IsFresh(now time.Time, maxAge time.Duration) bool {
	if s == nil || s.FetchedAt.IsZero() || maxAge <= 0 {
		return false
	}
	return now.Sub(s.FetchedAt) <= maxAge
}

// CatalogSnapshotFreshness is the default staleness window used by health classifiers when the
// caller doesn't override it. Two minutes covers a 30s ticker dropping a few consecutive calls.
const CatalogSnapshotFreshness = 2 * time.Minute

// CatalogAuditor is the extension point for the future "did this catalog change break my
// config?" checks the operator wants. Each registered auditor runs once per refresh, in
// registration order, with the freshly published snapshot. Auditors should be cheap and
// allocate their own logger fields; they MUST NOT mutate the snapshot.
//
// Planned auditors (not yet implemented; this hook is here so they can land without touching
// the polling loop):
//   - Routing-policy rules referencing models no longer present.
//   - Fallback chain emptied after intersection with the live catalog.
//   - RAG embedding model missing from the live catalog.
//   - Tool-router models missing from the live catalog.
type CatalogAuditor func(ctx context.Context, snap *CatalogSnapshot, res *config.Resolved, log *slog.Logger)

var (
	catalogAuditorsMu sync.RWMutex
	catalogAuditors   []CatalogAuditor
)

// RegisterCatalogAuditor appends an auditor to the chain that runs after every refresh.
// Safe to call from package init() in feature packages or from main during startup wiring.
func RegisterCatalogAuditor(a CatalogAuditor) {
	if a == nil {
		return
	}
	catalogAuditorsMu.Lock()
	catalogAuditors = append(catalogAuditors, a)
	catalogAuditorsMu.Unlock()
}

func snapshotAuditors() []CatalogAuditor {
	catalogAuditorsMu.RLock()
	defer catalogAuditorsMu.RUnlock()
	if len(catalogAuditors) == 0 {
		return nil
	}
	out := make([]CatalogAuditor, len(catalogAuditors))
	copy(out, catalogAuditors)
	return out
}

// buildCatalogSnapshot calls BiFrost `/v1/models` and shapes the response into a
// [CatalogSnapshot]. It does NOT log; callers (e.g. [RefreshAvailableModels]) are responsible
// for emitting `chat.chimera-broker.available_models`. Pure of cache writes / runtime mutation so it
// can be reused by tests and ad-hoc callers.
func buildCatalogSnapshot(ctx context.Context, res *config.Resolved, apiKey string, timeout time.Duration, log *slog.Logger) *CatalogSnapshot {
	out := &CatalogSnapshot{FetchedAt: time.Now().UTC()}
	if res == nil {
		out.FetchErr = "gateway config not resolved"
		return out
	}
	if strings.TrimSpace(apiKey) == "" {
		out.FetchErr = "missing upstream API key"
		return out
	}
	st, body, fetchOK := upstream.FetchOpenAIModels(ctx, res.UpstreamBaseURL, apiKey, timeout, log)
	if !fetchOK {
		out.FetchErr = "fetch /v1/models failed (status=" + httpStatusOrDash(st) + ")"
		return out
	}
	var list map[string]any
	if err := json.Unmarshal(body, &list); err != nil {
		out.FetchErr = "parse /v1/models: " + err.Error()
		return out
	}
	data, _ := list["data"].([]any)
	if data == nil {
		data = []any{}
	}
	data = filterOpenAIModelDataByFreeTier(data, res)

	provSet := map[string]struct{}{}
	modelSet := map[string]struct{}{}
	modelIDs := make([]string, 0, len(data))
	for _, raw := range data {
		m, mOK := raw.(map[string]any)
		if !mOK {
			continue
		}
		id, _ := m["id"].(string)
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		modelIDs = append(modelIDs, id)
		modelSet[id] = struct{}{}
		prov := ""
		if slash := strings.Index(id, "/"); slash > 0 {
			prov = id[:slash]
		} else if ob, ok := m["owned_by"].(string); ok {
			prov = strings.TrimSpace(ob)
		}
		if prov != "" {
			provSet[prov] = struct{}{}
		}
	}
	provs := make([]string, 0, len(provSet))
	for p := range provSet {
		provs = append(provs, p)
	}
	sort.Strings(provs)
	sort.Strings(modelIDs)

	out.OK = true
	out.CatalogModelCount = 1 + len(modelIDs) // virtual Chimera id + filtered upstream
	out.Providers = provs
	out.providerSet = provSet
	out.ModelIDs = modelIDs
	out.modelSet = modelSet
	return out
}

func httpStatusOrDash(st int) string {
	if st <= 0 {
		return "—"
	}
	return strings.TrimSpace(itoaShort(st))
}

func itoaShort(n int) string {
	// Avoid pulling strconv just for this; status codes are always 3 digits and small.
	if n == 0 {
		return "0"
	}
	buf := [4]byte{}
	i := len(buf)
	for n > 0 && i > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

// RefreshAvailableModels polls BiFrost `/v1/models`, stores the result on the runtime, emits
// the `chat.chimera-broker.available_models` slog line, and runs every registered catalog auditor.
//
// Safe to call from startup, periodic tickers, and HTTP request handlers concurrently —
// stores are atomic and auditors run on the calling goroutine. Returns the published snapshot
// so callers (tests / one-shot triggers) can act on it directly.
func RefreshAvailableModels(ctx context.Context, rt *Runtime, log *slog.Logger) *CatalogSnapshot {
	if rt == nil {
		return nil
	}
	apiKey := rt.UpstreamAPIKey()
	res, _, _ := rt.Snapshot()
	timeout := healthTimeout(res)
	snap := buildCatalogSnapshot(ctx, res, apiKey, timeout, log)
	rt.SetCatalogSnapshot(snap)
	emitAvailableModelsLog(snap, log)
	if auditors := snapshotAuditors(); len(auditors) > 0 {
		for _, a := range auditors {
			func() {
				defer func() {
					if r := recover(); r != nil && log != nil {
						log.Error("catalog auditor panicked", "msg", "gateway.catalog.auditor_panic", "panic", r)
					}
				}()
				a(ctx, snap, res, log)
			}()
		}
	}
	return snap
}

func emitAvailableModelsLog(snap *CatalogSnapshot, log *slog.Logger) {
	if log == nil || snap == nil {
		return
	}
	args := []any{
		"msg", "chat.chimera-broker.available_models",
		"service", "gateway",
		"ok", snap.OK,
		"fetched_at", snap.FetchedAt.UTC().Format(time.RFC3339),
	}
	if snap.OK {
		args = append(args,
			"catalog_model_count", snap.CatalogModelCount,
			"providers", snap.Providers,
			"provider_count", len(snap.Providers),
			"model_count", len(snap.ModelIDs),
		)
		catalogMergeLogMu.Lock()
		infoOnce := !catalogMergeInfoEmitted
		catalogMergeInfoEmitted = true
		catalogMergeLogMu.Unlock()
		if infoOnce {
			log.Info("upstream models (merged list)", args...)
		} else {
			log.Debug("upstream models (merged list)", args...)
		}
		return
	}
	args = append(args, "err", snap.FetchErr)
	log.Warn("upstream models unavailable", args...)
}

// LogUpstreamAvailableModelsForLogsUI is the historical entry point still used by callers that
// only want the side-effect (logging) without holding onto the snapshot. New call sites should
// prefer [RefreshAvailableModels] directly.
func LogUpstreamAvailableModelsForLogsUI(ctx context.Context, rt *Runtime, log *slog.Logger) {
	_ = RefreshAvailableModels(ctx, rt, log)
}

// StartCatalogPoller runs RefreshAvailableModels on a fixed interval until ctx is cancelled.
// Designed to run as a single goroutine started from `chimera serve`. The first refresh fires
// immediately so the snapshot cache is populated before downstream consumers ask for it.
//
// interval <= 0 disables polling.
func StartCatalogPoller(ctx context.Context, rt *Runtime, log *slog.Logger, interval time.Duration) {
	if rt == nil || interval <= 0 {
		return
	}
	go func() {
		// First refresh: immediate, with a per-call timeout so a slow BiFrost can't pin
		// startup. 25s mirrors the bound used in cmd/chimera/serve.go for the prior one-shot.
		runOnce := func() {
			callCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
			defer cancel()
			RefreshAvailableModels(callCtx, rt, log)
		}
		runOnce()
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				runOnce()
			}
		}
	}()
}
