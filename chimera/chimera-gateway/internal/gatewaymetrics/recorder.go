package gatewaymetrics

import "time"

// Recorder is implemented by *Store. Pass nil from callers to disable recording.
type Recorder interface {
	RecordUpstreamResponse(at time.Time, modelID string, status int, estRequestTokens int)
}

var _ Recorder = (*Store)(nil)
