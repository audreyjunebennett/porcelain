// Package config parses chimera-embed CLI flags and environment defaults.
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

// Config is the resolved runtime configuration for chimera-embed.
type Config struct {
	Listen                 string
	Bin                    string
	Backend                string
	Endpoint               string
	ModelPath              string
	CacheDir               string
	LogLevel               string
	CtxSize                int
	NGPULayers             int
	Pooling                string
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

// PrintHelp writes usage to stdout.
func PrintHelp() {
	fmt.Printf(`Chimera embed runtime

Usage:
  %s [flags]
  %s -version

Flags:
`, naming.ProductEmbedName, naming.ProductEmbedName)
	fs := flag.NewFlagSet(naming.ProductEmbedName, flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	bindFlags(fs, &Config{})
	fs.PrintDefaults()
}

// Parse reads flags from args. Returns io.EOF when -version was requested (already printed).
func Parse(args []string, build BuildInfo) (Config, error) {
	fs := flag.NewFlagSet(naming.ProductEmbedName, flag.ContinueOnError)
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
		fmt.Printf("%s %s\ncommit %s\nbuild date %s\n", naming.ProductEmbedName, build.Version, build.Commit, build.Date)
		return cfg, io.EOF
	}
	if strings.TrimSpace(strings.ToLower(cfg.Backend)) != naming.ProductLlamaServerBinName {
		return cfg, fmt.Errorf("%s must be %s for binary mode", naming.EnvEmbedBackend, naming.ProductLlamaServerBinName)
	}
	if strings.TrimSpace(cfg.ModelPath) == "" {
		return cfg, fmt.Errorf("model path is required (-model-path or %s)", naming.EnvEmbedModelPath)
	}
	return cfg, nil
}

func bindFlags(fs *flag.FlagSet, cfg *Config) {
	fs.StringVar(&cfg.Listen, "listen", envOrDefault(naming.EnvEmbedListen, naming.DefaultEmbedListen), "wrapper listen addr (host:port)")
	fs.StringVar(&cfg.Bin, "bin", envOrDefault(naming.EnvEmbedBin, naming.ProductLlamaServerBinName), "backend binary path")
	fs.StringVar(&cfg.Backend, "backend", envOrDefault(naming.EnvEmbedBackend, naming.ProductLlamaServerBinName), "backend name")
	fs.StringVar(&cfg.Endpoint, "endpoint", envOrDefault(naming.EnvEmbedEndpoint, naming.DefaultEmbedEndpoint), "backend endpoint host:port")
	fs.StringVar(&cfg.ModelPath, "model-path", envOrDefault(naming.EnvEmbedModelPath, naming.DefaultEmbedModelPath), "GGUF model path for llama-server")
	fs.StringVar(&cfg.CacheDir, "cache-dir", envOrDefault(naming.EnvEmbedCacheDir, naming.DefaultEmbedCacheDir), "optional model cache directory")
	fs.StringVar(&cfg.LogLevel, "log-level", envOrDefault(naming.EnvEmbedLogLevel, naming.DefaultEmbedLogLevel), "backend log level hint")
	fs.IntVar(&cfg.CtxSize, "ctx-size", envInt(naming.EnvEmbedCtxSize, naming.DefaultEmbedCtxSize), "llama-server context size")
	fs.IntVar(&cfg.NGPULayers, "n-gpu-layers", envInt(naming.EnvEmbedNGPULayers, naming.DefaultEmbedNGPULayers), "llama-server GPU layers (0 = CPU)")
	fs.StringVar(&cfg.Pooling, "pooling", envOrDefault(naming.EnvEmbedPooling, naming.DefaultEmbedPooling), "llama-server pooling mode")
	fs.DurationVar(&cfg.StartupTimeout, "startup-timeout", envDuration(naming.EnvEmbedTimeoutsStartup, 120*time.Second), "startup readiness timeout")
	fs.DurationVar(&cfg.ShutdownTimeout, "shutdown-timeout", envDuration(naming.EnvEmbedTimeoutsShutdown, contract.DefaultShutdownTimeout), "wrapper graceful shutdown timeout")
	fs.DurationVar(&cfg.TerminateWait, "terminate-wait", contract.DefaultTerminateWait, "wait before force-kill backend")
	fs.DurationVar(&cfg.BackoffInitial, "backoff-initial", contract.DefaultBackoffInitial, "restart backoff initial delay")
	fs.Float64Var(&cfg.BackoffMultiplier, "backoff-multiplier", contract.DefaultBackoffMultiplier, "restart backoff multiplier")
	fs.DurationVar(&cfg.BackoffMax, "backoff-max", contract.DefaultBackoffMax, "restart backoff max delay")
	fs.DurationVar(&cfg.BackoffResetAfter, "backoff-reset-after", contract.DefaultBackoffResetAfter, "healthy runtime to reset backoff")
	fs.BoolVar(&cfg.DebugEnableUpstream, "debug-enable-embed-logs", wruntime.EnvBool(contract.DebugEnableEnvKey(contract.ComponentEmbed)), "enable "+contract.DebugEmbedLogsPath)
	fs.BoolVar(&cfg.DebugAllowRemote, "debug-allow-remote", wruntime.EnvBool(contract.DebugAllowRemoteEnv), "allow /debug/* on non-loopback bind")
	fs.BoolVar(&cfg.ForwardUpstreamInDebug, "debug-forward-upstream", false, "forward upstream lines to stderr in debug mode")
	fs.StringVar(&cfg.UpstreamVersion, "upstream-version", "", "optional upstream version for status payload")
}

// ParseEndpoint splits host:port for the llama-server HTTP endpoint.
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
