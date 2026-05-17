package brokerline

import (
	"io"

	wline "github.com/lynn/porcelain/chimera/internal/wrapper/line"
)

// NewWriter wraps dst and rewrites each complete line to normalized JSON (see NormalizePayload).
func NewWriter(dst io.Writer) io.Writer {
	return wline.NewWriter(dst, NormalizePayload)
}
