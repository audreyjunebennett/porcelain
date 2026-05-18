#!/usr/bin/env bash
# Build bifrost-http from an existing BiFrost checkout (sourced by chimera-broker-bifrost-install.sh).
# Set BIFROST_SKIP_UI=1 to skip Next.js (npm) and embed a minimal ui/ stub — useful behind
# corporate TLS inspection where Google Fonts / npm fetches fail.
set -euo pipefail

_bifrost_stub_ui_dir() {
	local bifrost_dir="$1"
	printf '%s/transports/bifrost-http/ui\n' "$bifrost_dir"
}

bifrost_ensure_stub_ui() {
	local ui_dir
	ui_dir="$(_bifrost_stub_ui_dir "$1")"
	mkdir -p "$ui_dir"
	if [[ ! -f "$ui_dir/index.html" ]]; then
		cat >"$ui_dir/index.html" <<'EOF'
<!DOCTYPE html>
<html lang="en">
<head><meta charset="utf-8"><title>BiFrost</title></head>
<body><p>BiFrost HTTP API is running. The dashboard UI was not built (BIFROST_SKIP_UI=1).</p></body>
</html>
EOF
	fi
}

# bifrost_build_http BIFROST_DIR MAKE_BIN
bifrost_build_http() {
	local bifrost_dir="$1"
	local make_bin="$2"
	local skip_ui="${BIFROST_SKIP_UI:-}"

	if [[ "$skip_ui" == "1" ]]; then
		echo "==> Go workspace + bifrost-http build (BIFROST_SKIP_UI=1 — no Next.js UI)"
		bifrost_ensure_stub_ui "$bifrost_dir"
		"$make_bin" -C "$bifrost_dir" setup-workspace
		bifrost_build_http_go "$bifrost_dir"
		return 0
	fi

	echo "==> Go workspace + build in BiFrost (may run npm ci in ui/)"
	"$make_bin" -C "$bifrost_dir" setup-workspace
	"$make_bin" -C "$bifrost_dir" build LOCAL=1
}

# Native go build matching BiFrost Makefile (post build-ui); uses repo go.work from setup-workspace.
bifrost_build_http_go() {
	local bifrost_dir="$1"
	local version="${BIFROST_VERSION:-dev-build}"
	local target_os target_arch

	bifrost_ensure_stub_ui "$bifrost_dir"
	mkdir -p "$bifrost_dir/tmp"
	target_os="$(go env GOOS)"
	target_arch="$(go env GOARCH)"
	echo "    CGO go build -> tmp/bifrost-http ($target_os/$target_arch)"
	(
		cd "$bifrost_dir/transports/bifrost-http"
		CGO_ENABLED=1 GOOS="$target_os" GOARCH="$target_arch" go build \
			-ldflags="-w -s -X main.Version=v${version}" \
			-a -trimpath \
			-tags "sqlite_static" \
			-o "../../tmp/bifrost-http" \
			.
	)
}
