package conversationmerge

import (
	"crypto/sha256"
	"encoding/hex"
)

// RollingFingerprint hashes prior fingerprint plus this turn's normalized texts (separate from matching).
func RollingFingerprint(prevFingerprint, userNorm, modelNorm string) string {
	h := sha256.Sum256([]byte(prevFingerprint + "\x1e" + userNorm + "\x1e" + modelNorm))
	return hex.EncodeToString(h[:])
}

// DedupKey identifies duplicate completions for the same logical request retry.
func DedupKey(conversationID, incomingFingerprint, userNormalized string) string {
	h := sha256.Sum256([]byte(conversationID + "\x00" + incomingFingerprint + "\x00" + userNormalized))
	return hex.EncodeToString(h[:])
}
