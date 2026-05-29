package embedui_test

import (
	"strings"
	"testing"

	"github.com/dop251/goja"
)

func TestChatState_autoTitleFromMessage(t *testing.T) {
	vm := goja.New()
	loadChatState(t, vm)
	short, err := vm.RunString(`ChimeraChat.State.autoTitleFromMessage("hello world")`)
	if err != nil {
		t.Fatal(err)
	}
	if short.String() != "hello world" {
		t.Fatalf("short=%q", short)
	}
	longText := strings.Repeat("a", 100)
	long, err := vm.RunString(`ChimeraChat.State.autoTitleFromMessage("` + longText + `")`)
	if err != nil {
		t.Fatal(err)
	}
	got := long.String()
	if len([]rune(got)) != 83 {
		t.Fatalf("want 80 chars + ..., got len=%d %q", len([]rune(got)), got)
	}
	if !strings.HasSuffix(got, "...") {
		t.Fatalf("missing suffix: %q", got)
	}

	punct, err := vm.RunString(`ChimeraChat.State.autoTitleFromMessage("Hello, world and more")`)
	if err != nil {
		t.Fatal(err)
	}
	if punct.String() != "Hello," {
		t.Fatalf("punct=%q", punct)
	}
}

func TestChatHTML_includesTitleBar(t *testing.T) {
	html := mustReadFile(t, embeduiRoot(t)+"/chat.html")
	if !strings.Contains(html, `id="chat-title-bar"`) {
		t.Fatal("missing chat-title-bar host")
	}
	if !strings.Contains(html, "render/titleBar.js") {
		t.Fatal("missing titleBar.js script")
	}
}
