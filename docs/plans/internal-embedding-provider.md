# Internal embedding provider (v0.3)

## Summary

Supervised **chimera-embed** wrapper launches **llama-server** in `--embedding` mode (GGUF **nomic-embed-text**, 768-dim). Gateway and indexer use the existing OpenAI-compatible **`POST /v1/embeddings`** client (`ragembed`); when `internal_embedding.enabled` is true, `rag.embedding` is rewired to the local backend instead of Ollama via chimera-broker.

## Runtime choice

| Option | Decision |
|--------|----------|
| ONNX in-process | Deferred — higher integration cost; GGUF + llama-server matches assistants `docker/llmservice-llamacpp` |
| Sidecar (llama-server) | **Selected** — same wrapper pattern as chimera-vectorstore / chimera-broker |
| Ollama for embeddings | Replaced when internal provider enabled |

## Legal / weights

- **nomic-embed-text** — Apache 2.0; redistribution of GGUF is operator/org policy.
- Default path: operator-supplied **`internal_embedding.model_path`** (no weights vendored in repo).
- Future: download-on-first-enable with checksum (org mirror friendly).

## Config (`gateway.yaml`)

```yaml
internal_embedding:
  enabled: true
  provider: "internal"
  model: "internal/nomic-embed-text"
  dim: 768
  base_url: "http://127.0.0.1:8090"
  model_path: "../data/embedding/models/nomic-embed-text.gguf"
  cache_dir: "../data/embedding/cache"
  log_level: "info"

rag:
  enabled: true
  embedding:
    path: "/v1/embeddings"
    model: "ollama/nomic-embed-text:latest"  # auto-rewired to internal/* when internal_embedding is on
    dim: 768
```

## Supervision

`chimera-supervisor` starts **chimera-embed** after vectorstore when `internal_embedding.enabled` (before broker/gateway). Flags: `-embed-bin`, `-embed-listen` (7750), `-embed-endpoint` (8090), `-embed-model-path`, `-wait-embed`.

## Tests

- Unit: config, llamaserver start validation, embedprobe, indexer internal health
- E2E: chimera-embed wrapper + fake llama-server (`make chimera-embed-test-e2e`)

## Recommendation

**Pilot behind flag in v0.3** — ship `internal_embedding.enabled: false` default; enable for operators with GGUF on disk and llama-server in `chimera/bin/`. Full wizard combobox entry can follow once pilot validates RAM/disk on target machines.
