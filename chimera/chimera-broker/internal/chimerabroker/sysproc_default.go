//go:build !windows

package chimerabroker

import "os/exec"

func applyNoConsoleWindow(cmd *exec.Cmd) {}
