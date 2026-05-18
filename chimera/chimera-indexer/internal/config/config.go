// Package config parses chimera-indexer wrapper CLI flags and environment defaults.
package config

import (
	"flag"
	"fmt"
	"io"
	"os"
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

// Config is the resolved runtime configuration for the chimera-indexer wrapper.
type Config struct {
	Listen                 string
	Bin                    string
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
	BackendArgs            []string
}

// PrintHelp writes wrapper-mode usage to stdout.
func PrintHelp() {
	fmt.Printf(`Chimera indexer runtime

Usage:
  %s [flags] [backend flags]
  %s -version

Backend flags are passed through to the embedded backend mode (for example: --one-shot, --config, --gateway-url).

Flags:
`, naming.ProductIndexerBinName, naming.ProductIndexerBinName)
	fs := flag.NewFlagSet(naming.ProductIndexerBinName, flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	bindFlags(fs, &Config{})
	fs.PrintDefaults()
}

// Parse reads wrapper flags from args. Returns io.EOF when -version was requested (already printed).
func Parse(args []string, build BuildInfo) (Config, error) {
	fs := flag.NewFlagSet(naming.ProductIndexerBinName, flag.ContinueOnError)
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
		fmt.Printf("%s %s\ncommit %s\nbuild date %s\n", naming.ProductIndexerBinName, build.Version, build.Commit, build.Date)
		return cfg, io.EOF
	}
	cfg.BackendArgs = fs.Args()
	return cfg, nil
}

func bindFlags(fs *flag.FlagSet, cfg *Config) {
	fs.StringVar(&cfg.Listen, "listen", envOrDefault("INDEXER__LISTEN", "127.0.0.1:7750"), "wrapper listen addr (host:port)")
	fs.StringVar(&cfg.Bin, "bin", envOrDefault("INDEXER__BIN", ""), "indexer backend binary path")
	fs.DurationVar(&cfg.StartupTimeout, "startup-timeout", envDuration("INDEXER__TIMEOUTS__STARTUP", contract.DefaultStartupTimeout), "startup readiness timeout")
	fs.DurationVar(&cfg.ShutdownTimeout, "shutdown-timeout", envDuration("INDEXER__TIMEOUTS__SHUTDOWN", contract.DefaultShutdownTimeout), "wrapper graceful shutdown timeout")
	fs.DurationVar(&cfg.TerminateWait, "terminate-wait", contract.DefaultTerminateWait, "wait before force-kill backend")
	fs.DurationVar(&cfg.BackoffInitial, "backoff-initial", contract.DefaultBackoffInitial, "restart backoff initial delay")
	fs.Float64Var(&cfg.BackoffMultiplier, "backoff-multiplier", contract.DefaultBackoffMultiplier, "restart backoff multiplier")
	fs.DurationVar(&cfg.BackoffMax, "backoff-max", contract.DefaultBackoffMax, "restart backoff max delay")
	fs.DurationVar(&cfg.BackoffResetAfter, "backoff-reset-after", contract.DefaultBackoffResetAfter, "healthy runtime to reset backoff")
	fs.BoolVar(&cfg.DebugEnableUpstream, "debug-enable-upstream-logs", wruntime.EnvBool(contract.DebugEnableEnvKey), "enable /debug/upstream/logs")
	fs.BoolVar(&cfg.DebugAllowRemote, "debug-allow-remote", wruntime.EnvBool(contract.DebugAllowRemoteEnv), "allow /debug/* on non-loopback bind")
	fs.BoolVar(&cfg.ForwardUpstreamInDebug, "debug-forward-upstream", false, "forward upstream lines to stderr in debug mode")
	fs.StringVar(&cfg.UpstreamVersion, "upstream-version", "", "optional backend version for status payload")
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
