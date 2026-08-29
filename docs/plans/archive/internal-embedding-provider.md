# Internal embedding provider

| Field | Value |
|-------|-------|
| **Doc kind** | `feature-plan` |
| **Status** | `shipped` |
| **Areas** | Gateway, indexer, supervisor, `chimera-embed` |
| **As-built** | [`features/internal-embedding-provider.md`](../../features/internal-embedding-provider.md) |
| **Source** | PR #14 behavior salvaged onto current `main`; old branch not merged or rebased |

## Goal

Allow gateway-owned indexing and retrieval embeddings to use a supervised local `llama-server` with an operator-supplied Nomic GGUF model when `internal_embedding` is enabled, without requiring Ollama for embeddings.

## Delivered boundary

- Opt-in `internal_embedding` config and direct RAG endpoint selection.
- Current-contract `chimera-embed` wrapper and supervisor lifecycle integration.
- Direct endpoint/dimension probe for indexer health.
- Recognition by the existing RAG embedding settings API.
- Focused build/test wiring and fake-server E2E coverage.

## Explicitly not carried forward from PR #14

- Unrelated provider/settings UI redesigns, context-limit work, workspace styling, log presentation work, or config architecture.
- Old installer, cleanup, release-packaging, and model-download scripts.
- Deleted files or old plans that current feature records supersede.

## Decision notes

- Sidecar inference remains the selected approach; the existing gateway `ragembed` client already speaks the required OpenAI-compatible API.
- The feature defaults off. Operators retain the broker/Ollama path unless they explicitly enable it.
- Porcelain supervises the process but does not redistribute llama.cpp or model weights.
