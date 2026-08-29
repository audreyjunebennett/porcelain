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

type BuildInfo struct {
	Version string
	Commit  string
	Date    string
}

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

func PrintHelp() {
	fmt.Printf(`Chimera internal embedding runtime

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
	fs.StringVar(&cfg.Bin, "bin", envOrDefault(naming.EnvEmbedBin, naming.ProductLlamaServerBinName), "llama-server binary path")
	fs.StringVar(&cfg.Backend, "backend", envOrDefault(naming.EnvEmbedBackend, naming.ProductLlamaServerBinName), "backend name")
	fs.StringVar(&cfg.Endpoint, "endpoint", envOrDefault(naming.EnvEmbedEndpoint, naming.DefaultEmbedEndpoint), "llama-server endpoint host:port")
	fs.StringVar(&cfg.ModelPath, "model-path", envOrDefault(naming.EnvEmbedModelPath, naming.DefaultEmbedModelPath), "GGUF embedding model path")
	fs.StringVar(&cfg.CacheDir, "cache-dir", envOrDefault(naming.EnvEmbedCacheDir, naming.DefaultEmbedCacheDir), "llama-server working/cache directory")
	fs.StringVar(&cfg.LogLevel, "log-level", envOrDefault(naming.EnvEmbedLogLevel, naming.DefaultEmbedLogLevel), "backend log level hint")
	fs.IntVar(&cfg.CtxSize, "ctx-size", envInt(naming.EnvEmbedCtxSize, naming.DefaultEmbedCtxSize), "llama-server context size")
	fs.IntVar(&cfg.NGPULayers, "n-gpu-layers", envInt(naming.EnvEmbedNGPULayers, naming.DefaultEmbedNGPULayers), "llama-server GPU layers (0 = CPU)")
	fs.StringVar(&cfg.Pooling, "pooling", envOrDefault(naming.EnvEmbedPooling, naming.DefaultEmbedPooling), "embedding pooling mode")
	fs.DurationVar(&cfg.StartupTimeout, "startup-timeout", envDuration(naming.EnvEmbedTimeoutsStartup, 120*time.Second), "startup readiness timeout")
	fs.DurationVar(&cfg.ShutdownTimeout, "shutdown-timeout", envDuration(naming.EnvEmbedTimeoutsShutdown, contract.DefaultShutdownTimeout), "wrapper graceful shutdown timeout")
	fs.DurationVar(&cfg.TerminateWait, "terminate-wait", contract.DefaultTerminateWait, "wait before force-killing llama-server")
	fs.DurationVar(&cfg.BackoffInitial, "backoff-initial", contract.DefaultBackoffInitial, "restart backoff initial delay")
	fs.Float64Var(&cfg.BackoffMultiplier, "backoff-multiplier", contract.DefaultBackoffMultiplier, "restart backoff multiplier")
	fs.DurationVar(&cfg.BackoffMax, "backoff-max", contract.DefaultBackoffMax, "restart backoff max delay")
	fs.DurationVar(&cfg.BackoffResetAfter, "backoff-reset-after", contract.DefaultBackoffResetAfter, "healthy runtime to reset backoff")
	fs.BoolVar(&cfg.DebugEnableUpstream, "debug-enable-embed-logs", wruntime.EnvBool(contract.DebugEnableEnvKey(contract.ComponentEmbed)), "enable "+contract.DebugEmbedLogsPath)
	fs.BoolVar(&cfg.DebugAllowRemote, "debug-allow-remote", wruntime.EnvBool(contract.DebugAllowRemoteEnv), "allow /debug/* on non-loopback bind")
	fs.BoolVar(&cfg.ForwardUpstreamInDebug, "debug-forward-upstream", false, "forward upstream lines to stderr in debug mode")
	fs.StringVar(&cfg.UpstreamVersion, "upstream-version", "", "optional llama-server version for status")
}

func ParseEndpoint(endpoint string) (string, int, error) {
	host, portText, ok := strings.Cut(strings.TrimSpace(endpoint), ":")
	if !ok || strings.TrimSpace(host) == "" || strings.TrimSpace(portText) == "" {
		return "", 0, fmt.Errorf("invalid endpoint %q, expected host:port", endpoint)
	}
	port, err := strconv.Atoi(strings.TrimSpace(portText))
	if err != nil || port <= 0 || port > 65535 {
		return "", 0, fmt.Errorf("invalid endpoint port in %q", endpoint)
	}
	return strings.TrimSpace(host), port, nil
}

func envOrDefault(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func envDuration(key string, fallback time.Duration) time.Duration {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func envInt(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}
