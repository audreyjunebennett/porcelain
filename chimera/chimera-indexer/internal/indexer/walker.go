package indexer

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Candidate is a file discovered by Walk that the caller should consider
// ingesting. Path is forward-slash, root-relative.
type Candidate struct {
	Root    Root
	RelPath string
	AbsPath string
	Size    int64
}

// WalkOptions controls Walk behavior. The v0.2 indexer always sets
// FollowSymlinks=false and provides binary-detection knobs from Resolved.
type WalkOptions struct {
	Matcher              *Matcher
	MaxFileBytes         int64
	BinaryNullByteSample int
	BinaryNullByteRatio  float64
	OnSkip               func(rel, reason string)
}

// Walk enumerates files under root that pass ignore + binary checks. It does
// not follow symlinks. Errors from individual files are reported via OnSkip
// and do not stop the walk.
func Walk(root Root, opts WalkOptions) ([]Candidate, error) {
	var out []Candidate
	skip := opts.OnSkip
	if skip == nil {
		skip = func(string, string) {}
	}
	err := filepath.WalkDir(root.AbsPath, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			skip(relOrEmpty(root.AbsPath, p), "walk error: "+err.Error())
			if d != nil && d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.Type()&os.ModeSymlink != 0 {
			return nil // never follow symlinks in v0.2
		}
		rel, ok := relPath(root.AbsPath, p)
		if !ok {
			return nil
		}
		if rel == "." {
			return nil
		}
		if d.IsDir() {
			if opts.Matcher != nil && opts.Matcher.Match(rel+"/") {
				skip(rel, "ignored dir")
				return filepath.SkipDir
			}
			return nil
		}
		if opts.Matcher != nil && opts.Matcher.Match(rel) {
			skip(rel, "ignored file")
			return nil
		}
		info, err := d.Info()
		if err != nil {
			skip(rel, "stat: "+err.Error())
			return nil
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		if opts.MaxFileBytes > 0 && info.Size() > opts.MaxFileBytes {
			skip(rel, "exceeds max_file_bytes")
			return nil
		}
		bin, err := IsBinaryFile(p, opts.BinaryNullByteSample, opts.BinaryNullByteRatio)
		if err != nil {
			skip(rel, "open: "+err.Error())
			return nil
		}
		if bin {
			skip(rel, "binary")
			return nil
		}
		out = append(out, Candidate{Root: root, RelPath: rel, AbsPath: p, Size: info.Size()})
		return nil
	})
	return out, err
}

func relPath(root, p string) (string, bool) {
	rel, err := filepath.Rel(root, p)
	if err != nil {
		return "", false
	}
	rel = filepath.ToSlash(rel)
	if rel == "" || strings.HasPrefix(rel, "..") {
		return "", false
	}
	return rel, true
}

func relOrEmpty(root, p string) string {
	if r, ok := relPath(root, p); ok {
		return r
	}
	return ""
}
