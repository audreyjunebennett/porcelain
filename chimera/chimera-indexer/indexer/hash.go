package indexer

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
)

// HashFile streams the file at path through SHA-256 and returns the hex
// digest prefixed with the algorithm tag the gateway accepts ("sha256:").
func HashFile(path string) (string, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, f)
	if err != nil {
		return "", n, err
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil)), n, nil
}
