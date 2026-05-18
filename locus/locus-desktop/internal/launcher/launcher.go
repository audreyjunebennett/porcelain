package launcher

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/lynn/porcelain/internal/binfind"
	"github.com/lynn/porcelain/internal/locus"
	"github.com/lynn/porcelain/locus/locus-desktop/internal/supervisor"
	"github.com/lynn/porcelain/locus/locus-desktop/internal/telemetry"
)

const (
	AttachStartupTimeout       = 60 * time.Second
	LaunchLockTimeout          = 4 * time.Second
	ReadinessTimeout           = 45 * time.Second
	OwnedSupervisorStopTimeout = 45 * time.Second
)

// FilterSupervisorArgs removes launcher-only flags before passing args to chimera-supervisor.
func FilterSupervisorArgs(args []string) []string {
	out := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		raw := strings.TrimSpace(args[i])
		lower := strings.ToLower(raw)
		if lower == locus.LegacyDesktopArg {
			continue
		}
		if lower == locus.FlagLogDir || lower == locus.FlagLogDirLong {
			if i+1 < len(args) {
				i++
			}
			continue
		}
		if strings.HasPrefix(lower, locus.FlagLogDir+"=") || strings.HasPrefix(lower, locus.FlagLogDirLong+"=") {
			continue
		}
		out = append(out, args[i])
	}
	return out
}

// LogDir resolves the supervisor log directory from env, flags, and runtime root.
func LogDir(args []string, runtimeRoot string) string {
	dir := strings.TrimSpace(os.Getenv(locus.EnvLogDir))
	if dir == "" {
		dir = filepath.Join(runtimeRoot, locus.DirData)
	}
	for i := 0; i < len(args); i++ {
		raw := strings.TrimSpace(args[i])
		lower := strings.ToLower(raw)
		switch {
		case lower == locus.FlagLogDir || lower == locus.FlagLogDirLong:
			if i+1 < len(args) {
				dir = strings.TrimSpace(args[i+1])
				i++
			}
		case strings.HasPrefix(lower, locus.FlagLogDir+"="), strings.HasPrefix(lower, locus.FlagLogDirLong+"="):
			if idx := strings.IndexByte(raw, '='); idx >= 0 {
				dir = strings.TrimSpace(raw[idx+1:])
			}
		}
	}
	if strings.TrimSpace(dir) == "" {
		dir = filepath.Join(runtimeRoot, locus.DirData)
	}
	if !isRootedPath(dir) {
		dir = filepath.Join(runtimeRoot, dir)
	}
	return filepath.Clean(dir)
}

func isRootedPath(p string) bool {
	if filepath.IsAbs(p) {
		return true
	}
	if strings.HasPrefix(p, string(os.PathSeparator)) {
		return true
	}
	if runtime.GOOS == "windows" && strings.HasPrefix(p, "/") {
		return true
	}
	return false
}

// ResolveSupervisorBinary locates chimera-supervisor next to locus-desktop.
func ResolveSupervisorBinary() (string, error) {
	exeDir := binfind.ExecutableDir()
	if exeDir != "" {
		if p := binfind.FirstInExeDirs(exeDir, locus.SupervisorSearchNames()); p != "" {
			return p, nil
		}
	}
	if runtime.GOOS == "windows" {
		return "", fmt.Errorf("%s.exe not found next to %s", locus.BinSupervisor, locus.BinDesktop)
	}
	return "", fmt.Errorf("%s not found next to %s", locus.BinSupervisor, locus.BinDesktop)
}

// OpenSupervisorLog opens the append-only supervisor log under logDir.
func OpenSupervisorLog(logDir string) (*os.File, error) {
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return nil, fmt.Errorf("mkdir log dir %s: %w", logDir, err)
	}
	logPath := locus.SupervisorLogPath(logDir)
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", logPath, err)
	}
	_, _ = fmt.Fprintf(f, "\n[%s] %s launching supervisor\n", time.Now().UTC().Format(time.RFC3339Nano), locus.BinDesktop)
	return f, nil
}

// AcquireLaunchLock prevents concurrent desktop launches from starting multiple supervisors.
func AcquireLaunchLock(runtimeRoot string, timeout time.Duration) (func(), error) {
	if timeout <= 0 {
		timeout = LaunchLockTimeout
	}
	lockPath := telemetry.LaunchLockPath(runtimeRoot)
	if err := os.MkdirAll(locus.RunDirPath(runtimeRoot), 0o755); err != nil {
		return nil, err
	}
	deadline := time.Now().Add(timeout)
	for {
		f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
		if err == nil {
			_, _ = fmt.Fprintf(f, "pid=%d\nstarted=%s\n", os.Getpid(), time.Now().UTC().Format(time.RFC3339Nano))
			_ = f.Close()
			return func() { _ = os.Remove(lockPath) }, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return nil, err
		}
		if st, statErr := os.Stat(lockPath); statErr == nil && time.Since(st.ModTime()) > 2*time.Minute {
			_ = os.Remove(lockPath)
			continue
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("another desktop launch is already in progress (%s)", lockPath)
		}
		time.Sleep(120 * time.Millisecond)
	}
}

// RedactArgs redacts sensitive supervisor flag values for diagnostics.
func RedactArgs(args []string) []string {
	out := make([]string, 0, len(args))
	redactNext := false
	for _, a := range args {
		if redactNext {
			out = append(out, "<redacted>")
			redactNext = false
			continue
		}
		lower := strings.ToLower(strings.TrimSpace(a))
		if lower == "" {
			out = append(out, a)
			continue
		}
		if strings.HasPrefix(lower, "-") {
			k := lower
			if idx := strings.IndexByte(lower, '='); idx >= 0 {
				key := lower[:idx]
				if isSensitiveArgKey(key) {
					out = append(out, key+"=<redacted>")
					continue
				}
			}
			if isSensitiveArgKey(k) {
				out = append(out, a)
				redactNext = true
				continue
			}
		}
		out = append(out, a)
	}
	return out
}

func isSensitiveArgKey(k string) bool {
	key := strings.ToLower(strings.TrimLeft(strings.TrimSpace(k), "-"))
	return strings.Contains(key, "token") ||
		strings.Contains(key, "secret") ||
		strings.Contains(key, "pass") ||
		strings.Contains(key, "apikey") ||
		strings.Contains(key, "api-key") ||
		strings.Contains(key, "key")
}

// StopOwnedSupervisor stops a desktop-owned supervisor and its supervised children.
// controlBaseURL should be the supervisor control API base (e.g. http://127.0.0.1:7710).
func StopOwnedSupervisor(cmd *exec.Cmd, controlBaseURL string) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	waitCh := make(chan error, 1)
	go func() {
		waitCh <- cmd.Wait()
	}()

	supervisor.RequestShutdown(controlBaseURL)
	if err := cmd.Process.Signal(os.Interrupt); err != nil && !errors.Is(err, os.ErrProcessDone) {
		// Interrupt may fail on Windows when the desktop has no console; HTTP shutdown above is primary.
		if controlBaseURL == "" {
			return err
		}
	}

	select {
	case err := <-waitCh:
		return err
	case <-time.After(OwnedSupervisorStopTimeout):
		pid := cmd.Process.Pid
		if err := forceKillProcessTree(pid); err != nil && !errors.Is(err, os.ErrProcessDone) {
			if killErr := cmd.Process.Kill(); killErr != nil && !errors.Is(killErr, os.ErrProcessDone) {
				return killErr
			}
		}
		<-waitCh
		return nil
	}
}

// OwnedProcess starts chimera-supervisor as a child of the desktop launcher.
type OwnedProcess struct {
	Cmd     *exec.Cmd
	LogFile *os.File
	Bin     string
}

// StartOwnedSupervisor execs chimera-supervisor with pass-through args.
func StartOwnedSupervisor(runtimeRoot, logDir, bin string, launchArgs []string) (*OwnedProcess, error) {
	logFile, err := OpenSupervisorLog(logDir)
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(bin, launchArgs...)
	cmd.Dir = runtimeRoot
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	applyNoConsoleWindow(cmd)
	if err := cmd.Start(); err != nil {
		_ = logFile.Close()
		return nil, fmt.Errorf("start %s: %w", locus.BinSupervisor, err)
	}
	return &OwnedProcess{Cmd: cmd, LogFile: logFile, Bin: bin}, nil
}
