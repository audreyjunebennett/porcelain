//go:build !windows

package llamaserver

import "os/exec"

func applyNoConsoleWindow(cmd *exec.Cmd) {}
