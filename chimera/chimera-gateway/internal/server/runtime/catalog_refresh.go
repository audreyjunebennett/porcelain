package runtime

import (
	"context"
	"log/slog"
	"time"

	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/operatorstore"
	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/server/catalog"
	"github.com/lynn/porcelain/chimera/internal/config"
)

// HealthTimeout returns the configured upstream health probe timeout.
func HealthTimeout(res *config.Resolved) time.Duration {
	if res == nil {
		return 2 * time.Second
	}
	return time.Duration(res.HealthTimeoutMs) * time.Millisecond
}

func healthTimeout(res *config.Resolved) time.Duration {
	return HealthTimeout(res)
}

// RefreshAvailableModels polls chimera-broker `/v1/models`, stores the result on the runtime, emits
// the catalog slog line, and runs registered catalog auditors.
func RefreshAvailableModels(ctx context.Context, rt *Runtime, log *slog.Logger) *catalog.CatalogSnapshot {
	if rt == nil {
		return nil
	}
	apiKey := rt.UpstreamAPIKey()
	res, _ := rt.Snapshot()
	snap := catalog.BuildSnapshot(ctx, res, apiKey, healthTimeout(res), log)
	rt.SetCatalogSnapshot(snap)
	catalog.EmitAvailableModelsLog(snap, log)
	if snap != nil && snap.OK {
		rt.mu.RLock()
		store := rt.operator
		rt.mu.RUnlock()
		if store != nil {
			if err := operatorstore.BootstrapProviderModelAvailability(ctx, store, res, snap.ModelIDs, log); err != nil && log != nil {
				log.Warn("provider model availability bootstrap failed", "msg", "gateway.provider_models.bootstrap_failed", "err", err)
			} else if err := rt.ReloadProviderModelAvailability(ctx); err != nil && log != nil {
				log.Warn("provider model registry reload failed", "msg", "gateway.provider_models.reload_failed", "err", err)
			}
		}
	}
	for _, a := range catalog.SnapshotAuditors() {
		func() {
			defer func() {
				if r := recover(); r != nil && log != nil {
					log.Error("catalog auditor panicked", "msg", "gateway.catalog.auditor_panic", "panic", r)
				}
			}()
			a(ctx, snap, res, log)
		}()
	}
	return snap
}

// LogBrokerAvailableModelsForLogsUI refreshes and logs the merged chimera-broker model catalog.
func LogBrokerAvailableModelsForLogsUI(ctx context.Context, rt *Runtime, log *slog.Logger) {
	_ = RefreshAvailableModels(ctx, rt, log)
}

// StartCatalogPoller runs RefreshAvailableModels on interval until ctx is cancelled.
func StartCatalogPoller(ctx context.Context, rt *Runtime, log *slog.Logger, interval time.Duration) {
	if rt == nil || interval <= 0 {
		return
	}
	go func() {
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
