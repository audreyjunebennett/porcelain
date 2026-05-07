# Plan: Short title (under ~10 words)

| Field | Value |
|-------|-------|
| **Doc kind** | `feature-plan` |
| **Owners / areas** | Gateway, indexer, desktop, … |
| **Status** | `draft` |
| **Targets** | e.g. gateway v0.4, indexer v0.6 |
| **Last updated** | See git history |
| **Supersedes / superseded by** | None |

## At a glance

One to three sentences in plain language. Lead with the value to the operator or end-user, not the implementation. Avoid acronyms when a normal phrase will do, and avoid words that only make sense to people already deep in the codebase.

| Phase | Outcome | Status |
|-------|---------|--------|
| [Phase 1 — Short phase title](#phase-1--short-phase-title) | One-line user-facing outcome | `todo` |
| [Phase 2 — Short phase title](#phase-2--short-phase-title) | One-line user-facing outcome | `todo` |
| [Phase 3 — Short phase title](#phase-3--short-phase-title) | One-line user-facing outcome | `todo` |

---

## Background

Why this work matters: the problem, the user pain, and any constraints worth knowing before reading the phases. Keep this section short — link to operator docs or related plans rather than re-explaining them.

**Related docs:** [`some-doc.md`](../some-doc.md), [`other-plan.md`](other-plan.md).

---

## Phase 1 — Short phase title

**Goal.** One sentence describing the user-visible outcome of this phase.

**Deliverables**

- Concrete artifacts: code, docs, scripts, configuration.
- Each bullet should be small enough to verify in a single PR.

**Acceptance**

- How a reviewer or operator can tell this phase is done.

**Status:** `todo`

---

## Phase 2 — Short phase title

**Goal.** …

**Deliverables**

- …

**Acceptance**

- …

**Status:** `todo`

---

## Phase 3 — Short phase title

**Goal.** …

**Deliverables**

- …

**Acceptance**

- …

**Status:** `todo`

---

## Open questions

Decisions still pending. Remove this section once everything is resolved.

1. …
2. …

---

## References

- Code: `internal/...`, `cmd/...`
- Docs: [`overview.md`](../overview.md), [`configuration.md`](../configuration.md)
- Tickets / PRs: …

---

## Authoring notes (delete before publishing)

These notes are the authoring contract for new plans. Copy this file into `docs/plans/` (same folder only — no subfolders), rename it, and fill it in. Do not invent a different layout.

**Workflow.** When asked to create, draft, start, or add a plan: copy [`_template.md`](_template.md) to a new file under `docs/plans/`, replace placeholders, then delete this section in the published doc. After creating a plan from chat, mention the new path so the operator can open it.

**File name.** Lower-case, hyphenated, no `.plan.md` suffix — e.g. `docs/plans/scoped-feature-name.md`. The H1 (`# Plan: …`) should match the file name’s intent.

**Required structure (in order)**

1. **`# Plan: <title>`** — short, action-oriented.
2. **Front-matter table** — use the labels in this template. **`Doc kind`:** one of `feature-plan` · `version-roadmap` · `refactor-plan` · `release-notes` · `working-notes` · `research/exploration`. **`Status`:** one of `draft` · `active` · `shipped` · `deferred` · `superseded`.
3. **`## At a glance`** — immediately after the table: one to three sentences focused on operator/user value (no jargon; avoid acronyms when a plain phrase works), then the phase status table. Each table row links to a real heading below; per-phase status values are `todo` · `active` · `done` · `deferred`.
4. **Detailed phase sections** — one `##` (or `###`) heading per phase row, same order as the table, each with **Goal**, **Deliverables**, **Acceptance**, and **Status**.

**Don’t**

- Add YAML front matter — metadata stays in the Markdown table.
- Move **At a glance** below the title or bury it under **Background**.
- Put step-by-step checkboxes in the at-a-glance table (detail belongs in each phase).
- Ship this **Authoring notes** section — delete it before publishing.

**Anchor links.** GitHub-flavored slugs: lowercase, drop punctuation, replace spaces with `-`. An em-dash surrounded by spaces becomes `--`. Example: `## Phase 2 — Project & flavor tagging` → `#phase-2--project--flavor-tagging`. Verify links resolve before finishing.

**At a glance is the contract.** Anyone skimming the doc should learn the goal, phases, and completion state from that section alone; detailed write-ups stay below.

**Keep phases at the phase level.** The status table tracks phases, not individual tasks.
