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
	navRibbon := mustReadFile(t, embeduiRoot(t)+"/shell/navRibbon.js")
	for _, needle := range []string{"NARROW_BREAKPOINT", "shell-ribbon--narrow", "applyViewportRibbonLayout"} {
		if !strings.Contains(navRibbon, needle) {
			t.Fatalf("navRibbon.js missing %q", needle)
		}
	}
	chatCSS := mustReadFile(t, embeduiRoot(t)+"/styles/chat.css")
	if !strings.Contains(chatCSS, "embed-mobile.css") {
		t.Fatal("chat.css must import embed-mobile.css (shell loads chat.css on index.html)")
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
	css := mustReadFile(t, embeduiRoot(t)+"/styles/chat.css")
	if !strings.Contains(css, "embed-mobile.css") {
		t.Fatal("chat.css must import embed-mobile.css for phone-width layout")
	}
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

func TestSettingsMobileCSS_summaryGridSameWhenOpen(t *testing.T) {
	css := mustReadFile(t, embeduiRoot(t)+"/styles/settings-mobile.css")
	design := mustReadFile(t, embeduiRoot(t)+"/styles/design-01.css")
	if strings.Contains(css, ":not([open])") {
		t.Fatal("settings-mobile.css must not gate summary grid on :not([open]); open cards use the same header layout")
	}
	if !strings.Contains(design, "@media (min-width: 481px)") || !strings.Contains(design, "grid-template-columns: auto minmax(0, 1fr) auto 1rem") {
		t.Fatal("design-01.css must scope desktop 4-column summary grid to min-width 481px")
	}
	for _, needle := range []string{
		`grid-template-areas`,
		`"avatar title"`,
		`grid-template-columns: 1.75rem minmax(0, 1fr)`,
		`:not(.sg-op-user-saved) > summary > .sum-status`,
		`justify-content: flex-start`,
		`max-width: 100%`,
	} {
		if !strings.Contains(css, needle) {
			t.Fatalf("settings-mobile.css missing %q", needle)
		}
	}
}
