package chat

import "testing"

func TestUsageFromOpenAIChatSSE_lastChunk(t *testing.T) {
	t.Parallel()
	sse := []byte(`data: {"choices":[{"delta":{"content":"x"}}]}

data: {"choices":[],"usage":{"prompt_tokens":10,"completion_tokens":20,"total_tokens":30}}

`)
	p, c, tot, ok := usageFromOpenAIChatSSE(sse)
	if !ok || p != 10 || c != 20 || tot != 30 {
		t.Fatalf("got prompt=%d completion=%d total=%d ok=%v", p, c, tot, ok)
	}
}
