# Feature: Operator log message registry

| Field | Value |
|-------|-------|
| **Doc kind** | `feature-record` |
| **Areas** | Gateway logs UI, structured log emission, indexer/broker/vectorstore copy |
| **Status** | `current` |
| **Introduced** | 2026-05 operator copy refactor |
| **Originated from** | [`plans/operator-message-registry.md`](../plans/operator-message-registry.md) |
| **Related features** | [Operator settings UI](operator-settings-ui.md), [Indexer health and operator logs](indexer-health-and-operator-logs.md) |
| **Depends on** | `messages.yaml`, go generate pipeline, settings embed renderers |
| **Last updated** | See git history |

## At a glance

Structured log lines use stable **`msg` slugs** in Go (`slog` fields). The **operator message registry** (`internal/operatorcopy/messages.yaml`) is the single catalog for canonical slugs, legacy aliases, English summary templates, optional formatters, presentation **shapes**, and metrics counter hints. `go generate` validates YAML and emits `operator_copy.js` for the settings UI; Go codegen emits slug constants in `internal/naming/log_messages.go`. Settings renderers call `operatorMessage()` instead of scattered switch statements—changing operator-facing copy edits one YAML file.

## Operator-visible behavior

- **Summary column** — Event log and summarized cards show plain-language lines derived from registry `summary` / `formatter` / `append` rules.
- **Gallery previews** — Every slug requires `gallery_preview` for `/ui/settings/gallery` documentation rows.
- **Service-specific tone** — Gateway, conversation, broker, vectorstore, and indexer slugs share one registry; indexer-heavy lines use `operatorMessageIndexer.js` formatters where needed.
- **Shape-driven UI** — Tags like `http.access`, `chat.routing`, `rag`, `ingest` drive `inferShape` and card metric rollups.

## System behavior and contracts

**Invariants**

- Registry holds **copy + identity + presentation hints only** — not storage, normalize logic, or metrics DB schema.
- Canonical slug is the YAML `slug` key; `aliases` map legacy or mistyped emit strings.
- English only (`locale: en`).
- Emit sites should use generated Go constants (Phase 5), not raw string literals, for new code.
- Changing copy requires `go generate ./internal/operatorcopy/...` and `make contracts-check` in CI.

**Decisions**

| Topic | Decision |
|-------|----------|
| Canonical path | `internal/operatorcopy/messages.yaml` (embedded in Go) |
| JS output | `embed/embedui/settings/operator_copy.js` (generated) |
| Bootstrap | `bootstrap_registry.go` + `go run ./cmd/bootstrap` to expand catalog |
| Inventory | `scripts/operatorcopy-inventory.ps1` / `.sh` diff emit sites vs YAML |
| Drift fix | Broker `broker.*` and vectorstore `vectorstore.*` slugs aligned (legacy `chimera-broker.*` / `qdrant.*` aliases) |

**Persistence**

- YAML embedded at build time; no runtime DB.

## Interfaces

| Surface | Detail |
|---------|--------|
| Source | `internal/operatorcopy/messages.yaml` |
| Generated JS | `operator_copy.js` — `Slug`, `operatorMessage`, `inferShapeForFlat`, `metricsCounterForFlat` |
| Render entry | `settings/render/operatorMessage.js`, `operatorMessageServices.js`, `operatorMessageIndexer.js` |
| Go constants | `internal/naming/log_messages.go` (generated) |
| Commands | `go generate ./internal/operatorcopy/...`, `make contracts-generate`, `make contracts-check` |

## Code map

| Concern | Location |
|---------|------|
| Schema + load | `internal/operatorcopy/schema.go`, `load.go`, `embed.go` |
| Codegen | `internal/operatorcopy/genoperatorcopy/`, `genlogmessages/` |
| Validate CLI | `internal/operatorcopy/cmd/validate/` |
| Bootstrap | `internal/operatorcopy/bootstrap_registry.go`, `cmd/bootstrap/` |
| Inventory | `internal/operatorcopy/cmd/inventory/`, `inventory-report.txt` |
| UI wiring | `settings_app.js` (`primaryLogMessage`), `summarizedFeed.js` |
| Tests | `internal/operatorcopy/registry_test.go`, `embedui_test/operator_message_test.go` |

## Verification

```bash
go test ./internal/operatorcopy/...
go test ./chimera/chimera-gateway/internal/server/adminui/embed/embedui_test -run OperatorMessage
make contracts-check
```

Manual: edit a slug `summary` in YAML, regenerate, rebuild gateway, confirm settings log Summary column reflects new text.

## Out of scope and known gaps

- Non-English locales.
- Runtime-editable copy from UI (file + generate only).
- 100% slug coverage — inventory report tracks remaining string literals.

## References

- Plan: [`plans/operator-message-registry.md`](../plans/operator-message-registry.md)
- Presentation shapes: [`plans/log-presentation-layer.md`](../plans/log-presentation-layer.md)
- Package README: [`internal/operatorcopy/README.md`](../../internal/operatorcopy/README.md)
