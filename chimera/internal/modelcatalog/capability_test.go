package modelcatalog

import (
	"reflect"
	"testing"
)

func TestClassifyGeneralAndSpecialistModels(t *testing.T) {
	tests := []struct {
		id   string
		caps []string
		want Status
		role string
	}{
		{id: "groq/openai/gpt-oss-120b", want: StatusIncluded, role: "general_text"},
		{id: "ollama/qwen3-vl:8b", caps: []string{"completion", "vision", "tools"}, want: StatusIncluded, role: "general_text"},
		{id: "gemini/gemini-2.5-flash-preview-tts", want: StatusExcluded, role: "audio"},
		{id: "gemini/gemini-3.1-flash-live-preview", want: StatusExcluded, role: "audio"},
		{id: "groq/whisper-large-v3", want: StatusExcluded, role: "audio"},
		{id: "ollama/nomic-embed-text:latest", caps: []string{"embedding"}, want: StatusExcluded, role: "embedding"},
		{id: "groq/openai/gpt-oss-safeguard-20b", want: StatusExcluded, role: "safety"},
		{id: "gemini/gemini-3-pro-image-preview", want: StatusExcluded, role: "image_generation"},
		{id: "gemini/veo-3.1-generate-preview", want: StatusExcluded, role: "video_generation"},
		{id: "gemini/lyria-3-pro-preview", want: StatusExcluded, role: "music_generation"},
		{id: "gemini/deep-research-preview-04-2026", want: StatusExcluded, role: "research_agent"},
		{id: "gemini/gemini-2.5-computer-use-preview-10-2025", want: StatusExcluded, role: "computer_control"},
		{id: "gemini/antigravity-preview-05-2026", want: StatusExcluded, role: "computer_control"},
		{id: "gemini/aqa", want: StatusExcluded, role: "attributed_qa"},
		{id: "provider/new-opaque-endpoint", want: StatusSkipped, role: "unknown"},
	}
	for _, tc := range tests {
		t.Run(tc.id, func(t *testing.T) {
			got := Classify(tc.id, tc.caps)
			if got.Status != tc.want || got.Role != tc.role || got.Reason == "" {
				t.Fatalf("got %+v want status=%s role=%s", got, tc.want, tc.role)
			}
		})
	}
}

func TestOrderByPreference(t *testing.T) {
	ids := []string{"groq/qwen/a", "ollama/qwen:b", "gemini/gemini-c"}
	got := OrderByPreference(ids, []string{"gemini/gemini-c", "missing/x"}, true)
	want := []string{"gemini/gemini-c", "ollama/qwen:b", "groq/qwen/a"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}
