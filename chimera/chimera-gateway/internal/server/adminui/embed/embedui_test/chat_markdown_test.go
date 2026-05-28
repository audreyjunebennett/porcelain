package embedui_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/dop251/goja"
)

func chatUIPath(t *testing.T, rel ...string) string {
	t.Helper()
	base := filepath.Join(embeduiRoot(t), "chat")
	return filepath.Join(append([]string{base}, rel...)...)
}

func loadChatMarkdown(t *testing.T, vm *goja.Runtime) {
	t.Helper()
	evalJS(t, vm, uiEmbedPath(t, "util", "escape.js"))
	evalJS(t, vm, chatUIPath(t, "render", "markdown.js"))
	evalJS(t, vm, chatUIPath(t, "render", "snippet.js"))
}

func loadChatMessages(t *testing.T, vm *goja.Runtime) {
	t.Helper()
	loadChatMarkdown(t, vm)
	evalJS(t, vm, chatUIPath(t, "render", "messages.js"))
}

func TestChatMarkdown_closeOpenHtmlTags_closesUnclosedStrong(t *testing.T) {
	vm := goja.New()
	loadChatMarkdown(t, vm)

	out, err := vm.RunString(`
		ChimeraChat.Render.Markdown.closeOpenHtmlTags('<p><strong>partial')
	`)
	if err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.Contains(got, "</strong>") {
		t.Fatalf("expected closing strong tag, got %q", got)
	}
}

func TestChatMarkdown_renderPartial_closesTrailingBold(t *testing.T) {
	vm := goja.New()
	loadChatMarkdown(t, vm)

	out, err := vm.RunString(`
		ChimeraChat.Render.Markdown.renderPartial('**bold fragment')
	`)
	if err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.Contains(got, "<strong>") || !strings.Contains(got, "</strong>") {
		t.Fatalf("expected balanced strong tags, got %q", got)
	}
}

func TestChatMarkdown_renderPartial_closesOpenCodeFence(t *testing.T) {
	vm := goja.New()
	loadChatMarkdown(t, vm)

	out, err := vm.RunString("ChimeraChat.Render.Markdown.renderPartial(\"before\\n```go\\nfmt.Println(\\\"hi\\\")\")")
	if err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.Contains(got, "<pre><code>") || !strings.Contains(got, "</code></pre>") {
		t.Fatalf("expected closed code fence, got %q", got)
	}
}

func TestChatSnippet_render_doesNotBleedIntoNextSnippet(t *testing.T) {
	vm := goja.New()
	loadChatMarkdown(t, vm)

	out, err := vm.RunString(`
		(function () {
			var a = ChimeraChat.Render.Snippet.render('doc.md', '**partial bold');
			var b = ChimeraChat.Render.Snippet.render('doc.md', 'normal text');
			return a + b;
		})()
	`)
	if err != nil {
		t.Fatal(err)
	}
	got := out.String()
	strongOpens := strings.Count(got, "<strong>")
	strongCloses := strings.Count(got, "</strong>")
	if strongOpens != strongCloses {
		t.Fatalf("unbalanced strong tags: opens=%d closes=%d html=%q", strongOpens, strongCloses, got)
	}
	if !strings.Contains(got, "normal text") {
		t.Fatalf("expected second snippet text to render, got %q", got)
	}
}

func TestChatMarkdown_renderSafe_preservesCompleteMarkdown(t *testing.T) {
	vm := goja.New()
	loadChatMarkdown(t, vm)

	out, err := vm.RunString(`
		ChimeraChat.Render.Markdown.renderSafe('**bold** and ` + "`code`" + `')
	`)
	if err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.Contains(got, "<strong>bold</strong>") || !strings.Contains(got, "<code>code</code>") {
		t.Fatalf("expected normal markdown rendering, got %q", got)
	}
}

func TestChatMessages_renderMessage_doesNotBleedIntoFooter(t *testing.T) {
	vm := goja.New()
	loadChatMessages(t, vm)

	out, err := vm.RunString(`
		ChimeraChat.Render.Messages.renderMessage({
			id: "m1",
			role: "assistant",
			status: "streaming",
			content: "**partial bold",
			ragHits: [{ source: "doc.md", text: "snippet body", score: 0.5 }]
		})
	`)
	if err != nil {
		t.Fatal(err)
	}
	got := out.String()
	strongOpens := strings.Count(got, "<strong>")
	strongCloses := strings.Count(got, "</strong>")
	if strongOpens != strongCloses {
		t.Fatalf("unbalanced strong tags in message html: opens=%d closes=%d", strongOpens, strongCloses)
	}
	if !strings.Contains(got, "Workspace Snippets") {
		t.Fatalf("expected footer in message html, got %q", got)
	}
	if !strings.Contains(got, "readiness_score") {
		t.Fatalf("expected readiness_score icon in snippet summary, got %q", got)
	}
	footerIdx := strings.Index(got, "chat-msg__footer")
	footer := got[footerIdx:]
	if strings.Contains(footer, "<strong>") {
		t.Fatalf("footer should not inherit unclosed formatting, got %q", footer)
	}
}
