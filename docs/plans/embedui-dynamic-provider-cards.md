# Plan: Dynamic provider cards on settings

| Field | Value |
|-------|-------|
| **Doc kind** | `feature-plan` |
| **Owners / areas** | Gateway admin UI (`adminui/embed`), admin BFF (`api/ui/state`, `api/ui/provider/*`) |
| **Status** | `draft` |
| **Targets** | Gateway / operator UI v0.3 |
| **Last updated** | See git history |
| **Supersedes / superseded by** | None |

## At a glance

The settings summarized feed always shows three provider cards (Groq, Gemini, Ollama) even when chimera-broker only has one provider configured. Operators should add providers from a catalog via **Add provider**, see only the cards they chose, and get a simpler empty card (no 24h usage table or scoped log) until API keys or an Ollama base URL are configured.

| Phase | Outcome | Status |
|-------|---------|--------|
| [Phase 1 — Catalog and visible provider state](#phase-1--catalog-and-visible-provider-state) | Single catalog module; feed cards driven by visible ids, not a fixed trio | `todo` |
| [Phase 2 — Add provider picker UI](#phase-2--add-provider-picker-ui) | Section-head **Add provider** opens a scrollable overlay picker with cancel | `todo` |
| [Phase 3 — Empty provider card UX](#phase-3--empty-provider-card-ux) | Hide Model usage (24h) and scoped provider log until credentials exist | `todo` |
| [Phase 4 — Backend and route alignment](#phase-4--backend-and-route-alignment) | State/health probes and save routes follow catalog + configured providers | `todo` |
| [Phase 5 — Tests and gallery parity](#phase-5--tests-and-gallery-parity) | Goja/summarized tests and static gallery fixtures updated | `todo` |

---

## Background

Provider configuration on **`/ui/settings`** (summarized feed) is implemented as collapsible cards (`admin-provider-{id}`). Card **metadata** (title, avatar, subtitle) and **which ids appear at all** are duplicated in several places. The gateway BFF probes a fixed name list for `GET /api/ui/state` and `GET /api/ui/chimera-broker/providers`, independent of what is registered in the broker config store (`GET /api/governance/providers` via `brokeradmin.ListConfiguredProviders`).

Today an operator with only Ollama in `data/broker/config.json` still sees Groq and Gemini cards because the UI roster is hard-coded. Adding a new upstream later requires editing multiple JS and Go constants.

**Related docs:** [`version-v0.3.md`](../version-v0.3.md), [`plans/logs-ui-page-data-refreshing.md`](logs-ui-page-data-refreshing.md), [`plans/chimera-gateway-package-boundaries.md`](chimera-gateway-package-boundaries.md), [`configuration.md`](../configuration.md).

**Out of scope (for this plan):** Catalog/free-tier Makefile tooling (`catalog-write-free`, `config/provider-free-tier.yaml`), provider rate-limit YAML, and routing-policy generator ordering—these use provider *model ids* (e.g. `groq/...`) but not the settings card roster.

---

## Static provider roster — inventory

Use this table when replacing the fixed trio. **Roster** = code that defines which provider *ids* exist for UI/BFF; **Presentation** = per-id display/behavior branches; **Consumer** = reads roster or assumes three cards.

### Gateway Go — roster and probes

| Location | Symbol / pattern | Role |
|----------|------------------|------|
| `chimera/chimera-gateway/internal/server/adminui/apirut/runtime.go` | `BrokerProviderNames = []string{"groq", "gemini", "ollama"}` | **Canonical Go roster** for state + provider-health BFF |
| `chimera/chimera-gateway/internal/server/adminui/api/state/build.go` | `for _, name := range apirut.BrokerProviderNames` | Builds `providers` map in `GET /api/ui/state` |
| `chimera/chimera-gateway/internal/server/adminui/api/providers/handlers.go` | `fetchChimeraBrokerProviderHealth(..., apirut.BrokerProviderNames, ...)` | `GET /api/ui/chimera-broker/providers` health strip |
| `chimera/chimera-gateway/internal/server/adminui/api/save/register.go` | `[]string{"groq", "gemini"}` + dedicated `ollama/base_url` route | **Save routes** only registered for groq/gemini keys |
| `chimera/chimera-gateway/internal/server/adminui/api/providers/handlers.go` | `switch name { case "ollama": ... }` in `ClassifyBrokerProviderResult` | Ollama-specific health classification (not a name list) |

**Dynamic already (not a fixed trio):** `brokeradmin.ListConfiguredProviders` / `GetProviderForProbe` use live broker governance; state assembly still *iterates* the fixed `BrokerProviderNames` list when building the UI map.

### Settings embed UI — roster and feed model

| Location | Symbol / pattern | Role |
|----------|------------------|------|
| `.../settings/app/summarizedFeed.js` | `ADMIN_PROVIDER_PATCH_SPECS` (3 entries) | **Canonical JS card roster**; passed as `adminProviderSpecs` into summarized model |
| `.../settings/app/summarizedFeed.js` | Loops at ~807, ~1410, ~1460, ~1489, `summarizedModelState` ~6230 | Patch-on-poll, `onlyCardIds`, `patchAdminProviderCard`, model build |
| `.../settings/summarized/model.js` | `state.adminProviderSpecs` | Emits one `admin-provider-{id}` card per spec |
| `.../settings/app/summarizedDirtyRouting.js` | `ADMIN_PROVIDER_IDS = ["groq", "gemini", "ollama"]` | Dirty-routing patch targets provider cards |
| `.../settings/render/cards/adminWorkflows.js` | Three `buildAdminProviderCardHtml("groq"|"gemini"|"ollama", ...)` | Legacy static feed section (keep in sync or remove if unused) |

### Settings embed UI — presentation and handlers (per-id branches)

| Location | Pattern | Role |
|----------|---------|------|
| `.../settings/render/cards/adminProvider.js` | `isOllama`, `groqKeyVal` / `geminiKeyVal`, placeholders `gsk-…` / `AIza…` | Card body layout and key inputs |
| `.../settings/render/cards/adminShared.js` | `adminProviderIntro` links map; `adminProviderAvatarClass`; `adminProviderTierSpan` | Display metadata for routing/fallback tables |
| `.../settings/handlers/admin.js` | `admin-groq-key`, `admin-gemini-key`; `prov === "groq"|"gemini"`; `adminProviderKeyDraft.groq/gemini` | Input drafts and save handler input ids |
| `.../settings_app.js` | `adminProviderKeyDraft = { groq: null, gemini: null }` | App-level draft state |

### Tests and fixtures

| Location | Notes |
|----------|--------|
| `embedui_test/settings_summarized_model_test.go` | `adminProviderSpecs: [{ id: "groq", ... }]`, expects `admin-provider-groq` |
| `embedui_test/settings_summarized_dirty_test.go` | Expects `admin-provider-groq` in card ids |
| `embedui_test/settings_components_test.go` | Provider health list fixtures use groq/gemini |
| `embedui_test/goja_test.go` | `adminProviderKeyDraft: { groq, gemini }` |
| `internal/server/server_test.go` | E2E `POST /api/ui/provider/groq/keys`, `ollama/base_url` |
| `internal/server/availablemodels_test.go` | `[]string{"groq", "gemini", "ollama"}` catalog expectation |
| `internal/server/adminui/api/providers/handlers_test.go` | `FetchBrokerProviderHealth(..., []string{"groq", "ollama"}, ...)` |

### Static gallery (demo data only)

| Location | Notes |
|----------|--------|
| `embed/embedui/gallery/gallery-unified-operator-routing.js` | Tier colors and sample fallback chain for groq/gemini/ollama |
| `embed/embedui/settings/gallery/gallery-unified-operator-routing.js` | Copy of gallery routing demo |
| `docs/component-gallery/gallery-unified-operator-routing.js` | Published gallery copy |

### Docs mentioning the fixed trio

| Location | Notes |
|----------|--------|
| `docs/version-v0.3.md` | Logs UI phase notes: `admin-groq-key`, patch groq/gemini/ollama |
| `docs/plans/logs-ui-page-data-refreshing.md` | Same interaction contract |
| `docs/plans/chimera-gateway-package-boundaries.md` | Card id `admin-provider-ollama` |

---

## Phase 1 — Catalog and visible provider state

**Goal.** One extensible **provider catalog** (id, title, avatar, subtitle, kind: keyed vs ollama) and a **visible provider ids** list determine which cards exist in the summarized model.

**Deliverables**

- New module e.g. `settings/providers/catalog.js` exporting `ADMIN_PROVIDER_CATALOG` and `lookupProviderSpec(id)`.
- Replace `ADMIN_PROVIDER_PATCH_SPECS` usages with `visibleIds.map(lookup)` (or equivalent on `ctx`).
- `ctx.adminVisibleProviderIds` (array) + optional `localStorage` persistence key documented in module header.
- Initial visible set policy implemented (see [Open questions](#open-questions)).
- `summarizedDirtyRouting.js`: derive provider ids from visible list or catalog, not `ADMIN_PROVIDER_IDS` constant.
- `adminProviderKeyDraft` becomes a string-keyed map (`draft[id]`), not `{ groq, gemini }`.

**Acceptance**

- Summarized model with zero visible ids emits no `admin-provider-*` cards; adding an id via state produces exactly one card.
- Admin poll patch loop iterates visible ids only (no hidden groq/gemini cards).

**Status:** `todo`

---

## Phase 2 — Add provider picker UI

**Goal.** **Providers** section matches **Users** / **Workspaces**: title row with **Add provider** on the right; choosing a provider adds its card without shifting the rest of the feed layout.

**Deliverables**

- Update `adminProvidersSectionBreakHtml` in `summarizedFeed.js` to use `operatorSectionHeadHtml("Providers", "hub", { actionHtml: operatorSectionAddBtn(...) })`.
- Picker markup: anchored overlay (`position: absolute`, opens upward), full feed width (`--embed-container-max`), scrollable list, **Cancel** control, `aria-expanded` on trigger.
- CSS in `styles/design-01.css` or `admin-forms.css` for `.sg-op-provider-picker` (max-height + `overflow-y: auto`).
- Handlers in `handlers/admin.js`: open, cancel, select (`data-admin-action` + `data-provider-id`); select pushes id into `adminVisibleProviderIds` and `scheduleStoryRebuild()`.
- Picker list = catalog entries not already visible.

**Acceptance**

- Open picker → list appears above intro/cards; cancel closes without new card.
- Select Groq → one new `admin-provider-groq` card; Add provider disabled or entry hidden for ids already visible.

**Status:** `todo`

---

## Phase 3 — Empty provider card UX

**Goal.** Cards without credentials show configuration only—no misleading empty usage table or scoped log.

**Deliverables**

- `providerHasCredentials(providerId, row)` in `adminProvider.js` (or shared helper): keyed providers → `keys.length > 0` or `key_configured`; ollama → non-empty `ollama_base_url` (respect `adminOllamaUrlDraft`).
- When false: omit `Model usage (24h)` block and `adminScopedEvlogPanelFromEvents("Scoped log — …")`.
- Keep intro + API key list + add-key row (or Ollama URL editor).

**Acceptance**

- Goja or unit test: HTML for groq with `keys: []` contains no `Model usage (24h)` and no `Scoped log —`.
- After saving a key (or Ollama URL), rebuild shows usage + scoped log sections.

**Status:** `todo`

---

## Phase 4 — Backend and route alignment

**Goal.** BFF probes and save routes support the same catalog the UI uses, and optional discovery of broker-configured providers.

**Deliverables**

- Refactor `BrokerProviderNames` into catalog-driven list (shared with UI via new `GET /api/ui/providers/catalog` **or** embed-only catalog with Go mirror in `apirut`).
- `BuildResponse`: include configured provider ids from `ListConfiguredProviders` (for seeding visible set or `configured` field on state).
- `save/register.go`: register key routes from catalog keyed providers (not hard-coded `groq`, `gemini` only)—or document wildcard `POST /api/ui/provider/{name}/keys` if safe.
- Health endpoint: probe union of catalog ids and configured ids (or visible ids from client—decide in open questions).

**Acceptance**

- With only `ollama` in broker config, `GET /api/ui/state` does not require groq/gemini entries unless UI/catalog asks for them.
- `POST /api/ui/provider/{catalog-id}/keys` works for any catalog keyed provider after normalize-merge on missing provider.

**Status:** `todo`

---

## Phase 5 — Tests and gallery parity

**Goal.** Tests and static gallery reflect dynamic visibility, not an assumed trio.

**Deliverables**

- Update `settings_summarized_model_test.go`, `settings_summarized_dirty_test.go`, `goja_test.go` for dynamic specs.
- Add test for empty credentials HTML (Phase 3).
- Align gallery routing demo tier labels with catalog helper or comment as fixture-only.
- Update `docs/version-v0.3.md` Logs UI notes when groq/gemini-specific input ids generalize.

**Acceptance**

- `go test ./chimera/chimera-gateway/internal/server/adminui/embed/...` passes.
- Manual smoke: add/remove visible provider, picker cancel, key save restores usage section.

**Status:** `todo`

---

## Open questions answered

1. **Initial visible providers:** seed from broker `governance/providers`; Do not use or restore from `localStorage`.
2. **Catalog source of truth:** Use `GET /api/ui/providers/catalog` so Go BFF and UI stay aligned. Remove the embed JS.
3. **Removing a card:** When a provider card is removed it should also DELETE provider from broker config. Currently there is no provider card removal path only managment of the API keys. But when the card is removed when there are API keye it would remove the provider from the broker config.
4. **Health strip on service cards:** show only configured providers.

---

## References

- Roster (Go): `chimera/chimera-gateway/internal/server/adminui/apirut/runtime.go`
- Roster (JS): `chimera/chimera-gateway/internal/server/adminui/embed/embedui/settings/app/summarizedFeed.js` (`ADMIN_PROVIDER_PATCH_SPECS`)
- Card render: `.../settings/render/cards/adminProvider.js`, `adminShared.js`
- Model: `.../settings/summarized/model.js`
- Broker configured list: `chimera/chimera-gateway/internal/brokeradmin/provider_probe.go` (`ListConfiguredProviders`)
- Prior interaction work: [`plans/logs-ui-page-data-refreshing.md`](logs-ui-page-data-refreshing.md)
