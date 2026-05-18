package supervise

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

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
