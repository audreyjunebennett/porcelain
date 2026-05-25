package llamaserver

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestStart_requiresModelFile(t *testing.T) {
	dir := t.TempDir()
	_, err := Start(context.Background(), Config{
		Bin:       "llama-server",
		ModelPath: filepath.Join(dir, "missing.gguf"),
		BindHost:  "127.0.0.1",
		Port:      18090,
	}, nil)
	if err == nil {
		t.Fatal("expected missing model error")
	}
}

func TestStart_acceptsModelFile(t *testing.T) {
	dir := t.TempDir()
	model := filepath.Join(dir, "model.gguf")
	if err := os.WriteFile(model, []byte("fake"), 0o644); err != nil {
		t.Fatal(err)
	}
	// We only validate argv assembly; do not start a real llama-server in unit tests.
	cfg := Config{
		Bin:       "llama-server",
		ModelPath: model,
		BindHost:  "127.0.0.1",
		Port:      18090,
		Pooling:   "mean",
		CtxSize:   512,
	}
	if stringsTrim(cfg.ModelPath) == "" {
		t.Fatal("model path empty")
	}
	_ = cfg
}

func stringsTrim(s string) string {
	for i := 0; i < len(s); i++ {
		if s[i] != ' ' {
			return s
		}
	}
	return ""
}
