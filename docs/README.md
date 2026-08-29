# Chimera operator documentation

Documentation is split by **purpose**. Start with [`CURRENT.md`](CURRENT.md) for authority and the current release line, then follow the link that matches your task.

```
docs/
├── README.md              ← you are here (index)
├── CURRENT.md             ← authority map + current release
├── chimera.plan.md        product vision + normative requirements
├── design.md              north-star routing architecture (future depth)
├── installation.md …      runbooks (install, configure, network, supervisor)
├── indexer.md             chimera-indexer operator quick start
├── version-v0.*.md        release-train roadmaps
├── features/              as-built behavior (source of truth for agents)
├── plans/                 open work (draft / active only)
│   └── archive/           shipped delivery history
└── reference/             stable integration references (BiFrost, …)
```

**Current release line: v0.3.3** — branding, operator `/ui/*`, virtual models, workspace indexer, RAG, and `chimera-supervisor` wrapper stack. Active train notes: [version-v0.3.md](version-v0.3.md). Earlier patches: [version-v0.2.md — Shipped releases](version-v0.2.md#shipped-releases-v020-through-v022).

**Operator UI:** after login, app shell at `/ui`; configuration and live logs at **`/ui/settings`**. JSON/SSE APIs under `/api/ui/*`.

---

## Runbooks — install, run, configure

| Document | Description |
|----------|-------------|
| [installation.md](installation.md) | Toolchains, `make chimera-install`, wrapper builds, first run |
| [supervisor.md](supervisor.md) | `chimera-supervisor` and the wrapper stack |
| [network.md](network.md) | Process layout, ports, traffic flow |
| [configuration.md](configuration.md) | Gateway config files, env vars, reload semantics |
| [packaging.md](packaging.md) | GoReleaser releases, artifacts, `chimera -version` |
| [indexer.md](indexer.md) | `chimera-indexer` operator quick start |
| [../SECURITY.md](../SECURITY.md) | Tokens, logging redaction, local attack surface |

Naming hard-cut notes live in the feature record [product-naming-contract](features/product-naming-contract.md) and archived plan [v0-3-naming-migration](plans/archive/v0-3-naming-migration.md).

---

## As-built — feature records

**[`features/README.md`](features/README.md)** — platform contracts (wrappers, naming, log lines, chat pipeline) and operator features (UI, indexer, virtual models, RAG). Use these when extending or debugging shipped behavior.

---

## Product requirements and vision

| Document | Description |
|----------|-------------|
| [CURRENT.md](CURRENT.md) | What is authoritative now; current release; agent summary |
| [chimera.plan.md](chimera.plan.md) | Normative product requirements and roadmap pointers |
| [design.md](design.md) | Cognitive routing north star (not all shipped) |
| [reference/bifrost-upstream.md](reference/bifrost-upstream.md) | BiFrost backend behind `chimera-broker` |
| [reference/tokencount-notes.md](reference/tokencount-notes.md) | Token estimate trade-offs (design discussion) |

---

## Release trains

| Version | Doc | Status |
|---------|-----|--------|
| v0.1 | [version-v0.1.md](version-v0.1.md) | shipped |
| v0.1.1 | [version-v0.1.1.md](version-v0.1.1.md) | shipped |
| v0.2 | [version-v0.2.md](version-v0.2.md) | shipped |
| **v0.3 (current, through 0.3.3)** | [version-v0.3.md](version-v0.3.md) | active |
| v0.4 (future) | [version-v0.4.md](version-v0.4.md) | draft |
| v0.5 (future) | [version-v0.5.md](version-v0.5.md) | draft |

Template for new version docs: [_version-template.md](_version-template.md).

---

## Plans — open work and archive

**[`plans/README.md`](plans/README.md)** — **Active** and **Draft** plans agents can pick up. Completed plans live under [`plans/archive/`](plans/archive/README.md). Plans preserve *why* and *how*; feature records preserve *what must stay true*.
