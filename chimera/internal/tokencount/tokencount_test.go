package tokencount

import "testing"

func TestCount_known(t *testing.T) {
	// cl100k_base: "hello world" -> 2 tokens
	n, err := Count("hello world")
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("got %d tokens, want 2", n)
	}
}

func TestCount_empty(t *testing.T) {
	n, err := Count("")
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("got %d, want 0", n)
	}
}

func TestCountEncoding_o200k_base_known(t *testing.T) {
	n, err := CountEncoding(EncodingO200kBase, "hello world")
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("got %d tokens, want 2", n)
	}
}
