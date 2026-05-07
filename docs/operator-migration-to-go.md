# Operator note — Go-only repository

This repository ships **only** the **Go** `claudia` binary. The former **TypeScript** gateway, **Docker Compose** stack, and **LiteLLM** service definitions have been removed in favor of **local** `claudia` / `claudia serve` with **BiFrost**.

Configuration is unchanged for operators already on `config/gateway.yaml`, `tokens.yaml`, and `routing-policy.yaml` (optional `provider-free-tier.yaml` when using the free-tier filter) — see [configuration.md](configuration.md) and [installation.md](installation.md).
