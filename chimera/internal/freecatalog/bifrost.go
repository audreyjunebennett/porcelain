package freecatalog

import (
	"strings"
)

// ToGroqBiFrost maps a Groq docs "MODEL ID" string to a BiFrost unified id (always groq/...).
func ToGroqBiFrost(source string) string {
	s := strings.TrimSpace(source)
	if s == "" || strings.EqualFold(s, "MODEL ID") || strings.HasPrefix(s, "---") {
		return ""
	}
	if strings.HasPrefix(s, "groq/") {
		return s
	}
	return "groq/" + s
}

// ToGeminiBiFrost maps a Gemini API model id from pricing/docs to a BiFrost unified id.
func ToGeminiBiFrost(source string) string {
	s := strings.TrimSpace(source)
	if s == "" || !strings.HasPrefix(s, "gemini-") {
		return ""
	}
	if strings.HasPrefix(s, "gemini/") {
		return s
	}
	return "gemini/" + s
}
