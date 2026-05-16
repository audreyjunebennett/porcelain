//go:build windows
// +build windows

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
)

func init() {
	hideConsole()
}

func hideConsole() {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	user32 := syscall.NewLazyDLL("user32.dll")
	getConsole := kernel32.NewProc("GetConsoleWindow")
	showWindow := user32.NewProc("ShowWindow")

	hwnd, _, _ := getConsole.Call()
	if hwnd != 0 {
		showWindow.Call(hwnd, 0) // SW_HIDE = 0
	}
}

func main() {
	// Get the directory where this launcher exe is located
	exePath, err := os.Executable()
	if err != nil {
		os.Exit(1)
	}
	exeDir := filepath.Dir(exePath)

	// Support both layouts:
	// - repo root: porcelain.exe -> chimera/bin/porcelain-desktop.exe
	// - chimera/bin: porcelain.exe -> porcelain-desktop.exe beside it
	candidates := []string{
		filepath.Join(exeDir, "porcelain-desktop.exe"),
		filepath.Join(exeDir, "chimera", "bin", "porcelain-desktop.exe"),
	}

	desktopExePath := ""
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			desktopExePath = candidate
			break
		}
	}
	if desktopExePath == "" {
		os.Exit(1)
	}

	// Launch porcelain-desktop.exe silently
	cmd := exec.Command(desktopExePath)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: 0x08000000, // CREATE_NO_WINDOW
	}
	cmd.Start()
}
