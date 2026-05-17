//go:build windows

package runtime

import "os"

func sendGracefulTerminate(p *os.Process) error {
	// Prefer Interrupt so wrapper runtimes can shut down HTTP and backends via TerminateThenKill.
	if err := p.Signal(os.Interrupt); err == nil {
		return nil
	}
	return p.Kill()
}
