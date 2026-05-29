# Chimera operator documentation

Documentation is split by **purpose**. Start here, then follow the link that matches your task.

```
docs/
├── README.md              ← you are here (index)
├── chimera.plan.md        product vision + normative requirements
├── design.md              north-star routing architecture (future depth)
├── installation.md …      runbooks (install, configure, network, supervisor)
├── indexer.md             chimera-indexer operator quick start
├── version-v0.*.md        shipped release trains
├── features/              as-built behavior (source of truth for agents)
├── plans/                   delivery history and draft work
└── reference/             stable integration references (BiFrost, …)
```

Current release line: **v0.2.x+** with RAG, workspace indexer, operator `/ui/*`, virtual models, and `chimera-supervisor` wrapper stack. Patch history: [version-v0.2.md — Shipped releases](version-v0.2.md#shipped-releases-v020-through-v022).

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
| [migration-v0-3-naming.md](migration-v0-3-naming.md) | v0.3 naming and env key map |
| [indexer.md](indexer.md) | `chimera-indexer` operator quick start |
| [gui-testing.md](gui-testing.md) | Desktop webview (`-tags desktop`), manual checklist |
| [../SECURITY.md](../SECURITY.md) | Tokens, logging redaction, local attack surface |

---

## As-built — feature records

**[`features/README.md`](features/README.md)** — platform contracts (wrappers, naming, log lines, chat pipeline) and operator features (UI, indexer, virtual models, RAG). Use these when extending or debugging shipped behavior.

---

## Product requirements and vision

| Document | Description |
|----------|-------------|
| [chimera.plan.md](chimera.plan.md) | Normative product requirements and roadmap pointers |
| [design.md](design.md) | Cognitive routing north star (not all shipped) |
| [reference/bifrost-upstream.md](reference/bifrost-upstream.md) | BiFrost backend behind `chimera-broker` |
| [reference/tokencount-notes.md](reference/tokencount-notes.md) | Token estimate trade-offs (design discussion) |

---

## Release trains

| Version | Doc |
|---------|-----|
| v0.1 | [version-v0.1.md](version-v0.1.md) |
| v0.1.1 | [version-v0.1.1.md](version-v0.1.1.md) |
| v0.2 | [version-v0.2.md](version-v0.2.md) |
| v0.3 | [version-v0.3.md](version-v0.3.md) |
| v0.4 (future) | [version-v0.4.md](version-v0.4.md) |

Template for new version docs: [_version-template.md](_version-template.md).

---

## Plans — delivery history

**[`plans/README.md`](plans/README.md)** — shipped plans (with As-built links to features), drafts, and infra-only work. Plans preserve *why* and *how* something was built; feature records preserve *what must stay true*.
