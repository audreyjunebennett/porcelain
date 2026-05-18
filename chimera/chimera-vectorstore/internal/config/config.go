// Package config parses chimera-vectorstore CLI flags and environment defaults.
package config

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/lynn/porcelain/chimera/internal/wrapper/contract"
	wruntime "github.com/lynn/porcelain/chimera/internal/wrapper/runtime"
	"github.com/lynn/porcelain/internal/naming"
)

// BuildInfo is injected at link time for -version output.
type BuildInfo struct {
	Version string
	Commit  string
	Date    string
}

// Config is the resolved runtime configuration for chimera-vectorstore.
type Config struct {
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

// PrintHelp writes usage to stdout.
func PrintHelp() {
	fmt.Printf(`Chimera vectorstore runtime

Usage:
  %s [flags]
  %s -version

Flags:
`, naming.ProductVectorstoreName, naming.ProductVectorstoreName)
	fs := flag.NewFlagSet(naming.ProductVectorstoreName, flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	bindFlags(fs, &Config{})
	fs.PrintDefaults()
}

// Parse reads flags from args. Returns io.EOF when -version was requested (already printed).
func Parse(args []string, build BuildInfo) (Config, error) {
	fs := flag.NewFlagSet(naming.ProductVectorstoreName, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	cfg := Config{}
	var showVersion bool
	bindFlags(fs, &cfg)
	fs.BoolVar(&showVersion, "version", false, "print version")
	fs.BoolVar(&showVersion, "v", false, "print version")
	if err := fs.Parse(args); err != nil {
		return cfg, err
	}
	if showVersion {
		fmt.Printf("%s %s\ncommit %s\nbuild date %s\n", naming.ProductVectorstoreName, build.Version, build.Commit, build.Date)
		return cfg, io.EOF
	}
	if strings.TrimSpace(strings.ToLower(cfg.Backend)) != naming.ProductQdrantBinName {
		return cfg, fmt.Errorf("%s must be %s for binary mode", naming.EnvVectorstoreBackend, naming.ProductQdrantBinName)
	}
	return cfg, nil
}

func bindFlags(fs *flag.FlagSet, cfg *Config) {
	fs.StringVar(&cfg.Listen, "listen", envOrDefault(naming.EnvVectorstoreListen, naming.DefaultVectorstoreListen), "wrapper listen addr (host:port)")
	fs.StringVar(&cfg.Bin, "bin", envOrDefault(naming.EnvVectorstoreBin, naming.ProductQdrantBinName), "backend binary path")
	fs.StringVar(&cfg.Backend, "backend", envOrDefault(naming.EnvVectorstoreBackend, naming.ProductQdrantBinName), "backend name")
	fs.StringVar(&cfg.Endpoint, "endpoint", envOrDefault(naming.EnvVectorstoreEndpoint, naming.DefaultVectorstoreEndpoint), "backend endpoint host:port")
	fs.StringVar(&cfg.DataPath, "data-path", envOrDefault(naming.EnvVectorstoreDataPath, naming.DefaultVectorstoreDataPath), "backend data path")
	fs.StringVar(&cfg.LogLevel, "log-level", envOrDefault(naming.EnvVectorstoreLogLevel, naming.DefaultVectorstoreLogLevel), "backend log level")
	fs.DurationVar(&cfg.StartupTimeout, "startup-timeout", envDuration(naming.EnvVectorstoreTimeoutsStartup, contract.DefaultStartupTimeout), "startup readiness timeout")
	fs.DurationVar(&cfg.ShutdownTimeout, "shutdown-timeout", envDuration(naming.EnvVectorstoreTimeoutsShutdown, contract.DefaultShutdownTimeout), "wrapper graceful shutdown timeout")
	fs.DurationVar(&cfg.TerminateWait, "terminate-wait", contract.DefaultTerminateWait, "wait before force-kill backend")
	fs.DurationVar(&cfg.BackoffInitial, "backoff-initial", contract.DefaultBackoffInitial, "restart backoff initial delay")
	fs.Float64Var(&cfg.BackoffMultiplier, "backoff-multiplier", contract.DefaultBackoffMultiplier, "restart backoff multiplier")
	fs.DurationVar(&cfg.BackoffMax, "backoff-max", contract.DefaultBackoffMax, "restart backoff max delay")
	fs.DurationVar(&cfg.BackoffResetAfter, "backoff-reset-after", contract.DefaultBackoffResetAfter, "healthy runtime to reset backoff")
	fs.BoolVar(&cfg.DebugEnableUpstream, "debug-enable-vectorstore-logs", wruntime.EnvBool(contract.DebugEnableEnvKey(contract.ComponentVectorstore)), "enable "+contract.DebugVectorstoreLogsPath)
	fs.BoolVar(&cfg.DebugAllowRemote, "debug-allow-remote", wruntime.EnvBool(contract.DebugAllowRemoteEnv), "allow /debug/* on non-loopback bind")
	fs.BoolVar(&cfg.ForwardUpstreamInDebug, "debug-forward-upstream", false, "forward upstream lines to stderr in debug mode")
	fs.StringVar(&cfg.UpstreamVersion, "upstream-version", "", "optional upstream version for status payload")
	fs.IntVar(&cfg.GRPCPort, "grpc-port", envInt(naming.EnvVectorstoreGRPCPort, naming.DefaultVectorstoreGRPCPort), "qdrant grpc port")
}

// ParseEndpoint splits host:port for the Qdrant HTTP endpoint.
func ParseEndpoint(endpoint string) (host string, port int, err error) {
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
