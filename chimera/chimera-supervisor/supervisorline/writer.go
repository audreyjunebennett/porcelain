package supervisorline

import (
	"io"

	wline "github.com/lynn/porcelain/chimera/internal/wrapper/line"
)

// NewWriter returns a line-buffering writer that emits normalized JSON per line.
func NewWriter(dst io.Writer) io.Writer {
	return wline.NewWriter(dst, NormalizePayload)
}
