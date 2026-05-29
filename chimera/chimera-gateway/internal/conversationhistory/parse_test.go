package conversationhistory

import "testing"

func TestAssistantContentFromResponse_json(t *testing.T) {
	body := []byte(`{"choices":[{"message":{"content":"hello world"}}]}`)
	got := AssistantContentFromResponse(false, body)
	if got != "hello world" {
		t.Fatalf("got=%q", got)
	}
}

func TestAssistantContentFromResponse_sse(t *testing.T) {
	body := []byte("data: {\"choices\":[{\"delta\":{\"content\":\"hel\"}}]}\n\ndata: {\"choices\":[{\"delta\":{\"content\":\"lo\"}}]}\n\n")
	got := AssistantContentFromResponse(true, body)
	if got != "hello" {
		t.Fatalf("got=%q", got)
	}
}

func TestUsageFromResponse_json(t *testing.T) {
	body := []byte(`{"usage":{"prompt_tokens":1,"completion_tokens":2,"total_tokens":3}}`)
	p, c, tot, ok := UsageFromResponse(false, body)
	if !ok || p != 1 || c != 2 || tot != 3 {
		t.Fatalf("p=%d c=%d tot=%d ok=%v", p, c, tot, ok)
	}
}
