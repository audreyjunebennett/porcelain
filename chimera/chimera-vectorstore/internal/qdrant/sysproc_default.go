//go:build !windows

package qdrant

import "os/exec"

func applyNoConsoleWindow(cmd *exec.Cmd) {}
