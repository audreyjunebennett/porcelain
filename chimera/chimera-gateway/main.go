package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/lynn/porcelain/chimera/internal/gatewayline"
	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/proc"
	"github.com/lynn/porcelain/chimera/chimera-gateway/internal/server"
	"github.com/lynn/porcelain/chimera/internal/config"
	"github.com/lynn/porcelain/chimera/internal/logfmt"
	"github.com/lynn/porcelain/chimera/internal/netaddr"
	"github.com/lynn/porcelain/chimera/internal/platform"
	"github.com/lynn/porcelain/chimera/internal/servicelogs"
	"github.com/lynn/porcelain/chimera/internal/supervisorlogs"
	"github.com/lynn/porcelain/chimera/internal/upstream"
	"github.com/lynn/porcelain/chimera/internal/wrapper/contract"
	wruntime "github.com/lynn/porcelain/chimera/internal/wrapper/runtime"
	"github.com/lynn/porcelain/internal/naming"
)

func main() {
	_ = godotenv.Load("env")
	_ = godotenv.Load(".env")

	args := os.Args[1:]
	if len(args) > 0 && args[0] == "--gateway-backend" {
		if err := runGatewayBackend(args[1:]); err != nil {
			fmt.Fprintf(os.Stderr, "chimera-gateway backend: %v\n", err)
			os.Exit(1)
		}
		return
	}
	if len(args) > 0 && (args[0] == "-version" || args[0] == "--version") {
		fmt.Printf("chimera-gateway %s\ncommit %s\nbuild date %s\n", version, commit, date)
		return
	}
	for _, a := range args {
		if a == "-h" || a == "--help" {
			printHelp()
			return
		}
	}
	if err := run(args); err != nil {
		fmt.Fprintf(os.Stderr, "chimera-gateway: %v\n", err)
		os.Exit(exitCodeForError(err))
	}
}

func printHelp() {
	fmt.Printf(`Chimera gateway runtime

Usage:
  chimera-gateway [flags]
  chimera-gateway -version

Flags:
`)
	fs := flag.NewFlagSet("chimera-gateway", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	_ = fs.String("listen", "", "Wrapper listen address (default: 127.0.0.1:7720)")
	_ = fs.String("bin", "", "Gateway backend binary path")
	_ = fs.String("config", "", "Path to "+naming.GatewayConfigFileTarget+" (default: $"+naming.EnvGatewayConfigTarget+" or ./"+naming.DefaultGatewayConfigRelPath+")")
	_ = fs.String("gateway-listen", "", "Backend listen override passed to gateway binary")
	fs.PrintDefaults()
}

type gatewayConfig struct {
	Listen                 string
	Bin                    string
	ConfigPath             string
	GatewayListen          string
	UpstreamOverride       string
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
}

func parseConfig(args []string) (gatewayConfig, error) {
	fs := flag.NewFlagSet("chimera-gateway", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	cfg := gatewayConfig{}
	var showVersion bool
	fs.StringVar(&cfg.Listen, "listen", envOrDefault(naming.EnvGatewayListen, naming.DefaultGatewayListen), "wrapper listen addr (host:port)")
	fs.StringVar(&cfg.Bin, "bin", envOrDefault(naming.EnvGatewayBin, defaultGatewayBackendBin()), "gateway backend binary path")
	fs.StringVar(&cfg.ConfigPath, "config", envOrDefault(naming.EnvGatewayConfigTarget, ""), "path to "+naming.GatewayConfigFileTarget)
	fs.StringVar(&cfg.GatewayListen, "gateway-listen", envOrDefault(naming.EnvGatewayBackendListen, ""), "backend listen override")
	fs.StringVar(&cfg.UpstreamOverride, "upstream-override", envOrDefault(naming.EnvGatewayUpstreamOverride, ""), "override upstream base URL for backend runtime")
	fs.DurationVar(&cfg.StartupTimeout, "startup-timeout", envDuration(naming.EnvGatewayTimeoutsStartup, contract.DefaultStartupTimeout), "startup readiness timeout")
	fs.DurationVar(&cfg.ShutdownTimeout, "shutdown-timeout", envDuration(naming.EnvGatewayTimeoutsShutdown, contract.DefaultShutdownTimeout), "wrapper graceful shutdown timeout")
	fs.DurationVar(&cfg.TerminateWait, "terminate-wait", contract.DefaultTerminateWait, "wait before force-kill backend")
	fs.DurationVar(&cfg.BackoffInitial, "backoff-initial", contract.DefaultBackoffInitial, "restart backoff initial delay")
	fs.Float64Var(&cfg.BackoffMultiplier, "backoff-multiplier", contract.DefaultBackoffMultiplier, "restart backoff multiplier")
	fs.DurationVar(&cfg.BackoffMax, "backoff-max", contract.DefaultBackoffMax, "restart backoff max delay")
	fs.DurationVar(&cfg.BackoffResetAfter, "backoff-reset-after", contract.DefaultBackoffResetAfter, "healthy runtime to reset backoff")
	fs.BoolVar(&cfg.DebugEnableUpstream, "debug-enable-upstream-logs", wruntime.EnvBool(contract.DebugEnableEnvKey), "enable /debug/upstream/logs")
	fs.BoolVar(&cfg.DebugAllowRemote, "debug-allow-remote", wruntime.EnvBool(contract.DebugAllowRemoteEnv), "allow /debug/* on non-loopback bind")
	fs.BoolVar(&cfg.ForwardUpstreamInDebug, "debug-forward-upstream", false, "forward upstream lines to stderr in debug mode")
	fs.StringVar(&cfg.UpstreamVersion, "upstream-version", "", "optional gateway upstream version for status payload")
	fs.BoolVar(&showVersion, "version", false, "print version")
	fs.BoolVar(&showVersion, "v", false, "print version")
	if err := fs.Parse(args); err != nil {
		return cfg, err
	}
	if showVersion {
		fmt.Printf("chimera-gateway %s\ncommit %s\nbuild date %s\n", version, commit, date)
		return cfg, io.EOF
	}
	return cfg, nil
}

func run(args []string) error {
	cfg, err := parseConfig(args)
	if err != nil {
		if errors.Is(err, io.EOF) {
			return nil
		}
		return wruntime.WrapExitError(contract.ExitConfigError, err)
	}
	log := logfmt.NewLogger(os.Stderr, logfmt.JSONEnabled(), slog.LevelInfo)
	adapter := &gatewayAdapter{cfg: cfg}

	rootCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return wruntime.Run(rootCtx, wruntime.Config{
		Component:              contract.ComponentGateway,
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
		ReadyMessage:           "gateway.ready",
		UpstreamLineMessage:    "gateway.upstream.line",
		HTTPServerErrorMessage: "gateway.wrapper.http.server_error",
		UpstreamLineWrapper:    wrapGatewayLine,
	}, adapter, log)
}

func exitCodeForError(err error) int {
	var ee *wruntime.ExitError
	if errors.As(err, &ee) {
		return ee.Code
	}
	return contract.ExitInternal
}

type gatewayAdapter struct {
	cfg gatewayConfig
}

func (a *gatewayAdapter) Start(ctx context.Context, capture io.Writer, log *slog.Logger) (*exec.Cmd, error) {
	path := strings.TrimSpace(a.cfg.ConfigPath)
	if path == "" {
		var err error
		path, err = config.ResolveGatewayConfigPath()
		if err != nil {
			return nil, err
		}
	}
	res, err := config.LoadGatewayYAML(path, log)
	if err != nil {
		return nil, err
	}
	listen := strings.TrimSpace(a.cfg.GatewayListen)
	if listen == "" {
		listen = res.ListenAddr()
	}
	if _, _, err := parseListenHostPort(listen); err != nil {
		return nil, err
	}

	bin := strings.TrimSpace(a.cfg.Bin)
	args := []string{"-config", path, "-listen", listen}
	if u := strings.TrimSpace(a.cfg.UpstreamOverride); u != "" {
		args = append(args, "-upstream-override", u)
	}
	if useEmbeddedBackend(bin) {
		exe, err := os.Executable()
		if err != nil {
			return nil, fmt.Errorf("resolve executable for embedded backend: %w", err)
		}
		args = append([]string{"--gateway-backend"}, args...)
		if log != nil && strings.Contains(strings.ToLower(filepath.Base(bin)), "supervisor") {
			log.Warn("ignoring supervisor backend for gateway-only mode", "msg", "gateway.wrapper.backend.supervisor_ignored", "requested_bin", bin)
		}
		bin = exe
	}

	out := io.MultiWriter(capture, os.Stdout)
	stdout := gatewayline.NewWriter(out)
	stderr := gatewayline.NewWriter(out)
	cmd := exec.CommandContext(ctx, bin, args...)
	proc.ApplyNoConsoleWindow(cmd)
	cmd.Env = os.Environ()
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start gateway backend: %w", err)
	}
	return cmd, nil
}

func (a *gatewayAdapter) ReadyURL() string {
	path := strings.TrimSpace(a.cfg.ConfigPath)
	if path == "" {
		return "http://127.0.0.1:3000/health"
	}
	res, err := config.LoadGatewayYAML(path, nil)
	if err != nil {
		return "http://127.0.0.1:3000/health"
	}
	listen := strings.TrimSpace(a.cfg.GatewayListen)
	if listen == "" {
		listen = res.ListenAddr()
	}
	if useEmbeddedBackend(a.cfg.Bin) {
		return "http://" + listen + "/"
	}
	return "http://" + listen + "/health"
}

func (a *gatewayAdapter) MetricsURL() string {
	if useEmbeddedBackend(a.cfg.Bin) {
		return ""
	}
	path := strings.TrimSpace(a.cfg.ConfigPath)
	if path == "" {
		return ""
	}
	res, err := config.LoadGatewayYAML(path, nil)
	if err != nil {
		return ""
	}
	listen := strings.TrimSpace(a.cfg.GatewayListen)
	if listen == "" {
		listen = res.ListenAddr()
	}
	return "http://" + listen + "/metrics"
}

func (a *gatewayAdapter) BackendName() string {
	return "custom"
}

func parseListenHostPort(addr string) (string, int, error) {
	host, portStr, ok := strings.Cut(strings.TrimSpace(addr), ":")
	if !ok {
		return "", 0, fmt.Errorf("invalid listen address %q, expected host:port", addr)
	}
	host = strings.TrimSpace(host)
	portStr = strings.TrimSpace(portStr)
	if host == "" || portStr == "" {
		return "", 0, fmt.Errorf("invalid listen address %q, expected host:port", addr)
	}
	p, err := strconv.Atoi(portStr)
	if err != nil || p <= 0 || p > 65535 {
		return "", 0, fmt.Errorf("invalid listen port in %q", addr)
	}
	return host, p, nil
}

func defaultGatewayBackendBin() string {
	return envOrDefault(naming.EnvGatewayBackendBinDefault, naming.ProductGatewayBackendBinName)
}

func wrapGatewayLine(raw string) string {
	return string(gatewayline.NormalizePayload(raw))
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

func useEmbeddedBackend(bin string) bool {
	bin = strings.TrimSpace(bin)
	if bin == "" {
		return true
	}
	base := strings.ToLower(filepath.Base(bin))
	if strings.Contains(base, "chimera-supervisor") {
		return true
	}
	if strings.Contains(base, "chimera-gateway") && !strings.Contains(base, "backend") {
		return true
	}
	if base == naming.ProductGatewayBackendBinName || base == naming.ProductGatewayBackendBinName+".exe" {
		if _, err := exec.LookPath(bin); err != nil {
			return true
		}
	}
	return false
}

func runGatewayBackend(args []string) error {
	fs := flag.NewFlagSet("chimera-gateway-backend", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	cfgPath := envOrDefault(naming.EnvGatewayConfigTarget, "")
	listen := ""
	upstreamOverride := ""
	fs.StringVar(&cfgPath, "config", cfgPath, "path to "+naming.GatewayConfigFileTarget)
	fs.StringVar(&listen, "listen", "", "listen override")
	fs.StringVar(&upstreamOverride, "upstream-override", envOrDefault(naming.EnvGatewayUpstreamOverride, ""), "upstream base URL override")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(cfgPath) == "" {
		var err error
		cfgPath, err = config.ResolveGatewayConfigPath()
		if err != nil {
			return err
		}
	}
	log := buildLogger(cfgPath)
	rt, err := server.NewRuntimeWithUpstreamOverride(cfgPath, log, strings.TrimSpace(upstreamOverride))
	if err != nil {
		return err
	}
	res, _, _ := rt.Snapshot()
	addr := netaddr.ListenAddrOverride(res, listen)
	overlay := &server.StatusOverlay{EffectiveListen: addr}
	rootCtx, stopRoot := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopRoot()

	if server.BootstrapMode(rt) {
		h := server.NewBootstrapMux(rt, log, overlay)
		addrs := server.BootstrapTCPAddrs(res, listen)
		prim, shut, stopped, err := server.StartHTTPListeners(h, addrs, log)
		if err != nil {
			return err
		}
		overlay.EffectiveListen = prim.String()
		go func() {
			<-rootCtx.Done()
			shCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			_ = shut(shCtx)
		}()
		<-stopped
		return nil
	}

	uiOpts := server.NewUIOptions()
	if logfmt.SupervisedMode() {
		logStore := servicelogs.New(servicelogs.DefaultMaxLines)
		uiOpts.LogStore = logStore
		if supURL := strings.TrimSpace(os.Getenv(naming.EnvSupervisorControlURLTarget)); supURL != "" {
			supervisorlogs.StartMirror(rootCtx, supURL, logStore, log)
		} else if log != nil {
			log.Warn("supervised gateway missing supervisor control URL; logs UI disabled",
				"msg", "gateway.supervisor_logs.control_url_missing",
				"env", naming.EnvSupervisorControlURLTarget)
		}
	}
	h := server.NewMux(rt, log, overlay, uiOpts)
	srv := &http.Server{Handler: h}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	var pollInterval time.Duration
	if rtRes, _, _ := rt.Snapshot(); rtRes != nil {
		pollInterval = time.Duration(rtRes.AvailableModelsPollMs) * time.Millisecond
	}
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
	go func() {
		<-rootCtx.Done()
		shCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		_ = srv.Shutdown(shCtx)
	}()
	serveErr := srv.Serve(ln)
	if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
		return serveErr
	}
	return nil
}

func buildLogger(gatewayPath string) *slog.Logger {
	return buildLoggerTo(os.Stdout, gatewayPath)
}

func buildLoggerTo(w io.Writer, gatewayPath string) *slog.Logger {
	lvl := slog.LevelInfo
	if e := os.Getenv("LOG_LEVEL"); e != "" {
		lvl = parseLogLevel(e)
	} else {
		res, err := config.LoadGatewayYAML(gatewayPath, nil)
		if err == nil {
			lvl = parseLogLevel(res.LogLevel)
		}
	}
	return logfmt.NewLogger(w, logfmt.JSONEnabled(), lvl)
}

func parseLogLevel(s string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "trace":
		return platform.LevelTrace
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
