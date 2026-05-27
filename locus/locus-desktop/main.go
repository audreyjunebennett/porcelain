package main

import (
	"fmt"
	"os"

	"github.com/joho/godotenv"
	"github.com/lynn/porcelain/internal/locus"
	"github.com/lynn/porcelain/locus/locus-desktop/internal/app"
)

func main() {
	_ = godotenv.Load("env")
	_ = godotenv.Load(".env")

	args := os.Args[1:]
	headless := false
	for len(args) > 0 && (args[0] == locus.FlagHeadless || args[0] == locus.FlagHeadlessShort) {
		headless = true
		args = args[1:]
	}
	if len(args) > 0 && (args[0] == "-version" || args[0] == "--version") {
		fmt.Printf("%s %s\ncommit %s\nbuild date %s\n", locus.BinDesktop, version, commit, date)
		return
	}
	for _, a := range args {
		if a == "-h" || a == "--help" {
			printHelp()
			return
		}
	}

	app.Run(app.Config{
		Args:        args,
		OpenWebview: !headless,
		Shell:       desktopShell{},
	})
}

func printHelp() {
	fmt.Printf(`Locus desktop launcher

Usage:
  %s [flags]
  %s %s [flags]
  %s -version

This binary launches or attaches to %s and opens desktop UI.
Launcher-only flags:
  %s <path>   Supervisor log directory (default: %s)

Environment:
  %s=1   Append lifecycle events to %s/%s/%s
`,
		locus.BinDesktop,
		locus.BinDesktop, locus.FlagHeadless,
		locus.BinDesktop,
		locus.BinSupervisor,
		locus.FlagLogDirLong, locus.DirData,
		locus.EnvTrace, locus.DirData, locus.DirDesktopState, locus.FileLifecycleLog,
	)
}

type desktopShell struct{}

func (desktopShell) Run(req app.UIRequest) {
	runDesktopWebview(req)
}
