package vectorstore

import (
	"strings"
	"testing"
)

func TestCollectionName_Deterministic(t *testing.T) {
	c := Coords{TenantID: "Acme Co.", ProjectID: "My App", FlavorID: "main"}
	a := CollectionName(c)
	b := CollectionName(c)
	if a != b {
		t.Fatalf("non-deterministic: %q vs %q", a, b)
	}
	if !strings.HasPrefix(a, "chimera-acme-co-my-app-main-") {
		t.Fatalf("unexpected name: %q", a)
	}
}

func TestCollectionName_DifferentInputsDiffer(t *testing.T) {
	a := CollectionName(Coords{TenantID: "t1", ProjectID: "p"})
	b := CollectionName(Coords{TenantID: "t2", ProjectID: "p"})
	if a == b {
		t.Fatalf("collision: %q", a)
	}
}

func TestCollectionName_EmptyFlavorAllowed(t *testing.T) {
	a := CollectionName(Coords{TenantID: "t", ProjectID: "p"})
	if a == "" {
		t.Fatal("empty name")
	}
	if !strings.Contains(a, "-_-") {
		t.Fatalf("expected placeholder for empty flavor, got %q", a)
	}
}

func TestCollectionName_SameSlugCollidingInputsHashDiffers(t *testing.T) {
	a := CollectionName(Coords{TenantID: "Foo Bar", ProjectID: "p"})
	b := CollectionName(Coords{TenantID: "foo-bar", ProjectID: "p"})
	if a == b {
		t.Fatalf("hash suffix should disambiguate: %q", a)
	}
}

func TestPointID_DeterministicAndUUIDShape(t *testing.T) {
	c := Coords{TenantID: "t", ProjectID: "p", FlavorID: "f"}
	a := PointID(c, "src/main.go", 0)
	b := PointID(c, "src/main.go", 0)
	if a != b {
		t.Fatalf("non-deterministic id")
	}
	if d := PointID(c, "src/main.go", 1); d == a {
		t.Fatalf("chunk index should affect id")
	}
	parts := strings.Split(a, "-")
	if len(parts) != 5 || len(parts[0]) != 8 || len(parts[1]) != 4 || len(parts[2]) != 4 || len(parts[3]) != 4 || len(parts[4]) != 12 {
		t.Fatalf("not UUID shape: %q", a)
	}
}
