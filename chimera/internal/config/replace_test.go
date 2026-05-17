package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReplaceFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "f.txt")
	if err := ReplaceFile(p, []byte("a"), 0o644); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(p)
	if err != nil || string(b) != "a" {
		t.Fatalf("got %q err %v", b, err)
	}
	if err := ReplaceFile(p, []byte("bb"), 0o644); err != nil {
		t.Fatal(err)
	}
	b, _ = os.ReadFile(p)
	if string(b) != "bb" {
		t.Fatalf("got %q", b)
	}
}

func TestCommitRoutingAndGateway_rollbackGateway(t *testing.T) {
	dir := t.TempDir()
	routePath := filepath.Join(dir, "routing-policy.yaml")
	gwPath := filepath.Join(dir, "gateway.yaml")
	if err := os.WriteFile(routePath, []byte("route-v1"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(gwPath, []byte("gw-v1"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(gwPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(gwPath, 0o755); err != nil {
		t.Fatal(err)
	}
	err := CommitRoutingAndGateway(routePath, []byte("route-v2"), 0o644, gwPath, []byte("gw-v2"), 0o644)
	if err == nil {
		t.Fatal("expected error when gateway path is not a writable file")
	}
	rb, _ := os.ReadFile(routePath)
	if string(rb) != "route-v1" {
		t.Fatalf("routing file not rolled back: %q", rb)
	}
}

func TestCommitRoutingAndGateway_success(t *testing.T) {
	dir := t.TempDir()
	routePath := filepath.Join(dir, "routing-policy.yaml")
	gwPath := filepath.Join(dir, "gateway.yaml")
	err := CommitRoutingAndGateway(routePath, []byte("r2"), 0o644, gwPath, []byte("g2"), 0o644)
	if err != nil {
		t.Fatal(err)
	}
	rb, _ := os.ReadFile(routePath)
	gb, _ := os.ReadFile(gwPath)
	if string(rb) != "r2" || string(gb) != "g2" {
		t.Fatalf("r=%q g=%q", rb, gb)
	}
}
