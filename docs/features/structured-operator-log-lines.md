# Feature: Structured operator log lines

| Field | Value |
|-------|-------|
| **Doc kind** | `platform-contract` |
| **Areas** | `internal/wrapper/line`, `*line` normalizers, supervisor ingest, settings UI |
| **Status** | `current` |
| **Introduced** | Log presentation + supervisor fidelity trains (2026-05) |
| **Originated from** | [`plans/log-supervisor-normalization-fidelity.md`](../plans/log-supervisor-normalization-fidelity.md), [`plans/log-presentation-layer.md`](../plans/log-presentation-layer.md) |
| **Related features** | [Operator log message registry](operator-log-message-registry.md), [Operator settings UI](operator-settings-ui.md), [Chimera wrapper binary contract](chimera-wrapper-binary-contract.md) |
| **Depends on** | Wrapper stdout/stderr capture, `servicelogs` ring buffer |
| **Last updated** | See git history |

## At a glance

Every supervised service emits **one JSON object per log line** on stderr (or stdout where configured). Per-service `*line` packages rewrite raw upstream or process output into a **stable Chimera schema** with canonical field order, stable `msg` slugs, and `_chimera_norm: 1` marking. Supervisor ingest may run normalizers **twice** on child output; `ReorderNormalizedJSON` must preserve extension keys losslessly so structured attrs (`path`, `collection`, `queue_depth`, indexer job fields, …) survive into the operator log buffer and desktop mirror file.

## Operator-visible behavior

The settings event log and conversation timelines consume these normalized lines. Summary view uses [operator message registry](operator-log-message-registry.md) templates keyed by `msg`. Detailed view shows the full field grid. Bare `msg`-only rows after supervisor ingest indicate a **regression** in the normalization pipeline.

## System behavior and contracts

**Invariants**

- **One JSON object per line** — no multi-line pretty-print in production paths.
- **Canonical field order** — `timestamp`, `level`, `service`, `msg`, optional attrs, extension keys (sorted), `_chimera_norm` last.
- **`_chimera_norm: 1`** — line already normalized; downstream must not double-transform content.
- **Lossless reorder** — unknown/extension keys are preserved (sorted) after canonical fields; second-pass ingest must not drop attrs.
- **UTC timestamps** — normalized lines use RFC3339 UTC in `timestamp`.
- **Service identity** — `service` field uses Chimera component names (`chimera-gateway`, `indexer`, … per normalizer).

**Per-service normalizers**

| Package | Upstream / source |
|---------|-------------------|
| `gatewayline` | `chimera-gateway` backend process |
| `brokerline` | `chimera-broker` wrapper + BiFrost upstream |
| `vectorstoreline` | `chimera-vectorstore` wrapper + Qdrant upstream |
| `indexerline` | `chimera-indexer` |
| `supervisorline` | `chimera-supervisor` control plane |

**Decisions**

| Topic | Decision |
|-------|----------|
| Line writer API | `line.NewWriter(dst, normalize)` buffers until `\n`, then emits normalized bytes |
| Plain vs JSON input | Non-`{` lines go through plain normalizer; JSON through JSON normalizer |
| Desktop mirror | `data/locus-desktop-supervisor.log` receives same normalized JSON as supervisor buffer |
| UI wire format | Settings UI does not change log JSON shape — only presentation |

## Interfaces

| Surface | Detail |
|---------|--------|
| Writer hook | `*line.NewWriter(io.Writer, func(string) []byte)` |
| Reorder | `line.ReorderNormalizedJSON([]byte) []byte` |
| Slug catalog | `internal/operatorcopy/messages.yaml` → generated constants |
| Ingest | Supervisor `LogSink` tees child stdout/stderr through service `*line.NewWriter` into `servicelogs.Store` |

## Code map

| Concern | Location |
|---------|----------|
| Core line writer + reorder | `chimera/internal/wrapper/line/core.go`, `record.go` |
| Timestamps / upstream detail | `chimera/internal/wrapper/line/timestamp.go`, `upstream.go` |
| Gateway normalizer | `chimera/internal/gatewayline/writer.go` |
| Broker / vectorstore / indexer / supervisor | `chimera/chimera-broker/internal/brokerline/`, `chimera-vectorstore/internal/vectorstoreline/`, `chimera-indexer/internal/indexerline/`, `chimera-supervisor/internal/supervisorline/` |
| Supervisor tee + second pass | `chimera/chimera-supervisor/internal/supervise/` (`LogSink`) |
| Operator buffer | `chimera/internal/servicelogs/store.go` |
| Message registry | `internal/operatorcopy/` |

## Verification

```bash
go test ./chimera/internal/wrapper/line/...
go test ./chimera/internal/gatewayline/...
go test ./chimera/chimera-indexer/internal/indexerline/...
```

Table-driven tests include `TestNormalizePayloadSupervisorSecondPass` in gatewayline / brokerline / vectorstoreline packages.

## Out of scope and known gaps

- Log **presentation** copy and shapes — see [operator-log-message-registry](operator-log-message-registry.md).
- Conversation lifecycle slug spec — [`log-conversations`](../plans/log-conversations.md) (historical); behavior surfaced in settings UI feature.

## References

- Delivery plans: [`log-supervisor-normalization-fidelity.md`](../plans/log-supervisor-normalization-fidelity.md), [`log-presentation-layer.md`](../plans/log-presentation-layer.md)
- Settings consumer: [`operator-settings-ui.md`](operator-settings-ui.md)
