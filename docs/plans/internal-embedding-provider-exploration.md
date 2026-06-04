## Plan: Internal Embedding Provider Exploration

| Field                          | Value                         |
|--------------------------------|-------------------------------|
| **Doc kind**                   | `feature-plan`                |
| **Owners / areas**             | Gateway, indexer              |
| **Status**                     | `draft`                       |
| **Targets**                    | gateway v0.4, indexer Phase 7 |
| **Last updated**               | See git history               |
| **Supersedes / superseded by** | None                          |

## At a glance

Explore loading and running an embedding model inside the gateway stack to reduce reliance on external services like Ollama. This plan aims to provide an internal embedding capability, starting off by default and configurable via `gateway.yaml`. The goal is to expose an OpenAI-compatible `/embeddings` endpoint on localhost.

| Phase                                                                            | Outcome                                                         | Status |
|----------------------------------------------------------------------------------|-----------------------------------------------------------------|--------|
| [Phase 1 — Embedding Model Selection](#phase-1--embedding-model-selection)       | Select a suitable embedding model                               | `todo` |
| [Phase 2 — Inference Backend Evaluation](#phase-2--inference-backend-evaluation) | Evaluate inference backend options (native runtime vs. sidecar) | `todo` |
| [Phase 3 — Integration with Indexer](#phase-3--integration-with-indexer)         | Integrate internal embedding with indexer                       | `todo` |

## Background

The current reliance on external services like Ollama for embeddings poses operational challenges. By developing an internal embedding provider, we can improve performance, reduce latency, and enhance control over the embedding process.

## Phase 1 — Embedding Model Selection

**Goal.** Select a suitable embedding model that meets performance and accuracy requirements.

**Deliverables**

* Research and shortlist candidate models (e.g., BGE-M3, bge-base-en-v1.5)
* Evaluate models based on criteria like performance, accuracy, and license terms
* Recommend a model for further exploration

**Acceptance**

* A documented list of candidate models and their characteristics
* A recommended model for further exploration

**Status:** `todo`

## Phase 2 — Inference Backend Evaluation

**Goal.** Evaluate options for running the embedding model, including native runtimes and sidecars.

**Deliverables**

* Research and evaluate native runtime options (e.g., ONNX Runtime, GGUF)
* Assess sidecar options for isolation and scalability
* Recommend an approach for running the embedding model

**Acceptance**

* A documented evaluation of native runtime and sidecar options
* A recommended approach for running the embedding model

**Status:** `todo`

## Phase 3 — Integration with Indexer

**Goal.** Integrate the internal embedding provider with the indexer.

**Deliverables**

* Implement OpenAI-compatible `/embeddings` endpoint
* Integrate with indexer for seamless embedding
* Test and validate the integrated solution

**Acceptance**

* A working implementation of the internal embedding provider
* Successful integration with indexer

**Status:** `todo`

## Open questions

1. What are the key performance metrics for the internal embedding provider?
2. How will we ensure compatibility with existing indexer and gateway components?

## References

* Code: `internal/...`, `cmd/...`
* Docs: [`configuration.md`](../configuration.md)
* Tickets / PRs: …