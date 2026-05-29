package embedui_test

import (
	"strings"
	"testing"
)

func TestIndexHTML_includesNavRibbon(t *testing.T) {
	html := mustReadFile(t, embeduiRoot(t)+"/index.html")
	for _, needle := range []string{
		`id="shell-ribbon"`,
		`class="shell-ribbon"`,
		`data-ribbon-action="toggle"`,
		`data-ribbon-action="new-chat"`,
		`data-ribbon-action="settings"`,
		`id="chat-history"`,
		`id="shell-ribbon-filters"`,
		`class="shell-ribbon__footer"`,
		"side_navigation",
		"Porcelain",
		"historyClient.js",
		"historyPanel.js",
		"navRibbon.js",
		"shell-ribbon.css",
		"getShellReturnConversationId",
		"setShellReturnConversationId",
	} {
		if !strings.Contains(html, needle) {
			t.Fatalf("index.html missing %q", needle)
		}
	}
	for _, forbidden := range []string{
		`id="shell-top"`,
		`id="btn-reload"`,
		`id="shell-chat-copy-all"`,
		"shell.css",
	} {
		if strings.Contains(html, forbidden) {
			t.Fatalf("index.html should not include %q", forbidden)
		}
	}
}

func TestChatHTML_noEmbeddedHistoryPanel(t *testing.T) {
	html := mustReadFile(t, embeduiRoot(t)+"/chat.html")
	for _, forbidden := range []string{
		`id="chat-history"`,
		"historyPanel.js",
	} {
		if strings.Contains(html, forbidden) {
			t.Fatalf("chat.html should not include %q (history panel lives in shell ribbon)", forbidden)
		}
	}
	if !strings.Contains(html, "historyClient.js") {
		t.Fatal("chat.html must include historyClient.js for opening conversations")
	}
}
