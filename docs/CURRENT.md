# Current documentation authority

Short map for humans and agents: **what is authoritative now**, what is historical, and where to pick up work.

| Question | Read |
|----------|------|
| What shipped and must stay true? | [`features/README.md`](features/README.md) |
| What can I implement next? | [`plans/README.md`](plans/README.md) — **Active** and **Draft** only |
| How do I install / run / configure? | [Runbooks in this folder](README.md#runbooks--install-run-configure) |
| Which release train are we on? | **v0.3.3** (see below) |
| Why was something built this way? | [`plans/archive/`](plans/archive/README.md) (delivery history) |
| Product vision / normative requirements | [`chimera.plan.md`](chimera.plan.md) (vision; verify against features before implementing) |
| Stable integration notes | [`reference/`](reference/) |

## Release line

**Current shipped line: v0.3.x** (through **v0.3.3**).

| Doc | Role | Status |
|-----|------|--------|
| [`version-v0.1.md`](version-v0.1.md) | Shipped train | `shipped` |
| [`version-v0.1.1.md`](version-v0.1.1.md) | Shipped train | `shipped` |
| [`version-v0.2.md`](version-v0.2.md) | Shipped train | `shipped` |
| [`version-v0.3.md`](version-v0.3.md) | Current train (setup wizard still open) | `active` |
| [`version-v0.4.md`](version-v0.4.md) | Future train | `draft` |
| [`version-v0.5.md`](version-v0.5.md) | Future train | `draft` |

Version docs are **release-train roadmaps**, not the day-to-day work queue. Prefer an **Active** or **Draft** plan (or a feature record for follow-up) when starting implementation. When a version theme has an execution plan, the plan’s status wins over a stale row in the version table.

## Document kinds

| Kind | Location | Lifecycle |
|------|----------|-----------|
| **Feature record** | `docs/features/` | As-built behavior; `current` / `partial` / `active` / `deprecated` |
| **Plan (open)** | `docs/plans/*.md` | Only `draft` or `active` work |
| **Plan (archive)** | `docs/plans/archive/` | `shipped` / `done` / `superseded` — do not treat as a to-do list |
| **Version roadmap** | `docs/version-v0.*.md` | Release themes and acceptance narrative |
| **Runbook** | `docs/*.md` (install, config, …) | Operator how-to |
| **Reference** | `docs/reference/` | Stable external/integration knowledge |

## Agent rules (summary)

1. Implement from a **feature record** when one exists; use archived plans only for rationale.
2. Treat **draft** plans as intent — verify the codebase before assuming behavior exists.
3. After shipping, update or create the feature record; move the plan to `plans/archive/` and set status `shipped` or `done`.
4. Do not invent work from archived plans or from version-doc rows whose linked plans are already archived.

See also [`.cursor/rules/docs-plans-vs-features.mdc`](../.cursor/rules/docs-plans-vs-features.mdc).
