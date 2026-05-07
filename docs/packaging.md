# Packaging and releases (Phase 4)

The **Go `claudia`** binary is built with **[GoReleaser](https://goreleaser.com/)** v2. Each release archive also includes the **Qdrant** binary for the matching OS/arch (`QDRANT_RELEASE` in repo-root `deps.lock`, fetched by `scripts/release-snapshot-qdrant.sh` before packaging). **BiFrost** is **not** bundled (license, size, CGO); operators use `make claudia-install` (or `make install` to include desktop OS deps) — see [supervisor.md](supervisor.md).

## GitHub releases vs local snapshot

- **Tag push (`v*`)** — [`.github/workflows/release.yml`](../.github/workflows/release.yml) runs `goreleaser release --clean` and uploads assets. This is **not** the `make release-snapshot` target.
- `make release-snapshot` — local snapshot only (no GitHub upload); same archive layout under `dist/`.

## Personal full bundle (BiFrost + desktop UI)

`make package` writes `dist/personal/claudia-bundle_<os>_<arch>/` with **desktop `claudia`**, `bifrost-http`, `qdrant`, and `config/` + `env.example`. Requires `./bin/bifrost-http` and `./bin/qdrant` from `make claudia-install`, plus a **CGO / WebView** toolchain — use `make install` (runs `claudia-install` then `desktop-install`) or run those two targets by hand.

## Artifact layout

Each GitHub **Release** (git tag **`v*`, e.g. `v0.1.0`**) publishes:

| File | Contents |
|------|----------|
| `claudia_<version>_<os>_<arch>.tar.gz` | Linux/macOS: `claudia`, `qdrant`, starter `config/` (`gateway.yaml`, `tokens.example.yaml` (not auto-copied to `tokens.yaml`), `bifrost.config.json`, `routing-policy.yaml`, `provider-free-tier.yaml`), `env.example`, `README.md`, `README_ARCHIVE.txt`, `PACKAGING.md` |
| `claudia_<version>_windows_amd64.zip` | Windows: `claudia.exe`, `qdrant.exe`, same config and docs |
| `checksums.txt` | SHA-256 checksums for the archives |

Architectures: **linux/darwin** **amd64** and **arm64**; **windows amd64** only (no **windows/arm64**).

## Prerequisites on the target machine

- **Config:** copy or mount `config/gateway.yaml`, `config/bifrost.config.json` (and `routing-policy.yaml`, `provider-free-tier.yaml` paths as in YAML). `config/tokens.yaml` is created on first-run setup (localhost) or by copying `tokens.example.yaml`. See [configuration.md](configuration.md) and [plans/version-v0.1.md](plans/version-v0.1.md) §5.
- **Environment:** `CLAUDIA_UPSTREAM_API_KEY` and provider keys (`GROQ_API_KEY`, etc.) — or a `.env` file in the **working directory** (the binary loads it at startup).
- **Upstream:** BiFrost (or another OpenAI-compatible proxy) reachable at `upstream.base_url`.

## Install (quick)

**Linux / macOS**

```bash
tar xzf claudia_<version>_linux_amd64.tar.gz
cd claudia_<version>_linux_amd64   # or matching folder name inside the archive
./claudia -version
./claudia -h
# Optional: ./qdrant with env from Qdrant docs, or claudia serve -qdrant-bin ./qdrant
```

**Windows**

Extract the `.zip`, copy `env.example` to `.env`, install **BiFrost** separately or use `make package` for a full folder. The public zip’s `claudia.exe` is built **without** `-tags desktop` (no native webview); use `claudia serve` / `claudia --headless` for the supervisor, or `claudia gateway` for gateway-only. SmartScreen may warn on first run for unsigned binaries; **code signing** is a documented follow-up.

## Cutting a release (maintainers)

1. Ensure `go test ./...` and `gofmt` are clean.
2. Tag: `git tag v0.x.y` and `git push origin v0.x.y` (prerelease: `v0.1.0-rc.1`, etc.).
3. **GitHub Actions** workflow **Release** runs GoReleaser and uploads assets (requires default `GITHUB_TOKEN` with **contents: write**).

Release notes may reference [SECURITY.md](../SECURITY.md).

Local snapshot (no upload):

```bash
make release-snapshot
# or: goreleaser release --snapshot --clean
```

Artifacts appear under `dist/` (gitignored).

## Version string

Release builds embed the tag, commit, and commit date:

```bash
claudia -version
```

Plain `go build` without `-ldflags` reports `dev`, `none`, `unknown`.

## Qdrant licensing

Qdrant is **Apache-2.0**. The archive **redistributes** the official prebuilt binary from [github.com/qdrant/qdrant](https://github.com/qdrant/qdrant/releases). Bump `QDRANT_RELEASE` in `deps.lock` when you want a newer Qdrant in releases.

## BiFrost version pinning

Releases version `claudia` and bundle **Qdrant** as above. Record the **BiFrost** image tag or binary version you tested against in release notes; bundling BiFrost remains out of scope.

## Desktop UI (WebView)

Native panel UI is `go build -tags desktop` (`make desktop-build` → `claudia-desktop`). GoReleaser archives use **CGO_ENABLED=0**, so they do **not** include WebView; use `make package` for a double-clickable desktop stack on your machine. See [gui-testing.md](gui-testing.md).

## Follow-ups (not in Phase 4)

- **macOS** notarization / **Windows** Authenticode signing.
- `LICENSE` / third-party notices in archives once a license is chosen for publication.
