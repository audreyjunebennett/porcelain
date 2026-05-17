# Network architecture (local processes)

## Logical flow

- **IDE / Continue** → **chimera-gateway** (`POST /v1/chat/completions`, `GET /v1/models`) with `Authorization: Bearer <gateway token>`.
- **chimera-gateway** → **BiFrost** (`/v1/chat/completions`, `/v1/models`) with `Authorization: Bearer <CHIMERA_UPSTREAM_API_KEY>` (BiFrost often accepts a placeholder unless governance keys are enabled).
- **BiFrost** → providers using `GROQ_API_KEY`, `GEMINI_API_KEY`, etc. per `config/bifrost.config.json`.
- **RAG (`rag.enabled`)**: **Chimera** → **Qdrant** for retrieval; **Chimera** ← `chimera-indexer` (or other clients) via `POST /v1/ingest` and indexer APIs. Without RAG, the gateway does not call Qdrant.

## Typical local ports

| Process | Default port | Role |
|---------|----------------|------|
| **chimera** | **3000** | Client-facing gateway |
| **bifrost-http** | **8080** | AI gateway (default upstream) |
| **qdrant** | **6333** (HTTP), **6334** (gRPC) | Vectors (optional, v0.2+) |

`chimera serve` binds BiFrost and Qdrant on loopback by default; `config/gateway.yaml` `upstream.base_url` should point at that upstream (e.g. `http://127.0.0.1:8080`). `chimera serve` overrides the upstream URL to match the supervised BiFrost listen address.

**On the host**, use `http://127.0.0.1:3000` for Continue’s `apiBase` (plus `/v1` as required by your client).

## Wrapper control ports and callable paths

Wrapper binaries expose a control-plane HTTP surface (`/healthz`, `/readyz`, `/status`, `/metrics`) on their own listen address, separate from the backend service endpoint they supervise.

| Wrapper binary | Wrapper control port (`-listen`) | Backend service port (default) | Paths callable on wrapper port |
|---------|----------------|----------------|------|
| **chimera-supervisor** | **127.0.0.1:7710** (recommended wrapper control port via `-listen`) | Supervises broker/vectorstore wrappers and gateway | `/healthz`, `/readyz`, `/status`, `/metrics` |
| **chimera-gateway** | **127.0.0.1:7720** | **127.0.0.1:3000** (gateway serve port) | `/healthz`, `/readyz`, `/status`, `/metrics`, `/debug/upstream/logs`* |
| **chimera-broker** | **127.0.0.1:7730** | **127.0.0.1:8080** (broker backend endpoint) | `/healthz`, `/readyz`, `/status`, `/metrics`, `/debug/upstream/logs`* |
| **chimera-vectorstore** | **127.0.0.1:7740** | **127.0.0.1:6333** HTTP (`6334` gRPC) | `/healthz`, `/readyz`, `/status`, `/metrics`, `/debug/upstream/logs`* |
| **chimera-indexer** | **127.0.0.1:7750** | Worker backend (`chimera-indexer --indexer-backend`) | `/healthz`, `/readyz`, `/status`, `/metrics`, `/debug/upstream/logs`* |

\* `/debug/upstream/logs` is disabled by default and returns `404` unless explicitly enabled.

Wrapper paths by intent:

- `GET /healthz`: wrapper process is alive (liveness).
- `GET /readyz`: backend is ready for traffic (readiness).
- `GET /status`: normalized status payload (`ok`, `degraded`, or `error`) with component/backend metadata.
- `GET /metrics`: wrapper metrics and prefixed upstream metrics when available.
- `GET /debug/upstream/logs`: optional debug ring buffer endpoint (gated by debug config).

## Trust boundary (v0.1)

Traffic is **plain HTTP** by default on the loopback or trusted LAN. Hardening and TLS are **out of scope for v0.1**; see the plan (**v0.7** security).
