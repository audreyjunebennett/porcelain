//go:build !windows

package launcher

import "os/exec"

func applyNoConsoleWindow(cmd *exec.Cmd) {}
