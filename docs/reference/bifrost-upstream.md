# BiFrost upstream reference

Maxim **BiFrost** (`bifrost-http`) is the default **LLM broker backend** behind **`chimera-broker`**. The gateway talks OpenAI-compatible HTTP to the broker; the broker holds provider keys and relays to Groq, Gemini, Ollama, and other configured backends.

**Related:** [supervisor.md](../supervisor.md), [configuration.md](../configuration.md), [network.md](../network.md), [product naming](../features/product-naming-contract.md), [gateway chat routing pipeline](../features/gateway-chat-routing-pipeline.md).

---

## Stack layout

| Layer | Binary | Role |
|-------|--------|------|
| Client | IDE / Continue | `POST /v1/chat/completions` → **chimera-gateway** |
| Gateway | **chimera-gateway** | Virtual models, routing, RAG, logs |
| Wrapper | **chimera-broker** | Supervises `bifrost-http`; `BROKER__*` env |
| Upstream | **bifrost-http** | Multi-provider OpenAI-compatible proxy |

Typical local bring-up:

```bash
export CHIMERA_BROKER_API_KEY=bifrost-local-dummy
export GROQ_API_KEY=...   # per config/bifrost.config.json
make chimera-install
make chimera-supervisor-run   # or: make up
```

`make chimera-install` pins upstream versions in `chimera/deps.lock` and places `bifrost-http` under `./bin/`.

---

## Listen addresses and health

| Item | Default |
|------|---------|
| Broker **backend** (BiFrost HTTP) | `127.0.0.1:8080` |
| Broker **wrapper control** | `127.0.0.1:7730` (see [network.md](../network.md)) |
| BiFrost health | `GET http://127.0.0.1:8080/health` |

The supervisor wires `upstream.base_url` in gateway config to the supervised broker backend address.

---

## Bootstrap configuration

1. **`config/bifrost.config.json`** — Providers and `env.VAR_NAME` key references (no raw secrets in JSON). The broker wrapper copies this into the BiFrost data directory as `config.json` without rewriting strings.
2. **Environment** — `GROQ_API_KEY`, `GEMINI_API_KEY`, `CHIMERA_BROKER_API_KEY`, etc., in the supervisor/shell environment (or `.env` loaded by the parent process).
3. **Model catalog** — `./scripts/list-bifrost-models.sh` or `curl` `GET /api/models?unfiltered=true&limit=500` on the BiFrost backend.

---

## `env.*` in bootstrap JSON

BiFrost resolves environment references based on the **Go field type** in its schemas, not on arbitrary JSON strings.

| Location | Type in BiFrost | `"env.MY_VAR"` in JSON |
|----------|-----------------|-------------------------|
| `providers.<name>.keys[].value` | `EnvVar` | **Yes** — resolved from process environment at runtime |
| `providers.<name>.network_config.base_url` | Plain `string` | **No** — used **literally**; `env.OLLAMA_BASE_URL` is not expanded |
| Other `EnvVar`-typed fields (Azure, Vertex, Bedrock, MCP, …) | `EnvVar` | **Yes**, per BiFrost rules |

**Ollama:** set `network_config.base_url` to a real URL (e.g. `http://localhost:11434`) in `config/bifrost.config.json`.

**`env.example`:** documents URLs and keys for operators; it does not substitute into non-`EnvVar` fields.

---

## Gateway → broker wiring

Gateway YAML uses `upstream.*` for the OpenAI-compatible hop. Legacy `litellm` / `health.litellm_url` keys are still accepted when the corresponding `upstream` / `health.upstream_url` fields are omitted.

| Field | Role |
|-------|------|
| `upstream.base_url` | Broker backend root (e.g. `http://127.0.0.1:8080`). Supervisor overrides to match supervised listen addresses. |
| `upstream.api_key_env` | Env var for `Authorization: Bearer` on upstream `/v1/*`. Default `CHIMERA_BROKER_API_KEY`. |
| Virtual model stacks | Per-model fallback chains and routing rules in operator SQLite — see [operator-virtual-models](../features/operator-virtual-models.md). |
| `paths.tokens` / routing policy | Gateway auth and policy file paths. |

---

## Request correlation for operator logs

The gateway sets upstream **`X-Request-Id`** to its structured `request_id` on each `/v1/chat/completions` relay. This is the primary header for BiFrost correlation because clients such as Continue do not reliably send custom Chimera headers.

BiFrost subprocess rows may join a conversation only if BiFrost exposes `X-Request-Id` in its logs. When it does not, gateway in-process relay logs (`chat.bifrost.*`, `chat.routing.*`, `conversation.upstream.*`) remain canonical for the conversation card; BiFrost subprocess rows stay on the BiFrost service card.

---

## Compatibility (broker behind gateway)

| Area | Behavior |
|------|----------|
| **Chat** | `POST /v1/chat/completions`; streaming SSE pass-through |
| **Model list** | Gateway merges upstream catalog with operator virtual models |
| **Health** | `GET {base_url}/health` — gateway `/health` includes upstream probe |
| **Fallback** | **429** / selected **5xx** / admission blocks walk the virtual model fallback chain |

---

## Upstream documentation

- [BiFrost docs](https://docs.getbifrost.ai/)
- Delivery notes: [plans/upstream-llm-bifrost.md](../plans/upstream-llm-bifrost.md)
