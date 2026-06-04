// chimera-indexer is the v0.4 workspace file indexer for the Chimera gateway runtime.
//
// It walks configured roots, applies .chimeraignore + .gitignore + binary
// detection, hashes whole files, and POSTs them to /v1/ingest. Watching uses
// fsnotify for incremental updates.
//
// Usage:
//
//	chimera-indexer --config .locus/indexer.config.yaml [--root path]... [--gateway-url URL]
//
// Environment:
//
//	CHIMERA_GATEWAY_URL base URL of the gateway (default port 3000)
//	CHIMERA_GATEWAY_TOKEN bearer token; must equal a secret: entry in
//	                     config/api-keys.yaml on the gateway side
//
// On startup the binary loads `env` and then `.env` (later wins) from the
// current working directory, mirroring the main `chimera` binary so operators
// can keep one secrets file for both.
//
// When --config names an explicit YAML layer (desktop supervised mode),
// saves to that file apply YAML tuning in-process. Watch directories come from
// GET /v1/indexer/workspaces (operator SQLite), not from YAML roots.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/joho/godotenv"
	"github.com/lynn/porcelain/chimera/chimera-indexer/adapter"
	idxconfig "github.com/lynn/porcelain/chimera/chimera-indexer/internal/config"
	"github.com/lynn/porcelain/chimera/chimera-indexer/internal/indexer"
	"github.com/lynn/porcelain/chimera/internal/logfmt"
	"github.com/lynn/porcelain/chimera/internal/wrapper/contract"
	wruntime "github.com/lynn/porcelain/chimera/internal/wrapper/runtime"
)

var errSupervisedReload = errors.New("indexer supervised config hot-reload")

const supervisedWatchShutdownGrace = 40 * time.Second

func materializeSupervisedRoots(ctx context.Context, log *slog.Logger, cfg *indexer.Resolved) bool {
	if cfg == nil || !cfg.SupervisedLayer {
		return true
	}
	cl := indexer.NewGatewayClient(cfg.GatewayURL, cfg.Token, cfg.RequestTimeout)
	if err := indexer.MaterializeRootsFromGateway(ctx, cl, cfg, indexer.RetryPolicyFromResolved(*cfg)); err != nil {
		if log != nil {
			log.Warn("gateway workspaces fetch failed",
				"err", err,
				"msg", "indexer.supervised.workspaces_apply_failed",
				"type", "indexer.supervised.workspaces_apply_failed",
			)
		}
		return false
	}
	return true
}

type rootList []string

func (r *rootList) String() string { return strings.Join(*r, ",") }
func (r *rootList) Set(v string) error {
	*r = append(*r, v)
	return nil
}

func main() {
	args := os.Args[1:]
	if len(args) > 0 && (args[0] == "-h" || args[0] == "--help") {
		idxconfig.PrintHelp()
		return
	}
	if len(args) > 0 && args[0] == "--indexer-backend" {
		if err := runBackend(args[1:]); err != nil {
			fmt.Fprintln(os.Stderr, "chimera-indexer backend:", err)
			os.Exit(1)
		}
		return
	}
	if err := run(args); err != nil {
		fmt.Fprintln(os.Stderr, "chimera-indexer:", err)
		os.Exit(exitCodeForError(err))
	}
}

func drainReloadSignals(ch <-chan struct{}) {
	for {
		select {
		case <-ch:
		default:
			return
		}
	}
}

func runOneShot(parentCtx context.Context, cfg indexer.Resolved, logJSON bool, baseLog *slog.Logger) error {
	if !materializeSupervisedRoots(parentCtx, baseLog, &cfg) {
		return fmt.Errorf("supervised indexer: could not load workspace directories from gateway")
	}
	if cfg.SupervisedLayer && len(cfg.Roots) == 0 {
		return fmt.Errorf("supervised indexer: no workspace directories from gateway (GET /v1/indexer/workspaces); add paths in /ui/settings workspaces")
	}
	runID := uuid.NewString()
	log := attachSessionLogger(logJSON, baseLog, runID)
	client := indexer.NewGatewayClient(cfg.GatewayURL, cfg.Token, cfg.RequestTimeout)
	client.IndexRunID = runID

	ix := indexer.New(cfg, client, log)
	defer ix.Close()
	ix.SetRunWatchMode(false)
	if _, err := ix.FetchAndLogConfig(parentCtx); err != nil {
		var he *indexer.HTTPError
		if errors.As(err, &he) && he.Status == 503 && strings.Contains(strings.ToLower(he.Body), "rag is not enabled") {
			return fmt.Errorf("gateway at %s has RAG disabled — set rag.enabled=true in config/gateway.yaml and restart the gateway", cfg.GatewayURL)
		}
		log.Warn("continuing despite config fetch failure", "err", err)
	}
	ix.LogIndexerRunStart()
	if !ix.ScheduleInitialScan() {
		return fmt.Errorf("could not schedule initial scan (queue closed)")
	}

	drainCtx, drainCancel := context.WithCancel(parentCtx)
	go func() {
		for {
			if ix.Queue().Len() == 0 {
				drainCancel()
				return
			}
			select {
			case <-parentCtx.Done():
				drainCancel()
				return
			default:
			}
		}
	}()
	ix.RunWorkers(drainCtx)
	ix.Queue().Close()
	ix.EmitStorageStatsAndState(parentCtx, false)
	ix.FlushSkipSummaries()
	ix.FlushIngestSummaries()
	log.Info("indexer run done", indexer.RunDoneAttrs("one-shot", ix.OpsSnapshot())...)
	return nil
}

func attachSessionLogger(logJSON bool, baseLog *slog.Logger, runID string) *slog.Logger {
	if baseLog == nil {
		baseLog = indexer.StderrLogger(logJSON, slog.LevelInfo)
	}
	return baseLog.With("index_run_id", runID, "service", "indexer")
}

func waitWatchersShutdown(sessionCtx context.Context, log *slog.Logger, watchDone <-chan error) error {
	select {
	case errW := <-watchDone:
		return errW
	case <-sessionCtx.Done():
		timer := time.NewTimer(supervisedWatchShutdownGrace)
		defer timer.Stop()
		select {
		case errW := <-watchDone:
			return errW
		case <-timer.C:
			if log != nil {
				log.Error("filesystem watchers did not stop after shutdown was requested",
					"msg", "indexer.supervised.watch_shutdown_timeout",
					"type", "indexer.supervised.watch_shutdown_timeout",
					"grace_sec", int(supervisedWatchShutdownGrace.Seconds()),
					"cause", context.Cause(sessionCtx),
				)
			}
			return fmt.Errorf("watch shutdown timed out after %s: %w", supervisedWatchShutdownGrace, context.Cause(sessionCtx))
		}
	}
}

func runSupervisedLongSession(
	sessionCtx context.Context,
	cfg indexer.Resolved,
	processRunID string,
	activeIndexer *atomic.Pointer[indexer.Indexer],
	logJSON bool,
	logLevel string,
	hotReloadCount int,
	baseLog *slog.Logger,
) error {
	if strings.TrimSpace(logLevel) != "" {
		cfg.LogLevel = indexer.ParseLogLevel(logLevel)
	}
	sessionBase := indexer.StderrLogger(logJSON, cfg.LogLevel)
	log := attachSessionLogger(logJSON, sessionBase, processRunID)

	client := indexer.NewGatewayClient(cfg.GatewayURL, cfg.Token, cfg.RequestTimeout)
	client.IndexRunID = processRunID

	ix := indexer.New(cfg, client, log)
	activeIndexer.Store(ix)
	defer func() {
		activeIndexer.Store(nil)
		ix.Close()
	}()
	ix.SetRunWatchMode(true)

	if _, err := ix.FetchAndLogConfig(sessionCtx); err != nil {
		var he *indexer.HTTPError
		if errors.As(err, &he) && he.Status == 503 && strings.Contains(strings.ToLower(he.Body), "rag is not enabled") {
			return fmt.Errorf("gateway at %s has RAG disabled — set rag.enabled=true in config/gateway.yaml and restart the gateway", cfg.GatewayURL)
		}
		log.Warn("continuing despite config fetch failure", "err", err)
	}
	if hotReloadCount > 0 {
		log.Info("indexer supervised YAML tuning reloaded",
			"msg", "indexer.supervised.hot_reload",
			"type", "indexer.supervised.hot_reload",
			"n", hotReloadCount,
		)
	}
	ix.LogIndexerRunStart()
	if !ix.ScheduleInitialScan() {
		return fmt.Errorf("could not schedule initial scan (queue closed)")
	}

	doneWorkers := make(chan struct{})
	watchDone := make(chan error, 1)
	go func() { defer close(doneWorkers); ix.RunWorkers(sessionCtx) }()
	go ix.RunObservationLoop(sessionCtx, true)
	go func() { watchDone <- ix.RunWatchers(sessionCtx) }()

	errW := waitWatchersShutdown(sessionCtx, log, watchDone)
	ix.Queue().Close()
	<-doneWorkers
	if errW != nil {
		log.Error("watcher exited", "err", errW)
	}
	if sessionCtx.Err() != nil {
		if errW != nil {
			return errW
		}
		return sessionCtx.Err()
	}
	if errW != nil {
		return errW
	}
	ix.FlushSkipSummaries()
	ix.FlushIngestSummaries()
	log.Info("indexer run stopped", indexer.RunDoneAttrs("watch", ix.OpsSnapshot())...)
	return nil
}

func resolveSupervisedCfg(wd, cfgFlag, gatewayURL string, roots rootList) (indexer.Resolved, error) {
	fc, err := indexer.LoadLayeredConfig(wd, cfgFlag)
	if err != nil {
		return indexer.Resolved{}, err
	}
	ov := indexer.Overrides{
		GatewayURL:         gatewayURL,
		Roots:              roots,
		ExplicitConfigPath: cfgFlag,
		AllowEmptyRoots:    true,
	}
	return indexer.Resolve(fc, os.Getenv, ov)
}

func runWatchWithHotReload(ctx context.Context, wd string, absSupervisedCfg string, cfgFlag string, gatewayURL string, roots rootList, logJSON bool, logLevel string, baseLog *slog.Logger) error {
	processRunID := uuid.NewString()
	baseLog = attachSessionLogger(logJSON, baseLog, processRunID)

	reloadCh := make(chan struct{}, 1)
	signalTuningReload := func() {
		select {
		case reloadCh <- struct{}{}:
		default:
		}
	}
	go func() {
		werr := indexer.WatchConfigPathForReload(ctx, absSupervisedCfg, indexer.DefaultConfigReloadDebounce, signalTuningReload, baseLog)
		if werr != nil && ctx.Err() == nil && !errors.Is(werr, context.Canceled) {
			baseLog.Warn("indexer supervised config watch ended", "err", werr)
		}
	}()

	var wsFpMu sync.Mutex
	var wsFingerprint string
	reindexTracker := indexer.NewReindexTracker()
	coherenceReporter := indexer.NewCoherenceReporter(baseLog)
	var activeIndexer atomic.Pointer[indexer.Indexer]

	var pollCancel context.CancelFunc
	var pollWG sync.WaitGroup
	startWorkspacePoll := func(interval time.Duration) {
		if pollCancel != nil {
			pollCancel()
			pollWG.Wait()
		}
		pollCtx, cancel := context.WithCancel(ctx)
		pollCancel = cancel
		pollWG.Add(1)
		go func() {
			defer pollWG.Done()
			runWorkspacePoll(pollCtx, wd, cfgFlag, gatewayURL, baseLog, interval, &wsFpMu, &wsFingerprint, reindexTracker, coherenceReporter, &activeIndexer)
		}()
	}
	defer func() {
		if pollCancel != nil {
			pollCancel()
			pollWG.Wait()
		}
	}()

	hotN := 0
	var cfg indexer.Resolved
	for {
		var err error
		cfg, err = resolveSupervisedCfg(wd, cfgFlag, gatewayURL, roots)
		if err != nil {
			return err
		}
		if !materializeSupervisedRoots(ctx, baseLog, &cfg) {
			wsFpMu.Lock()
			wsFingerprint = ""
			wsFpMu.Unlock()
		} else {
			wsFpMu.Lock()
			wsFingerprint = indexer.RootsSnapshotFingerprint(cfg.Roots)
			wsFpMu.Unlock()
		}
		if len(cfg.Roots) > 0 {
			break
		}
		baseLog.Debug(
			"supervised indexer waiting for at least one workspace path from the gateway (GET /v1/indexer/workspaces)",
			"msg", "indexer.supervised.wait_roots",
			"type", "indexer.supervised.wait_roots",
			"config_path", absSupervisedCfg,
		)
		if !waitReloadOrCtxTimeout(ctx, reloadCh, 15*time.Second) {
			return ctx.Err()
		}
	}

	sessionCtx, sessionCancel := context.WithCancel(ctx)
	defer sessionCancel()
	sessionDone := make(chan error, 1)
	startWorkspacePoll(indexer.WorkspacesPollInterval(cfg))
	go func() {
		sessionDone <- runSupervisedLongSession(sessionCtx, cfg, processRunID, &activeIndexer, logJSON, logLevel, hotN, baseLog)
	}()

	for {
		select {
		case <-ctx.Done():
			sessionCancel()
			<-sessionDone
			return ctx.Err()
		case <-reloadCh:
			drainReloadSignals(reloadCh)
			tcfg, err := resolveSupervisedCfg(wd, cfgFlag, gatewayURL, roots)
			if err != nil {
				baseLog.Error("indexer supervised config reload skipped (resolve error)",
					"err", err, "msg", "indexer.supervised.hot_reload_resolve_error",
				)
				continue
			}
			if ix := activeIndexer.Load(); ix != nil {
				ix.ApplyTuning(tcfg)
				hotN++
				baseLog.Info("indexer supervised YAML tuning applied in-process",
					"msg", "indexer.supervised.hot_reload",
					"type", "indexer.supervised.hot_reload",
					"n", hotN,
				)
				startWorkspacePoll(indexer.WorkspacesPollInterval(tcfg))
			}
		case err := <-sessionDone:
			if err != nil && ctx.Err() == nil {
				baseLog.Error("supervised watch session exited unexpectedly",
					"err", err, "msg", "indexer.supervised.session_fatal_exit",
				)
				return err
			}
			return err
		}
	}
}

func waitReloadOrCtx(ctx context.Context, reloadCh <-chan struct{}) {
	select {
	case <-ctx.Done():
	case <-reloadCh:
	}
}

func waitReloadOrCtxTimeout(ctx context.Context, reloadCh <-chan struct{}, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-reloadCh:
		return true
	case <-timer.C:
		return true
	}
}

func runWorkspacePoll(
	ctx context.Context,
	wd, cfgFlag, gatewayURL string,
	baseLog *slog.Logger,
	interval time.Duration,
	wsFpMu *sync.Mutex,
	wsFingerprint *string,
	reindexTracker *indexer.ReindexTracker,
	coherenceReporter *indexer.CoherenceReporter,
	activeIndexer *atomic.Pointer[indexer.Indexer],
) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			ix := activeIndexer.Load()
			if ix == nil {
				continue
			}
			gwURL := strings.TrimSpace(gatewayURL)
			if gwURL == "" {
				gwURL = strings.TrimSpace(os.Getenv(indexer.EnvGatewayURL))
			}
			token := strings.TrimSpace(os.Getenv(indexer.EnvGatewayToken))
			if gwURL == "" || token == "" {
				continue
			}
			cl := indexer.NewGatewayClient(gwURL, token, 30*time.Second)
			resp, err := cl.FetchWorkspaces(ctx, nil, indexer.SessionRetryPolicy{MaxAttempts: 1})
			if err != nil {
				baseLog.Debug("workspace poll: fetch failed",
					"msg", "indexer.supervised.workspaces_poll",
					"type", "indexer.supervised.workspaces_poll",
					"err", err,
				)
				continue
			}
			newFp := indexer.WorkspacesRootsFingerprint(resp)
			wsFpMu.Lock()
			prev := *wsFingerprint
			wsFpMu.Unlock()

			if fc, err := indexer.LoadLayeredConfig(wd, cfgFlag); err == nil {
				ov := indexer.Overrides{GatewayURL: gatewayURL, ExplicitConfigPath: cfgFlag, AllowEmptyRoots: true}
				if cfg, err := indexer.Resolve(fc, os.Getenv, ov); err == nil {
					if st, err := indexer.OpenSyncState(cfg.SyncStatePath); err == nil && st != nil {
						if roots, rerr := indexer.RootsFromWorkspacesResponse(resp); rerr == nil {
							indexer.PushStaleSources(ctx, cl, cfg, nil, st, roots, coherenceReporter)
						}
						if indexer.ApplyWorkspacesReindex(ctx, st, resp, reindexTracker, baseLog) {
							_ = st.Close()
							_ = ix.ScheduleInitialScan()
							wsFpMu.Lock()
							*wsFingerprint = newFp
							wsFpMu.Unlock()
							continue
						}
						_ = st.Close()
					}
				}
			}

			if newFp == prev {
				baseLog.Debug("workspace poll: no change",
					"msg", "indexer.supervised.workspaces_poll",
					"type", "indexer.supervised.workspaces_poll",
					"paths_hash", newFp,
				)
				continue
			}

			newRoots, err := indexer.RootsFromWorkspacesResponse(resp)
			if err != nil {
				baseLog.Warn("workspace poll: could not materialize roots",
					"msg", "indexer.supervised.workspaces_apply_failed",
					"err", err,
				)
				continue
			}

			watchPaths := indexer.WatchRootPathsFromResponse(resp)
			baseLog.Info("supervised workspace list changed; applying incremental root update",
				"msg", "indexer.supervised.workspaces_changed",
				"type", "indexer.supervised.workspaces_changed",
				"prev_paths_hash", prev,
				"new_paths_hash", newFp,
				"workspace_ids", indexer.WorkspaceIDsFromResponse(resp),
				"roots", len(watchPaths),
				"watch_root_paths", watchPaths,
			)

			deadline := time.Now().Add(10 * time.Minute)
			for time.Now().Before(deadline) {
				if ix.Queue().Len() == 0 {
					break
				}
				select {
				case <-ctx.Done():
					return
				case <-time.After(5 * time.Second):
				}
			}

			changed, err := ix.ApplyRootsSnapshot(ctx, newRoots)
			if err != nil {
				baseLog.Warn("workspace poll: apply roots failed", "err", err)
				continue
			}
			if changed {
				wsFpMu.Lock()
				*wsFingerprint = indexer.RootsSnapshotFingerprint(ix.GetRoots())
				wsFpMu.Unlock()
				baseLog.Info("supervised workspace roots applied in-process",
					"msg", "indexer.supervised.workspaces_applied",
					"type", "indexer.supervised.workspaces_applied",
					"paths_hash", newFp,
				)
			}
		}
	}
}

func runBackend(args []string) error {
	_ = godotenv.Load("env")
	_ = godotenv.Load(".env")

	var (
		cfgPath     string
		gatewayURL  string
		roots       rootList
		oneShot     bool
		showVersion bool
		logJSON     bool
		logLevel    string
	)
	fs := flag.NewFlagSet("chimera-indexer-backend", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&cfgPath, "config", "", "optional indexer YAML")
	fs.StringVar(&gatewayURL, "gateway-url", "", "override gateway URL")
	fs.Var(&roots, "root", "watch root (repeatable)")
	fs.BoolVar(&oneShot, "one-shot", false, "single scan pass and exit")
	fs.BoolVar(&showVersion, "version", false, "print version and exit")
	fs.BoolVar(&logJSON, "log-json", false, "structured JSON logs on stderr")
	fs.StringVar(&logLevel, "log-level", "", "override log_level")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if showVersion {
		fmt.Printf("chimera-indexer %s\ncommit %s\nbuild date %s\n", version, commit, date)
		return nil
	}

	wd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getwd: %w", err)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	fc, err := indexer.LoadLayeredConfig(wd, cfgPath)
	if err != nil {
		return err
	}
	ov := indexer.Overrides{GatewayURL: gatewayURL, Roots: roots}
	if p := strings.TrimSpace(cfgPath); p != "" {
		ov.ExplicitConfigPath = p
		ov.AllowEmptyRoots = true
	}
	cfg, err := indexer.Resolve(fc, os.Getenv, ov)
	if err != nil {
		return err
	}
	if strings.TrimSpace(logLevel) != "" {
		cfg.LogLevel = indexer.ParseLogLevel(logLevel)
	}
	baseLog := indexer.StderrLogger(logJSON, cfg.LogLevel)

	if oneShot {
		return runOneShot(ctx, cfg, logJSON, baseLog)
	}

	if strings.TrimSpace(cfgPath) != "" {
		absCfg, errPath := filepath.Abs(strings.TrimSpace(cfgPath))
		if errPath != nil {
			return fmt.Errorf("indexer supervised config path: %w", errPath)
		}
		err := runWatchWithHotReload(ctx, wd, absCfg, cfgPath, gatewayURL, roots, logJSON, logLevel, baseLog)
		if err != nil {
			baseLog.Error("supervised indexer process exiting",
				"err", err, "msg", "indexer.supervised.process_exit",
			)
		}
		return err
	}

	runID := uuid.NewString()
	log := attachSessionLogger(logJSON, baseLog, runID)
	var nilIx atomic.Pointer[indexer.Indexer]
	return runSupervisedLongSession(ctx, cfg, runID, &nilIx, logJSON, logLevel, 0, log)
}

func run(args []string) error {
	_ = godotenv.Load("env")
	_ = godotenv.Load(".env")
	cfg, err := idxconfig.Parse(args, idxconfig.BuildInfo{
		Version: version,
		Commit:  commit,
		Date:    date,
	})
	if err != nil {
		if errors.Is(err, io.EOF) {
			return nil
		}
		return wruntime.WrapExitError(contract.ExitConfigError, err)
	}
	log := logfmt.NewLogger(os.Stderr, logfmt.JSONEnabled(), slog.LevelInfo)
	idx := &adapter.Indexer{Cfg: cfg}
	rootCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return wruntime.Run(rootCtx, wruntime.Config{
		Component:              componentName,
		BackendMode:            "binary",
		Listen:                 cfg.Listen,
		StartupTimeout:         cfg.StartupTimeout,
		ShutdownTimeout:        cfg.ShutdownTimeout,
		TerminateWait:          cfg.TerminateWait,
		BackoffInitial:         cfg.BackoffInitial,
		BackoffMultiplier:      cfg.BackoffMultiplier,
		BackoffMax:             cfg.BackoffMax,
		BackoffResetAfter:      cfg.BackoffResetAfter,
		DebugEnableUpstream:    cfg.DebugEnableUpstream,
		DebugAllowRemote:       cfg.DebugAllowRemote,
		ForwardUpstreamInDebug: cfg.ForwardUpstreamInDebug,
		UpstreamVersion:        cfg.UpstreamVersion,
		WrapperVersion:         version,
		BuildCommit:            commit,
		ReadyMessage:           "indexer.ready",
		UpstreamLineMessage:    "indexer.upstream.line",
		HTTPServerErrorMessage: "indexer.http.server_error",
		UpstreamLineWrapper:    adapter.WrapUpstreamLine,
	}, idx, log)
}

const componentName = contract.ComponentIndexer

func exitCodeForError(err error) int {
	var ee *wruntime.ExitError
	if errors.As(err, &ee) {
		return ee.Code
	}
	return contract.ExitInternal
}
