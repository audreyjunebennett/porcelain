package gatewayline

import (
	"io"

	wline "github.com/lynn/porcelain/chimera/internal/wrapper/line"
)

func NewWriter(dst io.Writer) io.Writer {
	return wline.NewWriter(dst, NormalizePayload)
}
