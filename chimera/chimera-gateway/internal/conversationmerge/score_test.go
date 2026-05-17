package conversationmerge

import (
	"math"
	"testing"
)

func TestCosineSimilarity_parallel(t *testing.T) {
	a := []float32{1, 0, 0}
	b := []float32{1, 0, 0}
	if v := CosineSimilarity(a, b); math.Abs(v-1) > 1e-6 {
		t.Fatalf("got %v", v)
	}
}

func TestWordJaccard_overlap(t *testing.T) {
	j := WordJaccard("hello world", "world test")
	if j <= 0 || j >= 1 {
		t.Fatalf("got %v", j)
	}
}

func TestMatchScore_weights(t *testing.T) {
	s := MatchScore(1, 1, 1)
	if math.Abs(s-1) > 1e-9 {
		t.Fatalf("got %v", s)
	}
}
