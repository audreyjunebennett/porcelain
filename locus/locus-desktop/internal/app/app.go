package app

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/lynn/porcelain/internal/locus"
	"github.com/lynn/porcelain/locus/locus-desktop/internal/launcher"
	"github.com/lynn/porcelain/locus/locus-desktop/internal/page"
	"github.com/lynn/porcelain/locus/locus-desktop/internal/paths"
	"github.com/lynn/porcelain/locus/locus-desktop/internal/supervisor"
	"github.com/lynn/porcelain/locus/locus-desktop/internal/telemetry"
)

const (
	runtimeHealthPollInterval  = 2 * time.Second
	runtimeHealthMissThreshold = 3
)

// UIRequest describes what the native shell should open.
type UIRequest struct {
	PanelURL      string
	Unreachable   bool
	BaseURL       string
	Owned         bool
	RuntimeLossCh <-chan string
	RootCtx       context.Context
	StopRoot      context.CancelFunc
}

// Shell opens or blocks for the desktop UI (webview or headless wait).
type Shell interface {
	Run(req UIRequest)
}

// Config drives a single desktop launcher run.
type Config struct {
	Args        []string
	OpenWebview bool
	Shell       Shell
}

// Run launches or attaches to chimera-supervisor and opens the UI shell.
func Run(cfg Config) {
	runtimeRoot := paths.RuntimeRoot()
	launchArgs := launcher.FilterSupervisorArgs(cfg.Args)
	logDir := launcher.LogDir(cfg.Args, runtimeRoot)
	baseURL := supervisor.BaseURL(cfg.Args)

	rootCtx, stopRoot := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stopRoot()

	telemetry.RecordLifecycle(runtimeRoot, telemetry.StateInit, "desktop launcher start", map[string]any{
		"base_url": baseURL,
	})

	var md telemetry.LaunchMetadata
	md.BaseURL = baseURL
	writeMeta := func() {
		telemetry.WriteLaunchMetadata(runtimeRoot, md)
	}

	var ownedProc *launcher.OwnedProcess
	owned := false
	defer func() {
		if ownedProc != nil && ownedProc.LogFile != nil {
			_ = ownedProc.LogFile.Close()
		}
	}()

	if supervisor.Reachable(baseURL) {
		md.Mode = telemetry.LaunchAttachExisting
	} else {
		telemetry.RecordLifecycle(runtimeRoot, telemetry.StateLaunchAttach, "attach check failed; launch path", map[string]any{
			"base_url": baseURL,
		})
		unlock, lockErr := launcher.AcquireLaunchLock(runtimeRoot, launcher.LaunchLockTimeout)
		if lockErr != nil {
			locus.Logf("startup lock: %v\n", lockErr)
			os.Exit(1)
		}
		defer unlock()

		if supervisor.Reachable(baseURL) {
			telemetry.RecordLifecycle(runtimeRoot, telemetry.StateLaunchAttach, "attached while waiting for launch lock", map[string]any{
				"base_url": baseURL,
			})
			md.Mode = telemetry.LaunchAttachExisting
		} else {
			bin, err := launcher.ResolveSupervisorBinary()
			if err != nil {
				md.Mode = telemetry.LaunchFailed
				md.Error = "resolve " + locus.BinSupervisor + ": " + err.Error()
				writeMeta()
				locus.Logf("resolve %s: %v\n", locus.BinSupervisor, err)
				os.Exit(1)
			}
			telemetry.RecordLifecycle(runtimeRoot, telemetry.StateLaunchAttach, "starting owned supervisor", map[string]any{
				"supervisor_bin": bin,
				"log_dir":        logDir,
			})
			ownedProc, err = launcher.StartOwnedSupervisor(runtimeRoot, logDir, bin, launchArgs)
			if err != nil {
				md.Mode = telemetry.LaunchFailed
				md.SupervisorBin = bin
				md.LaunchArgsRedacted = launcher.RedactArgs(launchArgs)
				md.SupervisorWorkDir = runtimeRoot
				md.Error = err.Error()
				writeMeta()
				locus.Logf("%v\n", err)
				os.Exit(1)
			}
			owned = true
			md.Mode = telemetry.LaunchLaunchOwned
			md.SupervisorBin = bin
			md.SupervisorOwned = true
			md.SupervisorPID = ownedProc.Cmd.Process.Pid
			md.SupervisorWorkDir = ownedProc.Cmd.Dir
			md.SupervisorLogPath = ownedProc.LogFile.Name()
			md.LaunchArgsRedacted = launcher.RedactArgs(launchArgs)

			telemetry.RecordLifecycle(runtimeRoot, telemetry.StateWaitReady, "waiting for supervisor after launch", map[string]any{
				"timeout_ms": launcher.AttachStartupTimeout.Milliseconds(),
			})
			if !supervisor.WaitReachable(baseURL, launcher.AttachStartupTimeout) {
				_ = launcher.StopOwnedSupervisor(ownedProc.Cmd, baseURL)
				owned = false
				md.Mode = telemetry.LaunchFailed
				md.SupervisorOwned = false
				md.Error = "timed out waiting for " + locus.BinSupervisor
				writeMeta()
				telemetry.RecordLifecycle(runtimeRoot, telemetry.StateFailed, md.Error, map[string]any{"base_url": baseURL})
				failUI(cfg, runtimeRoot, baseURL, true, "Timed out waiting for "+locus.BinSupervisor+" startup", rootCtx, stopRoot)
				return
			}
		}
	}

	telemetry.RecordLifecycle(runtimeRoot, telemetry.StateWaitReady, "waiting for readiness", map[string]any{
		"timeout_ms": launcher.ReadinessTimeout.Milliseconds(),
	})
	ready, readinessDetail := supervisor.WaitReady(baseURL, launcher.ReadinessTimeout)
	if !ready {
		if owned && ownedProc != nil {
			_ = launcher.StopOwnedSupervisor(ownedProc.Cmd, baseURL)
			md.SupervisorOwned = false
		}
		md.Mode = telemetry.LaunchFailed
		md.Error = readinessDetail
		writeMeta()
		telemetry.RecordLifecycle(runtimeRoot, telemetry.StateFailed, readinessDetail, map[string]any{"detail": readinessDetail})
		failUI(cfg, runtimeRoot, baseURL, owned, readinessDetail, rootCtx, stopRoot)
		return
	}

	writeMeta()

	entryURL := supervisor.EntryURL(baseURL)
	telemetry.RecordLifecycle(runtimeRoot, telemetry.StateOpenUI, "entry route selected", map[string]any{
		"entry_url": entryURL,
	})

	runtimeLossCh := make(chan string, 1)
	go supervisor.MonitorRuntimeLoss(rootCtx, baseURL, runtimeLossCh, runtimeHealthPollInterval, runtimeHealthMissThreshold)

	if owned && ownedProc != nil {
		defer func() {
			_ = launcher.StopOwnedSupervisor(ownedProc.Cmd, baseURL)
			telemetry.RecordLifecycle(runtimeRoot, telemetry.StateShutdown, "owned supervisor stopped on desktop close", nil)
		}()
	}

	if cfg.OpenWebview {
		cfg.Shell.Run(UIRequest{
			PanelURL:      entryURL,
			BaseURL:       baseURL,
			RuntimeLossCh: runtimeLossCh,
			RootCtx:       rootCtx,
			StopRoot:      stopRoot,
		})
		return
	}

	select {
	case <-rootCtx.Done():
	case reason := <-runtimeLossCh:
		telemetry.RecordLifecycle(runtimeRoot, telemetry.StateRuntimeLost, reason, map[string]any{"base_url": baseURL})
		locus.Logf("supervisor runtime lost: %s\n", reason)
	}
}

func failUI(cfg Config, runtimeRoot, baseURL string, owned bool, reason string, rootCtx context.Context, stopRoot context.CancelFunc) {
	telemetry.RecordLifecycle(runtimeRoot, telemetry.StateFailed, reason, map[string]any{
		"base_url": baseURL,
		"owned":    owned,
	})
	if !cfg.OpenWebview {
		locus.Logf("cannot connect to supervisor: %s\n", reason)
		os.Exit(1)
	}
	cfg.Shell.Run(UIRequest{
		PanelURL:    page.UnreachableDataURL(baseURL, reason, owned),
		Unreachable: true,
		BaseURL:     baseURL,
		Owned:       owned,
		RootCtx:     rootCtx,
		StopRoot:    stopRoot,
	})
}
