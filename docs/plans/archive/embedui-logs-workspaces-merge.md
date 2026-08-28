# Plan: Unify logs and workspace indexers in embed UI

| Field                          | Value                                                                            |
|--------------------------------|----------------------------------------------------------------------------------|
| **Doc kind**                   | `feature-plan`                                                                   |
| **Owners / areas**             | Gateway embed UI (`embedui`), operator logs, indexer configuration               |
| **Status**                     | `done`                                                                           |
| **Targets**                    | Gateway embed UI; aligns with workspace / project / flavor model in indexer docs |
| **Last updated**               | See git history                                                                  |
| **Supersedes / superseded by** | None                                                                             |

## At a glance

Operators manage **workspaces** (who indexes what, under which project and optional flavor) in the same place they watch **logs**, instead of switching tabs. The **Indexers** tab and its dedicated surface go away; the logs view gains a **Workspaces** section with a single, consistent card model for creating, configuring, and monitoring each workspace indexer.

| Phase                                                                                                       | Outcome                                                                                            | Status |
|-------------------------------------------------------------------------------------------------------------|----------------------------------------------------------------------------------------------------|--------|
| [Phase 1 — Shell and navigation](#phase-1--shell-and-navigation)                                            | Indexer tab removed; logs view owns workspace management entry points                              | `done` |
| [Phase 2 — Workspaces section structure](#phase-2--workspaces-section-structure)                            | “Workspaces” title, combined description, Create control, no per-card short blurbs                 | `done` |
| [Phase 3 — Card UX — create, save, directory picker](#phase-3--card-ux--create-save-directory-picker)       | New and unsaved cards: fields, labels, watched paths, Add path / Remove path, Cancel / Save        | `done` |
| [Phase 4 — Saved cards — monitoring, configure, delete](#phase-4--saved-cards--monitoring-configure-delete) | Read-only identity after save; Configure / edit / delete; drop collection status; file count rules | `done` |

---

## Background

Today the embed UI splits **logs** and **indexers** into separate tabs. Workspace indexers already appear as expandable cards with summaries and paths. This plan **merges** that behavior into the logs view under a **Workspaces** heading, **removes** the duplicate tab and its routes or wiring, and **refines** the card so setup (create / edit) and monitoring (events, recent files) share one component with explicit modes.

**Related docs:** [`log-view-indexer.md`](log-view-indexer.md), [`indexer.md`](indexer.md), [`idea-workspace-embedding-scope.md`](idea-workspace-embedding-scope.md).

**Clarifications (refined from draft)**

- **Save validation:** Persisting a workspace requires a **workspace id** and **project id**. **Flavor id** remains optional (empty means base / project-only scope per existing product rules). The draft phrase “save the indexer without it” is treated as a typo: **do not** persist an incomplete workspace to configuration; Save stays disabled or shows validation feedback until required fields are present.
- **Exit edit mode:** Use **Cancel** (discard in-session edits or remove an unsaved draft) or **Save** (persist). The draft’s “press Configure again to finish” conflicts with replacing Configure with Cancel/Save; **Configure** only **enters** edit mode. Optionally show a clear **editing** state on the card (style or label) while Cancel/Save are visible.
- **Unsaved card and live data:** A newly created card should **appear immediately** in the list in **expanded** form and should be wired so that, once saved, it is the same **workspace indexer** row that can show **recently evaluated files** and the **full event log**. Before save (no stable identity in config), those areas stay **empty or placeholder** until ids exist and the backend recognizes the workspace.
- **Header user label:** The card title row includes a read-only **user label** slot using the **same operator-facing label** the UI already shows for workspace indexers (authenticated session / indexer config), not an editable field. **Project id** and **flavor id** in the header mirror the body fields; blank when unset.
- **Watched paths control:** Paths are shown in a **list box** (`<select size="…">` or equivalent pattern). **Remove** is enabled only when the user has **selected** one path in that list; it removes the selected entry.

---

## Phase 1 — Shell and navigation

**Goal.** Operators no longer open a separate **Indexers** tab; workspace management lives only under logs.

**Deliverables**

- Remove the **Indexers** tab (and any nav affordance) from the embed UI shell.
- Remove dead code paths, assets, and handlers that existed only for the standalone indexer page or tab, without breaking `/ui/logs` (and tests that cover routing or HTML shell).
- Redirect or eliminate bookmarked indexer URLs if the project exposes them; document behavior in phase acceptance if a redirect is kept.

**Acceptance**

- Building and running the gateway, the operator UI shows **logs** as the single place for workspace indexer management; no orphan tab or broken link to the old surface.

**Status:** `done`

---

## Phase 2 — Workspaces section structure

**Goal.** The logs view introduces a clear **Workspaces** section with shared copy and a primary **Create** action.

**Deliverables**

- Rename the section title from the current **Indexers** (or equivalent) label to **Workspaces**.
- Add a **section description** below the title that **combines** (a) the introductory paragraph from the former indexers page and (b) the intent of the **short description** that currently appears on every workspace card — so the section explains once what workspaces and indexing mean.
- **Remove** the **short description** line from each workspace indexer card (no duplicate blurb per card).
- Add a **Create** button on the **same row** as the **Workspaces** title, **right-aligned** (layout matches existing header patterns in `embedui`).

**Acceptance**

- One title row: **Workspaces** (left) and **Create** (right); below it, a single block of descriptive text; cards no longer repeat that explanation.

**Status:** `done`

---

## Phase 3 — Card UX — create, save, directory picker

**Goal.** New and unsaved workspace rows use one card pattern: editable identity fields, watched paths with directory picker, and explicit Cancel / Save.

**Deliverables**

- **Create** adds a **new card** that is **not yet persisted** to configuration. The card opens **expanded** by default.
- **Card header (unsaved / editing new):**
  - **Title area:** read-only **labels** in one row: **user label** (from session/config — same source as today’s indexer cards), **project id**, and **flavor id** from the body fields — **blank** when unset (no placeholder clutter beyond existing design).
  - **Right side:** **Cancel** and **Save**. **Cancel** **removes** the unsaved workspace row entirely. **Save** runs validation and, if **workspace id** and **project id** are set, **writes** the workspace into runtime configuration and **persists** configuration (same semantics as today when adding an indexer from the UI).
- **Card body:**
  - Three **text fields** on **one row**: **workspace id**, **project id**, **flavor id**, each with a **label** above the field.
  - Editing **project id** and/or **flavor id** updates the **project** and **flavor** header labels in real time (user label is **not** edited here — it continues to come from auth/config).
  - **Watched paths:** a **Watched paths** label, then a **list box** showing one path per row (native multi-line `<select>` or accessible equivalent), then **Add** and **Remove** immediately after the list box.
  - **Add:** opens the **same directory chooser** used on the former indexer page; on confirm, **append** the absolute path to the list box (dedupe if the same path is added twice — match existing behavior).
  - **Remove:** **enabled** only when the user has **selected** a path in the **list box**; removes **that** selected path from the list.
  - **Path-derived defaults:** when choosing a directory, if **workspace id** and **project id** are **both** blank, **populate** them from the path using the **same rules** as the old indexer page. If **either** is already non-blank, **do not** overwrite.
- **Saving** locks **workspace id**, **project id**, and **flavor id** to **read-only** display in the card header (per Phase 4).

**Acceptance**

- Operator can create a draft card, pick directories, see labels update, cancel without persisting, or save and see configuration updated (verified via existing config APIs or restart behavior as today).

**Status:** `done`

---

## Phase 4 — Saved cards — monitoring, configure, delete

**Goal.** Saved workspaces match the monitoring story (events, files) without legacy “collection status,” with sensible empty states and a safe edit/delete path.

**Deliverables**

- **Remove** the **collection status** field and value from **all** workspace indexer cards.
- **Workspace file count:** when the count is **0**, **do not** show the file count (during setup and whenever zero). Show it again when **greater than zero**.
- **Recently evaluated files** and its box are **hidden** when there is no file-level activity in the scoped log window (no placeholder block).
- **Monitoring:** After save, the card remains the same row used for **recently evaluated files** and **full event log** (wire to existing log aggregation as today’s indexer cards do).
- **Configure (saved workspaces only):**
  - In the card’s **expanded** area, provide a **Configure** control. **Pressing Configure** enters **edit mode** for that workspace.
  - In edit mode, **Configure** is replaced by **Cancel** and **Save** (same header region behavior as new cards for consistency). **Cancel** discards edits and returns to the saved snapshot. **Save** persists changes to configuration.
  - **Editing rules for existing workspaces:** **Watched paths** may change (add/remove via the same Add/Remove controls, directory picker, and list selection for Remove). **Workspace ID** is the operator-store row id (read-only in the Summary block). **Project id** and **Flavor id** stay read-only once set (shown in Summary).
  - **Delete:** While editing an **existing** workspace, show **Delete**; confirm if the UI already uses confirmations elsewhere; removing the workspace drops it from configuration and removes the card after success.
- **Visual clarity:** It should be obvious when a card is in **edit mode** (e.g., replacing Configure with Cancel/Save, optional “Editing” hint).

**Acceptance**

- Saved card: no collection status; file count hidden at zero; Configure flows work; Delete removes workspace from config; event/file panels still attach to the row after edits.

**Status:** `done`

---

## References

- Code: `internal/server/embedui/` (`logs.js`, `logs.css`, `shell.html`), `internal/server/ui_handlers.go`, `internal/server/uisession.go`, indexer config endpoints used by the UI.
- Plans: [`log-view-indexer.md`](log-view-indexer.md), [`indexer.md`](indexer.md).
- Tests: `internal/server/ui_logs_test.go`, `internal/server/server_test.go` (update when routes or HTML change).
