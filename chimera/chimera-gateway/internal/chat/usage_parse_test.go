package chat

import "testing"

func TestUsageFromChatCompletionJSON(t *testing.T) {
	t.Parallel()
	p, c, tot, ok := usageFromChatCompletionJSON([]byte(`{"usage":{"prompt_tokens":10,"completion_tokens":20,"total_tokens":30}}`))
	if !ok || p != 10 || c != 20 || tot != 30 {
		t.Fatalf("got prompt=%d completion=%d total=%d ok=%v", p, c, tot, ok)
	}
}

func TestUsageFromChatCompletionJSON_infersTotal(t *testing.T) {
	t.Parallel()
	p, c, tot, ok := usageFromChatCompletionJSON([]byte(`{"usage":{"prompt_tokens":5,"completion_tokens":7,"total_tokens":0}}`))
	if !ok || p != 5 || c != 7 || tot != 12 {
		t.Fatalf("got prompt=%d completion=%d total=%d ok=%v", p, c, tot, ok)
	}
}

func TestUsageFromChatCompletionJSON_invalid(t *testing.T) {
	t.Parallel()
	_, _, _, ok := usageFromChatCompletionJSON([]byte(`not json`))
	if ok {
		t.Fatal("expected !ok")
	}
}
