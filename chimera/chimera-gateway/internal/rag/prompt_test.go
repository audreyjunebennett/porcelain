package rag

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/lynn/porcelain/chimera/internal/vectorstore"
)

func TestLastUserText_String(t *testing.T) {
	raw := json.RawMessage(`[{"role":"system","content":"sys"},{"role":"user","content":"hello world"}]`)
	if got := LastUserText(raw); got != "hello world" {
		t.Fatalf("got %q", got)
	}
}

func TestLastUserText_MultimodalArray(t *testing.T) {
	raw := json.RawMessage(`[{"role":"user","content":[{"type":"text","text":"first"},{"type":"image_url","image_url":{}},{"type":"text","text":"second"}]}]`)
	got := LastUserText(raw)
	if !strings.Contains(got, "first") || !strings.Contains(got, "second") {
		t.Fatalf("got %q", got)
	}
}

func TestLastUserText_PicksLastUser(t *testing.T) {
	raw := json.RawMessage(`[{"role":"user","content":"old"},{"role":"assistant","content":"a"},{"role":"user","content":"new"}]`)
	if got := LastUserText(raw); got != "new" {
		t.Fatalf("got %q", got)
	}
}

func TestLastUserText_NoUserReturnsEmpty(t *testing.T) {
	raw := json.RawMessage(`[{"role":"system","content":"sys"}]`)
	if got := LastUserText(raw); got != "" {
		t.Fatalf("got %q", got)
	}
}

func TestFormatRetrievedContext(t *testing.T) {
	hits := []vectorstore.Hit{
		{ID: "1", Score: 0.92, Payload: vectorstore.Payload{Text: "alpha", Source: "a.md"}},
		{ID: "2", Score: 0.85, Payload: vectorstore.Payload{Text: "beta", Source: "b.md"}},
	}
	got := FormatRetrievedContext(hits)
	for _, want := range []string{"### Retrieved context", "a.md", "alpha", "b.md", "beta"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in: %s", want, got)
		}
	}
}

func TestInjectSystemMessage(t *testing.T) {
	body := map[string]json.RawMessage{
		"messages": json.RawMessage(`[{"role":"user","content":"q"}]`),
	}
	InjectSystemMessage(body, "ctx")
	var arr []map[string]any
	if err := json.Unmarshal(body["messages"], &arr); err != nil {
		t.Fatal(err)
	}
	if len(arr) != 2 || arr[0]["role"] != "system" || arr[0]["content"] != "ctx" {
		t.Fatalf("got %+v", arr)
	}
	if arr[1]["role"] != "user" {
		t.Fatalf("user not preserved")
	}
}

func TestInjectSystemMessage_NoOpWhenContextEmpty(t *testing.T) {
	body := map[string]json.RawMessage{
		"messages": json.RawMessage(`[{"role":"user","content":"q"}]`),
	}
	orig := body["messages"]
	InjectSystemMessage(body, "")
	if string(body["messages"]) != string(orig) {
		t.Fatalf("body mutated when context empty")
	}
}
