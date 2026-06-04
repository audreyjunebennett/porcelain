# Operator shared UI (`embedui/shared/`)

Wizard-eligible primitives for settings cards and future setup flows. Namespace: **`globalThis.ChimeraShared`**.

Served at `/ui/assets/shared/*.js` (see `embed/routes.go`). Load before `settings/render/cards/adminShared.js` on the settings page.

| Module | Exports |
|--------|---------|
| `operatorFeedback.js` | Status line + save-button pending state |
| `configureEdit.js` | Configure affordance, edit-mode cancel helper |
| `yamlEditor.js` | YAML wrap dirty class + overlay scroll sync |
| `draftInput.js` | Provider key / Ollama URL draft tracking on `ctx` |
| `providerCredentials.js` | Key-add + Ollama URL HTML and save handlers |
| `scopedEvlog.js` | In-card scoped event log panel shell |
| `adminAction.js` | `runJson` — shared POST/PUT success/error/toast wrapper |
| `editToolbar.js` | Icon toolbar buttons (`sg-op-yaml-ov-btn`) |
| `workspacePaths.js` | Watched-paths select + Add/Remove row |
| `serviceHealth.js` | Health segment tone + `metricsWrapHtml` |

Settings-only glue remains in `settings/handlers/` and `settings/app/summarizedFeed.js`.
