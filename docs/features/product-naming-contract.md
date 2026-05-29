# Feature: Product naming contract

| Field | Value |
|-------|-------|
| **Doc kind** | `platform-contract` |
| **Areas** | All `chimera-*` / `locus-*` binaries, docs, scripts, HTTP headers |
| **Status** | `current` |
| **Introduced** | v0.3 naming hard cut |
| **Originated from** | [`plans/v0-3-naming-migration.md`](../plans/v0-3-naming-migration.md) |
| **Related features** | [Chimera wrapper binary contract](chimera-wrapper-binary-contract.md) |
| **Depends on** | None |
| **Last updated** | See git history |

## At a glance

Porcelain uses **stable product names** for binaries, environment variables, HTTP headers, and config file basenames. Operator-facing surfaces say **Chimera** (gateway stack) and **Locus** (desktop workspace shell); upstream product names (`qdrant`, `bifrost-http`) are install/build artifacts only. Canonical constants live in `internal/naming/contracts.go` and are mirrored in `scripts/chimera-names.sh`. New binaries and integrations must import naming constants — not hard-code legacy names.

## Operator-visible behavior

Operators see Chimera names in the UI, logs, CLI help, and packaging. Config paths use `gateway.yaml`, `api-keys.yaml`, `routing-policy.yaml` under `config/`. Desktop binary is `locus-desktop`; supervised stack entrypoint is `chimera-supervisor`.

## System behavior and contracts

**Product binaries (public basenames)**

| Constant | Value | Role |
|----------|-------|------|
| `ProductSupervisorName` | `chimera-supervisor` | Stack orchestrator |
| `ProductDesktopName` | `locus-desktop` | Desktop shell |
| `ProductGatewayBinName` | `chimera-gateway` | Gateway wrapper |
| `ProductBrokerName` | `chimera-broker` | LLM broker wrapper |
| `ProductVectorstoreName` | `chimera-vectorstore` | Vector store wrapper |
| `ProductIndexerBinName` | `chimera-indexer` | Workspace indexer |
| `ProductQdrantBinName` | `qdrant` | Upstream vectorstore binary (wrapper `--bin`) |
| `ProductBifrostHTTPBinName` | `bifrost-http` | Upstream broker binary (install scripts) |

**Env prefix families**

| Prefix | Consumer |
|--------|----------|
| `GATEWAY__*` | `chimera-gateway` wrapper |
| `BROKER__*` | `chimera-broker` wrapper |
| `VECTORSTORE__*` | `chimera-vectorstore` wrapper |
| `CHIMERA_*` | Cross-stack targets (`CHIMERA_GATEWAY_URL`, `CHIMERA_GATEWAY_TOKEN`, `CHIMERA_GATEWAY_CONFIG`, `CHIMERA_BROKER_API_KEY`, `CHIMERA_SUPERVISOR_CONTROL_URL`, `CHIMERA_ADMINUI_ROOT`) |
| `LOCUS_DESKTOP_*` | Desktop trace/log dir |

**HTTP headers (`X-Chimera-*`)**

Includes `X-Chimera-Project`, `X-Chimera-Flavor-Id`, `X-Chimera-RAG-Hits`, `X-Chimera-Conversation-Id`, `X-Chimera-Upstream-Model`, `X-Chimera-Workspace-Id`, and related chat/indexer correlation headers — see `internal/naming/contracts.go` for the full list.

**Config / data layout**

| Item | Path / name |
|------|-------------|
| Gateway config | `config/gateway.yaml` |
| API keys | `config/api-keys.yaml` |
| Routing policy | `config/routing-policy.yaml` |
| Runtime data root | `data/` |
| Supervisor state | `data/chimera-supervisor/` |
| Indexer hidden state | `.locus/` (per-workspace sync files) |

**Invariants**

- Operator docs, UI labels, and supervisor-managed process names use **Chimera product names**.
- Upstream names appear in architecture/debug/error contexts only.
- Shell scripts and Make targets align with `scripts/chimera-names.sh`.

## Interfaces

| Surface | Detail |
|---------|--------|
| Go constants | `internal/naming/contracts.go` |
| Codegen | `internal/naming/cmd/gencontracts/` |
| Locus shared names | `internal/locus/res.go` (desktop runtime files, bridge names) |
| Migration map | [`migration-v0-3-naming.md`](../migration-v0-3-naming.md) |

## Code map

| Concern | Location |
|---------|----------|
| Canonical constants | `internal/naming/contracts.go` |
| Generated tests | `internal/naming/contracts_gen_test.go` |
| Shell mirror | `scripts/chimera-names.sh` |
| Desktop runtime names | `internal/locus/res.go` |

## Verification

```bash
go test ./internal/naming/...
```

## Out of scope and known gaps

- Unified env **precedence** across binaries — draft [`env-precedence-contract`](../plans/env-precedence-contract.md).
- Legacy YAML keys inside old config files may still use pre-cutover paths until migrated.

## References

- Delivery plan: [`v0-3-naming-migration.md`](../plans/v0-3-naming-migration.md)
- Operator migration: [`migration-v0-3-naming.md`](../migration-v0-3-naming.md)
- Wrapper env details: [`chimera-wrapper-binary-contract.md`](chimera-wrapper-binary-contract.md)
