# Cognitive Router & Meta-Policy Engine Architecture

> **North-star architecture** — not all layers below are shipped. For **as-built** chat routing, RAG, and virtual models see [features/README.md](features/README.md) and [gateway-chat-routing-pipeline.md](features/gateway-chat-routing-pipeline.md). Normative product requirements: [chimera.plan.md](chimera.plan.md).

## Purpose

This document describes the architecture of a **Cognitive Routing Gateway** sitting in front of an LLM infrastructure gateway (e.g., BiFrost).
Its goal is to provide **semantic routing, policy enforcement, RAG integration, and privacy-aware model selection** while remaining compatible with editor tooling such as VS Code + Continue and OpenAI-compatible APIs.

This system intentionally **does not replace** existing LLM gateways or agent frameworks. Instead, it forms an intermediate decision layer focused on:

- **What is allowed** (policy)
- **What is optimal** (routing)
- **What context may be used** (RAG + privacy boundaries)

---

## Architectural Overview

```
Editor / Client Tooling (VS Code, Continue)
            ↓
  Cognitive Routing Gateway
    - Meta-Policy Engine
    - Classification & Annotation Layer
    - Cognitive Router
    - RAG / Vector Subsystem
            ↓
        BiFrost Gateway
            ↓
        LLM Providers
    (local + cloud)
```

---

## Meta-Policy Engine (Hard Constraints)

The **Meta-Policy Engine** evaluates every request before any model, router, or retriever is invoked.

It answers:
> *"What is allowed for this request?"*

### Responsibilities
- Enforce privacy boundaries (local-only vs cloud-allowed)
- Enforce cost ceilings
- Enforce project/workspace constraints
- Enforce RAG eligibility

Policies are **non-negotiable** and deterministic. This layer **never calls an LLM**.

---

## Classification & Annotation Layer

This layer extracts **signals** and attaches **metadata** to a turn.

### Classifiers

Classifiers determine:
- Task type (debug, refactor, summarize, design)
- Complexity (low / medium / high)
- Sensitivity (public / internal / private)

Classifier output example:
```json
{
  "task_type": "code_refactor",
  "complexity": "medium",
  "sensitivity": "private"
}
```

### Per-Turn Annotations

Annotations are metadata carried alongside the request (not prompt text):

```json
{
  "preferred_cost": "low",
  "allow_cloud": false,
  "requires_rag": true,
  "workspace_id": "project-x"
}
```

### Conversational Intent Tagging

Intent tags represent the user's **goal**, not phrasing. Examples:
- explore
- debug
- refactor
- explain

Intents may persist across turns and influence routing decisions.

---

## Cognitive Router

The Cognitive Router answers:
> *"Given what is allowed, what is optimal?"*

### Responsibilities
- Select model tier
- Decide between local vs cloud
- Trigger fan-out execution
- Invoke judge models
- Escalate when confidence is low

### Fan-Out + Judge Pattern

```
Prompt
 ├─ cheap local model
 ├─ cheap cloud model (if allowed)
 ├─ mid-tier model
 ↓
Judge model
 ↓
Final answer or escalation
```

Judge models score, compare, and decide escalation without accessing private context unless permitted.

---

## RAG / Vector Subsystem

### Workspace-Scoped Indexing

Each project/workspace has a unique ID. Indexed chunks include metadata:

```json
{
  "workspace_id": "project-x",
  "file_path": "src/auth/login.ts",
  "sensitivity": "private",
  "hash": "..."
}
```

### Retrieval Flow
1. Policy approval
2. Workspace-scoped vector search
3. Top-K result selection
4. Context packaging

### Context Injection

```
SYSTEM:
You may use the following context if relevant.

CONTEXT:
<snippets>

USER:
<question>
```

---

## Privacy-Preserving RAG Strategies

### Rule: Routing Before Retrieval

LLMs never decide data eligibility.

### Safe Patterns

1. **Local-only RAG**
   - Private context → local models only

2. **Summary Indirection**
   - Local model summarizes
   - Cloud models see only redacted summaries

3. **Split-Brain Reasoning**
   - Local model answers factual questions
   - Cloud model formats or explains

---

## Data Structures & Storage

### Configuration
- `configs/projects/{workspace_id}.yaml`
- `configs/policies.yaml`
- `configs/models.yaml`

### Runtime Metadata
- Per-turn annotations (in-memory)
- Conversational state
- Audit logs (append-only)

### Vector Store
- Workspace-scoped namespaces
- Chunk-level metadata
- Hash-based reindexing

---

## OpenAI-Compatible API Envelope

```json
{
  "model": "auto",
  "messages": [...],
  "metadata": {
    "workspace_id": "project-x",
    "annotations": {...}
  }
}
```

---

## Ideal Order of Implementation

1. Meta-Policy Engine
2. Annotations & deterministic classifiers
3. Cognitive Router (static → fan-out → judges)
4. BiFrost integration
5. RAG indexer
6. Privacy-aware context injection
7. Observability and auditing

---

## Non-Goals

- Full agent runtimes
- Tool execution
- Autonomous planning loops

---

## Summary

This system forms a **missing middle layer** between developer tools and LLM infrastructure, providing semantic routing, strong privacy guarantees, and extensibility without locking into a single agent framework.
