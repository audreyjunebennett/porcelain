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
	embedadapter "github.com/lynn/porcelain/chimera/chimera-embed/adapter"
	indexeradapter "github.com/lynn/porcelain/chimera/chimera-indexer/adapter"
	svconfig "github.com/lynn/porcelain/chimera/chimera-supervisor/internal/config"
	"github.com/lynn/porcelain/chimera/chimera-supervisor/internal/control"
	"github.com/lynn/porcelain/chimera/chimera-supervisor/internal/proc"
	vectorstoreadapter "github.com/lynn/porcelain/chimera/chimera-vectorstore/adapter"
	gwconfig "github.com/lynn/porcelain/chimera/internal/config"
	"github.com/lynn/porcelain/chimera/internal/gatewayline"
	"github.com/lynn/porcelain/chimera/internal/servicelogs"
	"github.com/lynn/porcelain/chimera/internal/tokens"
	"github.com/lynn/porcelain/internal/naming"
)

func startGatewayChild(cfg svconfig.Config, path, controlBaseURL string, logStore *servicelogs.Store, logLevel slog.Level, log *slog.Logger, controlState *control.State, gatewayProc **exec.Cmd, gatewayWaitErr *chan error, gatewayReadyzURL string, stopChildrenFast func()) error {
	gatewayArgs := []string{
		"-config", path,
		"-listen", strings.TrimSpace(cfg.GatewayListen),
		"-broker-override", fmt.Sprintf("http://%s", strings.TrimSpace(cfg.BrokerEndpoint)),
	}
	if cfg.WaitGateway > 0 {
		gatewayArgs = append(gatewayArgs, "-startup-timeout", cfg.WaitGateway.String())
	}
	if res, err := gwconfig.LoadGatewayYAML(path, nil); err == nil && res != nil {
		gatewayArgs = append(gatewayArgs, "-gateway-listen", res.ListenAddr())
	}
	gatewayArgs = WrapperArgs(gatewayArgs)
	cmd := exec.Command(strings.TrimSpace(cfg.GatewayBin), gatewayArgs...)
	cmd.Env = mergeEnv(ChildEnv(controlBaseURL))
	proc.ApplyNoConsoleWindow(cmd)
	gatewayChildSink := LogSink(logStore.Writer(servicelogs.SourceChimeraGateway), gatewayline.NewWriter, logLevel)
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

func startVectorstoreChild(cfg svconfig.Config, res *gwconfig.Resolved, controlBaseURL string, logStore *servicelogs.Store, logLevel slog.Level, log *slog.Logger, controlState *control.State, vectorstoreWrapperBin string, vectorstoreProc **exec.Cmd, vectorstoreWait *chan error, vectorstoreReadyzURL string, stopChildrenFast func()) error {
	vectorstoreBackendBin := svconfig.DefaultQdrantBin()
	vectorstoreLogLevel := ""
	if res != nil {
		vectorstoreLogLevel = res.RAG.QdrantLogLevel
	}
	vectorstoreArgs := appendBackendLogLevel(WrapperArgs([]string{
		"-listen", strings.TrimSpace(cfg.VectorstoreListen),
		"-bin", vectorstoreBackendBin,
		"-endpoint", strings.TrimSpace(cfg.VectorstoreEndpoint),
		"-data-path", strings.TrimSpace(cfg.VectorstoreDataPath),
	}), vectorstoreLogLevel)
	cmd := exec.Command(strings.TrimSpace(vectorstoreWrapperBin), vectorstoreArgs...)
	cmd.Env = mergeEnv(ChildEnv(controlBaseURL))
	proc.ApplyNoConsoleWindow(cmd)
	vectorstoreChildSink := LogSink(logStore.Writer(servicelogs.SourceChimeraVectorstore), vectorstoreadapter.ChildLogWriter, logLevel)
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

func startEmbedChild(cfg svconfig.Config, res *gwconfig.Resolved, controlBaseURL string, logStore *servicelogs.Store, logLevel slog.Level, log *slog.Logger, controlState *control.State, embedWrapperBin string, embedProc **exec.Cmd, embedWait *chan error, embedReadyzURL string, stopChildrenFast func()) error {
	embedBackendBin := svconfig.DefaultLlamaServerBin()
	modelPath := strings.TrimSpace(cfg.EmbedModelPath)
	cacheDir := strings.TrimSpace(cfg.EmbedCacheDir)
	embedLogLevel := ""
	if res != nil {
		if modelPath == "" {
			modelPath = res.InternalEmbedding.ModelPath
		}
		if cacheDir == "" {
			cacheDir = res.InternalEmbedding.CacheDir
		}
		embedLogLevel = res.InternalEmbedding.LogLevel
	}
	embedArgs := appendBackendLogLevel(WrapperArgs([]string{
		"-listen", strings.TrimSpace(cfg.EmbedListen),
		"-bin", embedBackendBin,
		"-endpoint", strings.TrimSpace(cfg.EmbedEndpoint),
		"-model-path", modelPath,
		"-cache-dir", cacheDir,
	}), embedLogLevel)
	cmd := exec.Command(strings.TrimSpace(embedWrapperBin), embedArgs...)
	cmd.Env = mergeEnv(ChildEnv(controlBaseURL))
	proc.ApplyNoConsoleWindow(cmd)
	embedChildSink := LogSink(logStore.Writer(servicelogs.SourceChimeraEmbed), embedadapter.ChildLogWriter, logLevel)
	cmd.Stdout = embedChildSink
	cmd.Stderr = embedChildSink
	if embedErr := cmd.Start(); embedErr != nil {
		controlState.SetEmbedReady(false)
		controlState.SetLastError(embedErr.Error())
		stopChildrenFast()
		return svconfig.Exitf(1, "start chimera-embed: %v", embedErr)
	}
	*embedProc = cmd
	ch := make(chan error, 1)
	go func() { ch <- cmd.Wait() }()
	*embedWait = ch
	if !cfg.NoWaitEmbed {
		wCtx, wCancel := context.WithTimeout(context.Background(), cfg.WaitEmbed)
		err := waitHealthy(wCtx, embedReadyzURL, cfg.WaitEmbed, log, "")
		wCancel()
		if err != nil {
			controlState.SetEmbedReady(false)
			controlState.SetLastError(err.Error())
			stopChildrenFast()
			<-ch
			return svconfig.Exitf(1, "chimera-embed not healthy: %v", err)
		}
	}
	controlState.SetEmbedReady(true)
	return nil
}

func startBrokerChild(cfg svconfig.Config, res *gwconfig.Resolved, controlBaseURL string, logStore *servicelogs.Store, logLevel slog.Level, log *slog.Logger, controlState *control.State, brokerProc **exec.Cmd, brokerWaitErr *chan error, brokerReadyzURL string, vectorstoreWait chan error, stopChildrenFast func()) error {
	brokerBackendBin := svconfig.DefaultBifrostBin()
	brokerLogLevel := ""
	if res != nil {
		brokerLogLevel = res.BrokerLogLevel
	}
	brokerArgs := appendBackendLogLevel(WrapperArgs([]string{
		"-listen", strings.TrimSpace(cfg.BrokerListen),
		"-bin", brokerBackendBin,
		"-endpoint", strings.TrimSpace(cfg.BrokerEndpoint),
		"-data-path", strings.TrimSpace(cfg.BrokerDataDir),
	}), brokerLogLevel)
	cmd := exec.Command(strings.TrimSpace(cfg.BrokerBin), brokerArgs...)
	cmd.Env = mergeEnv(ChildEnv(controlBaseURL))
	proc.ApplyNoConsoleWindow(cmd)
	brokerChildSink := LogSink(logStore.Writer(servicelogs.SourceChimeraBroker), brokeradapter.ChildLogWriter, logLevel)
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

func startIndexerChild(res *gwconfig.Resolved, cfg svconfig.Config, path, controlBaseURL string, logStore *servicelogs.Store, logLevel slog.Level, log *slog.Logger, indexerCtx context.Context, indexerProc **exec.Cmd, indexerWait *chan error) {
	idxScope := res.IndexerSupervisedEnabled && (res.RAG.Enabled || res.IndexerSupervisedStartWhenRAGDisabled)
	if !idxScope {
		if log != nil {
			log.Info("indexer supervised disabled", "msg", "chimera-supervisor.indexer.skipped",
				"supervised_enabled", res.IndexerSupervisedEnabled,
				"rag_enabled", res.RAG.Enabled,
				"start_when_rag_disabled", res.IndexerSupervisedStartWhenRAGDisabled)
		}
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
	idxSink := LogSink(logStore.Writer(servicelogs.SourceChimeraIndexer), indexeradapter.ChildLogWriter, logLevel)
	gwLocal := gatewayPublicURLFromResolved(res)
	gwToken := resolveIndexerGatewayToken(res.TokensPath)
	if gwToken == "" && log != nil {
		log.Warn("indexer gateway token missing", "msg", "chimera-supervisor.indexer.token_missing",
			"hint", "set CHIMERA_GATEWAY_TOKEN or add api-keys.yaml rows")
	}
	idxLogJSON := res.IndexerSupervisedLogJSON || cfg.LogJSON
	childEnv := ChildEnv(controlBaseURL)
	if gwToken != "" {
		childEnv[naming.EnvGatewayTokenTarget] = gwToken
	}
	cmd, ierr := StartIndexer(indexerCtx, IndexerConfig{
		Bin: idxBin, ConfigPath: res.IndexerSupervisedConfigPath, WorkDir: wd, GatewayURL: gwLocal, GatewayToken: gwToken,
		LogJSON: idxLogJSON, Stdout: idxSink, Stderr: idxSink, Env: childEnv,
	}, log)
	if ierr != nil {
		if log != nil {
			log.Warn("indexer supervised start skipped", "msg", "chimera-supervisor.indexer.start_failed", "err", ierr)
		}
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
