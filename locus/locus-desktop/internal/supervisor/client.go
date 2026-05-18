package supervisor

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/lynn/porcelain/internal/locus"
)

// BaseURL derives the supervisor HTTP base URL from launcher args (-listen).
func BaseURL(args []string) string {
	addr := locus.DefaultSupervisorListen
	for i := 0; i < len(args); i++ {
		raw := strings.TrimSpace(args[i])
		lower := strings.ToLower(raw)
		switch {
		case lower == "-listen" || lower == "--listen":
			if i+1 < len(args) {
				addr = strings.TrimSpace(args[i+1])
				i++
			}
		case strings.HasPrefix(lower, "-listen=") || strings.HasPrefix(lower, "--listen="):
			if idx := strings.IndexByte(raw, '='); idx >= 0 {
				addr = strings.TrimSpace(raw[idx+1:])
			}
		}
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return "http://" + locus.DefaultSupervisorListen
	}
	host = strings.TrimSpace(host)
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	if strings.Contains(host, ":") && !strings.HasPrefix(host, "[") {
		host = "[" + host + "]"
	}
	return "http://" + host + ":" + port
}

// Reachable reports whether /healthz returns 200.
func Reachable(baseURL string) bool {
	client := &http.Client{Timeout: 1200 * time.Millisecond}
	resp, err := client.Get(strings.TrimRight(baseURL, "/") + "/healthz")
	if err != nil {
		return false
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// WaitReachable polls Reachable until timeout.
func WaitReachable(baseURL string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if Reachable(baseURL) {
			return true
		}
		time.Sleep(350 * time.Millisecond)
	}
	return false
}

// WaitReady polls Ready until timeout.
func WaitReady(baseURL string, timeout time.Duration) (bool, string) {
	deadline := time.Now().Add(timeout)
	last := "readiness endpoint unavailable"
	for time.Now().Before(deadline) {
		ready, detail := Ready(baseURL)
		if ready {
			return true, ""
		}
		if strings.TrimSpace(detail) != "" {
			last = detail
		}
		time.Sleep(350 * time.Millisecond)
	}
	return false, last
}

// Ready reports supervisor readiness via /readyz, with a narrow /status fallback for bootstrap.
func Ready(baseURL string) (bool, string) {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	client := &http.Client{Timeout: 1500 * time.Millisecond}
	resp, err := client.Get(baseURL + "/readyz")
	if err != nil {
		return false, "readyz unavailable"
	}
	if resp.StatusCode == http.StatusOK {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
		return true, ""
	}
	if resp.StatusCode == http.StatusServiceUnavailable {
		_ = resp.Body.Close()
		return readyFromStatus(client, baseURL)
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	return false, fmt.Sprintf("readyz returned status %d", resp.StatusCode)
}

func readyFromStatus(client *http.Client, baseURL string) (bool, string) {
	resp, err := client.Get(baseURL + "/status")
	if err != nil {
		return false, "supervisor reports degraded readiness"
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false, "supervisor reports degraded readiness"
	}
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var body map[string]any
	if json.Unmarshal(raw, &body) != nil {
		return false, "supervisor reports degraded readiness"
	}
	if bootstrap, ok := body["bootstrap"].(bool); ok && bootstrap {
		return true, ""
	}
	if det, _ := body["details"].(map[string]any); det != nil {
		if ui, _ := det["operator_ui"].(map[string]any); ui != nil {
			if b, _ := ui["bootstrap"].(bool); b {
				return true, ""
			}
		}
	}
	return false, "supervisor reports degraded readiness"
}

// EntryURL resolves the webview entry route from supervisor /status.
func EntryURL(supervisorBase string) string {
	uiBase, bootstrap := operatorUIFromStatus(supervisorBase)
	if uiBase == "" {
		uiBase = strings.TrimRight(strings.TrimSpace(supervisorBase), "/")
	}
	if bootstrap {
		return uiBase + "/ui/setup"
	}
	return uiBase + "/ui/login?next=" + url.PathEscape(locus.DefaultLoginNextPath)
}

func operatorUIFromStatus(supervisorBase string) (uiBase string, bootstrap bool) {
	supervisorBase = strings.TrimRight(strings.TrimSpace(supervisorBase), "/")
	if supervisorBase == "" {
		return "", false
	}
	client := &http.Client{Timeout: 1500 * time.Millisecond}
	resp, err := client.Get(supervisorBase + "/status")
	if err != nil {
		return "", false
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	var body map[string]any
	if json.Unmarshal(raw, &body) != nil {
		return "", false
	}
	det, _ := body["details"].(map[string]any)
	if det == nil {
		return "", false
	}
	ui, _ := det["operator_ui"].(map[string]any)
	if ui == nil {
		return "", false
	}
	s, _ := ui["base_url"].(string)
	s = strings.TrimRight(strings.TrimSpace(s), "/")
	if s == "" {
		return "", false
	}
	b, _ := ui["bootstrap"].(bool)
	return s, b
}

// MonitorRuntimeLoss sends one reason on out after consecutive health misses.
func MonitorRuntimeLoss(ctx context.Context, baseURL string, out chan<- string, pollInterval time.Duration, missThreshold int) {
	if out == nil {
		return
	}
	t := time.NewTicker(pollInterval)
	defer t.Stop()
	misses := 0
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if Reachable(baseURL) {
				misses = 0
				continue
			}
			misses++
			if misses >= missThreshold {
				select {
				case out <- "runtime health checks failed":
				default:
				}
				return
			}
		}
	}
}
