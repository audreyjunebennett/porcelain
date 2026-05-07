package platform

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// OpenURLInBrowser opens http(s) URLs in the OS default browser (not an embedded webview).
func OpenURLInBrowser(raw string) error {
	u := strings.TrimSpace(raw)
	if !strings.HasPrefix(u, "http://") && !strings.HasPrefix(u, "https://") {
		return fmt.Errorf("unsupported url scheme")
	}
	switch runtime.GOOS {
	case "windows":
		cmd := exec.Command("cmd", "/c", "start", "", u)
		cmd.Stdout = nil
		cmd.Stderr = nil
		return cmd.Start()
	case "darwin":
		return exec.Command("open", u).Start()
	default:
		return exec.Command("xdg-open", u).Start()
	}
}

// repoRootHint returns a directory used to resolve repository-relative paths for desktop helpers.
// When the executable lives in a bin/ directory (make install / desktop layout), the parent is used.
func repoRootHint() string {
	exe, err := os.Executable()
	if err != nil {
		wd, _ := os.Getwd()
		return wd
	}
	dir := filepath.Dir(exe)
	if strings.EqualFold(filepath.Base(dir), "bin") {
		return filepath.Clean(filepath.Join(dir, ".."))
	}
	return dir
}

// RevealProjectPath opens the OS file manager focused on rel, a slash-separated path relative to the repository root.
// For files, the folder is opened with the file selected where the platform supports it.
func RevealProjectPath(rel string) error {
	rel = strings.TrimSpace(rel)
	rel = filepath.ToSlash(rel)
	rel = strings.TrimPrefix(rel, "/")
	if rel == "" || strings.Contains(rel, "..") {
		return fmt.Errorf("invalid path")
	}
	nativeRel := filepath.FromSlash(rel)
	root := repoRootHint()
	abs := filepath.Join(root, nativeRel)
	abs, err := filepath.Abs(abs)
	if err != nil {
		return err
	}
	st, err := os.Stat(abs)
	if err != nil {
		return fmt.Errorf("path not found: %w", err)
	}
	if st.IsDir() {
		return openDir(abs)
	}
	return revealFile(abs)
}

func openDir(abs string) error {
	switch runtime.GOOS {
	case "windows":
		return exec.Command("explorer", abs).Start()
	case "darwin":
		return exec.Command("open", abs).Start()
	default:
		return exec.Command("xdg-open", abs).Start()
	}
}

func revealFile(abs string) error {
	switch runtime.GOOS {
	case "windows":
		// "/select,<path>" — comma immediately after select
		arg := "/select," + abs
		return exec.Command("explorer", arg).Start()
	case "darwin":
		return exec.Command("open", "-R", abs).Start()
	default:
		return openDir(filepath.Dir(abs))
	}
}
