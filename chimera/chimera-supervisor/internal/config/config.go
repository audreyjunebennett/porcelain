// Package config parses chimera-supervisor CLI flags and environment defaults.
package config

import (
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/lynn/porcelain/chimera/internal/wrapper/contract"
	"github.com/lynn/porcelain/internal/naming"
)

// BuildInfo is injected at link time for -version output.
type BuildInfo struct {
	Version string
	Commit  string
	Date    string
}

// Config is the resolved runtime configuration for chimera-supervisor.
type Config struct {
	ConfigPath          string
	Listen              string
	GatewayBin          string
	GatewayListen       string
	WaitGateway         time.Duration
	NoWaitGateway       bool
	BrokerBin           string
	BrokerListen        string
	BrokerEndpoint      string
	BrokerDataDir       string
	WaitBroker          time.Duration
	NoWaitBroker        bool
	VectorstoreBin      string
	VectorstoreListen   string
	VectorstoreEndpoint string
	VectorstoreDataPath string
	WaitVectorstore     time.Duration
	NoWaitVectorstore   bool
	LogJSON             bool
	ShutdownTimeout     time.Duration
	TerminateWait       time.Duration
}

// PrintHelp writes usage to stdout.
func PrintHelp() {
	fmt.Printf(`Chimera supervisor runtime

Usage:
  %s [flags]
  %s -version

This binary supervises gateway + wrapper processes + optional indexer in headless mode.

Flags:
`, naming.ProductSupervisorName, naming.ProductSupervisorName)
	fs := flag.NewFlagSet(naming.ProductSupervisorName, flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	bindFlags(fs, &Config{})
	fs.PrintDefaults()
}

// Parse reads flags from args. Returns io.EOF when -version was requested (already printed).
func Parse(args []string, build BuildInfo) (Config, error) {
	fs := flag.NewFlagSet(naming.ProductSupervisorName, flag.ContinueOnError)
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
		fmt.Printf("%s %s\ncommit %s\nbuild date %s\n", naming.ProductSupervisorName, build.Version, build.Commit, build.Date)
		return cfg, io.EOF
	}
	return cfg, nil
}

func bindFlags(fs *flag.FlagSet, cfg *Config) {
	fs.StringVar(&cfg.ConfigPath, "config", "", "Path to gateway.yaml")
	fs.StringVar(&cfg.Listen, "listen", defaultSupervisorListen(), "chimera-supervisor control API listen host:port")
	fs.StringVar(&cfg.GatewayBin, "gateway-bin", DefaultGatewayBin(), "chimera-gateway wrapper binary")
	fs.StringVar(&cfg.GatewayListen, "gateway-listen", naming.DefaultGatewayListen, "chimera-gateway wrapper listen host:port")
	fs.DurationVar(&cfg.WaitGateway, "wait-gateway", 60*time.Second, "Max time to poll chimera-gateway /readyz before exit")
	fs.BoolVar(&cfg.NoWaitGateway, "no-wait-gateway", false, "Skip chimera-gateway readiness poll")
	fs.StringVar(&cfg.BrokerBin, "broker-bin", DefaultBrokerBin(), "chimera-broker wrapper binary")
	fs.StringVar(&cfg.BrokerListen, "broker-listen", naming.DefaultBrokerListen, "chimera-broker wrapper listen host:port")
	fs.StringVar(&cfg.BrokerEndpoint, "broker-endpoint", naming.DefaultBrokerEndpoint, "broker backend endpoint host:port for chimera-broker --endpoint")
	fs.StringVar(&cfg.BrokerDataDir, "broker-data-dir", "data/broker", "broker data path for chimera-broker --data-path")
	fs.DurationVar(&cfg.WaitBroker, "wait-broker", 60*time.Second, "Max time to poll chimera-broker /readyz before exit")
	fs.BoolVar(&cfg.NoWaitBroker, "no-wait-broker", false, "Skip chimera-broker readiness poll")
	fs.StringVar(&cfg.VectorstoreBin, "vectorstore-bin", DefaultVectorstoreBin(), "chimera-vectorstore wrapper binary")
	fs.StringVar(&cfg.VectorstoreListen, "vectorstore-listen", naming.DefaultVectorstoreListen, "chimera-vectorstore wrapper listen host:port")
	fs.StringVar(&cfg.VectorstoreEndpoint, "vectorstore-endpoint", naming.DefaultVectorstoreEndpoint, "vectorstore backend endpoint host:port for chimera-vectorstore --endpoint")
	fs.StringVar(&cfg.VectorstoreDataPath, "vectorstore-data-path", "data/vectorstore", "vectorstore data path for chimera-vectorstore --data-path")
	fs.DurationVar(&cfg.WaitVectorstore, "wait-vectorstore", 60*time.Second, "Max time to poll chimera-vectorstore /readyz before exit")
	fs.BoolVar(&cfg.NoWaitVectorstore, "no-wait-vectorstore", false, "Skip chimera-vectorstore readiness poll")
	fs.BoolVar(&cfg.LogJSON, "log-json", true, "Emit JSON logs (supervisor, wrappers, supervised indexer)")
	fs.DurationVar(&cfg.ShutdownTimeout, "shutdown-timeout", contract.DefaultShutdownTimeout, "Max wait per supervised child during graceful shutdown")
	fs.DurationVar(&cfg.TerminateWait, "terminate-wait", contract.DefaultTerminateWait, "Grace period after signal before force-killing a wrapper/backend")
}

func defaultSupervisorListen() string {
	return "127.0.0.1:7710"
}
