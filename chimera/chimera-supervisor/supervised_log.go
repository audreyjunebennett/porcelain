package main

import (
	"errors"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	wruntime "github.com/lynn/porcelain/chimera/internal/wrapper/runtime"
)

// supervisedChildEnv is applied to wrapper and indexer children started by the supervisor.
func supervisedChildEnv(controlBaseURL string) map[string]string {
	out := map[string]string{
		"CHIMERA_LOG_JSON":   "1",
		"CHIMERA_SUPERVISED": "1",
	}
	if u := strings.TrimSpace(controlBaseURL); u != "" {
		out["CHIMERA_SUPERVISOR_CONTROL_URL"] = u
	}
	return out
}

// supervisedWrapperArgs are appended to chimera-gateway, chimera-broker, and chimera-vectorstore.
func supervisedWrapperArgs(base []string) []string {
	out := append([]string(nil), base...)
	out = append(out, "-debug-forward-upstream")
	return out
}

// supervisedLogSink normalizes child stdout/stderr to JSON lines and records them in the ring buffer.
func supervisedLogSink(storeWriter io.Writer, normalize func(io.Writer) io.Writer) io.Writer {
	return normalize(io.MultiWriter(storeWriter, os.Stdout))
}

type supervisedChild struct {
	name   string
	cmd    *exec.Cmd
	waitCh <-chan error
}

func signalSupervisedChild(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	_ = cmd.Process.Signal(os.Interrupt)
}

// shutdownSupervisedChildren signals all children, waits up to terminateWait in parallel, then force-stops stragglers.
func shutdownSupervisedChildren(log *slog.Logger, terminateWait time.Duration, children ...supervisedChild) {
	var pending []supervisedChild
	for _, c := range children {
		if c.cmd == nil || c.cmd.Process == nil {
			continue
		}
		pending = append(pending, c)
	}
	if len(pending) == 0 {
		return
	}

	for _, c := range pending {
		if log != nil {
			log.Info("signaling child shutdown", "msg", "chimera-supervisor.shutdown.child_signaling", "child", c.name, "pid", c.cmd.Process.Pid)
		}
		signalSupervisedChild(c.cmd)
	}

	var wg sync.WaitGroup
	for _, c := range pending {
		wg.Add(1)
		go func(c supervisedChild) {
			defer wg.Done()
			waitChildExit(log, c, terminateWait)
		}(c)
	}
	wg.Wait()
}

func waitChildExit(log *slog.Logger, c supervisedChild, timeout time.Duration) {
	if c.waitCh == nil {
		return
	}
	select {
	case werr := <-c.waitCh:
		logSupervisedChildExit(log, c.name, werr, false)
	case <-time.After(timeout):
		if log != nil {
			log.Warn("child did not exit within grace period; forcing stop",
				"msg", "chimera-supervisor.shutdown.child_force_kill",
				"child", c.name,
				"pid", c.cmd.Process.Pid,
				"timeout", timeout)
		}
		forceStopSupervisedChild(log, c)
	}
}

func forceStopSupervisedChild(log *slog.Logger, c supervisedChild) {
	if c.cmd == nil || c.cmd.Process == nil {
		return
	}
	pid := c.cmd.Process.Pid
	if runtime.GOOS == "windows" {
		// Kill wrapper + backend tree first; wrappers often ignore os.Interrupt without a console.
		if err := forceKillProcessTree(pid); err != nil && log != nil {
			log.Warn("process-tree kill failed",
				"msg", "chimera-supervisor.shutdown.child_tree_kill_failed",
				"child", c.name,
				"pid", pid,
				"err", err)
		}
	} else if err := wruntime.TerminateThenKill(c.cmd, 5*time.Second); err != nil && log != nil {
		log.Warn("forced child stop", "msg", "chimera-supervisor.shutdown.child_force_kill_failed", "child", c.name, "err", err)
	}

	select {
	case werr := <-c.waitCh:
		logSupervisedChildExit(log, c.name, werr, true)
	case <-time.After(5 * time.Second):
		if runtime.GOOS != "windows" {
			if err := forceKillProcessTree(pid); err != nil && log != nil {
				log.Warn("process-tree kill fallback failed",
					"msg", "chimera-supervisor.shutdown.child_tree_kill_failed",
					"child", c.name,
					"pid", pid,
					"err", err)
			}
		}
		select {
		case werr := <-c.waitCh:
			logSupervisedChildExit(log, c.name, werr, true)
		case <-time.After(3 * time.Second):
			if log != nil {
				log.Warn("child still running after force stop", "msg", "chimera-supervisor.shutdown.child_stuck", "child", c.name, "pid", pid)
			}
		}
	}
}

func logSupervisedChildExit(log *slog.Logger, name string, werr error, forced bool) {
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
			// Forced stop on Windows commonly yields exit status 1; not an operator error.
			log.Info(name+" process finished after forced stop", attrs...)
			return
		}
		log.Warn(name+" process finished", attrs...)
		return
	}
	log.Info(name+" process finished", attrs...)
}

// killSupervisedWrapperFamilies force-stops wrapper process trees (startup abort paths).
func killSupervisedWrapperFamilies(cmds ...*exec.Cmd) {
	for _, cmd := range cmds {
		if cmd == nil || cmd.Process == nil {
			continue
		}
		pid := cmd.Process.Pid
		if runtime.GOOS == "windows" {
			if err := forceKillProcessTree(pid); err != nil {
				_ = cmd.Process.Kill()
			}
			continue
		}
		_ = cmd.Process.Signal(os.Interrupt)
	}
}
