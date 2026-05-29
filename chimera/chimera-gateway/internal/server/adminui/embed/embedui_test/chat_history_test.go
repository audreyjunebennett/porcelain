package embedui_test

import (
	"strings"
	"testing"
)

func TestChatHTML_layoutIsMainOnly(t *testing.T) {
	html := mustReadFile(t, embeduiRoot(t)+"/chat.html")
	if !strings.Contains(html, `class="chat-main"`) {
		t.Fatal("chat.html missing chat-main")
	}
	if strings.Contains(html, `class="chat-history"`) {
		t.Fatal("chat.html should not include embedded chat-history sidebar")
	}
	if !strings.Contains(html, "historyClient.js") {
		t.Fatal("chat.html must load historyClient.js to open saved conversations")
	}
}
