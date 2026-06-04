package embedui_test

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

// cardMountFiles are Phase 4 feed-log card modules using mount*(ctx) IIFEs.
var cardMountFiles = []string{
	"serviceFeed.js",
	"feedLogConv.js",
	"indexerRun.js",
	"indexerWorkspace.js",
}

// TestCardMount_noVarShadowsLocalFunction fails when extraction left both
// `var name = ctx.name` and `function name` in the same mount closure (var assignment
// overwrites the hoisted function with undefined).
func TestCardMount_noVarShadowsLocalFunction(t *testing.T) {
	t.Helper()
	fnPat := regexp.MustCompile(`(?m)^  function ([A-Za-z_$][\w$]*)\s*\(`)
	varPat := regexp.MustCompile(`(?m)^  var ([A-Za-z_$][\w$]*) = ctx\.[A-Za-z_$][\w$]*\s*;`)

	for _, file := range cardMountFiles {
		path := cardsUIPath(t, file)
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		functions := make(map[string]bool)
		for _, m := range fnPat.FindAllStringSubmatch(string(body), -1) {
			if len(m) > 1 {
				functions[m[1]] = true
			}
		}
		for _, m := range varPat.FindAllStringSubmatch(string(body), -1) {
			if len(m) < 2 {
				continue
			}
			name := m[1]
			if functions[name] {
				t.Errorf("%s: var %s = ctx.%s shadows function %s in the same mount — remove the var line",
					filepath.Base(path), name, name, name)
			}
		}
	}
}
