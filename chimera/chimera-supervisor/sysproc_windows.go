//go:build windows

package main

import (
	"os/exec"
	"syscall"
)

// CREATE_NO_WINDOW — child console apps do not allocate a visible console.
const createNoWindow = 0x08000000

func applyNoConsoleWindow(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.HideWindow = true
	cmd.SysProcAttr.CreationFlags |= createNoWindow
}
