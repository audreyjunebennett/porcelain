#!/usr/bin/env bash
# Supervisor log normalization fidelity (docs/plans/log-supervisor-normalization-fidelity.md).
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"
echo '[STEP] Log supervisor normalization fidelity'
go test ./chimera/internal/wrapper/line/... -count=1
go test ./chimera/internal/gatewayline/... -count=1
go test ./chimera/chimera-broker/internal/brokerline/... -count=1
go test ./chimera/chimera-vectorstore/internal/vectorstoreline/... -count=1
go test ./chimera/chimera-indexer/internal/indexerline/... -count=1
