# Version X.Y - Short title

| Field | Value |
|-------|-------|
| **Doc kind** | `version-roadmap` |
| **Owners / areas** | Gateway, desktop, indexer, docs, ... |
| **Status** | `draft` |
| **Targets** | Gateway vX.Y |
| **Last updated** | See git history |
| **Supersedes / superseded by** | Builds on [`version-vX.Y-1.md`](version-vX.Y-1.md) |

## At a glance

One to three short sentences that explain the value of this version for the operator or end user. Avoid implementation-first wording unless the implementation detail is what the operator experiences.

| Theme | Outcome | Status |
|-------|---------|--------|
| [Short title 1](#short-title-1) | One-line user-facing outcome | `todo` |
| [Short title 2](#short-title-2) | One-line user-facing outcome | `todo` |
| [Short title 3](#short-title-3) | One-line user-facing outcome | `todo` |

---

## What this version is

Describe the release in a paragraph or two: the problem it solves, the operator workflow it improves, and the boundaries that make this version distinct from the previous one.

**Companion docs:** [`claudia-gateway.plan.md`](claudia-gateway.plan.md), [`configuration.md`](configuration.md), [`plans/example.md`](plans/example.md).

---

## Short title

**Goal:** One sentence describing the outcome.

**Scope**

- Concrete product behavior, APIs, docs, or scripts included in this version.
- Keep this list at feature/theme level; detailed tasks belong in issues or implementation plans.

**Acceptance**

- How a reviewer or operator can tell this theme shipped.

**Status:** `todo`

---

## Short title

**Goal:** ...

**Scope**

- ...

**Acceptance**

- ...

**Status:** `todo`

---

## Short title

**Goal:** ...

**Scope**

- ...

**Acceptance**

- ...

**Status:** `todo`

---

## Explicitly not this version

- Work intentionally deferred to a later version.
- Compatibility or migration decisions that are out of scope for this release.

---

## Verification

| Area | Quick check |
|------|-------------|
| Feature/theme | Operator-visible check or command |
| Docs/config | Docs and examples updated |
| Tests | Test target or manual verification path |

---

## See also

- [`version-vX.Y-1.md`](version-vX.Y-1.md) - previous version
- [`releases-vX.Y.x.md`](releases-vX.Y.x.md) - release notes, if this version has patch releases
- [`plans/_template.md`](plans/_template.md) - phase-level plan template

---

## Authoring notes (delete before publishing)

These notes are the authoring contract for **version roadmaps** (this layout). Copy this file into `docs/` as `version-vX.Y.md`, replace placeholders, then delete this section in the published doc. Do not invent a different layout.

**Workflow.** When asked to draft or add a version roadmap: copy [`_version-template.md`](_version-template.md) to `docs/version-vX.Y.md`, fill it in, then remove this section. After creating the doc from chat, mention the new path so the operator can open it. Use **[`plans/_template.md`](plans/_template.md)** for phase-level feature plans; this file is for release-shaped themes and scope boundaries.

**File names**

- **Roadmap:** `docs/version-vX.Y.md` (example: [`version-v0.3.md`](version-v0.3.md)).
- **Patch-line release notes:** `docs/releases-vX.Y.x.md` — different purpose (shipping notes per patch), not a substitute for the roadmap template.

**Title.** `# Version X.Y - Short title`. The title should read like a release headline, not an internal codename.

**Required structure (in order)**

1. **`# Version X.Y - …`** — short, operator-facing.
2. **Front-matter table** — keep the labels in this template. **`Doc kind`:** typically `version-roadmap` for this document type. **`Status`:** one of `draft` · `active` · `shipped` · `deferred` · `superseded`.
3. **`## At a glance`** — immediately after the table: one to three sentences on value for the operator or end user, then the theme table (`Theme | Outcome | Status`). Each row links to a real `## Theme …` heading below; per-theme **`Status`** values are `todo` · `active` · `done` · `deferred`.
4. **`## What this version is`** — boundaries and companion links.
5. **Theme sections** — one `## Theme N - …` per at-a-glance row, **same order** as the table, each with **Goal**, **Scope**, **Acceptance**, and **Status**.
6. **`## Explicitly not this version`** — deferrals and out-of-scope decisions.
7. **`## Verification`** — operator-visible checks.
8. **`## See also`** — prior version, release-notes doc if any, plan template pointer.

**Patch release summaries** (when using `releases-vX.Y.x.md`): title like `# Releases vX.Y.0, vX.Y.1 - operator summary`; the overview table may use **`Release | Outcome | Status`** instead of themes.

**Don’t**

- Add YAML front matter — metadata stays in the Markdown table.
- Move **At a glance** below the front-matter table or bury it under **What this version is**.
- Put step-by-step checkboxes in the at-a-glance table (detail belongs under each theme).
- Ship this **Authoring notes** section — delete it before publishing.

**Link paths.** Version docs live in `docs/`; peer docs use `configuration.md`, `overview.md`, etc. Plans use `plans/<name>.md`. Repo root and code use `../README.md`, `../internal/...`, as needed.

**Anchor links.** GitHub-flavored slugs: lowercase, drop punctuation, replace spaces with `-`. Em-dashes surrounded by spaces become `--`. Example: `## Theme 2 — Billing clarity` → `#theme-2--billing-clarity`. Each at-a-glance row must link to an actual heading; verify links before finishing. Prefer stable heading text so existing links survive edits.

**At a glance is the contract.** Readers should infer goals, themes, and what is done from that section alone; deeper narrative stays below.

**Keep themes at the theme level.** The overview table tracks themes, not individual tasks — those belong in issues or [`plans/_template.md`](plans/_template.md)-style plans.
