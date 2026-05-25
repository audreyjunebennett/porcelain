package adapter

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"strings"

	"github.com/lynn/porcelain/chimera/chimera-embed/internal/config"
	"github.com/lynn/porcelain/chimera/chimera-embed/internal/embedline"
	"github.com/lynn/porcelain/chimera/chimera-embed/internal/llamaserver"
	"github.com/lynn/porcelain/chimera/internal/wrapper/contract"
	"github.com/lynn/porcelain/internal/naming"
)

// LlamaServer implements wruntime.Adapter for supervised llama-server embedding mode.
type LlamaServer struct {
	Cfg  config.Config
	Host string
	Port int
}

func (a *LlamaServer) Start(ctx context.Context, capture io.Writer, log *slog.Logger) (*exec.Cmd, error) {
	return llamaserver.Start(ctx, llamaserver.Config{
		Bin:        a.Cfg.Bin,
		ModelPath:  a.Cfg.ModelPath,
		CacheDir:   a.Cfg.CacheDir,
		BindHost:   a.Host,
		Port:       a.Port,
		CtxSize:    a.Cfg.CtxSize,
		NGPULayers: a.Cfg.NGPULayers,
		Pooling:    a.Cfg.Pooling,
		LogLevel:   a.Cfg.LogLevel,
		Stdout:     embedline.NewWriter(io.MultiWriter(capture, os.Stdout)),
		Stderr:     embedline.NewWriter(io.MultiWriter(capture, os.Stderr)),
	}, log)
}

func (a *LlamaServer) ReadyURL() string {
	return fmt.Sprintf("http://%s:%d/health", strings.TrimSpace(a.Host), a.Port)
}

func (a *LlamaServer) MetricsURL() string {
	return fmt.Sprintf("http://%s:%d%s", strings.TrimSpace(a.Host), a.Port, contract.MetricsPath)
}

func (a *LlamaServer) BackendName() string {
	return naming.ProductLlamaServerBinName
}

// ChildLogWriter normalizes supervised chimera-embed stdout for the operator log buffer.
func ChildLogWriter(dst io.Writer) io.Writer {
	return embedline.NewWriter(dst)
}

// WrapUpstreamLine normalizes one raw llama-server log line for the wrapper runtime.
func WrapUpstreamLine(raw string) string {
	return string(embedline.NormalizePayload(raw))
}
