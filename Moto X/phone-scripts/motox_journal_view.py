"""Safe, phone-friendly rendering for deterministic Moto X journal Markdown."""

from __future__ import annotations

import html
import re
from datetime import datetime
from urllib.parse import quote, urlencode


HEADING_RE = re.compile(r"^##\s+(.+?)(?:\s+·\s+([^·]+))?$")
ANCHOR_RE = re.compile(r'^<a id="([^"]+)"></a>$')
TIME_RE = re.compile(r"^\*\*([^*]+)\*\*$")
SPEAKER_RE = re.compile(r"^\*\*([^*]+):\*\*\s*(.*)$")
AUDIO_RE = re.compile(r"^\[Audio\]\(\.\./audio/([^/)]+)\)$")
AMBIENT_RE = re.compile(r"^<!--\s*(.+?)\s*-->$")


def parse_journal(markdown: str) -> list[dict]:
    """Parse the small, deterministic Markdown grammar emitted by MotoXStore."""
    conversations: list[dict] = []
    current: dict | None = None
    pending_time = ""

    for raw_line in markdown.splitlines():
        line = raw_line.strip()
        if not line or line.startswith("# "):
            continue

        heading = HEADING_RE.fullmatch(line)
        if heading:
            current = {
                "time_range": heading.group(1).strip(),
                "status": (heading.group(2) or "").strip(),
                "anchor": "",
                "turns": [],
                "notes": [],
            }
            conversations.append(current)
            pending_time = ""
            continue
        if current is None:
            continue

        anchor = ANCHOR_RE.fullmatch(line)
        if anchor:
            current["anchor"] = anchor.group(1)
            continue

        speaker = SPEAKER_RE.fullmatch(line)
        if speaker:
            current["turns"].append(
                {
                    "time": pending_time,
                    "speaker": speaker.group(1).strip(),
                    "text": speaker.group(2).strip(),
                    "audio": "",
                }
            )
            pending_time = ""
            continue

        timestamp = TIME_RE.fullmatch(line)
        if timestamp:
            pending_time = timestamp.group(1).strip()
            continue

        audio = AUDIO_RE.fullmatch(line)
        if audio and current["turns"]:
            current["turns"][-1]["audio"] = audio.group(1)
            continue

        ambient = AMBIENT_RE.fullmatch(line)
        if ambient:
            note = ambient.group(1).strip()
            if not note.startswith("Generated from "):
                current["notes"].append(note)
            continue

        # Preserve unexpected/multiline transcript text instead of dropping it.
        if pending_time:
            current["turns"].append(
                {
                    "time": pending_time,
                    "speaker": "Unsorted",
                    "text": line,
                    "audio": "",
                }
            )
            pending_time = ""
        elif current["turns"]:
            current["turns"][-1]["text"] = (
                current["turns"][-1]["text"] + "\n" + line
            ).strip()

    return conversations


def _speaker_class(speaker: str) -> str:
    normalized = re.sub(r"[^a-z0-9]+", "-", speaker.lower()).strip("-")
    return normalized or "unknown"


def _render_bubble(
    *,
    speaker: str,
    source: str,
    timestamp: str,
    text: str,
    audio_filename: str,
    audio_start: float | None = None,
    audio_end: float | None = None,
    capture_id: str | None = None,
) -> str:
    identity_state = ""
    if source in {"confirmed", "predicted"}:
        state_text = "confirmed" if source == "confirmed" else "AI identified"
        identity_state = f'<span class="identity-state">{state_text}</span>'
    audio = ""
    if audio_filename:
        filename = quote(audio_filename, safe="")
        summary = "Play source clip"
        range_attributes = ""
        if audio_start is not None and audio_end is not None:
            summary = "Play turn"
            range_attributes = (
                f' data-audio-start="{max(0.0, audio_start):.3f}"'
                f' data-audio-end="{max(audio_start, audio_end):.3f}"'
            )
        audio = (
            '<div class="clip">'
            f'<button class="clip-play" type="button" data-audio-src="/api/motox/journal-audio/{filename}"'
            f'{range_attributes}>▶ {summary}</button>'
            '<div class="clip-slot" hidden></div></div>'
        )
    speaker_markup = f'<strong>{html.escape(speaker)}</strong>'
    if capture_id:
        parameters: dict[str, str] = {"capture": capture_id}
        if audio_start is not None and audio_end is not None:
            parameters["start"] = f"{max(0.0, audio_start):.3f}"
            parameters["end"] = f"{max(audio_start, audio_end):.3f}"
        edit_href = html.escape(f"/review?{urlencode(parameters)}", quote=True)
        speaker_markup = (
            f'<a class="speaker-edit" href="{edit_href}" '
            f'aria-label="Teach Claudia about {html.escape(speaker, quote=True)}">'
            f'{html.escape(speaker)}</a>'
        )
    return (
        f'<article class="turn speaker-{_speaker_class(speaker)}">'
        '<div class="turn-meta">'
        f'{speaker_markup}{identity_state}'
        f'<time>{html.escape(timestamp)}</time>'
        "</div>"
        f'<p>{html.escape(text).replace(chr(10), "<br>")}</p>'
        f'<div class="turn-actions">{audio}</div></article>'
    )


def render_journal_html(
    markdown: str,
    day: str,
    speaker_layers: dict[str, dict] | None = None,
    *,
    source_path: str | None = None,
    page: int = 1,
    page_size: int | None = None,
    page_path: str | None = None,
) -> str:
    conversations = parse_journal(markdown)
    speaker_layers = speaker_layers or {}
    total_conversations = len(conversations)
    safe_page = max(1, int(page))
    total_pages = 1
    if page_size:
        safe_size = max(1, int(page_size))
        total_pages = max(1, (total_conversations + safe_size - 1) // safe_size)
        safe_page = min(safe_page, total_pages)
        end = max(0, total_conversations - (safe_page - 1) * safe_size)
        start = max(0, end - safe_size)
        conversations = conversations[start:end]
    try:
        parsed_day = datetime.strptime(day, "%Y-%m-%d")
        display_day = parsed_day.strftime("%A, %B %d").replace(" 0", " ")
    except ValueError:
        display_day = day

    cards: list[str] = []
    turn_count = 0
    enriched_count = 0
    for index, conversation in enumerate(conversations):
        turns: list[str] = []
        for turn in conversation["turns"]:
            layer = speaker_layers.get(turn["audio"], {})
            capture_id = layer.get("capture_id")
            if not capture_id:
                capture_match = re.match(r"^(.+)_speech\.[^.]+$", turn["audio"])
                capture_id = capture_match.group(1) if capture_match else None
            if layer:
                enriched_count += 1
            identified = list(layer.get("speakers") or [])
            segments = list(layer.get("segments") or [])
            if segments:
                for segment in segments:
                    turn_count += 1
                    offset = float(segment["audio_start_seconds"])
                    timestamp = f'{turn["time"]} · +{offset:.1f}s'
                    turns.append(
                        _render_bubble(
                            speaker=segment["speaker"],
                            source=segment.get("source", "unassigned"),
                            timestamp=timestamp,
                            text=segment["text"],
                            audio_filename=turn["audio"],
                            audio_start=offset,
                            audio_end=float(segment["audio_end_seconds"]),
                            capture_id=capture_id,
                        )
                    )
                continue

            turn_count += 1
            display_speaker = (
                identified[0]
                if len(identified) == 1 and (
                    not layer.get("duration_seconds")
                    or sum(
                        max(0.0, float(item["audio_end_seconds"]) - float(item["audio_start_seconds"]))
                        for item in layer.get("turns", [])
                        if item.get("speaker") == identified[0]
                    ) / float(layer["duration_seconds"]) >= 0.65
                )
                else "Unsorted" if identified else turn["speaker"]
            )
            turns.append(
                _render_bubble(
                    speaker=display_speaker,
                    source=(
                        layer.get("source", "unassigned")
                        if len(identified) == 1 and display_speaker == identified[0]
                        else "unassigned"
                    ),
                    timestamp=turn["time"],
                    text=turn["text"],
                    audio_filename=turn["audio"],
                    capture_id=capture_id,
                )
            )

        notes = "".join(
            f'<div class="context-note">✦ {html.escape(note)}</div>'
            for note in conversation["notes"]
        )
        anchor = html.escape(conversation["anchor"] or f"conversation-{index}", quote=True)
        raw_status = conversation["status"]
        status = html.escape(
            "recording now" if raw_status == "active"
            else "" if raw_status == "completed"
            else raw_status
        )
        status_badge = f'<span class="status">{status}</span>' if status else ""
        cards.append(
            f'<section class="conversation" id="{anchor}">'
            '<header class="conversation-head">'
            f'<h2>{html.escape(conversation["time_range"])}</h2>{status_badge}'
            "</header>"
            f'{"".join(turns)}{notes}</section>'
        )

    if cards:
        body = "".join(cards)
    else:
        body = (
            '<section class="empty"><div class="empty-spark">✦</div>'
            "<h2>Quiet so far</h2><p>Today’s conversations will appear here.</p></section>"
        )

    source_path = source_path or f"/api/motox/journal/{quote(day, safe='')}"
    pagination = ""
    if page_size and total_pages > 1:
        base = html.escape(page_path or "", quote=True)
        newer = (
            f'<a href="{base}?page={safe_page - 1}">← Newer</a>'
            if safe_page > 1 else "<span></span>"
        )
        older = (
            f'<a href="{base}?page={safe_page + 1}">Older →</a>'
            if safe_page < total_pages else "<span></span>"
        )
        pagination = (
            f'<nav class="journal-pages">{newer}'
            f'<span>Page {safe_page} of {total_pages}</span>{older}</nav>'
        )
    conversation_summary = (
        f"{total_conversations} conversations · showing {len(conversations)}"
        if page_size and total_conversations > len(conversations)
        else f"{total_conversations} conversations"
    )
    return f"""<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width,initial-scale=1,viewport-fit=cover">
  <meta name="theme-color" content="#000000">
  <meta name="color-scheme" content="dark">
  <meta name="apple-mobile-web-app-capable" content="yes">
  <meta name="apple-mobile-web-app-status-bar-style" content="black-translucent">
  <link rel="manifest" href="/dashboard-assets/manifest.webmanifest">
  <link rel="stylesheet" href="/dashboard-assets/journal.css?v=20260906-speaker-link">
  <title>Today’s journal · Claudia</title>
</head>
<body>
  <main>
    <nav><a class="back" href="/dashboard" aria-label="Back to Claudia">‹</a><span>MOTO X MEMORY</span></nav>
    <header class="page-head">
      <div><p class="eyebrow">TODAY’S JOURNAL</p><h1>{html.escape(display_day)}</h1></div>
      <a class="teach" href="/review">teach Claudia</a>
    </header>
    <p class="summary">{conversation_summary} · {turn_count} moments · {enriched_count} speaker-enriched clips</p>
    <div class="journal">{body}</div>
    {pagination}
    <footer><a href="{html.escape(source_path, quote=True)}">view source Markdown</a></footer>
  </main>
  <script>
    document.addEventListener('click', event => {{
      const button = event.target.closest('.clip-play');
      if (!button) return;
      const slot = button.nextElementSibling;
      const player = document.createElement('audio');
      player.controls = true;
      player.preload = 'metadata';
      player.src = button.dataset.audioSrc;
      const start = Number(button.dataset.audioStart);
      const end = Number(button.dataset.audioEnd);
      if (Number.isFinite(start)) {{
        player.addEventListener('loadedmetadata', () => {{ player.currentTime = start; }}, {{once: true}});
      }}
      if (Number.isFinite(end)) {{
        player.addEventListener('timeupdate', () => {{
          if (player.currentTime >= end) player.pause();
        }});
      }}
      slot.appendChild(player);
      slot.hidden = false;
      button.hidden = true;
      player.play().catch(() => {{}});
    }});
  </script>
</body>
</html>"""
