//go:build !windows

package main

import "os/exec"

func applyNoConsoleWindow(cmd *exec.Cmd) {}
