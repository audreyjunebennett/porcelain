# Chimera Gateway Runtime

**Porcelain** is an **OpenAI-compatible AI workspace** that brings together **services** and **clients**. **Chimera** is the service stack—HTTP servers, wrapper processes, operator tools, and backend components (gateway, indexer, vector store, and related runtime). **Locus** is the client stack—desktop and workspace apps that connect to Chimera. Operator docs are in `docs/`; milestones in `docs/`; historical and active plans in `docs/plans`.

## Quick start

You need **GNU Make**.

On Windows:

```powershell
pwsh -ExecutionPolicy Bypass -File scripts/install-make.ps1
```

On Ubuntu and macOS:

```bash
bash scripts/install-make.sh
```

### Install and Start (all-in-one)

One-shot onboarding: get dependencies, seed config if needed, build the gateway, and start the supervisor in the background.

```bash
make up
```

## Installing Dependencies and Building the Service

### Install

Install all the dependencies and build the dependent projects.

```bash
make install
```

`make install` brings in all build tools and necessary dependencies for the chimera and locus products.

**Dependencies**

- **Go (1.22+)** — builds the gateway and BiFrost’s Go code.
- **Node.js (20+)** — BiFrost’s UI is built with npm during install; it is not shipped inside the `chimera` binary.
- **Git** — BiFrost is vendored from a tracked revision, not embedded in the clone.
- **GNU Make** — single entrypoint for install and build targets from the repo root.
- **gcc or clang** — BiFrost’s HTTP server is built with **CGO**; the Go toolchain must invoke a C compiler or the build fails early.
- **bash, curl, tar** (and **unzip** on Windows) — reliable way to download and unpack release artifacts the same way on every platform.

Full reference: [docs/installation.md](docs/installation.md).

### Configuration

Create local config files from the shipped examples when they are missing.

```bash
make configure
```

Full reference: [docs/configuration.md](docs/configuration.md).

### Build Chimera and Locus products 

The install process all Chimera services and Locus clients.

```bash
make build
```

### Start Chimera-Supervisor

The Chimera-Supervisor runs and manages: gateway; vector-store; broker, and indexer.

```bash
make chimera-supervisor-run
```

Further reference: [docs/supervisor.md](docs/supervisor.md).

### Start Locus-Desktop

The Locus-Desktop creates a system native webview that starts the chimera-supervisor, if not started, and then connects to it.

```bash
make locus-desktop-run
```

## Testing and Linting

| Target | What it does |
| ------ | ------------ |
| `make fmt-check` | Fails if `gofmt` would change any file |
| `make fmt` | Formats all the project code with `gofmt` |
| `make test` | Runs all unit and end-to-end tests for all products |
| `make test-unit` | Run all the unit tests |
| `make test-e2e` | Run all the end-to-end tests for all products |
| `make precommit` | Runs `fmt-check`, `vet`, and `test` |

## Repo Management and Packaging

### Clean up built binaries

Remove all **built binaries** and other dependencies.

```bash
make clean
```

## Documentation

- **Index:** [docs/README.md](docs/README.md)
- **Overview / ports:** [docs/overview.md](docs/overview.md), [docs/network.md](docs/network.md)
- **Installation:** [docs/installation.md](docs/installation.md)
- **Configuration:** [docs/configuration.md](docs/configuration.md)
- **Supervisor:** [docs/supervisor.md](docs/supervisor.md)
- **Packaging / releases:** [docs/packaging.md](docs/packaging.md)
- **Security:** [SECURITY.md](SECURITY.md)
- **Product / requirements (normative):** [docs/porcelain.plan.md](docs/porcelain.plan.md)

## Development roadmap

| Version | Where to read |
|---------|---------------|
| **v0.1** | [Working notes](docs/version-v0.1.md) |
| **v0.1.1** | [Tool router, metrics, quotas](docs/version-v0.1.1.md) |
| **v0.2.0 – v0.2.2** | [Shipped releases + capability plan](docs/version-v0.2.md) |
| **v0.3.0** | [Working plan — v0.3](docs/version-v0.3.md) |
| **v0.4.0** | [Working plan — v0.4](docs/version-v0.4.md) |
| **Later** | [Release roadmap](docs/porcelain.plan.md#release-roadmap) in [docs/porcelain.plan.md](docs/porcelain.plan.md) |

## License

Private / unspecified — add a `LICENSE` if you publish.
