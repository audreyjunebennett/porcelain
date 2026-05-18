package platform

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/lynn/porcelain/locus/locus-desktop/internal/paths"
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

// RevealProjectPath opens the OS file manager focused on rel, relative to the runtime root.
func RevealProjectPath(rel string) error {
	rel = strings.TrimSpace(rel)
	rel = filepath.ToSlash(rel)
	rel = strings.TrimPrefix(rel, "/")
	if rel == "" || strings.Contains(rel, "..") {
		return fmt.Errorf("invalid path")
	}
	nativeRel := filepath.FromSlash(rel)
	root := paths.RuntimeRoot()
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
		arg := "/select," + abs
		return exec.Command("explorer", arg).Start()
	case "darwin":
		return exec.Command("open", "-R", abs).Start()
	default:
		return openDir(filepath.Dir(abs))
	}
}
