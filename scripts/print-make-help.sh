#!/usr/bin/env bash
# Printed by `make help` so Windows/PowerShell/cmd do not mangle quotes or `echo`/printf handling.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
# shellcheck source=scripts/chimera-names.sh
source "$ROOT/scripts/chimera-names.sh"

echo "Chimera (Go) - README order (primary flow: make up = install -> build -> background stack)"
echo
echo "  make up                 configure + install + build + run"
echo
echo "  make configure          copy config/gateway.example.yaml -> config/gateway.yaml if missing"
echo "  make install            ${CHIMERA_MAKE_INSTALL_TARGET} + desktop-install"
echo "  make build              ${CHIMERA_MAKE_BUILD_TARGET} + desktop-build"
echo "  make run                Starts ${CHIMERA_GATEWAY_BIN_BASE} + BiFrost + Qdrant + Desktop"
echo "  make package            packages all binaries to dist/personal/: desktop ${CHIMERA_GATEWAY_BIN_BASE} + bifrost-http + qdrant + config"
echo
echo "  make catalog-free                 fetch free tier models from pricing docs on web -> config/free-tier-catalog.snapshot.yaml"
echo "  make catalog-available            GET BiFrost /v1/models -> config/catalog-available.snapshot.yaml"
echo "  make config-provider-free-tier    calculate intersection of free and available models -> config/provider-free-tier.generated.yaml"
echo
echo "  make ${CHIMERA_MAKE_INSTALL_TARGET}    verify toolchain + bootstrap BiFrost/Qdrant from deps.lock (idempotent)"
echo "  make ${CHIMERA_MAKE_BUILD_TARGET}      go build -o ${CHIMERA_GATEWAY_BIN_BASE} ./${CHIMERA_CMD_GATEWAY} (headless; no CGO)"
echo "  make ${CHIMERA_MAKE_RUN_TARGET}        go run ./${CHIMERA_CMD_GATEWAY}"
echo
echo "  make desktop-install    native deps for WebView + CGO (Debian/Ubuntu, macOS CLT, Windows hints)"
echo "  make desktop-build      go build -tags desktop -> ./${CHIMERA_DESKTOP_BIN_BASE}[.exe] (CGO required)"
echo "  make desktop-run        desktop-build if missing, then ${CHIMERA_DESKTOP_BIN_BASE} (supervisor + UI; --headless for no window)"
echo
echo "  make indexer-build      go build -o ${CHIMERA_INDEX_BIN_BASE}[.exe] ./${CHIMERA_CMD_INDEXER} (workspace file indexer; v0.2)"
echo "  make indexer-run        go run ./${CHIMERA_CMD_INDEXER} (pass flags via ARGS=...)"
echo "  make indexer-install    go install ./${CHIMERA_CMD_INDEXER}"
echo
echo "  make release-install    goreleaser v2 (go install) + curl/tar/unzip for Qdrant packaging hook"
echo "  make release-snapshot   local goreleaser snapshot -> dist/ (GitHub uses .github/workflows/release.yml on v* tags)"
echo
echo "  make ${CHIMERA_MAKE_SERVE_TARGET}      foreground: go run serve + ./bin/bifrost-http + ./bin/qdrant"
echo "  make ${CHIMERA_MAKE_START_TARGET}      background ./${CHIMERA_GATEWAY_BIN_BASE} serve (UP_STACK=0 omits Qdrant); $(chimera_log_path), $(chimera_pid_path)"
echo "  make ${CHIMERA_MAKE_STATUS_TARGET}     PID file + HTTP probes (gateway / BiFrost / Qdrant)"
echo "  make ${CHIMERA_MAKE_STOP_TARGET}       stop background supervisor from $(chimera_pid_path)"
echo "  make logs               tail background ${CHIMERA_GATEWAY_BIN_BASE} (make ${CHIMERA_MAKE_START_TARGET}) $(chimera_log_path)"
echo
echo "  make test                    all test-* targets; omit desktop: SKIP_DESKTOP=1 (-race on Unix)"
echo "  make test-internal           go test ./internal/..."
echo "  make ${CHIMERA_MAKE_TEST_GATEWAY_TARGET}            go test ./${CHIMERA_CMD_GATEWAY} (default tags)"
echo "  make test-desktop            go test -tags desktop ./${CHIMERA_CMD_GATEWAY} (CGO)"
echo "  make test-catalog-free       go test that cmd package"
echo "  make test-catalog-available  go test that cmd package"
echo
echo "  make fmt                gofmt -w cmd internal"
echo "  make fmt-check          fail if gofmt would change files"
echo "  make vet                vet-module + vet-desktop (omit desktop: SKIP_DESKTOP=1)"
echo "  make vet-module         go vet ./..."
echo "  make vet-desktop        go vet -tags desktop ./${CHIMERA_CMD_GATEWAY} (CGO)"
echo
echo "  make clean              remove ${CHIMERA_GATEWAY_BIN_BASE}[.exe], ${CHIMERA_INDEX_BIN_BASE}[.exe], ${CHIMERA_DESKTOP_BIN_BASE}[.exe], dist/"
echo "  make clean-all          remove clean + bin/ + packaging/qdrant-bundles + packages + node_modules + .deps + run + logs (CONFIRM=1)"
echo "  make clean-data         remove data/bifrost + data/qdrant + data/gateway (fresh BiFrost/Qdrant/metrics; needs CONFIRM=1)"
echo
echo "  make precommit          fmt-check, vet, test (SKIP_DESKTOP=1 skips desktop vet/test)"
echo "  make bash               interactive bash (-il); Windows: Git for Windows bash"
echo "  make tokencount-file    bytes + cl100k_base + o200k_base for FILE=path (go run tokencount -f)"
