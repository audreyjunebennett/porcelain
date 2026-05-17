package line

import (
	"bytes"
	"encoding/json"
	"io"
	"net/url"
	"strconv"
	"strings"
	"sync"
)

// NewWriter wraps dst and rewrites each complete line using normalize.
func NewWriter(dst io.Writer, normalize func(string) []byte) io.Writer {
	if dst == nil || normalize == nil {
		return nil
	}
	return &lineWriter{dst: dst, normalize: normalize}
}

type lineWriter struct {
	dst       io.Writer
	normalize func(string) []byte
	buf       []byte
	mu        sync.Mutex
}

func (w *lineWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.buf = append(w.buf, p...)
	for {
		i := bytes.IndexByte(w.buf, '\n')
		if i < 0 {
			break
		}
		line := string(w.buf[:i])
		w.buf = w.buf[i+1:]
		out := w.normalize(line)
		if len(out) == 0 {
			continue
		}
		if _, err := w.dst.Write(out); err != nil {
			return len(p), err
		}
		if _, err := w.dst.Write([]byte{'\n'}); err != nil {
			return len(p), err
		}
	}
	const maxFrag = 64 << 10
	if gap := len(w.buf) - maxFrag; gap > 0 {
		w.buf = w.buf[gap:]
	}
	return len(p), nil
}

// NormalizePerLine provides common line normalization scaffolding.
func NormalizePerLine(
	raw string,
	alreadyNormalized func([]byte) ([]byte, bool),
	normalizePlain func(string) []byte,
	normalizeJSON func(string) []byte,
) []byte {
	raw = strings.TrimSuffix(strings.TrimSpace(raw), "\r")
	if raw == "" {
		return nil
	}
	if alreadyNormalized != nil {
		if ja, ok := alreadyNormalized([]byte(raw)); ok {
			return ja
		}
	}
	if raw[0] != '{' {
		if normalizePlain == nil {
			return nil
		}
		return normalizePlain(raw)
	}
	if normalizeJSON == nil {
		return nil
	}
	return normalizeJSON(raw)
}

// AlreadyNormalizedChimera checks for _chimera_norm=1 and expected msg/service prefix.
func AlreadyNormalizedChimera(raw []byte, msgPrefix, service string) ([]byte, bool) {
	var ck struct {
		Msg         string `json:"msg"`
		Service     string `json:"service"`
		ChimeraNorm int    `json:"_chimera_norm"`
	}
	if json.Unmarshal(raw, &ck) != nil {
		return nil, false
	}
	if ck.ChimeraNorm == 1 && strings.HasPrefix(ck.Msg, msgPrefix) && ck.Service == service {
		return raw, true
	}
	return nil, false
}

func TrimRunes(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

func JSONString(fields map[string]json.RawMessage, key string) string {
	raw, ok := fields[key]
	if !ok {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) != nil {
		return ""
	}
	return s
}

func IntFromJSON(fields map[string]json.RawMessage, key string) int {
	raw, ok := fields[key]
	if !ok {
		return 0
	}
	var f float64
	if json.Unmarshal(raw, &f) == nil {
		return int(f)
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		n, _ := strconv.Atoi(strings.TrimSpace(s))
		return n
	}
	var n int
	if json.Unmarshal(raw, &n) == nil {
		return n
	}
	return 0
}

func FloatFromJSON(fields map[string]json.RawMessage, key string) float64 {
	raw, ok := fields[key]
	if !ok {
		return 0
	}
	var f float64
	if json.Unmarshal(raw, &f) == nil {
		return f
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		x, _ := strconv.ParseFloat(strings.TrimSpace(s), 64)
		return x
	}
	return 0
}

func PortFromURL(u string) int {
	pu, err := url.Parse(u)
	if err != nil {
		return 0
	}
	if pu.Port() != "" {
		p, _ := strconv.Atoi(pu.Port())
		return p
	}
	switch strings.ToLower(pu.Scheme) {
	case "http":
		return 80
	case "https":
		return 443
	default:
		return 0
	}
}
