//go:build !windows

package main

func forceKillProcessTree(pid int) error {
	_ = pid
	return nil
}
