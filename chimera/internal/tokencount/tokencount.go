// Package tokencount provides fast local token counts via tiktoken-compatible encodings.
// Counts may differ from the upstream provider when the routed model uses another tokenizer.
package tokencount

import (
	"fmt"
	"sync"

	"github.com/pkoukk/tiktoken-go"
)

// Common tiktoken encoding names used by this gateway.
const (
	EncodingCl100kBase = "cl100k_base"
	EncodingO200kBase  = "o200k_base"
)

var (
	encMu    sync.Mutex
	encoders = map[string]*tiktoken.Tiktoken{}
)

func encoder(name string) (*tiktoken.Tiktoken, error) {
	encMu.Lock()
	defer encMu.Unlock()
	if e, ok := encoders[name]; ok {
		return e, nil
	}
	e, err := tiktoken.GetEncoding(name)
	if err != nil {
		return nil, fmt.Errorf("tiktoken encoding %q: %w", name, err)
	}
	encoders[name] = e
	return e, nil
}

// CountEncoding returns the token count for s using the named tiktoken encoding (EncodeOrdinary).
func CountEncoding(encodingName, s string) (int, error) {
	e, err := encoder(encodingName)
	if err != nil {
		return 0, err
	}
	return len(e.EncodeOrdinary(s)), nil
}

// Count returns the cl100k_base token count for s (ordinary BPE; no special-token handling).
func Count(s string) (int, error) {
	return CountEncoding(EncodingCl100kBase, s)
}
