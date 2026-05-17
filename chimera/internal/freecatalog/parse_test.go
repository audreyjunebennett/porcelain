package freecatalog

import "testing"

const groqFixture = `
## Rate Limits

| MODEL ID | RPM | RPD |
| --- | --- | --- |
| llama-3.1-8b-instant | 30 | 14.4K |
| openai/gpt-oss-20b | 30 | 1K |
| MODEL ID | 1 | 2 |
`

func TestParseGroqRateLimitPage(t *testing.T) {
	got := ParseGroqRateLimitPage(groqFixture)
	if len(got) != 2 || got[0] != "llama-3.1-8b-instant" || got[1] != "openai/gpt-oss-20b" {
		t.Fatalf("%#v", got)
	}
}

func TestParseGroqRateLimitRows_sevenColumns(t *testing.T) {
	const page = `
| MODEL ID | RPM | RPD | TPM | TPD | ASH | ASD |
| --- | --- | --- | --- | --- | --- | --- |
| groq/compound | 30 | 250 | 70K | - | - | - |
| whisper-large-v3 | 20 | 2K | - | - | 7.2K | 28.8K |
`
	rows := ParseGroqRateLimitRows(page)
	if len(rows) != 2 {
		t.Fatalf("got %#v", rows)
	}
	if rows[0].SourceID != "groq/compound" || rows[0].RPM != "30" || rows[0].RPD != "250" || rows[0].TPM != "70K" || rows[0].TPD != "-" || rows[0].ASH != "-" || rows[0].ASD != "-" {
		t.Fatalf("compound row %#v", rows[0])
	}
	if rows[1].SourceID != "whisper-large-v3" || rows[1].ASH != "7.2K" || rows[1].ASD != "28.8K" {
		t.Fatalf("whisper row %#v", rows[1])
	}
}

func TestParseGroqRateLimitRows_groqHTMLTable(t *testing.T) {
	const page = `<table><thead><tr>
<th>MODEL ID</th><th>RPM</th><th>RPD</th><th>TPM</th><th>TPD</th><th>ASH</th><th>ASD</th>
</tr></thead><tbody><tr>
<td><div>groq/compound</div></td><td>30</td><td>250</td><td>70K</td><td>-</td><td>-</td><td>-</td>
</tr></tbody></table>`
	rows := ParseGroqRateLimitRows(page)
	if len(rows) != 1 || rows[0].SourceID != "groq/compound" || rows[0].TPM != "70K" {
		t.Fatalf("got %#v", rows)
	}
}

func TestParseGroqRateLimitRows_skipsHeaderTable(t *testing.T) {
	const page = `
| MODEL ID | RPM | RPD | TPM | TPD | ASH | ASD |
| meta/x | 30 | 1K | 1K | 1K | - | - |
## Rate Limit Headers
| retry-after | 2 | seconds |
| x-ratelimit-limit-requests | 14400 | x |
`
	rows := ParseGroqRateLimitRows(page)
	if len(rows) != 1 || rows[0].SourceID != "meta/x" {
		t.Fatalf("got %#v", rows)
	}
}

func TestParseGeminiPricingFreeInputModels(t *testing.T) {
	sec := "## Gemini 3 Flash Preview\n\n_`gemini-3-flash-preview`_\n\n### Standard\n\n" +
		"| Free Tier | Paid Tier |\n| Input price | Free of charge | $1.00 |\n"
	body := "preamble\n" + sec
	got := ParseGeminiPricingFreeInputModels(body)
	if len(got) != 1 || got[0] != "gemini-3-flash-preview" {
		t.Fatalf("%#v", got)
	}
}

func TestParseGeminiPricingFreeInputModels_html(t *testing.T) {
	const sec = `<h2>Gemini 2.0 Flash</h2>
<em><code>gemini-2.0-flash</code></em>
<table class="pricing-table"><tr><td>Input price</td><td>Free of charge</td><td>$0.10</td></tr></table>`
	got := ParseGeminiPricingFreeInputModels(sec)
	if len(got) != 1 || got[0] != "gemini-2.0-flash" {
		t.Fatalf("%#v", got)
	}
}

func TestParseGeminiSkipsNoFreeInput(t *testing.T) {
	sec := "## Gemini 3.1 Pro Preview\n\n_`gemini-3.1-pro-preview`_\n\n" +
		"| Free Tier | Paid |\n| Input price | Not available | $2 |\n"
	got := ParseGeminiPricingFreeInputModels(sec)
	if len(got) != 0 {
		t.Fatalf("expected skip, got %#v", got)
	}
}
