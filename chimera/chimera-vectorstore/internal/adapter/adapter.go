package adapter

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"strings"

	"github.com/lynn/porcelain/chimera/chimera-vectorstore/internal/config"
	"github.com/lynn/porcelain/chimera/chimera-vectorstore/internal/qdrant"
	"github.com/lynn/porcelain/chimera/chimera-vectorstore/vectorstoreline"
	"github.com/lynn/porcelain/chimera/internal/wrapper/contract"
	"github.com/lynn/porcelain/internal/naming"
)

// Qdrant implements wruntime.Adapter for a supervised Qdrant binary.
type Qdrant struct {
	Cfg  config.Config
	Host string
	Port int
}

func (a *Qdrant) Start(ctx context.Context, capture io.Writer, log *slog.Logger) (*exec.Cmd, error) {
	return qdrant.Start(ctx, qdrant.Config{
		Bin:        a.Cfg.Bin,
		StorageDir: a.Cfg.DataPath,
		BindHost:   a.Host,
		HTTPPort:   a.Port,
		GRPCPort:   a.Cfg.GRPCPort,
		LogLevel:   a.Cfg.LogLevel,
		Stdout:     vectorstoreline.NewWriter(io.MultiWriter(capture, os.Stdout)),
		Stderr:     vectorstoreline.NewWriter(io.MultiWriter(capture, os.Stderr)),
	}, log)
}

func (a *Qdrant) ReadyURL() string {
	return fmt.Sprintf("http://%s:%d/collections", strings.TrimSpace(a.Host), a.Port)
}

func (a *Qdrant) MetricsURL() string {
	return fmt.Sprintf("http://%s:%d%s", strings.TrimSpace(a.Host), a.Port, contract.MetricsPath)
}

func (a *Qdrant) BackendName() string {
	return naming.ProductQdrantBinName
}

// WrapUpstreamLine normalizes one raw Qdrant log line for the wrapper runtime.
func WrapUpstreamLine(raw string) string {
	return string(vectorstoreline.NormalizePayload(raw))
}
