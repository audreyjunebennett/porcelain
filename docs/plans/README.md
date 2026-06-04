# Engineering plans

**Plans** describe what to build, track delivery phases, and preserve rationale. They are **delivery history** — not the source of truth for current behavior.

**As-built behavior** lives in [`docs/features/`](../features/README.md). When a plan ships, add an **As-built** link in its front-matter table (see [`_template.md`](_template.md)).

| | Plan (`docs/plans/`) | Feature record (`docs/features/`) |
|--|---------------------|-----------------------------------|
| Question | What will we build? | What exists and what must stay true? |
| Lifecycle | `draft` → `active` → `shipped` / `done` | `current` / `partial` / `deprecated` |
| Agent use | Intent for **draft** / **active** work | Behavior, invariants, routes, code map |

**New plan:** copy [`_template.md`](_template.md), rename, fill in, delete authoring notes.

---

## Shipped — with feature records

These plans are complete. Read the linked feature record for as-built behavior.

| Plan | Feature record(s) |
|------|-------------------|
| [indexer.md](indexer.md) | [indexer](../features/indexer.md), [workspaces](../features/indexer-workspaces.md), [ingest pipeline](../features/indexer-ingest-pipeline.md), [health/logs](../features/indexer-health-and-operator-logs.md), [gateway RAG](../features/gateway-rag-ingest-and-retrieval.md) |
| [indexer-workspaces-sqlite-gateway-api.md](indexer-workspaces-sqlite-gateway-api.md) | [indexer-workspaces](../features/indexer-workspaces.md) |
| [indexer-workspaces-accurate-reporting.md](indexer-workspaces-accurate-reporting.md) | [indexer-workspaces](../features/indexer-workspaces.md), [health/logs](../features/indexer-health-and-operator-logs.md) |
| [indexer-scan-and-fanout-jobs.md](indexer-scan-and-fanout-jobs.md) | [indexer-ingest-pipeline](../features/indexer-ingest-pipeline.md) |
| [indexer-health-and-quiet-logs.md](indexer-health-and-quiet-logs.md) | [indexer-health-and-operator-logs](../features/indexer-health-and-operator-logs.md) |
| [operator-chat-ui.md](operator-chat-ui.md) | [operator-chat-ui](../features/operator-chat-ui.md) |
| [operator-conversation-history.md](operator-conversation-history.md) | [operator-conversation-history](../features/operator-conversation-history.md), [session auth](../features/operator-ui-session-auth.md), [operator SQLite](../features/operator-sqlite-store.md) |
| [unified-logs-operator-shell.md](unified-logs-operator-shell.md) | [operator-settings-ui](../features/operator-settings-ui.md), [ribbon](../features/operator-left-navigation-ribbon.md) |
| [embedui-operator-settings-routes.md](embedui-operator-settings-routes.md) | [operator-settings-ui](../features/operator-settings-ui.md), [ribbon](../features/operator-left-navigation-ribbon.md) |
| [log-presentation-layer.md](log-presentation-layer.md) | [operator-log-message-registry](../features/operator-log-message-registry.md), [settings UI](../features/operator-settings-ui.md) |
| [log-conversations.md](log-conversations.md) | [operator-settings-ui](../features/operator-settings-ui.md) |
| [operator-message-registry.md](operator-message-registry.md) | [operator-log-message-registry](../features/operator-log-message-registry.md) |
| [virtual-models-operator.md](virtual-models-operator.md) | [operator-virtual-models](../features/operator-virtual-models.md), [gateway chat routing pipeline](../features/gateway-chat-routing-pipeline.md) |
| [remove-legacy-gateway-routing.md](remove-legacy-gateway-routing.md) | [operator-virtual-models](../features/operator-virtual-models.md), [gateway chat routing pipeline](../features/gateway-chat-routing-pipeline.md) |
| [provider-model-availability.md](provider-model-availability.md) | [operator-provider-model-availability](../features/operator-provider-model-availability.md) |
| [context-window-admission.md](context-window-admission.md) | [context-window-admission](../features/context-window-admission.md) (`partial`) |
| [adminui-filesystem-dev-mode.md](adminui-filesystem-dev-mode.md) | [operator-ui-filesystem-dev-mode](../features/operator-ui-filesystem-dev-mode.md) |

### Platform contracts

| Plan | Feature record(s) |
|------|-------------------|
| [vectorstore-broker-wrapper-hard-cut.md](vectorstore-broker-wrapper-hard-cut.md) | [wrapper binary contract](../features/chimera-wrapper-binary-contract.md), [product naming](../features/product-naming-contract.md), [structured log lines](../features/structured-operator-log-lines.md) |
| [v0-3-naming-migration.md](v0-3-naming-migration.md) | [product naming](../features/product-naming-contract.md) |
| [log-supervisor-normalization-fidelity.md](log-supervisor-normalization-fidelity.md) | [structured log lines](../features/structured-operator-log-lines.md), [indexer health/logs](../features/indexer-health-and-operator-logs.md), [settings UI](../features/operator-settings-ui.md) |
| [locus-desktop-supervisor-contract.md](locus-desktop-supervisor-contract.md) | [locus-desktop-supervisor](../features/locus-desktop-supervisor.md) (`partial`) |

---

## Active

| Plan | Feature record(s) |
|------|-------------------|
| [operator-embed-ui-mobile-layout.md](operator-embed-ui-mobile-layout.md) | [operator-embed-ui-mobile-layout](../features/operator-embed-ui-mobile-layout.md) (`active` — Phases 1–3 shipped) |

---

## Draft — future work (no feature record yet)

Create a feature record when implementation ships.

| Plan | Summary |
|------|---------|
| [indexer-embedding-model-and-workspace-purge.md](indexer-embedding-model-and-workspace-purge.md) | Operator embedding model selector on indexer card; workspace delete drops vector collection |
| [operator-workspace-search.md](operator-workspace-search.md) | Direct workspace search API, `/ui/search`, ribbon nav |
| [embedui-settings-card-cleanup.md](embedui-settings-card-cleanup.md) | Settings feed and card component refactor for consistency |
| [embedui-feed-log-service-split.md](embedui-feed-log-service-split.md) | Split `feedLogService.js` into service-only + indexer modules; remove dead extraction code |
| [indexer-manifest-ingest.md](indexer-manifest-ingest.md) | Manifest-only ingest, line-number snippets |
| [indexer-memory-usage-analysis.md](indexer-memory-usage-analysis.md) | Idle ~1.2 GB RSS on Windows; pprof attribution and mitigations |
| [indexer-sync-state-sqlite-and-force-reindex.md](indexer-sync-state-sqlite-and-force-reindex.md) | SQLite sync checkpoints, force re-index |
| [operator-cli.md](operator-cli.md) | `chimeractl` operator CLI |
| [env-precedence-contract.md](env-precedence-contract.md) | Unified env/config precedence |
| [internal-embedding-provider-exploration.md](internal-embedding-provider-exploration.md) | Exploration — not a product feature |

---

## Shipped — plan only (no feature record)

Infra, refactor, tooling, or dev workflow. The plan remains the reference; no operator feature record is expected.

| Plan | Notes |
|------|-------|
| [adminui-filesystem-dev-mode.md](adminui-filesystem-dev-mode.md) | Filesystem embed UI dev mode — see [feature](../features/operator-ui-filesystem-dev-mode.md) |
| [chimera-gateway-refactor.md](chimera-gateway-refactor.md) | v0.3 gateway modularization |
| [chimera-gateway-package-boundaries.md](chimera-gateway-package-boundaries.md) | Package layout |
| [desktop-ui.md](desktop-ui.md) | Locus desktop shell (partially superseded by ribbon + settings) |
| [makefile.md](makefile.md) | Build tooling |
| [upstream-llm-bifrost.md](upstream-llm-bifrost.md) | BiFrost upstream integration |
| Embed UI scaffolding | [embedui-component-system.md](embedui-component-system.md), [embedui-component-gallery.md](embedui-component-gallery.md), [embedui-theme-styleguide.md](embedui-theme-styleguide.md), [embedui-dynamic-provider-cards.md](embedui-dynamic-provider-cards.md), [embedui-event-log-panel.md](embedui-event-log-panel.md), [embedui-logs-workspaces-merge.md](embedui-logs-workspaces-merge.md) |
| Log plumbing | [log-gateway.md](log-gateway.md), [log-bifrost.md](log-bifrost.md), [log-qdrant.md](log-qdrant.md), [log-view-refactor.md](log-view-refactor.md), [log-view-indexer.md](log-view-indexer.md), [logs-ui-page-data-refreshing.md](logs-ui-page-data-refreshing.md), [supervisor-info-log-trim.md](supervisor-info-log-trim.md) |

---

## Superseded

| Plan | Superseded by |
|------|---------------|
| [logs-ui-maintainability.md](logs-ui-maintainability.md) | [chimera-gateway-refactor.md](chimera-gateway-refactor.md) Phases 5–6; as-built in [operator-settings-ui](../features/operator-settings-ui.md) |
