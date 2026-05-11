#!/usr/bin/env bash
# Printed by `make help` so Windows/PowerShell/cmd do not mangle quotes or `echo`/printf handling.
set -euo pipefail
echo "Claudia (Go) - README order (primary flow: make up = install -> build -> background stack)"
echo
echo "  make up                 configure + install + build + run"
echo
echo "  make configure          copy config/gateway.example.yaml -> config/gateway.yaml if missing"
echo "  make install            claudia-install + desktop-install"
echo "  make build              claudia-build + desktop-build"
echo "  make run                Starts Claudia + BiFrost + Qdrant + Desktop"
echo "  make package            packages all binaries to dist/personal/: desktop claudia + bifrost-http + qdrant + config"
echo
echo "  make catalog-free                 fetch free tier models from pricing docs on web -> config/free-tier-catalog.snapshot.yaml"
echo "  make catalog-available            GET BiFrost /v1/models -> config/catalog-available.snapshot.yaml"
echo "  make config-provider-free-tier    calculate intersection of free and available models -> config/provider-free-tier.generated.yaml"
echo
echo "  make claudia-install    verify toolchain + bootstrap BiFrost/Qdrant from deps.lock (idempotent)"
echo "  make claudia-build      go build -o claudia ./cmd/claudia (headless; no CGO)"
echo "  make claudia-run        go run ./cmd/claudia"
echo
echo "  make desktop-install    native deps for WebView + CGO (Debian/Ubuntu, macOS CLT, Windows hints)"
echo "  make desktop-build      go build -tags desktop -> ./claudia-desktop[.exe] (CGO required)"
echo "  make desktop-run        desktop-build if missing, then claudia-desktop (supervisor + UI; --headless for no window)"
echo
echo "  make indexer-build      go build -o claudia-index[.exe] ./cmd/claudia-index (workspace file indexer; v0.2)"
echo "  make indexer-run        go run ./cmd/claudia-index (pass flags via ARGS=...)"
echo "  make indexer-install    go install ./cmd/claudia-index"
echo
echo "  make release-install    goreleaser v2 (go install) + curl/tar/unzip for Qdrant packaging hook"
echo "  make release-snapshot   local goreleaser snapshot -> dist/ (GitHub uses .github/workflows/release.yml on v* tags)"
echo
echo "  make claudia-serve      foreground: go run serve + ./bin/bifrost-http + ./bin/qdrant"
echo "  make claudia-start      background ./claudia serve (UP_STACK=0 omits Qdrant); logs/claudia.log, run/claudia.pid"
echo "  make claudia-status     PID file + HTTP probes (gateway / BiFrost / Qdrant)"
echo "  make claudia-stop       stop background supervisor from run/claudia.pid"
echo "  make logs               tail background claudia (make claudia-start) logs/claudia.log"
echo
echo "  make test                    all test-* targets; omit desktop: SKIP_DESKTOP=1 (-race on Unix)"
echo "  make test-internal           go test ./internal/..."
echo "  make test-claudia            go test ./cmd/claudia (default tags)"
echo "  make test-desktop            go test -tags desktop ./cmd/claudia (CGO)"
echo "  make test-catalog-free       go test that cmd package"
echo "  make test-catalog-available  go test that cmd package"
echo
echo "  make fmt                gofmt -w cmd internal"
echo "  make fmt-check          fail if gofmt would change files"
echo "  make vet                vet-module + vet-desktop (omit desktop: SKIP_DESKTOP=1)"
echo "  make vet-module         go vet ./..."
echo "  make vet-desktop        go vet -tags desktop ./cmd/claudia (CGO)"
echo
echo "  make clean              remove claudia[.exe], claudia-desktop[.exe], dist/"
echo "  make clean-all          remove clean + bin/ + packaging/qdrant-bundles + packages + node_modules + .deps + run + logs (+ Windows: %APPDATA%/claudia*.exe) (CONFIRM=1)"
echo "  make clean-data         remove data/bifrost + data/qdrant + data/gateway (fresh BiFrost/Qdrant/metrics; needs CONFIRM=1)"
echo "  make save-state         copy ./data -> temp/sessions/<timestamp>_<sha>/data (optional COMMENT=...)"
echo
echo "  make precommit          fmt-check, vet, test (SKIP_DESKTOP=1 skips desktop vet/test)"
echo "  make bash               interactive bash (-il); Windows: Git for Windows bash"
echo "  make tokencount-file    bytes + cl100k_base + o200k_base for FILE=path (go run tokencount -f)"
