package freecatalog

import (
	"strings"

	"golang.org/x/net/html"
)

// groqHTMLDataMinCols matches the live docs table (MODEL ID + 6 limit columns). Shorter tables
// (e.g. “Rate limit headers” with 3 columns) must not be mistaken for model rows.
const groqHTMLDataMinCols = 7

func parseGroqFromHTMLTable(body string) []GroqDocRow {
	doc, err := html.Parse(strings.NewReader(body))
	if err != nil {
		return nil
	}
	var best []GroqDocRow
	for _, tbl := range allTablesPreorder(doc) {
		rows := groqDataRowsFromHTMLTable(tbl)
		if len(rows) > len(best) {
			best = rows
		}
	}
	return best
}

func allTablesPreorder(n *html.Node) []*html.Node {
	var out []*html.Node
	if n.Type == html.ElementNode && n.Data == "table" {
		out = append(out, n)
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		out = append(out, allTablesPreorder(c)...)
	}
	return out
}

func groqDataRowsFromHTMLTable(tbl *html.Node) []GroqDocRow {
	seen := make(map[string]struct{})
	var out []GroqDocRow
	for _, tr := range tableBodyRows(tbl) {
		var cells []string
		for _, td := range childElements(tr, "td") {
			cells = append(cells, nodePlainText(td))
		}
		row, ok := groqRowFromCells(cells, groqHTMLDataMinCols)
		if !ok {
			continue
		}
		if _, dup := seen[row.SourceID]; dup {
			continue
		}
		seen[row.SourceID] = struct{}{}
		out = append(out, row)
	}
	return out
}

func tableBodyRows(table *html.Node) []*html.Node {
	tbody := firstChildElement(table, "tbody")
	if tbody != nil {
		return childElements(tbody, "tr")
	}
	var rows []*html.Node
	for c := table.FirstChild; c != nil; c = c.NextSibling {
		if c.Type != html.ElementNode || c.Data != "tr" {
			continue
		}
		if len(childElements(c, "th")) > 0 {
			continue
		}
		rows = append(rows, c)
	}
	return rows
}

func firstChildElement(n *html.Node, tag string) *html.Node {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode && c.Data == tag {
			return c
		}
	}
	return nil
}

func childElements(n *html.Node, tag string) []*html.Node {
	var out []*html.Node
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.ElementNode && c.Data == tag {
			out = append(out, c)
		}
	}
	return out
}

func nodePlainText(n *html.Node) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			b.WriteString(n.Data)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return strings.Join(strings.Fields(b.String()), " ")
}
