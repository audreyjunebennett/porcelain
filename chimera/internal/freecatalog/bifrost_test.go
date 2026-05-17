package freecatalog

import "testing"

func TestToGroqBiFrost(t *testing.T) {
	if s := ToGroqBiFrost("llama-3.1-8b-instant"); s != "groq/llama-3.1-8b-instant" {
		t.Fatal(s)
	}
	if s := ToGroqBiFrost("openai/gpt-oss-20b"); s != "groq/openai/gpt-oss-20b" {
		t.Fatal(s)
	}
}

func TestToGeminiBiFrost(t *testing.T) {
	if s := ToGeminiBiFrost("gemini-2.0-flash"); s != "gemini/gemini-2.0-flash" {
		t.Fatal(s)
	}
}
