package supervise

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	svconfig "github.com/lynn/porcelain/chimera/chimera-supervisor/internal/config"
	"github.com/lynn/porcelain/chimera/chimera-supervisor/internal/control"
	"github.com/lynn/porcelain/chimera/chimera-supervisor/internal/supervisorline"
	"github.com/lynn/porcelain/chimera/internal/brokerclient"
	gwconfig "github.com/lynn/porcelain/chimera/internal/config"
	"github.com/lynn/porcelain/chimera/internal/logfmt"
	"github.com/lynn/porcelain/chimera/internal/servicelogs"
	"github.com/lynn/porcelain/chimera/internal/tokens"
)

// Run supervises gateway, broker, vectorstore wrappers, and optional indexer until ctx is canceled.
func Run(ctx context.Context, cfg svconfig.Config, version, commit string) error {
	path := strings.TrimSpace(cfg.ConfigPath)
	if path == "" {
		var err error
		path, err = gwconfig.ResolveGatewayConfigPath()
		if err != nil {
			return svconfig.Exitf(2, "%v", err)
		}
	}

	logStore := servicelogs.New(servicelogs.DefaultMaxLines)
	logLevel := resolveLogLevel(path)
	supSink := LogSink(logStore.Writer(servicelogs.SourceChimeraSupervisor), supervisorline.NewWriter, logLevel)
	log := buildLogger(supSink, logLevel, cfg.LogJSON)
	if cfg.LogJSON {
		_ = os.Setenv(logfmt.EnvLogJSON, "1")
	}
	res, err := gwconfig.LoadGatewayYAML(path, nil)
	if err != nil {
		return svconfig.Exitf(1, "load gateway.yaml: %v", err)
	}

	log.Info("supervisor startup seed", "msg", "chimera-supervisor.startup.seed")
	rootCtx, stopRoot := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stopRoot()

	bootstrap := false
	if strings.TrimSpace(res.TokensPath) != "" {
		bootstrap = tokens.IsBootstrapMode(res.TokensPath)
	}
	vectorstoreWrapperBin := strings.TrimSpace(cfg.VectorstoreBin)
	controlState := control.NewState()
	controlState.SetVersions(version, commit)
	controlState.SetRequired(true, vectorstoreWrapperBin != "")
	controlState.SetEndpoints(strings.TrimSpace(cfg.BrokerEndpoint), strings.TrimSpace(cfg.VectorstoreEndpoint))
	controlState.SetOperatorUI(gatewayPublicURLFromResolved(res), bootstrap)
	controlListen := strings.TrimSpace(cfg.Listen)
	if controlListen == "" {
		controlListen = "127.0.0.1:7710"
	}
	controlBaseURL := fmt.Sprintf("http://%s", controlListen)
	controlSrv := &http.Server{Addr: controlListen, Handler: control.Handler(controlState, logStore, stopRoot)}
	controlLn, controlErr := net.Listen("tcp", controlListen)
	if controlErr != nil {
		return svconfig.Exitf(1, "listen %s: %v", controlListen, controlErr)
	}
	go func() {
		if err := controlSrv.Serve(controlLn); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("supervisor control server exit", "msg", "chimera-supervisor.control.server_error", "listen", controlListen, "err", err)
		}
	}()
	defer func() {
		shCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = controlSrv.Shutdown(shCtx)
	}()

	vectorstoreReadyzURL := ""
	if vectorstoreWrapperBin != "" {
		vectorstoreReadyzURL = fmt.Sprintf("http://%s/readyz", strings.TrimSpace(cfg.VectorstoreListen))
	}
	gatewayReadyzURL := fmt.Sprintf("http://%s/readyz", strings.TrimSpace(cfg.GatewayListen))
	brokerReadyzURL := fmt.Sprintf("http://%s/readyz", strings.TrimSpace(cfg.BrokerListen))

	var (
		gatewayProc     *exec.Cmd
		gatewayWaitErr  chan error
		vectorstoreProc *exec.Cmd
		vectorstoreWait chan error
		brokerProc      *exec.Cmd
		brokerWaitErr   chan error
		indexerProc     *exec.Cmd
		indexerWait     chan error
	)

	indexerCtx, stopIndexer := context.WithCancel(ctx)
	var supervisedShutdownOnce sync.Once
	stopChildrenGraceful := func() {
		supervisedShutdownOnce.Do(func() {
			stopIndexer()
			shutdownGrace := cfg.ShutdownTimeout
			if cfg.TerminateWait > shutdownGrace {
				shutdownGrace = cfg.TerminateWait
			}
			ShutdownChildren(log, shutdownGrace,
				Child{Name: "gateway", Cmd: gatewayProc, WaitCh: gatewayWaitErr},
				Child{Name: "vectorstore", Cmd: vectorstoreProc, WaitCh: vectorstoreWait},
				Child{Name: "broker", Cmd: brokerProc, WaitCh: brokerWaitErr},
				Child{Name: "indexer", Cmd: indexerProc, WaitCh: indexerWait},
			)
			log.Info("supervised shutdown complete", "msg", "chimera-supervisor.shutdown.children_done")
		})
	}
	stopChildrenFast := func() {
		supervisedShutdownOnce.Do(func() {
			stopIndexer()
			KillWrapperFamilies(gatewayProc, brokerProc, vectorstoreProc)
		})
	}

	if !bootstrap {
		// Start vectorstore and broker before the gateway wrapper: the inner gateway
		// /health probe requires upstream broker (and vectorstore when RAG is on), so
		// gateway readiness cannot succeed until those backends are up.
		if vectorstoreWrapperBin != "" {
			if err := startVectorstoreChild(cfg, res, controlBaseURL, logStore, logLevel, log, controlState, vectorstoreWrapperBin, &vectorstoreProc, &vectorstoreWait, vectorstoreReadyzURL, stopChildrenFast); err != nil {
				return err
			}
		}
		if err := startBrokerChild(cfg, res, controlBaseURL, logStore, logLevel, log, controlState, &brokerProc, &brokerWaitErr, brokerReadyzURL, vectorstoreWait, stopChildrenFast); err != nil {
			return err
		}
		if err := startGatewayChild(cfg, path, controlBaseURL, logStore, logLevel, log, controlState, &gatewayProc, &gatewayWaitErr, gatewayReadyzURL, stopChildrenFast); err != nil {
			return err
		}
		startIndexerChild(res, cfg, path, controlBaseURL, logStore, logLevel, log, indexerCtx, &indexerProc, &indexerWait)
	} else {
		// Bootstrap: gateway-only loopback setup surface until api-keys.yaml exists.
		if err := startGatewayChild(cfg, path, controlBaseURL, logStore, logLevel, log, controlState, &gatewayProc, &gatewayWaitErr, gatewayReadyzURL, stopChildrenFast); err != nil {
			return err
		}
	}

	go func() {
		<-rootCtx.Done()
		log.Info("received shutdown signal", "msg", "chimera-supervisor.shutdown.signal_received")
		log.Info("shutting down gracefully", "msg", "chimera-supervisor.shutdown.graceful_start")
		stopChildrenGraceful()
	}()
	brokerclient.RunSupervisedChildHealthMonitor(rootCtx, log, "gateway", gatewayReadyzURL, 15*time.Second, 30*time.Second, !cfg.NoWaitGateway)
	brokerclient.RunSupervisedChildHealthMonitor(rootCtx, log, "broker", brokerReadyzURL, 15*time.Second, 30*time.Second, !cfg.NoWaitBroker)
	if vectorstoreReadyzURL != "" {
		brokerclient.RunSupervisedChildHealthMonitor(rootCtx, log, "vectorstore", vectorstoreReadyzURL, 15*time.Second, 30*time.Second, !cfg.NoWaitVectorstore)
	}
	<-rootCtx.Done()
	stopChildrenGraceful()
	return nil
}
