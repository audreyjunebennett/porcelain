//go:build windows

package proc

import (
	"os/exec"
	"syscall"
)

// CREATE_NO_WINDOW — child console apps do not allocate a visible console.
const createNoWindow = 0x08000000

// ApplyNoConsoleWindow configures cmd so children do not spawn a visible console.
func ApplyNoConsoleWindow(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.HideWindow = true
	cmd.SysProcAttr.CreationFlags |= createNoWindow
}
