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
// saves to that file trigger an automatic indexer restart of the watcher
// session without restarting the desktop process. Watch directories come from
// GET /v1/indexer/workspaces (operator SQLite), not from YAML roots. If none are
// configured yet, the process stays alive and retries periodically and on YAML
// tuning edits until at least one path exists.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/joho/godotenv"
	"github.com/lynn/porcelain/chimera/chimera-indexer/indexer"
	"github.com/lynn/porcelain/chimera/chimera-indexer/indexerline"
	"github.com/lynn/porcelain/chimera/internal/wrapper/contract"
	wruntime "github.com/lynn/porcelain/chimera/internal/wrapper/runtime"
	"github.com/lynn/porcelain/internal/platform"
)

// errSupervisedReload is a cancel-cause marker: the supervised --config file
// changed and the watch session is cycling (not a hard failure).
var errSupervisedReload = errors.New("indexer supervised config hot-reload")

const componentName = contract.ComponentIndexer

func materializeSupervisedRoots(ctx context.Context, log *slog.Logger, cfg *indexer.Resolved) {
	if cfg == nil || !cfg.SupervisedLayer {
		return
	}
	cl := indexer.NewGatewayClient(cfg.GatewayURL, cfg.Token, cfg.RequestTimeout)
	if err := indexer.MaterializeRootsFromGateway(ctx, cl, cfg, indexer.RetryPolicyFromResolved(*cfg)); err != nil {
		if log != nil {
			log.Warn("gateway workspaces fetch failed", "err", err)
		}
	}
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
		printHelp()
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
	materializeSupervisedRoots(parentCtx, baseLog, &cfg)
	if cfg.SupervisedLayer && len(cfg.Roots) == 0 {
		return fmt.Errorf("supervised indexer: no workspace directories from gateway (GET /v1/indexer/workspaces); add paths in the logs UI workspaces list")
	}
	runID := uuid.NewString()
	log := attachSessionLogger(logJSON, baseLog, runID)
	client := indexer.NewGatewayClient(cfg.GatewayURL, cfg.Token, cfg.RequestTimeout)
	client.IndexRunID = runID

	ix := indexer.New(cfg, client, log)
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
	log.Info("indexer run done", indexer.RunDoneAttrs("one-shot", ix.OpsSnapshot())...)
	return nil
}

func attachSessionLogger(logJSON bool, baseLog *slog.Logger, runID string) *slog.Logger {
	if baseLog == nil {
		baseLog = indexer.StderrLogger(logJSON, slog.LevelInfo)
	}
	return baseLog.With("index_run_id", runID, "service", "indexer")
}

// runWatchSession runs one supervised watch session. sessionQueueCh receives the
// session's Queue once the Indexer is ready so callers can check idle state.
func runWatchSession(sessionCtx context.Context, wd string, cfgPath string, gatewayURL string, roots rootList, logJSON bool, logLevel string, hotReloadCount int, sessionQueueCh chan<- *indexer.Queue) error {
	fc, err := indexer.LoadLayeredConfig(wd, cfgPath)
	if err != nil {
		return err
	}
	ov := indexer.Overrides{GatewayURL: gatewayURL, Roots: roots}
	if strings.TrimSpace(cfgPath) != "" {
		ov.ExplicitConfigPath = cfgPath
		// Supervised --config: roots come from GET /v1/indexer/workspaces, not YAML (may be empty until API fetch).
		ov.AllowEmptyRoots = true
	}
	cfg, err := indexer.Resolve(fc, os.Getenv, ov)
	if err != nil {
		return err
	}
	if strings.TrimSpace(logLevel) != "" {
		cfg.LogLevel = indexer.ParseLogLevel(logLevel)
	}

	runID := uuid.NewString()
	sessionBase := indexer.StderrLogger(logJSON, cfg.LogLevel)
	log := attachSessionLogger(logJSON, sessionBase, runID)

	// Supervised mode resolves with Roots=[] because YAML roots are ignored;
	// repopulate from GET /v1/indexer/workspaces so the session's indexer has
	// the same watch list the outer hot-reload loop just materialized.
	materializeSupervisedRoots(sessionCtx, sessionBase, &cfg)

	client := indexer.NewGatewayClient(cfg.GatewayURL, cfg.Token, cfg.RequestTimeout)
	client.IndexRunID = runID

	ix := indexer.New(cfg, client, log)
	// Expose the queue so the outer loop can wait for idle before triggering
	// a workspace-change reload, avoiding disruption of in-flight ingest work.
	if sessionQueueCh != nil {
		select {
		case sessionQueueCh <- ix.Queue():
		default:
		}
	}
	if _, err := ix.FetchAndLogConfig(sessionCtx); err != nil {
		var he *indexer.HTTPError
		if errors.As(err, &he) && he.Status == 503 && strings.Contains(strings.ToLower(he.Body), "rag is not enabled") {
			return fmt.Errorf("gateway at %s has RAG disabled — set rag.enabled=true in config/gateway.yaml and restart the gateway", cfg.GatewayURL)
		}
		log.Warn("continuing despite config fetch failure", "err", err)
	}
	if hotReloadCount > 0 {
		log.Info("indexer supervised config hot-reload; starting new watch session",
			"msg", "indexer.supervised.hot_reload",
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

	errW := <-watchDone
	ix.Queue().Close()
	<-doneWorkers
	if errW != nil {
		log.Error("watcher exited", "err", errW)
	}
	if errors.Is(context.Cause(sessionCtx), errSupervisedReload) {
		return errSupervisedReload
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
	log.Info("indexer run stopped", indexer.RunDoneAttrs("watch", ix.OpsSnapshot())...)
	return nil
}

// defaultWorkspacesPollInterval is how often the supervised workspace poller
// re-fetches GET /v1/indexer/workspaces while a watch session is running.
// A new workspace created in the operator UI will be picked up within this window.
const defaultWorkspacesPollInterval = 30 * time.Second

func runWatchWithHotReload(ctx context.Context, wd string, absSupervisedCfg string, cfgFlag string, gatewayURL string, roots rootList, logJSON bool, logLevel string, baseLog *slog.Logger) error {
	reloadCh := make(chan struct{}, 1)
	signalReload := func() {
		select {
		case reloadCh <- struct{}{}:
		default:
		}
	}
	go func() {
		werr := indexer.WatchConfigPathForReload(ctx, absSupervisedCfg, indexer.DefaultConfigReloadDebounce, signalReload, baseLog)
		if werr != nil && ctx.Err() == nil && !errors.Is(werr, context.Canceled) {
			baseLog.Warn("indexer supervised config watch ended", "err", werr)
		}
	}()

	// wsFingerprint is updated in the outer loop after each materialize call so
	// the workspace poll goroutine always compares against the set that is
	// currently active (or last attempted).
	var wsFpMu sync.Mutex
	var wsFingerprint string

	// activeSessionQueue is set by the outer loop to the current session's queue
	// so the workspace poll goroutine can wait for idle before triggering reload.
	var activeSessionQueue atomic.Value // stores *indexer.Queue

	// Workspace poll goroutine: periodically fetches GET /v1/indexer/workspaces
	// while a session is running. When a workspace change is detected it waits
	// until the active session's queue is empty before calling signalReload, so
	// existing in-flight ingest work for other workspaces finishes first.
	go func() {
		ticker := time.NewTicker(defaultWorkspacesPollInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
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
				newFp := indexer.WorkspacesResponseFingerprint(resp)
				wsFpMu.Lock()
				prev := wsFingerprint
				wsFpMu.Unlock()
				if newFp == prev {
					baseLog.Debug("workspace poll: no change",
						"msg", "indexer.supervised.workspaces_poll",
						"type", "indexer.supervised.workspaces_poll",
						"workspaces", newFp,
					)
					continue
				}
				baseLog.Info("supervised workspace list changed; waiting for session idle before reload",
					"msg", "indexer.supervised.workspaces_changed",
					"type", "indexer.supervised.workspaces_changed",
					"prev_workspaces", prev,
					"new_workspaces", newFp,
				)
				// Optimistically advance the shared fingerprint to the new set so the
				// next poll tick doesn't fire a duplicate reload while the outer loop
				// is still processing this one (e.g. during LoadLayeredConfig or
				// materializeSupervisedRoots). The outer loop will overwrite this with
				// the authoritative value after it successfully materialises roots.
				wsFpMu.Lock()
				wsFingerprint = newFp
				wsFpMu.Unlock()
				// Wait up to 10 minutes for the active session's queue to drain
				// so we don't interrupt in-flight ingest work for existing workspaces.
				deadline := time.Now().Add(10 * time.Minute)
				for time.Now().Before(deadline) {
					q, _ := activeSessionQueue.Load().(*indexer.Queue)
					if q == nil || q.Len() == 0 {
						break
					}
					select {
					case <-ctx.Done():
						return
					case <-time.After(5 * time.Second):
					}
				}
				baseLog.Info("session idle (or timeout); reloading watch session for new workspace",
					"msg", "indexer.supervised.workspaces_reload",
					"type", "indexer.supervised.workspaces_reload",
					"new_workspaces", newFp,
				)
				signalReload()
			}
		}
	}()

	hotN := 0
	for {
		drainReloadSignals(reloadCh)

		fc, err := indexer.LoadLayeredConfig(wd, cfgFlag)
		if err != nil {
			if hotN == 0 {
				return err
			}
			baseLog.Error("indexer supervised config reload skipped (invalid YAML)",
				"err", err,
				"config_layer", cfgFlag,
				"msg", "indexer.supervised.hot_reload_yaml_error",
			)
			select {
			case <-reloadCh:
				continue
			case <-ctx.Done():
				return ctx.Err()
			}
		}

		resolveOV := indexer.Overrides{
			GatewayURL:         gatewayURL,
			Roots:              roots,
			ExplicitConfigPath: cfgFlag,
			AllowEmptyRoots:    true,
		}
		cfg, err := indexer.Resolve(fc, os.Getenv, resolveOV)
		if err != nil {
			if hotN == 0 {
				return err
			}
			baseLog.Error("indexer supervised config reload skipped (resolve error)",
				"err", err,
				"config_layer", cfgFlag,
				"msg", "indexer.supervised.hot_reload_resolve_error",
			)
			select {
			case <-reloadCh:
				continue
			case <-ctx.Done():
				return ctx.Err()
			}
		}

		materializeSupervisedRoots(ctx, baseLog, &cfg)

		// Update fingerprint so workspace poll goroutine has a baseline after each session cycle.
		wsFpMu.Lock()
		wsFingerprint = indexer.WorkspacesFingerprint(cfg.Roots)
		wsFpMu.Unlock()

		if len(cfg.Roots) == 0 {
			baseLog.Info(
				"supervised indexer waiting for at least one workspace path from the gateway (GET /v1/indexer/workspaces)",
				"msg", "indexer.supervised.wait_roots",
				"type", "indexer.supervised.wait_roots",
				"config_path", absSupervisedCfg,
			)
			timer := time.NewTimer(15 * time.Second)
			select {
			case <-ctx.Done():
				if !timer.Stop() {
					<-timer.C
				}
				return ctx.Err()
			case <-reloadCh:
				if !timer.Stop() {
					<-timer.C
				}
				continue
			case <-timer.C:
				continue
			}
		}

		sessionCtx, cancel := context.WithCancelCause(ctx)

		// sessionQueueCh receives the Queue from runWatchSession once the Indexer
		// is ready so we can share it with the workspace poll goroutine.
		sessionQueueCh := make(chan *indexer.Queue, 1)
		activeSessionQueue.Store((*indexer.Queue)(nil)) // clear previous session's queue
		sessDone := make(chan error, 1)
		go func() {
			sessDone <- runWatchSession(sessionCtx, wd, cfgFlag, gatewayURL, roots, logJSON, logLevel, hotN, sessionQueueCh)
		}()
		// Non-blocking receive: session may not have started yet, but the poll
		// goroutine will pick up the queue on its next tick regardless.
		select {
		case q := <-sessionQueueCh:
			activeSessionQueue.Store(q)
		default:
		}

		// Drain the queue channel in a goroutine so we get the queue even if the
		// session starts after we already checked above.
		go func() {
			select {
			case q := <-sessionQueueCh:
				activeSessionQueue.Store(q)
			case <-sessionCtx.Done():
			}
		}()

		select {
		case <-ctx.Done():
			cancel(context.Canceled)
			_ = <-sessDone
			activeSessionQueue.Store((*indexer.Queue)(nil))
			return ctx.Err()
		case <-reloadCh:
			cancel(errSupervisedReload)
			<-sessDone
			activeSessionQueue.Store((*indexer.Queue)(nil))
			hotN++
			continue
		case err := <-sessDone:
			activeSessionQueue.Store((*indexer.Queue)(nil))
			if err != nil {
				cancel(context.Canceled)
				return err
			}
			cancel(context.Canceled)
			return nil
		}
	}
}

func runBackend(args []string) error {
	// Load env files from cwd (missing files ignored) before reading flags so
	// operators can stash CHIMERA_GATEWAY_URL/_TOKEN in `.env` next to the
	// gateway's own .env. Matches cmd/chimera behavior.
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
	fs.StringVar(&cfgPath, "config", "", "optional indexer YAML merged after ~/.locus/indexer.config.yaml and ./.locus/indexer.config.yaml")
	fs.StringVar(&gatewayURL, "gateway-url", "", "override gateway URL (env "+indexer.EnvGatewayURL+")")
	fs.Var(&roots, "root", "watch root (repeatable; overrides config 'roots')")
	fs.BoolVar(&oneShot, "one-shot", false, "perform a single scan + ingest pass and exit")
	fs.BoolVar(&showVersion, "version", false, "print version and exit")
	fs.BoolVar(&logJSON, "log-json", false, "emit structured JSON logs on stderr (v0.5 supervised / operator UI)")
	fs.StringVar(&logLevel, "log-level", "", "override indexer log_level (debug, info, warn, error)")
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

	explicitConfigLayer := strings.TrimSpace(cfgPath) != ""
	if oneShot {
		return runOneShot(ctx, cfg, logJSON, baseLog)
	}

	if explicitConfigLayer {
		absCfg, errPath := filepath.Abs(strings.TrimSpace(cfgPath))
		if errPath != nil {
			return fmt.Errorf("indexer supervised config path: %w", errPath)
		}
		return runWatchWithHotReload(ctx, wd, absCfg, cfgPath, gatewayURL, roots, logJSON, logLevel, baseLog)
	}

	return runWatchSession(ctx, wd, cfgPath, gatewayURL, roots, logJSON, logLevel, 0, nil)
}

type indexerConfig struct {
	Listen                 string
	Bin                    string
	StartupTimeout         time.Duration
	ShutdownTimeout        time.Duration
	TerminateWait          time.Duration
	BackoffInitial         time.Duration
	BackoffMultiplier      float64
	BackoffMax             time.Duration
	BackoffResetAfter      time.Duration
	DebugEnableUpstream    bool
	DebugAllowRemote       bool
	ForwardUpstreamInDebug bool
	UpstreamVersion        string
	BackendArgs            []string
}

func parseConfig(args []string) (indexerConfig, error) {
	fs := flag.NewFlagSet("chimera-indexer", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	cfg := indexerConfig{}
	var showVersion bool
	fs.StringVar(&cfg.Listen, "listen", envOrDefault("INDEXER__LISTEN", "127.0.0.1:7750"), "wrapper listen addr (host:port)")
	fs.StringVar(&cfg.Bin, "bin", envOrDefault("INDEXER__BIN", ""), "indexer backend binary path")
	fs.DurationVar(&cfg.StartupTimeout, "startup-timeout", envDuration("INDEXER__TIMEOUTS__STARTUP", contract.DefaultStartupTimeout), "startup readiness timeout")
	fs.DurationVar(&cfg.ShutdownTimeout, "shutdown-timeout", envDuration("INDEXER__TIMEOUTS__SHUTDOWN", contract.DefaultShutdownTimeout), "wrapper graceful shutdown timeout")
	fs.DurationVar(&cfg.TerminateWait, "terminate-wait", contract.DefaultTerminateWait, "wait before force-kill backend")
	fs.DurationVar(&cfg.BackoffInitial, "backoff-initial", contract.DefaultBackoffInitial, "restart backoff initial delay")
	fs.Float64Var(&cfg.BackoffMultiplier, "backoff-multiplier", contract.DefaultBackoffMultiplier, "restart backoff multiplier")
	fs.DurationVar(&cfg.BackoffMax, "backoff-max", contract.DefaultBackoffMax, "restart backoff max delay")
	fs.DurationVar(&cfg.BackoffResetAfter, "backoff-reset-after", contract.DefaultBackoffResetAfter, "healthy runtime to reset backoff")
	fs.BoolVar(&cfg.DebugEnableUpstream, "debug-enable-upstream-logs", wruntime.EnvBool(contract.DebugEnableEnvKey), "enable /debug/upstream/logs")
	fs.BoolVar(&cfg.DebugAllowRemote, "debug-allow-remote", wruntime.EnvBool(contract.DebugAllowRemoteEnv), "allow /debug/* on non-loopback bind")
	fs.BoolVar(&cfg.ForwardUpstreamInDebug, "debug-forward-upstream", false, "forward upstream lines to stderr in debug mode")
	fs.StringVar(&cfg.UpstreamVersion, "upstream-version", "", "optional backend version for status payload")
	fs.BoolVar(&showVersion, "version", false, "print version")
	fs.BoolVar(&showVersion, "v", false, "print version")
	if err := fs.Parse(args); err != nil {
		return cfg, err
	}
	if showVersion {
		fmt.Printf("chimera-indexer %s\ncommit %s\nbuild date %s\n", version, commit, date)
		return cfg, io.EOF
	}
	cfg.BackendArgs = fs.Args()
	return cfg, nil
}

func printHelp() {
	fmt.Printf(`Chimera indexer runtime

Usage:
  chimera-indexer [flags] [backend flags]
  chimera-indexer -version

Backend flags are passed through to the embedded backend mode (for example: --one-shot, --config, --gateway-url).

Flags:
`)
	fs := flag.NewFlagSet("chimera-indexer", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	_ = fs.String("listen", envOrDefault("INDEXER__LISTEN", "127.0.0.1:7750"), "wrapper listen addr (host:port)")
	_ = fs.String("bin", envOrDefault("INDEXER__BIN", ""), "indexer backend binary path")
	_ = fs.Duration("startup-timeout", envDuration("INDEXER__TIMEOUTS__STARTUP", contract.DefaultStartupTimeout), "startup readiness timeout")
	_ = fs.Duration("shutdown-timeout", envDuration("INDEXER__TIMEOUTS__SHUTDOWN", contract.DefaultShutdownTimeout), "wrapper graceful shutdown timeout")
	_ = fs.Duration("terminate-wait", contract.DefaultTerminateWait, "wait before force-kill backend")
	_ = fs.Duration("backoff-initial", contract.DefaultBackoffInitial, "restart backoff initial delay")
	_ = fs.Float64("backoff-multiplier", contract.DefaultBackoffMultiplier, "restart backoff multiplier")
	_ = fs.Duration("backoff-max", contract.DefaultBackoffMax, "restart backoff max delay")
	_ = fs.Duration("backoff-reset-after", contract.DefaultBackoffResetAfter, "healthy runtime to reset backoff")
	_ = fs.Bool("debug-enable-upstream-logs", wruntime.EnvBool(contract.DebugEnableEnvKey), "enable /debug/upstream/logs")
	_ = fs.Bool("debug-allow-remote", wruntime.EnvBool(contract.DebugAllowRemoteEnv), "allow /debug/* on non-loopback bind")
	_ = fs.Bool("debug-forward-upstream", false, "forward upstream lines to stderr in debug mode")
	_ = fs.String("upstream-version", "", "optional backend version for status payload")
	_ = fs.Bool("version", false, "print version")
	_ = fs.Bool("v", false, "print version")
	fs.PrintDefaults()
}

type indexerAdapter struct {
	cfg indexerConfig
}

func run(args []string) error {
	_ = godotenv.Load("env")
	_ = godotenv.Load(".env")
	cfg, err := parseConfig(args)
	if err != nil {
		if errors.Is(err, io.EOF) {
			return nil
		}
		return wruntime.WrapExitError(contract.ExitConfigError, err)
	}
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))
	adapter := &indexerAdapter{cfg: cfg}
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
		UpstreamLineWrapper:    wrapIndexerLine,
	}, adapter, log)
}

func (a *indexerAdapter) Start(ctx context.Context, capture io.Writer, _ *slog.Logger) (*exec.Cmd, error) {
	bin := strings.TrimSpace(a.cfg.Bin)
	args := append([]string{}, a.cfg.BackendArgs...)
	if bin == "" {
		exe, err := os.Executable()
		if err != nil {
			return nil, fmt.Errorf("resolve executable for embedded backend: %w", err)
		}
		bin = exe
		args = append([]string{"--indexer-backend"}, args...)
	}
	stdout := platform.StdoutTee(indexerline.NewWriter(capture))
	stderr := platform.StderrTee(indexerline.NewWriter(capture))
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = os.Environ()
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start indexer backend: %w", err)
	}
	return cmd, nil
}

func (a *indexerAdapter) ReadyURL() string {
	return ""
}

func (a *indexerAdapter) MetricsURL() string {
	return ""
}

func (a *indexerAdapter) BackendName() string {
	return "custom"
}

func wrapIndexerLine(raw string) string {
	return string(indexerline.NormalizePayload(raw))
}

func envOrDefault(key, def string) string {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	return v
}

func envDuration(key string, def time.Duration) time.Duration {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return def
	}
	return d
}

func exitCodeForError(err error) int {
	var ee *wruntime.ExitError
	if errors.As(err, &ee) {
		return ee.Code
	}
	return contract.ExitInternal
}
