package config

import (
	"os"
	"path/filepath"
	"testing"
)

func FuzzLoadGatewayYAML(f *testing.F) {
	seed := []byte(`gateway:
  semver: "0.1.0"
broker:
  base_url: "http://127.0.0.1:8080"
paths:
  api_keys: "./api-keys.yaml"
`)
	f.Add(seed)
	f.Fuzz(func(t *testing.T, data []byte) {
		dir := t.TempDir()
		p := filepath.Join(dir, "gateway.yaml")
		if err := os.WriteFile(p, data, 0o644); err != nil {
			t.Fatal(err)
		}
		_, _ = LoadGatewayYAML(p, nil)
	})
}
