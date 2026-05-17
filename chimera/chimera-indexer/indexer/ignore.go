package indexer

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	gitignore "github.com/sabhiram/go-gitignore"
)

// Default .chimeraignore patterns. These follow the indexer plan's "sensible
// defaults" guidance and explicitly exclude common secret-bearing files,
// build outputs, and large binary directories.
var defaultChimeraIgnore = []string{
	".git/",
	".hg/",
	".svn/",
	".gitignore",
	".gitattributes",
	".gitmodules",
	".chimeraignore",
	".dockerignore",
	".editorconfig",
	"node_modules/",
	"vendor/",
	"target/",
	"dist/",
	"build/",
	".venv/",
	"venv/",
	"__pycache__/",
	".env",
	".env.*",
	"*.key",
	"*.pem",
	"*.lock",
	"*.log",
	"*.bin",
	"*.exe",
	"*.dll",
	"*.so",
	"*.dylib",
	"*.zip",
	"*.tar",
	"*.tar.gz",
	"*.tgz",
	"*.7z",
	"*.png",
	"*.jpg",
	"*.jpeg",
	"*.gif",
	"*.webp",
	"*.pdf",
	"*.mp4",
	"*.mov",
	"*.mp3",
	"*.wasm",
	"*.parquet",
	"*.sqlite",
	"*.sqlite3",
	"*.db",
}

// Matcher answers ignore decisions for files relative to a root path. It
// composes (in order):
//  1. Default .chimeraignore patterns.
//  2. Caller-supplied extra patterns.
//  3. Patterns parsed from <root>/.chimeraignore.
//  4. Patterns parsed from <root>/.gitignore.
type Matcher struct {
	matchers []*gitignore.GitIgnore
}

// NewMatcher loads ignore patterns rooted at root. extra is applied as
// additional patterns (appended after defaults). Missing dotfiles are
// ignored; any other read or parse error is returned.
func NewMatcher(root string, extra []string) (*Matcher, error) {
	m := &Matcher{}
	m.matchers = append(m.matchers, gitignore.CompileIgnoreLines(defaultChimeraIgnore...))
	if len(extra) > 0 {
		m.matchers = append(m.matchers, gitignore.CompileIgnoreLines(extra...))
	}
	for _, name := range []string{".chimeraignore", ".gitignore"} {
		p := filepath.Join(root, name)
		gi, err := loadGitignore(p)
		if err != nil {
			return nil, err
		}
		if gi != nil {
			m.matchers = append(m.matchers, gi)
		}
	}
	return m, nil
}

func loadGitignore(path string) (*gitignore.GitIgnore, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	lines := strings.Split(string(b), "\n")
	return gitignore.CompileIgnoreLines(lines...), nil
}

// Match reports whether a path (relative to the root, forward-slash form)
// matches any configured ignore pattern. Directory paths should end in "/".
func (m *Matcher) Match(rel string) bool {
	if m == nil {
		return false
	}
	rel = filepath.ToSlash(rel)
	for _, gi := range m.matchers {
		if gi != nil && gi.MatchesPath(rel) {
			return true
		}
	}
	return false
}

// looksBinary returns true if the head of buf appears to be non-text. It
// flags anything containing a NUL byte in the inspected window or a high
// fraction of non-printable, non-whitespace bytes.
func looksBinary(buf []byte, ratio float64) bool {
	if len(buf) == 0 {
		return false
	}
	if bytes.IndexByte(buf, 0x00) >= 0 {
		return true
	}
	if ratio <= 0 {
		return false
	}
	bad := 0
	for _, b := range buf {
		switch {
		case b == 0x09, b == 0x0a, b == 0x0d:
		case b >= 0x20 && b < 0x7f:
		case b >= 0x80:
		default:
			bad++
		}
	}
	return float64(bad)/float64(len(buf)) > ratio
}

// IsBinaryFile reads up to sampleBytes from the file and applies looksBinary.
func IsBinaryFile(path string, sampleBytes int, ratio float64) (bool, error) {
	if sampleBytes <= 0 {
		sampleBytes = 8000
	}
	f, err := os.Open(path)
	if err != nil {
		return false, err
	}
	defer f.Close()
	buf := make([]byte, sampleBytes)
	n, err := f.Read(buf)
	if err != nil && !errors.Is(err, fs.ErrClosed) && err.Error() != "EOF" {
		// Allow EOF (short files); other errors propagate.
		if !isEOF(err) {
			return false, err
		}
	}
	return looksBinary(buf[:n], ratio), nil
}

func isEOF(err error) bool {
	return err != nil && err.Error() == "EOF"
}
