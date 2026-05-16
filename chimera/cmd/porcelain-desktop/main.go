//go:build windows
// +build windows

package main

import (
	"context"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
)

var (
	kernel32     = syscall.NewLazyDLL("kernel32.dll")
	user32       = syscall.NewLazyDLL("user32.dll")
	getConsole   = kernel32.NewProc("GetConsoleWindow")
	showWindow   = user32.NewProc("ShowWindow")
)

func init() {
	hideConsole()
}

func hideConsole() {
	hwnd, _, _ := getConsole.Call()
	if hwnd != 0 {
		showWindow.Call(hwnd, 0) // SW_HIDE = 0
	}
}

type Service struct {
	Name string
	Cmd  *exec.Cmd
}

func main() {
	// Get repo root by looking for locus_api.py
	exePath, err := os.Executable()
	if err != nil {
		exePath, _ = os.Getwd()
	}

	// Start from exe directory and go up until we find the repo
	repoRoot := filepath.Dir(filepath.Dir(filepath.Dir(exePath)))

	// Verify we found the right directory by checking for locus_api.py
	for i := 0; i < 5; i++ {
		if _, err := os.Stat(filepath.Join(repoRoot, "locus_api.py")); err == nil {
			break
		}
		repoRoot = filepath.Dir(repoRoot)
	}

	// Ensure .data/logs exists
	logsDir := filepath.Join(repoRoot, ".data", "logs")
	os.MkdirAll(logsDir, 0o755)

	// Start all services
	services := startAllServices(repoRoot)

	// Wait for interrupt (Ctrl+C or window close)
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	<-sigChan

	// Gracefully shutdown all
	shutdownTimeout := time.Second * 10
	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	shutdownAllServices(ctx, services)
}

func startAllServices(repoRoot string) []Service {
	var services []Service

	// Start Relay (Python, port 9999)
	relayLogPath := filepath.Join(repoRoot, ".data", "logs", "relay.log")
	relayLog, _ := os.OpenFile(relayLogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	relayCmd := exec.Command("python", "locus/relay_server.py")
	relayCmd.Dir = repoRoot
	relayCmd.Stdout = relayLog
	relayCmd.Stderr = relayLog
	relayCmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: 0x08000000, // CREATE_NO_WINDOW
	}
	relayCmd.Start()
	services = append(services, Service{"relay", relayCmd})
	defer relayLog.Close()

	// Start Locus (Python, port 11435)
	locusLogPath := filepath.Join(repoRoot, ".data", "logs", "locus.log")
	locusLog, _ := os.OpenFile(locusLogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	locusCmd := exec.Command("python", "locus_api.py")
	locusCmd.Dir = repoRoot
	locusCmd.Stdout = locusLog
	locusCmd.Stderr = locusLog
	locusCmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: 0x08000000,
	}
	locusCmd.Start()
	services = append(services, Service{"locus", locusCmd})
	defer locusLog.Close()

	// Start Moto X Receiver (Python, port 9001) - optional if file exists
	receiverPath := filepath.Join(repoRoot, "Moto X", "receiver.py")
	if _, err := os.Stat(receiverPath); err == nil {
		receiverLogPath := filepath.Join(repoRoot, ".data", "logs", "receiver.log")
		receiverLog, _ := os.OpenFile(receiverLogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		receiverCmd := exec.Command("python", "Moto X/receiver.py")
		receiverCmd.Dir = repoRoot
		receiverCmd.Stdout = receiverLog
		receiverCmd.Stderr = receiverLog
		receiverCmd.SysProcAttr = &syscall.SysProcAttr{
			CreationFlags: 0x08000000,
		}
		receiverCmd.Start()
		services = append(services, Service{"receiver", receiverCmd})
		defer receiverLog.Close()
	}

	// Start Chimera (gateway mode — webview is opened separately via Edge PWA)
	chimeraLogPath := filepath.Join(repoRoot, ".data", "logs", "chimera.log")
	chimeraLog, _ := os.OpenFile(chimeraLogPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	configPath := filepath.Join(repoRoot, "chimera", "config", "gateway.yaml")
	chimeraCmd := exec.Command("chimera/bin/chimera.exe", "gateway", "-config", configPath)
	chimeraCmd.Dir = repoRoot
	chimeraCmd.Stdout = chimeraLog
	chimeraCmd.Stderr = chimeraLog
	chimeraCmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: 0x08000000,
	}
	chimeraCmd.Start()
	services = append(services, Service{"chimera", chimeraCmd})
	defer chimeraLog.Close()

	// Open Locus workspace in Edge PWA mode (frameless, app-like window)
	go openLocusPWA()

	return services
}

func openLocusPWA() {
	// Wait for Locus to be ready before opening
	time.Sleep(4 * time.Second)

	locusURL := "http://127.0.0.1:11435/web"

	// Try Edge first (supports --app= mode for frameless PWA window)
	edgePaths := []string{
		`C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`,
		`C:\Program Files\Microsoft\Edge\Application\msedge.exe`,
	}
	for _, edgePath := range edgePaths {
		if _, err := os.Stat(edgePath); err == nil {
			cmd := exec.Command(edgePath, "--app="+locusURL)
			cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x08000000}
			if cmd.Start() == nil {
				return
			}
		}
	}

	// Fallback: open in default browser
	exec.Command("cmd", "/c", "start", locusURL).Start()
}

func shutdownAllServices(ctx context.Context, services []Service) {
	for _, svc := range services {
		if svc.Cmd.Process != nil {
			svc.Cmd.Process.Signal(syscall.SIGTERM)
		}
	}

	// Wait up to timeout for graceful exit
	done := make(chan error, len(services))
	for _, svc := range services {
		go func(s Service) {
			done <- s.Cmd.Wait()
		}(svc)
	}

	// Collect results with timeout
	for i := 0; i < len(services); i++ {
		select {
		case <-done:
		case <-ctx.Done():
			// Timeout, force kill
			for _, svc := range services {
				if svc.Cmd.Process != nil {
					svc.Cmd.Process.Kill()
				}
			}
			return
		}
	}
}
