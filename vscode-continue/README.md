# VS Code Continue — Claudia Gateway (v0.2.x)

Use **Continue** with an **OpenAI-compatible** provider pointed at **Claudia Gateway**, not directly at LiteLLM.

## Values you need

1. `apiBase` — Gateway URL including the OpenAI API prefix, e.g. `http://localhost:3000/v1` (adjust host/port if you publish differently).
2. `apiKey` — A **gateway token** from `config/tokens.yaml` (same string as `Authorization: Bearer …`).
3. `model` — The virtual id from `GET /v1/models`, e.g. `Claudia-0.2.0` (must match `gateway.semver` in `config/gateway.yaml`). The gateway also serves a **Continue** helper at `/ui/continue` (after login) to validate files and write snippets.

Continue reference: [Continue configuration](https://docs.continue.dev/reference).

## Custom headers (v0.2+ RAG)

**v0.1** does not require project or flavor headers for chat. For **v0.2+**, plan on sending:

- `X-Claudia-Project` — project slug for collection routing.
- `X-Claudia-Flavor-Id` — optional corpus key within the project.

Exact YAML keys depend on your Continue version (`requestOptions`, `defaultRequestOptions`, etc.). See `config.yml` in this folder for copy-paste snippets.

## Workspace layout

Copy the relevant blocks into your workspace `.continue/config.yaml` (some Continue versions expect `config.yaml` rather than `config.yml`).
