"""
Claudia PWA Server — Chat, Journal, Files, Notes
Requirements: pip install fastapi uvicorn httpx

Run:
  python D:/Rebirth/pwa/server.py

Access (default port 8081; set CLAUDIA_PWA_PORT to override):
  PC:     http://localhost:8081
  iPhone: http://<tailscale-ip>:8081
  Quick capture: http://<tailscale-ip>:8081/capture
"""
import json
import os
import socket
import sys
from datetime import datetime
from pathlib import Path

import httpx
import uvicorn
from fastapi import FastAPI, HTTPException, Request
from fastapi.responses import HTMLResponse, JSONResponse, StreamingResponse, FileResponse, Response
from fastapi.staticfiles import StaticFiles

# Load D:\Rebirth\.env if present (gitignored, holds API keys)
try:
    from dotenv import load_dotenv
    _env = Path(__file__).resolve().parent.parent / ".env"
    if _env.exists():
        load_dotenv(_env)
except ImportError:
    pass  # dotenv optional — system env vars also work

# ── Config ───────────────────────────────────────────────────────────────────
NOTES_DIR       = Path(os.environ.get("CLAUDIA_NOTES_DIR",       r"D:\Notes"))
TRANSCRIPTS_DIR = Path(os.environ.get("CLAUDIA_TRANSCRIPTS_DIR", r"D:\Rebirth\Moto X\claudia_motoxaudio_data\conversations"))
FILES_ROOT      = Path(os.environ.get("CLAUDIA_FILES_ROOT",      r"D:\\"))
ASSETS_DIR      = Path(os.environ.get("CLAUDIA_ASSETS_DIR",      r"D:\Rebirth\assets"))
PORT            = int(os.environ.get("CLAUDIA_PWA_PORT",          "8081"))
BEE_ICON_PATH   = ASSETS_DIR / "bee" / "bee-svgrepo-com.svg"
CANONICAL_ICON_PATH = ASSETS_DIR / "Canonical Icons" / "icon.svg"

# API keys — must come from .env or system env. Empty default = clear failure if not set.
GROQ_KEY    = os.environ.get("GROQ_API_KEY",   "")
GEMINI_KEY  = os.environ.get("GEMINI_API_KEY", "")
OLLAMA_URL  = os.environ.get("OLLAMA_BASE_URL", "http://localhost:11434")

GROQ_MODEL   = os.environ.get("GROQ_MODEL",   "llama-3.3-70b-versatile")
GEMINI_MODEL = os.environ.get("GEMINI_MODEL", "gemini-2.0-flash")
LOCAL_MODEL  = os.environ.get("CLAUDIA_LOCAL_MODEL", "qwen3:30b-a3b")

# Warn loudly at startup if any key is missing — prevents silent fallback to wrong provider
_missing_keys = [n for n, v in (("GROQ_API_KEY", GROQ_KEY), ("GEMINI_API_KEY", GEMINI_KEY)) if not v]
if _missing_keys:
    print(f"\n  [!] WARNING: missing API key(s) in env: {', '.join(_missing_keys)}", file=sys.stderr)
    print(f"  [!] Add them to D:\\Rebirth\\.env or set as system env vars.\n", file=sys.stderr)

# ── Gateway (claudia-gateway on port 3000) ────────────────────────────────────
GATEWAY_URL   = os.environ.get("CLAUDIA_GATEWAY_URL",   "http://localhost:3000")
GATEWAY_TOKEN = os.environ.get("CLAUDIA_GATEWAY_TOKEN", "claudia-loves-lynn")
# Fallback virtual model id if GET /status is unavailable (must match gateway semver / gateway.yaml)
GATEWAY_VIRTUAL_FALLBACK = os.environ.get("CLAUDIA_GATEWAY_VIRTUAL_MODEL", "Claudia-0.2.0")

SYSTEM_PROMPT = (
    "You are Claudia, Ruby's personal AI assistant. You're warm, smart, and a little cute. "
    "You know about Ruby's projects: Rebirth (her local AI system), the Moto X audio lifelog "
    "(voice journal that transcribes automatically), and her creative work. "
    "Help her stay organized, think through ideas, and get things done. "
    "Be direct but warm — like a brilliant friend, not a corporate assistant. "
    "When naming notes, be concise and descriptive."
)

NOTES_DIR.mkdir(parents=True, exist_ok=True)
THIS_DIR = Path(__file__).resolve().parent
CLAUDIA_WEB_HTML = THIS_DIR / "static" / "claudia_web.html"
WEB_APP_BUNDLE = THIS_DIR / "static" / "web_app.js"
LEGACY_TABBED_HTML = THIS_DIR / "static" / "legacy_tabbed_app.html"

app = FastAPI(title="Claudia")

# Mount /assets/* for the PWA's icon/asset library at D:\Rebirth\assets\
if ASSETS_DIR.exists():
    app.mount("/assets", StaticFiles(directory=str(ASSETS_DIR)), name="assets")


@app.get("/health")
async def health():
    """Simple app health check for local dev probes."""
    return {"ok": True, "app": "Claudia", "port": PORT}


def claudia_shell_html() -> str:
    """Original Claudia PWA shell + cache-bust for web_app.js."""
    html = CLAUDIA_WEB_HTML.read_text(encoding="utf-8")
    try:
        ver = str(int(WEB_APP_BUNDLE.stat().st_mtime))
    except OSError:
        ver = "0"
    return html.replace("__BUILD_ID__", ver)


# ── Helpers ───────────────────────────────────────────────────────────────────

async def gateway_virtual_model(client: httpx.AsyncClient) -> str | None:
    """Read virtual Claudia model id from gateway GET /status (required for RAG)."""
    try:
        r = await client.get(f"{GATEWAY_URL}/status", timeout=3)
        if r.status_code != 200:
            return None
        gw = (r.json() or {}).get("gateway") or {}
        vm = gw.get("virtual_model")
        return vm.strip() if isinstance(vm, str) and vm.strip() else None
    except Exception:
        return None


def safe_path(raw: str) -> Path:
    """Resolve path and ensure it's on D: drive."""
    p = Path(raw).resolve()
    if not str(p).upper().startswith("D:"):
        raise HTTPException(400, "Path must be on D: drive")
    return p


async def stream_openai_compat(
    base_url: str,
    api_key: str,
    model: str,
    messages: list,
    extra_headers: dict | None = None,
):
    """Stream from any OpenAI-compatible endpoint and yield SSE chunks."""
    headers = {
        "Content-Type": "application/json",
    }

    # Gemini API uses key as query param, not Authorization header
    if "generativelanguage.googleapis.com" in base_url:
        url = f"{base_url}/v1/chat/completions?key={api_key}"
    else:
        headers["Authorization"] = f"Bearer {api_key}"
        url = f"{base_url}/v1/chat/completions"

    if extra_headers:
        headers.update(extra_headers)
    body = {"model": model, "messages": messages, "stream": True}
    try:
        async with httpx.AsyncClient(timeout=120) as client:
            async with client.stream(
                "POST", url,
                headers=headers, json=body
            ) as resp:
                if resp.status_code != 200:
                    err = await resp.aread()
                    yield f"data: {json.dumps({'type':'error','text': err.decode()})}\n\n"
                    return
                async for line in resp.aiter_lines():
                    if not line.startswith("data: "):
                        continue
                    payload = line[6:]
                    if payload == "[DONE]":
                        yield 'data: {"type":"done"}\n\n'
                        return
                    try:
                        chunk = json.loads(payload)
                        text = chunk["choices"][0]["delta"].get("content", "")
                        if text:
                            yield f"data: {json.dumps({'type':'content','text':text})}\n\n"
                    except Exception:
                        pass
    except Exception as exc:
        yield f"data: {json.dumps({'type':'error','text':str(exc)})}\n\n"


# ── Static pages ──────────────────────────────────────────────────────────────

@app.get("/", response_class=HTMLResponse)
async def root():
    """Landing page — pwa/server.py serves file/notes/code APIs.
    The original PWA (with sign-in, conversations, sidebar) lives on the
    orchestrator at https://localhost:11435/web — its routes don't exist here.
    """
    return f"""<!DOCTYPE html>
<html><head><meta charset="UTF-8"><title>Claudia · routes</title>
<meta name="viewport" content="width=device-width, initial-scale=1">
<style>
body{{font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif;background:#F6F1EE;color:#4A3F4F;margin:0;padding:30px;line-height:1.6;max-width:640px;margin:0 auto;}}
h1{{color:#D89AA8;font-size:22px;margin:0 0 6px;}}
.sub{{color:#9B8FA0;font-size:13px;margin-bottom:24px;}}
.card{{background:#FDFAF6;border:1.5px solid #DBC9B5;border-radius:14px;padding:18px 20px;margin-bottom:14px;box-shadow:0 2px 6px rgba(74,63,79,.06);}}
.card h2{{font-size:14px;margin:0 0 4px;color:#D89AA8;}}
.card a{{display:inline-block;color:#84B8BA;font-family:"SF Mono",Consolas,monospace;font-size:14px;font-weight:600;text-decoration:none;}}
.card a:hover{{text-decoration:underline;}}
.note{{font-size:12px;color:#9B8FA0;margin-top:6px;}}
</style></head><body>
<h1>✨ Claudia · pwa server</h1>
<p class="sub">port {PORT} — file / notes / code APIs only (8080 reserved for Bifrost). See your routes:</p>

<div class="card">
  <h2>Original PWA (sidebar, conversations, sign-in)</h2>
  <a href="https://localhost:11435/web">https://localhost:11435/web</a>
  <p class="note">runs on the <b>orchestrator</b> (port 11435, HTTPS — accept self-signed cert once)</p>
</div>

<div class="card">
  <h2>✨ Code editor (new!)</h2>
  <a href="/code">/code</a>
  <p class="note">file tree + Monaco editor + chat about current file</p>
</div>

<div class="card">
  <h2>Legacy tabbed app (chat / journal / files / notes)</h2>
  <a href="/legacy-app">/legacy-app</a>
</div>

<div class="card">
  <h2>Quick capture</h2>
  <a href="/capture">/capture</a>
</div>
</body></html>"""


@app.get("/web", response_class=HTMLResponse)
async def claudia_web_page():
    """The original PWA shell — but its routes (/conversations, /api/auth/*, etc.)
    only exist on the orchestrator. Redirect to make this clear."""
    return """<!DOCTYPE html><html><head><meta http-equiv="refresh" content="0;url=https://localhost:11435/web">
<title>→ orchestrator</title></head>
<body style="font-family:sans-serif;padding:30px;background:#F6F1EE;color:#4A3F4F;">
<h2>↗ redirecting to the orchestrator</h2>
<p>The original PWA lives at <a href="https://localhost:11435/web">https://localhost:11435/web</a>.</p>
<p>Your browser may warn about a self-signed cert — click Advanced → Proceed once and it remembers.</p>
</body></html>"""


@app.get("/legacy-app", response_class=HTMLResponse)
async def legacy_tabbed_app():
    """FastAPI-only tabbed UI (journal / files / notes) — replacement experiment."""
    if not LEGACY_TABBED_HTML.exists():
        raise HTTPException(404, "legacy_tabbed_app.html missing")
    return LEGACY_TABBED_HTML.read_text(encoding="utf-8")


@app.get("/code", response_class=HTMLResponse)
async def code_page():
    """Mobile-first AI code editor — file tree + Monaco + chat with current file as context."""
    code_html = THIS_DIR / "static" / "code.html"
    if not code_html.exists():
        raise HTTPException(404, "code.html missing")
    return code_html.read_text(encoding="utf-8")


@app.get("/web_app.js")
async def web_app_js():
    if not WEB_APP_BUNDLE.exists():
        raise HTTPException(
            404,
            "web_app.js not built — run: python pwa/build_web_app.py",
        )
    return FileResponse(
        WEB_APP_BUNDLE,
        media_type="application/javascript",
        headers={"Cache-Control": "no-cache"},
    )


@app.get("/claudia.webmanifest")
async def claudia_manifest():
    return FileResponse(
        THIS_DIR / "static" / "claudia.webmanifest",
        media_type="application/manifest+json",
    )


def _icon_alias_response():
    return FileResponse(THIS_DIR / "static" / "icon.svg", media_type="image/svg+xml")


def _first_existing_path(paths: list[Path]) -> Path | None:
    for path in paths:
        if path.exists() and path.is_file():
            return path
    return None


def _asset_candidates(bucket: str, asset_path: str) -> list[Path]:
    normalized_bucket = (bucket or "").strip().lower()
    normalized_asset = asset_path.replace("/", os.sep).replace("\\", os.sep).strip(os.sep)
    alias_roots = {
        "bucket_tree_flowers": ASSETS_DIR / "bucket tree flowers",
        "horns": ASSETS_DIR / "horns",
    }
    roots = [alias_roots.get(normalized_bucket, ASSETS_DIR / bucket)]
    if normalized_bucket == "horns":
        roots.append(ASSETS_DIR / "ex out crossed out")
    return [root / normalized_asset for root in roots if root]


def _guess_media_type(path: Path) -> str:
    ext = path.suffix.lower()
    if ext == ".svg":
        return "image/svg+xml"
    if ext == ".png":
        return "image/png"
    if ext in {".jpg", ".jpeg"}:
        return "image/jpeg"
    if ext == ".webp":
        return "image/webp"
    return "application/octet-stream"


@app.get("/bee.svg")
async def bee_svg():
    """Send button art in original UI."""
    if BEE_ICON_PATH.exists():
        raw = BEE_ICON_PATH.read_text(encoding="utf-8")
        raw = raw.replace('stroke="#000000"', 'stroke="#ff88ee"').replace('stroke-opacity="0.9"', 'stroke-opacity="0.98"')
        return Response(content=raw, media_type="image/svg+xml")
    return _icon_alias_response()


@app.get("/header_icon.svg")
async def header_icon():
    icon_path = _first_existing_path([CANONICAL_ICON_PATH, THIS_DIR / "static" / "icon.svg"])
    if not icon_path:
        raise HTTPException(404, "header icon missing")
    return FileResponse(icon_path, media_type="image/svg+xml")


@app.get("/chat_icon.png")
async def chat_icon():
    icon_path = _first_existing_path(
        [
            THIS_DIR / "static" / "claudia-avatar-sm.png",
            THIS_DIR / "static" / "claudia-avatar.png",
        ]
    )
    if not icon_path:
        raise HTTPException(404, "chat icon missing")
    return FileResponse(icon_path, media_type="image/png")


@app.get("/api/asset/{bucket}/{asset_path:path}")
async def asset_file(bucket: str, asset_path: str):
    asset = _first_existing_path(_asset_candidates(bucket, asset_path))
    if not asset:
        raise HTTPException(404, "asset missing")
    return FileResponse(asset, media_type=_guess_media_type(asset))


@app.get("/claudia_avatar.svg")
async def claudia_avatar_svg():
    """Typing / header avatar fallback."""
    return _icon_alias_response()


@app.get("/capture", response_class=HTMLResponse)
async def capture_page():
    return (THIS_DIR / "static" / "capture.html").read_text(encoding="utf-8")

@app.get("/icon.svg")
async def icon():
    return FileResponse(THIS_DIR / "static" / "icon.svg", media_type="image/svg+xml")

@app.get("/icon-capture.svg")
async def icon_capture():
    return FileResponse(THIS_DIR / "static" / "icon-capture.svg", media_type="image/svg+xml")


@app.get("/manifest.json")
async def manifest():
    return FileResponse(THIS_DIR / "static" / "manifest.json", media_type="application/manifest+json")


@app.get("/sw.js")
async def service_worker():
    return FileResponse(
        THIS_DIR / "static" / "sw.js",
        media_type="application/javascript",
        headers={"Cache-Control": "no-cache"},
    )

@app.get("/claudia-avatar.png")
async def claudia_avatar():
    return FileResponse(THIS_DIR / "static" / "claudia-avatar.png", media_type="image/png")

@app.get("/claudia-avatar-sm.png")
async def claudia_avatar_sm():
    return FileResponse(THIS_DIR / "static" / "claudia-avatar-sm.png", media_type="image/png")


# ── Models ────────────────────────────────────────────────────────────────────

@app.get("/api/models")
async def list_models():
    models = []
    # Local Ollama models
    try:
        async with httpx.AsyncClient(timeout=3) as client:
            r = await client.get(f"{OLLAMA_URL}/api/tags")
            if r.status_code == 200:
                for m in r.json().get("models", []):
                    models.append({
                        "id": m["name"],
                        "provider": "ollama",
                        "label": f"Local · {m['name']}",
                    })
    except Exception:
        pass
    # Gateway — virtual model (Claudia-x.y.z) triggers RAG + routing; raw upstream ids skip RAG
    try:
        async with httpx.AsyncClient(timeout=3) as client:
            r = await client.get(f"{GATEWAY_URL}/health")
            if r.status_code == 200:
                vm = await gateway_virtual_model(client) or GATEWAY_VIRTUAL_FALLBACK
                models.insert(0, {
                    "id": vm,
                    "provider": "gateway",
                    "label": "✦ Gateway (RAG + auto-route)",
                })
    except Exception:
        pass  # gateway not running, just skip it
    models.append({"id": GEMINI_MODEL, "provider": "gemini", "label": f"Gemini · {GEMINI_MODEL}"})
    models.append({"id": GROQ_MODEL,   "provider": "groq",   "label": f"Groq · {GROQ_MODEL}"})
    return models


@app.get("/v1/models")
async def openai_models():
    """Small OpenAI-compatible models listing for local tool probes."""
    models = await list_models()
    return {
        "object": "list",
        "data": [
            {
                "id": model["id"],
                "object": "model",
                "created": 0,
                "owned_by": model["provider"],
            }
            for model in models
        ],
    }


@app.post("/v1/embeddings")
async def openai_embeddings(request: Request):
    """Return a harmless placeholder embedding for local compatibility probes."""
    body = await request.json()
    inputs = body.get("input", "")
    model = body.get("model", "claudia-local")

    if isinstance(inputs, list):
        data = [
            {
                "object": "embedding",
                "index": idx,
                "embedding": [0.0],
            }
            for idx, _ in enumerate(inputs)
        ]
        prompt_tokens = sum(len(str(item)) for item in inputs)
    else:
        data = [{"object": "embedding", "index": 0, "embedding": [0.0]}]
        prompt_tokens = len(str(inputs))

    return {
        "object": "list",
        "data": data,
        "model": model,
        "usage": {
            "prompt_tokens": prompt_tokens,
            "total_tokens": prompt_tokens,
        },
    }


# ── Chat ──────────────────────────────────────────────────────────────────────

@app.post("/api/chat")
async def chat(request: Request):
    body = await request.json()
    messages  = body.get("messages", [])
    model     = body.get("model")
    project   = body.get("project", "").strip()   # optional RAG project (transcripts / notes)

    if not messages or messages[0].get("role") != "system":
        messages = [{"role": "system", "content": SYSTEM_PROMPT}] + messages

    # Gateway-only routing — no fallbacks to other providers
    if not model:
        try:
            async with httpx.AsyncClient(timeout=3) as client:
                gw_status = await client.get(f"{GATEWAY_URL}/status")
                if gw_status.status_code == 200:
                    gw = (gw_status.json() or {}).get("gateway") or {}
                    model = gw.get("virtual_model") or GATEWAY_VIRTUAL_FALLBACK
        except Exception:
            pass
    model = model or GATEWAY_VIRTUAL_FALLBACK

    gw_headers = {"X-Claudia-Project": project} if project else None
    gen = stream_openai_compat(GATEWAY_URL, GATEWAY_TOKEN, model, messages, gw_headers)

    return StreamingResponse(
        gen,
        media_type="text/event-stream",
        headers={"Cache-Control": "no-cache", "X-Accel-Buffering": "no"},
    )


@app.get("/api/gateway/status")
async def gateway_status():
    """Check gateway health and expose virtual model id for RAG-aware chat."""
    out: dict = {"online": False, "url": GATEWAY_URL, "virtual_model": None, "semver": None}
    try:
        async with httpx.AsyncClient(timeout=3) as client:
            h = await client.get(f"{GATEWAY_URL}/health")
            out["online"] = h.status_code == 200
            st = await client.get(f"{GATEWAY_URL}/status")
            if st.status_code == 200:
                data = st.json() or {}
                gw = data.get("gateway") or {}
                out["virtual_model"] = gw.get("virtual_model")
                out["semver"] = gw.get("semver")
            if not out.get("virtual_model"):
                out["virtual_model"] = GATEWAY_VIRTUAL_FALLBACK
    except Exception:
        pass
    return out


# ── Transcripts ───────────────────────────────────────────────────────────────

@app.get("/api/transcripts")
async def list_transcripts():
    if not TRANSCRIPTS_DIR.exists():
        return []
    convos = []
    for f in sorted(TRANSCRIPTS_DIR.glob("conversation_*.md"), reverse=True):
        try:
            # Parse a readable date from filename: conversation_2026-05-01_22-03-31.md
            parts = f.stem.replace("conversation_", "").split("_")
            date_str = parts[0] if parts else ""
            time_str = parts[1].replace("-", ":") if len(parts) > 1 else ""
            label = f"{date_str} {time_str}".strip()
        except Exception:
            label = f.stem
        convos.append({
            "id": f.name,
            "label": label,
            "modified": f.stat().st_mtime,
        })
    return convos

@app.get("/api/transcripts/{filename}")
async def read_transcript(filename: str):
    path = TRANSCRIPTS_DIR / filename
    if not path.exists() or path.suffix != ".md":
        raise HTTPException(404)
    return {"content": path.read_text(encoding="utf-8")}


# ── Files ─────────────────────────────────────────────────────────────────────

TEXT_EXTS = {
    ".md", ".txt", ".py", ".js", ".ts", ".jsx", ".tsx",
    ".json", ".yaml", ".yml", ".toml", ".csv", ".html",
    ".css", ".sh", ".bat", ".ps1", ".env", ".gitignore",
    ".continueignore", ".stignore", ".xml", ".svg", ".ini",
    ".cfg", ".conf", ".log", ".dockerfile",
}

# Dotfiles like .gitignore have empty Path.suffix, so check whole name too.
def is_text_path(p) -> bool:
    suf = p.suffix.lower()
    if suf and suf in TEXT_EXTS:
        return True
    name = p.name.lower()
    if name in TEXT_EXTS:  # e.g. ".gitignore", ".env"
        return True
    if name in {"dockerfile", "makefile", "readme", "license"}:
        return True
    return False

@app.get("/api/files")
async def list_files(path: str = "D:\\"):
    p = safe_path(path)
    if not p.exists():
        raise HTTPException(404, "Path not found")
    if p.is_file():
        raise HTTPException(400, "Use /api/files/read for files")
    entries = []
    try:
        items = sorted(p.iterdir(), key=lambda x: (x.is_file(), x.name.lower()))
        for item in items:
            try:
                entries.append({
                    "name": item.name,
                    "path": str(item),
                    "is_dir": item.is_dir(),
                    "size": item.stat().st_size if item.is_file() else None,
                    "ext": item.suffix.lower() if item.is_file() else None,
                    "readable": is_text_path(item) if item.is_file() else False,
                })
            except PermissionError:
                pass
    except PermissionError:
        raise HTTPException(403, "Permission denied")
    return entries

@app.get("/api/files/read")
async def read_file(path: str):
    p = safe_path(path)
    if not p.exists() or not p.is_file():
        raise HTTPException(404)
    if not is_text_path(p):
        raise HTTPException(400, f"Cannot read binary file ({p.suffix})")
    try:
        return {"content": p.read_text(encoding="utf-8", errors="replace"), "path": str(p)}
    except PermissionError:
        raise HTTPException(403, "Permission denied")


@app.post("/api/files/write")
async def write_file(request: Request):
    """Write content to a text file. Body: {path, content}. Used by the /code editor."""
    body = await request.json()
    path = body.get("path", "").strip()
    content = body.get("content")
    if not path or content is None:
        raise HTTPException(400, "path and content required")
    p = safe_path(path)
    if not is_text_path(p):
        raise HTTPException(400, f"Refusing to write non-text file ({p.suffix})")
    try:
        p.parent.mkdir(parents=True, exist_ok=True)
        p.write_text(content, encoding="utf-8")
        return {"ok": True, "path": str(p), "bytes": len(content.encode("utf-8"))}
    except PermissionError:
        raise HTTPException(403, "Permission denied")


# ── Notes ─────────────────────────────────────────────────────────────────────

@app.get("/api/notes")
async def list_notes():
    notes = []
    for f in sorted(NOTES_DIR.glob("*.md"), key=lambda x: x.stat().st_mtime, reverse=True):
        notes.append({
            "filename": f.name,
            "modified": f.stat().st_mtime,
            "size": f.stat().st_size,
        })
    return notes

@app.get("/api/notes/{filename}")
async def read_note(filename: str):
    p = NOTES_DIR / filename
    if not p.exists():
        raise HTTPException(404)
    return {"content": p.read_text(encoding="utf-8"), "filename": filename}

@app.post("/api/notes")
async def create_note(request: Request):
    body = await request.json()
    content  = body.get("content", "").strip()
    filename = body.get("filename", "").strip()

    if not content:
        raise HTTPException(400, "Content required")

    if not filename:
        # Ask Groq to name it (fast)
        try:
            async with httpx.AsyncClient(timeout=15) as client:
                resp = await client.post(
                    "https://api.groq.com/openai/v1/chat/completions",
                    headers={"Authorization": f"Bearer {GROQ_KEY}"},
                    json={
                        "model": "llama-3.3-70b-versatile",
                        "messages": [
                            {"role": "system", "content":
                                "Generate a short filename for this note. "
                                "Respond with ONLY the filename — no extension, no explanation. "
                                "Use kebab-case. Max 5 words."},
                            {"role": "user", "content": f"Name this note:\n\n{content[:500]}"},
                        ],
                        "max_tokens": 20,
                    },
                )
                raw = resp.json()["choices"][0]["message"]["content"].strip()
                filename = "".join(
                    c if c.isalnum() or c in "-_ " else "-" for c in raw
                ).replace(" ", "-").strip("-")[:60]
        except Exception:
            filename = datetime.now().strftime("note-%Y-%m-%d-%H-%M")

    if not filename.endswith(".md"):
        filename += ".md"

    full = f"# {filename[:-3].replace('-', ' ').title()}\n\n*{datetime.now().strftime('%Y-%m-%d %H:%M')}*\n\n{content}"
    (NOTES_DIR / filename).write_text(full, encoding="utf-8")
    return {"filename": filename, "path": str(NOTES_DIR / filename)}

@app.put("/api/notes/{filename}")
async def update_note(filename: str, request: Request):
    body = await request.json()
    p = NOTES_DIR / filename
    p.write_text(body.get("content", ""), encoding="utf-8")
    return {"filename": filename}

@app.delete("/api/notes/{filename}")
async def delete_note(filename: str):
    p = NOTES_DIR / filename
    if not p.exists():
        raise HTTPException(404)
    p.unlink()
    return {"ok": True}


# ── Run ───────────────────────────────────────────────────────────────────────

def get_ips():
    """Return (tailscale_ip, local_ip) — either may be None."""
    tailscale = None
    local = None
    try:
        for info in socket.getaddrinfo(socket.gethostname(), None, socket.AF_INET):
            ip = info[4][0]
            if ip.startswith("127."):
                continue
            if ip.startswith("100."):
                tailscale = ip          # Tailscale is always 100.x.x.x
            elif local is None:
                local = ip              # first non-loopback = LAN
    except Exception:
        pass
    return tailscale, local


if __name__ == "__main__":
    # Force UTF-8 stdout so ANSI box drawing and emoji print on Windows cp1252 consoles
    if sys.platform == "win32":
        try:
            sys.stdout.reconfigure(encoding="utf-8")
            sys.stderr.reconfigure(encoding="utf-8")
        except Exception:
            pass

    tailscale_ip, local_ip = get_ips()

    W  = "\033[0m"    # reset
    PK = "\033[95m"   # pink-ish (bright magenta)
    TE = "\033[96m"   # teal (bright cyan)
    GR = "\033[92m"   # green
    YL = "\033[93m"   # yellow
    DM = "\033[2m"    # dim

    print()
    print(f"{PK}  ✦ Claudia is running!{W}")
    print()

    # ── PC / Chrome testing
    print(f"{DM}  ┌─ On this PC (Chrome / bug-testing){W}")
    print(f"  │  {GR}➜  http://localhost:{PORT}{W}          {DM}← original Claudia PWA (web_app_parts){W}")
    print(f"  │  {GR}➜  http://localhost:{PORT}/legacy-app{W}  {DM}← tabbed UI (notes / journal / files){W}")
    print(f"  │  {GR}➜  http://localhost:{PORT}/capture{W}     {DM}← quick notes{W}")
    print()

    # ── iPhone via Tailscale
    if tailscale_ip:
        print(f"{DM}  ┌─ On your iPhone (Tailscale){W}")
        print(f"  │  {TE}➜  http://{tailscale_ip}:{PORT}{W}          {DM}← add to homescreen{W}")
        print(f"  │  {TE}➜  http://{tailscale_ip}:{PORT}/capture{W}  {DM}← capture bookmark{W}")
    else:
        print(f"{DM}  ┌─ On your iPhone (Tailscale){W}")
        print(f"  │  {YL}⚠  Tailscale IP not found — is Tailscale connected?{W}")
        if local_ip:
            print(f"  │  {YL}   (LAN fallback: http://{local_ip}:{PORT}){W}")
    print()

    # ── Local LAN fallback
    if local_ip:
        print(f"{DM}  ┌─ Same WiFi (no Tailscale needed){W}")
        print(f"  │  {DM}➜  http://{local_ip}:{PORT}{W}")
        print()

    # ── Paths
    print(f"{DM}  Notes  →  {NOTES_DIR}{W}")
    print(f"{DM}  Journal→  {TRANSCRIPTS_DIR}{W}")
    print()

    uvicorn.run(app, host="0.0.0.0", port=PORT)
