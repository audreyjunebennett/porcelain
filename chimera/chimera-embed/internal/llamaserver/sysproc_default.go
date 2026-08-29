//go:build !windows

package llamaserver

import "os/exec"

func applyNoConsoleWindow(_ *exec.Cmd) {}
