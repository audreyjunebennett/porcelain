#!/usr/bin/env bash
# Deprecated name — use scripts/chimera-install-bootstrap.sh or make chimera-install.
exec "$(cd "$(dirname "$0")" && pwd)/chimera-install-bootstrap.sh" "$@"
