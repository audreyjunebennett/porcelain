# Feature: Internal embedding provider

| Field | Value |
|-------|-------|
| **Doc kind** | `feature-record` |
| **Areas** | `chimera-embed`, gateway RAG, indexer health, supervisor |
| **Status** | `current` |
| **Introduced** | v0.3.3 follow-up salvage from PR #14 |
| **Originated from** | [`plans/archive/internal-embedding-provider.md`](../plans/archive/internal-embedding-provider.md) |
| **Related features** | [Gateway RAG ingest and retrieval](gateway-rag-ingest-and-retrieval.md), [Workspace file indexer](indexer.md), [Wrapper binary contract](chimera-wrapper-binary-contract.md) |
| **Last updated** | See git history |

## At a glance

When `internal_embedding.enabled` is true, `chimera-supervisor` starts `chimera-embed`, which supervises an operator-supplied `llama-server` in embedding-only mode with an operator-supplied GGUF model. The gateway sends its existing OpenAI-compatible `/v1/embeddings` requests directly to that local server. The indexer remains transport-only: it sends file content to the gateway and never embeds locally.

The feature is off by default. Porcelain does not download or redistribute model weights or `llama-server`.

## System behavior and contracts

- Enabling `internal_embedding` overrides the resolved `rag.embedding.base_url`, `model`, and `dim` with the internal block.
- Relative `model_path` and `cache_dir` values resolve from the directory containing `gateway.yaml`.
- Supervisor startup order is vectorstore → internal embed (when enabled) → broker → gateway → indexer.
- `chimera-embed` uses the shared wrapper contract: `/healthz`, `/readyz`, `/status`, `/metrics`, structured logs, readiness gating, restart/backoff, and graceful shutdown.
- The llama backend receives `-m`, `--embedding`, `--host`, `--port`, context/GPU/pooling settings, and `--metrics`; readiness is `GET /health`.
- `GET /v1/indexer/storage/health` probes the internal `/v1/embeddings` endpoint and verifies the returned vector dimension. It does not require the internal model to appear in the broker catalog.
- The existing RAG embedding settings API reports the configured internal model as the sole active candidate while internal embedding is enabled. Selecting a broker model requires disabling `internal_embedding` first.

## Interfaces

| Surface | Detail |
|---------|--------|
| Gateway config | `internal_embedding.enabled`, `provider`, `model`, `dim`, `base_url`, `model_path`, `cache_dir`, `log_level` |
| Wrapper env | `EMBED__LISTEN`, `EMBED__BIN`, `EMBED__ENDPOINT`, `EMBED__MODEL_PATH`, `EMBED__CACHE_DIR`, `EMBED__CTX_SIZE`, `EMBED__N_GPU_LAYERS`, `EMBED__POOLING`, timeout/debug keys |
| Supervisor flags | `-embed-bin`, `-embed-backend-bin`, `-embed-listen`, `-embed-endpoint`, `-embed-model-path`, `-embed-cache-dir`, `-wait-embed`, `-no-wait-embed` |
| Backend API | `GET /health`, `POST /v1/embeddings`, optional `/metrics` from `llama-server` |

## Code map

| Concern | Location |
|---------|----------|
| Wrapper and llama adapter | `chimera/chimera-embed/` |
| Config resolution / RAG selection | `chimera/internal/config/internal_embedding.go` |
| Supervisor lifecycle | `chimera/chimera-supervisor/internal/supervise/` |
| Direct endpoint health | `chimera/chimera-gateway/internal/server/indexerapi/health.go` |
| Existing embedding client/probe | `chimera/chimera-gateway/internal/rag/ragembed/` |

## Verification

```bash
go test ./chimera/chimera-embed/... -count=1
go test ./chimera/chimera-supervisor/... -count=1
go test ./chimera/chimera-gateway/internal/server/indexerapi/... -count=1
go test ./chimera/chimera-gateway/internal/server/adminui/api/rag/... -count=1
```

## Known gaps

- Operators must provision a compatible `llama-server` binary and GGUF embedding model.
- Enabling or disabling the supervised child requires a supervisor restart; gateway RAG config reload alone cannot change the process tree.
- No setup-wizard flow downloads weights or estimates hardware requirements.
