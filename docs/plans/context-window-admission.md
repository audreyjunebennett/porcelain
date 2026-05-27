# Plan: Context window admission on chat path

| Field | Value |
|-------|-------|
| **Doc kind** | `feature-plan` |
| **Owners / areas** | Gateway chat routing, `internal/providerlimits`, catalog polling, Make/catalog CLIs, operator docs |
| **Status** | `done` |
| **Targets** | Gateway next patch after v0.2 routing baseline |
| **Last updated** | 2026-05-24 |
| **Supersedes / superseded by** | Extends [`version-v0.1.1.md`](../version-v0.1.1.md) §3.7 (TPM/RPM admission); complements [`tokencount-talk.md`](../tokencount-talk.md) |

## At a glance

Large IDE clients (e.g. Cline Act mode) send multi‑thousand‑token prompts. The gateway already skips upstream models when **TPM/RPM quotas** would be exceeded, but it does not check **context window** before calling upstream. Failures like Groq `request_too_large` on HTTP 400 terminate the fallback chain before **Ollama** entries are tried. This plan adds per‑model **context window** and optional **body byte** limits to `provider-model-limits.yaml`, enforces them on every chat attempt like TPM (skip model, continue fallback), seeds limits from the broker catalog, and treats upstream context overflow as a retriable fallback signal.

| Phase | Outcome | Status |
|-------|---------|--------|
| [Phase 1 — Limits schema and admission primitives](#phase-1--limits-schema-and-admission-primitives) | YAML carries context caps; `providerlimits` can deny a request before upstream HTTP | `done` |
| [Phase 2 — Chat routing and upstream retry](#phase-2--chat-routing-and-upstream-retry) | Virtual-model fallback skips over‑large models and retries on `request_too_large` | `done` |
| [Phase 3 — Catalog seeding tooling](#phase-3--catalog-seeding-tooling) | `make` target fills `context_window` from `catalog-available.snapshot.yaml` | `done` |
| [Phase 4 — Live catalog overlay](#phase-4--live-catalog-overlay) | Runtime `/v1/models` poll supplies defaults; YAML overrides win | `in progress` |
| [Phase 5 — Operator visibility](#phase-5--operator-visibility) | Logs and docs explain context skips alongside TPM blocks | `done` |

---

## Background

### Problem observed (2026‑05‑24)

Operator session with Cline → `Chimera-0.2.0` → fallback chain length 11 (`config/gateway.yaml`):

1. RAG attaches 8 vector hits (~15–16k estimated tokens after injection).
2. Attempts 1–7: **local TPM guard** blocks (request ~16k vs Groq TPM caps 6k–12k).
3. Attempt 8 `groq/groq/compound`: Groq **rate limit** → gateway **retries** (correct).
4. Attempt 9 `groq/groq/compound-mini`: Groq **`request_too_large`** on HTTP **400** → gateway **returns error to client** (incorrect).
5. Attempts 10–11 `ollama/llama3.2:3b`, `ollama/qwen3.5:9b`: **never attempted**.

Root causes in current code:

| Gap | Location | Effect |
|-----|----------|--------|
| No context pre-check | `internal/providerlimits` — only RPM/RPD/TPM/TPD | Models with high TPM but small effective context still get HTTP calls |
| `request_too_large` not retriable | `shouldRetryVirtualModelFallback` in [`chat.go`](../../chimera/chimera-gateway/internal/chat/chat.go) — retries HTTP **413**, not 400 + `error.code` | Terminal failure at position 9 |
| Catalog metadata unused | [`catalog-available.snapshot.yaml`](../../config/catalog-available.snapshot.yaml) has `context_length`; [`CatalogSnapshot`](../../chimera/chimera-gateway/internal/server/catalog/availablemodels.go) stores ids only | No runtime context data |
| Token estimate omits `max_tokens` reserve | `prepareChatPayload` + `tokencount.Count` on marshalled JSON only | Under-count vs upstream window math |
| Catalog can overstate limits | e.g. `groq/groq/compound-mini` lists `context_length: 131072` but rejected ~16k prompt | Need operator override field |

### Design distinction: TPM vs context

| Dimension | TPM (today) | Context (this plan) |
|-----------|-------------|---------------------|
| **Question** | “Would this minute/day quota be exceeded?” | “Does this single request fit the model window?” |
| **Data source** | Metrics SQLite + YAML caps | YAML caps (+ optional live catalog) |
| **Formula** | `usage + est ≤ tpm` | `est_prompt + max_tokens_reserve ≤ effective_window` (+ optional byte cap) |
| **On deny** | Skip model in fallback loop | Same — skip, do not call upstream |
| **Metrics** | Required (`LimitsGuard` nil without metrics DB) | **Not** stored in metrics; pure per-request check |

RAG injection order is already correct: attach retrieved context **before** token witness and routing ([`server.go`](../../chimera/chimera-gateway/internal/server/server.go) `handleV1Chat`). Context admission runs on the **final** proxied body.

**Related docs:** [`configuration.md`](../configuration.md) (`provider-model-limits.yaml`), [`tokencount-talk.md`](../tokencount-talk.md), [`version-v0.2.md`](../version-v0.2.md) § usage metrics.

---

## Phase 1 — Limits schema and admission primitives

**Goal.** Operators can declare per‑provider/model context caps in the same file as TPM; the gateway can evaluate them without metrics I/O.

**Deliverables**

- Bump `schema_version` to **2** in [`provider-model-limits.example.yaml`](../../config/provider-model-limits.example.yaml) and document new fields in [`configuration.md`](../configuration.md).
- Extend [`internal/providerlimits`](../../chimera/internal/providerlimits):

  **New optional fields** (same merge order as today: model > provider > defaults):

  | Field | Type | Purpose |
  |-------|------|---------|
  | `context_window` | int64 | Total context tokens (typically from catalog `context_length`) |
  | `max_prompt_tokens` | int64 | Stricter prompt-only cap when vendor/catalog lies |
  | `max_body_bytes` | int64 | Marshalled JSON byte cap (Groq entity-size / proxy limits) |
  | `context_safety_factor` | float64 | Per-layer multiplier (default global e.g. `0.9` in `defaults`) |

  **Effective cap:** `floor(min(context_window, max_prompt_tokens_if_set) × safety_factor)`.

  **New admission API:**

  ```go
  type RequestAdmission struct {
      EstPromptTokens int64 // tiktoken on marshalled body (same as today)
      MaxTokens       int64 // from client `max_tokens`; see Open questions
      BodyBytes       int64 // len(out) after json.Marshal
  }

  const (
      ReasonContext   Reason = "context_window"
      ReasonBodySize  Reason = "request_body_bytes"
  )

  func DecideContext(eff Effective, req RequestAdmission) Decision
  ```

  Extend `Guard.Allow` to accept `RequestAdmission`, run **`DecideContext` first**, then existing `Decide` for RPM/TPM (unchanged metrics path).

- Unit tests in `admission_test.go`, `resolve_test.go`, `guard_test.go`:
  - Merge inherits `context_window` from provider → model override.
  - Deny when `est + max_tokens > cap`; allow when under.
  - `max_prompt_tokens` tighter than `context_window` wins.
  - Nil fields → no enforcement on that dimension.
  - Context check runs without metrics store (guard with `Usage: nil` still enforces context if configured).

**Acceptance**

- `go test ./chimera/internal/providerlimits/...` passes.
- Invalid YAML (unknown fields, negative values) rejected at parse; gateway still falls back to empty spec on load error (existing behavior).
- Example YAML documents at least one **override** entry for `groq/groq/compound-mini` (operator-maintained stricter cap until vendor metadata is trusted).

**Status:** `done`

---

## Phase 2 — Chat routing and upstream retry

**Goal.** Context/body denials behave like TPM denials in the virtual-model fallback loop; upstream context overflow advances to the next model (including Ollama).

**Deliverables**

- [`internal/chat/chat.go`](../../chimera/chimera-gateway/internal/chat/chat.go):

  1. **Parse `max_tokens`** from incoming `body` map (`json.RawMessage` → int). Document default when absent (see Open questions).
  2. After `prepareChatPayload`, call extended guard:

     ```go
     guard.Allow(ctx, upstreamModel, providerlimits.RequestAdmission{
         EstPromptTokens: int64(est),
         MaxTokens:       maxTokensFromBody(body),
         BodyBytes:       int64(len(out)),
     })
     ```

  3. On deny with `ReasonContext` or `ReasonBodySize`: log `chat.provider_limits.blocked` with `reason` set (same code path as TPM), **`continue`** if not last chain entry; else **429** `gateway_provider_limits` with message naming the dimension (mirror TPM exhaustion).
  4. **Upstream retry:** extend `shouldRetryVirtualModelFallback` to return true when:
     - HTTP status is `413`, **or**
     - HTTP 400/422 with parsed `error.code` in `{request_too_large, context_length_exceeded, …}` (start with `request_too_large`; add codes as discovered).
  5. Optionally extend `excluded413` map naming to cover models that failed context (same “don’t retry same model twice on same request” semantics).

- Update [`chat_limits_test.go`](../../chimera/chimera-gateway/internal/chat/chat_limits_test.go):
  - Context block skips model, succeeds on next in chain (parallel to existing TPM skip test).
  - Simulated upstream 400 + `request_too_large` retries to Ollama-like model.
  - Chain exhausted on context → 429 shape.

- Wire unchanged in [`server.go`](../../chimera/chimera-gateway/internal/server/server.go) — still passes `rt.LimitsGuard()`; guard API change is internal.

**Acceptance**

- Repro scenario (large Cline prompt, RAG attached, Groq TPM-saturated): gateway reaches an `ollama/*` model instead of returning Groq `request_too_large` to the client when Ollama is up and within context cap.
- `go test ./chimera/chimera-gateway/internal/chat/...` passes.
- No regression: existing TPM skip / rate-limit retry tests still pass.

**Status:** `done`

---

## Phase 3 — Catalog seeding tooling

**Goal.** Operators do not hand-copy `context_length` for dozens of models; a Make target seeds YAML from the broker catalog snapshot.

**Deliverables**

- New CLI e.g. [`chimera/cmd/catalog-write-limits`](../../chimera/cmd/) (or extend [`catalog-write-available`](../../chimera/cmd/catalog-write-available/main.go)):
  - **Input:** `config/catalog-available.snapshot.yaml` (from existing `make catalog-available`).
  - **Output:** fragment or patched `provider-model-limits.yaml` adding `context_window` per `data[].id` where `context_length` is present.
  - **Preserve** existing RPM/TPM/RPD/TPD values (merge, do not wipe operator quotas).
  - **Ollama gap:** models without `context_length` in catalog → skip with warning, or apply static defaults map in the tool (document in CLI help).
- Makefile target e.g. `make catalog-limits` (document in [`makefile.md`](makefile.md)).
- Seed committed [`provider-model-limits.yaml`](../../config/provider-model-limits.yaml) with `context_window` for fallback-chain models; add **`max_prompt_tokens`** override for `groq/groq/compound-mini` pending vendor fix.

**Acceptance**

- `make catalog-available && make catalog-limits` produces deterministic YAML diff (reviewable).
- Groq models in fallback chain have `context_window` populated from snapshot.
- Tool exits non-zero on missing input file; does not delete TPM fields.

**Status:** `done`

---

## Phase 4 — Live catalog overlay

**Goal.** Context defaults stay current when broker catalog changes (new models, Ollama online/offline) without restarting the gateway.

**Deliverables**

- Extend [`CatalogSnapshot`](../../chimera/chimera-gateway/internal/server/catalog/availablemodels.go):
  - While parsing `/v1/models` `data[]`, capture `context_length` (and optionally `max_input_tokens` when present) into `ModelContext map[string]int64`.
  - Accessor: `(s *CatalogSnapshot) ContextLength(modelID string) (int64, bool)`.
- Extend [`Runtime.LimitsGuard()`](../../chimera/chimera-gateway/internal/server/runtime/runtime.go) (or a thin wrapper used by chat):
  - When resolving effective limits for `upstreamID`, overlay: `context_window = yaml.context_window ?? catalog.context_length` (YAML always wins when set).
  - If catalog snapshot nil/stale/missing field → YAML-only behavior (no regression).
- Tests: catalog provides context when YAML omits field; YAML override beats catalog; stale snapshot does not erase YAML.

**Acceptance**

- With empty `context_window` in YAML but live catalog entry, guard enforces catalog value.
- Operator override in YAML still applies after catalog refresh tick (~30s per `health.available_models_poll_ms`).

**Status:** `done`

---

## Phase 5 — Operator visibility

**Goal.** Operators can see **why** a model was skipped (context vs TPM) in logs and docs without reading code.

**Deliverables**

- Structured log fields on `chat.provider_limits.blocked` for context denies:
  - `outgoingTokens`, `max_tokens`, `body_bytes`, `context_cap`, `reason` (`context_window` | `request_body_bytes`).
- Optional operator-copy slug in [`internal/operatorcopy/messages.yaml`](../../internal/operatorcopy/messages.yaml) for evlog rendering (follow existing `chat.provider_limits.blocked` pattern).
- Update [`configuration.md`](../configuration.md) § provider-model-limits with schema v2, formulas, and link to `make catalog-limits`.
- Short note in [`tokencount-talk.md`](../tokencount-talk.md) cross-linking context admission (implementation ordering item #2 satisfied).

**Acceptance**

- Log line from a context skip is human-parseable in supervisor JSONL.
- Docs describe merge order and difference from TPM.

**Status:** `done`

---

## Open questions

Resolve before or during implementation; remove this section when closed.

1. **Default `max_tokens` when client omits it:** Use `0` (prompt-only check).
2. **`max_prompt_tokens` for `compound-mini`:** [config\catalog-available.snapshot.yaml](config\catalog-available.snapshot.yaml)#678-681 says that the context is 131072
3. **Global `max_body_bytes` in defaults:** Ship `3500000` (~3.5 MB Groq rule of thumb from tokencount discussion)
4. **Schema migration:** There are no users and no legacy support is required.
5. **Context deny on last chain entry:** Stay consistent and use the 400 error that the context is too large. 
6. **Ollama context defaults:** Make a chimera/cmd that lists all the models `ollama list` and captures details about the models `ollama show <MODEL_ID>` if that is not provided from bifrost, ollama running locally on default port, or running the binary.

---

## References

### Code (primary touch points)

| Area | Path |
|------|------|
| TPM admission (pattern to mirror) | [`chimera/internal/providerlimits/`](../../chimera/internal/providerlimits/) |
| Virtual-model fallback loop | [`chimera/chimera-gateway/internal/chat/chat.go`](../../chimera/chimera-gateway/internal/chat/chat.go) `WithVirtualModelFallback` |
| RAG then routing order | [`chimera/chimera-gateway/internal/server/server.go`](../../chimera/chimera-gateway/internal/server/server.go) `handleV1Chat` |
| Limits guard wiring | [`chimera/chimera-gateway/internal/server/runtime/runtime.go`](../../chimera/chimera-gateway/internal/server/runtime/runtime.go) `LimitsGuard` |
| Live catalog poll | [`chimera/chimera-gateway/internal/server/catalog/availablemodels.go`](../../chimera/chimera-gateway/internal/server/catalog/availablemodels.go) |
| Token estimate | [`chimera/internal/tokencount/`](../../chimera/internal/tokencount/), [`docs/tokencount-talk.md`](../tokencount-talk.md) |
| Config load | [`chimera/internal/config/config.go`](../../chimera/internal/config/config.go) `ProviderLimitsSpec` |

### Config examples

- [`config/provider-model-limits.yaml`](../../config/provider-model-limits.yaml) — live quotas (extend with context fields)
- [`config/catalog-available.snapshot.yaml`](../../config/catalog-available.snapshot.yaml) — `context_length` source for seeding
- [`config/gateway.yaml`](../../config/gateway.yaml) — `routing.fallback_chain` (11 entries; Ollama last)

### Tests to extend

- [`chimera/internal/providerlimits/admission_test.go`](../../chimera/internal/providerlimits/admission_test.go)
- [`chimera/chimera-gateway/internal/chat/chat_limits_test.go`](../../chimera/chimera-gateway/internal/chat/chat_limits_test.go)

### Explicit non-goals (v1)

- Automatic RAG truncation when over context (skip model only; no prompt surgery).
- Switching tokenizers to Llama-aligned counts (keep `cl100k_base` parity with TPM metrics).
- Persisting context checks in metrics SQLite.
- UI metrics card showing per-model context caps (optional follow-up).
