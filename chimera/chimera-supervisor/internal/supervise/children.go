package supervise

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"

	brokeradapter "github.com/lynn/porcelain/chimera/chimera-broker/adapter"
	"github.com/lynn/porcelain/chimera/chimera-gateway/gatewayline"
	indexeradapter "github.com/lynn/porcelain/chimera/chimera-indexer/adapter"
	svconfig "github.com/lynn/porcelain/chimera/chimera-supervisor/internal/config"
	"github.com/lynn/porcelain/chimera/chimera-supervisor/internal/control"
	"github.com/lynn/porcelain/chimera/chimera-supervisor/internal/proc"
	vectorstoreadapter "github.com/lynn/porcelain/chimera/chimera-vectorstore/adapter"
	gwconfig "github.com/lynn/porcelain/chimera/internal/config"
	"github.com/lynn/porcelain/chimera/internal/servicelogs"
	"github.com/lynn/porcelain/chimera/internal/tokens"
	"github.com/lynn/porcelain/internal/naming"
)

func startGatewayChild(cfg svconfig.Config, path, controlBaseURL string, logStore *servicelogs.Store, log *slog.Logger, controlState *control.State, gatewayProc **exec.Cmd, gatewayWaitErr *chan error, gatewayReadyzURL string, stopChildrenFast func()) error {
	gatewayArgs := WrapperArgs([]string{
		"-config", path,
		"-listen", strings.TrimSpace(cfg.GatewayListen),
		"-upstream-override", fmt.Sprintf("http://%s", strings.TrimSpace(cfg.BrokerEndpoint)),
	})
	cmd := exec.Command(strings.TrimSpace(cfg.GatewayBin), gatewayArgs...)
	cmd.Env = mergeEnv(ChildEnv(controlBaseURL))
	proc.ApplyNoConsoleWindow(cmd)
	gatewayChildSink := LogSink(logStore.Writer(servicelogs.SourceChimeraGateway), gatewayline.NewWriter)
	cmd.Stdout = gatewayChildSink
	cmd.Stderr = gatewayChildSink
	if gerr := cmd.Start(); gerr != nil {
		controlState.SetLastError(gerr.Error())
		stopChildrenFast()
		if errors.Is(gerr, exec.ErrNotFound) || strings.Contains(gerr.Error(), "executable file not found") {
			return svconfig.Exitf(1, "start chimera-gateway: %v\n\nNo chimera-gateway wrapper binary found (place chimera-gateway next to chimera-supervisor, PATH, or pass -gateway-bin). From repo root:\n  make chimera-gateway-build\n  ./chimera-supervisor -gateway-bin ./chimera/bin/chimera-gateway", gerr)
		}
		return svconfig.Exitf(1, "start chimera-gateway: %v", gerr)
	}
	*gatewayProc = cmd
	ch := make(chan error, 1)
	go func() { ch <- cmd.Wait() }()
	*gatewayWaitErr = ch
	if !cfg.NoWaitGateway {
		wCtx, wCancel := context.WithTimeout(context.Background(), cfg.WaitGateway)
		err := waitHealthy(wCtx, gatewayReadyzURL, cfg.WaitGateway, log, "")
		wCancel()
		if err != nil {
			controlState.SetLastError(err.Error())
			stopChildrenFast()
			<-ch
			return svconfig.Exitf(1, "chimera-gateway not healthy: %v", err)
		}
	}
	return nil
}

func startVectorstoreChild(cfg svconfig.Config, controlBaseURL string, logStore *servicelogs.Store, log *slog.Logger, controlState *control.State, vectorstoreWrapperBin string, vectorstoreProc **exec.Cmd, vectorstoreWait *chan error, vectorstoreReadyzURL string, stopChildrenFast func()) error {
	vectorstoreBackendBin := svconfig.DefaultQdrantBin()
	vectorstoreArgs := WrapperArgs([]string{
		"-listen", strings.TrimSpace(cfg.VectorstoreListen),
		"-bin", vectorstoreBackendBin,
		"-endpoint", strings.TrimSpace(cfg.VectorstoreEndpoint),
		"-data-path", strings.TrimSpace(cfg.VectorstoreDataPath),
	})
	cmd := exec.Command(strings.TrimSpace(vectorstoreWrapperBin), vectorstoreArgs...)
	cmd.Env = mergeEnv(ChildEnv(controlBaseURL))
	proc.ApplyNoConsoleWindow(cmd)
	vectorstoreChildSink := LogSink(logStore.Writer(servicelogs.SourceChimeraVectorstore), vectorstoreadapter.ChildLogWriter)
	cmd.Stdout = vectorstoreChildSink
	cmd.Stderr = vectorstoreChildSink
	if vectorstoreErr := cmd.Start(); vectorstoreErr != nil {
		controlState.SetVectorstoreReady(false)
		controlState.SetLastError(vectorstoreErr.Error())
		stopChildrenFast()
		return svconfig.Exitf(1, "start chimera-vectorstore: %v", vectorstoreErr)
	}
	*vectorstoreProc = cmd
	ch := make(chan error, 1)
	go func() { ch <- cmd.Wait() }()
	*vectorstoreWait = ch
	if !cfg.NoWaitVectorstore {
		wCtx, wCancel := context.WithTimeout(context.Background(), cfg.WaitVectorstore)
		err := waitHealthy(wCtx, vectorstoreReadyzURL, cfg.WaitVectorstore, log, "")
		wCancel()
		if err != nil {
			controlState.SetVectorstoreReady(false)
			controlState.SetLastError(err.Error())
			stopChildrenFast()
			<-ch
			return svconfig.Exitf(1, "chimera-vectorstore not healthy: %v", err)
		}
	}
	controlState.SetVectorstoreReady(true)
	return nil
}

func startBrokerChild(cfg svconfig.Config, controlBaseURL string, logStore *servicelogs.Store, log *slog.Logger, controlState *control.State, brokerProc **exec.Cmd, brokerWaitErr *chan error, brokerReadyzURL string, vectorstoreWait chan error, stopChildrenFast func()) error {
	brokerBackendBin := svconfig.DefaultBifrostBin()
	brokerArgs := WrapperArgs([]string{
		"-listen", strings.TrimSpace(cfg.BrokerListen),
		"-bin", brokerBackendBin,
		"-endpoint", strings.TrimSpace(cfg.BrokerEndpoint),
		"-data-path", strings.TrimSpace(cfg.BrokerDataDir),
	})
	cmd := exec.Command(strings.TrimSpace(cfg.BrokerBin), brokerArgs...)
	cmd.Env = mergeEnv(ChildEnv(controlBaseURL))
	proc.ApplyNoConsoleWindow(cmd)
	brokerChildSink := LogSink(logStore.Writer(servicelogs.SourceChimeraBroker), brokeradapter.ChildLogWriter)
	cmd.Stdout = brokerChildSink
	cmd.Stderr = brokerChildSink
	if berr := cmd.Start(); berr != nil {
		controlState.SetBrokerReady(false)
		controlState.SetLastError(berr.Error())
		stopChildrenFast()
		if vectorstoreWait != nil {
			<-vectorstoreWait
		}
		if errors.Is(berr, exec.ErrNotFound) || strings.Contains(berr.Error(), "executable file not found") {
			return svconfig.Exitf(1, "%v\n\nNo chimera-broker wrapper binary found (place chimera-broker next to chimera-supervisor, PATH, or pass -broker-bin). From repo root:\n  make chimera-broker-build\n  ./chimera-supervisor -broker-bin ./chimera/bin/chimera-broker", berr)
		}
		return svconfig.Exitf(1, "%v", berr)
	}
	*brokerProc = cmd
	ch := make(chan error, 1)
	go func() { ch <- cmd.Wait() }()
	*brokerWaitErr = ch
	if !cfg.NoWaitBroker {
		wCtx, wCancel := context.WithTimeout(context.Background(), cfg.WaitBroker)
		err := waitHealthy(wCtx, brokerReadyzURL, cfg.WaitBroker, log, "")
		wCancel()
		if err != nil {
			controlState.SetBrokerReady(false)
			controlState.SetLastError(err.Error())
			stopChildrenFast()
			if vectorstoreWait != nil {
				<-vectorstoreWait
			}
			<-ch
			return svconfig.Exitf(1, "chimera-broker not healthy: %v", err)
		}
	}
	controlState.SetBrokerReady(true)
	return nil
}

func startIndexerChild(res *gwconfig.Resolved, cfg svconfig.Config, path, controlBaseURL string, logStore *servicelogs.Store, log *slog.Logger, indexerCtx context.Context, indexerProc **exec.Cmd, indexerWait *chan error) {
	idxScope := res.IndexerSupervisedEnabled && (res.RAG.Enabled || res.IndexerSupervisedStartWhenRAGDisabled)
	if !idxScope {
		return
	}
	idxBin := strings.TrimSpace(res.IndexerSupervisedBin)
	if idxBin == "" {
		idxBin = svconfig.DefaultIndexerBin()
	}
	wd, werr := os.Getwd()
	if werr != nil {
		return
	}
	idxSink := LogSink(logStore.Writer(servicelogs.SourceChimeraIndexer), indexeradapter.ChildLogWriter)
	gwLocal := gatewayPublicURLFromResolved(res)
	gwToken := resolveIndexerGatewayToken(res.TokensPath)
	idxLogJSON := res.IndexerSupervisedLogJSON || cfg.LogJSON
	cmd, ierr := StartIndexer(indexerCtx, IndexerConfig{
		Bin: idxBin, ConfigPath: res.IndexerSupervisedConfigPath, WorkDir: wd, GatewayURL: gwLocal, GatewayToken: gwToken,
		LogJSON: idxLogJSON, Stdout: idxSink, Stderr: idxSink, Env: ChildEnv(controlBaseURL),
	}, log)
	if ierr != nil {
		return
	}
	*indexerProc = cmd
	ch := make(chan error, 1)
	go func() { ch <- cmd.Wait() }()
	*indexerWait = ch
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
