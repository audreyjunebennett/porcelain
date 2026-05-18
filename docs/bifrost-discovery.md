# BiFrost discovery (Phase 0 archive)

This document records **Phase 0** of [go-bifrost-migration.plan.md](plans/go-bifrost-migration.plan.md): running **Maxim BiFrost** as the OpenAI-compatible upstream for Chimera. The gateway implementation is **Go** only; use `chimera` or `chimera serve` with a local `bifrost-http` binary (see [supervisor.md](supervisor.md)).

**Operator verification (2026-04-03).** BiFrost was exercised with the repo configuration; **VS Code** (OpenAI-compatible client → **Chimera** at the gateway base URL) was used for real chat.

---

## How we run BiFrost today

| Item | Value |
|------|--------|
| **Binary** | `bifrost-http` — e.g. `make chimera-install` → `./bin/bifrost-http`; pins in `chimera/deps.lock`, or `chimera serve` supervises it |
| **Listen** | Default `127.0.0.1:8080` (`chimera serve` flags: `-bifrost-bind`, `-bifrost-port`) |
| **Health** | `GET http://127.0.0.1:8080/health` |
| **Bootstrap config** | Repo `config/bifrost.config.json` — copied into BiFrost data dir as `config.json` when using `chimera serve` |

**Minimal bring-up**

```bash
export CHIMERA_BROKER_API_KEY=bifrost-local-dummy
export GROQ_API_KEY=...   # per bifrost.config.json
make chimera-install      # once — versions from chimera/deps.lock
make chimera-serve        # or: make up
```

---

## Minimal BiFrost configuration

1. `config/bifrost.config.json` — providers and `env.VAR_NAME` key references (no raw secrets in JSON).
2. **Environment** — `GROQ_API_KEY`, `GEMINI_API_KEY`, etc., in the shell (or `.env`) when starting `chimera` / `chimera serve`.
3. **Optional:** `./scripts/list-bifrost-models.sh` or `curl` `/api/models?unfiltered=true&limit=500` on BiFrost.

---

## `env.*` in bootstrap JSON (what BiFrost actually expands)

BiFrost does **not** treat every string in `config.json` as an environment reference. Resolution depends on the **Go field type** in Maxim’s BiFrost schemas, not on the JSON key name.

| Location | Type in BiFrost | `"env.MY_VAR"` in JSON |
|----------|-----------------|-------------------------|
| `providers.<name>.keys[].value` | `EnvVar` | **Yes** — BiFrost records the reference and resolves `MY_VAR` from the process environment when it needs the secret (same idea as `env.GROQ_API_KEY`, `env.GEMINI_API_KEY`). |
| `providers.<name>.network_config.base_url` | Plain `string` | **No** — the value is stored and used **literally**. A value like `env.OLLAMA_BASE_URL` is **not** looked up in the environment; the Ollama (and similar) code paths trim it and use it as the HTTP base URL, which breaks if you intended indirection. |
| Other `EnvVar`-typed fields (Azure, Vertex, Bedrock, MCP `connection_string`, etc.) | `EnvVar` | **Yes**, per BiFrost’s rules for those structs. |

**Implications for Ollama:** set `network_config.base_url` to a real URL string (e.g. `http://localhost:11434`) in `config/bifrost.config.json`, unless you introduce your own preprocessing before BiFrost reads the file.

**Chimera today:** `chimera serve` copies `config/bifrost.config.json` into the BiFrost data directory as `config.json` without rewriting or expanding strings ([`CopyConfigJSON`](../internal/supervisor/bifrost.go)); environment variables from `.env` still apply to **key** values BiFrost resolves at runtime.

**`env.example`:** A variable such as `OLLAMA_BASE_URL` documents the URL you use for local Ollama; it does **not** by itself substitute into `network_config.base_url` in BiFrost (see table above).

Upstream BiFrost has had requests to support `env.*` more broadly across arbitrary JSON strings (not only `EnvVar` fields); until that exists, `base_url` stays a literal string in the bootstrap file.

---

## Gateway configuration — Chimera → BiFrost

Gateway YAML uses `upstream.*` for the OpenAI-compatible hop (BiFrost or any compatible proxy). Legacy `litellm` / `health.litellm_url` keys are still accepted when the corresponding `upstream` / `health.upstream_url` fields are omitted.

| Field | Role |
|-------|------|
| `upstream.base_url` | Upstream root. Local default `http://127.0.0.1:8080`. `chimera serve` overrides this to match the supervised BiFrost. |
| `upstream.api_key_env` | Env var for `Authorization: Bearer` on upstream `/v1/*`. Default `CHIMERA_BROKER_API_KEY`. |
| `routing.fallback_chain` | Ordered BiFrost model ids as `provider/model`. |
| `paths.tokens` / `paths.routing_policy` | Gateway auth and routing policy. |

### Request correlation for operator logs

The gateway sets upstream `X-Request-Id` to its structured `request_id` on each `/v1/chat/completions` relay. This is the only request header Phase 5 depends on for BiFrost correlation because common clients such as VS Code Continue and Cline do not reliably send or preserve custom Chimera headers.

BiFrost subprocess rows may join a conversation only if BiFrost exposes `X-Request-Id` in its own logs. When it does not, the gateway's in-process relay logs (`chat.bifrost.*`, `chat.routing.*`, and `conversation.upstream.*`) remain canonical for the conversation card; BiFrost subprocess rows stay on the BiFrost service card.

---

## Compatibility notes (BiFrost behind Chimera)

| Area | Behavior |
|------|----------|
| **Chat** | `POST /v1/chat/completions`; streaming SSE pass-through |
| **Model list** | Gateway prefers `GET /api/models?unfiltered=true&limit=500`, maps to `provider/name`, prepends `chimera-<semver>` |
| **Health** | `GET {base_url}/health` — JSON `checks.upstream` reflects upstream probe |
| **Fallback** | **429** / selected **5xx** walk `routing.fallback_chain` |

---

## References

- [configuration.md](configuration.md)
- [supervisor.md](supervisor.md)
- [go-bifrost-migration.plan.md](plans/go-bifrost-migration.plan.md)
- Upstream: [BiFrost docs](https://docs.getbifrost.ai/)
