package embed

import (
	"embed"
	"io/fs"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/lynn/porcelain/internal/naming"
)

//go:embed embedui/login.html embedui/settings.html embedui/settings.css embedui/ui.css embedui/theme-tokens.css embedui/embed-theme.js embedui/styles/* embedui/ui/* embedui/ui/*/* embedui/ui/vendor/* embedui/shared/* embedui/settings_app.js embedui/settings_entry.js embedui/settings/* embedui/settings/*/* embedui/index.html embedui/chat.html embedui/setup.html embedui/settings/gallery.html embedui/gallery/* embedui/chat/* embedui/chat/*/* embedui/shell/*
var embeddedFS embed.FS

type assetState struct {
	fromDisk bool
	root     string
}

var (
	assetsMu      sync.Mutex
	assetsCached  *assetState
	assetsEnv     string
	gatewayListen string
)

// SetGatewayListenAddr records the gateway HTTP listen address used to decide whether
// CHIMERA_ADMINUI_ROOT is allowed (loopback only). Call before serving operator UI routes.
func SetGatewayListenAddr(addr string) {
	assetsMu.Lock()
	defer assetsMu.Unlock()
	gatewayListen = strings.TrimSpace(addr)
	assetsCached = nil
	assetsEnv = ""
}

// ReadFile returns bytes for an operator UI asset (e.g. embedui/setup.html).
func ReadFile(name string) ([]byte, error) {
	st := activeAssets()
	if st.fromDisk {
		return fs.ReadFile(os.DirFS(st.root), name)
	}
	return embeddedFS.ReadFile(name)
}

// AssetsFromDisk reports whether CHIMERA_ADMINUI_ROOT is active.
func AssetsFromDisk() bool {
	return activeAssets().fromDisk
}

// DiskRoot returns the resolved filesystem root when AssetsFromDisk is true.
func DiskRoot() string {
	return activeAssets().root
}

func activeAssets() assetState {
	env := strings.TrimSpace(os.Getenv(naming.EnvAdminUIRoot))
	assetsMu.Lock()
	defer assetsMu.Unlock()
	if assetsCached != nil && env == assetsEnv {
		return *assetsCached
	}
	st := assetState{}
	if root, ok := resolveAdminUIRoot(env); ok {
		if listenAllowsFilesystemAssets() {
			st.fromDisk = true
			st.root = root
			if log := slog.Default(); log != nil {
				log.Info("serving operator UI assets from filesystem",
					"msg", "gateway.startup.adminui_filesystem",
					"root", root,
					"env", naming.EnvAdminUIRoot,
					"listen", effectiveGatewayListen(),
				)
			}
		} else {
			warnFilesystemAssetsRemoteBind(root, effectiveGatewayListen())
		}
	}
	assetsCached = &st
	assetsEnv = env
	return st
}

// resolveAdminUIRoot returns the directory for os.DirFS such that ReadFile("embedui/...") works.
func resolveAdminUIRoot(raw string) (string, bool) {
	if raw == "" {
		return "", false
	}
	abs, err := filepath.Abs(raw)
	if err != nil {
		warnInvalidAdminUIRoot(raw, err)
		return "", false
	}
	if root, ok := adminUIRootCandidate(abs); ok {
		return root, true
	}
	if strings.EqualFold(filepath.Base(abs), "embedui") {
		if root, ok := adminUIRootCandidate(filepath.Dir(abs)); ok {
			return root, true
		}
	}
	warnInvalidAdminUIRoot(raw, nil)
	return "", false
}

func adminUIRootCandidate(root string) (string, bool) {
	st, err := os.Stat(filepath.Join(root, "embedui", "settings.html"))
	if err != nil || st.IsDir() {
		return "", false
	}
	return root, true
}

func effectiveGatewayListen() string {
	if gatewayListen != "" {
		return gatewayListen
	}
	return strings.TrimSpace(os.Getenv(naming.EnvGatewayBackendListen))
}

func listenAllowsFilesystemAssets() bool {
	addr := effectiveGatewayListen()
	if addr == "" {
		return false
	}
	return isLoopbackListen(addr)
}

func isLoopbackListen(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return false
	}
	h := strings.Trim(strings.TrimSpace(host), "[]")
	return h == "127.0.0.1" || strings.EqualFold(h, "localhost") || h == "::1"
}

func warnFilesystemAssetsRemoteBind(root, listen string) {
	log := slog.Default()
	if log == nil {
		return
	}
	log.Warn("CHIMERA_ADMINUI_ROOT ignored: gateway listen is not loopback; using embedded operator UI assets",
		"msg", "gateway.startup.adminui_filesystem_remote_denied",
		"env", naming.EnvAdminUIRoot,
		"root", root,
		"listen", listen,
	)
}

func warnInvalidAdminUIRoot(raw string, err error) {
	log := slog.Default()
	if log == nil {
		return
	}
	args := []any{
		"msg", "gateway.startup.adminui_filesystem_ignored",
		"env", naming.EnvAdminUIRoot,
		"path", raw,
	}
	if err != nil {
		args = append(args, "err", err)
	}
	log.Warn("invalid CHIMERA_ADMINUI_ROOT; using embedded operator UI assets", args...)
}
