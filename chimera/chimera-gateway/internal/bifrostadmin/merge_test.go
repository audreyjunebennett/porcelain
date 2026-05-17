package bifrostadmin

import (
	"encoding/json"
	"testing"
)

func TestAppendProviderAPIKey_preservesConcurrency(t *testing.T) {
	in := []byte(`{"name":"groq","keys":[{"id":"k1","name":"groq-default","weight":1,"value":{"value":"***"}}],"concurrency_and_buffer_size":{"concurrency":5,"buffer_size":10}}`)
	out, err := AppendProviderAPIKey("groq", in, "new-secret")
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(out, &doc); err != nil {
		t.Fatal(err)
	}
	keys := doc["keys"].([]any)
	if len(keys) != 2 {
		t.Fatalf("want 2 keys, got %d", len(keys))
	}
	k1 := keys[1].(map[string]any)
	if k1["value"] != "new-secret" {
		t.Fatalf("%+v", k1)
	}
	name, _ := k1["name"].(string)
	if name != "chimera-groq-key-1" {
		t.Fatalf("name %q", name)
	}
	if _, has := k1["models"]; has {
		t.Fatalf("expected no models field, got %+v", k1["models"])
	}
	if k1["weight"].(float64) != 1 {
		t.Fatalf("weight %+v", k1["weight"])
	}
	k0 := keys[0].(map[string]any)
	if k0["weight"].(float64) != 1 {
		t.Fatalf("equalized keys[0] weight %+v", k0["weight"])
	}
	cb := doc["concurrency_and_buffer_size"].(map[string]any)
	if cb["concurrency"].(float64) != 5 {
		t.Fatalf("%+v", cb)
	}
}

func TestAppendProviderAPIKey_secondNameIncrements(t *testing.T) {
	in := []byte(`{"keys":[{"name":"chimera-groq-key-1","value":"a","weight":1}]}`)
	out, err := AppendProviderAPIKey("groq", in, "b")
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	_ = json.Unmarshal(out, &doc)
	keys := doc["keys"].([]any)
	if len(keys) != 2 {
		t.Fatal(len(keys))
	}
	n2 := keys[1].(map[string]any)["name"].(string)
	if n2 != "chimera-groq-key-2" {
		t.Fatalf("got %q", n2)
	}
}

func TestAppendProviderAPIKey_emptyRoot(t *testing.T) {
	out, err := AppendProviderAPIKey("gemini", []byte("{}"), "first-key")
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(out, &doc); err != nil {
		t.Fatal(err)
	}
	keys := doc["keys"].([]any)
	if len(keys) != 1 {
		t.Fatalf("keys: %v", keys)
	}
	k0 := keys[0].(map[string]any)
	if k0["name"] != "chimera-gemini-key-1" {
		t.Fatalf("name: %v", k0["name"])
	}
}

func TestAppendProviderAPIKey_namesDifferByProvider(t *testing.T) {
	gq, err := AppendProviderAPIKey("groq", []byte("{}"), "a")
	if err != nil {
		t.Fatal(err)
	}
	gm, err := AppendProviderAPIKey("gemini", []byte("{}"), "b")
	if err != nil {
		t.Fatal(err)
	}
	var a, b map[string]any
	_ = json.Unmarshal(gq, &a)
	_ = json.Unmarshal(gm, &b)
	n0 := a["keys"].([]any)[0].(map[string]any)["name"]
	n1 := b["keys"].([]any)[0].(map[string]any)["name"]
	if n0 == n1 {
		t.Fatalf("both names %v", n0)
	}
}

func TestRemoveProviderKeyByName(t *testing.T) {
	in := []byte(`{"name":"groq","keys":[
		{"name":"chimera-groq-key-1","value":"a","weight":1},
		{"name":"other","value":"b","weight":1}
	]}`)
	out, err := RemoveProviderKeyByName(in, "chimera-groq-key-1")
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	_ = json.Unmarshal(out, &doc)
	keys := doc["keys"].([]any)
	if len(keys) != 1 {
		t.Fatal(len(keys))
	}
	if keys[0].(map[string]any)["name"] != "other" {
		t.Fatalf("%+v", keys[0])
	}
}

func TestRemoveProviderKeyByName_emptyName(t *testing.T) {
	_, err := RemoveProviderKeyByName([]byte(`{"keys":[]}`), "  ")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestMergeOllamaBaseURL(t *testing.T) {
	in := []byte(`{"name":"ollama","keys":[],"network_config":{"base_url":"http://old:11434"},"concurrency_and_buffer_size":{"concurrency":1,"buffer_size":2}}`)
	out, err := MergeOllamaBaseURL(in, "http://host:11434")
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	_ = json.Unmarshal(out, &doc)
	nc := doc["network_config"].(map[string]any)
	if nc["base_url"] != "http://host:11434" {
		t.Fatalf("%+v", nc)
	}
}

func TestMergeOllamaBaseURL_emptyRoot(t *testing.T) {
	out, err := MergeOllamaBaseURL([]byte("{}"), "http://host:11434")
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := json.Unmarshal(out, &doc); err != nil {
		t.Fatal(err)
	}
	nc := doc["network_config"].(map[string]any)
	if nc["base_url"] != "http://host:11434" {
		t.Fatalf("%+v", nc)
	}
	keys := doc["keys"].([]any)
	if len(keys) != 0 {
		t.Fatalf("keys: %v", keys)
	}
}
