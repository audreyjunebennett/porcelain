# Porcelain Project: Naming & Architecture

## Overview

**Porcelain** is the complete creative system shipped to the world. It consists of three core subsystems, each with its own identity and purpose.

## The Three Pillars

### 1. **Locus** — Creative Workspace Server
**Port:** 11435  
**Language:** Python (FastAPI)  
**Where:** `D:\Rebirth\pwa\` and `D:\Previously Claudia Core\`

The beating heart of the creative workspace. Locus runs on your iPhone via Tailscale, giving you a handcrafted, intimate interface for:
- Code editing and scripting
- Tweet composition and sharing
- Art and creative tools
- Music production
- Therapy journaling and reflection

Locus is **artisan**—thoughtful, warm, and built for flow. It knows the user intimately (via local identity) and responds to their creative rhythm.

### 2. **Chimera** — Intelligent Routing & Memory Layer
**Where:** `D:\Rebirth\claudia-gateway\` (Lynn's gateway refactor, separate sprint)

The connective tissue. Chimera routes requests, manages memory and RAG, and bridges the gap between the creative workspace and the AI backbone. It's the layer that remembers context, learns your habits, and routes your intent to the right tool.

### 3. **Porcelain** — The Unified System
**The Ship:** What users know by name. Porcelain is the full creative suite—seamless, unified, emotionally resonant.

## Design Philosophy

**Handcrafted, not mass-produced.** Porcelain rejects the hollow optimization of venture-backed platforms. Every surface is intentional. Every feature has weight.

**Artisan aesthetic.** Like fine ceramics, Porcelain is built with care. The interface is quiet, the interactions are deliberate, and the system serves the artist—not the ad network.

**Personal, not exploitative.** Locus runs locally or on your own infrastructure via Tailscale. Your memory lives in Chimera under your control. No tracking, no dark patterns, no "engagement metrics."

## Multi-User Support

Both Locus and Chimera support multi-user access via Tailscale:
- **Audrey** (Ruby) — Primary creative user
- **Lynn** — Memory/gateway administrator  
- **Raven** — (Future or secondary access)

Each user has their own identity context and memory thread.

## Implementation Notes

- Locus logs should read "Locus online" (not "Claudia Orchestrator started")
- File structure reflects the old "Claudia Core" naming—gradually migrate as refactoring occurs
- Chimera is handled in a separate refactor sprint (not this one)
- Keep the artisan philosophy in code comments and docs

---

**Last Updated:** 2026-05-06
