# Desktop / admin UI (replaces Fyne `claudia-gui`)

The old **Fyne** nested module was removed in favor of:

1. **Gateway-served operator UI** — open `http://<gateway>/ui/login` (session cookie after you post a gateway token to `POST /api/ui/login`). Control panel: `/ui/panel`.
2. **Optional native shell** — build `claudia-desktop` with `make desktop-build` (`-tags desktop`, **CGO**, WebKitGTK / WebView2), then run `./claudia-desktop` (supervisor + webview) or `./claudia-desktop --headless` with the same flags as `claudia serve` (see `make desktop-run`). On **Windows**, the desktop binary is linked as `windowsgui`, so launching `claudia.exe` from Explorer shows **only the webview**; supervised **bifrost-http** / **qdrant** are started **without** extra console windows (logs appear under `/ui/logs` after login). The tabbed `/ui/desktop` shell emphasizes **Logs** (with summary rollups); detailed gateway-recorded upstream usage remains under `/ui/metrics` (SQLite). When **no** console is attached, process output is **not** tee’d to `os.Stdout`/`Stderr` (that can block or break services under `windowsgui`); it still goes to the in-memory log buffer for the UI.

Normative plan: [ui-tool.plan.md](plans/ui-tool.plan.md).
