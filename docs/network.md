# Network architecture (local processes)

## Logical flow

- **IDE / Continue** → **chimera-gateway** (`POST /v1/chat/completions`, `GET /v1/models`) with `Authorization: Bearer <gateway token>`.
- **chimera-gateway** → **chimera-broker** (`/v1/chat/completions`, `/v1/models`) with `Authorization: Bearer <CHIMERA_BROKER_API_KEY>` (broker often accepts a placeholder unless governance keys are enabled).
- **chimera-broker** → provider APIs using `GROQ_API_KEY`, `GEMINI_API_KEY`, etc. per `config/bifrost.config.json` (BiFrost-shaped backend config).
- **RAG (`rag.enabled`)**: **chimera-gateway** → **chimera-vectorstore** for retrieval; **chimera-gateway** ← **chimera-indexer** (or other clients) via `POST /v1/ingest` and indexer APIs. Without RAG, the gateway does not call the vectorstore.

Operator vocabulary and refactor plan: [plans/chimera-gateway-refactor.md](plans/chimera-gateway-refactor.md).

## Typical local ports

| Process | Default port | Role |
|---------|----------------|------|
| **chimera-gateway** (backend) | **3000** | Client-facing API |
| **chimera-broker** (wrapper) | **7730** control / **8080** backend | LLM relay |
| **chimera-vectorstore** (wrapper) | **7740** control / **6333** HTTP, **6334** gRPC | Vectors (optional) |

`chimera-supervisor` manages broker and vectorstore wrappers on loopback by default. `config/gateway.yaml` `upstream.base_url` should point at the broker backend (e.g. `http://127.0.0.1:8080`). The supervisor overrides URLs to match supervised listen addresses.

**On the host**, use `http://127.0.0.1:3000` for Continue’s `apiBase` (plus `/v1` as required by your client).

## Wrapper control ports and callable paths

Wrapper binaries expose a control-plane HTTP surface (`/healthz`, `/readyz`, `/status`, `/metrics`) on their own listen address, separate from the backend service endpoint they supervise.

| Wrapper binary | Wrapper control port (`-listen`) | Backend service port (default) | Paths callable on wrapper port |
|---------|----------------|----------------|------|
| **chimera-supervisor** | **127.0.0.1:7710** (recommended wrapper control port via `-listen`) | Supervises broker/vectorstore wrappers and gateway | `/healthz`, `/readyz`, `/status`, `/metrics` |
| **chimera-gateway** | **127.0.0.1:7720** | **127.0.0.1:3000** (gateway serve port) | `/healthz`, `/readyz`, `/status`, `/metrics`, `/debug/broker/logs`* |
| **chimera-broker** | **127.0.0.1:7730** | **127.0.0.1:8080** (broker backend endpoint) | `/healthz`, `/readyz`, `/status`, `/metrics`, `/debug/broker/logs`* |
| **chimera-vectorstore** | **127.0.0.1:7740** | **127.0.0.1:6333** HTTP (`6334` gRPC) | `/healthz`, `/readyz`, `/status`, `/metrics`, `/debug/vectorstore/logs`* |
| **chimera-indexer** | **127.0.0.1:7750** | Worker backend (`chimera-indexer --indexer-backend`) | `/healthz`, `/readyz`, `/status`, `/metrics`, `/debug/broker/logs`* |

\* Debug log ring buffers are disabled by default (`404`) unless explicitly enabled. Path rename across wrappers is tracked in [plans/chimera-gateway-refactor.md](plans/chimera-gateway-refactor.md) open questions.

Wrapper paths by intent:

- `GET /healthz`: wrapper process is alive (liveness).
- `GET /readyz`: backend is ready for traffic (readiness).
- `GET /status`: normalized status payload (`ok`, `degraded`, or `error`) with component/backend metadata.
- `GET /metrics`: wrapper metrics and prefixed upstream metrics when available.
- `GET /debug/broker/logs` / `GET /debug/vectorstore/logs`: optional debug ring buffer endpoints (gated by `DEBUG__ENABLE_BROKER_LOGS` / `DEBUG__ENABLE_VECTORSTORE_LOGS` or matching wrapper flags).

## Trust boundary (v0.1)

Traffic is **plain HTTP** by default on the loopback or trusted LAN. Hardening and TLS are **out of scope for v0.1**; see the plan (**v0.7** security).
