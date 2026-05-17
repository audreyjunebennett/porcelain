package routing

import (
	"testing"

	"github.com/lynn/porcelain/chimera/internal/routinggen"
)

func TestValidatePolicyYAML_generated(t *testing.T) {
	b, err := routinggen.BuildRoutingPolicyYAML([]string{"groq/fast", "ollama/local"})
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidatePolicyYAML(b); err != nil {
		t.Fatal(err)
	}
}

func TestValidatePolicyYAML_rejectsEmpty(t *testing.T) {
	if err := ValidatePolicyYAML([]byte("rules: []\nambiguous_default_model: \"\"\n")); err == nil {
		t.Fatal("expected error")
	}
	if err := ValidatePolicyYAML([]byte("")); err == nil {
		t.Fatal("expected error")
	}
	if err := ValidatePolicyYAML([]byte("not: yaml: :")); err == nil {
		t.Fatal("expected error")
	}
}

func TestValidatePolicyYAML_withPreambleComment(t *testing.T) {
	y := "# hi\nambiguous_default_model: \"x\"\nrules:\n  - name: d\n    when: {}\n    models:\n      - \"x\"\n"
	if err := ValidatePolicyYAML([]byte(y)); err != nil {
		t.Fatal(err)
	}
}
