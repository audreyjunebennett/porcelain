package adapter

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"strings"

	"github.com/lynn/porcelain/chimera/chimera-broker/internal/brokerline"
	"github.com/lynn/porcelain/chimera/chimera-broker/internal/chimerabroker"
	"github.com/lynn/porcelain/chimera/chimera-broker/internal/config"
	"github.com/lynn/porcelain/chimera/internal/wrapper/contract"
)

// ChimeraBroker implements wruntime.Adapter for supervised chimera-broker-http (BiFrost).
type ChimeraBroker struct {
	Cfg  config.Config
	Host string
	Port int
}

func (a *ChimeraBroker) Start(ctx context.Context, capture io.Writer, log *slog.Logger) (*exec.Cmd, error) {
	return chimerabroker.Start(ctx, chimerabroker.Config{
		Bin:        a.Cfg.Bin,
		ConfigJSON: a.Cfg.ChimeraBrokerConfig,
		DataDir:    a.Cfg.DataPath,
		BindHost:   a.Host,
		Port:       a.Port,
		LogLevel:   a.Cfg.LogLevel,
		LogStyle:   a.Cfg.ChimeraBrokerLogStyle,
		Stdout:     brokerline.NewWriter(io.MultiWriter(capture, os.Stdout)),
		Stderr:     brokerline.NewWriter(io.MultiWriter(capture, os.Stderr)),
	}, log)
}

func (a *ChimeraBroker) ReadyURL() string {
	return fmt.Sprintf("http://%s:%d/health", strings.TrimSpace(a.Host), a.Port)
}

func (a *ChimeraBroker) MetricsURL() string {
	return fmt.Sprintf("http://%s:%d%s", strings.TrimSpace(a.Host), a.Port, contract.MetricsPath)
}

func (a *ChimeraBroker) BackendName() string {
	return "bifrost"
}

// ChildLogWriter normalizes supervised chimera-broker stdout for the operator log buffer.
func ChildLogWriter(dst io.Writer) io.Writer {
	return brokerline.NewWriter(dst)
}

// WrapUpstreamLine normalizes one raw backend log line for the wrapper runtime.
func WrapUpstreamLine(raw string) string {
	return string(brokerline.NormalizePayload(raw))
}
