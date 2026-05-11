package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/lynn/claudia-gateway/internal/config"
	"github.com/lynn/claudia-gateway/internal/platform"
	"github.com/lynn/claudia-gateway/internal/server"
	"github.com/lynn/claudia-gateway/internal/servicelogs"
	"github.com/lynn/claudia-gateway/internal/upstream"
)

func runGateway(args []string) {
	fs := flag.NewFlagSet("claudia", flag.ExitOnError)
	configPath := fs.String("config", "", "Path to gateway.yaml (default: $CLAUDIA_GATEWAY_CONFIG or ./config/gateway.yaml)")
	listen := fs.String("listen", "", "Override listen address (default: gateway.listen_host:listen_port from yaml)")
	_ = fs.Parse(args)

	path := strings.TrimSpace(*configPath)
	if path == "" {
		var err error
		path, err = config.ResolveGatewayConfigPath()
		if err != nil {
			fmt.Fprintln(os.Stderr, "claudia:", err)
			os.Exit(2)
		}
	}

	logStore := servicelogs.New(servicelogs.DefaultMaxLines)
	gwSink := logStore.Writer("gateway")
	log := buildLoggerTo(platform.StdoutTee(gwSink), path)
	// Ensure the operator UI log buffer is never empty, even in GUI builds.
	log.Info("gateway startup seed", "msg", "gateway.startup.seed")
	rt, err := server.NewRuntime(path, log)
	if err != nil {
		fmt.Fprintf(os.Stderr, "claudia: load gateway config: %v\n", err)
		os.Exit(1)
	}

	res, _, _ := rt.Snapshot()
	addr := server.ListenAddrOverride(res, *listen)
	bootstrap := server.BootstrapMode(rt)
	overlay := &server.StatusOverlay{EffectiveListen: addr}
	if bootstrap {
		overlay.Supervisor = nil
	}

	uiOpts := server.NewUIOptions()
	uiOpts.LogStore = logStore

	if bootstrap {
		h := server.NewBootstrapMux(rt, log, overlay)
		addrs := server.BootstrapTCPAddrs(res, *listen)
		prim, shut, stopped, lerr := server.StartHTTPListeners(h, addrs, log)
		if lerr != nil {
			fmt.Fprintf(os.Stderr, "claudia: listen: %v\n", lerr)
			os.Exit(1)
		}
		overlay.EffectiveListen = prim.String()
		log.Info("gateway listening", "msg", "gateway.startup.listening",
			"bootstrap", true, "addr", prim.String(), "upstream", res.UpstreamBaseURL, "config", path)
		rootCtx, stopRoot := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stopRoot()
		go func() {
			<-rootCtx.Done()
			shCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			if err := shut(shCtx); err != nil && log != nil {
				log.Info("http shutdown", "msg", "gateway.shutdown.http", "err", err)
			}
		}()
		<-stopped
		return
	}

	h := server.NewMux(rt, log, overlay, uiOpts)
	log.Info("gateway listening", "msg", "gateway.startup.listening", "addr", addr, "upstream", res.UpstreamBaseURL, "config", path)
	rootCtx := context.Background()
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
	if err := http.ListenAndServe(addr, h); err != nil {
		log.Error("server exit", "msg", "gateway.http.server_error", "err", err)
		os.Exit(1)
	}
}
