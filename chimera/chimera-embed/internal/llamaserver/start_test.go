package llamaserver

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestCommandBuildsEmbeddingServerArgs(t *testing.T) {
	dir := t.TempDir()
	model := filepath.Join(dir, "model.gguf")
	if err := os.WriteFile(model, []byte("fake"), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd, err := Command(Config{Bin: "llama-server", ModelPath: model, CacheDir: filepath.Join(dir, "cache"), BindHost: "127.0.0.1", Port: 18090, CtxSize: 512, NGPULayers: 4, Pooling: "mean"})
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"--embedding", "--metrics", "--pooling", "mean", "--port", "18090"} {
		if !slices.Contains(cmd.Args, want) {
			t.Fatalf("args missing %q: %v", want, cmd.Args)
		}
	}
}

func TestCommandRequiresModelFile(t *testing.T) {
	_, err := Command(Config{Bin: "llama-server", ModelPath: filepath.Join(t.TempDir(), "missing.gguf"), BindHost: "127.0.0.1", Port: 18090})
	if err == nil {
		t.Fatal("expected missing model error")
	}
}
