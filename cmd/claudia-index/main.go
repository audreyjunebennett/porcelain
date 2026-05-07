// claudia-index is the v0.4 workspace file indexer for the Claudia Gateway.
//
// It walks configured roots, applies .claudiaignore + .gitignore + binary
// detection, hashes whole files, and POSTs them to /v1/ingest. Watching uses
// fsnotify for incremental updates.
//
// Usage:
//
//	claudia-index --config .claudia/indexer.config.yaml [--root path]... [--gateway-url URL]
//
// Environment:
//
//	CLAUDIA_GATEWAY_URL    base URL of the gateway (default port 3000)
//	CLAUDIA_GATEWAY_TOKEN  bearer token; must equal a token: entry in
//	                       config/tokens.yaml on the gateway side
//
// On startup the binary loads `env` and then `.env` (later wins) from the
// current working directory, mirroring the main `claudia` binary so operators
// can keep one secrets file for both.
//
// When --config names an explicit YAML layer (desktop supervised mode),
// saves to that file trigger an automatic indexer restart of the watcher
// session without restarting the desktop process.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/google/uuid"
	"github.com/joho/godotenv"
	"github.com/lynn/claudia-gateway/internal/indexer"
)

// errSupervisedReload is a cancel-cause marker: the supervised --config file
// changed and the watch session is cycling (not a hard failure).
var errSupervisedReload = errors.New("indexer supervised config hot-reload")

type rootList []string

func (r *rootList) String() string { return strings.Join(*r, ",") }
func (r *rootList) Set(v string) error {
	*r = append(*r, v)
	return nil
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "claudia-index:", err)
		os.Exit(1)
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

func runWatchSession(sessionCtx context.Context, wd string, cfgPath string, gatewayURL string, roots rootList, logJSON bool, logLevel string, hotReloadCount int) error {
	fc, err := indexer.LoadLayeredConfig(wd, cfgPath)
	if err != nil {
		return err
	}
	ov := indexer.Overrides{GatewayURL: gatewayURL, Roots: roots}
	if strings.TrimSpace(cfgPath) != "" {
		ov.ExplicitConfigPath = cfgPath
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
	client := indexer.NewGatewayClient(cfg.GatewayURL, cfg.Token, cfg.RequestTimeout)
	client.IndexRunID = runID

	ix := indexer.New(cfg, client, log)
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

	hotN := 0
	for {
		drainReloadSignals(reloadCh)

		_, err := indexer.LoadLayeredConfig(wd, cfgFlag)
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

		sessionCtx, cancel := context.WithCancelCause(ctx)

		sessDone := make(chan error, 1)
		go func() {
			sessDone <- runWatchSession(sessionCtx, wd, cfgFlag, gatewayURL, roots, logJSON, logLevel, hotN)
		}()

		select {
		case <-ctx.Done():
			cancel(context.Canceled)
			_ = <-sessDone
			return ctx.Err()
		case <-reloadCh:
			cancel(errSupervisedReload)
			<-sessDone
			hotN++
			continue
		case err := <-sessDone:
			if err != nil {
				cancel(context.Canceled)
				return err
			}
			cancel(context.Canceled)
			return nil
		}
	}
}

func run() error {
	// Load env files from cwd (missing files ignored) before reading flags so
	// operators can stash CLAUDIA_GATEWAY_URL/_TOKEN in `.env` next to the
	// gateway's own .env. Matches cmd/claudia behavior.
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
	flag.StringVar(&cfgPath, "config", "", "optional indexer YAML merged after ~/.claudia/indexer.config.yaml and ./.claudia/indexer.config.yaml")
	flag.StringVar(&gatewayURL, "gateway-url", "", "override gateway URL (env "+indexer.EnvGatewayURL+")")
	flag.Var(&roots, "root", "watch root (repeatable; overrides config 'roots')")
	flag.BoolVar(&oneShot, "one-shot", false, "perform a single scan + ingest pass and exit")
	flag.BoolVar(&showVersion, "version", false, "print version and exit")
	flag.BoolVar(&logJSON, "log-json", false, "emit structured JSON logs on stderr (v0.5 supervised / operator UI)")
	flag.StringVar(&logLevel, "log-level", "", "override indexer log_level (debug, info, warn, error)")
	flag.Parse()

	if showVersion {
		fmt.Println("claudia-index v0.5.0")
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
	if strings.TrimSpace(cfgPath) != "" {
		ov.ExplicitConfigPath = cfgPath
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

	return runWatchSession(ctx, wd, cfgPath, gatewayURL, roots, logJSON, logLevel, 0)
}
