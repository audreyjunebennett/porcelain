# Token counting discussion (conversation wrap-up)

This document summarizes a multi-turn discussion about how **chimera-gateway** estimates tokens today, why **upstream errors** (e.g. Groq **413**) can disagree with local counts, and what **several models** suggested for more accurate pre-calculation. It is **not** a commitment to implement every idea; see the detailed sections for trade-offs.

---

## Quick summary — who suggested what

| Source | Focus | Main suggestions |
|--------|--------|-------------------|
| **Cursor (assistant)** | Grounded in this repo | Today: **`json.Marshal` of the full proxied body** → `cl100k_base` `EncodeOrdinary` only (no template tax, no `max_tokens` reserve). **413** comes from **upstream** rules, not “gateway count + estimated response.” **Bytes vs tokens:** large JSON/base64 can hit **byte limits** before token limits. **Parity:** `tiktoken-go` *is* tiktoken-compatible; compare encodings (e.g. `o200k_base`) or Python goldens without bloating prod. **Heuristics:** prefer **structured** checks (payload bytes, `prompt + max_tokens ≤ window`, optional tools slack) over blind **2× tools** or **4–6×** on top of an already full-body count (**double-count risk**). |
| **Gemini** | General API / ops advice | **(1) Chat template tax:** ~4–10 tokens per turn; Groq/Llama-style: ~**4 per message + 3** for final assistant priming; **Gemini:** use `countTokens` API instead of guessing. **(2) JSON vs 413:** treat **HTTP body size** (~**4 MB** Groq rule of thumb); cap **~3.5 MB** bytes. **(3) Local formula:** e.g. `(stringTokens × 1.05) + (messageCount × 8) + reserve`; **Llama 3–style tokenizer** for Groq; **tools × ~1.2**; always account for `max_tokens` as **reservation** against context; **~90%** of advertised limits as internal cap; **~12 tokens per** system/user/assistant block variant. |
| **Grok** | Heuristic overlays | **Double-count encoded tools** (literal second pass on tool cost) — flagged as **too blunt** if the base already includes `tools` in full JSON. Later: **+4 tokens per message**; **+50–200** when assistant has `tool_calls`; **+3–5** per `role: tool` message; **+3** end assistant priming; **+10–20** request-level overhead — adopt **spirit** with **config**, but `tool_calls` should be **size-based**, not a flat 50–200. |
| **Groq (`llama-3.3-70b-versatile`)** | Tool-heavy requests | Break down tool cost: **~5–10** tokens name+description; **~2–5** per parameter schema chunk; **~2–5** “between tools”; **~10–20** structured prefix. Proposed **multipliers on tool JSON token count:** **simple 2–3×**, **complex 4–6×**. Treat as **rough**; actual depends on **internal formatting**. |

### Repo tooling (added during this discussion)

- `make tokencount-file FILE=path/to/file` — runs `go run ./cmd/chimera tokencount -f "$(FILE)"` and prints **byte size**, `cl100k_base`, and `o200k_base` token counts for that file (requires `FILE=…`; see `make help`).
- Gateway **metrics / TPM admission** still use the **chat path** estimate (full marshalled body + `cl100k_base` via `internal/tokencount`), not the Makefile-only dual-encoding display.

---

## Detailed notes — Cursor (assistant in this thread)

**Current implementation (facts)**  

- `internal/chat` builds the outbound body with `json.Marshal` (after setting `model` and `stream`), then `internal/tokencount.Count` runs **tiktoken `cl100k_base`** `EncodeOrdinary` on the **entire UTF-8 string** of that JSON.  
- There is **no** concatenation of only `messages[].content`; **no** per-message template tax, **no** `max_tokens` reserve, **no** tool multiplier in that path.  
- **Provider limits** (`internal/providerlimits`) compare YAML TPM/RPM/etc. to metrics using that **same** estimate; a **TPM block** surfaces as **429** `gateway_provider_limits` with `reason=tpm`, not Groq’s **413**. **Context admission** (same estimate + client `max_tokens` reserve) runs before upstream HTTP; a **context block** logs `chat.provider_limits.blocked` with `reason=context_window` and skips to the next fallback model. See [`context-window-admission.md`](plans/context-window-admission.md) and [`configuration.md`](configuration.md) § provider-model-limits.

**413 vs local estimate**  

- Upstream uses **its own** tokenization and limits (**context**, `max_tokens`, internal tool rendering). A local **~5000** estimate can still **413** if their count is higher, `max_tokens` eats the rest of the window, or a **byte / proxy** limit fires first.

**Bytes vs tokens**  

- **Bytes** measure the HTTP body; **tokens** measure BPE pieces. They diverge with **base64**, **huge `tools`**, or **reverse-proxy max body** — you can fail on **size** while a token heuristic still looks “fine.”

**`cl100k_base` vs “tiktoken”**  

- The repo uses `github.com/pkoukk/tiktoken-go`; `cl100k_base` is one encoding defined in that ecosystem. Comparisons are really **encoding vs encoding** (e.g. `o200k_base`) or **Go vs Python** goldens—not “tiktoken vs cl100k.”

**Critique of other models’ numbers**  

- **Literal double-count or 4–6× on top of full-body tiktoken** risks **double-counting** the `tools` slice unless you **replace** that slice’s contribution or add only `max(0, adjusted − raw_tools_tokens)`.  
- Flat **+50–200** for any `tool_calls` is too wide; prefer **size-linked** slack.  
- Structured parsing of `tools` is **feasible**; combine **tiktoken on extracted strings** with **small configurable** template constants rather than treating chat-generated integers as specs.

**Implementation ordering (suggested earlier in thread)**  

1. Outbound **JSON byte cap** + clear gateway error where useful. **Done** — `max_body_bytes` in `provider-model-limits.yaml` (`request_body_bytes` deny).
2. **`max_tokens` + context window** reserve (when context per model is known). **Done** — context admission on chat path; see [`context-window-admission.md`](plans/context-window-admission.md).
3. Per-message / tool **overhead** (YAML-tunable).  
4. **Groq:** Llama-aligned tokenizer when maintainable.  
5. **Gemini:** `countTokens` on the Gemini path.  
6. Optional global fallback formula only if parsing is unavailable.

---

## Detailed notes — Gemini (third-party suggestions)

Gemini framed three **layers**:

1. **Chat template “tax”**  
   - APIs wrap text in **special / role** tokens; encoding **raw strings only** under-counts by on the order of **~4–10 tokens per turn**.  
   - For **Groq / Llama-style** templates, one concrete pattern mentioned: **~4 tokens per message** plus **~3** for the **final assistant** priming segment.  
   - For **Gemini itself**, it recommended `countTokens` (described as low-cost and accurate) instead of purely local guessing.

2. **JSON payload vs token gap (especially 413)**  
   - Some **413** behavior is tied to **request / entity size in bytes**, not token math.  
   - Rule of thumb: keep total JSON under about **4 MB** for Groq; optionally enforce a **~3.5 MB** safety cap using something equivalent to `Buffer.byteLength(JSON.stringify(payload))` in Node terms (in Go: `len(jsonBytes)` on the marshalled body).

3. **Updated local strategy**  
   - Example weighted form: **Total ≈ (stringTokens × 1.05) + (messageCount × 8) + reserve**.  
   - **Tokenizer:** consider **Llama 3–oriented** patterns for Groq instead of assuming `cl100k_base` matches server counts.  
   - **Tools:** token-count tool JSON and apply about **×1.2** for internal “flattening.”  
   - **Completion reserve:** enforce `prompt + max_tokens ≤ context_window` mentally before trusting prompt size.  
   - **Buffer:** use ~**90%** of advertised caps; add metadata padding (e.g. **~12 tokens per** system/user/assistant block in one variant); stress that **`max_tokens` reserves** capacity—it is not “free” on top of the prompt.

**Cursor’s cross-cutting note:** Treat **(1)** and **(3)** as **design knobs**; treat **(2)** as **orthogonal byte enforcement**. If the base remains **full-body tiktoken**, additive template numbers may **overlap** JSON syntax already counted—tune so you do not stack redundant slack.

---

## Detailed notes — Grok

**Suggestion A — “double count” encoded tools**  

- Idea: tools cost more than one pass of raw JSON suggests.  
- **Risk:** With a **single full-body** count, tools are **already inside** the string once; “double count” without **replacing** that slice **over-counts** and tightens TPM / metrics incorrectly unless carefully defined.

**Suggestion B — additive overlays**  

- **+4 tokens per message** (template overhead).  
- **+50–200** when an **assistant** message contains `tool_calls` (wide band).  
- **+3–5** extra per `role: "tool"` message.  
- **+3** at the end for **final assistant priming**.  
- **+10–20** request-level overhead.

**Assessment (as discussed):** Per-message, end priming, and small request padding are **reasonable as configurable defaults** if aligned with your counting base. The `tool_calls` line should be **scaled by payload size** (or tiktoken on the `tool_calls` subtree), not a flat 50–200. Always clarify whether these sit **on top of full-body tiktoken** (overlap risk) or on top of a **narrower semantic base**.

---

## Detailed notes — Groq (`llama-3.3-70b-versatile` chat output)

The model gave a **decomposed** story for **tool-related** prompt growth:

| Ingredient (as stated) | Rough range cited |
|--------------------------|-------------------|
| Tool **name + description** | ~**5–10** tokens each |
| **Parameter schemas** | ~**2–5** tokens each (complexity-dependent) |
| **Role / separators** between tools | ~**2–5** tokens per tool |
| **Structured prefix** for tools | ~**10–20** tokens (complexity-dependent) |

It then proposed **multipliers on the tiktoken count of the tool’s JSON alone**:

- **Simple tools:** ~**2–3×**  
- **Complex tools:** ~**4–6×**  
- Example: **100** tool-JSON tokens → **200–300** or **400–600** estimated.

**What is usable in code**  

- You **can** parse `tools[]` (OpenAI-style `function` tools), walk **names, descriptions, and `parameters` JSON Schema** (with **depth / size caps**), and emit either:  
  - **tiktoken on extracted fragments** + small constants, or  
  - **pure counting** (number of tools, properties, nested nodes) × **YAML-configured** tokens per node.

**Caveats**  

- The numeric ranges are **not derived from your JSON**; they are **prior guesses**.  
- **4–6×** on top of an estimate that **already includes** the same tool JSON in a **full-body** string is **dangerous** unless you **subtract** the raw tools contribution first or only add `max(0, k·T_tools − T_tools)`.  
- **Calibration** with real `usage.prompt_tokens` (when logged) beats adopting **2–6×** as a universal law.

---

## One-line “where the repo is today”

**Gateway:** `json.Marshal` → entire string → `cl100k_base` `EncodeOrdinary` → metrics + quota admission. **CLI / Make:** `make tokencount-file FILE=…` additionally prints **bytes** and `o200k_base` for file comparison—**not** wired into the proxy path unless you change `internal/chat`.
