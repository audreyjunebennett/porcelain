//go:build !windows

package proc

// ForceKillProcessTree is a no-op on non-Windows platforms.
func ForceKillProcessTree(pid int) error {
	_ = pid
	return nil
}
