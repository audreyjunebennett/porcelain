package paths

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/lynn/porcelain/internal/binfind"
	"github.com/lynn/porcelain/internal/locus"
)

// RuntimeRoot is the porcelain runtime root (parent of locus/bin when installed).
func RuntimeRoot() string {
	exeDir := binfind.ExecutableDir()
	if exeDir != "" {
		if strings.EqualFold(filepath.Base(exeDir), "bin") {
			return filepath.Dir(exeDir)
		}
		parent := filepath.Dir(exeDir)
		if strings.EqualFold(filepath.Base(parent), "bin") {
			return filepath.Dir(parent)
		}
	}
	if wd, err := os.Getwd(); err == nil {
		if st, serr := os.Stat(filepath.Join(wd, locus.DirConfig)); serr == nil && st.IsDir() {
			return wd
		}
		return wd
	}
	if exeDir != "" {
		return filepath.Dir(exeDir)
	}
	return "."
}
