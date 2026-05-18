"""
Locus: creative workspace API (part of Porcelain). Runs on port 11435.
Start from project root: python locus_api.py

All AI chat goes through Chimera (gateway on port 3000) → BiFrost (port 8080) → local/cloud model.
Set LOCUS_GATEWAY_URL, LOCUS_GATEWAY_TOKEN, LOCUS_GATEWAY_VIRTUAL_MODEL in .env to configure.

Key endpoints:
  GET  /                         -> service info
  GET  /web                      -> Locus PWA (the main UI)
  POST /conversations/{id}/messages -> send a message, get an AI reply
  GET  /conversations            -> list all saved conversations
  POST /v1/chat/completions      -> OpenAI-compatible chat API
  POST /api/chat                 -> Ollama-compatible chat API
  GET  /continue/conversations   -> VS Code Continue chat history (read-only)
  GET  /grok/conversations       -> Grok export history (read-only)
  GET  /cursor/conversations     -> Cursor chat history (read-only)
  POST /plan/append              -> append to project plan
"""
import asyncio
import os
import sys
from pathlib import Path

PROJECT_ROOT = Path(__file__).resolve().parent
# ASSETS_ROOT is the repo root so that ASSETS_ROOT / "assets" / <name> resolves correctly.
ASSETS_ROOT = PROJECT_ROOT
if str(PROJECT_ROOT) not in sys.path:
    sys.path.insert(0, str(PROJECT_ROOT))
# Add scripts directory (might be called 'scripts' or 'Scripts')
scripts_dir = PROJECT_ROOT / "scripts"
if scripts_dir.exists():
    sys.path.insert(0, str(scripts_dir))
else:
    scripts_dir = PROJECT_ROOT / "Scripts"
    if scripts_dir.exists():
        sys.path.insert(0, str(scripts_dir))

# Optional: load .env from project root (e.g. LOCUS_ACCESS_TOKEN) so you don't commit secrets
_env_file = PROJECT_ROOT / ".env"
if _env_file.exists():
    try:
        from dotenv import load_dotenv
        load_dotenv(_env_file)
    except ImportError:
        pass  # python-dotenv not installed; use system env or batch file instead

GATEWAY_URL = os.environ.get("LOCUS_GATEWAY_URL", "http://localhost:3000").rstrip("/")
GATEWAY_TOKEN = os.environ.get("LOCUS_GATEWAY_TOKEN", "porcelain-loves-lynn")
GATEWAY_VIRTUAL_MODEL = os.environ.get("LOCUS_GATEWAY_VIRTUAL_MODEL", "ollama/chat")

# Optional: ensure UTF-8 on Windows for responses
if sys.platform == "win32":
    try:
        sys.stdout.reconfigure(encoding="utf-8")
    except Exception:
        pass

# Configure logging to file (silent startup mode)
import logging
LOG_DIR = PROJECT_ROOT / ".data" / "logs"
LOG_DIR.mkdir(parents=True, exist_ok=True)

logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s - %(name)s - %(levelname)s - %(message)s',
    handlers=[
        logging.FileHandler(LOG_DIR / "locus.log", encoding="utf-8"),
    ]
)

# Silence uvicorn access logs
logging.getLogger("uvicorn.access").setLevel(logging.WARNING)
logging.getLogger("uvicorn.error").setLevel(logging.WARNING)

import base64
import json
import random
import re
import tempfile
import threading
import time
import unicodedata
import uuid
from datetime import datetime, timezone
from pathlib import Path

from fastapi import Depends, FastAPI, Header, HTTPException, Request
from fastapi.responses import FileResponse, HTMLResponse, JSONResponse, RedirectResponse, Response
from pydantic import BaseModel

# Single-person mode: no auth
DEFAULT_USER = ""


def get_current_user(
    x_user: str | None = Header(None, alias="X-User"),
) -> str:
    """Single-user mode: always returns DEFAULT_USER (empty string). X-User header accepted for future multi-user support."""
    return (x_user or "").strip() or DEFAULT_USER


def get_user_display_name(user_id: str) -> str:
    return user_id or "User"

# Import after path setup so search_index / paths_config resolve
from export_conversation_log import append_exchange_to_log
from paths_config import GROK_CONVERSATIONS
from plan_tools import add_to_project_plan_recent as _add_to_project_plan_recent, add_to_backlog as _add_to_backlog

# Conversation store for /web tabs and persistent history
STORE_DIR = PROJECT_ROOT / ".data"
STORE_FILE = STORE_DIR / "mobile_conversations.json"
ENGAGEMENT_FILE = STORE_DIR / "engagement.json"
_engagement_lock = threading.Lock()  # serialize read/write on Windows to avoid PermissionError on replace
RETURN_THRESHOLD = 4  # suggest "Mark as important?" after this many opens (less sensitive; was 2)

# Phone photos: image + description saved here so we can re-run vision later or update with a better model
PHONE_PHOTOS_DIR = PROJECT_ROOT / "Journal_Database" / "Phone_Photos"

# Discord bot — always off, everyone uses the PWA
DISCORD_OFF = True
JOURNALS_DIR = PROJECT_ROOT / "Journal_Database" / "Journals"
QUICK_FACTS_PATH = STORE_DIR / "locus_quick_facts.md"
REMINDERS_PATH = STORE_DIR / "reminders.json"
QUICK_CAPTURE_PATH = PROJECT_ROOT / "Journal_Database" / "Quick_Capture.md"
EGGPLANT_DIR = ASSETS_ROOT / "assets" / "eggplant"
EGGPLANT_COLLECTION_FILE = STORE_DIR / "eggplant_collection.json"
BOMB_DIR = ASSETS_ROOT / "assets" / "bomb"
BUCKET_TREE_FLOWERS_DIR = ASSETS_ROOT / "assets" / "bucket tree flowers"  # branch/flower SVGs for activity breakdown
ASSETS_FILE_DIR = ASSETS_ROOT / "assets" / "file"  # sidebar 4-way actions icon
ASSETS_PENCIL_DIR = ASSETS_ROOT / "assets" / "pencil"  # edit (send again) icon
ASSETS_HORNS_DIR = ASSETS_ROOT / "assets" / "ex out crossed out"  # cancel (stay loose) icon
BAIT_DELAY_SECONDS = 55  # seconds before a saved bomb "attracts" a visitor (fishing feel)
DASHBOARD_PORT = int(os.environ.get("DASHBOARD_PORT", "11437"))  # standalone dashboard server (dashboard_server.py)

# Per-user avatar: default characters (DiceBear 9.x, no key; cute girly styles) + optional custom URL
AVATAR_STORE_FILE = STORE_DIR / "user_avatars.json"
# Per-user pronouns and optional "about me" for Lynn/Raven (injected into system prompt when they chat)
USER_PROFILES_FILE = STORE_DIR / "user_profiles.json"
DICEBEAR_BASE = "https://api.dicebear.com/9.x"
# Canonical Ruby & Hahli (user avatar) — first so it's the default when nothing is set
RUBY_HAHLI_CHARACTER = {"id": "ruby_hahli", "name": "Ruby & Hahli", "avatarUrl": "/user_avatar.svg"}
DEFAULT_CHARACTERS = [
    {"id": "blossom", "name": "Blossom", "avatarUrl": f"{DICEBEAR_BASE}/lorelei/svg?seed=blossom"},
    {"id": "luna", "name": "Luna", "avatarUrl": f"{DICEBEAR_BASE}/lorelei/svg?seed=luna"},
    {"id": "stella", "name": "Stella", "avatarUrl": f"{DICEBEAR_BASE}/lorelei/svg?seed=stella"},
    {"id": "peach", "name": "Peach", "avatarUrl": f"{DICEBEAR_BASE}/big-smile/svg?seed=peach"},
    {"id": "mochi", "name": "Mochi", "avatarUrl": f"{DICEBEAR_BASE}/big-smile/svg?seed=mochi"},
    {"id": "honey", "name": "Honey", "avatarUrl": f"{DICEBEAR_BASE}/adventurer/svg?seed=honey"},
    {"id": "bubble", "name": "Bubble", "avatarUrl": f"{DICEBEAR_BASE}/notionists/svg?seed=bubble"},
]


def _load_store():
    if not STORE_FILE.exists():
        return {"conversations": []}
    try:
        with open(STORE_FILE, encoding="utf-8") as f:
            return json.load(f)
    except (json.JSONDecodeError, OSError):
        return {"conversations": []}


def _save_store(data):
    STORE_DIR.mkdir(parents=True, exist_ok=True)
    with open(STORE_FILE, "w", encoding="utf-8") as f:
        json.dump(data, f, indent=2, ensure_ascii=False)
        f.flush()
        try:
            os.fsync(f.fileno())
        except OSError:
            pass


def _load_engagement():
    with _engagement_lock:
        if not ENGAGEMENT_FILE.exists():
            return {"by_key": {}}
        try:
            with open(ENGAGEMENT_FILE, encoding="utf-8") as f:
                return json.load(f)
        except (json.JSONDecodeError, OSError):
            return {"by_key": {}}


def _save_engagement(data):
    STORE_DIR.mkdir(parents=True, exist_ok=True)
    tmp = ENGAGEMENT_FILE.with_suffix(".tmp")
    with _engagement_lock:
        try:
            with open(tmp, "w", encoding="utf-8") as f:
                json.dump(data, f, indent=2, ensure_ascii=False)
                f.flush()
                try:
                    os.fsync(f.fileno())
                except OSError:
                    pass
            # On Windows, replace() can raise PermissionError if target is still open; retry then fallback
            for attempt in range(3):
                try:
                    tmp.replace(ENGAGEMENT_FILE)
                    return
                except PermissionError:
                    if attempt < 2:
                        time.sleep(0.05 * (attempt + 1))
                    else:
                        # Fallback: write directly so we don't 500 (less atomic)
                        try:
                            with open(ENGAGEMENT_FILE, "w", encoding="utf-8") as f:
                                json.dump(data, f, indent=2, ensure_ascii=False)
                                f.flush()
                        finally:
                            if tmp.exists():
                                try:
                                    tmp.unlink()
                                except OSError:
                                    pass
        except Exception:
            if tmp.exists():
                try:
                    tmp.unlink()
                except OSError:
                    pass
            raise


def _engagement_key(source: str, conv_id: str) -> str:
    return f"{source}:{conv_id}"


def _conversations_for_user(data: dict, user_id: str) -> list:
    """Return conversation list for this user (owner field; legacy convos without owner = ruby)."""
    convos = data.get("conversations", [])
    return [c for c in convos if (c.get("owner") or DEFAULT_USER) == user_id]


def _archive_for_user(data: dict, user_id: str) -> list:
    """Return archive list for this user."""
    archive = data.get("archive", [])
    return [c for c in archive if (c.get("owner") or DEFAULT_USER) == user_id]


# Clear 404 body for bug testing: mobile conversation missing or wrong account
MOBILE_CONVERSATION_404 = (
    "Conversation not found or not a mobile conversation. "
    "Check that the id is correct and the conversation belongs to your account."
)


def _get_conversation_by_id(data: dict, conv_id: str, user_id: str):
    """Get one conversation by id if it belongs to user (active list or archive)."""
    conv_id_str = str(conv_id).strip() if conv_id else ""
    if not conv_id_str:
        return None
    for c in data.get("conversations", []):
        if str(c.get("id") or "").strip() == conv_id_str and (c.get("owner") or DEFAULT_USER) == user_id:
            return c
    for c in data.get("archive", []):
        if str(c.get("id") or "").strip() == conv_id_str and (c.get("owner") or DEFAULT_USER) == user_id:
            return c
    return None


def _load_avatar_store() -> dict:
    """Per-user avatar: characterId and optional customUrl."""
    if not AVATAR_STORE_FILE.exists():
        return {}
    try:
        with open(AVATAR_STORE_FILE, encoding="utf-8") as f:
            return json.load(f)
    except (json.JSONDecodeError, OSError):
        return {}


def _save_avatar_store(data: dict) -> None:
    STORE_DIR.mkdir(parents=True, exist_ok=True)
    with open(AVATAR_STORE_FILE, "w", encoding="utf-8") as f:
        json.dump(data, f, indent=2, ensure_ascii=False)


def _load_user_profiles() -> dict:
    """Per-user pronouns and optional about_me (for prompt injection when Lynn/Raven chat)."""
    if not USER_PROFILES_FILE.exists():
        return {}
    try:
        with open(USER_PROFILES_FILE, encoding="utf-8") as f:
            return json.load(f)
    except (json.JSONDecodeError, OSError):
        return {}


def _save_user_profiles(data: dict) -> None:
    STORE_DIR.mkdir(parents=True, exist_ok=True)
    with open(USER_PROFILES_FILE, "w", encoding="utf-8") as f:
        json.dump(data, f, indent=2, ensure_ascii=False)


def _get_user_profile(user_id: str) -> dict:
    """Return {pronouns: str, about_me: str} for user (default empty strings)."""
    data = _load_user_profiles()
    entry = data.get(user_id) or {}
    return {
        "pronouns": (entry.get("pronouns") or "").strip(),
        "about_me": (entry.get("about_me") or "").strip(),
    }


def _sparkline_from_messages(messages: list, num_buckets: int = 10, max_messages: int = 100) -> list[int]:
    """Bucket last N messages by position into num_buckets counts; normalize to 0—10 for bar height."""
    if not messages:
        return [0] * num_buckets
    recent = messages[-max_messages:]
    n = len(recent)
    buckets = [0] * num_buckets
    for i, _ in enumerate(recent):
        idx = min(int((i / n) * num_buckets), num_buckets - 1) if n else 0
        buckets[idx] += 1
    mx = max(buckets) or 1
    return [int(round((b / mx) * 10)) for b in buckets]


def _activity_buckets(messages: list) -> dict:
    """Count messages, files, code snippets, media from conversation messages for Activity Breakdown."""
    messages_list = messages or []
    n_messages = len(messages_list)
    n_files = 0
    n_code = 0
    n_media = 0
    for m in messages_list:
        content = (m.get("content") or "").lower()
        if "[file:" in content or "[attached:" in content or ".pdf" in content or "attach" in content:
            n_files += 1
        if "```" in content:
            n_code += max(0, content.count("```") // 2)
        if "image" in content or ".png" in content or ".jpg" in content or "base64" in content:
            n_media += 1
    return {"messages": n_messages, "files": n_files, "code_snippets": n_code, "media": n_media}


def _get_branches(c: dict) -> list[list[dict]]:
    """Return list of message lists (branches). Legacy convos have no 'branches' -> single branch from 'messages'."""
    if "branches" in c and isinstance(c["branches"], list) and c["branches"]:
        return c["branches"]
    return [c.get("messages") or []]


def _branch_point(branches: list[list[dict]]) -> int:
    """Index of first message where branches diverge (length of common prefix). Used for inline thread switcher."""
    if not branches or len(branches) < 2:
        return 0
    min_len = min(len(b) for b in branches)
    for j in range(min_len):
        first = branches[0][j]
        role0, content0 = first.get("role"), first.get("content")
        for b in branches[1:]:
            m = b[j]
            if m.get("role") != role0 or m.get("content") != content0:
                return j
    return min_len


def _conversation_summary(c, feedback_liked: int | None = None, feedback_noted: int | None = None):
    """Use cached sparkline_data/message_count when present (set on send_message) so list_conversations stays fast.
    feedback_liked/feedback_noted: thumbs up/down counts for activity readouts; when provided (e.g. from list) use them."""
    branches = _get_branches(c)
    messages = branches[0] if branches else []
    if "message_count" in c and "sparkline_data" in c:
        message_count = c["message_count"]
        sparkline_data = c["sparkline_data"]
    else:
        message_count = len(messages)
        sparkline_data = _sparkline_from_messages(messages) if messages else []
    liked = feedback_liked if feedback_liked is not None else c.get("feedback_liked", 0)
    noted = feedback_noted if feedback_noted is not None else c.get("feedback_noted", 0)
    return {
        "id": c["id"],
        "title": c.get("title") or "Chat",
        "created_at": c.get("created_at", ""),
        "updated_at": c.get("updated_at", ""),
        "pinned": bool(c.get("pinned", False)),
        "important": bool(c.get("important", False)),
        "message_count": message_count,
        "sparkline_data": sparkline_data,
        "feedback_liked": liked,
        "feedback_noted": noted,
    }


def _run_mobile_tools(message: str) -> str | None:
    """Run tools when the user asks for calendar, Gmail, journal, weather, web search, URL, etc.
    Multiple tools can run per message (e.g. calendar + journal); results are concatenated.
    Returns None if nothing to run."""
    msg = (message or "").strip().lower()
    if not msg:
        return None
    results: list[str] = []

    # --- Calendar (same as Continue: get_today_schedule, get_upcoming_events, search_events) ---
    calendar_today = (
        "what's on today" in msg or "today's schedule" in msg or "what do i have today" in msg
        or "whats on today" in msg or "my calendar today" in msg or "calendar today" in msg
        or "what's on my calendar" in msg and "week" not in msg and "upcoming" not in msg
    )
    calendar_upcoming = (
        "upcoming events" in msg or "this week" in msg and ("calendar" in msg or "schedule" in msg or "have" in msg)
        or "what do i have this week" in msg or "events this week" in msg
    )
    calendar_search = "find event" in msg or "search event" in msg or "event about" in msg or "calendar search" in msg
    if calendar_today:
        try:
            from calendar_tools import get_today_schedule
            results.append(f"[get_today_schedule]\n{get_today_schedule()}")
        except Exception as e:
            results.append(f"[get_today_schedule failed] {e}")
    elif calendar_upcoming:
        try:
            from calendar_tools import get_upcoming_events
            results.append(f"[get_upcoming_events]\n{get_upcoming_events(days=7, max_results=20)}")
        except Exception as e:
            results.append(f"[get_upcoming_events failed] {e}")
    elif calendar_search:
        # Extract query: "find event X" / "event about Y"
        query = message.strip()
        for prefix in ("find event", "search event", "event about", "calendar search"):
            if query.lower().startswith(prefix):
                query = query[len(prefix):].strip()
                break
        if len(query) > 2:
            try:
                from calendar_tools import search_events
                results.append(f"[search_events]\n{search_events(query, days_back=30, days_ahead=60, max_results=15)}")
            except Exception as e:
                results.append(f"[search_events failed] {e}")

    # --- Gmail (search only from PWA; read/draft/send stay in Continue for safety) ---
    gmail_triggers = ("check my email", "my email", "search email", "search gmail", "gmail", "recent email", "unread email")
    if any(t in msg for t in gmail_triggers):
        query = "is:unread" if ("unread" in msg or "check" in msg or "recent" in msg) else "in:inbox"
        for sep in (" for ", " about "):
            if sep in msg:
                parts = message.lower().split(sep, 1)
                if len(parts) == 2 and len(parts[1].strip()) > 1:
                    query = parts[1].strip()[:100]
                break
        try:
            from gmail_tools import search_gmail
            results.append(f"[search_gmail]\n{search_gmail(query, max_results=10)}")
        except Exception as e:
            results.append(f"[search_gmail failed] {e}")

    # --- Journal by date: "what did I write on YYYY-MM-DD" / "journal for March 5" / "journal today" ---
    journal_triggers = ("journal for", "journal on", "what did i write on", "what i wrote on", "journal entry for")
    if any(t in msg for t in journal_triggers) or ("journal" in msg and ("today" in msg or "yesterday" in msg)):
        date_str = ""
        # YYYY-MM-DD
        iso = re.search(r"(\d{4})-(\d{2})-(\d{2})", message)
        if iso:
            date_str = iso.group(0)[:10]
        if not date_str and ("today" in msg or "tonight" in msg):
            date_str = datetime.now(timezone.utc).astimezone().strftime("%Y-%m-%d")
        if not date_str and "yesterday" in msg:
            from datetime import timedelta
            date_str = (datetime.now(timezone.utc).astimezone() - timedelta(days=1)).strftime("%Y-%m-%d")
        if not date_str:
            # "March 5" / "March 5 2026"
            month_day = re.search(r"(january|february|march|april|may|june|july|august|september|october|november|december)\s+(\d{1,2})(?:\s*,?\s*(\d{4}))?", message, re.I)
            if month_day:
                try:
                    y = int(month_day.group(3)) if month_day.group(3) else datetime.now().year
                    m = datetime.strptime(month_day.group(1)[:3], "%b").month
                    d = int(month_day.group(2))
                    date_str = f"{y}-{m:02d}-{d:02d}"
                except Exception:
                    pass
        if date_str:
            try:
                from journal_tools import get_journal_for_date
                results.append(f"[get_journal_for_date({date_str})]\n{get_journal_for_date(date_str)}")
            except Exception as e:
                results.append(f"[get_journal_for_date failed] {e}")

    # --- URL fetch ---
    url_match = re.search(r"https?://[^\s\)\]\"]+", message, re.IGNORECASE)
    if url_match:
        url = url_match.group(0).rstrip(".,;:)")
        try:
            from web_tools import fetch_url
            result = fetch_url(url)
            if result:
                results.append(f"[fetch_url({url})]\n{result}")
        except Exception as e:
            results.append(f"[fetch_url failed] {e}")
    # --- Current time ---
    time_triggers = ("current time", "what time", "time is it", "time right now", "time now", "exact time", "time of day", "what's the time")
    if any(t in msg for t in time_triggers) and "weather" not in msg:
        local = datetime.now(timezone.utc).astimezone()
        try:
            time_str = local.strftime("%#I:%M %p") if sys.platform == "win32" else local.strftime("%-I:%M %p")
        except Exception:
            time_str = local.strftime("%H:%M")
        results.append(f"[current_time]\nThe current time is {time_str}.")
    # --- Current date ---
    date_triggers = ("what's today", "what day is it", "what day is today", "what's the date", "current date", "today's date", "what date is it")
    if any(t in msg for t in date_triggers):
        local = datetime.now(timezone.utc).astimezone()
        date_str = local.strftime("%A, %B %d, %Y")
        results.append(f"[current_date]\nToday is {date_str}.")
    # --- Weather ---
    if "weather" in msg:
        location = ""
        m = re.search(r"weather\s+(?:in|for|at)\s+([^.?!]+)", message, re.I)
        if m:
            location = m.group(1).strip()
        else:
            m = re.search(r"(?:the\s+)?([A-Za-z][A-Za-z\s,]+?)\s+weather", message, re.I)
            if m:
                location = m.group(1).strip()
        if location and location.lower().startswith("the "):
            location = location[4:].strip()
        if location:
            location = re.sub(r"\s+Minnesota\s*$", ", MN", location, flags=re.I).strip()
            location = re.sub(r"\s+MN\s*$", ", MN", location, flags=re.I).strip()
            if "," not in location and len(location.split()) <= 3:
                location = f"{location.strip()}, MN"
        if location and "minnesapolis" in location.lower():
            location = location.lower().replace("minnesapolis", "minneapolis").strip()
        try:
            from weather_tools import get_weather
            results.append(f"[get_weather]\n{get_weather(location=location or 'Winona, MN', days=1)}")
        except Exception as e:
            results.append(f"[get_weather failed] {e}")
    # --- Web search ---
    search_triggers = ("search", "look up", "lookup", "web search", "find online", "google", "can you search", "do a web search")
    if any(t in msg for t in search_triggers):
        query = message.strip()
        for prefix in (
            "can you search for", "can you do a web search for", "web search for",
            "search for", "look up", "lookup", "find online", "google", "search ",
        ):
            if query.lower().startswith(prefix):
                query = query[len(prefix):].strip()
                break
        for sep in (" for ", " about "):
            if sep in msg:
                idx = message.lower().find(sep)
                query = message[idx + len(sep):].strip()
                break
        if not query or len(query) < 2:
            query = message.strip()
        if len(query) > 200:
            query = query[:200]
        try:
            from search_tools import web_search
            results.append(f"[web_search]\n{web_search(query, num=6)}")
        except Exception as e:
            results.append(f"[web_search failed] {e}")
    # --- SVG â†' PNG via Inkscape ---
    convert_triggers = ("convert", "export", "turn .svg into", "svg to png")
    if any(t in msg for t in convert_triggers) and ".svg" in message:
        path_match = re.search(r"[\w./\\\-]+\.svg", message, re.IGNORECASE)
        if path_match:
            raw = path_match.group(0).strip().replace("\\", "/")
            if os.path.isabs(raw):
                p = Path(raw)
            elif raw.startswith("assets/"):
                p = (ASSETS_ROOT / raw).resolve()
            else:
                p = (PROJECT_ROOT / raw).resolve()
            if not p.exists():
                p = ASSETS_ROOT / "assets" / "girl" / (raw.split("/")[-1] if "/" in raw else raw)
            if p.suffix.lower() != ".svg":
                p = p.with_suffix(".svg")
            try:
                from inkscape_convert import convert_svg_to_png
                out = convert_svg_to_png(p)
                if out:
                    try:
                        rel = out.relative_to(PROJECT_ROOT).as_posix()
                    except ValueError:
                        try:
                            rel = out.relative_to(ASSETS_ROOT).as_posix()
                        except ValueError:
                            rel = str(out)
                    results.append(f"[inkscape_convert]\nConverted to PNG. Output: {rel}")
                else:
                    results.append("[inkscape_convert] Inkscape not found or export failed. Install Inkscape or export the SVG manually in the app.")
            except Exception as e:
                results.append(f"[inkscape_convert failed] {e}")
    if results:
        return "\n\n---\n\n".join(results)
    return None




def _parse_thinking_reply(raw: str) -> list[dict]:
    """If reply starts with [THINKING: ...], split into thinking + main reply for double-bubble display. Thinking can be multi-line (until closing ]). Returns list of {content, style}."""
    raw = (raw or "").strip()
    if not raw:
        return [{"content": "(no reply)", "style": "final"}]
    m = re.match(r"^\s*\[THINKING:\s*([\s\S]*?)\]\s*(?:\n+)?([\s\S]*)", raw, re.IGNORECASE)
    if m:
        thinking = m.group(1).strip()
        rest = (m.group(2) or "").strip()
        if thinking:
            out = [{"content": thinking, "style": "thinking"}]
            if rest:
                out.append({"content": rest, "style": "final"})
            return out
    return [{"content": raw, "style": "final"}]


def _parse_learning_abstract_and_summary(raw: str) -> tuple[str, str | None, str | None]:
    """Extract [ABSTRACT: ...] and end summary (## Summary / ## Key takeaways or [SUMMARY: ...]) from a Learning-mode reply. Returns (content_unchanged, abstract, summary)."""
    if not raw or not isinstance(raw, str):
        return (raw or "", None, None)
    text = raw.strip()
    abstract = None
    summary = None
    # Optional: strip [ABSTRACT: ...] from the start for display (we keep it in content so user sees it; abstract is for sidebar/vector)
    ab_match = re.match(r"^\s*\[ABSTRACT:\s*([^\]]+)\]\s*(?:\n+)?", text, re.IGNORECASE)
    if ab_match:
        abstract = ab_match.group(1).strip()
    # End summary: ## Summary or ## Key takeaways (capture until end or next ##)
    sum_header = re.search(
        r"\n(?:##\s*Summary|##\s*Key takeaways)\s*\n([\s\S]*?)(?=\n##\s|\n\[THINKING:|\Z)",
        text,
        re.IGNORECASE,
    )
    if sum_header:
        summary = sum_header.group(1).strip()
    if not summary:
        sum_tag = re.search(r"\[SUMMARY:\s*([\s\S]*?)\]\s*$", text, re.IGNORECASE)
        if sum_tag:
            summary = sum_tag.group(1).strip()
    return (raw, abstract or None, summary or None)


DEFAULT_QUICK_REPLIES = ["Tell me more", "Explain that simply", "Give me an example", "What else?"]
# For short/casual messages (e.g. "hey~", "hi!") so "What else?" etc. don't appear after a greeting
CASUAL_QUICK_REPLIES = [
    "hehe how are you???? <3",
    "uuuuugh hi bestie",
    "omg the strangest thing happened....",
    "<3",
]


def _is_casual_message(text: str) -> bool:
    """True if the message is short or greeting-like; use casual quick replies instead of deep/LLM ones."""
    t = (text or "").strip()
    if not t or len(t) > 55:
        return False
    # Short messages: greetings, emoji-only, very brief
    lower = t.lower()
    greetings = ("hey", "hi ", "hi!", "hello", "heya", "hiya", "yo ", "sup", "hii", "hiii", "hey~", "hey!")
    if any(lower.startswith(g) or lower == g.rstrip() or t == g for g in greetings):
        return True
    if len(t) <= 25 and not any(c in t for c in "?.!;"):
        return True  # very short and not a question
    return False


def _gateway_complete(system: str, user: str, *, timeout: int = 15) -> str | None:
    """One-shot completion via the Chimera gateway. Returns content string or None on failure."""
    try:
        import requests as _req
    except ImportError:
        return None
    try:
        r = _req.post(
            f"{GATEWAY_URL}/v1/chat/completions",
            headers={
                "Authorization": f"Bearer {GATEWAY_TOKEN}",
                "Content-Type": "application/json",
            },
            json={
                "model": GATEWAY_VIRTUAL_MODEL,
                "messages": [
                    {"role": "system", "content": system},
                    {"role": "user", "content": user},
                ],
                "stream": False,
            },
            timeout=timeout,
        )
        r.raise_for_status()
        content = ((r.json().get("choices") or [{}])[0].get("message", {}) or {}).get("content", "").strip()
        return content or None
    except Exception:
        return None


def _generate_followup_suggestions(assistant_reply: str, mode: str | None) -> list[str]:
    """Generate 4 short follow-up options via the Chimera gateway; on failure returns DEFAULT_QUICK_REPLIES."""
    text = (assistant_reply or "").strip()
    if not text:
        return list(DEFAULT_QUICK_REPLIES)
    # Use final part only (skip [THINKING: ...]) for length check
    check_text = text
    if "[THINKING:" in text.upper():
        parts = re.split(r"\[THINKING:[^\]]*\]", text, flags=re.IGNORECASE, maxsplit=1)
        check_text = (parts[-1].strip() if len(parts) > 1 else text)
    if _is_casual_message(check_text):
        return list(CASUAL_QUICK_REPLIES)
    if len(check_text) < 10:
        return list(DEFAULT_QUICK_REPLIES)
    # Use final part only (skip [THINKING: ...]) and cap length for prompt
    if "[THINKING:" in text.upper():
        parts = re.split(r"\[THINKING:[^\]]*\]", text, flags=re.IGNORECASE, maxsplit=1)
        text = (parts[-1].strip() if len(parts) > 1 else text)[:600]
    else:
        text = text[:600]
    system = (
        "You output exactly 4 short follow-up options that a user could tap to reply to the assistant's message. "
        "Each option must be a direct, specific response to what the assistant just said (e.g. a question they asked, a topic they mentioned). "
        "Keep each to 2-6 words; no quotes or numbering. Output only the 4 lines, one per line, nothing else."
    )
    user = f"Assistant just said:\n\n{text}\n\nOutput exactly 4 short follow-up options (one per line):"
    raw = _gateway_complete(system, user, timeout=15)
    if not raw:
        return list(DEFAULT_QUICK_REPLIES)
    lines = [ln.strip() for ln in raw.replace("\r", "\n").split("\n") if ln.strip()]
    out: list[str] = []
    for ln in lines:
        if len(out) >= 4:
            break
        ln = re.sub(r"^[\d\.\)\-]\s*", "", ln).strip()
        if not ln or ln.lower().startswith("output") or ln.lower().startswith("here are"):
            continue
        if len(ln) > 45:
            ln = ln[:42].rstrip() + "…"
        out.append(ln)
    if len(out) >= 3:
        return out[:4]
    return list(DEFAULT_QUICK_REPLIES)


def _generate_floating_thoughts(assistant_reply: str, mode: str | None) -> list[str]:
    """Lightweight: 2-3 context-aware follow-ups only. Chimera owns model routing."""
    text = (assistant_reply or "").strip()
    if not text:
        return []
    check_text = text
    if "[THINKING:" in text.upper():
        parts = re.split(r"\[THINKING:[^\]]*\]", text, flags=re.IGNORECASE, maxsplit=1)
        check_text = (parts[-1].strip() if len(parts) > 1 else text)
    if _is_casual_message(check_text) or len(check_text) < 20:
        return []
    if "[THINKING:" in text.upper():
        parts = re.split(r"\[THINKING:[^\]]*\]", text, flags=re.IGNORECASE, maxsplit=1)
        text = (parts[-1].strip() if len(parts) > 1 else text)[:500]
    else:
        text = text[:500]
    system = (
        "Output exactly 2 or 3 short follow-up options the user could say in reply. "
        "Each must be a direct, specific response to what the assistant just said. "
        "Keep each to 2-6 words. Output only the 2 or 3 lines, one per line, nothing else."
    )
    user = f"Assistant just said:\n\n{text}\n\nOutput exactly 2 or 3 short follow-up options (one per line):"
    raw = _gateway_complete(system, user, timeout=12)
    if not raw:
        return []
    lines = [ln.strip() for ln in raw.replace("\r", "\n").split("\n") if ln.strip()]
    out = []
    for ln in lines:
        if len(out) >= 3:
            break
        ln = re.sub(r"^[\d\.\)\-]\s*", "", ln).strip()
        if not ln or ln.lower().startswith("output") or ln.lower().startswith("here are"):
            continue
        if len(ln) > 40:
            ln = ln[:37].rstrip() + "..."
        out.append(ln)
    if 2 <= len(out) <= 3:
        return out
    return []

def _mobile_chat_reply(
    messages: list[dict],
    tool_results: str | None = None,
    current_user: str | None = None,
    mode: str | None = None,
    _system_override: str | None = None,
) -> str:
    """Pass the conversation to Chimera (gateway). Chimera owns personality, memory, and model routing.

    messages: list of {"role": "user"|"assistant", "content": "..."} — latest user message is last.
    tool_results: factual output from Locus-local tools (calendar/journal/etc); prepended as system context.
    current_user / mode: forwarded as headers so Chimera can route and personalize.
    _system_override: optional system prompt (group-chat framing); prepended before tool_results.
    """
    try:
        import requests as _req
    except ImportError:
        return "Locus needs the `requests` package — run: py -m pip install requests"

    user_msgs = [m for m in messages if m.get("role") == "user"]
    if not user_msgs or not user_msgs[-1].get("content", "").strip():
        return "Say something!"

    hist = messages[-30:]
    chat_msgs = [{"role": m["role"], "content": m["content"]} for m in hist]

    # Local context Chimera can't see from its side: framing + tool output
    system_parts: list[str] = []
    if _system_override:
        system_parts.append(_system_override.strip())
    if tool_results and tool_results.strip():
        system_parts.append(
            "--- Tool results from Locus (use exact numbers when answering) ---\n"
            + tool_results.strip()
            + "\n--- End tool results ---"
        )
    if system_parts:
        chat_msgs.insert(0, {"role": "system", "content": "\n\n".join(system_parts)})

    headers = {
        "Authorization": f"Bearer {GATEWAY_TOKEN}",
        "Content-Type": "application/json",
    }
    if current_user:
        headers["X-User"] = current_user
    if mode:
        headers["X-Locus-Mode"] = mode

    try:
        r = _req.post(
            f"{GATEWAY_URL}/v1/chat/completions",
            headers=headers,
            json={
                "model": GATEWAY_VIRTUAL_MODEL,
                "messages": chat_msgs,
                "stream": False,
            },
            timeout=180,
        )
        r.raise_for_status()
        return ((r.json().get("choices") or [{}])[0].get("message", {}) or {}).get("content", "").strip()
    except Exception as e:
        return f"Chimera gateway didn't respond — is it running at {GATEWAY_URL}? ({e})"


app = FastAPI(title="Locus API", version="0.1.0")



# Dashboard (journal, quick facts, reminders, angel/demon collector) on same port at /dashboard
import dashboard_server as _dashboard_mod
_dashboard_mod.DASHBOARD_BASE_PATH = "/dashboard"
app.mount("/dashboard", _dashboard_mod.app, name="dashboard")

ORCHESTRATOR_MODEL = "locus"
DEFAULT_MODEL = "deepseek-coder-v2:16b"


class ChatMessage(BaseModel):
    role: str  # user | assistant | system
    content: str


class ChatRequest(BaseModel):
    model: str = ORCHESTRATOR_MODEL
    messages: list[ChatMessage]
    stream: bool = False


class OpenAIChatMessage(BaseModel):
    role: str
    content: str


class OpenAIChatRequest(BaseModel):
    model: str = ORCHESTRATOR_MODEL
    messages: list[OpenAIChatMessage]
    stream: bool | None = None


@app.get("/.well-known/appspecific/com.chrome.devtools.json")
def chrome_devtools_well_known():
    """Satisfy Chrome DevTools probe so it stops requesting this and logging 404."""
    return JSONResponse(content={})


@app.get("/api/config")
def api_config():
    """Public config for PWA: whether access token is required (when port is forwarded)."""
    return {"accessTokenRequired": False}


@app.get("/api/tags")
def api_tags():
    """Ollama-compatible: list models. We expose one 'model' so Enchanted can select it."""
    return {
        "models": [
            {
                "name": ORCHESTRATOR_MODEL,
                "modified_at": "2026-03-01T00:00:00.000Z",
                "size": 0,
                "digest": "",
                "details": {"family": "locus", "parameter_size": "0", "quantization_level": ""},
            }
        ],
    }


@app.post("/api/chat")
def api_chat(body: ChatRequest):
    """Ollama-compatible chat: run full message history through Claudia chat orchestration."""
    if not body.messages:
        raise HTTPException(status_code=400, detail="messages required")
    user_parts = [m.content for m in body.messages if m.role == "user"]
    if not user_parts:
        raise HTTPException(status_code=400, detail="at least one user message required")
    latest_user = user_parts[-1].strip()
    if not latest_user:
        raise HTTPException(status_code=400, detail="last user message is empty")
    tool_results = _run_mobile_tools(latest_user)
    history = [{"role": m.role, "content": m.content} for m in body.messages]
    reply = _mobile_chat_reply(history, tool_results=tool_results, current_user=DEFAULT_USER)

    try:
        append_exchange_to_log("mobile", "ruby", latest_user, reply, user_label="Ruby", assistant_label="Locus")
    except Exception:
        pass
    return {
        "model": body.model,
        "created_at": "",  # optional
        "response": reply,
        "done": True,
        "done_reason": "stop",
        "message": {"role": "assistant", "content": reply},
    }


@app.get("/v1/models")
def openai_models():
    """OpenAI-compatible models listing for clients that expect /v1/models."""
    now = int(time.time())
    return {
        "object": "list",
        "data": [
            {
                "id": ORCHESTRATOR_MODEL,
                "object": "model",
                "created": now,
                "owned_by": "local-locus",
            }
        ],
    }


@app.post("/v1/chat/completions")
def openai_chat(body: OpenAIChatRequest):
    """OpenAI-compatible chat completion endpoint."""
    if not body.messages:
        raise HTTPException(status_code=400, detail="messages required")

    user_parts = [m.content for m in body.messages if m.role == "user"]
    if not user_parts:
        raise HTTPException(status_code=400, detail="at least one user message required")
    latest_user = user_parts[-1].strip()
    if not latest_user:
        raise HTTPException(status_code=400, detail="last user message is empty")

    tool_results = _run_mobile_tools(latest_user)
    history = [{"role": m.role, "content": m.content} for m in body.messages]
    reply = _mobile_chat_reply(history, tool_results=tool_results, current_user=DEFAULT_USER)
    try:
        append_exchange_to_log("mobile", "ruby", latest_user, reply, user_label="Ruby", assistant_label="Locus")
    except Exception:
        pass
    now = int(time.time())
    return {
        "id": f"chatcmpl-locus-{now}",
        "object": "chat.completion",
        "created": now,
        "model": body.model,
        "choices": [
            {
                "index": 0,
                "message": {"role": "assistant", "content": reply},
                "finish_reason": "stop",
            }
        ],
        "usage": {
            "prompt_tokens": 0,
            "completion_tokens": 0,
            "total_tokens": 0,
        },
    }


# --- Conversation store API (tabs + persistent history for /web) ---

def _conversation_matches_query(c: dict, q: str) -> bool:
    """True if conversation title or any message content contains q (case-insensitive)."""
    if not q:
        return True
    q_lower = q.strip().lower()
    if (c.get("title") or "").lower().find(q_lower) != -1:
        return True
    for m in c.get("messages") or []:
        if (m.get("content") or "").lower().find(q_lower) != -1:
            return True
    return False


# Search index: title + first/last message snippet so Grok/Cursor/Continue can match inside message content
_SEARCHABLE_SNIPPET_CHARS = 500


def _searchable_text_for_convo(title: str, messages: list) -> str:
    """Build a single searchable string from title and first/last message content (first N chars each)."""
    parts = [(title or "").strip()]
    if not messages:
        return " ".join(parts)
    first_content = (messages[0].get("content") or "").strip()[:_SEARCHABLE_SNIPPET_CHARS]
    if first_content:
        parts.append(first_content)
    if len(messages) > 1:
        last_content = (messages[-1].get("content") or "").strip()[:_SEARCHABLE_SNIPPET_CHARS]
        if last_content != first_content and last_content:
            parts.append(last_content)
    return " ".join(parts)


def _external_convo_matches_query(c: dict, q_lower: str) -> bool:
    """True if external convo (Grok/Cursor/Continue) title or searchable_text contains q_lower."""
    if not q_lower:
        return True
    title = (c.get("title") or "").lower()
    searchable = (c.get("searchable_text") or "").lower()
    return q_lower in title or q_lower in searchable


def _generate_chat_title(first_user_message: str, first_assistant_reply: str) -> str | None:
    """Ask Chimera for a 3-6 word chat title from the first exchange. Returns None on failure or timeout."""
    first_user_message = (first_user_message or "").strip()[:400]
    first_assistant_reply = (first_assistant_reply or "").strip()[:300]
    if not first_user_message:
        return None
    system = "You are a titling helper. Reply with only a short chat title: 3 to 6 words, no quotes, no period. Capture the topic or mood of the conversation."
    user = f"First message: {first_user_message}\nFirst reply: {first_assistant_reply}\nTitle:"
    raw = _gateway_complete(system, user, timeout=8)
    if not raw:
        return None
    raw = raw.strip('"\'').split("\n")[0].strip()[:60]
    return raw if raw else None

@app.get("/conversations")
def list_conversations(
    request: Request,
    q: str | None = None,
    current_user: str = Depends(get_current_user),
):
    """List conversations — pinned first, then by updated_at desc. If q is set, only return convos where title or any message content contains q (case-insensitive). Special: q=star, q=starred, or a star emoji (â­ â˜… â˜† ðŸŒŸ) returns only starred (important) convos. When signed in and q is set, Grok/Cursor/Continue are included: search matches title plus a content index (first/last message snippet) so search finds text inside messages. Grok/Cursor/Continue are only included in search when signed in."""
    data = _load_store()
    convos = list(_conversations_for_user(data, current_user))
    eng = _load_engagement()
    by_key = eng.get("by_key", {})
    for c in convos:
        cid = str(c.get("id") or "")
        key = _engagement_key("mobile", cid)
        if key in by_key and "important" in by_key[key]:
            c["important"] = bool(by_key[key]["important"])
    # Hide test/API-check convos from sidebar (smoke test creates these)
    convos = [c for c in convos if (c.get("title") or "").strip() not in ("Smoke test", "API check")]
    q_raw = (q or "").strip()
    q_clean = q_raw.lower()
    # Starred-only: "star", "starred", or a star emoji (â­ â˜… â˜† ðŸŒŸ etc.). Normalize so PC emoji (e.g. with variation selector U+FE0F) matches.
    star_emojis = ("\u2b50", "\u2605", "\u2606", "\u1f31f", "\u272a")  # â­ â˜… â˜† ðŸŒŸ âœª
    _q_norm = unicodedata.normalize("NFC", q_raw)
    _q_norm = "".join(c for c in _q_norm if c not in "\uFE00\uFE01\uFE02\uFE03\uFE04\uFE05\uFE06\uFE07\uFE08\uFE09\uFE0A\uFE0B\uFE0C\uFE0D\uFE0E\uFE0F")
    is_starred_only = q_clean in ("star", "starred") or _q_norm in star_emojis
    if is_starred_only:
        convos = [c for c in convos if c.get("important")]
    elif q_clean:
        convos = [c for c in convos if _conversation_matches_query(c, q)]
    pinned = sorted([c for c in convos if c.get("pinned")], key=lambda c: c.get("updated_at", "") or "", reverse=True)
    unpinned = sorted([c for c in convos if not c.get("pinned")], key=lambda c: c.get("updated_at", "") or "", reverse=True)
    all_fb = _get_all_feedback_counts()
    def summary(c, src="mobile"):
        cid = str(c.get("id") or "")
        liked, noted = all_fb.get(cid, (0, 0)) if src == "mobile" else (0, 0)
        s = _conversation_summary(c, feedback_liked=liked, feedback_noted=noted)
        s["source"] = src
        return s
    combined = [summary(c, "mobile") for c in pinned + unpinned]
    if True:
        try:
            grok_list = _list_grok_conversations()
            eng = _load_engagement()
            by_key = eng.setdefault("by_key", {})
            migrated = False
            for c in grok_list:
                gid = str(c.get("id") or "")
                key = _engagement_key("grok", gid)
                if "\u2705" in (c.get("title") or ""):
                    entry = by_key.setdefault(key, {"open_count": 0, "last_opened_at": None, "important": False})
                    if not entry.get("important"):
                        entry["important"] = True
                        by_key[key] = entry
                        migrated = True
                if key in by_key and "important" in by_key[key]:
                    c["important"] = bool(by_key[key]["important"])
            if migrated:
                _save_engagement(eng)
            if q_clean or is_starred_only:
                if is_starred_only:
                    for c in grok_list:
                        if c.get("important"):
                            combined.append(summary(c, "grok"))
                    cursor_list = _list_cursor_conversations()
                    for c in cursor_list:
                        key = _engagement_key("cursor", str(c.get("id") or ""))
                        if key in by_key and by_key[key].get("important"):
                            c["important"] = True
                            combined.append(summary(c, "cursor"))
                    continue_list = _list_continue_sessions()
                    for c in continue_list:
                        key = _engagement_key("continue", str(c.get("id") or ""))
                        if key in by_key and by_key[key].get("important"):
                            c["important"] = True
                            combined.append(summary(c, "continue"))
                    combined = sorted(combined, key=lambda c: c.get("updated_at", "") or "", reverse=True)
                elif q_clean:
                    grok_matches = [c for c in grok_list if _external_convo_matches_query(c, q_clean)]
                    for c in grok_matches:
                        combined.append(summary(c, "grok"))
                    cursor_list = _list_cursor_conversations()
                    for c in cursor_list:
                        key = _engagement_key("cursor", str(c.get("id") or ""))
                        if key in by_key and "important" in by_key[key]:
                            c["important"] = bool(by_key[key]["important"])
                    cursor_matches = [c for c in cursor_list if _external_convo_matches_query(c, q_clean)]
                    for c in cursor_matches:
                        combined.append(summary(c, "cursor"))
                    continue_list = _list_continue_sessions()
                    for c in continue_list:
                        key = _engagement_key("continue", str(c.get("id") or ""))
                        if key in by_key and "important" in by_key[key]:
                            c["important"] = bool(by_key[key]["important"])
                    continue_matches = [c for c in continue_list if _external_convo_matches_query(c, q_clean)]
                    for c in continue_matches:
                        combined.append(summary(c, "continue"))
                    combined = sorted(combined, key=lambda c: c.get("updated_at", "") or "", reverse=True)
        except Exception:
            pass
    payload = {"conversations": combined}
    return JSONResponse(
        content=payload,
        headers={"Cache-Control": "no-store, no-cache, must-revalidate", "Pragma": "no-cache"},
    )


@app.get("/conversations/archive")
def list_archive(current_user: str = Depends(get_current_user)):
    """List archived conversations."""
    data = _load_store()
    return {"conversations": [_conversation_summary(c) for c in _archive_for_user(data, current_user)]}


@app.get("/api/search-conversations")
def search_conversations(q: str = "", semantic: bool = False, current_user: str = Depends(get_current_user)):
    """Search past conversations with keyword matching. Optional semantic=true for vector search via Chimera.
    Returns: [{ "id": "...", "title": "...", "date": "...", "score": 0.95, "snippet": "...", "source": "keyword|semantic" }]
    """
    if not q or not q.strip():
        raise HTTPException(400, "Query parameter 'q' required")

    results = []

    # --- Semantic search via Chimera (if requested) ---
    if semantic:
        try:
            import requests
            headers = {"X-Vibe-Token": GATEWAY_TOKEN}
            resp = requests.post(
                f"{GATEWAY_URL}/search",
                json={"query": q.strip(), "top_k": 20},
                headers=headers,
                timeout=5
            )
            if resp.status_code == 200:
                chimera_results = resp.json() or []
                # Transform Chimera format -> Locus format (Chimera should return structured results)
                data = _load_store()
                conv_map = {c.get("id"): c for c in data.get("conversations", []) + data.get("archive", [])}

                for r in chimera_results:
                    conv_id = r.get("id") or r.get("conversation_id")
                    if conv_id and conv_id in conv_map:
                        c = conv_map[conv_id]
                        if (c.get("owner") or DEFAULT_USER) == current_user:
                            results.append({
                                "id": conv_id,
                                "title": r.get("title") or c.get("title") or "Chat",
                                "date": r.get("date") or c.get("created_at", "")[:10],
                                "score": round(r.get("score", 0.5), 2),
                                "snippet": r.get("snippet") or "",
                                "source": "semantic"
                            })

                if results:
                    results.sort(key=lambda x: x["score"], reverse=True)
                    return results[:20]
                # If Chimera returned no results, fall back to keyword search
        except Exception as e:
            import logging
            logging.warning(f"Chimera search failed ({e}), falling back to keyword search")

    # --- Keyword search on mobile conversations (fallback or if semantic=false) ---
    data = _load_store()
    convos = list(_conversations_for_user(data, current_user))

    q_lower = q.strip().lower()
    for c in convos:
        if _conversation_matches_query(c, q):
            title = c.get("title") or "Chat"
            cid = str(c.get("id") or "")
            created = c.get("created_at") or c.get("updated_at") or ""
            # Extract snippet from first matching message or title
            snippet = ""
            for m in c.get("messages") or []:
                content = (m.get("content") or "").lower()
                if q_lower in content:
                    idx = content.find(q_lower)
                    start = max(0, idx - 50)
                    end = min(len(m.get("content") or ""), idx + 100)
                    snippet = m.get("content", "")[start:end].strip()
                    if start > 0:
                        snippet = "..." + snippet
                    if end < len(m.get("content") or ""):
                        snippet = snippet + "..."
                    break
            if not snippet:
                snippet = title[:100]

            # Count matches for scoring
            match_count = sum(1 for m in c.get("messages") or [] if q_lower in (m.get("content") or "").lower())
            match_count += (1 if q_lower in title.lower() else 0)
            score = min(1.0, 0.5 + (match_count * 0.1))

            results.append({
                "id": cid,
                "title": title,
                "date": created[:10] if created else "unknown",
                "score": round(score, 2),
                "snippet": snippet,
                "source": "keyword",
            })

    results.sort(key=lambda x: x["score"], reverse=True)
    return results[:20]


@app.get("/conversations/{conv_id}")
def get_conversation(
    conv_id: str,
    branch: int = 0,
    current_user: str = Depends(get_current_user),
):
    """Get one conversation with full messages. Optional branch=0|1|... for threaded convos (1/2, 2/2)."""
    data = _load_store()
    c = _get_conversation_by_id(data, conv_id, current_user)
    if not c:
        raise HTTPException(status_code=404, detail=MOBILE_CONVERSATION_404)
    branches = _get_branches(c)
    branch_index = max(0, min(branch, len(branches) - 1)) if branches else 0
    out = dict(c)
    out["messages"] = branches[branch_index] if branches else (c.get("messages") or [])
    out["branch_count"] = len(branches)
    out["branch_index"] = branch_index
    if branches and len(branches) > 1:
        out["branch_point"] = _branch_point(branches)
    return out


@app.get("/api/chat_image", response_class=Response)
def serve_chat_image(path: str = "", current_user: str = Depends(get_current_user)):
    """Serve an image stored for chat (Phone_Photos). path = relative to project root; must be under this user's folder."""
    if not path or ".." in path:
        raise HTTPException(status_code=400, detail="invalid path")
    rel = path.replace("\\", "/").strip()
    if not rel.startswith("Journal_Database/Phone_Photos/"):
        raise HTTPException(status_code=403, detail="path not allowed")
    allowed_prefix = f"Journal_Database/Phone_Photos/{current_user}/"
    # Ruby can also serve legacy paths (Phone_Photos/YYYY-MM-DD/...) for backward compat
    if rel.startswith(allowed_prefix):
        pass
    elif current_user == DEFAULT_USER and not (rel.startswith("Journal_Database/Phone_Photos/lynn/") or rel.startswith("Journal_Database/Phone_Photos/raven/")):
        pass
    else:
        raise HTTPException(status_code=403, detail="path not allowed for this user")
    full = (PROJECT_ROOT / rel).resolve()
    try:
        if not full.is_file() or not str(full).startswith(str(PROJECT_ROOT.resolve())):
            raise HTTPException(status_code=404, detail="not found")
    except (OSError, ValueError):
        raise HTTPException(status_code=404, detail="not found")
    suffix = full.suffix.lower()
    media = "image/jpeg" if suffix in (".jpg", ".jpeg") else "image/png" if suffix == ".png" else "image/gif" if suffix == ".gif" else "image/webp" if suffix == ".webp" else "application/octet-stream"
    try:
        return Response(content=full.read_bytes(), media_type=media)
    except OSError:
        raise HTTPException(status_code=404, detail="not found")


def _activity_response(
    messages: list,
    title: str,
    message_count: int | None = None,
    sparkline_data: list | None = None,
    feedback_liked: int = 0,
    feedback_noted: int = 0,
    title_history: list | None = None,
):
    """Build activity breakdown payload from messages, optional cached counts, feedback (thumbs), and title rename history."""
    buckets = _activity_buckets(messages)
    msg_count = message_count if message_count is not None else len(messages)
    sparkline = sparkline_data if sparkline_data is not None else _sparkline_from_messages(messages)
    out = {
        "title": title or "Chat",
        "message_count": msg_count,
        "files": buckets["files"],
        "code_snippets": buckets["code_snippets"],
        "media": buckets["media"],
        "feedback_liked": feedback_liked,
        "feedback_noted": feedback_noted,
        "sparkline_data": sparkline,
    }
    if title_history:
        out["title_history"] = title_history
    return out


@app.get("/activity")
def get_activity_by_source(
    source: str = "mobile",
    id: str = "",
    current_user: str = Depends(get_current_user),
):
    """Activity breakdown for any source (mobile, continue, grok, cursor). Use source= and id= query params."""
    conv_id = (id or "").strip()
    src = (source or "mobile").strip().lower() or "mobile"
    if not conv_id:
        raise HTTPException(status_code=400, detail="id required")
    if src == "mobile":
        data = _load_store()
        c = _get_conversation_by_id(data, conv_id, current_user)
        if not c:
            raise HTTPException(status_code=404, detail=MOBILE_CONVERSATION_404)
        messages = c.get("messages") or []
        liked, noted = _get_feedback_counts_for_conversation(conv_id)
        return _activity_response(
            messages,
            c.get("title") or "Chat",
            c.get("message_count"),
            c.get("sparkline_data"),
            feedback_liked=liked,
            feedback_noted=noted,
        )
    if src == "continue":
        messages = _get_continue_session_messages(conv_id)
        if messages is None:
            raise HTTPException(status_code=404, detail="Continue session not found")
        meta = next((s for s in _list_continue_sessions() if s.get("id") == conv_id), None)
        title = (meta.get("title") or "Untitled") if meta else "Untitled"
        return _activity_response(messages, title)
    if src == "grok":
        messages = _get_grok_messages(conv_id)
        if messages is None:
            raise HTTPException(status_code=404, detail="Grok conversation not found")
        meta = next((c for c in _list_grok_conversations() if c.get("id") == conv_id), None)
        title = (meta.get("title") or "Grok chat") if meta else "Grok chat"
        return _activity_response(messages, title)
    if src == "cursor":
        messages = _get_cursor_messages(conv_id)
        if messages is None:
            raise HTTPException(status_code=404, detail="Cursor conversation not found")
        meta = next((c for c in _list_cursor_conversations() if c.get("id") == conv_id), None)
        title = (meta.get("title") or "Cursor chat") if meta else "Cursor chat"
        return _activity_response(messages, title)
    raise HTTPException(status_code=400, detail="source must be mobile, continue, grok, or cursor")


@app.get("/conversations/{conv_id}/activity")
def get_conversation_activity(conv_id: str, current_user: str = Depends(get_current_user)):
    """Activity Breakdown for one conversation: bucket counts (messages, files, code_snippets, media, liked/noted) + sparkline (mobile only)."""
    data = _load_store()
    c = _get_conversation_by_id(data, conv_id, current_user)
    if not c:
        raise HTTPException(status_code=404, detail=MOBILE_CONVERSATION_404)
    branches = _get_branches(c)
    messages = branches[0] if branches else (c.get("messages") or [])
    liked, noted = _get_feedback_counts_for_conversation(conv_id)
    return _activity_response(
        messages,
        c.get("title") or "Chat",
        c.get("message_count"),
        c.get("sparkline_data"),
        feedback_liked=liked,
        feedback_noted=noted,
        title_history=c.get("title_history"),
    )


@app.post("/conversations")
def create_conversation(current_user: str = Depends(get_current_user)):
    """Create a new conversation; returns id and summary."""
    data = _load_store()
    now = datetime.now(timezone.utc).isoformat()
    conv_id = str(uuid.uuid4())
    new_c = {
        "id": conv_id,
        "owner": current_user,
        "title": "New chat",
        "created_at": now,
        "updated_at": now,
        "messages": [],
    }
    data.setdefault("conversations", []).insert(0, new_c)
    _save_store(data)
    return _conversation_summary(new_c)


class ForkConversationBody(BaseModel):
    title: str = "Continued chat"
    messages: list[dict] = []
    branches: list[list[dict]] | None = None  # optional: when forking from Grok (or other) with threads


@app.post("/conversations/fork")
def fork_conversation(body: ForkConversationBody, current_user: str = Depends(get_current_user)):
    """Create a new mobile conversation pre-seeded with messages from another source. If branches is provided (e.g. from Grok export with threads), the new convo supports 1/2, 2/2 threading."""
    data = _load_store()
    now = datetime.now(timezone.utc).isoformat()
    conv_id = str(uuid.uuid4())

    def normalize(msgs: list[dict]) -> list[dict]:
        return [
            {"role": m.get("role", "user"), "content": (m.get("content") or "").strip()}
            for m in msgs
            if (m.get("content") or "").strip()
        ]

    if body.branches and isinstance(body.branches, list) and len(body.branches) > 0:
        branches = [normalize(b) for b in body.branches if isinstance(b, list)]
        if not branches:
            branches = [[]]
        new_c = {
            "id": conv_id,
            "owner": current_user,
            "title": body.title or "Continued chat",
            "created_at": now,
            "updated_at": now,
            "pinned": False,
            "branches": branches,
        }
        first_branch = branches[0]
        new_c["message_count"] = len(first_branch)
        new_c["sparkline_data"] = _sparkline_from_messages(first_branch)
    else:
        messages = normalize(body.messages)
        new_c = {
            "id": conv_id,
            "owner": current_user,
            "title": body.title or "Continued chat",
            "created_at": now,
            "updated_at": now,
            "pinned": False,
            "messages": messages,
        }
    data.setdefault("conversations", []).insert(0, new_c)
    _save_store(data)
    out = _conversation_summary(new_c)
    out["messages"] = _get_branches(new_c)[0] if "branches" in new_c else new_c.get("messages", [])
    out["branch_count"] = len(_get_branches(new_c))
    out["branch_index"] = 0
    return out


class FileAttachmentItem(BaseModel):
    file_base64: str = ""
    file_name: str = "file"
    file_mime: str | None = None


class SendMessageBody(BaseModel):
    content: str = ""
    image_base64: str | None = None  # optional: base64 image (or data URL with base64) so Claudia can "see" it via vision
    file_base64: str | None = None  # optional: single file (backward compat)
    file_name: str | None = None
    file_mime: str | None = None
    files: list[FileAttachmentItem] | None = None  # optional: multiple files (e.g. PDFs); max 6 total with image
    mode: str | None = None  # "bestie" | "therapist" | "learning" — affects system prompt tone
    branch_index: int | None = None  # for threaded convos: which branch (0, 1, ...) to append to; default 0
    batch: list[str] | None = None  # optional: multiple user messages in one go (quadruple-text); no image/file; one reply


def _is_image_request(text: str) -> bool:
    """True if the user message looks like a request to generate an image (draw, picture of, etc.)."""
    t = (text or "").strip().lower()
    if not t:
        return False
    patterns = [
        r"draw\s",
        r"generate\s*(an?)?\s*(image|picture|photo|pic)\b",
        r"picture\s+of\b",
        r"create\s*(an?)?\s*image\b",
        r"make\s*(me\s*)?(an?)?\s*image\b",
        r"can you draw\b",
        r"draw me\b",
    ]
    return any(re.search(p, t) for p in patterns)


def _try_generate_image_ollama(prompt: str, user_id: str) -> str | None:
    """Try to generate an image with Ollama (e.g. flux). Saves to Phone_Photos, returns relative path or None."""
    import subprocess
    import shutil
    now = datetime.now(timezone.utc)
    date_dir = now.strftime("%Y-%m-%d")
    user_slug = user_id if user_id else DEFAULT_USER
    save_dir = PHONE_PHOTOS_DIR / user_slug / date_dir
    save_dir.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(prefix="ollama_img_") as tmpdir:
        try:
            # Ollama image models: flux, flux:schnell, sdxl, etc. Try flux first (common).
            proc = subprocess.run(
                ["ollama", "run", "flux", prompt[:500]],
                cwd=tmpdir,
                capture_output=True,
                text=True,
                timeout=90,
            )
            # Some Ollama image flows write to cwd; check for new image files
            for f in Path(tmpdir).iterdir():
                if f.suffix.lower() in (".png", ".jpg", ".jpeg", ".webp"):
                    dest = save_dir / f"{now.strftime('%Y-%m-%d_%H%M%S')}_{uuid.uuid4().hex[:8]}{f.suffix}"
                    shutil.copy2(f, dest)
                    try:
                        return dest.relative_to(PROJECT_ROOT).as_posix()
                    except ValueError:
                        return str(dest).replace("\\", "/")
        except (FileNotFoundError, subprocess.TimeoutExpired, Exception):
            pass
    return None


def _describe_image_from_base64(image_b64: str, user_id: str = DEFAULT_USER) -> tuple[str, str | None]:
    """Decode base64 image, save to Journal_Database/Phone_Photos/{user_id}/YYYY-MM-DD/, describe with vision.
    Returns (description, relative_path_for_chat). relative_path is e.g. Journal_Database/Phone_Photos/ruby/2026-03-03/xxx.jpg for embedding in chat."""
    raw = image_b64.strip()
    if raw.startswith("data:"):
        idx = raw.find("base64,")
        raw = raw[idx + 7:] if idx >= 0 else raw
    try:
        data = base64.b64decode(raw, validate=True)
    except Exception as e:
        return (f"[Image decode failed: {e}]", None)
    if len(data) > 20 * 1024 * 1024:
        return ("[Image too large; max ~20MB.]", None)
    suffix = ".png"
    if data[:8] == b"\x89PNG\r\n\x1a\n":
        suffix = ".png"
    elif data[:2] == b"\xff\xd8":
        suffix = ".jpg"
    elif data[:6] in (b"GIF87a", b"GIF89a"):
        suffix = ".gif"
    elif data[:4] == b"RIFF" and data[8:12] == b"WEBP":
        suffix = ".webp"
    now = datetime.now(timezone.utc)
    date_dir = now.strftime("%Y-%m-%d")
    user_slug = user_id if user_id else DEFAULT_USER
    save_dir = PHONE_PHOTOS_DIR / user_slug / date_dir
    save_dir.mkdir(parents=True, exist_ok=True)
    base_name = f"{now.strftime('%Y-%m-%d_%H%M%S')}_{uuid.uuid4().hex[:8]}"
    image_path = save_dir / f"{base_name}{suffix}"
    try:
        image_path.write_bytes(data)
    except Exception as e:
        return (f"[Could not save image: {e}]", None)
    try:
        rel_path = image_path.relative_to(PROJECT_ROOT).as_posix()
    except ValueError:
        rel_path = str(image_path).replace("\\", "/")
    prompt = "Describe this image: what's in it, any text visible, mood or context. Be concise but useful so someone can reply to it."
    desc = ""
    model_used = ""
    from web_tools import describe_image_with_cloud, describe_image_with_ollama
    if os.environ.get("GROQ_API_KEY") or os.environ.get("GEMINI_API_KEY") or os.environ.get("OPENAI_API_KEY"):
        cloud_desc = describe_image_with_cloud(path=str(image_path), prompt=prompt, delete_after=False)
        if not (cloud_desc.startswith("Error") or cloud_desc.startswith("[Vision: no cloud")):
            desc = cloud_desc
            model_used = "cloud (Groq/Gemini/OpenAI)"
    if not desc:
        try:
            desc = describe_image_with_ollama(str(image_path), prompt=prompt, delete_after=False)
            model_used = os.environ.get("VISION_MODEL", "llava:7b")
        except Exception as e:
            desc = f"[Vision failed: {e}. Is Ollama running? Try: ollama pull llava:7b. Image saved so you can re-run later.]"
            model_used = "none"
    desc_path = save_dir / f"{base_name}_description.md"
    created_iso = now.isoformat()
    try:
        desc_path.write_text(
            f"---\nmodel: {model_used}\ncreated: {created_iso}\n---\n\n{desc}",
            encoding="utf-8",
        )
    except Exception:
        pass
    return (desc, rel_path)


def _extract_pdf_text_from_base64(file_b64: str) -> str:
    """Decode base64 PDF and return extracted text. Returns error message on failure."""
    raw = file_b64.strip()
    if raw.startswith("data:"):
        idx = raw.find("base64,")
        raw = raw[idx + 7 :] if idx >= 0 else raw
    try:
        data = base64.b64decode(raw, validate=True)
    except Exception as e:
        return f"[PDF decode failed: {e}]"
    if len(data) > 20 * 1024 * 1024:
        return "[PDF too large; max ~20MB.]"
    try:
        from pypdf import PdfReader
        from io import BytesIO
        reader = PdfReader(BytesIO(data))
        parts = []
        for p in reader.pages:
            t = p.extract_text()
            if t:
                parts.append(t)
        return "\n\n".join(parts).strip() or "[PDF has no extractable text.]"
    except ImportError:
        return "[PDF text extraction requires: pip install pypdf]"
    except Exception as e:
        return f"[PDF extract failed: {e}]"


def _is_echoed_file_content(reply: str, user_content: str, tool_results: str | None) -> bool:
    """Heuristic: model sometimes echoes back sent file content as its reply. Treat as echo if reply is long and matches start of user/tool content."""
    if not reply or len(reply.strip()) < 800:
        return False
    combined = (user_content or "") + "\n\n" + (tool_results or "")
    if "[File:" not in combined and "[file_from_" not in combined:
        return False
    a = reply.strip()[:200].lower()
    b = combined.strip()[:200].lower()
    # Same doc header (e.g. "# AI Assistant" or "```md AI_ONBOARDING")
    if a and b and (a == b or (len(a) > 50 and a[:50] == b[:50])):
        return True
    return False


@app.post("/conversations/{conv_id}/messages")
def send_message(conv_id: str, body: SendMessageBody, current_user: str = Depends(get_current_user)):
    """Append user message (and optional image or file), get reply using full chat history, append it, return reply. Or batch: multiple user messages, one reply."""
    data = _load_store()
    c = _get_conversation_by_id(data, conv_id, current_user)
    if not c:
        raise HTTPException(status_code=404, detail=MOBILE_CONVERSATION_404)
    archive = data.get("archive", [])
    if c in archive:
        data["archive"] = [x for x in archive if x is not c]
        c.pop("archived_at", None)
        c["pinned"] = False
        data.setdefault("conversations", []).insert(0, c)
        _save_store(data)

    branches = _get_branches(c)
    branch_index = max(0, min((body.branch_index if body.branch_index is not None else 0), len(branches) - 1)) if (branches and len(branches) > 0) else 0
    messages = (branches[branch_index] if (branches and len(branches) > 0) else c.setdefault("messages", []))

    # Batch path: multiple user messages (quadruple-text), one reply
    batch = [s for s in (body.batch or []) if (s or "").strip()]
    if batch:
        history = _flatten_messages_for_history(messages[-28:])
        for item in batch:
            t = (item or "").strip() or "(no content)"
            history.append({"role": "user", "content": t})
            messages.append({"role": "user", "content": t})
        chat_mode = (body.mode or "").strip().lower() or None
        if chat_mode and chat_mode not in ("bestie", "therapist", "learning", "ai_tasks"):
            chat_mode = None
        tool_results = _run_mobile_tools(history[-1]["content"]) if history else None
        try:
            reply = _mobile_chat_reply(history, tool_results=tool_results, current_user=current_user, mode=chat_mode)
        except Exception:
            import logging
            logging.getLogger(__name__).exception("mobile chat reply failed (batch)")
            reply = "Locus couldn't reply right now — try again."
        replies = _parse_thinking_reply(reply)
        last_reply_content = replies[-1]["content"] if replies else reply
        learning_abstract, learning_summary = None, None
        if chat_mode == "learning" and last_reply_content:
            _, learning_abstract, learning_summary = _parse_learning_abstract_and_summary(last_reply_content)
        quick_replies_list = _generate_floating_thoughts(last_reply_content, chat_mode)
        for i, part in enumerate(replies):
            asst = {"role": "assistant", "content": part["content"], "style": part.get("style") or "final"}
            if i == len(replies) - 1:
                asst["quick_replies"] = quick_replies_list
                if learning_abstract is not None:
                    asst["abstract"] = learning_abstract
                if learning_summary is not None:
                    asst["summary"] = learning_summary
            messages.append(asst)
        if branches:
            c["branches"] = list(branches)
            c["branches"][branch_index] = messages
        else:
            c["messages"] = messages
        now = datetime.now(timezone.utc).isoformat()
        c["updated_at"] = now
        if learning_abstract is not None:
            c["last_abstract"] = learning_abstract
        if learning_summary is not None:
            c["last_summary"] = learning_summary
        first_branch = (c.get("branches") or [c.get("messages", [])])[0]
        c["message_count"] = len(first_branch)
        c["sparkline_data"] = _sparkline_from_messages(first_branch)
        _save_store(data)
        out = {"reply": last_reply_content, "replies": replies, "title": c.get("title") or "New chat", "updated_at": c.get("updated_at", now), "quick_replies": quick_replies_list}
        if learning_abstract is not None:
            out["abstract"] = learning_abstract
        if learning_summary is not None:
            out["summary"] = learning_summary
        return out

    text = (body.content or "").strip()
    image_b64 = (body.image_base64 or "").strip() or None
    file_b64 = (body.file_base64 or "").strip() or None
    file_name = (body.file_name or "").strip() or "file"
    file_mime = (body.file_mime or "").strip() or None
    extra_files = body.files or []
    if len(extra_files) > 6:
        extra_files = extra_files[:6]
    if not text and not image_b64 and not file_b64 and not extra_files:
        raise HTTPException(status_code=400, detail="content, image_base64, or file(s) required")

    tool_results = _run_mobile_tools(text) if text else None
    image_desc = None
    image_path_rel = None
    file_names_for_display = []
    if image_b64:
        image_desc, image_path_rel = _describe_image_from_base64(image_b64, current_user)
        display_name = get_user_display_name(current_user)
        block = f"[image_from_{display_name}]\n{image_desc}"
        tool_results = f"{tool_results}\n\n{block}" if tool_results else block
    if file_b64 and file_mime == "application/pdf":
        pdf_text = _extract_pdf_text_from_base64(file_b64)
        file_block = f"[file_from_Ruby: {file_name}]\n{pdf_text}"
        tool_results = f"{tool_results}\n\n{file_block}" if tool_results else file_block
        file_names_for_display.append(file_name)
    for item in extra_files:
        b64 = (item.file_base64 or "").strip()
        name = (item.file_name or "file").strip() or "file"
        mime = (item.file_mime or "").strip() or None
        if not b64:
            continue
        if mime == "application/pdf":
            pdf_text = _extract_pdf_text_from_base64(b64)
            file_block = f"[file_from_Ruby: {name}]\n{pdf_text}"
            tool_results = f"{tool_results}\n\n{file_block}" if tool_results else file_block
            file_names_for_display.append(name)

    # Message we show in history: text + optional image/file note(s)
    no_real_text = not text or text == "(no text)"
    display_content = text if text else "[Image]"
    if file_b64 or file_names_for_display:
        all_names = file_names_for_display if file_names_for_display else [file_name]
        file_part = ", ".join("[File: " + n + "]" for n in all_names)
        display_content = (text + "\n" + file_part).strip() if not no_real_text else file_part
    if image_desc and image_desc.startswith("[") and "failed" in image_desc:
        display_content = (display_content + "\n[Image: could not describe]").strip() if display_content else "[Image: could not describe]"
    elif image_desc:
        display_content = (display_content + "\n[Image: " + image_desc[:200] + ("…" if len(image_desc) > 200 else "") + "]").strip() if display_content else f"[Image: {image_desc[:300]}{'…' if len(image_desc) > 300 else ''}]"

    # Optional: try image generation when user asks for an image (e.g. "draw a cat")
    generated_image_path = None
    if _is_image_request(text):
        generated_image_path = _try_generate_image_ollama(text, current_user)
        if generated_image_path and tool_results:
            tool_results = f"{tool_results}\n\n[You generated an image for them; it's attached.]"
        elif generated_image_path:
            tool_results = "[You generated an image for them; it's attached.]"

    # Resolve which message list we're appending to (branch for threaded convos)
    branches = _get_branches(c)
    branch_index = 0
    if branches:
        branch_index = max(0, min((body.branch_index if body.branch_index is not None else 0), len(branches) - 1))
    messages = branches[branch_index] if branches else c.setdefault("messages", [])

    # Pass full conversation history (selected variant per assistant) + new user message
    history = _flatten_messages_for_history(messages[-28:])
    if text:
        user_label = text
    elif image_b64:
        user_label = "(sent an image)"
    elif file_names_for_display:
        user_label = f"(sent {len(file_names_for_display)} files)" if len(file_names_for_display) > 1 else f"(sent file: {file_names_for_display[0]})"
    elif file_b64:
        user_label = f"(sent file: {file_name})"
    else:
        user_label = "(no content)"
    history.append({"role": "user", "content": text or user_label})
    chat_mode = (body.mode or "").strip().lower() or None
    if chat_mode and chat_mode not in ("bestie", "therapist", "learning", "ai_tasks"):
        chat_mode = None
    try:
        reply = _mobile_chat_reply(history, tool_results=tool_results, current_user=current_user, mode=chat_mode)
    except Exception:
        import logging
        logging.getLogger(__name__).exception("mobile chat reply failed")
        reply = "Locus couldn't reply right now — is Ollama running? Try again."

    if _is_echoed_file_content(reply, display_content, tool_results):
        reply = "I've got those files — what would you like me to do with them? Summarize, compare, or something else?"
    if generated_image_path and not reply.strip():
        reply = "Here's your image \u2665"
    replies = _parse_thinking_reply(reply)
    last_reply_content = replies[-1]["content"] if replies else reply
    learning_abstract, learning_summary = None, None
    if chat_mode == "learning" and last_reply_content:
        _, learning_abstract, learning_summary = _parse_learning_abstract_and_summary(last_reply_content)
    now = datetime.now(timezone.utc).isoformat()
    user_msg = {"role": "user", "content": display_content}
    if image_path_rel:
        user_msg["image_path"] = image_path_rel
    if file_names_for_display:
        user_msg["file_names"] = list(file_names_for_display)
    messages.append(user_msg)
    quick_replies_list = _generate_floating_thoughts(last_reply_content, chat_mode)
    for i, part in enumerate(replies):
        asst = {"role": "assistant", "content": part["content"], "style": part.get("style") or "final"}
        if generated_image_path and i == len(replies) - 1:
            asst["generated_image_path"] = generated_image_path
        if i == len(replies) - 1:
            asst["quick_replies"] = quick_replies_list
            if learning_abstract is not None:
                asst["abstract"] = learning_abstract
            if learning_summary is not None:
                asst["summary"] = learning_summary
        messages.append(asst)
    if branches:
        c["branches"] = list(branches)
        c["branches"][branch_index] = messages
    else:
        c["messages"] = messages
    c["updated_at"] = now
    if learning_abstract is not None:
        c["last_abstract"] = learning_abstract
    if learning_summary is not None:
        c["last_summary"] = learning_summary
    # For list/summary use first branch length
    first_branch = (c.get("branches") or [c.get("messages", [])])[0]
    c["message_count"] = len(first_branch)
    c["sparkline_data"] = _sparkline_from_messages(first_branch)
    if c.get("title") == "New chat" or not c.get("title"):
        generated = _generate_chat_title(display_content, last_reply_content)
        if generated:
            c["title"] = generated[:60]
        else:
            c["title"] = (display_content[:50] + "…") if len(display_content) > 50 else (display_content or "Image")
    _save_store(data)
    try:
        append_exchange_to_log(
            "mobile",
            current_user,
            display_content,
            last_reply_content,
            user_label=get_user_display_name(current_user),
            assistant_label="Locus",
        )
    except Exception:
        pass
    out = {"reply": last_reply_content, "replies": replies, "title": c.get("title") or "New chat", "updated_at": c.get("updated_at", now), "quick_replies": quick_replies_list}
    if learning_abstract is not None:
        out["abstract"] = learning_abstract
    if learning_summary is not None:
        out["summary"] = learning_summary
    if image_path_rel:
        out["image_path"] = image_path_rel
    if generated_image_path:
        out["generated_image_path"] = generated_image_path
    return out


class RegenerateBody(BaseModel):
    """Optional body for regenerate; currently no fields required."""


@app.post("/conversations/{conv_id}/regenerate")
def regenerate_last_reply(conv_id: str, body: RegenerateBody | None = None, current_user: str = Depends(get_current_user)):
    """Regenerate the last assistant message: remove it, re-run with same history, replace with new reply."""
    data = _load_store()
    c = _get_conversation_by_id(data, conv_id, current_user)
    if not c:
        raise HTTPException(status_code=404, detail=MOBILE_CONVERSATION_404)
    messages = c.get("messages", [])
    if not messages or messages[-1].get("role") != "assistant":
        raise HTTPException(status_code=400, detail="last message must be from assistant")
    messages.pop()
    history = _flatten_messages_for_history(messages[-28:])
    try:
        reply = _mobile_chat_reply(history, tool_results=None, current_user=current_user)
    except Exception:
        import logging
        logging.getLogger(__name__).exception("regenerate failed")
        reply = "Locus couldn't reply right now — try again."
    now = datetime.now(timezone.utc).isoformat()
    c["messages"].append({"role": "assistant", "content": reply})
    c["updated_at"] = now
    c["message_count"] = len(c["messages"])
    c["sparkline_data"] = _sparkline_from_messages(c["messages"])
    _save_store(data)
    try:
        last_user = next((m.get("content") or "" for m in reversed(messages) if m.get("role") == "user"), "")
        if last_user:
            append_exchange_to_log(
                "mobile",
                current_user,
                last_user,
                reply,
                user_label=get_user_display_name(current_user),
                assistant_label="Locus",
            )
    except Exception:
        pass
    return {"reply": reply, "updated_at": c["updated_at"]}


def _message_content(m: dict) -> str:
    """Return display content for a message; for assistant with variants use selected variant."""
    if m.get("role") != "assistant":
        return m.get("content") or ""
    variants = m.get("variants")
    if isinstance(variants, list) and variants:
        sel = m.get("selected")
        if isinstance(sel, int) and 0 <= sel < len(variants):
            return variants[sel] if isinstance(variants[sel], str) else str(variants[sel])
        return variants[0] if isinstance(variants[0], str) else str(variants[0])
    return m.get("content") or ""


def _flatten_messages_for_history(messages: list[dict]) -> list[dict]:
    """Build list of {role, content} for model context; assistant messages use selected variant."""
    return [{"role": m.get("role", "user"), "content": _message_content(m)} for m in messages]


class EditAndContinueBody(BaseModel):
    message_index: int  # index of the user message to replace (must be user role)
    content: str  # new text for that message; Claudia generates a new reply (adds a variant)
    branch_index: int = 0  # which thread (branch) when conversation has multiple


@app.post("/conversations/{conv_id}/edit_and_continue")
def edit_and_continue(conv_id: str, body: EditAndContinueBody, current_user: str = Depends(get_current_user)):
    """Edit user message and generate a new assistant reply (Grok-style). Adds a new variant to the assistant message; does not truncate the thread."""
    data = _load_store()
    c = _get_conversation_by_id(data, conv_id, current_user)
    if not c:
        raise HTTPException(status_code=404, detail=MOBILE_CONVERSATION_404)
    branches = _get_branches(c)
    bi = max(0, min(body.branch_index, len(branches) - 1)) if branches else 0
    messages = list(branches[bi] if branches else (c.get("messages") or []))
    idx = body.message_index
    if idx < 0 or idx >= len(messages) or messages[idx].get("role") != "user":
        raise HTTPException(status_code=400, detail="message_index must point to a user message")
    new_content = (body.content or "").strip()
    if not new_content:
        raise HTTPException(status_code=400, detail="content required")
    # Update user message
    messages[idx] = dict(messages[idx], content=new_content)
    has_asst_after = idx + 1 < len(messages) and messages[idx + 1].get("role") == "assistant"
    if has_asst_after:
        asst = messages[idx + 1]
        current_asst_content = _message_content(asst)
        # Build history up to and including current assistant reply, then new user message
        history = _flatten_messages_for_history(messages[: idx + 1][-28:])
        history.append({"role": "user", "content": new_content})
    else:
        # No assistant after this user message (e.g. quick-reply choice just sent, or reply not saved yet) — generate first reply
        history = _flatten_messages_for_history(messages[: idx + 1][-28:])
    try:
        reply = _mobile_chat_reply(history, tool_results=None, current_user=current_user)
    except Exception:
        import logging
        logging.getLogger(__name__).exception("edit_and_continue failed")
        reply = "Locus couldn't reply right now — try again."
    if has_asst_after:
        # Add new variant to existing assistant message
        variants = asst.get("variants")
        if not isinstance(variants, list):
            variants = [current_asst_content] if current_asst_content else []
        variants.append(reply)
        sel = len(variants) - 1
        messages[idx + 1] = dict(asst, content=reply, variants=variants, selected=sel)
    else:
        # Append new assistant message (e.g. after editing a quick-reply choice)
        new_asst = {"role": "assistant", "content": reply}
        messages = messages[: idx + 1] + [new_asst]
    now = datetime.now(timezone.utc).isoformat()
    if branches:
        branches[bi] = messages
        c["branches"] = list(branches)
        c["message_count"] = len((c.get("branches") or [[]])[0])
        c["sparkline_data"] = _sparkline_from_messages((c["branches"])[0])
    else:
        c["messages"] = messages
        c["message_count"] = len(messages)
        c["sparkline_data"] = _sparkline_from_messages(messages)
    c["updated_at"] = now
    _save_store(data)
    try:
        append_exchange_to_log(
            "mobile",
            current_user,
            new_content,
            reply,
            user_label=get_user_display_name(current_user),
            assistant_label="Locus",
        )
    except Exception:
        pass
    return {"reply": reply, "title": c.get("title") or "New chat", "updated_at": now, "messages": messages}


MAX_BRANCHES = 10  # max threads per conversation (1/10 … 10/10)


class ForkBranchBody(BaseModel):
    message_index: int  # index of the user message we're editing (start new thread from here)
    content: str  # edited text; that branch is trimmed to here, new branch = prefix + this + reply
    mode: str | None = None  # bestie | therapist | learning
    branch_index: int = 0  # which thread we're forking from (0 … branch_count-1)


@app.post("/conversations/{conv_id}/fork_branch")
def fork_branch(conv_id: str, body: ForkBranchBody, current_user: str = Depends(get_current_user)):
    """Start a new in-conversation thread from an edited user message. The current thread is trimmed to that
    message; a new thread is added with the edited message + one assistant reply. Up to MAX_BRANCHES threads
    (default 10). Returns the new branch so frontend can show e.g. 3/10 and continue there."""
    data = _load_store()
    c = _get_conversation_by_id(data, conv_id, current_user)
    if not c:
        raise HTTPException(status_code=404, detail=MOBILE_CONVERSATION_404)
    branches = _get_branches(c)
    bi = max(0, min(body.branch_index, len(branches) - 1)) if branches else 0
    messages = branches[bi]
    idx = body.message_index
    if idx < 0 or idx >= len(messages) or messages[idx].get("role") != "user":
        raise HTTPException(status_code=400, detail="message_index must point to a user message")
    new_content = (body.content or "").strip()
    if not new_content:
        raise HTTPException(status_code=400, detail="content required")
    if len(branches) >= MAX_BRANCHES:
        raise HTTPException(
            status_code=400,
            detail=f"Maximum {MAX_BRANCHES} threads per conversation. Start a new chat to continue.",
        )

    # Trim the forked thread to messages up to and including this user message
    trimmed_branch = list(messages[: idx + 1])
    # New thread: prefix before this message + edited user message + new assistant reply
    prefix = list(messages[:idx])
    history = _flatten_messages_for_history(prefix[-28:])
    history.append({"role": "user", "content": new_content})
    chat_mode = (body.mode or "").strip().lower() or None
    if chat_mode and chat_mode not in ("bestie", "therapist", "learning", "ai_tasks"):
        chat_mode = None
    try:
        reply = _mobile_chat_reply(history, tool_results=None, current_user=current_user, mode=chat_mode)
    except Exception:
        import logging
        logging.getLogger(__name__).exception("fork_branch reply failed")
        reply = "Locus couldn't reply right now — try again."
    now = datetime.now(timezone.utc).isoformat()
    edited_user_msg = {"role": "user", "content": new_content}
    assistant_msg = {"role": "assistant", "content": reply}
    new_branch = prefix + [edited_user_msg, assistant_msg]

    new_branches = branches[:bi] + [trimmed_branch] + branches[bi + 1 :] + [new_branch]
    c["branches"] = new_branches
    c["updated_at"] = now
    c["message_count"] = len(trimmed_branch)
    c["sparkline_data"] = _sparkline_from_messages(trimmed_branch)
    _save_store(data)
    try:
        append_exchange_to_log(
            "mobile",
            current_user,
            new_content,
            reply,
            user_label=get_user_display_name(current_user),
            assistant_label="Locus",
        )
    except Exception:
        pass
    new_index = len(new_branches) - 1
    return {
        "reply": reply,
        "title": c.get("title") or "New chat",
        "updated_at": now,
        "messages": new_branch,
        "branch_count": len(new_branches),
        "branch_index": new_index,
    }


class SelectVariantBody(BaseModel):
    variant_index: int  # 0-based index into variants
    branch_index: int = 0  # which thread (branch) when conversation has multiple


@app.post("/conversations/{conv_id}/messages/{msg_index:int}/select_variant")
def select_variant(
    conv_id: str, msg_index: int, body: SelectVariantBody, current_user: str = Depends(get_current_user)
):
    """Set which variant of an assistant message is selected (for Grok-style 1/2, 2/2 picker)."""
    data = _load_store()
    c = _get_conversation_by_id(data, conv_id, current_user)
    if not c:
        raise HTTPException(status_code=404, detail=MOBILE_CONVERSATION_404)
    branches = _get_branches(c)
    bi = max(0, min(body.branch_index, len(branches) - 1)) if branches else 0
    messages = list(branches[bi] if branches else (c.get("messages") or []))
    if msg_index < 0 or msg_index >= len(messages) or messages[msg_index].get("role") != "assistant":
        raise HTTPException(status_code=400, detail="message_index must point to an assistant message")
    m = messages[msg_index]
    variants = m.get("variants")
    if not isinstance(variants, list) or not variants:
        raise HTTPException(status_code=400, detail="message has no variants")
    vi = body.variant_index
    if vi < 0 or vi >= len(variants):
        raise HTTPException(status_code=400, detail="variant_index out of range")
    content = variants[vi] if isinstance(variants[vi], str) else str(variants[vi])
    messages[msg_index] = dict(m, content=content, selected=vi)
    if branches:
        branches[bi] = messages
        c["branches"] = list(branches)
    else:
        c["messages"] = messages
    c["updated_at"] = datetime.now(timezone.utc).isoformat()
    _save_store(data)
    return {"ok": True, "content": content, "variant_index": vi, "total": len(variants)}


CHAT_FEEDBACK_FILE = STORE_DIR / "chat_feedback.json"


class FeedbackBody(BaseModel):
    message_index: int
    rating: str  # "up" | "down"
    branch_index: int = 0  # which thread/branch (0 = main) for branched convos


def _append_chat_feedback(conv_id: str, message_index: int, rating: str) -> None:
    STORE_DIR.mkdir(parents=True, exist_ok=True)
    entries = []
    if CHAT_FEEDBACK_FILE.exists():
        try:
            with open(CHAT_FEEDBACK_FILE, encoding="utf-8") as f:
                entries = json.load(f)
        except Exception:
            entries = []
    if not isinstance(entries, list):
        entries = []
    entries.append({
        "conv_id": conv_id,
        "message_index": message_index,
        "rating": rating,
        "created_at": datetime.now(timezone.utc).isoformat(),
    })
    with open(CHAT_FEEDBACK_FILE, "w", encoding="utf-8") as f:
        json.dump(entries, f, indent=2)


def _get_conversation_feedback(conv_id: str) -> dict[int, str]:
    """Return { message_index: "up"|"down" } for this conversation (latest rating per message)."""
    out: dict[int, str] = {}
    if not CHAT_FEEDBACK_FILE.exists():
        return out
    try:
        with open(CHAT_FEEDBACK_FILE, encoding="utf-8") as f:
            entries = json.load(f)
    except Exception:
        return out
    if not isinstance(entries, list):
        return out
    for e in entries:
        if e.get("conv_id") != conv_id:
            continue
        idx = e.get("message_index")
        if idx is not None and isinstance(idx, int) and e.get("rating") in ("up", "down"):
            out[idx] = e["rating"]
    return out


def _get_feedback_counts_for_conversation(conv_id: str) -> tuple[int, int]:
    """Return (liked_count, noted_count) for one conversation. Used for activity readouts."""
    fb = _get_conversation_feedback(conv_id)
    liked = sum(1 for r in fb.values() if r == "up")
    noted = sum(1 for r in fb.values() if r == "down")
    return (liked, noted)


def _get_all_feedback_counts() -> dict[str, tuple[int, int]]:
    """Return { conv_id: (liked_count, noted_count) } from chat_feedback.json. One file read for list/readouts."""
    out: dict[str, tuple[int, int]] = {}
    if not CHAT_FEEDBACK_FILE.exists():
        return out
    try:
        with open(CHAT_FEEDBACK_FILE, encoding="utf-8") as f:
            entries = json.load(f)
    except Exception:
        return out
    if not isinstance(entries, list):
        return out
    # Latest rating per (conv_id, message_index)
    by_conv_msg: dict[str, dict[int, str]] = {}
    for e in entries:
        cid = e.get("conv_id")
        if cid is None:
            continue
        cid = str(cid)
        idx = e.get("message_index")
        if idx is not None and isinstance(idx, int) and e.get("rating") in ("up", "down"):
            if cid not in by_conv_msg:
                by_conv_msg[cid] = {}
            by_conv_msg[cid][idx] = e["rating"]
    for cid, msg_ratings in by_conv_msg.items():
        liked = sum(1 for r in msg_ratings.values() if r == "up")
        noted = sum(1 for r in msg_ratings.values() if r == "down")
        out[cid] = (liked, noted)
    return out


@app.get("/conversations/{conv_id}/feedback")
def get_feedback(conv_id: str, current_user: str = Depends(get_current_user)):
    """Return stored thumbs up/down for this conversation (for persistence when re-opening chat)."""
    data = _load_store()
    c = _get_conversation_by_id(data, conv_id, current_user)
    if not c:
        raise HTTPException(status_code=404, detail=MOBILE_CONVERSATION_404)
    fb = _get_conversation_feedback(conv_id)
    return {"feedback": fb}


@app.post("/conversations/{conv_id}/feedback")
def post_feedback(conv_id: str, body: FeedbackBody, current_user: str = Depends(get_current_user)):
    """Record thumbs up/down on an assistant message. Persisted in chat_feedback.json for real (mobile) chats;
    counts are used in conversation list and Activity Breakdown readouts (Liked / Noted buckets)."""
    if body.rating not in ("up", "down"):
        raise HTTPException(status_code=400, detail="rating must be 'up' or 'down'")
    data = _load_store()
    c = _get_conversation_by_id(data, conv_id, current_user)
    if not c:
        raise HTTPException(status_code=404, detail=MOBILE_CONVERSATION_404)
    branches = _get_branches(c)
    bi = max(0, min(body.branch_index, len(branches) - 1)) if branches else 0
    messages = branches[bi] if branches else (c.get("messages") or [])
    if body.message_index < 0 or body.message_index >= len(messages):
        raise HTTPException(status_code=400, detail="message_index out of range")
    if messages[body.message_index].get("role") != "assistant":
        raise HTTPException(status_code=400, detail="can only rate assistant messages")
    _append_chat_feedback(conv_id, body.message_index, body.rating)
    return {"ok": True, "rating": body.rating}


class PatchConversationBody(BaseModel):
    pinned: bool | None = None
    title: str | None = None
    important: bool | None = None


@app.patch("/conversations/{conv_id}")
def patch_conversation(conv_id: str, body: PatchConversationBody, current_user: str = Depends(get_current_user)):
    """Update a conversation's pinned status, title, or important flag."""
    data = _load_store()
    c = _get_conversation_by_id(data, conv_id, current_user)
    if not c:
        raise HTTPException(status_code=404, detail=MOBILE_CONVERSATION_404)
    if body.pinned is not None:
        c["pinned"] = body.pinned
        if body.pinned is False:
            c["updated_at"] = datetime.now(timezone.utc).isoformat()
    if body.title is not None:
        new_title = body.title[:200]
        old_title = c.get("title") or "Chat"
        if old_title != new_title:
            c.setdefault("title_history", []).append({
                "from": old_title,
                "to": new_title,
                "at": datetime.now(timezone.utc).isoformat(),
            })
        c["title"] = new_title
    if body.important is not None:
        c["important"] = body.important
    _save_store(data)
    return _conversation_summary(c)


@app.delete("/conversations/{conv_id}")
def delete_conversation(conv_id: str, current_user: str = Depends(get_current_user)):
    """Soft-delete: moves conversation to archive, never permanently removed."""
    data = _load_store()
    conv = _get_conversation_by_id(data, conv_id, current_user)
    if not conv:
        raise HTTPException(status_code=404, detail=MOBILE_CONVERSATION_404)
    data["conversations"] = [c for c in data["conversations"] if c.get("id") != conv_id]
    conv["archived_at"] = datetime.now(timezone.utc).isoformat()
    conv["pinned"] = False
    data.setdefault("archive", []).insert(0, conv)
    _save_store(data)
    return {"archived": True}


@app.post("/conversations/{conv_id}/restore")
def restore_conversation(conv_id: str, current_user: str = Depends(get_current_user)):
    """Restore a conversation from archive back to the active list."""
    data = _load_store()
    conv = next(
        (c for c in data.get("archive", []) if c.get("id") == conv_id and (c.get("owner") or DEFAULT_USER) == current_user),
        None,
    )
    if not conv:
        raise HTTPException(status_code=404, detail="archived conversation not found")
    data["archive"] = [c for c in data["archive"] if c.get("id") != conv_id]
    conv.pop("archived_at", None)
    data.setdefault("conversations", []).insert(0, conv)
    _save_store(data)
    return _conversation_summary(conv)


class RecordEngagementBody(BaseModel):
    source: str = "mobile"  # mobile | grok | continue | cursor
    id: str = ""  # conversation id


class MarkImportantBody(BaseModel):
    source: str = "mobile"
    id: str = ""
    important: bool = True


@app.post("/conversations/engagement/record")
def record_engagement(body: RecordEngagementBody, current_user: str = Depends(get_current_user)):
    """Record that a conversation was opened. Returns suggest_important when you've returned to it a few times (for 'Mark as important?' prompt)."""
    conv_id = (body.id or "").strip()
    source = (body.source or "mobile").strip().lower() or "mobile"
    if not conv_id:
        raise HTTPException(status_code=400, detail="id required")
    if source == "mobile" and not _get_conversation_by_id(_load_store(), conv_id, current_user):
        raise HTTPException(status_code=404, detail=MOBILE_CONVERSATION_404)
    eng = _load_engagement()
    key = _engagement_key(source, conv_id)
    by_key = eng.setdefault("by_key", {})
    entry = by_key.setdefault(key, {"open_count": 0, "last_opened_at": None, "important": False})
    entry["open_count"] = (entry.get("open_count") or 0) + 1
    entry["last_opened_at"] = datetime.now(timezone.utc).isoformat()
    by_key[key] = entry
    _save_engagement(eng)
    important = bool(entry.get("important"))
    open_count = entry["open_count"]
    suggest_important = open_count >= RETURN_THRESHOLD and not important
    return {"open_count": open_count, "important": important, "suggest_important": suggest_important}


@app.post("/conversations/engagement/mark_important")
def mark_important(body: MarkImportantBody, current_user: str = Depends(get_current_user)):
    """Mark a conversation as important (so it can be surfaced / used for Wrapped-style stats). For mobile, also updates the conversation in the main store."""
    conv_id = str((body.id or "").strip())
    source = (body.source or "mobile").strip().lower() or "mobile"
    if not conv_id:
        raise HTTPException(status_code=400, detail="id required")
    if source == "mobile":
        data = _load_store()
        c = _get_conversation_by_id(data, conv_id, current_user)
        if not c:
            raise HTTPException(status_code=404, detail=MOBILE_CONVERSATION_404)
    eng = _load_engagement()
    key = _engagement_key(source, conv_id)
    by_key = eng.setdefault("by_key", {})
    entry = by_key.setdefault(key, {"open_count": 0, "last_opened_at": None, "important": False})
    entry["important"] = body.important
    by_key[key] = entry
    _save_engagement(eng)
    if source == "mobile":
        data = _load_store()
        c = _get_conversation_by_id(data, conv_id, current_user)
        if c:
                c["important"] = body.important
                _save_store(data)
                return {"important": body.important, "updated": True}
    return {"important": body.important, "updated": True}


# --- Avatar / default characters (pick your look; cute girly presets + optional custom URL) ---


# Local avatar assets (assets/girl, assets/cute) for PWA avatar picker
AVATAR_LOCAL_FOLDERS = ("girl", "cute")
AVATAR_IMAGE_EXTS = (".svg", ".png", ".jpg", ".jpeg", ".webp")


def _svg_appears_black_only(path: Path) -> bool:
    """True if the SVG uses only black/dark fills (would be invisible on dark theme). Skip these in the picker."""
    try:
        raw = path.read_text(encoding="utf-8", errors="replace")[:8192]
    except OSError:
        return True  # skip on read error
    raw_lower = raw.lower()
    # Black / very dark fills that disappear on dark theme (include rgb(0,0,0), #111, #222, etc.)
    black_patterns = (
        "fill:#000", "fill:#000000", 'fill="#000', 'fill="#000000', "fill:black", 'fill="black"',
        "fill:rgb(0,0,0)", "fill:rgb(0, 0, 0)", "fill:#111", "fill:#222",
        "fill:#0a0a0a", "fill:#1a1a1a", "stroke:#000", "stroke:black",
    )
    has_black_or_dark = any(p in raw_lower for p in black_patterns)
    # Check for any clearly non-black fill (hex with meaningful color)
    for m in re.finditer(r"fill[\s:=]+[\"']?#?([0-9a-f]{3,8})[\"']?", raw_lower):
        hex_part = (m.group(1) or "").lstrip("0") or "0"
        if len(hex_part) >= 2:
            # Allow only if it's not black/dark: 000, 111, 222, 0a0a0a, etc.
            if hex_part not in ("0", "00", "000", "111", "222", "333", "0a0a0a", "1a1a1a", "0a0", "1a1", "2a2"):
                return False  # has a visible color
    for m in re.finditer(r"fill=[\"']#?([0-9a-f]{3,8})[\"']", raw_lower):
        hex_part = (m.group(1) or "").lstrip("0") or "0"
        if len(hex_part) >= 2 and hex_part not in ("0", "00", "000", "111", "222", "333", "0a0a0a", "1a1a1a", "0a0", "1a1", "2a2"):
            return False
    if re.search(r"rgb\s*\(\s*(\d+)\s*,\s*(\d+)\s*,\s*(\d+)\s*\)", raw_lower):
        for m in re.finditer(r"fill[\s:=]+[\"']?rgb\s*\(\s*(\d+)\s*,\s*(\d+)\s*,\s*(\d+)\s*\)", raw_lower):
            r, g, b = int(m.group(1)), int(m.group(2)), int(m.group(3))
            if r > 40 or g > 40 or b > 40:
                return False  # has a visible color
    if not has_black_or_dark:
        return False
    return True  # only black/dark found, skip


def _list_local_avatar_assets() -> list[dict]:
    """List avatar images from assets/girl and assets/cute for avatar picker. Returns [{id, name, avatarUrl}]. Skips SVGs that are black-only (invisible on dark theme)."""
    out: list[dict] = []
    assets_dir = ASSETS_ROOT / "assets"
    for folder in AVATAR_LOCAL_FOLDERS:
        sub = assets_dir / folder
        if not sub.is_dir():
            continue
        for f in sorted(sub.iterdir()):
            if not f.is_file() or f.suffix.lower() not in AVATAR_IMAGE_EXTS:
                continue
            if f.suffix.lower() == ".svg" and _svg_appears_black_only(f):
                continue  # skip black-only SVGs so they don't appear as blank circles
            name = f.stem.replace("-", " ").replace("_", " ").title()
            aid = f"{folder}-{f.stem}"[:64]
            url = f"/api/avatar/asset/{folder}/{f.name}"
            out.append({"id": aid, "name": name, "avatarUrl": url})
    return out


@app.get("/api/avatar/local")
def list_local_avatar_assets():
    """List avatar images from assets/girl and assets/cute (cute girl avatars etc.) for PWA picker."""
    return {"characters": _list_local_avatar_assets()}


@app.get("/api/avatar/asset/{folder}/{filename:path}", response_class=Response)
def serve_avatar_asset(folder: str, filename: str):
    """Serve a single image from assets/girl or assets/cute for avatar display."""
    if folder not in AVATAR_LOCAL_FOLDERS or ".." in filename or "\\" in filename:
        raise HTTPException(status_code=400, detail="invalid path")
    path = (ASSETS_ROOT / "assets" / folder / filename).resolve()
    base = (ASSETS_ROOT / "assets" / folder).resolve()
    if not path.is_file() or not str(path).startswith(str(base)):
        raise HTTPException(status_code=404, detail="not found")
    media = "image/svg+xml" if path.suffix.lower() == ".svg" else "image/png"
    if path.suffix.lower() in (".jpg", ".jpeg"):
        media = "image/jpeg"
    if path.suffix.lower() == ".webp":
        media = "image/webp"
    return Response(content=path.read_bytes(), media_type=media)


@app.get("/api/asset/bucket_tree_flowers/{filename:path}", response_class=Response)
def serve_bucket_tree_flower(filename: str):
    """Serve branch/flower SVGs from assets/bucket tree flowers for activity breakdown nodes."""
    if ".." in filename or "\\" in filename:
        raise HTTPException(status_code=400, detail="invalid path")
    path = (BUCKET_TREE_FLOWERS_DIR / filename).resolve()
    try:
        path.relative_to(BUCKET_TREE_FLOWERS_DIR.resolve())
    except ValueError:
        raise HTTPException(status_code=404, detail="not found")
    if not path.is_file():
        raise HTTPException(status_code=404, detail="not found")
    media = "image/svg+xml" if path.suffix.lower() == ".svg" else "image/png"
    if path.suffix.lower() in (".jpg", ".jpeg"):
        media = "image/jpeg"
    return Response(content=path.read_bytes(), media_type=media)


@app.get("/api/asset/bomb/{filename:path}", response_class=Response)
def serve_asset_bomb(filename: str):
    """Serve SVGs from assets/bomb (e.g. bomb icon for sidebar rename/title action)."""
    if ".." in filename or "\\" in filename:
        raise HTTPException(status_code=400, detail="invalid path")
    path = (BOMB_DIR / filename).resolve()
    try:
        path.relative_to(BOMB_DIR.resolve())
    except ValueError:
        raise HTTPException(status_code=404, detail="not found")
    if not path.is_file():
        raise HTTPException(status_code=404, detail="not found")
    media = "image/svg+xml" if path.suffix.lower() == ".svg" else "image/png"
    if path.suffix.lower() in (".jpg", ".jpeg"):
        media = "image/jpeg"
    return Response(content=path.read_bytes(), media_type=media)


@app.get("/api/asset/file/{filename:path}", response_class=Response)
def serve_asset_file(filename: str):
    """Serve SVGs from assets/file (e.g. four-round-point-connection for sidebar actions)."""
    if ".." in filename or "\\" in filename:
        raise HTTPException(status_code=400, detail="invalid path")
    path = (ASSETS_FILE_DIR / filename).resolve()
    try:
        path.relative_to(ASSETS_FILE_DIR.resolve())
    except ValueError:
        raise HTTPException(status_code=404, detail="not found")
    if not path.is_file():
        raise HTTPException(status_code=404, detail="not found")
    media = "image/svg+xml" if path.suffix.lower() == ".svg" else "image/png"
    if path.suffix.lower() in (".jpg", ".jpeg"):
        media = "image/jpeg"
    return Response(content=path.read_bytes(), media_type=media)


@app.get("/api/asset/pencil/{filename:path}", response_class=Response)
def serve_asset_pencil(filename: str):
    """Serve SVGs from assets/pencil (edit / send again icon)."""
    if ".." in filename or "\\" in filename:
        raise HTTPException(status_code=400, detail="invalid path")
    path = (ASSETS_PENCIL_DIR / filename).resolve()
    try:
        path.relative_to(ASSETS_PENCIL_DIR.resolve())
    except ValueError:
        raise HTTPException(status_code=404, detail="not found")
    if not path.is_file():
        raise HTTPException(status_code=404, detail="not found")
    media = "image/svg+xml" if path.suffix.lower() == ".svg" else "image/png"
    if path.suffix.lower() in (".jpg", ".jpeg"):
        media = "image/jpeg"
    return Response(content=path.read_bytes(), media_type=media)


@app.get("/api/asset/horns/{filename:path}", response_class=Response)
def serve_asset_horns(filename: str):
    """Serve SVGs from assets/ex out crossed out (cancel / stay loose icon)."""
    if ".." in filename or "\\" in filename:
        raise HTTPException(status_code=400, detail="invalid path")
    path = (ASSETS_HORNS_DIR / filename).resolve()
    try:
        path.relative_to(ASSETS_HORNS_DIR.resolve())
    except ValueError:
        raise HTTPException(status_code=404, detail="not found")
    if not path.is_file():
        raise HTTPException(status_code=404, detail="not found")
    media = "image/svg+xml" if path.suffix.lower() == ".svg" else "image/png"
    if path.suffix.lower() in (".jpg", ".jpeg"):
        media = "image/jpeg"
    return Response(content=path.read_bytes(), media_type=media)


@app.get("/api/avatar/characters")
def list_avatar_characters():
    """List avatar characters for the PWA picker (Ruby canon + local girlie assets + DiceBear presets)."""
    # Order matters: Ruby's canon avatar first, then all local "girlie" assets, then online presets.
    return {"characters": [RUBY_HAHLI_CHARACTER, *_list_local_avatar_assets(), *list(DEFAULT_CHARACTERS)]}


class AvatarMeBody(BaseModel):
    character_id: str | None = None  # id from /api/avatar/characters
    custom_url: str | None = None  # optional; if set, overrides character for display


@app.get("/api/avatar/me")
def get_my_avatar(current_user: str = Depends(get_current_user)):
    """Current user's avatar: selected character or custom URL. Returns avatarUrl for the PWA to show in chat."""
    data = _load_avatar_store()
    entry = data.get(current_user) or {}
    custom = (entry.get("customUrl") or "").strip() or None
    if custom:
        return {"avatarUrl": custom, "characterId": None, "customUrl": custom}
    cid = entry.get("characterId") or ""
    if cid == RUBY_HAHLI_CHARACTER["id"]:
        return {"avatarUrl": RUBY_HAHLI_CHARACTER["avatarUrl"], "characterId": cid, "customUrl": None}
    # Match against local assets and presets
    for c in _list_local_avatar_assets():
        if c["id"] == cid:
            return {"avatarUrl": c["avatarUrl"], "characterId": cid, "customUrl": None}
    for c in DEFAULT_CHARACTERS:
        if c["id"] == cid:
            return {"avatarUrl": c["avatarUrl"], "characterId": cid, "customUrl": None}
    # Default: Ruby canon avatar
    return {
        "avatarUrl": RUBY_HAHLI_CHARACTER["avatarUrl"],
        "characterId": RUBY_HAHLI_CHARACTER["id"],
        "customUrl": None,
    }


@app.post("/api/avatar/me")
def set_my_avatar(body: AvatarMeBody, current_user: str = Depends(get_current_user)):
    """Set current user's avatar: pick a default character and/or set a custom image URL (e.g. from an SVG site)."""
    data = _load_avatar_store()
    data.setdefault(current_user, {})
    if body.character_id is not None:
        data[current_user]["characterId"] = body.character_id.strip() or RUBY_HAHLI_CHARACTER["id"]
        data[current_user]["customUrl"] = None  # clear custom URL so character takes effect
    if body.custom_url is not None:
        data[current_user]["customUrl"] = (body.custom_url.strip() or None)
    _save_avatar_store(data)
    # Return same shape as GET /api/avatar/me
    entry = data.get(current_user) or {}
    custom = (entry.get("customUrl") or "").strip() or None
    if custom:
        return {"avatarUrl": custom, "characterId": None, "customUrl": custom}
    cid = entry.get("characterId") or ""
    if cid == RUBY_HAHLI_CHARACTER["id"]:
        return {"avatarUrl": RUBY_HAHLI_CHARACTER["avatarUrl"], "characterId": cid, "customUrl": None}
    for c in _list_local_avatar_assets():
        if c["id"] == cid:
            return {"avatarUrl": c["avatarUrl"], "characterId": cid, "customUrl": None}
    for c in DEFAULT_CHARACTERS:
        if c["id"] == cid:
            return {"avatarUrl": c["avatarUrl"], "characterId": cid, "customUrl": None}
    return {
        "avatarUrl": RUBY_HAHLI_CHARACTER["avatarUrl"],
        "characterId": RUBY_HAHLI_CHARACTER["id"],
        "customUrl": None,
    }


# --- User profile: pronouns + optional about_me (for Lynn/Raven; injected into system prompt) ---


@app.get("/api/user/profile")
def get_user_profile(current_user: str = Depends(get_current_user)):
    """Current user's pronouns and about_me (for PWA sidebar and prompt injection)."""
    profile = _get_user_profile(current_user)
    return {"pronouns": profile["pronouns"], "about_me": profile["about_me"]}


class UserProfileBody(BaseModel):
    pronouns: str | None = None
    about_me: str | None = None


@app.patch("/api/user/profile")
def update_user_profile(body: UserProfileBody, current_user: str = Depends(get_current_user)):
    """Update current user's pronouns and/or about_me (optional fields)."""
    data = _load_user_profiles()
    data.setdefault(current_user, {})
    if body.pronouns is not None:
        data[current_user]["pronouns"] = (body.pronouns or "").strip()[:80]
    if body.about_me is not None:
        data[current_user]["about_me"] = (body.about_me or "").strip()[:500]
    _save_user_profiles(data)
    profile = _get_user_profile(current_user)
    return {"pronouns": profile["pronouns"], "about_me": profile["about_me"]}




# --- Creative draft documents (tweets, lyrics) — inline in chat, versioned ---

CREATIVE_DRAFTS_DIR = PROJECT_ROOT / "Journal_Database" / "Creative_Drafts"
CREATIVE_DRAFTS_VERSIONS_DIR = CREATIVE_DRAFTS_DIR / "versions"


def _slug_from_title(title: str, doc_type: str) -> str:
    """Safe filename stem from title + type (e.g. 'Tweet draft' -> 'Tweet_draft')."""
    import re
    s = (title or doc_type or "draft").strip()[:60]
    s = re.sub(r"[^\w\s-]", "", s)
    s = re.sub(r"[-\s]+", "_", s).strip("_") or "draft"
    return s


def _is_creative_reply(user_message: str, reply: str) -> bool:
    """Heuristic: user asked for tweet/lyrics and reply looks like a draft (not an error)."""
    if not reply or len(reply.strip()) < 10:
        return False
    low = (user_message or "").lower()
    if "couldn't reply" in reply or "try again" in reply or "Error" in reply:
        return False
    if "tweet" in low or "write me a tweet" in low:
        return len(reply) <= 4000
    if "lyric" in low or "song lyric" in low or "write me a song" in low or "help me write" in low:
        return len(reply) <= 15000
    return False


def _draft_type_from_message(user_message: str) -> str:
    low = (user_message or "").lower()
    if "tweet" in low:
        return "tweet"
    if "lyric" in low or "song" in low:
        return "lyrics"
    return "draft"


def _create_draft_document(
    title: str, content: str, doc_type: str, conversation_id: str
) -> dict | None:
    """Save draft to Journal_Database/Creative_Drafts; return {path, title, type, content} for the message."""
    CREATIVE_DRAFTS_DIR.mkdir(parents=True, exist_ok=True)
    CREATIVE_DRAFTS_VERSIONS_DIR.mkdir(parents=True, exist_ok=True)
    slug = _slug_from_title(title, doc_type)
    stem = f"{slug}_{doc_type}"
    base_name = f"{stem}.md"
    rel_path = f"Journal_Database/Creative_Drafts/{base_name}"
    full = PROJECT_ROOT / rel_path
    if full.exists():
        from datetime import datetime, timezone
        backup = CREATIVE_DRAFTS_VERSIONS_DIR / f"{stem}_{datetime.now(timezone.utc).strftime('%Y%m%d_%H%M%S')}.md"
        try:
            backup.write_text(full.read_text(encoding="utf-8", errors="replace"), encoding="utf-8")
        except OSError:
            pass
    try:
        full.parent.mkdir(parents=True, exist_ok=True)
        full.write_text(content, encoding="utf-8")
    except OSError:
        return None
    return {"path": rel_path, "title": title, "type": doc_type, "content": content}


def _backup_draft_before_write(rel_path: str) -> None:
    """If path is under Creative_Drafts (and not versions/), copy current file to versions/ before overwrite."""
    if "Creative_Drafts/versions/" in rel_path or not rel_path.startswith("Journal_Database/Creative_Drafts/"):
        return
    full = PROJECT_ROOT / rel_path
    if not full.is_file():
        return
    from datetime import datetime, timezone
    stem = full.stem
    backup_name = f"{stem}_{datetime.now(timezone.utc).strftime('%Y%m%d_%H%M%S')}.md"
    backup = CREATIVE_DRAFTS_VERSIONS_DIR / backup_name
    try:
        backup.write_text(full.read_text(encoding="utf-8", errors="replace"), encoding="utf-8")
    except OSError:
        pass


# --- Project files (browse / view / edit from phone, like Grok doc side panel) ---
# Ruby: full project root. Lynn/Raven: isolated folder each (Journal_Database/User_Files/<user>/).
USER_FILES_BASE = PROJECT_ROOT / "Journal_Database" / "User_Files"


def _files_root_for_user(user_id: str) -> Path:
    """Root for file list/read/write: Ruby = project root; Lynn/Raven = their own folder (created if missing)."""
    if not user_id or user_id == DEFAULT_USER:
        return PROJECT_ROOT
    root = USER_FILES_BASE / user_id
    root.mkdir(parents=True, exist_ok=True)
    return root


_FILES_EXCLUDED_DIRS = frozenset({
    ".git", "__pycache__", "node_modules", ".data", ".cursor", ".continue",
    "venv", ".venv", "env", ".env", ".cursorignore", "Bestie", "AI",
})
_FILES_READ_EXTENSIONS = frozenset({
    ".md", ".txt", ".py", ".json", ".yaml", ".yml", ".html", ".js", ".css",
    ".ts", ".tsx", ".jsx", ".sh", ".bat", ".csv", ".xml", ".toml", ".ini",
    ".cfg", ".rst", ".mdc", ".mdx",
    # Creative: vector art, music notation, lyrics, LaTeX
    ".svg", ".ly", ".abc", ".tex", ".lrc",
})
_FILES_WRITE_EXTENSIONS = _FILES_READ_EXTENSIONS  # same for now; exclude .log if desired

# Viewable in browser (images + PDF): serve raw bytes with correct Content-Type
_FILES_VIEWABLE_EXTENSIONS = frozenset({
    ".png", ".jpg", ".jpeg", ".gif", ".webp", ".bmp", ".ico",
    ".svg",  # display as image (also in READ as text)
})
_FILES_PDF_EXTENSIONS = frozenset({".pdf"})
_FILES_SERVE_MEDIA = {
    ".png": "image/png", ".jpg": "image/jpeg", ".jpeg": "image/jpeg",
    ".gif": "image/gif", ".webp": "image/webp", ".bmp": "image/bmp",
    ".ico": "image/x-icon", ".svg": "image/svg+xml", ".pdf": "application/pdf",
}


def _resolve_project_path(relative_path: str, must_exist: bool = False, files_root: Path | None = None) -> Path | None:
    """Resolve a relative path under files_root (default PROJECT_ROOT). Returns None if invalid (escape, excluded dir when Ruby)."""
    if relative_path is not None and ".." in relative_path:
        return None
    rel = (relative_path or "").replace("\\", "/").strip("/")
    if files_root is not None:
        base = files_root.resolve()
    elif rel == "assets" or rel.startswith("assets/"):
        base = ASSETS_ROOT.resolve()
    else:
        base = PROJECT_ROOT.resolve()
    if not rel:
        root = base
    else:
        root = (base / rel).resolve()
    try:
        if not str(root).startswith(str(base)):
            return None
    except (OSError, ValueError):
        return None
    # When using full project root (Ruby), reject excluded dirs in path
    if base == PROJECT_ROOT.resolve():
        for part in Path(rel).parts if rel else []:
            if part in _FILES_EXCLUDED_DIRS:
                return None
    if must_exist and not root.exists():
        return None
    return root


def _allowed_read(path: Path) -> bool:
    if not path.is_file():
        return False
    return path.suffix.lower() in _FILES_READ_EXTENSIONS


def _allowed_write(path: Path) -> bool:
    return path.suffix.lower() in _FILES_WRITE_EXTENSIONS


def _allowed_serve(path: Path) -> bool:
    """True if file can be served as binary (image/PDF) for viewing."""
    if not path.is_file():
        return False
    ext = path.suffix.lower()
    return ext in _FILES_VIEWABLE_EXTENSIONS or ext in _FILES_PDF_EXTENSIONS


def _file_list_matches_query(name: str, rel_path: str, q: str, ext_filter: str) -> bool:
    """True if entry matches search query. ext_filter is set when q is extension-like (.svg, *.svg, svg)."""
    name_l = (name or "").lower()
    path_l = (rel_path or "").lower()
    if ext_filter:
        return name_l.endswith(ext_filter) or name_l == ext_filter.lstrip(".")
    return q in name_l or q in path_l


@app.get("/api/files/list")
def api_files_list(path: str = "", q: str = None, current_user: str = Depends(get_current_user)):
    """List directory entries. Optional q=search: .svg / *.svg / svg = by extension (recursive); any other text = substring in name/path (recursive)."""
    files_root = _files_root_for_user(current_user)
    root = _resolve_project_path(path, must_exist=False, files_root=files_root)
    if root is None:
        raise HTTPException(status_code=400, detail="invalid or disallowed path")
    if not root.exists():
        raise HTTPException(status_code=404, detail="path not found")
    if not root.is_dir():
        raise HTTPException(status_code=400, detail="not a directory")
    entries = []
    prefix = path.replace("\\", "/").strip("/")
    query = (q or "").strip()
    ext_filter = ""
    if query:
        q_l = query.lower()
        if q_l.startswith("*."):
            ext_filter = q_l[1:]  # *.svg -> .svg
        elif q_l.startswith("."):
            ext_filter = q_l
        elif "." not in q_l and q_l.replace("-", "").replace("_", "").isalnum():
            ext_filter = "." + q_l  # svg -> .svg
    try:
        if not query:
            # Single-level list (current behaviour)
            for p in sorted(root.iterdir(), key=lambda x: (not x.is_dir(), x.name.lower())):
                if p.name.startswith(".") and p.name not in (".cursor", ".cursorignore"):
                    continue
                if p.is_dir() and p.name in _FILES_EXCLUDED_DIRS:
                    continue
                rel = f"{prefix}/{p.name}" if prefix else p.name
                entries.append({
                    "name": p.name,
                    "path": rel,
                    "type": "dir" if p.is_dir() else "file",
                })
        else:
            # Recursive search (Explorer-style: .svg, *.svg, or substring)
            base = files_root.resolve()
            max_entries = 500
            max_depth = 25
            q_l = query.lower()

            def walk(dir_path: Path, depth: int) -> None:
                if len(entries) >= max_entries or depth > max_depth:
                    return
                try:
                    for p in sorted(dir_path.iterdir(), key=lambda x: (not x.is_dir(), x.name.lower())):
                        if len(entries) >= max_entries:
                            return
                        if p.name.startswith(".") and p.name not in (".cursor", ".cursorignore"):
                            continue
                        if p.is_dir():
                            if p.name in _FILES_EXCLUDED_DIRS:
                                continue
                            try:
                                rel_s = p.relative_to(base).as_posix()
                            except ValueError:
                                continue
                            if _file_list_matches_query(p.name, rel_s, q_l, ext_filter):
                                entries.append({"name": p.name, "path": rel_s, "type": "dir"})
                            walk(p, depth + 1)
                        else:
                            try:
                                rel_s = p.relative_to(base).as_posix()
                            except ValueError:
                                continue
                            if _file_list_matches_query(p.name, rel_s, q_l, ext_filter):
                                entries.append({"name": p.name, "path": rel_s, "type": "file"})
                except OSError:
                    pass

            walk(root, 0)
    except OSError:
        raise HTTPException(status_code=403, detail="cannot list directory")
    return {"path": path or "/", "entries": entries}


@app.get("/api/files/read")
def api_files_read(path: str, current_user: str = Depends(get_current_user)):
    """Read file content as text. path = relative to user's file root (project for Ruby, User_Files/<user>/ for Lynn/Raven)."""
    if not path:
        raise HTTPException(status_code=400, detail="path required")
    files_root = _files_root_for_user(current_user)
    root = _resolve_project_path(path, must_exist=True, files_root=files_root)
    if root is None:
        raise HTTPException(status_code=400, detail="invalid or disallowed path")
    if not root.is_file():
        raise HTTPException(status_code=400, detail="not a file")
    if not _allowed_read(root):
        raise HTTPException(status_code=400, detail="file type not allowed for read")
    try:
        content = root.read_text(encoding="utf-8", errors="replace")
    except OSError:
        raise HTTPException(status_code=403, detail="cannot read file")
    return {"path": path, "content": content}


@app.get("/api/files/serve")
def api_files_serve(path: str, current_user: str = Depends(get_current_user)):
    """Serve a file as binary for viewing (images, PDF). Returns raw bytes with correct Content-Type."""
    if not path:
        raise HTTPException(status_code=400, detail="path required")
    files_root = _files_root_for_user(current_user)
    root = _resolve_project_path(path, must_exist=True, files_root=files_root)
    if root is None:
        raise HTTPException(status_code=400, detail="invalid or disallowed path")
    if not root.is_file():
        raise HTTPException(status_code=400, detail="not a file")
    if not _allowed_serve(root):
        raise HTTPException(status_code=400, detail="file type not allowed for viewing")
    ext = root.suffix.lower()
    media_type = _FILES_SERVE_MEDIA.get(ext, "application/octet-stream")
    try:
        return FileResponse(str(root), media_type=media_type)
    except OSError:
        raise HTTPException(status_code=403, detail="cannot read file")


class FilesWriteBody(BaseModel):
    path: str = ""
    content: str = ""


@app.post("/api/files/write")
def api_files_write(body: FilesWriteBody, current_user: str = Depends(get_current_user)):
    """Write file content. path = relative to user's file root (project for Ruby, User_Files/<user>/ for Lynn/Raven). Validates extension."""
    if not body.path:
        raise HTTPException(status_code=400, detail="path required")
    files_root = _files_root_for_user(current_user)
    root = _resolve_project_path(body.path, must_exist=False, files_root=files_root)
    if root is None:
        raise HTTPException(status_code=400, detail="invalid or disallowed path")
    if not _allowed_write(root):
        raise HTTPException(status_code=400, detail="file type not allowed for write")
    _backup_draft_before_write(body.path)
    root.parent.mkdir(parents=True, exist_ok=True)
    tmp = root.with_suffix(root.suffix + ".tmp")
    try:
        tmp.write_text(body.content, encoding="utf-8")
        tmp.replace(root)
    except OSError as e:
        raise HTTPException(status_code=500, detail=f"write failed: {e}")
    return {"path": body.path, "ok": True}


class ConvertSvgBody(BaseModel):
    """Path to an SVG file (project-relative, e.g. assets/girl/icon.svg)."""
    path: str = ""


@app.post("/api/convert/svg-to-png")
def api_convert_svg_to_png(body: ConvertSvgBody, current_user: str = Depends(get_current_user)):
    """Convert an SVG to PNG using Inkscape (full render: vector + bitmap layers). Path must be under project root. Returns output path (project-relative)."""
    if not body.path or not body.path.strip():
        raise HTTPException(status_code=400, detail="path required")
    root = _resolve_project_path(body.path.strip(), must_exist=True)
    if root is None or root.suffix.lower() != ".svg":
        raise HTTPException(status_code=400, detail="invalid path or not an SVG file")
    try:
        from inkscape_convert import convert_svg_to_png
        out = convert_svg_to_png(root)
    except Exception as e:
        raise HTTPException(status_code=500, detail=f"convert failed: {e}")
    if not out:
        raise HTTPException(status_code=500, detail="Inkscape not found or export failed")
    try:
        rel = out.relative_to(PROJECT_ROOT).as_posix()
    except ValueError:
        try:
            rel = out.relative_to(ASSETS_ROOT).as_posix()
        except ValueError:
            rel = str(out)
    return {"output_path": rel, "ok": True}


class PlanAppendBody(BaseModel):
    """Add one line to project plan Recent Updates or to webapp backlog. Used by webapp 'Add to plan' / 'Add to backlog'."""
    target: str = "backlog"  # "project_plan_recent" | "backlog"
    text: str = ""
    section: str = ""  # optional; for backlog, e.g. "Plan Mode & planning in the webapp"


@app.post("/plan/append")
def api_plan_append(body: PlanAppendBody, current_user: str = Depends(get_current_user)):
    """Append one line to PROJECT_PLAN.md Recent Updates or to LOCUS_BACKLOG. From webapp or any client. target: 'project_plan_recent' or 'backlog'; text: the line to add; section: optional backlog section heading."""
    text = (body.text or "").strip()
    if not text:
        raise HTTPException(status_code=400, detail="text required")
    target = (body.target or "backlog").strip().lower()
    section = (body.section or "").strip()
    try:
        if target == "project_plan_recent":
            msg = _add_to_project_plan_recent(text)
        else:
            msg = _add_to_backlog(text, section_heading=section)
    except Exception as e:
        raise HTTPException(status_code=500, detail=str(e))
    if msg.startswith("Error"):
        raise HTTPException(status_code=400, detail=msg)
    return {"ok": True, "message": msg}


# --- Continue (VS Code) sessions — read-only for phone ---

WORKSPACE_FILTER_CANDIDATES = (
    "Locus",
    "Claudia-Core",
    "Claudia-Core-code-workspace",
)


def _continue_sessions_dir():
    """Continue sessions dir; override with CONTINUE_SESSIONS_DIR if Continue moved or you use a different path."""
    custom = os.environ.get("CONTINUE_SESSIONS_DIR", "").strip()
    if custom:
        return Path(custom)
    return Path(os.environ.get("USERPROFILE", os.path.expanduser("~"))) / ".continue" / "sessions"


def _extract_continue_message_text(msg):
    """Plain text from a Continue history message content (can be list of parts or string)."""
    content = msg.get("content") or []
    if isinstance(content, str):
        return content.strip()
    parts = []
    for part in content:
        if isinstance(part, dict):
            if part.get("type") == "text":
                parts.append(part.get("text") or "")
            elif "text" in part:
                parts.append(part["text"] or "")
        elif isinstance(part, str):
            parts.append(part)
    return "\n".join(parts).strip()


def _normalize_workspace_for_filter(workspace: str) -> str:
    """Decode URI-style path so 'Locus' matches file:///.../Claudia%20Core."""
    if not workspace:
        return ""
    try:
        from urllib.parse import unquote
        return unquote(workspace).lower()
    except Exception:
        return workspace.lower()


def _workspace_matches_project(workspace: str) -> bool:
    """Robust workspace matcher across spaced/hyphenated path variants."""
    if not workspace:
        return False
    import re
    ws_norm = _normalize_workspace_for_filter(workspace)
    ws_compact = re.sub(r"[^a-z0-9]+", " ", ws_norm).strip()
    for raw in WORKSPACE_FILTER_CANDIDATES:
        f = (raw or "").strip().lower()
        if not f:
            continue
        if f in ws_norm:
            return True
        f_compact = re.sub(r"[^a-z0-9]+", " ", f).strip()
        if f_compact and f_compact in ws_compact:
            return True
    return False


def _list_continue_sessions():
    """List Continue sessions for our workspace. Returns list of { id, title, created_at, updated_at, source }."""
    sessions_dir = _continue_sessions_dir()
    index_file = sessions_dir / "sessions.json"
    if not index_file.exists():
        return []
    try:
        raw = index_file.read_text(encoding="utf-8")
        entries = json.loads(raw)
    except (OSError, json.JSONDecodeError):
        return []
    out = []
    for e in entries:
        # Continue may use workspaceDirectory or workspace or folder
        workspace = (
            e.get("workspaceDirectory") or e.get("workspace") or e.get("folder") or ""
        )
        if not _workspace_matches_project(workspace):
            continue
        sid = e.get("sessionId")
        if not sid:
            continue
        title = (e.get("title") or "Untitled").strip()
        date_created = e.get("dateCreated")
        date_updated = e.get("dateUpdated") or e.get("lastUpdatedAt") or e.get("lastMessageAt") or date_created
        try:
            ts = int(date_created) / 1000 if date_created else None
            created_at = datetime.fromtimestamp(ts, timezone.utc).isoformat().replace("+00:00", "Z") if ts else ""
        except (TypeError, ValueError, OSError):
            created_at = ""
        try:
            uts = int(date_updated) / 1000 if date_updated else None
            updated_at = datetime.fromtimestamp(uts, timezone.utc).isoformat().replace("+00:00", "Z") if uts else created_at
        except (TypeError, ValueError, OSError):
            updated_at = created_at
        messages = _get_continue_session_messages(sid) or []
        message_count = len(messages)
        sparkline_data = _sparkline_from_messages(messages) if messages else []
        searchable_text = _searchable_text_for_convo(title or "Untitled", messages)
        out.append({
            "id": sid,
            "title": title or "Untitled",
            "created_at": created_at,
            "updated_at": updated_at,
            "source": "continue",
            "message_count": message_count,
            "sparkline_data": sparkline_data,
            "searchable_text": searchable_text,
        })
    out.sort(key=lambda x: x.get("updated_at", "") or "0", reverse=True)
    return out


def _get_continue_session_messages(session_id: str):
    """Get messages for one Continue session. Returns list of { role, content } or None if not found."""
    sessions_dir = _continue_sessions_dir()
    session_file = sessions_dir / f"{session_id}.json"
    if not session_file.exists():
        return None
    try:
        data = json.loads(session_file.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError):
        return None
    history = data.get("history") or []
    messages = []
    for turn in history:
        msg = turn.get("message") or {}
        role = (msg.get("role") or "user").lower()
        text = _extract_continue_message_text(msg)
        if not text:
            continue
        messages.append({"role": role, "content": text})
    return messages


@app.get("/continue/conversations")
def list_continue_conversations():
    """List Continue (VS Code) sessions for this project — read-only on phone. Requires sign-in."""
    sessions = _list_continue_sessions()
    return {"conversations": sessions}


@app.get("/continue/conversations/{session_id}")
def get_continue_conversation(session_id: str):
    """Get one Continue session with messages — read-only."""
    messages = _get_continue_session_messages(session_id)
    if messages is None:
        raise HTTPException(status_code=404, detail="Continue session not found")
    sessions = _list_continue_sessions()
    meta = next((s for s in sessions if s.get("id") == session_id), None)
    title = (meta.get("title") or "Untitled") if meta else "Untitled"
    return {
        "id": session_id,
        "title": title,
        "source": "continue",
        "messages": messages,
    }


# --- Grok conversations (read-only from Data_Sources/Grok_Export/conversations) ---

_grok_cache: dict = {"ts": 0.0, "data": []}
_GROK_CACHE_TTL = 300  # seconds — re-scan files at most every 5 minutes

def _list_grok_conversations():
    """List Grok export conversations: id, title, created_at, updated_at, source."""
    import time as _time
    if _time.time() - _grok_cache["ts"] < _GROK_CACHE_TTL and _grok_cache["data"]:
        return _grok_cache["data"]
    out = []
    if not GROK_CONVERSATIONS.exists():
        return out
    for path in sorted(GROK_CONVERSATIONS.glob("*.json"), key=lambda p: p.stat().st_mtime, reverse=True):
        try:
            data = json.loads(path.read_text(encoding="utf-8"))
        except (OSError, json.JSONDecodeError):
            continue
        cid = data.get("conversation_id") or path.stem
        title = (data.get("title") or data.get("title_or_preview") or cid or "Grok chat").strip()[:200]
        exported = data.get("exported_at") or ""
        messages = data.get("messages") or []
        if not messages and isinstance(data.get("branches"), list) and data["branches"]:
            messages = data["branches"][0] if isinstance(data["branches"][0], list) else []
        first_ts = None
        last_ts = None
        for m in messages:
            t = m.get("timestamp")
            if t is not None:
                try:
                    ms = int(t) if t > 1e12 else int(t) * 1000
                    first_ts = ms if first_ts is None else min(first_ts, ms)
                    last_ts = ms if last_ts is None else max(last_ts, ms)
                except (TypeError, ValueError):
                    pass
        if first_ts:
            try:
                created_at = datetime.fromtimestamp(first_ts / 1000.0, timezone.utc).isoformat().replace("+00:00", "Z")
            except (TypeError, ValueError, OSError):
                created_at = exported[:10] if exported else ""
        else:
            created_at = exported[:10] if exported else ""
        if last_ts:
            try:
                updated_at = datetime.fromtimestamp(last_ts / 1000.0, timezone.utc).isoformat().replace("+00:00", "Z")
            except (TypeError, ValueError, OSError):
                updated_at = created_at
        else:
            updated_at = created_at
        msg_list = [{"role": (m.get("role") or "user").lower(), "content": (m.get("text") or "").strip()} for m in messages if (m.get("text") or "").strip()]
        message_count = len(msg_list)
        sparkline_data = _sparkline_from_messages(msg_list) if msg_list else []
        searchable_text = _searchable_text_for_convo(title or "Grok chat", msg_list)
        out.append({
            "id": cid,
            "title": title or "Grok chat",
            "created_at": created_at,
            "updated_at": updated_at,
            "source": "grok",
            "message_count": message_count,
            "sparkline_data": sparkline_data,
            "searchable_text": searchable_text,
        })
    import time as _time
    _grok_cache["ts"] = _time.time()
    _grok_cache["data"] = out
    return out


def _normalize_grok_message(m: dict) -> dict | None:
    """Normalize one Grok export message to { role, content }. Returns None if empty."""
    role = (m.get("role") or "user").lower()
    text = (m.get("text") or "").strip()
    if not text:
        return None
    return {"role": role, "content": text}


def _get_grok_messages(conv_id: str):
    """Get messages for one Grok conversation (first branch only). Returns list of { role, content } or None."""
    out = _get_grok_conversation_with_branches(conv_id)
    if out is None:
        return None
    return out["messages"]


def _get_grok_conversation_with_branches(conv_id: str) -> dict | None:
    """Get one Grok conversation with optional branches. Returns { messages, branches, branch_count } or None.
    Export JSON may have top-level 'branches' (array of message arrays); else we use 'messages' as single branch."""
    if not GROK_CONVERSATIONS.exists():
        return None
    for path in GROK_CONVERSATIONS.glob("*.json"):
        try:
            data = json.loads(path.read_text(encoding="utf-8"))
        except (OSError, json.JSONDecodeError):
            continue
        cid = data.get("conversation_id") or path.stem
        if cid != conv_id and path.stem != conv_id:
            continue
        raw_branches = data.get("branches")
        if isinstance(raw_branches, list) and raw_branches:
            branches = []
            for raw_list in raw_branches:
                if not isinstance(raw_list, list):
                    continue
                msgs = []
                for m in raw_list:
                    nm = _normalize_grok_message(m) if isinstance(m, dict) else None
                    if nm:
                        msgs.append(nm)
                branches.append(msgs)
            if not branches:
                branches = [[]]
        else:
            messages = []
            for m in data.get("messages") or []:
                nm = _normalize_grok_message(m) if isinstance(m, dict) else None
                if nm:
                    messages.append(nm)
            branches = [messages]
        return {
            "messages": branches[0],
            "branches": branches,
            "branch_count": len(branches),
        }
    return None


@app.get("/grok/conversations")
def list_grok_conversations():
    """List Grok export conversations — read-only on phone. Requires sign-in."""
    return {"conversations": _list_grok_conversations()}


@app.get("/grok/conversations/{conv_id}")
def get_grok_conversation(
    conv_id: str,
    branch: int = 0,
):
    """Get one Grok conversation with messages — read-only. Optional ?branch=0|1|... for threaded exports."""
    out = _get_grok_conversation_with_branches(conv_id)
    if out is None:
        raise HTTPException(status_code=404, detail="Grok conversation not found")
    branches = out["branches"]
    branch_index = max(0, min(branch, len(branches) - 1)) if branches else 0
    messages = branches[branch_index] if branches else []
    meta = next((c for c in _list_grok_conversations() if c.get("id") == conv_id), None)
    title = (meta.get("title") or "Grok chat") if meta else "Grok chat"
    resp = {"id": conv_id, "title": title, "source": "grok", "messages": messages, "branch_count": len(branches), "branch_index": branch_index}
    if branch == 0 and len(branches) > 1:
        resp["branches"] = branches
    return resp


# --- Cursor conversations (read-only from Cursor workspaceStorage + globalStorage) ---

def _cursor_workspace_storage():
    base = os.environ.get("APPDATA") or (Path(os.environ.get("USERPROFILE", "")) / "AppData" / "Roaming")
    return Path(base) / "Cursor" / "User" / "workspaceStorage"


def _cursor_global_storage():
    base = os.environ.get("APPDATA") or (Path(os.environ.get("USERPROFILE", "")) / "AppData" / "Roaming")
    return Path(base) / "Cursor" / "User" / "globalStorage"


def _find_cursor_workspace_hash(workspace_storage: Path) -> str | None:
    for entry in workspace_storage.iterdir():
        if not entry.is_dir():
            continue
        wp = entry / "workspace.json"
        if not wp.exists():
            continue
        try:
            data = json.loads(wp.read_text(encoding="utf-8"))
            folder = data.get("folder") or ""
            if _workspace_matches_project(folder):
                return entry.name
        except (OSError, json.JSONDecodeError):
            continue
    return None


def _list_cursor_conversations():
    """List Cursor sidebar chats + composers for this workspace. Returns list of { id, title, created_at, updated_at, source }."""
    import sqlite3
    out = []
    ws = _cursor_workspace_storage()
    gs = _cursor_global_storage()
    if not ws.exists():
        return out
    hash_id = _find_cursor_workspace_hash(ws)
    if not hash_id:
        return out
    db_path = ws / hash_id / "state.vscdb"
    if not db_path.exists():
        return out
    try:
        conn = sqlite3.connect(f"file:{db_path}?mode=ro", uri=True)
    except sqlite3.OperationalError:
        return out
    try:
        cur = conn.cursor()
        cur.execute("SELECT value FROM ItemTable WHERE [key] = ?", ("workbench.panel.aichat.view.aichat.chatdata",))
        row = cur.fetchone()
        if row:
            try:
                chat_data = json.loads(row[0])
                for tab in (chat_data.get("tabs") or []):
                    tab_id = tab.get("tabId", "")
                    title = (tab.get("chatTitle") or "").split("\n")[0][:80] or "Cursor chat"
                    last_send = tab.get("lastSendTime") or 0
                    try:
                        ts = int(last_send) / 1000
                        updated = datetime.fromtimestamp(ts, timezone.utc).isoformat().replace("+00:00", "Z")
                    except (TypeError, ValueError, OSError):
                        updated = ""
                    cid = "tab_" + tab_id
                    msgs = _get_cursor_messages(cid) or []
                    mc = len(msgs)
                    sp = _sparkline_from_messages(msgs) if msgs else []
                    searchable_text = _searchable_text_for_convo(title or "Cursor chat", msgs)
                    out.append({"id": cid, "title": title, "created_at": updated, "updated_at": updated, "source": "cursor", "message_count": mc, "sparkline_data": sp, "searchable_text": searchable_text})
            except json.JSONDecodeError:
                pass
        cur.execute("SELECT value FROM ItemTable WHERE [key] = ?", ("composer.composerData",))
        comp_row = cur.fetchone()
        conn.close()
    except Exception:
        try:
            conn.close()
        except Exception:
            pass
        return out
    if not comp_row:
        return out
    try:
        composer_data = json.loads(comp_row[0])
        all_composers = composer_data.get("allComposers") or []
    except json.JSONDecodeError:
        return out
    global_db = gs / "state.vscdb"
    if not global_db.exists():
        return out
    try:
        gconn = sqlite3.connect(f"file:{global_db}?mode=ro", uri=True)
    except sqlite3.OperationalError:
        return out
    try:
        for comp in all_composers:
            cid = comp.get("composerId") or comp.get("id", "")
            name = (comp.get("name") or "").split("\n")[0][:80] or "Composer"
            created = comp.get("createdAt") or 0
            updated_raw = comp.get("lastUpdatedAt") or comp.get("updatedAt") or comp.get("lastSendTime") or created
            try:
                ts = int(created) / 1000
                created_at = datetime.fromtimestamp(ts, timezone.utc).isoformat().replace("+00:00", "Z")
            except (TypeError, ValueError, OSError):
                created_at = ""
            try:
                uts = int(updated_raw) / 1000
                updated_at = datetime.fromtimestamp(uts, timezone.utc).isoformat().replace("+00:00", "Z")
            except (TypeError, ValueError, OSError):
                updated_at = created_at
            conv_id = "composer_" + cid
            msgs = _get_cursor_messages(conv_id) or []
            mc = len(msgs)
            sp = _sparkline_from_messages(msgs) if msgs else []
            searchable_text = _searchable_text_for_convo(name or "Composer", msgs)
            out.append({"id": conv_id, "title": name, "created_at": created_at, "updated_at": updated_at, "source": "cursor", "message_count": mc, "sparkline_data": sp, "searchable_text": searchable_text})
        gconn.close()
    except Exception:
        try:
            gconn.close()
        except Exception:
            pass
    out.sort(key=lambda x: x.get("updated_at", "") or "0", reverse=True)
    return out


def _get_cursor_messages(conv_id: str):
    """Get messages for one Cursor tab or composer. Returns list of { role, content } or None."""
    import sqlite3
    if conv_id.startswith("tab_"):
        tab_id = conv_id[4:]
        ws = _cursor_workspace_storage()
        hash_id = _find_cursor_workspace_hash(ws)
        if not hash_id or not (ws / hash_id / "state.vscdb").exists():
            return None
        try:
            conn = sqlite3.connect(f"file:{ws / hash_id / 'state.vscdb'}?mode=ro", uri=True)
        except sqlite3.OperationalError:
            return None
        try:
            cur = conn.cursor()
            cur.execute("SELECT value FROM ItemTable WHERE [key] = ?", ("workbench.panel.aichat.view.aichat.chatdata",))
            row = cur.fetchone()
            conn.close()
        except Exception:
            try:
                conn.close()
            except Exception:
                pass
            return None
        if not row:
            return None
        try:
            chat_data = json.loads(row[0])
            for tab in (chat_data.get("tabs") or []):
                if tab.get("tabId") != tab_id:
                    continue
                messages = []
                for bubble in (tab.get("bubbles") or []):
                    typ = (bubble.get("type") or "").lower()
                    text = bubble.get("text") or bubble.get("content") or ""
                    if isinstance(text, list):
                        text = " ".join(p.get("text", p) if isinstance(p, dict) else str(p) for p in text)
                    text = (text or "").strip()
                    if not text:
                        continue
                    role = "user" if typ == "user" else "assistant"
                    messages.append({"role": role, "content": text})
                return messages
        except json.JSONDecodeError:
            pass
        return None
    if conv_id.startswith("composer_"):
        composer_id = conv_id[9:]
        gs = _cursor_global_storage()
        if not (gs / "state.vscdb").exists():
            return None
        try:
            gconn = sqlite3.connect(f"file:{gs / 'state.vscdb'}?mode=ro", uri=True)
        except sqlite3.OperationalError:
            return None
        try:
            gcur = gconn.cursor()
            gcur.execute("SELECT value FROM cursorDiskKV WHERE [key] = ?", (f"composerData:{composer_id}",))
            row = gcur.fetchone()
            gconn.close()
        except Exception:
            try:
                gconn.close()
            except Exception:
                pass
            return None
        if not row:
            return None
        try:
            body = json.loads(row[0])
            messages = []
            for msg in (body.get("conversation") or body.get("messages") or body.get("bubbles") or []):
                if not isinstance(msg, dict):
                    continue
                role = (msg.get("role") or msg.get("type") or "").lower()
                if role not in ("user", "human"):
                    role = "assistant"
                text = msg.get("text") or msg.get("content") or msg.get("rawText") or ""
                if isinstance(text, list):
                    text = " ".join(p.get("text", p) if isinstance(p, dict) else str(p) for p in text)
                text = (text or "").strip()
                if not text:
                    continue
                messages.append({"role": role, "content": text})
            if not messages:
                headers = body.get("fullConversationHeadersOnly") or []
                for h in headers:
                    if isinstance(h, dict):
                        t = h.get("type", 0)
                        role = "user" if t == 1 else "assistant"
                        messages.append({"role": role, "content": f"[{role} message]"})
                inline = (body.get("text") or "").strip() or (body.get("richText") or "").strip()
                if inline:
                    messages.append({"role": "assistant", "content": "(Exported snippet)\n\n" + inline[:15000]})
            return messages if messages else None
        except json.JSONDecodeError:
            pass
        return None
    return None


@app.get("/cursor/conversations")
def list_cursor_conversations():
    """List Cursor sidebar chats + composers for this project — read-only on phone. Requires sign-in."""
    return {"conversations": _list_cursor_conversations()}


@app.get("/cursor/conversations/{conv_id}")
def get_cursor_conversation(conv_id: str):
    """Get one Cursor conversation (tab or composer) — read-only."""
    messages = _get_cursor_messages(conv_id)
    if messages is None:
        raise HTTPException(status_code=404, detail="Cursor conversation not found")
    meta = next((c for c in _list_cursor_conversations() if c.get("id") == conv_id), None)
    title = (meta.get("title") or "Cursor chat") if meta else "Cursor chat"
    return {"id": conv_id, "title": title, "source": "cursor", "messages": messages}


@app.get("/")
def root():
    return {
        "service": "Locus API",
        "ollama_compatible": True,
        "docs": "/docs",
        "web_chat": "/web",
        "manifest": "/locus.webmanifest",
    }


def _html_esc(s: str) -> str:
    return (s or "").replace("&", "&amp;").replace("<", "&lt;").replace(">", "&gt;").replace('"', "&quot;").replace("'", "&#39;")


@app.get("/files", response_class=HTMLResponse)
def files_page():
    """Project file browser + viewer/editor for phone — browse, view, edit, save; send file to chat to discuss with Claudia."""
    html = """<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1, viewport-fit=cover">
  <meta http-equiv="Cache-Control" content="no-cache, no-store, must-revalidate">
  <title>Project files — Claudia</title>
  <style>
    :root { --bg:#0f0b18; --surface:#1a1228; --border:rgba(255,122,217,.25); --text:#f0eaff; --text-dim:#a89cc8; --pink:#ff7ad9; }
    * { box-sizing: border-box; }
    body { margin:0; font-family: system-ui, sans-serif; background: var(--bg); color: var(--text); min-height: 100vh; min-height: 100dvh; padding-top: env(safe-area-inset-top); padding-left: env(safe-area-inset-left); padding-right: env(safe-area-inset-right); padding-bottom: env(safe-area-inset-bottom); }
    .files-hdr { display: flex; align-items: center; gap: 12px; padding: 12px 16px; border-bottom: 1px solid var(--border); flex-wrap: wrap; }
    .files-hdr a { color: var(--pink); text-decoration: none; font-weight: 600; }
    .files-hdr a:hover { text-decoration: underline; }
    #fileBreadcrumb { flex: 1; min-width: 0; font-size: 13px; color: var(--text-dim); }
    .file-bread-item { color: var(--pink); }
    .file-bread-sep { color: var(--text-dim); pointer-events: none; }
    .files-search-wrap { width: 100%; padding: 8px 0; flex-shrink: 0; }
    #fileSearch { width: 100%; padding: 10px 14px; border-radius: 12px; border: 1px solid var(--border); background: var(--surface); color: var(--text); font-size: 14px; outline: none; box-sizing: border-box; }
    #fileSearch:focus { border-color: var(--pink); }
    #fileSearch::placeholder { color: var(--text-dim); }
    #fileList { padding: 8px; display: block; }
    .file-list-row { display: flex; align-items: center; gap: 10px; padding: 12px 14px; border-radius: 12px; cursor: pointer; transition: background .15s; }
    .file-list-row:active { background: rgba(255,122,217,.12); }
    .file-list-icon { font-size: 20px; }
    .file-list-name { font-size: 15px; }
    .file-list-err { padding: 20px; color: var(--text-dim); }
    #fileViewerWrap { display: none; padding: 12px 16px; flex-direction: column; height: calc(100vh - 120px); box-sizing: border-box; }
    #fileViewerLabel { font-size: 13px; color: var(--pink); margin-bottom: 8px; word-break: break-all; flex-shrink: 0; }
    #fileViewerPre, #fileViewerTextarea { flex: 1; min-height: 200px; padding: 12px; border-radius: 12px; border: 1px solid var(--border); background: var(--surface); color: var(--text); font-size: 14px; line-height: 1.5; white-space: pre-wrap; word-break: break-word; overflow: auto; font-family: inherit; resize: none; }
    #fileViewerTextarea { display: none; }
    #fileViewerImageWrap { display: none; flex: 1; min-height: 200px; overflow: auto; -webkit-overflow-scrolling: touch; padding: 12px 0; }
    #fileViewerImageWrap img { max-width: 100%; height: auto; display: block; border-radius: 12px; border: 1px solid var(--border); }
    #fileViewerPdfWrap { display: none; flex: 1; flex-direction: column; min-height: 300px; overflow: auto; -webkit-overflow-scrolling: touch; }
    #fileViewerPdfWrap embed { width: 100%; min-height: 400px; border-radius: 12px; border: 1px solid var(--border); flex: 1; }
    .file-viewer-pdf-fallback { margin-top: 10px; }
    .file-viewer-pdf-fallback a { color: var(--pink); font-weight: 600; }
    .file-viewer-actions { display: flex; gap: 10px; margin-top: 12px; flex-wrap: wrap; flex-shrink: 0; }
    .file-viewer-actions button { padding: 10px 16px; border-radius: 12px; font-size: 14px; font-weight: 600; cursor: pointer; border: none; }
    #fileBtnBack { background: var(--surface); color: var(--text); border: 1px solid var(--border); }
    #fileBtnEdit { background: rgba(255,122,217,.2); color: var(--pink); border: 1px solid var(--pink); }
    #fileBtnSave { display: none; background: var(--pink); color: #111; }
    #fileBtnSendToChat { background: rgba(255,122,217,.25); color: var(--pink); border: 1px solid var(--pink); }
  </style>
</head>
<body>
  <div class="files-hdr">
    <a href="/web">â† Chat</a>
""" + ('' if GAMES_OFF else ('    <a href="/dashboard">Dashboard</a>\n' if not DASHBOARD_OFF else '    <a href="/dashboard/angel-demon">Angels &amp; Demons</a>\n')) + """
    <div id="fileBreadcrumb">Project</div>
  </div>
  <div class="files-search-wrap">
    <input type="search" id="fileSearch" placeholder="Search files and folders…" autocomplete="off" aria-label="Search files">
  </div>
  <div id="fileList"></div>
  <div id="fileViewerWrap">
    <div id="fileViewerLabel"></div>
    <pre id="fileViewerPre"></pre>
    <textarea id="fileViewerTextarea" spellcheck="false"></textarea>
    <div id="fileViewerImageWrap"><img id="fileViewerImage" alt="Preview" style="max-width:100%"></div>
    <div id="fileViewerPdfWrap"><embed id="fileViewerPdf" type="application/pdf"><p class="file-viewer-pdf-fallback">If the PDF does not appear above, <a id="fileViewerPdfOpen" href="#" target="_blank" rel="noopener">open it in a new tab</a>.</p></div>
    <div class="file-viewer-actions">
      <button type="button" id="fileBtnBack">â† Back</button>
      <button type="button" id="fileBtnEdit">Edit</button>
      <button type="button" id="fileBtnSave" style="display:none">Save</button>
      <button type="button" id="fileBtnSendToChat" title="Open chat and ask Claudia about this file">Send to chat</button>
    </div>
    <p class="files-ai-hint" style="font-size:12px;color:var(--text-dim);margin-top:8px;">Use <strong>Send to chat</strong> to open Claudia with this file — ask her to summarize, explain, or edit.</p>
  </div>
  <script src="/files.js?v=2"></script>
</body>
</html>"""
    return Response(content=html, media_type="text/html; charset=utf-8", headers={"Cache-Control": "no-store"})


# PWA icon: beauty-style face (assets/beauty.png) + cat ears (assets/cat-ears.png), on old TV
LOCUS_ICON_SVG = '''<?xml version="1.0" encoding="UTF-8"?>
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 512 512" width="512" height="512">
  <defs>
    <linearGradient id="frameLight" x1="0%" y1="0%" x2="100%" y2="100%">
      <stop offset="0%" style="stop-color:#e8e8e8"/>
      <stop offset="50%" style="stop-color:#a0a0a0"/>
      <stop offset="100%" style="stop-color:#707070"/>
    </linearGradient>
    <!-- Glass-style purple screen: base tint -->
    <linearGradient id="screenBg" x1="0%" y1="100%" x2="0%" y2="0%">
      <stop offset="0%" style="stop-color:#b86dd1"/>
      <stop offset="100%" style="stop-color:#c77bff"/>
    </linearGradient>
    <!-- Specular highlight (glass reflection, top-left) -->
    <linearGradient id="glassSpecular" x1="0%" y1="0%" x2="100%" y2="100%">
      <stop offset="0%" style="stop-color:rgba(255,255,255,0.45)"/>
      <stop offset="28%" style="stop-color:rgba(255,255,255,0.12)"/>
      <stop offset="100%" style="stop-color:rgba(255,255,255,0)"/>
    </linearGradient>
    <!-- Edge highlight (Fresnel-like glass rim) -->
    <linearGradient id="glassEdge" x1="0%" y1="0%" x2="0%" y2="100%">
      <stop offset="0%" style="stop-color:rgba(255,255,255,0.25)"/>
      <stop offset="15%" style="stop-color:rgba(255,255,255,0)"/>
      <stop offset="85%" style="stop-color:rgba(255,255,255,0)"/>
      <stop offset="100%" style="stop-color:rgba(255,255,255,0.12)"/>
    </linearGradient>
    <pattern id="scanlines" width="4" height="4" patternUnits="userSpaceOnUse">
      <rect width="4" height="1" fill="rgba(0,0,0,0.05)"/>
    </pattern>
    <mask id="screenMask">
      <rect x="52" y="52" width="408" height="408" rx="40" ry="40" fill="white"/>
    </mask>
  </defs>
  <!-- Outer TV frame (beveled silver) -->
  <rect x="8" y="8" width="496" height="496" rx="72" ry="72" fill="url(#frameLight)" stroke="#505050" stroke-width="2"/>
  <!-- Black bezel -->
  <rect x="36" y="36" width="440" height="440" rx="52" ry="52" fill="#0a0a0a"/>
  <!-- Screen: glass layer 1 (purple base) -->
  <rect x="52" y="52" width="408" height="408" rx="40" ry="40" fill="url(#screenBg)"/>
  <!-- Glass layer 2: specular reflection (clipped to screen) -->
  <rect x="52" y="52" width="408" height="408" rx="40" ry="40" fill="url(#glassSpecular)" mask="url(#screenMask)"/>
  <!-- Glass layer 3: soft edge highlight -->
  <rect x="52" y="52" width="408" height="408" rx="40" ry="40" fill="url(#glassEdge)" mask="url(#screenMask)"/>
  <rect x="52" y="52" width="408" height="408" rx="40" ry="40" fill="url(#scanlines)"/>
  <!-- Strong specular (glass hot-spot, Apple Glass—style) -->
  <ellipse cx="140" cy="100" rx="140" ry="70" fill="rgba(255,255,255,0.22)" mask="url(#screenMask)"/>
  <ellipse cx="180" cy="120" rx="100" ry="50" fill="rgba(255,255,255,0.14)" mask="url(#screenMask)"/>
  <!-- Beauty-style face (beauty.png): round face, big eyes with shine, eyebrows, smile, bangs, top bun -->
  <g fill="none" stroke="rgba(255,255,255,0.92)" stroke-width="4" stroke-linecap="round" stroke-linejoin="round">
    <ellipse cx="256" cy="268" rx="72" ry="84"/>
    <!-- Bangs sweeping across forehead -->
    <path d="M182 198 Q256 178 330 198 Q322 228 256 236 Q190 228 182 198 Z"/>
    <!-- Side hair -->
    <path d="M182 214 Q162 268 174 328"/>
    <path d="M330 214 Q350 268 338 328"/>
    <!-- Eyebrows (thin curved) -->
    <path d="M218 244 Q232 238 246 244"/>
    <path d="M266 244 Q280 238 294 244"/>
    <!-- Nose (tiny curve) -->
    <path d="M252 268 Q256 272 260 268"/>
    <!-- Smile -->
    <path d="M238 288 Q256 298 274 288"/>
    <!-- Rounded neckline -->
    <path d="M220 332 Q256 358 292 332" stroke-width="3.5"/>
  </g>
  <!-- Top bun (beauty icon) -->
  <circle cx="256" cy="188" r="22" fill="none" stroke="rgba(255,255,255,0.9)" stroke-width="4"/>
  <!-- Cat ears (cat-ears.png style): triangular, rounded tip, inner ear -->
  <g fill="none" stroke="rgba(255,255,255,0.9)" stroke-width="4" stroke-linejoin="round">
    <path d="M218 172 L188 108 L238 168 Z"/>
    <path d="M294 172 L324 108 L274 168 Z"/>
  </g>
  <path d="M212 156 L192 120 L222 162 Z" fill="rgba(255,255,255,0.52)" stroke="none"/>
  <path d="M300 156 L320 120 L290 162 Z" fill="rgba(255,255,255,0.52)" stroke="none"/>
  <!-- Large round eyes with white highlight (beauty style) -->
  <path fill-rule="evenodd" fill="rgba(255,255,255,0.96)" stroke="rgba(255,255,255,0.88)" stroke-width="2" d="M232 258 A12 14 0 0 1 208 258 A12 14 0 0 1 232 258 Z M224 252 A4 4 0 0 1 224 251.99 Z"/>
  <path fill-rule="evenodd" fill="rgba(255,255,255,0.96)" stroke="rgba(255,255,255,0.88)" stroke-width="2" d="M304 258 A12 14 0 0 1 280 258 A12 14 0 0 1 304 258 Z M296 252 A4 4 0 0 1 296 251.99 Z"/>
</svg>'''

# Header-only: TV head with switchable face states (default + thinking) for cartoon-y mood
HEADER_TV_ICON_SVG = '''<?xml version="1.0" encoding="UTF-8"?>
<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 512 512" width="100%" height="100%" style="display:block;overflow:hidden">
  <defs>
    <linearGradient id="hFrameLight" x1="0%" y1="0%" x2="100%" y2="100%">
      <stop offset="0%" stop-color="#e8e8e8"/>
      <stop offset="50%" stop-color="#a0a0a0"/>
      <stop offset="100%" stop-color="#707070"/>
    </linearGradient>
    <linearGradient id="hScreenBg" x1="0%" y1="100%" x2="0%" y2="0%">
      <stop offset="0%" stop-color="#7220b4"/>
      <stop offset="100%" stop-color="#c055ff"/>
    </linearGradient>
    <pattern id="hScanlines" width="4" height="4" patternUnits="userSpaceOnUse">
      <rect width="4" height="1" fill="rgba(0,0,0,0.04)"/>
    </pattern>
    <mask id="hScreenMask">
      <rect x="52" y="52" width="408" height="408" rx="40" ry="40" fill="white"/>
    </mask>
    <radialGradient id="hSkin" cx="44%" cy="36%" r="62%">
      <stop offset="0%" stop-color="#ffe8cc"/>
      <stop offset="100%" stop-color="#f0a870"/>
    </radialGradient>
    <radialGradient id="hBlushL" cx="50%" cy="50%" r="50%">
      <stop offset="0%" stop-color="#ff88bb" stop-opacity="0.95"/>
      <stop offset="70%" stop-color="#ff88bb" stop-opacity="0.5"/>
      <stop offset="100%" stop-color="#ff88bb" stop-opacity="0"/>
    </radialGradient>
    <radialGradient id="hBlushR" cx="50%" cy="50%" r="50%">
      <stop offset="0%" stop-color="#ff88bb" stop-opacity="0.95"/>
      <stop offset="70%" stop-color="#ff88bb" stop-opacity="0.5"/>
      <stop offset="100%" stop-color="#ff88bb" stop-opacity="0"/>
    </radialGradient>
  </defs>

  <!-- TV outer frame (metallic) -->
  <rect x="8" y="8" width="496" height="496" rx="72" ry="72" fill="url(#hFrameLight)" stroke="#505050" stroke-width="2"/>
  <!-- TV bezel (black) -->
  <rect x="36" y="36" width="440" height="440" rx="52" ry="52" fill="#0a0a0a"/>
  <!-- Purple screen -->
  <rect x="52" y="52" width="408" height="408" rx="40" ry="40" fill="url(#hScreenBg)"/>

  <!-- Everything below is masked to the screen -->
  <g mask="url(#hScreenMask)">
    <rect x="52" y="52" width="408" height="408" fill="url(#hScanlines)"/>

    <!-- Cat ears: dark hair triangles with pink inner, sit above face -->
    <polygon points="148,258 196,82 256,232" fill="#1c0940"/>
    <polygon points="256,232 316,82 364,258" fill="#1c0940"/>
    <!-- Ear inner (pink) -->
    <polygon points="164,250 200,104 248,226" fill="#e878aa"/>
    <polygon points="264,226 312,104 348,250" fill="#e878aa"/>

    <!-- Hair cap: dark oval covering top of head + side strands -->
    <ellipse cx="256" cy="232" rx="124" ry="98" fill="#1c0940"/>
    <!-- Side hair strands (hang beside face) -->
    <ellipse cx="152" cy="340" rx="46" ry="100" fill="#1c0940"/>
    <ellipse cx="360" cy="340" rx="46" ry="100" fill="#1c0940"/>

    <!-- Face skin (peach) -->
    <ellipse cx="256" cy="304" rx="120" ry="116" fill="url(#hSkin)"/>

    <!-- Hair fringe line (dark sweep across forehead) -->
    <path d="M134,268 Q158,206 200,212 Q224,217 244,248 Q256,262 268,248 Q288,217 312,212 Q354,206 378,268 Q352,228 316,224 Q292,220 274,250 Q256,264 238,250 Q220,220 196,224 Q160,228 134,268 Z" fill="#1c0940"/>

    <!-- Blush cheeks (obvious pink — always visible) -->
    <ellipse cx="176" cy="322" rx="42" ry="26" fill="url(#hBlushL)"/>
    <ellipse cx="336" cy="322" rx="42" ry="26" fill="url(#hBlushR)"/>

    <!-- â•â•â• FACE STATES â•â•â• -->

    <!-- DEFAULT: cute wide eyes, obvious smile, visible â™¡ — "trapped but adorable" -->
    <g id="face-default">
      <!-- Left eye: bright sclera + vivid purple iris + dark pupil + big sparkles -->
      <ellipse cx="206" cy="284" rx="30" ry="34" fill="#fff"/>
      <ellipse cx="206" cy="289" rx="21" ry="25" fill="#6b3cd0"/>
      <ellipse cx="206" cy="291" rx="14" ry="17" fill="#0d0420"/>
      <circle cx="218" cy="276" r="10" fill="#fff"/>
      <circle cx="198" cy="287" r="6" fill="#fff"/>
      <!-- Right eye -->
      <ellipse cx="306" cy="284" rx="30" ry="34" fill="#fff"/>
      <ellipse cx="306" cy="289" rx="21" ry="25" fill="#6b3cd0"/>
      <ellipse cx="306" cy="291" rx="14" ry="17" fill="#0d0420"/>
      <circle cx="318" cy="276" r="10" fill="#fff"/>
      <circle cx="298" cy="287" r="6" fill="#fff"/>
      <!-- Eyebrows (thick, dark, clearly arched) -->
      <path d="M178,268 Q206,252 234,268" fill="none" stroke="#0d0420" stroke-width="14" stroke-linecap="round"/>
      <path d="M278,268 Q306,252 334,268" fill="none" stroke="#0d0420" stroke-width="14" stroke-linecap="round"/>
      <!-- Cat nose -->
      <path d="M246,312 Q256,322 266,312" fill="none" stroke="#b86848" stroke-width="8" stroke-linecap="round"/>
      <!-- Obvious smile (thick, bright) -->
      <path d="M232,342 Q256,368 280,342" fill="none" stroke="#d03060" stroke-width="12" stroke-linecap="round"/>
      <!-- Teeth hint -->
      <path d="M236,346 Q256,362 276,346 Q256,354 236,346 Z" fill="#fff"/>
      <!-- Visible â™¡ (bright, not faint) -->
      <path d="M378,116 C384,100 400,94 406,110 C412,94 428,100 434,116 C442,138 406,160 406,160 C406,160 370,138 378,116 Z" fill="#ff70b0" stroke="#ffa0d0" stroke-width="3"/>
    </g>

    <!-- THINKING: obvious half-lidded eyes, wavy "hmm" mouth, big dots -->
    <g id="face-thinking" style="display:none">
      <!-- Left eye (half-lidded — lid covers more) -->
      <ellipse cx="206" cy="284" rx="30" ry="34" fill="#fff"/>
      <ellipse cx="206" cy="289" rx="21" ry="25" fill="#6b3cd0"/>
      <ellipse cx="206" cy="291" rx="13" ry="16" fill="#0d0420"/>
      <circle cx="214" cy="282" r="8" fill="#fff"/>
      <!-- Upper lid (obvious droop) -->
      <path d="M174,272 Q206,252 238,272 L238,290 Q206,278 174,290 Z" fill="#1c0940"/>
      <path d="M178,268 Q206,254 234,268" fill="none" stroke="#0d0420" stroke-width="14" stroke-linecap="round"/>
      <!-- Right eye (half-lidded) -->
      <ellipse cx="306" cy="284" rx="30" ry="34" fill="#fff"/>
      <ellipse cx="306" cy="289" rx="21" ry="25" fill="#6b3cd0"/>
      <ellipse cx="306" cy="291" rx="13" ry="16" fill="#0d0420"/>
      <circle cx="314" cy="282" r="8" fill="#fff"/>
      <path d="M274,272 Q306,252 338,272 L338,290 Q306,278 274,290 Z" fill="#1c0940"/>
      <path d="M278,268 Q306,254 334,268" fill="none" stroke="#0d0420" stroke-width="14" stroke-linecap="round"/>
      <!-- Nose -->
      <path d="M248,314 Q256,322 264,314" fill="none" stroke="#b86848" stroke-width="8" stroke-linecap="round"/>
      <!-- Wavy "hmm..." mouth (thick, visible) -->
      <path d="M234,346 Q248,336 262,346 Q276,356 290,346" fill="none" stroke="#d03060" stroke-width="11" stroke-linecap="round"/>
      <!-- Big obvious thinking dots -->
      <circle class="think-dot" cx="224" cy="394" r="14" fill="#ffa0e0"/>
      <circle class="think-dot d2" cx="256" cy="394" r="14" fill="#ffa0e0"/>
      <circle class="think-dot d3" cx="288" cy="394" r="14" fill="#ffa0e0"/>
    </g>

    <!-- LAUGHING: obvious crescent eyes (^u^), big open laugh, bright stars -->
    <g id="face-laughing" style="display:none">
      <!-- Left eye: happy crescent (thick dark line) -->
      <path d="M174,278 Q206,246 238,278 Q206,282 174,278 Z" fill="#0d0420"/>
      <path d="M178,266 Q206,252 234,266" fill="none" stroke="#0d0420" stroke-width="14" stroke-linecap="round"/>
      <!-- Star sparkle left (bigger, brighter) -->
      <polygon points="166,232 172,250 190,250 176,262 181,280 166,268 151,280 156,262 142,250 160,250" fill="#ffdd00" stroke="#fff" stroke-width="2"/>
      <!-- Right eye squinted -->
      <path d="M274,278 Q306,246 338,278 Q306,282 274,278 Z" fill="#0d0420"/>
      <path d="M278,266 Q306,252 334,266" fill="none" stroke="#0d0420" stroke-width="14" stroke-linecap="round"/>
      <polygon points="326,232 332,250 350,250 336,262 341,280 326,268 311,280 316,262 302,250 320,250" fill="#ffdd00" stroke="#fff" stroke-width="2"/>
      <!-- Nose -->
      <path d="M248,314 Q256,322 264,314" fill="none" stroke="#b86848" stroke-width="8" stroke-linecap="round"/>
      <!-- Big obvious laugh mouth -->
      <path d="M216,330 Q256,388 296,330 Q256,352 216,330 Z" fill="#d03060"/>
      <path d="M220,334 Q256,362 292,334 Q256,350 220,334 Z" fill="#fff"/>
    </g>

    <!-- JUDGY: obvious flat brow left, raised brow right, squint, flat mouth -->
    <g id="face-judgy" style="display:none">
      <!-- Left eye -->
      <ellipse cx="206" cy="284" rx="30" ry="34" fill="#fff"/>
      <ellipse cx="210" cy="289" rx="21" ry="25" fill="#6b3cd0"/>
      <ellipse cx="212" cy="291" rx="14" ry="17" fill="#0d0420"/>
      <circle cx="218" cy="278" r="8" fill="#fff"/>
      <!-- Left brow: clearly flat/furrowed -->
      <path d="M178,264 L234,264" fill="none" stroke="#0d0420" stroke-width="14" stroke-linecap="round"/>
      <!-- Right eye (skeptical squint — obvious half lid) -->
      <ellipse cx="306" cy="284" rx="30" ry="34" fill="#fff"/>
      <ellipse cx="302" cy="289" rx="21" ry="25" fill="#6b3cd0"/>
      <ellipse cx="300" cy="291" rx="14" ry="17" fill="#0d0420"/>
      <circle cx="312" cy="280" r="7" fill="#fff"/>
      <path d="M274,266 Q306,250 338,266 L338,284 Q306,274 274,284 Z" fill="#1c0940"/>
      <!-- Right brow: obviously raised -->
      <path d="M278,248 Q308,232 334,252" fill="none" stroke="#0d0420" stroke-width="14" stroke-linecap="round"/>
      <!-- Nose -->
      <path d="M248,314 Q256,322 264,314" fill="none" stroke="#b86848" stroke-width="8" stroke-linecap="round"/>
      <!-- Obvious flat unamused mouth -->
      <path d="M232,348 L280,348" fill="none" stroke="#d03060" stroke-width="12" stroke-linecap="round"/>
    </g>

    <!-- SILLY: obvious â˜… left eye, dizzy right, tongue out grin -->
    <g id="face-silly" style="display:none">
      <!-- Left eye: big bright star (â˜…) -->
      <ellipse cx="206" cy="284" rx="30" ry="34" fill="#fff"/>
      <polygon points="206,254 214,274 236,274 219,288 226,308 206,296 186,308 193,288 176,274 198,274" fill="#ffcc00" stroke="#fff" stroke-width="2"/>
      <!-- Right eye: dizzy (clear outline) -->
      <ellipse cx="306" cy="284" rx="30" ry="34" fill="#fff"/>
      <ellipse cx="306" cy="284" rx="21" ry="23" fill="#7040c8"/>
      <ellipse cx="306" cy="284" rx="13" ry="14" fill="#0d0420"/>
      <circle cx="306" cy="284" r="10" fill="none" stroke="#fff" stroke-width="4"/>
      <circle cx="316" cy="273" r="7" fill="#fff"/>
      <!-- Eyebrows (bouncy, thick) -->
      <path d="M178,266 Q206,248 234,266" fill="none" stroke="#0d0420" stroke-width="14" stroke-linecap="round"/>
      <path d="M278,266 Q306,248 334,266" fill="none" stroke="#0d0420" stroke-width="14" stroke-linecap="round"/>
      <!-- Nose -->
      <path d="M248,314 Q256,322 264,314" fill="none" stroke="#b86848" stroke-width="8" stroke-linecap="round"/>
      <!-- Obvious tongue-out grin -->
      <path d="M222,330 Q256,362 290,330 Q256,350 222,330 Z" fill="#d03060"/>
      <path d="M224,334 Q256,354 288,334 Q256,346 224,334 Z" fill="#fff"/>
      <!-- Big obvious tongue -->
      <ellipse cx="256" cy="362" rx="26" ry="22" fill="#ff5588"/>
      <path d="M256,356 L256,378" fill="none" stroke="#dd3366" stroke-width="6" stroke-linecap="round"/>
    </g>

    <!-- Glass sheen (subtle so expressions stay visible) -->
    <ellipse cx="208" cy="132" rx="148" ry="74" fill="rgba(255,255,255,0.06)"/>
    <ellipse cx="172" cy="114" rx="96" ry="48" fill="rgba(255,255,255,0.04)"/>
  </g>
</svg>'''

# Canonical icons for all webapp users (Claudia, Lynn, Raven): PWA icon, chat avatars, shortcuts
_CANONICAL_ICONS_DIR = ASSETS_ROOT / "assets" / "Canonical Icons"
# PWA icon subfolders (camera, send, mail, headphone, etc.) now live directly under assets/
# assets/cute/ kept for beauty.png and angel.png only
_ASSETS_CUTE = ASSETS_ROOT / "assets"
CUSTOM_ICON_PATH = _CANONICAL_ICONS_DIR / "icon.svg"
CHAT_AVATAR_PATH = _CANONICAL_ICONS_DIR / "Claudia_icon_avatar.png"
# In-chat avatar: noun girl-with-cat (with pink fill so not see-through)
_CHAT_AVATAR_CANDIDATES = [
    ASSETS_ROOT / "assets" / "subgirls" / "noun-girl-with-cat-1773345.svg",
    ASSETS_ROOT / "assets" / "girl" / "noun-girl-with-cat-1773345.svg",
]
CHAT_AVATAR_SVG_PATH = next((p for p in _CHAT_AVATAR_CANDIDATES if p.exists()), _CHAT_AVATAR_CANDIDATES[0])
# User (Ruby) avatar: full icon, no circle clip
USER_AVATAR_PATH = _CANONICAL_ICONS_DIR / "Ruby and Hahli.svg"
# Claudia in-chat avatar: full icon, no circle (skin already filled); try subgirls then girl
_LOCUS_AVATAR_CANDIDATES = [
    ASSETS_ROOT / "assets" / "subgirls" / "ClaudiaPNGVSGicon.svg",
    ASSETS_ROOT / "assets" / "girl" / "ClaudiaPNGVSGicon.svg",
]
LOCUS_AVATAR_SVG_PATH = next((p for p in _LOCUS_AVATAR_CANDIDATES if p.exists()), _LOCUS_AVATAR_CANDIDATES[0])


def _icon_svg_content() -> str:
    """Return icon SVG string (custom file or built-in)."""
    if CUSTOM_ICON_PATH.exists():
        return CUSTOM_ICON_PATH.read_text(encoding="utf-8")
    return LOCUS_ICON_SVG


@app.get("/icon.svg", response_class=Response)
def icon_svg():
    """PWA icon: custom assets/icon.svg if present, else built-in Claudia TV icon."""
    return Response(content=_icon_svg_content(), media_type="image/svg+xml")


FORK_ICON_PATH = ASSETS_ROOT / "assets" / "forkgitstuff stuff" / "tuning-yellow-fork-svgrepo-com (3).svg"
BEE_ICON_PATH = ASSETS_ROOT / "assets" / "bee" / "bee-svgrepo-com.svg"

# Plan yellow for header fork icon (tuning fork), with yellow neon glow in CSS
FORK_YELLOW_FILL = "#FFE135"


@app.get("/fork.svg", response_class=Response)
def fork_svg():
    """Tuning fork icon for header Fork button: plan yellow with yellow neon glow, vertical so it fits next to + New."""
    if not FORK_ICON_PATH.exists():
        raise HTTPException(status_code=404, detail="Fork icon not found")
    raw = FORK_ICON_PATH.read_text(encoding="utf-8")
    # Unify all fills to plan yellow (tuning fork is vertical; was multi-color)
    raw = re.sub(r'style="[^"]*"', lambda m: re.sub(r'opacity:[^;"]+', "opacity:1", re.sub(r'fill:[^;"]+', f"fill:{FORK_YELLOW_FILL}", m.group(0))), raw)
    raw = re.sub(r'fill:#[0-9A-Fa-f]{3,8}', f"fill:{FORK_YELLOW_FILL}", raw)
    raw = re.sub(r'fill="[^"]*"', f'fill="{FORK_YELLOW_FILL}"', raw)
    return Response(content=raw, media_type="image/svg+xml")


# Neon pink/purple for send-button bee (visible on dark button; img can't inherit currentColor)
BEE_NEON_STROKE = "#ff88ee"

@app.get("/bee.svg", response_class=Response)
def bee_svg():
    """Bee icon for send-chat button (replaces paper airplane). Neon pink/purple so it's visible on dark button."""
    if not BEE_ICON_PATH.exists():
        raise HTTPException(status_code=404, detail="Bee icon not found")
    raw = BEE_ICON_PATH.read_text(encoding="utf-8")
    raw = raw.replace('stroke="#000000"', f'stroke="{BEE_NEON_STROKE}"').replace("stroke-opacity=\"0.9\"", "stroke-opacity=\"0.98\"")
    return Response(content=raw, media_type="image/svg+xml")


def _chat_avatar_svg_content() -> str | None:
    """Return girl-with-cat SVG with pink background so she's not see-through. None if file missing."""
    if not CHAT_AVATAR_SVG_PATH.exists():
        return None
    raw = CHAT_AVATAR_SVG_PATH.read_text(encoding="utf-8")
    # Insert pink background rect so skin/body isn't transparent (viewBox 0 0 81.4 124.5)
    pink_bg = '<rect x="0" y="0" width="81.4" height="124.5" fill="#ffb6c1"/>'
    # Insert after first <g><g> so it sits behind the figure
    if "<g><g>" in raw:
        raw = raw.replace("<g><g>", "<g><g>" + pink_bg, 1)
    else:
        raw = raw.replace("<g>", "<g>" + pink_bg, 1)
    return raw


@app.get("/chat_icon.svg", response_class=Response)
def chat_icon_svg():
    """In-chat avatar: noun girl-with-cat with pink fill (not see-through). Fallback: 404 so client can use .png."""
    content = _chat_avatar_svg_content()
    if content is None:
        raise HTTPException(status_code=404, detail="Chat avatar SVG not found")
    return Response(content=content, media_type="image/svg+xml")


@app.get("/user_avatar.svg", response_class=Response)
def user_avatar_svg():
    """User (Ruby) in-chat avatar: Ruby and Hahli.svg, full icon (no circle)."""
    if not USER_AVATAR_PATH.exists():
        from fastapi.responses import RedirectResponse
        return RedirectResponse(url="/icon.svg", status_code=302)
    return Response(content=USER_AVATAR_PATH.read_text(encoding="utf-8"), media_type="image/svg+xml")


@app.get("/locus_avatar.svg", response_class=Response)
def locus_avatar_svg():
    """Claudia in-chat avatar: ClaudiaPNGVSGicon.svg, full icon (no circle), skin already filled."""
    if not LOCUS_AVATAR_SVG_PATH.exists():
        from fastapi.responses import RedirectResponse
        return RedirectResponse(url="/icon.svg", status_code=302)
    return Response(content=LOCUS_AVATAR_SVG_PATH.read_text(encoding="utf-8"), media_type="image/svg+xml")


@app.get("/chat_icon.png", response_class=Response)
def chat_icon_png():
    """In-chat avatar: PNG if present (Canonical Icons). Fallback: redirect to icon.svg."""
    if CHAT_AVATAR_PATH.exists():
        return Response(content=CHAT_AVATAR_PATH.read_bytes(), media_type="image/png")
    from fastapi.responses import RedirectResponse
    return RedirectResponse(url="/icon.svg", status_code=302)


@app.get("/header_icon.svg", response_class=Response)
def header_icon_svg():
    """TV head with mood layers (default + thinking) for header avatar; inline SVG so JS can toggle state."""
    return Response(content=HEADER_TV_ICON_SVG, media_type="image/svg+xml")


def _resolve_pwa_icon(filename: str) -> Path | None:
    """Resolve icon under assets/ (flat or in any subfolder). Returns path or None."""
    if ".." in filename or "/" in filename or "\\" in filename:
        return None
    base = _ASSETS_CUTE.resolve()
    # Try flat first
    flat = (_ASSETS_CUTE / filename).resolve()
    if flat.is_file() and str(flat).startswith(str(base)):
        return flat
    # Search all immediate subfolders (camera/, send/, mail/, headphone/, etc.)
    for sub in _ASSETS_CUTE.iterdir():
        if sub.is_dir():
            candidate = (sub / filename).resolve()
            if candidate.is_file() and str(candidate).startswith(str(base)):
                return candidate
    return None


@app.get("/pwa_icons/{filename:path}", response_class=Response)
def pwa_icon(filename: str):
    """Serve SVGs from assets/cute/ (and subfolders camera, send, mail, etc.) for PWA."""
    path = _resolve_pwa_icon(filename)
    if path is None:
        return Response(status_code=404)
    content = path.read_bytes()
    return Response(content=content, media_type="image/svg+xml")


_BEDROOM_BG_PATH = ASSETS_ROOT / "assets" / "locus_bedroom.png"
_BEDROOM_BG_ALT = ASSETS_ROOT / "assets" / "background photos" / "locus_bedroom.png"


# 1x1 transparent PNG when bedroom image is missing (avoids 404 on Room tab)
_BEDROOM_BG_FALLBACK = base64.b64decode(
    "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="
)


@app.get("/bedroom_bg.png", response_class=Response)
def bedroom_bg():
    """Serve Minecraft bedroom background image for Room tab (PNG). If missing, serve 1x1 transparent so no 404."""
    if _BEDROOM_BG_PATH.is_file():
        return Response(content=_BEDROOM_BG_PATH.read_bytes(), media_type="image/png")
    return Response(content=_BEDROOM_BG_FALLBACK, media_type="image/png")


@app.get("/favicon.ico", response_class=RedirectResponse)
def favicon():
    """Avoid 404 when browser requests /favicon.ico; redirect to app icon."""
    return RedirectResponse(url="/icon.svg", status_code=302)


@app.get("/apple-touch-icon.png", response_class=Response)
@app.get("/apple-touch-icon-120x120.png", response_class=Response)
@app.get("/apple-touch-icon-120x120-precomposed.png", response_class=Response)
def apple_touch_icon():
    """Serve same icon as SVG so iOS gets 200 instead of 404 (Safari can use SVG for touch icon)."""
    return Response(content=_icon_svg_content(), media_type="image/svg+xml")


@app.get("/locus.webmanifest", response_class=JSONResponse)
def manifest():
    """PWA-style manifest so Safari/Chrome know this is 'installable'."""
    return {
        "name": "Porcelain",
        "short_name": "Porcelain",
        "start_url": "/web",
        "display": "standalone",
        "background_color": "#ff7ad9",
        "theme_color": "#ff7ad9",
        "icons": [
            {"src": "/icon.svg", "sizes": "any", "type": "image/svg+xml", "purpose": "any"},
            {"src": "/icon.svg", "sizes": "512x512", "type": "image/svg+xml", "purpose": "maskable"},
        ],
    }


# Static PWA app script (loaded by /web so the browser parses plain JS, not HTML-embedded)
_WEB_APP_JS_PATH = Path(__file__).resolve().parent / "static_web_app.js"
_FILES_JS_PATH = Path(__file__).resolve().parent / "files_app.js"


@app.get("/files.js")
def files_js():
    """Serve project files browser script."""
    if not _FILES_JS_PATH.exists():
        return Response("/* files_app.js not found */", status_code=404, media_type="text/plain")
    content = _FILES_JS_PATH.read_text(encoding="utf-8")
    return Response(
        content,
        media_type="application/javascript; charset=utf-8",
        headers={"Cache-Control": "no-store, no-cache, must-revalidate", "Pragma": "no-cache"},
    )


@app.get("/web_app.js")
def web_app_js():
    """Serve the PWA app script from a static file so the browser never parses it inside HTML."""
    if not _WEB_APP_JS_PATH.exists():
        return Response("/* static_web_app.js not found */", status_code=404, media_type="text/plain")
    content = _WEB_APP_JS_PATH.read_text(encoding="utf-8")
    return Response(
        content,
        media_type="application/javascript; charset=utf-8",
        headers={"Cache-Control": "no-store, no-cache, must-revalidate", "Pragma": "no-cache"},
    )


@app.get("/access", response_class=HTMLResponse)
def access_page(request: Request):
    """Simple page showing the shareable app URL so you can copy it for Lynn/Raven. See Documentation/Claudia/PHONE_ACCESS_FOR_FRIENDS.md."""
    base = str(request.base_url).rstrip("/")
    app_url = f"{base}/web"
    html = f"""<!DOCTYPE html><html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Share Claudia app</title>
<style>body{{font-family:system-ui;background:#0a0614;color:#f0eaff;padding:20px;max-width:480px;margin:0 auto;}}
h1{{font-size:18px;margin-bottom:12px;color:#ff7ad9;}}
p{{font-size:14px;line-height:1.5;margin-bottom:16px;color:#c0b0d0;}}
a{{color:#ff7ad9;}}
input{{width:100%;padding:12px;border-radius:8px;border:1px solid #3a2a4a;background:#1a0a1a;color:#f0eaff;font-size:14px;box-sizing:border-box;}}
.btn{{display:inline-block;margin-top:8px;padding:10px 16px;background:#ff7ad9;color:#111;border:none;border-radius:8px;font-weight:700;cursor:pointer;font-size:14px;}}</style></head><body>
<h1>Share the app with Lynn &amp; Raven</h1>
<p>They open this link in their phone browser, then choose their name in &quot;Chat as&quot;. No Discord needed.</p>
<input type="text" id="url" value="{app_url}" readonly style="user-select:all">
<button class="btn" onclick="navigator.clipboard.writeText(document.getElementById('url').value);this.textContent='Copied!';setTimeout(function(){{this.textContent='Copy link';}}.bind(this),1500)">Copy link</button>
<p style="margin-top:20px;"><a href="/web">Open Claudia app</a>""" + ('' if GAMES_OFF else (' &middot; <a href="/dashboard">Dashboard</a>' if not DASHBOARD_OFF else ' &middot; <a href="/dashboard/angel-demon">Angels &amp; Demons</a>')) + """</p>
<p style="font-size:12px;color:#806090;">Same WiFi at home, or scan QR code in Chimera Setup tab for remote access. See <a href="http://localhost:3000">Chimera Setup</a> for details.</p>
</body></html>"""
    return HTMLResponse(html)


@app.get("/web")
def web_chat():
    """Mobile-first PWA chat UI: chat bubbles, pinnable convos, sidebar, typing indicator."""
    html = """
<!doctype html>
<html>
<head>
  <!-- PWA HTML v39 -->
  <!-- Claude-style: fork icon, buttons, focus-visible -->
  <meta charset="utf-8">
  <meta http-equiv="Cache-Control" content="no-cache, no-store, must-revalidate">
  <meta http-equiv="Pragma" content="no-cache">
  <meta http-equiv="Expires" content="0">
  <title>Porcelain</title>
  <meta name="viewport" content="width=device-width, initial-scale=1, viewport-fit=cover, interactive-widget=overlays-content">
  <meta name="theme-color" content="#0a0614">
  <meta name="mobile-web-app-capable" content="yes">
  <meta name="apple-mobile-web-app-status-bar-style" content="black-translucent">
  <meta name="apple-mobile-web-app-title" content="Porcelain">
  <link rel="manifest" href="/locus.webmanifest">
  <link rel="icon" type="image/svg+xml" href="/icon.svg">
  <link rel="apple-touch-icon" href="/icon.svg">
  <style>
    :root {
      --pink: #ff7ad9; --pink-dim: rgba(255,122,217,0.12);
      --bg: #0a0614; --surface: #120920; --surface2: #1e1030;
      --border: #321854; --text: #f0eaff; --text-dim: #8060a8;
      --locus-bg: #1f0f38; --locus-text: #f0d8ff; --locus-border: #5a2890;
      --ruby-bg: #ff7ad9; --ruby-text: #111;
      --locus-av-bg: #f5d0e0;
    }
    *{box-sizing:border-box;margin:0;padding:0;-webkit-tap-highlight-color:transparent}
    /* PWA / home screen: fill entire viewport so no black at top (status bar) or bottom (home indicator) */
    html{background:#0a0614;background:var(--bg);height:100%;min-height:100dvh}
    body{position:relative;height:100%;height:100dvh;min-height:100dvh;min-height:-webkit-fill-available;max-height:100dvh;font-family:-apple-system,BlinkMacSystemFont,'SF Pro Text','Segoe UI',sans-serif;background:transparent;color:var(--text);overflow:hidden;overscroll-behavior:none}
    body::before{content:'';position:fixed;top:0;left:0;right:0;bottom:0;background:#0a0614;background:var(--bg);z-index:-2;pointer-events:none}
    #app{height:100dvh;min-height:100dvh;min-height:-webkit-fill-available;max-height:100dvh;padding-top:max(env(safe-area-inset-top),8px);padding-bottom:0;padding-left:env(safe-area-inset-left);padding-right:env(safe-area-inset-right);box-sizing:border-box;display:flex;flex-direction:column;overflow:hidden;background:var(--bg)}
    /* â"€â"€ Sidebar overlay â"€â"€ */
    #sideOverlay{position:fixed;top:0;left:0;right:0;bottom:0;background:rgba(0,0,0,.6);z-index:40;opacity:0;pointer-events:none;transition:opacity .22s}
    #sideOverlay.open{opacity:1;pointer-events:auto}
    /* â"€â"€ Sidebar â"€â"€ */
    #sidebar{position:fixed;top:env(safe-area-inset-top);left:0;bottom:env(safe-area-inset-bottom);width:min(288px,85vw);background:var(--surface);z-index:50;display:flex;flex-direction:column;transform:translateX(-100%);transition:transform .25s cubic-bezier(.4,0,.2,1);border-right:1px solid var(--border)}
    #sidebar.open{transform:translateX(0)}
    .sb-head{padding:14px 16px 8px;display:flex;align-items:center;justify-content:space-between;border-bottom:1px solid var(--border);flex-shrink:0}
    .sb-head-title{font-size:17px;font-weight:700}
    .sb-workspace{font-size:11px;color:var(--text-dim);padding:0 16px 10px;border-bottom:1px solid var(--border);flex-shrink:0;line-height:1.3}
    .sb-workspace strong{color:var(--pink);font-weight:600}
    .sb-user-row{padding:8px 16px 10px;border-bottom:1px solid var(--border);flex-shrink:0;display:flex;align-items:center;gap:8px}
    .sb-user-label{font-size:12px;color:var(--text-dim)}
    .sb-user-select{flex:1;max-width:140px;padding:6px 10px;border-radius:8px;border:1px solid var(--border);background:var(--bg);color:var(--text);font-size:13px;cursor:pointer}
    .sb-user-select:focus{outline:none;border-color:var(--pink)}
    .sb-auth-row{padding:6px 16px 8px;border-bottom:1px solid var(--border);flex-shrink:0;display:flex;align-items:center;gap:8px;flex-wrap:wrap}
    .sb-auth-btn{padding:6px 12px;font-size:12px;font-weight:600;color:var(--pink);background:transparent;border:1px solid var(--pink);border-radius:8px;cursor:pointer;-webkit-appearance:none;appearance:none;transition:background .15s,color .15s}
    .sb-auth-btn:hover,.sb-auth-btn:active{background:var(--pink);color:#111}
    .sb-auth-btn:focus-visible{outline:2px solid var(--pink);outline-offset:2px}
    .sb-auth-out{border-color:var(--text-dim);color:var(--text-dim)}
    .sb-auth-out:hover,.sb-auth-out:active{background:var(--text-dim);color:#111}
    .sb-signed-in{font-size:12px;color:var(--text-dim)}
    .sb-avatar-picker-wrap{padding:8px 16px 10px;border-bottom:1px solid var(--border);flex-shrink:0}
    .sb-avatar-picker-label{font-size:12px;color:var(--text-dim);margin-bottom:6px}
    .sb-avatar-picker{display:flex;flex-wrap:wrap;gap:6px}
    .sb-avatar-picker .av-opt{width:40px;height:40px;border-radius:50%;border:2px solid rgba(255,122,217,.5);padding:0;background:var(--bg);cursor:pointer;overflow:hidden;flex-shrink:0;transition:border-color .15s,box-shadow .15s;box-shadow:0 0 6px rgba(255,122,217,.25)}
    .sb-avatar-picker .av-opt:hover,.sb-avatar-picker .av-opt.selected{border-color:var(--pink);box-shadow:0 0 10px rgba(255,122,217,.5),0 0 4px rgba(255,122,217,.35)}
    .sb-avatar-picker .av-opt img{width:100%;height:100%;object-fit:cover;display:block}
    .sb-avatar-custom-wrap{display:flex;gap:6px;align-items:center}
    .sb-avatar-custom-input{flex:1;min-width:0;padding:6px 10px;border-radius:8px;border:1px solid var(--border);background:var(--bg);color:var(--text);font-size:12px}
    .sb-avatar-custom-btn{padding:6px 12px;border-radius:8px;border:1px solid var(--pink);background:transparent;color:var(--pink);font-size:12px;cursor:pointer;flex-shrink:0}
    .sb-avatar-custom-btn:disabled{opacity:.5;cursor:not-allowed;border-color:var(--border);color:var(--text-dim)}
    .sb-user-profile-wrap{padding:10px 12px;border-bottom:1px solid var(--border);flex-shrink:0}
    .sb-user-profile-label{font-size:12px;color:var(--text-dim);margin-bottom:6px;display:block}
    .sb-user-profile-input,.sb-user-profile-textarea{width:100%;box-sizing:border-box;padding:6px 10px;border-radius:8px;border:1px solid var(--border);background:var(--bg);color:var(--text);font-size:12px;margin-bottom:6px;font-family:inherit}
    .sb-user-profile-textarea{resize:vertical;min-height:44px}
    .sb-user-profile-save{padding:6px 12px;border-radius:8px;border:1px solid var(--pink);background:transparent;color:var(--pink);font-size:12px;cursor:pointer}
    .sb-nav-links{display:flex;flex-wrap:wrap;gap:6px;padding:8px 16px 10px;border-bottom:1px solid var(--border);flex-shrink:0}
    .sb-nav-link{font-size:13px;font-weight:600;color:var(--pink);text-decoration:none;padding:6px 12px;border-radius:10px;border:1px solid var(--pink);white-space:nowrap;transition:background .15s,color .15s}
    .sb-nav-link:hover,.sb-nav-link:active{background:var(--pink);color:#111}
    .sb-nav-link:focus-visible{outline:2px solid var(--pink);outline-offset:2px}
    .sb-new{background:linear-gradient(135deg,#ff7ad9 0%,#e030ff 50%,#9020d0 100%);color:#111;border:none;border-radius:20px;padding:6px 14px;font-size:13px;font-weight:700;cursor:pointer;box-shadow:0 0 12px rgba(255,122,217,.4);transition:box-shadow .15s,transform .1s}
    .sb-new:hover,.sb-new:active{box-shadow:0 0 16px rgba(255,122,217,.5)}
    .sb-new:focus-visible{outline:2px solid var(--pink);outline-offset:2px}
    .sb-top-buttons{display:flex;flex-wrap:wrap;gap:6px;padding:8px 12px;border-bottom:1px solid var(--border);flex-shrink:0}
    .sb-top-btn{font-size:12px;font-weight:600;color:var(--pink);background:transparent;border:1px solid var(--pink);border-radius:8px;padding:6px 10px;cursor:pointer;text-decoration:none;-webkit-appearance:none;appearance:none;transition:background .15s,color .15s;white-space:nowrap}
    .sb-top-btn:hover,.sb-top-btn:active{background:var(--pink);color:#111}
    .sb-top-btn.open{background:var(--pink);color:#111}
    .sb-user-panel{overflow:hidden;max-height:0;transition:max-height .3s ease-out;flex-shrink:0}
    .sb-user-panel.open{max-height:75vh}
    .sb-user-panel-inner{overflow-y:auto;max-height:75vh;padding-bottom:8px}
    .sb-search{padding:10px 12px;border-bottom:1px solid var(--border);flex-shrink:0}
    .sb-search input{width:100%;padding:8px 12px;border-radius:10px;border:1px solid var(--border);background:var(--bg);color:var(--text);font-size:14px;outline:none}
    .sb-search input:focus{border-color:var(--pink)}
    #semanticToggle:hover{border-color:var(--pink)!important;color:var(--pink)!important}
    .sb-list{flex:1;overflow-y:auto;-webkit-overflow-scrolling:touch}
    .sb-section{padding:8px 12px 4px;font-size:10px;font-weight:700;text-transform:uppercase;letter-spacing:.08em;color:var(--text-dim)}
    .sb-item{display:flex;align-items:center;gap:8px;padding:10px 12px;cursor:pointer;border-left:3px solid transparent;background:transparent;width:100%;text-align:left;color:var(--text);border-bottom:1px solid #1a0c2e;transition:background .1s}
    .sb-item:hover,.sb-item:active{background:var(--surface2)}
    .sb-item.active{background:var(--surface2);border-left-color:var(--pink)}
    .sb-item .txt{flex:1;min-width:0}
    .sb-item .ttl{font-size:13px;white-space:nowrap;overflow:hidden;text-overflow:ellipsis;line-height:1.3}
    .sb-item .ttl .sb-title-edit{font-size:13px;line-height:1.3;width:100%;min-width:0;padding:2px 4px;margin:-2px -4px;background:var(--surface2);border:1px solid var(--pink);border-radius:6px;color:var(--text);outline:none}
    .sb-item-star{color:var(--pink);margin-left:2px;font-size:12px;position:relative;z-index:1}
    .sb-item-star-wrap{position:relative;display:inline-block;vertical-align:middle;margin-left:2px;width:20px;height:20px;overflow:visible}
    .star-orbit-sparkles{position:absolute;left:50%;top:50%;width:0;height:0;transform:translate(-50%,-50%);pointer-events:none;z-index:0;perspective:100px}
    .star-orbit-wrap{position:absolute;left:50%;top:50%;width:0;height:0;transform-style:preserve-3d;transform-origin:center center;animation:star-sparkle-orbit-3d var(--orbit-duration,2s) ease-in-out infinite;animation-delay:var(--orbit-phase,0);--orbit-tilt-off:0deg}
    .star-orbit-wrap.star-orbit-tilt-in{--orbit-tilt:52deg}.star-orbit-wrap.star-orbit-tilt-out{--orbit-tilt:-58deg}
    .star-orbit-wrap.star-orbit-rev{animation-direction:reverse}
    .star-orbit-wrap.star-orbit-inner{--orbit-radius:6px}.star-orbit-wrap.star-orbit-outer{--orbit-radius:10px}
    .star-orbit-dot{position:absolute;left:50%;top:50%;width:4px;height:4px;margin-left:var(--orbit-radius,8px);margin-top:-2px;border-radius:50%;background:var(--orbit-color,var(--pink));color:var(--orbit-color,var(--pink));box-shadow:0 0 6px currentColor,0 0 10px currentColor;backface-visibility:visible;animation:star-dot-depth var(--orbit-duration,2s) ease-in-out infinite;animation-delay:var(--orbit-phase,0)}
    .star-orbit-dot::after{content:'';position:absolute;right:100%;top:50%;width:12px;height:3px;margin-top:-1.5px;margin-right:2px;background:linear-gradient(90deg,transparent 0%,var(--orbit-color,var(--pink)) 40%,var(--orbit-color,var(--pink)) 100%);opacity:.85;border-radius:2px;pointer-events:none;filter:blur(0.5px);box-shadow:0 0 8px var(--orbit-color,var(--pink))}
    .star-orbit-wrap.star-orbit-rev .star-orbit-dot::after{right:auto;left:100%;margin-right:0;margin-left:2px;background:linear-gradient(270deg,transparent 0%,var(--orbit-color,var(--pink)) 60%,var(--orbit-color,var(--pink)) 100%)}
    @keyframes star-sparkle-orbit-3d{0%{transform:rotateX(calc(var(--orbit-tilt,55deg) + var(--orbit-tilt-off,0deg))) rotateY(0deg)}25%{transform:rotateX(calc(var(--orbit-tilt,55deg) + var(--orbit-tilt-off,0deg) + 5deg)) rotateY(90deg)}50%{transform:rotateX(calc(var(--orbit-tilt,55deg) + var(--orbit-tilt-off,0deg) - 4deg)) rotateY(180deg)}75%{transform:rotateX(calc(var(--orbit-tilt,55deg) + var(--orbit-tilt-off,0deg) + 3deg)) rotateY(270deg)}100%{transform:rotateX(calc(var(--orbit-tilt,55deg) + var(--orbit-tilt-off,0deg))) rotateY(360deg)}}
    @keyframes star-dot-depth{0%{opacity:1;transform:scale(1.12)}30%{opacity:.55;transform:scale(0.88)}50%{opacity:1;transform:scale(1.1)}80%{opacity:.6;transform:scale(0.9)}100%{opacity:1;transform:scale(1.12)}}
    .sb-item .ts{font-size:11px;color:var(--text-dim);margin-top:2px}
    .sb-item .ts-row{display:flex;align-items:center;justify-content:space-between;gap:6px;margin-top:2px;min-height:0}
    .sb-item .ts-row .ts{margin-top:0;flex:1;min-width:0;overflow-x:auto;overflow-y:hidden;white-space:nowrap;-webkit-overflow-scrolling:touch}
    .mini-sparkline-wrapper{display:flex;align-items:flex-end;gap:2px;height:14px;width:40px;flex-shrink:0;padding-bottom:2px;will-change:transform;transform:translateZ(0);filter:drop-shadow(0 0 4px rgba(188,19,254,.5));contain:layout style paint}
    .spark-bar{flex:1;min-width:2px;min-height:2px;background:#bc13fe;border-radius:1px;box-shadow:0 0 4px rgba(188,19,254,.5);transition:height .25s ease}
    .sb-item.pinned .spark-bar{animation:spark-pulse 2s ease-in-out infinite}
    .sb-item.pinned .mini-sparkline-wrapper{filter:drop-shadow(0 0 5px rgba(188,19,254,.6))}
    .mini-sparkline-wrapper{cursor:pointer;border-radius:4px;padding:2px}
    @keyframes spark-pulse{0%,100%{filter:brightness(1);opacity:1}50%{filter:brightness(1.35);opacity:.95}}
    /* Activity Breakdown overlay (tap sparkline) — Gemini concept */
    .activity-breakdown-overlay{display:none;position:fixed;inset:0;z-index:60;align-items:center;justify-content:center;padding:16px}
    .activity-breakdown-overlay.show{display:flex}
    .activity-breakdown-backdrop{position:absolute;inset:0;background:rgba(0,0,0,.6);cursor:pointer}
    .activity-breakdown-card{position:relative;width:min(320px,100%);max-height:85vh;overflow-y:auto;background:var(--surface);border:2px solid var(--pink);border-radius:16px;box-shadow:0 0 24px rgba(255,122,217,.35),0 0 48px rgba(188,19,254,.2);padding:16px}
    .activity-breakdown-title{font-size:16px;font-weight:700;color:var(--text);margin:0 0 12px;padding-right:28px}
    .activity-title-history{padding:10px 12px;margin-bottom:8px;border-radius:10px;background:rgba(40,20,60,.5);border:1px solid rgba(255,122,217,.2);display:flex;flex-direction:column;gap:4px}
    .activity-title-history-label{font-size:10px;font-weight:700;text-transform:uppercase;letter-spacing:.08em;color:var(--text-dim)}
    .activity-title-history-list{font-size:13px;color:var(--text);line-height:1.4}
    .activity-title-history-date{font-size:11px;color:var(--text-dim);font-weight:normal}
    .activity-buckets{display:flex;flex-direction:column;gap:10px}
    .activity-bucket{display:flex;flex-direction:column;gap:4px;padding:10px 12px;border-radius:12px;background:rgba(30,16,48,.6);border:1px solid var(--border);cursor:pointer;transition:transform .1s,box-shadow .15s}
    .activity-bucket:hover,.activity-bucket:active{transform:scale(1.02);box-shadow:0 0 12px rgba(188,19,254,.25)}
    .activity-bucket.expanded{background:rgba(40,20,60,.8)}
    .activity-bucket-label{font-size:11px;font-weight:700;text-transform:uppercase;letter-spacing:.06em;color:var(--text-dim)}
    .activity-bucket-count{font-size:18px;font-weight:700;color:var(--text)}
    /* Tree branch (bucket tree) + flower petals (pedals) — branches as mask, random petals as nodes */
    .activity-tree-wrap{position:relative;margin:8px 0 4px;padding:6px 0;min-height:56px}
    .activity-branch-wrap .activity-branch-bg{position:absolute;left:0;right:0;top:0;bottom:0;width:100%;height:100%;-webkit-mask-size:contain;mask-size:contain;-webkit-mask-repeat:no-repeat;mask-repeat:no-repeat;-webkit-mask-position:center;mask-position:center;opacity:.75;filter:drop-shadow(0 0 6px var(--tree-color))}
    .activity-branch-wrap .activity-branch-nodes{position:absolute;left:0;right:0;top:0;bottom:0;z-index:1;pointer-events:none}
    .activity-branch-wrap .activity-tree-node{position:absolute;width:24px;height:24px;transform:translate(-50%,-50%);display:flex;align-items:center;justify-content:center;padding:0;opacity:.35;transition:opacity .3s ease,filter .3s ease}
    .activity-branch-wrap .activity-node-flower{display:block;width:20px;height:20px;object-fit:contain;opacity:.4;transition:opacity .3s ease,filter .3s ease,transform .2s ease}
    .activity-branch-wrap .activity-tree-node.lit{opacity:1}
    .activity-branch-wrap .activity-tree-node.lit .activity-node-flower{opacity:1;filter:drop-shadow(0 0 6px var(--node-color)) drop-shadow(0 0 12px var(--node-color)) drop-shadow(0 0 4px var(--node-color))}
    .activity-branch-wrap.tree-full .activity-tree-node.lit .activity-node-flower{animation:flower-sparkle 1.2s ease-in-out infinite}
    @keyframes flower-sparkle{0%,100%{filter:drop-shadow(0 0 6px var(--node-color)) drop-shadow(0 0 12px var(--node-color))}50%{filter:drop-shadow(0 0 10px var(--node-color)) drop-shadow(0 0 20px var(--node-color)) drop-shadow(0 0 8px var(--node-color))}}
    /* Falling neon sparks/glitter when tree is full (7/7) */
    .activity-tree-sparks{position:absolute;inset:0;pointer-events:none;overflow:hidden}
    .tree-spark{position:absolute;top:-6px;width:3px;height:3px;border-radius:50%;background:var(--tree-color);box-shadow:0 0 6px var(--tree-color),0 0 10px var(--tree-color);opacity:0;animation:spark-fall 1.4s ease-in forwards}
    @keyframes spark-fall{0%{opacity:0;transform:translateY(0) scale(1)}15%{opacity:.95;transform:translateY(8px) scale(1.2)}85%{opacity:.6;transform:translateY(55px) scale(0.8)}100%{opacity:0;transform:translateY(70px) scale(0.5)}}
    .activity-bucket-detail{font-size:13px;color:var(--text-dim);margin-top:6px;display:none}
    .activity-bucket.expanded .activity-bucket-detail{display:block}
    .activity-breakdown-close{position:absolute;top:12px;right:12px;width:28px;height:28px;border:none;border-radius:50%;background:var(--surface2);color:var(--text);font-size:20px;line-height:1;cursor:pointer;display:flex;align-items:center;justify-content:center}
    .activity-breakdown-close:hover{background:var(--border);color:var(--pink)}
    /* Pin left, title center, delete right; .acts kept for archive restore layout */
    .sb-item .acts{display:flex;gap:8px;flex-shrink:0;align-items:center}
    /* â"€â"€ Expandable 4-way actions: one button (four-round icon) expands into Pin, Star, Rename, Delete â"€â"€ */
    .sb-item-actions-wrap{position:relative;width:32px;height:32px;flex-shrink:0}
    .sb-actions-trigger{position:absolute;top:0;left:0;width:32px;height:32px;min-width:32px;min-height:32px;padding:0;display:flex;align-items:center;justify-content:center;border-radius:8px;border:2px solid transparent;background:linear-gradient(#06000f,#06000f) padding-box,linear-gradient(135deg,#ff44e0,#c030e8,#e050ff,#ff88f0,#9020d0,#ff44e0) border-box;background-origin:padding-box,border-box;background-clip:padding-box,border-box;cursor:pointer;box-shadow:0 0 8px rgba(255,68,224,.4),0 0 16px rgba(200,50,240,.35),0 0 24px rgba(160,40,220,.25),inset 0 0 10px rgba(180,30,220,.1);transition:opacity .2s,transform .2s,box-shadow .25s}
    .sb-actions-trigger:hover{box-shadow:0 0 12px rgba(255,136,240,.5),0 0 22px rgba(255,100,238,.45)}
    .sb-actions-trigger:active{transform:scale(0.94)}
    .sb-actions-trigger.sparkle{animation:sb-trigger-sparkle .4s ease-out}
    @keyframes sb-trigger-sparkle{0%{box-shadow:0 0 8px rgba(255,68,224,.4),0 0 16px rgba(200,50,240,.35)}50%{box-shadow:0 0 20px rgba(255,136,255,.9),0 0 36px rgba(255,80,240,.7),0 0 48px rgba(230,50,255,.5)}100%{box-shadow:0 0 8px rgba(255,68,224,.4),0 0 16px rgba(200,50,240,.35)}}
    .sb-actions-trigger-icon{width:20px;height:20px;object-fit:contain;filter:invert(0.55) sepia(0.5) saturate(4) hue-rotate(280deg) drop-shadow(0 0 2px rgba(255,136,230,.9))}
    .sb-item-actions-wrap.overlay-open .sb-actions-trigger{opacity:0;pointer-events:none}
    .sb-actions-four{display:none}
    .sb-actions-overlay{position:fixed;inset:0;z-index:100;pointer-events:none;opacity:0;visibility:hidden;transition:opacity .28s ease}
    .sb-actions-overlay.show{pointer-events:auto;opacity:1;visibility:visible}
    .sb-actions-overlay-backdrop{position:absolute;inset:0;cursor:default}
    .sb-actions-four-float{position:fixed;width:72px;height:72px;display:grid;grid-template-columns:1fr 1fr;grid-template-rows:1fr 1fr;gap:4px;padding:2px;pointer-events:auto}
    .sb-actions-four-float .act{width:32px;height:32px;min-width:32px;min-height:32px;max-width:32px;max-height:32px;-webkit-appearance:none;appearance:none;position:relative;border-radius:8px;border:2px solid transparent;background:linear-gradient(#06000f,#06000f) padding-box,linear-gradient(135deg,#ff44e0,#c030e8,#e050ff,#ff88f0,#9020d0,#ff44e0) border-box;background-origin:padding-box,border-box;background-clip:padding-box,border-box;display:flex;align-items:center;justify-content:center;font-size:14px;font-weight:900;line-height:1;cursor:pointer;color:#ff88e0;text-shadow:0 0 8px rgba(255,100,235,.9);box-shadow:0 0 8px rgba(255,68,224,.4),0 0 16px rgba(200,50,240,.35);transition:box-shadow .18s,transform .1s}
    .sb-actions-four-float .act:active{transform:scale(0.94)}
    .sb-actions-four-float .pin-btn{color:#ff99ee;background:linear-gradient(#06000f,#06000f) padding-box,linear-gradient(135deg,#ff66ee,#ff44e0,#e030ff,#c030e8,#ff88f0,#ff44e0) border-box;background-origin:padding-box,border-box;background-clip:padding-box,border-box}
    .sb-actions-four-float .star-btn{color:#ffcc66}
    .sb-actions-four-float .rename-btn{color:transparent;padding:0}
    .sb-actions-four-float .rename-btn .sb-actions-rename-icon{width:18px;height:18px;object-fit:contain;display:block;pointer-events:none;filter:drop-shadow(0 0 4px rgba(255,136,230,.8))}
    .sb-actions-four-float .del-btn{color:#c060f0;background:linear-gradient(#06000f,#06000f) padding-box,linear-gradient(135deg,#9020d8,#a030e0,#c040f0,#9020d8,#e050ff,#9020d8) border-box;background-origin:padding-box,border-box;background-clip:padding-box,border-box}
    .sb-actions-four .act{width:32px;height:32px;min-width:32px;min-height:32px;max-width:32px;max-height:32px}
    .sb-item .act.rename-btn{color:transparent;padding:0}
    .sb-item .act.rename-btn .sb-actions-rename-icon{width:18px;height:18px;object-fit:contain;display:block;pointer-events:none;filter:drop-shadow(0 0 4px rgba(255,136,230,.8))}
    /* â"€â"€ Neon card buttons: small, ends of row; gradient/shimmer border + glow â"€â"€ */
    .sb-item .act{
      -webkit-appearance:none;appearance:none;
      position:relative;
      width:32px;height:28px;min-width:32px;min-height:28px;max-width:32px;max-height:28px;
      border-radius:8px;
      border:2px solid transparent;
      background:linear-gradient(#06000f,#06000f) padding-box,linear-gradient(135deg,#ff44e0,#c030e8,#e050ff,#ff88f0,#9020d0,#ff44e0) border-box;
      background-origin:padding-box,border-box;background-clip:padding-box,border-box;
      display:flex;align-items:center;justify-content:center;
      font-size:14px;font-weight:900;line-height:1;cursor:pointer;flex-shrink:0;
      color:#ff88e0;
      text-shadow:0 0 8px rgba(255,100,235,.9),0 0 16px rgba(230,60,255,.55);
      box-shadow:0 0 8px rgba(255,68,224,.4),0 0 16px rgba(200,50,240,.35),0 0 24px rgba(160,40,220,.25),inset 0 0 10px rgba(180,30,220,.1);
      transition:box-shadow .18s,color .15s,transform .1s}
    .sb-item .act:not(.restore-btn){flex-shrink:0}
    .sb-item .act:active{transform:scale(0.94)}
    .sb-item .pin-btn{
      color:#ff99ee;
      background:linear-gradient(#06000f,#06000f) padding-box,linear-gradient(135deg,#ff66ee,#ff44e0,#e030ff,#c030e8,#ff88f0,#ff44e0) border-box;
      background-origin:padding-box,border-box;background-clip:padding-box,border-box;
      box-shadow:0 0 8px rgba(255,68,224,.5),0 0 14px rgba(255,80,230,.4),0 0 22px rgba(200,50,240,.3),inset 0 0 10px rgba(255,60,220,.08)}
    .sb-item .del-btn{
      color:#c060f0;
      background:linear-gradient(#06000f,#06000f) padding-box,linear-gradient(135deg,#9020d8,#a030e0,#c040f0,#9020d8,#e050ff,#9020d8) border-box;
      background-origin:padding-box,border-box;background-clip:padding-box,border-box;
      box-shadow:0 0 8px rgba(144,32,216,.45),0 0 14px rgba(160,40,220,.35),0 0 22px rgba(180,50,240,.25),inset 0 0 10px rgba(140,20,200,.08)}
    .sb-item .pin-btn:hover,.sb-item .del-btn:hover{
      color:#fff;
      text-shadow:0 0 12px #fff,0 0 22px rgba(255,120,240,.8);
      box-shadow:0 0 10px rgba(255,136,240,.5),0 0 18px rgba(255,100,238,.45),0 0 30px rgba(255,80,240,.4),0 0 48px rgba(220,50,240,.25),inset 0 0 16px rgba(255,100,245,.1)}
    .sb-item .act.pinned{
      color:#ffc0f8;
      text-shadow:0 0 8px rgba(255,180,255,.9),0 0 18px rgba(255,140,245,.6);
      background:linear-gradient(#06000f,#06000f) padding-box,linear-gradient(135deg,#ff88f0,#ff66ee,#ff44e0,#e030ff,#ff88f0,#ff44e0) border-box;
      background-origin:padding-box,border-box;background-clip:padding-box,border-box;
      box-shadow:0 0 10px rgba(255,136,240,.5),0 0 18px rgba(255,100,240,.4),0 0 28px rgba(255,80,230,.3),inset 0 0 12px rgba(255,120,250,.1)}
    .sb-item .star-btn{min-width:24px;width:24px;height:24px;padding:0;font-size:13px;line-height:1;color:var(--text-dim)}
    .sb-item .star-btn.starred{color:#ffb830;text-shadow:0 0 6px rgba(255,180,80,.7)}
    .sb-item .star-btn:hover{color:#ffc850;text-shadow:0 0 8px rgba(255,200,100,.8)}
    .sb-item .act.restore-btn{min-width:auto;width:auto;padding:0 18px;font-size:14px;font-weight:700;min-height:40px;border-radius:0;color:#ff99ee}
    .badge{font-size:10px;padding:2px 6px;border-radius:4px;white-space:nowrap;flex-shrink:0;background:var(--border);color:var(--text-dim)}
    .badge.mobile{background:#3a1430;color:var(--pink)}
    .badge.vscode{background:#0d2035;color:#69c}
    .badge.grok{background:#0d2010;color:#6d6}
    .badge.cursor{background:#251510;color:#c96}
    /* â"€â"€ Header (TV girl avatar removed for now) â"€â"€ */
    /* Header: respect iOS notch/status bar so no empty black at top */
    #hdr{background:var(--surface);border-bottom:1px solid var(--border);padding:6px 12px;padding-top:max(10px,env(safe-area-inset-top));display:flex;align-items:center;gap:10px;flex-shrink:0}
    #menuBtn{background:none;border:none;color:var(--text);cursor:pointer;font-size:20px;padding:6px;line-height:1;flex-shrink:0;border-radius:8px;-webkit-appearance:none;appearance:none;transition:background .15s,color .15s}
    #menuBtn:hover,#menuBtn:active{background:var(--surface2);color:var(--pink)}
    #menuBtn:focus-visible{outline:2px solid var(--pink);outline-offset:2px}
    img{max-width:100%;box-sizing:border-box}
    .bubble img{max-width:100%;height:auto;border-radius:8px;display:block}
    .chat-img-wrap{{margin-bottom:8px;}}
    .bubble .chat-img{{max-width:220px;max-height:200px;width:auto;height:auto;object-fit:cover;border-radius:12px;cursor:pointer;display:block;transition:opacity .2s,transform .15s;animation:chat-img-fade .35s ease-out}}
    .bubble .chat-img:active{{opacity:.92}}
    @keyframes chat-img-fade{{0%{{opacity:0}}100%{{opacity:1}}}}
    .chat-attach-row{{display:flex;flex-wrap:wrap;gap:8px;margin-bottom:8px;align-items:center}}
    .chat-attach-chip{{display:inline-flex;align-items:center;gap:6px;padding:6px 10px;border-radius:10px;background:rgba(0,0,0,.12);font-size:13px;font-weight:500}}
    .msg-wrap.user .chat-attach-chip{{background:rgba(0,0,0,.18)}}
    .chat-attach-icon{{width:20px;height:20px;flex-shrink:0;object-fit:contain}}
    #hdrInfo{flex:1;min-width:0}
    #hdrName{font-size:16px;font-weight:700;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
    #hdrSub{font-size:12px;color:var(--text-dim);margin-top:1px;white-space:nowrap;overflow-x:auto;overflow-y:hidden;-webkit-overflow-scrolling:touch;min-width:0}
    #headerNewBtn{flex-shrink:0}
    .hdr-actions{display:flex;align-items:center;gap:10px;flex-shrink:0}
    .hdr-copy-export-wrap{display:inline-flex;position:relative}
    .hdr-copy-export-dropdown{position:absolute;right:0;top:100%;margin-top:4px;min-width:180px;background:var(--surface);border:1px solid var(--border);border-radius:10px;box-shadow:0 8px 24px rgba(0,0,0,.4);z-index:1000;padding:6px 0;display:none;flex-direction:column}
    .hdr-copy-export-dropdown.open{display:flex}
    .hdr-dropdown-item{display:block;width:100%;text-align:left;padding:10px 14px;border:none;background:none;color:var(--text);font-size:14px;cursor:pointer;transition:background .15s;-webkit-appearance:none;appearance:none;font-family:inherit}
    .hdr-dropdown-item:hover,.hdr-dropdown-item:active{background:rgba(255,122,217,.15);color:var(--pink)}
    .hdr-copy-export-btn[data-copied="1"]::after{content:"Copied!";position:absolute;bottom:100%;left:50%;transform:translateX(-50%);margin-bottom:4px;padding:4px 8px;font-size:11px;font-weight:700;background:var(--pink);color:#111;border-radius:6px;white-space:nowrap;pointer-events:none;animation:fade-out-up .2s ease .8s forwards}
    .hdr-icon-btn{flex-shrink:0;padding:6px;margin:0;font-size:0;background:transparent;border-radius:10px;border:1px solid var(--pink);color:var(--pink);cursor:pointer;-webkit-appearance:none;appearance:none;display:inline-flex;align-items:center;justify-content:center;position:relative;transition:background .15s,border-color .15s,color .15s}
    .hdr-icon-btn:hover,.hdr-icon-btn:active{background:var(--pink);color:#111}
    .hdr-icon-btn:focus-visible{outline:2px solid var(--pink);outline-offset:2px}
    .hdr-icon-btn svg,.hdr-icon-btn .hdr-fork-icon{width:18px;height:18px;display:block;flex-shrink:0}
    .hdr-icon-btn svg,.hdr-icon-btn .hdr-fork-icon{filter:brightness(0) saturate(100%) invert(72%) sepia(49%) saturate(1200%) hue-rotate(290deg);}
    .hdr-icon-btn:hover svg,.hdr-icon-btn:hover .hdr-fork-icon,.hdr-icon-btn:active svg,.hdr-icon-btn:active .hdr-fork-icon{filter:brightness(0) saturate(100%);}
    .hdr-icon-btn[data-copied="1"]::after{content:"Copied!";position:absolute;bottom:100%;left:50%;transform:translateX(-50%);margin-bottom:4px;padding:4px 8px;font-size:11px;font-weight:700;background:var(--pink);color:#111;border-radius:6px;white-space:nowrap;pointer-events:none;animation:fade-out-up .2s ease .8s forwards}
    @keyframes fade-out-up{to{opacity:0;transform:translateX(-50%) translateY(-4px)}}
    /* â"€â"€ Chat area â"€â"€ (extra bottom padding so last message isn't clipped above input/tab bar on iPhone) */
    #chatArea{flex:1;min-height:0;overflow-y:auto;overflow-x:hidden;-webkit-overflow-scrolling:touch;overscroll-behavior-y:contain;padding:14px max(16px,env(safe-area-inset-right)) calc(24px + max(16px,env(safe-area-inset-bottom))) max(16px,env(safe-area-inset-left));display:flex;flex-direction:column;gap:2px}
    body.bubble-no-wrap #chatArea{overflow-x:auto}
    .msg-wrap{display:flex;flex-direction:column;margin-bottom:4px}
    .msg-wrap.user{align-items:flex-end}
    .msg-wrap.assistant{align-items:flex-start}
    .msg-row{display:flex;align-items:center;gap:8px;max-width:82%}
    .msg-wrap.user .msg-row{flex-direction:row-reverse}
    .msg-av{width:40px;height:40px;min-width:40px;max-width:40px !important;min-height:40px;max-height:40px !important;border-radius:0;overflow:visible;flex-shrink:0;margin-bottom:2px;background:transparent;position:relative;contain:none;display:flex;align-items:center;justify-content:center}
    .msg-av img{width:40px !important;height:40px !important;max-width:40px !important;max-height:40px !important;min-width:0;object-fit:contain;display:block;vertical-align:top}
    .msg-av.user-av{display:flex;align-items:center;justify-content:center;font-size:14px;color:#111;width:72px;height:72px;min-width:72px;max-width:72px !important;min-height:72px;max-height:72px !important;border-radius:50%;background:radial-gradient(ellipse 120% 120% at 50% 25%,rgba(255,140,200,0.4) 0%,rgba(255,120,190,0.22) 20%,rgba(255,100,180,0.1) 45%,rgba(255,80,160,0.04) 70%,transparent 100%);padding:0;box-shadow:none}
    .msg-av.user-av img{width:64px !important;height:64px !important;max-width:64px !important;max-height:64px !important}
    .bubble{padding:10px 14px;border-radius:18px;font-size:15px;line-height:1.52;white-space:pre-wrap;word-break:break-word;overflow-wrap:break-word;letter-spacing:.01em;max-width:100%;min-width:0}
    .bubble pre,.bubble code,.bubble .bubble-text pre,.bubble .bubble-text code{white-space:pre-wrap !important;word-break:break-word;overflow-wrap:break-word;max-width:100%}
    .copyable-block{max-width:100%;min-width:0;overflow-wrap:break-word}
    .copyable-block pre,.copyable-block code{white-space:pre-wrap !important;word-break:break-word;overflow-wrap:break-word}
    body.bubble-no-wrap .bubble{white-space:pre-wrap;word-break:normal;overflow-wrap:normal}
    body.bubble-no-wrap .bubble pre,body.bubble-no-wrap .bubble code,body.bubble-no-wrap .bubble .bubble-text pre,body.bubble-no-wrap .bubble .bubble-text code{white-space:pre !important;word-break:normal;overflow-wrap:normal;overflow-x:auto}
    body.bubble-no-wrap .copyable-block{overflow-wrap:normal;overflow-x:auto}
    body.bubble-no-wrap .copyable-block pre,body.bubble-no-wrap .copyable-block code{white-space:pre !important;overflow-x:auto}
    .msg-wrap.user .bubble{background:var(--ruby-bg);color:var(--ruby-text);border-bottom-right-radius:4px}
    .msg-wrap.assistant .bubble{background:var(--locus-bg);color:var(--locus-text);border-bottom-left-radius:4px;border:1px solid var(--locus-border)}
    .msg-wrap.assistant.thinking-msg .bubble{font-size:14px;opacity:.95;border-color:rgba(80,200,180,.4)}
    .thinking-details{width:100%;max-width:100%;margin:0}
    .thinking-summary{font-size:13px;font-weight:600;color:var(--text-dim);cursor:pointer;list-style:none;padding:4px 0;user-select:none}
    .thinking-summary::-webkit-details-marker{display:none}
    .thinking-summary::before{content:'\u25B8';display:inline-block;margin-right:6px;transition:transform .2s}
    .thinking-details[open] .thinking-summary::before{transform:rotate(90deg)}
    .thinking-readout{background:rgba(40,40,50,.85);color:rgba(220,220,230,.95);font-size:12px;line-height:1.5;white-space:pre-wrap;word-break:break-word;max-height:280px;overflow-y:auto;padding:10px 12px;border-radius:10px;margin-top:6px;border:1px solid rgba(80,200,180,.25);font-family:ui-monospace,SFMono-Regular,Consolas,monospace}
    mark.search-highlight{background:rgba(255,122,217,.35);color:inherit;border-radius:3px;padding:0 2px;box-shadow:0 0 12px rgba(255,122,217,.6),0 0 4px rgba(255,122,217,.4)}
    .chat-search-bar{display:none;align-items:center;gap:8px;padding:8px 12px;background:var(--surface2);border-bottom:1px solid var(--locus-border);flex-shrink:0}
    .chat-search-bar.open{display:flex}
    .chat-search-bar input{flex:1;min-width:0;padding:8px 12px;border-radius:10px;border:1px solid var(--locus-border);background:var(--locus-bg);color:var(--locus-text);font-size:14px}
    .chat-search-label{font-size:12px;color:var(--text-dim);white-space:nowrap}
    .chat-search-bar button{padding:6px 10px;border-radius:8px;border:1px solid var(--locus-border);background:var(--locus-bg);color:var(--locus-text);cursor:pointer;font-size:14px}
    .chat-search-bar button:hover:not(:disabled){opacity:.9}
    .chat-search-bar button:disabled{opacity:.5;cursor:default}
    mark.chat-search-mark{background:rgba(255,122,217,.4);color:inherit;border-radius:2px;padding:0 1px}
    .msg-wrap.long-bubble .bubble{max-height:12em;overflow:hidden;transition:max-height .25s ease}
    .msg-wrap.long-bubble.expanded .bubble{max-height:none}
    .msg-row.msg-expand-row{margin-top:2px}
    .msg-expand-btn{font-size:12px;padding:4px 10px;border-radius:8px;border:1px solid rgba(255,122,217,.35);background:transparent;color:var(--pink);cursor:pointer}
    .msg-expand-btn:hover{background:rgba(255,122,217,.15)}
    .msg-wrap.cascade-in{opacity:0;transform:translateY(-8px);animation:msg-cascade-in .28s ease-out forwards}
    @keyframes msg-cascade-in{0%{opacity:0;transform:translateY(-8px)}100%{opacity:1;transform:translateY(0)}}
    .msg-time{font-size:11px;color:var(--text-dim);padding:1px 6px}
    /* â"€â"€ In-chat quick replies (multiple choice on iPhone) â"€â"€ */
    .msg-row.quick-replies{align-items:center;margin-top:4px}
    .quick-reply-wrap{display:flex;flex-wrap:wrap;gap:8px;max-width:82%}
    .quick-reply-btn{font-size:14px;padding:8px 14px;border-radius:14px;border:1px solid var(--locus-border);background:var(--locus-bg);color:var(--locus-text);cursor:pointer;transition:opacity .15s}
    .quick-reply-btn:hover{opacity:.9}
    .chat-doc-cards{display:flex;flex-direction:column;gap:10px;max-width:82%;margin-top:10px}
    .chat-doc-card{border:1px solid var(--locus-border);border-radius:12px;background:rgba(40,20,50,.4);padding:12px 14px}
    .chat-doc-card h4{font-size:13px;font-weight:700;color:var(--pink);margin:0 0 6px;display:flex;align-items:center;gap:8px}
    .chat-doc-type{font-size:10px;text-transform:uppercase;opacity:.85;font-weight:600}
    .chat-doc-preview{font-size:13px;line-height:1.45;color:var(--locus-text);white-space:pre-wrap;max-height:80px;overflow:hidden;margin-bottom:10px}
    .chat-doc-actions{display:flex;gap:8px;flex-wrap:wrap}
    .chat-doc-edit-btn,.chat-doc-files-btn{font-size:12px;padding:6px 12px;border-radius:10px;border:1px solid var(--pink);background:transparent;color:var(--pink);cursor:pointer;font-weight:600;-webkit-appearance:none;appearance:none}
    .chat-doc-edit-btn:hover,.chat-doc-files-btn:hover{background:var(--pink);color:#111}
    .draft-editor-overlay{position:fixed;inset:0;background:rgba(0,0,0,.7);z-index:2000;display:flex;align-items:center;justify-content:center;padding:16px}
    .draft-editor-modal{background:var(--locus-bg);border:1px solid var(--locus-border);border-radius:16px;max-width:100%;width:480px;max-height:85vh;display:flex;flex-direction:column;overflow:hidden}
    .draft-editor-modal h3{padding:14px 16px;margin:0;font-size:16px;border-bottom:1px solid var(--locus-border)}
    .draft-editor-modal textarea{flex:1;min-height:200px;padding:14px;font-size:14px;line-height:1.5;resize:vertical;border:none;background:transparent;color:var(--locus-text)}
    .draft-editor-modal .modal-actions{padding:12px 16px;display:flex;gap:10px;justify-content:flex-end;border-top:1px solid var(--locus-border)}
    .settings-panel-overlay{position:fixed;inset:0;background:rgba(0,0,0,.7);z-index:2000;display:none;align-items:center;justify-content:center;padding:16px}
    .settings-panel-overlay.show{display:flex}
    .settings-panel-modal{background:var(--locus-bg);border:1px solid var(--locus-border);border-radius:16px;max-width:100%;width:400px;max-height:85vh;display:flex;flex-direction:column;overflow:hidden;box-shadow:0 0 24px rgba(0,0,0,.3)}
    .settings-panel-modal h3{padding:14px 16px;margin:0;font-size:16px;border-bottom:1px solid var(--locus-border);color:var(--text)}
    .settings-panel-modal .settings-body{overflow-y:auto;padding:14px 16px;display:flex;flex-direction:column;gap:12px}
    .settings-panel-modal .settings-row{display:flex;align-items:center;justify-content:space-between;gap:12px}
    .settings-panel-modal .settings-row label{font-size:14px;color:var(--locus-text);cursor:pointer;flex:1}
    .settings-panel-modal .settings-row input[type="number"]{width:72px;padding:6px 10px;border-radius:8px;border:1px solid var(--locus-border);background:var(--surface);color:var(--text);font-size:14px}
    .settings-panel-modal .modal-actions{padding:12px 16px;display:flex;gap:10px;justify-content:flex-end;border-top:1px solid var(--locus-border)}
    .settings-panel-modal .settings-dev-section{margin-top:8px;padding-top:12px;border-top:1px solid var(--locus-border)}
    .settings-panel-modal .settings-dev-title{font-size:12px;font-weight:700;text-transform:uppercase;letter-spacing:.05em;color:var(--text-dim);margin-bottom:6px}
    .settings-panel-modal .settings-row-actions{display:flex;gap:8px;flex-wrap:wrap}
    .settings-panel-modal .settings-dev-btn{font-size:12px;padding:6px 12px;border-radius:8px;border:1px solid var(--locus-border);background:var(--surface);color:var(--locus-text);cursor:pointer;-webkit-appearance:none;appearance:none}
    .settings-panel-modal .settings-dev-btn:hover{background:rgba(255,122,217,.12);color:var(--pink);border-color:rgba(255,122,217,.35)}
    .quick-reply-btn.used{opacity:.6;cursor:default;pointer-events:none}
    .quick-reply-wrap.quick-replies-used .quick-reply-btn{opacity:.55;cursor:default;pointer-events:none;filter:grayscale(.4)}
    .quick-reply-wrap.quick-replies-used .quick-reply-btn.chosen{opacity:.75;filter:grayscale(0);border-color:rgba(255,122,217,.4)}
    .msg-wrap.quick-reply-choice .bubble{opacity:.95;border-left:3px solid rgba(255,122,217,.5)}
    /* â"€â"€ Floating thoughts (2—3 lightweight options) â"€â"€ */
    .floating-thoughts-row{margin-top:2px}
    .floating-thoughts{font-size:13px;color:var(--locus-text);opacity:.85;max-width:82%;display:flex;flex-wrap:wrap;align-items:center;gap:0}
    .floating-thought-sep{opacity:.6;user-select:none;pointer-events:none}
    .floating-thought-link{cursor:pointer;border:none;background:none;padding:0;font:inherit;color:inherit;text-decoration:none;border-radius:2px}
    .floating-thought-link:hover{text-decoration:underline;opacity:1}
    .floating-thought-link:focus{outline:1px solid rgba(255,122,217,.5);outline-offset:2px}
    .floating-thoughts-used .floating-thought-link{cursor:default;pointer-events:none;opacity:.5}
    .floating-thoughts-used .floating-thought-link.chosen{opacity:1;color:var(--pink);font-weight:500}
    /* â"€â"€ Edit message & fork (start new thread from here, Grok/Cursor-style) â"€â"€ */
    .msg-actions{margin-top:2px}
    .msg-wrap.user .msg-actions{flex-direction:row-reverse}
    .bubble-actions{max-width:82%;display:flex;justify-content:flex-end;gap:8px}
    .msg-wrap.user .bubble-actions{justify-content:flex-end}
    .edit-msg-btn-wrap{position:relative;display:inline-flex}
    .edit-msg-btn{display:inline-flex;align-items:center;justify-content:center;padding:4px 6px;border-radius:8px;border:1px solid rgba(255,122,217,.25);background:rgba(30,16,48,.4);color:var(--pink);opacity:.7;cursor:pointer;transition:opacity .15s,color .15s,border-color .15s,box-shadow .15s,transform .2s}
    .edit-msg-btn svg{flex-shrink:0;filter:drop-shadow(0 0 3px rgba(255,122,217,.4))}
    .edit-msg-btn:hover,.edit-msg-btn:focus{opacity:1;color:#ff99ee;border-color:rgba(255,122,217,.5);box-shadow:0 0 6px rgba(255,122,217,.25)}
    .edit-msg-btn.popping{opacity:1;transform:scale(1.15);pointer-events:none;border-color:var(--pink);box-shadow:0 0 10px rgba(255,122,217,.5)}
    .edit-msg-btn.popping svg{filter:drop-shadow(0 0 6px rgba(255,122,217,.7))}
    .icon-bubble-neon,.icon-bubbles-pop-neon,.icon-copy-neon{display:block}
    .copy-msg-btn{display:inline-flex;align-items:center;justify-content:center;padding:4px 6px;border-radius:8px;border:1px solid rgba(255,122,217,.25);background:rgba(30,16,48,.4);color:var(--pink);opacity:.7;cursor:pointer;transition:opacity .15s,color .15s,border-color .15s;font-size:12px}
    .copy-msg-btn svg{flex-shrink:0;filter:drop-shadow(0 0 3px rgba(255,122,217,.4))}
    .copy-msg-btn:hover,.copy-msg-btn:focus{opacity:1;color:#ff99ee;border-color:rgba(255,122,217,.5)}
    .copy-msg-btn .copy-feedback{font-weight:600;color:#ff99ee}
    .feedback-btn{display:inline-flex;align-items:center;justify-content:center;padding:4px 6px;border-radius:8px;border:1px solid rgba(255,122,217,.25);background:rgba(30,16,48,.4);color:var(--pink);opacity:.7;cursor:pointer;transition:opacity .15s,color .15s;font-size:14px}
    .feedback-btn:hover{opacity:1;color:#ff99ee}
    .feedback-btn.feedback-sent{opacity:1;cursor:default;background:rgba(30,16,48,.4)}
    .feedback-btn.feedback-up.feedback-sent{color:#b8b830;border-color:rgba(180,180,48,.8);box-shadow:0 0 6px rgba(180,180,48,.5)}
    .feedback-btn.feedback-down.feedback-sent{color:#e06030;border-color:rgba(224,80,48,.8);box-shadow:0 0 6px rgba(224,80,48,.5)}
    .variant-picker{display:inline-flex;align-items:center;gap:4px;margin-right:6px}
    .variant-picker-btn{width:26px;height:26px;padding:0;border-radius:6px;border:1px solid rgba(255,122,217,.3);background:rgba(30,16,48,.5);color:var(--pink);font-size:14px;cursor:pointer;display:flex;align-items:center;justify-content:center;-webkit-appearance:none;appearance:none;transition:opacity .15s,color .15s}
    .variant-picker-btn:hover{opacity:1;color:#ff99ee;border-color:rgba(255,122,217,.5)}
    .variant-picker-label{font-size:12px;font-weight:600;color:var(--text-dim);min-width:2.2em;text-align:center}
    .pop-sparkles{position:absolute;inset:-8px;pointer-events:none}
    .pop-sparkle-dot{position:absolute;width:4px;height:4px;border-radius:50%;background:var(--pink);box-shadow:0 0 6px var(--pink);opacity:0;animation:pop-sparkle-burst .5s ease-out forwards}
    @keyframes pop-sparkle-burst{0%{opacity:0;transform:scale(0) translate(0,0)}40%{opacity:1;transform:scale(1.2) translate(0,0)}100%{opacity:0;transform:scale(0.6) translate(var(--sparkle-dx,0),var(--sparkle-dy,0))}}
    /* â"€â"€ Poof dismiss: cloud + pink/yellow sparkles where something disappears â"€â"€ */
    .poof-dismissing{opacity:0;pointer-events:none;transform:scale(0.97);transition:opacity .22s ease-out,transform .22s ease-out}
    .poof-overlay{position:fixed;pointer-events:none;z-index:99999;overflow:visible}
    .poof-cloud{position:absolute;border-radius:50%;background:radial-gradient(circle at center,rgba(255,200,230,.7) 0%,rgba(255,150,220,.4) 25%,rgba(255,200,120,.25) 45%,transparent 70%);opacity:0;animation:poof-cloud .55s ease-out forwards}
    .poof-sparkle{position:absolute;width:6px;height:6px;border-radius:50%;left:50%;top:50%;margin-left:-3px;margin-top:-3px;opacity:0;animation:poof-sparkle-cascade .6s ease-out forwards;box-shadow:0 0 8px currentColor}
    @keyframes poof-cloud{0%{opacity:0;transform:scale(0.3)}25%{opacity:1;transform:scale(1.15)}100%{opacity:0;transform:scale(1.8)}}
    @keyframes poof-sparkle-cascade{0%{opacity:0;transform:translate(0,0) scale(0)}15%{opacity:1;transform:translate(0,0) scale(1)}100%{opacity:0;transform:translate(var(--poof-dx,0),var(--poof-dy,0)) scale(0.4)}}
    .edit-msg-inline{width:100%;max-width:100%;margin-top:6px;transition:opacity .2s ease-out,transform .2s ease-out}
    .edit-msg-textarea{width:100%;max-width:100%;box-sizing:border-box;padding:10px 12px;border-radius:12px;border:2px solid var(--border);background:var(--bg);color:var(--text);font-size:14px;line-height:1.5;resize:vertical;min-height:80px;font-family:inherit;outline:none}
    .edit-msg-textarea:focus{border-color:var(--pink)}
    .edit-msg-buttons{display:flex;gap:10px;margin-top:8px;flex-wrap:wrap;align-items:center}
    .edit-msg-cancel,.edit-msg-fork,.edit-msg-send-here{width:26px;height:26px;min-width:26px;min-height:26px;padding:0;border-radius:50%;cursor:pointer;-webkit-appearance:none;appearance:none;display:inline-flex;align-items:center;justify-content:center;border:none;transition:box-shadow .2s,transform .18s}
    .edit-msg-cancel{background:transparent;position:relative;overflow:visible;box-shadow:none}
    .edit-msg-cancel:active{transform:scale(0.92)}
    .edit-msg-cancel::before{content:'';position:absolute;inset:-6px;border-radius:50%;background:radial-gradient(circle at center,rgba(255,248,120,.22) 0%,rgba(220,235,100,.12) 40%,transparent 65%);border:1px solid rgba(200,220,100,.28);pointer-events:none;z-index:0}
    .edit-msg-cancel:hover::before{background:radial-gradient(circle at center,rgba(255,250,120,.3) 0%,rgba(220,235,100,.16) 40%,transparent 65%);border-color:rgba(200,220,100,.4)}
    .edit-msg-cancel .edit-msg-icon-horns{width:16px;height:16px;filter:brightness(1.15) sepia(0.4) saturate(3.5) hue-rotate(2deg);position:relative;z-index:1}
    .edit-msg-send-here{background:rgba(32,178,170,.85);box-shadow:0 0 8px rgba(32,178,170,.45),0 0 12px rgba(32,178,170,.25);border:2px solid rgba(32,178,170,.9)}
    .edit-msg-send-here:hover:not(:disabled){box-shadow:0 0 10px rgba(32,178,170,.6),0 0 14px rgba(32,178,170,.4)}
    .edit-msg-send-here .edit-msg-icon-pencil{width:12px;height:12px;opacity:.95}
    .edit-msg-send-here:hover:not(:disabled) .edit-msg-icon-pencil{opacity:1}
    .edit-msg-send-here.sparkle{animation:edit-msg-pencil-sparkle .4s ease-out}
    @keyframes edit-msg-pencil-sparkle{0%{transform:scale(1);box-shadow:0 0 8px rgba(32,178,170,.45),0 0 12px rgba(32,178,170,.25)}45%{transform:scale(1.12);box-shadow:0 0 12px rgba(32,178,170,.7),0 0 18px rgba(32,178,170,.45)}100%{transform:scale(1);box-shadow:0 0 8px rgba(32,178,170,.45),0 0 12px rgba(32,178,170,.25)}}
    .edit-msg-send-here:disabled{opacity:.6;cursor:not-allowed}
    .edit-msg-send-here:disabled[aria-label*="Sending"]{cursor:wait}
    .edit-msg-fork{background:var(--pink);border:2px solid var(--pink);color:#111}
    .edit-msg-fork svg{flex-shrink:0;width:12px;height:12px;color:rgba(255,255,255,.98);filter:drop-shadow(0 0 2px rgba(255,255,255,.5))}
    .edit-msg-fork:hover:not(:disabled){box-shadow:0 0 14px rgba(255,122,217,.55)}
    .edit-msg-fork:disabled{opacity:.7;cursor:wait}
    .edit-msg-fork:disabled svg{opacity:.9}
    /* â"€â"€ Typing indicator â"€â"€ */
    #typing{display:none;align-items:flex-end;gap:8px;margin-bottom:4px}
    #typing.show{display:flex}
    .typing-av{width:40px;height:40px;min-width:40px;max-width:40px !important;min-height:40px;max-height:40px !important;border-radius:0;overflow:visible;flex-shrink:0;position:relative;background:transparent}
    .typing-av img{width:40px !important;height:40px !important;max-width:40px !important;max-height:40px !important;min-width:0;object-fit:contain;display:block;vertical-align:top}
    .typing-bubble{background:var(--locus-bg);border:1px solid var(--locus-border);border-radius:18px;border-bottom-left-radius:4px;padding:12px 16px;display:flex;gap:8px;align-items:center}
    .dot{width:6px;height:6px;border-radius:50%;background:var(--pink);animation:boing 1.2s ease infinite}
    /* Context indicator in input row (word-count warning, Cursor-style) */
    .context-indicator{flex-shrink:0;display:flex;align-items:center;justify-content:center;color:var(--pink);opacity:.85;transition:transform .2s ease}
    .context-indicator.medium{opacity:1}
    .context-indicator.high{opacity:1;transform:scale(1.1)}
    .context-indicator-track{color:rgba(255,122,217,.25);stroke:currentColor}
    .context-indicator-arc{color:var(--pink);stroke:currentColor;transition:stroke-dashoffset .25s ease}
    .dot:nth-child(2){animation-delay:.2s}.dot:nth-child(3){animation-delay:.4s}
    @keyframes boing{0%,60%,100%{transform:translateY(0)}30%{transform:translateY(-7px)}}
    /* â"€â"€ Generating image (Cursor-style: gray box + dark wave loading bar) â"€â"€ */
    #generatingImage{display:none;align-items:flex-end;gap:8px;margin-bottom:4px}
    #generatingImage.show{display:flex}
    #generatingImage .generating-av{width:40px;height:40px;min-width:40px;max-width:40px !important;min-height:40px;max-height:40px !important;border-radius:0;overflow:visible;flex-shrink:0;background:transparent}
    #generatingImage .generating-av img{width:40px !important;height:40px !important;max-width:40px !important;max-height:40px !important;min-width:0;object-fit:contain;display:block;vertical-align:top}
    .generating-placeholder{background:var(--surface2);border:1px solid var(--border);border-radius:18px;border-bottom-left-radius:4px;overflow:hidden;min-width:200px;max-width:280px;min-height:120px;display:flex;flex-direction:column;align-items:stretch;justify-content:flex-end}
    .generating-wave-wrap{height:6px;background:rgba(0,0,0,.2);width:100%;overflow:hidden}
    .generating-wave{height:100%;width:40%;background:linear-gradient(90deg,transparent,rgba(80,40,120,.9),transparent);animation:generating-wave 1.4s ease-in-out infinite}
    @keyframes generating-wave{0%{transform:translateX(-120%)}100%{transform:translateX(320%)}}
    .generating-text{font-size:13px;padding:12px 14px;line-height:1.4;text-align:center;color:var(--text-dim);background:linear-gradient(90deg,var(--text-dim) 0%,var(--text-dim) 35%,var(--pink) 50%,var(--text-dim) 65%,var(--text-dim) 100%);background-size:200% 100%;background-position:100% 0;-webkit-background-clip:text;background-clip:text;-webkit-text-fill-color:transparent;animation:generating-text-shimmer 2s ease-in-out infinite}
    @keyframes generating-text-shimmer{0%{background-position:100% 0}100%{background-position:-100% 0}}
    /* â"€â"€ Empty state â"€â"€ */
    .empty-state{flex:1;display:flex;flex-direction:column;align-items:center;justify-content:center;gap:12px;color:var(--text-dim);padding:32px;text-align:center}
    .empty-state .em-icon{font-size:52px}
    .empty-state .em-title{font-size:19px;font-weight:700;color:var(--text)}
    .empty-state .em-sub{font-size:14px;line-height:1.5;max-width:280px}
    /* â"€â"€ Engagement: "Mark as important?" when you return to a chat â"€â"€ */
    .engagement-banner{display:none;flex-shrink:0;align-items:center;justify-content:center;gap:10px;flex-wrap:wrap;padding:10px 16px;background:rgba(50,20,80,.6);border-top:1px solid var(--border);border-bottom:1px solid rgba(255,122,217,.2);font-size:13px;color:var(--text)}
    .engagement-banner.show{display:flex}
    .engagement-banner-text{flex:1;min-width:0}
    .engagement-banner-yes,.engagement-banner-no{padding:6px 14px;border-radius:10px;font-size:13px;font-weight:600;cursor:pointer;-webkit-appearance:none;appearance:none}
    .engagement-banner-yes{background:var(--pink);color:#111;border:2px solid var(--pink)}
    .engagement-banner-yes:hover{box-shadow:0 0 10px rgba(255,122,217,.5)}
    .engagement-banner-yes:focus-visible,.engagement-banner-no:focus-visible{outline:2px solid var(--pink);outline-offset:2px}
    .engagement-banner-no{background:transparent;border:1px solid var(--border);color:var(--text-dim)}
    .engagement-banner-no:hover{color:var(--text);border-color:var(--text-dim)}
    /* â"€â"€ Read-only hint â"€â"€ */
    #roHint{background:var(--surface2);border-top:1px solid var(--border);padding:10px 16px;font-size:13px;color:var(--text-dim);text-align:center;flex-shrink:0;display:none;align-items:center;justify-content:center;gap:12px;flex-wrap:wrap}
    #forkBtn{background:var(--pink);color:#111;border:none;border-radius:20px;padding:6px 14px;font-size:13px;font-weight:700;cursor:pointer;white-space:nowrap;flex-shrink:0}
    #forkBtn:active{opacity:.8}
    .ro-fork-wrap{position:relative;display:inline-flex;align-items:center;flex-shrink:0}
    .ro-fork-btn{display:inline-flex;align-items:center;justify-content:center;width:36px;height:36px;border-radius:50%;background:var(--pink);color:#111;border:none;cursor:pointer;padding:0;flex-shrink:0;box-shadow:0 0 12px rgba(255,122,217,.35)}
    .ro-fork-btn .icon-bubbles-pop-neon{width:18px;height:18px;color:#111}
    .ro-fork-btn:hover,.ro-fork-btn:focus{opacity:.95;box-shadow:0 0 14px rgba(255,122,217,.5)}
    .ro-fork-btn:disabled{opacity:.6;cursor:not-allowed}
    .ro-fork-dropdown{position:absolute;top:100%;right:0;margin-top:6px;background:var(--surface2);border:1px solid var(--border);border-radius:10px;padding:10px 12px;min-width:160px;box-shadow:0 8px 24px rgba(0,0,0,.35);z-index:100}
    .ro-fork-dropdown-text{margin:0 0 10px;font-size:13px;color:var(--text);}
    .ro-fork-yes,.ro-fork-cancel{display:inline-block;margin-right:8px;padding:6px 12px;border-radius:8px;font-size:13px;font-weight:600;cursor:pointer;border:none}
    .ro-fork-yes{background:var(--pink);color:#111}
    .ro-fork-cancel{background:transparent;color:var(--text-dim);border:1px solid var(--border)}
    .ro-fork-cancel:hover{color:var(--text);border-color:var(--text-dim)}
    .ro-fork-label{font-weight:600;color:var(--pink);white-space:nowrap}
    /* â"€â"€ Thread switcher (1/2, 2/2 when conversation has branches) â"€â"€ */
    .thread-switcher{display:flex;align-items:center;justify-content:center;gap:12px;padding:8px 16px;background:rgba(40,20,50,.5);border-top:1px solid var(--border);flex-shrink:0}
    .thread-switcher-inline{display:flex;align-items:center;justify-content:center;gap:10px;padding:6px 12px;margin-top:8px;margin-left:0;background:rgba(40,20,50,.4);border-radius:10px;border:1px solid var(--border);flex-shrink:0}
    .thread-switcher-prev,.thread-switcher-next{width:36px;height:36px;border-radius:10px;border:1px solid var(--pink);background:transparent;color:var(--pink);font-size:18px;line-height:1;cursor:pointer;display:flex;align-items:center;justify-content:center;-webkit-appearance:none;appearance:none;transition:background .15s,color .15s}
    .thread-switcher-inline .thread-switcher-prev,.thread-switcher-inline .thread-switcher-next{width:32px;height:32px;font-size:16px}
    .thread-switcher-prev:hover,.thread-switcher-next:hover,.thread-switcher-prev:active,.thread-switcher-next:active{background:var(--pink);color:#111}
    .thread-switcher-prev:disabled,.thread-switcher-next:disabled{opacity:.5;cursor:not-allowed}
    .thread-switcher-label{font-size:14px;font-weight:700;color:var(--text);min-width:2.5em;text-align:center}
    /* â"€â"€ Input area â"€â"€ (extend surface below so no black bar between input and tab bar) */
    #inputArea{position:relative;background:var(--surface);border-top:3px solid rgba(255,122,217,.6);padding:10px 16px;padding-bottom:max(18px,env(safe-area-inset-bottom));padding-left:max(16px,env(safe-area-inset-left));padding-right:max(16px,env(safe-area-inset-right));display:flex;flex-direction:column;flex-shrink:0;min-height:52px;transition:border-color .2s,box-shadow .2s}
    #inputArea::after{content:'';position:absolute;top:100%;left:0;right:0;height:100px;background:var(--surface);z-index:-1}
    #inputArea.mode-bestie{border-top-color:rgba(255,122,217,.7);box-shadow:0 -2px 12px rgba(255,122,217,.15)}
    #inputArea.mode-therapist{border-top-color:rgba(80,200,180,.75);box-shadow:0 -2px 12px rgba(80,200,180,.2)}
    #inputArea.mode-learning{border-top-color:rgba(255,180,80,.75);box-shadow:0 -2px 12px rgba(255,180,80,.2)}
    #inputArea.mode-ai_tasks{border-top-color:rgba(140,120,255,.75);box-shadow:0 -2px 12px rgba(140,120,255,.2)}
    .pasted-prompt-wrap{margin:0 0 8px;padding:10px 12px;border-radius:12px;background:rgba(255,122,217,.08);border:1px solid rgba(255,122,217,.3)}
    .pasted-prompt-preview{font-size:13px;line-height:1.4;color:var(--text);white-space:pre-wrap;word-break:break-word;max-height:4.2em;overflow:hidden}
    .pasted-prompt-label{display:inline-block;margin-top:6px;padding:3px 10px;border-radius:999px;background:#555;color:#fff;font-size:11px;font-weight:600;text-transform:uppercase;letter-spacing:.04em}
    .chat-mode-label{font-size:11px;font-weight:800;text-transform:uppercase;letter-spacing:.08em;color:var(--text-dim);margin-bottom:4px;flex-shrink:0}
    #inputArea.mode-bestie .chat-mode-label{color:rgba(255,122,217,.95)}
    #inputArea.mode-therapist .chat-mode-label{color:rgba(80,200,180,.95)}
    #inputArea.mode-learning .chat-mode-label{color:rgba(255,180,80,.95)}
    #inputArea.mode-ai_tasks .chat-mode-label{color:rgba(140,120,255,.95)}
    #inputArea .input-row{display:flex;align-items:flex-end;gap:10px;width:100%;min-width:0;flex-wrap:nowrap}
    .file-bubbles{display:flex;flex-wrap:wrap;gap:8px;padding:0 0 8px;align-items:center;min-height:0}
    .file-bubbles:empty{display:none}
    .file-bubble{display:inline-flex;align-items:center;gap:6px;padding:6px 10px;border-radius:12px;background:rgba(255,122,217,.12);border:1px solid rgba(255,122,217,.4);font-size:12px;font-weight:600;color:var(--text);max-width:100%;flex-shrink:0;transform-origin:center center}
    .file-bubble.file-bubble-enter{animation:file-bubble-pop .52s cubic-bezier(0.34,1.4,0.64,1) forwards}
    @keyframes file-bubble-pop{0%{opacity:0;transform:scale(0.4) skewX(-4deg)}35%{opacity:1;transform:scale(1.12) skewX(2deg)}55%{transform:scale(0.96) skewX(-1deg)}75%{transform:scale(1.03) skewX(0.5deg)}100%{opacity:1;transform:scale(1) skewX(0)}}
    .file-bubble .file-bubble-thumb{width:32px;height:32px;object-fit:cover;border-radius:8px;flex-shrink:0}
    .file-bubble .file-bubble-icon{font-size:1.2em;line-height:1;flex-shrink:0}
    .file-bubble .file-bubble-label{min-width:0;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
    .file-bubble .file-bubble-remove{width:22px;height:22px;min-width:22px;border:none;border-radius:50%;background:rgba(0,0,0,.3);color:#fff;font-size:14px;line-height:1;cursor:pointer;display:flex;align-items:center;justify-content:center;flex-shrink:0}
    .file-bubble .file-bubble-remove:hover,.file-bubble .file-bubble-remove:active{background:rgba(255,122,217,.5)}
    .file-bubble--image{border-color:rgba(255,122,217,.55);box-shadow:0 0 10px rgba(255,122,217,.35),0 0 4px rgba(255,122,217,.25)}
    .file-bubble--pdf{border-color:rgba(255,159,67,.6);background:rgba(255,159,67,.1);box-shadow:0 0 10px rgba(255,159,67,.4),0 0 4px rgba(255,159,67,.3)}
    .file-bubble--pdf .file-bubble-remove:hover,.file-bubble--pdf .file-bubble-remove:active{background:rgba(255,159,67,.5)}
    .file-bubble--markdown{border-color:rgba(0,255,136,.55);background:rgba(0,255,136,.08);box-shadow:0 0 10px rgba(0,255,136,.35),0 0 4px rgba(0,255,136,.25)}
    .file-bubble--markdown .file-bubble-remove:hover,.file-bubble--markdown .file-bubble-remove:active{background:rgba(0,255,136,.45)}
    .file-bubble--text{border-color:rgba(0,212,255,.55);background:rgba(0,212,255,.08);box-shadow:0 0 10px rgba(0,212,255,.35),0 0 4px rgba(0,212,255,.25)}
    .file-bubble--text .file-bubble-remove:hover,.file-bubble--text .file-bubble-remove:active{background:rgba(0,212,255,.45)}
    .file-bubble--audio{border-color:rgba(168,85,247,.6);background:rgba(168,85,247,.1);box-shadow:0 0 10px rgba(168,85,247,.4),0 0 4px rgba(168,85,247,.3)}
    .file-bubble--audio .file-bubble-remove:hover,.file-bubble--audio .file-bubble-remove:active{background:rgba(168,85,247,.5)}
    .file-bubble--file{border-color:rgba(255,122,217,.4);box-shadow:0 0 8px rgba(255,122,217,.3)}
    .attach-preview-bar{display:flex;align-items:center;gap:12px;padding:10px 12px;padding-left:max(12px,env(safe-area-inset-left));background:rgba(255,122,217,.1);border-bottom:1px solid rgba(255,122,217,.25);flex-shrink:0}
    .attach-preview-bar img{max-width:72px;max-height:72px;width:72px;height:72px;object-fit:cover;border-radius:10px;display:block;border:1px solid var(--border)}
    .attach-preview-bar .attach-preview-close{position:absolute;top:10px;right:max(10px,env(safe-area-inset-right));width:32px;height:32px;min-width:32px;min-height:32px;border-radius:50%;border:none;background:rgba(0,0,0,.4);color:#fff;font-size:20px;line-height:1;cursor:pointer;display:flex;align-items:center;justify-content:center;-webkit-tap-highlight-color:transparent;flex-shrink:0}
    .attach-preview-bar .attach-preview-close:hover,.attach-preview-bar .attach-preview-close:active{background:rgba(255,122,217,.5)}
    .attach-preview-bar .attach-preview-hint{font-size:13px;color:var(--text-dim);flex:1;min-width:0;padding-right:40px}
    #msgInput{flex:1 1 auto;min-width:0;width:100%;box-sizing:border-box;border-radius:22px;border:2px solid var(--border);background:var(--bg);color:var(--text);padding:10px 14px;font-size:16px;line-height:1.4;resize:none;font-family:inherit;outline:none;max-height:130px;min-height:42px;overflow-y:auto;transition:border-color .15s;-webkit-appearance:none;appearance:none}
    #msgInput::placeholder{color:var(--text-dim);opacity:1}
    #msgInput:focus,#msgInput:focus-visible{border-color:var(--pink);outline:2px solid var(--pink);outline-offset:2px}
    #attachImgBtn{width:38px;height:38px;min-width:38px;min-height:38px;border-radius:50%;border:2px solid var(--pink);background:var(--bg);color:var(--pink);cursor:pointer;font-size:19px;flex:0 0 38px;display:flex;align-items:center;justify-content:center;padding:0;-webkit-appearance:none;appearance:none;box-shadow:0 0 0 1px var(--border),0 0 10px rgba(255,122,217,.25);line-height:1;overflow:hidden}
    #attachImgBtn .attach-btn-icon{filter:drop-shadow(0 0 4px var(--pink));transition:filter .15s}
    #attachImgBtn:hover,#attachImgBtn:focus{background:var(--surface2);border-color:var(--pink);box-shadow:0 0 0 1px var(--border),0 0 14px rgba(255,122,217,.5),0 0 24px rgba(255,100,235,.3)}
    #attachImgBtn:hover .attach-btn-icon,#attachImgBtn:focus .attach-btn-icon{filter:drop-shadow(0 0 6px var(--pink)) drop-shadow(0 0 12px rgba(255,122,217,.8))}
    /* Send button: same purple/pink shimmer gradient as pin buttons */
    #sendBtn{-webkit-appearance:none;appearance:none;width:42px;height:42px;min-width:42px;min-height:42px;border-radius:50%;border:2px solid transparent;background:linear-gradient(#06000f,#06000f) padding-box,linear-gradient(135deg,#ff66ee,#ff44e0,#e030ff,#c030e8,#ff88f0,#ff44e0) border-box;background-origin:padding-box,border-box;background-clip:padding-box,border-box;color:#ff99ee;cursor:pointer;font-size:18px;font-weight:900;display:flex;align-items:center;justify-content:center;flex:0 0 42px;padding:0;line-height:1;box-shadow:0 0 8px rgba(255,68,224,.5),0 0 14px rgba(255,80,230,.4),0 0 22px rgba(200,50,240,.3),inset 0 0 10px rgba(255,60,220,.08);transition:opacity .15s,transform .1s,box-shadow .2s,color .15s;overflow:hidden;position:relative}
    #sendBtn .send-btn-icon{width:26px;height:26px;max-width:26px;max-height:26px;display:block;object-fit:contain;flex:0 0 auto;pointer-events:none;filter:drop-shadow(0 0 4px rgba(255,136,238,.7)) drop-shadow(0 0 8px rgba(255,100,240,.5));transition:filter .15s}
    #sendBtn:active{transform:scale(.92)}
    #sendBtn:hover:not(:disabled){color:#fff;box-shadow:0 0 10px rgba(255,136,240,.5),0 0 18px rgba(255,100,238,.45),0 0 30px rgba(255,80,240,.4),0 0 48px rgba(220,50,240,.25),inset 0 0 16px rgba(255,100,245,.1)}
    #sendBtn:hover:not(:disabled) .send-btn-icon{filter:drop-shadow(0 0 6px rgba(255,136,238,.9)) drop-shadow(0 0 12px rgba(255,100,240,.6)) drop-shadow(0 0 4px rgba(255,255,255,.2))}
    #sendBtn:disabled{opacity:.4;cursor:default}
    #sendBtn:focus-visible{outline:2px solid rgba(255,255,255,.9);outline-offset:2px}
    .voice-input-btn,#voiceInputBtn{-webkit-appearance:none;appearance:none;width:36px;height:36px;min-width:36px;min-height:36px;border-radius:50%;border:1px solid var(--locus-border);background:var(--locus-bg);color:var(--pink);cursor:pointer;font-size:18px;display:flex;align-items:center;justify-content:center;flex:0 0 36px;padding:0;transition:opacity .15s,background .15s;overflow:hidden}
    .voice-input-btn:hover,#voiceInputBtn:hover{opacity:.9;background:rgba(255,122,217,.12)}
    .voice-input-btn.listening,#voiceInputBtn.listening{background:rgba(255,122,217,.3);box-shadow:0 0 12px rgba(255,122,217,.4)}
    #attachImgBtn:focus-visible{outline:2px solid var(--pink);outline-offset:2px}
    /* Screenshot: PC only (hidden on iPhone/narrow viewport) — used inside attach menu */
    .screenshot-pc-only{display:none !important}
    @media (min-width: 768px){.screenshot-pc-only{display:flex !important}}
    /* Paperclip menu popover: Bestie/Therapist/Learning + Photos + Files + Screenshot (PC only) */
    #attachMenuPopover{position:absolute;bottom:100%;left:0;margin-bottom:6px;background:var(--surface2);border:2px solid var(--border);border-radius:12px;padding:8px;min-width:180px;box-shadow:0 0 20px rgba(0,0,0,.4),0 0 40px rgba(255,68,224,.15);z-index:40;display:none;flex-direction:column;gap:2px}
    #attachMenuPopover.open{display:flex}
    .attach-menu-item{-webkit-appearance:none;appearance:none;border:none;border-radius:8px;padding:8px 12px;background:transparent;color:var(--text);font-size:13px;font-weight:600;cursor:pointer;text-align:left;transition:background .15s,color .15s;display:flex;align-items:center}
    .attach-menu-item:hover,.attach-menu-item:focus{background:rgba(255,122,217,.15);color:var(--pink)}
    .attach-menu-item.mode-btn.active{background:rgba(255,122,217,.2);color:var(--pink)}
    .attach-menu-sep{height:1px;background:var(--border);margin:4px 0}
    .mode-btn{font-size:12px}
    .attach-menu-roles-wrap{position:relative}
    #rolesSubmenuPopover{position:absolute;left:100%;top:0;margin-left:6px;background:var(--surface2);border:2px solid var(--border);border-radius:12px;padding:8px;min-width:140px;box-shadow:0 0 20px rgba(0,0,0,.4),0 0 40px rgba(255,68,224,.15);z-index:41;display:none;flex-direction:column;gap:2px}
    #rolesSubmenuPopover.open{display:flex}
    @media (max-width: 380px){#rolesSubmenuPopover{left:auto;right:100%;margin-left:0;margin-right:6px}}
    /* â"€â"€ Markdown in bubbles â"€â"€ */
    .bubble h2,.bubble h3,.bubble h4{font-weight:700;margin:.5em 0 .25em;line-height:1.25}
    .bubble h2{font-size:1.1em}.bubble h3{font-size:1em}.bubble h4{font-size:.92em;opacity:.85}
    .bubble strong{font-weight:700}
    .bubble em{font-style:italic}
    .bubble code{font-family:'SF Mono','Fira Code',monospace;font-size:.82em;padding:1px 5px;border-radius:4px;background:rgba(0,0,0,.3)}
    .msg-wrap.user .bubble code{background:rgba(0,0,0,.18)}
    .bubble pre{margin:.4em 0;border-radius:8px;overflow-x:auto}
    .bubble pre code{display:block;padding:10px 12px;background:rgba(0,0,0,.35);font-size:.8em;line-height:1.5;white-space:pre}
    .msg-wrap.user .bubble pre code{background:rgba(0,0,0,.2)}
    .copyable-block{position:relative;margin:.5em 0;border-radius:8px;overflow:hidden;background:rgba(0,0,0,.35);border:1px solid rgba(255,255,255,.1)}
    .copyable-block pre{margin:0}
    .copyable-block .copyable-block-btn{position:absolute;top:6px;right:8px;padding:4px 10px;border-radius:999px;border:none;background:#444;color:#fff;font-size:12px;font-weight:600;cursor:pointer;opacity:.9;transition:opacity .15s}
    .copyable-block .copyable-block-btn:hover,.copyable-block .copyable-block-btn:active{opacity:1}
    .copyable-block .copyable-block-btn.copied{background:var(--pink,#ff7ad9);color:#111}
    .bubble ul,.bubble ol{margin:.4em 0 .4em 1.2em;padding:0}
    .bubble li{margin:.2em 0;line-height:1.45}
    .bubble blockquote{border-left:3px solid rgba(255,122,217,.5);margin:.4em 0;padding:.2em .6em;opacity:.8}
    .msg-wrap.user .bubble blockquote{border-left-color:rgba(0,0,0,.3)}
    .bubble hr{border:none;border-top:1px solid rgba(255,255,255,.2);margin:.4em 0}
    .bubble a{color:var(--pink);text-decoration:underline}
    .msg-wrap.user .bubble a{color:#5a0030}
    .bubble p{margin:.35em 0}.bubble p:first-child{margin-top:0}.bubble p:last-child{margin-bottom:0}
    /* Group chat: sender label above bubble */
    .msg-sender-row{padding:2px 0 0;min-height:0}
    .msg-sender-label{font-size:11px;font-weight:600;text-transform:uppercase;letter-spacing:.05em;color:var(--pink);opacity:.9}
    .group-vote-bar{display:flex;flex-wrap:wrap;align-items:center;gap:10px;padding:10px 12px;margin:0 8px 8px;background:rgba(255,122,217,.12);border:1px solid rgba(255,122,217,.35);border-radius:12px;font-size:13px;color:var(--text)}
    .group-vote-label{flex:1;min-width:0}
    .group-vote-btn{padding:8px 14px;border-radius:12px;border:1px solid var(--pink);background:var(--pink);color:#111;font-size:14px;font-weight:600;cursor:pointer;transition:opacity .15s;-webkit-appearance:none;appearance:none}
    .group-vote-btn:hover,.group-vote-btn:active{opacity:.9}
    .group-vote-btn:disabled{opacity:.6;cursor:default}
    .assistant-plan{margin:.5em 0;border:1px solid rgba(255,122,217,.35);border-radius:8px;background:rgba(0,0,0,.2);overflow:hidden}
    .assistant-plan summary{cursor:pointer;padding:6px 10px;font-size:.9em;font-weight:600;color:var(--pink);list-style:none;display:flex;align-items:center;gap:6px}
    .assistant-plan summary::-webkit-details-marker{display:none}
    .assistant-plan summary:before{content:'\u25B6';font-size:.7em;opacity:.8;transition:transform .2s}
    .assistant-plan[open] summary:before{transform:rotate(90deg)}
    .assistant-plan .bubble-text{padding:8px 10px 10px;font-size:.92em;border-top:1px solid rgba(255,255,255,.08)}
    /* Auto-detect viewport: desktop = sidebar visible; mobile = overlay. Works with Chrome DevTools device emulation. */
    @media (min-width: 768px) {
      #sidebar{transform:translateX(0);width:288px}
      #mainWrap{margin-left:288px;transition:margin-left .25s cubic-bezier(.4,0,.2,1)}
    }
    #app.sidebar-collapsed #sidebar{transform:translateX(-100%)}
    #app.sidebar-collapsed #mainWrap{margin-left:0}
    /* â"€â"€ Tab bar (Chat / Room): fill to screen edge so no black (PWA/home screen safe area) â"€â"€ */
    #tabBar{position:fixed;bottom:0;left:0;right:0;min-height:calc(56px + max(24px,env(safe-area-inset-bottom)));padding-bottom:max(24px,env(safe-area-inset-bottom));padding-left:env(safe-area-inset-left);padding-right:env(safe-area-inset-right);background:var(--surface);border-top:1px solid var(--border);display:flex;align-items:stretch;z-index:30;box-sizing:border-box;-webkit-background-clip:padding-box;background-clip:padding-box}
    #tabBar::before{content:'';position:absolute;left:0;right:0;bottom:0;height:max(34px,env(safe-area-inset-bottom));background:var(--surface);z-index:-1;pointer-events:none}
    #tabBar::after{content:'';position:absolute;top:100%;left:0;right:0;height:80px;background:var(--surface);z-index:-1;pointer-events:none}
    #tabBar .tab{flex:1;border:none;background:none;color:var(--text-dim);font-size:14px;font-weight:600;cursor:pointer;display:flex;align-items:center;justify-content:center;gap:6px;transition:color .15s;-webkit-tap-highlight-color:transparent}
    #tabBar .tab:hover,#tabBar .tab:active{color:var(--text)}
    #tabBar .tab.active{color:var(--pink)}
    #tabBar .tab-indicator{position:absolute;bottom:0;left:0;height:3px;background:var(--pink);border-radius:3px 3px 0 0;transition:left .25s cubic-bezier(.4,0,.2,1),width .25s cubic-bezier(.4,0,.2,1);pointer-events:none}
    /* â"€â"€ Room tab disabled: hide only Room tab and room panel; keep Chat and Social visible â"€â"€ */
    #app.room-tab-disabled #tabBar .tab[data-tab="room"]{display:none}
    #app.room-tab-disabled #roomPanel{display:none !important}
    #app.room-tab-disabled #mainWrap{padding-bottom:calc(72px + max(28px,env(safe-area-inset-bottom)))}
    /* â"€â"€ Room panel (Claudia bedroom) â"€â"€ */
    #roomPanel{display:none;position:fixed;top:0;left:0;right:0;bottom:calc(56px + max(20px,env(safe-area-inset-bottom)));z-index:25;background:var(--bg)}
    #roomPanel.visible{display:block}
    #app.show-room #mainWrap{display:none}
    #app.show-room #roomPanel{display:block}
    /* â"€â"€ Right sidebar: working doc (Claude-style temp file), long paste, downloadable â"€â"€ */
    #rightSidebar{position:fixed;top:0;right:0;bottom:0;width:min(400px,92vw);max-width:100%;background:var(--surface);border-left:2px solid rgba(255,122,217,.4);box-shadow:-4px 0 20px rgba(0,0,0,.3);z-index:35;display:flex;flex-direction:column;overflow:hidden;transform:translateX(100%);transition:transform .25s cubic-bezier(.4,0,.2,1);padding-top:max(12px,env(safe-area-inset-top));padding-bottom:max(12px,env(safe-area-inset-bottom));padding-left:12px;padding-right:12px;box-sizing:border-box}
    #app.right-sidebar-open #rightSidebar{transform:translateX(0)}
    #rightSidebar .working-doc-hdr{display:flex;align-items:center;justify-content:space-between;flex-shrink:0;padding:8px 0 10px;border-bottom:1px solid var(--border);margin-bottom:10px}
    #rightSidebar .working-doc-hdr h2{font-size:15px;font-weight:700;color:var(--pink,#ff7ad9);margin:0}
    #rightSidebar .working-doc-close{background:none;border:none;color:var(--text-dim);font-size:22px;cursor:pointer;padding:4px 8px;line-height:1;-webkit-tap-highlight-color:transparent}
    #rightSidebar .working-doc-close:hover,#rightSidebar .working-doc-close:active{color:var(--text)}
    #rightSidebar .working-doc-body{flex:1;min-height:0;display:flex;flex-direction:column}
    #rightSidebar #workingDocContent{flex:1;min-height:120px;width:100%;padding:12px;border-radius:10px;border:1px solid var(--border);background:var(--bg);color:var(--text);font-family:inherit;font-size:14px;line-height:1.5;resize:none;box-sizing:border-box;-webkit-tap-highlight-color:transparent}
    #rightSidebar #workingDocContent::placeholder{color:var(--text-dim)}
    #rightSidebar .working-doc-actions{display:flex;gap:10px;flex-shrink:0;margin-top:10px}
    #rightSidebar .working-doc-download{flex-shrink:0;padding:10px 16px;border-radius:10px;border:none;background:rgba(255,122,217,.25);color:var(--pink,#ff7ad9);font-weight:600;font-size:14px;cursor:pointer;-webkit-tap-highlight-color:transparent}
    #rightSidebar .working-doc-download:hover,#rightSidebar .working-doc-download:active{background:rgba(255,122,217,.4)}
    #workingDocBtn{flex-shrink:0;padding:8px 12px;border-radius:8px;border:1px solid rgba(255,122,217,.35);background:rgba(255,122,217,.12);color:var(--pink,#ff7ad9);font-size:13px;font-weight:600;cursor:pointer;-webkit-tap-highlight-color:transparent}
    #workingDocBtn:hover,#workingDocBtn:active{background:rgba(255,122,217,.25)}
    #bedroomScene{position:absolute;inset:0;overflow:hidden;display:flex;align-items:center;justify-content:center;background:#1a1510}
    #bedroomScene img{max-width:100%;max-height:100%;width:auto;height:auto;object-fit:contain;display:block}
    #bedroomScene .scene-inner{position:relative;width:100%;height:100%;max-width:100%;max-height:100%}
    #bedroomScene .scene-inner img{position:absolute;inset:0;width:100%;height:100%;object-fit:contain}
    #bedroomScene .scene-inner{position:relative;width:100%;height:100%}
    #mainWrap{flex:1;min-height:0;display:flex;flex-direction:column;overflow:hidden;padding-bottom:calc(72px + max(28px,env(safe-area-inset-bottom)))}
    #claudiaSprite{position:absolute;width:28px;height:36px;left:40%;top:55%;margin-left:-14px;margin-top:-18px;pointer-events:none;z-index:10;transition:left 2.5s linear, top 2.5s linear}
    #claudiaSprite #activityBubble{left:50%;top:0;transform:translate(-50%,-100%);position:absolute;white-space:nowrap}
    #claudiaSprite.walk{transition-duration:2.5s}
    #claudiaSprite.idle{}
    #claudiaSprite.sleep{opacity:.9}
    #activityBubble{position:absolute;left:50%;top:0;transform:translate(-50%,-100%);padding:4px 10px;border-radius:12px;background:rgba(20,10,30,.92);border:1px solid var(--border);font-size:18px;white-space:nowrap;z-index:11;opacity:0;pointer-events:none;transition:opacity .2s}
    #activityBubble.show{opacity:1;animation:activity-pop .4s ease-out}
    @keyframes activity-pop{0%{transform:translate(-50%,-100%) scale(0.8)}60%{transform:translate(-50%,-100%) scale(1.05)}100%{transform:translate(-50%,-100%) scale(1)}}
    #activityBubble.zzz{animation:zzz-float 1.5s ease-in-out infinite}
    @keyframes zzz-float{0%,100%{transform:translate(-50%,-100%) translateY(0)}50%{transform:translate(-50%,-100%) translateY(-6px)}}
    .pwa-version{position:fixed;bottom:calc(env(safe-area-inset-bottom) + 2px);right:8px;font-size:10px;color:var(--text-dim);opacity:.85;z-index:5;pointer-events:none;}
  </style>
  <script src="https://cdn.jsdelivr.net/npm/marked@9/marked.min.js"></script>
  <script>/* Ensure chat list visible on desktop */ document.addEventListener('DOMContentLoaded',function(){var m=window.matchMedia&&window.matchMedia('(min-width: 768px)');if(m&&m.matches){var app=document.getElementById('app');if(app)app.classList.remove('sidebar-collapsed');}});</script>
</head>
<body>
<div id="app" class="room-tab-disabled">
  <div id="sideOverlay"></div>
  <div id="sidebar">
    <div class="sb-head">
      <span class="sb-head-title">Chats</span>
      <button class="sb-new" id="sbNew">+ New</button>
    </div>
    <div class="sb-top-buttons">
      <button type="button" id="sbUserBtn" class="sb-top-btn" aria-expanded="false" aria-controls="sbUserPanel" title="User, avatar, sign in">User</button>
""" + ('' if GAMES_OFF else ('<a href="/dashboard" class="sb-top-btn" title="Journal, quick facts, reminders">Dashboard</a>' if not DASHBOARD_OFF else '<a href="/dashboard/angel-demon" class="sb-top-btn" title="Angel/demon collector game">Angels &amp; Demons</a>')) + """
      <a href="/files" class="sb-top-btn" title="Browse project files">Files</a>
    </div>
    <div id="sbUserPanel" class="sb-user-panel" role="region" aria-label="User settings">
      <div class="sb-user-panel-inner">
        <div id="sbWorkspace" class="sb-workspace"><strong>Locus</strong> Â· same workspace as your PC.</div>
        <div class="sb-user-row" id="sbUserRow">
          <label for="userSelect" class="sb-user-label">Your name:</label>
          <input type="text" id="userSelect" class="sb-user-select" placeholder="enter your name…" autocomplete="off" maxlength="40" spellcheck="false">
        </div>
        <div class="sb-avatar-picker-wrap" id="avatarPickerWrap">
          <div class="sb-avatar-picker-label">Your look</div>
          <div id="avatarPicker" class="sb-avatar-picker" role="listbox" aria-label="Pick your avatar"></div>
          <div class="sb-avatar-custom-wrap" style="margin-top:6px">
            <input type="url" id="avatarCustomUrl" class="sb-avatar-custom-input" placeholder="Or paste image URL..." autocomplete="off">
            <button type="button" id="avatarCustomBtn" class="sb-avatar-custom-btn">Use</button>
          </div>
        </div>
        <div id="userProfileWrap" class="sb-user-profile-wrap" style="display:none">
          <div class="sb-user-profile-label">Pronouns &amp; about you (for Claudia)</div>
          <input type="text" id="userProfilePronouns" class="sb-user-profile-input" placeholder="e.g. she/her" maxlength="80" aria-label="Pronouns">
          <textarea id="userProfileAbout" class="sb-user-profile-textarea" placeholder="Short about you (e.g. Lynn, Ruby&#39;s mom, likes gardening)" rows="2" maxlength="500" aria-label="About you"></textarea>
          <button type="button" id="userProfileSave" class="sb-user-profile-save">Save</button>
        </div>
        <div class="sb-refresh-row" style="padding:10px 12px 8px;border-top:1px solid var(--border);flex-shrink:0">
          <button type="button" id="sbRefreshBtn" class="sb-refresh-btn" title="Refresh chat list" aria-label="Refresh chat list" style="width:100%;padding:8px 12px;font-size:13px;font-weight:600;color:var(--pink,#ff7ad9);background:rgba(255,122,217,.12);border:1px solid rgba(255,122,217,.3);border-radius:8px;cursor:pointer">Refresh chat list</button>
        </div>
      </div>
    </div>
    <div class="sb-search-row" style="display:flex;align-items:center;gap:6px;padding:4px 8px 0">
      <div class="sb-search" style="flex:1"><input type="text" id="sbSearch" placeholder="Search or ⭐ for starred" autocomplete="off"></div>
      <button type="button" id="semanticToggle" title="Semantic search via Chimera (vector search)" onclick="toggleSemanticSearch(this)" style="flex-shrink:0;background:none;border:1px solid var(--border);border-radius:8px;color:#888;font-size:14px;padding:4px 7px;cursor:pointer;transition:all .2s;line-height:1">✨</button>
    </div>
    <div class="sb-list" id="sbList"></div>
    <div id="sbActionsOverlay" class="sb-actions-overlay" aria-hidden="true"><div class="sb-actions-overlay-backdrop" aria-hidden="true"></div><div class="sb-actions-four-float"></div></div>
  </div>
  <div id="mainWrap">
  <div id="hdr">
    <button id="menuBtn">&#9776;</button>
    <div id="hdrInfo"><div id="hdrName">Porcelain &#9825;</div><div id="hdrSub">your ai bestie</div></div>
    <div class="hdr-actions">
      <button type="button" id="workingDocBtn" title="Working doc (paste long text or save replies here)">&#128196; Doc</button>
      <button type="button" class="sb-new" id="headerNewBtn" title="New chat">+ New</button>
    </div>
  </div>
  <div id="chatArea">
    <div id="typing" role="status" aria-label="Typing...">
      <div class="typing-av"><img src="/locus_avatar.svg" alt="" style="width:40px;height:40px;max-width:40px;max-height:40px;object-fit:contain;display:block" onerror="this.src='/chat_icon.png'; this.onerror=function(){this.src='/icon.svg';}"></div>
      <div class="typing-bubble" title="Thinking... To see reasoning in a reply, try asking &quot;show your thinking&quot; or &quot;how did you get there?&quot;">
        <div class="dot"></div><div class="dot"></div><div class="dot"></div>
      </div>
    </div>
    <div id="generatingImage" aria-live="polite" aria-label="Image generating">
      <div class="generating-av"><img src="/locus_avatar.svg" alt="" style="width:40px;height:40px;max-width:40px;max-height:40px;object-fit:contain;display:block" onerror="this.src='/chat_icon.png'; this.onerror=function(){this.src='/icon.svg';}"></div>
      <div class="generating-placeholder">
        <div class="generating-wave-wrap"><div class="generating-wave"></div></div>
        <div class="generating-text">Still generating, almost there (expect to finish within 60s likely)</div>
      </div>
    </div>
  </div>
  <div id="threadSwitcher" class="thread-switcher" style="display:none" role="navigation" aria-label="Switch thread">
    <button type="button" class="thread-switcher-prev" aria-label="Previous thread">&#8249;</button>
    <span class="thread-switcher-label">1/2</span>
    <button type="button" class="thread-switcher-next" aria-label="Next thread">&#8250;</button>
  </div>
  <div id="engagementBanner" class="engagement-banner" aria-live="polite" role="region" aria-label="Mark chat important" style="display:none">
    <span class="engagement-banner-text"></span>
    <button type="button" class="engagement-banner-yes">Yes</button>
    <button type="button" class="engagement-banner-no">No</button>
  </div>
  <div id="roHint"></div>
  <div id="inputArea" class="mode-bestie" data-mode="bestie" role="region" aria-label="Compose message">
    <div id="chatModeLabel" class="chat-mode-label" aria-live="polite">Bestie</div>
    <div id="fileBubbles" class="file-bubbles" aria-label="Attached files"></div>
    <div id="attachMenuPopover" class="attach-menu-popover" role="menu" aria-label="Attach and options">
      <div class="attach-menu-roles-wrap">
        <button type="button" role="menuitem" class="attach-menu-item" data-action="roles" id="rolesMenuBtn" aria-haspopup="true" aria-expanded="false">Roles</button>
        <div id="rolesSubmenuPopover" class="attach-menu-subpopover" role="menu" aria-label="Chat role">
          <button type="button" role="menuitem" class="attach-menu-item mode-btn active" data-mode="bestie">Bestie</button>
          <button type="button" role="menuitem" class="attach-menu-item mode-btn" data-mode="therapist">Therapist</button>
          <button type="button" role="menuitem" class="attach-menu-item mode-btn" data-mode="learning">Learning</button>
          <button type="button" role="menuitem" class="attach-menu-item mode-btn" data-mode="ai_tasks">Go-to</button>
        </div>
      </div>
      <div class="attach-menu-sep"></div>
      <button type="button" role="menuitem" class="attach-menu-item" data-action="photos">Photos</button>
      <button type="button" role="menuitem" class="attach-menu-item" data-action="files">Files</button>
      <button type="button" role="menuitem" class="attach-menu-item screenshot-pc-only" data-action="screenshot">Screenshot</button>
      <div class="attach-menu-sep"></div>
      <button type="button" role="menuitem" class="attach-menu-item" data-action="search">Search in chat</button>
      <button type="button" role="menuitem" class="attach-menu-item" data-action="copy">Copy conversation</button>
      <button type="button" role="menuitem" class="attach-menu-item" data-action="export">Export as Markdown</button>
    </div>
    <div class="input-row">
      <input type="file" id="imgFile" accept="image/*" style="display:none">
      <input type="file" id="fileInput" accept=".pdf,.txt,.md,.json,.csv,.log,.html,.xml,.jsonl" style="display:none">
      <button type="button" id="attachImgBtn" title="Attach and options" aria-label="Attach and options" aria-haspopup="true" aria-expanded="false"><svg class="attach-btn-icon" viewBox="0 0 24 24" xmlns="http://www.w3.org/2000/svg" width="20" height="20" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M21.44 11.05l-9.19 9.19a6 6 0 0 1-8.49-8.49l9.19-9.19a4 4 0 0 1 5.66 5.66l-9.2 9.19a2 2 0 0 1-2.83-2.83l8.49-8.48"/></svg></button>
      <textarea id="msgInput" rows="1" placeholder="Message..." autocomplete="off" autocorrect="on" autocapitalize="sentences"></textarea>
      <div id="contextIndicator" class="context-indicator" aria-hidden="true" role="img" aria-label="Context length" title="Message length">
        <svg class="context-indicator-svg" viewBox="0 0 32 32" width="24" height="24">
          <circle class="context-indicator-track" cx="16" cy="16" r="14" fill="none" stroke="currentColor" stroke-width="2"/>
          <circle id="contextIndicatorArc" class="context-indicator-arc" cx="16" cy="16" r="14" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-dasharray="88" stroke-dashoffset="88" transform="rotate(-90 16 16)"/>
        </svg>
      </div>
      <button type="button" id="voiceInputBtn" class="voice-input-btn" title="Voice input (speak to type)" aria-label="Voice input">&#127908;</button>
      <button type="button" id="sendBtn" title="Send" aria-label="Send"><img src="/bee.svg" alt="" width="32" height="32" class="send-btn-icon" aria-hidden="true"></button>
    </div>
  </div>
  </div>
  <div id="rightSidebar" role="complementary" aria-label="Working doc" aria-hidden="true">
    <div class="working-doc-hdr">
      <h2>Working doc</h2>
      <button type="button" class="working-doc-close" id="rightSidebarClose" aria-label="Close working doc">&times;</button>
    </div>
    <div class="working-doc-body">
      <textarea id="workingDocContent" placeholder="Paste long text here, or open a reply from Claudia. Edits here are sent with your message."></textarea>
      <div class="working-doc-actions">
        <button type="button" class="working-doc-download" id="workingDocDownload" style="display:none">Download</button>
      </div>
    </div>
  </div>
  <div id="activityBreakdownOverlay" class="activity-breakdown-overlay" aria-hidden="true" inert>
    <div class="activity-breakdown-backdrop"></div>
    <div class="activity-breakdown-card">
      <h3 class="activity-breakdown-title"></h3>
      <div class="activity-buckets" role="list"></div>
      <button type="button" class="activity-breakdown-close" aria-label="Close">Ã—</button>
    </div>
  </div>
  <div id="roomPanel" aria-hidden="true">
    <div id="bedroomScene">
      <div class="scene-inner">
        <img id="bedroomBg" src="/bedroom_bg.png" alt="Claudia&#39;s Room">
        <div id="claudiaSprite" class="idle"><svg viewBox="0 0 28 36" xmlns="http://www.w3.org/2000/svg" width="28" height="36"><ellipse cx="14" cy="10" rx="8" ry="9" fill="#f5d0e0"/><path d="M8 20 L14 32 L20 20 Z" fill="#e8b8d0"/><circle cx="10" cy="9" r="1.5" fill="#333"/><circle cx="18" cy="9" r="1.5" fill="#333"/></svg><div id="activityBubble" aria-live="polite"></div></div>
      </div>
    </div>
  </div>
  <div id="tabBar" role="tablist">
    <span id="tabIndicator" class="tab-indicator" aria-hidden="true"></span>
    <button type="button" class="tab active" data-tab="chat" role="tab" aria-selected="true"> Chat</button>
  </div>
  <div id="pwaVersion" class="pwa-version" aria-hidden="true">v39 __BUILD_ID__</div>
    <script src="/web_app.js?v=39__BUILD_ID__"></script>
</body></html>
    """
    build_id = str(int(_WEB_APP_JS_PATH.stat().st_mtime)) if _WEB_APP_JS_PATH.exists() else "0"
    html = html.replace("__BUILD_ID__", build_id)
    return Response(content=html, media_type="text/html; charset=utf-8", headers={"Cache-Control": "no-store, no-cache, must-revalidate", "Pragma": "no-cache", "Expires": "0"})


def _ollama_running() -> bool:
    """True if Ollama API responds on 11434."""
    try:
        import urllib.request
        req = urllib.request.Request("http://localhost:11434/api/tags", method="GET")
        with urllib.request.urlopen(req, timeout=2) as _:
            return True
    except Exception:
        return False


def _ensure_ollama() -> None:
    """Legacy no-op: Chimera owns model routing and fallback startup."""
    print("  Ollama: skipped (Chimera owns model routing)")

def _start_discord_bot_in_background() -> None:
    """Start Discord Claudia bot in a daemon thread so one process runs chat + Discord. Skipped when DISCORD_OFF."""
    if DISCORD_OFF:
        return
    if not os.environ.get("DISCORD_BOT_TOKEN", "").strip():
        print("  Discord: skipped (set DISCORD_BOT_TOKEN to enable)")
        return
    import threading
    try:
        import discord_claudia_bot
    except ImportError as e:
        print("  Discord: skipped (discord.py not installed:", e, ")")
        return
    thread = threading.Thread(target=lambda: discord_claudia_bot.main(), daemon=True)
    thread.start()
    print("  Discord: bot starting in background (approval in this terminal)")


if __name__ == "__main__":
    import uvicorn
    if sys.platform == "win32" and hasattr(asyncio, "WindowsSelectorEventLoopPolicy"):
        # Avoid noisy Proactor SSL disconnect tracebacks on Windows when browsers
        # close HTTPS connections abruptly (common with PWA/devtools probes).
        asyncio.set_event_loop_policy(asyncio.WindowsSelectorEventLoopPolicy())
    port = 11435
    ssl_keyfile = os.environ.get("LOCUS_SSL_KEYFILE", "").strip()
    ssl_certfile = os.environ.get("LOCUS_SSL_CERTFILE", "").strip()
    if ssl_keyfile:
        ssl_keyfile = Path(ssl_keyfile).resolve()
    if ssl_certfile:
        ssl_certfile = Path(ssl_certfile).resolve()
    use_ssl = bool(ssl_keyfile and ssl_certfile and ssl_keyfile.is_file() and ssl_certfile.is_file())
    if (os.environ.get("LOCUS_SSL_KEYFILE") or os.environ.get("LOCUS_SSL_CERTFILE")) and not use_ssl:
        print("  HTTPS skipped: cert files not found (env is set). Run: python Scripts/generate_self_signed_cert.py")
    scheme = "https" if use_ssl else "http"
    print("Locus online (creative workspace server)" + ("" if DISCORD_OFF else " + Discord"))
    print(f"  PWA HTML + JS from: {_WEB_APP_JS_PATH.resolve()}")
    if use_ssl:
        print(f"  HTTPS: {ssl_certfile} (self-signed — browser will show warning once)")
    _ensure_ollama()
    _start_discord_bot_in_background()
    print(f"  {scheme}://localhost:{port}/web      -> PWA chat (iPhone: Add to Home Screen)")
    if not DASHBOARD_OFF and not GAMES_OFF:
        print(f"  {scheme}://localhost:{port}/dashboard -> journal, quick facts, reminders (same port)")
    try:
        _s = __import__("socket").socket(__import__("socket").AF_INET, __import__("socket").SOCK_DGRAM)
        _s.connect(("8.8.8.8", 80))
        _lan_ip = _s.getsockname()[0]
        _s.close()
    except Exception:
        _lan_ip = None
    if _lan_ip and _lan_ip != "127.0.0.1":
        print(f"  On iPhone (same Wi-Fi): {scheme}://{_lan_ip}:{port}/web")
    else:
        print(f"  On iPhone (same Wi-Fi): use {scheme}://<this-PC-IP>:{port}/web")
    if use_ssl:
        uvicorn.run(app, host="0.0.0.0", port=port, ssl_keyfile=str(ssl_keyfile), ssl_certfile=str(ssl_certfile))
    else:
        uvicorn.run(app, host="0.0.0.0", port=port)

