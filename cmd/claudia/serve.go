package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/lynn/claudia-gateway/internal/config"
	"github.com/lynn/claudia-gateway/internal/platform"
	"github.com/lynn/claudia-gateway/internal/server"
	"github.com/lynn/claudia-gateway/internal/servicelogs"
	"github.com/lynn/claudia-gateway/internal/servicelogs/bifrostline"
	"github.com/lynn/claudia-gateway/internal/servicelogs/qdrantline"
	"github.com/lynn/claudia-gateway/internal/supervisor"
	"github.com/lynn/claudia-gateway/internal/upstream"
)

func gatewayPublicURL(ln net.Addr) string {
	addr := ln.String()
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return "http://127.0.0.1:3000"
	}
	if host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	if strings.Contains(host, ":") && !strings.HasPrefix(host, "[") {
		return fmt.Sprintf("http://[%s]:%s", host, port)
	}
	return fmt.Sprintf("http://%s:%s", host, port)
}

func gatewayPublicURLFromResolved(res *config.Resolved) string {
	if res == nil {
		return "http://127.0.0.1:3000"
	}
	host := strings.TrimSpace(res.ListenHost)
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	if strings.Contains(host, ":") && !strings.HasPrefix(host, "[") {
		return fmt.Sprintf("http://[%s]:%d", host, res.ListenPort)
	}
	return fmt.Sprintf("http://%s:%d", host, res.ListenPort)
}

func panelURLFromListenAddr(ln net.Addr) string {
	return gatewayPublicURL(ln) + "/ui/panel"
}

// webviewEntryURL is opened by the native desktop shell.
// Porcelain's desktop wrapper starts Locus before Chimera, so the main app
// should land on the Locus workspace instead of the gateway admin shell.
func webviewEntryURL(ln net.Addr, bootstrap bool) string {
	base := gatewayPublicURL(ln)
	if bootstrap {
		return base + "/ui/setup"
	}
	return "http://127.0.0.1:11435/web"
}

func waitForChildExit(name string, cmd *exec.Cmd, waitCh <-chan error, timeout time.Duration, log *slog.Logger) {
	if waitCh == nil {
		return
	}
	select {
	case werr := <-waitCh:
		if werr != nil && log != nil {
			log.Debug(name+" process finished", "msg", "gateway.supervisor.child.exited", "child", name, "err", werr)
		}
		return
	case <-time.After(timeout):
	}

	if cmd != nil && cmd.Process != nil {
		if log != nil {
			log.Warn(name+" did not exit after context cancel; forcing kill",
				"msg", "gateway.shutdown.child_force_kill", "child", name, "pid", cmd.Process.Pid, "timeout", timeout)
		}
		if err := cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) && log != nil {
			log.Warn(name+" kill failed", "msg", "gateway.shutdown.child_force_kill", "child", name, "err", err, "detail", "kill_send_failed")
		}
	}

	select {
	case werr := <-waitCh:
		if werr != nil && log != nil {
			log.Debug(name+" process finished after kill", "msg", "gateway.supervisor.child.exited", "child", name, "err", werr)
		}
	case <-time.After(5 * time.Second):
		if log != nil {
			log.Warn(name+" still has not exited after forced kill", "msg", "gateway.shutdown.child_stuck", "child", name)
		}
	}
}

func runServe(args []string, openWebview bool) {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	configPath := fs.String("config", "", "Path to gateway.yaml (default: $CLAUDIA_GATEWAY_CONFIG or ./config/gateway.yaml)")
	listen := fs.String("listen", "", "Override Claudia listen address (host:port or :port)")

	bifrostBin := fs.String("bifrost-bin", defaultSupervisorBifrostBin(), "BiFrost HTTP binary (PATH or path; defaults to bifrost-http next to this executable if present, else bifrost on PATH)")
	bifrostConfig := fs.String("bifrost-config", "config/bifrost.config.json", "Source bifrost.config.json (copied to data dir as config.json)")
	bifrostDataDir := fs.String("bifrost-data-dir", defaultSupervisorDataSubdir("bifrost"), "BiFrost working directory (created; SQLite and config live here)")
	bifrostBind := fs.String("bifrost-bind", "127.0.0.1", "BiFrost bind address (-host)")
	bifrostPort := fs.Int("bifrost-port", 8080, "BiFrost listen port (-port)")
	bifrostLogLevel := fs.String("bifrost-log-level", "", "BiFrost -log-level; empty uses upstream.bifrost_log_level from gateway.yaml, then info")
	bifrostLogStyle := fs.String("bifrost-log-style", "json", "BiFrost -log-style (json or pretty)")
	upstreamHost := fs.String("upstream-host", "127.0.0.1", "Host for gateway upstream.base_url (Claudia → BiFrost); use 127.0.0.1 when bifrost-bind is 0.0.0.0")
	waitTimeout := fs.Duration("wait-bifrost", 60*time.Second, "Max time to poll BiFrost /health before exit")
	noWait := fs.Bool("no-wait-bifrost", false, "Skip readiness poll (not recommended)")

	qdrantBin := fs.String("qdrant-bin", defaultSupervisorQdrantBin(), "Qdrant binary (PATH or path); empty skips Qdrant (defaults to qdrant next to this executable if present)")
	qdrantStorage := fs.String("qdrant-storage", defaultSupervisorDataSubdir("qdrant"), "Qdrant storage directory (created)")
	qdrantBind := fs.String("qdrant-bind", "127.0.0.1", "Qdrant QDRANT__SERVICE__HOST")
	qdrantHTTPPort := fs.Int("qdrant-http-port", 6333, "Qdrant HTTP port")
	qdrantGRPCPort := fs.Int("qdrant-grpc-port", 6334, "Qdrant gRPC port")
	qdrantHealthHost := fs.String("qdrant-health-host", "127.0.0.1", "Host for GET /readyz probe (use 127.0.0.1 when qdrant-bind is 0.0.0.0)")
	waitQdrant := fs.Duration("wait-qdrant", 60*time.Second, "Max time to poll Qdrant /readyz before exit")
	noWaitQdrant := fs.Bool("no-wait-qdrant", false, "Skip Qdrant readiness poll")
	qdrantLogLevelFlag := fs.String("qdrant-log-level", "", "Qdrant QDRANT__LOGGER__LOG_LEVEL; empty uses rag.qdrant.log_level from gateway.yaml")

	_ = fs.Parse(args)

	path := strings.TrimSpace(*configPath)
	if path == "" {
		var err error
		path, err = config.ResolveGatewayConfigPath()
		if err != nil {
			fmt.Fprintln(os.Stderr, "claudia serve:", err)
			os.Exit(2)
		}
	}

	logStore := servicelogs.New(servicelogs.DefaultMaxLines)
	gwSink := logStore.Writer("gateway")
	log := buildLoggerTo(platform.StdoutTee(gwSink), path)
	upstreamURL := fmt.Sprintf("http://%s:%d", strings.TrimSpace(*upstreamHost), *bifrostPort)
	healthURL := fmt.Sprintf("http://%s:%d/health", strings.TrimSpace(*upstreamHost), *bifrostPort)

	rt, err := server.NewRuntimeWithUpstreamOverride(path, log, upstreamURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "claudia serve: load gateway config: %v\n", err)
		os.Exit(1)
	}
	res, _, _ := rt.Snapshot()

	bifrostLvl := strings.TrimSpace(*bifrostLogLevel)
	if bifrostLvl == "" {
		bifrostLvl = strings.TrimSpace(res.BifrostLogLevel)
	}
	if bifrostLvl == "" {
		bifrostLvl = "info"
	}
	qdrantLvl := strings.TrimSpace(*qdrantLogLevelFlag)
	if qdrantLvl == "" {
		qdrantLvl = strings.TrimSpace(res.RAG.QdrantLogLevel)
	}

	var diskLog *os.File
	if openWebview {
		dir := filepath.Dir(res.MetricsSQLitePath)
		if mkErr := os.MkdirAll(dir, 0755); mkErr != nil {
			log.Warn("disk log: mkdir", "msg", "gateway.startup.disk_log", "phase", "mkdir", "dir", dir, "err", mkErr)
		} else {
			p := filepath.Join(dir, "claudia-desktop.log")
			f, oerr := os.OpenFile(p, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
			if oerr != nil {
				log.Warn("disk log: open", "msg", "gateway.startup.disk_log", "phase", "open", "path", p, "err", oerr)
			} else {
				diskLog = f
				logStore.SetMirror(f)
				log.Info("disk logging enabled", "msg", "gateway.startup.disk_log", "path", p)
			}
		}
	}
	defer func() {
		logStore.SetMirror(nil)
		if diskLog != nil {
			_ = diskLog.Close()
		}
	}()

	// Ensure the operator UI log buffer is never empty, even in GUI builds
	// where stdout/stderr may not be visible/attached (also seeds disk log on desktop).
	log.Info("gateway startup seed", "msg", "gateway.startup.seed")

	bootstrap := server.BootstrapMode(rt)
	qBin := strings.TrimSpace(*qdrantBin)
	qdrantReadyzURL := ""
	if qBin != "" {
		qdrantReadyzURL = fmt.Sprintf("http://%s:%d/readyz", strings.TrimSpace(*qdrantHealthHost), *qdrantHTTPPort)
	}

	childCtx, stopChildren := context.WithCancel(context.Background())
	var qdrantProc *exec.Cmd
	var qdrantWait chan error
	var bifrostProc *exec.Cmd
	var bifrostWaitErr chan error
	var indexerProc *exec.Cmd
	var indexerWait chan error

	if !bootstrap {
		if qBin != "" {
			qSink := qdrantline.NewWriter(logStore.Writer("qdrant"))
			qcfg := supervisor.QdrantConfig{
				Bin:        qBin,
				StorageDir: *qdrantStorage,
				BindHost:   strings.TrimSpace(*qdrantBind),
				HTTPPort:   *qdrantHTTPPort,
				GRPCPort:   *qdrantGRPCPort,
				LogLevel:   qdrantLvl,
				Stdout:     platform.StdoutTee(qSink),
				Stderr:     platform.StderrTee(qSink),
			}
			var qerr error
			qdrantProc, qerr = supervisor.StartQdrant(childCtx, qcfg, log)
			if qerr != nil {
				stopChildren()
				fmt.Fprintf(os.Stderr, "claudia serve: %v\n", qerr)
				os.Exit(1)
			}
			qdrantWait = make(chan error, 1)
			go func() {
				qdrantWait <- qdrantProc.Wait()
			}()
			if !*noWaitQdrant {
				wCtx, wCancel := context.WithTimeout(context.Background(), *waitQdrant)
				err := supervisor.WaitHealthy(wCtx, qdrantReadyzURL, *waitQdrant, log, "qdrant")
				wCancel()
				if err != nil {
					stopChildren()
					<-qdrantWait
					fmt.Fprintf(os.Stderr, "claudia serve: qdrant not healthy: %v\n", err)
					os.Exit(1)
				}
			}
		}

		bSink := bifrostline.NewWriter(logStore.Writer("bifrost"))
		bcfg := supervisor.BifrostConfig{
			Bin:        *bifrostBin,
			ConfigJSON: *bifrostConfig,
			DataDir:    *bifrostDataDir,
			BindHost:   strings.TrimSpace(*bifrostBind),
			Port:       *bifrostPort,
			LogLevel:   bifrostLvl,
			LogStyle:   strings.TrimSpace(*bifrostLogStyle),
			Stdout:     platform.StdoutTee(bSink),
			Stderr:     platform.StderrTee(bSink),
		}
		var berr error
		bifrostProc, berr = supervisor.StartBifrost(childCtx, bcfg, log)
		if berr != nil {
			stopChildren()
			if qdrantWait != nil {
				<-qdrantWait
			}
			fmt.Fprintf(os.Stderr, "claudia serve: %v\n", berr)
			if errors.Is(berr, exec.ErrNotFound) || strings.Contains(berr.Error(), "executable file not found") {
				fmt.Fprintln(os.Stderr, "")
				fmt.Fprintln(os.Stderr, "No BiFrost HTTP binary found (place bifrost-http next to claudia, PATH, or pass -bifrost-bin). From repo root:")
				fmt.Fprintln(os.Stderr, "  make claudia-install")
				fmt.Fprintln(os.Stderr, "  ./claudia serve -bifrost-bin ./bin/bifrost-http")
				fmt.Fprintln(os.Stderr, "Or: make package  (full folder with bifrost-http + qdrant + config)")
				fmt.Fprintln(os.Stderr, "See docs/supervisor.md — Obtaining the BiFrost binary.")
			}
			os.Exit(1)
		}
		bifrostWaitErr = make(chan error, 1)
		go func() {
			bifrostWaitErr <- bifrostProc.Wait()
		}()

		if !*noWait {
			wCtx, wCancel := context.WithTimeout(context.Background(), *waitTimeout)
			err := supervisor.WaitHealthy(wCtx, healthURL, *waitTimeout, log, "bifrost")
			wCancel()
			if err != nil {
				stopChildren()
				if qdrantWait != nil {
					<-qdrantWait
				}
				<-bifrostWaitErr
				fmt.Fprintf(os.Stderr, "claudia serve: bifrost not healthy: %v\n", err)
				os.Exit(1)
			}
		}

		idxScope := res.IndexerSupervisedEnabled && (res.RAG.Enabled || res.IndexerSupervisedStartWhenRAGDisabled)
		if idxScope {
			idxBin := strings.TrimSpace(res.IndexerSupervisedBin)
			if idxBin == "" {
				idxBin = defaultSupervisorIndexerBin()
			}
			wd, werr := os.Getwd()
			if werr != nil {
				if log != nil {
					log.Warn("indexer supervised: getwd", "msg", "gateway.supervisor.indexer.not_started", "err", werr, "detail", "getwd")
				}
			} else {
				idxSink := logStore.Writer("indexer")
				gwLocal := gatewayPublicURLFromResolved(res)
				var ierr error
				indexerProc, ierr = supervisor.StartIndexer(childCtx, supervisor.IndexerConfig{
					Bin:        idxBin,
					ConfigPath: res.IndexerSupervisedConfigPath,
					WorkDir:    wd,
					GatewayURL: gwLocal,
					LogJSON:    res.IndexerSupervisedLogJSON,
					Stdout:     platform.StdoutTee(idxSink),
					Stderr:     platform.StderrTee(idxSink),
				}, log)
				if ierr != nil {
					if log != nil {
						log.Warn("indexer supervised not started", "msg", "gateway.supervisor.indexer.not_started", "err", ierr, "bin", idxBin)
					}
				} else {
					indexerWait = make(chan error, 1)
					go func() {
						indexerWait <- indexerProc.Wait()
					}()
				}
			}
		}
	} else if log != nil {
		log.Info("gateway bootstrap mode",
			"msg", "gateway.startup.bootstrap",
			"api_keys_path", res.TokensPath, "tokens_path", res.TokensPath,
			"hint", "create gateway token at /ui/setup, then restart")
	}

	addr := server.ListenAddrOverride(res, *listen)
	qdrantHTTP := ""
	if qBin != "" {
		qdrantHTTP = fmt.Sprintf("%s:%d", strings.TrimSpace(*qdrantHealthHost), *qdrantHTTPPort)
	}
	idxPath := ""
	if indexerWait != nil {
		idxPath = res.IndexerSupervisedConfigPath
	}
	overlay := &server.StatusOverlay{
		EffectiveListen: addr,
		Supervisor: &server.SupervisorInfo{
			BifrostListen:     fmt.Sprintf("%s:%d", strings.TrimSpace(*bifrostBind), *bifrostPort),
			QdrantSupervised:  qBin != "" && !bootstrap,
			QdrantHTTP:        qdrantHTTP,
			IndexerSupervised: indexerWait != nil,
			IndexerConfigPath: idxPath,
		},
	}
	if bootstrap {
		overlay.Supervisor = nil
	}

	uiOpts := server.NewUIOptions()
	uiOpts.LogStore = logStore
	var h http.Handler
	if bootstrap {
		h = server.NewBootstrapMux(rt, log, overlay)
	} else {
		h = server.NewMux(rt, log, overlay, uiOpts)
	}

	rootCtx, stopRoot := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopRoot()

	var entryURL string
	var serveErr error

	if bootstrap {
		addrs := server.BootstrapTCPAddrs(res, *listen)
		prim, shut, stopped, lerr := server.StartHTTPListeners(h, addrs, log)
		if lerr != nil {
			stopChildren()
			if qdrantWait != nil {
				<-qdrantWait
			}
			if bifrostWaitErr != nil {
				<-bifrostWaitErr
			}
			if log != nil {
				log.Error("listen", "msg", "gateway.listen.failed", "addrs", addrs, "err", lerr)
			}
			fmt.Fprintf(os.Stderr, "claudia serve: listen: %v\n", lerr)
			os.Exit(1)
		}
		overlay.EffectiveListen = prim.String()
		entryURL = gatewayPublicURL(prim) + "/ui/setup"
		if openWebview {
			entryURL = webviewEntryURL(prim, true)
		}
		go func() {
			<-rootCtx.Done()
			shCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			if err := shut(shCtx); err != nil && log != nil {
				log.Info("http shutdown", "msg", "gateway.shutdown.http", "err", err)
			}
		}()
		log.Info("gateway listening", "msg", "gateway.startup.listening",
			"bootstrap", true, "addr", prim.String(), "ui", entryURL, "config", path)
		runDesktopWebview(openWebview, entryURL, stopRoot, rootCtx)
		<-stopped
	} else {
		srv := &http.Server{Handler: h}
		ln, lerr := net.Listen("tcp", addr)
		if lerr != nil {
			stopChildren()
			if qdrantWait != nil {
				<-qdrantWait
			}
			if bifrostWaitErr != nil {
				<-bifrostWaitErr
			}
			if log != nil {
				log.Error("listen", "msg", "gateway.listen.failed", "addr", addr, "err", lerr)
			}
			fmt.Fprintf(os.Stderr, "claudia serve: listen %s: %v\n", addr, lerr)
			os.Exit(1)
		}
		entryURL = panelURLFromListenAddr(ln.Addr())
		if openWebview {
			entryURL = webviewEntryURL(ln.Addr(), false)
		}
		serveErrCh := make(chan error, 1)
		go func() {
			serveErrCh <- srv.Serve(ln)
		}()
		go func() {
			<-rootCtx.Done()
			shCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			if err := srv.Shutdown(shCtx); err != nil && log != nil {
				log.Info("http shutdown", "msg", "gateway.shutdown.http", "err", err)
			}
		}()
		log.Info("gateway listening", "msg", "gateway.startup.listening",
			"addr", ln.Addr().String(), "ui", entryURL, "upstream", upstreamURL, "bifrost_data", *bifrostDataDir, "qdrant_supervised", qBin != "", "indexer_supervised", indexerWait != nil, "config", path)
		// Periodic BiFrost `/v1/models` poll. The catalog snapshot drives the Provider health
		// strip (logs UI) and is the anchor for future routing-rule / fallback-chain /
		// embedding-model / router-model auditors registered via server.RegisterCatalogAuditor.
		// Period configurable via gateway.yaml `health.available_models_poll_ms`; <=0 disables.
		var pollInterval time.Duration
		if rtRes, _, _ := rt.Snapshot(); rtRes != nil {
			pollInterval = time.Duration(rtRes.AvailableModelsPollMs) * time.Millisecond
		}
		// Defer the first refresh briefly so BiFrost has a chance to come up healthy.
		go func() {
			time.Sleep(500 * time.Millisecond)
			server.StartCatalogPoller(rootCtx, rt, log, pollInterval)
		}()
		upstream.RunGatewayUpstreamHealthMonitor(rootCtx, log, 15*time.Second, 30*time.Second,
			func(pctx context.Context) (string, string, time.Duration, bool) {
				r, _, _ := rt.Snapshot()
				if r == nil || strings.TrimSpace(r.HealthUpstreamURL) == "" {
					return "", "", 0, false
				}
				to := time.Duration(r.HealthTimeoutMs) * time.Millisecond
				if to <= 0 {
					to = 5 * time.Second
				}
				return r.HealthUpstreamURL, rt.UpstreamAPIKey(), to, true
			})
		upstream.RunSupervisedChildHealthMonitor(rootCtx, log, "bifrost", healthURL, 15*time.Second, 30*time.Second, !*noWait)
		if qdrantReadyzURL != "" {
			upstream.RunSupervisedChildHealthMonitor(rootCtx, log, "qdrant", qdrantReadyzURL, 15*time.Second, 30*time.Second, !*noWaitQdrant)
		}
		runDesktopWebview(openWebview, entryURL, stopRoot, rootCtx)
		serveErr = <-serveErrCh
	}

	stopChildren()
	waitForChildExit("qdrant", qdrantProc, qdrantWait, 10*time.Second, log)
	waitForChildExit("bifrost", bifrostProc, bifrostWaitErr, 10*time.Second, log)
	waitForChildExit("indexer", indexerProc, indexerWait, 10*time.Second, log)

	if serveErr != nil && serveErr != http.ErrServerClosed {
		log.Error("http server", "msg", "gateway.http.server_error", "err", serveErr)
		stopRoot()
		os.Exit(1)
	}
}
