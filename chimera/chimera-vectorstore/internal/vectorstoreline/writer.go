package vectorstoreline

import (
	"io"

	wline "github.com/lynn/porcelain/chimera/internal/wrapper/line"
)

// NewWriter wraps dst and rewrites each complete line to normalized JSON (see NormalizePayload).
// Successful batched HTTP ingest paths are demoted to DEBUG; periodic INFO summaries are emitted
// via vectorstore.http.upsert.summary.
func NewWriter(dst io.Writer) io.Writer {
	if dst == nil {
		return nil
	}
	registerHTTPSummaryDst(dst)
	return wline.NewWriter(dst, func(line string) []byte {
		return postProcessNormalizedLine(NormalizePayload(line))
	})
}
