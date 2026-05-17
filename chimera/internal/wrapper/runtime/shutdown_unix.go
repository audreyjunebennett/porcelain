//go:build !windows

package runtime

import "os"

func sendGracefulTerminate(p *os.Process) error {
	return p.Signal(os.Interrupt)
}
