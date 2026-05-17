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
	"github.com/lynn/porcelain/chimera/chimera-vectorstore/vectorstoreline"
	"github.com/lynn/porcelain/chimera/internal/logfmt"
	"github.com/lynn/porcelain/chimera/internal/wrapper/contract"
	wruntime "github.com/lynn/porcelain/chimera/internal/wrapper/runtime"
)

const componentName = contract.ComponentVectorstore

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
		fmt.Fprintf(os.Stderr, "chimera-vectorstore: %v\n", err)
		os.Exit(exitCodeForError(err))
	}
}

func printHelp() {
	fmt.Printf(`Chimera vectorstore runtime

Usage:
  chimera-vectorstore [flags]
  chimera-vectorstore -version

Flags:
`)
	fs := flag.NewFlagSet("chimera-vectorstore", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	_ = fs.String("listen", envOrDefault("VECTORSTORE__LISTEN", "127.0.0.1:7740"), "wrapper listen addr (host:port)")
	_ = fs.String("bin", envOrDefault("VECTORSTORE__BIN", "qdrant"), "backend binary path")
	_ = fs.String("backend", envOrDefault("VECTORSTORE__BACKEND", "qdrant"), "backend name")
	_ = fs.String("endpoint", envOrDefault("VECTORSTORE__ENDPOINT", "127.0.0.1:6333"), "backend endpoint host:port")
	_ = fs.String("data-path", envOrDefault("VECTORSTORE__DATA_PATH", "data/qdrant"), "backend data path")
	_ = fs.String("log-level", envOrDefault("VECTORSTORE__LOG_LEVEL", "info"), "backend log level")
	_ = fs.Duration("startup-timeout", envDuration("VECTORSTORE__TIMEOUTS__STARTUP", contract.DefaultStartupTimeout), "startup readiness timeout")
	_ = fs.Duration("shutdown-timeout", envDuration("VECTORSTORE__TIMEOUTS__SHUTDOWN", contract.DefaultShutdownTimeout), "wrapper graceful shutdown timeout")
	_ = fs.Duration("terminate-wait", contract.DefaultTerminateWait, "wait before force-kill backend")
	_ = fs.Duration("backoff-initial", contract.DefaultBackoffInitial, "restart backoff initial delay")
	_ = fs.Float64("backoff-multiplier", contract.DefaultBackoffMultiplier, "restart backoff multiplier")
	_ = fs.Duration("backoff-max", contract.DefaultBackoffMax, "restart backoff max delay")
	_ = fs.Duration("backoff-reset-after", contract.DefaultBackoffResetAfter, "healthy runtime to reset backoff")
	_ = fs.Bool("debug-enable-upstream-logs", wruntime.EnvBool(contract.DebugEnableEnvKey), "enable /debug/upstream/logs")
	_ = fs.Bool("debug-allow-remote", wruntime.EnvBool(contract.DebugAllowRemoteEnv), "allow /debug/* on non-loopback bind")
	_ = fs.Bool("debug-forward-upstream", false, "forward upstream lines to stderr in debug mode")
	_ = fs.String("upstream-version", "", "optional upstream version for status payload")
	_ = fs.Int("grpc-port", envInt("VECTORSTORE__GRPC_PORT", 6334), "qdrant grpc port")
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

type vectorstoreConfig struct {
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
	GRPCPort               int
}

func parseConfig(args []string) (vectorstoreConfig, error) {
	fs := flag.NewFlagSet("chimera-vectorstore", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	cfg := vectorstoreConfig{}
	var showVersion bool
	fs.StringVar(&cfg.Listen, "listen", envOrDefault("VECTORSTORE__LISTEN", "127.0.0.1:7740"), "wrapper listen addr (host:port)")
	fs.StringVar(&cfg.Bin, "bin", envOrDefault("VECTORSTORE__BIN", "qdrant"), "backend binary path")
	fs.StringVar(&cfg.Backend, "backend", envOrDefault("VECTORSTORE__BACKEND", "qdrant"), "backend name")
	fs.StringVar(&cfg.Endpoint, "endpoint", envOrDefault("VECTORSTORE__ENDPOINT", "127.0.0.1:6333"), "backend endpoint host:port")
	fs.StringVar(&cfg.DataPath, "data-path", envOrDefault("VECTORSTORE__DATA_PATH", "data/qdrant"), "backend data path")
	fs.StringVar(&cfg.LogLevel, "log-level", envOrDefault("VECTORSTORE__LOG_LEVEL", "info"), "backend log level")
	fs.DurationVar(&cfg.StartupTimeout, "startup-timeout", envDuration("VECTORSTORE__TIMEOUTS__STARTUP", contract.DefaultStartupTimeout), "startup readiness timeout")
	fs.DurationVar(&cfg.ShutdownTimeout, "shutdown-timeout", envDuration("VECTORSTORE__TIMEOUTS__SHUTDOWN", contract.DefaultShutdownTimeout), "wrapper graceful shutdown timeout")
	fs.DurationVar(&cfg.TerminateWait, "terminate-wait", contract.DefaultTerminateWait, "wait before force-kill backend")
	fs.DurationVar(&cfg.BackoffInitial, "backoff-initial", contract.DefaultBackoffInitial, "restart backoff initial delay")
	fs.Float64Var(&cfg.BackoffMultiplier, "backoff-multiplier", contract.DefaultBackoffMultiplier, "restart backoff multiplier")
	fs.DurationVar(&cfg.BackoffMax, "backoff-max", contract.DefaultBackoffMax, "restart backoff max delay")
	fs.DurationVar(&cfg.BackoffResetAfter, "backoff-reset-after", contract.DefaultBackoffResetAfter, "healthy runtime to reset backoff")
	fs.BoolVar(&cfg.DebugEnableUpstream, "debug-enable-upstream-logs", wruntime.EnvBool(contract.DebugEnableEnvKey), "enable /debug/upstream/logs")
	fs.BoolVar(&cfg.DebugAllowRemote, "debug-allow-remote", wruntime.EnvBool(contract.DebugAllowRemoteEnv), "allow /debug/* on non-loopback bind")
	fs.BoolVar(&cfg.ForwardUpstreamInDebug, "debug-forward-upstream", false, "forward upstream lines to stderr in debug mode")
	fs.StringVar(&cfg.UpstreamVersion, "upstream-version", "", "optional upstream version for status payload")
	fs.IntVar(&cfg.GRPCPort, "grpc-port", envInt("VECTORSTORE__GRPC_PORT", 6334), "qdrant grpc port")
	fs.BoolVar(&showVersion, "version", false, "print version")
	fs.BoolVar(&showVersion, "v", false, "print version")
	if err := fs.Parse(args); err != nil {
		return cfg, err
	}
	if showVersion {
		fmt.Printf("chimera-vectorstore %s\ncommit %s\nbuild date %s\n", version, commit, date)
		return cfg, io.EOF
	}
	if strings.TrimSpace(strings.ToLower(cfg.Backend)) != "qdrant" {
		return cfg, fmt.Errorf("VECTORSTORE__BACKEND must be qdrant for binary mode")
	}
	return cfg, nil
}

type endpointVectorstoreAdapter struct {
	cfg  vectorstoreConfig
	host string
	port int
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
	adapter := &endpointVectorstoreAdapter{
		cfg:  cfg,
		host: host,
		port: port,
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
		ReadyMessage:           "vectorstore.ready",
		UpstreamLineMessage:    "vectorstore.upstream.line",
		HTTPServerErrorMessage: "vectorstore.http.server_error",
		UpstreamLineWrapper:    wrapVectorstoreLine,
	}, adapter, log)
}

func (a *endpointVectorstoreAdapter) Start(ctx context.Context, capture io.Writer, log *slog.Logger) (*exec.Cmd, error) {
	cfg := QdrantConfig{
		Bin:        a.cfg.Bin,
		StorageDir: a.cfg.DataPath,
		BindHost:   a.host,
		HTTPPort:   a.port,
		GRPCPort:   a.cfg.GRPCPort,
		LogLevel:   a.cfg.LogLevel,
		Stdout:     vectorstoreline.NewWriter(io.MultiWriter(capture, os.Stdout)),
		Stderr:     vectorstoreline.NewWriter(io.MultiWriter(capture, os.Stderr)),
	}
	return StartQdrant(ctx, cfg, log)
}

func (a *endpointVectorstoreAdapter) ReadyURL() string {
	return fmt.Sprintf("http://%s:%d/collections", strings.TrimSpace(a.host), a.port)
}

func (a *endpointVectorstoreAdapter) MetricsURL() string {
	return fmt.Sprintf("http://%s:%d%s", strings.TrimSpace(a.host), a.port, contract.MetricsPath)
}

func (a *endpointVectorstoreAdapter) BackendName() string {
	return "qdrant"
}

func nextBackoff(cfg vectorstoreConfig, attempt int) time.Duration {
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

func envInt(key string, def int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
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

func wrapVectorstoreLine(raw string) string {
	return string(vectorstoreline.NormalizePayload(raw))
}
