package adapter

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"strings"

	"github.com/lynn/porcelain/chimera/chimera-indexer/internal/config"
	"github.com/lynn/porcelain/chimera/chimera-indexer/internal/indexerline"
	"github.com/lynn/porcelain/chimera/chimera-indexer/internal/platform"
)

// Indexer implements wruntime.Adapter for a supervised chimera-indexer backend.
type Indexer struct {
	Cfg config.Config
}

func (a *Indexer) Start(ctx context.Context, capture io.Writer, _ *slog.Logger) (*exec.Cmd, error) {
	bin := strings.TrimSpace(a.Cfg.Bin)
	args := append([]string(nil), a.Cfg.BackendArgs...)
	if bin == "" {
		exe, err := os.Executable()
		if err != nil {
			return nil, fmt.Errorf("resolve executable for embedded backend: %w", err)
		}
		bin = exe
		args = append([]string{"--indexer-backend"}, args...)
	}
	stdout := platform.StdoutTee(indexerline.NewWriter(capture))
	stderr := platform.StderrTee(indexerline.NewWriter(capture))
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = os.Environ()
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start indexer backend: %w", err)
	}
	return cmd, nil
}

func (a *Indexer) ReadyURL() string   { return "" }
func (a *Indexer) MetricsURL() string { return "" }

func (a *Indexer) BackendName() string { return "custom" }

// ChildLogWriter normalizes supervised chimera-indexer stdout for the operator log buffer.
func ChildLogWriter(dst io.Writer) io.Writer {
	return indexerline.NewWriter(dst)
}

// WrapUpstreamLine normalizes one raw backend log line for the wrapper runtime.
func WrapUpstreamLine(raw string) string {
	return string(indexerline.NormalizePayload(raw))
}
