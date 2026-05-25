package config

import (
	"strings"
	"testing"
)

func TestParse_requiresModelPath(t *testing.T) {
	_, err := Parse([]string{"-model-path", ""}, BuildInfo{})
	if err == nil || !strings.Contains(err.Error(), "model path") {
		t.Fatalf("expected model path error, got %v", err)
	}
}

func TestParseEndpoint(t *testing.T) {
	host, port, err := ParseEndpoint("127.0.0.1:8090")
	if err != nil || host != "127.0.0.1" || port != 8090 {
		t.Fatalf("endpoint: host=%q port=%d err=%v", host, port, err)
	}
}
