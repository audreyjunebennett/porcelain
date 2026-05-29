package embedui_test

import (
	"testing"

	"github.com/dop251/goja"
)

func loadChatState(t *testing.T, vm *goja.Runtime) {
	t.Helper()
	evalJS(t, vm, embeduiRoot(t)+"/chat/state.js")
}

func TestChatState_newConversationId(t *testing.T) {
	vm := goja.New()
	loadChatState(t, vm)
	id, err := vm.RunString("ChimeraChat.State.newConversationId()")
	if err != nil {
		t.Fatal(err)
	}
	s := id.String()
	if len(s) < 8 || len(s) > 128 {
		t.Fatalf("id length out of range: %q", s)
	}
	for _, c := range s {
		ok := (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') ||
			c == '-' || c == '_' || c == '.'
		if !ok {
			t.Fatalf("invalid char in id %q", s)
		}
	}
	id2, err := vm.RunString("ChimeraChat.State.newConversationId()")
	if err != nil {
		t.Fatal(err)
	}
	if id2.String() == s {
		t.Fatal("expected unique conversation ids")
	}
}
