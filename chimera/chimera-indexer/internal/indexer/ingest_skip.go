package indexer

import (
	"bufio"
	"io"
	"os"
	"unicode"
)

// fileHasNoIngestableText reports whether POST /v1/ingest would reject the file
// with "empty document text": the UTF-8 body trims to nothing (same rule as
// strings.TrimSpace in the gateway).
func fileHasNoIngestableText(path string) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()
	br := bufio.NewReader(f)
	for {
		r, _, err := br.ReadRune()
		if err != nil {
			if err == io.EOF {
				return true, nil
			}
			return false, err
		}
		if !unicode.IsSpace(r) {
			return false, nil
		}
	}
}
