package page

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/lynn/porcelain/internal/locus"
)

// UnreachableDataURL is the startup failure page shown in the webview.
func UnreachableDataURL(baseURL, reason string, owned bool) string {
	mode := "attached to existing supervisor"
	if owned {
		mode = "started by this desktop instance"
	}
	html := fmt.Sprintf(`<!doctype html><html><head><meta charset="utf-8"><title>Cannot connect to supervisor</title>`+
		`<style>body{font-family:Segoe UI,Arial,sans-serif;background:#0f1115;color:#e7e9ee;margin:0;padding:24px;}`+
		`h1{margin:0 0 8px 0;font-size:22px;}p{margin:8px 0;line-height:1.45;}`+
		`code{background:#1b1f29;padding:2px 6px;border-radius:4px;} .box{border:1px solid #2f3545;border-radius:8px;padding:14px;margin-top:14px;background:#151925;}</style></head><body>`+
		`<h1>Cannot connect to supervisor</h1>`+
		`<p>Locus desktop could not establish a healthy connection to <code>%s</code>.</p>`+
		`<div class="box"><p><strong>Detail:</strong> %s</p><p><strong>Ownership:</strong> %s</p></div>`+
		`<div class="box"><p><strong>Try:</strong></p>`+
		`<p>1) Verify <code>`+locus.BinSupervisor+`</code>, <code>`+locus.BinBroker+`</code>, and <code>`+locus.BinVectorstore+`</code> binaries exist in runtime paths.</p>`+
		`<p>2) Check for port conflicts on 3000, 6333, 6334, 7710, 7720, 7730, 7740, 7750, and 8080.</p>`+
		`<p>3) Relaunch desktop after stopping stale local runtime processes.</p></div></body></html>`,
		escapeHTML(baseURL), escapeHTML(reason), escapeHTML(mode))
	return "data:text/html;charset=utf-8," + url.PathEscape(html)
}

// RuntimeLossDataURL is a short page when supervisor health is lost after startup.
func RuntimeLossDataURL(baseURL, reason string) string {
	html := fmt.Sprintf(`<!doctype html><html><head><meta charset="utf-8"><title>Supervisor connection lost</title>`+
		`<style>body{font-family:Segoe UI,Arial,sans-serif;background:#0f1115;color:#e7e9ee;margin:24px;}`+
		`h1{font-size:20px;}p{line-height:1.45;}code{background:#1b1f29;padding:2px 6px;border-radius:4px;}</style></head><body>`+
		`<h1>Supervisor connection lost</h1><p>%s</p><p>Endpoint: <code>%s</code></p>`+
		`<p>Close and relaunch Locus desktop after the runtime is healthy again.</p></body></html>`,
		escapeHTML(reason), escapeHTML(baseURL))
	return "data:text/html;charset=utf-8," + url.PathEscape(html)
}

func escapeHTML(s string) string {
	r := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		"\"", "&quot;",
		"'", "&#39;",
	)
	return r.Replace(s)
}
