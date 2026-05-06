package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/lynn/claudia-gateway/internal/config"
	"github.com/lynn/claudia-gateway/internal/platform"
	"github.com/lynn/claudia-gateway/internal/server"
	"github.com/lynn/claudia-gateway/internal/servicelogs"
	"github.com/lynn/claudia-gateway/internal/supervisor"
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

// webviewEntryURL is opened by the native desktop shell: setup (bootstrap) or login → /ui/desktop.
func webviewEntryURL(ln net.Addr, bootstrap bool) string {
	base := gatewayPublicURL(ln)
	if bootstrap {
		return base + "/ui/setup"
	}
	return base + "/ui/login?next=" + url.PathEscape("/ui/desktop")
}

func waitForChildExit(name string, cmd *exec.Cmd, waitCh <-chan error, timeout time.Duration, log *slog.Logger) {
	if waitCh == nil {
		return
	}
	select {
	case werr := <-waitCh:
		if werr != nil && log != nil {
			log.Debug(name+" process finished", "err", werr)
		}
		return
	case <-time.After(timeout):
	}

	if cmd != nil && cmd.Process != nil {
		if log != nil {
			log.Warn(name+" did not exit after context cancel; forcing kill", "pid", cmd.Process.Pid, "timeout", timeout)
		}
		if err := cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) && log != nil {
			log.Warn(name+" kill failed", "err", err)
		}
	}

	select {
	case werr := <-waitCh:
		if werr != nil && log != nil {
			log.Debug(name+" process finished after kill", "err", werr)
		}
	case <-time.After(5 * time.Second):
		if log != nil {
			log.Warn(name + " still has not exited after forced kill")
		}
	}
}

func runServe(args []string, openWebview bool) {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	configPath := fs.String("config", "", "Path to gateway.yaml (default: $CLAUDIA_GATEWAY_CONFIG or ./config/gateway.yaml)")
	listen := fs.String("listen", "", "Override Claudia listen address (host:port or :port)")

	bifrostBin := fs.String("bifrost-bin", defaultSupervisorBifrostBin(), "BiFrost HTTP binary (PATH or path; defaults to bifrost-http next to this executable if present, else bifrost on PATH)")
	bifrostConfig := fs.String("bifrost-config", "config/bifrost.config.json", "Source bifrost.config.json (copied to data dir as config.json)")
	bifrostDataDir := fs.String("bifrost-data-dir", "data/bifrost", "BiFrost working directory (created; SQLite and config live here)")
	bifrostBind := fs.String("bifrost-bind", "127.0.0.1", "BiFrost bind address (-host)")
	bifrostPort := fs.Int("bifrost-port", 8080, "BiFrost listen port (-port)")
	bifrostLogLevel := fs.String("bifrost-log-level", "info", "BiFrost -log-level (debug, info, warn, error)")
	bifrostLogStyle := fs.String("bifrost-log-style", "json", "BiFrost -log-style (json or pretty)")
	upstreamHost := fs.String("upstream-host", "127.0.0.1", "Host for gateway upstream.base_url (Claudia → BiFrost); use 127.0.0.1 when bifrost-bind is 0.0.0.0")
	waitTimeout := fs.Duration("wait-bifrost", 60*time.Second, "Max time to poll BiFrost /health before exit")
	noWait := fs.Bool("no-wait-bifrost", false, "Skip readiness poll (not recommended)")

	qdrantBin := fs.String("qdrant-bin", defaultSupervisorQdrantBin(), "Qdrant binary (PATH or path); empty skips Qdrant (defaults to qdrant next to this executable if present)")
	qdrantStorage := fs.String("qdrant-storage", "data/qdrant", "Qdrant storage directory (created)")
	qdrantBind := fs.String("qdrant-bind", "127.0.0.1", "Qdrant QDRANT__SERVICE__HOST")
	qdrantHTTPPort := fs.Int("qdrant-http-port", 6333, "Qdrant HTTP port")
	qdrantGRPCPort := fs.Int("qdrant-grpc-port", 6334, "Qdrant gRPC port")
	qdrantHealthHost := fs.String("qdrant-health-host", "127.0.0.1", "Host for GET /readyz probe (use 127.0.0.1 when qdrant-bind is 0.0.0.0)")
	waitQdrant := fs.Duration("wait-qdrant", 60*time.Second, "Max time to poll Qdrant /readyz before exit")
	noWaitQdrant := fs.Bool("no-wait-qdrant", false, "Skip Qdrant readiness poll")

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
	bootstrap := server.BootstrapMode(rt)
	res, _, _ := rt.Snapshot()
	qBin := strings.TrimSpace(*qdrantBin)

	childCtx, stopChildren := context.WithCancel(context.Background())
	var qdrantProc *exec.Cmd
	var qdrantWait chan error
	var bifrostProc *exec.Cmd
	var bifrostWaitErr chan error
	var indexerProc *exec.Cmd
	var indexerWait chan error

	if !bootstrap {
		if qBin != "" {
			qSink := logStore.Writer("qdrant")
			qcfg := supervisor.QdrantConfig{
				Bin:        qBin,
				StorageDir: *qdrantStorage,
				BindHost:   strings.TrimSpace(*qdrantBind),
				HTTPPort:   *qdrantHTTPPort,
				GRPCPort:   *qdrantGRPCPort,
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
				qHealth := fmt.Sprintf("http://%s:%d/readyz", strings.TrimSpace(*qdrantHealthHost), *qdrantHTTPPort)
				wCtx, wCancel := context.WithTimeout(context.Background(), *waitQdrant)
				err := supervisor.WaitHealthy(wCtx, qHealth, *waitQdrant, log)
				wCancel()
				if err != nil {
					stopChildren()
					<-qdrantWait
					fmt.Fprintf(os.Stderr, "claudia serve: qdrant not healthy: %v\n", err)
					os.Exit(1)
				}
			}
		}

		bSink := logStore.Writer("bifrost")
		bcfg := supervisor.BifrostConfig{
			Bin:        *bifrostBin,
			ConfigJSON: *bifrostConfig,
			DataDir:    *bifrostDataDir,
			BindHost:   strings.TrimSpace(*bifrostBind),
			Port:       *bifrostPort,
			LogLevel:   strings.TrimSpace(*bifrostLogLevel),
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
			err := supervisor.WaitHealthy(wCtx, healthURL, *waitTimeout, log)
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
					log.Warn("indexer supervised: getwd", "err", werr)
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
						log.Warn("indexer supervised not started", "err", ierr, "bin", idxBin)
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
		log.Info("claudia serve: bootstrap mode (create gateway token at /ui/setup, then restart)", "tokens_path", res.TokensPath)
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
				log.Error("listen", "addrs", addrs, "err", lerr)
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
				log.Warn("http shutdown", "err", err)
			}
		}()
		log.Info("claudia serve: gateway listening (bootstrap)", "addr", prim.String(), "ui", entryURL, "config", path)
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
				log.Error("listen", "addr", addr, "err", lerr)
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
				log.Warn("http shutdown", "err", err)
			}
		}()
		log.Info("claudia serve: gateway listening", "addr", ln.Addr().String(), "ui", entryURL, "upstream", upstreamURL, "bifrost_data", *bifrostDataDir, "qdrant_supervised", qBin != "", "indexer_supervised", indexerWait != nil, "config", path)
		runDesktopWebview(openWebview, entryURL, stopRoot, rootCtx)
		serveErr = <-serveErrCh
	}

	stopChildren()
	waitForChildExit("qdrant", qdrantProc, qdrantWait, 10*time.Second, log)
	waitForChildExit("bifrost", bifrostProc, bifrostWaitErr, 10*time.Second, log)
	waitForChildExit("indexer", indexerProc, indexerWait, 10*time.Second, log)

	if serveErr != nil && serveErr != http.ErrServerClosed {
		log.Error("http server", "err", serveErr)
		stopRoot()
		os.Exit(1)
	}
}
