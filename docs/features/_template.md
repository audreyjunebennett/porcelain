# Feature: Short title (under ~10 words)

| Field | Value |
|-------|-------|
| **Doc kind** | `feature-record` or `platform-contract` |
| **Areas** | Gateway embed UI, operator SQLite, … |
| **Status** | `current` |
| **Introduced** | e.g. gateway v0.2, PR #… |
| **Originated from** | [`plans/example-plan.md`](../plans/example-plan.md) |
| **Related features** | None |
| **Depends on** | Session auth, operator SQLite, … |
| **Last updated** | See git history |

## At a glance

Two or three sentences describing what exists **now**, in plain language. Lead with observable operator or system behavior—not implementation details, file paths, or acronyms unless the operator sees them in the UI.

## Operator-visible behavior

What the operator sees and can do after this feature ships:

- Routes, panels, commands, and default UX choices.
- What **New chat**, reload, or navigation do to persisted state.

## System behavior and contracts

**Invariants** — rules future changes must preserve:

- …

**Decisions** — distilled from the plan and follow-up implementation chats:

| Topic | Decision |
|-------|----------|
| … | … |

**Identity / auth / scoping** (omit section if not applicable)

- Who owns data; how sessions, principals, or tenants scope access.

**Persistence** (omit section if not applicable)

- Stores, retention, delete semantics—summary level unless a column or invariant is critical.

## Interfaces

Only surfaces adjacent features or agents need when modifying this area:

| Surface | Detail |
|---------|--------|
| HTTP routes | `GET /api/ui/…` — purpose |
| Headers | `X-Chimera-…` — meaning |
| Config | `gateway.yaml` keys |
| Events / SSE | … |

## Code map

| Concern | Location |
|---------|----------|
| UI | `chimera/chimera-gateway/internal/server/adminui/embed/embedui/…` |
| API handlers | `internal/server/adminui/…` |
| Store / migrations | `internal/operatorstore/…`, `migrations/chimera-gateway/operator/` |
| Tests | `embedui_test/…` |

Keep this table small—entry points agents should open first, not an exhaustive file list.

## Verification

How to confirm behavior without re-reading the full implementation:

- `go test ./chimera/chimera-gateway/internal/server/adminui/embed/embedui_test -run …`
- Manual: open `/ui/…`, …

## Out of scope and known gaps

- Deferred behavior (link to plan **Future work** when one exists).
- Known limitations agents should not “fix” without explicit intent.

## References

- Delivery plan (historical): [`plans/example-plan.md`](../plans/example-plan.md)
- Operator docs (run/configure): [`configuration.md`](../configuration.md)
- PRs / commits: …

---

## Authoring notes (delete before publishing)

These notes are the authoring contract for new feature records. Copy this file into `docs/features/` (same folder only — no subfolders), rename it, and fill it in. Do not invent a different layout.

**Workflow.** Create a feature record when a capability expresses durable system behavior worth preserving for future agents—not for every bug fix or small refactor. Typical triggers:

1. A plan in `docs/plans/` reached **`shipped`** — distill behavior, contracts, and code map from the plan, git history, and any follow-up chat refinements.
2. Chat-only requirements arc — skip the plan; write the feature record directly when behavior is stable.

After publishing, link the plan (if any) to this doc via **As-built** in the plan front-matter table, and add a row to the appropriate section in [`README.md`](README.md) (Platform contracts or Operator features).

**File name.** Lower-case, hyphenated — e.g. `docs/features/operator-conversation-history.md`. The H1 (`# Feature: …`) should match the file name’s intent.

**Required structure (in order)**

1. **`# Feature: <title>`** — short, names the capability (not the delivery effort).
2. **Front-matter table** — use the labels in this template. **`Doc kind`:** `feature-record` (operator-visible product behavior) or `platform-contract` (wrapper/binary/integration contracts for extenders). **`Status`:** one of `current` · `partial` · `deprecated`.
3. **`## At a glance`** — immediately after the table: what exists now (past tense / present state), not future intent.
4. **Behavior and contracts** — operator-visible behavior, invariants, decisions, scoping, persistence (when relevant).
5. **`## Interfaces`** and **`## Code map`** — what agents need to navigate the codebase.
6. **`## Verification`**, **`## Out of scope`**, **`## References`**.

**Relationship to plans**

| | Plan (`docs/plans/`) | Feature record (`docs/features/`) |
|--|---------------------|-----------------------------------|
| Question | What will we build? | What exists and what must stay true? |
| Lifecycle | `draft` → `active` → `shipped` | `current` / `partial`; update when behavior changes |

Plans are delivery history. Feature records are the **source of truth for as-built behavior**. Do not copy phase deliverables wholesale—lift decisions, invariants, interfaces, and entry points.

**Don’t**

- Add YAML front matter — metadata stays in the Markdown table.
- Move **At a glance** below the title or bury it under long background prose.
- Duplicate operator install/run docs — link to `docs/supervisor.md`, `docs/configuration.md`, etc.
- Ship this **Authoring notes** section — delete it before publishing.

**At a glance is the contract.** Anyone skimming the doc (human or agent) should learn what the feature does and its key rules from that section plus **System behavior and contracts**.
