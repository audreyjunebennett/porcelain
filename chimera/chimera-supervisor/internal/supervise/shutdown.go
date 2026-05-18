package supervise

import (
	"errors"
	"log/slog"
	"os"
	"os/exec"
	"runtime"
	"sync"
	"time"

	"github.com/lynn/porcelain/chimera/chimera-supervisor/internal/proc"
	wruntime "github.com/lynn/porcelain/chimera/internal/wrapper/runtime"
)

// Child describes one supervised process during shutdown.
type Child struct {
	Name   string
	Cmd    *exec.Cmd
	WaitCh <-chan error
}

func signalChild(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	_ = cmd.Process.Signal(os.Interrupt)
}

// ShutdownChildren signals all children, waits up to terminateWait in parallel, then force-stops stragglers.
func ShutdownChildren(log *slog.Logger, terminateWait time.Duration, children ...Child) {
	var pending []Child
	for _, c := range children {
		if c.Cmd == nil || c.Cmd.Process == nil {
			continue
		}
		pending = append(pending, c)
	}
	if len(pending) == 0 {
		return
	}

	for _, c := range pending {
		if log != nil {
			log.Info("signaling child shutdown", "msg", "chimera-supervisor.shutdown.child_signaling", "child", c.Name, "pid", c.Cmd.Process.Pid)
		}
		signalChild(c.Cmd)
	}

	var wg sync.WaitGroup
	for _, c := range pending {
		wg.Add(1)
		go func(c Child) {
			defer wg.Done()
			waitChildExit(log, c, terminateWait)
		}(c)
	}
	wg.Wait()
}

func waitChildExit(log *slog.Logger, c Child, timeout time.Duration) {
	if c.WaitCh == nil {
		return
	}
	select {
	case werr := <-c.WaitCh:
		logChildExit(log, c.Name, werr, false)
	case <-time.After(timeout):
		if log != nil {
			log.Warn("child did not exit within grace period; forcing stop",
				"msg", "chimera-supervisor.shutdown.child_force_kill",
				"child", c.Name,
				"pid", c.Cmd.Process.Pid,
				"timeout", timeout)
		}
		forceStopChild(log, c)
	}
}

func forceStopChild(log *slog.Logger, c Child) {
	if c.Cmd == nil || c.Cmd.Process == nil {
		return
	}
	pid := c.Cmd.Process.Pid
	if runtime.GOOS == "windows" {
		if err := proc.ForceKillProcessTree(pid); err != nil && log != nil {
			log.Warn("process-tree kill failed",
				"msg", "chimera-supervisor.shutdown.child_tree_kill_failed",
				"child", c.Name,
				"pid", pid,
				"err", err)
		}
	} else if err := wruntime.TerminateThenKill(c.Cmd, 5*time.Second); err != nil && log != nil {
		log.Warn("forced child stop", "msg", "chimera-supervisor.shutdown.child_force_kill_failed", "child", c.Name, "err", err)
	}

	select {
	case werr := <-c.WaitCh:
		logChildExit(log, c.Name, werr, true)
	case <-time.After(5 * time.Second):
		if runtime.GOOS != "windows" {
			if err := proc.ForceKillProcessTree(pid); err != nil && log != nil {
				log.Warn("process-tree kill fallback failed",
					"msg", "chimera-supervisor.shutdown.child_tree_kill_failed",
					"child", c.Name,
					"pid", pid,
					"err", err)
			}
		}
		select {
		case werr := <-c.WaitCh:
			logChildExit(log, c.Name, werr, true)
		case <-time.After(3 * time.Second):
			if log != nil {
				log.Warn("child still running after force stop", "msg", "chimera-supervisor.shutdown.child_stuck", "child", c.Name, "pid", pid)
			}
		}
	}
}

func logChildExit(log *slog.Logger, name string, werr error, forced bool) {
	if log == nil {
		return
	}
	attrs := []any{
		"msg", "chimera-supervisor.child.exited",
		"child", name,
		"forced", forced,
	}
	if werr != nil {
		attrs = append(attrs, "err", werr.Error())
		var ee *exec.ExitError
		if errors.As(werr, &ee) {
			attrs = append(attrs, "exit_code", ee.ExitCode())
		}
		if forced {
			log.Info(name+" process finished after forced stop", attrs...)
			return
		}
		log.Warn(name+" process finished", attrs...)
		return
	}
	log.Info(name+" process finished", attrs...)
}

// KillWrapperFamilies force-stops wrapper process trees (startup abort paths).
func KillWrapperFamilies(cmds ...*exec.Cmd) {
	for _, cmd := range cmds {
		if cmd == nil || cmd.Process == nil {
			continue
		}
		pid := cmd.Process.Pid
		if runtime.GOOS == "windows" {
			if err := proc.ForceKillProcessTree(pid); err != nil {
				_ = cmd.Process.Kill()
			}
			continue
		}
		_ = cmd.Process.Signal(os.Interrupt)
	}
}
