# Token counting — design notes

> **As-built behavior:** [context-window-admission](../features/context-window-admission.md), [gateway-chat-routing-pipeline](../features/gateway-chat-routing-pipeline.md), [configuration.md](../configuration.md) § provider-model-limits.

This document captures trade-offs about how **chimera-gateway** estimates tokens, why **upstream errors** (e.g. Groq **413**) can disagree with local counts, and what several models suggested for more accurate pre-calculation. It is **not** a commitment to implement every idea.

---

## Quick summary — who suggested what

| Source | Focus | Main suggestions |
|--------|--------|-------------------|
| **Cursor (assistant)** | Grounded in this repo | Today: **`json.Marshal` of the full proxied body** → `cl100k_base` `EncodeOrdinary` only (no template tax, no `max_tokens` reserve). **413** comes from **upstream** rules, not “gateway count + estimated response.” **Bytes vs tokens:** large JSON/base64 can hit **byte limits** before token limits. **Parity:** `tiktoken-go` *is* tiktoken-compatible; compare encodings (e.g. `o200k_base`) or Python goldens without bloating prod. **Heuristics:** prefer **structured** checks (payload bytes, `prompt + max_tokens ≤ window`, optional tools slack) over blind **2× tools** or **4–6×** on top of an already full-body count (**double-count risk**). |
| **Gemini** | General API / ops advice | **(1) Chat template tax:** ~4–10 tokens per turn; Groq/Llama-style: ~**4 per message + 3** for final assistant priming; **Gemini:** use `countTokens` API instead of guessing. **(2) JSON vs 413:** treat **HTTP body size** (~**4 MB** Groq rule of thumb); cap **~3.5 MB** bytes. **(3) Local formula:** e.g. `(stringTokens × 1.05) + (messageCount × 8) + reserve`; **Llama 3–style tokenizer** for Groq; **tools × ~1.2**; always account for `max_tokens` as **reservation** against context; **~90%** of advertised limits as internal cap; **~12 tokens per** system/user/assistant block variant. |
| **Grok** | Heuristic overlays | **Double-count encoded tools** (literal second pass on tool cost) — flagged as **too blunt** if the base already includes `tools` in full JSON. Later: **+4 tokens per message**; **+50–200** when assistant has `tool_calls`; **+3–5** per `role: tool` message; **+3** end assistant priming; **+10–20** request-level overhead — adopt **spirit** with **config**, but `tool_calls` should be **size-based**, not a flat 50–200. |
| **Groq (`llama-3.3-70b-versatile`)** | Tool-heavy requests | Break down tool cost: **~5–10** tokens name+description; **~2–5** per parameter schema chunk; **~2–5** “between tools”; **~10–20** structured prefix. Proposed **multipliers on tool JSON token count:** **simple 2–3×**, **complex 4–6×**. Treat as **rough**; actual depends on **internal formatting**. |

### Repo tooling

- `make tokencount-file FILE=path/to/file` — runs `go run ./cmd/chimera tokencount -f "$(FILE)"` and prints **byte size**, `cl100k_base`, and `o200k_base` token counts for that file (requires `FILE=…`; see `make help`).
- Gateway **metrics / TPM admission** still use the **chat path** estimate (full marshalled body + `cl100k_base` via `internal/tokencount`), not the Makefile-only dual-encoding display.

---

## Current implementation (facts)

- `internal/chat` builds the outbound body with `json.Marshal` (after setting `model` and `stream`), then `internal/tokencount.Count` runs **tiktoken `cl100k_base`** `EncodeOrdinary` on the **entire UTF-8 string** of that JSON.
- There is **no** per-message template tax, **no** `max_tokens` reserve, **no** tool multiplier in that path.
- **Provider limits** compare YAML TPM/RPM/etc. to metrics using that **same** estimate; a **TPM block** surfaces as **429** `gateway_provider_limits` with `reason=tpm`, not Groq’s **413**. **Context admission** runs before upstream HTTP; a **context block** logs `chat.provider_limits.blocked` with `reason=context_window` and skips to the next fallback model.

**413 vs local estimate:** Upstream uses **its own** tokenization and limits. A local estimate can still **413** if their count is higher, `max_tokens` eats the rest of the window, or a **byte / proxy** limit fires first.

**Bytes vs tokens:** **Bytes** measure the HTTP body; **tokens** measure BPE pieces. They diverge with **base64**, **huge `tools`**, or **reverse-proxy max body**.

---

## Implementation ordering (suggested)

1. Outbound **JSON byte cap** + clear gateway error. **Done** — `max_body_bytes` in `provider-model-limits.yaml`.
2. **`max_tokens` + context window** reserve. **Done** — see [context-window-admission](../features/context-window-admission.md).
3. Per-message / tool **overhead** (YAML-tunable).
4. **Groq:** Llama-aligned tokenizer when maintainable.
5. **Gemini:** `countTokens` on the Gemini path.
6. Optional global fallback formula only if parsing is unavailable.

---

## One-line summary

**Gateway:** `json.Marshal` → entire string → `cl100k_base` `EncodeOrdinary` → metrics + quota admission. **CLI / Make:** `make tokencount-file FILE=…` additionally prints **bytes** and `o200k_base` for file comparison — **not** wired into the proxy path unless you change `internal/chat`.

For the full multi-model debate (Gemini/Grok/Groq detailed sections), see git history of `docs/tokencount-talk.md` (removed; content summarized above).
