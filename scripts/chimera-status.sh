#!/usr/bin/env bash
# Report background supervisor PID and probe default HTTP endpoints (see config/gateway.yaml).
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"
# shellcheck source=scripts/chimera-names.sh
source "$ROOT/scripts/chimera-names.sh"

# Prefer CHIMERA_STATUS_*; keep CLAUDIA_STATUS_* for existing env overrides.
GW_PORT="${CHIMERA_STATUS_GW_PORT:-${CLAUDIA_STATUS_GW_PORT:-3000}}"
BF_PORT="${CHIMERA_STATUS_BF_PORT:-${CLAUDIA_STATUS_BF_PORT:-8080}}"
QD_PORT="${CHIMERA_STATUS_QD_PORT:-${CLAUDIA_STATUS_QD_PORT:-6333}}"

pidf="$(chimera_pid_path)"

echo "==> ${CHIMERA_MAKE_STATUS_TARGET}"
if [[ -f "$pidf" ]]; then
	pid="$(cat "$pidf")"
	if kill -0 "$pid" 2>/dev/null; then
		echo "    supervisor pid $pid: running"
	else
		echo "    $pidf present but process $pid: not running (stale)"
	fi
else
	echo "    background supervisor: not started (no $pidf; use make ${CHIMERA_MAKE_START_TARGET} or make up)"
fi

probe() {
	local name="$1" url="$2"
	local code
	if code=$(curl -fsS -o /dev/null -w "%{http_code}" --max-time 2 "$url" 2>/dev/null); then
		echo "    $name $url → HTTP $code"
	else
		echo "    $name $url → unreachable"
	fi
}

# Gateway /health: HTTP code + body (same as curl -sS http://127.0.0.1:<port>/health).
probe_gateway_health() {
	local url="http://127.0.0.1:${GW_PORT}/health"
	local tmp code body
	tmp="$(mktemp)"
	code=$(curl -sS --max-time 2 -o "$tmp" -w "%{http_code}" "$url" 2>/dev/null) || true
	if [[ -z "${code:-}" ]]; then
		echo "    Gateway  $url → unreachable"
		rm -f "$tmp"
		return
	fi
	body=$(tr -d '\r' <"$tmp" 2>/dev/null || true)
	rm -f "$tmp"
	echo "    Gateway  $url → HTTP $code"
	if [[ -n "$body" ]]; then
		printf '            %s\n' "$body"
	fi
}

probe_gateway_health
probe "BiFrost " "http://127.0.0.1:${BF_PORT}/health"
probe "Qdrant  " "http://127.0.0.1:${QD_PORT}/readyz"

echo "    URLs: http://127.0.0.1:${GW_PORT}  http://127.0.0.1:${BF_PORT}  http://127.0.0.1:${QD_PORT}"
