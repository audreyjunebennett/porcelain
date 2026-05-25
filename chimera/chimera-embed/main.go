package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/joho/godotenv"
	"github.com/lynn/porcelain/chimera/chimera-embed/adapter"
	"github.com/lynn/porcelain/chimera/chimera-embed/internal/config"
	"github.com/lynn/porcelain/chimera/internal/logfmt"
	"github.com/lynn/porcelain/chimera/internal/wrapper/contract"
	wruntime "github.com/lynn/porcelain/chimera/internal/wrapper/runtime"
	"github.com/lynn/porcelain/internal/naming"
)

func main() {
	_ = godotenv.Load("env")
	_ = godotenv.Load(".env")
	for _, a := range os.Args[1:] {
		if a == "-h" || a == "--help" {
			config.PrintHelp()
			return
		}
	}
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", naming.ProductEmbedName, err)
		os.Exit(exitCodeForError(err))
	}
}

func exitCodeForError(err error) int {
	var ee *wruntime.ExitError
	if errors.As(err, &ee) {
		return ee.Code
	}
	return contract.ExitInternal
}

func run(args []string) error {
	cfg, err := config.Parse(args, config.BuildInfo{
		Version: version,
		Commit:  commit,
		Date:    date,
	})
	if err != nil {
		if errors.Is(err, io.EOF) {
			return nil
		}
		return wruntime.WrapExitError(contract.ExitConfigError, err)
	}
	host, port, err := config.ParseEndpoint(cfg.Endpoint)
	if err != nil {
		return wruntime.WrapExitError(contract.ExitConfigError, err)
	}

	log := logfmt.NewLogger(os.Stderr, logfmt.JSONEnabled(), slog.LevelInfo)
	embedAdapter := &adapter.LlamaServer{Cfg: cfg, Host: host, Port: port}

	rootCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return wruntime.Run(rootCtx, wruntime.Config{
		Component:              contract.ComponentEmbed,
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
		ReadyMessage:           "embed.ready",
		UpstreamLineMessage:    "embed.upstream.line",
		HTTPServerErrorMessage: "embed.http.server_error",
		UpstreamLineWrapper:    adapter.WrapUpstreamLine,
	}, embedAdapter, log)
}
