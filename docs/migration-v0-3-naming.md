# v0.3 naming migration guide

This guide documents operator-facing breaking changes in the v0.3 naming train.

## Config and credential files

- `config/tokens.yaml` -> `config/api-keys.yaml`
- `config/tokens.example.yaml` -> `config/api-keys.example.yaml`
- In `gateway.yaml`, use `paths.api_keys: "./api-keys.yaml"`.
- Credential schema:
  - top-level key `api_keys`
  - per-row key `secret`

## Environment variables

- Required env name family:
  - `CHIMERA_GATEWAY_CONFIG`
  - `CHIMERA_GATEWAY_URL`
  - `CHIMERA_GATEWAY_TOKEN`
  - `CHIMERA_UPSTREAM_API_KEY`

## Hidden state directories

- Indexer hidden directory: `.locus`
  - `.locus/indexer.config.yaml`
  - `.locus/indexer.sync-state.json`

## Release and package artifacts

- GoReleaser project/binary/archive prefix uses `chimera`.
- `make package` emits `dist/personal/chimera-bundle_<os>_<arch>/`.
- Package config includes `api-keys.example.yaml`.

## CI, workflows, and operational surfaces

- Build/release workflows use `chimera` archive/binary names and `locus-desktop` for desktop-tagged builds.
- Personal bundle output carries both runtime executables:
  - `chimera-supervisor[.exe]` for supervised stack lifecycle
  - `locus-desktop[.exe]` for native webview shell

## Operator migration steps (breaking path/name contracts)

- Credential file migration:
  - from `config/tokens.yaml` to `config/api-keys.yaml`
  - from `paths.tokens` to `paths.api_keys` in `gateway.yaml`
- Environment migration:
  - from `CHIMERA_GATEWAY_CONFIG` to `CHIMERA_GATEWAY_CONFIG`

## Final cutover status

- v0.3 uses hard-cut naming only.
- Legacy aliases (`CHIMERA_*`, `X-Chimera-*`, `paths.tokens`, `tokens.yaml`, `.chimera/*`) are retired and unsupported.
  - from `CHIMERA_GATEWAY_URL` to `CHIMERA_GATEWAY_URL`
  - from `CHIMERA_GATEWAY_TOKEN` to `CHIMERA_GATEWAY_TOKEN`
  - from `CHIMERA_UPSTREAM_API_KEY` to `CHIMERA_UPSTREAM_API_KEY`
- Hidden-state migration:
  - from `.chimera/indexer.config.yaml` to `.locus/indexer.config.yaml`
  - from `.chimera/indexer.sync-state.json` to `.locus/indexer.sync-state.json`
- Runtime data reset (optional, for a clean local stack):
  - run `make clean-data CONFIRM=1` to clear `data/bifrost`, `data/qdrant`, and `data/gateway`.

