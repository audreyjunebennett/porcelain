//go:build !windows

package launcher

import "os"

func forceKillProcessTree(pid int) error {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return proc.Kill()
}
