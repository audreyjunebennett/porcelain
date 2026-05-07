# Network architecture (local processes)

## Logical flow

- **IDE / Continue** → **Claudia Gateway** (`POST /v1/chat/completions`, `GET /v1/models`) with `Authorization: Bearer <gateway token>`.
- **Claudia Gateway** → **BiFrost** (`/v1/chat/completions`, `/v1/models`) with `Authorization: Bearer <CLAUDIA_UPSTREAM_API_KEY>` (BiFrost often accepts a placeholder unless governance keys are enabled).
- **BiFrost** → providers using `GROQ_API_KEY`, `GEMINI_API_KEY`, etc. per `config/bifrost.config.json`.
- **RAG (`rag.enabled`)**: **Claudia** → **Qdrant** for retrieval; **Claudia** ← `claudia-index` (or other clients) via `POST /v1/ingest` and indexer APIs. Without RAG, the gateway does not call Qdrant.

## Typical local ports

| Process | Default port | Role |
|---------|----------------|------|
| **claudia** | **3000** | Client-facing gateway |
| **bifrost-http** | **8080** | AI gateway (default upstream) |
| **qdrant** | **6333** (HTTP), **6334** (gRPC) | Vectors (optional, v0.2+) |

`claudia serve` binds BiFrost and Qdrant on loopback by default; `config/gateway.yaml` `upstream.base_url` should point at that upstream (e.g. `http://127.0.0.1:8080`). `claudia serve` overrides the upstream URL to match the supervised BiFrost listen address.

**On the host**, use `http://127.0.0.1:3000` for Continue’s `apiBase` (plus `/v1` as required by your client).

## Trust boundary (v0.1)

Traffic is **plain HTTP** by default on the loopback or trusted LAN. Hardening and TLS are **out of scope for v0.1**; see the plan (**v0.7** security).
