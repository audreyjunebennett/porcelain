# Engineering plans

**Plans** describe what to build and track delivery phases. They are **not** the source of truth for current behavior.

**As-built behavior** lives in [`docs/features/`](../features/README.md).  
**Authority map / current release:** [`docs/CURRENT.md`](../CURRENT.md).

| | Open plans (`docs/plans/`) | Archived plans (`docs/plans/archive/`) | Feature records |
|--|---------------------------|----------------------------------------|-----------------|
| Question | What should we build next? | Why / how was this delivered? | What exists and must stay true? |
| Lifecycle | `draft` → `active` | `shipped` / `done` / `superseded` | `current` / `partial` / `deprecated` |
| Agent use | **Implement from these** | Rationale only | Behavior, invariants, routes, code map |

**New plan:** copy [`_template.md`](_template.md), rename, fill in, delete authoring notes. Keep it here while `draft` or `active`. When it ships, set status, add **As-built**, and move the file to [`archive/`](archive/).

---

## Active

Work in progress. Prefer these (and feature records) over version-doc theme tables.

| Plan | Feature record(s) / notes |
|------|---------------------------|
| [operator-embed-ui-mobile-layout.md](operator-embed-ui-mobile-layout.md) | [operator-embed-ui-mobile-layout](../features/operator-embed-ui-mobile-layout.md) (`active` — Phases 1–3 shipped) |
| [operator-workspace-search.md](operator-workspace-search.md) | [operator-workspace-search](../features/operator-workspace-search.md) (`partial` — wizard step still open) |
| [moto-x-roadmap.md](moto-x-roadmap.md) | [receiver manager](../features/moto-x-receiver-manager.md), [review and corrections](../features/moto-x-review-and-corrections.md) |

---

## Draft — future work

No complete feature record yet (or exploration only). Create or update a feature record when implementation ships.

| Plan | Summary |
|------|---------|
| [indexer-manifest-ingest.md](indexer-manifest-ingest.md) | Manifest-only ingest, line-number snippets |
| [env-precedence-contract.md](env-precedence-contract.md) | Unified env/config precedence |
| [operator-cli.md](operator-cli.md) | `chimeractl` / operator CLI |

---

## Archive

**[archive/](archive/)** holds every completed plan (45 files). Indexes below are for discovery; the files themselves are historical.

### With feature records

| Plan | Feature record(s) |
|------|-------------------|
| [indexer.md](archive/indexer.md) | [indexer](../features/indexer.md), [workspaces](../features/indexer-workspaces.md), [ingest pipeline](../features/indexer-ingest-pipeline.md), [health/logs](../features/indexer-health-and-operator-logs.md), [gateway RAG](../features/gateway-rag-ingest-and-retrieval.md) |
| [indexer-workspaces-sqlite-gateway-api.md](archive/indexer-workspaces-sqlite-gateway-api.md) | [indexer-workspaces](../features/indexer-workspaces.md) |
| [indexer-workspaces-accurate-reporting.md](archive/indexer-workspaces-accurate-reporting.md) | [indexer-workspaces](../features/indexer-workspaces.md), [health/logs](../features/indexer-health-and-operator-logs.md) |
| [indexer-scan-and-fanout-jobs.md](archive/indexer-scan-and-fanout-jobs.md) | [indexer-ingest-pipeline](../features/indexer-ingest-pipeline.md) |
| [indexer-health-and-quiet-logs.md](archive/indexer-health-and-quiet-logs.md) | [indexer-health-and-operator-logs](../features/indexer-health-and-operator-logs.md) |
| [indexer-embedding-model-and-workspace-purge.md](archive/indexer-embedding-model-and-workspace-purge.md) | [indexer-workspaces](../features/indexer-workspaces.md), [indexer](../features/indexer.md) |
| [indexer-sync-state-sqlite-and-force-reindex.md](archive/indexer-sync-state-sqlite-and-force-reindex.md) | [indexer-ingest-pipeline](../features/indexer-ingest-pipeline.md) |
| [indexer-memory-usage-analysis.md](archive/indexer-memory-usage-analysis.md) | [indexer](../features/indexer.md) (analysis / mitigations) |
| [operator-chat-ui.md](archive/operator-chat-ui.md) | [operator-chat-ui](../features/operator-chat-ui.md) |
| [operator-conversation-history.md](archive/operator-conversation-history.md) | [operator-conversation-history](../features/operator-conversation-history.md), [session auth](../features/operator-ui-session-auth.md), [operator SQLite](../features/operator-sqlite-store.md) |
| [unified-logs-operator-shell.md](archive/unified-logs-operator-shell.md) | [operator-settings-ui](../features/operator-settings-ui.md), [ribbon](../features/operator-left-navigation-ribbon.md) |
| [embedui-operator-settings-routes.md](archive/embedui-operator-settings-routes.md) | [operator-settings-ui](../features/operator-settings-ui.md), [ribbon](../features/operator-left-navigation-ribbon.md) |
| [embedui-settings-card-cleanup.md](archive/embedui-settings-card-cleanup.md) | [operator-settings-ui](../features/operator-settings-ui.md) |
| [embedui-feed-log-service-split.md](archive/embedui-feed-log-service-split.md) | [operator-settings-ui](../features/operator-settings-ui.md) |
| [log-presentation-layer.md](archive/log-presentation-layer.md) | [operator-log-message-registry](../features/operator-log-message-registry.md), [settings UI](../features/operator-settings-ui.md) |
| [log-conversations.md](archive/log-conversations.md) | [operator-settings-ui](../features/operator-settings-ui.md) |
| [operator-message-registry.md](archive/operator-message-registry.md) | [operator-log-message-registry](../features/operator-log-message-registry.md) |
| [virtual-models-operator.md](archive/virtual-models-operator.md) | [operator-virtual-models](../features/operator-virtual-models.md), [gateway chat routing pipeline](../features/gateway-chat-routing-pipeline.md) |
| [remove-legacy-gateway-routing.md](archive/remove-legacy-gateway-routing.md) | [operator-virtual-models](../features/operator-virtual-models.md), [gateway chat routing pipeline](../features/gateway-chat-routing-pipeline.md) |
| [provider-model-availability.md](archive/provider-model-availability.md) | [operator-provider-model-availability](../features/operator-provider-model-availability.md) |
| [context-window-admission.md](archive/context-window-admission.md) | [context-window-admission](../features/context-window-admission.md) (`partial`) |
| [internal-embedding-provider.md](archive/internal-embedding-provider.md) | [internal embedding provider](../features/internal-embedding-provider.md) |
| [adminui-filesystem-dev-mode.md](archive/adminui-filesystem-dev-mode.md) | [operator-ui-filesystem-dev-mode](../features/operator-ui-filesystem-dev-mode.md) |

### Platform contracts

| Plan | Feature record(s) |
|------|-------------------|
| [vectorstore-broker-wrapper-hard-cut.md](archive/vectorstore-broker-wrapper-hard-cut.md) | [wrapper binary contract](../features/chimera-wrapper-binary-contract.md), [product naming](../features/product-naming-contract.md), [structured log lines](../features/structured-operator-log-lines.md) |
| [v0-3-naming-migration.md](archive/v0-3-naming-migration.md) | [product naming](../features/product-naming-contract.md) |
| [log-supervisor-normalization-fidelity.md](archive/log-supervisor-normalization-fidelity.md) | [structured log lines](../features/structured-operator-log-lines.md), [indexer health/logs](../features/indexer-health-and-operator-logs.md), [settings UI](../features/operator-settings-ui.md) |
| [locus-desktop-supervisor-contract.md](archive/locus-desktop-supervisor-contract.md) | [locus-desktop-supervisor](../features/locus-desktop-supervisor.md) (`partial`) |

### Plan only (infra / tooling / scaffolding)

| Plan | Notes |
|------|-------|
| [chimera-gateway-refactor.md](archive/chimera-gateway-refactor.md) | v0.3 gateway modularization |
| [chimera-gateway-package-boundaries.md](archive/chimera-gateway-package-boundaries.md) | Package layout |
| [desktop-ui.md](archive/desktop-ui.md) | Locus desktop shell (partially superseded by ribbon + settings) |
| [makefile.md](archive/makefile.md) | Build tooling |
| [upstream-llm-bifrost.md](archive/upstream-llm-bifrost.md) | BiFrost upstream integration |
| Embed UI scaffolding | [embedui-component-system.md](archive/embedui-component-system.md), [embedui-component-gallery.md](archive/embedui-component-gallery.md), [embedui-theme-styleguide.md](archive/embedui-theme-styleguide.md), [embedui-dynamic-provider-cards.md](archive/embedui-dynamic-provider-cards.md), [embedui-event-log-panel.md](archive/embedui-event-log-panel.md), [embedui-logs-workspaces-merge.md](archive/embedui-logs-workspaces-merge.md) |
| Log plumbing | [log-gateway.md](archive/log-gateway.md), [log-bifrost.md](archive/log-bifrost.md), [log-qdrant.md](archive/log-qdrant.md), [log-view-refactor.md](archive/log-view-refactor.md), [log-view-indexer.md](archive/log-view-indexer.md), [logs-ui-page-data-refreshing.md](archive/logs-ui-page-data-refreshing.md), [supervisor-info-log-trim.md](archive/supervisor-info-log-trim.md) |

### Superseded

| Plan | Superseded by |
|------|---------------|
| [logs-ui-maintainability.md](archive/logs-ui-maintainability.md) | [chimera-gateway-refactor.md](archive/chimera-gateway-refactor.md) Phases 5–6; as-built in [operator-settings-ui](../features/operator-settings-ui.md) |
