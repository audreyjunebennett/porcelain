// Package modelcatalog classifies broker model ids for safe general text routing.
package modelcatalog

import (
	"sort"
	"strings"
)

// Status is the result of applying the general chat/coding capability policy.
type Status string

const (
	StatusIncluded Status = "included"
	StatusExcluded Status = "excluded"
	StatusSkipped  Status = "skipped"
)

// Decision explains whether a model may enter a generated general chat/coding chain.
type Decision struct {
	ModelID string `json:"model_id" yaml:"model_id"`
	Status  Status `json:"status" yaml:"status"`
	Role    string `json:"role" yaml:"role"`
	Reason  string `json:"reason" yaml:"reason"`
}

var specialistMarkers = []struct {
	role    string
	markers []string
}{
	{role: "embedding", markers: []string{"embed", "embedding"}},
	{role: "audio", markers: []string{"whisper", "transcrib", "tts", "text-to-speech", "speech-to-text", "orpheus", "native-audio", "live-preview", "live-translate", "audio-preview"}},
	{role: "image_generation", markers: []string{"imagen", "image", "nano-banana", "dall-e", "diffusion", "stable-diffusion", "flux-"}},
	{role: "video_generation", markers: []string{"/veo", "veo-"}},
	{role: "music_generation", markers: []string{"lyria", "music-generation"}},
	{role: "safety", markers: []string{"prompt-guard", "safeguard", "safety", "moderation", "llama-guard", "shield"}},
	{role: "reranker", markers: []string{"rerank"}},
	{role: "robotics", markers: []string{"robotics"}},
	{role: "research_agent", markers: []string{"deep-research"}},
	{role: "computer_control", markers: []string{"computer-use", "antigravity"}},
	{role: "attributed_qa", markers: []string{"/aqa"}},
}

var generalFamilyMarkers = []string{
	"allam", "claude", "codestral", "coder", "command", "compound", "deepseek",
	"devstral", "gemini", "gemma", "gpt", "granite", "kimi", "llama", "mistral",
	"mixtral", "nemotron", "phi", "qwen", "starcoder", "yi-",
}

// Classify returns a conservative, explainable decision. Provider capability metadata wins
// when present; otherwise only known general text model families are included. Specialist
// markers always win so an embedding, TTS, safety, or similar endpoint cannot silently enter.
func Classify(modelID string, capabilities []string) Decision {
	id := strings.TrimSpace(modelID)
	lower := strings.ToLower(id)
	if id == "" {
		return Decision{ModelID: id, Status: StatusSkipped, Role: "unknown", Reason: "empty model id"}
	}
	for _, group := range specialistMarkers {
		for _, marker := range group.markers {
			if strings.Contains(lower, marker) {
				return Decision{ModelID: id, Status: StatusExcluded, Role: group.role, Reason: "specialist endpoint marker: " + marker}
			}
		}
	}

	capSet := make(map[string]struct{}, len(capabilities))
	for _, capability := range capabilities {
		capability = strings.ToLower(strings.TrimSpace(capability))
		if capability != "" {
			capSet[capability] = struct{}{}
		}
	}
	for _, capability := range []string{"completion", "chat", "text-generation", "tools"} {
		if _, ok := capSet[capability]; ok {
			return Decision{ModelID: id, Status: StatusIncluded, Role: "general_text", Reason: "provider capability: " + capability}
		}
	}
	if len(capSet) > 0 {
		return Decision{ModelID: id, Status: StatusSkipped, Role: "unknown", Reason: "provider capabilities do not declare text completion/chat"}
	}

	for _, marker := range generalFamilyMarkers {
		if strings.Contains(lower, marker) {
			return Decision{ModelID: id, Status: StatusIncluded, Role: "general_text", Reason: "known general text model family: " + marker}
		}
	}
	return Decision{ModelID: id, Status: StatusSkipped, Role: "unknown", Reason: "no trusted general text capability evidence"}
}

// FilterGeneralCandidates classifies every id and returns included ids in input order.
func FilterGeneralCandidates(ids []string) ([]string, []Decision) {
	seen := make(map[string]struct{}, len(ids))
	eligible := make([]string, 0, len(ids))
	decisions := make([]Decision, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		decision := Classify(id, nil)
		decisions = append(decisions, decision)
		if decision.Status == StatusIncluded {
			eligible = append(eligible, id)
		}
	}
	return eligible, decisions
}

// OrderByPreference preserves explicit operator preference for eligible models, then appends
// the remainder deterministically. localFirst is an explicit privacy policy for the remainder.
func OrderByPreference(ids, preference []string, localFirst bool) []string {
	eligible := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id != "" {
			eligible[id] = struct{}{}
		}
	}
	out := make([]string, 0, len(eligible))
	used := make(map[string]struct{}, len(eligible))
	for _, id := range preference {
		id = strings.TrimSpace(id)
		if _, ok := eligible[id]; !ok {
			continue
		}
		if _, ok := used[id]; ok {
			continue
		}
		used[id] = struct{}{}
		out = append(out, id)
	}
	remaining := make([]string, 0, len(eligible)-len(out))
	for id := range eligible {
		if _, ok := used[id]; !ok {
			remaining = append(remaining, id)
		}
	}
	sort.Slice(remaining, func(i, j int) bool {
		if localFirst {
			il, jl := strings.HasPrefix(remaining[i], "ollama/"), strings.HasPrefix(remaining[j], "ollama/")
			if il != jl {
				return il
			}
		}
		return remaining[i] < remaining[j]
	})
	return append(out, remaining...)
}
