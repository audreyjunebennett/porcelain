// Package locus holds shared Locus desktop names: binaries, env vars, runtime files, and webview bridges.
// Values align with internal/naming and scripts/chimera-names.sh.
package locus

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/lynn/porcelain/internal/binfind"
	"github.com/lynn/porcelain/internal/naming"
)

// Product binaries (basename without directory).
const (
	BinDesktop     = naming.ProductDesktopName
	BinSupervisor  = naming.ProductSupervisorName
	BinBroker      = naming.ProductBrokerName
	BinBrokerHTTP  = naming.ProductBrokerHTTPBinName
	BinBifrostHTTP = naming.ProductBifrostHTTPBinName
	BinVectorstore = naming.ProductVectorstoreName
	BinQdrant      = naming.ProductQdrantBinName
	BinGateway     = naming.ProductGatewayBinName
	BinIndexer     = naming.ProductIndexerBinName
)

// Environment variables read by locus-desktop.
const (
	EnvTrace  = naming.EnvDesktopTrace
	EnvLogDir = naming.EnvDesktopLogDir
)

// Runtime directory and file names (under the porcelain runtime root).
const (
	DirRun             = "run"
	DirData            = "data"
	DirConfig          = "config"
	FileLaunchLock     = "locus-desktop-launch.lock"
	FileLaunchMetadata = "locus-desktop-launch.json"
	FileLifecycleLog   = "locus-desktop-events.jsonl"
	FileSupervisorLog  = "locus-desktop-supervisor.log"
)

// Launcher-only CLI flags (stripped before execing chimera-supervisor).
const (
	FlagLogDir        = "-log-dir"
	FlagLogDirLong    = "--log-dir"
	FlagHeadless      = "--headless"
	FlagHeadlessShort = "-headless"
	LegacyDesktopArg  = "desktop"
)

// Webview JavaScript bridge names (gateway operator UI calls these on window / window.top).
const (
	BridgePickFolder        = "chimeraPickFolder"
	BridgeOpenExternalURL   = "chimeraOpenExternalURL"
	BridgeRevealProjectPath = "chimeraRevealProjectPath"
)

// Network and UI defaults.
const (
	DefaultSupervisorListen = "127.0.0.1:7710"
	DefaultLoginNextPath    = "/ui/logs?focus=admin"
	WindowTitle             = "Locus"
)

// LogPrefix is prepended to locus-desktop stderr lines.
const LogPrefix = BinDesktop + ":"

// SupervisorSearchNames returns platform-ordered basenames for chimera-supervisor lookup.
func SupervisorSearchNames() []string {
	return binfind.SearchNames(BinSupervisor)
}

// RunDirPath returns <runtimeRoot>/run.
func RunDirPath(runtimeRoot string) string {
	return filepath.Join(runtimeRoot, DirRun)
}

// LaunchLockPath returns the single-instance launch lock file path.
func LaunchLockPath(runtimeRoot string) string {
	return filepath.Join(RunDirPath(runtimeRoot), FileLaunchLock)
}

// LaunchMetadataPath returns the launch diagnostics JSON path.
func LaunchMetadataPath(runtimeRoot string) string {
	return filepath.Join(RunDirPath(runtimeRoot), FileLaunchMetadata)
}

// LifecycleEventsPath returns the optional trace JSONL path.
func LifecycleEventsPath(runtimeRoot string) string {
	return filepath.Join(RunDirPath(runtimeRoot), FileLifecycleLog)
}

// SupervisorLogPath returns the supervisor log file path under logDir.
func SupervisorLogPath(logDir string) string {
	return filepath.Join(logDir, FileSupervisorLog)
}

// Logf writes a prefixed line to stderr.
func Logf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, LogPrefix+" "+format, args...)
}

// Logln writes a prefixed line to stderr.
func Logln(args ...any) {
	fmt.Fprint(os.Stderr, LogPrefix+" ")
	fmt.Fprintln(os.Stderr, args...)
}
