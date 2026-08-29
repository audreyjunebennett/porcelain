//go:build windows

package llamaserver

import (
	"os/exec"
	"syscall"
)

func applyNoConsoleWindow(cmd *exec.Cmd) {
	if cmd != nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x08000000}
	}
}
