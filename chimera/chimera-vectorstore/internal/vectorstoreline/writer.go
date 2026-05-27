package vectorstoreline

import (
	"io"

	wline "github.com/lynn/porcelain/chimera/internal/wrapper/line"
)

// RegisterHTTPSummaryDestination registers dst for batched vectorstore.http.upsert.summary
// emission. Call once per logical log sink (e.g. supervisor capture). NewWriter does not
// register automatically — duplicate registration (e.g. stdout and stderr both teeing to the
// same capture) would emit identical summary lines.
func RegisterHTTPSummaryDestination(dst io.Writer) {
	registerHTTPSummaryDst(dst)
}

// NewWriter wraps dst and rewrites each complete line to normalized JSON (see NormalizePayload).
// Successful batched HTTP ingest paths are demoted to DEBUG; periodic INFO summaries are emitted
// via vectorstore.http.upsert.summary on destinations registered with RegisterHTTPSummaryDestination.
func NewWriter(dst io.Writer) io.Writer {
	if dst == nil {
		return nil
	}
	return wline.NewWriter(dst, func(line string) []byte {
		return postProcessNormalizedLine(NormalizePayload(line))
	})
}
