// Package supervisorlogs subscribes to chimera-supervisor loopback log APIs and mirrors into servicelogs.Store.
package supervisorlogs

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/lynn/porcelain/chimera/internal/servicelogs"
)

const subscribeChanBuf = 256

// StartMirror connects to baseURL (e.g. http://127.0.0.1:7710), replays the supervisor buffer,
// then streams live entries into dst until ctx is cancelled.
func StartMirror(ctx context.Context, baseURL string, dst *servicelogs.Store, log *slog.Logger) {
	if dst == nil || strings.TrimSpace(baseURL) == "" {
		return
	}
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	go runMirror(ctx, base, dst, log)
}

func runMirror(ctx context.Context, base string, dst *servicelogs.Store, log *slog.Logger) {
	client := &http.Client{Timeout: 0}
	if err := catchUp(ctx, client, base, dst); err != nil && ctx.Err() == nil && log != nil {
		log.Warn("supervisor log catch-up failed", "msg", "gateway.supervisor_logs.catchup_failed", "base", base, "err", err)
	}
	backoff := time.Second
	for ctx.Err() == nil {
		err := streamLive(ctx, client, base, dst)
		if ctx.Err() != nil {
			return
		}
		if log != nil {
			log.Warn("supervisor log stream disconnected", "msg", "gateway.supervisor_logs.stream_disconnected", "base", base, "err", err, "retry_after", backoff)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(backoff):
		}
		if backoff < 15*time.Second {
			backoff += time.Second
		}
	}
}

func catchUp(ctx context.Context, client *http.Client, base string, dst *servicelogs.Store) error {
	u := base + "/logs?since=0"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: %s", u, res.Status)
	}
	var body servicelogs.PollResponse
	if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
		return err
	}
	if len(body.Lines) > 0 {
		dst.Import(body.Lines)
	}
	return nil
}

func streamLive(ctx context.Context, client *http.Client, base string, dst *servicelogs.Store) error {
	u := base + "/logs/stream?replay=none"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: %s", u, res.Status)
	}

	events := make(chan servicelogs.Entry, subscribeChanBuf)
	go func() {
		defer close(events)
		sc := bufio.NewScanner(res.Body)
		const maxLine = 512 << 10
		buf := make([]byte, 0, 64<<10)
		sc.Buffer(buf, maxLine)
		var data []byte
		for sc.Scan() {
			line := sc.Text()
			if strings.HasPrefix(line, "data:") {
				data = []byte(strings.TrimSpace(strings.TrimPrefix(line, "data:")))
				continue
			}
			if line == "" && len(data) > 0 {
				var ent servicelogs.Entry
				if err := json.Unmarshal(data, &ent); err == nil && ent.Seq > 0 {
					select {
					case events <- ent:
					default:
						// drop if importer blocked; Import is fast
						_ = ent
					}
				}
				data = nil
			}
		}
		if err := sc.Err(); err != nil && ctx.Err() == nil {
			return
		}
	}()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case ent, ok := <-events:
			if !ok {
				return io.EOF
			}
			dst.Import([]servicelogs.Entry{ent})
		}
	}
}
