package conversationwitness

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRedactSecrets_bearer(t *testing.T) {
	t.Parallel()
	in := "Authorization: Bearer abcdefghijklmnop123456"
	out := RedactSecrets(in)
	if !strings.Contains(out, "***") || out == in {
		t.Fatalf("expected redaction: %q", out)
	}
}

func TestRequestWitnessStats_basic(t *testing.T) {
	t.Parallel()
	body := map[string]json.RawMessage{
		"messages": json.RawMessage(`[
			{"role":"system","content":"sys"},
			{"role":"user","content":"hello there"},
			{"role":"assistant","content":"hi"}
		]`),
		"tools": json.RawMessage(`[{"type":"function","function":{"name":"x"}}]`),
	}
	n, rc, pe, td, ok := requestWitnessStats(body)
	if !ok || n != 3 || td != 1 || pe < 5 {
		t.Fatalf("n=%d rc=%q pe=%d td=%d ok=%v", n, rc, pe, td, ok)
	}
	var m map[string]int
	if err := json.Unmarshal([]byte(rc), &m); err != nil {
		t.Fatal(err)
	}
	if m["user"] != 1 || m["system"] != 1 || m["assistant"] != 1 {
		t.Fatalf("roles=%v", m)
	}
}

func TestResponseWitnessStats_nonStream(t *testing.T) {
	t.Parallel()
	b := []byte(`{"choices":[{"finish_reason":"stop","message":{"content":"abc def"}}]}`)
	c, fr, ch, ok := responseWitnessStats(false, b)
	if !ok || ch != 1 || fr != "stop" || c != 7 {
		t.Fatalf("c=%d fr=%q ch=%d ok=%v", c, fr, ch, ok)
	}
}

func TestSplitHeadTail(t *testing.T) {
	t.Parallel()
	h, tail := SplitHeadTail("0123456789abcdefgh", 4)
	if h != "0123" || tail != "efgh" {
		t.Fatalf("h=%q tail=%q", h, tail)
	}
}
