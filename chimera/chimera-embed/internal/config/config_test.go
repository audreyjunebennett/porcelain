package config

import (
	"strings"
	"testing"
)

func TestParseRequiresModelPath(t *testing.T) {
	t.Setenv("EMBED__MODEL_PATH", "")
	_, err := Parse([]string{"-model-path", ""}, BuildInfo{})
	if err == nil || !strings.Contains(err.Error(), "model path") {
		t.Fatalf("expected model path error, got %v", err)
	}
}

func TestParseEndpoint(t *testing.T) {
	host, port, err := ParseEndpoint("127.0.0.1:8090")
	if err != nil || host != "127.0.0.1" || port != 8090 {
		t.Fatalf("host=%q port=%d err=%v", host, port, err)
	}
}
