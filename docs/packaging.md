# Chimera packaging and releases

Release archives are built with **[GoReleaser](https://goreleaser.com/)** v2. Each archive includes the **Chimera** Go binaries (`chimera-gateway`, `chimera-broker`, `chimera-supervisor`, `chimera-vectorstore`, `chimera-indexer`), the **Qdrant** binary for the matching OS/arch (`QDRANT_RELEASE` in `chimera/deps.lock`, fetched by `scripts/release-build-qdrant.sh`), and starter config. **BiFrost** (`bifrost-http`) is **not** bundled (license, size, CGO); operators use `make chimera-broker-install` — see [supervisor.md](supervisor.md).

## Make targets

| Target | Purpose |
|--------|---------|
| `make release-install` | Install GoReleaser and ensure curl/tar/unzip for the Qdrant hook |
| `make release-build` | Local snapshot archives under `dist/` (no GitHub upload) |
| `make release-package` | Full **desktop** folder under `dist/personal/chimera-bundle_<os>_<arch>/` (Locus + Chimera + bifrost-http + qdrant) |

## GitHub releases vs local build

- **Tag push (`v*`)** — [`.github/workflows/release.yml`](../.github/workflows/release.yml) runs `goreleaser release --clean` and uploads assets.
- **`make release-build`** — same archive layout locally (`goreleaser release --snapshot --clean`).

## Personal desktop bundle

`make release-package` writes `dist/personal/chimera-bundle_<os>_<arch>/` with `locus/bin/` (desktop + full Chimera stack + `bifrost-http` + `qdrant`) and `locus/config/`. Depends on `make chimera-build` and `make locus-desktop-build` (CGO / WebView). For OS deps, run `make install` first.

## Artifact layout

Each GitHub **Release** (git tag **`v*`, e.g. `v0.1.0`**) publishes:

| File | Contents |
|------|----------|
| `chimera_<version>_<os>_<arch>.tar.gz` | Linux/macOS: `chimera-gateway`, `chimera-broker`, `chimera-supervisor`, `chimera-vectorstore`, `chimera-indexer`, `qdrant`, starter `config/`, `env.example`, `README.md`, `README_ARCHIVE.txt`, `PACKAGING.md` |
| `chimera_<version>_windows_amd64.zip` | Windows: same binaries with `.exe` where applicable |
| `checksums.txt` | SHA-256 checksums for the archives |

Architectures: **linux/darwin** **amd64** and **arm64**; **windows amd64** only (no **windows/arm64**).

## Prerequisites on the target machine

- **Config:** `config/gateway.yaml`, `config/chimera-broker.config.json`, `provider-free-tier.yaml` (archives ship examples as starting points). Routing policy lives on virtual models in operator SQLite, not a global YAML file. `config/api-keys.yaml` from setup or copy of `api-keys.example.yaml`. See [configuration.md](configuration.md).
- **Environment:** provider keys in `.env` (copy from `env.example`).
- **BiFrost:** install `bifrost-http` separately, or use `make release-package` for a local bundle that includes it.

## Install (quick)

**Linux / macOS**

```bash
tar xzf chimera_<version>_linux_amd64.tar.gz
cd chimera_<version>_linux_amd64
./chimera-gateway -version
./chimera-supervisor -h
```

**Windows**

Extract the `.zip`, copy `env.example` to `.env`, install **BiFrost** separately or use `make release-package` for a full folder. GoReleaser binaries have **no** WebView; use `locus-desktop` from `release-package` for a double-click UI.

## Cutting a release (maintainers)

1. Ensure `go test ./...` and `gofmt` are clean.
2. Tag: `git tag v0.x.y` and `git push origin v0.x.y`.
3. **GitHub Actions** workflow **Release** runs GoReleaser and uploads assets.

Local snapshot (no upload):

```bash
make release-build
```

Artifacts appear under `dist/` (gitignored).

## Version string

Release builds embed the tag, commit, and commit date:

```bash
./chimera-gateway -version
```

Plain `go build` without `-ldflags` reports `dev`, `none`, `unknown`.

## Qdrant licensing

Qdrant is **Apache-2.0**. Archives redistribute the official prebuilt binary from [github.com/qdrant/qdrant](https://github.com/qdrant/qdrant/releases). Bump `QDRANT_RELEASE` in `chimera/deps.lock` when you want a newer Qdrant.

## Desktop UI (WebView)

Native panel UI: `make locus-desktop-build` → `locus-desktop`. GoReleaser archives use **CGO_ENABLED=0** (no WebView). Use `make release-package` for a double-clickable desktop stack. See [gui-testing.md](gui-testing.md).

## Follow-ups

- **macOS** notarization / **Windows** Authenticode signing.
- `LICENSE` / third-party notices in archives once chosen for publication.
