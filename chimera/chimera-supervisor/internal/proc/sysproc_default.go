//go:build !windows

package proc

import "os/exec"

// ApplyNoConsoleWindow is a no-op on non-Windows platforms.
func ApplyNoConsoleWindow(cmd *exec.Cmd) {}
