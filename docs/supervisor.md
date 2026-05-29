# Supervised stack (`chimera-supervisor`)

One command starts the **Chimera wrapper stack**: optional `chimera-vectorstore`, `chimera-broker`, `chimera-gateway`, and optional `chimera-indexer`. Each managed service is a **wrapper binary** that supervises its upstream backend behind a shared contract (health, readiness, lifecycle, structured logs). **SIGINT** / **SIGTERM** triggers graceful shutdown of the control plane and managed children.

**Platform contracts (read before adding binaries):**

- [Wrapper binary contract](features/chimera-wrapper-binary-contract.md)
- [Structured log lines](features/structured-operator-log-lines.md)
- [Product naming](features/product-naming-contract.md)
- [Locus desktop ↔ supervisor](features/locus-desktop-supervisor.md) (desktop launcher)

## Runtime layout

| Piece | Role |
|-------|------|
| **`chimera-supervisor`** | Parent orchestrator; control HTTP plane (`/healthz`, `/readyz`, `/status`, `/shutdown`); tees child logs into `servicelogs` ring buffer. |
| **`chimera-vectorstore`** | Wrapper around upstream vector store (default `qdrant` via `--bin`); exposes Chimera listen addr; translates `VECTORSTORE__*` env. |
| **`chimera-broker`** | Wrapper around upstream LLM broker (BiFrost `bifrost-http` via `--bin`); exposes Chimera listen addr; translates `BROKER__*` env. |
| **`chimera-gateway`** | Wrapper around gateway backend process; operator UI at `/ui/*`; merges config, RAG, chat, operator SQLite. |
| **`chimera-indexer`** (optional) | Workspace file watcher; polls gateway for SQLite workspace list; ingest via gateway APIs. |

The supervisor **does not** exec upstream `qdrant` or `bifrost-http` directly — only Chimera wrapper binaries. Upstream paths are passed to wrappers as `--bin`.

Gateway upstream URL is wired to the supervised broker endpoint. When `rag.enabled` is true, the gateway uses the supervised vector store.

## Obtaining binaries

```bash
make chimera-install    # BiFrost + Qdrant upstream + build wrappers
make chimera-supervisor-build
make chimera-supervisor-run   # foreground stack
```

Full desktop path: `make up` (install, build, run `locus-desktop`). See [installation.md](installation.md) and [packaging.md](packaging.md).

Provider keys (`GROQ_API_KEY`, `GEMINI_API_KEY`, …) and `CHIMERA_BROKER_API_KEY` are read from the **supervisor process environment** and inherited by children.

## Common supervisor flags

Run `./chimera-supervisor -h` for the full list. Typical overrides:

| Flag | Meaning |
|------|---------|
| `-listen` | Supervisor control plane bind (desktop derives webview base from this) |
| `-gateway-bin` | Path to `chimera-gateway` wrapper |
| `-broker-bin` | Path to `chimera-broker` wrapper |
| `-vectorstore-bin` | Path to `chimera-vectorstore` wrapper |
| `-indexer-bin` | Path to `chimera-indexer` (when supervised indexer enabled) |
| `-config` | Gateway YAML path (default `config/gateway.yaml`) |

Wrapper-specific tuning uses `GATEWAY__*`, `BROKER__*`, `VECTORSTORE__*` env vars — see [product naming contract](features/product-naming-contract.md).

## Make targets

- `make chimera-install` — toolchain + upstream deps per `chimera/deps.lock`
- `make chimera-supervisor-run` — foreground supervised stack
- `make chimera-start` / `make chimera-stop` / `make chimera-status` — background lifecycle (`data/chimera-supervisor/`)
- `make locus-desktop-run` — desktop shell (connect-first to supervisor)

## Logs and operator UI

Child stdout/stderr pass through per-service `*line` normalizers into the in-process log buffer consumed by **`/ui/settings`**. Log JSON shape is defined in [structured operator log lines](features/structured-operator-log-lines.md).

After login: app shell **`/ui`**, settings and event log **`/ui/settings`**. See [README.md](README.md).

## Manual checklist

1. `make chimera-install` and `make chimera-supervisor-build`.
2. Run `make chimera-supervisor-run`; confirm supervisor `/readyz` and gateway `/health`.
3. Open `/ui/login` (or use `locus-desktop`).
4. SIGINT the supervisor; confirm wrapper children exit (no orphan upstream processes).

## Related docs

- [configuration.md](configuration.md) — gateway YAML, reload
- [indexer.md](indexer.md) — indexer operator guide
- [network.md](network.md) — ports and traffic flow
- Historical delivery: [`plans/vectorstore-broker-wrapper-hard-cut.md`](plans/vectorstore-broker-wrapper-hard-cut.md)
