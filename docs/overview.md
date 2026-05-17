# High-level overview

**chimera-gateway** is a small **Go** service in front of **BiFrost** (or any OpenAI-compatible upstream). IDEs and agents (for example **VS Code Continue**) use a **single OpenAI-compatible base URL** and one **virtual model id** (`chimera-<semver>`, e.g. `Chimera-0.1.0`) instead of switching models manually in the UI. The gateway validates **gateway-issued API tokens**, optionally applies **routing rules** to pick an initial backend model, then walks a **configured fallback chain** on **429** or **5xx** from the upstream.

**BiFrost** holds **provider API keys** and talks to Groq, Gemini, and other backends per `config/bifrost.config.json`. The gateway calls BiFrost **only over HTTP**.

**Qdrant** is used by the gateway when `rag.enabled` is **true** in `config/gateway.yaml`: query-time retrieval for the virtual model, `POST /v1/ingest`, and indexer endpoints. Supervise it with `chimera serve` (typical local stack). With `indexer.supervised.enabled`, the same supervisor can start `chimera-indexer` for workspace indexing.

**Shipped in v0.2.x:** virtual model + fallback chain (unchanged from v0.1), YAML tokens and routing policy with **mtime reload**, `GET /health` (upstream + optional Qdrant when RAG is on), **ingestion**, **indexer REST**, `chimera-indexer`, optional **supervised indexer**, correlated logging and logs UI (**v0.2.1**), and shell/indexer/Continue operator pages (**v0.2.2**). Summary: [version-v0.2.md — Shipped releases](version-v0.2.md#shipped-releases-v020-through-v022).
