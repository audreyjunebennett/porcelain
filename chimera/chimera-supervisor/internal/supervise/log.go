package supervise

import (
	"io"
	"log/slog"
	"os"
	"strings"

	gwconfig "github.com/lynn/porcelain/chimera/internal/config"
	"github.com/lynn/porcelain/chimera/internal/logfmt"
)

// LogSink normalizes child stdout/stderr to JSON lines and records them in the ring buffer.
func LogSink(storeWriter io.Writer, normalize func(io.Writer) io.Writer) io.Writer {
	return normalize(io.MultiWriter(storeWriter, os.Stdout))
}

func buildLogger(w io.Writer, gatewayPath string, json bool) *slog.Logger {
	lvl := slog.LevelInfo
	if e := os.Getenv("LOG_LEVEL"); e != "" {
		lvl = parseLogLevel(e)
	} else {
		res, err := gwconfig.LoadGatewayYAML(gatewayPath, nil)
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
