package bifrostadmin

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSummarizeProvider_plainKey(t *testing.T) {
	// Assemble key from fragments so static secret scanners do not match vendor key patterns.
	key := strings.Join([]string{"unit", "plain", "credential", "7890"}, "-")
	body, err := json.Marshal(map[string]any{
		"name": "groq",
		"keys": []any{
			map[string]any{
				"value": map[string]any{"value": key},
			},
		},
		"network_config": map[string]any{},
	})
	if err != nil {
		t.Fatal(err)
	}
	s, err := SummarizeProvider("groq", body)
	if err != nil {
		t.Fatal(err)
	}
	if !s.KeyConfigured {
		t.Fatal("expected configured")
	}
	if s.KeyHint != "••••7890" {
		t.Fatalf("hint %q", s.KeyHint)
	}
}

func TestSummarizeProvider_env(t *testing.T) {
	envName := strings.Join([]string{"UNITTEST", "PROVIDER", "SECRET"}, "_")
	body, err := json.Marshal(map[string]any{
		"keys": []any{
			map[string]any{
				"value": map[string]any{
					"from_env": true,
					"env_var":  envName,
				},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	s, err := SummarizeProvider("groq", body)
	if err != nil {
		t.Fatal(err)
	}
	want := "env:" + envName
	if !s.KeyConfigured || s.KeyHint != want {
		t.Fatalf("%+v", s)
	}
}

func TestSummarizeProvider_stringInlineKey(t *testing.T) {
	key := strings.Join([]string{"opaque", "inline", "fixture", "wxyz"}, "-")
	body, err := json.Marshal(map[string]any{
		"name": "gemini",
		"keys": []any{
			map[string]any{"name": "x", "value": key},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	s, err := SummarizeProvider("gemini", body)
	if err != nil {
		t.Fatal(err)
	}
	if !s.KeyConfigured {
		t.Fatal("expected configured")
	}
	if s.KeyHint != "••••wxyz" {
		t.Fatalf("hint %q", s.KeyHint)
	}
}

func TestSummarizeProvider_ollamaURL(t *testing.T) {
	body := []byte(`{"keys":[],"network_config":{"base_url":"http://localhost:11434"}}`)
	s, err := SummarizeProvider("ollama", body)
	if err != nil {
		t.Fatal(err)
	}
	if s.OllamaBaseURL != "http://localhost:11434" {
		t.Fatalf("%+v", s)
	}
}

func TestSummarizeProviderKeys_chimeraOrder(t *testing.T) {
	body := []byte(`{"keys":[
		{"name":"chimera-groq-key-2","value":"b"},
		{"name":"alpha-other","value":"a"},
		{"name":"chimera-groq-key-1","value":"c"}
	]}`)
	keys, err := SummarizeProviderKeys("groq", body)
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 3 {
		t.Fatalf("len %d", len(keys))
	}
	if keys[0].Name != "chimera-groq-key-1" || keys[1].Name != "chimera-groq-key-2" {
		t.Fatalf("order: %+v", keys)
	}
	if keys[2].Name != "alpha-other" {
		t.Fatalf("tail: %+v", keys)
	}
}

func TestSummarizeProvider_multiKeyHint(t *testing.T) {
	body := []byte(`{"keys":[
		{"name":"chimera-groq-key-1","value":"***"},
		{"name":"chimera-groq-key-2","value":"***"}
	]}`)
	s, err := SummarizeProvider("groq", body)
	if err != nil {
		t.Fatal(err)
	}
	if !s.KeyConfigured {
		t.Fatal("expected configured")
	}
	if s.KeyHint != "2 keys" {
		t.Fatalf("hint %q", s.KeyHint)
	}
}
