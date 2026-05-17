//go:build !windows

package main

import "os"

func sendGracefulTerminate(p *os.Process) error {
	return p.Signal(os.Interrupt)
}

