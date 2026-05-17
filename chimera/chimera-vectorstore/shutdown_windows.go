//go:build windows

package main

import "os"

func sendGracefulTerminate(p *os.Process) error {
	if err := p.Signal(os.Interrupt); err == nil {
		return nil
	}
	return p.Kill()
}
