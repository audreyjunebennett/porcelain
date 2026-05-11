package qdrantline

import (
	"bytes"
	"io"
)

// NewWriter wraps dst and rewrites each complete line to normalized JSON (see NormalizePayload).
func NewWriter(dst io.Writer) io.Writer {
	if dst == nil {
		return nil
	}
	return &lineWriter{dst: dst}
}

type lineWriter struct {
	dst io.Writer
	buf []byte
}

func (w *lineWriter) Write(p []byte) (int, error) {
	w.buf = append(w.buf, p...)
	for {
		i := bytes.IndexByte(w.buf, '\n')
		if i < 0 {
			break
		}
		line := string(w.buf[:i])
		w.buf = w.buf[i+1:]
		out := NormalizePayload(line)
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
