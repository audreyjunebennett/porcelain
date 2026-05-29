# Feature: Operator SQLite store

| Field | Value |
|-------|-------|
| **Doc kind** | `platform-contract` |
| **Areas** | Gateway operator persistence, migrations, feature stores |
| **Status** | `current` |
| **Introduced** | Indexer workspaces Phase 1+ |
| **Originated from** | [`plans/indexer-workspaces-sqlite-gateway-api.md`](../plans/indexer-workspaces-sqlite-gateway-api.md), [`virtual-models-operator.md`](../plans/virtual-models-operator.md) |
| **Related features** | [Indexer workspaces](indexer-workspaces.md), [Operator virtual models](operator-virtual-models.md), [Operator provider model availability](operator-provider-model-availability.md), [Operator conversation history](operator-conversation-history.md) |
| **Depends on** | Gateway runtime path for `operator.sqlite` |
| **Last updated** | See git history |

## At a glance

Operator configuration and durable UI data live in a single **SQLite** database (`operator.sqlite`) opened by the gateway at startup. Versioned SQL migrations under `migrations/chimera-gateway/operator/` apply idempotently; the indexer **never** opens this file — it reads workspaces via HTTP. Feature packages use `internal/operatorstore` for typed CRUD (workspaces, virtual models, provider model availability, conversations).

## Operator-visible behavior

- Workspace rows, virtual models, provider toggles, and saved chat threads survive gateway restarts.
- Settings cards reflect DB state; saving does not rewrite `gateway.yaml` for workspaces or virtual models.
- First-run bootstrap may seed virtual models and provider availability from YAML (see store bootstrap helpers).

## System behavior and contracts

**Invariants**

- **Single writer** — Gateway process only; `MaxOpenConns(1)`, WAL mode, `busy_timeout=5000`, foreign keys on.
- **Migration filenames** — `000NNN_name.sql`; applied versions recorded in `operator_migrations`.
- **Tenant scoping** — Rows keyed by `tenant_id` / session principal for multi-tenant-ready schema (single-operator deployments use one tenant).
- **Indexer isolation** — Workspace paths and metadata are API-accessible only; no direct SQLite access from `chimera-indexer`.

**Tables (migration order)**

| Migration | Domain |
|-----------|--------|
| `000001_workspaces` | Indexer workspaces + paths |
| `000002_virtual_models` | Virtual model definitions and routing attachments |
| `000003_provider_model_availability` | Per-tenant upstream model toggles |
| `000004_conversation_history` | Chat threads, messages, RAG hit metadata |

**Decisions**

| Topic | Decision |
|-------|----------|
| Store API | `operatorstore.Open(path, migrationsDir, log)` |
| Bootstrap | `operatorstore` bootstrap imports legacy YAML VM stack on first run |
| Metrics DB | Separate SQLite under `migrations/chimera-gateway/metrics/` (not operator store) |

## Interfaces

| Surface | Detail |
|---------|--------|
| File | `operator.sqlite` (path from gateway runtime config) |
| Migrations | `migrations/chimera-gateway/operator/*.sql` |
| HTTP | Feature-specific `/api/ui/*` handlers call store methods |

## Code map

| Concern | Location |
|---------|----------|
| Open + migrations | `internal/operatorstore/store.go`, `migrate.go` |
| Workspaces | `internal/operatorstore/store.go` (workspace CRUD) |
| Virtual models | `internal/operatorstore/virtual_models.go` |
| Provider availability | `internal/operatorstore/provider_models.go`, `provider_models_bootstrap.go` |
| Conversations | `internal/operatorstore/conversations.go` |
| Runtime wiring | `internal/server/runtime/` |

## Verification

```bash
go test ./chimera/chimera-gateway/internal/operatorstore/...
```

## Out of scope and known gaps

- Indexer sync state — still JSON files (see [indexer.md](indexer.md)); not in operator SQLite.
- Planned SQLite sync checkpoints — [`indexer-sync-state-sqlite`](../plans/indexer-sync-state-sqlite-and-force-reindex.md) draft.

## References

- Workspace plan: [`indexer-workspaces-sqlite-gateway-api.md`](../plans/indexer-workspaces-sqlite-gateway-api.md)
- Virtual models plan: [`virtual-models-operator.md`](../plans/virtual-models-operator.md)
