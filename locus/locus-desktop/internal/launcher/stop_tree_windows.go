//go:build windows

package launcher

import (
	"errors"
	"fmt"
	"os/exec"
)

func forceKillProcessTree(pid int) error {
	if pid <= 0 {
		return fmt.Errorf("invalid pid %d", pid)
	}
	cmd := exec.Command("taskkill", "/T", "/F", "/PID", fmt.Sprintf("%d", pid))
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 128 {
			return nil
		}
		return fmt.Errorf("taskkill /T /F /PID %d: %w", pid, err)
	}
	return nil
}
