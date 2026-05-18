// Package binfind resolves sibling binaries next to the running executable.
package binfind

import (
	"os"
	"path/filepath"
	"runtime"
)

// ExecutableDir returns the directory containing the current binary, or "" on error.
func ExecutableDir() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	return filepath.Dir(exe)
}

// SearchNames returns platform-ordered basenames for a binary (e.g. foo.exe, foo on Windows).
func SearchNames(base string) []string {
	if runtime.GOOS == "windows" {
		return []string{base + ".exe", base}
	}
	return []string{base}
}

// FirstInExeDirs looks for the first existing file named in names under exeDir, exeDir/bin, and exeDir/chimera/bin.
func FirstInExeDirs(exeDir string, names []string) string {
	if exeDir == "" {
		return ""
	}
	for _, d := range []string{
		exeDir,
		filepath.Join(exeDir, "bin"),
		filepath.Join(exeDir, "chimera", "bin"),
	} {
		if p := firstExistingFile(d, names); p != "" {
			return p
		}
	}
	return ""
}

func firstExistingFile(dir string, names []string) string {
	for _, n := range names {
		p := filepath.Join(dir, n)
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
	}
	return ""
}
