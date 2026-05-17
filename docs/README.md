# Chimera gateway runtime — operator documentation

**Install, run, configure:** documents in this directory (`docs/`).

**Roadmaps, release notes, and engineering plans:** [`plans/`](plans/README.md).

Current release line: **v0.2.x** — RAG, ingest, workspace indexer (`chimera-indexer`), supervised-stack option, operator `/ui/*`, desktop webview. Shipped patches (v0.2.0–v0.2.2): [version-v0.2.md — Shipped releases](version-v0.2.md#shipped-releases-v020-through-v022).

## Operator docs (`docs/`)

| Document | Description |
|----------|-------------|
| [overview.md](overview.md) | What the gateway and BiFrost do; stack capabilities |
| [network.md](network.md) | Local process layout, ports, traffic flow |
| [installation.md](installation.md) | Toolchains, `make chimera-install` / `make install`, BiFrost/Qdrant binaries, `chimera` build |
| [configuration.md](configuration.md) | Gateway config files, env vars, reload semantics |
| [supervisor.md](supervisor.md) | `chimera serve`: BiFrost subprocess + Go gateway |
| [packaging.md](packaging.md) | GoReleaser releases, artifacts, `chimera -version` |
| [migration-v0-3-naming.md](migration-v0-3-naming.md) | Operator migration map for v0.3 naming contracts |
| [bifrost-discovery.md](bifrost-discovery.md) | BiFrost compatibility matrix and discovery notes |
| [design.md](design.md) | Design notes for the gateway service |
| [indexer.md](indexer.md) | `chimera-indexer` operator guide |
| [gui-testing.md](gui-testing.md) | Desktop webview (`-tags desktop`), manual checklist, build deps |
| [operator-migration-to-go.md](operator-migration-to-go.md) | Notes for operators coming from the legacy stack |
| [../SECURITY.md](../SECURITY.md) | Tokens, logging redaction, local attack surface |

Normative product requirements: [porcelain.plan.md](porcelain.plan.md).

## Plans & versions (`docs/plans/`)

See [plans/README.md](plans/README.md) for version milestones, release write-ups, Makefile/tooling plans, indexer roadmap, and UI/log presentation plans.
