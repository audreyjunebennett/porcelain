// Package config parses chimera-broker CLI flags and environment defaults.
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

// Config is the resolved runtime configuration for chimera-broker.
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
	ChimeraBrokerConfig    string
	ChimeraBrokerLogStyle  string
}

// PrintHelp writes usage to stdout.
func PrintHelp() {
	fmt.Printf(`Chimera broker runtime

Usage:
  %s [flags]
  %s -version

Flags:
`, naming.ProductBrokerName, naming.ProductBrokerName)
	fs := flag.NewFlagSet(naming.ProductBrokerName, flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	bindFlags(fs, &Config{})
	fs.PrintDefaults()
}

// Parse reads flags from args. Returns io.EOF when -version was requested (already printed).
func Parse(args []string, build BuildInfo) (Config, error) {
	fs := flag.NewFlagSet(naming.ProductBrokerName, flag.ContinueOnError)
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
		fmt.Printf("%s %s\ncommit %s\nbuild date %s\n", naming.ProductBrokerName, build.Version, build.Commit, build.Date)
		return cfg, io.EOF
	}
	if strings.TrimSpace(strings.ToLower(cfg.Backend)) != naming.ProductBrokerName {
		return cfg, fmt.Errorf("%s must be %s for binary mode", naming.EnvBrokerBackend, naming.ProductBrokerName)
	}
	return cfg, nil
}

func bindFlags(fs *flag.FlagSet, cfg *Config) {
	fs.StringVar(&cfg.Listen, "listen", envOrDefault(naming.EnvBrokerListen, naming.DefaultBrokerListen), "wrapper listen addr (host:port)")
	fs.StringVar(&cfg.Bin, "bin", envOrDefault(naming.EnvBrokerBin, naming.ProductBrokerHTTPBinName), "backend binary path")
	fs.StringVar(&cfg.Backend, "backend", envOrDefault(naming.EnvBrokerBackend, naming.ProductBrokerName), "backend name")
	fs.StringVar(&cfg.Endpoint, "endpoint", envOrDefault(naming.EnvBrokerEndpoint, naming.DefaultBrokerEndpoint), "backend endpoint host:port")
	fs.StringVar(&cfg.DataPath, "data-path", envOrDefault(naming.EnvBrokerDataPath, naming.DefaultBrokerDataPath), "backend data path")
	fs.StringVar(&cfg.LogLevel, "log-level", envOrDefault(naming.EnvBrokerLogLevel, naming.DefaultBrokerLogLevel), "backend log level")
	fs.DurationVar(&cfg.StartupTimeout, "startup-timeout", envDuration(naming.EnvBrokerTimeoutsStartup, contract.DefaultStartupTimeout), "startup readiness timeout")
	fs.DurationVar(&cfg.ShutdownTimeout, "shutdown-timeout", envDuration(naming.EnvBrokerTimeoutsShutdown, contract.DefaultShutdownTimeout), "wrapper graceful shutdown timeout")
	fs.DurationVar(&cfg.TerminateWait, "terminate-wait", contract.DefaultTerminateWait, "wait before force-kill backend")
	fs.DurationVar(&cfg.BackoffInitial, "backoff-initial", contract.DefaultBackoffInitial, "restart backoff initial delay")
	fs.Float64Var(&cfg.BackoffMultiplier, "backoff-multiplier", contract.DefaultBackoffMultiplier, "restart backoff multiplier")
	fs.DurationVar(&cfg.BackoffMax, "backoff-max", contract.DefaultBackoffMax, "restart backoff max delay")
	fs.DurationVar(&cfg.BackoffResetAfter, "backoff-reset-after", contract.DefaultBackoffResetAfter, "healthy runtime to reset backoff")
	fs.BoolVar(&cfg.DebugEnableUpstream, "debug-enable-upstream-logs", wruntime.EnvBool(contract.DebugEnableEnvKey), "enable /debug/upstream/logs")
	fs.BoolVar(&cfg.DebugAllowRemote, "debug-allow-remote", wruntime.EnvBool(contract.DebugAllowRemoteEnv), "allow /debug/* on non-loopback bind")
	fs.BoolVar(&cfg.ForwardUpstreamInDebug, "debug-forward-upstream", false, "forward upstream lines to stderr in debug mode")
	fs.StringVar(&cfg.UpstreamVersion, "upstream-version", "", "optional upstream version for status payload")
	fs.StringVar(&cfg.ChimeraBrokerConfig, "chimera-broker-config", envOrDefault(naming.EnvBrokerChimeraBrokerConfig, naming.DefaultBrokerConfigPath), "chimera-broker config JSON path")
	fs.StringVar(&cfg.ChimeraBrokerLogStyle, "chimera-broker-log-style", envOrDefault(naming.EnvBrokerChimeraBrokerLogStyle, naming.DefaultBrokerLogStyle), "chimera-broker log style")
}

// ParseEndpoint splits host:port for the chimera-broker HTTP backend.
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
