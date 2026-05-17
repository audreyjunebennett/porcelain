package transform

import (
	"context"
	"encoding/json"
	"testing"
)

func TestParseScoreList_directArray(t *testing.T) {
	raw := `[{"name":"a","confidence":0.9},{"name":"b","confidence":0.1}]`
	got, err := parseScoreList(raw)
	if err != nil || len(got) != 2 || got[0].Name != "a" || got[1].Confidence != 0.1 {
		t.Fatalf("%+v err=%v", got, err)
	}
}

func TestParseScoreList_wrappedAndFence(t *testing.T) {
	raw := "```json\n{\"tools\":[{\"name\":\"x\",\"confidence\":0.5}]}\n```"
	got, err := parseScoreList(raw)
	if err != nil || len(got) != 1 || got[0].Name != "x" {
		t.Fatalf("%+v err=%v", got, err)
	}
}

func TestFilterToolsByConfidence(t *testing.T) {
	tools := []json.RawMessage{
		json.RawMessage(`{"type":"function","function":{"name":"keep","description":""}}`),
		json.RawMessage(`{"type":"function","function":{"name":"drop","description":""}}`),
	}
	names := []string{"keep", "drop"}
	scores := map[string]float64{"keep": 0.8, "drop": 0.2}
	got := filterToolsByConfidence(tools, names, scores, 0.35)
	if len(got) != 1 {
		t.Fatalf("len=%d", len(got))
	}
}

func TestFilterToolsByConfidence_allBelow_keepsAll(t *testing.T) {
	tools := []json.RawMessage{
		json.RawMessage(`{"type":"function","function":{"name":"a","description":""}}`),
	}
	names := []string{"a"}
	scores := map[string]float64{"a": 0.1}
	got := filterToolsByConfidence(tools, names, scores, 0.9)
	if len(got) != len(tools) {
		t.Fatal("expected fail-open to full list")
	}
}

func TestApplyToolRouter_disabled_noop(t *testing.T) {
	body := map[string]json.RawMessage{
		"tools": json.RawMessage(`[{"type":"function","function":{"name":"x","description":""}}]`),
	}
	out, sum := ApplyToolRouter(context.Background(), body, Config{Enabled: false, RouterModels: []string{"m"}})
	if sum.Ran {
		t.Fatal("expected router not to run when disabled")
	}
	if out["tools"] == nil || string(out["tools"]) != string(body["tools"]) {
		t.Fatal("expected unchanged")
	}
}
