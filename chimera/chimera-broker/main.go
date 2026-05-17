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
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/lynn/porcelain/chimera/chimera-broker/brokerline"
	"github.com/lynn/porcelain/chimera/internal/logfmt"
	"github.com/lynn/porcelain/chimera/internal/wrapper/contract"
	wruntime "github.com/lynn/porcelain/chimera/internal/wrapper/runtime"
)

const componentName = contract.ComponentBroker

func main() {
	_ = godotenv.Load("env")
	_ = godotenv.Load(".env")
	for _, a := range os.Args[1:] {
		if a == "-h" || a == "--help" {
			printHelp()
			return
		}
	}
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "chimera-broker: %v\n", err)
		os.Exit(exitCodeForError(err))
	}
}

func printHelp() {
	fmt.Printf(`Chimera broker runtime

Usage:
  chimera-broker [flags]
  chimera-broker -version

Flags:
`)
	fs := flag.NewFlagSet("chimera-broker", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	_ = fs.String("listen", envOrDefault("BROKER__LISTEN", "127.0.0.1:7730"), "wrapper listen addr (host:port)")
	_ = fs.String("bin", envOrDefault("BROKER__BIN", "chimera-broker-http"), "backend binary path")
	_ = fs.String("backend", envOrDefault("BROKER__BACKEND", "chimera-broker"), "backend name")
	_ = fs.String("endpoint", envOrDefault("BROKER__ENDPOINT", "127.0.0.1:8080"), "backend endpoint host:port")
	_ = fs.String("data-path", envOrDefault("BROKER__DATA_PATH", "data/chimera-broker"), "backend data path")
	_ = fs.String("log-level", envOrDefault("BROKER__LOG_LEVEL", "info"), "backend log level")
	_ = fs.Duration("startup-timeout", envDuration("BROKER__TIMEOUTS__STARTUP", contract.DefaultStartupTimeout), "startup readiness timeout")
	_ = fs.Duration("shutdown-timeout", envDuration("BROKER__TIMEOUTS__SHUTDOWN", contract.DefaultShutdownTimeout), "wrapper graceful shutdown timeout")
	_ = fs.Duration("terminate-wait", contract.DefaultTerminateWait, "wait before force-kill backend")
	_ = fs.Duration("backoff-initial", contract.DefaultBackoffInitial, "restart backoff initial delay")
	_ = fs.Float64("backoff-multiplier", contract.DefaultBackoffMultiplier, "restart backoff multiplier")
	_ = fs.Duration("backoff-max", contract.DefaultBackoffMax, "restart backoff max delay")
	_ = fs.Duration("backoff-reset-after", contract.DefaultBackoffResetAfter, "healthy runtime to reset backoff")
	_ = fs.Bool("debug-enable-upstream-logs", wruntime.EnvBool(contract.DebugEnableEnvKey), "enable /debug/upstream/logs")
	_ = fs.Bool("debug-allow-remote", wruntime.EnvBool(contract.DebugAllowRemoteEnv), "allow /debug/* on non-loopback bind")
	_ = fs.Bool("debug-forward-upstream", false, "forward upstream lines to stderr in debug mode")
	_ = fs.String("upstream-version", "", "optional upstream version for status payload")
	_ = fs.String("chimera-broker-config", envOrDefault("BROKER__CHIMERA_BROKER_CONFIG", "config/chimera-broker.config.json"), "chimera-broker config JSON path")
	_ = fs.String("chimera-broker-log-style", envOrDefault("BROKER__CHIMERA_BROKER_LOG_STYLE", "json"), "chimera-broker log style")
	_ = fs.Bool("version", false, "print version")
	_ = fs.Bool("v", false, "print version")
	fs.PrintDefaults()
}

func exitCodeForError(err error) int {
	var ee *wruntime.ExitError
	if errors.As(err, &ee) {
		return ee.Code
	}
	return contract.ExitInternal
}

type brokerConfig struct {
	Listen                 string
	Bin                    string
	Backend                string
	Endpoint               string
	DataPath               string
	LogLevel               string
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
	ChimeraBrokerConfig    string
	ChimeraBrokerLogStyle  string
}

func parseConfig(args []string) (brokerConfig, error) {
	fs := flag.NewFlagSet("chimera-broker", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	cfg := brokerConfig{}
	var showVersion bool
	fs.StringVar(&cfg.Listen, "listen", envOrDefault("BROKER__LISTEN", "127.0.0.1:7730"), "wrapper listen addr (host:port)")
	fs.StringVar(&cfg.Bin, "bin", envOrDefault("BROKER__BIN", "chimera-broker-http"), "backend binary path")
	fs.StringVar(&cfg.Backend, "backend", envOrDefault("BROKER__BACKEND", "chimera-broker"), "backend name")
	fs.StringVar(&cfg.Endpoint, "endpoint", envOrDefault("BROKER__ENDPOINT", "127.0.0.1:8080"), "backend endpoint host:port")
	fs.StringVar(&cfg.DataPath, "data-path", envOrDefault("BROKER__DATA_PATH", "data/chimera-broker"), "backend data path")
	fs.StringVar(&cfg.LogLevel, "log-level", envOrDefault("BROKER__LOG_LEVEL", "info"), "backend log level")
	fs.DurationVar(&cfg.StartupTimeout, "startup-timeout", envDuration("BROKER__TIMEOUTS__STARTUP", contract.DefaultStartupTimeout), "startup readiness timeout")
	fs.DurationVar(&cfg.ShutdownTimeout, "shutdown-timeout", envDuration("BROKER__TIMEOUTS__SHUTDOWN", contract.DefaultShutdownTimeout), "wrapper graceful shutdown timeout")
	fs.DurationVar(&cfg.TerminateWait, "terminate-wait", contract.DefaultTerminateWait, "wait before force-kill backend")
	fs.DurationVar(&cfg.BackoffInitial, "backoff-initial", contract.DefaultBackoffInitial, "restart backoff initial delay")
	fs.Float64Var(&cfg.BackoffMultiplier, "backoff-multiplier", contract.DefaultBackoffMultiplier, "restart backoff multiplier")
	fs.DurationVar(&cfg.BackoffMax, "backoff-max", contract.DefaultBackoffMax, "restart backoff max delay")
	fs.DurationVar(&cfg.BackoffResetAfter, "backoff-reset-after", contract.DefaultBackoffResetAfter, "healthy runtime to reset backoff")
	fs.BoolVar(&cfg.DebugEnableUpstream, "debug-enable-upstream-logs", wruntime.EnvBool(contract.DebugEnableEnvKey), "enable /debug/upstream/logs")
	fs.BoolVar(&cfg.DebugAllowRemote, "debug-allow-remote", wruntime.EnvBool(contract.DebugAllowRemoteEnv), "allow /debug/* on non-loopback bind")
	fs.BoolVar(&cfg.ForwardUpstreamInDebug, "debug-forward-upstream", false, "forward upstream lines to stderr in debug mode")
	fs.StringVar(&cfg.UpstreamVersion, "upstream-version", "", "optional upstream version for status payload")
	defaultConfigPath := envOrDefault("BROKER__CHIMERA_BROKER_CONFIG", "config/chimera-broker.config.json")
	defaultLogStyle := envOrDefault("BROKER__CHIMERA_BROKER_LOG_STYLE", "json")
	fs.StringVar(&cfg.ChimeraBrokerConfig, "chimera-broker-config", defaultConfigPath, "chimera-broker config JSON path")
	fs.StringVar(&cfg.ChimeraBrokerLogStyle, "chimera-broker-log-style", defaultLogStyle, "chimera-broker log style")
	fs.BoolVar(&showVersion, "version", false, "print version")
	fs.BoolVar(&showVersion, "v", false, "print version")
	if err := fs.Parse(args); err != nil {
		return cfg, err
	}
	if showVersion {
		fmt.Printf("chimera-broker %s\ncommit %s\nbuild date %s\n", version, commit, date)
		return cfg, io.EOF
	}
	backendName := strings.TrimSpace(strings.ToLower(cfg.Backend))
	if backendName != "chimera-broker" {
		return cfg, fmt.Errorf("BROKER__BACKEND must be chimera-broker for binary mode")
	}
	return cfg, nil
}

type brokerAdapter struct {
	cfg brokerConfig
}

func run(args []string) error {
	cfg, err := parseConfig(args)
	if err != nil {
		if errors.Is(err, io.EOF) {
			return nil
		}
		return wruntime.WrapExitError(contract.ExitConfigError, err)
	}
	host, port, err := parseEndpoint(cfg.Endpoint)
	if err != nil {
		return wruntime.WrapExitError(contract.ExitConfigError, err)
	}
	log := logfmt.NewLogger(os.Stderr, logfmt.JSONEnabled(), slog.LevelInfo)
	adapter := &brokerAdapter{
		cfg: cfg,
	}

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
		ReadyMessage:           "broker.ready",
		UpstreamLineMessage:    "broker.upstream.line",
		HTTPServerErrorMessage: "broker.http.server_error",
		UpstreamLineWrapper:    wrapBrokerLine,
	}, &endpointBrokerAdapter{
		adapter: *adapter,
		host:    host,
		port:    port,
	}, log)
}

type endpointBrokerAdapter struct {
	adapter brokerAdapter
	host    string
	port    int
}

func (a *endpointBrokerAdapter) Start(ctx context.Context, capture io.Writer, log *slog.Logger) (*exec.Cmd, error) {
	cfg := ChimeraBrokerConfig{
		Bin:        a.adapter.cfg.Bin,
		ConfigJSON: a.adapter.cfg.ChimeraBrokerConfig,
		DataDir:    a.adapter.cfg.DataPath,
		BindHost:   a.host,
		Port:       a.port,
		LogLevel:   a.adapter.cfg.LogLevel,
		LogStyle:   a.adapter.cfg.ChimeraBrokerLogStyle,
		Stdout:     brokerline.NewWriter(io.MultiWriter(capture, os.Stdout)),
		Stderr:     brokerline.NewWriter(io.MultiWriter(capture, os.Stderr)),
	}
	return StartChimeraBroker(ctx, cfg, log)
}

func (a *endpointBrokerAdapter) ReadyURL() string {
	return fmt.Sprintf("http://%s:%d/models", strings.TrimSpace(a.host), a.port)
}

func (a *endpointBrokerAdapter) MetricsURL() string {
	return fmt.Sprintf("http://%s:%d%s", strings.TrimSpace(a.host), a.port, contract.MetricsPath)
}

func (a *endpointBrokerAdapter) BackendName() string {
	return "chimera-broker"
}

func nextBackoff(cfg brokerConfig, attempt int) time.Duration {
	return wruntime.NextBackoff(wruntime.BackoffConfig{
		Initial:    cfg.BackoffInitial,
		Multiplier: cfg.BackoffMultiplier,
		Max:        cfg.BackoffMax,
	}, attempt)
}

func isLoopbackBind(addr string) bool {
	return wruntime.IsLoopbackBind(addr)
}

func prefixUpstreamMetrics(raw string) string {
	return wruntime.PrefixUpstreamMetrics(raw)
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

func parseEndpoint(endpoint string) (string, int, error) {
	host, portStr, ok := strings.Cut(strings.TrimSpace(endpoint), ":")
	if !ok || strings.TrimSpace(host) == "" || strings.TrimSpace(portStr) == "" {
		return "", 0, fmt.Errorf("invalid endpoint %q, expected host:port", endpoint)
	}
	p, err := strconv.Atoi(strings.TrimSpace(portStr))
	if err != nil || p <= 0 || p > 65535 {
		return "", 0, fmt.Errorf("invalid endpoint port in %q", endpoint)
	}
	return strings.TrimSpace(host), p, nil
}

func wrapBrokerLine(raw string) string {
	return string(brokerline.NormalizePayload(raw))
}
