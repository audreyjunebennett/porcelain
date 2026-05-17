package conversationmerge

import (
	"encoding/binary"
	"fmt"
	"math"
)

// EncodeEmbedding stores float32 vector as little-endian raw bytes (dim inferred from len).
func EncodeEmbedding(v []float32) []byte {
	if len(v) == 0 {
		return nil
	}
	b := make([]byte, 4*len(v))
	for i, f := range v {
		binary.LittleEndian.PutUint32(b[i*4:], math.Float32bits(f))
	}
	return b
}

// DecodeEmbedding reverses EncodeEmbedding.
func DecodeEmbedding(dim int, blob []byte) ([]float32, error) {
	if dim <= 0 {
		return nil, fmt.Errorf("embedding: invalid dim %d", dim)
	}
	want := dim * 4
	if len(blob) != want {
		return nil, fmt.Errorf("embedding: blob len %d want %d", len(blob), want)
	}
	out := make([]float32, dim)
	for i := 0; i < dim; i++ {
		u := binary.LittleEndian.Uint32(blob[i*4:])
		out[i] = math.Float32frombits(u)
	}
	return out, nil
}
