package gencontracts

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/lynn/porcelain/internal/naming"
)

// DefaultContractsJSPath is the embed path for operator settings contracts (repo-relative).
const DefaultContractsJSPath = "chimera/chimera-gateway/internal/server/adminui/embed/embedui/settings/contracts.js"

// WriteContractsJS emits adminui embed contracts.js from internal/naming metadata.
func WriteContractsJS(w io.Writer) error {
	var b strings.Builder
	b.WriteString("/**\n")
	b.WriteString(" * Operator settings UI constants — generated from internal/naming (gateway_logs.go, contracts.go, logs_ui.go).\n")
	b.WriteString(" * DO NOT EDIT; run: make operator-contracts-generate\n")
	b.WriteString(" */\n")
	b.WriteString("(function () {\n")
	b.WriteString("  globalThis.ChimeraSettings = globalThis.ChimeraSettings || {};\n")
	b.WriteString("  globalThis.ChimeraSettings.Contracts = {\n")

	writeConsts := func(items []naming.LogsUIStringConst) {
		for _, c := range items {
			fmt.Fprintf(&b, "    %s: %s,\n", c.JSName, strconv.Quote(c.Value))
		}
	}

	writeConsts(naming.LogsUIProductNames)
	b.WriteString("\n")
	writeConsts(naming.LogsUILogSources)
	b.WriteString("\n")
	writeConsts(naming.LogsUITimelineKinds)

	b.WriteString("\n    /** Request-timeline bar keys (product display names). */\n")
	b.WriteString("    TimelineBarKinds: [\n")
	for _, bar := range naming.LogsUITimelineBarKinds {
		fmt.Fprintf(&b, "      { key: %s, label: %s },\n", strconv.Quote(bar.Key), strconv.Quote(bar.Label))
	}
	b.WriteString("    ],\n\n")

	b.WriteString("    serviceBadgeClass: function (productKey) {\n")
	b.WriteString("      var k = String(productKey || \"\").toLowerCase();\n")
	for _, rule := range naming.LogsUIServiceBadgeRules {
		conds := make([]string, len(rule.Keys))
		for i, key := range rule.Keys {
			conds[i] = fmt.Sprintf("k === %s", strconv.Quote(strings.ToLower(key)))
		}
		fmt.Fprintf(&b, "      if (%s) return %q;\n", strings.Join(conds, " || "), rule.Class)
	}
	fmt.Fprintf(&b, "      return %q;\n", naming.LogsUIServiceBadgeDefault)
	b.WriteString("    }\n")
	b.WriteString("  };\n")
	b.WriteString("})();\n")

	_, err := io.WriteString(w, b.String())
	return err
}
