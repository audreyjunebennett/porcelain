package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/lynn/porcelain/chimera/chimera-broker/brokerline"
	"github.com/lynn/porcelain/chimera/chimera-gateway/gatewayline"
	"github.com/lynn/porcelain/chimera/chimera-indexer/indexer"
	"github.com/lynn/porcelain/chimera/chimera-indexer/indexerline"
	"github.com/lynn/porcelain/chimera/chimera-supervisor/supervisorline"
	"github.com/lynn/porcelain/chimera/chimera-vectorstore/vectorstoreline"
	"github.com/lynn/porcelain/chimera/internal/config"
	"github.com/lynn/porcelain/chimera/internal/logfmt"
	"github.com/lynn/porcelain/chimera/internal/naming"
	"github.com/lynn/porcelain/chimera/internal/servicelogs"
	"github.com/lynn/porcelain/chimera/internal/tokens"
	"github.com/lynn/porcelain/chimera/internal/upstream"
	wruntime "github.com/lynn/porcelain/chimera/internal/wrapper/runtime"
)

func main() {
	_ = godotenv.Load("env")
	_ = godotenv.Load(".env")

	args := os.Args[1:]
	if len(args) > 0 && (args[0] == "-version" || args[0] == "--version") {
		fmt.Printf("chimera-supervisor %s\ncommit %s\nbuild date %s\n", version, commit, date)
		return
	}
	for _, a := range args {
		if a == "-h" || a == "--help" {
			printHelp()
			return
		}
	}
	runSupervisor(args)
}

func printHelp() {
	fmt.Printf(`Chimera supervisor runtime

Usage:
  chimera-supervisor [flags]
  chimera-supervisor -version

This binary supervises gateway + wrapper processes + optional indexer in headless mode.

Flags:
`)
	fs := flag.NewFlagSet("chimera-supervisor", flag.ContinueOnError)
	fs.SetOutput(os.Stdout)
	_ = fs.String("config", "", "Path to gateway.yaml")
	_ = fs.String("listen", "127.0.0.1:7710", "chimera-supervisor control API listen host:port")
	_ = fs.String("gateway-bin", defaultSupervisorGatewayBin(), "chimera-gateway wrapper binary")
	_ = fs.String("gateway-listen", "127.0.0.1:7720", "chimera-gateway wrapper listen host:port")
	_ = fs.Duration("wait-gateway", 60*time.Second, "Max time to poll chimera-gateway /readyz before exit")
	_ = fs.Bool("no-wait-gateway", false, "Skip chimera-gateway readiness poll")
	_ = fs.String("broker-bin", defaultSupervisorBrokerBin(), "chimera-broker wrapper binary")
	_ = fs.String("broker-listen", "127.0.0.1:7730", "chimera-broker wrapper listen host:port")
	_ = fs.String("broker-endpoint", "127.0.0.1:8080", "broker backend endpoint host:port for chimera-broker --endpoint")
	_ = fs.String("broker-data-dir", "data/broker", "broker data path for chimera-broker --data-path")
	_ = fs.Duration("wait-broker", 60*time.Second, "Max time to poll chimera-broker /readyz before exit")
	_ = fs.Bool("no-wait-broker", false, "Skip chimera-broker readiness poll")
	_ = fs.String("vectorstore-bin", defaultSupervisorVectorstoreBin(), "chimera-vectorstore wrapper binary")
	_ = fs.String("vectorstore-listen", "127.0.0.1:7740", "chimera-vectorstore wrapper listen host:port")
	_ = fs.String("vectorstore-endpoint", "127.0.0.1:6333", "vectorstore backend endpoint host:port for chimera-vectorstore --endpoint")
	_ = fs.String("vectorstore-data-path", "data/vectorstore", "vectorstore data path for chimera-vectorstore --data-path")
	_ = fs.Duration("wait-vectorstore", 60*time.Second, "Max time to poll chimera-vectorstore /readyz before exit")
	_ = fs.Bool("no-wait-vectorstore", false, "Skip chimera-vectorstore readiness poll")
	fs.PrintDefaults()
}

func gatewayPublicURLFromResolved(res *config.Resolved) string {
	if res == nil {
		return "http://127.0.0.1:7710"
	}
	host := strings.TrimSpace(res.ListenHost)
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	if strings.Contains(host, ":") && !strings.HasPrefix(host, "[") {
		return fmt.Sprintf("http://[%s]:%d", host, res.ListenPort)
	}
	return fmt.Sprintf("http://%s:%d", host, res.ListenPort)
}

func waitForChildExit(name string, cmd *exec.Cmd, waitCh <-chan error, timeout time.Duration, log *slog.Logger) {
	if waitCh == nil {
		return
	}
	select {
	case werr := <-waitCh:
		if werr != nil && log != nil {
			log.Debug(name+" process finished", "msg", "chimera-supervisor.child.exited", "child", name, "err", werr)
		}
		return
	default:
	}
	select {
	case werr := <-waitCh:
		if werr != nil && log != nil {
			log.Debug(name+" process finished", "msg", "chimera-supervisor.child.exited", "child", name, "err", werr)
		}
		return
	case <-time.After(timeout):
	}

	if cmd != nil && cmd.Process != nil {
		if log != nil {
			log.Warn(name+" did not exit within shutdown window; forcing kill",
				"msg", "chimera-supervisor.shutdown.child_force_kill", "child", name, "pid", cmd.Process.Pid, "timeout", timeout)
		}
		if err := wruntime.TerminateThenKill(cmd, 5*time.Second); err != nil && log != nil {
			log.Warn(name+" graceful stop failed", "msg", "chimera-supervisor.shutdown.child_force_kill", "child", name, "err", err)
		}
	}

	select {
	case werr := <-waitCh:
		if werr != nil && log != nil {
			log.Debug(name+" process finished after kill", "msg", "chimera-supervisor.child.exited", "child", name, "err", werr)
		}
	case <-time.After(5 * time.Second):
		if log != nil {
			log.Warn(name+" still has not exited after forced kill", "msg", "chimera-supervisor.shutdown.child_stuck", "child", name)
		}
		if cmd == nil || cmd.Process == nil {
			return
		}
		if err := forceKillProcessTree(cmd.Process.Pid); err != nil {
			if log != nil {
				log.Warn(name+" process-tree kill fallback failed",
					"msg", "chimera-supervisor.shutdown.child_tree_kill_failed",
					"child", name,
					"pid", cmd.Process.Pid,
					"err", err)
			}
			return
		}
		if log != nil {
			log.Warn(name+" process-tree kill fallback executed",
				"msg", "chimera-supervisor.shutdown.child_tree_kill",
				"child", name,
				"pid", cmd.Process.Pid)
		}
		select {
		case werr := <-waitCh:
			if werr != nil && log != nil {
				log.Debug(name+" process finished after process-tree kill", "msg", "chimera-supervisor.child.exited", "child", name, "err", werr)
			}
		case <-time.After(3 * time.Second):
			if log != nil {
				log.Warn(name+" still has not exited after process-tree kill", "msg", "chimera-supervisor.shutdown.child_stuck", "child", name, "pid", cmd.Process.Pid)
			}
		}
	}
}

func resolveIndexerGatewayToken(tokensPath string) string {
	if v := strings.TrimSpace(os.Getenv(naming.EnvGatewayTokenTarget)); v != "" {
		return v
	}
	metas, err := tokens.ListTokenMeta(strings.TrimSpace(tokensPath))
	if err != nil || len(metas) == 0 {
		return ""
	}
	return strings.TrimSpace(metas[0].Token)
}

func runSupervisor(args []string) {
	fs := flag.NewFlagSet("chimera-supervisor", flag.ExitOnError)
	configPath := fs.String("config", "", "Path to gateway.yaml")
	listen := fs.String("listen", "127.0.0.1:7710", "chimera-supervisor control API listen host:port")
	gatewayBin := fs.String("gateway-bin", defaultSupervisorGatewayBin(), "chimera-gateway wrapper binary")
	gatewayListen := fs.String("gateway-listen", "127.0.0.1:7720", "chimera-gateway wrapper listen host:port")
	waitGateway := fs.Duration("wait-gateway", 60*time.Second, "Max time to poll chimera-gateway /readyz before exit")
	noWaitGateway := fs.Bool("no-wait-gateway", false, "Skip chimera-gateway readiness poll")
	brokerBin := fs.String("broker-bin", defaultSupervisorBrokerBin(), "chimera-broker wrapper binary")
	brokerListen := fs.String("broker-listen", "127.0.0.1:7730", "chimera-broker wrapper listen host:port")
	brokerEndpoint := fs.String("broker-endpoint", "127.0.0.1:8080", "broker backend endpoint host:port for chimera-broker --endpoint")
	brokerDataDir := fs.String("broker-data-dir", "data/broker", "broker data path for chimera-broker --data-path")
	waitTimeout := fs.Duration("wait-broker", 60*time.Second, "Max time to poll chimera-broker /readyz before exit")
	noWait := fs.Bool("no-wait-broker", false, "Skip chimera-broker readiness poll")
	vectorstoreBin := fs.String("vectorstore-bin", defaultSupervisorVectorstoreBin(), "chimera-vectorstore wrapper binary")
	vectorstoreListen := fs.String("vectorstore-listen", "127.0.0.1:7740", "chimera-vectorstore wrapper listen host:port")
	vectorstoreEndpoint := fs.String("vectorstore-endpoint", "127.0.0.1:6333", "vectorstore backend endpoint host:port for chimera-vectorstore --endpoint")
	vectorstoreDataPath := fs.String("vectorstore-data-path", "data/vectorstore", "vectorstore data path for chimera-vectorstore --data-path")
	waitVectorstore := fs.Duration("wait-vectorstore", 60*time.Second, "Max time to poll chimera-vectorstore /readyz before exit")
	noWaitVectorstore := fs.Bool("no-wait-vectorstore", false, "Skip chimera-vectorstore readiness poll")
	logJSON := fs.Bool("log-json", true, "Emit JSON logs (supervisor, wrappers, supervised indexer)")
	shutdownTimeout := fs.Duration("shutdown-timeout", 15*time.Second, "Max wait per supervised child during graceful shutdown")
	terminateWait := fs.Duration("terminate-wait", 10*time.Second, "Grace period after signal before force-killing a wrapper/backend")
	_ = fs.Parse(args)

	path := strings.TrimSpace(*configPath)
	if path == "" {
		var err error
		path, err = config.ResolveGatewayConfigPath()
		if err != nil {
			fmt.Fprintln(os.Stderr, "chimera-supervisor:", err)
			os.Exit(2)
		}
	}

	logStore := servicelogs.New(servicelogs.DefaultMaxLines)
	supSink := supervisedLogSink(logStore.Writer(servicelogs.SourceChimeraSupervisor), supervisorline.NewWriter)
	log := buildLoggerTo(supSink, path, *logJSON)
	if *logJSON {
		_ = os.Setenv(logfmt.EnvLogJSON, "1")
	}
	res, err := config.LoadGatewayYAML(path, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "chimera-supervisor: load gateway.yaml: %v\n", err)
		os.Exit(1)
	}

	log.Info("supervisor startup seed", "msg", "chimera-supervisor.startup.seed")
	bootstrap := false
	if strings.TrimSpace(res.TokensPath) != "" {
		bootstrap = tokens.IsBootstrapMode(res.TokensPath)
	}
	vectorstoreWrapperBin := strings.TrimSpace(*vectorstoreBin)
	controlState := newSupervisorControlState()
	controlState.setVersions(version, commit)
	controlState.setRequired(true, vectorstoreWrapperBin != "")
	controlState.setEndpoints(strings.TrimSpace(*brokerEndpoint), strings.TrimSpace(*vectorstoreEndpoint))
	controlState.setOperatorUI(gatewayPublicURLFromResolved(res), bootstrap)
	controlListen := strings.TrimSpace(*listen)
	if controlListen == "" {
		controlListen = "127.0.0.1:7710"
	}
	controlBaseURL := fmt.Sprintf("http://%s", controlListen)
	controlSrv := &http.Server{Addr: controlListen, Handler: buildWrapperControlMux(controlState, logStore)}
	controlLn, controlErr := net.Listen("tcp", controlListen)
	if controlErr != nil {
		fmt.Fprintf(os.Stderr, "chimera-supervisor: listen %s: %v\n", controlListen, controlErr)
		os.Exit(1)
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
		vectorstoreReadyzURL = fmt.Sprintf("http://%s/readyz", strings.TrimSpace(*vectorstoreListen))
	}
	gatewayReadyzURL := fmt.Sprintf("http://%s/readyz", strings.TrimSpace(*gatewayListen))
	brokerReadyzURL := fmt.Sprintf("http://%s/readyz", strings.TrimSpace(*brokerListen))

	var gatewayProc *exec.Cmd
	var gatewayWaitErr chan error
	var vectorstoreProc *exec.Cmd
	var vectorstoreWait chan error
	var brokerProc *exec.Cmd
	var brokerWaitErr chan error
	var indexerProc *exec.Cmd
	var indexerWait chan error

	indexerCtx, stopIndexer := context.WithCancel(context.Background())
	var supervisedShutdownOnce sync.Once
	stopChildrenGraceful := func() {
		supervisedShutdownOnce.Do(func() {
			stopIndexer()
			shutdownGrace := *shutdownTimeout
			if *terminateWait > shutdownGrace {
				shutdownGrace = *terminateWait
			}
			shutdownSupervisedChildren(log, shutdownGrace,
				supervisedChild{name: "gateway", cmd: gatewayProc, waitCh: gatewayWaitErr},
				supervisedChild{name: "vectorstore", cmd: vectorstoreProc, waitCh: vectorstoreWait},
				supervisedChild{name: "broker", cmd: brokerProc, waitCh: brokerWaitErr},
				supervisedChild{name: "indexer", cmd: indexerProc, waitCh: indexerWait},
			)
			if log != nil {
				log.Info("supervised shutdown complete", "msg", "chimera-supervisor.shutdown.children_done")
			}
		})
	}
	stopChildrenFast := func() {
		supervisedShutdownOnce.Do(func() {
			stopIndexer()
			killSupervisedWrapperFamilies(gatewayProc, brokerProc, vectorstoreProc)
		})
	}

	if !bootstrap {
		gatewayArgs := supervisedWrapperArgs([]string{
			"-config", path,
			"-listen", strings.TrimSpace(*gatewayListen),
			"-upstream-override", fmt.Sprintf("http://%s", strings.TrimSpace(*brokerEndpoint)),
		})
		gatewayProc = exec.Command(strings.TrimSpace(*gatewayBin), gatewayArgs...)
		gatewayProc.Env = mergeEnv(supervisedChildEnv(controlBaseURL))
		applyNoConsoleWindow(gatewayProc)
		gatewayChildSink := supervisedLogSink(logStore.Writer(servicelogs.SourceChimeraGateway), gatewayline.NewWriter)
		gatewayProc.Stdout = gatewayChildSink
		gatewayProc.Stderr = gatewayChildSink
		gerr := gatewayProc.Start()
		if gerr != nil {
			controlState.setLastError(gerr.Error())
			stopChildrenFast()
			fmt.Fprintf(os.Stderr, "chimera-supervisor: start chimera-gateway: %v\n", gerr)
			if errors.Is(gerr, exec.ErrNotFound) || strings.Contains(gerr.Error(), "executable file not found") {
				fmt.Fprintln(os.Stderr, "")
				fmt.Fprintln(os.Stderr, "No chimera-gateway wrapper binary found (place chimera-gateway next to chimera-supervisor, PATH, or pass -gateway-bin). From repo root:")
				fmt.Fprintln(os.Stderr, "  make chimera-gateway-build")
				fmt.Fprintln(os.Stderr, "  ./chimera-supervisor -gateway-bin ./chimera/bin/chimera-gateway")
			}
			os.Exit(1)
		}
		gatewayWaitErr = make(chan error, 1)
		go func() { gatewayWaitErr <- gatewayProc.Wait() }()
		if !*noWaitGateway {
			wCtx, wCancel := context.WithTimeout(context.Background(), *waitGateway)
			err := waitHealthy(wCtx, gatewayReadyzURL, *waitGateway, log, "")
			wCancel()
			if err != nil {
				controlState.setLastError(err.Error())
				stopChildrenFast()
				<-gatewayWaitErr
				fmt.Fprintf(os.Stderr, "chimera-supervisor: chimera-gateway not healthy: %v\n", err)
				os.Exit(1)
			}
		}

		if vectorstoreWrapperBin != "" {
			vectorstoreBackendBin := defaultSupervisorQdrantBin()
			vectorstoreArgs := supervisedWrapperArgs([]string{
				"-listen", strings.TrimSpace(*vectorstoreListen),
				"-bin", vectorstoreBackendBin,
				"-endpoint", strings.TrimSpace(*vectorstoreEndpoint),
				"-data-path", strings.TrimSpace(*vectorstoreDataPath),
			})
			var vectorstoreErr error
			vectorstoreProc = exec.Command(strings.TrimSpace(vectorstoreWrapperBin), vectorstoreArgs...)
			vectorstoreProc.Env = mergeEnv(supervisedChildEnv(controlBaseURL))
			applyNoConsoleWindow(vectorstoreProc)
			vectorstoreChildSink := supervisedLogSink(logStore.Writer(servicelogs.SourceChimeraVectorstore), vectorstoreline.NewWriter)
			vectorstoreProc.Stdout = vectorstoreChildSink
			vectorstoreProc.Stderr = vectorstoreChildSink
			vectorstoreErr = vectorstoreProc.Start()
			if vectorstoreErr != nil {
				controlState.setVectorstoreReady(false)
				controlState.setLastError(vectorstoreErr.Error())
				stopChildrenFast()
				fmt.Fprintf(os.Stderr, "chimera-supervisor: start chimera-vectorstore: %v\n", vectorstoreErr)
				os.Exit(1)
			}
			vectorstoreWait = make(chan error, 1)
			go func() { vectorstoreWait <- vectorstoreProc.Wait() }()
			if !*noWaitVectorstore {
				wCtx, wCancel := context.WithTimeout(context.Background(), *waitVectorstore)
				err := waitHealthy(wCtx, vectorstoreReadyzURL, *waitVectorstore, log, "")
				wCancel()
				if err != nil {
					controlState.setVectorstoreReady(false)
					controlState.setLastError(err.Error())
					stopChildrenFast()
					<-vectorstoreWait
					fmt.Fprintf(os.Stderr, "chimera-supervisor: chimera-vectorstore not healthy: %v\n", err)
					os.Exit(1)
				}
			}
			controlState.setVectorstoreReady(true)
		}

		brokerBackendBin := defaultSupervisorBifrostBin()
		brokerArgs := supervisedWrapperArgs([]string{
			"-listen", strings.TrimSpace(*brokerListen),
			"-bin", brokerBackendBin,
			"-endpoint", strings.TrimSpace(*brokerEndpoint),
			"-data-path", strings.TrimSpace(*brokerDataDir),
		})
		var berr error
		brokerProc = exec.Command(strings.TrimSpace(*brokerBin), brokerArgs...)
		brokerProc.Env = mergeEnv(supervisedChildEnv(controlBaseURL))
		applyNoConsoleWindow(brokerProc)
		brokerChildSink := supervisedLogSink(logStore.Writer(servicelogs.SourceChimeraBroker), brokerline.NewWriter)
		brokerProc.Stdout = brokerChildSink
		brokerProc.Stderr = brokerChildSink
		berr = brokerProc.Start()
		if berr != nil {
			controlState.setBrokerReady(false)
			controlState.setLastError(berr.Error())
			stopChildrenFast()
			if vectorstoreWait != nil {
				<-vectorstoreWait
			}
			fmt.Fprintf(os.Stderr, "chimera-supervisor: %v\n", berr)
			if errors.Is(berr, exec.ErrNotFound) || strings.Contains(berr.Error(), "executable file not found") {
				fmt.Fprintln(os.Stderr, "")
				fmt.Fprintln(os.Stderr, "No chimera-broker wrapper binary found (place chimera-broker next to chimera-supervisor, PATH, or pass -broker-bin). From repo root:")
				fmt.Fprintln(os.Stderr, "  make chimera-broker-build")
				fmt.Fprintln(os.Stderr, "  ./chimera-supervisor -broker-bin ./chimera/bin/chimera-broker")
			}
			os.Exit(1)
		}
		brokerWaitErr = make(chan error, 1)
		go func() { brokerWaitErr <- brokerProc.Wait() }()
		if !*noWait {
			wCtx, wCancel := context.WithTimeout(context.Background(), *waitTimeout)
			err := waitHealthy(wCtx, brokerReadyzURL, *waitTimeout, log, "")
			wCancel()
			if err != nil {
				controlState.setBrokerReady(false)
				controlState.setLastError(err.Error())
				stopChildrenFast()
				if vectorstoreWait != nil {
					<-vectorstoreWait
				}
				<-brokerWaitErr
				fmt.Fprintf(os.Stderr, "chimera-supervisor: chimera-broker not healthy: %v\n", err)
				os.Exit(1)
			}
		}
		controlState.setBrokerReady(true)

		idxScope := res.IndexerSupervisedEnabled && (res.RAG.Enabled || res.IndexerSupervisedStartWhenRAGDisabled)
		if idxScope {
			idxBin := strings.TrimSpace(res.IndexerSupervisedBin)
			if idxBin == "" {
				idxBin = defaultSupervisorIndexerBin()
			}
			if wd, werr := os.Getwd(); werr == nil {
				idxSink := supervisedLogSink(logStore.Writer(servicelogs.SourceChimeraIndexer), indexerline.NewWriter)
				gwLocal := gatewayPublicURLFromResolved(res)
				gwToken := resolveIndexerGatewayToken(res.TokensPath)
				idxLogJSON := res.IndexerSupervisedLogJSON || *logJSON
				var ierr error
				indexerProc, ierr = startIndexer(indexerCtx, indexerConfig{
					Bin: idxBin, ConfigPath: res.IndexerSupervisedConfigPath, WorkDir: wd, GatewayURL: gwLocal, GatewayToken: gwToken,
					LogJSON: idxLogJSON, Stdout: idxSink, Stderr: idxSink, Env: supervisedChildEnv(controlBaseURL),
				}, log)
				if ierr == nil {
					indexerWait = make(chan error, 1)
					go func() { indexerWait <- indexerProc.Wait() }()
				}
			}
		}
	}

	rootCtx, stopRoot := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopRoot()
	go func() {
		<-rootCtx.Done()
		log.Info("received shutdown signal", "msg", "chimera-supervisor.shutdown.signal_received")
		log.Info("shutting down gracefully", "msg", "chimera-supervisor.shutdown.graceful_start")
		stopChildrenGraceful()
	}()
	upstream.RunSupervisedChildHealthMonitor(rootCtx, log, "gateway", gatewayReadyzURL, 15*time.Second, 30*time.Second, !*noWaitGateway)
	upstream.RunSupervisedChildHealthMonitor(rootCtx, log, "broker", brokerReadyzURL, 15*time.Second, 30*time.Second, !*noWait)
	if vectorstoreReadyzURL != "" {
		upstream.RunSupervisedChildHealthMonitor(rootCtx, log, "vectorstore", vectorstoreReadyzURL, 15*time.Second, 30*time.Second, !*noWaitVectorstore)
	}
	<-rootCtx.Done()
	stopChildrenGraceful()
}

func defaultSupervisorIndexerBin() string {
	dir := executableDir()
	names := []string{"chimera-indexer"}
	if runtime.GOOS == "windows" {
		names = []string{"chimera-indexer.exe", "chimera-indexer"}
	}
	if p := firstExistingInSearchDirs(dir, names); p != "" {
		return p
	}
	if runtime.GOOS == "windows" {
		return "chimera-indexer.exe"
	}
	return "chimera-indexer"
}

func defaultSupervisorBrokerBin() string {
	dir := executableDir()
	names := []string{"chimera-broker"}
	if runtime.GOOS == "windows" {
		names = []string{"chimera-broker.exe", "chimera-broker"}
	}
	if p := firstExistingInSearchDirs(dir, names); p != "" {
		return p
	}
	if runtime.GOOS == "windows" {
		return "chimera-broker.exe"
	}
	return "chimera-broker"
}

func defaultSupervisorVectorstoreBin() string {
	dir := executableDir()
	names := []string{"chimera-vectorstore"}
	if runtime.GOOS == "windows" {
		names = []string{"chimera-vectorstore.exe", "chimera-vectorstore"}
	}
	return firstExistingInSearchDirs(dir, names)
}

func defaultSupervisorQdrantBin() string {
	dir := executableDir()
	names := []string{"qdrant"}
	if runtime.GOOS == "windows" {
		names = []string{"qdrant.exe", "qdrant"}
	}
	if p := firstExistingInSearchDirs(dir, names); p != "" {
		return p
	}
	if runtime.GOOS == "windows" {
		return "qdrant.exe"
	}
	return "qdrant"
}

func defaultSupervisorBifrostBin() string {
	dir := executableDir()
	names := []string{"bifrost-http", "bifrost"}
	if runtime.GOOS == "windows" {
		names = []string{"bifrost-http.exe", "bifrost.exe", "bifrost-http", "bifrost"}
	}
	if p := firstExistingInSearchDirs(dir, names); p != "" {
		return p
	}
	if runtime.GOOS == "windows" {
		return "bifrost-http.exe"
	}
	return "bifrost-http"
}

func defaultSupervisorGatewayBin() string {
	dir := executableDir()
	names := []string{"chimera-gateway"}
	if runtime.GOOS == "windows" {
		names = []string{"chimera-gateway.exe", "chimera-gateway"}
	}
	if p := firstExistingInSearchDirs(dir, names); p != "" {
		return p
	}
	if runtime.GOOS == "windows" {
		return "chimera-gateway.exe"
	}
	return "chimera-gateway"
}

func executableDir() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	return filepath.Dir(exe)
}

func firstExistingFile(dir string, names []string) string {
	for _, n := range names {
		p := filepath.Join(dir, n)
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
	}
	return ""
}

func firstExistingInSearchDirs(exeDir string, names []string) string {
	if exeDir == "" {
		return ""
	}
	for _, d := range []string{
		exeDir,
		filepath.Join(exeDir, "bin"),
		filepath.Join(exeDir, "chimera", "bin"),
	} {
		if p := firstExistingFile(d, names); p != "" {
			return p
		}
	}
	return ""
}

func buildLoggerTo(w io.Writer, gatewayPath string, json bool) *slog.Logger {
	lvl := slog.LevelInfo
	if e := os.Getenv("LOG_LEVEL"); e != "" {
		lvl = parseLogLevel(e)
	} else {
		res, err := config.LoadGatewayYAML(gatewayPath, nil)
		if err == nil {
			lvl = parseLogLevel(res.LogLevel)
		}
	}
	return logfmt.NewLogger(w, json, lvl)
}

func parseLogLevel(v string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

type indexerConfig struct {
	Bin          string
	ConfigPath   string
	WorkDir      string
	GatewayURL   string
	GatewayToken string
	LogJSON      bool
	Stdout       io.Writer
	Stderr       io.Writer
	Env          map[string]string
	RawExec      bool
	Args         []string
}

func startIndexer(ctx context.Context, cfg indexerConfig, log *slog.Logger) (*exec.Cmd, error) {
	if cfg.RawExec {
		bin := strings.TrimSpace(cfg.Bin)
		if bin == "" {
			return nil, fmt.Errorf("indexer: empty Bin")
		}
		var err error
		bin, err = absBinIfNeeded(bin)
		if err != nil {
			return nil, fmt.Errorf("resolve indexer binary path: %w", err)
		}
		cmd := exec.CommandContext(ctx, bin, cfg.Args...)
		cmd.Dir = strings.TrimSpace(cfg.WorkDir)
		if cmd.Dir == "" {
			wd, err := os.Getwd()
			if err != nil {
				return nil, fmt.Errorf("indexer work dir: %w", err)
			}
			cmd.Dir = wd
		}
		env := map[string]string{}
		if u := strings.TrimSpace(cfg.GatewayURL); u != "" {
			env[indexer.EnvGatewayURL] = strings.TrimSuffix(u, "/")
		}
		if t := strings.TrimSpace(cfg.GatewayToken); t != "" {
			env[indexer.EnvGatewayToken] = t
		}
		for k, v := range cfg.Env {
			env[k] = v
		}
		cmd.Env = mergeEnv(env)
		out := cfg.Stdout
		if out == nil {
			out = os.Stdout
		}
		errOut := cfg.Stderr
		if errOut == nil {
			errOut = os.Stderr
		}
		cmd.Stdout = out
		cmd.Stderr = errOut
		applyNoConsoleWindow(cmd)
		if err := cmd.Start(); err != nil {
			return nil, fmt.Errorf("indexer start: %w", err)
		}
		if log != nil {
			log.Debug("indexer supervised (raw exec)", "msg", "chimera-supervisor.indexer.raw_exec", "bin", bin, "args", cfg.Args)
		}
		return cmd, nil
	}

	bin := strings.TrimSpace(cfg.Bin)
	if bin == "" {
		return nil, fmt.Errorf("indexer: empty Bin")
	}
	var err error
	bin, err = absBinIfNeeded(bin)
	if err != nil {
		return nil, fmt.Errorf("resolve indexer binary path: %w", err)
	}
	cfgPath := strings.TrimSpace(cfg.ConfigPath)
	if cfgPath == "" {
		return nil, fmt.Errorf("indexer: empty ConfigPath")
	}
	cfgPath, err = filepath.Abs(cfgPath)
	if err != nil {
		return nil, fmt.Errorf("indexer config path: %w", err)
	}
	if err := indexer.EnsureSupervisedConfigFile(cfgPath); err != nil {
		return nil, fmt.Errorf("indexer config file: %w", err)
	}

	argv := []string{"--indexer-backend", "--config", cfgPath}
	if cfg.LogJSON {
		argv = append(argv, "--log-json")
	}
	workDir := strings.TrimSpace(cfg.WorkDir)
	if workDir == "" {
		return nil, fmt.Errorf("indexer: empty WorkDir")
	}

	cmd := exec.CommandContext(ctx, bin, argv...)
	cmd.Dir = workDir
	env := map[string]string{}
	if u := strings.TrimSpace(cfg.GatewayURL); u != "" {
		env[indexer.EnvGatewayURL] = strings.TrimSuffix(u, "/")
	}
	if t := strings.TrimSpace(cfg.GatewayToken); t != "" {
		env[indexer.EnvGatewayToken] = t
	}
	for k, v := range cfg.Env {
		env[k] = v
	}
	cmd.Env = mergeEnv(env)
	out := cfg.Stdout
	if out == nil {
		out = os.Stdout
	}
	errOut := cfg.Stderr
	if errOut == nil {
		errOut = os.Stderr
	}
	cmd.Stdout = out
	cmd.Stderr = errOut
	applyNoConsoleWindow(cmd)
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("indexer start: %w", err)
	}
	if log != nil {
		log.Info("indexer supervised", "msg", "chimera-supervisor.indexer.starting", "bin", bin, "config", cfgPath, "workdir", workDir, "log_json", cfg.LogJSON)
	}
	return cmd, nil
}

func waitHealthy(ctx context.Context, healthURL string, timeout time.Duration, log *slog.Logger, child string) error {
	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: 2 * time.Second}
	interval := 200 * time.Millisecond
	for {
		if timeout > 0 && time.Now().After(deadline) {
			return fmt.Errorf("timeout waiting for %s", healthURL)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, healthURL, nil)
		if err != nil {
			return err
		}
		res, err := client.Do(req)
		if err == nil {
			_, _ = io.Copy(io.Discard, res.Body)
			res.Body.Close()
			if res.StatusCode >= 200 && res.StatusCode < 300 {
				if log != nil {
					switch child {
					case "chimera-vectorstore":
						log.Info("chimera-vectorstore health OK", "msg", "chimera-supervisor.chimera-vectorstore.ready", "url", healthURL)
					case "chimera-broker":
						log.Info("chimera-broker health OK", "msg", "chimera-supervisor.chimera-broker.ready", "url", healthURL)
					}
				}
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(interval):
		}
	}
}

func absBinIfNeeded(bin string) (string, error) {
	if filepath.IsAbs(bin) {
		return bin, nil
	}
	if !strings.ContainsAny(bin, `/\`) {
		return bin, nil
	}
	return filepath.Abs(bin)
}

func mergeEnv(overrides map[string]string) []string {
	m := make(map[string]string)
	for _, e := range os.Environ() {
		i := strings.IndexByte(e, '=')
		if i <= 0 {
			continue
		}
		m[e[:i]] = e[i+1:]
	}
	for k, v := range overrides {
		m[k] = v
	}
	out := make([]string, 0, len(m))
	for k, v := range m {
		out = append(out, k+"="+v)
	}
	return out
}
